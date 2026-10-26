package app_test

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/noderunner/bingle/internal/games/cthulhuwars"
)

// The eighth round: spellbook pictograms, the zone of captured units, Elder
// Signs that open only when there is something to show, the free step-off and
// the awakening rules in every card.

func TestWebCthulhuWars_SpellbooksAndRequirementsWearSpellbooksNotTicks(t *testing.T) {
	baseURL := newRESTTestServer(t)
	// Great Cthulhu has Devolve (a spellbook), and two requirements are met.
	doc := cwPosition(t,
		[]cwUnit{{1, "cultist", "south-pacific", 2}, {2, "cultist", "asia", 1}},
		map[string]int{"south-pacific": 1, "asia": 2},
		[5]int{0, 5, 3, 3, 3}, [5]uint8{0, 1 << 1},
		func(d *cthulhuwars.Data) { d.Reqs[1] = 1<<0 | 1<<2 })
	room := cwGameFrom(t, baseURL, doc)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	var mu sync.Mutex
	var jsErrors []string
	collectJSErrors(ctx, &mu, &jsErrors)
	openGameAs(t, ctx, baseURL, room, "cthulhu-wars", "Alice")
	waitJS(t, ctx, "the cult card", `document.querySelectorAll('#seat-1 .faction-card').length === 5`, 15*time.Second)
	// Open the two sections.
	for _, card := range []string{"books", "reqs"} {
		if err := chromedp.Run(ctx, chromedp.Evaluate(`document.querySelector('#seat-1 .faction-card[data-card="`+card+`"]').open = true; true`, nil)); err != nil {
			t.Fatal(err)
		}
	}
	count := func(sel string) int { return evalInt(t, ctx, `document.querySelectorAll('`+sel+`').length`) }
	// Spellbooks: one unlocked (colour), five to come (greyed out).
	if n := count(`#seat-1 [data-card="books"] .row-mark svg.spell-mark.on`); n != 1 {
		t.Errorf("%d unlocked spellbook icons, want 1", n)
	}
	if n := count(`#seat-1 [data-card="books"] .row-mark svg.spell-mark.off`); n != 5 {
		t.Errorf("%d greyed spellbook icons, want 5", n)
	}
	// Requirements: two met (colour), four not yet (greyed out).
	if n := count(`#seat-1 [data-card="reqs"] .row-mark svg.spell-mark.on`); n != 2 {
		t.Errorf("%d met requirement icons, want 2", n)
	}
	if n := count(`#seat-1 [data-card="reqs"] .row-mark svg.spell-mark.off`); n != 4 {
		t.Errorf("%d greyed requirement icons, want 4", n)
	}
	// No green ticks.
	for _, card := range []string{"books", "reqs"} {
		if got := evalString(t, ctx, `[...document.querySelectorAll('#seat-1 [data-card="`+card+`"] .row-mark')].map(e => e.textContent).join("")`); strings.ContainsAny(got, "✓○") {
			t.Errorf("%s still has ticks: %q", card, got)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if len(jsErrors) > 0 {
		t.Errorf("uncaught JS exceptions: %v", jsErrors)
	}
}

func TestWebCthulhuWars_ABookAbilityWearsAColouredSpellbookAndARequirementAGreyOne(t *testing.T) {
	baseURL := newRESTTestServer(t)
	// Great Cthulhu to move: Dreams (a spellbook) and the requirement actions
	// of Crawling Chaos are not his; he has Submerge (bit 4) with Cthulhu in
	// the South Pacific, and "Dreams" (bit 2) with an enemy Acolyte to replace.
	doc := cwPosition(t,
		[]cwUnit{{1, "cultist", "south-pacific", 1}, {1, "cthulhu", "south-pacific", 1}, {2, "cultist", "asia", 1}},
		map[string]int{"south-pacific": 1, "asia": 2},
		[5]int{0, 8, 3, 3, 3}, [5]uint8{0, 1<<4 | 1<<2}, nil)
	room := cwGameFrom(t, baseURL, doc)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	var mu sync.Mutex
	var jsErrors []string
	collectJSErrors(ctx, &mu, &jsErrors)
	openGameAs(t, ctx, baseURL, room, "cthulhu-wars", "Alice")
	waitJS(t, ctx, "Submerge", `document.getElementById("action-submerge") !== null`, 15*time.Second)
	// Submerge is an ability of an unlocked spellbook: a coloured one.
	if n := evalInt(t, ctx, `document.querySelectorAll('#action-submerge svg.spell-mark.on').length`); n != 1 {
		t.Errorf("%d coloured spellbooks on Submerge, want 1", n)
	}
	if got := evalString(t, ctx, `document.getElementById("action-submerge").dataset.source`); got != "book" {
		t.Errorf("source = %q", got)
	}
	// Actions that are not of a spellbook wear none.
	for _, id := range []string{"action-recruit", "action-gate", "action-pass", "action-awaken-cthulhu"} {
		if n := evalInt(t, ctx, `(document.getElementById("`+id+`") ? document.getElementById("`+id+`").querySelectorAll("svg.spell-mark").length : 0)`); n != 0 {
			t.Errorf("#%s wears a spellbook", id)
		}
	}
	// Its card says where the action comes from.
	hoverOver(t, ctx, "#action-submerge")
	waitJS(t, ctx, "the card", `document.querySelector('.hover-card') !== null`, 5*time.Second)
	if got := evalString(t, ctx, `document.querySelector('.hover-card').textContent`); !strings.Contains(got, "spellbook you have unlocked") {
		t.Errorf("card = %q", got)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(jsErrors) > 0 {
		t.Errorf("uncaught JS exceptions: %v", jsErrors)
	}
}

func TestWebCthulhuWars_AnActionThatMeetsARequirementWearsAGreySpellbook(t *testing.T) {
	baseURL := newRESTTestServer(t)
	// Crawling Chaos is the seat the page plays: it is the creator's seat.
	var created struct {
		RoomCode string `json:"room_code"`
	}
	doc := cwPosition(t,
		[]cwUnit{{1, "cultist", "south-pacific", 1}, {2, "cultist", "asia", 1}},
		map[string]int{"south-pacific": 1, "asia": 2},
		[5]int{0, 3, 8, 3, 3}, [5]uint8{}, func(d *cthulhuwars.Data) { d.Turn.Seat = 2 })
	body := map[string]any{
		"game_def_id": "cthulhu-wars", "player_name": "Alice", "seat": 2,
		"seat_bots":   map[string]string{"1": "random", "3": "random", "4": "random"},
		"start_state": json.RawMessage(doc), "start_seat": 2,
	}
	if code := doJSON(t, "POST", baseURL+"/api/games", body, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	var mu sync.Mutex
	var jsErrors []string
	collectJSErrors(ctx, &mu, &jsErrors)
	openGameAs(t, ctx, baseURL, created.RoomCode, "cthulhu-wars", "Alice")
	waitJS(t, ctx, "the requirement buttons", `document.getElementById("action-pay-4") !== null`, 20*time.Second)
	for _, id := range []string{"action-pay-4", "action-pay-6"} {
		if n := evalInt(t, ctx, `document.querySelectorAll('#`+id+` svg.spell-mark.off').length`); n != 1 {
			t.Errorf("#%s: %d greyed spellbooks, want 1", id, n)
		}
		if n := evalInt(t, ctx, `document.querySelectorAll('#`+id+` svg.spell-mark.on').length`); n != 0 {
			t.Errorf("#%s wears a coloured spellbook", id)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if len(jsErrors) > 0 {
		t.Errorf("uncaught JS exceptions: %v", jsErrors)
	}
}

func TestWebCthulhuWars_CapturedUnitsAreShownUnderTheCultThatHoldsThem(t *testing.T) {
	baseURL := newRESTTestServer(t)
	doc := cwPosition(t,
		[]cwUnit{{1, "cultist", "south-pacific", 2}, {2, "cultist", "asia", 1}},
		map[string]int{"south-pacific": 1, "asia": 2},
		[5]int{0, 5, 3, 3, 3}, [5]uint8{},
		func(d *cthulhuwars.Data) { d.Captured[1][2] = 2 }) // Great Cthulhu holds two Crawling Chaos Cultists
	room := cwGameFrom(t, baseURL, doc)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	var mu sync.Mutex
	var jsErrors []string
	collectJSErrors(ctx, &mu, &jsErrors)
	openGameAs(t, ctx, baseURL, room, "cthulhu-wars", "Alice")
	waitJS(t, ctx, "the zone", `document.getElementById("captured-1") !== null`, 15*time.Second)
	// Two icons, in the colour of the cult they belong to, under the units.
	if n := evalInt(t, ctx, `document.querySelectorAll('#captured-1 .pool-unit.captured[data-owner="2"]').length`); n != 2 {
		t.Errorf("%d captured icons, want 2", n)
	}
	if got := evalString(t, ctx, `document.querySelector('#captured-1 .zone-title').textContent`); !strings.Contains(got, "Captured") {
		t.Errorf("title = %q", got)
	}
	if n := evalInt(t, ctx, `document.querySelectorAll('#seat-1 [data-card="units"] #captured-1').length`); n != 1 {
		t.Errorf("the zone is not under the Units section")
	}
	// Hover: whose it is and what becomes of it.
	hoverOver(t, ctx, `#captured-1 .pool-unit.captured`)
	waitJS(t, ctx, "the card", `document.querySelector('.hover-card') !== null`, 5*time.Second)
	if got := evalString(t, ctx, `document.querySelector('.hover-card').textContent`); !strings.Contains(got, "goes back to Crawling Chaos's pool in Gather Power") {
		t.Errorf("card = %q", got)
	}
	// The others hold nobody: no zone.
	if n := evalInt(t, ctx, `document.querySelectorAll('#captured-2, #captured-3, #captured-4').length`); n != 0 {
		t.Errorf("%d zones of cults that hold nobody", n)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(jsErrors) > 0 {
		t.Errorf("uncaught JS exceptions: %v", jsErrors)
	}
}

func TestWebCthulhuWars_SteppingOffTheGateInPlaceIsFreeAndAsksForNothing(t *testing.T) {
	baseURL := newRESTTestServer(t)
	doc := cwPosition(t,
		[]cwUnit{{1, "cultist", "south-pacific", 2}, {2, "cultist", "asia", 1}},
		map[string]int{"south-pacific": 1, "asia": 2},
		[5]int{0, 5, 3, 3, 3}, [5]uint8{}, nil)
	room := cwGameFrom(t, baseURL, doc)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	var mu sync.Mutex
	var jsErrors []string
	collectJSErrors(ctx, &mu, &jsErrors)
	openGameAs(t, ctx, baseURL, room, "cthulhu-wars", "Alice")
	waitJS(t, ctx, "the Gate", `document.querySelector('#board .gate-chip.gate-pickable[data-position="south-pacific"]') !== null`, 15*time.Second)
	clickSVG(t, ctx, `#board .gate-chip[data-position="south-pacific"]`)
	waitJS(t, ctx, "its own area is a destination", `document.querySelector('#board .gate-chip.gate-target[data-position="south-pacific"]') !== null`, 10*time.Second)
	clickSVG(t, ctx, `#board .gate-chip[data-position="south-pacific"]`)
	waitJS(t, ctx, "abandoned", `document.querySelector('#board .gate-chip.abandoned[data-position="south-pacific"]') !== null`, 15*time.Second)
	// It cost nothing, no Move is open: nothing to finish, nothing to take back.
	waitJS(t, ctx, "power", `document.querySelector('#seat-1 [data-counter="power"]').textContent === "5"`, 10*time.Second)
	if n := evalInt(t, ctx, `document.querySelectorAll('#action-done').length`); n != 0 {
		t.Errorf("a Done button after a free step-off")
	}
	if n := evalInt(t, ctx, `document.querySelectorAll('#undo-move').length`); n != 0 {
		t.Errorf("an Undo button after a free step-off")
	}
	// The turn goes on as before: the main actions are all there.
	if n := evalInt(t, ctx, `document.querySelectorAll('#action-recruit, #action-gate').length`); n < 1 {
		t.Errorf("the turn did not go on")
	}
	// Moving the unit to another area is a Move step: 1 Power, Done on offer.
	clickSVG(t, ctx, `#board .token.token-target[data-position="south-pacific"][data-piece="cultist"]`)
	waitJS(t, ctx, "a destination", `document.querySelector('#board .cell.area.clickable[data-position="new-zealand"]') !== null`, 10*time.Second)
	clickCell(t, ctx, "new-zealand")
	waitJS(t, ctx, "power paid", `document.querySelector('#seat-1 [data-counter="power"]').textContent === "4"`, 10*time.Second)
	mu.Lock()
	defer mu.Unlock()
	if len(jsErrors) > 0 {
		t.Errorf("uncaught JS exceptions: %v", jsErrors)
	}
}

func TestWebCthulhuWars_EveryCardOfAGreatOldOneSaysHowToAwakenIt(t *testing.T) {
	baseURL := newRESTTestServer(t)
	doc := cwPosition(t,
		[]cwUnit{{1, "cultist", "south-pacific", 1}, {1, "cthulhu", "south-pacific", 1}, {2, "cultist", "asia", 1}},
		map[string]int{"south-pacific": 1, "asia": 2},
		[5]int{0, 10, 3, 3, 3}, [5]uint8{}, nil)
	room := cwGameFrom(t, baseURL, doc)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	var mu sync.Mutex
	var jsErrors []string
	collectJSErrors(ctx, &mu, &jsErrors)
	openGameAs(t, ctx, baseURL, room, "cthulhu-wars", "Alice")
	waitJS(t, ctx, "the pool", `document.querySelector('#seat-1 .pool-unit[data-piece="cthulhu"]') !== null`, 15*time.Second)
	// The roster icon of the Great Old One: the numbered steps.
	hoverOver(t, ctx, `#seat-1 .pool-unit[data-piece="cthulhu"]`)
	waitJS(t, ctx, "the card", `document.querySelector('.hover-card') !== null`, 5*time.Second)
	card := evalString(t, ctx, `document.querySelector('.hover-card').textContent`)
	for _, want := range []string{"How to awaken", "There must be a Gate in South Pacific", "Pay 10 Power; 4 Power every time after the first", "gain 1 Elder Sign"} {
		if !strings.Contains(card, want) {
			t.Errorf("the pool icon's card lacks %q: %s", want, card)
		}
	}
	if n := evalInt(t, ctx, `document.querySelectorAll('.hover-card ol.awaken-steps li').length`); n != 3 {
		t.Errorf("%d steps, want 3", n)
	}
	// The card that opens when the unit is clicked on the map.
	clickSVG(t, ctx, `#board .token[data-position="south-pacific"][data-piece="cthulhu"]`)
	waitJS(t, ctx, "the unit card", `document.getElementById("unit-info") !== null`, 5*time.Second)
	if got := evalString(t, ctx, `document.getElementById("unit-info").textContent`); !strings.Contains(got, "How to awaken") || !strings.Contains(got, "Pay 10 Power") {
		t.Errorf("the unit card lacks the steps: %s", got)
	}
	// The rules window: every Great Old One, on the Actions tab.
	clickSel(t, ctx, "#show-rules")
	waitJS(t, ctx, "the rules", `document.querySelectorAll('#rules-dialog [role="tab"]').length > 0`, 10*time.Second)
	clickSel(t, ctx, `#rules-dialog [role="tab"][data-tab="actions"]`)
	waitJS(t, ctx, "the actions page", `document.getElementById("rules-panel-actions") !== null`, 5*time.Second)
	page := evalString(t, ctx, `document.getElementById("rules-panel-actions").textContent`)
	for _, want := range []string{
		"Cthulhu", "starting area", "Nyarlathotep", "control a Gate", "Shub-Niggurath", "2 Cultists",
		"King in Yellow", "no Gate", "Hastur", "where the King in Yellow stands", "Rhan-Tegoth", "Windwalker sigil",
		"Ithaqua", "replaces that Gate", "Tsathoggua", "Formless Spawn",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the Actions page lacks %q", want)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if len(jsErrors) > 0 {
		t.Errorf("uncaught JS exceptions: %v", jsErrors)
	}
}
