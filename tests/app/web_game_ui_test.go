// Browser-driven tests for the game page: rendering (pieces, colors, grid),
// live updates over WebSocket, and the interactive move-building UI
// (placing, stack moves, history, undo), all against a real headless Chrome.
//
// DOM contract these tests rely on (see internal/web/static/js):
//
//	#board svg                    the board
//	.piece[data-position,data-piece,data-color,data-top]   one per piece
//	.piece-body                   the piece's visible shape
//	.grid-line                    board lines ("points" style boards)
//	.cell[data-position]          a square; .clickable when it is a legal next click
//	button.hand-piece[data-piece] a placeable piece in the player's hand
//	#count-picker button[data-count]
//	#submit-move, #reset-move, #undo-step, #undo-move, #resign
//	#move-list button.mv[data-seq], #hist-prev/#hist-next/#hist-first/#hist-last
//	#status-message, #pending-move
package app_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

// openGameAs signs the browser in as name for room (the UI remembers who you
// are in localStorage) and opens the game page, waiting for the board.
func openGameAs(t *testing.T, ctx context.Context, baseURL, room, def, name string) {
	t.Helper()
	games, _ := json.Marshal([]map[string]any{{"code": room, "def": def, "name": name, "ts": time.Now().UnixMilli()}})
	nameJSON, _ := json.Marshal(name)
	if err := chromedp.Run(ctx,
		chromedp.Navigate(baseURL+"/healthz"),
		chromedp.Evaluate(fmt.Sprintf(`localStorage.setItem("bingle.games", %q); localStorage.setItem("bingle.name", %q); true`, games, nameJSON), nil),
		chromedp.Navigate(baseURL+"/game/"+room),
		chromedp.WaitReady("#board-svg, #card-table", chromedp.ByQuery),
	); err != nil {
		t.Fatalf("open /game/%s as %s: %v", room, name, err)
	}
}

// waitJS polls expr until it evaluates truthy, failing after timeout.
func waitJS(t *testing.T, ctx context.Context, what, expr string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last any
	for time.Now().Before(deadline) {
		var v bool
		if err := chromedp.Run(ctx, chromedp.Evaluate(`Boolean(`+expr+`)`, &v)); err == nil && v {
			return
		}
		last = expr
		time.Sleep(60 * time.Millisecond)
	}
	var body string
	_ = chromedp.Run(ctx, chromedp.Evaluate(`document.body.innerText.slice(0, 600)`, &body))
	t.Fatalf("timed out waiting for %s (%v); page text:\n%s", what, last, body)
}

// clickCell dispatches a click on the hit area of a board square.
func clickCell(t *testing.T, ctx context.Context, pos string) {
	t.Helper()
	js := fmt.Sprintf(`(function(){
		var el = document.querySelector('#board .cell[data-position=%q] .cell-hit');
		if (!el) return false;
		el.dispatchEvent(new MouseEvent('click', {bubbles: true}));
		return true;
	})()`, pos)
	var ok bool
	if err := chromedp.Run(ctx, chromedp.Evaluate(js, &ok)); err != nil || !ok {
		t.Fatalf("click cell %s: ok=%v err=%v", pos, ok, err)
	}
}

func clickSel(t *testing.T, ctx context.Context, sel string) {
	t.Helper()
	js := fmt.Sprintf(`(function(){ var el = document.querySelector(%q); if (!el || el.disabled) return false; el.click(); return true; })()`, sel)
	var ok bool
	if err := chromedp.Run(ctx, chromedp.Evaluate(js, &ok)); err != nil || !ok {
		t.Fatalf("click %s: ok=%v err=%v", sel, ok, err)
	}
}

func evalString(t *testing.T, ctx context.Context, expr string) string {
	t.Helper()
	var s string
	if err := chromedp.Run(ctx, chromedp.Evaluate(expr, &s)); err != nil {
		t.Fatalf("evaluate %s: %v", expr, err)
	}
	return s
}

func evalInt(t *testing.T, ctx context.Context, expr string) int {
	t.Helper()
	var n int
	if err := chromedp.Run(ctx, chromedp.Evaluate(expr, &n)); err != nil {
		t.Fatalf("evaluate %s: %v", expr, err)
	}
	return n
}

// piecesJS lists "position:color:piece" for every rendered piece.
const piecesJS = `Array.from(document.querySelectorAll("#board svg .piece")).map(function (el) {
	return el.getAttribute("data-position") + ":" + el.getAttribute("data-color") + ":" + el.getAttribute("data-piece");
}).sort().join(",")`

// ---- rendering --------------------------------------------------------------

func TestWebGame_RendersOnePiecePerOccupant(t *testing.T) {
	baseURL := newRESTTestServer(t)
	room := createRoom(t, baseURL, "Alice")

	ctx, cancel := newChromedpContext(t, 30*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.Navigate(baseURL+"/game/"+room), chromedp.WaitReady("#board-svg", chromedp.ByID)); err != nil {
		t.Fatal(err)
	}

	want := "2,0:black:stone,2,8:white:stone,4,0:black:stone,4,8:white:stone,6,0:black:stone,6,8:white:stone"
	if got := evalString(t, ctx, piecesJS); got != want {
		t.Errorf("rendered pieces = %s\nwant            = %s", got, want)
	}
	// Not one per grid cell.
	if n := evalInt(t, ctx, `document.querySelectorAll("#board svg .piece").length`); n != 6 {
		t.Errorf("piece count = %d, want 6", n)
	}
}

func TestWebGame_PieceColorsAreDistinguishable(t *testing.T) {
	baseURL := newRESTTestServer(t)
	room := createRoom(t, baseURL, "Bob")

	ctx, cancel := newChromedpContext(t, 30*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.Navigate(baseURL+"/game/"+room), chromedp.WaitReady("#board-svg", chromedp.ByID)); err != nil {
		t.Fatal(err)
	}

	var fills map[string][]string
	js := `(function () {
		var out = {};
		document.querySelectorAll("#board svg .piece").forEach(function (el) {
			var c = el.getAttribute("data-color");
			var body = el.querySelector(".piece-body");
			var cs = window.getComputedStyle(body);
			(out[c] = out[c] || []).push(cs.fill + "|" + cs.stroke);
		});
		return out;
	})()`
	if err := chromedp.Run(ctx, chromedp.Evaluate(js, &fills)); err != nil {
		t.Fatal(err)
	}
	if len(fills["black"]) != 3 || len(fills["white"]) != 3 {
		t.Fatalf("fills by color = %v, want 3 black and 3 white", fills)
	}
	if fills["black"][0] == fills["white"][0] {
		t.Errorf("black and white pieces are styled identically: %q", fills["black"][0])
	}
	// Every piece must have a visible outline: a stroke that is not the
	// same colour as its own fill.
	for color, list := range fills {
		for _, fs := range list {
			parts := strings.SplitN(fs, "|", 2)
			if parts[0] == parts[1] {
				t.Errorf("%s piece has stroke == fill (%s): invisible outline", color, parts[0])
			}
		}
	}
}

func TestWebGame_BoardHasVisibleGridLines(t *testing.T) {
	baseURL := newRESTTestServer(t)
	room := createRoom(t, baseURL, "Alice")

	ctx, cancel := newChromedpContext(t, 20*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.Navigate(baseURL+"/game/"+room), chromedp.WaitReady("#board-svg", chromedp.ByID)); err != nil {
		t.Fatal(err)
	}

	var strokes []string
	js := `Array.from(document.querySelectorAll("#board svg line.grid-line")).map(function (el) { return window.getComputedStyle(el).stroke; })`
	if err := chromedp.Run(ctx, chromedp.Evaluate(js, &strokes)); err != nil {
		t.Fatal(err)
	}
	// The fixture is a 9x9 "points" board: 9 vertical + 9 horizontal lines.
	if len(strokes) != 18 {
		t.Fatalf("grid lines = %d, want 18", len(strokes))
	}
	for i, s := range strokes {
		if s == "" || s == "none" || s == "rgb(255, 255, 255)" {
			t.Errorf("grid line %d has an invisible stroke %q", i, s)
		}
	}
}

func TestWebGame_TakBoardShowsCoordinatesAndStacks(t *testing.T) {
	baseURL := newRESTTestServer(t)
	room := newTakRoom(t, baseURL, "tak-5x5", "Alice")
	joinRoom(t, baseURL, room, "Bob")
	// c3 gets a stack of two: Alice places (black stone), Bob places, then
	// stacks are made in the middle game.
	for _, m := range []struct{ who, mv string }{
		{"Alice", "c3"}, {"Bob", "c4"}, // opening: c3 black flat, c4 white flat
		{"Alice", "c4-"}, // white's c4 stone steps onto c3: stack [black, white]
	} {
		if code, out := moveNotation(t, baseURL, room, m.who, m.mv); code != 200 {
			t.Fatalf("%s %s: %d %v", m.who, m.mv, code, out)
		}
	}

	ctx, cancel := newChromedpContext(t, 30*time.Second)
	defer cancel()
	openGameAs(t, ctx, baseURL, room, "tak-5x5", "Alice")

	if n := evalInt(t, ctx, `document.querySelectorAll("#board .coord").length`); n != 20 {
		t.Errorf("coordinate labels = %d, want 20 (5 files and 5 ranks, on both sides)", n)
	}
	if n := evalInt(t, ctx, `document.querySelectorAll('#board .piece[data-position="c3"]').length`); n != 2 {
		t.Errorf("pieces drawn at c3 = %d, want a stack of 2", n)
	}
	if got := evalString(t, ctx, `document.querySelector('#board .cell[data-position="c3"] .height-badge-text').textContent`); got != "2" {
		t.Errorf("height badge = %q, want 2", got)
	}
	// Only the top of each stack is flagged as the top.
	if got := evalString(t, ctx, `Array.from(document.querySelectorAll('#board .piece[data-position="c3"]')).map(function(e){return e.getAttribute("data-top")+":"+e.getAttribute("data-color")}).join(",")`); got != "false:black,true:white" {
		t.Errorf("c3 stack (bottom to top) = %q, want false:black,true:white", got)
	}
}

