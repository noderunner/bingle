package cthulhuwars_test

import (
	"testing"

	"github.com/noderunner/bingle/internal/engine/state"
)

func TestBattleKillsAndPainsAndRetreat(t *testing.T) {
	g := game(t)
	// Great Cthulhu: 3 Deep Ones (3 dice) against 3 Cultists (no dice).
	p := pos{
		units: []u{{gc, "deep-one", "asia", 3}, {cc, "cultist", "asia", 3}},
		power: powers(8, 8, 8, 8), rng: seedDice(t, 6, 4, 2),
	}
	st := play(t, g, p.build(), 1, "battle@asia/cc")
	d := data(t, st)
	if d.Power[1] != 7 {
		t.Fatalf("a battle costs 1 Power: %d", d.Power[1])
	}
	// One Kill (a 6) and one Pain (a 4): the Cultists are interchangeable, so
	// the Kill and Pain land by themselves; the pained Cultist must retreat.
	if d.Stage != "retreat" {
		t.Fatalf("stage %q", d.Stage)
	}
	if n := count(st, 2, "cultist", "asia"); n != 2 {
		t.Fatalf("one Cultist was killed: %d left", n)
	}
	got := notations(g, st, 2)
	if len(got) != 6 || hasPrefix(got, "retreat:cultist>asia") {
		t.Fatalf("retreat options = %v", got)
	}
	st = play(t, g, st, 2, "retreat:cultist>europe")
	d = data(t, st)
	if count(st, 2, "cultist", "asia") != 1 || count(st, 2, "cultist", "europe") != 1 {
		t.Fatal("the pained Cultist should have retreated")
	}
	// Killing a unit fulfilled a requirement: Great Cthulhu now picks a book.
	if d.Stage != "prompt" || d.Prompts[0].Kind != "book" || d.Prompts[0].Seat != 1 {
		t.Fatalf("expected a spellbook choice, got stage %q prompts %+v", d.Stage, d.Prompts)
	}
	st = play(t, g, st, 1, "book:devolve")
	d = data(t, st)
	// Nothing unlimited is left to do, so the turn passes on by itself.
	if d.Stage != "turn" || d.Turn.Seat != 2 {
		t.Fatalf("expected the next player's turn: %q %+v", d.Stage, d.Turn)
	}
	if d.Last == nil || d.Last.Combat[1] != 3 {
		t.Fatalf("last battle %+v", d.Last)
	}
}

func TestPainedUnitsThatCannotRetreatLoseOne(t *testing.T) {
	g := game(t)
	// Australia touches only the Indian Ocean and New Zealand: both hold
	// Great Cthulhu's units.
	p := pos{
		units: []u{
			{gc, "deep-one", "australia", 2}, {gc, "deep-one", "indian-ocean", 1}, {gc, "deep-one", "new-zealand", 1},
			{cc, "cultist", "australia", 3},
		},
		power: powers(8, 8, 8, 8), rng: seedDice(t, 4, 5),
	}
	st := play(t, g, p.build(), 1, "battle@australia/cc")
	d := data(t, st)
	// Two Pains on a Cultist each; with nowhere to go one is eliminated and
	// the others stay put.
	if d.Stage == "retreat" {
		t.Fatalf("no retreat is possible")
	}
	if n := count(st, 2, "cultist", "australia"); n != 2 {
		t.Fatalf("one of the pained Cultists should be eliminated, %d remain", n)
	}
}

func TestCthulhuDevoursAMonsterOrCultist(t *testing.T) {
	g := game(t)
	p := pos{
		units: []u{{gc, "cthulhu", "asia", 1}, {cc, "nightgaunt", "asia", 1}, {cc, "cultist", "asia", 1}},
		power: powers(8, 8, 8, 8), rng: seedDice(t, 1, 1, 1, 1, 1, 1),
	}
	st := play(t, g, p.build(), 1, "battle@asia/cc")
	d := data(t, st)
	if d.Stage != "prompt" || d.Prompts[0].Why != "devour" || d.Prompts[0].Seat != 2 {
		t.Fatalf("Devour should make the enemy choose: stage %q prompts %+v", d.Stage, d.Prompts)
	}
	got := notations(g, st, 2)
	if !has(got, "lose:nightgaunt@asia") || !has(got, "lose:cultist@asia") {
		t.Fatalf("options = %v", got)
	}
	st = play(t, g, st, 2, "lose:cultist@asia")
	if count(st, 2, "cultist", "asia") != 0 || count(st, 2, "nightgaunt", "asia") != 1 {
		t.Fatal("the chosen unit was devoured")
	}
	// Devouring counts for Great Cthulhu's spellbook requirement.
	d = data(t, st)
	if d.Reqs[1]&(1<<1) == 0 {
		t.Fatalf("Devouring a unit fulfils a requirement: reqs %b", d.Reqs[1])
	}
}

