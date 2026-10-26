package cthulhuwars_test

import (
	"testing"

	"github.com/noderunner/bingle/internal/engine/state"
)

// A reaction window knows whether the action that opened it was aimed at the
// reacting player: at the player itself, at one of its units, or at an area
// where it has a unit or a Gate. The session uses it for the setting "When
// I'm targeted". Only actions open windows: the effects of a battle (Madness
// moving a unit, say) are not actions.

// targetBoard has Great Cthulhu (seat 1) holding the Devolve reaction — a
// Cultist on its Gate in the South Pacific, another in Asia, the pool full of
// Deep Ones — and Crawling Chaos (seat 2) ready to act.
func targetBoard(extra ...u) pos {
	units := []u{
		{gc, "cultist", "south-pacific", 1}, {gc, "cultist", "asia", 1},
		{cc, "cultist", "europe", 1}, {cc, "cultist", "south-pacific", 1}, {cc, "cultist", "africa", 2},
		{bg, "cultist", "south-america-west", 1},
	}
	return pos{
		units: append(units, extra...),
		gates: map[string]int{"south-pacific": gc, "europe": cc},
		power: powers(6, 6, 6, 6), turn: cc,
		books: [5]uint8{0, books(bkDevolve), 0, 0, 0},
	}
}

// windowAfter plays the notation as Crawling Chaos and returns the reaction
// window that opens for Great Cthulhu.
func windowAfter(t *testing.T, p pos, notation string) *state.Reaction {
	t.Helper()
	g := game(t)
	st := play(t, g, p.build(), cc, notation)
	r := state.PendingReaction(g, st)
	if r == nil || r.Seat != gc || r.Trigger != cc || r.Ability != "Devolve" {
		t.Fatalf("after %q: reaction %+v\n%s", notation, r, state.RenderText(g, st))
	}
	return r
}

func TestAnActionInAnAreaWhereTheReactorHasUnitsTargetsIt(t *testing.T) {
	// Recruiting in the South Pacific, where Great Cthulhu stands.
	if r := windowAfter(t, targetBoard(), "recruit@south-pacific"); !r.Targeted {
		t.Error("recruiting among Great Cthulhu's units does not target it")
	}
	// Recruiting in Europe, where it has nothing.
	if r := windowAfter(t, targetBoard(), "recruit@europe"); r.Targeted {
		t.Error("recruiting in Europe targets Great Cthulhu")
	}
}

func TestAnActionNamingThePlayerTargetsIt(t *testing.T) {
	// Capturing one of its Cultists: the Cultist leaves the area, the player
	// was named. (The capture earns Crawling Chaos a spellbook first.)
	g := game(t)
	p := targetBoard(u{cc, "nightgaunt", "asia", 1})
	st := play(t, g, p.build(), cc, "capture@asia/gc")
	st = play(t, g, st, cc, "book:abduct")
	r := state.PendingReaction(g, st)
	if r == nil || r.Seat != gc || r.Trigger != cc || !r.Targeted {
		t.Fatalf("after the capture: reaction %+v\n%s", r, state.RenderText(g, st))
	}
	if n := count(st, gc, "cultist", "asia"); n != 0 {
		t.Fatalf("the Cultist is still in Asia")
	}
}

func TestThousandFormsTargetsEveryOtherPlayer(t *testing.T) {
	p := targetBoard(u{cc, "nyarlathotep", "europe", 1})
	p.books[cc] = books(bkForms)
	p.rng = seedDice(t, 3)
	if r := windowAfter(t, p, "forms"); !r.Targeted {
		t.Error("The Thousand Forms is aimed at the other players")
	}
}

func TestAMoveTargetsEveryAreaItEnters(t *testing.T) {
	g := game(t)
	p := targetBoard()
	// A Move whose steps enter Asia (Great Cthulhu's Cultist stands there) and
	// an empty area: aimed at both.
	st := play(t, g, p.build(), cc, "go:cultist@africa>asia")
	st = play(t, g, st, cc, "go:cultist@africa>north-atlantic")
	st = play(t, g, st, cc, "done")
	r := state.PendingReaction(g, st)
	if r == nil || r.Seat != gc || !r.Targeted {
		t.Fatalf("a Move into Asia: reaction %+v", r)
	}
	// A Move that stays away from it.
	st = play(t, g, p.build(), cc, "go:cultist@africa>north-atlantic")
	st = play(t, g, st, cc, "done")
	r = state.PendingReaction(g, st)
	if r == nil || r.Seat != gc || r.Targeted {
		t.Fatalf("a Move into the North Atlantic: reaction %+v", r)
	}
}

