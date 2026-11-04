package app_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"

	"github.com/noderunner/bingle/internal/engine/catalog"
)

// The themed sets of cards: chosen when a game is set up, the same for everyone
// at the table and fixed for the whole game (ADR 0085).

func TestCardSets_TheServerKeepsTheChoiceAndRefusesWhatCannotWearOne(t *testing.T) {
	baseURL := newRESTTestServer(t)
	create := func(def, set string) (int, string) {
		var out struct {
			RoomCode string `json:"room_code"`
		}
		code := doJSON(t, "POST", baseURL+"/api/games", map[string]any{"game_def_id": def, "player_name": "Ana", "card_set": set}, &out)
		return code, out.RoomCode
	}
	code, room := create("nyet", "amphibians")
	if code != 200 {
		t.Fatalf("create with a set: %d", code)
	}
	var game struct {
		CardSet string `json:"card_set"`
	}
	if code := doJSON(t, "GET", baseURL+"/api/games/"+room, nil, &game); code != 200 || game.CardSet != "amphibians" {
		t.Errorf("GET the game: %d, card_set %q", code, game.CardSet)
	}
	if code, _ := create("nyet", "no-such-set"); code != 400 {
		t.Errorf("an unknown set: %d, want 400", code)
	}
	if code, _ := create("tak-5x5", "amphibians"); code != 400 {
		t.Errorf("a game without cards: %d, want 400", code)
	}
	code, room = create("nyet", "")
	if code != 200 {
		t.Fatalf("create without a set: %d", code)
	}
	var plain map[string]any
	doJSON(t, "GET", baseURL+"/api/games/"+room, nil, &plain)
	if v, ok := plain["card_set"]; ok {
		t.Errorf("a game without a set says %v", v)
	}
}

// The setup page offers the sets for a game with a standard deck, and for no other.
func TestWebCardSets_ChosenWhenSettingUpTheGame(t *testing.T) {
	baseURL := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.Navigate(baseURL+"/"), chromedp.WaitVisible(`.game-card[data-family="nyet"]`, chromedp.ByQuery)); err != nil {
		t.Fatal(err)
	}
	clickSel(t, ctx, `.game-card[data-family="gomoku"]`)
	waitJS(t, ctx, "the setup of a game without cards", `document.querySelector('#setup-preview svg') !== null`, 10*time.Second)
	if !evalBool(t, ctx, `document.getElementById("card-set-field").hidden`) {
		t.Error("a game without cards offers a set of cards")
	}
	if err := chromedp.Run(ctx, chromedp.Navigate(baseURL+"/"), chromedp.WaitVisible(`.game-card[data-family="nyet"]`, chromedp.ByQuery)); err != nil {
		t.Fatal(err)
	}
	clickSel(t, ctx, `.game-card[data-family="nyet"]`)
	waitJS(t, ctx, "the field", `!document.getElementById("card-set-field").hidden`, 10*time.Second)
	if got := evalString(t, ctx, `[...document.querySelectorAll('#card-set option')].map(o => o.textContent).join("|")`); got != "The game's own cards|Ancient Civilizations|Amphibians|Fantasy Creatures|Lovecraftian|Atomic Age" {
		t.Errorf("the choices are %q", got)
	}
	// The picture of a set: its back, its eight suits (those beyond the game's four dimmed) and its four jokers.
	evalString(t, ctx, `(function(){ var s = document.getElementById("card-set"); s.value = "lovecraftian"; s.dispatchEvent(new Event("change", {bubbles: true})); return "ok"; })()`)
	waitJS(t, ctx, "the picture", `document.querySelector('#card-set-preview svg.ts-preview') !== null`, 5*time.Second)
	if n := evalInt(t, ctx, `document.querySelectorAll('#card-set-preview .ts-chip').length`); n != 12 {
		t.Errorf("the picture has %d icons, want 12", n)
	}
	if n := evalInt(t, ctx, `document.querySelectorAll('#card-set-preview .ts-chip.ts-unused').length`); n != 4 {
		t.Errorf("%d suits are dimmed, want the 4 a game of 4 suits does not reach", n)
	}
	// Starting the game carries the choice to the table.
	if err := chromedp.Run(ctx, chromedp.Evaluate(`localStorage.setItem("bingle.name", JSON.stringify("Ana"))`, nil)); err != nil {
		t.Fatal(err)
	}
	clickSel(t, ctx, `#create-game-form button[type="submit"]`)
	waitJS(t, ctx, "the game page", `location.pathname.startsWith("/game/")`, 15*time.Second)
	var room string
	if err := chromedp.Run(ctx, chromedp.Evaluate(`location.pathname.split("/").pop()`, &room)); err != nil {
		t.Fatal(err)
	}
	var game struct {
		CardSet string `json:"card_set"`
	}
	if doJSON(t, "GET", baseURL+"/api/games/"+room, nil, &game); game.CardSet != "lovecraftian" {
		t.Errorf("the game was created with the set %q", game.CardSet)
	}
}

