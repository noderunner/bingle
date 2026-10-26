package cthulhuwars_test

import (
	"testing"

	"github.com/noderunner/bingle/internal/engine/state"
)

func TestStartPutsAGateAndSixCultistsInEachStartArea(t *testing.T) {
	g := game(t)
	st := state.InitialState(g)
	d := data(t, st)
	starts := map[int]string{1: "south-pacific", 2: "asia", 3: "south-america-west", 4: "europe"}
	for seat, area := range starts {
		if d.Gates[area] != seat {
			t.Errorf("seat %d should control the Gate in %s: %v", seat, area, d.Gates)
		}
		if n := count(st, seat, "cultist", area); n != 6 {
			t.Errorf("seat %d has %d Cultists in %s, want 6", seat, n, area)
		}
		if d.Power[seat] != 8 || d.Doom[seat] != 0 {
			t.Errorf("seat %d power %d doom %d", seat, d.Power[seat], d.Doom[seat])
		}
	}
	if d.First != 1 || d.Phase != "action" {
		t.Errorf("first %d phase %q", d.First, d.Phase)
	}
	if err := state.Validate(g, st); err != nil {
		t.Fatal(err)
	}
	if !state.NeedsStartRecord(g) {
		t.Error("the start carries a seed and must be recorded")
	}
}

func TestRecruitCostsOneAndNeedsAUnitInTheArea(t *testing.T) {
	g := game(t)
	p := pos{units: []u{{gc, "cultist", "south-pacific", 1}}, gates: map[string]int{"south-pacific": gc}, power: powers(8, 8, 8, 8)}
	st := p.build()
	got := notations(g, st, 1)
	if !has(got, "recruit@south-pacific") || hasPrefix(got, "recruit@asia") {
		t.Fatalf("recruit options = %v", got)
	}
	st = play(t, g, st, 1, "recruit@south-pacific")
	d := data(t, st)
	if d.Power[1] != 7 || count(st, 1, "cultist", "south-pacific") != 2 {
		t.Fatalf("power %d cultists %d", d.Power[1], count(st, 1, "cultist", "south-pacific"))
	}
	// A cult with no units on the map may recruit anywhere.
	p = pos{power: powers(8, 8, 8, 8)}
	if got := notations(g, p.build(), 1); !has(got, "recruit@asia") || !has(got, "recruit@antarctica") {
		t.Fatalf("a cult with no units may recruit anywhere: %v", got)
	}
}

func TestMoveCostsOnePerUnitAndEachUnitMovesOnce(t *testing.T) {
	g := game(t)
	p := pos{units: []u{{gc, "cultist", "south-pacific", 2}}, gates: map[string]int{"south-pacific": gc}, power: powers(8, 8, 8, 8)}
	st := p.build()
	// Moving a unit is itself the first step of a Move action: there is no
	// separate "begin moving" decision to idle in.
	got := notations(g, st, 1)
	for _, want := range []string{"go:cultist@south-pacific>new-zealand", "go:cultist@south-pacific>north-pacific", "go:cultist@south-pacific>south-america-west"} {
		if !has(got, want) {
			t.Errorf("missing %s in %v", want, got)
		}
	}
	if has(got, "done") || has(got, "move") {
		t.Errorf("a turn cannot be \"done\" before anything moved: %v", got)
	}
	if hasPrefix(got, "go:cultist@south-pacific>asia") {
		t.Errorf("only adjacent areas are reachable: %v", got)
	}
	st = play(t, g, st, 1, "go:cultist@south-pacific>new-zealand")
	d := data(t, st)
	if d.Stage != "moving" || d.Power[1] != 7 {
		t.Fatalf("stage %q power %d", d.Stage, d.Power[1])
	}
	// The unit that moved may not move again; the other still can.
	got = notations(g, st, 1)
	if hasPrefix(got, "go:cultist@new-zealand") {
		t.Errorf("a unit moves once per action: %v", got)
	}
	// The Cultist that is left stands on the Gate: it leaves with "leave".
	if !hasPrefix(got, "leave:cultist@south-pacific") || hasPrefix(got, "go:cultist@south-pacific") || !has(got, "done") {
		t.Errorf("the other Cultist can still move, or we are done: %v", got)
	}
	st = play(t, g, st, 1, "done")
	d = data(t, st)
	// Only giving a Gate up remains as a free action, which never keeps a
	// turn open: the Move was the turn's action and play passes on.
	if d.Stage != "turn" || d.Turn.Seat != 2 || d.Turn.Main {
		t.Fatalf("after the Move action: %+v stage %q", d.Turn, d.Stage)
	}
	// The gate lost one of its two Cultists but is still controlled.
	if d.Gates["south-pacific"] != 1 {
		t.Errorf("the Gate should still be controlled: %v", d.Gates)
	}
}

