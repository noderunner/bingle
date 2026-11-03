package app_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

// fitsTheWindowJS is true when nothing of the fitted page lies below or beside the
// window, and every cell of the board is on the screen.
const fitsTheWindowJS = `(function () {
	var page = document.scrollingElement;
	if (page.scrollHeight > innerHeight || page.scrollWidth > innerWidth) return false;
	var rail = document.getElementById("play-rail").getBoundingClientRect();
	if (rail.bottom > innerHeight + 0.5 || rail.right > innerWidth + 0.5) return false;
	return [...document.querySelectorAll("#board .cell")].every(function (c) {
		var r = c.getBoundingClientRect();
		return r.top >= 0 && r.left >= 0 && r.bottom <= innerHeight + 0.5 && r.right <= innerWidth + 0.5;
	});
})()`

// The board's place on the page, to see that it stays there.
const boardPlaceJS = `JSON.stringify(document.querySelector("#board svg").getBoundingClientRect())`

// fittedGame opens a Blooms game against a bot at the size of window asked for.
func fittedGame(t *testing.T, ctx context.Context, base, def string, w, h int64) {
	t.Helper()
	var created struct {
		RoomCode string `json:"room_code"`
	}
	if code := doJSON(t, "POST", base+"/api/games", map[string]any{"game_def_id": def, "player_name": "Alice", "opponent": "random"}, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	if err := chromedp.Run(ctx, chromedp.EmulateViewport(w, h)); err != nil {
		t.Fatal(err)
	}
	openGameAs(t, ctx, base, created.RoomCode, def, "Alice")
	waitJS(t, ctx, "the stones", `document.querySelectorAll("#action-card #piece-buttons button").length === 2`, 15*time.Second)
}

// At every size of window the page stays still: the board is all in view with the
// cards beside it (under it, on an upright phone), and the cards scroll on their own.
func TestWebFitScreen_TheBoardStaysInViewAtEverySize(t *testing.T) {
	base := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 120*time.Second)
	defer cancel()
	for _, size := range []struct {
		def   string
		w, h  int64
		below bool
	}{
		{"blooms", 1920, 1080, false},
		{"blooms", 1280, 720, false},
		{"blooms-6", 1024, 600, false},
		{"blooms", 844, 390, false},
		{"blooms", 390, 844, true},
		{"blooms", 360, 640, true},
	} {
		name := fmt.Sprintf("%s at %dx%d", size.def, size.w, size.h)
		fittedGame(t, ctx, base, size.def, size.w, size.h)
		if !evalBool(t, ctx, fitsTheWindowJS) {
			t.Errorf("%s: the page does not fit the window", name)
		}
		beside := `document.getElementById("play-rail").getBoundingClientRect().left >= document.querySelector("#board svg").getBoundingClientRect().right`
		under := `document.getElementById("play-rail").getBoundingClientRect().top >= document.querySelector("#board svg").getBoundingClientRect().bottom`
		if want := map[bool]string{false: beside, true: under}[size.below]; !evalBool(t, ctx, want) {
			t.Errorf("%s: the cards are not where they should be", name)
		}
		// A board that is too small to play on would also "fit".
		if !evalBool(t, ctx, `Math.min(document.querySelector("#board svg").getBoundingClientRect().width, document.querySelector("#board svg").getBoundingClientRect().height) >= 220`) {
			t.Errorf("%s: the board is too small: %s", name, evalString(t, ctx, boardPlaceJS))
		}
		before := evalString(t, ctx, boardPlaceJS)
		evalBool(t, ctx, `(function () { var r = document.getElementById("play-rail"); r.scrollTop = r.scrollHeight; return true; })()`)
		if after := evalString(t, ctx, boardPlaceJS); after != before {
			t.Errorf("%s: scrolling the cards moved the board: %s then %s", name, before, after)
		}
		if !evalBool(t, ctx, `scrollY === 0`) {
			t.Errorf("%s: the page itself scrolled", name)
		}
	}
}