func TestWebGame_UnknownRoomShowsErrorWithoutJSExceptions(t *testing.T) {
	baseURL := newRESTTestServer(t)

	ctx, cancel := newChromedpContext(t, 30*time.Second)
	defer cancel()

	var mu sync.Mutex
	var jsErrors []string
	chromedp.ListenTarget(ctx, func(ev any) {
		if e, ok := ev.(*runtime.EventExceptionThrown); ok && e.ExceptionDetails != nil {
			mu.Lock()
			jsErrors = append(jsErrors, e.ExceptionDetails.Error())
			mu.Unlock()
		}
	})

	if err := chromedp.Run(ctx, chromedp.Navigate(baseURL+"/game/QNVWXY"), chromedp.WaitVisible("#board-error", chromedp.ByID)); err != nil {
		t.Fatalf("expected an error panel for an unknown room: %v", err)
	}
	if n := evalInt(t, ctx, `document.querySelectorAll("#board svg .piece").length`); n != 0 {
		t.Errorf("drew %d pieces for an unknown room", n)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(jsErrors) > 0 {
		t.Errorf("uncaught JS exceptions: %v", jsErrors)
	}
}

// ---- live updates -------------------------------------------------------------

func TestWebGame_OpensWebSocketOnLoad(t *testing.T) {
	baseURL := newRESTTestServer(t)
	room := createRoom(t, baseURL, "Alice")

	ctx, cancel := newChromedpContext(t, 30*time.Second)
	defer cancel()

	var mu sync.Mutex
	var urls []string
	chromedp.ListenTarget(ctx, func(ev any) {
		if e, ok := ev.(*network.EventWebSocketCreated); ok {
			mu.Lock()
			urls = append(urls, e.URL)
			mu.Unlock()
		}
	})
	if err := chromedp.Run(ctx, network.Enable(), chromedp.Navigate(baseURL+"/game/"+room), chromedp.WaitReady("#board-svg", chromedp.ByID)); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(urls)
		mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(urls) == 0 {
		t.Fatal("page never opened a WebSocket")
	}
	if !strings.Contains(urls[0], "/ws/"+room) || !strings.Contains(urls[0], "role=spectator") {
		t.Errorf("websocket url = %q, want a spectator connection to /ws/%s", urls[0], room)
	}
}

func TestWebGame_LiveUpdateAcrossTabsWithoutReload(t *testing.T) {
	baseURL := newRESTTestServer(t)
	room := newTakRoom(t, baseURL, "test-freeplay", "Alice")
	joinRoom(t, baseURL, room, "Bob")

	ctx1, cancel1 := newChromedpContext(t, 40*time.Second)
	defer cancel1()
	ctx2, cancel2 := newChromedpContext(t, 40*time.Second)
	defer cancel2()

	for i, ctx := range []context.Context{ctx1, ctx2} {
		if err := chromedp.Run(ctx, chromedp.Navigate(baseURL+"/game/"+room), chromedp.WaitReady("#board-svg", chromedp.ByID)); err != nil {
			t.Fatalf("tab %d: %v", i+1, err)
		}
	}
	// A marker proves tab 2 is never reloaded.
	if err := chromedp.Run(ctx2, chromedp.Evaluate(`window.__tab2marker = "alive"; true`, nil)); err != nil {
		t.Fatal(err)
	}
	// Let both sockets connect before the move is made.
	waitJS(t, ctx2, "live badge", `document.getElementById("conn-badge") && document.getElementById("conn-badge").textContent === "live"`, 10*time.Second)

	if code, out := moveNotation(t, baseURL, room, "Alice", "2,0-2,1"); code != 200 {
		t.Fatalf("move: %d %v", code, out)
	}

	for i, ctx := range []context.Context{ctx1, ctx2} {
		waitJS(t, ctx, fmt.Sprintf("tab %d to show the stone at 2,1", i+1),
			`document.querySelector('#board .piece[data-position="2,1"]') && !document.querySelector('#board .piece[data-position="2,0"]')`, 10*time.Second)
	}
	if got := evalString(t, ctx2, `window.__tab2marker || "reloaded"`); got != "alive" {
		t.Errorf("tab 2 was reloaded instead of updating in place")
	}
	// The move appears in the move list and the turn passed to Bob.
	waitJS(t, ctx2, "move list entry", `document.querySelector('#move-list button.mv[data-seq="1"]')`, 5*time.Second)
	if got := evalString(t, ctx2, `document.querySelector('#seat-2').className`); !strings.Contains(got, "to-move") {
		t.Errorf("seat 2 strip classes = %q, want it marked to-move", got)
	}
}

// ---- interaction ------------------------------------------------------------------

func TestWebGame_ClickToMoveAPiece(t *testing.T) {
	baseURL := newRESTTestServer(t)
	room := newTakRoom(t, baseURL, "test-freeplay", "Alice")
	joinRoom(t, baseURL, room, "Bob")

	ctx, cancel := newChromedpContext(t, 40*time.Second)
	defer cancel()
	openGameAs(t, ctx, baseURL, room, "test-freeplay", "Alice")

	waitJS(t, ctx, "my turn", `document.getElementById("status-message").textContent.indexOf("Your turn") === 0`, 10*time.Second)
	// Alice (black) can start a move from any of her stones.
	waitJS(t, ctx, "clickable stones", `document.querySelectorAll('#board .cell.clickable').length === 3`, 5*time.Second)

	clickCell(t, ctx, "4,0")
	waitJS(t, ctx, "destinations", `document.querySelector('#board .cell.clickable[data-position="4,1"]')`, 5*time.Second)
	if n := evalInt(t, ctx, `document.querySelectorAll('#board .cell.clickable').length`); n != 3 {
		t.Errorf("clickable destinations = %d, want 3 (left, right, down from 4,0)", n)
	}
	clickCell(t, ctx, "4,1")

	// The move is complete: nothing is sent until it is submitted.
	waitJS(t, ctx, "submit button", `document.getElementById("submit-move")`, 5*time.Second)
	var hist []map[string]any
	doJSON(t, "GET", baseURL+"/api/games/"+room+"/moves", nil, &hist)
	if len(hist) != 0 {
		t.Fatalf("move was sent before Submit: %v", hist)
	}

	clickSel(t, ctx, "#submit-move")
	// (The preview already draws the stone at 4,1, so wait for the move to
	// show up in the move list, which only happens once it is recorded.)
	waitJS(t, ctx, "the move in the move list", `document.querySelector('#move-list button.mv[data-seq="1"]')`, 10*time.Second)
	doJSON(t, "GET", baseURL+"/api/games/"+room+"/moves", nil, &hist)
	if len(hist) != 1 || hist[0]["notation"] != "4,0-4,1" {
		t.Errorf("history = %v, want one move 4,0-4,1", hist)
	}
	waitJS(t, ctx, "waiting message", `document.getElementById("status-message").textContent.indexOf("Waiting for Bob") === 0`, 5*time.Second)
}

func TestWebGame_TakPlacementAndStackMoveByClicking(t *testing.T) {
	baseURL := newRESTTestServer(t)
	room := newTakRoom(t, baseURL, "tak-5x5", "Alice")
	joinRoom(t, baseURL, room, "Bob")
	// Opening plies, then a few quiet moves.
	script := []struct{ who, mv string }{
		{"Alice", "e5"}, // opening: black flat on e5
		{"Bob", "a1"},   // opening: white flat on a1
		{"Alice", "b1"}, // white flat b1
		{"Bob", "e4"},   // black flat e4
	}
	for _, m := range script {
		if code, out := moveNotation(t, baseURL, room, m.who, m.mv); code != 200 {
			t.Fatalf("%s %s: %d %v", m.who, m.mv, code, out)
		}
	}
	// Alice (white) now owns a1 (flat) and b1 (flat). It is her move.

	ctx, cancel := newChromedpContext(t, 40*time.Second)
	defer cancel()
	openGameAs(t, ctx, baseURL, room, "tak-5x5", "Alice")
	waitJS(t, ctx, "my turn", `document.getElementById("status-message").textContent.indexOf("Your turn") === 0`, 10*time.Second)

	// Hand: flat/wall pieces and the capstone are choosable.
	waitJS(t, ctx, "hand pieces", `document.querySelectorAll('#hand-white button.hand-piece.choosable').length === 3`, 5*time.Second)

	// 1) Place a standing stone on c3: wall button, then the square.
	clickSel(t, ctx, `#hand-white button.hand-piece[data-piece="wall"]`)
	waitJS(t, ctx, "empty squares become targets", `document.querySelectorAll('#board .cell.clickable').length === 21`, 5*time.Second)
	clickCell(t, ctx, "c3")
	waitJS(t, ctx, "submit for the placement", `document.getElementById("submit-move") && document.getElementById("submit-move").textContent.indexOf("Sc3") >= 0`, 5*time.Second)
	// Preview shows the wall before it is committed.
	waitJS(t, ctx, "ghost preview of the wall", `document.querySelector('#board .piece[data-position="c3"][data-piece="wall"]')`, 5*time.Second)
	clickSel(t, ctx, "#reset-move")
	waitJS(t, ctx, "wall gone after reset", `!document.querySelector('#board .piece[data-position="c3"]')`, 5*time.Second)

	// 2) Move the b1 stone onto a1: a stack move of one piece.
	clickCell(t, ctx, "b1")
	waitJS(t, ctx, "a1 offered as destination", `document.querySelector('#board .cell.clickable[data-position="a1"]')`, 5*time.Second)
	clickCell(t, ctx, "a1")
	waitJS(t, ctx, "submit b1<", `document.getElementById("submit-move") && document.getElementById("submit-move").textContent.indexOf("b1<") >= 0`, 5*time.Second)
	clickSel(t, ctx, "#submit-move")

	waitJS(t, ctx, "the move in the move list", `document.querySelector('#move-list button.mv[data-seq="5"]')`, 10*time.Second)
	waitJS(t, ctx, "stack of two at a1", `document.querySelectorAll('#board .piece[data-position="a1"]').length === 2`, 10*time.Second)
	var st restState
	doJSON(t, "GET", baseURL+"/api/games/"+room+"/state", nil, &st)
	if len(st.State.Cells["a1"]) != 2 || st.State.Cells["a1"][1].Color != "white" {
		t.Errorf("a1 = %+v, want white on top of the original white flat", st.State.Cells["a1"])
	}

	// 3) Bob moves; then Alice picks a count: a1 holds 2, so a count picker
	// appears with 1 and 2.
	if code, out := moveNotation(t, baseURL, room, "Bob", "d4"); code != 200 {
		t.Fatalf("Bob d4: %d %v", code, out)
	}
	waitJS(t, ctx, "my turn again", `document.getElementById("status-message").textContent.indexOf("Your turn") === 0`, 10*time.Second)
	clickCell(t, ctx, "a1")
	waitJS(t, ctx, "count picker", `document.querySelectorAll('#count-picker button').length === 2`, 5*time.Second)
	clickSel(t, ctx, `#count-picker button[data-count="2"]`)
	// Only the squares along a line from a1 are offered, first square only.
	waitJS(t, ctx, "adjacent squares offered", `document.querySelector('#board .cell.clickable[data-position="b1"]') && document.querySelector('#board .cell.clickable[data-position="a2"]')`, 5*time.Second)
	clickCell(t, ctx, "a2") // drop 1 on a2
	clickCell(t, ctx, "a3") // drop 1 on a3
	waitJS(t, ctx, "submit 2a1+11", `document.getElementById("submit-move") && document.getElementById("submit-move").textContent.indexOf("2a1+11") >= 0`, 5*time.Second)

	// Back one step removes the last drop; replaying it works.
	clickSel(t, ctx, "#undo-step")
	waitJS(t, ctx, "submit button gone", `!document.getElementById("submit-move")`, 5*time.Second)
	clickCell(t, ctx, "a3")
	waitJS(t, ctx, "submit again", `document.getElementById("submit-move")`, 5*time.Second)
	clickSel(t, ctx, "#submit-move")

	waitJS(t, ctx, "the move in the move list", `document.querySelector('#move-list button.mv[data-seq="7"]')`, 10*time.Second)
	waitJS(t, ctx, "pieces spread over a2 and a3",
		`document.querySelectorAll('#board .piece[data-position="a2"]').length === 1 && document.querySelectorAll('#board .piece[data-position="a3"]').length === 1 && document.querySelectorAll('#board .piece[data-position="a1"]').length === 0`, 10*time.Second)
}

func TestWebGame_HistoryNavigationAndUndo(t *testing.T) {
	baseURL := newRESTTestServer(t)
	room := newTakRoom(t, baseURL, "test-freeplay", "Alice")
	joinRoom(t, baseURL, room, "Bob")
	for _, m := range []struct{ who, mv string }{{"Alice", "2,0-2,1"}, {"Bob", "2,8-2,7"}, {"Alice", "4,0-4,1"}} {
		if code, out := moveNotation(t, baseURL, room, m.who, m.mv); code != 200 {
			t.Fatalf("%s %s: %d %v", m.who, m.mv, code, out)
		}
	}

	ctx, cancel := newChromedpContext(t, 40*time.Second)
	defer cancel()
	// Bob's perspective: it is his turn, so he can also undo his own move... he
	// has made one move and Alice has moved since, so there is nothing to
	// undo: the button is not offered.
	openGameAs(t, ctx, baseURL, room, "test-freeplay", "Bob")
	waitJS(t, ctx, "three moves listed", `document.querySelectorAll('#move-list button.mv').length === 3`, 10*time.Second)
	if got := evalString(t, ctx, `String(document.getElementById("undo-move") === null)`); got != "true" {
		t.Errorf("undo should not be offered to Bob (Alice moved after him)")
	}

	// Jump to move 1: only Alice's first move has been played.
	clickSel(t, ctx, `#move-list button.mv[data-seq="1"]`)
	waitJS(t, ctx, "history view", `document.getElementById("return-live") && document.querySelector('#board .piece[data-position="2,1"]') && document.querySelector('#board .piece[data-position="4,0"]') && document.querySelector('#board .piece[data-position="2,8"]')`, 10*time.Second)
	if n := evalInt(t, ctx, `document.querySelectorAll('#board .cell.clickable').length`); n != 0 {
		t.Errorf("board is interactive while viewing history (%d clickable cells)", n)
	}
	// Step forward with the controls and return to live.
	clickSel(t, ctx, "#hist-next")
	waitJS(t, ctx, "after move 2", `document.querySelector('#board .piece[data-position="2,7"]')`, 10*time.Second)
	clickSel(t, ctx, "#hist-last")
	waitJS(t, ctx, "live again", `!document.getElementById("return-live") && document.querySelector('#board .piece[data-position="4,1"]')`, 10*time.Second)

	// Alice undoes her last move (two-step confirmation).
	ctxA, cancelA := newChromedpContext(t, 40*time.Second)
	defer cancelA()
	openGameAs(t, ctxA, baseURL, room, "test-freeplay", "Alice")
	waitJS(t, ctxA, "undo offered", `document.getElementById("undo-move") && !document.getElementById("undo-move").disabled`, 10*time.Second)
	clickSel(t, ctxA, "#undo-move")
	waitJS(t, ctxA, "confirmation", `document.getElementById("undo-move").textContent.indexOf("Really") >= 0`, 5*time.Second)
	clickSel(t, ctxA, "#undo-move")
	waitJS(t, ctxA, "move taken back", `document.querySelectorAll('#move-list button.mv').length === 2 && document.querySelector('#board .piece[data-position="4,0"]')`, 10*time.Second)

	// Bob's open tab sees the undo live.
	waitJS(t, ctx, "Bob's tab follows the undo", `document.querySelectorAll('#move-list button.mv').length === 2 && document.querySelector('#board .piece[data-position="4,0"]')`, 10*time.Second)
}

func TestWebGame_TitleAnnouncesYourTurn(t *testing.T) {
	baseURL := newRESTTestServer(t)
	room := newTakRoom(t, baseURL, "tak-3x3", "Alice")
	joinRoom(t, baseURL, room, "Bob")
	moveNotation(t, baseURL, room, "Alice", "a1")

	ctx, cancel := newChromedpContext(t, 30*time.Second)
	defer cancel()
	openGameAs(t, ctx, baseURL, room, "tak-3x3", "Bob")
	waitJS(t, ctx, "turn marker in the title", `document.title.indexOf("Your turn") >= 0`, 10*time.Second)
}

func TestWebGame_ResignShowsResult(t *testing.T) {
	baseURL := newRESTTestServer(t)
	room := newTakRoom(t, baseURL, "tak-3x3", "Alice")
	joinRoom(t, baseURL, room, "Bob")

	ctx, cancel := newChromedpContext(t, 30*time.Second)
	defer cancel()
	openGameAs(t, ctx, baseURL, room, "tak-3x3", "Alice")
	waitJS(t, ctx, "resign button", `document.getElementById("resign") && !document.getElementById("resign").disabled`, 10*time.Second)
	clickSel(t, ctx, "#resign")
	clickSel(t, ctx, "#resign")
	waitJS(t, ctx, "result message", `document.getElementById("status-message").textContent.indexOf("won by resignation") >= 0`, 10*time.Second)
}

// ---- lobby -----------------------------------------------------------------------

func TestWebLobby_CreateAgainstBotOpensPlayableGame(t *testing.T) {
	baseURL := newRESTTestServer(t)

	ctx, cancel := newChromedpContext(t, 40*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx,
		chromedp.Navigate(baseURL+"/"),
		chromedp.WaitVisible(`.game-card[data-family="tak-5x5"]`, chromedp.ByQuery),
	); err != nil {
		t.Fatal(err)
	}
	// The setup of a game shows once the game is picked from the gallery.
	if evalBool(t, ctx, `!document.getElementById("game-setup").hidden`) {
		t.Error("the setup form shows before a game was picked")
	}
	clickSel(t, ctx, `.game-card[data-family="tak-5x5"]`)
	if err := chromedp.Run(ctx, chromedp.WaitVisible("#create-game-form", chromedp.ByID)); err != nil {
		t.Fatal(err)
	}
	// Games are grouped by family and Tak is the default.
	waitJS(t, ctx, "options grouped", `document.querySelector('#game_def_id optgroup[label="Tak"]')`, 10*time.Second)
	if got := evalString(t, ctx, `document.getElementById("game_def_id").value`); got != "tak-5x5" {
		t.Errorf("default game = %q, want tak-5x5", got)
	}
	// Bot choices were fetched from the server.
	waitJS(t, ctx, "bot choices", `document.querySelector('input[name="opponent"][value="tactical"]')`, 10*time.Second)

	if err := chromedp.Run(ctx,
		chromedp.SendKeys("#create_player_name", "Zoe", chromedp.ByID),
		chromedp.Evaluate(`document.querySelector('input[name="opponent"][value="random"]').click(); true`, nil),
		chromedp.Evaluate(`document.querySelector('#create-game-form button[type=submit]').click(); true`, nil),
	); err != nil {
		t.Fatal(err)
	}
	pollLocation(t, ctx, gamePageURLPattern)
	waitJS(t, ctx, "board", `document.querySelector("#board svg")`, 10*time.Second)
	waitJS(t, ctx, "my turn against the bot", `document.getElementById("status-message").textContent.indexOf("Opening") === 0`, 10*time.Second)
	// The bot is seated.
	if got := evalString(t, ctx, `[...document.querySelectorAll(".seat-strip .name")].map(e => e.textContent).join(",")`); !strings.Contains(got, "Bot (Random)") {
		t.Errorf("players = %q, want the bot listed", got)
	}
}

func TestWebGame_AnalysisModeExploresWithoutPlaying(t *testing.T) {
	baseURL := newRESTTestServer(t)
	room := newTakRoom(t, baseURL, "tak-3x3", "Alice")
	joinRoom(t, baseURL, room, "Bob")
	for _, m := range []struct{ who, mv string }{{"Alice", "a1"}, {"Bob", "c3"}} {
		if code, out := moveNotation(t, baseURL, room, m.who, m.mv); code != 200 {
			t.Fatalf("%s %s: %d %v", m.who, m.mv, code, out)
		}
	}

	ctx, cancel := newChromedpContext(t, 40*time.Second)
	defer cancel()
	// Bob has just moved, so it is Alice's turn: Bob can still analyze.
	openGameAs(t, ctx, baseURL, room, "tak-3x3", "Bob")
	waitJS(t, ctx, "analyze button", `document.getElementById("analyze") && !document.getElementById("analyze").disabled`, 10*time.Second)
	clickSel(t, ctx, "#analyze")
	waitJS(t, ctx, "analysis banner", `document.getElementById("exit-analysis") && document.getElementById("status-message").textContent.indexOf("Analysis") === 0`, 10*time.Second)

	// White (seat 1) moves in the analysis: pick a flat from white's hand and
	// place it on b2.
	waitJS(t, ctx, "white's hand is choosable", `document.querySelector('#hand-white button.hand-piece[data-piece="flat"]:not([disabled])')`, 10*time.Second)
	clickSel(t, ctx, `#hand-white button.hand-piece[data-piece="flat"]`)
	clickCell(t, ctx, "b2")
	waitJS(t, ctx, "variation shows b2", `document.getElementById("variation") && document.getElementById("variation").textContent.indexOf("b2") >= 0`, 10*time.Second)
	waitJS(t, ctx, "piece drawn at b2", `document.querySelector('#board .piece[data-position="b2"]')`, 5*time.Second)

	// Now it is black's move in the variation, and the real game is untouched.
	waitJS(t, ctx, "black to move", `document.getElementById("status-message").textContent.indexOf("Black") >= 0`, 5*time.Second)
	var st restState
	doJSON(t, "GET", baseURL+"/api/games/"+room+"/state", nil, &st)
	if st.MoveSeq != 2 || len(st.State.Cells) != 2 {
		t.Errorf("analysis changed the real game: move_seq=%d cells=%d", st.MoveSeq, len(st.State.Cells))
	}

	// Take the move back, then leave.
	clickSel(t, ctx, "#analysis-back")
	waitJS(t, ctx, "variation cleared", `!document.querySelector('#board .piece[data-position="b2"]')`, 10*time.Second)
	clickSel(t, ctx, "#exit-analysis")
	waitJS(t, ctx, "back to the live game", `!document.getElementById("exit-analysis")`, 5*time.Second)
}

func TestWebGame_RematchAgainstBotSwapsSides(t *testing.T) {
	baseURL := newRESTTestServer(t)
	var created struct {
		RoomCode string `json:"room_code"`
	}
	if code := doJSON(t, "POST", baseURL+"/api/games",
		map[string]any{"game_def_id": "tak-3x3", "player_name": "Zoe", "opponent": "random"}, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}

	ctx, cancel := newChromedpContext(t, 40*time.Second)
	defer cancel()
	openGameAs(t, ctx, baseURL, created.RoomCode, "tak-3x3", "Zoe")
	waitJS(t, ctx, "resign", `document.getElementById("resign") && !document.getElementById("resign").disabled`, 10*time.Second)
	clickSel(t, ctx, "#resign")
	clickSel(t, ctx, "#resign")
	waitJS(t, ctx, "rematch button", `document.getElementById("rematch")`, 10*time.Second)
	clickSel(t, ctx, "#rematch")

	pollLocation(t, ctx, gamePageURLPattern)
	waitJS(t, ctx, "new game loaded", `document.getElementById("status-message") && document.getElementById("board-svg")`, 10*time.Second)
	// Zoe now plays second: the bot (white) has already made the first move.
	waitJS(t, ctx, "my turn after the bot's opening", `document.getElementById("status-message").textContent.indexOf("Opening") === 0`, 10*time.Second)
	if n := evalInt(t, ctx, `document.querySelectorAll("#board svg .piece").length`); n != 1 {
		t.Errorf("pieces on the board = %d, want the bot's single opening stone", n)
	}
	if got := evalString(t, ctx, `document.querySelector('#seat-2 .name').textContent`); got != "Zoe" {
		t.Errorf("seat 2 = %q, want Zoe", got)
	}
}

func TestWebGame_IdentityGateLetsANewcomerJoin(t *testing.T) {
	baseURL := newRESTTestServer(t)
	room := newTakRoom(t, baseURL, "tak-3x3", "Alice")

	ctx, cancel := newChromedpContext(t, 40*time.Second)
	defer cancel()
	// A fresh browser: nobody has told the page who this is.
	if err := chromedp.Run(ctx,
		chromedp.Navigate(baseURL+"/game/"+room),
		chromedp.WaitVisible("#identity-gate", chromedp.ByID),
	); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "waiting message", `document.getElementById("status-message").textContent.indexOf("Waiting for an opponent") === 0`, 5*time.Second)

	if err := chromedp.Run(ctx,
		chromedp.SendKeys("#gate-name", "Carol", chromedp.ByID),
		chromedp.Click("#gate-join", chromedp.ByID),
	); err != nil {
		t.Fatal(err)
	}
	// Carol takes the open seat and the game becomes active.
	waitJS(t, ctx, "gate gone and Carol seated", `!document.getElementById("identity-gate") && [...document.querySelectorAll(".seat-strip .name")].some(e => e.textContent.indexOf("Carol") >= 0)`, 10*time.Second)
	waitJS(t, ctx, "opening prompt for Carol? no: Alice (seat 1) moves first", `document.getElementById("status-message").textContent.indexOf("Waiting for Alice") === 0`, 10*time.Second)

	var game struct {
		Status string                  `json:"status"`
		Seats  []struct{ Name string } `json:"seats"`
	}
	doJSON(t, "GET", baseURL+"/api/games/"+room, nil, &game)
	if game.Status != "active" || len(game.Seats) != 2 || game.Seats[1].Name != "Carol" {
		t.Errorf("game = %+v, want active with Carol in seat 2", game)
	}
}