func TestLeavingAGateAloneAbandonsIt(t *testing.T) {
	g := game(t)
	p := pos{units: []u{{gc, "cultist", "south-pacific", 1}}, gates: map[string]int{"south-pacific": gc}, power: powers(8, 8, 8, 8)}
	// The only unit has moved, so the Move action ends by itself.
	// It stands on the Gate, so it leaves it: "leave", not "go".
	if got := notations(g, p.build(), 1); hasPrefix(got, "go:cultist@south-pacific") || !has(got, "leave:cultist@south-pacific>new-zealand") {
		t.Fatalf("the unit on the Gate leaves with leave: %v", got)
	}
	st := play(t, g, p.build(), 1, "leave:cultist@south-pacific>new-zealand")
	if d := data(t, st); d.Gates["south-pacific"] != 0 {
		t.Fatalf("the Gate should be abandoned: %v", d.Gates)
	}
}

func TestFlightLetsCrawlingChaosMoveTwoAreas(t *testing.T) {
	g := game(t)
	p := pos{units: []u{{cc, "cultist", "asia", 1}}, gates: map[string]int{"asia": cc}, power: powers(8, 8, 8, 8), turn: 2}
	st := p.build()
	got := notations(g, st, 2)
	if !has(got, "leave:cultist@asia>australia") || !has(got, "leave:cultist@asia>africa") || !has(got, "leave:cultist@asia>new-zealand") {
		t.Fatalf("Flight should reach two areas away: %v", got)
	}
	if hasPrefix(got, "leave:cultist@asia>antarctica") {
		// asia → indian-ocean → antarctica is two steps.
	} else {
		t.Errorf("antarctica is two steps away: %v", got)
	}
	// Great Cthulhu cannot.
	q := pos{units: []u{{gc, "cultist", "asia", 1}}, power: powers(8, 8, 8, 8)}
	st = q.build()
	if hasPrefix(notations(g, st, 1), "go:cultist@asia>australia") {
		t.Error("only Crawling Chaos flies")
	}
}

func TestMoveRunsOutWithPower(t *testing.T) {
	g := game(t)
	p := pos{units: []u{{gc, "cultist", "south-pacific", 3}}, gates: map[string]int{"south-pacific": gc}, power: powers(2, 8, 8, 8)}
	st := play(t, g, p.build(), 1, "go:cultist@south-pacific>new-zealand")
	st = play(t, g, st, 1, "go:cultist@south-pacific>new-zealand")
	d := data(t, st)
	if d.Power[1] != 0 {
		t.Fatalf("power %d", d.Power[1])
	}
	if d.Turn.Seat == 1 {
		t.Fatalf("a player at 0 Power is done: %+v", d.Turn)
	}
}

func TestCreateGateCostsThreeAndNeedsACultistWithNoGate(t *testing.T) {
	g := game(t)
	p := pos{units: []u{{bg, "cultist", "africa", 1}, {bg, "cultist", "asia", 1}}, gates: map[string]int{"asia": bg}, power: powers(8, 8, 8, 8), turn: 3}
	st := p.build()
	got := notations(g, st, 3)
	if !has(got, "gate@africa") {
		t.Fatalf("a Cultist may build a Gate: %v", got)
	}
	if hasPrefix(got, "gate@asia") {
		t.Fatalf("one Gate per area: %v", got)
	}
	st = play(t, g, st, 3, "gate@africa")
	d := data(t, st)
	if d.Power[3] != 5 || d.Gates["africa"] != 3 {
		t.Fatalf("power %d gates %v", d.Power[3], d.Gates)
	}
	// With 2 Power it cannot.
	q := pos{units: []u{{bg, "cultist", "africa", 1}}, power: powers(8, 8, 2, 8), turn: 3}
	if hasPrefix(notations(g, q.build(), 3), "gate@") {
		t.Fatal("a Gate costs 3 Power")
	}
}

