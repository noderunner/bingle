package app_test

import (
	"context"
	"fmt"
	"strings"
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
	waitJS(t, ctx, "the stones", `document.querySelectorAll("#impulse-card #piece-buttons button").length === 2`, 15*time.Second)
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
		{"blooms-10", 1024, 600, false},
		{"blooms-10", 390, 844, true},
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
	waitJS(t, ctx, "the idle card", `document.getElementById("impulse-title") !== null && document.getElementById("impulse-title").textContent === "Alice"`, 15*time.Second)
	if got := evalString(t, ctx, `document.getElementById("impulse-text").textContent`); got != "Place or done." {
		t.Errorf("the other player's impulse says %q", got)
	}
	if evalBool(t, ctx, `document.getElementById("impulse-card").classList.contains("acting")`) {
		t.Error("the actions are drawn out on the other player's turn")
	}
	evalBool(t, ctx, `(function () { var r = document.getElementById("play-rail"); r.scrollTop = r.scrollHeight - r.clientHeight - 60; document.getElementById("chat-input").focus(); return true; })()`)
	if !evalBool(t, ctx, `document.getElementById("play-rail").scrollTop > 40`) {
		t.Fatal("the cards should need scrolling in a window this small")
	}
	scrolled := evalString(t, ctx, `String(document.getElementById("play-rail").scrollTop)`)
	if evalBool(t, ctx, `document.getElementById("impulse-card").getBoundingClientRect().bottom > document.getElementById("play-rail").getBoundingClientRect().top`) {
		t.Error("the idle card should have scrolled away")
	}

	var legal struct {
		Moves []struct {
			Notation string `json:"notation"`
		} `json:"options"`
	}
	if code := doJSONAs(t, alice, "GET", base+"/api/games/"+room+"/selections", nil, &legal); code != 200 || len(legal.Moves) == 0 {
		t.Fatalf("legal: %d", code)
	}
	if code := moveAs(t, alice, base, room, "Alice", legal.Moves[0].Notation); code != 200 {
		t.Fatalf("move: %d", code)
	}

	waitJS(t, ctx, "the actions", `document.querySelector("#impulse-card.acting #piece-buttons button") !== null`, 15*time.Second)
	// The history has the move.
	waitJS(t, ctx, "the log", `document.querySelectorAll("#move-list button.mv").length === 1`, 5*time.Second)
	if !evalBool(t, ctx, `(function () {
		var rail = document.getElementById("play-rail").getBoundingClientRect();
		var card = document.getElementById("impulse-card").getBoundingClientRect();
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
		var card = document.getElementById("impulse-card").getBoundingClientRect();
		return document.getElementById("play-rail").scrollHeight > rail.height && card.top >= rail.top && card.bottom <= rail.bottom;
	})()`, 5*time.Second)
}

// Every kind of game gets this page: a board and a hand, a board that grows, a board with
// cards to choose from, a table of cards, a game of patience and a map.
func TestWebFitScreen_EveryKindOfGameFits(t *testing.T) {
	base := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 150*time.Second)
	defer cancel()
	for _, g := range []struct {
		def    string
		solo   bool
		play   string // the play area
		pinned string // what the player chooses from goes in the actions card
	}{
		{"tak-5x5", false, "#board svg", "#impulse-card .hand button"},
		{"hive-base", false, "#board svg", "#impulse-card .hand button"},
		{"onitama", false, "#board svg", "#impulse-card #cards-panel"},
		{"go-9x9", false, "#board svg", "#impulse-card"},
		{"nyet", false, ".card-table", "#impulse-card"},
		{"spider", true, ".sol-table", "#impulse-card"},
		{"test-council", false, "#board svg", "#impulse-card .action-buttons"},
	} {
		body := map[string]any{"game_def_id": g.def, "player_name": "Alice"}
		if !g.solo {
			body["opponent"] = "random"
		}
		var created struct {
			RoomCode string `json:"room_code"`
		}
		if code := doJSON(t, "POST", base+"/api/games", body, &created); code != 200 {
			t.Fatalf("%s: create: %d", g.def, code)
		}
		for _, size := range [][2]int64{{1920, 1080}, {1366, 768}} {
			name := fmt.Sprintf("%s at %dx%d", g.def, size[0], size[1])
			if err := chromedp.Run(ctx, chromedp.EmulateViewport(size[0], size[1])); err != nil {
				t.Fatal(err)
			}
			openGameAs(t, ctx, base, created.RoomCode, g.def, "Alice")
			waitJS(t, ctx, name+": the choices", `document.querySelector("`+g.pinned+`") !== null`, 20*time.Second)
			if !evalBool(t, ctx, fitsTheWindowJS) {
				t.Errorf("%s: the page does not fit the window", name)
			}
			if evalBool(t, ctx, `document.querySelector(".status-bar") !== null`) {
				t.Errorf("%s: a status bar is drawn", name)
			}
			if !evalBool(t, ctx, `document.getElementById("play-rail").getBoundingClientRect().left >= document.querySelector("`+g.play+`").getBoundingClientRect().right`) {
				t.Errorf("%s: the cards are not beside the play area", name)
			}
			if evalInt(t, ctx, `Math.round(document.querySelector("`+g.play+`").getBoundingClientRect().width)`) < 500 {
				t.Errorf("%s: the play area is too small", name)
			}
		}
	}
}

