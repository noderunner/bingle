// Tests of the tutorial page (ADR 0077): the Blooms tutorial is played through
// the browser from its first step to the end — highlights, the hint, only the
// expected move clickable, the scripted replies played by the page, Next on a
// step to read, progress surviving a reload — and then finished against a bot.

package app_test

import (
	"context"
	"strconv"
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
	return openTutorialOf(t, ctx, base, "blooms", "blooms-basics")
}

// openTutorialOf starts a tutorial of a game from the browser and opens its room.
func openTutorialOf(t *testing.T, ctx context.Context, base, game, id string) string {
	t.Helper()
	if err := chromedp.Run(ctx, chromedp.Navigate(base+"/")); err != nil {
		t.Fatal(err)
	}
	var room string
	start := `fetch("/api/tutorials", {method: "POST", headers: {"Content-Type": "application/json", "X-Bingle-Session": "1"},
		body: JSON.stringify({game_def_id: "` + game + `", tutorial: "` + id + `", player_name: "Lia"})}).then(r => r.json()).then(j => j.room_code)`
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
	waitJS(t, ctx, "step "+n, `/^Step `+n+` of \d+$/.test((document.getElementById("tutorial-count") || {}).textContent || "")`, 15*time.Second)
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
	next := func() { t.Helper(); clickSel(t, ctx, "#tutorial-next") }
	stones := `[...document.querySelectorAll("#board .cell")].filter(c => c.dataset.height !== "0").length`

	// Step 1: what the game is about. There is nobody to talk to: no chat, no voice.
	waitStep(t, ctx, "1")
	if text := evalString(t, ctx, `document.getElementById("tutorial-text").innerText`); !strings.Contains(text, "Blooms") {
		t.Fatalf("the first step's text: %q", text)
	}
	if evalBool(t, ctx, `!!document.getElementById("chat-card") || !!document.querySelector(".voice-card, #voice-card")`) {
		t.Fatal("a tutorial shows the chat or the voice card")
	}
	next()

	// Step 2 shows a finished game; the room's own board is untouched.
	waitStep(t, ctx, "2")
	waitJS(t, ctx, "the finished game on the board", stones+` > 15`, 10*time.Second)
	if text := evalString(t, ctx, `document.getElementById("tutorial-text").innerText`); !strings.Contains(text, "captured 17 stones") {
		t.Fatalf("the end of a game: %q", text)
	}
	next()
	waitStep(t, ctx, "3")
	waitJS(t, ctx, "the room's empty board again", stones+` === 0`, 10*time.Second)
	next()
	waitStep(t, ctx, "4")
	next()

	// Step 5: the colour buttons stay where they are; red is picked out, yellow is inert.
	waitStep(t, ctx, "5")
	waitJS(t, ctx, "red picked out", `!!document.querySelector('button[data-piece="red"].tutorial-target')`, 10*time.Second)
	if !evalBool(t, ctx, `document.querySelector('button[data-piece="yellow"]').disabled`) {
		t.Fatal("the yellow button is not inert")
	}
	if got := clickables(t, ctx); got != "" {
		t.Fatalf("cells can be clicked before the colour is chosen: %q", got)
	}
	clickSel(t, ctx, "#tutorial-show-hint")
	waitJS(t, ctx, "the hint", `/Press the red stone/.test((document.getElementById("tutorial-hint") || {}).textContent || "")`, 5*time.Second)
	clickSel(t, ctx, `button[data-piece="red"]`)
	waitJS(t, ctx, "d4 to be the only cell", `[...document.querySelectorAll("#board .cell.clickable")].map(c => c.dataset.position).join(",") === "d4"`, 10*time.Second)
	clickCell(t, ctx, "d4")

	// The page plays the two scripted replies for the other seat.
	waitStep(t, ctx, "6")
	waitJS(t, ctx, "the replies on the board", `document.querySelector('#board .cell[data-position="a4"]').dataset.height === "1"`, 10*time.Second)

	// Step 6: red on b3, then yellow on b4.
	waitJS(t, ctx, "red picked out", `!!document.querySelector('button[data-piece="red"].tutorial-target')`, 10*time.Second)
	clickSel(t, ctx, `button[data-piece="red"]`)
	waitJS(t, ctx, "b3", `[...document.querySelectorAll("#board .cell.clickable")].map(c => c.dataset.position).join(",") === "b3"`, 10*time.Second)
	clickCell(t, ctx, "b3")
	waitJS(t, ctx, "yellow picked out", `!!document.querySelector('button[data-piece="yellow"].tutorial-target')`, 10*time.Second)

	// Progress comes from the moves played: a reload finds the same step.
	if err := chromedp.Run(ctx, chromedp.Reload()); err != nil {
		t.Fatal(err)
	}
	waitStep(t, ctx, "6")
	waitJS(t, ctx, "yellow picked out again", `!!document.querySelector('button[data-piece="yellow"].tutorial-target')`, 10*time.Second)
	clickSel(t, ctx, `button[data-piece="yellow"]`)
	waitJS(t, ctx, "b4", `[...document.querySelectorAll("#board .cell.clickable")].map(c => c.dataset.position).join(",") === "b4"`, 10*time.Second)
	clickCell(t, ctx, "b4")

	// Step 7: the arrow, red on a5, then End turn; the blue stone at a4 is captured.
	waitStep(t, ctx, "7")
	if !evalBool(t, ctx, `!!document.querySelector("#board .board-arrow")`) {
		t.Fatal("step 7 draws no arrow")
	}
	waitJS(t, ctx, "red picked out", `!!document.querySelector('button[data-piece="red"].tutorial-target')`, 10*time.Second)
	clickSel(t, ctx, `button[data-piece="red"]`)
	waitJS(t, ctx, "a5", `[...document.querySelectorAll("#board .cell.clickable")].map(c => c.dataset.position).join(",") === "a5"`, 10*time.Second)
	clickCell(t, ctx, "a5")
	waitJS(t, ctx, "End turn picked out", `!!document.querySelector('#move-bar button[data-action="done"].tutorial-target')`, 10*time.Second)
	finishTurn(t, ctx)

	// Step 8 is to read: Finish ends the tutorial.
	waitStep(t, ctx, "8")
	waitJS(t, ctx, "the capture", `document.querySelector('#board .cell[data-position="a4"]').dataset.height === "0"`, 10*time.Second)
	if got := clickables(t, ctx); got != "" {
		t.Fatalf("a step to read offers moves: %q", got)
	}
	next()
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

// The game picker shows which games have tutorials, and the chosen game's
// tutorials start under the name in its form.
func TestWebTutorial_ListInTheGamePicker(t *testing.T) {
	base := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.Navigate(base+"/")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the gallery", `!!document.querySelector('.game-card[data-family="blooms"]')`, 15*time.Second)
	if !evalBool(t, ctx, `!!document.querySelector('.game-card[data-family="blooms"] .badge.tutorial')`) {
		t.Fatal("the Blooms card does not say it has a tutorial")
	}
	if evalBool(t, ctx, `!!document.querySelector('.game-card[data-family="gomoku"] .badge.tutorial')`) {
		t.Fatal("the Gomoku card says it has a tutorial")
	}
	clickSel(t, ctx, `.game-card[data-family="blooms"]`)
	waitJS(t, ctx, "the tutorials of the setup", `!document.getElementById("setup-tutorials").hidden && !!document.querySelector('#setup-tutorials button[data-tutorial="blooms-basics"]')`, 10*time.Second)
	if text := evalString(t, ctx, `document.getElementById("setup-tutorials").innerText`); !strings.Contains(text, "Learn to play") || !strings.Contains(text, "How to play Blooms") {
		t.Fatalf("the setup's tutorials: %q", text)
	}

	// Without a name it asks for one; with one it opens the tutorial.
	setName(t, ctx, "")
	clickSel(t, ctx, `#setup-tutorials button[data-tutorial="blooms-basics"]`)
	waitJS(t, ctx, "the request for a name", `!document.querySelector("#setup-tutorials .tutorial-error").hidden`, 5*time.Second)
	setName(t, ctx, "Nia")
	clickSel(t, ctx, `#setup-tutorials button[data-tutorial="blooms-basics"]`)
	waitJS(t, ctx, "the tutorial", `location.pathname.startsWith("/game/") && !!document.getElementById("tutorial-count")`, 15*time.Second)
	waitStep(t, ctx, "1")
	if name := evalString(t, ctx, `[...document.querySelectorAll(".seat-strip, .seat")].map(e => e.innerText).join(" ")`); !strings.Contains(name, "Nia") {
		t.Fatalf("the learner's name is not Nia: %q", name)
	}
}

// The rules window has a Tutorials tab, which marks a finished tutorial and
// starts one under the name given there.
func TestWebTutorial_TabInTheRulesWindow(t *testing.T) {
	base := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx,
		chromedp.Navigate(base+"/"),
		chromedp.Evaluate(`localStorage.setItem("bingle.tutorials.done", JSON.stringify(["blooms/blooms-basics"])); localStorage.setItem("bingle.name", JSON.stringify("Oli")); true`, nil),
		chromedp.Navigate(base+"/?game=blooms"),
	); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the setup", `!document.getElementById("game-setup").hidden`, 15*time.Second)
	clickSel(t, ctx, "#game-rules-button")
	waitJS(t, ctx, "the Tutorials tab", `!!document.querySelector('#rules-dialog [data-tab="tutorials"]')`, 10*time.Second)
	clickSel(t, ctx, `#rules-dialog [data-tab="tutorials"]`)
	waitJS(t, ctx, "the list", `!document.getElementById("rules-panel-tutorials").hidden`, 5*time.Second)
	text := evalString(t, ctx, `document.getElementById("rules-panel-tutorials").innerText`)
	for _, want := range []string{"Learn the rules", "How to play Blooms", "how a bloom is captured", "✓ Done"} {
		if !strings.Contains(text, want) {
			t.Fatalf("the Tutorials tab lacks %q: %q", want, text)
		}
	}
	if v := evalString(t, ctx, `document.getElementById("tutorial-name").value`); v != "Oli" {
		t.Fatalf("the name field: %q, want the remembered name", v)
	}
	// A finished tutorial can be taken again.
	if label := evalString(t, ctx, `document.querySelector('#rules-panel-tutorials button[data-tutorial="blooms-basics"]').textContent`); label != "Again" {
		t.Fatalf("the button of a finished tutorial: %q", label)
	}
	clickSel(t, ctx, `#rules-panel-tutorials button[data-tutorial="blooms-basics"]`)
	waitJS(t, ctx, "the tutorial", `location.pathname.startsWith("/game/") && !!document.getElementById("tutorial-count")`, 15*time.Second)

	// A game without tutorials has no such tab.
	if err := chromedp.Run(ctx, chromedp.Navigate(base+"/?game=gomoku")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the setup", `!document.getElementById("game-setup").hidden`, 15*time.Second)
	clickSel(t, ctx, "#game-rules-button")
	waitJS(t, ctx, "the rules", `!!document.querySelector('#rules-dialog [data-tab]')`, 10*time.Second)
	if evalBool(t, ctx, `!!document.querySelector('#rules-dialog [data-tab="tutorials"]')`) {
		t.Fatal("Gomoku's rules have a Tutorials tab")
	}
	if !evalBool(t, ctx, `document.getElementById("setup-tutorials").hidden`) {
		t.Fatal("Gomoku's setup shows tutorials")
	}
}

// setName types a name into the game picker's form.
func setName(t *testing.T, ctx context.Context, name string) {
	t.Helper()
	if err := chromedp.Run(ctx, chromedp.Evaluate(`document.getElementById("create_player_name").value = `+strconv.Quote(name)+`; true`, nil)); err != nil {
		t.Fatal(err)
	}
}

// The Meridians tutorial, played through: a shown end, a line of sight, and
// a Dark stone that comes off when nothing can see it any more.
func TestWebTutorial_PlaysThroughTheMeridiansTutorial(t *testing.T) {
	base := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 120*time.Second)
	defer cancel()
	openTutorialOf(t, ctx, base, "meridians", "meridians-basics")
	next := func() { t.Helper(); clickSel(t, ctx, "#tutorial-next") }
	stones := `[...document.querySelectorAll("#board .cell")].filter(c => c.dataset.height !== "0").length`
	only := func(pos string) {
		t.Helper()
		waitJS(t, ctx, pos+" to be the only point", `[...document.querySelectorAll("#board .cell.clickable")].map(c => c.dataset.position).join(",") === "`+pos+`"`, 10*time.Second)
		clickCell(t, ctx, pos)
	}

	waitStep(t, ctx, "1")
	next()
	waitStep(t, ctx, "2")
	waitJS(t, ctx, "the finished game", stones+` > 20`, 10*time.Second)
	next()
	waitStep(t, ctx, "3")
	waitJS(t, ctx, "the empty board", stones+` === 0`, 10*time.Second)
	if !evalBool(t, ctx, `!!document.querySelector('#board .cell[data-position="e6"] .hint-ring')`) {
		t.Fatal("the line is not highlighted")
	}
	next()
	waitStep(t, ctx, "4")
	next()
	waitStep(t, ctx, "5")
	only("g6")
	waitStep(t, ctx, "6")
	only("g3")
	waitStep(t, ctx, "7")
	if !evalBool(t, ctx, `!!document.querySelector("#board .board-arrow")`) {
		t.Fatal("step 7 draws no arrow")
	}
	only("c6")
	waitStep(t, ctx, "8")
	waitJS(t, ctx, "Dark's reply", `document.querySelector('#board .cell[data-position="f4"]').dataset.height === "1"`, 10*time.Second)
	if !evalBool(t, ctx, `document.querySelector('#board .cell[data-position="c8"]').dataset.height === "0"`) {
		t.Fatal("Dark's stone on c8 is still on the board")
	}
	next()
	waitJS(t, ctx, "the end", `!!document.getElementById("tutorial-done")`, 10*time.Second)
}

