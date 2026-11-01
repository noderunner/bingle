// Tests of POST /api/client-error — the one observability route on the public
// listener, so what it refuses matters as much as what it keeps.

package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/noderunner/bingle/internal/app"
	"github.com/noderunner/bingle/internal/journal"
	"github.com/noderunner/bingle/internal/metrics"
)

// newErrorServer builds a server with the reporting endpoint on and returns
// the public base URL, the private handler and the database.
func newErrorServer(t *testing.T, opts ...app.Option) (string, http.Handler, *journal.DB) {
	t.Helper()
	db, err := journal.Open(t.TempDir() + "/bingle-metrics.db")
	if err != nil {
		t.Fatalf("journal.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	mx := metrics.New("9.9.9")
	all := append([]app.Option{
		app.WithMetrics(mx),
		app.WithJournal(db, 0),
		app.WithHistory(journal.PageOptions{DB: db, Version: "9.9.9", Every: 5 * time.Minute, RetainDays: 14}),
		app.WithClientErrors(),
	}, opts...)
	a, err := app.New(newTestConfig(t), all...)
	if err != nil {
		t.Fatalf("app.New: %v", err)
	}
	t.Cleanup(func() { _ = a.Close() })
	registerFixtures(t, a)

	srv := httptest.NewServer(a.Router())
	t.Cleanup(srv.Close)
	return srv.URL, a.MetricsHandler(), db
}

// postError sends one report and returns the status.
func postError(t *testing.T, baseURL string, report map[string]any) int {
	t.Helper()
	body, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	resp, err := http.Post(baseURL+"/api/client-error", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /api/client-error: %v", err)
	}
	_ = resp.Body.Close()
	return resp.StatusCode
}

// storedErrors reads the faults back off the private handler.
func storedErrors(t *testing.T, private http.Handler) []journal.ClientError {
	t.Helper()
	w := httptest.NewRecorder()
	private.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/metrics/client-errors", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("GET /metrics/client-errors = %d", w.Code)
	}
	var body struct {
		Errors []journal.ClientError `json:"errors"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding: %v (%s)", err, w.Body.String())
	}
	return body.Errors
}

// A report is kept, and the same one again is counted rather than repeated.
func TestClientError_IsKeptAndThenCounted(t *testing.T) {
	baseURL, private, _ := newErrorServer(t)

	report := map[string]any{
		"message": "Cannot read properties of null (reading 'board')",
		"stack":   "at draw (/static/js/game.js:120:7)",
		"page":    "/game/{code}",
		"version": "9.9.9",
		"browser": "Firefox 142 desktop",
	}
	if code := postError(t, baseURL, report); code != http.StatusNoContent {
		t.Fatalf("the first report = %d, want 204", code)
	}
	errs := storedErrors(t, private)
	if len(errs) != 1 || errs[0].Count != 1 {
		t.Fatalf("after one report: %+v", errs)
	}

	if code := postError(t, baseURL, report); code != http.StatusNoContent {
		t.Fatalf("the second report = %d", code)
	}
	errs = storedErrors(t, private)
	if len(errs) != 1 {
		t.Fatalf("the same fault twice made %d rows, want 1", len(errs))
	}
	if errs[0].Count != 2 {
		t.Errorf("the row counts %d, want 2", errs[0].Count)
	}
}

// Nothing a stranger posts becomes a metric label or reaches the row
// unchecked: this endpoint is on the public listener.
func TestClientError_NothingSecretReachesTheRow(t *testing.T) {
	baseURL, private, _ := newErrorServer(t)

	// Long enough to match the scrubber's rule (24 or more characters of
	// base64url), and deliberately not shaped like any real provider's key:
	// a fixture that looks like a credential is one a secret scanner has to
	// flag, and it cannot tell an invented one from a leaked one.
	const (
		token = "not-a-real-secret-only-a-fixture-0000"
		room  = "AB12CD"
	)
	postError(t, baseURL, map[string]any{
		"message": "failed for " + token + " at /game/" + room + " and /batches/" + room,
		"stack":   "at join (/game/" + room + "?name=Ana:1:2)\nat x (" + token + ")",
		"page":    "/game/{code}",
		"version": "9.9.9",
		"browser": "Firefox 142 desktop",
	})

	errs := storedErrors(t, private)
	if len(errs) != 1 {
		t.Fatalf("%d rows", len(errs))
	}
	row := errs[0]
	for what, secret := range map[string]string{
		"a token":       token,
		"the room code": room,
	} {
		if strings.Contains(row.Message, secret) {
			t.Errorf("the message carries %s: %q", what, row.Message)
		}
		if strings.Contains(row.Stack, secret) {
			t.Errorf("the stack carries %s: %q", what, row.Stack)
		}
	}
	if !strings.Contains(row.Message, "[redacted]") || !strings.Contains(row.Message, "/game/{code}") {
		t.Errorf("the message was not scrubbed as expected: %q", row.Message)
	}
}

// The page and the version are labels, so they are closed sets however hard
// somebody pushes.
func TestClientError_ThePageAndVersionAreClosedSets(t *testing.T) {
	baseURL, private, _ := newErrorServer(t)

	for i, c := range []struct{ page, version, wantPage, wantVersion string }{
		{page: "/game/{code}", version: "9.9.9", wantPage: "/game/{code}", wantVersion: "9.9.9"},
		{page: "/", version: "0.1.2-beta.3", wantPage: "/", wantVersion: "0.1.2-beta.3"},
		{page: "/invented/{anything}", version: "9.9.9", wantPage: "other", wantVersion: "9.9.9"},
		{page: strings.Repeat("x", 500), version: "9.9.9", wantPage: "other", wantVersion: "9.9.9"},
		{page: "/", version: "not a version", wantPage: "/", wantVersion: "unknown"},
		{page: "/", version: strings.Repeat("9", 200), wantPage: "/", wantVersion: "unknown"},
	} {
		postError(t, baseURL, map[string]any{
			"message": "fault number " + string(rune('a'+i)),
			"page":    c.page,
			"version": c.version,
		})
	}

	errs := storedErrors(t, private)
	if len(errs) != 6 {
		t.Fatalf("%d rows, want 6", len(errs))
	}
	pages := map[string]bool{}
	versions := map[string]bool{}
	for _, e := range errs {
		pages[e.Page] = true
		versions[e.Version] = true
	}
	for page := range pages {
		switch page {
		case "/", "/game/{code}", "other":
		default:
			t.Errorf("a page outside the closed set reached a row: %q", page)
		}
	}
	for v := range versions {
		switch v {
		case "9.9.9", "0.1.2-beta.3", "unknown":
		default:
			t.Errorf("a version outside the closed set reached a row: %q", v)
		}
	}
}

// A flood from one address is refused, so the endpoint cannot be used to fill
// the disk.
func TestClientError_AFloodIsRefused(t *testing.T) {
	baseURL, private, _ := newErrorServer(t)

	const tried = 60
	for i := 0; i < tried; i++ {
		// Each one a different fault, so dedup is not what stops them.
		if code := postError(t, baseURL, map[string]any{
			"message": "fault " + strings.Repeat("x", i%7) + string(rune('a'+i%26)) + string(rune('A'+i/26)),
			"page":    "/",
			"version": "9.9.9",
		}); code != http.StatusNoContent {
			// 204 whatever happens: a browser can do nothing with a refusal,
			// and telling a stranger which reports were kept is information
			// this endpoint has no reason to give out.
			t.Fatalf("report %d = %d, want 204", i, code)
		}
	}

	errs := storedErrors(t, private)
	if len(errs) == 0 {
		t.Fatal("every report was refused")
	}
	if len(errs) >= tried {
		t.Errorf("%d of %d reports from one address were kept: the flood was not refused", len(errs), tried)
	}
	t.Logf("%d of %d reports were kept before the limit bit", len(errs), tried)
}

// A body that is not a report, or has nothing in it, is dropped quietly.
func TestClientError_RubbishIsDroppedQuietly(t *testing.T) {
	baseURL, private, _ := newErrorServer(t)

	for _, body := range []string{``, `{`, `[]`, `"a string"`, `{"message":""}`, `{"message":"   "}`} {
		resp, err := http.Post(baseURL+"/api/client-error", "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatalf("POST %q: %v", body, err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusNoContent {
			t.Errorf("POST %q = %d, want 204", body, resp.StatusCode)
		}
	}
	if errs := storedErrors(t, private); len(errs) != 0 {
		t.Errorf("rubbish was stored: %+v", errs)
	}

	// An enormous body is cut off rather than read into memory.
	huge := `{"message":"` + strings.Repeat("x", 200000) + `","page":"/","version":"9.9.9"}`
	resp, err := http.Post(baseURL+"/api/client-error", "application/json", strings.NewReader(huge))
	if err != nil {
		t.Fatalf("POST a huge body: %v", err)
	}
	_ = resp.Body.Close()
	for _, e := range storedErrors(t, private) {
		if len(e.Message) > 1000 {
			t.Errorf("a message of %d characters was stored", len(e.Message))
		}
	}
}

// BINGLE_CLIENT_ERRORS=off removes the route altogether.
func TestClientError_OffMeansTheRouteIsNotThere(t *testing.T) {
	db, err := journal.Open(t.TempDir() + "/bingle-metrics.db")
	if err != nil {
		t.Fatalf("journal.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	// No app.WithClientErrors().
	a, err := app.New(newTestConfig(t), app.WithMetrics(metrics.New("9.9.9")), app.WithJournal(db, 0))
	if err != nil {
		t.Fatalf("app.New: %v", err)
	}
	registerFixtures(t, a)
	t.Cleanup(func() { _ = a.Close() })
	srv := httptest.NewServer(a.Router())
	t.Cleanup(srv.Close)

	resp, err := http.Post(srv.URL+"/api/client-error", "application/json", strings.NewReader(`{"message":"x"}`))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent {
		t.Error("the route answered with the reporting endpoint off")
	}

	n, err := db.Counts(context.Background())
	if err != nil {
		t.Fatalf("counting: %v", err)
	}
	if n["client_errors"] != 0 {
		t.Errorf("%d faults were stored with the endpoint off", n["client_errors"])
	}
}

// Reading the faults back is on the private listener, never the public one.
func TestClientError_AreNotReadableFromThePublicListener(t *testing.T) {
	baseURL, _, _ := newErrorServer(t)
	postError(t, baseURL, map[string]any{"message": "a fault", "page": "/", "version": "9.9.9"})

	for _, path := range []string{"/metrics/client-errors", "/api/client-error", "/api/client-errors"} {
		resp, err := http.Get(baseURL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		body := make([]byte, 512)
		n, _ := resp.Body.Read(body)
		_ = resp.Body.Close()
		if resp.StatusCode == http.StatusOK && strings.Contains(string(body[:n]), "a fault") {
			t.Errorf("GET %s on the public listener served the faults", path)
		}
	}
}
