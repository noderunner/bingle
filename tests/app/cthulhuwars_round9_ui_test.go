package app_test

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/noderunner/bingle/internal/games/cthulhuwars"
)

// The ninth round: Desecrate rolls first and places its unit after, the die is
// final, the Yellow Sign card counts the Desecration tokens, and every card of a
// Great Old One lists its abilities.

// ysGame creates a custom position in which Alice plays Yellow Sign (seat 4) to
// move, with the others bots.
func ysGame(t *testing.T, baseURL, doc string) string {
	t.Helper()
	var created struct {
		RoomCode string `json:"room_code"`
	}
	body := map[string]any{
		"game_def_id": "cthulhu-wars", "player_name": "Alice", "seat": 4,
		"seat_bots":   map[string]string{"1": "random", "2": "random", "3": "random"},
		"start_state": json.RawMessage(doc), "start_seat": 4,
	}
	if code := doJSON(t, "POST", baseURL+"/api/games", body, &created); code != 200 {
		t.Fatalf("create: %d", code)
	}
	return created.RoomCode
}

func TestWebCthulhuWars_DesecrateRollsAtOnceThenPlacesAUnitAndCannotBeTakenBack(t *testing.T) {
	baseURL := newRESTTestServer(t)
	doc := cwPosition(t,
		[]cwUnit{{4, "king-in-yellow", "europe", 1}, {4, "undead", "europe", 2}, {1, "cultist", "south-pacific", 1}},
		map[string]int{"south-pacific": 1},
		[5]int{0, 3, 3, 3, 6}, [5]uint8{},
		func(d *cthulhuwars.Data) { d.Turn.Seat = 4; d.RNG = 4242; d.Desec = map[string]bool{"africa": true} })
	room := ysGame(t, baseURL, doc)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	var mu sync.Mutex
	var jsErrors []string
	collectJSErrors(ctx, &mu, &jsErrors)
	openGameAs(t, ctx, baseURL, room, "cthulhu-wars", "Alice")
	waitJS(t, ctx, "the Desecrate button", `document.getElementById("action-desecrate") !== null`, 15*time.Second)
	if got := evalString(t, ctx, `document.getElementById("action-desecrate").textContent`); got != "Desecrate (2)" {
		t.Errorf("button = %q", got)
	}
	// Its card: the roll, the token and the unit that is placed whatever the roll.
	hoverOver(t, ctx, "#action-desecrate")
	waitJS(t, ctx, "the card", `document.querySelector('.hover-card') !== null`, 5*time.Second)
	card := evalString(t, ctx, `document.querySelector('.hover-card').textContent`)
	for _, want := range []string{"roll 1 die", "Whether you succeed or fail", "cost of 2 or less", "cannot be taken back"} {
		if !strings.Contains(card, want) {
			t.Errorf("the Desecrate card lacks %q: %s", want, card)
		}
	}
	// One click rolls the die; no unit is asked about before it.
	clickSel(t, ctx, "#action-desecrate")
	waitJS(t, ctx, "the unit to place", `document.querySelector('#move-bar .piece-choice[data-piece="byakhee"]') !== null`, 15*time.Second)
	if got := evalString(t, ctx, `document.getElementById("move-hint").textContent`); got != "Choose the unit to place:" {
		t.Errorf("hint = %q", got)
	}
	for _, typ := range []string{"cultist", "undead", "byakhee"} {
		if n := evalInt(t, ctx, `document.querySelectorAll('#move-bar .piece-choice[data-piece="`+typ+`"]').length`); n != 1 {
			t.Errorf("%d buttons for %s", n, typ)
		}
	}
	// The die has been rolled: nothing offers to take it back.
	if n := evalInt(t, ctx, `document.querySelectorAll('#undo-move').length`); n != 0 {
		t.Errorf("an Undo button after the roll")
	}
	// The log has the die and what it was set against.
	waitJS(t, ctx, "the roll in the log", `document.body.innerText.indexOf("the die shows") >= 0`, 10*time.Second)
	// Place a Byakhee: it stands there, whatever the die said.
	clickSel(t, ctx, `#move-bar .piece-choice[data-piece="byakhee"]`)
	waitJS(t, ctx, "the Byakhee", `document.querySelector('#board .token[data-position="europe"][data-piece="byakhee"]') !== null`, 15*time.Second)
	// And the roll is still final.
	if n := evalInt(t, ctx, `document.querySelectorAll('#undo-move').length`); n != 0 {
		t.Errorf("an Undo button after the unit was placed")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(jsErrors) > 0 {
		t.Errorf("uncaught JS exceptions: %v", jsErrors)
	}
}

func TestWebCthulhuWars_TheYellowSignCardCountsTheDesecrationTokensFirst(t *testing.T) {
	baseURL := newRESTTestServer(t)
	doc := cwPosition(t,
		[]cwUnit{{4, "king-in-yellow", "europe", 1}, {1, "cultist", "south-pacific", 1}},
		map[string]int{"south-pacific": 1},
		[5]int{0, 3, 3, 3, 6}, [5]uint8{},
		func(d *cthulhuwars.Data) {
			d.Turn.Seat = 4
			d.Desec = map[string]bool{"africa": true, "asia": true, "europe": true}
		})
	room := ysGame(t, baseURL, doc)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	var mu sync.Mutex
	var jsErrors []string
	collectJSErrors(ctx, &mu, &jsErrors)
	openGameAs(t, ctx, baseURL, room, "cthulhu-wars", "Alice")
	waitJS(t, ctx, "the Yellow Sign card", `document.querySelector('#seat-4 [data-counter="desecrations"]') !== null`, 15*time.Second)
	// The icon and the number come before Power, Doom, Elder Signs and Gates.
	order := evalString(t, ctx, `[...document.querySelectorAll('#seat-4 .seat-stats .counter')].map(c => c.dataset.counter).join(",")`)
	if order != "desecrations,power,doom,signs,gates" {
		t.Errorf("the numbers come in the order %q", order)
	}
	if got := evalString(t, ctx, `document.querySelector('#seat-4 [data-counter="desecrations"]').textContent.trim()`); got != "3" {
		t.Errorf("tokens = %q", got)
	}
	if n := evalInt(t, ctx, `document.querySelectorAll('#seat-4 [data-counter="desecrations"] svg.stat-icon').length`); n != 1 {
		t.Errorf("%d icons", n)
	}
	// No other cult has it.
	if n := evalInt(t, ctx, `document.querySelectorAll('#seat-1 [data-counter="desecrations"], #seat-2 [data-counter="desecrations"], #seat-3 [data-counter="desecrations"]').length`); n != 0 {
		t.Errorf("%d token counters on other cults", n)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(jsErrors) > 0 {
		t.Errorf("uncaught JS exceptions: %v", jsErrors)
	}
}

func TestWebCthulhuWars_EveryGreatOldOnesCardListsItsAbilities(t *testing.T) {
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
	waitJS(t, ctx, "the cards", `document.querySelector('#seat-1 .pool-unit[data-piece="cthulhu"]') !== null`, 15*time.Second)
	// The pool icon of Cthulhu: Devour, with its timing.
	hoverOver(t, ctx, `#seat-1 .pool-unit[data-piece="cthulhu"]`)
	waitJS(t, ctx, "the card", `document.querySelector('.hover-card') !== null`, 5*time.Second)
	card := evalString(t, ctx, `document.querySelector('.hover-card').textContent`)
	for _, want := range []string{"Devour", "(Pre-Battle)", "eliminates one of his Monsters or Cultists", "How to awaken"} {
		if !strings.Contains(card, want) {
			t.Errorf("Cthulhu's card lacks %q: %s", want, card)
		}
	}
	// The rules window: the table of abilities, with the two that were missing.
	clickSel(t, ctx, "#show-rules")
	waitJS(t, ctx, "the rules", `document.querySelectorAll('#rules-dialog [role="tab"]').length > 0`, 10*time.Second)
	clickSel(t, ctx, `#rules-dialog [role="tab"][data-tab="actions"]`)
	waitJS(t, ctx, "the actions page", `document.getElementById("rules-panel-actions") !== null`, 5*time.Second)
	page := evalString(t, ctx, `document.getElementById("rules-panel-actions").textContent`)
	for _, want := range []string{
		"Devour", "The Harbinger", "Avatar", "Desecrate", "Vengeance", "Eternal", "Ferox", "Lethargy",
		"Whether you succeed or fail", "cost of 2 or less",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the Actions page lacks %q", want)
		}
	}
	clickSel(t, ctx, `#rules-dialog [role="tab"][data-tab="factions"]`)
	waitJS(t, ctx, "the factions page", `document.getElementById("rules-panel-factions") !== null`, 5*time.Second)
	cults := evalString(t, ctx, `document.getElementById("rules-panel-factions").textContent`)
	for _, want := range []string{"Vengeance", "Desecrate", "Eternal", "Ferox", "Lethargy", "Avatar", "The Harbinger", "Devour"} {
		if !strings.Contains(cults, want) {
			t.Errorf("the cults page lacks %q", want)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if len(jsErrors) > 0 {
		t.Errorf("uncaught JS exceptions: %v", jsErrors)
	}
}

// A move that rolled a die is marked in the move list (the page stops offering
// Undo) and the server refuses to take it back.
func TestCthulhuWarsARolledMoveIsLockedAndCannotBeUndone(t *testing.T) {
	baseURL := newRESTTestServer(t)
	doc := cwPosition(t,
		[]cwUnit{{4, "king-in-yellow", "europe", 1}, {4, "undead", "europe", 2}, {1, "cultist", "south-pacific", 1}},
		map[string]int{"south-pacific": 1},
		[5]int{0, 3, 3, 3, 6}, [5]uint8{},
		func(d *cthulhuwars.Data) { d.Turn.Seat = 4; d.RNG = 4242 })
	room := ysGame(t, baseURL, doc)
	var mv map[string]any
	if code := doJSON(t, "POST", baseURL+"/api/games/"+room+"/moves", map[string]any{"player_name": "Alice", "notation": "desecrate"}, &mv); code != 200 {
		t.Fatalf("desecrate: %d %v", code, mv)
	}
	if mv["move"].(map[string]any)["locked"] != true {
		t.Errorf("the move that rolled the die is not marked: %v", mv["move"])
	}
	var list []map[string]any
	if code := doJSON(t, "GET", baseURL+"/api/games/"+room+"/moves", nil, &list); code != 200 || len(list) == 0 {
		t.Fatalf("moves: %d", code)
	}
	if list[0]["locked"] != true {
		t.Errorf("the move list does not say it is locked: %v", list[0])
	}
	var refused map[string]any
	if code := doJSON(t, "POST", baseURL+"/api/games/"+room+"/undo", map[string]any{"player_name": "Alice"}, &refused); code != 409 {
		t.Errorf("undo across the roll: %d %v, want 409", code, refused)
	}
	// The unit is chosen on seeing the die: that is locked in as well.
	var placed map[string]any
	if code := doJSON(t, "POST", baseURL+"/api/games/"+room+"/moves", map[string]any{"player_name": "Alice", "notation": "desecrate:undead"}, &placed); code != 200 {
		t.Fatalf("place the unit: %d %v", code, placed)
	}
	if placed["move"].(map[string]any)["locked"] != true {
		t.Errorf("the choice after the roll is not locked: %v", placed["move"])
	}
	refused = nil
	if code := doJSON(t, "POST", baseURL+"/api/games/"+room+"/undo", map[string]any{"player_name": "Alice"}, &refused); code != 409 {
		t.Errorf("undo of the choice after the roll: %d %v, want 409", code, refused)
	}
	// A move that rolled nothing is not locked.
	doc2 := cwPosition(t,
		[]cwUnit{{4, "cultist", "europe", 1}, {1, "cultist", "south-pacific", 1}},
		map[string]int{"south-pacific": 1},
		[5]int{0, 3, 3, 3, 6}, [5]uint8{},
		func(d *cthulhuwars.Data) { d.Turn.Seat = 4 })
	room2 := ysGame(t, baseURL, doc2)
	var mv2 map[string]any
	if code := doJSON(t, "POST", baseURL+"/api/games/"+room2+"/moves", map[string]any{"player_name": "Alice", "notation": "recruit@europe"}, &mv2); code != 200 {
		t.Fatalf("recruit: %d %v", code, mv2)
	}
	if mv2["move"].(map[string]any)["locked"] == true {
		t.Errorf("a recruit rolled nothing: %v", mv2["move"])
	}
	var undone map[string]any
	if code := doJSON(t, "POST", baseURL+"/api/games/"+room2+"/undo", map[string]any{"player_name": "Alice"}, &undone); code != 200 {
		t.Errorf("undo of a recruit: %d %v", code, undone)
	}
}