func TestWebGame_SpectatorSeesBoardButCannotMove(t *testing.T) {
	baseURL := newRESTTestServer(t)
	room := newTakRoom(t, baseURL, "tak-3x3", "Alice")
	joinRoom(t, baseURL, room, "Bob")
	moveNotation(t, baseURL, room, "Alice", "a1")

	ctx, cancel := newChromedpContext(t, 40*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx,
		chromedp.Navigate(baseURL+"/game/"+room),
		chromedp.WaitVisible("#identity-gate", chromedp.ByID),
		chromedp.Click("#gate-watch", chromedp.ByID),
	); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "spectator view", `!document.getElementById("identity-gate") && document.querySelector('#board .piece[data-position="a1"]')`, 10*time.Second)
	if n := evalInt(t, ctx, `document.querySelectorAll('#board .cell.clickable').length`); n != 0 {
		t.Errorf("a spectator sees %d clickable squares", n)
	}
	if got := evalString(t, ctx, `String(!!document.getElementById("undo-move"))`); got != "false" {
		t.Error("a spectator should not get undo/resign controls")
	}
	// Moves made by the players still arrive live.
	moveNotation(t, baseURL, room, "Bob", "c3")
	waitJS(t, ctx, "live move for the spectator", `document.querySelector('#board .piece[data-position="c3"]')`, 10*time.Second)
}