func TestSummonNeedsAControlledGateAndPaysTheCost(t *testing.T) {
	g := game(t)
	p := pos{units: []u{{gc, "cultist", "south-pacific", 1}, {gc, "cultist", "australia", 1}}, gates: map[string]int{"south-pacific": gc, "australia": 0}, power: powers(8, 8, 8, 8)}
	st := p.build()
	got := notations(g, st, 1)
	if !has(got, "summon:starspawn@south-pacific") || hasPrefix(got, "summon:deep-one@australia") {
		t.Fatalf("summon options = %v", got)
	}
	st = play(t, g, st, 1, "summon:starspawn@south-pacific")
	d := data(t, st)
	if d.Power[1] != 5 || count(st, 1, "starspawn", "south-pacific") != 1 {
		t.Fatalf("power %d", d.Power[1])
	}
	// A turn's main action is done; only unlimited actions remain.
	if !d.Turn.Main {
		t.Fatal("summoning is the main action")
	}
	if hasPrefix(notations(g, st, 1), "recruit") {
		t.Fatal("no second main action")
	}
}

func TestBlackGoatSummonsAsAnUnlimitedAction(t *testing.T) {
	g := game(t)
	p := pos{units: []u{{bg, "cultist", "south-america-west", 1}}, gates: map[string]int{"south-america-west": bg}, power: powers(8, 8, 8, 8), turn: 3}
	st := p.build()
	st = play(t, g, st, 3, "summon:ghoul@south-america-west")
	d := data(t, st)
	if d.Turn.Main {
		t.Fatal("Fertility Cult: summoning does not use the main action")
	}
	st = play(t, g, st, 3, "summon:fungi@south-america-west")
	if !has(notations(g, st, 3), "recruit@south-america-west") {
		t.Fatal("the main action should still be available")
	}
}

func TestBlackGoatMayEndTheTurnWithASingleSummon(t *testing.T) {
	g := game(t)
	p := pos{units: []u{{bg, "cultist", "south-america-west", 1}}, gates: map[string]int{"south-america-west": bg}, power: powers(8, 8, 8, 8), turn: 3}
	st := p.build()
	// Before any action only Pass ends the turn (and it drops the Power).
	got := notations(g, st, 3)
	if !has(got, "pass") || has(got, "done") {
		t.Fatalf("at the start of the turn: %v", got)
	}
	// One Ghoul, summoned with the unlimited action: Done is offered at once,
	// beside more summons and the main actions.
	st = play(t, g, st, 3, "summon:ghoul@south-america-west")
	got = notations(g, st, 3)
	if !has(got, "done") || has(got, "pass") {
		t.Fatalf("after one summon the player may press Done, not Pass: %v", got)
	}
	if !hasPrefix(got, "summon:") || !hasPrefix(got, "recruit@") {
		t.Fatalf("more summons and the main action stay open: %v", got)
	}
	powerBefore := data(t, st).Power[3]
	st = play(t, g, st, 3, "done")
	d := data(t, st)
	if d.Power[3] != powerBefore {
		t.Errorf("Done keeps the Power: %d, was %d", d.Power[3], powerBefore)
	}
	// ... and play passes to the next player.
	if d.Turn == nil || d.Turn.Seat == 3 {
		t.Fatalf("the turn did not pass on: %+v", d.Turn)
	}
	if n := count(st, 3, "ghoul", "south-america-west"); n != 1 {
		t.Errorf("%d Ghouls, want the single one", n)
	}
}