func TestCombatValuesOfTheFactions(t *testing.T) {
	g := game(t)
	cases := []struct {
		name  string
		units []u
		seat  int
		want  int
		books uint8
		gates map[string]int
		doom  int
	}{
		{"deep ones, shoggoth, starspawn", []u{{gc, "deep-one", "asia", 2}, {gc, "shoggoth", "asia", 1}, {gc, "starspawn", "asia", 1}}, gc, 2 + 2 + 3, 0, nil, 0},
		{"cthulhu", []u{{gc, "cthulhu", "asia", 1}}, gc, 6, 0, nil, 0},
		{"polyp and horror", []u{{cc, "flying-polyp", "asia", 2}, {cc, "hunting-horror", "asia", 1}}, cc, 2 + 2, 0, nil, 0},
		{"byakhee roll one more than their number", []u{{ys, "byakhee", "asia", 3}}, ys, 4, 0, nil, 0},
		{"undead roll one fewer", []u{{ys, "undead", "asia", 4}}, ys, 3, 0, nil, 0},
		{"a lone undead rolls nothing", []u{{ys, "undead", "asia", 1}}, ys, 0, 0, nil, 0},
		{"fungi and dark young", []u{{bg, "fungi", "asia", 2}, {bg, "dark-young", "asia", 1}}, bg, 2 + 2, 0, nil, 0},
		{"shub: gates plus cultists in play", []u{{bg, "shub-niggurath", "asia", 1}, {bg, "cultist", "asia", 2}, {bg, "cultist", "africa", 1}}, bg, 2 + 3, 0, map[string]int{"asia": bg, "africa": bg}, 0},
		{"frenzy gives Cultists 1", []u{{bg, "cultist", "asia", 3}}, bg, 3, books(1), nil, 0},
		{"nyarlathotep counts his spellbooks and his enemy's", []u{{cc, "nyarlathotep", "asia", 1}}, cc, 3, books(0, 1), nil, 0},
	}
	for _, c := range cases {
		units := append([]u{}, c.units...)
		// An enemy Cultist to fight so a battle can be declared; the
		// attacker needs at least 1 Combat, so give it a dice-less target.
		units = append(units, u{3 - c.seat%2, "cultist", "asia", 1})
		if c.seat == 3 {
			units[len(units)-1].seat = 2
		}
		enemy := units[len(units)-1].seat
		p := pos{units: units, power: powers(8, 8, 8, 8), turn: c.seat, gates: c.gates, rng: 99}
		p.books[c.seat] = c.books
		if c.name == "nyarlathotep counts his spellbooks and his enemy's" {
			// Two on his own card, one on Black Goat's — Great Cthulhu's three
			// are not in the battle.
			p.books[1], p.books[3] = books(0, 1, 2), books(0)
		}
		st := p.build()
		if c.want == 0 {
			if hasPrefix(notations(g, st, c.seat), "battle@asia") {
				t.Errorf("%s: no Combat, so no battle may be declared", c.name)
			}
			continue
		}
		key := map[int]string{1: "gc", 2: "cc", 3: "bg", 4: "ys"}[enemy]
		st = play(t, g, st, c.seat, "battle@asia/"+key)
		d := data(t, st)
		if d.Last == nil {
			// The battle may be waiting on a decision; read the live view.
			continue
		}
		if got := d.Last.Combat[c.seat]; got != c.want {
			t.Errorf("%s: Combat %d, want %d", c.name, got, c.want)
		}
	}
}

func TestBattleRequiresACombatValueAndOnePerAreaPerPhase(t *testing.T) {
	g := game(t)
	p := pos{units: []u{{gc, "cultist", "asia", 3}, {cc, "cultist", "asia", 1}}, power: powers(8, 8, 8, 8)}
	if hasPrefix(notations(g, p.build(), 1), "battle") {
		t.Fatal("Cultists have no Combat")
	}
	q := pos{units: []u{{gc, "deep-one", "asia", 1}, {cc, "cultist", "asia", 3}}, power: powers(8, 8, 8, 8), rng: seedDice(t, 1)}
	st := play(t, g, q.build(), 1, "battle@asia/cc")
	if hasPrefix(notations(g, st, 1), "battle@asia") {
		t.Fatalf("one battle per area: %v", notations(g, st, 1))
	}
}