func TestWebLobby_ListsYourGamesWithYourTurnFlag(t *testing.T) {
	baseURL := newRESTTestServer(t)
	room := newTakRoom(t, baseURL, "tak-3x3", "Alice")
	joinRoom(t, baseURL, room, "Bob")

	ctx, cancel := newChromedpContext(t, 40*time.Second)
	defer cancel()
	games, _ := json.Marshal([]map[string]any{{"code": room, "def": "tak-3x3", "name": "Alice", "ts": time.Now().UnixMilli()}})
	if err := chromedp.Run(ctx,
		chromedp.Navigate(baseURL+"/healthz"),
		chromedp.Evaluate(fmt.Sprintf(`localStorage.setItem("bingle.games", %q); true`, games), nil),
		chromedp.Navigate(baseURL+"/"),
	); err != nil {
		t.Fatal(err)
	}
	// The icon of the top bar counts the games waiting for you ...
	waitJS(t, ctx, "badge", `(function(){ const b = document.getElementById("active-badge"); return b && !b.hidden && b.textContent === "1"; })()`, 10*time.Second)
	// ... and opens the list of them.
	clickSel(t, ctx, "#open-active-games")
	waitJS(t, ctx, "my games list", `document.querySelector("#active-games-dialog[open] #my-games li .badge.turn")`, 10*time.Second)
	if got := evalString(t, ctx, `document.querySelector("#my-games li").textContent`); !strings.Contains(got, "vs Bob") || !strings.Contains(got, "Your turn") {
		t.Errorf("my-games row = %q, want opponent and a your-turn flag", got)
	}
}

