package app_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// The seventh round: the units of a cult all shown (those away greyed out), a
// button for every Great Old One, buttons that say what they cost and open a
// card with their rule, and the words above them.

// hoverOver shows the hover card of an element (the same events a pointer sends).
func hoverOver(t *testing.T, ctx context.Context, sel string) {
	t.Helper()
	js := `(function(){ var el = document.querySelector(` + itoaQuote(sel) + `); if (!el) return false; el.dispatchEvent(new MouseEvent("mouseenter", {bubbles: false})); return true; })()`
	var ok bool
	if err := chromedp.Run(ctx, chromedp.Evaluate(js, &ok)); err != nil || !ok {
		t.Fatalf("hover %s: ok=%v err=%v", sel, ok, err)
	}
}

func TestWebCthulhuWars_ActionButtonsShowTheirCostAndTheirRuleOnHover(t *testing.T) {
	baseURL := newRESTTestServer(t)
	doc := cwPosition(t,
		[]cwUnit{{1, "cultist", "south-pacific", 2}, {1, "cultist", "new-zealand", 1}, {2, "cultist", "asia", 1}},
		map[string]int{"south-pacific": 1, "asia": 2},
		[5]int{0, 10, 3, 3, 3}, [5]uint8{}, nil)
	room := cwGameFrom(t, baseURL, doc)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	var mu sync.Mutex
	var jsErrors []string
	collectJSErrors(ctx, &mu, &jsErrors)
	openGameAs(t, ctx, baseURL, room, "cthulhu-wars", "Alice")
	waitJS(t, ctx, "the buttons", `document.getElementById("action-gate") !== null`, 15*time.Second)

	// The words above the buttons on a normal turn.
	if got := evalString(t, ctx, `document.getElementById("move-hint").textContent`); got != "Click a unit on the map to move, or choose an action:" {
		t.Errorf("hint = %q", got)
	}
	// The Gate's button says what it costs, and no longer says "Create".
	if got := evalString(t, ctx, `document.getElementById("action-gate").textContent`); got != "Build Gate (3)" {
		t.Errorf("gate button = %q", got)
	}
	if got := evalString(t, ctx, `document.getElementById("action-recruit").textContent`); got != "Recruit Cultist (1)" {
		t.Errorf("recruit button = %q", got)
	}
	// Great Old Ones, like monsters, have a button each with the cost on it; the
	// generic ones are gone.
	if got := evalString(t, ctx, `document.getElementById("action-awaken-cthulhu").dataset.cost`); got != "10" {
		t.Errorf("Cthulhu costs %q", got)
	}
	if got := evalString(t, ctx, `document.getElementById("action-awaken-cthulhu").textContent`); !strings.Contains(got, "Awaken Cthulhu") {
		t.Errorf("awaken button = %q", got)
	}
	if got := evalString(t, ctx, `document.getElementById("action-summon-deep-one").dataset.cost`); got != "1" {
		t.Errorf("Deep One costs %q", got)
	}
	for _, id := range []string{"action-awaken", "action-summon", "action-abandon", "action-control"} {
		if n := evalInt(t, ctx, `document.querySelectorAll('#`+id+`').length`); n != 0 {
			t.Errorf("#%s is on the page", id)
		}
	}

	// Hovering the Gate's button: what it costs and where it can be built.
	hoverOver(t, ctx, "#action-gate")
	waitJS(t, ctx, "the card of the Gate", `document.querySelector('.hover-card') !== null`, 5*time.Second)
	card := evalString(t, ctx, `document.querySelector('.hover-card').textContent`)
	for _, want := range []string{"Build Gate", "3 Power", "one Gate to an area", "no Gate yet"} {
		if !strings.Contains(card, want) {
			t.Errorf("the Gate's card lacks %q: %s", want, card)
		}
	}
	// A Great Old One's: its numbers and the place its card asks for.
	hoverOver(t, ctx, "#action-awaken-cthulhu")
	waitJS(t, ctx, "the card of Cthulhu", `document.querySelector('.hover-card') !== null && document.querySelector('.hover-card').textContent.indexOf("Cthulhu") >= 0`, 5*time.Second)
	card = evalString(t, ctx, `document.querySelector('.hover-card').textContent`)
	for _, want := range []string{"Awaken", "10 Power", "How to awaken", "Gate in South Pacific"} {
		if !strings.Contains(card, want) {
			t.Errorf("Cthulhu's card lacks %q: %s", want, card)
		}
	}
	// Pass costs nothing: no price on it.
	if got := evalString(t, ctx, `document.getElementById("action-pass").textContent`); strings.Contains(got, "(0)") {
		t.Errorf("pass button = %q", got)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(jsErrors) > 0 {
		t.Errorf("uncaught JS exceptions: %v", jsErrors)
	}
}

func TestWebCthulhuWars_TheUnitsOfTheCultAreAllListedAndThoseAwayAreGreyedOut(t *testing.T) {
	baseURL := newRESTTestServer(t)
	doc := cwPosition(t,
		[]cwUnit{{1, "cultist", "south-pacific", 3}, {1, "cultist", "new-zealand", 1}, {1, "deep-one", "south-pacific", 1}, {2, "cultist", "asia", 1}},
		map[string]int{"south-pacific": 1, "asia": 2},
		[5]int{0, 5, 3, 3, 3}, [5]uint8{}, nil)
	room := cwGameFrom(t, baseURL, doc)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	var mu sync.Mutex
	var jsErrors []string
	collectJSErrors(ctx, &mu, &jsErrors)
	openGameAs(t, ctx, baseURL, room, "cthulhu-wars", "Alice")
	waitJS(t, ctx, "the cult card", `document.querySelectorAll('#pool-1 .pool-unit').length > 0`, 15*time.Second)
	// 6 Acolytes, 4 Deep Ones, 2 Shoggoths, 2 Star Spawn and Cthulhu: 15 units.
	if n := evalInt(t, ctx, `document.querySelectorAll('#pool-1 .pool-unit').length`); n != 15 {
		t.Errorf("%d unit icons, want 15", n)
	}
	// 5 of them stand on the map: grey; the rest are in the pool.
	if n := evalInt(t, ctx, `document.querySelectorAll('#pool-1 .pool-unit.away').length`); n != 5 {
		t.Errorf("%d greyed icons, want 5", n)
	}
	if n := evalInt(t, ctx, `document.querySelectorAll('#pool-1 .pool-unit[data-zone="pool"]').length`); n != 10 {
		t.Errorf("%d icons in the pool, want 10", n)
	}
	if n := evalInt(t, ctx, `document.querySelectorAll('#pool-1 .pool-unit.away[data-piece="cultist"]').length`); n != 4 {
		t.Errorf("%d Acolytes away, want 4", n)
	}
	if got := evalString(t, ctx, `document.querySelector('#seat-1 [data-card="units"] summary').textContent`); !strings.Contains(got, "10 in the pool") {
		t.Errorf("summary = %q", got)
	}
	// An icon away says where that unit is.
	hoverOver(t, ctx, `#pool-1 .pool-unit.away[data-piece="cultist"]`)
	waitJS(t, ctx, "the card", `document.querySelector('.hover-card') !== null`, 5*time.Second)
	card := evalString(t, ctx, `document.querySelector('.hover-card').textContent`)
	if !strings.Contains(card, "This one:") || (!strings.Contains(card, "New Zealand") && !strings.Contains(card, "South Pacific")) {
		t.Errorf("card = %s", card)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(jsErrors) > 0 {
		t.Errorf("uncaught JS exceptions: %v", jsErrors)
	}
}

func TestWebCthulhuWars_AwakeningByItsButtonLeadsToAHeadedSpellbookChoice(t *testing.T) {
	baseURL := newRESTTestServer(t)
	doc := cwPosition(t,
		[]cwUnit{{1, "cultist", "south-pacific", 1}, {2, "cultist", "asia", 1}},
		map[string]int{"south-pacific": 1, "asia": 2},
		[5]int{0, 10, 3, 3, 3}, [5]uint8{}, nil)
	room := cwGameFrom(t, baseURL, doc)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	var mu sync.Mutex
	var jsErrors []string
	collectJSErrors(ctx, &mu, &jsErrors)
	openGameAs(t, ctx, baseURL, room, "cthulhu-wars", "Alice")
	waitJS(t, ctx, "the awaken button", `document.getElementById("action-awaken-cthulhu") !== null`, 15*time.Second)
	clickSel(t, ctx, "#action-awaken-cthulhu")
	waitJS(t, ctx, "the area to awaken in", `document.querySelector('#board .cell.area.clickable[data-position="south-pacific"]') !== null`, 10*time.Second)
	clickCell(t, ctx, "south-pacific")
	// Cthulhu's awakening meets a requirement: the spellbook to unlock.
	waitJS(t, ctx, "the choice of a spellbook", `document.getElementById("action-book-absorb") !== null`, 15*time.Second)
	if got := evalString(t, ctx, `document.getElementById("move-hint").textContent`); got != "Choose a spellbook to unlock:" {
		t.Errorf("hint = %q", got)
	}
	if n := evalInt(t, ctx, `document.querySelectorAll('#move-bar [data-action^="book-"]').length`); n != 6 {
		t.Errorf("%d spellbook buttons, want 6", n)
	}
	// Each has a card with the rule of the spellbook.
	hoverOver(t, ctx, "#action-book-submerge")
	waitJS(t, ctx, "the card", `document.querySelector('.hover-card') !== null`, 5*time.Second)
	card := evalString(t, ctx, `document.querySelector('.hover-card').textContent`)
	for _, want := range []string{"Submerge", "Action: Cost 1", "faction card"} {
		if !strings.Contains(card, want) {
			t.Errorf("the card of Submerge lacks %q: %s", want, card)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if len(jsErrors) > 0 {
		t.Errorf("uncaught JS exceptions: %v", jsErrors)
	}
}