func TestStepsOffAGateAreFreeAndOpenNoWindow(t *testing.T) {
	g := game(t)
	st := targetBoard().build()
	if !has(notations(g, st, cc), "leave:cultist@europe>europe") {
		t.Fatalf("no step-off: %v", notations(g, st, cc))
	}
	st = play(t, g, st, cc, "leave:cultist@europe>europe")
	if r := state.PendingReaction(g, st); r != nil {
		t.Fatalf("a free action opened a window: %+v", r)
	}
}

// settle plays the first legal decision of whoever must decide until the
// position is back at a turn, or a reaction waits.
func settle(t *testing.T, st *state.BoardState) *state.BoardState {
	t.Helper()
	g := game(t)
	for i := 0; i < 60; i++ {
		if r := state.PendingReaction(g, st); r != nil {
			return st
		}
		if d := data(t, st); d.Stage == "turn" || d.Stage == "" {
			return st
		}
		seat, _ := state.TurnAfter(g, st, 1)
		legal := state.LegalMoves(g, st, seat)
		if len(legal) == 0 {
			t.Fatalf("seat %d has nothing to decide\n%s", seat, state.RenderText(g, st))
		}
		res, err := state.Apply(g, st, seat, legal[0])
		if err != nil {
			t.Fatal(err)
		}
		st = res.NewState
	}
	t.Fatal("the battle does not end")
	return st
}

func TestABattleTargetsTheDefenderAndTheAreaItIsFoughtIn(t *testing.T) {
	g := game(t)
	build := func(gcAt string) pos {
		return pos{
			units: []u{
				{cc, "flying-polyp", "africa", 1}, {bg, "cultist", "africa", 2},
				{gc, "cultist", gcAt, 1}, {gc, "cultist", "south-pacific", 1},
			},
			gates: map[string]int{"south-pacific": gc},
			power: powers(6, 6, 6, 6), turn: cc,
			books: [5]uint8{0, books(bkDevolve), 0, 0, 0},
			rng:   seedDice(t, 1, 1),
		}
	}
	// Great Cthulhu has a Cultist in the area of the battle: targeted.
	st := settle(t, play(t, g, build("africa").build(), cc, "battle@africa/bg"))
	if r := state.PendingReaction(g, st); r == nil || r.Seat != gc || !r.Targeted || r.Trigger != cc {
		t.Fatalf("a battle where Great Cthulhu stands: %+v", r)
	}
	// ... and not when it stands elsewhere.
	st = settle(t, play(t, g, build("asia").build(), cc, "battle@africa/bg"))
	if r := state.PendingReaction(g, st); r == nil || r.Seat != gc || r.Targeted || r.Trigger != cc {
		t.Fatalf("a battle that does not concern Great Cthulhu: %+v", r)
	}
}

// Madness moves an enemy unit after a battle: an effect, not an action. The
// unit arriving where Great Cthulhu stands does not target it — the window it
// is offered after the battle is the battle's, aimed at the area it was
// fought in.
func TestAPostBattleEffectIsNotAnAction(t *testing.T) {
	g := game(t)
	p := pos{
		units: []u{
			{ys, "undead", "africa", 3}, {bg, "cultist", "africa", 3},
			{gc, "cultist", "asia", 1}, {gc, "cultist", "south-pacific", 1},
		},
		gates: map[string]int{"south-pacific": gc},
		power: powers(6, 6, 6, 6), turn: ys,
		books: [5]uint8{0, books(bkDevolve), books(bkMadness), 0, 0},
		rng:   seedDice(t, 6, 4, 2),
	}
	st := play(t, g, p.build(), ys, "battle@africa/bg")
	// The battle's pained Cultist retreats where Crawling Chaos (Madness, not
	// in the battle) says: Asia, where Great Cthulhu's Cultist stands.
	r := state.PendingReaction(g, st)
	if r == nil || r.Seat != cc || r.Ability != "Madness" || r.Trigger != ys || r.Targeted {
		t.Fatalf("the Madness window: %+v", r)
	}
	st = play(t, g, st, cc, "retreat:cultist>asia/bg")
	if n := count(st, bg, "cultist", "asia"); n != 1 {
		t.Fatalf("the Cultist did not retreat to Asia: %d", n)
	}
	// The action was Yellow Sign's battle in Africa: Great Cthulhu has nothing
	// there, so it is not targeted, however the unit was moved.
	r = state.PendingReaction(g, st)
	if r == nil || r.Seat != gc || r.Ability != "Devolve" || r.Trigger != ys || r.Targeted {
		t.Fatalf("Great Cthulhu's window after the battle: %+v", r)
	}
}
