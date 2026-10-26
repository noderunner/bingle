package app_test

import (
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

// autoStepJS performs one UI action for the player: finish a pending
// selection (count, verb, hex) or start one (hex, unit from hand, verb).
const autoStepJS = `(function () {
	var bar = document.querySelector(".status-bar");
	if (!bar || !bar.classList.contains("your-turn")) return "wait";
	var pending = !!document.getElementById("pending-move");
	function click(el, what) {
		el.dispatchEvent(new MouseEvent("click", { bubbles: true, cancelable: true }));
		return what;
	}
	var count = document.querySelector("#count-picker button");
	var act = document.querySelector(".action-buttons button");
	var cell = document.querySelector("#board .cell.clickable");
	var piece = document.querySelector(".hand-piece.choosable");
	if (count) return click(count, "count " + count.textContent);
	if (pending) {
		if (act) return click(act, "action " + act.getAttribute("data-action"));
		if (cell) return click(cell, "cell " + cell.getAttribute("data-position"));
	}
	if (cell) return click(cell, "cell " + cell.getAttribute("data-position"));
	if (piece) return click(piece, "piece " + piece.getAttribute("data-piece"));
	if (act) return click(act, "action " + act.getAttribute("data-action"));
	return "stuck";
})()`

func TestWebCourtfall_PlayThroughSetupIntoTheGame(t *testing.T) {
	baseURL := newRESTTestServer(t)
	var created struct {
		RoomCode string `json:"room_code"`
	}
	if code := doJSON(t, "POST", baseURL+"/api/games", map[string]any{"game_def_id": "courtfall", "player_name": "Alice", "opponent": "random"}, &created); code != 200 {
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
	openGameAs(t, ctx, baseURL, created.RoomCode, "courtfall", "Alice")
	waitJS(t, ctx, "board", `document.querySelectorAll('#board .cell').length === 37`, 15*time.Second)

	stuck := 0
	var log []string
	for i := 0; i < 260 && stuck < 40; i++ {
		var what string
		if err := chromedp.Run(ctx, chromedp.Evaluate(autoStepJS, &what)); err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
		log = append(log, what)
		switch what {
		case "wait", "stuck":
			stuck++
			time.Sleep(80 * time.Millisecond)
		default:
			stuck = 0
			time.Sleep(60 * time.Millisecond)
		}
		st := cfGet(t, baseURL, created.RoomCode)
		if st.State.Ext.Phase == "play" && st.MoveSeq >= 36 {
			break
		}
		if st.Status == "finished" {
			break
		}
	}

	st := cfGet(t, baseURL, created.RoomCode)
	if st.State.Ext.Phase != "play" || st.MoveSeq < 24 {
		t.Fatalf("the UI should have driven the game into play: phase %q after %d moves; last steps %v", st.State.Ext.Phase, st.MoveSeq, log[max(0, len(log)-12):])
	}
	// The board shows terrain tints, ten units and the scoreboards.
	waitJS(t, ctx, "ten units", `document.querySelectorAll('#board .piece').length === 10`, 10*time.Second)
	if n := evalInt(t, ctx, `document.querySelectorAll('#board .cell-bg.tint').length`); n < 10 {
		t.Errorf("only %d tinted hexes", n)
	}
	waitJS(t, ctx, "scoreboards", `document.querySelectorAll('.seat-strip [data-counter="momentum"]').length === 2 && document.querySelectorAll('.seat-strip .court-tag').length === 2`, 5*time.Second)
	waitJS(t, ctx, "the history tab", `document.getElementById("events") !== null && document.getElementById("tab-history") !== null && document.getElementById("tab-moves") !== null`, 5*time.Second)
	// The terrain legend is in the rules window now, as its Key tab.
	clickSel(t, ctx, "#show-rules")
	waitJS(t, ctx, "the key", `document.querySelector('.rules-tab[data-tab="key"]') !== null`, 5*time.Second)
	clickSel(t, ctx, `.rules-tab[data-tab="key"]`)
	waitJS(t, ctx, "key rows", `document.querySelectorAll('#rules-panel-key tbody tr').length >= 3`, 5*time.Second)
	mu.Lock()
	defer mu.Unlock()
	if len(jsErrors) > 0 {
		t.Errorf("uncaught JS exceptions: %v", jsErrors)
	}
}

func TestWebCourtfall_TheTargetAnswersAStrikeAndSeesTheDice(t *testing.T) {
	baseURL := newRESTTestServer(t)
	room := newTakRoom(t, baseURL, "courtfall", "Alice")
	joinRoom(t, baseURL, room, "Bob")
	rng := rand.New(rand.NewSource(11))
	seat, strike := cfToStrike(t, baseURL, room, rng)
	attacker, defender := cfNames[seat], cfNames[3-seat]
	if code, out := moveNotation(t, baseURL, room, attacker, strike); code != 200 {
		t.Fatalf("strike %s: %d %v", strike, code, out)
	}

	ctx, cancel := newChromedpContext(t, 90*time.Second)
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
	openGameAs(t, ctx, baseURL, room, "courtfall", defender)
	waitJS(t, ctx, "answer prompt", `document.querySelectorAll("#count-picker button").length >= 1`, 15*time.Second)
	waitJS(t, ctx, "committed in secret", `document.getElementById("exchange") && document.getElementById("exchange").textContent.indexOf("committed in secret") >= 0 && document.getElementById("exchange").getAttribute("data-resolved") === "0"`, 5*time.Second)
	if err := chromedp.Run(ctx, chromedp.Evaluate(`document.querySelector("#count-picker button").click(); true`, nil)); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "resolved exchange with dice", `document.getElementById("exchange") && document.getElementById("exchange").getAttribute("data-resolved") === "1" && document.querySelectorAll("#exchange .die").length >= 2`, 15*time.Second)
	waitJS(t, ctx, "narration", `document.querySelectorAll("#events li").length >= 2`, 5*time.Second)

	// The attacker's page shows the same resolved dice after the answer.
	openGameAs(t, ctx, baseURL, room, "courtfall", attacker)
	waitJS(t, ctx, "attacker sees dice", `document.querySelectorAll("#exchange .die").length >= 2`, 15*time.Second)
	mu.Lock()
	defer mu.Unlock()
	if len(jsErrors) > 0 {
		t.Errorf("uncaught JS exceptions: %v", jsErrors)
	}
}
