package app_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/chromedp"
)

// The list of games: a Beta mark while a game is below 1.0, the players each
// game takes, a card with the game's details while the pointer is over it, and
// no summary or board-size chips (there was no room for them).
func TestHomeGames_ListShowsBetaPlayersAndADetailsCard(t *testing.T) {
	baseURL := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx,
		chromedp.Navigate(baseURL+"/"),
		chromedp.WaitVisible(`.game-card[data-family="tak-5x5"]`, chromedp.ByQuery),
	); err != nil {
		t.Fatal(err)
	}
	// Gone from the cards: the summary, and the board sizes of the variants.
	if n := evalInt(t, ctx, `document.querySelectorAll(".game-card-desc, .game-card .chip").length`); n != 0 {
		t.Errorf("%d summaries or size chips are still on the cards", n)
	}
	// Every game is a beta for now, and says how many can play it.
	if n := evalInt(t, ctx, `document.querySelectorAll(".game-card").length`); n < 20 {
		t.Fatalf("%d game cards", n)
	}
	if n := evalInt(t, ctx, `document.querySelectorAll(".game-card").length - document.querySelectorAll(".game-card .badge.beta").length`); n != 0 {
		t.Errorf("%d games have no Beta mark", n)
	}
	for family, want := range map[string]string{"tak-5x5": "2 players", "cthulhu-wars": "3–5 players", "nyet": "2–5 players", "hive": "2 players"} {
		got := evalString(t, ctx, fmt.Sprintf(`(function(){ var e = document.querySelector('.game-card[data-family=%q] .game-card-players'); return e ? e.textContent : "" })()`, family))
		if got != want {
			t.Errorf("%s: players = %q, want %q", family, got, want)
		}
	}
	if got := evalString(t, ctx, `document.querySelector('.game-card[data-family="tak-5x5"] .badge.beta').getAttribute("title")`); !strings.Contains(got, "0.1.0") {
		t.Errorf("the Beta mark does not say the version: %q", got)
	}
	// Cthulhu Wars and Hive have pictures of their own: four sigils, and a cluster of tiles.
	waitJS(t, ctx, "the Cthulhu Wars sigils", `document.querySelectorAll('.game-card[data-family="cthulhu-wars"] .game-card-preview svg .game-icon-sigil').length === 4`, 15*time.Second)
	waitJS(t, ctx, "the Hive tiles", `document.querySelectorAll('.game-card[data-family="hive"] .game-card-preview svg .piece').length >= 5`, 15*time.Second)
	// Hovering over a game shows its name, designers, publisher and description.
	hover := func(sel string) {
		var x, y float64
		evalString(t, ctx, fmt.Sprintf(`(function(){ document.querySelector(%q).scrollIntoView({block: "center"}); return "ok" })()`, sel))
		if err := chromedp.Run(ctx, chromedp.Evaluate(fmt.Sprintf(`(function(){ var r = document.querySelector(%q).getBoundingClientRect(); return r.left + r.width / 2 })()`, sel), &x), chromedp.Evaluate(fmt.Sprintf(`(function(){ var r = document.querySelector(%q).getBoundingClientRect(); return r.top + r.height / 2 })()`, sel), &y)); err != nil {
			t.Fatal(err)
		}
		if err := chromedp.Run(ctx, chromedp.ActionFunc(func(c context.Context) error {
			return input.DispatchMouseEvent(input.MouseMoved, x, y).Do(c)
		})); err != nil {
			t.Fatal(err)
		}
	}
	if evalBool(t, ctx, `!!document.getElementById("game-tip") && !document.getElementById("game-tip").hidden`) {
		t.Error("the details card shows before the pointer is over a game")
	}
	hover(`.game-card[data-family="tak-5x5"]`)
	waitJS(t, ctx, "the details card", `!!document.getElementById("game-tip") && !document.getElementById("game-tip").hidden`, 5*time.Second)
	tip := evalString(t, ctx, `document.getElementById("game-tip").textContent`)
	for _, want := range []string{"Tak", "James Ernest", "Patrick Rothfuss", "Cheapass Games", "unbroken road", "0.1.0"} {
		if !strings.Contains(tip, want) {
			t.Errorf("the details card of Tak lacks %q: %q", want, tip)
		}
	}
	// Moving off the game closes it.
	if err := chromedp.Run(ctx, chromedp.ActionFunc(func(c context.Context) error {
		return input.DispatchMouseEvent(input.MouseMoved, 2, 2).Do(c)
	})); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the details card closes", `document.getElementById("game-tip").hidden`, 5*time.Second)
	// A game whose designer or publisher is not known leaves that line out, and never says "null".
	hover(`.game-card[data-family="go-9x9"]`)
	waitJS(t, ctx, "the Go card", `!document.getElementById("game-tip").hidden && document.getElementById("game-tip").textContent.indexOf("Go") >= 0`, 5*time.Second)
	if got := evalString(t, ctx, `document.getElementById("game-tip").textContent`); strings.Contains(got, "null") || strings.Contains(got, "Designer") || strings.Contains(got, "Publisher") || strings.Contains(got, "Traditional") {
		t.Errorf("the Go card lists what nobody knows: %q", got)
	}
	hover(`.game-card[data-family="courtfall"]`)
	waitJS(t, ctx, "the Courtfall card", `document.getElementById("game-tip").textContent.indexOf("Courtfall") >= 0`, 5*time.Second)
	if got := evalString(t, ctx, `document.getElementById("game-tip").textContent`); !strings.Contains(got, "Miles Stevenson") || strings.Contains(got, "null") {
		t.Errorf("the Courtfall card: %q", got)
	}
	// A game with no known publisher leaves the line out.
	hover(`.game-card[data-family="blooms"]`)
	waitJS(t, ctx, "the Blooms card", `!document.getElementById("game-tip").hidden && document.getElementById("game-tip").textContent.indexOf("Nick Bentley") >= 0`, 5*time.Second)
	if got := evalString(t, ctx, `document.getElementById("game-tip").textContent`); strings.Contains(got, "Publisher") || strings.Contains(got, "null") {
		t.Errorf("the Blooms card names a publisher nobody gave: %q", got)
	}
	// The setup of a game: the rules button is the rule book, named Rules.
	clickSel(t, ctx, `.game-card[data-family="tak-5x5"]`)
	waitJS(t, ctx, "setup", `!document.getElementById("game-setup").hidden`, 5*time.Second)
	if n := evalInt(t, ctx, `document.querySelectorAll('#game-rules-button svg[data-icon="rules"]').length`); n != 1 {
		t.Errorf("the rules button does not carry the rule book (%d)", n)
	}
	if got := evalString(t, ctx, `document.getElementById("game-rules-button").getAttribute("aria-label")`); got != "Rules" {
		t.Errorf("the rules button is named %q", got)
	}
	if got := evalString(t, ctx, `document.getElementById("game-rules-button").textContent.trim()`); got != "" {
		t.Errorf("the rules button still holds the word %q", got)
	}
	if evalBool(t, ctx, `!document.getElementById("game-tip").hidden`) {
		t.Error("the details card stays open after a game is picked")
	}
}