func TestWebGame_TimedGameShowsCountdownAndEndsOnTime(t *testing.T) {
	baseURL := newRESTTestServer(t)
	var created struct {
		RoomCode string `json:"room_code"`
	}
	if code := doJSON(t, "POST", baseURL+"/api/games",
		map[string]any{"game_def_id": "tak-3x3", "player_name": "Alice", "move_time_seconds": 4}, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	joinRoom(t, baseURL, created.RoomCode, "Bob")

	ctx, cancel := newChromedpContext(t, 40*time.Second)
	defer cancel()
	openGameAs(t, ctx, baseURL, created.RoomCode, "tak-3x3", "Alice")

	// A countdown appears on the strip of the player to move (Alice).
	waitJS(t, ctx, "clock on the seat to move", `document.querySelector('#seat-1 [data-clock]') && /^⏱ 0:0[1-4]$/.test(document.querySelector('#seat-1 [data-clock]').textContent)`, 8*time.Second)
	if got := evalInt(t, ctx, `document.querySelectorAll('#seat-2 [data-clock]').length`); got != 0 {
		t.Error("the waiting player must not show a clock")
	}

	// Nobody moves: when the countdown reaches zero the page asks the server,
	// which ends the game on time.
	waitJS(t, ctx, "timeout result", `document.getElementById("status-message").textContent.indexOf("on time") >= 0`, 15*time.Second)
	if got := evalString(t, ctx, `document.getElementById("status-message").textContent`); !strings.Contains(got, "Bob") {
		t.Errorf("message = %q, want Bob named as the winner", got)
	}
}

func TestWebGame_DrawOfferAppearsAndCanBeAccepted(t *testing.T) {
	baseURL := newRESTTestServer(t)
	room := newTakRoom(t, baseURL, "tak-3x3", "Alice")
	joinRoom(t, baseURL, room, "Bob")

	ctxA, cancelA := newChromedpContext(t, 40*time.Second)
	defer cancelA()
	ctxB, cancelB := newChromedpContext(t, 40*time.Second)
	defer cancelB()
	openGameAs(t, ctxA, baseURL, room, "tak-3x3", "Alice")
	openGameAs(t, ctxB, baseURL, room, "tak-3x3", "Bob")
	waitJS(t, ctxB, "live", `document.getElementById("conn-badge").textContent === "live"`, 10*time.Second)

	waitJS(t, ctxA, "offer button", `document.getElementById("offer-draw") && !document.getElementById("offer-draw").disabled`, 10*time.Second)
	clickSel(t, ctxA, "#offer-draw")
	waitJS(t, ctxA, "own offer banner", `document.getElementById("draw-banner") && document.getElementById("draw-cancel")`, 10*time.Second)

	// Bob sees it live and can accept.
	waitJS(t, ctxB, "incoming offer", `document.getElementById("draw-accept")`, 10*time.Second)
	clickSel(t, ctxB, "#draw-accept")
	waitJS(t, ctxB, "draw result", `document.getElementById("status-message").textContent.indexOf("draw") >= 0`, 10*time.Second)
	waitJS(t, ctxA, "Alice sees the draw", `document.getElementById("status-message").textContent.indexOf("draw") >= 0 && !document.getElementById("draw-banner")`, 10*time.Second)
}

func TestWebGame_DraughtsMandatoryCaptureByClicking(t *testing.T) {
	baseURL := newRESTTestServer(t)
	room := newTakRoom(t, baseURL, "draughts", "Alice")
	joinRoom(t, baseURL, room, "Bob")
	for _, m := range []struct{ who, mv string }{{"Alice", "c3-d4"}, {"Bob", "f6-e5"}} {
		if code, out := moveNotation(t, baseURL, room, m.who, m.mv); code != 200 {
			t.Fatalf("%s %s: %d %v", m.who, m.mv, code, out)
		}
	}

	ctx, cancel := newChromedpContext(t, 40*time.Second)
	defer cancel()
	openGameAs(t, ctx, baseURL, room, "draughts", "Alice")
	waitJS(t, ctx, "my turn", `document.getElementById("status-message").textContent.indexOf("Your turn") === 0`, 10*time.Second)

	// The capture is compulsory: only the capturing man can be selected.
	waitJS(t, ctx, "only d4 is selectable", `document.querySelectorAll('#board .cell.clickable').length === 1 && document.querySelector('#board .cell.clickable[data-position="d4"]')`, 10*time.Second)
	clickCell(t, ctx, "d4")
	waitJS(t, ctx, "landing offered", `document.querySelector('#board .cell.clickable[data-position="f6"]')`, 5*time.Second)
	clickCell(t, ctx, "f6")
	waitJS(t, ctx, "submit capture", `document.getElementById("submit-move") && document.getElementById("submit-move").textContent.indexOf("d4xf6") >= 0`, 5*time.Second)
	clickSel(t, ctx, "#submit-move")

	waitJS(t, ctx, "capture recorded", `document.querySelector('#move-list button.mv[data-seq="3"]')`, 10*time.Second)
	var st restState
	doJSON(t, "GET", baseURL+"/api/games/"+room+"/state", nil, &st)
	if _, ok := st.State.Cells["e5"]; ok {
		t.Error("the captured man is still on e5")
	}
	if got := st.State.Cells["f6"]; len(got) != 1 || got[0].Color != "black" {
		t.Errorf("f6 = %+v, want the capturing black man", got)
	}
}

func TestWebGame_QuickPlacementByClickingAnEmptySquare(t *testing.T) {
	baseURL := newRESTTestServer(t)
	room := newTakRoom(t, baseURL, "tak-5x5", "Alice")
	joinRoom(t, baseURL, room, "Bob")
	for _, m := range []struct{ who, mv string }{{"Alice", "a5"}, {"Bob", "e1"}} {
		if code, out := moveNotation(t, baseURL, room, m.who, m.mv); code != 200 {
			t.Fatalf("%s %s: %d %v", m.who, m.mv, code, out)
		}
	}

	ctx, cancel := newChromedpContext(t, 40*time.Second)
	defer cancel()
	openGameAs(t, ctx, baseURL, room, "tak-5x5", "Alice")
	waitJS(t, ctx, "my turn", `document.getElementById("status-message").textContent.indexOf("Your turn") === 0`, 10*time.Second)

	// Empty squares are directly clickable (quick placement).
	waitJS(t, ctx, "quick targets", `document.querySelectorAll('#board .cell.quick').length === 23`, 5*time.Second)

	// A plain click places the default piece (a flat stone).
	clickCell(t, ctx, "c3")
	waitJS(t, ctx, "submit flat on c3", `document.getElementById("submit-move") && document.getElementById("submit-move").textContent.trim() === "Submit c3"`, 5*time.Second)
	clickSel(t, ctx, "#reset-move")

	// Shift-click places a standing stone.
	shiftClick := `(function(){
		var el = document.querySelector('#board .cell[data-position="d4"] .cell-hit');
		el.dispatchEvent(new MouseEvent('click', {bubbles: true, shiftKey: true}));
		return true;
	})()`
	var ok bool
	if err := chromedp.Run(ctx, chromedp.Evaluate(shiftClick, &ok)); err != nil || !ok {
		t.Fatalf("shift-click: %v", err)
	}
	waitJS(t, ctx, "submit wall on d4", `document.getElementById("submit-move") && document.getElementById("submit-move").textContent.trim() === "Submit Sd4"`, 5*time.Second)
	clickSel(t, ctx, "#submit-move")
	waitJS(t, ctx, "wall placed", `document.querySelector('#move-list button.mv[data-seq="3"]')`, 10*time.Second)
	var st restState
	doJSON(t, "GET", baseURL+"/api/games/"+room+"/state", nil, &st)
	if got := st.State.Cells["d4"]; len(got) != 1 || got[0].Piece != "wall" {
		t.Errorf("d4 = %+v, want a wall", got)
	}
}

func TestWebGame_GoPassButtonAndScoring(t *testing.T) {
	baseURL := newRESTTestServer(t)
	room := newTakRoom(t, baseURL, "go-9x9", "Alice")
	joinRoom(t, baseURL, room, "Bob")
	if code, out := moveNotation(t, baseURL, room, "Alice", "e5"); code != 200 {
		t.Fatalf("e5: %d %v", code, out)
	}

	ctx, cancel := newChromedpContext(t, 40*time.Second)
	defer cancel()
	openGameAs(t, ctx, baseURL, room, "go-9x9", "Bob")
	waitJS(t, ctx, "Bob's turn", `document.getElementById("status-message").textContent.indexOf("Your turn") === 0`, 10*time.Second)

	// A star point and 80 free intersections; pass is offered as a button.
	if n := evalInt(t, ctx, `document.querySelectorAll('#board .hoshi').length`); n != 5 {
		t.Errorf("hoshi = %d, want 5 on a 9x9 board", n)
	}
	waitJS(t, ctx, "pass button", `document.getElementById("action-pass")`, 5*time.Second)
	clickSel(t, ctx, "#action-pass")
	waitJS(t, ctx, "submit pass", `document.getElementById("submit-move") && document.getElementById("submit-move").textContent.trim() === "Submit pass"`, 5*time.Second)
	clickSel(t, ctx, "#submit-move")
	waitJS(t, ctx, "the pass in the move list", `document.querySelector('#move-list button.mv[data-seq="2"]')`, 10*time.Second)

	// Alice passes too: the game ends by score.
	if code, out := moveNotation(t, baseURL, room, "Alice", "pass"); code != 200 {
		t.Fatalf("pass: %d %v", code, out)
	}
	waitJS(t, ctx, "score shown", `/on points/.test(document.getElementById("status-message").textContent) && /Black 81/.test(document.getElementById("status-message").textContent)`, 10*time.Second)
}

func TestWebGame_HexagonBoardPlacementAndLosingByThree(t *testing.T) {
	baseURL := newRESTTestServer(t)
	room := newTakRoom(t, baseURL, "yavalath", "Alice")
	joinRoom(t, baseURL, room, "Bob")
	// Alice has a5 and b5; Bob has two scattered stones. Placing c5 would
	// make three in a row, which loses in Yavalath.
	for _, m := range []struct{ who, mv string }{{"Alice", "a5"}, {"Bob", "e1"}, {"Alice", "b5"}, {"Bob", "i1"}} {
		if code, out := moveNotation(t, baseURL, room, m.who, m.mv); code != 200 {
			t.Fatalf("%s %s: %d %v", m.who, m.mv, code, out)
		}
	}

	ctx, cancel := newChromedpContext(t, 40*time.Second)
	defer cancel()
	openGameAs(t, ctx, baseURL, room, "yavalath", "Alice")
	waitJS(t, ctx, "my turn", `document.getElementById("status-message").textContent.indexOf("Your turn") === 0`, 10*time.Second)

	// 61 hexagons; labels follow the outline: 9 columns and 9 rows.
	if n := evalInt(t, ctx, `document.querySelectorAll('#board .cell').length`); n != 61 {
		t.Errorf("cells = %d, want 61", n)
	}
	if n := evalInt(t, ctx, `document.querySelectorAll('#board .coord').length`); n != 18 {
		t.Errorf("coordinate labels = %d, want 18", n)
	}
	// Most of the board is clickable, so squares are not marked up; the
	// hint tells the player what to do.
	if n := evalInt(t, ctx, `document.querySelectorAll('#board .target-dot, #board .quick-dot').length`); n != 0 {
		t.Errorf("target markers = %d, want none on an open board", n)
	}
	if got := evalString(t, ctx, `document.querySelector('#move-bar .hint').textContent`); !strings.Contains(got, "empty square") {
		t.Errorf("hint = %q, want a placement hint", got)
	}

	clickCell(t, ctx, "c5")
	waitJS(t, ctx, "submit c5", `document.getElementById("submit-move") && document.getElementById("submit-move").textContent.trim() === "Submit c5"`, 5*time.Second)
	clickSel(t, ctx, "#submit-move")
	waitJS(t, ctx, "result shown", `/three-in-a-row|Bob wins|Bob won/i.test(document.getElementById("status-message").textContent)`, 10*time.Second)

	var st restState
	doJSON(t, "GET", baseURL+"/api/games/"+room+"/state", nil, &st)
	if st.Status != "finished" || st.WinnerSeat == nil || *st.WinnerSeat != 2 || st.EndReason != "three-in-a-row" {
		t.Errorf("status %q winner %v reason %q, want finished / seat 2 / three-in-a-row", st.Status, st.WinnerSeat, st.EndReason)
	}
}

func TestWebGame_PlayFromAnalysisPositionStartsANewGame(t *testing.T) {
	baseURL := newRESTTestServer(t)
	room := newTakRoom(t, baseURL, "tak-5x5", "Alice")
	joinRoom(t, baseURL, room, "Bob")
	for _, m := range []struct{ who, mv string }{{"Alice", "a5"}, {"Bob", "e1"}, {"Alice", "c3"}} {
		if code, out := moveNotation(t, baseURL, room, m.who, m.mv); code != 200 {
			t.Fatalf("%s %s: %d %v", m.who, m.mv, code, out)
		}
	}

	ctx, cancel := newChromedpContext(t, 40*time.Second)
	defer cancel()
	openGameAs(t, ctx, baseURL, room, "tak-5x5", "Alice")
	waitJS(t, ctx, "live game", `document.querySelector('#move-list button.mv[data-seq="3"]')`, 10*time.Second)

	clickSel(t, ctx, "#analyze")
	waitJS(t, ctx, "analysis banner", `document.getElementById("exit-analysis")`, 10*time.Second)
	waitJS(t, ctx, "play-from-here button", `document.getElementById("play-from-here")`, 10*time.Second)

	clickSel(t, ctx, "#play-from-here")
	waitJS(t, ctx, "dialog", `document.getElementById("fh-start")`, 5*time.Second)
	// Play it against the random bot.
	if err := chromedp.Run(ctx, chromedp.Evaluate(`(function(){
		var s = document.getElementById("fh-opponent"); s.value = "random";
		return s.value;
	})()`, nil)); err != nil {
		t.Fatal(err)
	}
	clickSel(t, ctx, "#fh-start")
	waitJS(t, ctx, "navigated to a new room", `location.pathname.indexOf("/game/") === 0 && location.pathname !== "/game/`+room+`"`, 15*time.Second)
	waitJS(t, ctx, "board", `document.getElementById("board-svg")`, 10*time.Second)

	var newRoom string
	if err := chromedp.Run(ctx, chromedp.Evaluate(`location.pathname.split("/").pop()`, &newRoom)); err != nil {
		t.Fatal(err)
	}
	var st restState
	doJSON(t, "GET", baseURL+"/api/games/"+newRoom+"/state?at_seq=0", nil, &st)
	// Three stones of the analysed position (a5, e1, c3), not an empty board.
	if len(st.State.Cells) != 3 {
		t.Errorf("new game's start has %d occupied squares, want 3 (the analysed position)", len(st.State.Cells))
	}
}

func TestWebGame_TakebackRequestAcceptedByOpponent(t *testing.T) {
	baseURL := newRESTTestServer(t)
	room := newTakRoom(t, baseURL, "tak-5x5", "Alice")
	joinRoom(t, baseURL, room, "Bob")
	// Alice a5, Bob e1, Alice c3, Bob c4: Alice cannot undo directly now.
	for _, m := range []struct{ who, mv string }{{"Alice", "a5"}, {"Bob", "e1"}, {"Alice", "c3"}, {"Bob", "c4"}} {
		if code, out := moveNotation(t, baseURL, room, m.who, m.mv); code != 200 {
			t.Fatalf("%s %s: %d %v", m.who, m.mv, code, out)
		}
	}

	alice, cancelA := newChromedpContext(t, 40*time.Second)
	defer cancelA()
	openGameAs(t, alice, baseURL, room, "tak-5x5", "Alice")
	waitJS(t, alice, "request button", `document.getElementById("request-takeback")`, 10*time.Second)
	if got := evalString(t, alice, `String(document.getElementById("undo-move") === null)`); got != "true" {
		t.Errorf("undo should not be offered once the opponent replied")
	}
	clickSel(t, alice, "#request-takeback")
	waitJS(t, alice, "waiting banner", `document.getElementById("takeback-banner") && /waiting for your opponent/.test(document.getElementById("takeback-banner").textContent)`, 10*time.Second)

	bob, cancelB := newChromedpContext(t, 40*time.Second)
	defer cancelB()
	openGameAs(t, bob, baseURL, room, "tak-5x5", "Bob")
	waitJS(t, bob, "request banner", `document.getElementById("takeback-accept")`, 10*time.Second)
	clickSel(t, bob, "#takeback-accept")

	// Alice's page follows live: the game is back at move 2 with Alice to move.
	waitJS(t, alice, "rewound", `document.querySelectorAll('#move-list button.mv').length === 2 && !document.getElementById("takeback-banner")`, 10*time.Second)
	var st restState
	doJSON(t, "GET", baseURL+"/api/games/"+room+"/state", nil, &st)
	if st.MoveSeq != 2 || st.TurnSeat == nil || *st.TurnSeat != 1 || st.State.Cells["c3"] != nil {
		t.Errorf("after takeback: move_seq=%d turn=%v c3=%v", st.MoveSeq, st.TurnSeat, st.State.Cells["c3"])
	}
}

func TestWebGame_ArrowKeysMoveFocusAcrossTheBoard(t *testing.T) {
	baseURL := newRESTTestServer(t)
	room := newTakRoom(t, baseURL, "tic-tac-toe", "Alice")
	joinRoom(t, baseURL, room, "Bob")

	ctx, cancel := newChromedpContext(t, 40*time.Second)
	defer cancel()
	openGameAs(t, ctx, baseURL, room, "tic-tac-toe", "Alice")
	waitJS(t, ctx, "my turn", `document.getElementById("status-message").textContent.indexOf("Your turn") === 0`, 10*time.Second)

	// One roving tab stop for the whole board.
	if n := evalInt(t, ctx, `document.querySelectorAll('#board .cell[tabindex="0"]').length`); n != 1 {
		t.Fatalf("tab stops on the board = %d, want 1", n)
	}
	key := func(k string) {
		t.Helper()
		js := `(function(){ var el = document.activeElement; el.dispatchEvent(new KeyboardEvent("keydown", {key: ` + `"` + k + `"` + `, bubbles: true, cancelable: true})); return true; })()`
		var ok bool
		if err := chromedp.Run(ctx, chromedp.Evaluate(js, &ok)); err != nil {
			t.Fatal(err)
		}
	}
	focused := func() string {
		return evalString(t, ctx, `(document.activeElement.closest(".cell")||{getAttribute:function(){return ""}}).getAttribute("data-position")`)
	}

	// Focus the board by focusing the first tab stop, then walk.
	if err := chromedp.Run(ctx, chromedp.Evaluate(`document.querySelector('#board .cell[tabindex="0"]').focus(); true`, nil)); err != nil {
		t.Fatal(err)
	}
	start := focused()
	if start == "" {
		t.Fatal("no square focused")
	}
	key("ArrowRight")
	right := focused()
	key("ArrowDown")
	down := focused()
	key("ArrowLeft")
	key("ArrowUp")
	back := focused()
	if right == start || down == right || back == "" {
		t.Errorf("focus did not move: start=%s right=%s down=%s back=%s", start, right, down, back)
	}
	// Still exactly one tab stop, and arrows did not browse the history.
	if n := evalInt(t, ctx, `document.querySelectorAll('#board .cell[tabindex="0"]').length`); n != 1 {
		t.Errorf("tab stops after moving = %d, want 1", n)
	}
}

func TestWebGame_MovesAnimateAndCanBeSwitchedOff(t *testing.T) {
	baseURL := newRESTTestServer(t)
	room := newTakRoom(t, baseURL, "tak-5x5", "Alice")
	joinRoom(t, baseURL, room, "Bob")
	moveNotation(t, baseURL, room, "Alice", "a5")
	moveNotation(t, baseURL, room, "Bob", "e1")

	ctx, cancel := newChromedpContext(t, 40*time.Second)
	defer cancel()
	openGameAs(t, ctx, baseURL, room, "tak-5x5", "Alice")
	waitJS(t, ctx, "my turn", `document.getElementById("status-message").textContent.indexOf("Your turn") === 0`, 10*time.Second)

	// Count animations started on pieces; Bob's reply arrives over the
	// WebSocket and should slide/pop in.
	if err := chromedp.Run(ctx, chromedp.Evaluate(`(function(){
		window.__anims = 0;
		var orig = Element.prototype.animate;
		Element.prototype.animate = function(){ window.__anims++; return orig.apply(this, arguments); };
		return true;
	})()`, nil)); err != nil {
		t.Fatal(err)
	}
	if code, out := moveNotation(t, baseURL, room, "Alice", "c3"); code != 200 {
		t.Fatalf("c3: %d %v", code, out)
	}
	if code, out := moveNotation(t, baseURL, room, "Bob", "d4"); code != 200 {
		t.Fatalf("d4: %d %v", code, out)
	}
	waitJS(t, ctx, "pieces animated", `window.__anims > 0`, 10*time.Second)

	// Switching the setting off stops them.
	clickSel(t, ctx, "#opt-animate")
	if err := chromedp.Run(ctx, chromedp.Evaluate(`window.__anims = 0; true`, nil)); err != nil {
		t.Fatal(err)
	}
	if code, out := moveNotation(t, baseURL, room, "Alice", "b2"); code != 200 {
		t.Fatalf("b2: %d %v", code, out)
	}
	waitJS(t, ctx, "b2 drawn", `document.querySelector('#board .piece[data-position="b2"]')`, 10*time.Second)
	if n := evalInt(t, ctx, `window.__anims`); n != 0 {
		t.Errorf("animations with the setting off = %d, want 0", n)
	}
}

func TestWebGame_ImportMovesIntoAnalysis(t *testing.T) {
	baseURL := newRESTTestServer(t)
	room := newTakRoom(t, baseURL, "tak-5x5", "Alice")
	joinRoom(t, baseURL, room, "Bob")

	ctx, cancel := newChromedpContext(t, 40*time.Second)
	defer cancel()
	openGameAs(t, ctx, baseURL, room, "tak-5x5", "Alice")
	waitJS(t, ctx, "analyze button", `document.getElementById("analyze")`, 10*time.Second)

	clickSel(t, ctx, "#analyze")
	waitJS(t, ctx, "analysis", `document.getElementById("analysis-import")`, 10*time.Second)
	clickSel(t, ctx, "#analysis-import")
	waitJS(t, ctx, "dialog", `document.getElementById("import-text")`, 5*time.Second)

	// PTN-flavoured text with numbers, a comment and a result; the last
	// move is illegal (a5 is occupied), so three moves should load.
	text := "[Size \"5\"]\n1. a5 e1 {opening} 2. c3 1-0 a5"
	if err := chromedp.Run(ctx, chromedp.SetValue("#import-text", text, chromedp.ByID)); err != nil {
		t.Fatal(err)
	}
	clickSel(t, ctx, "#import-load")
	waitJS(t, ctx, "partial load message", `/Loaded 3 of 4 moves/.test(document.getElementById("import-msg").textContent)`, 10*time.Second)
	if err := chromedp.Run(ctx, chromedp.Evaluate(`document.getElementById("import-dialog").close(); true`, nil)); err != nil {
		t.Fatal(err)
	}
	if n := evalInt(t, ctx, `document.querySelectorAll('#board .piece').length`); n != 3 {
		t.Errorf("pieces on the analysis board = %d, want 3 (a5, e1, c3)", n)
	}
}

func TestWebGame_AmazonsCompoundMoveByClicking(t *testing.T) {
	baseURL := newRESTTestServer(t)
	room := newTakRoom(t, baseURL, "amazons", "Alice")
	joinRoom(t, baseURL, room, "Bob")

	ctx, cancel := newChromedpContext(t, 40*time.Second)
	defer cancel()
	openGameAs(t, ctx, baseURL, room, "amazons", "Alice")
	waitJS(t, ctx, "my turn", `document.getElementById("status-message").textContent.indexOf("Your turn") === 0`, 10*time.Second)

	// Only the four queens can start a move.
	if n := evalInt(t, ctx, `document.querySelectorAll('#board .cell.clickable').length`); n != 4 {
		t.Fatalf("clickable squares at the start = %d, want the 4 queens", n)
	}
	clickCell(t, ctx, "d1")
	// d1 slides along the first rank and the file and diagonals: many squares.
	waitJS(t, ctx, "destinations", `document.querySelectorAll('#board .cell.clickable').length > 10`, 5*time.Second)
	clickCell(t, ctx, "d5")
	// From d5 the arrow may go anywhere on its lines, including back to d1.
	waitJS(t, ctx, "aim squares", `document.querySelector('#board .cell[data-position="d1"].clickable')`, 5*time.Second)
	clickCell(t, ctx, "f5")
	waitJS(t, ctx, "submit", `document.getElementById("submit-move") && document.getElementById("submit-move").textContent.trim() === "Submit d1-d5/f5"`, 5*time.Second)
	clickSel(t, ctx, "#submit-move")
	waitJS(t, ctx, "move listed", `document.querySelector('#move-list button.mv[data-seq="1"]')`, 10*time.Second)

	waitJS(t, ctx, "arrow drawn", `document.querySelector('#board .piece[data-position="f5"][data-piece="arrow"]')`, 5*time.Second)
	var st restState
	doJSON(t, "GET", baseURL+"/api/games/"+room+"/state", nil, &st)
	if st.MoveSeq != 1 || len(st.State.Cells["d5"]) != 1 || len(st.State.Cells["f5"]) != 1 || st.State.Cells["d1"] != nil {
		t.Errorf("state after the move: seq %d d5=%v f5=%v d1=%v", st.MoveSeq, st.State.Cells["d5"], st.State.Cells["f5"], st.State.Cells["d1"])
	}
}

// A literal "null" or "undefined" in the page is a rendering slip (a missing
// child passed to DOM append); no page of any game state should contain one.
func TestWebGame_NoStrayNullOrUndefinedText(t *testing.T) {
	baseURL := newRESTTestServer(t)
	var created struct {
		RoomCode string `json:"room_code"`
	}
	doJSON(t, "POST", baseURL+"/api/games", map[string]any{"game_def_id": "tak-5x5", "player_name": "Alice", "opponent": "tactical"}, &created)
	moveNotation(t, baseURL, created.RoomCode, "Alice", "a5")

	ctx, cancel := newChromedpContext(t, 40*time.Second)
	defer cancel()
	openGameAs(t, ctx, baseURL, created.RoomCode, "tak-5x5", "Alice")
	waitJS(t, ctx, "board", `document.getElementById("hint")`, 10*time.Second)
	check := func(when string) {
		t.Helper()
		bad := evalString(t, ctx, `(function(){
			var t = document.body.innerText;
			var hits = [];
			["null", "undefined", "NaN", "[object Object]"].forEach(function(w){
				if (new RegExp("(^|\\s)" + w.replace(/[\[\]]/g, "\\$&") + "(\\s|$)").test(t)) hits.push(w);
			});
			return hits.join(",");
		})()`)
		if bad != "" {
			t.Errorf("%s: page contains stray %q text", when, bad)
		}
	}
	check("live game")
	clickSel(t, ctx, "#analyze")
	waitJS(t, ctx, "analysis", `document.getElementById("exit-analysis")`, 10*time.Second)
	check("analysis")
	clickSel(t, ctx, "#exit-analysis")
	clickSel(t, ctx, "#resign")
	clickSel(t, ctx, "#resign")
	waitJS(t, ctx, "finished", `document.querySelector(".status-bar.over, #rematch")`, 10*time.Second)
	check("finished game")
}

func TestWebLobby_TabShowsHowManyGamesAwaitYou(t *testing.T) {
	baseURL := newRESTTestServer(t)
	// Two bot games where it is Alice's turn.
	var codes []string
	for i := 0; i < 2; i++ {
		var created struct {
			RoomCode string `json:"room_code"`
		}
		doJSON(t, "POST", baseURL+"/api/games", map[string]any{"game_def_id": "tic-tac-toe", "player_name": "Alice", "opponent": "random"}, &created)
		codes = append(codes, created.RoomCode)
	}
	games, _ := json.Marshal([]map[string]any{
		{"code": codes[0], "def": "tic-tac-toe", "name": "Alice", "ts": time.Now().UnixMilli()},
		{"code": codes[1], "def": "tic-tac-toe", "name": "Alice", "ts": time.Now().UnixMilli()},
	})
	ctx, cancel := newChromedpContext(t, 30*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx,
		chromedp.Navigate(baseURL+"/healthz"),
		chromedp.Evaluate(fmt.Sprintf(`localStorage.setItem("bingle.games", %q); true`, games), nil),
		chromedp.Navigate(baseURL+"/"),
	); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "title count", `document.title === "(2) Bingle"`, 10*time.Second)
	if got := evalString(t, ctx, `document.getElementById("favicon").href.slice(0, 40)`); !strings.HasPrefix(got, "data:image/svg+xml") {
		t.Errorf("favicon not badged: %q", got)
	}
}

