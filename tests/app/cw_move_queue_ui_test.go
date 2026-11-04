// The units of a Move are gathered in the impulse card, from one area, and sent
// together when the player clicks their destination.

package app_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

func moveQueueGame(t *testing.T, power int) (context.Context, func()) {
	t.Helper()
	baseURL := newRESTTestServer(t)
	doc := cwPosition(t,
		[]cwUnit{{1, "cultist", "south-pacific", 3}, {1, "deep-one", "south-pacific", 1}, {1, "cultist", "new-zealand", 1}, {2, "cultist", "asia", 1}},
		map[string]int{"asia": 2},
		[5]int{0, power, 3, 3, 3}, [5]uint8{}, nil)
	room := cwGameFrom(t, baseURL, doc)
	ctx, cancel := newChromedpContext(t, 90*time.Second)
	if err := chromedp.Run(ctx, chromedp.EmulateViewport(1400, 900)); err != nil {
		t.Fatal(err)
	}
	openGameAs(t, ctx, baseURL, room, "cthulhu-wars", "Alice")
	waitJS(t, ctx, "the buttons", `document.getElementById("action-recruit") !== null`, 15*time.Second)
	return ctx, cancel
}

func clickUnit(t *testing.T, ctx context.Context, area, piece string) {
	t.Helper()
	if !evalBool(t, ctx, `(document.querySelector('#board .token[data-position="`+area+`"][data-piece="`+piece+`"]').dispatchEvent(new MouseEvent("click", {bubbles: true})), true)`) {
		t.Fatalf("click on %s in %s", piece, area)
	}
}

func TestWebCthulhuWars_AMoveQueuesItsUnitsThenSendsThemTogether(t *testing.T) {
	ctx, cancel := moveQueueGame(t, 10)
	defer cancel()
	text := func() string { return evalString(t, ctx, `document.getElementById("impulse-text").textContent`) }
	cost := func() string { return evalString(t, ctx, `document.getElementById("move-queue-cost").textContent`) }
	badge := `document.querySelector('.queue-unit[data-piece="cultist"] .queue-count')`

	// The first unit starts the impulse, with its icon and the Power it costs.
	clickUnit(t, ctx, "south-pacific", "cultist")
	waitJS(t, ctx, "the queue", `document.getElementById("move-queue") !== null`, 5*time.Second)
	if got := text(); !strings.HasPrefix(got, "Select remaining units in ") || !strings.HasSuffix(got, " to move, followed by a destination.") {
		t.Errorf("the impulse says %q", got)
	}
	if !strings.Contains(text(), "South Pacific") {
		t.Errorf("the impulse does not name the area: %q", text())
	}
	if got := cost(); got != "1" {
		t.Errorf("one unit costs %q", got)
	}
	if evalBool(t, ctx, badge+` !== null`) {
		t.Error("one unit has a stack badge")
	}
	for _, id := range []string{"reset-move", "action-gate", "action-pass"} {
		if evalBool(t, ctx, `document.getElementById("`+id+`") !== null`) {
			t.Errorf("#%s is on the card during the queue", id)
		}
	}

	// The same type grows a badge; a new type adds an icon.
	clickUnit(t, ctx, "south-pacific", "cultist")
	waitJS(t, ctx, "a badge of two", badge+` && `+badge+`.textContent === "2"`, 5*time.Second)
	clickUnit(t, ctx, "south-pacific", "deep-one")
	waitJS(t, ctx, "the second icon", `document.querySelector('.queue-unit[data-piece="deep-one"]') !== null`, 5*time.Second)
	if got := cost(); got != "3" {
		t.Errorf("three units cost %q", got)
	}
	// Units elsewhere wait for another queue.
	if evalBool(t, ctx, `document.querySelector('#board .token[data-position="new-zealand"].clickable, #board .token[data-position="new-zealand"].selectable') !== null`) {
		t.Error("a unit in another area can be added")
	}

	// A click on an icon takes that unit out.
	clickSel(t, ctx, `.queue-unit[data-piece="deep-one"]`)
	waitJS(t, ctx, "the icon gone", `document.querySelector('.queue-unit[data-piece="deep-one"]') === null`, 5*time.Second)
	if got := cost(); got != "2" {
		t.Errorf("two units cost %q", got)
	}

	// Back empties the queue and returns to the impulse before it.
	clickSel(t, ctx, "#undo-step")
	waitJS(t, ctx, "the actions again", `document.getElementById("move-queue") === null && document.getElementById("action-gate") !== null`, 5*time.Second)
	if got := text(); got != "Choose an action." {
		t.Errorf("after Back the impulse says %q", got)
	}

	// Two units to a destination: one click sends both, and the next card sums it up.
	clickUnit(t, ctx, "south-pacific", "cultist")
	clickUnit(t, ctx, "south-pacific", "cultist")
	waitJS(t, ctx, "a badge of two", badge+` && `+badge+`.textContent === "2"`, 5*time.Second)
	clickCell(t, ctx, "new-zealand")
	waitJS(t, ctx, "the summary", `document.getElementById("impulse-text").textContent.indexOf("Moved 2 Acolyte Cultists to New Zealand for 2 Power") === 0`, 15*time.Second)
	waitJS(t, ctx, "Done and Back", `document.getElementById("action-done") !== null && document.getElementById("undo-step") !== null`, 10*time.Second)
	if evalBool(t, ctx, `document.getElementById("reset-move") !== null`) {
		t.Error("Reset is on the card")
	}

	// Back takes the whole group back.
	clickSel(t, ctx, "#undo-step")
	waitJS(t, ctx, "the turn again", `document.getElementById("action-gate") !== null && document.getElementById("impulse-text").textContent === "Choose an action."`, 15*time.Second)
	if n := evalInt(t, ctx, `document.querySelectorAll('#move-list .mv').length`); n != 0 {
		t.Errorf("%d moves left after Back", n)
	}
}

