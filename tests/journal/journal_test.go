// Tests of the slow-request journal: what it keeps, what it throws away, and
// what a kept row says.

package journal_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/noderunner/bingle/internal/journal"
	"github.com/noderunner/bingle/internal/logging"
	"github.com/noderunner/bingle/internal/trace"
)

// safeBuffer is a bytes.Buffer the handler goroutines can share.
type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *safeBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *safeBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// setup builds a router with the middlewares in the order internal/app uses
// them, over a journal on its own file. It returns the handler, the journal
// and the log.
func setup(t *testing.T, slower time.Duration, routes func(chi.Router)) (http.Handler, *journal.DB, *safeBuffer) {
	t.Helper()
	db, err := journal.Open(filepath.Join(t.TempDir(), "bingle-metrics.db"))
	if err != nil {
		t.Fatalf("journal.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	buf := &safeBuffer{}
	lg := logging.New(buf, slog.LevelDebug, logging.FormatText)

	r := chi.NewRouter()
	r.Use(logging.RequestID)
	r.Use(logging.Access(lg))
	r.Use(journal.Middleware(journal.Options{DB: db, Logger: lg, Slower: slower}))
	r.Use(logging.Recover(lg))
	routes(r)
	return r, db, buf
}

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	return w
}

// rows reads the journal.
func rows(t *testing.T, db *journal.DB) []journal.SlowRequest {
	t.Helper()
	got, err := db.Recent(context.Background(), 100)
	if err != nil {
		t.Fatalf("reading the journal: %v", err)
	}
	return got
}

// spansOf decodes a row's breakdown.
func spansOf(t *testing.T, r journal.SlowRequest) []map[string]any {
	t.Helper()
	var out []map[string]any
	if err := json.Unmarshal([]byte(r.Spans), &out); err != nil {
		t.Fatalf("the row's spans are not JSON: %v (%q)", err, r.Spans)
	}
	return out
}

// A slow handler leaves exactly one row, and the row says where the time went
// in the order it was spent.
func TestJournal_ASlowRequestWritesOneRowWithItsSpansInOrder(t *testing.T) {
	h, db, buf := setup(t, 20*time.Millisecond, func(r chi.Router) {
		r.Get("/api/games/{code}/moves", func(w http.ResponseWriter, req *http.Request) {
			ctx, outer := trace.Start(req.Context(), "session.ApplyMove")
			time.Sleep(10 * time.Millisecond)

			_, lock := trace.Start(ctx, "room.lock")
			time.Sleep(30 * time.Millisecond)
			lock.End()

			_, store := trace.Start(ctx, "store")
			store.Set("stmt", "RecordMove")
			time.Sleep(5 * time.Millisecond)
			store.End()

			outer.End()
			_, _ = w.Write([]byte("ok"))
		})
	})

	if w := get(t, h, "/api/games/AB12CD/moves"); w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}

	got := rows(t, db)
	if len(got) != 1 {
		t.Fatalf("the journal holds %d rows, want exactly 1", len(got))
	}
	row := got[0]

	if row.Route != "/api/games/{code}/moves" {
		t.Errorf("route = %q, want the chi pattern", row.Route)
	}
	if row.Method != http.MethodGet || row.Status != http.StatusOK {
		t.Errorf("row = %s %d", row.Method, row.Status)
	}
	if row.Duration < 40*time.Millisecond {
		t.Errorf("duration = %v, want at least the 45ms the handler slept", row.Duration)
	}
	if row.RequestID == "" {
		t.Error("the row has no request id, so it cannot be matched to a log line")
	}
	if row.At.IsZero() {
		t.Error("the row has no time")
	}

	// The spans, in the order they started, with the nesting intact.
	spans := spansOf(t, row)
	var names []string
	for _, s := range spans {
		names = append(names, s["name"].(string))
	}
	want := []string{"handler", "session.ApplyMove", "room.lock", "store"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Errorf("spans = %v, want %v", names, want)
	}
	if spans[1]["parent"].(float64) != 0 || spans[2]["parent"].(float64) != 1 {
		t.Errorf("the nesting is wrong: %v", spans)
	}
	if row.SpanCount != 4 {
		t.Errorf("span_count = %d, want 4", row.SpanCount)
	}

	// room.lock is the slowest phase, which is the whole point of the line.
	if lock := spans[2]["ms"].(float64); lock < 25 {
		t.Errorf("room.lock took %vms, want about 30", lock)
	}
	if attrs, ok := spans[3]["attrs"].(map[string]any); !ok || attrs["stmt"] != "RecordMove" {
		t.Errorf("the store span's attributes = %v", spans[3]["attrs"])
	}

	// And a WARN line a person can read, naming the phase that cost the most.
	line := buf.String()
	if !strings.Contains(line, `level=WARN msg="a slow request"`) {
		t.Errorf("no WARN line for the slow request:\n%s", line)
	}
	if !strings.Contains(line, "room.lock") {
		t.Errorf("the WARN line does not name the slowest phase:\n%s", line)
	}
	if strings.Contains(line, "AB12CD") {
		t.Errorf("the WARN line carries the room code:\n%s", line)
	}
}

