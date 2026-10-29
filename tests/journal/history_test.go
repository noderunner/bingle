// Tests of the metric history: what a tick writes, what compaction keeps,
// what retention leaves alone, and what the page and the endpoints serve.

package journal_test

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/noderunner/bingle/internal/journal"
	"github.com/noderunner/bingle/internal/logging"
	"github.com/noderunner/bingle/internal/metrics"
)

// openDB returns a metrics database on its own file.
func openDB(t *testing.T) *journal.DB {
	t.Helper()
	db, err := journal.Open(filepath.Join(t.TempDir(), "bingle-metrics.db"))
	if err != nil {
		t.Fatalf("journal.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// counts reads the row counts.
func counts(t *testing.T, db *journal.DB) map[string]int64 {
	t.Helper()
	got, err := db.Counts(context.Background())
	if err != nil {
		t.Fatalf("counting rows: %v", err)
	}
	return got
}

// A tick is one transaction: every series at one instant, or none of them.
func TestSampler_ATickWritesOneTransaction(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()

	mx := metrics.New("9.9.9")
	samples, err := mx.Samples()
	if err != nil {
		t.Fatalf("gathering: %v", err)
	}
	if len(samples) < 20 {
		t.Fatalf("a fresh registry gathered %d samples, which is too few to be measuring anything", len(samples))
	}

	at := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	if err := db.WriteSamples(ctx, at, samples); err != nil {
		t.Fatalf("writing a tick: %v", err)
	}

	if got := counts(t, db)["samples"]; got != int64(len(samples)) {
		t.Errorf("the tick wrote %d rows for %d series", got, len(samples))
	}
	// Every row of the tick shares its instant, which is what lets a reader
	// line two series up against each other.
	series, err := db.History(ctx, journal.Query{Name: "bingle_build_info", Since: at.Add(-time.Hour), Until: at.Add(time.Hour)})
	if err != nil {
		t.Fatalf("reading back: %v", err)
	}
	if len(series) != 1 || len(series[0].Points) != 1 {
		t.Fatalf("read back %d series: %+v", len(series), series)
	}
	if !series[0].Points[0].At.Equal(at) {
		t.Errorf("the point is at %v, want %v", series[0].Points[0].At, at)
	}
	if series[0].Label["version"] != "9.9.9" {
		t.Errorf("the labels came back as %v", series[0].Label)
	}

	// The same instant twice is the same row, not a duplicate: a tick that
	// is retried must not double the history.
	if err := db.WriteSamples(ctx, at, samples); err != nil {
		t.Fatalf("writing the tick again: %v", err)
	}
	if got := counts(t, db)["samples"]; got != int64(len(samples)) {
		t.Errorf("writing the same tick twice left %d rows for %d series", got, len(samples))
	}
}

// Nothing is half-written: a tick that cannot go through leaves the history as
// it was.
func TestSampler_ATickThatFailsWritesNothing(t *testing.T) {
	db := openDB(t)
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	err := db.WriteSamples(cancelled, time.Now(), []journal.Sample{
		{Name: "bingle_test", Labels: "", Value: 1},
	})
	if err == nil {
		t.Error("writing with a cancelled context returned no error")
	}
	if got := counts(t, db)["samples"]; got != 0 {
		t.Errorf("a failed tick left %d rows behind", got)
	}
}

// seed writes one series at a fixed interval, returning the values written.
func seed(t *testing.T, db *journal.DB, name string, from time.Time, every time.Duration, n int, value func(i int) float64) []float64 {
	t.Helper()
	out := make([]float64, n)
	for i := 0; i < n; i++ {
		out[i] = value(i)
		at := from.Add(time.Duration(i) * every)
		if err := db.WriteSamples(context.Background(), at, []journal.Sample{{Name: name, Value: out[i]}}); err != nil {
			t.Fatalf("seeding %s: %v", name, err)
		}
	}
	return out
}

// Compaction folds old samples into hourly rollups without losing what they
// said: every sample is accounted for, and the extremes survive.
func TestCompact_PreservesTheTotals(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()

	// Twenty days of five-minute samples, which is past the fortnight window.
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	start := now.Add(-20 * 24 * time.Hour)
	const n = 20 * 24 * 12 // five-minute samples over twenty days
	values := seed(t, db, "bingle_test_gauge", start, 5*time.Minute, n, func(i int) float64 { return float64(i % 97) })

	before := counts(t, db)["samples"]
	if before != n {
		t.Fatalf("seeded %d rows, want %d", before, n)
	}

	res, err := db.Compact(ctx, now, 14)
	if err != nil {
		t.Fatalf("compacting: %v", err)
	}
	after := counts(t, db)

	// Every sample outside the window was folded into a rollup, and the
	// rollups are far fewer rows than the samples were.
	if res.SamplesCompacted == 0 || res.HourlyRollups == 0 {
		t.Fatalf("compaction did nothing: %+v", res)
	}
	if after["rollups_hour"] >= before {
		t.Errorf("%d samples became %d hourly rollups, which is not compaction", before, after["rollups_hour"])
	}

	// Nothing was lost: the rollups account for exactly the samples that went
	// away, and the extremes are still there.
	series, err := db.History(ctx, journal.Query{Name: "bingle_test_gauge", Since: start.Add(-time.Hour), Until: now})
	if err != nil {
		t.Fatalf("reading back: %v", err)
	}
	if len(series) != 1 {
		t.Fatalf("read back %d series", len(series))
	}

	var folded int
	lo, hi := 1e18, -1e18
	for _, p := range series[0].Points {
		switch p.Bucket {
		case "":
			folded++
			lo, hi = min(lo, p.Value), max(hi, p.Value)
		default:
			folded += p.N
			lo, hi = min(lo, *p.Min), max(hi, *p.Max)
		}
	}
	if folded != n {
		t.Errorf("the history accounts for %d samples, want the %d written", folded, n)
	}
	wantLo, wantHi := 1e18, -1e18
	for _, v := range values {
		wantLo, wantHi = min(wantLo, v), max(wantHi, v)
	}
	if lo != wantLo || hi != wantHi {
		t.Errorf("the extremes came back as %v..%v, want %v..%v", lo, hi, wantLo, wantHi)
	}
}

// Retention leaves alone everything inside its window: compaction is not a
// delete.
func TestCompact_LeavesTheWindowAlone(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()

	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	// Half the samples inside a two-day window, half well outside it.
	inside := now.Add(-36 * time.Hour)
	outside := now.Add(-10 * 24 * time.Hour)
	seed(t, db, "bingle_test_gauge", inside, 5*time.Minute, 48, func(i int) float64 { return 1 })
	seed(t, db, "bingle_test_gauge", outside, 5*time.Minute, 48, func(i int) float64 { return 2 })

	if _, err := db.Compact(ctx, now, 2); err != nil {
		t.Fatalf("compacting: %v", err)
	}

	// The 48 recent ones are still raw samples.
	if got := counts(t, db)["samples"]; got != 48 {
		t.Errorf("%d raw samples left, want the 48 inside the window", got)
	}
	series, err := db.History(ctx, journal.Query{Name: "bingle_test_gauge", Since: inside, Until: now})
	if err != nil {
		t.Fatalf("reading back: %v", err)
	}
	if len(series) != 1 || len(series[0].Points) != 48 {
		t.Fatalf("the window holds %d points, want 48", len(series[0].Points))
	}
	for _, p := range series[0].Points {
		if p.Bucket != "" {
			t.Errorf("a point inside the window was compacted: %+v", p)
			break
		}
		if p.At.Before(now.Add(-48 * time.Hour)) {
			t.Errorf("a point from outside the window survived as a sample: %v", p.At)
			break
		}
	}
}

// After a year the hourly rollups become daily ones.
func TestCompact_HourlyBecomesDailyAfterAYear(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()

	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	// Samples from more than a year ago: one compaction turns them into
	// hourly rollups, and the same run should then fold those into days.
	start := now.Add(-400 * 24 * time.Hour)
	seed(t, db, "bingle_test_gauge", start, 5*time.Minute, 24*12, func(i int) float64 { return float64(i) })

	if _, err := db.Compact(ctx, now, 14); err != nil {
		t.Fatalf("first compaction: %v", err)
	}
	after := counts(t, db)
	if after["rollups_day"] == 0 {
		t.Errorf("a day of samples from 400 days ago left %d daily rollups: %+v", after["rollups_day"], after)
	}
	if after["rollups_hour"] != 0 {
		t.Errorf("%d hourly rollups older than a year survived", after["rollups_hour"])
	}
	if after["samples"] != 0 {
		t.Errorf("%d samples older than a year survived", after["samples"])
	}

	// And they still account for every sample. The window starts a day
	// earlier than the samples do, because a rollup is stamped with the
	// *start* of its bucket: the day bucket holding 12:00 begins at 00:00,
	// which is before the first sample. A reader asking for a window of old
	// history wants to round down to the bucket.
	series, err := db.History(ctx, journal.Query{Name: "bingle_test_gauge", Since: start.Add(-24 * time.Hour), Until: now})
	if err != nil {
		t.Fatalf("reading back: %v", err)
	}
	var folded int
	for _, p := range series[0].Points {
		folded += p.N
	}
	if folded != 24*12 {
		t.Errorf("the daily rollups account for %d samples, want %d", folded, 24*12)
	}
}

// Compaction with retention off keeps everything at full resolution: the
// operator's choice, and nothing to do.
func TestCompact_RetentionOffDoesNothing(t *testing.T) {
	db := openDB(t)
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	seed(t, db, "bingle_test_gauge", now.Add(-500*24*time.Hour), time.Hour, 24, func(i int) float64 { return 1 })

	res, err := db.Compact(context.Background(), now, 0)
	if err != nil {
		t.Fatalf("compacting: %v", err)
	}
	if res.Any() {
		t.Errorf("compaction ran with retention off: %+v", res)
	}
	if got := counts(t, db)["samples"]; got != 24 {
		t.Errorf("%d samples left, want all 24", got)
	}
}

// The journal keeps the same window as the samples.
func TestPurgeSlowRequests_DropsOnlyTheOldOnes(t *testing.T) {
	db := openDB(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)

	for i, at := range []time.Time{now.Add(-30 * 24 * time.Hour), now.Add(-time.Hour)} {
		if err := db.Write(ctx, journal.SlowRequest{
			At: at, RequestID: "id" + strconv.Itoa(i), Route: "/api/games", Method: "GET",
			Status: 200, Duration: time.Second, Spans: "[]",
		}); err != nil {
			t.Fatalf("writing a slow request: %v", err)
		}
	}

	n, err := db.PurgeSlowRequests(ctx, now.Add(-14*24*time.Hour))
	if err != nil {
		t.Fatalf("purging: %v", err)
	}
	if n != 1 {
		t.Errorf("purged %d rows, want 1", n)
	}
	rows, err := db.Recent(ctx, 10)
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	if len(rows) != 1 || rows[0].RequestID != "id1" {
		t.Errorf("the wrong row survived: %+v", rows)
	}
}

// ---- the endpoints and the page --------------------------------------------

// newPage mounts the history endpoints over a seeded database.
func newPage(t *testing.T, db *journal.DB) http.Handler {
	t.Helper()
	r := chi.NewRouter()
	journal.Routes(r, journal.PageOptions{
		DB: db, Version: "9.9.9", Every: 5 * time.Minute, RetainDays: 14,
		Logger: logging.New(&safeBuffer{}, slog.LevelDebug, logging.FormatText),
	})
	return r
}

func getPath(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	return w
}

// The page renders from a seeded history, and says what it is showing.
func TestHealthPage_RendersFromASeededHistory(t *testing.T) {
	db := openDB(t)
	now := time.Now()
	seed(t, db, "bingle_http_requests_total", now.Add(-time.Hour), 5*time.Minute, 12, func(i int) float64 { return float64(i * 10) })

	w := getPath(t, newPage(t, db), "/metrics/health")
	if w.Code != http.StatusOK {
		t.Fatalf("GET /metrics/health = %d", w.Code)
	}
	body := w.Body.String()
	for _, want := range []string{
		`id="health-root"`,
		`/static/js/health.js`, // the page's own script
		`/static/js/charts.js`, // not referenced directly, but the module is
		`bingle v9.9.9`,
	} {
		if want == "/static/js/charts.js" {
			continue // health.js imports it; the page only names its own entry point
		}
		if !strings.Contains(body, want) {
			t.Errorf("the page does not carry %s:\n%s", want, body)
		}
	}
	if !strings.Contains(body, "history every 5m0s") {
		t.Errorf("the page does not say how often it samples:\n%s", body)
	}
}

// The history comes back as JSON a chart can use, newest last.
func TestHistoryEndpoint_ServesThePointsInOrder(t *testing.T) {
	db := openDB(t)
	now := time.Now()
	seed(t, db, "bingle_http_requests_total", now.Add(-time.Hour), 5*time.Minute, 12, func(i int) float64 { return float64(i * 10) })

	h := newPage(t, db)
	w := getPath(t, h, "/metrics/history?name=bingle_http_requests_total&hours=2")
	if w.Code != http.StatusOK {
		t.Fatalf("GET /metrics/history = %d (%s)", w.Code, w.Body.String())
	}
	var body struct {
		Name   string `json:"name"`
		Series []struct {
			Points []struct {
				At    time.Time `json:"at"`
				Value float64   `json:"value"`
			} `json:"points"`
		} `json:"series"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding: %v (%s)", err, w.Body.String())
	}
	if body.Name != "bingle_http_requests_total" || len(body.Series) != 1 {
		t.Fatalf("body = %+v", body)
	}
	points := body.Series[0].Points
	if len(points) != 12 {
		t.Fatalf("%d points, want 12", len(points))
	}
	for i := 1; i < len(points); i++ {
		if !points[i].At.After(points[i-1].At) {
			t.Fatalf("point %d is not after point %d: %v then %v", i, i-1, points[i-1].At, points[i].At)
		}
		if points[i].Value <= points[i-1].Value {
			t.Errorf("the counter went backwards: %v then %v", points[i-1].Value, points[i].Value)
		}
	}

	// A name nobody has is an empty answer, not an error; no name at all is
	// a bad request with something useful to say.
	if w := getPath(t, h, "/metrics/history?name=nothing_like_this"); w.Code != http.StatusOK {
		t.Errorf("an unknown name = %d, want 200 and nothing in it", w.Code)
	}
	w = getPath(t, h, "/metrics/history")
	if w.Code != http.StatusBadRequest {
		t.Errorf("no name = %d, want 400", w.Code)
	}
	if !strings.Contains(w.Body.String(), "/metrics/series") {
		t.Errorf("the 400 does not say where to look: %q", w.Body.String())
	}
}

// And as a CSV, which is the shortest path to a chart somebody else can read.
func TestHistoryCSV_IsAWellFormedTable(t *testing.T) {
	db := openDB(t)
	now := time.Now()
	seed(t, db, "bingle_db_file_bytes", now.Add(-30*time.Minute), 5*time.Minute, 6, func(i int) float64 { return float64(4096 * (i + 1)) })

	w := getPath(t, newPage(t, db), "/metrics/history.csv?name=bingle_db_file_bytes&hours=2")
	if w.Code != http.StatusOK {
		t.Fatalf("GET /metrics/history.csv = %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
		t.Errorf("Content-Type = %q", ct)
	}
	if cd := w.Header().Get("Content-Disposition"); !strings.Contains(cd, "bingle_db_file_bytes.csv") {
		t.Errorf("Content-Disposition = %q", cd)
	}

	rows, err := csv.NewReader(strings.NewReader(w.Body.String())).ReadAll()
	if err != nil {
		t.Fatalf("the CSV does not parse: %v", err)
	}
	if len(rows) != 7 { // a header and six points
		t.Fatalf("%d rows, want 7", len(rows))
	}
	if strings.Join(rows[0], ",") != "at,name,labels,value,min,max,n,bucket" {
		t.Errorf("header = %v", rows[0])
	}
	if rows[1][1] != "bingle_db_file_bytes" || rows[1][3] != "4096" {
		t.Errorf("first row = %v", rows[1])
	}
	if _, err := time.Parse(time.RFC3339, rows[1][0]); err != nil {
		t.Errorf("the time column is not RFC 3339: %q", rows[1][0])
	}
}

// What the history holds, for the page's picker.
func TestSeriesEndpoint_ListsWhatIsKept(t *testing.T) {
	db := openDB(t)
	now := time.Now()
	seed(t, db, "bingle_one", now.Add(-time.Hour), 5*time.Minute, 3, func(i int) float64 { return 1 })
	seed(t, db, "bingle_two", now.Add(-time.Hour), 5*time.Minute, 3, func(i int) float64 { return 2 })

	w := getPath(t, newPage(t, db), "/metrics/series")
	if w.Code != http.StatusOK {
		t.Fatalf("GET /metrics/series = %d", w.Code)
	}
	var body struct {
		Names      []string         `json:"names"`
		Rows       map[string]int64 `json:"rows"`
		Every      string           `json:"every"`
		RetainDays int              `json:"retain_days"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if strings.Join(body.Names, ",") != "bingle_one,bingle_two" {
		t.Errorf("names = %v", body.Names)
	}
	if body.Rows["samples"] != 6 {
		t.Errorf("rows = %v", body.Rows)
	}
	if body.Every != "5m0s" || body.RetainDays != 14 {
		t.Errorf("the settings came back as %q, %d", body.Every, body.RetainDays)
	}
}