// A unit in the queue is off the map, and a unit taken out of the queue is back on it.
func TestWebCthulhuWars_AQueuedUnitLeavesTheMap(t *testing.T) {
	baseURL := newRESTTestServer(t)
	doc := cwPosition(t,
		[]cwUnit{{1, "cultist", "south-pacific", 3}, {1, "deep-one", "south-pacific", 1}, {2, "cultist", "asia", 1}},
		map[string]int{"south-pacific": 1, "asia": 2},
		[5]int{0, 10, 3, 3, 3}, [5]uint8{}, nil)
	room := cwGameFrom(t, baseURL, doc)
	ctx, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	openGameAs(t, ctx, baseURL, room, "cthulhu-wars", "Alice")
	waitJS(t, ctx, "the buttons", `document.getElementById("action-recruit") !== null`, 15*time.Second)
	token := func(piece string) string {
		return `document.querySelector('#board .token[data-position="south-pacific"][data-piece="` + piece + `"]')`
	}
	cultists := func(what, want string) {
		t.Helper()
		waitJS(t, ctx, what, token("cultist")+` && `+token("cultist")+`.dataset.count === "`+want+`"`, 5*time.Second)
	}
	holder := `document.querySelector('#board .gate-chip[data-position="south-pacific"]').dataset.holder`

	// Three Cultists, one of them on the Gate: a stack of two beside it.
	cultists("a stack of two", "2")
	clickUnit(t, ctx, "south-pacific", "cultist")
	cultists("one lifted off", "1")
	// The last of a type leaves no token behind.
	clickUnit(t, ctx, "south-pacific", "deep-one")
	waitJS(t, ctx, "the Deep One lifted off", token("deep-one")+` === null`, 5*time.Second)
	// A click on its icon puts it back.
	clickSel(t, ctx, `.queue-unit[data-piece="deep-one"]`)
	waitJS(t, ctx, "the Deep One back", token("deep-one")+` !== null`, 5*time.Second)

	// The unit on the Gate leaves the Gate empty, and the Gate stays.
	clickSVG(t, ctx, `#board .gate-chip[data-position="south-pacific"]`)
	waitJS(t, ctx, "the Gate without its figure", holder+` === ""`, 5*time.Second)
	cultists("the stack beside the Gate as it was", "1")
	clickSel(t, ctx, `.queue-unit[data-piece="gate:cultist"]`)
	waitJS(t, ctx, "the figure back", holder+` === "cultist"`, 5*time.Second)

	// Back puts everything back.
	clickSel(t, ctx, "#undo-step")
	waitJS(t, ctx, "the queue gone", `document.getElementById("move-queue") === null`, 5*time.Second)
	cultists("the stack of two again", "2")

	// Nothing was sent, so nothing was paid.
	if got := evalString(t, ctx, `String(document.querySelector('#seat-1 [data-counter="power"]').textContent)`); got != "10" {
		t.Errorf("Power %s after a queue that went nowhere", got)
	}
}

