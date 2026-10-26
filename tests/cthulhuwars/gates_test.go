package cthulhuwars_test

import (
	"encoding/json"
	"testing"

	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/games/cthulhuwars"
)

// A Gate is occupied by one unit — an Acolyte, the High Priest or, with The
// Red Sign, a Dark Young — which is drawn on it, apart from the rest of its
// kind. The unit on a Gate gives it up by leaving (also to the area it stands
// in); any other unit of the area goes onto an abandoned Gate for free.

func TestTheGateOfAnAcolyteIsOccupiedByAnAcolyteAndTheViewSaysSo(t *testing.T) {
	tb := newTable(t, "cthulhu-wars", []string{"gc", "cc", "bg", "ys"}, true)
	st := tb.start(3)
	d := tb.data(st)
	if d.Holders["south-pacific"] != "cultist" {
		t.Fatalf("holders at the start: %v", d.Holders)
	}
	// The public view tells the client which figure to draw on each Gate.
	raw, err := json.Marshal(state.Public(tb.g, st, 1))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Ext struct {
			View struct {
				Holders map[string]string `json:"holders"`
			} `json:"view"`
		} `json:"ext"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	h := doc.Ext.View.Holders
	if h["south-pacific"] != "cultist" || h["asia"] != "cultist" || len(h) != 4 {
		t.Fatalf("view holders: %v", h)
	}
}

func TestAHighPriestThatBuildsAGateOccupiesIt(t *testing.T) {
	tb := newTable(t, "cthulhu-wars", []string{"gc", "cc", "bg", "ys"}, true)
	st := tb.build(xp{
		units: []xu{{"gc", "high-priest", "asia", 1}, {"cc", "cultist", "europe", 1}},
		power: map[string]int{"gc": 5}, turn: "gc",
	})
	st = tb.as(st, "gc", "gate@asia")
	d := tb.data(st)
	if d.Gates["asia"] != 1 || d.Holders["asia"] != "high-priest" {
		t.Fatalf("gates %v holders %v", d.Gates, d.Holders)
	}
}

func TestMovingAUnitThatDoesNotOccupyTheGateLeavesTheGateAlone(t *testing.T) {
	tb := newTable(t, "cthulhu-wars", []string{"gc", "cc", "bg", "ys"}, true)
	st := tb.build(xp{
		units: []xu{{"gc", "cultist", "south-pacific", 3}, {"cc", "cultist", "asia", 1}},
		gates: map[string]string{"south-pacific": "gc"}, power: map[string]int{"gc": 5}, turn: "gc",
		edit: func(d *cthulhuwars.Data) { d.Holders = map[string]string{"south-pacific": "cultist"} },
	})
	// Two of the three stand beside the Gate: they move with go.
	got := tb.can(st, "gc")
	if !has(got, "go:cultist@south-pacific>antarctica") || !has(got, "leave:cultist@south-pacific>antarctica") {
		t.Fatalf("moves: %v", got)
	}
	st = tb.as(st, "gc", "go:cultist@south-pacific>antarctica")
	d := tb.data(st)
	if d.Gates["south-pacific"] != 1 || d.Holders["south-pacific"] != "cultist" {
		t.Errorf("the Gate fell: gates %v holders %v", d.Gates, d.Holders)
	}
	if n := tb.n(st, "gc", "cultist", "antarctica"); n != 1 {
		t.Errorf("the Cultist did not move")
	}
	// Only one stands beside the Gate now: the stack moves once more, and the
	// one on the Gate may still leave.
	st = tb.as(st, "gc", "go:cultist@south-pacific>indian-ocean")
	if d := tb.data(st); d.Gates["south-pacific"] != 1 {
		t.Errorf("the Gate fell: %v", d.Gates)
	}
	got = tb.can(st, "gc")
	if hasPrefix(got, "go:cultist@south-pacific") || !has(got, "leave:cultist@south-pacific>south-pacific") {
		t.Errorf("only the one on the Gate is left to move: %v", got)
	}
}

func TestTheUnitOnTheGateLeavesAndAbandonsItEvenWithCompany(t *testing.T) {
	tb := newTable(t, "cthulhu-wars", []string{"gc", "cc", "bg", "ys"}, true)
	build := func() *state.BoardState {
		return tb.build(xp{
			units: []xu{{"gc", "cultist", "south-pacific", 3}, {"gc", "high-priest", "south-pacific", 1}, {"cc", "cultist", "asia", 1}},
			gates: map[string]string{"south-pacific": "gc"}, power: map[string]int{"gc": 5}, turn: "gc",
			edit: func(d *cthulhuwars.Data) { d.Holders = map[string]string{"south-pacific": "cultist"} },
		})
	}
	got := tb.can(build(), "gc")
	// An Acolyte stands on it: only an Acolyte leaves the Gate; the High
	// Priest is a unit like the others, and nobody goes by "go" to its own area.
	if !has(got, "leave:cultist@south-pacific>antarctica") || !has(got, "leave:cultist@south-pacific>south-pacific") ||
		hasPrefix(got, "leave:high-priest") || hasPrefix(got, "go:cultist@south-pacific>south-pacific") ||
		!has(got, "go:high-priest@south-pacific>antarctica") {
		t.Fatalf("moves: %v", got)
	}
	// It goes to another area: the Gate is abandoned although two Acolytes and
	// the High Priest stay.
	st := tb.as(build(), "gc", "leave:cultist@south-pacific>antarctica")
	d := tb.data(st)
	if d.Gates["south-pacific"] != 0 || d.Holders["south-pacific"] != "" {
		t.Fatalf("the Gate is still held: gates %v holders %v", d.Gates, d.Holders)
	}
	if tb.n(st, "gc", "cultist", "south-pacific") != 2 || tb.n(st, "gc", "cultist", "antarctica") != 1 || tb.n(st, "gc", "high-priest", "south-pacific") != 1 {
		t.Errorf("units: %d %d %d", tb.n(st, "gc", "cultist", "south-pacific"), tb.n(st, "gc", "cultist", "antarctica"), tb.n(st, "gc", "high-priest", "south-pacific"))
	}
	if p := tb.power(st, "gc"); p != 4 {
		t.Errorf("Power %d, want 4", p)
	}
	// The Move action goes on: the others are plain units now.
	if got := tb.can(st, "gc"); !has(got, "go:cultist@south-pacific>indian-ocean") || !has(got, "done") {
		t.Errorf("the others are free to move: %v", got)
	}
	// Nothing takes the Gate back by itself.
	st = tb.as(st, "gc", "done")
	if d := tb.data(st); d.Gates["south-pacific"] != 0 {
		t.Errorf("the Gate was taken by itself: %v", d.Gates)
	}
}

func TestSteppingOffTheGateIsFreeNoMoveAndOnceInATurn(t *testing.T) {
	tb := newTable(t, "cthulhu-wars", []string{"gc", "cc", "bg", "ys"}, true)
	st := tb.build(xp{
		units: []xu{{"gc", "cultist", "south-pacific", 2}, {"cc", "cultist", "asia", 1}},
		gates: map[string]string{"south-pacific": "gc"}, power: map[string]int{"gc": 5}, turn: "gc",
		edit: func(d *cthulhuwars.Data) { d.Holders = map[string]string{"south-pacific": "cultist"} },
	})
	st = tb.as(st, "gc", "leave:cultist@south-pacific>south-pacific")
	d := tb.data(st)
	if d.Gates["south-pacific"] != 0 || tb.n(st, "gc", "cultist", "south-pacific") != 2 || tb.power(st, "gc") != 5 {
		t.Fatalf("gates %v units %d power %d", d.Gates, tb.n(st, "gc", "cultist", "south-pacific"), tb.power(st, "gc"))
	}
	// It is no Move and no action: nothing is open, nothing to finish.
	if d.Stage != "turn" || d.Move != nil || d.Turn.Main {
		t.Errorf("stage %q move %+v main %v", d.Stage, d.Move, d.Turn.Main)
	}
	got := tb.can(st, "gc")
	if has(got, "done") || hasPrefix(got, "leave:cultist@south-pacific>south-pacific") || !has(got, "go:cultist@south-pacific>antarctica") {
		t.Errorf("moves: %v", got)
	}
	// Put it back (free) and the Gate still cannot be stepped off again this turn.
	st = tb.as(st, "gc", "control:cultist@south-pacific")
	if got := tb.can(st, "gc"); hasPrefix(got, "leave:cultist@south-pacific>south-pacific") {
		t.Errorf("a Gate changes hands once in a turn: %v", got)
	}
	// Moving it away is a Move step: it costs 1 Power and opens the Move.
	st = tb.as(st, "gc", "leave:cultist@south-pacific>antarctica")
	if d := tb.data(st); d.Stage != "moving" || tb.power(st, "gc") != 4 {
		t.Errorf("stage %q power %d", d.Stage, tb.power(st, "gc"))
	}
}

func TestAUnitBesideAnAbandonedGateGoesOnItForFreeInTheSameTurn(t *testing.T) {
	tb := newTable(t, "cthulhu-wars", []string{"gc", "cc", "bg", "ys"}, true)
	st := tb.build(xp{
		units: []xu{{"gc", "cultist", "south-pacific", 2}, {"gc", "high-priest", "south-pacific", 1}, {"cc", "cultist", "asia", 1}},
		gates: map[string]string{"south-pacific": "gc"}, power: map[string]int{"gc": 5}, turn: "gc",
		edit: func(d *cthulhuwars.Data) { d.Holders = map[string]string{"south-pacific": "cultist"} },
	})
	// Step off: free, and the turn is as open as before.
	st = tb.as(st, "gc", "leave:cultist@south-pacific>south-pacific")
	d := tb.data(st)
	if d.Turn.Seat != 1 || d.Stage != "turn" || d.Turn.Main {
		t.Fatalf("seat %d stage %q main %v", d.Turn.Seat, d.Stage, d.Turn.Main)
	}
	got := tb.can(st, "gc")
	if !has(got, "control:cultist@south-pacific") || !has(got, "control:high-priest@south-pacific") {
		t.Fatalf("moves: %v", got)
	}
	// The High Priest goes on the Gate: free, and the turn goes on.
	before := tb.power(st, "gc")
	st = tb.as(st, "gc", "control:high-priest@south-pacific")
	d = tb.data(st)
	if d.Gates["south-pacific"] != 1 || d.Holders["south-pacific"] != "high-priest" || tb.power(st, "gc") != before {
		t.Fatalf("gates %v holders %v power %d (was %d)", d.Gates, d.Holders, tb.power(st, "gc"), before)
	}
	// The Gate has changed hands this turn: not again before the next one.
	got = tb.can(st, "gc")
	if hasPrefix(got, "control") || hasPrefix(got, "leave:high-priest@south-pacific>south-pacific") {
		t.Fatalf("moves: %v", got)
	}
	// The Gate is controlled: the plain notation of old is gone.
	tb.refuse(st, "gc", "control@south-pacific")
	tb.refuse(st, "gc", "abandon@south-pacific")
}

func TestPuttingAUnitOnAnAbandonedGateIsFreeBeforeTheMainAction(t *testing.T) {
	tb := newTable(t, "cthulhu-wars", []string{"gc", "cc", "bg", "ys"}, true)
	st := tb.build(xp{
		units: []xu{{"gc", "cultist", "australia", 1}, {"cc", "cultist", "asia", 1}},
		gates: map[string]string{"australia": "-"}, power: map[string]int{"gc": 5}, turn: "gc",
	})
	st = tb.as(st, "gc", "control:cultist@australia")
	d := tb.data(st)
	if d.Gates["australia"] != 1 || d.Holders["australia"] != "cultist" || tb.power(st, "gc") != 5 || d.Turn.Main {
		t.Fatalf("gates %v holders %v power %d main %v", d.Gates, d.Holders, tb.power(st, "gc"), d.Turn.Main)
	}
	// A cult with no Cultist there cannot, and monsters do not control Gates.
	q := tb.build(xp{
		units: []xu{{"gc", "deep-one", "australia", 1}, {"cc", "cultist", "asia", 1}},
		gates: map[string]string{"australia": "-"}, power: map[string]int{"gc": 5}, turn: "gc",
	})
	if got := tb.can(q, "gc"); hasPrefix(got, "control") {
		t.Errorf("monsters do not control Gates: %v", got)
	}
	// Nor an enemy's Gate.
	r := tb.build(xp{
		units: []xu{{"gc", "cultist", "australia", 1}, {"cc", "cultist", "australia", 1}},
		gates: map[string]string{"australia": "cc"}, power: map[string]int{"gc": 5}, turn: "gc",
	})
	if got := tb.can(r, "gc"); hasPrefix(got, "control") {
		t.Errorf("an enemy's Gate is not taken like that: %v", got)
	}
}

func TestOnlyAnAbandonedGateKeepsTheTurnOpen(t *testing.T) {
	tb := newTable(t, "cthulhu-wars", []string{"gc", "cc", "bg", "ys"}, true)
	st := tb.build(xp{
		units: []xu{{"gc", "cultist", "south-pacific", 1}, {"gc", "high-priest", "south-pacific", 1}, {"cc", "cultist", "asia", 1}},
		gates: map[string]string{"south-pacific": "gc"}, power: map[string]int{"gc": 5}, turn: "gc",
		edit: func(d *cthulhuwars.Data) { d.Holders = map[string]string{"south-pacific": "cultist"} },
	})
	// Swapping the unit on a Gate of one's own is only offered, never waited for:
	// after the main action the turn ends by itself.
	if got := tb.can(st, "gc"); !has(got, "control:high-priest@south-pacific") {
		t.Fatalf("the swap is not offered: %v", got)
	}
	st = tb.as(st, "gc", "recruit@south-pacific")
	if d := tb.data(st); d.Turn.Seat == 1 && d.Stage == "turn" && d.Turn.Main {
		t.Errorf("the turn waits for a swap")
	}
}

func TestADarkYoungWithTheRedSignOccupiesAGateNoAcolyteHolds(t *testing.T) {
	tb := newTable(t, "cthulhu-wars", []string{"gc", "cc", "bg", "ys"}, false)
	st := tb.build(xp{
		units: []xu{{"bg", "dark-young", "africa", 1}, {"gc", "cultist", "asia", 1}},
		power: map[string]int{"bg": 5}, turn: "bg", books: map[string][]int{"bg": {3}},
	})
	st = tb.as(st, "bg", "gate@africa")
	d := tb.data(st)
	if d.Gates["africa"] != 3 || d.Holders["africa"] != "dark-young" {
		t.Fatalf("gates %v holders %v", d.Gates, d.Holders)
	}
	// The Dark Young leaves like any occupant (on a turn of its own).
	st = tb.build(xp{
		units: []xu{{"bg", "dark-young", "africa", 1}, {"gc", "cultist", "asia", 1}},
		gates: map[string]string{"africa": "bg"}, power: map[string]int{"bg": 5}, turn: "bg", books: map[string][]int{"bg": {3}},
		edit: func(d *cthulhuwars.Data) { d.Holders = map[string]string{"africa": "dark-young"} },
	})
	if got := tb.can(st, "bg"); !has(got, "leave:dark-young@africa>africa") {
		t.Fatalf("no step-off: %v", got)
	}
	st = tb.as(st, "bg", "leave:dark-young@africa>africa")
	if d := tb.data(st); d.Gates["africa"] != 0 {
		t.Errorf("gates %v", d.Gates)
	}
}

func TestACultistArrivingAtAnAbandonedGateOccupiesIt(t *testing.T) {
	tb := newTable(t, "cthulhu-wars", []string{"gc", "cc", "bg", "ys"}, true)
	st := tb.build(xp{
		units: []xu{{"gc", "high-priest", "south-pacific", 1}, {"gc", "cultist", "antarctica", 1}, {"cc", "cultist", "asia", 1}},
		gates: map[string]string{"south-pacific": "-"}, power: map[string]int{"gc": 5}, turn: "gc",
	})
	// The abandoned Gate: a Cultist walks in and holds it; the High Priest
	// already there could have been put on it for free.
	st = tb.as(st, "gc", "go:cultist@antarctica>south-pacific")
	d := tb.data(st)
	if d.Gates["south-pacific"] != 1 || d.Holders["south-pacific"] != "cultist" {
		t.Fatalf("gates %v holders %v", d.Gates, d.Holders)
	}
}

func TestCapturingTheCultistOnTheGateLeavesTheGateAbandoned(t *testing.T) {
	tb := newTable(t, "cthulhu-wars", []string{"gc", "cc", "bg", "ys"}, true)
	st := tb.build(xp{
		units: []xu{{"gc", "cultist", "south-pacific", 1}, {"gc", "high-priest", "south-pacific", 1}, {"cc", "nightgaunt", "south-pacific", 1}},
		gates: map[string]string{"south-pacific": "gc"}, power: map[string]int{"cc": 5}, turn: "cc",
		edit: func(d *cthulhuwars.Data) { d.Holders = map[string]string{"south-pacific": "cultist"} },
	})
	// The only Cultist is the one on the Gate: no choice, and the Gate stays
	// open even though the High Priest could have held it — the owner puts
	// him there on its own turn. The Nightgaunt cannot hold a Gate.
	got := tb.can(st, "cc")
	if !has(got, "capture@south-pacific/gc") || hasPrefix(got, "capture:gate") {
		t.Fatalf("one Cultist, no choice: %v", got)
	}
	st = tb.as(st, "cc", "capture@south-pacific/gc")
	d := tb.data(st)
	if d.Gates["south-pacific"] != 0 || d.Holders["south-pacific"] != "" {
		t.Fatalf("gates %v holders %v", d.Gates, d.Holders)
	}
	if d.Captured[2][1] != 1 {
		t.Fatalf("captured %v", d.Captured)
	}
}

func TestTheCaptorChoosesBetweenTheCultistOnTheGateAndOneBesideIt(t *testing.T) {
	tb := newTable(t, "cthulhu-wars", []string{"gc", "cc", "bg", "ys"}, true)
	build := func() *state.BoardState {
		return tb.build(xp{
			units: []xu{{"gc", "cultist", "south-pacific", 2}, {"cc", "nightgaunt", "south-pacific", 1}, {"cc", "cultist", "south-pacific", 1}},
			gates: map[string]string{"south-pacific": "gc"}, power: map[string]int{"cc": 5}, turn: "cc",
			edit: func(d *cthulhuwars.Data) { d.Holders = map[string]string{"south-pacific": "cultist"} },
		})
	}
	st := build()
	got := tb.can(st, "cc")
	if !has(got, "capture:cultist@south-pacific/gc") || !has(got, "capture:gate:cultist@south-pacific/gc") || has(got, "capture@south-pacific/gc") {
		t.Fatalf("the choice is between the stack and the Gate: %v", got)
	}
	// One beside the Gate: the Gate stays with its owner.
	beside := tb.as(st, "cc", "capture:cultist@south-pacific/gc")
	if d := tb.data(beside); d.Gates["south-pacific"] != 1 || d.Holders["south-pacific"] != "cultist" || d.Captured[2][1] != 1 {
		t.Fatalf("stack: gates %v holders %v captured %v", d.Gates, d.Holders, d.Captured)
	}
	// The one on the Gate: it stays abandoned for its owner, who still has a
	// Cultist there, and the captor's own Cultist takes it at once.
	onIt := tb.as(build(), "cc", "capture:gate:cultist@south-pacific/gc")
	d := tb.data(onIt)
	if d.Gates["south-pacific"] != 2 || d.Holders["south-pacific"] != "cultist" || d.Captured[2][1] != 1 {
		t.Fatalf("gate: gates %v holders %v captured %v", d.Gates, d.Holders, d.Captured)
	}
	if n := count(onIt, 1, "cultist", "south-pacific"); n != 1 {
		t.Fatalf("the owner keeps the other Cultist there, has %d", n)
	}
	// A captor with no Cultist there leaves the Gate abandoned, and the owner
	// can put the Cultist it kept on it again in its own turn.
	alone := tb.as(tb.build(xp{
		units: []xu{{"gc", "cultist", "south-pacific", 2}, {"cc", "nightgaunt", "south-pacific", 1}},
		gates: map[string]string{"south-pacific": "gc"}, power: map[string]int{"cc": 5}, turn: "cc",
		edit: func(d *cthulhuwars.Data) { d.Holders = map[string]string{"south-pacific": "cultist"} },
	}), "cc", "capture:gate:cultist@south-pacific/gc")
	if d := tb.data(alone); d.Gates["south-pacific"] != 0 {
		t.Fatalf("an abandoned Gate: gates %v", d.Gates)
	}
}

func TestAUnitGoesOnTheGateInTheMiddleOfAMoveAndThePlayerNeedNotPressDoneFirst(t *testing.T) {
	tb := newTable(t, "cthulhu-wars", []string{"gc", "cc", "bg", "ys"}, true)
	st := tb.build(xp{
		units: []xu{{"gc", "cultist", "south-pacific", 1}, {"gc", "high-priest", "south-pacific", 1}, {"cc", "cultist", "asia", 1}},
		gates: map[string]string{"south-pacific": "gc"}, power: map[string]int{"gc": 5}, turn: "gc",
		edit: func(d *cthulhuwars.Data) { d.Holders = map[string]string{"south-pacific": "cultist"} },
	})
	// The Acolyte walks away; the Move is still open (the High Priest may go
	// too), and the free action is on offer.
	st = tb.as(st, "gc", "leave:cultist@south-pacific>antarctica")
	if d := tb.data(st); d.Stage != "moving" || d.Gates["south-pacific"] != 0 {
		t.Fatalf("stage %q gates %v", d.Stage, d.Gates)
	}
	got := tb.can(st, "gc")
	if !has(got, "control:high-priest@south-pacific") || !has(got, "done") {
		t.Fatalf("moves: %v", got)
	}
	// The High Priest goes onto the Gate, in the middle of the Move.
	st = tb.as(st, "gc", "control:high-priest@south-pacific")
	d := tb.data(st)
	if d.Gates["south-pacific"] != 1 || d.Holders["south-pacific"] != "high-priest" || tb.power(st, "gc") != 4 {
		t.Fatalf("gates %v holders %v power %d", d.Gates, d.Holders, tb.power(st, "gc"))
	}
	// The Move action goes on: the High Priest, now on the Gate, may still
	// leave it — and no swap is offered in the middle of a Move.
	got = tb.can(st, "gc")
	if hasPrefix(got, "control") || !has(got, "leave:high-priest@south-pacific>antarctica") || !has(got, "done") {
		t.Errorf("moves: %v", got)
	}
}

func TestStepOffsAndSwapsAreNotMadeForever(t *testing.T) {
	tb := newTable(t, "cthulhu-wars", []string{"gc", "cc", "bg", "ys"}, true)
	st := tb.build(xp{
		units: []xu{{"gc", "cultist", "south-pacific", 1}, {"gc", "high-priest", "south-pacific", 1}, {"cc", "cultist", "asia", 1}},
		gates: map[string]string{"south-pacific": "gc"}, power: map[string]int{"gc": 5}, turn: "gc",
		edit: func(d *cthulhuwars.Data) { d.Holders = map[string]string{"south-pacific": "cultist"} },
	})
	// One swap: after it the Gate has changed hands this turn.
	st = tb.as(st, "gc", "control:high-priest@south-pacific")
	got := tb.can(st, "gc")
	if hasPrefix(got, "control") || hasPrefix(got, "leave:high-priest@south-pacific>south-pacific") {
		t.Errorf("the Gate changed hands once already: %v", got)
	}
	// A new turn, a new chance (a round of passes brings it back).
	st2 := tb.build(xp{
		units: []xu{{"gc", "cultist", "south-pacific", 1}, {"gc", "high-priest", "south-pacific", 1}, {"cc", "cultist", "asia", 1}},
		gates: map[string]string{"south-pacific": "gc"}, power: map[string]int{"gc": 5}, turn: "gc",
		edit: func(d *cthulhuwars.Data) { d.Holders = map[string]string{"south-pacific": "high-priest"} },
	})
	if got := tb.can(st2, "gc"); !has(got, "control:cultist@south-pacific") || !has(got, "leave:high-priest@south-pacific>south-pacific") {
		t.Errorf("moves: %v", got)
	}
}
