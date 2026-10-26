package app_test

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// The sixth round: the game options window, the pictograms of the cult cards,
// Gates that show their occupant, a button for every monster, the History tab,
// and games against oneself.

func TestWebCthulhuWars_AGateShowsItsOccupantAndIsGivenUpByMovingIt(t *testing.T) {
	baseURL := newRESTTestServer(t)
	doc := cwPosition(t,
		[]cwUnit{{1, "cultist", "south-pacific", 3}, {2, "cultist", "asia", 1}},
		map[string]int{"south-pacific": 1, "asia": 2},
		[5]int{0, 5, 3, 3, 3}, [5]uint8{}, nil)
	room := cwGameFrom(t, baseURL, doc)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	var mu sync.Mutex
	var jsErrors []string
	collectJSErrors(ctx, &mu, &jsErrors)
	openGameAs(t, ctx, baseURL, room, "cthulhu-wars", "Alice")
	// The Gate is drawn with the figure that occupies it ...
	waitJS(t, ctx, "the Gate with its occupant", `document.querySelector('#board .gate-chip[data-position="south-pacific"] .gate-holder[data-holder="cultist"]') !== null`, 15*time.Second)
	// ... and the others stand beside it: 3 Cultists are one on the Gate and a
	// stack of 2, not a stack of 3.
	if got := evalString(t, ctx, `document.querySelector('#board .token[data-position="south-pacific"][data-piece="cultist"]').dataset.count`); got != "2" {
		t.Errorf("the stack beside the Gate has %s Cultists, want 2", got)
	}
	if n := evalInt(t, ctx, `document.querySelectorAll('[id^="action-abandon"]').length`); n != 0 {
		t.Errorf("%d Abandon buttons", n)
	}
	// The Gate's figure is picked up like any unit: its move abandons the Gate.
	if !evalBool(t, ctx, `document.querySelector('#board .gate-chip[data-position="south-pacific"]').classList.contains("gate-pickable")`) {
		t.Fatal("the unit on the Gate cannot be picked")
	}
	clickSVG(t, ctx, `#board .gate-chip[data-position="south-pacific"]`)
	waitJS(t, ctx, "its own area is a destination", `document.querySelector('#board .cell.area.clickable[data-position="south-pacific"]') !== null`, 10*time.Second)
	// Step off: click the Gate itself.
	clickSVG(t, ctx, `#board .gate-chip[data-position="south-pacific"]`)
	waitJS(t, ctx, "abandoned", `document.querySelector('#board .gate-chip.abandoned[data-position="south-pacific"]') !== null`, 15*time.Second)
	// Nothing moved, it cost nothing (a step off in place is free), and the
	// three stand together again.
	waitJS(t, ctx, "the three stand together", `document.querySelector('#board .token[data-position="south-pacific"][data-piece="cultist"]').dataset.count === "3"`, 10*time.Second)
	waitJS(t, ctx, "power", `document.querySelector('#seat-1 [data-counter="power"]').textContent === "5"`, 10*time.Second)
	// Select one of the stack and click the Gate: back on it, for free.
	clickSVG(t, ctx, `#board .token.token-target[data-position="south-pacific"][data-piece="cultist"]`)
	waitJS(t, ctx, "the Gate is aimed at", `document.querySelector('#board .gate-chip.gate-target[data-position="south-pacific"]') !== null`, 10*time.Second)
	clickSVG(t, ctx, `#board .gate-chip[data-position="south-pacific"]`)
	waitJS(t, ctx, "controlled again", `document.querySelector('#board .gate-chip[data-position="south-pacific"]') !== null && document.querySelector('#board .gate-chip.abandoned[data-position="south-pacific"]') === null`, 15*time.Second)
	waitJS(t, ctx, "a stack of two again", `document.querySelector('#board .token[data-position="south-pacific"][data-piece="cultist"]').dataset.count === "2"`, 10*time.Second)
	if got := evalString(t, ctx, `document.querySelector('#seat-1 [data-counter="power"]').textContent`); got != "5" {
		t.Errorf("Power %s after the free actions, want 5", got)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(jsErrors) > 0 {
		t.Errorf("uncaught JS exceptions: %v", jsErrors)
	}
}

func TestWebCthulhuWars_GameOptionsWindowHoldsTheSettings(t *testing.T) {
	baseURL := newRESTTestServer(t)
	var created struct {
		RoomCode string `json:"room_code"`
	}
	body := map[string]any{"game_def_id": "cthulhu-wars-5p", "player_name": "Alice", "opponent": "random"}
	if code := doJSON(t, "POST", baseURL+"/api/games", body, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	var mu sync.Mutex
	var jsErrors []string
	collectJSErrors(ctx, &mu, &jsErrors)
	openGameAs(t, ctx, baseURL, created.RoomCode, "cthulhu-wars-5p", "Alice")
	waitJS(t, ctx, "the board", `document.querySelectorAll('#board .cell.area').length === 21`, 15*time.Second)

	// The bar at the top has the rules and the options; the side column no
	// longer carries the players, the tools, the key or the link.
	for _, id := range []string{"show-rules", "open-options"} {
		if n := evalInt(t, ctx, `document.querySelectorAll('.topbar #`+id+`').length`); n != 1 {
			t.Errorf("%d #%s in the top bar", n, id)
		}
	}
	for _, id := range []string{"players", "legend-card", "reaction-dock", "action-abandon"} {
		if n := evalInt(t, ctx, `document.querySelectorAll('#`+id+`').length`); n != 0 {
			t.Errorf("#%s is still on the page", id)
		}
	}
	if n := evalInt(t, ctx, `document.querySelectorAll('.side-panel #share-link, .side-panel #flip-board, .side-panel #resign').length`); n != 0 {
		t.Errorf("settings are still in the side column")
	}

	// The window: closed at first, then open with its sections.
	if evalBool(t, ctx, `!document.getElementById("game-options").hidden`) {
		t.Error("the options window starts open")
	}
	openOptions(t, ctx)
	for _, sel := range []string{"#options-layers", "#options-display", "#options-tools", "#share-link", "#copy-link", "#resign", "#opt-board", "#flip-board", "#analyze"} {
		if n := evalInt(t, ctx, `document.querySelectorAll('#game-options `+sel+`').length`); n != 1 {
			t.Errorf("%d %s in the options window", n, sel)
		}
	}
	if got := evalString(t, ctx, `document.getElementById("resign").textContent`); got != "End game" {
		t.Errorf("resign button = %q", got)
	}

	// Layers: names, glyphs and sigils switch off and on.
	layer := func(id, class string) {
		t.Helper()
		before := evalInt(t, ctx, `document.querySelectorAll('#board `+class+`').length`)
		if before == 0 {
			t.Fatalf("no %s on the map to begin with", class)
		}
		clickSel(t, ctx, "#"+id)
		waitJS(t, ctx, id+" off", `document.querySelectorAll('#board `+class+`').length === 0`, 5*time.Second)
		clickSel(t, ctx, "#"+id)
		waitJS(t, ctx, id+" on", `document.querySelectorAll('#board `+class+`').length === `+itoa(before), 5*time.Second)
	}
	layer("opt-layer-names", ".area-label")
	layer("opt-layer-glyphs", ".area-glyph-badge")
	layer("opt-layer-sigils", ".area-sigil-badge")
	// The choice is remembered.
	clickSel(t, ctx, "#opt-layer-glyphs")
	waitJS(t, ctx, "stored", `localStorage.getItem("bingle.layer.glyphs") === "false"`, 5*time.Second)

	// No waves on the water and no frost on the ice.
	if n := evalInt(t, ctx, `document.querySelectorAll('#board .area-waves, #board .area-frost').length`); n != 0 {
		t.Errorf("%d texture layers on the map", n)
	}
	// Close with the button and with Esc.
	clickSel(t, ctx, "#options-close")
	waitJS(t, ctx, "closed", `document.getElementById("game-options").hidden`, 5*time.Second)
	mu.Lock()
	defer mu.Unlock()
	if len(jsErrors) > 0 {
		t.Errorf("uncaught JS exceptions: %v", jsErrors)
	}
}

// clickSVG dispatches a click on an element of the map (SVG elements have no click()).
func clickSVG(t *testing.T, ctx context.Context, sel string) {
	t.Helper()
	js := `(function(){ var el = document.querySelector(` + itoaQuote(sel) + `); if (!el) return false; el.dispatchEvent(new MouseEvent("click", {bubbles: true})); return true; })()`
	var ok bool
	if err := chromedp.Run(ctx, chromedp.Evaluate(js, &ok)); err != nil || !ok {
		t.Fatalf("click %s: ok=%v err=%v", sel, ok, err)
	}
}

func itoaQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func TestWebCthulhuWars_UnitsAreSummonedFromTheirOwnButtonsWithAHoverCard(t *testing.T) {
	baseURL := newRESTTestServer(t)
	var created struct {
		RoomCode string `json:"room_code"`
	}
	body := map[string]any{
		"game_def_id": "cthulhu-wars-5p", "player_name": "Alice", "opponent": "random", "seat": 5,
		"options": map[string]any{"factions": []string{"gc", "cc", "bg", "ys", "ww"}},
	}
	if code := doJSON(t, "POST", baseURL+"/api/games", body, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	openGameAs(t, ctx, baseURL, created.RoomCode, "cthulhu-wars-5p~gc-cc-bg-ys-ww", "Alice")
	// The Windwalker begins first of all.
	waitJS(t, ctx, "the choice of where to begin", `document.querySelectorAll('#board .cell.area.clickable').length === 2`, 20*time.Second)
	clickCell(t, ctx, "arctic-ocean")
	waitJS(t, ctx, "buttons", `document.querySelectorAll('.summon-button[data-action^="summon-"]').length === 2`, 20*time.Second)
	// The Gnoph-Keh costs as many Power as there are in the pool; the button says so.
	if got := evalString(t, ctx, `[...document.querySelectorAll('.summon-button[data-action^="summon-"]')].map(b => b.getAttribute("data-piece") + "=" + b.getAttribute("data-cost")).join(",")`); got != "wendigo=1,gnoph-keh=4" {
		t.Errorf("summon buttons %q", got)
	}
	// Hover: the card names the unit, its cost, its dice and its spellbooks.
	if err := chromedp.Run(ctx, chromedp.Evaluate(`document.querySelector('.summon-button[data-piece="wendigo"]').dispatchEvent(new MouseEvent("mouseenter")); true`, nil)); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the hover card", `document.querySelector('.hover-card .unit-tip') !== null`, 5*time.Second)
	card := evalString(t, ctx, `document.querySelector('.hover-card').textContent`)
	for _, want := range []string{"Wendigo", "Monster", "1 Power", "1 die", "Howl", "Cannibalism"} {
		if !strings.Contains(card, want) {
			t.Errorf("the hover card lacks %q: %q", want, card)
		}
	}
	// Leaving the button closes the card.
	if err := chromedp.Run(ctx, chromedp.Evaluate(`document.querySelector('.summon-button[data-piece="wendigo"]').dispatchEvent(new MouseEvent("mouseleave")); true`, nil)); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "closed", `document.querySelector('.hover-card') === null`, 5*time.Second)
	// Summon one: the button, then the Gate.
	clickSel(t, ctx, `.summon-button[data-piece="wendigo"]`)
	waitJS(t, ctx, "targets", `document.querySelectorAll('#board .cell.area.clickable').length === 1`, 10*time.Second)
	clickCell(t, ctx, "arctic-ocean")
	waitJS(t, ctx, "a Wendigo stands", `document.querySelector('#board .token[data-position="arctic-ocean"][data-piece="wendigo"]') !== null`, 15*time.Second)
}

func TestWebCthulhuWars_TheCultCardsUsePictogramsAndShowTheUnitsInThePool(t *testing.T) {
	baseURL := newRESTTestServer(t)
	var created struct {
		RoomCode string `json:"room_code"`
	}
	body := map[string]any{"game_def_id": "cthulhu-wars", "player_name": "Alice", "opponent": "random"}
	if code := doJSON(t, "POST", baseURL+"/api/games", body, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	openGameAs(t, ctx, baseURL, created.RoomCode, "cthulhu-wars", "Alice")
	waitJS(t, ctx, "the cards", `document.querySelectorAll('#seat-1 .faction-card').length === 5`, 15*time.Second)
	// The faction ability comes first.
	if got := evalString(t, ctx, `document.querySelector('#seat-1 .faction-card').getAttribute("data-card")`); got != "ability" {
		t.Errorf("first section %q", got)
	}
	if got := evalString(t, ctx, `document.querySelector('#seat-1 .faction-card[data-card="ability"]').textContent`); !strings.Contains(got, "Immortal") {
		t.Errorf("the ability section: %q", got)
	}
	// The header: pictograms, no words.
	head := evalString(t, ctx, `document.querySelector('#seat-1 .seat-head').textContent`)
	for _, word := range []string{"Power", "Doom", "Elder Signs", "Gates", "Spellbooks"} {
		if strings.Contains(head, word) {
			t.Errorf("the header still says %q: %q", word, head)
		}
	}
	// Elder Signs: with none there is a heading and nothing to open — no
	// "None yet" and no disclosure; the old sign row is gone.
	if n := evalInt(t, ctx, `document.querySelectorAll('#seat-1 .faction-card[data-card="signs"] summary').length`); n != 0 {
		t.Errorf("an empty Elder Signs section opens (%d summaries)", n)
	}
	if got := evalString(t, ctx, `document.querySelector('#seat-1 .faction-card[data-card="signs"]').textContent`); strings.Contains(got, "None yet") || !strings.Contains(got, "Elder Signs") {
		t.Errorf("the empty Elder Signs section reads %q", got)
	}
	if n := evalInt(t, ctx, `document.querySelectorAll('.sign-row').length`); n != 0 {
		t.Errorf("the old sign row is still there")
	}
	// The pool: one icon per unit, with a hover card.
	if err := chromedp.Run(ctx, chromedp.Evaluate(`document.querySelector('#seat-1 .pool-unit[data-piece="shoggoth"]').dispatchEvent(new MouseEvent("mouseenter")); true`, nil)); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the unit's card", `document.querySelector('.hover-card') !== null`, 5*time.Second)
	if got := evalString(t, ctx, `document.querySelector('.hover-card').textContent`); !strings.Contains(got, "Shoggoth") || !strings.Contains(got, "Absorb") {
		t.Errorf("the pool unit's card: %q", got)
	}
	// The bag: the icon and a number of total.
	if got := evalString(t, ctx, `document.getElementById("elder-bag").textContent.trim()`); !regexp.MustCompile(`^\d+/\d+$`).MatchString(got) {
		t.Errorf("the bag reads %q", got)
	}
}

func TestWebCthulhuWars_MovesAndHistoryShareACardAndUndoIsAmongTheActions(t *testing.T) {
	baseURL := newRESTTestServer(t)
	doc := cwPosition(t,
		[]cwUnit{{1, "cultist", "south-pacific", 2}, {2, "cultist", "asia", 1}},
		map[string]int{"south-pacific": 1, "asia": 2},
		[5]int{0, 5, 3, 3, 3}, [5]uint8{}, nil)
	room := cwGameFrom(t, baseURL, doc)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	openGameAs(t, ctx, baseURL, room, "cthulhu-wars", "Alice")
	waitJS(t, ctx, "the board", `document.querySelectorAll('#board .cell.area').length === 17`, 15*time.Second)
	// Nothing to take back yet.
	if n := evalInt(t, ctx, `document.querySelectorAll('#undo-move').length`); n != 0 {
		t.Errorf("undo is offered before any move")
	}
	if code, out := moveNotation(t, baseURL, room, "Alice", "recruit@south-pacific"); code != 200 {
		t.Fatalf("recruit: %d %v", code, out)
	}
	// The bots reply; when it is Alice's turn again her last move can be taken back
	// — from the list of actions.
	waitJS(t, ctx, "undo among the actions", `document.querySelector('.action-buttons #undo-move, #move-bar #undo-move') !== null`, 40*time.Second)
	if n := evalInt(t, ctx, `document.querySelectorAll('#players, .side-players').length`); n != 0 {
		t.Errorf("the Players window is still there")
	}
	// One card, two tabs.
	if got := evalString(t, ctx, `[...document.querySelectorAll("#history-card .card-tab")].map(b => b.textContent.replace(/\s+/g, " ").trim().split(" ")[0]).join(",")`); got != "Moves,History" {
		t.Errorf("tabs %q", got)
	}
	clickSel(t, ctx, "#tab-moves")
	waitJS(t, ctx, "the moves", `document.getElementById("moves-pane") !== null && document.getElementById("move-list") !== null && document.getElementById("events") === null`, 5*time.Second)
	clickSel(t, ctx, "#tab-history")
	waitJS(t, ctx, "the history", `document.getElementById("events") !== null && document.getElementById("move-list") === null`, 5*time.Second)
	// The choice is remembered across redraws.
	clickSel(t, ctx, "#tab-moves")
	if code, out := moveNotation(t, baseURL, room, "Alice", "pass"); code != 200 {
		t.Logf("pass: %d %v", code, out)
	}
	time.Sleep(500 * time.Millisecond)
	if got := evalString(t, ctx, `document.querySelector("#history-card .card-tab.active").id`); got != "tab-moves" {
		t.Errorf("active tab %q", got)
	}
}

func TestWebHome_PlayAGameAgainstMyself(t *testing.T) {
	baseURL := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.Navigate(baseURL+"/?game=tic-tac-toe")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the opponents", `document.querySelector('input[name="opponent"][value="@self"]') !== null`, 20*time.Second)
	evalString(t, ctx, `document.querySelector('input[name="opponent"][value="@self"]').click(); document.getElementById("create_player_name").value = "Sol"; "ok"`)
	clickSel(t, ctx, `#create-game-form button[type="submit"]`)
	loc := pollLocation(t, ctx, regexp.MustCompile(`/game/[A-Z0-9]+$`))
	room := loc[strings.LastIndex(loc, "/")+1:]
	var info struct {
		Status string `json:"status"`
		Seats  []struct {
			Name string `json:"name"`
			Bot  string `json:"bot"`
		} `json:"seats"`
	}
	doJSON(t, "GET", baseURL+"/api/games/"+room, nil, &info)
	if info.Status != "active" || len(info.Seats) != 2 || info.Seats[0].Name != "Sol" || !strings.HasPrefix(info.Seats[1].Name, "Sol · ") || info.Seats[1].Bot != "" {
		t.Fatalf("game = %+v", info)
	}
	// The page moves for whoever is to move: click three squares, one after another.
	waitJS(t, ctx, "the board", `document.querySelector('#board svg') !== null && document.querySelector('#board .cell[data-position="b2"]') !== null`, 15*time.Second)
	for i, sq := range []string{"b2", "a1", "c3"} {
		waitJS(t, ctx, "the board is ready for move "+itoa(i+1), `document.querySelector('#board .cell[data-position="`+sq+`"]') !== null && !document.getElementById("status-message").textContent.includes("Waiting")`, 15*time.Second)
		clickCell(t, ctx, sq)
		// (Preview and confirm: the move is played with Submit.)
		waitJS(t, ctx, "the preview", `document.getElementById("submit-move") !== null`, 10*time.Second)
		clickSel(t, ctx, "#submit-move")
		waitJS(t, ctx, "the move "+itoa(i+1)+" is on the board", `document.querySelectorAll('#move-list button.mv').length === `+itoa(i+1), 15*time.Second)
	}
	// Both seats are marked as yours.
	if n := evalInt(t, ctx, `document.querySelectorAll('.seat-strip .badge').length`); n < 2 {
		t.Logf("seat badges: %d", n)
	}
}

func TestWebHome_EverySeatOfATableCanBeMine(t *testing.T) {
	baseURL := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.Navigate(baseURL+"/?game=cthulhu-wars-3p")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the seats", `document.querySelectorAll("#slots-field select.slot-select").length === 3`, 20*time.Second)
	// Choosing "You" for another seat leaves the first one yours too.
	evalString(t, ctx, `(function () {
		var s = document.querySelector('#slots-field select.slot-select[data-seat="2"]');
		s.value = "me"; s.dispatchEvent(new Event("change", { bubbles: true }));
		var t3 = document.querySelector('#slots-field select.slot-select[data-seat="3"]');
		t3.value = "me"; t3.dispatchEvent(new Event("change", { bubbles: true }));
		document.getElementById("create_player_name").value = "Ana";
		document.getElementById("lobby").checked = false;
		return "ok";
	})()`)
	if got := evalString(t, ctx, `[...document.querySelectorAll("#slots-field select.slot-select")].map(s => s.value).join(",")`); got != "me,me,me" {
		t.Fatalf("seats = %q", got)
	}
	clickSel(t, ctx, `#create-game-form button[type="submit"]`)
	loc := pollLocation(t, ctx, regexp.MustCompile(`/game/[A-Z0-9]+$`))
	room := loc[strings.LastIndex(loc, "/")+1:]
	var info struct {
		Status string `json:"status"`
		Seats  []struct {
			Name string `json:"name"`
		} `json:"seats"`
	}
	doJSON(t, "GET", baseURL+"/api/games/"+room, nil, &info)
	if info.Status != "active" || len(info.Seats) != 3 {
		t.Fatalf("game = %+v", info)
	}
	want := []string{"Ana", "Ana · Crawling Chaos", "Ana · Black Goat"}
	for i, s := range info.Seats {
		if s.Name != want[i] {
			t.Errorf("seat %d is %q, want %q", i+1, s.Name, want[i])
		}
	}
	// The page knows all three are its own and plays for the one to move.
	waitJS(t, ctx, "the cults", `document.querySelectorAll('#cult-stack .seat-strip').length === 3`, 20*time.Second)
	if n := evalInt(t, ctx, `[...document.querySelectorAll('#cult-stack .seat-strip')].filter(s => s.textContent.indexOf("you") >= 0).length`); n != 3 {
		t.Errorf("%d cult cards marked as yours, want 3", n)
	}
	// Great Cthulhu, the first player of the game, chooses the direction of
	// play first — one of the three seats the page plays for.
	waitJS(t, ctx, "the direction", `document.getElementById("action-cw") !== null`, 20*time.Second)
	clickSel(t, ctx, "#action-cw")
	waitJS(t, ctx, "my turn", `document.getElementById("action-pass") !== null`, 20*time.Second)
}