// When the turn comes to a player who has scrolled down to the chat and is typing, the
// actions are in view at once, and nothing else moves: not the cards, not the cursor.
func TestWebFitScreen_TheTurnBringsTheActionsIntoView(t *testing.T) {
	base := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	var created struct {
		RoomCode string `json:"room_code"`
	}
	alice := newGuestKey(t, base)
	if code := doJSONAs(t, alice, "POST", base+"/api/games", map[string]any{"game_def_id": "blooms", "player_name": "Alice"}, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	room := created.RoomCode
	if err := chromedp.Run(ctx, chromedp.EmulateViewport(1024, 600), chromedp.Navigate(base+"/")); err != nil {
		t.Fatal(err)
	}
	var joined bool
	join := fmt.Sprintf(`fetch("/api/games/%s/join", {method: "POST", headers: {"Content-Type": "application/json", "X-Bingle-Session": "1"}, body: JSON.stringify({player_name: "Bob"})}).then(r => r.ok)`, room)
	if err := chromedp.Run(ctx, chromedp.Evaluate(join, &joined, func(p *runtime.EvaluateParams) *runtime.EvaluateParams { return p.WithAwaitPromise(true) })); err != nil || !joined {
		t.Fatalf("join: %v %v", joined, err)
	}
	openGameAs(t, ctx, base, room, "blooms", "Bob")

	// Alice's turn: the card is there, quietly, and scrolls away with the others.
	waitJS(t, ctx, "the idle card", `document.getElementById("action-idle") !== null`, 15*time.Second)
	if evalBool(t, ctx, `document.getElementById("action-card").classList.contains("acting")`) {
		t.Error("the actions are drawn out on the other player's turn")
	}
	evalBool(t, ctx, `(function () { var r = document.getElementById("play-rail"); r.scrollTop = r.scrollHeight - r.clientHeight - 60; document.getElementById("chat-input").focus(); return true; })()`)
	if !evalBool(t, ctx, `document.getElementById("play-rail").scrollTop > 40`) {
		t.Fatal("the cards should need scrolling in a window this small")
	}
	scrolled := evalString(t, ctx, `String(document.getElementById("play-rail").scrollTop)`)
	if evalBool(t, ctx, `document.getElementById("action-card").getBoundingClientRect().bottom > document.getElementById("play-rail").getBoundingClientRect().top`) {
		t.Error("the idle card should have scrolled away")
	}

	var legal struct {
		Moves []struct {
			Notation string `json:"notation"`
		} `json:"moves"`
	}
	if code := doJSON(t, "GET", base+"/api/games/"+room+"/moves/legal", nil, &legal); code != 200 || len(legal.Moves) == 0 {
		t.Fatalf("legal: %d", code)
	}
	if code := moveAs(t, alice, base, room, "Alice", legal.Moves[0].Notation); code != 200 {
		t.Fatalf("move: %d", code)
	}

	waitJS(t, ctx, "the actions", `document.querySelector("#action-card.acting #piece-buttons button") !== null`, 15*time.Second)
	// The history calls the player by the seat's name, in its colours.
	waitJS(t, ctx, "the log", `/Warm places a/.test(document.getElementById("events").textContent) && document.querySelector("#events .seat-word") !== null`, 5*time.Second)
	if !evalBool(t, ctx, `(function () {
		var rail = document.getElementById("play-rail").getBoundingClientRect();
		var card = document.getElementById("action-card").getBoundingClientRect();
		return card.top >= rail.top && card.bottom <= rail.bottom;
	})()`) {
		t.Error("the actions are not in view on the player's turn")
	}
	if got := evalString(t, ctx, `String(document.getElementById("play-rail").scrollTop)`); got != scrolled {
		t.Errorf("the cards moved from %s to %s when the turn came", scrolled, got)
	}
	waitJS(t, ctx, "the keyboard still in the chat", `document.activeElement && document.activeElement.id === "chat-input"`, 5*time.Second)
	evalBool(t, ctx, `(document.querySelector("#piece-buttons button").click(), true)`)
	waitJS(t, ctx, "the stone chosen", `document.getElementById("pending-move") !== null`, 10*time.Second)
	if got := evalString(t, ctx, `String(document.getElementById("play-rail").scrollTop)`); got != scrolled {
		t.Errorf("a redraw moved the cards from %s to %s", scrolled, got)
	}
}

// The tutorial's words are beside the board, and it fits as well.
func TestWebFitScreen_TheTutorialIsBesideTheBoard(t *testing.T) {
	base := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.EmulateViewport(1280, 720)); err != nil {
		t.Fatal(err)
	}
	openTutorialOf(t, ctx, base, "blooms", "blooms-basics")
	if !evalBool(t, ctx, `document.querySelector("#play-rail > #tutorial") === document.getElementById("play-rail").firstElementChild`) {
		t.Error("the tutorial should come first among the cards")
	}
	if !evalBool(t, ctx, fitsTheWindowJS) {
		t.Error("the tutorial page does not fit the window")
	}
	// A long step of the tutorial does not push the stones out of reach.
	if err := chromedp.Run(ctx, chromedp.EmulateViewport(1024, 500)); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the actions in view", `(function () {
		var rail = document.getElementById("play-rail").getBoundingClientRect();
		var card = document.getElementById("action-card").getBoundingClientRect();
		return document.getElementById("play-rail").scrollHeight > rail.height && card.top >= rail.top && card.bottom <= rail.bottom;
	})()`, 5*time.Second)
}

// Only Blooms has this layout so far, and a game of Blooms in its lobby does not.
func TestWebFitScreen_OtherGamesKeepTheirPage(t *testing.T) {
	base := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	var created struct {
		RoomCode string `json:"room_code"`
	}
	if code := doJSON(t, "POST", base+"/api/games", map[string]any{"game_def_id": "tak-5x5", "player_name": "Alice", "opponent": "random"}, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	if err := chromedp.Run(ctx, chromedp.EmulateViewport(1280, 720)); err != nil {
		t.Fatal(err)
	}
	openGameAs(t, ctx, base, created.RoomCode, "tak-5x5", "Alice")
	waitJS(t, ctx, "the page", `document.querySelector(".game-layout") !== null`, 15*time.Second)
	if evalBool(t, ctx, `document.body.classList.contains("fit-screen") || document.getElementById("play-rail") !== null`) {
		t.Error("Tak was given the fitted layout")
	}
}

// The bar over the board is gone while a game goes on, and its words are for a screen
// reader. The cards say what is going on, in Blooms' own words.
func TestWebFitScreen_TheCardsSayWhatTheBarSaid(t *testing.T) {
	base := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	var created struct {
		RoomCode string `json:"room_code"`
	}
	if code := doJSON(t, "POST", base+"/api/games", map[string]any{"game_def_id": "blooms-4", "player_name": "Alice", "opponent": "random"}, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	if err := chromedp.Run(ctx, chromedp.EmulateViewport(1280, 720)); err != nil {
		t.Fatal(err)
	}
	openGameAs(t, ctx, base, created.RoomCode, "blooms-4", "Alice")
	waitJS(t, ctx, "the stones", `document.querySelectorAll("#piece-buttons button").length === 2`, 15*time.Second)
	if evalBool(t, ctx, `document.querySelector(".status-bar") !== null`) {
		t.Error("the status bar is drawn during the game")
	}
	if got := evalString(t, ctx, `document.getElementById("move-hint").textContent`); got != "Choose a stone to place." {
		t.Errorf("the first hint is %q", got)
	}
	if got := evalString(t, ctx, `[...document.querySelectorAll("#piece-buttons button")].map((b) => b.innerText.trim()).join()`); got != "Place,Place" {
		t.Errorf("the stones' buttons say %q", got)
	}
	if got := evalString(t, ctx, `document.querySelector('#seat-1 [data-counter="captured"]').innerText`); got != "Captured: 0/15" {
		t.Errorf("the counter says %q", got)
	}
	if got := evalString(t, ctx, `[...document.querySelectorAll(".seat-strip .seat-word")].map((w) => w.textContent).sort().join()`); got != "Cool,Warm" {
		t.Errorf("the seats are called %q", got)
	}
	if !evalBool(t, ctx, `getComputedStyle(document.querySelector("#seat-1 .seat-word")).backgroundImage.startsWith("linear-gradient") && getComputedStyle(document.querySelector("#seat-1 .swatch")).backgroundImage.startsWith("linear-gradient")`) {
		t.Error("the seat's word and swatch should be gradients")
	}

	// The opening turn is one stone.
	evalBool(t, ctx, `(document.querySelector('#piece-buttons button').click(), true)`)
	waitJS(t, ctx, "the cells", `document.querySelector("#board .cell.clickable") !== null`, 10*time.Second)
	evalBool(t, ctx, `(document.querySelector("#board .cell.clickable").dispatchEvent(new MouseEvent("click", {bubbles: true})), true)`)
	// The turn comes back from the bot, and after a first stone the hint says what is left.
	waitJS(t, ctx, "the stones", `document.querySelector("#action-card.acting #piece-buttons button") !== null`, 20*time.Second)
	evalBool(t, ctx, `(document.querySelector('#piece-buttons button').click(), true)`)
	waitJS(t, ctx, "the cells", `document.querySelector("#board .cell.clickable") !== null`, 10*time.Second)
	evalBool(t, ctx, `(document.querySelector("#board .cell.clickable").dispatchEvent(new MouseEvent("click", {bubbles: true})), true)`)
	waitJS(t, ctx, "the second stone", `document.getElementById("move-hint") && document.getElementById("move-hint").textContent === "Place or end."`, 10*time.Second)
	if got := evalString(t, ctx, `document.querySelector('#move-bar button[data-action="done"]').innerText.trim()`); got != "End" {
		t.Errorf("the end of the turn says %q", got)
	}
}

// Back returns to an earlier step with its board as it stood. Nothing is played from
// there, and Next comes forward to where the learner was.
func TestWebTutorial_BackLooksAtEarlierSteps(t *testing.T) {
	base := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.EmulateViewport(1280, 800)); err != nil {
		t.Fatal(err)
	}
	room := openTutorial(t, ctx, base)
	stones := `[...document.querySelectorAll("#board .cell")].filter(c => c.dataset.height !== "0").length`
	waitStep(t, ctx, "1")
	if evalBool(t, ctx, `document.getElementById("tutorial-back") !== null`) {
		t.Error("the first step has nothing to go back to")
	}
	for _, n := range []string{"2", "3", "4", "5", "6"} {
		clickSel(t, ctx, "#tutorial-next")
		waitStep(t, ctx, n)
	}
	// A step to read, and a picture.
	clickSel(t, ctx, "#tutorial-back")
	waitStep(t, ctx, "5")
	waitJS(t, ctx, "its picture", stones+` === 5`, 10*time.Second)
	clickSel(t, ctx, "#tutorial-next")
	waitStep(t, ctx, "6")
	waitJS(t, ctx, "the room's board", stones+` === 0`, 10*time.Second)

	// Play on to step 7, where moves have been made.
	waitJS(t, ctx, "red picked out", `!!document.querySelector('button[data-piece="red"].tutorial-target')`, 10*time.Second)
	clickSel(t, ctx, `button[data-piece="red"]`)
	waitJS(t, ctx, "d4", `[...document.querySelectorAll("#board .cell.clickable")].map(c => c.dataset.position).join(",") === "d4"`, 10*time.Second)
	clickCell(t, ctx, "d4")
	waitStep(t, ctx, "7")
	waitJS(t, ctx, "the replies", stones+` === 3`, 10*time.Second)
	played := cfGet(t, base, room).MoveSeq

	// Back to the step before them: the board without the moves, and nothing to click.
	clickSel(t, ctx, "#tutorial-back")
	waitStep(t, ctx, "6")
	waitJS(t, ctx, "the board before the moves", stones+` === 0`, 10*time.Second)
	if got := clickables(t, ctx); got != "" {
		t.Errorf("a step looked back on can be played: %q", got)
	}
	if evalBool(t, ctx, `document.querySelector(".status-bar") !== null || document.getElementById("return-live") !== null`) {
		t.Error("looking back should be steered by the tutorial's own buttons")
	}
	time.Sleep(1200 * time.Millisecond) // long enough for a scripted reply, if one were played
	if got := cfGet(t, base, room).MoveSeq; got != played {
		t.Errorf("looking back played moves: %d then %d", played, got)
	}

	// Forward again, to go on with the tutorial as if nothing had happened.
	clickSel(t, ctx, "#tutorial-next")
	waitStep(t, ctx, "7")
	waitJS(t, ctx, "the board as it is", stones+` === 3`, 10*time.Second)
	waitJS(t, ctx, "red picked out", `!!document.querySelector('button[data-piece="red"].tutorial-target')`, 10*time.Second)
	clickSel(t, ctx, `button[data-piece="red"]`)
	waitJS(t, ctx, "b3", `[...document.querySelectorAll("#board .cell.clickable")].map(c => c.dataset.position).join(",") === "b3"`, 10*time.Second)
}