// The Mycelium tutorial is played through a whole confluence: spreads are two
// clicks on the board, the second conversion of a hyphae spread one, and what
// each step says has happened to the nodes is what the board shows.
func TestWebTutorial_PlaysThroughTheMyceliumTutorial(t *testing.T) {
	base := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 120*time.Second)
	defer cancel()
	openTutorialOf(t, ctx, base, "mycelium", "mycelium-basics")
	next := func() { t.Helper(); clickSel(t, ctx, "#tutorial-next") }
	nodes := `[...document.querySelectorAll("#board .cell")].filter(c => c.dataset.height !== "0").length`
	clickable := `[...document.querySelectorAll("#board .cell.clickable")].map(c => c.dataset.position).join(",")`
	// play makes one step's move, cell by cell: each click must have taken, the
	// next cell being offered or the step having moved on, since a click that
	// lands in a redraw is lost.
	play := func(n string, cells ...string) {
		t.Helper()
		waitStep(t, ctx, n)
		for i, c := range cells {
			waitJS(t, ctx, c+" to be the only cell", clickable+` === "`+c+`"`, 10*time.Second)
			took := `!/^Step ` + n + ` of /.test(document.getElementById("tutorial-count").textContent)`
			if i+1 < len(cells) {
				took = clickable + ` === "` + cells[i+1] + `"`
			}
			clickCellUntil(t, ctx, c, took, 10*time.Second)
		}
	}
	colours := func(want string) {
		t.Helper()
		js := `[...document.querySelectorAll("#board .cell")].filter(c => c.dataset.height !== "0").map(c => c.dataset.position + ":" + c.querySelector("[data-color]").dataset.color[0]).sort().join(",")`
		waitJS(t, ctx, "the nodes "+want, js+` === "`+want+`"`, 10*time.Second)
	}

	waitStep(t, ctx, "1")
	next()
	waitStep(t, ctx, "2")
	waitJS(t, ctx, "the finished game", nodes+` === 18`, 10*time.Second)
	next()
	waitStep(t, ctx, "3")
	waitJS(t, ctx, "the empty board", nodes+` === 0`, 10*time.Second)
	next()
	waitStep(t, ctx, "4")
	next()
	play("5", "i4")
	play("6", "f7")
	play("7", "b8")
	play("8", "b9")
	play("9", "e5")
	waitStep(t, ctx, "10")
	colours("b8:b,b9:b,c8:g,d7:g,d8:g,e5:b,f2:g,f7:g,i4:b")
	next()
	play("11", "i4", "f7")
	play("12", "f7", "f2")
	play("13", "b9", "c8")
	play("14", "d8")
	waitStep(t, ctx, "15")
	colours("b8:b,b9:b,c8:b,d7:g,d8:b,e7:g,f2:b,f7:b,i4:b")
	next()
	waitStep(t, ctx, "16")
	next()
	waitJS(t, ctx, "the end", `!!document.getElementById("tutorial-done")`, 10*time.Second)
}