// The queue stops growing at the Power there is to pay for it.
func TestWebCthulhuWars_AMoveQueueStopsAtThePowerThereIs(t *testing.T) {
	ctx, cancel := moveQueueGame(t, 2)
	defer cancel()
	clickUnit(t, ctx, "south-pacific", "cultist")
	clickUnit(t, ctx, "south-pacific", "cultist")
	badge := `document.querySelector('.queue-unit[data-piece="cultist"] .queue-count')`
	waitJS(t, ctx, "a badge of two", badge+` && `+badge+`.textContent === "2"`, 5*time.Second)
	clickUnit(t, ctx, "south-pacific", "cultist")
	clickUnit(t, ctx, "south-pacific", "deep-one")
	time.Sleep(300 * time.Millisecond)
	if got := evalString(t, ctx, badge+`.textContent`); got != "2" {
		t.Errorf("the badge says %q with 2 Power", got)
	}
	if evalBool(t, ctx, `document.querySelector('.queue-unit[data-piece="deep-one"]') !== null`) {
		t.Error("a unit the player cannot pay for joined the queue")
	}
	// Spending the last of the Power still stops at the card that confirms it.
	clickCell(t, ctx, "new-zealand")
	waitJS(t, ctx, "the summary", `document.getElementById("impulse-text").textContent.indexOf("Moved 2 Acolyte Cultists") === 0`, 15*time.Second)
	waitJS(t, ctx, "Done and Back", `document.getElementById("action-done") !== null && document.getElementById("undo-step") !== null`, 10*time.Second)
}

// Pass says Pass, and with Power left the next impulse asks whether to give it up.
func TestWebCthulhuWars_PassAsksBeforeGivingUpPower(t *testing.T) {
	ctx, cancel := moveQueueGame(t, 4)
	defer cancel()
	if got := evalString(t, ctx, `document.getElementById("action-pass").textContent`); got != "Pass" {
		t.Errorf("the button says %q", got)
	}
	clickSel(t, ctx, "#action-pass")
	waitJS(t, ctx, "the question", `document.getElementById("pass-confirm") !== null`, 5*time.Second)
	if got := evalString(t, ctx, `document.getElementById("impulse-text").textContent`); got != "You will lose your 4 unspent Power." {
		t.Errorf("the impulse says %q", got)
	}
	if !evalBool(t, ctx, `document.querySelector('#seat-1 [data-counter="power"]').textContent.indexOf("4") >= 0`) {
		t.Error("asking cost the Power")
	}
	// Back keeps it.
	clickSel(t, ctx, "#undo-step")
	waitJS(t, ctx, "the actions again", `document.getElementById("action-pass") !== null && document.getElementById("impulse-text").textContent === "Choose an action."`, 5*time.Second)
	// Confirming spends it.
	clickSel(t, ctx, "#action-pass")
	waitJS(t, ctx, "the question", `document.getElementById("pass-confirm") !== null`, 5*time.Second)
	clickSel(t, ctx, "#pass-confirm")
	waitJS(t, ctx, "the pass", `document.body.textContent.indexOf("passes, dropping to 0 Power") >= 0`, 15*time.Second)
}

// A cult's card and the impulse card use its glyph and its colour, and no colour box.
func TestWebCthulhuWars_TheCultsWordsAreInItsColour(t *testing.T) {
	ctx, cancel := moveQueueGame(t, 4)
	defer cancel()
	if n := evalInt(t, ctx, `document.querySelectorAll("#seat-1 .seat-head .swatch").length`); n != 0 {
		t.Errorf("%d colour boxes on the card", n)
	}
	if !evalBool(t, ctx, `document.querySelector("#seat-1 .seat-head .seat-glyph") !== null`) {
		t.Error("no glyph on the card")
	}
	if !evalBool(t, ctx, `getComputedStyle(document.querySelector("#seat-1 .name")).color !== getComputedStyle(document.querySelector("#impulse-text")).color`) {
		t.Error("the name is not in the cult's colour")
	}
	if !evalBool(t, ctx, `document.querySelector("#impulse-title img.seat-glyph") !== null`) {
		t.Error("no glyph in the impulse card")
	}
	if got := evalString(t, ctx, `document.getElementById("impulse-title").textContent`); got != "Alice" {
		t.Errorf("the impulse title says %q", got)
	}
	if !evalBool(t, ctx, `document.querySelector("#impulse-title").style.color !== ""`) {
		t.Error("the impulse title is not in the cult's colour")
	}
	// The first player's card shows the direction, with no words.
	if got := evalString(t, ctx, `document.getElementById("first-mark").textContent`); got != "↻" {
		t.Errorf("the first player's mark is %q", got)
	}
}

// The window of game options has a capitalised title, and never writes the word null.
func TestWebCthulhuWars_GameOptionsReadWell(t *testing.T) {
	ctx, cancel := moveQueueGame(t, 4)
	defer cancel()
	clickSel(t, ctx, "#open-options")
	waitJS(t, ctx, "the window", `!document.getElementById("game-options").hidden`, 5*time.Second)
	if got := evalString(t, ctx, `document.querySelector("#game-options h2").textContent`); got != "Game Options" {
		t.Errorf("the title is %q", got)
	}
	if got := evalString(t, ctx, `document.getElementById("game-options").innerText`); strings.Contains(got, "null") || strings.Contains(got, "undefined") {
		t.Errorf("the window says %q", got)
	}
}
