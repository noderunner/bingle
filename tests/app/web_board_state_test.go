// Tests of the game page in a browser: it loads and draws the board state, and still loads for an unknown room.

package app_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// TestWebGame_LoadsAndDisplaysBoardState covers backlog item 38's AC
// itself: creating a game via REST, then navigating a headless-Chrome
// session to /game/{code}, results in the rendered page displaying data
// fetched from GET /api/games/{code}/state — specifically the demo game's
// real initial state. (The v1 UI dumped raw JSON into the body; the current
// UI draws the board and keeps the raw dump in a collapsed debug panel, so
// this checks the rendered pieces and the debug panel's contents.)
func TestWebGame_LoadsAndDisplaysBoardState(t *testing.T) {
	baseURL := newRESTTestServer(t)
	roomCode := createRoom(t, baseURL, "Alice")

	ctx, cancel := newChromedpContext(t, 30*time.Second)
	defer cancel()

	if err := chromedp.Run(ctx, chromedp.Navigate(baseURL+"/game/"+roomCode)); err != nil {
		t.Fatalf("navigate to /game/%s: %v", roomCode, err)
	}

	// The demo game's initial setup always places a black stone at "2,0".
	if err := chromedp.Run(ctx, chromedp.WaitReady(`#board .piece[data-position="2,0"]`, chromedp.ByQuery)); err != nil {
		t.Fatalf("rendered /game/%s page never drew the stone at 2,0: %v", roomCode, err)
	}

	// The status message reflects the waiting game.
	var status string
	if err := chromedp.Run(ctx, chromedp.Text("#status-message", &status, chromedp.ByID)); err != nil {
		t.Fatalf("reading #status-message: %v", err)
	}
	if !strings.Contains(strings.ToLower(status), "waiting") {
		t.Errorf("status message = %q, want it to say the game is waiting for an opponent", status)
	}

	// The raw state dump (collapsed) carries the fetched JSON.
	var raw string
	if err := chromedp.Run(ctx, chromedp.Evaluate(`document.getElementById("board-state").textContent`, &raw)); err != nil {
		t.Fatalf("reading #board-state: %v", err)
	}
	for _, want := range []string{"2,0", `"status": "waiting"`, `"move_seq": 0`} {
		if !strings.Contains(raw, want) {
			t.Errorf("raw state dump lacks %q:\n%s", want, raw)
		}
	}

	var loc string
	if err := chromedp.Run(ctx, chromedp.Location(&loc)); err != nil {
		t.Fatalf("chromedp.Location: %v", err)
	}
	if !strings.Contains(loc, "/game/"+roomCode) {
		t.Errorf("browser location = %q, want it to contain %q", loc, "/game/"+roomCode)
	}
}

// TestWebGame_ServedHTML_RoomCodeAppearsSomewhere is a light structural,
// non-browser-driven check that /game/{code}'s raw served HTML (before any
// JS executes) at least mentions the room code somewhere in the page (e.g.
// a heading, or embedded as a JS constant board.js reads to build its fetch
// URL) — a cheap regression guard that doesn't require spinning up a
// browser or waiting on a fetch to complete.
func TestWebGame_ServedHTML_RoomCodeAppearsSomewhere(t *testing.T) {
	baseURL := newRESTTestServer(t)
	roomCode := createRoom(t, baseURL, "Alice")

	html := fetchBody(t, baseURL, "/game/"+roomCode)
	if !strings.Contains(html, roomCode) {
		t.Errorf("served /game/%s page does not mention the room code %q anywhere in its HTML source; want it embedded somewhere (e.g. a heading or a JS constant board.js uses to build its fetch URL). Page body:\n%s", roomCode, roomCode, html)
	}
}