// An ordinary request is thrown away. Keeping every request would be a trace
// store, and bingle has nowhere to put one.
func TestJournal_AFastRequestIsThrownAway(t *testing.T) {
	h, db, _ := setup(t, time.Second, func(r chi.Router) {
		r.Get("/api/game-defs", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("[]")) })
	})

	for i := 0; i < 20; i++ {
		get(t, h, "/api/game-defs")
	}
	if got := rows(t, db); len(got) != 0 {
		t.Errorf("the journal kept %d fast requests", len(got))
	}
}

// A request the server failed is written down however fast it was: a 500
// nobody can explain is exactly what the journal is for.
func TestJournal_AFailedRequestIsKeptHoweverFast(t *testing.T) {
	h, db, _ := setup(t, time.Hour, func(r chi.Router) {
		r.Get("/boom", func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "sorry", http.StatusInternalServerError)
		})
		r.Get("/refused", func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "no", http.StatusBadRequest)
		})
	})

	get(t, h, "/boom")
	get(t, h, "/refused") // a client error is the client's business, not a fault

	got := rows(t, db)
	if len(got) != 1 {
		t.Fatalf("the journal holds %d rows, want 1 (the 500 only)", len(got))
	}
	if got[0].Status != http.StatusInternalServerError || got[0].Route != "/boom" {
		t.Errorf("row = %d %s", got[0].Status, got[0].Route)
	}
	if got[0].Err == "" {
		t.Error("the row does not say what went wrong")
	}
}

// A handler that panicked is the hardest kind of 500 to explain, so it must be
// the one most certainly written down.
func TestJournal_APanickingRequestIsWrittenDown(t *testing.T) {
	h, db, _ := setup(t, time.Hour, func(r chi.Router) {
		r.Get("/api/games/{code}", func(req http.ResponseWriter, r *http.Request) {
			_, span := trace.Start(r.Context(), "store")
			span.Set("stmt", "GetGame")
			span.End()
			panic("the ruleset fell over")
		})
	})

	if w := get(t, h, "/api/games/AB12CD"); w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}

	got := rows(t, db)
	if len(got) != 1 {
		t.Fatalf("the journal holds %d rows, want 1", len(got))
	}
	// The spans recorded before the panic are kept, which is what says how far
	// the request got.
	spans := spansOf(t, got[0])
	if len(spans) != 2 || spans[1]["name"] != "store" {
		t.Errorf("spans = %v, want the handler and the store span before the panic", spans)
	}
}

// A WebSocket is a connection that lasts as long as the player is on the page.
// It is not a slow request.
func TestJournal_AnAcceptedSocketIsNotASlowRequest(t *testing.T) {
	h, db, _ := setup(t, time.Millisecond, func(r chi.Router) {
		r.Get("/ws/{code}", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusSwitchingProtocols)
			time.Sleep(20 * time.Millisecond)
		})
	})

	get(t, h, "/ws/AB12CD")
	if got := rows(t, db); len(got) != 0 {
		t.Errorf("an accepted socket was written down as a slow request: %+v", got)
	}
}