func TestBlackGoatWithNothingLeftToDoAfterASummonEndsTheTurn(t *testing.T) {
	g := game(t)
	// One Power buys the Ghoul and nothing else is possible afterwards.
	p := pos{units: []u{{bg, "cultist", "south-america-west", 1}}, gates: map[string]int{"south-america-west": bg}, power: powers(8, 8, 1, 8), turn: 3}
	st := play(t, g, p.build(), 3, "summon:ghoul@south-america-west")
	if d := data(t, st); d.Turn == nil || d.Turn.Seat == 3 {
		t.Fatalf("with no Power left the turn is over: %+v", d.Turn)
	}
}

func TestAwakenCthulhuNeedsAControlledGateInAnOcean(t *testing.T) {
	g := game(t)
	p := pos{units: []u{{gc, "cultist", "south-pacific", 1}, {gc, "cultist", "south-america-west", 1}}, gates: map[string]int{"south-pacific": gc, "south-america-west": gc}, power: powers(10, 8, 8, 8)}
	st := p.build()
	got := notations(g, st, 1)
	if !has(got, "awaken:cthulhu@south-pacific") || hasPrefix(got, "awaken:cthulhu@south-america-west") {
		t.Fatalf("awaken options = %v", got)
	}
	st = play(t, g, st, 1, "awaken:cthulhu@south-pacific")
	d := data(t, st)
	if d.Power[1] != 0 || count(st, 1, "cthulhu", "south-pacific") != 1 {
		t.Fatalf("power %d", d.Power[1])
	}
	// Awakening earned a spellbook requirement: choose one of six.
	if d.Stage != "prompt" {
		t.Fatalf("stage %q, want a spellbook choice", d.Stage)
	}
	got = notations(g, st, 1)
	if len(got) != 6 || !has(got, "book:absorb") || !has(got, "book:yha") {
		t.Fatalf("books = %v", got)
	}
	st = play(t, g, st, 1, "book:submerge")
	if d := data(t, st); d.Books[1] != books(4) {
		t.Fatalf("books mask %b", d.Books[1])
	}
}

func TestImmortalCthulhuCostsFourToAwakenAgain(t *testing.T) {
	g := game(t)
	p := pos{units: []u{{gc, "cultist", "south-pacific", 1}}, gates: map[string]int{"south-pacific": gc}, power: powers(4, 8, 8, 8),
		awoken: map[string]bool{"1|0": true}, reqs: [5]uint8{0, books(4)}}
	if !has(notations(g, p.build(), 1), "awaken:cthulhu@south-pacific") {
		t.Fatal("a second awakening costs only 4")
	}
	p.awoken = nil
	if hasPrefix(notations(g, p.build(), 1), "awaken:cthulhu") {
		t.Fatal("the first awakening costs 10")
	}
}

func TestAwakenShubNiggurathEliminatesTwoCultists(t *testing.T) {
	g := game(t)
	p := pos{units: []u{{bg, "cultist", "south-america-west", 2}, {bg, "cultist", "africa", 1}}, gates: map[string]int{"south-america-west": bg}, power: powers(8, 8, 8, 8), turn: 3}
	st := play(t, g, p.build(), 3, "awaken:shub-niggurath@south-america-west")
	d := data(t, st)
	if d.Power[3] != 0 {
		t.Fatalf("power %d", d.Power[3])
	}
	// Two Cultists must go.
	if d.Stage == "prompt" {
		st = play(t, g, st, 3, "lose:cultist@africa")
	}
	total := count(st, 3, "cultist", "south-america-west") + count(st, 3, "cultist", "africa")
	if total != 1 {
		t.Fatalf("%d Cultists remain, want 1 of the 3", total)
	}
	if count(st, 3, "shub-niggurath", "south-america-west") != 1 {
		t.Fatal("Shub-Niggurath should stand at the Gate")
	}
}