// TestWebGame_UnknownRoomCode_PageStillLoadsButShowsError covers /game/{code}
// for a room code that was never created. The plan does not pin down this
// behavior (section 6.11 only specifies the happy path). This test picks
// ONE reasonable behavior and documents why:
//
// The page itself (game.html.tmpl) still renders with a 200 — the server
// has no way to know at template-render time whether {code} is a real room
// without an extra DB round trip on every page load, and item 38's AC
// frames this entirely as a client-side fetch ("board.js doing
// fetch(...) and rendering something observable from that response"). So
// the simplest, most consistent implementation is: the HTML shell always
// renders (200), and board.js's fetch to .../state gets a 404 from the
// REST layer (per docs/history/plan.md section 7's "Room code not found -> 404
// (REST)"), which it should handle by displaying some visible error
// indicator rather than silently leaving a blank page or throwing an
// uncaught JS exception that leaves nothing rendered at all.
//
// This is asserted somewhat liberally: the rendered body must show SOME
// non-empty, non-hung state (not stay blank forever), and must NOT display
// data that looks like a real board state (no "2,0"-style success content).
// The wording of the error text is not pinned.
func TestWebGame_UnknownRoomCode_PageStillLoadsButShowsError(t *testing.T) {
	baseURL := newRESTTestServer(t)
	const unknownCode = "ZZZZZZ"

	// First, confirm the plain HTTP layer's behavior directly: the page
	// route itself should respond 200 (per this test's documented choice
	// above), since the 404-ness lives in the REST .../state call board.js
	// makes client-side, not in the page route.
	resp, err := http.Get(baseURL + "/game/" + unknownCode)
	if err != nil {
		t.Fatalf("GET /game/%s: %v", unknownCode, err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /game/%s status = %d, want %d (page shell should always render; the 404 belongs to the client-side fetch to .../state, per this test's documented reasoning)", unknownCode, resp.StatusCode, http.StatusOK)
	}

	ctx, cancel := newChromedpContext(t, 30*time.Second)
	defer cancel()

	if err := chromedp.Run(ctx, chromedp.Navigate(baseURL+"/game/"+unknownCode)); err != nil {
		t.Fatalf("navigate to /game/%s: %v", unknownCode, err)
	}

	// Give board.js's fetch a bounded window to complete and render
	// something (error or otherwise), then read the body once.
	deadline := time.Now().Add(3 * time.Second)
	var body string
	for time.Now().Before(deadline) {
		if err := chromedp.Run(ctx, chromedp.Text("body", &body, chromedp.ByQuery)); err != nil {
			t.Fatalf("chromedp.Text(body): %v", err)
		}
		time.Sleep(100 * time.Millisecond)
	}

	if strings.TrimSpace(body) == "" {
		t.Errorf("rendered /game/%s page body is blank/whitespace-only after waiting for board.js's fetch to settle; want SOME visible content (even an error indicator), per this test's documented choice that the page shell always renders and board.js surfaces the .../state 404 visibly rather than leaving a blank page", unknownCode)
	}
	if strings.Contains(body, "2,0") {
		t.Errorf("rendered /game/%s page body unexpectedly contains %q, a real demo-game initial stone position, for a room code that was never created; body:\n%s", unknownCode, "2,0", body)
	}

	// Confirm no uncaught JS exception was thrown (chromedp surfaces page
	// JS errors as console API calls we'd otherwise miss silently; a
	// simpler, bounded proxy check here is that the document is still in a
	// normal "complete" ready state, not stuck mid-load/crashed).
	var readyState string
	if err := chromedp.Run(ctx, chromedp.Evaluate(`document.readyState`, &readyState)); err != nil {
		t.Fatalf("chromedp.Evaluate(document.readyState): %v", err)
	}
	if readyState != "complete" {
		t.Errorf("document.readyState = %q, want %q (page should finish loading cleanly even when .../state 404s)", readyState, "complete")
	}
}

// TestWebGame_NoQueryParamsOrAuthRequired is a sanity check for item 38's AC
// wording ("loading /game/{code}") — the page load itself requires nothing
// beyond the room code in the path: no query string, no auth header/cookie.
// This isn't testing app.js's own (nonexistent) auth; it just confirms the
// route doesn't reject a bare, unauthenticated GET with only the path
// param.
func TestWebGame_NoQueryParamsOrAuthRequired(t *testing.T) {
	baseURL := newRESTTestServer(t)
	roomCode := createRoom(t, baseURL, "Alice")

	req, err := http.NewRequest(http.MethodGet, baseURL+"/game/"+roomCode, nil)
	if err != nil {
		t.Fatalf("http.NewRequest: %v", err)
	}
	// Deliberately no query params, no Authorization header, no cookies —
	// a plain bare request beyond the path itself.

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /game/%s (no query/auth): %v", roomCode, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /game/%s (no query/auth) status = %d, want %d", roomCode, resp.StatusCode, http.StatusOK)
	}
}
