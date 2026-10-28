package logging

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/go-chi/chi/v5"
)

// RequestIDHeader is the header a request id is read from and echoed in.
const RequestIDHeader = "X-Request-Id"

// maxRequestID is how long an inbound request id may be before bingle
// replaces it with one of its own.
const maxRequestID = 64

type requestIDKey struct{}

// RequestID gives every request an id, which the lines logged about it carry
// and the response echoes in X-Request-Id, so one request can be followed from
// a browser's network tab into the log. An inbound id is kept when it is short
// and printable — a proxy in front of bingle has usually set a better one —
// and replaced when it is neither.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := cleanRequestID(r.Header.Get(RequestIDHeader))
		if id == "" {
			id = newRequestID()
		}
		w.Header().Set(RequestIDHeader, id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey{}, id)))
	})
}

// RequestIDFrom returns the request id ctx carries, or "" outside a request.
func RequestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

// newRequestID is eight random bytes in hex.
func newRequestID() string {
	var b [8]byte
	_, _ = rand.Read(b[:]) // crypto/rand.Read never returns an error
	return hex.EncodeToString(b[:])
}

// cleanRequestID returns id if it is safe to put in a log line and a response
// header — printable ASCII, no spaces, not too long — and "" otherwise.
func cleanRequestID(id string) string {
	if id == "" || len(id) > maxRequestID {
		return ""
	}
	for i := 0; i < len(id); i++ {
		if id[i] <= ' ' || id[i] > '~' {
			return ""
		}
	}
	return id
}

// Recover turns a panic in a handler into one ERROR line with its stack and a
// 500. Without it net/http drops the connection and records nothing, so a
// panic in a handler is invisible — which is why this is the middleware the
// rest of the observability work needed first.
//
// It goes inside Access, not outside it: a panic unwinds past everything
// outside the middleware that recovers, so a recover registered first would
// make a panicking request the one request the access log never sees.
func Recover(lg *slog.Logger) func(http.Handler) http.Handler {
	lg = orDefault(lg)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rec := &recorder{ResponseWriter: w}
			defer func() {
				v := recover()
				if v == nil {
					return
				}
				// ErrAbortHandler is how a handler says "drop this
				// connection, quietly"; net/http expects to see it.
				if v == http.ErrAbortHandler {
					panic(v)
				}
				lg.LogAttrs(r.Context(), slog.LevelError, "a request panicked",
					slog.String("method", r.Method),
					slog.String("route", Route(r)),
					slog.String("id", RequestIDFrom(r.Context())),
					slog.Any("panic", v),
					slog.String("stack", string(debug.Stack())),
				)
				if !rec.wrote {
					http.Error(rec, "internal server error", http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(rec, r)
		})
	}
}

// Access logs one line per request — including a request that panicked, as
// long as Recover is registered inside it: at info when the server failed, and
// at debug otherwise, so a normal server says nothing per request until
// BINGLE_LOG_LEVEL=debug asks it to.
//
// The line carries chi's route pattern and never the path the request came to:
// a path holds room codes (see Room).
//
// A WebSocket is not a request that finishes, it is a connection that lasts as
// long as the player is on the page, so an accepted upgrade is logged when it
// is accepted and again when it closes. A rejected one is an ordinary request
// and is logged as one.
func Access(lg *slog.Logger) func(http.Handler) http.Handler {
	lg = orDefault(lg)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &recorder{ResponseWriter: w}
			rec.onUpgrade = func() {
				lg.LogAttrs(r.Context(), slog.LevelDebug, "websocket open",
					slog.String("method", r.Method),
					slog.String("route", Route(r)),
					slog.String("id", RequestIDFrom(r.Context())),
				)
			}

			next.ServeHTTP(rec, r)

			dur := time.Since(start)
			if rec.statusOrOK() == http.StatusSwitchingProtocols {
				lg.LogAttrs(r.Context(), slog.LevelDebug, "websocket closed",
					slog.String("method", r.Method),
					slog.String("route", Route(r)),
					slog.String("id", RequestIDFrom(r.Context())),
					slog.Duration("dur", dur),
				)
				return
			}
			level := slog.LevelDebug
			if rec.statusOrOK() >= http.StatusInternalServerError {
				level = slog.LevelInfo
			}
			lg.LogAttrs(r.Context(), level, "request",
				slog.String("method", r.Method),
				slog.String("route", Route(r)),
				slog.Int("status", rec.statusOrOK()),
				slog.Int64("bytes", rec.bytes),
				slog.Duration("dur", dur),
				slog.String("id", RequestIDFrom(r.Context())),
			)
		})
	}
}

// Route is the chi route pattern the request matched, such as
// "/api/games/{code}" — the safe stand-in for the path, which carries the room
// code itself. It is "" for a request that matched nothing.
func Route(r *http.Request) string {
	return chi.RouteContext(r.Context()).RoutePattern()
}

// recorder is the http.ResponseWriter the middlewares wrap a handler's in, to
// learn the status and the size of a response they do not write themselves.
//
// It deliberately implements nothing but Unwrap beyond that: net/http's
// ResponseController and the WebSocket library both follow Unwrap to reach the
// real writer, so hijacking and flushing keep working without this type
// having to know about them.
type recorder struct {
	http.ResponseWriter
	status    int
	bytes     int64
	wrote     bool
	onUpgrade func() // called once, when an upgrade is accepted
}

func (rc *recorder) WriteHeader(status int) {
	if rc.wrote {
		rc.ResponseWriter.WriteHeader(status) // let net/http complain, as it would without us
		return
	}
	rc.status = status
	rc.wrote = true
	rc.ResponseWriter.WriteHeader(status)
	if status == http.StatusSwitchingProtocols && rc.onUpgrade != nil {
		rc.onUpgrade()
	}
}

// statusOrOK is the status the client saw: a handler that writes nothing at
// all still ends in a 200 from net/http.
func (rc *recorder) statusOrOK() int {
	if rc.status == 0 {
		return http.StatusOK
	}
	return rc.status
}

func (rc *recorder) Write(b []byte) (int, error) {
	if !rc.wrote {
		rc.WriteHeader(http.StatusOK) // what net/http does on a first Write
	}
	n, err := rc.ResponseWriter.Write(b)
	rc.bytes += int64(n)
	return n, err
}

// Unwrap returns the wrapped writer, for http.ResponseController and for the
// WebSocket upgrade's hijack.
func (rc *recorder) Unwrap() http.ResponseWriter { return rc.ResponseWriter }

func orDefault(lg *slog.Logger) *slog.Logger {
	if lg == nil {
		return slog.Default()
	}
	return lg
}
