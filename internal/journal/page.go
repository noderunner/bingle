package journal

import (
	"context"
	"embed"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
)

//go:embed health.html.tmpl
var pageFS embed.FS

var pageTemplate = template.Must(template.ParseFS(pageFS, "health.html.tmpl"))

// PageOptions are what the Health page and the history endpoints need.
type PageOptions struct {
	// DB is the history. Nil serves the page with nothing in it, which is
	// what a server with BINGLE_METRICS_DB empty shows.
	DB *DB
	// Version is the engine's version, for the page's footer.
	Version string
	// Every and RetainDays are the sampler's settings, so the page can say
	// what it is showing.
	Every      time.Duration
	RetainDays int
	// Logger is the server's logger; nil means slog's default.
	Logger *slog.Logger
	// Sample, when set, is called when somebody asks the page for a fresh
	// sample rather than waiting for the next tick.
	Sample func(ctx context.Context)
}

// defaultWindowHours is how much history the page shows when nothing says
// otherwise: a day, which is the window somebody looking into "it was slow
// last night" actually wants.
const defaultWindowHours = 24

// maxWindowHours bounds the window a reader may ask for: a year.
const maxWindowHours = 24 * 366

// Routes mounts the history endpoints and the Health page.
//
// Everything here goes on the **private** listener only. It says which games
// are being played and how busy the server is, and the public site has no
// authentication of any kind (ADR 0064).
func Routes(r chi.Router, opts PageOptions) {
	h := &pageHandlers{opts: opts}
	r.Get("/metrics/health", h.page)
	r.Get("/metrics/history", h.history)
	r.Get("/metrics/history.csv", h.historyCSV)
	r.Get("/metrics/series", h.series)
	r.Post("/metrics/sample", h.sampleNow)
}

type pageHandlers struct {
	opts PageOptions
}

func (h *pageHandlers) log() *slog.Logger {
	if h.opts.Logger == nil {
		return slog.Default()
	}
	return h.opts.Logger
}

// page serves the Health page itself, which is a shell: the script fetches the
// history and draws it with the charts the batch reports already use.
func (h *pageHandlers) page(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	data := struct {
		Version     string
		Every       string
		RetainDays  int
		WindowHours int
	}{
		Version:     h.opts.Version,
		Every:       h.opts.Every.String(),
		RetainDays:  h.opts.RetainDays,
		WindowHours: windowHours(r),
	}
	if err := pageTemplate.ExecuteTemplate(w, "health.html.tmpl", data); err != nil {
		h.log().Error("the health page could not be rendered", "err", err)
	}
}

// series lists what the history holds, for the page's picker.
func (h *pageHandlers) series(w http.ResponseWriter, r *http.Request) {
	names, err := h.opts.DB.Names(r.Context())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	counts, err := h.opts.DB.Counts(r.Context())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, map[string]any{
		"names":        names,
		"rows":         counts,
		"every":        h.opts.Every.String(),
		"retain_days":  h.opts.RetainDays,
		"window_hours": windowHours(r),
	})
}

// history answers GET /metrics/history?name=…&since=…&labels=…
func (h *pageHandlers) history(w http.ResponseWriter, r *http.Request) {
	q, err := queryFrom(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	series, err := h.opts.DB.History(r.Context(), q)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	writeJSON(w, map[string]any{
		"name":   q.Name,
		"since":  q.Since.UTC(),
		"until":  q.Until.UTC(),
		"series": series,
	})
}

// historyCSV is the same thing for a spreadsheet, which is the shortest path
// from "is this getting worse?" to a chart somebody else can read.
func (h *pageHandlers) historyCSV(w http.ResponseWriter, r *http.Request) {
	q, err := queryFrom(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	series, err := h.opts.DB.History(r.Context(), q)
	if err != nil {
		h.fail(w, r, err)
		return
	}

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+csvName(q.Name)+`"`)
	cw := csv.NewWriter(w)
	defer cw.Flush()
	if err := cw.Write([]string{"at", "name", "labels", "value", "min", "max", "n", "bucket"}); err != nil {
		return
	}
	for _, s := range series {
		for _, p := range s.Points {
			row := []string{
				p.At.UTC().Format(time.RFC3339), s.Name, s.Labels,
				strconv.FormatFloat(p.Value, 'g', -1, 64), "", "",
				strconv.Itoa(p.N), p.Bucket,
			}
			if p.Min != nil {
				row[4] = strconv.FormatFloat(*p.Min, 'g', -1, 64)
			}
			if p.Max != nil {
				row[5] = strconv.FormatFloat(*p.Max, 'g', -1, 64)
			}
			if err := cw.Write(row); err != nil {
				return
			}
		}
	}
}

// sampleNow takes a sample on demand, so somebody watching the page does not
// have to wait out the interval to see whether a change helped.
func (h *pageHandlers) sampleNow(w http.ResponseWriter, r *http.Request) {
	if h.opts.Sample == nil {
		http.Error(w, "the metric history is off", http.StatusNotFound)
		return
	}
	h.opts.Sample(r.Context())
	writeJSON(w, map[string]any{"sampled": true})
}

// queryFrom reads a history query off a request.
func queryFrom(r *http.Request) (Query, error) {
	q := Query{
		Name:   r.URL.Query().Get("name"),
		Labels: r.URL.Query().Get("labels"),
	}
	if q.Name == "" {
		return Query{}, fmt.Errorf("name is required: try /metrics/series for what there is")
	}
	hours := windowHours(r)
	q.Until = time.Now()
	q.Since = q.Until.Add(-time.Duration(hours) * time.Hour)
	if v := r.URL.Query().Get("since"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return Query{}, fmt.Errorf("since must be an RFC 3339 time such as 2026-10-28T19:00:00Z, or use hours=")
		}
		q.Since = t
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return Query{}, fmt.Errorf("limit must be a positive integer")
		}
		q.Limit = n
	}
	return q, nil
}

// windowHours is the window a request asked for, clamped.
func windowHours(r *http.Request) int {
	hours := defaultWindowHours
	if v := r.URL.Query().Get("hours"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			hours = n
		}
	}
	if hours > maxWindowHours {
		hours = maxWindowHours
	}
	return hours
}

// csvName is a filename a browser will accept, built from a series name.
func csvName(name string) string {
	safe := make([]byte, 0, len(name))
	for i := 0; i < len(name); i++ {
		c := name[i]
		if c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			safe = append(safe, c)
		}
	}
	if len(safe) == 0 {
		safe = []byte("history")
	}
	return string(safe) + ".csv"
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// fail answers a reader and says what happened in the log, which is where a
// fault on a monitoring endpoint belongs.
func (h *pageHandlers) fail(w http.ResponseWriter, r *http.Request, err error) {
	h.log().Error("the metric history could not be read", "path", r.URL.Path, "err", err)
	http.Error(w, "the metric history could not be read", http.StatusInternalServerError)
}