// A game that starts with an empty board shows a sample game in play on its card.
func TestHomeGames_EmptyStartsShowASampleGame(t *testing.T) {
	baseURL := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.Navigate(baseURL+"/"), chromedp.WaitVisible(`.game-card[data-family="tak-5x5"]`, chromedp.ByQuery)); err != nil {
		t.Fatal(err)
	}
	for _, family := range []string{"gomoku", "yavalath", "go-9x9", "hex-11", "havannah-8", "pente", "connect-four", "tic-tac-toe", "tak-5x5", "blooms", "yinsh", "courtfall"} {
		waitJS(t, ctx, family+": pieces on the card", fmt.Sprintf(`document.querySelectorAll('.game-card[data-family=%q] .game-card-preview svg .piece').length >= 4`, family), 20*time.Second)
	}
	// Nyet! has no board: a picture of cards.
	waitJS(t, ctx, "the Nyet! cards", `document.querySelectorAll('.game-card[data-family="nyet"] .game-card-preview svg rect').length >= 5`, 10*time.Second)
	// The setup page shows the same sample.
	clickSel(t, ctx, `.game-card[data-family="gomoku"]`)
	waitJS(t, ctx, "the setup preview", `document.querySelectorAll('#setup-preview svg .piece').length >= 6`, 10*time.Second)
}