func TestWebGame_HoverGhostPiecesOnPlacementTargets(t *testing.T) {
	baseURL := newRESTTestServer(t)
	room := newTakRoom(t, baseURL, "tic-tac-toe", "Alice")
	joinRoom(t, baseURL, room, "Bob")
	ctx, cancel := newChromedpContext(t, 30*time.Second)
	defer cancel()
	openGameAs(t, ctx, baseURL, room, "tic-tac-toe", "Alice")
	waitJS(t, ctx, "my turn", `document.getElementById("status-message").textContent.indexOf("Your turn") === 0`, 10*time.Second)

	// One translucent preview per empty square, none counted as real pieces.
	if n := evalInt(t, ctx, `document.querySelectorAll('#board .ghost-piece').length`); n != 9 {
		t.Errorf("ghost pieces = %d, want 9", n)
	}
	if n := evalInt(t, ctx, `document.querySelectorAll('#board .piece').length`); n != 0 {
		t.Errorf("real pieces = %d, want 0", n)
	}
	// Hidden until hovered.
	if got := evalString(t, ctx, `getComputedStyle(document.querySelector('#board .ghost-piece')).opacity`); got != "0" {
		t.Errorf("ghost opacity at rest = %s, want 0", got)
	}
}

func TestWebGame_SaveImageProducesAStandaloneSVG(t *testing.T) {
	baseURL := newRESTTestServer(t)
	room := newTakRoom(t, baseURL, "tak-5x5", "Alice")
	joinRoom(t, baseURL, room, "Bob")
	moveNotation(t, baseURL, room, "Alice", "a5")
	moveNotation(t, baseURL, room, "Bob", "e1")

	ctx, cancel := newChromedpContext(t, 30*time.Second)
	defer cancel()
	openGameAs(t, ctx, baseURL, room, "tak-5x5", "Alice")
	waitJS(t, ctx, "button", `document.getElementById("save-image")`, 10*time.Second)

	// Capture the blob the click would download instead of saving a file.
	if err := chromedp.Run(ctx, chromedp.Evaluate(`(function(){
		window.__svg = null;
		var orig = URL.createObjectURL;
		URL.createObjectURL = function(blob){ blob.text().then(function(t){ window.__svg = t; }); return orig.call(URL, blob); };
		HTMLAnchorElement.prototype.click = function(){};
		return true;
	})()`, nil)); err != nil {
		t.Fatal(err)
	}
	clickSel(t, ctx, "#save-image")
	waitJS(t, ctx, "svg captured", `window.__svg && window.__svg.length > 500`, 10*time.Second)
	svg := evalString(t, ctx, `window.__svg`)
	for _, want := range []string{"<svg", "xmlns=\"http://www.w3.org/2000/svg\"", "data-piece=\"flat\""} {
		if !strings.Contains(svg, want) {
			t.Errorf("exported SVG lacks %q", want)
		}
	}
	for _, unwanted := range []string{"cell-hit", "ghost-piece"} {
		if strings.Contains(svg, unwanted) {
			t.Errorf("exported SVG still contains interface element %q", unwanted)
		}
	}
	if !strings.Contains(svg, "fill:") {
		t.Error("exported SVG has no inlined styles")
	}
}