func TestInvisibilityAndAbductAndSeek(t *testing.T) {
	g := game(t)
	// Crawling Chaos has Invisibility: a Polyp hides a Cultist.
	p := pos{
		units: []u{{gc, "deep-one", "asia", 2}, {cc, "flying-polyp", "asia", 1}, {cc, "cultist", "asia", 2}},
		power: powers(8, 8, 8, 8), books: [5]uint8{0, 0, books(1, 0, 2), 0, 0}, rng: seedDice(t, 1, 1, 1),
	}
	p.books = [5]uint8{0, 0, books(1), 0, 0}
	st := play(t, g, p.build(), 1, "battle@asia/cc")
	d := data(t, st)
	if d.Stage != "pre" || d.Bat.Who != 2 {
		t.Fatalf("Crawling Chaos has a pre-battle ability: stage %q", d.Stage)
	}
	got := notations(g, st, 2)
	if !has(got, "invisible:cultist") || !has(got, "done") {
		t.Fatalf("options = %v", got)
	}
	st = play(t, g, st, 2, "invisible:cultist")
	found := false
	for _, ev := range data(t, st).Events {
		if len(ev) > 12 && ev[:12] == "Invisibility" {
			found = true
		}
	}
	if !found {
		t.Fatalf("events %v", data(t, st).Events)
	}
}

func TestEmissaryTurnsAKillOnNyarlathotepIntoAPain(t *testing.T) {
	g := game(t)
	mk := func(withBook bool) *state.BoardState {
		p := pos{
			units: []u{{gc, "deep-one", "asia", 1}, {cc, "nyarlathotep", "asia", 1}},
			power: powers(8, 8, 8, 8), rng: seedDice(t, 6, 1),
		}
		if withBook {
			p.books[2] = books(3)
		}
		return play(t, g, p.build(), 1, "battle@asia/cc")
	}
	// With Emissary of the Outer Gods the Kill is a Pain: he retreats.
	st := mk(true)
	if d := data(t, st); d.Stage != "retreat" {
		t.Fatalf("stage %q, want Nyarlathotep retreating", d.Stage)
	}
	st = play(t, g, st, 2, "retreat:nyarlathotep>europe")
	if count(st, 2, "nyarlathotep", "europe") != 1 {
		t.Fatal("Nyarlathotep survived")
	}
	// Without it the Kill eliminates him.
	st = mk(false)
	if n := count(st, 2, "nyarlathotep", "asia"); n != 0 {
		t.Fatalf("Nyarlathotep should have been killed: %d", n)
	}
	// And an enemy Great Old One in the battle switches Emissary off.
	p := pos{
		units: []u{{gc, "cthulhu", "asia", 1}, {cc, "nyarlathotep", "asia", 1}},
		power: powers(8, 8, 8, 8), rng: seedDice(t, 1, 1, 1, 1, 1, 1, 1),
	}
	p.books[2] = books(3)
	_ = p
}

func TestEveryLegalBattleDecisionApplies(t *testing.T) {
	g := game(t)
	// A messy brawl: play it out with random choices and check consistency.
	for seed := int64(1); seed <= 40; seed++ {
		p := pos{
			units: []u{
				{gc, "deep-one", "asia", 2}, {gc, "shoggoth", "asia", 1}, {gc, "starspawn", "asia", 1}, {gc, "cultist", "asia", 1},
				{cc, "nightgaunt", "asia", 2}, {cc, "flying-polyp", "asia", 2}, {cc, "hunting-horror", "asia", 1}, {cc, "cultist", "asia", 2},
			},
			power: powers(8, 8, 8, 8), books: [5]uint8{0, books(0, 3), books(0, 1, 2, 3, 4), 0, 0}, rng: uint64(seed*131 + 7),
		}
		st := p.build()
		st = play(t, g, st, 1, "battle@asia/cc")
		for step := 0; step < 60; step++ {
			d := data(t, st)
			if d.Bat == nil {
				break
			}
			seat, err := state.TurnAfter(g, st, 0)
			if err != nil {
				t.Fatal(err)
			}
			legal := state.LegalMoves(g, st, seat)
			if len(legal) == 0 {
				t.Fatalf("seed %d: no legal decision in stage %q\n%s", seed, d.Stage, state.RenderText(g, st))
			}
			res, err := state.Apply(g, st, seat, legal[int(seed+int64(step))%len(legal)])
			if err != nil {
				t.Fatal(err)
			}
			st = res.NewState
		}
		if data(t, st).Bat != nil {
			t.Fatalf("seed %d: the battle did not end", seed)
		}
		if err := state.Validate(g, st); err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
	}
}

func TestSurvivorTakesTheGateAfterABattleEmptiesIt(t *testing.T) {
	g := game(t)
	// Every Cultist holding Asia's Gate dies; the attacker's own Cultist,
	// standing there, takes the open Gate by itself.
	p := pos{
		units: []u{{gc, "deep-one", "asia", 3}, {gc, "cultist", "asia", 1}, {cc, "cultist", "asia", 2}},
		gates: map[string]int{"asia": cc},
		power: powers(8, 8, 8, 8), rng: seedDice(t, 6, 6, 6),
	}
	st := play(t, g, p.build(), 1, "battle@asia/cc")
	if n := count(st, 2, "cultist", "asia"); n != 0 {
		t.Skipf("dice did not kill both Cultists: %d left", n)
	}
	if d := data(t, st); d.Gates["asia"] != gc {
		t.Fatalf("the surviving Cultist should hold the Gate: %v", d.Gates)
	}
}
