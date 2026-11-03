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
