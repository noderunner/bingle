// Tests of the tutorial page (ADR 0077): the Blooms tutorial is played through
// the browser from its first step to the end — highlights, the hint, only the
// expected move clickable, the scripted replies played by the page, Next on a
// step to read, progress surviving a reload — and then finished against a bot.

package app_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

// openTutorial starts the Blooms tutorial from the browser, as the list of
// tutorials will, and opens its room.
func openTutorial(t *testing.T, ctx context.Context, base string) string {
	t.Helper()
	if err := chromedp.Run(ctx, chromedp.Navigate(base+"/")); err != nil {
		t.Fatal(err)
	}
	var room string
	start := `fetch("/api/tutorials", {method: "POST", headers: {"Content-Type": "application/json", "X-Bingle-Session": "1"},
		body: JSON.stringify({game_def_id: "blooms", tutorial: "blooms-basics", player_name: "Lia"})}).then(r => r.json()).then(j => j.room_code)`
	if err := chromedp.Run(ctx, chromedp.Evaluate(start, &room, func(p *runtime.EvaluateParams) *runtime.EvaluateParams { return p.WithAwaitPromise(true) })); err != nil || room == "" {
		t.Fatalf("starting the tutorial: %q %v", room, err)
	}
	if err := chromedp.Run(ctx, chromedp.Navigate(base+"/game/"+room)); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the tutorial card", `document.getElementById("tutorial-count")`, 15*time.Second)
	return room
}

func waitStep(t *testing.T, ctx context.Context, n string) {
	t.Helper()
	waitJS(t, ctx, "step "+n, `(document.getElementById("tutorial-count") || {}).textContent === "Step `+n+` of 4"`, 15*time.Second)
}

// clickables are the board's squares that can be clicked.
func clickables(t *testing.T, ctx context.Context) string {
	t.Helper()
	return evalString(t, ctx, `[...document.querySelectorAll("#board .cell.clickable")].map(c => c.dataset.position).sort().join(",")`)
}

