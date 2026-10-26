package app_test

import (
	"sync"
	"testing"
	"time"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

// fendoStepJS selects a piece and builds a fence beside it.
const fendoStepJS = `(function () {
	var bar = document.querySelector(".status-bar");
	if (!bar || !bar.classList.contains("your-turn")) return "wait";
	function click(el, what) {
		el.dispatchEvent(new MouseEvent("click", { bubbles: true, cancelable: true }));
		return what;
	}
	// Once a piece is selected the fence sides are offered: take the first.
	var fence = document.querySelector('.action-buttons button[data-action^="fence-"]');
	var cell = document.querySelector("#board .cell.clickable");
	if (fence) return click(fence, "fence");
	if (cell) return click(cell, "cell");
	return "stuck";
})()`

// The UI can play Fendo: a 7x7 board, scoreboards, and moves built from a
// piece, a square and a fence side; the fences are drawn on the board.
func TestWebFendo_PlayTurnsThroughTheUI(t *testing.T) {
	baseURL := newRESTTestServer(t)
	var created struct {
		RoomCode string `json:"room_code"`
	}
	if code := doJSON(t, "POST", baseURL+"/api/games", map[string]any{"game_def_id": "fendo", "player_name": "Alice", "opponent": "random"}, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	ctx, cancel := newChromedpContext(t, 120*time.Second)
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
	openGameAs(t, ctx, baseURL, created.RoomCode, "fendo", "Alice")
	waitJS(t, ctx, "49 squares", `document.querySelectorAll('#board .cell').length === 49`, 15*time.Second)
	waitJS(t, ctx, "scoreboards", `document.querySelectorAll('.seat-strip [data-counter="territory"]').length === 2`, 5*time.Second)
	stuck := 0
	for i := 0; i < 300 && stuck < 40; i++ {
		var what string
		if err := chromedp.Run(ctx, chromedp.Evaluate(fendoStepJS, &what)); err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
		switch what {
		case "wait", "stuck":
			stuck++
			time.Sleep(80 * time.Millisecond)
		default:
			stuck = 0
			time.Sleep(60 * time.Millisecond)
		}
		if st := cfGet(t, baseURL, created.RoomCode); st.MoveSeq >= 8 || st.Status == "finished" {
			break
		}
	}
	// A game can end early (a player may fence themselves into a corner), so
	// finished games need not have run to four moves.
	if st := cfGet(t, baseURL, created.RoomCode); st.MoveSeq < 4 && st.Status != "finished" {
		t.Fatalf("the UI should have played moves: %d (status %s)", st.MoveSeq, st.Status)
	}
	waitJS(t, ctx, "a fence on the board", `document.querySelectorAll('#board .fence').length > 0`, 5*time.Second)
	mu.Lock()
	defer mu.Unlock()
	if len(jsErrors) > 0 {
		t.Errorf("uncaught JS exceptions: %v", jsErrors)
	}
}