// tutorialDoNext makes the selection the step points at, however the page
// offers it: the picked-out button, the one unit that can be chosen, or the
// one area or square that can be clicked. It returns what it clicked.
func tutorialDoNext(t *testing.T, ctx context.Context) string {
	t.Helper()
	return evalString(t, ctx, `(() => {
		const click = (el) => el.dispatchEvent(new MouseEvent("click", {bubbles: true}));
		const btn = document.querySelector("button.tutorial-target:not([disabled])");
		if (btn) { btn.click(); return "button " + (btn.dataset.action || btn.dataset.piece || btn.dataset.count); }
		const tok = document.querySelector("#board .token.token-target");
		if (tok) { click(tok); return "unit " + tok.dataset.piece + "@" + tok.dataset.position; }
		const cell = document.querySelector("#board .cell.clickable");
		if (cell) { click(cell.querySelector(".cell-hit") || cell); return "area " + cell.dataset.position; }
		return "";
	})()`)
}

// The High Priests tutorial starts from a game written out at round 2 rather
// than played to, and takes the learner through recruiting, holding a Gate,
// capturing, and the Oath on their own turn and on another's at 0 Power.
func TestWebCthulhuWars_TheHighPriestsTutorial(t *testing.T) {
	base := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 180*time.Second)
	defer cancel()
	openTutorialOf(t, ctx, base, "cthulhu-wars", "high-priests")
	next := func() { t.Helper(); clickSel(t, ctx, "#tutorial-next") }
	count := `(document.getElementById("tutorial-count") || {}).textContent || ""`
	// play makes the step's selections until the tutorial moves on to step n.
	play := func(n string) {
		t.Helper()
		deadline := time.Now().Add(40 * time.Second)
		var did []string
		for time.Now().Before(deadline) {
			if evalString(t, ctx, count) == "Step "+n+" of 11" {
				return
			}
			if got := tutorialDoNext(t, ctx); got != "" {
				did = append(did, got)
			}
			time.Sleep(400 * time.Millisecond)
		}
		t.Fatalf("never reached step %s; clicked %v; the card says %q", n, did, evalString(t, ctx, `document.getElementById("tutorial").innerText`))
	}
	// A unit is a token, or the figure on a Gate (its chip says who holds it).
	units := func(area, piece, color string) bool {
		return evalBool(t, ctx, `!!document.querySelector('#board .token[data-position="`+area+`"][data-piece="`+piece+`"][data-color="`+color+`"], #board .gate-chip[data-position="`+area+`"][data-holder="`+piece+`"][data-color="`+color+`"]')`)
	}

	waitStep(t, ctx, "1")
	// The room is the written-out game: round 2, with the Yellow Sign's High Priest on the map.
	if !units("indian-ocean", "high-priest", "yellow") {
		t.Fatal("the start position is not the written-out round 2")
	}
	next()
	waitStep(t, ctx, "2")
	if !evalBool(t, ctx, `!!document.querySelector('#board .area-hint, #board [class*="area-hint"]')`) {
		t.Fatal("step 2 highlights no area")
	}
	next()
	waitStep(t, ctx, "3")
	next()
	waitStep(t, ctx, "4")
	play("5") // recruit him in Antarctica; the others act
	if !units("antarctica", "high-priest", "green") {
		t.Fatal("the High Priest was not recruited in Antarctica")
	}
	play("6") // pass on the Oath
	play("7") // capture the Yellow Sign's High Priest
	if units("indian-ocean", "high-priest", "yellow") {
		t.Fatal("the Yellow Sign's High Priest was not captured")
	}
	play("8") // the Oath on our own turn
	play("9") // recruit him again: 0 Power
	// Asked about the Oath on Crawling Chaos's turn, the learner is offered
	// no way to stop being asked: the script answers every reaction.
	waitJS(t, ctx, "the Oath asked", `/oath/i.test((document.querySelector("button.tutorial-target") || {}).textContent || "")`, 20*time.Second)
	if evalBool(t, ctx, `!!document.getElementById("ask-shortcuts")`) {
		t.Fatal("the reaction question offers shortcuts in a tutorial")
	}
	play("10") // the Oath at 0 Power, on Crawling Chaos's turn
	play("11") // a Deep One for the Gate
	if !units("south-pacific", "deep-one", "green") {
		t.Fatal("no Deep One in the South Pacific")
	}
	next()
	waitJS(t, ctx, "the end", `!!document.getElementById("tutorial-done")`, 10*time.Second)
}
