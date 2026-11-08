package app_test

import (
	"sync"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// tricksStepJS makes one choice of the Tricks fixture (tests/testdata/tricks) on the page:
// a card of the hand, or a box of the board of terms.
const tricksStepJS = `(function () {
	if (!document.querySelector("#impulse-card.acting")) return "wait";
	function click(el, what) {
		el.dispatchEvent(new MouseEvent("click", { bubbles: true, cancelable: true }));
		return what;
	}
	var card = document.querySelector("#card-table .pc-btn.playable");
	if (card) return click(card, "card");
	var box = document.querySelector("#choice-grid .cg-box.clickable");
	if (box) return click(box, "box");
	return "stuck";
})()`

// A card game plays through the page: a board of terms in the game's own pictures, crossed
// off by clicking, then a hand only its owner sees, played into tricks; and the players are
// the pictures the game's page script draws for them.
func TestWebTricks_PlayATableThroughThePage(t *testing.T) {
	baseURL := newRESTTestServer(t)
	var created struct {
		RoomCode string `json:"room_code"`
	}
	if code := doJSON(t, "POST", baseURL+"/api/games", map[string]any{"game_def_id": "test-tricks", "player_name": "Alice", "opponent": "random"}, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	ctx, cancel := newChromedpContext(t, 120*time.Second)
	defer cancel()
	var mu sync.Mutex
	var jsErrors []string
	collectJSErrors(ctx, &mu, &jsErrors)
	openGameAs(t, ctx, baseURL, created.RoomCode, "test-tricks", "Alice")
	waitJS(t, ctx, "the board of terms", `document.querySelectorAll('#choice-grid .cg-line').length === 2`, 15*time.Second)
	// Every box and line is a picture (info.page.choiceIcon), its words the tooltip.
	if n := evalInt(t, ctx, `document.querySelectorAll('#choice-grid .cg-box svg.tricks-icon, #choice-grid .cg-title svg.tricks-icon').length`); n != 9 {
		t.Errorf("%d pictures on the board, want its 7 boxes and 2 lines", n)
	}
	// A player is a picture in place of the swatch (info.page.seatIcon).
	if n := evalInt(t, ctx, `document.querySelectorAll('.seat-strip .seat-icon svg.tricks-seat').length`); n != 4 {
		t.Errorf("%d players drawn as pictures, want 4", n)
	}
	waitJS(t, ctx, "my hand: 8 faces", `document.querySelectorAll('#card-table .ct-seat.me .zone-hand .pcard.face').length === 8`, 10*time.Second)
	waitJS(t, ctx, "a hand count for each player", `[...document.querySelectorAll('.seat-strip [data-counter="cards"]')].map(e => e.textContent.trim()).join() === "8,8,8,8"`, 10*time.Second)
	stuck := 0
	for i := 0; i < 400 && stuck < 60; i++ {
		var what string
		if err := chromedp.Run(ctx, chromedp.Evaluate(tricksStepJS, &what)); err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
		if what == "wait" || what == "stuck" {
			stuck++
			time.Sleep(80 * time.Millisecond)
		} else {
			stuck = 0
			time.Sleep(40 * time.Millisecond)
		}
		if st := cfGet(t, baseURL, created.RoomCode); st.MoveSeq >= 12 || st.Status == "finished" {
			break
		}
	}
	if st := cfGet(t, baseURL, created.RoomCode); st.MoveSeq < 12 {
		t.Fatalf("the page crossed off the board and played cards for %d moves only", st.MoveSeq)
	}
	// The terms are set, and a box crossed off shows the player who crossed it.
	waitJS(t, ctx, "one box left on each line", `document.querySelectorAll('#choice-grid .cg-box.chosen').length === 2`, 5*time.Second)
	if n := evalInt(t, ctx, `document.querySelectorAll('#choice-grid .cg-box.crossed svg.tricks-seat').length`); n != 5 {
		t.Errorf("%d crossed boxes show who crossed them, want 5", n)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(jsErrors) > 0 {
		t.Errorf("uncaught JS exceptions: %v", jsErrors)
	}
}
