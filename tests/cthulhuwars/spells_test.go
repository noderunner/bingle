package cthulhuwars_test

import (
	"strings"
	"testing"

	"github.com/noderunner/bingle/internal/engine/state"
)

func TestAbsorbFeedsUnitsToAShoggothForThreeDiceEach(t *testing.T) {
	g := game(t)
	p := pos{
		units: []u{{gc, "shoggoth", "asia", 2}, {gc, "deep-one", "asia", 1}, {cc, "cultist", "asia", 1}},
		power: powers(8, 0, 0, 0), books: [5]uint8{0, books(bkAbsorb), 0, 0, 0}, rng: seedDice(t, 1, 1, 1, 1, 1, 1, 1),
	}
	st := play(t, g, p.build(), 1, "battle@asia/cc")
	d := data(t, st)
	if d.Stage != "pre" || d.Bat.Who != gc {
		t.Fatalf("stage %q: Great Cthulhu may Absorb first", d.Stage)
	}
	got := notations(g, st, gc)
	if !has(got, "absorb:deep-one") || !has(got, "done") {
		t.Fatalf("options = %v", got)
	}
	st = play(t, g, st, gc, "absorb:deep-one")
	if count(st, gc, "deep-one", "asia") != 0 {
		t.Fatal("the fed unit is eliminated")
	}
	st = play(t, g, st, gc, "done")
	d = data(t, st)
	if d.Last == nil {
		t.Fatalf("the battle should be rolled: stage %q", d.Stage)
	}
	// Two Shoggoths (2 each) plus 3 dice for the one fed.
	if d.Last.Combat[gc] != 2+2+3 {
		t.Fatalf("Combat %d, want 7", d.Last.Combat[gc])
	}
	if err := state.Validate(g, st); err != nil {
		t.Fatal(err)
	}
}

func TestRegenerateLetsAStarSpawnShrugOffTheFirstKill(t *testing.T) {
	g := game(t)
	mk := func(d1, d2 int) pos {
		return pos{
			units: []u{{gc, "starspawn", "asia", 1}, {gc, "deep-one", "asia", 1}, {cc, "hunting-horror", "asia", 1}},
			turn:  cc, power: powers(0, 8, 0, 0), books: [5]uint8{0, books(bkRegenerate), 0, 0, 0},
			rng: seedDice(t, d1, d2, 1, 1, 1, 1),
		}
	}
	eventsHave := func(st *state.BoardState, text string) bool {
		for _, ev := range data(t, st).Events {
			if strings.Contains(ev, text) {
				return true
			}
		}
		return false
	}
	// One Kill, assigned to the Star Spawn: it is pained instead of killed.
	st := play(t, g, mk(6, 1).build(), cc, "battle@asia/gc")
	if d := data(t, st); d.Stage != "assign" {
		t.Fatalf("stage %q, the owner assigns the Kill", d.Stage)
	}
	if got := notations(g, st, gc); !has(got, "assign:starspawn") || !has(got, "assign:deep-one") {
		t.Fatalf("options = %v", got)
	}
	st = play(t, g, st, gc, "assign:starspawn")
	if !eventsHave(st, "Regenerate") {
		t.Fatalf("events %q", data(t, st).Events)
	}
	if err := state.Validate(g, st); err != nil {
		t.Fatal(err)
	}
	// A Star Spawn that had no other unit to retreat with is still alive.
	alive := 0
	for _, id := range st.Occupied() {
		for _, o := range st.Stack(id) {
			if o.Piece == "starspawn" && o.Color == "green" {
				alive++
			}
		}
	}
	if alive != 1 {
		t.Fatalf("the Star Spawn must still be on the map (found %d)", alive)
	}
	// Two Kills on the Star Spawn kill it.
	st = play(t, g, mk(6, 6).build(), cc, "battle@asia/gc")
	st = play(t, g, st, gc, "assign:starspawn")
	if eventsHave(st, "Star Spawn is killed") {
		t.Fatalf("only the second Kill kills it: %q", data(t, st).Events)
	}
	if data(t, st).Stage == "assign" {
		st = play(t, g, st, gc, "assign:starspawn")
	}
	if !eventsHave(st, "Star Spawn is killed") {
		t.Fatalf("two Kills must kill the Star Spawn: %q", data(t, st).Events)
	}
}