// A board that grows as tiles are laid keeps its shape, or the columns would move at each turn.
func TestWebFitScreen_ColumnsStayPutAsHiveGrows(t *testing.T) {
	base := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	var created struct {
		RoomCode string `json:"room_code"`
	}
	if code := doJSON(t, "POST", base+"/api/games", map[string]any{"game_def_id": "hive-base", "player_name": "Alice", "opponent": "random"}, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	if err := chromedp.Run(ctx, chromedp.EmulateViewport(1366, 768)); err != nil {
		t.Fatal(err)
	}
	openGameAs(t, ctx, base, created.RoomCode, "hive-base", "Alice")
	left := `Math.round(document.getElementById("play-rail").getBoundingClientRect().left)`
	waitJS(t, ctx, "a tile", `document.querySelector(".hand-piece.choosable") !== null`, 15*time.Second)
	before := evalInt(t, ctx, left)
	evalBool(t, ctx, hiveStepJS+` !== null`)
	waitJS(t, ctx, "the cell", `document.querySelector("#board .cell.clickable") !== null`, 10*time.Second)
	evalBool(t, ctx, hiveStepJS+` !== null`)
	for deadline := time.Now().Add(20 * time.Second); cfGet(t, base, created.RoomCode).MoveSeq < 2 && time.Now().Before(deadline); {
		time.Sleep(200 * time.Millisecond)
	}
	waitJS(t, ctx, "the turn back", `document.querySelector("#impulse-card.acting") !== null`, 10*time.Second)
	if after := evalInt(t, ctx, left); after != before {
		t.Errorf("the cards moved from %d to %d", before, after)
	}
}

// A map has more to show than a column can: the cults, the talk and the history take turns,
// and the history stays a tab however much height there is to spare.
func TestWebFitScreen_TheCardsOfAMapTakeTurns(t *testing.T) {
	base := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	var created struct {
		RoomCode string `json:"room_code"`
	}
	if code := doJSON(t, "POST", base+"/api/games", map[string]any{"game_def_id": "cthulhu-wars", "player_name": "Alice", "opponent": "random"}, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	if err := chromedp.Run(ctx, chromedp.EmulateViewport(1920, 1080)); err != nil {
		t.Fatal(err)
	}
	openGameAs(t, ctx, base, created.RoomCode, "cthulhu-wars", "Alice")
	waitJS(t, ctx, "the tabs", `document.getElementById("rail-tab-cults") !== null`, 20*time.Second)
	historyIsATab := `document.querySelector(".board-wrap > #history-card") === null && document.querySelector("#rail-pane-history > #history-card") !== null && document.getElementById("rail-tab-history") !== null`
	if !evalBool(t, ctx, historyIsATab) {
		t.Error("the history should be a tab of the column")
	}
	// The map starts right under the round: no gap to lose its top to.
	if gap := evalInt(t, ctx, `Math.round(document.querySelector("#board svg").getBoundingClientRect().top - document.querySelector(".round-bar").getBoundingClientRect().bottom)`); gap > 16 {
		t.Errorf("%dpx between the round and the map", gap)
	}
	visible := func(id string) bool {
		return evalBool(t, ctx, `document.getElementById("`+id+`").offsetParent !== null`)
	}
	if !visible("cult-stack") || visible("chat-input") {
		t.Error("the cults should be showing, and the chat not")
	}
	evalBool(t, ctx, `(document.getElementById("chat-input").value = "half a thought", true)`)
	clickSel(t, ctx, "#rail-tab-talk")
	waitJS(t, ctx, "the chat", `document.getElementById("chat-input").offsetParent !== null`, 10*time.Second)
	if visible("cult-stack") {
		t.Error("the cults should be behind their tab")
	}
	// The actions stay in view whichever tab is open.
	if !visible("impulse-card") {
		t.Error("the actions went with the tab")
	}
	clickSel(t, ctx, "#rail-tab-cults")
	waitJS(t, ctx, "the cults", `document.getElementById("cult-stack").offsetParent !== null`, 10*time.Second)

	// With no height to spare it is the same.
	if err := chromedp.Run(ctx, chromedp.EmulateViewport(1200, 640)); err != nil {
		t.Fatal(err)
	}
	openGameAs(t, ctx, base, created.RoomCode, "cthulhu-wars", "Alice")
	waitJS(t, ctx, "the history tab", historyIsATab, 20*time.Second)
	if !evalBool(t, ctx, fitsTheWindowJS) {
		t.Error("the map does not fit the smaller window")
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
	if got := evalString(t, ctx, `document.getElementById("impulse-text").textContent`); got != "Choose a stone to place." {
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
	waitJS(t, ctx, "the stones", `document.querySelector("#impulse-card.acting #piece-buttons button") !== null`, 20*time.Second)
	evalBool(t, ctx, `(document.querySelector('#piece-buttons button').click(), true)`)
	waitJS(t, ctx, "the cells", `document.querySelector("#board .cell.clickable") !== null`, 10*time.Second)
	evalBool(t, ctx, `(document.querySelector("#board .cell.clickable").dispatchEvent(new MouseEvent("click", {bubbles: true})), true)`)
	waitJS(t, ctx, "the second stone", `document.getElementById("impulse-text") && document.getElementById("impulse-text").textContent === "Choose a stone to place, or choose an action."`, 10*time.Second)
	if got := evalString(t, ctx, `document.querySelector('#move-bar button[data-action="done"]').innerText.trim()`); got != "Done" {
		t.Errorf("the end of the turn says %q", got)
	}
}

// The board's column is as wide as the board, and the cards stand next to it in one column.
// Width that neither can use goes to the outer margins, not between them.
func TestWebFitScreen_TheBoardAndTheCardsShareTheWidth(t *testing.T) {
	base := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	rect := func(sel string) string { return `document.querySelector("` + sel + `").getBoundingClientRect()` }
	for _, size := range []struct{ w, h int64 }{{1920, 1080}, {2560, 1080}, {1280, 720}, {1024, 600}} {
		name := fmt.Sprintf("%dx%d", size.w, size.h)
		fittedGame(t, ctx, base, "blooms", size.w, size.h)
		if !evalBool(t, ctx, fitsTheWindowJS) {
			t.Errorf("%s: the page does not fit the window", name)
		}
		if gap := evalInt(t, ctx, `Math.round(`+rect("#play-rail")+`.left - `+rect("#board svg")+`.right)`); gap > 16 {
			t.Errorf("%s: %dpx between the board and the cards", name, gap)
		}
		// The svg is drawn at the board's own shape: nothing is left over inside it.
		if !evalBool(t, ctx, `(function () {
			var svg = document.querySelector("#board svg"), box = svg.getBoundingClientRect(), v = svg.viewBox.baseVal;
			return Math.abs(box.width / box.height - v.width / v.height) < 0.03;
		})()`) {
			t.Errorf("%s: the board does not fill its column: %s", name, evalString(t, ctx, boardPlaceJS))
		}
		if w := evalInt(t, ctx, `Math.round(`+rect("#play-rail")+`.width)`); w <= 380 {
			t.Errorf("%s: the cards are only %dpx wide", name, w)
		}
		if w := evalInt(t, ctx, `Math.round(`+rect("#play-rail")+`.width)`); w > 640 {
			t.Errorf("%s: the cards are %dpx wide", name, w)
		}
		if !evalBool(t, ctx, rect("#chat-card")+`.top >= `+rect("#impulse-card")+`.bottom`) {
			t.Errorf("%s: the cards should stand in one column", name)
		}
		// What is left over is shared by the two sides of the page.
		left, right := evalInt(t, ctx, `Math.round(`+rect("#board svg")+`.left)`), evalInt(t, ctx, `Math.round(innerWidth - `+rect("#play-rail")+`.right)`)
		if left-right > 16 || right-left > 16 {
			t.Errorf("%s: margins of %dpx and %dpx", name, left, right)
		}
	}
	// Upright, the board's row is no taller than the board.
	fittedGame(t, ctx, base, "blooms", 390, 844)
	if gap := evalInt(t, ctx, `Math.round(`+rect("#play-rail")+`.top - `+rect("#board svg")+`.bottom)`); gap > 16 {
		t.Errorf("upright: %dpx between the board and the cards", gap)
	}
}

// No bar on this page, whatever it would have said: the result, history and analysis
// are in the actions card, with their buttons.
func TestWebFitScreen_TheActionCardSaysWhatTheBarSaid(t *testing.T) {
	base := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	fittedGame(t, ctx, base, "blooms", 1280, 720)
	inCard := func(id string) string { return `document.querySelector("#impulse-card #` + id + `") !== null` }
	title := func(want string) {
		t.Helper()
		if got := evalString(t, ctx, `document.querySelector("#impulse-card h3").textContent`); got != want {
			t.Errorf("the card is called %q, not %q", got, want)
		}
		if evalBool(t, ctx, `document.querySelector(".status-bar") !== null`) {
			t.Errorf("%s: a status bar is drawn", want)
		}
	}

	// A move for the history to show, then the end of the game.
	title("Alice")
	clickSel(t, ctx, `#piece-buttons button[data-piece="red"]`)
	waitJS(t, ctx, "the cells", `document.querySelector("#board .cell.clickable") !== null`, 10*time.Second)
	evalBool(t, ctx, `(document.querySelector("#board .cell.clickable").dispatchEvent(new MouseEvent("click", {bubbles: true})), true)`)
	waitJS(t, ctx, "the turn back", `document.querySelector("#impulse-card.acting #piece-buttons button") !== null`, 20*time.Second)
	waitJS(t, ctx, "resign", `document.getElementById("resign") && !document.getElementById("resign").disabled`, 10*time.Second)
	clickSel(t, ctx, "#resign")
	clickSel(t, ctx, "#resign")
	waitJS(t, ctx, "the rematch in the card", inCard("rematch"), 10*time.Second)
	title("Game over")
	if got := evalString(t, ctx, `document.querySelector("#impulse-card #status-message").textContent`); !strings.Contains(got, "won by resignation") {
		t.Errorf("the result says %q", got)
	}

	// Looking back through the game.
	waitJS(t, ctx, "the first move", `document.getElementById("hist-first") && !document.getElementById("hist-first").disabled`, 10*time.Second)
	clickSel(t, ctx, "#hist-first")
	waitJS(t, ctx, "return to live in the card", inCard("return-live"), 10*time.Second)
	title("History: 0")
	if got := evalString(t, ctx, `document.getElementById("impulse-card").innerText`); strings.Contains(got, "Browsing history") {
		t.Errorf("the card says it twice: %q", got)
	}

	// Analysis from there.
	clickSel(t, ctx, "#analyze-here")
	waitJS(t, ctx, "exit in the card", inCard("exit-analysis"), 10*time.Second)
	title("Analysis")
	clickSel(t, ctx, "#exit-analysis")
	waitJS(t, ctx, "out of analysis", `document.getElementById("exit-analysis") === null`, 10*time.Second)
	if evalBool(t, ctx, `document.querySelector(".status-bar") !== null`) {
		t.Error("a status bar is drawn after analysis")
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
	if evalBool(t, ctx, `!document.getElementById("tutorial-back").disabled`) {
		t.Error("the first step has nothing to go back to")
	}
	for _, n := range []string{"2", "3", "4"} {
		clickSel(t, ctx, "#tutorial-next")
		waitStep(t, ctx, n)
	}
	// A step that asks a question keeps its answer behind the hint, with Next still there.
	if evalBool(t, ctx, `document.getElementById("tutorial-hint") !== null`) {
		t.Error("the answer should wait to be asked for")
	}
	clickSel(t, ctx, "#tutorial-show-hint")
	waitJS(t, ctx, "the answer", `(document.getElementById("tutorial-hint") || {}).textContent === "There are 3 red blooms and 2 yellow blooms."`, 10*time.Second)
	for _, n := range []string{"5", "6", "7", "8"} {
		clickSel(t, ctx, "#tutorial-next")
		waitStep(t, ctx, n)
	}
	// A step to read, and a picture.
	clickSel(t, ctx, "#tutorial-back")
	waitStep(t, ctx, "7")
	waitJS(t, ctx, "its picture", stones+` === 5`, 10*time.Second)
	clickSel(t, ctx, "#tutorial-next")
	waitStep(t, ctx, "8")
	waitJS(t, ctx, "the room's board", stones+` === 0`, 10*time.Second)

	// Play on to step 9, where moves have been made.
	waitJS(t, ctx, "red picked out", `!!document.querySelector('button[data-piece="red"].tutorial-target')`, 10*time.Second)
	clickSel(t, ctx, `button[data-piece="red"]`)
	waitJS(t, ctx, "d4", `[...document.querySelectorAll("#board .cell.clickable")].map(c => c.dataset.position).join(",") === "d4"`, 10*time.Second)
	clickCell(t, ctx, "d4")
	waitStep(t, ctx, "9")
	waitJS(t, ctx, "the replies", stones+` === 3`, 10*time.Second)
	played := cfGet(t, base, room).MoveSeq

	// Back to the step before them: the board without the moves, and nothing to click.
	clickSel(t, ctx, "#tutorial-back")
	waitStep(t, ctx, "8")
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
	waitStep(t, ctx, "9")
	waitJS(t, ctx, "the board as it is", stones+` === 3`, 10*time.Second)
	waitJS(t, ctx, "red picked out", `!!document.querySelector('button[data-piece="red"].tutorial-target')`, 10*time.Second)
	clickSel(t, ctx, `button[data-piece="red"]`)
	waitJS(t, ctx, "b3", `[...document.querySelectorAll("#board .cell.clickable")].map(c => c.dataset.position).join(",") === "b3"`, 10*time.Second)
}