func TestKingInYellowAwakensWithoutAGateAndHasturNeedsBoth(t *testing.T) {
	g := game(t)
	p := pos{units: []u{{ys, "cultist", "europe", 1}, {ys, "cultist", "africa", 1}}, gates: map[string]int{"europe": ys}, power: powers(8, 8, 8, 4), turn: 4}
	got := notations(g, p.build(), 4)
	if !has(got, "awaken:king-in-yellow@africa") || hasPrefix(got, "awaken:king-in-yellow@europe") {
		t.Fatalf("the King wakes where there is no Gate: %v", got)
	}
	q := pos{units: []u{{ys, "cultist", "europe", 1}, {ys, "king-in-yellow", "europe", 1}}, gates: map[string]int{"europe": ys}, power: powers(8, 8, 8, 10), turn: 4}
	if !has(notations(g, q.build(), 4), "awaken:hastur@europe") {
		t.Fatal("Hastur wakes at a Gate with the King in Yellow")
	}
	r := pos{units: []u{{ys, "cultist", "europe", 1}, {ys, "king-in-yellow", "africa", 1}}, gates: map[string]int{"europe": ys}, power: powers(8, 8, 8, 10), turn: 4}
	if hasPrefix(notations(g, r.build(), 4), "awaken:hastur") {
		t.Fatal("the King must stand at the Gate")
	}
}

func TestControlIsFreeAndTheOccupantStepsOffToAbandon(t *testing.T) {
	g := game(t)
	p := pos{units: []u{{gc, "cultist", "australia", 1}}, gates: map[string]int{"australia": 0}, power: powers(8, 8, 8, 8)}
	st := play(t, g, p.build(), 1, "control:cultist@australia")
	d := data(t, st)
	if d.Gates["australia"] != 1 || d.Power[1] != 8 || d.Turn.Main {
		t.Fatalf("gates %v power %d main %v", d.Gates, d.Power[1], d.Turn.Main)
	}
	if d.Holders["australia"] != "cultist" {
		t.Errorf("holders %v", d.Holders)
	}
	// There is no Abandon action any more: the unit that occupies the Gate
	// steps off it where it stands — a free action, no Move.
	refuse(t, g, st, 1, "abandon@australia")
	own := pos{units: []u{{gc, "cultist", "australia", 1}}, gates: map[string]int{"australia": 1}, power: powers(8, 8, 8, 8)}
	if got := notations(g, own.build(), 1); hasPrefix(got, "abandon") || !has(got, "leave:cultist@australia>australia") || hasPrefix(got, "go:cultist@australia>australia") {
		t.Fatalf("moves %v", got)
	}
	st = play(t, g, own.build(), 1, "leave:cultist@australia>australia")
	d = data(t, st)
	if d.Gates["australia"] != 0 || d.Power[1] != 8 || d.Holders["australia"] != "" {
		t.Fatalf("gates %v holders %v power %d", d.Gates, d.Holders, d.Power[1])
	}
	if count(st, gc, "cultist", "australia") != 1 {
		t.Fatal("the Cultist stays where it is")
	}
	// It is no Move and no action: the turn is as it was, nothing to confirm.
	if d.Stage != "turn" || d.Move != nil || d.Turn.Main || d.Turn.Seat != 1 {
		t.Fatalf("stage %q move %+v turn %+v", d.Stage, d.Move, d.Turn)
	}
	if got := notations(g, st, 1); has(got, "done") || !has(got, "control:cultist@australia") || has(got, "leave:cultist@australia>australia") {
		t.Fatalf("it may go back on the Gate, but not step off again this turn: %v", got)
	}
	// A cult with no Cultist there cannot take control.
	q := pos{units: []u{{gc, "deep-one", "australia", 1}}, gates: map[string]int{"australia": 0}, power: powers(8, 8, 8, 8)}
	if hasPrefix(notations(g, q.build(), 1), "control") {
		t.Fatal("monsters do not control Gates")
	}
}

func TestPassDropsToZeroAndEndsTheTurn(t *testing.T) {
	g := game(t)
	p := pos{units: []u{{gc, "cultist", "south-pacific", 1}}, gates: map[string]int{"south-pacific": gc}, power: powers(8, 8, 8, 8)}
	st := play(t, g, p.build(), 1, "pass")
	d := data(t, st)
	if d.Power[1] != 0 || d.Turn.Seat != 2 {
		t.Fatalf("power %d turn %d", d.Power[1], d.Turn.Seat)
	}
}

