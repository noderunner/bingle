package app_test

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/games/cthulhuwars"
)

// The fourteenth round: the neutral items window, Cultists captured off a Gate,
// the Yellow Sign glyphs off unless Yellow Sign plays, and no "&nbsp;" in the rules.

func TestWebCthulhuWars_TheNeutralWindowShowsWhatIsLeftToTake(t *testing.T) {
	baseURL := newRESTTestServer(t)
	room, id := cardGame(t, baseURL, func(d *cthulhuwars.Data, st *state.BoardState) {
		d.Loyal[2] |= 1 << 1  // the Crawling Chaos has bought the Star Vampire's card
		d.Neutral[2] = 1 << 0 // and holds Shriveling
		st.SetReserve("blue", "star-vampire", 3)
	})
	ctx, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	var mu sync.Mutex
	var jsErrors []string
	collectJSErrors(ctx, &mu, &jsErrors)
	openGameAs(t, ctx, baseURL, room, id, "Alice")
	waitJS(t, ctx, "the window", `document.getElementById("neutral-window") !== null`, 20*time.Second)
	// Units are icons alone, spellbooks the book and the name.
	if n := evalInt(t, ctx, `document.querySelectorAll('#neutral-window .neutral-unit svg').length`); n != 8+1+5 {
		t.Errorf("%d unit icons, want 14 (8 Monsters, a Terror, 5 Great Old Ones)", n)
	}
	if n := evalInt(t, ctx, `document.querySelectorAll('#neutral-window .neutral-unit').length`); n != 14 {
		t.Errorf("%d units", n)
	}
	if got := evalString(t, ctx, `Array.from(document.querySelectorAll('#neutral-window .neutral-unit')).map(function (e) { return e.textContent; }).join("")`); got != "" {
		t.Errorf("a unit carries text: %q", got)
	}
	if n := evalInt(t, ctx, `document.querySelectorAll('#neutral-window .neutral-book svg').length`); n != 5 {
		t.Errorf("%d spellbook pictograms, want 5", n)
	}
	books := evalString(t, ctx, `Array.from(document.querySelectorAll('#neutral-window .neutral-book')).map(function (e) { return e.textContent; }).join("|")`)
	if strings.Contains(books, "Shriveling") || !strings.Contains(books, "Stars Are Right") || !strings.Contains(books, "Undimensioned") {
		t.Errorf("books on offer: %s", books)
	}
	if n := evalInt(t, ctx, `document.querySelectorAll('#neutral-window [data-item="star-vampire"]').length`); n != 0 {
		t.Error("the bought card is still on offer")
	}
	// Hovering a spellbook: its rules.
	hoverOver(t, ctx, `#neutral-window .neutral-book[data-item="mao-ceremony"]`)
	waitJS(t, ctx, "the book's card", `document.querySelector('.hover-card') !== null`, 5*time.Second)
	card := evalString(t, ctx, `document.querySelector('.hover-card').textContent`)
	for _, want := range []string{"The Mao Ceremony", "Neutral spellbook", "sacrifice", "Ongoing"} {
		if !strings.Contains(card, want) {
			t.Errorf("the book's card lacks %q: %s", want, card)
		}
	}
	// A Loyalty Card: what it costs and what it does.
	hoverOver(t, ctx, `#neutral-window .neutral-unit[data-item="hound-of-tindalos"]`)
	waitJS(t, ctx, "the Terror's card", `document.querySelector('.hover-card') !== null && document.querySelector('.hover-card').textContent.indexOf("Hound of Tindalos") >= 0`, 5*time.Second)
	card = evalString(t, ctx, `document.querySelector('.hover-card').textContent`)
	for _, want := range []string{"Terror", "2 Doom and 2 Power", "Chronophage", "Combat"} {
		if !strings.Contains(card, want) {
			t.Errorf("the Terror's card lacks %q: %s", want, card)
		}
	}
	// An Independent Great Old One: how it awakens and its spellbook.
	hoverOver(t, ctx, `#neutral-window .neutral-unit[data-item="yig"]`)
	waitJS(t, ctx, "Yig's card", `document.querySelector('.hover-card') !== null && document.querySelector('.hover-card').textContent.indexOf("Yig") >= 0`, 5*time.Second)
	card = evalString(t, ctx, `document.querySelector('.hover-card').textContent`)
	for _, want := range []string{"Independent Great Old One", "How to awaken", "Spellbook", "Requirement"} {
		if !strings.Contains(card, want) {
			t.Errorf("Yig's card lacks %q: %s", want, card)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if len(jsErrors) > 0 {
		t.Errorf("uncaught JS exceptions: %v", jsErrors)
	}
}

func TestWebCthulhuWars_NoNeutralWindowWithoutNeutralContent(t *testing.T) {
	baseURL := newRESTTestServer(t)
	doc := cwPosition(t, []cwUnit{{1, "cultist", "south-pacific", 1}, {2, "cultist", "asia", 1}},
		map[string]int{"south-pacific": 1, "asia": 2}, [5]int{0, 5, 3, 3, 3}, [5]uint8{}, nil)
	room := cwGameFrom(t, baseURL, doc)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	openGameAs(t, ctx, baseURL, room, "cthulhu-wars", "Alice")
	waitJS(t, ctx, "the cult cards", `document.querySelectorAll('#pool-1 .pool-unit').length > 0`, 15*time.Second)
	if n := evalInt(t, ctx, `document.querySelectorAll('#neutral-window').length`); n != 0 {
		t.Errorf("%d neutral windows at a table without neutral content", n)
	}
}

func TestWebCthulhuWars_TheCaptorChoosesTheCultistOnTheGateOrOneBesideIt(t *testing.T) {
	baseURL := newRESTTestServer(t)
	doc := cwPosition(t,
		[]cwUnit{{1, "deep-one", "south-pacific", 1}, {1, "cultist", "south-pacific", 1}, {2, "cultist", "south-pacific", 2}, {3, "cultist", "asia", 1}},
		map[string]int{"south-pacific": 2, "asia": 3},
		[5]int{0, 5, 3, 3, 3}, [5]uint8{}, nil)
	room := cwGameFrom(t, baseURL, doc)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	var mu sync.Mutex
	var jsErrors []string
	collectJSErrors(ctx, &mu, &jsErrors)
	openGameAs(t, ctx, baseURL, room, "cthulhu-wars", "Alice")
	waitJS(t, ctx, "the capture button", `document.getElementById("action-capture") !== null`, 15*time.Second)
	clickSel(t, ctx, "#action-capture")
	clickSVG(t, ctx, `#board .cell.area[data-position="south-pacific"]`)
	// Which Cultist: the one beside the Gate or the one that holds it.
	waitJS(t, ctx, "the choice", `document.querySelectorAll('#piece-buttons .piece-choice').length === 2`, 10*time.Second)
	if got := evalString(t, ctx, `Array.from(document.querySelectorAll('#piece-buttons .piece-choice')).map(function (e) { return e.dataset.piece; }).sort().join(",")`); got != "cultist,gate:cultist" {
		t.Fatalf("the choices: %s", got)
	}
	if got := evalString(t, ctx, `document.querySelector('#piece-buttons .piece-choice.on-gate').title`); !strings.Contains(got, "abandoned") {
		t.Errorf("title of the Gate's Cultist: %q", got)
	}
	clickSel(t, ctx, `#piece-buttons .piece-choice[data-piece="gate:cultist"]`)
	// The Gate is the captor's now; one of the Crawling Chaos' Cultists is held.
	waitJS(t, ctx, "the Gate changes hands", `document.querySelector('#board .gate-chip.color-green[data-position="south-pacific"]') !== null`, 15*time.Second)
	waitJS(t, ctx, "a captive", `document.querySelectorAll('#captured-1 .pool-unit.captured').length === 1`, 10*time.Second)
	if got := evalString(t, ctx, `document.querySelector('#board .gate-chip[data-position="south-pacific"]').dataset.holder`); got != "cultist" {
		t.Errorf("the captor's %q holds the Gate", got)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(jsErrors) > 0 {
		t.Errorf("uncaught JS exceptions: %v", jsErrors)
	}
}

func TestWebCthulhuWars_YellowSignGlyphsAreOffUnlessYellowSignPlays(t *testing.T) {
	baseURL := newRESTTestServer(t)
	// Without Yellow Sign: no glyph badges, the box is unchecked, and ticking it shows them.
	var created struct {
		RoomCode string `json:"room_code"`
	}
	body := map[string]any{"game_def_id": "cthulhu-wars~gc-cc-bg-sl", "player_name": "Alice", "opponent": "random"}
	if code := doJSON(t, "POST", baseURL+"/api/games", body, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	var mu sync.Mutex
	var jsErrors []string
	collectJSErrors(ctx, &mu, &jsErrors)
	openGameAs(t, ctx, baseURL, created.RoomCode, "cthulhu-wars~gc-cc-bg-sl", "Alice")
	waitJS(t, ctx, "the map", `document.querySelectorAll('#board .cell.area').length > 0 && document.querySelectorAll('#board .area-label').length > 0`, 20*time.Second)
	if n := evalInt(t, ctx, `document.querySelectorAll('#board .area-glyph-badge').length`); n != 0 {
		t.Errorf("%d glyph badges at a table without Yellow Sign", n)
	}
	openOptions(t, ctx)
	if evalBool(t, ctx, `document.getElementById("opt-layer-glyphs").checked`) {
		t.Error("the glyphs box is ticked without Yellow Sign")
	}
	clickSel(t, ctx, "#opt-layer-glyphs")
	waitJS(t, ctx, "the glyphs", `document.querySelectorAll('#board .area-glyph-badge').length > 0`, 5*time.Second)
	// A player's own choice is kept.
	waitJS(t, ctx, "stored", `localStorage.getItem("bingle.layer.glyphs") === "true"`, 5*time.Second)
	mu.Lock()
	defer mu.Unlock()
	if len(jsErrors) > 0 {
		t.Errorf("uncaught JS exceptions: %v", jsErrors)
	}
}

func TestWebCthulhuWars_EveryReactionHasItsOwnBoxesAndMadnessStartsTicked(t *testing.T) {
	baseURL := newRESTTestServer(t)
	room := madnessGame(t, baseURL)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	var mu sync.Mutex
	var jsErrors []string
	collectJSErrors(ctx, &mu, &jsErrors)
	openGameAs(t, ctx, baseURL, room, "cthulhu-wars~gc-cc-bg-ys-hp", "Alice")
	waitJS(t, ctx, "the Madness boxes", `document.getElementById("ask-madness-targeted") !== null && document.getElementById("ask-unspeakable-oath-targeted") !== null`, 20*time.Second)
	openOptions(t, ctx)
	if n := evalInt(t, ctx, `document.querySelectorAll('#options-reactions .reaction-ability').length`); n != 2 {
		t.Fatalf("%d reactions in the options window, want 2 (Madness and the Unspeakable Oath)", n)
	}
	checked := func(id string) bool { return evalBool(t, ctx, `document.getElementById("`+id+`").checked`) }
	// Madness: every box ticked; the seat boxes follow "any action" and are fixed.
	for _, id := range []string{"ask-madness-targeted", "ask-madness-all", "ask-madness-seat-1", "ask-madness-seat-3", "ask-madness-seat-4"} {
		if !checked(id) {
			t.Errorf("#%s is not ticked by default", id)
		}
	}
	if !evalBool(t, ctx, `document.getElementById("ask-madness-seat-1").disabled`) {
		t.Error("the seat boxes of Madness can be changed while \"any action\" is on")
	}
	// The Oath: only "when I'm targeted", as for every other reaction.
	if !checked("ask-unspeakable-oath-targeted") || checked("ask-unspeakable-oath-all") || checked("ask-unspeakable-oath-seat-1") {
		t.Error("the Unspeakable Oath does not start with just \"when I'm targeted\"")
	}
	// A box of one reaction leaves the other alone.
	clickSel(t, ctx, "#ask-unspeakable-oath-all")
	waitJS(t, ctx, "the Oath asks about everything", `document.getElementById("ask-unspeakable-oath-state").textContent.indexOf("after every other player's action") >= 0`, 10*time.Second)
	if !checked("ask-madness-all") {
		t.Error("ticking the Oath's box unticked Madness")
	}
	clickSel(t, ctx, "#ask-madness-all")
	waitJS(t, ctx, "Madness is asked less", `document.getElementById("ask-madness-state").textContent.indexOf("after every other player's action") < 0`, 10*time.Second)
	if !checked("ask-unspeakable-oath-all") {
		t.Error("unticking Madness' box unticked the Oath's")
	}
	if checked("ask-madness-seat-3") || evalBool(t, ctx, `document.getElementById("ask-madness-seat-3").disabled`) {
		t.Error("the seat boxes of Madness should be free and unticked now")
	}
	// The server holds the two choices apart.
	var set struct {
		Choices map[string]struct {
			Targeted bool  `json:"targeted"`
			AskAll   bool  `json:"ask_all"`
			AskSeats []int `json:"ask_seats"`
		} `json:"choices"`
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		doJSON(t, "GET", baseURL+"/api/games/"+room+"/reactions?player_name=Alice", nil, &set)
		if !set.Choices["madness"].AskAll && set.Choices["unspeakable-oath"].AskAll {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the server holds %+v", set.Choices)
		}
		time.Sleep(100 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(jsErrors) > 0 {
		t.Errorf("uncaught JS exceptions: %v", jsErrors)
	}
}