func TestWebGame_PieRuleSwapButton(t *testing.T) {
	baseURL := newRESTTestServer(t)
	room := newTakRoom(t, baseURL, "hex-11", "Alice")
	joinRoom(t, baseURL, room, "Bob")
	if code, out := moveNotation(t, baseURL, room, "Alice", "f6"); code != 200 {
		t.Fatalf("f6: %d %v", code, out)
	}

	ctx, cancel := newChromedpContext(t, 30*time.Second)
	defer cancel()
	openGameAs(t, ctx, baseURL, room, "hex-11", "Bob")
	waitJS(t, ctx, "swap button", `document.getElementById("action-swap")`, 10*time.Second)
	clickSel(t, ctx, "#action-swap")
	waitJS(t, ctx, "submit swap", `document.getElementById("submit-move") && document.getElementById("submit-move").textContent.trim() === "Submit swap"`, 5*time.Second)
	clickSel(t, ctx, "#submit-move")
	waitJS(t, ctx, "swap recorded", `document.querySelector('#move-list button.mv[data-seq="2"]')`, 10*time.Second)

	var st restState
	doJSON(t, "GET", baseURL+"/api/games/"+room+"/state", nil, &st)
	cell := st.State.Cells["f6"]
	if len(cell) != 1 || cell[0].Color != "blue" {
		t.Errorf("f6 after the swap = %+v, want a blue stone", cell)
	}
	if st.TurnSeat == nil || *st.TurnSeat != 1 {
		t.Errorf("turn after the swap = %v, want seat 1", st.TurnSeat)
	}
}