func TestCaptureNeedsAProtectorOfHigherRank(t *testing.T) {
	g := game(t)
	// A monster captures an unprotected Cultist; a GOO outranks a monster.
	p := pos{units: []u{{gc, "deep-one", "asia", 1}, {cc, "cultist", "asia", 2}}, power: powers(8, 8, 8, 8)}
	st := p.build()
	if !has(notations(g, st, 1), "capture@asia/cc") {
		t.Fatal("a monster may capture a lone Cultist")
	}
	st = play(t, g, st, 1, "capture@asia/cc")
	d := data(t, st)
	if d.Captured[1][2] != 1 || count(st, 2, "cultist", "asia") != 1 || d.Power[1] != 7 {
		t.Fatalf("captured %v power %d", d.Captured, d.Power[1])
	}
	// The victim's own monster protects against a monster...
	q := pos{units: []u{{gc, "deep-one", "asia", 1}, {cc, "cultist", "asia", 1}, {cc, "nightgaunt", "asia", 1}}, power: powers(8, 8, 8, 8)}
	if hasPrefix(notations(g, q.build(), 1), "capture") {
		t.Fatal("a monster protects against a monster")
	}
	// ...but not against a Great Old One.
	r := pos{units: []u{{gc, "cthulhu", "asia", 1}, {cc, "cultist", "asia", 1}, {cc, "nightgaunt", "asia", 1}}, power: powers(8, 8, 8, 8)}
	if !has(notations(g, r.build(), 1), "capture@asia/cc") {
		t.Fatal("a Great Old One outranks a monster")
	}
	// A Great Old One of the victim protects against everything.
	s := pos{units: []u{{gc, "cthulhu", "asia", 1}, {cc, "cultist", "asia", 1}, {cc, "nyarlathotep", "asia", 1}}, power: powers(8, 8, 8, 8)}
	if hasPrefix(notations(g, s.build(), 1), "capture") {
		t.Fatal("a GOO protects against capture")
	}
	// Cultists cannot capture Cultists.
	v := pos{units: []u{{gc, "cultist", "asia", 1}, {cc, "cultist", "asia", 1}}, power: powers(8, 8, 8, 8)}
	if hasPrefix(notations(g, v.build(), 1), "capture") {
		t.Fatal("Cultists cannot capture")
	}
}

func TestIllegalActionsAreRefused(t *testing.T) {
	g := game(t)
	p := pos{
		units: []u{{gc, "cultist", "south-pacific", 4}, {gc, "deep-one", "south-pacific", 1}, {cc, "cultist", "asia", 1}, {gc, "cultist", "australia", 2}},
		gates: map[string]int{"south-pacific": gc},
		power: powers(8, 8, 8, 8),
	}
	st := p.build()
	for _, c := range []struct {
		seat     int
		notation string
		why      string
	}{
		{cc, "recruit@asia", "not Crawling Chaos' turn"},
		{gc, "recruit@asia", "no unit of Great Cthulhu in Asia"},
		{gc, "recruit@south-pacific", "the pool has no Cultist left"},
		{gc, "gate@south-pacific", "a Gate already stands there"},
		{gc, "summon:deep-one@australia", "no Gate there"},
		{gc, "summon:shub-niggurath@south-pacific", "not a monster of this cult"},
		{gc, "awaken:cthulhu@south-pacific", "Cthulhu costs 10 and only 8 Power is held"},
		{gc, "battle@south-pacific/cc", "no enemy there"},
		{gc, "capture@asia/cc", "no unit of ours in Asia"},
		{gc, "go:cultist@south-pacific>asia", "not in a Move action"},
		{gc, "ritual", "not in the Doom phase"},
		{gc, "book:absorb", "no book prompt"},
		{gc, "control@south-pacific", "the Gate is already ours"},
	} {
		refuse(t, g, st, c.seat, c.notation)
		_ = c.why
	}
}
