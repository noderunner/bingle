// A browser test of the error reporter: a page throws on purpose, and the
// fault turns up — which is the only way to know that errors.js is wired to
// the events it claims to hook.

package app_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// readErrorRows reads the faults off the private handler.
func readErrorRows(t *testing.T, private http.Handler) []clientErrorRow {
	t.Helper()
	w := httptest.NewRecorder()
	private.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/metrics/client-errors", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("GET /metrics/client-errors = %d", w.Code)
	}
	var body struct {
		Errors []clientErrorRow `json:"errors"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	return body.Errors
}

// containsRoom reports whether text carries the room code.
func containsRoom(text, room string) bool {
	return room != "" && strings.Contains(text, room)
}

// waitForErrors polls the private handler until it holds at least n faults.
func waitForErrors(t *testing.T, private http.Handler, n int) []clientErrorRow {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		rows := readErrorRows(t, private)
		if len(rows) >= n {
			return rows
		}
		if time.Now().After(deadline) {
			t.Fatalf("the history holds %d faults after 15s, want %d", len(rows), n)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

type clientErrorRow struct {
	Count   int64  `json:"count"`
	Page    string `json:"page"`
	Version string `json:"version"`
	Message string `json:"message"`
	Stack   string `json:"stack"`
	Browser string `json:"browser"`
}

// A real browser on a real page, throwing on purpose: the fault arrives with
// the page as a route pattern, the release it happened on, and a count that
// goes up the second time rather than making a second row.
func TestWebClientErrors_AThrowOnAPageIsReported(t *testing.T) {
	baseURL, private, _ := newErrorServer(t)

	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()

	// A game page, so the room code is in the URL and must not come back in
	// the report. The throw is deferred so that evaluating it succeeds and
	// the error reaches window's "error" event, which is what errors.js
	// hooks.
	room := createRoom(t, baseURL, "Ana")
	throw := `setTimeout(() => { throw new Error("a deliberate fault in a test"); }, 0); true`

	if err := chromedp.Run(ctx,
		chromedp.Navigate(baseURL+"/game/"+room),
		chromedp.WaitVisible(`body`, chromedp.ByQuery),
		chromedp.Evaluate(throw, nil),
	); err != nil {
		t.Fatalf("throwing on the game page: %v", err)
	}

	rows := waitForErrors(t, private, 1)
	if len(rows) != 1 {
		t.Fatalf("%d faults, want 1: %+v", len(rows), rows)
	}
	row := rows[0]
	if row.Count != 1 {
		t.Errorf("the fault counts %d, want 1", row.Count)
	}
	if row.Message != "a deliberate fault in a test" {
		t.Errorf("message = %q", row.Message)
	}
	if row.Page != "/game/{code}" {
		t.Errorf("page = %q, want the route pattern", row.Page)
	}
	if row.Version == "" || row.Version == "unknown" {
		t.Errorf("version = %q, want the release the page was served by", row.Version)
	}
	if row.Browser == "" || row.Browser == "unknown desktop" {
		t.Logf("browser = %q", row.Browser)
	}
	// The page's own URL carries the room code, and the report must not.
	if containsRoom(row.Message, room) || containsRoom(row.Stack, room) {
		t.Errorf("the room code reached the row: %q / %q", row.Message, row.Stack)
	}

	// The same fault again is the same row, counted twice. A fresh page load
	// is needed because the reporter sends only a few per page.
	if err := chromedp.Run(ctx,
		chromedp.Navigate(baseURL+"/game/"+room),
		chromedp.WaitVisible(`body`, chromedp.ByQuery),
		chromedp.Evaluate(throw, nil),
	); err != nil {
		t.Fatalf("throwing again: %v", err)
	}

	deadline := time.Now().Add(15 * time.Second)
	for {
		rows = readErrorRows(t, private)
		total := int64(0)
		for _, r := range rows {
			total += r.Count
		}
		if total >= 2 {
			if len(rows) != 1 {
				t.Errorf("the same fault twice made %d rows, want 1: %+v", len(rows), rows)
			} else if rows[0].Count != 2 {
				t.Errorf("the row counts %d, want 2", rows[0].Count)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the second throw never arrived: %+v", rows)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// An unhandled promise rejection is the other half of what errors.js hooks,
// and the half that is easy to forget.
func TestWebClientErrors_AnUnhandledRejectionIsReported(t *testing.T) {
	baseURL, private, _ := newErrorServer(t)

	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()

	if err := chromedp.Run(ctx,
		chromedp.Navigate(baseURL+"/"),
		chromedp.WaitVisible(`body`, chromedp.ByQuery),
		chromedp.Evaluate(`Promise.reject(new Error("a deliberate rejection")); true`, nil),
	); err != nil {
		t.Fatalf("rejecting on the home page: %v", err)
	}

	rows := waitForErrors(t, private, 1)
	if rows[0].Message != "a deliberate rejection" {
		t.Errorf("message = %q", rows[0].Message)
	}
	if rows[0].Page != "/" {
		t.Errorf("page = %q, want /", rows[0].Page)
	}
}