// Every player sees the set the game was set up with, and nothing at the table can change it.
func TestWebCardSets_EveryoneAtTheTableSeesTheSameSet(t *testing.T) {
	baseURL := newRESTTestServer(t)
	var created struct {
		RoomCode string `json:"room_code"`
	}
	if code := doJSON(t, "POST", baseURL+"/api/games", map[string]any{"game_def_id": "nyet", "player_name": "Ana", "also_seats": []int{3, 4}, "card_set": "amphibians"}, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	room := created.RoomCode
	if code := doJSON(t, "POST", baseURL+"/api/games/"+room+"/join", map[string]any{"player_name": "Bo", "seat": 2}, nil); code != 200 {
		t.Fatalf("join: %d", code)
	}
	look := func(ctx context.Context, name string) {
		t.Helper()
		openGameAs(t, ctx, baseURL, room, "nyet", name)
		waitJS(t, ctx, name+": the hand", `document.querySelectorAll('#card-table .ct-seat.me .zone-hand .pcard.face').length >= 10`, 15*time.Second)
		if n := evalInt(t, ctx, `document.querySelectorAll('#card-table .pcard.face:not([data-set="amphibians"])').length`); n != 0 {
			t.Errorf("%s: %d faces are not of the set", name, n)
		}
		if n := evalInt(t, ctx, `document.querySelectorAll('#card-table svg.pc-svg').length`); n != 0 {
			t.Errorf("%s: %d classic cards at a themed table", name, n)
		}
		// A card says what it is in the set, and what the rules call it.
		title := evalString(t, ctx, `document.querySelector('#card-table .zone-hand .pcard.face').title`)
		if !strings.Contains(title, " of ") || !strings.Contains(title, "(") {
			t.Errorf("%s: a card is called %q", name, title)
		}
		// The board of terms speaks of the same suits.
		trump := evalString(t, ctx, `document.querySelector('.cg-box[data-line="trump"]').title`)
		if !strings.Contains(trump, "(") {
			t.Errorf("%s: a trump box says %q", name, trump)
		}
		if n := evalInt(t, ctx, `document.querySelectorAll('.cg-box[data-line="trump"] svg.ts-chip').length`); n != 4 {
			t.Errorf("%s: %d trump boxes are drawn in the set, want 4", name, n)
		}
		// There is nothing to choose at the table.
		if evalBool(t, ctx, `document.getElementById("card-set") !== null`) {
			t.Errorf("%s: the table offers a choice of set", name)
		}
	}
	ana, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	look(ana, "Ana")
	bo, cancelBo := newChromedpContext(t, 90*time.Second)
	defer cancelBo()
	look(bo, "Bo")
	// A move (which arrives over the live connection) leaves the set in place.
	before := evalInt(t, bo, `document.querySelectorAll('.cg-box.crossed').length`)
	clickSel(t, ana, `.cg-box.clickable`)
	waitJS(t, bo, "the moves arrive", `document.querySelectorAll('.cg-box.crossed').length > `+itoa(before), 15*time.Second)
	if n := evalInt(t, bo, `document.querySelectorAll('#card-table .pcard.face').length - document.querySelectorAll('#card-table .pcard.face[data-set="amphibians"]').length`); n != 0 {
		t.Errorf("%d faces lost the set after a move", n)
	}
}

// A rematch is played with the same cards.
func TestWebCardSets_ARematchKeepsTheSet(t *testing.T) {
	baseURL := newRESTTestServer(t)
	var created struct {
		RoomCode string `json:"room_code"`
	}
	if code := doJSON(t, "POST", baseURL+"/api/games", map[string]any{"game_def_id": "nyet-2", "player_name": "Ana", "opponent": "random", "card_set": "fantasy"}, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	if code := doJSON(t, "POST", baseURL+"/api/games/"+created.RoomCode+"/resign", map[string]any{"player_name": "Ana"}, nil); code != 200 {
		t.Fatalf("resign: %d", code)
	}
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	openGameAs(t, ctx, baseURL, created.RoomCode, "nyet-2", "Ana")
	waitJS(t, ctx, "the rematch button", `document.getElementById("rematch") !== null`, 15*time.Second)
	clickSel(t, ctx, `#rematch`)
	waitJS(t, ctx, "the new game", `location.pathname.startsWith("/game/") && !location.pathname.endsWith("`+created.RoomCode+`")`, 15*time.Second)
	room := evalString(t, ctx, `location.pathname.split("/").pop()`)
	var game struct {
		CardSet string `json:"card_set"`
	}
	if doJSON(t, "GET", baseURL+"/api/games/"+room, nil, &game); game.CardSet != "fantasy" {
		t.Errorf("the rematch has the set %q", game.CardSet)
	}
}

// A game set up without a set keeps its own cards.
func TestWebCardSets_AGameWithoutOneKeepsItsOwnCards(t *testing.T) {
	baseURL := newRESTTestServer(t)
	room := nyetTable(t, baseURL, "nyet", 4)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	openGameAs(t, ctx, baseURL, room, "nyet", "Ana")
	waitJS(t, ctx, "the hand", `document.querySelectorAll('#card-table .zone-hand svg.pc-svg').length >= 5`, 15*time.Second)
	if n := evalInt(t, ctx, `document.querySelectorAll('#card-table .pcard.themed').length`); n != 0 {
		t.Errorf("%d themed cards in a game without a set", n)
	}
}

// What every set must have, checked on the sets themselves in a page.
func TestWebCardSets_EverySetIsCompleteAndLegible(t *testing.T) {
	baseURL := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.Navigate(baseURL+"/"), chromedp.WaitVisible(`.game-card[data-family="nyet"]`, chromedp.ByQuery)); err != nil {
		t.Fatal(err)
	}
	const script = `(async () => {
		const { CARD_SETS, setFace, setPreview, backSvg, useCardSet, currentCardSet } = await import("/static/js/card-sets.js");
		const { canLetter } = await import("/static/js/card-lettering.js");
		const lum = (hex) => {
			const n = parseInt(hex.slice(1), 16);
			const f = (v) => { v /= 255; return v <= 0.03928 ? v / 12.92 : Math.pow((v + 0.055) / 1.055, 2.4); };
			return 0.2126 * f((n >> 16) & 255) + 0.7152 * f((n >> 8) & 255) + 0.0722 * f(n & 255);
		};
		const contrast = (a, b) => { const [x, y] = [lum(a), lum(b)].sort((p, q) => q - p); return (x + 0.05) / (y + 0.05); };
		const problems = [];
		const ids = ["s1","s2","s3","s4","s5","s6"];
		const jokers = ["joker-a","joker-b"].map((id) => ({ id, label: "Joker", tags: ["joker"] }));
		const info = { cards: { standard: true, suit_order: ids, suits: Object.fromEntries(ids.map((i) => [i, { label: i }])), types: jokers } };
		const seen = new Set();
		for (const set of CARD_SETS) {
			const say = (m) => problems.push(set.id + ": " + m);
			if (set.suits.length !== 8) say(set.suits.length + " suits");
			if (set.jokers.length !== 4) say(set.jokers.length + " jokers");
			if (new Set(set.suits.map((x) => x.color)).size !== 8) say("two suits share a colour");
			for (const [i, x] of [...set.suits, ...set.jokers].entries()) {
				if (!x.label) say("a nameless icon at " + i);
				if (!x.art || !x.art.length) say(x.label + " has no art");
				const key = JSON.stringify(x.art);
				if (seen.has(key)) say(x.label + " has the art of another icon");
				seen.add(key);
			}
			for (const x of set.suits) if (contrast(x.color, set.paper) < 3) say(x.label + " is hard to read on the paper: " + contrast(x.color, set.paper).toFixed(2));
			if (backSvg(set).childElementCount < 3) say("the back is bare");
			// A game of six suits wears the first six, in its own order.
			ids.forEach((sid, i) => {
				const f = setFace(set, info, { suit: sid, label: "K" }, sid + "-K");
				if (!f.title.startsWith("K of " + set.suits[i].label)) say("suit " + (i + 1) + " is called " + f.title);
				if (!f.style.includes("--suit: " + set.suits[i].color + ";")) say("suit " + (i + 1) + " is coloured " + f.style);
			});
			// Ranks are the game's: integers and letters, laid out and lettered.
			for (const label of ["A","2","5","10","J","Q","K","13","X","II"]) {
				if (!canLetter(label)) say(label + " cannot be lettered");
				const f = setFace(set, info, { suit: "s1", label }, "s1-" + label);
				if (!f.picture.querySelector(".ts-rank path")) say("the rank " + label + " is not lettered");
			}
			const pips = (label) => setFace(set, info, { suit: "s1", label }, "s1-" + label).picture.querySelectorAll(":scope > g[transform^='translate']").length;
			for (const n of [2, 3, 5, 9, 10]) if (pips(String(n)) !== n) say(n + " has " + pips(String(n)) + " pips");
			// Each joker has the colours of two suits, and two jokers of one game differ.
			const a = setFace(set, info, jokers[0], "joker-a");
			const b = setFace(set, info, jokers[1], "joker-b");
			if (!a.style.includes("--suit: " + set.suits[0].color + "; --suit2: " + set.suits[1].color)) say("the first joker: " + a.style);
			if (!b.style.includes("--suit: " + set.suits[2].color + "; --suit2: " + set.suits[3].color)) say("the second joker: " + b.style);
			if (a.title === b.title) say("two jokers look alike");
			for (let i = 0; i < 4; i++) {
				const j = setFace(set, { cards: { ...info.cards, types: [{ id: "j", tags: ["joker"], label: "Joker" }] } }, { id: "j", tags: ["joker"], label: "Joker" }, "j#" + i);
				if (!j.title.startsWith(set.jokers[i].label)) say("the copy " + i + " of one joker is " + j.title);
				if (!j.picture.querySelector(".ts-rank path")) say("a joker has no name");
			}
			if (setPreview(set, 4).querySelectorAll(".ts-unused").length !== 4) say("the preview dims the wrong suits");
		}
		// A set is only worn by a game whose deck can wear it.
		useCardSet("ancient");
		if (currentCardSet(info) !== CARD_SETS[0]) problems.push("a standard deck does not wear the set");
		if (currentCardSet({ cards: { standard: false } }) !== null) problems.push("a special deck wears the set");
		if (currentCardSet({}) !== null) problems.push("a game without cards wears the set");
		useCardSet("");
		return JSON.stringify({ ids: CARD_SETS.map((x) => x.id), problems });
	})()`
	var raw string
	if err := chromedp.Run(ctx, chromedp.Evaluate(script, &raw, func(p *runtime.EvaluateParams) *runtime.EvaluateParams { return p.WithAwaitPromise(true) })); err != nil {
		t.Fatal(err)
	}
	var out struct {
		IDs      []string `json:"ids"`
		Problems []string `json:"problems"`
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatal(err)
	}
	for _, p := range out.Problems {
		t.Error(p)
	}
	// The server and the page must know the same sets.
	if strings.Join(out.IDs, ",") != strings.Join(catalog.CardSetIDs, ",") {
		t.Errorf("the page has the sets %v, the server %v", out.IDs, catalog.CardSetIDs)
	}
}
