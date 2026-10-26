package app_test

import (
	"sync"
	"testing"
	"time"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

// nyetStepJS plays a card if one can be played, else crosses the first open
// box of the board, else picks the first team or bonus button.
const nyetStepJS = `(function () {
	var bar = document.querySelector(".status-bar");
	if (!bar || !bar.classList.contains("your-turn")) return "wait";
	function click(el, what) {
		el.dispatchEvent(new MouseEvent("click", { bubbles: true, cancelable: true }));
		return what;
	}
	var card = document.querySelector("#card-table .pc-btn.playable");
	if (card) return click(card, "card");
	var box = document.querySelector("#choice-grid .cg-box.clickable");
	if (box) return click(box, "box");
	var btn = document.querySelector('.action-buttons button[data-action^="team-"], .action-buttons button[data-action^="bonus-"]');
	if (btn) return click(btn, "button");
	return "stuck";
})()`

// The UI can play Nyet!: the board of terms, hands that show only your own
// cards, and cards, boxes and team buttons chosen by clicking.
func TestWebNyet_PlayARoundThroughTheUI(t *testing.T) {
	baseURL := newRESTTestServer(t)
	var created struct {
		RoomCode string `json:"room_code"`
	}
	if code := doJSON(t, "POST", baseURL+"/api/games", map[string]any{"game_def_id": "nyet", "player_name": "Alice", "opponent": "random"}, &created); code != 200 {
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
	openGameAs(t, ctx, baseURL, created.RoomCode, "nyet", "Alice")
	waitJS(t, ctx, "the board of terms", `document.querySelectorAll('#choice-grid .cg-line').length === 5`, 15*time.Second)
	waitJS(t, ctx, "my hand: 15 faces", `document.querySelectorAll('#card-table .ct-seat.me .zone-hand .pcard.face').length === 15`, 10*time.Second)
	waitJS(t, ctx, "three opponents showing only backs", `document.querySelectorAll('#card-table .ct-others .zone-hand .pcard.face').length === 0 && document.querySelectorAll('#card-table .ct-others .zone-hand .pcard.back').length >= 36`, 10*time.Second)
	stuck := 0
	for i := 0; i < 600 && stuck < 40; i++ {
		var what string
		if err := chromedp.Run(ctx, chromedp.Evaluate(nyetStepJS, &what)); err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
		switch what {
		case "wait", "stuck":
			stuck++
			time.Sleep(80 * time.Millisecond)
		default:
			stuck = 0
			time.Sleep(40 * time.Millisecond)
		}
		if st := cfGet(t, baseURL, created.RoomCode); st.MoveSeq >= 70 || st.Status == "finished" {
			break
		}
	}
	if st := cfGet(t, baseURL, created.RoomCode); st.MoveSeq < 40 {
		var where string
		_ = chromedp.Run(ctx, chromedp.Evaluate(`document.querySelector(".status-bar").innerText.replace(/\n/g, " ") + " | buttons: " + [...document.querySelectorAll(".action-buttons button")].map(b => b.dataset.action).join(",") + " | cards: " + document.querySelectorAll("#card-table .pc-btn.playable").length`, &where))
		var legal struct {
			Moves []struct {
				Notation string `json:"notation"`
			} `json:"moves"`
		}
		doJSON(t, "GET", baseURL+"/api/games/"+created.RoomCode+"/moves/legal", nil, &legal)
		var builder string
		_ = chromedp.Run(ctx, chromedp.Evaluate(`document.querySelector("#move-bar") ? document.querySelector("#move-bar").innerText.replace(/\n/g, " ") : "no move bar"`, &builder))
		t.Fatalf("the UI should have crossed boxes and played cards: %d moves (%s); %d legal moves %v; move bar: %s", st.MoveSeq, where, len(legal.Moves), legal.Moves, builder)
	}
	// By now the terms are set and the tricks are being played.
	waitJS(t, ctx, "one box left on every line", `document.querySelectorAll('#choice-grid .cg-box.chosen').length === 5`, 5*time.Second)
	mu.Lock()
	defer mu.Unlock()
	if len(jsErrors) > 0 {
		t.Errorf("uncaught JS exceptions: %v", jsErrors)
	}
}