// With the journal off there is no collector at all, so a span costs a context
// lookup and nothing is kept.
func TestJournal_OffMeansNoCollectorAtAll(t *testing.T) {
	h, db, buf := setup(t, 0, func(r chi.Router) {
		r.Get("/slow", func(w http.ResponseWriter, req *http.Request) {
			if c := trace.FromContext(req.Context()); c != nil {
				t.Error("something is collecting spans with the journal off")
			}
			time.Sleep(10 * time.Millisecond)
			_, _ = w.Write([]byte("ok"))
		})
	})

	get(t, h, "/slow")
	if got := rows(t, db); len(got) != 0 {
		t.Errorf("the journal kept %d rows while off", len(got))
	}
	if strings.Contains(buf.String(), "a slow request") {
		t.Error("a WARN line was logged while the journal was off")
	}
}

// Without a database the journal still says its line: that is the whole
// journal for a server run with BINGLE_METRICS_DB empty.
func TestJournal_WithoutADatabaseStillLogsTheLine(t *testing.T) {
	buf := &safeBuffer{}
	lg := logging.New(buf, slog.LevelDebug, logging.FormatText)

	r := chi.NewRouter()
	r.Use(logging.RequestID)
	r.Use(logging.Access(lg))
	r.Use(journal.Middleware(journal.Options{DB: nil, Logger: lg, Slower: time.Millisecond}))
	r.Get("/slow", func(w http.ResponseWriter, req *http.Request) {
		_, span := trace.Start(req.Context(), "room.lock")
		time.Sleep(10 * time.Millisecond)
		span.End()
		_, _ = w.Write([]byte("ok"))
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/slow", nil))

	if !strings.Contains(buf.String(), `msg="a slow request"`) {
		t.Errorf("no WARN line without a database:\n%s", buf.String())
	}
	if !strings.Contains(buf.String(), "room.lock") {
		t.Errorf("the line does not name the phase:\n%s", buf.String())
	}
}

// A request that opens spans in a loop cannot grow the row without bound, and
// the row says it was cut short.
func TestJournal_ARunawayRequestIsCappedAndSaysSo(t *testing.T) {
	h, db, _ := setup(t, time.Microsecond, func(r chi.Router) {
		r.Get("/loop", func(w http.ResponseWriter, req *http.Request) {
			for i := 0; i < 500; i++ {
				_, span := trace.Start(req.Context(), "store")
				span.End()
			}
			_, _ = w.Write([]byte("ok"))
		})
	})

	get(t, h, "/loop")
	got := rows(t, db)
	if len(got) != 1 {
		t.Fatalf("the journal holds %d rows, want 1", len(got))
	}
	if got[0].SpansLost <= 0 {
		t.Error("a request with 500 spans did not report losing any")
	}
	if n := len(spansOf(t, got[0])); n >= 500 {
		t.Errorf("the row holds %d spans, which is not capped", n)
	}
}

// The breakdown is for a person at three in the morning: the phases that cost
// the time, longest first, with repeats collapsed.
func TestBreakdown_NamesTheExpensivePhasesLongestFirst(t *testing.T) {
	ctx, c, release := trace.Collect(context.Background())
	defer release()

	_, handler := trace.Start(ctx, "handler")
	for i := 0; i < 3; i++ {
		_, s := trace.Start(ctx, "store")
		time.Sleep(time.Millisecond)
		s.End()
	}
	_, lock := trace.Start(ctx, "room.lock")
	time.Sleep(20 * time.Millisecond)
	lock.End()
	handler.End()

	got := journal.Breakdown(c.Spans(), 0)
	if !strings.HasPrefix(got, "room.lock ") {
		t.Errorf("breakdown = %q, want the slowest phase first", got)
	}
	if !strings.Contains(got, "store x3") {
		t.Errorf("breakdown = %q, want the three store spans collapsed", got)
	}
	if strings.Contains(got, "handler") {
		t.Errorf("breakdown = %q, which repeats the whole request", got)
	}
}

func TestBreakdown_SaysWhenThereIsNothingToSay(t *testing.T) {
	if got := journal.Breakdown(nil, 0); got == "" {
		t.Error("the breakdown of no spans is empty")
	}
	if got := journal.Breakdown(nil, 7); !strings.Contains(got, "7") {
		t.Errorf("breakdown = %q, want it to mention the 7 spans lost", got)
	}
}