func TestWebTutorial_PlaysThroughTheBloomsTutorial(t *testing.T) {
	base := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 120*time.Second)
	defer cancel()
	room := openTutorial(t, ctx, base)

	// Step 1: the prose, the highlighted cell, and only that cell to click.
	waitStep(t, ctx, "1")
	if text := evalString(t, ctx, `document.getElementById("tutorial-text").innerText`); !strings.Contains(text, "Blooms") {
		t.Fatalf("the first step's text: %q", text)
	}
	if !evalBool(t, ctx, `!!document.querySelector('#board .cell[data-position="d4"] .hint-ring')`) {
		t.Fatal("d4 is not highlighted")
	}
	waitJS(t, ctx, "d4 to be the only move", `[...document.querySelectorAll("#board .cell.clickable")].map(c => c.dataset.position).join(",") === "d4"`, 10*time.Second)
	clickSel(t, ctx, "#tutorial-show-hint")
	waitJS(t, ctx, "the hint", `(document.getElementById("tutorial-hint") || {}).textContent === "Click the highlighted cell in the middle of the board."`, 5*time.Second)

	clickCell(t, ctx, "d4")
	// The page plays the two scripted replies for the other seat.
	waitStep(t, ctx, "2")
	waitJS(t, ctx, "the replies on the board", `!!document.querySelector('#board .cell[data-position="a4"] .piece, #board .cell[data-position="a4"] [class*="stone"], #board .cell[data-position="a4"][data-height="1"]')`, 10*time.Second)
	if evalBool(t, ctx, `!!document.getElementById("tutorial-hint")`) {
		t.Fatal("the first step's hint is still shown")
	}

	// Step 2: two stones of different colours.
	waitJS(t, ctx, "b3 to be the move", `[...document.querySelectorAll("#board .cell.clickable")].map(c => c.dataset.position).join(",") === "b3"`, 10*time.Second)
	clickCell(t, ctx, "b3")
	waitJS(t, ctx, "b4 to be the move", `[...document.querySelectorAll("#board .cell.clickable")].map(c => c.dataset.position).join(",") === "b4"`, 10*time.Second)

	// Progress comes from the moves played: a reload finds the same step.
	if err := chromedp.Run(ctx, chromedp.Reload()); err != nil {
		t.Fatal(err)
	}
	waitStep(t, ctx, "2")
	waitJS(t, ctx, "b4 to be the move again", `[...document.querySelectorAll("#board .cell.clickable")].map(c => c.dataset.position).join(",") === "b4"`, 10*time.Second)
	clickCell(t, ctx, "b4")

	// Step 3: the arrow, a5, then done; the blue stone at a4 is captured.
	waitStep(t, ctx, "3")
	if !evalBool(t, ctx, `!!document.querySelector("#board .board-arrow")`) {
		t.Fatal("step 3 draws no arrow")
	}
	waitJS(t, ctx, "a5 to be the move", `[...document.querySelectorAll("#board .cell.clickable")].map(c => c.dataset.position).join(",") === "a5"`, 10*time.Second)
	clickCell(t, ctx, "a5")
	finishTurn(t, ctx)

	// Step 4 is to read: Finish ends the tutorial.
	waitStep(t, ctx, "4")
	if !evalBool(t, ctx, `document.querySelector('#board .cell[data-position="a4"]').dataset.height === "0"`) {
		t.Fatal("the blue stone at a4 was not captured")
	}
	if got := clickables(t, ctx); got != "" {
		t.Fatalf("a step to read offers moves: %q", got)
	}
	clickSel(t, ctx, "#tutorial-next")
	waitJS(t, ctx, "the end", `!!document.getElementById("tutorial-done")`, 10*time.Second)
	if !evalBool(t, ctx, `JSON.parse(localStorage.getItem("bingle.tutorials.done") || "[]").includes("blooms/blooms-basics")`) {
		t.Fatal("the tutorial is not marked finished in the browser")
	}

	// Finishing against a bot opens an ordinary game from this position.
	clickSel(t, ctx, "#tutorial-bot")
	waitJS(t, ctx, "a new room", `location.pathname !== "/game/`+room+`" && location.pathname.startsWith("/game/")`, 15*time.Second)
	waitJS(t, ctx, "its board", `!!document.querySelector("#board-svg")`, 15*time.Second)
	if evalBool(t, ctx, `!!document.getElementById("tutorial")`) {
		t.Fatal("the game against the bot is still a tutorial")
	}
	if !evalBool(t, ctx, `document.querySelector('#board .cell[data-position="b3"]').dataset.height === "1"`) {
		t.Fatal("the game against the bot did not start from the tutorial's position")
	}
}

// finishTurn ends a Blooms turn after one stone: the "End turn" button of
// the move bar, the done decision.
func finishTurn(t *testing.T, ctx context.Context) {
	t.Helper()
	waitJS(t, ctx, "the End turn button", `!!document.querySelector('#move-bar button[data-action="done"]')`, 10*time.Second)
	clickSel(t, ctx, `#move-bar button[data-action="done"]`)
}

// Undoing a move leaves the script: the page says so and offers a fresh start.
func TestWebTutorial_OffScript(t *testing.T) {
	base := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	room := openTutorial(t, ctx, base)
	waitStep(t, ctx, "1")
	// Another move than the script's, made outside the page (as a second tab could).
	move := `fetch("/api/games/` + room + `/moves", {method: "POST", headers: {"Content-Type": "application/json", "X-Bingle-Session": "1"},
		body: JSON.stringify({player_name: "Lia", notation: "yellow@e5"})}).then(r => r.status)`
	var status int
	if err := chromedp.Run(ctx, chromedp.Evaluate(move, &status, func(p *runtime.EvaluateParams) *runtime.EvaluateParams { return p.WithAwaitPromise(true) })); err != nil || status != 200 {
		t.Fatalf("the off-script move: %d %v", status, err)
	}
	if err := chromedp.Run(ctx, chromedp.Reload()); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the off-script notice", `!!document.getElementById("tutorial-off")`, 15*time.Second)
	clickSel(t, ctx, "#tutorial-restart")
	waitJS(t, ctx, "a fresh room", `location.pathname !== "/game/`+room+`" && !!document.getElementById("tutorial-count")`, 15*time.Second)
	waitStep(t, ctx, "1")
}