// Every game must open and draw without a JavaScript exception, show every
// position of its board and let the player make a first move by clicking.
// One variant per family keeps it quick.
func TestWebGame_EveryGameOpensDrawsAndTakesAFirstClick(t *testing.T) {
	baseURL := newRESTTestServer(t)
	var defs []struct {
		ID          string `json:"id"`
		Family      string `json:"family"`
		BoardWidth  int    `json:"board_width"`
		BoardHeight int    `json:"board_height"`
	}
	doJSON(t, "GET", baseURL+"/api/game-defs", nil, &defs)
	seen := map[string]bool{}
	var picks []string
	for _, d := range defs {
		fam := d.Family
		if fam == "" {
			fam = d.ID
		}
		if seen[fam] {
			continue
		}
		seen[fam] = true
		picks = append(picks, d.ID)
	}
	if len(picks) < 12 {
		t.Fatalf("only %d families", len(picks))
	}

	ctx, cancel := newChromedpContext(t, 150*time.Second)
	defer cancel()
	var mu sync.Mutex
	var jsErrors []string
	chromedp.ListenTarget(ctx, func(ev any) {
		if e, ok := ev.(*runtime.EventExceptionThrown); ok && e.ExceptionDetails != nil {
			mu.Lock()
			jsErrors = append(jsErrors, e.ExceptionDetails.Error())
			mu.Unlock()
		}
	})

	for _, id := range picks {
		var created struct {
			RoomCode string `json:"room_code"`
		}
		if code := doJSON(t, "POST", baseURL+"/api/games", map[string]any{"game_def_id": id, "player_name": "Alice", "opponent": "random"}, &created); code != 200 {
			t.Fatalf("%s: create %d", id, code)
		}
		openGameAs(t, ctx, baseURL, created.RoomCode, id, "Alice")
		waitJS(t, ctx, id+" board", `(document.querySelectorAll('#board .cell').length > 0 || document.getElementById("card-table")) && document.getElementById("status-message")`, 15*time.Second)

		var legal struct {
			Moves []struct {
				Notation string `json:"notation"`
			} `json:"moves"`
		}
		doJSON(t, "GET", baseURL+"/api/games/"+created.RoomCode+"/moves/legal", nil, &legal)
		cells := evalInt(t, ctx, `document.querySelectorAll('#board .cell').length`)
		var info struct {
			Board struct {
				Style     string                `json:"style"`
				Positions []struct{ ID string } `json:"positions"`
			} `json:"board"`
		}
		doJSON(t, "GET", baseURL+"/api/game-defs/"+id, nil, &info)
		// A board-less ("free") game draws only the tiles and targets.
		if cells != len(info.Board.Positions) && info.Board.Style != "free" && info.Board.Style != "none" {
			t.Errorf("%s: drew %d cells for %d positions", id, cells, len(info.Board.Positions))
		}
		// Something is clickable when it is the player's turn (the random
		// bot never moved first for seat 1).
		if len(legal.Moves) > 0 {
			waitJS(t, ctx, id+" clickable", `document.querySelectorAll('#board .cell.clickable, #hand-white .choosable, #hand-black .choosable, .hand-piece.choosable, #board .cell.quick, #action-pass, #action-swap, #action-cw, .action-buttons button[data-action^="card-"], #piece-buttons button, .pc-btn.playable, .cg-box.clickable').length > 0`, 10*time.Second)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if len(jsErrors) > 0 {
		t.Errorf("uncaught JS exceptions: %v", jsErrors)
	}
}