func TestMadnessLetsCrawlingChaosChooseWhereTheEnemyRetreats(t *testing.T) {
	g := game(t)
	p := pos{
		units: []u{{gc, "deep-one", "asia", 1}, {cc, "hunting-horror", "asia", 1}},
		turn:  cc, power: powers(0, 8, 0, 0), books: [5]uint8{0, 0, books(bkMadness), 0, 0},
		rng: seedDice(t, 4, 1, 1),
	}
	st := play(t, g, p.build(), cc, "battle@asia/gc")
	d := data(t, st)
	if d.Stage != "retreat" {
		t.Fatalf("stage %q:\n%s", d.Stage, state.RenderText(g, st))
	}
	seat, err := state.TurnAfter(g, st, 0)
	if err != nil {
		t.Fatal(err)
	}
	if seat != cc {
		t.Fatalf("Madness: Crawling Chaos chooses the retreat, seat %d acts", seat)
	}
	got := notations(g, st, cc)
	if !hasPrefix(got, "retreat:deep-one>") {
		t.Fatalf("options = %v", got)
	}
}

func TestNecrophagyDragsGhoulsIntoABattleAndPainsBothSides(t *testing.T) {
	g := game(t)
	p := pos{
		units: []u{
			{bg, "fungi", "asia", 1}, {bg, "ghoul", "europe", 2}, {cc, "hunting-horror", "asia", 1}, {cc, "nightgaunt", "asia", 1},
		},
		turn: bg, power: powers(0, 0, 8, 0), books: [5]uint8{0, 0, 0, books(bkNecro), 0},
		rng: seedDice(t, 1, 1, 1, 1),
	}
	st := play(t, g, p.build(), bg, "battle@asia/cc")
	d := data(t, st)
	if d.Stage != "post" {
		t.Fatalf("stage %q:\n%s", d.Stage, state.RenderText(g, st))
	}
	if got := notations(g, st, bg); !has(got, "necro") {
		t.Fatalf("options = %v", got)
	}
	st = play(t, g, st, bg, "necro")
	if count(st, bg, "ghoul", "asia")+count(st, bg, "ghoul", "europe") != 2 {
		t.Fatal("no Ghoul is lost by being moved")
	}
	// Both Ghouls stand in Asia (or retreated with the Pain they took).
	if count(st, bg, "ghoul", "europe") != 0 && count(st, bg, "ghoul", "asia") == 0 {
		t.Fatal("the Ghouls did not move")
	}
	if err := state.Validate(g, st); err != nil {
		t.Fatal(err)
	}
}

func TestAbductTakesAnEnemyUnitOfTheirChoosing(t *testing.T) {
	g := game(t)
	p := pos{
		units: []u{{cc, "nightgaunt", "asia", 1}, {cc, "hunting-horror", "asia", 1}, {gc, "deep-one", "asia", 2}, {gc, "cultist", "asia", 1}},
		turn:  cc, power: powers(0, 8, 0, 0), books: [5]uint8{0, 0, books(bkAbduct), 0, 0},
		rng: seedDice(t, 1, 1, 1, 1),
	}
	st := play(t, g, p.build(), cc, "battle@asia/gc")
	d := data(t, st)
	if d.Stage != "pre" || d.Bat.Who != cc {
		t.Fatalf("stage %q", d.Stage)
	}
	got := notations(g, st, cc)
	if !has(got, "abduct") {
		t.Fatalf("options = %v", got)
	}
	st = play(t, g, st, cc, "abduct")
	d = data(t, st)
	// The enemy chooses which of its monsters or cultists is taken.
	if d.Stage != "prompt" || d.Prompts[0].Seat != gc || d.Prompts[0].Why != "abduct" {
		t.Fatalf("stage %q prompts %+v", d.Stage, d.Prompts)
	}
	if got := notations(g, st, gc); !hasPrefix(got, "lose:") {
		t.Fatalf("options = %v", got)
	}
	if err := state.Validate(g, st); err != nil {
		t.Fatal(err)
	}
}

func TestSeekAndDestroyBringsHuntingHorrorsToTheBattle(t *testing.T) {
	g := game(t)
	p := pos{
		units: []u{{cc, "hunting-horror", "asia", 1}, {cc, "hunting-horror", "africa", 2}, {gc, "deep-one", "asia", 1}},
		turn:  cc, power: powers(0, 8, 0, 0), books: [5]uint8{0, 0, books(bkSeek), 0, 0},
		rng: seedDice(t, 1, 1, 1, 1),
	}
	st := play(t, g, p.build(), cc, "battle@asia/gc")
	if got := notations(g, st, cc); !has(got, "seek@africa") {
		t.Fatalf("options = %v", got)
	}
	st = play(t, g, st, cc, "seek@africa")
	d := data(t, st)
	// 3 Horrors: 6 dice.
	if d.Last == nil || d.Last.Combat[cc] != 6 {
		t.Fatalf("last %+v", d.Last)
	}
	if count(st, cc, "hunting-horror", "africa") != 0 {
		t.Fatal("the Horrors left Africa")
	}
}
