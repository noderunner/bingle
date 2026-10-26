package app_test

import (
	"sync"
	"testing"
	"time"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

// hiveStepJS picks a tile from the supply, then a cell; once tiles are down it
// also selects a tile and its destination.
const hiveStepJS = `(function () {
	var bar = document.querySelector(".status-bar");
	if (!bar || !bar.classList.contains("your-turn")) return "wait";
	function click(el, what) {
		el.dispatchEvent(new MouseEvent("click", { bubbles: true, cancelable: true }));
		return what;
	}
	var cell = document.querySelector("#board .cell.clickable");
	var tile = document.querySelector(".hand-piece.choosable");
	if (cell) return click(cell, "cell");
	if (tile) return click(tile, "tile");
	var act = document.querySelector(".action-buttons button");
	if (act) return click(act, "action");
	return "stuck";
})()`

// The UI can play Hive: tiles on a table with no board drawn, the eight tiles in each supply
// with their counts, the Queen counters, and placing tiles by tile and cell.
func TestWebHive_PlayTurnsThroughTheUI(t *testing.T) {
	baseURL := newRESTTestServer(t)
	var created struct {
		RoomCode string `json:"room_code"`
	}
	if code := doJSON(t, "POST", baseURL+"/api/games", map[string]any{"game_def_id": "hive", "player_name": "Alice", "opponent": "random"}, &created); code != 200 {
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
	openGameAs(t, ctx, baseURL, created.RoomCode, "hive", "Alice")
	waitJS(t, ctx, "a board-less table", `!!document.querySelector('#board svg.free-board') && !document.querySelector('#board .board-frame') && document.querySelectorAll('#board .cell').length < 10`, 15*time.Second)
	waitJS(t, ctx, "eight kinds of tile in hand", `document.querySelectorAll('#hand-white .hand-piece').length === 8`, 10*time.Second)
	waitJS(t, ctx, "no Queen-surrounded counters", `document.querySelectorAll('.seat-strip [data-counter]').length === 0`, 5*time.Second)
	waitJS(t, ctx, "icon-only tiles with counts", `[...document.querySelectorAll('#hand-white .hand-piece.tile')].every((b) => b.querySelector('svg') && /^\d+$/.test(b.querySelector('.tile-count').textContent) && !/[a-z]{3}/i.test(b.textContent))`, 5*time.Second)
	if err := chromedp.Run(ctx, chromedp.Evaluate(`document.querySelector('#hand-white [data-piece="ant"]').dispatchEvent(new MouseEvent('mouseenter'))`, nil)); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the Ant's rules on hover", `(document.querySelector('.hover-card')||{textContent:''}).textContent.includes('Soldier Ant') && document.querySelector('.hover-card').textContent.includes('Slides any number') && !!document.querySelector('.hover-card > .piece-tip > svg')`, 5*time.Second)
	waitJS(t, ctx, "large tiles", `document.querySelector('#hand-white .hand-piece.tile svg').getBoundingClientRect().width > 70`, 5*time.Second)
	if err := chromedp.Run(ctx, chromedp.Evaluate(`(() => { const s = document.querySelector('#opt-tile-scale'); s.value = '0.5'; s.dispatchEvent(new Event('change')); })()`, nil)); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "tiles scaled down", `document.querySelector('#hand-white .hand-piece.tile svg').getBoundingClientRect().width < 60`, 5*time.Second)
	_ = chromedp.Run(ctx, chromedp.Evaluate(`document.querySelector('#hand-white [data-piece="ant"]').dispatchEvent(new MouseEvent('mouseleave'))`, nil))
	stuck := 0
	for i := 0; i < 200 && stuck < 40; i++ {
		var what string
		if err := chromedp.Run(ctx, chromedp.Evaluate(hiveStepJS, &what)); err != nil {
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
	if st := cfGet(t, baseURL, created.RoomCode); st.MoveSeq < 4 {
		t.Fatalf("the UI should have played tiles: %d moves", st.MoveSeq)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(jsErrors) > 0 {
		t.Errorf("uncaught JS exceptions: %v", jsErrors)
	}
}
