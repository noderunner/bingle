package cthulhuwars_test

import (
	"encoding/json"
	"testing"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

type abilityView struct {
	Name    string `json:"name"`
	Seat    int    `json:"seat"`
	Current bool   `json:"current"`
}

// battleAbilitiesOf reads the abilities of the battle's step from the position a player is shown.
func battleAbilitiesOf(t *testing.T, g *catalog.CompiledGame, st *state.BoardState) (string, []abilityView) {
	t.Helper()
	raw, err := json.Marshal(state.Public(g, st, 1))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Ext struct {
			View struct {
				Battle struct {
					Step      string        `json:"step"`
					Abilities []abilityView `json:"abilities"`
				} `json:"battle"`
			} `json:"view"`
		} `json:"ext"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	return doc.Ext.View.Battle.Step, doc.Ext.View.Battle.Abilities
}

func TestTheBattleViewListsThePreBattleAbilitiesAndTheOneWaitedOn(t *testing.T) {
	g := game(t)
	p := pos{
		units: []u{
			{gc, "shoggoth", "asia", 2}, {gc, "deep-one", "asia", 1},
			{cc, "nightgaunt", "asia", 1}, {cc, "cultist", "asia", 1},
		},
		power: powers(8, 8, 0, 0), books: [5]uint8{0, books(bkAbsorb), books(bkAbduct), 0, 0},
		rng: seedDice(t, 1, 1, 1, 1, 1, 1, 1, 1, 1),
	}
	st := play(t, g, p.build(), gc, "battle@asia/cc")
	step, list := battleAbilitiesOf(t, g, st)
	if step != "pre" || len(list) != 2 {
		t.Fatalf("step %q, abilities %+v", step, list)
	}
	if list[0] != (abilityView{"Absorb", gc, true}) {
		t.Errorf("first ability %+v: Great Cthulhu's Absorb is the one the game waits on", list[0])
	}
	if list[1] != (abilityView{"Abduct", cc, false}) {
		t.Errorf("second ability %+v: Crawling Chaos's Abduct is still to come", list[1])
	}
	// Great Cthulhu finishes: only the Abduct is left, and now the game waits on it.
	st = play(t, g, st, gc, "done")
	if _, list = battleAbilitiesOf(t, g, st); len(list) != 1 || list[0] != (abilityView{"Abduct", cc, true}) {
		t.Errorf("after Great Cthulhu is done: %+v", list)
	}
}

func TestDevourBelongsToTheCultWithCthulhuThoughTheEnemyAnswersIt(t *testing.T) {
	g := game(t)
	p := pos{
		units: []u{
			{gc, "cthulhu", "asia", 1}, {cc, "nightgaunt", "asia", 1}, {cc, "cultist", "asia", 1},
		},
		power: powers(8, 8, 0, 0), rng: seedDice(t, 1, 1, 1, 1, 1, 1, 1, 1, 1),
	}
	st := play(t, g, p.build(), gc, "battle@asia/cc")
	if _, list := battleAbilitiesOf(t, g, st); len(list) != 1 || list[0] != (abilityView{"Devour", gc, true}) {
		t.Errorf("abilities %+v", list)
	}
}

func TestTheBattleViewListsThePostBattleAbilities(t *testing.T) {
	g := game(t)
	p := pos{
		units: []u{
			{bg, "fungi", "asia", 1}, {bg, "ghoul", "europe", 2}, {cc, "hunting-horror", "asia", 1}, {cc, "nightgaunt", "asia", 1},
		},
		turn: bg, power: powers(0, 0, 8, 0), books: [5]uint8{0, 0, 0, books(bkNecro), 0},
		rng: seedDice(t, 1, 1, 1, 1),
	}
	st := play(t, g, p.build(), bg, "battle@asia/cc")
	step, list := battleAbilitiesOf(t, g, st)
	if step != "post" || len(list) != 1 || list[0] != (abilityView{"Necrophagy", bg, true}) {
		t.Errorf("step %q, abilities %+v", step, list)
	}
}

func TestABattleWithNoAbilitiesListsNone(t *testing.T) {
	g := game(t)
	p := pos{
		units: []u{{gc, "deep-one", "asia", 2}, {gc, "shoggoth", "asia", 1}, {cc, "nightgaunt", "asia", 2}, {cc, "cultist", "asia", 1}},
		power: powers(8, 8, 0, 0), rng: seedDice(t, 6, 6, 6, 6, 1, 1),
	}
	st := play(t, g, p.build(), gc, "battle@asia/cc")
	if _, list := battleAbilitiesOf(t, g, st); len(list) != 0 {
		t.Errorf("abilities %+v", list)
	}
}
