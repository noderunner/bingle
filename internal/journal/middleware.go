package journal

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/noderunner/bingle/internal/httpstat"
	"github.com/noderunner/bingle/internal/logging"
	"github.com/noderunner/bingle/internal/trace"
)

// Options are what the middleware needs.
type Options struct {
	// DB is where rows are written. Nil writes none, and the WARN line is
	// still logged — which is the whole journal for a server run with
	// BINGLE_METRICS_DB empty.
	DB *DB
	// Logger is the server's logger; nil means slog's default.
	Logger *slog.Logger
	// Slower is how slow a request has to be before it is written down. Zero
	// turns the journal off entirely, including the collector, so a request
	// pays nothing at all.
	Slower time.Duration
}

// writeTimeout bounds the insert. The response has already gone out, so this
// only decides how long the handler's goroutine lingers.
const writeTimeout = 5 * time.Second

// Middleware collects a request's spans and, when the request turns out to be
// worth it, writes them down.
//
// Worth it means: it took longer than Options.Slower, or the server failed it
// (5xx). Everything else is thrown away when the request finishes — which is
// the point. Keeping every request would be a trace store, and bingle has
// nowhere to put one.
//
// It goes outside internal/metrics' middleware and inside logging.Access, so
// the span for the whole handler covers everything the other two measure, and
// so the status is readable from the recorder they share.
func Middleware(opts Options) func(http.Handler) http.Handler {
	lg := opts.Logger
	if lg == nil {
		lg = slog.Default()
	}
	if opts.Slower <= 0 {
		// Off: no collector, no spans, no decision to make.
		return func(next http.Handler) http.Handler { return next }
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, collector, release := trace.Collect(r.Context())
			defer release()

			start := time.Now()
			ctx, span := trace.Start(ctx, "handler")
			r = r.WithContext(ctx)

			// Deferred so that a request which panics past this middleware
			// is still written down — a 500 nobody can explain is exactly
			// what the journal is for.
			defer func() {
				span.End()
				took := time.Since(start)

				status := http.StatusOK
				if rec := httpstat.From(r.Context()); rec != nil {
					status = rec.Status()
				}
				// A WebSocket is a connection that lasts as long as the
				// player is on the page; it is not a slow request.
				if status == http.StatusSwitchingProtocols {
					return
				}
				if took < opts.Slower && status < http.StatusInternalServerError {
					return
				}

				route := logging.Route(r)
				id := logging.RequestIDFrom(r.Context())
				spans := collector.Spans()
				lost := collector.Dropped()

				lg.LogAttrs(r.Context(), slog.LevelWarn, "a slow request",
					slog.String("method", r.Method),
					slog.String("route", route),
					slog.Int("status", status),
					slog.Duration("dur", took),
					slog.String("id", id),
					slog.String("phases", Breakdown(spans, lost)),
				)

				if opts.DB == nil {
					return
				}
				payload, err := json.Marshal(toJSON(spans))
				if err != nil {
					lg.Error("the journal could not write a request down", "id", id, "err", err)
					return
				}
				// The request's own context may already be cancelled — the
				// client has its answer — so the write gets one of its own.
				wctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), writeTimeout)
				defer cancel()
				row := SlowRequest{
					At: start, RequestID: id, Route: route, Method: r.Method,
					Status: status, Duration: took, Spans: string(payload),
					SpanCount: len(spans), SpansLost: lost,
				}
				if status >= http.StatusInternalServerError {
					row.Err = fmt.Sprintf("the server answered %d", status)
				}
				if err := opts.DB.Write(wctx, row); err != nil {
					lg.Error("the journal could not write a request down", "id", id, "err", err)
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}

// spanJSON is one span as the journal stores it. Durations are milliseconds,
// because that is how they are read.
type spanJSON struct {
	Name     string         `json:"name"`
	Parent   int            `json:"parent"`
	OffsetMS float64        `json:"at_ms"`
	MS       float64        `json:"ms"`
	Unended  bool           `json:"unended,omitempty"`
	Attrs    map[string]any `json:"attrs,omitempty"`
}

func toJSON(spans []trace.Record) []spanJSON {
	out := make([]spanJSON, 0, len(spans))
	for _, s := range spans {
		j := spanJSON{
			Name:     s.Name,
			Parent:   s.Parent,
			OffsetMS: ms(s.Offset),
			MS:       ms(s.Duration),
			Unended:  !s.Ended,
		}
		if len(s.Attrs) > 0 {
			j.Attrs = make(map[string]any, len(s.Attrs))
			for _, a := range s.Attrs {
				j.Attrs[a.Key] = a.Value
			}
		}
		out = append(out, j)
	}
	return out
}

func ms(d time.Duration) float64 {
	// Rounded to microseconds: more precision than that is noise, and it
	// keeps the JSON short.
	return float64(d.Microseconds()) / 1000
}

// Breakdown is the one-line form of a set of spans, for the WARN line: the
// phases that actually took the time, longest first.
//
// It is a line a person reads at three in the morning, so it names at most
// five phases and says what it left out.
func Breakdown(spans []trace.Record, lost int) string {
	const show = 5
	type phase struct {
		name string
		d    time.Duration
		n    int
	}
	// Spans of the same name are one phase: ten statements are "store x10".
	order := make([]string, 0, len(spans))
	byName := make(map[string]*phase, len(spans))
	for _, s := range spans {
		if s.Name == "handler" {
			continue // the whole request, already in the line as dur
		}
		p, ok := byName[s.Name]
		if !ok {
			p = &phase{name: s.Name}
			byName[s.Name] = p
			order = append(order, s.Name)
		}
		p.d += s.Duration
		p.n++
	}
	// Longest first, and stable for equal durations so a line does not
	// change between reads of the same request.
	phases := make([]*phase, 0, len(order))
	for _, name := range order {
		phases = append(phases, byName[name])
	}
	for i := 1; i < len(phases); i++ {
		for j := i; j > 0 && phases[j].d > phases[j-1].d; j-- {
			phases[j], phases[j-1] = phases[j-1], phases[j]
		}
	}

	var b []byte
	for i, p := range phases {
		if i == show {
			b = append(b, fmt.Sprintf(" and %d more", len(phases)-show)...)
			break
		}
		if i > 0 {
			b = append(b, ", "...)
		}
		b = append(b, p.name...)
		if p.n > 1 {
			b = append(b, fmt.Sprintf(" x%d", p.n)...)
		}
		b = append(b, fmt.Sprintf(" %.1fms", ms(p.d))...)
	}
	if len(b) == 0 {
		b = append(b, "nothing was timed"...)
	}
	if lost > 0 {
		b = append(b, fmt.Sprintf(" (+%d spans over the cap)", lost)...)
	}
	return string(b)
}
