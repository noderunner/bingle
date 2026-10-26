package cthulhuwars_test

import (
	"encoding/json"
	"testing"

	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/games/cthulhuwars"
)

// inDoom is a position in the Doom phase with every seat still to decide.
func inDoom(p pos) pos {
	p.phase = "doom"
	p.stage = "doom"
	p.doomx = &cthulhuwars.DoomState{Order: []int{1, 2, 3, 4}}
	p.turn = 0
	return p
}

func TestRitualGivesDoomPerGateAndAnElderSignPerGreatOldOne(t *testing.T) {
	g := game(t)
	p := inDoom(pos{
		units: []u{{gc, "cthulhu", "south-pacific", 1}, {gc, "cultist", "south-pacific", 1}, {gc, "cultist", "australia", 1}},
		gates: map[string]int{"south-pacific": gc, "australia": gc},
		power: powers(9, 0, 0, 0), reqs: [5]uint8{0, 0xff},
	})
	st := play(t, g, p.build(), 1, "ritual")
	d := data(t, st)
	if d.Power[1] != 4 || d.Doom[1] != 2 || len(d.Signs[1]) != 1 {
		t.Fatalf("power %d doom %d signs %v", d.Power[1], d.Doom[1], d.Signs[1])
	}
	if v := d.Signs[1][0]; v < 1 || v > 3 {
		t.Fatalf("an Elder Sign is worth 1 to 3: %d", v)
	}
	// Revealing it adds its value.
	before := d.Doom[1]
	val := d.Signs[1][0]
	st = play(t, g, st, 1, "reveal/1")
	d = data(t, st)
	if d.Doom[1] != before+val || len(d.Signs[1]) != 0 {
		t.Fatalf("doom %d (was %d, sign %d)", d.Doom[1], before, val)
	}
}

func TestRitualCostRisesAndTheInstantDeathSpaceEndsTheGame(t *testing.T) {
	g := game(t)
	p := inDoom(pos{
		units: []u{{gc, "cultist", "south-pacific", 1}, {cc, "cultist", "asia", 1}},
		gates: map[string]int{"south-pacific": gc, "asia": cc},
		power: powers(10, 10, 0, 0), ritual: 7,
		books: [5]uint8{0, 63, 63, 0, 0},
	})
	p.doom = [5]int{0, 4, 0, 0, 0}
	st := play(t, g, p.build(), 1, "ritual")
	d := data(t, st)
	// The marker stood on the last numbered space (a Ritual costs 10 there):
	// it moves to Instant Death.
	if d.Power[1] != 0 || d.Ritual != 8 || !d.Instant {
		t.Fatalf("power %d ritual %d instant %v", d.Power[1], d.Ritual, d.Instant)
	}
	// The next players may still take their Ritual at cost 10; once all
	// have decided the game ends by instant death.
	if got := notations(g, st, 2); !has(got, "ritual") {
		t.Fatalf("options = %v", got)
	}
	st = play(t, g, st, 2, "ritual")
	d = data(t, st)
	if d.Phase != "over" || d.Reason != "instant-death" || d.Winner != gc {
		t.Fatalf("phase %q reason %q winner %d", d.Phase, d.Reason, d.Winner)
	}
}

func TestThirtyDoomEndsTheGameAtTheEndOfTheDoomPhase(t *testing.T) {
	g := game(t)
	p := lastPass(pos{
		units: []u{{gc, "cultist", "south-pacific", 1}, {cc, "cultist", "asia", 1}},
		gates: map[string]int{"south-pacific": gc, "asia": cc},
		books: [5]uint8{0, 63, 0, 0, 0},
	})
	p.doom = [5]int{0, 29, 20, 0, 0}
	st := play(t, g, p.build(), 1, "pass")
	// The first player is a tie-break away; walk the decisions to the end.
	for i := 0; i < 20 && data(t, st).Phase != "over"; i++ {
		seat, err := state.TurnAfter(g, st, 0)
		if err != nil {
			t.Fatal(err)
		}
		legal := state.LegalMoves(g, st, seat)
		// Prefer the quiet choices.
		pick := legal[0]
		for _, mv := range legal {
			switch mv.Action {
			case "skip", "cw", "first", "book":
				pick = mv
			}
		}
		res, err := state.Apply(g, st, seat, pick)
		if err != nil {
			t.Fatal(err)
		}
		st = res.NewState
	}
	d := data(t, st)
	if d.Phase != "over" {
		t.Fatalf("the game should be over:\n%s", state.RenderText(g, st))
	}
	// Great Cthulhu has 30 Doom and all six spellbooks.
	if d.Winner != gc || d.Reason != "doom" {
		t.Fatalf("winner %d reason %q doom %v", d.Winner, d.Reason, d.Doom)
	}
}

func TestWithoutSixSpellbooksHumanityWins(t *testing.T) {
	g := game(t)
	p := lastPass(pos{
		units: []u{{gc, "cultist", "south-pacific", 1}, {cc, "cultist", "asia", 1}},
		gates: map[string]int{"south-pacific": gc, "asia": cc},
		books: [5]uint8{0, 0, 31, 0, 0}, // five books only
	})
	p.doom = [5]int{0, 0, 29, 0, 0}
	st := play(t, g, p.build(), 1, "pass")
	for i := 0; i < 20 && data(t, st).Phase != "over"; i++ {
		seat, _ := state.TurnAfter(g, st, 0)
		legal := state.LegalMoves(g, st, seat)
		pick := legal[0]
		for _, mv := range legal {
			switch mv.Action {
			case "skip", "cw", "first":
				pick = mv
			}
		}
		res, err := state.Apply(g, st, seat, pick)
		if err != nil {
			t.Fatal(err)
		}
		st = res.NewState
	}
	d := data(t, st)
	if d.Phase != "over" || d.Winner != 0 || d.Reason != "humanity" {
		t.Fatalf("phase %q winner %d reason %q", d.Phase, d.Winner, d.Reason)
	}
}

func TestOnlyPlayersWithAllSixSpellbooksCanWin(t *testing.T) {
	g := game(t)
	// Black Goat leads on Doom with five books; Crawling Chaos has six.
	p := lastPass(pos{
		units: []u{{gc, "cultist", "south-pacific", 1}, {cc, "cultist", "asia", 1}},
		gates: map[string]int{"south-pacific": gc, "asia": cc},
		books: [5]uint8{0, 0, 63, 31, 0},
	})
	p.doom = [5]int{0, 0, 12, 31, 0}
	st := play(t, g, p.build(), 1, "pass")
	for i := 0; i < 20 && data(t, st).Phase != "over"; i++ {
		seat, _ := state.TurnAfter(g, st, 0)
		legal := state.LegalMoves(g, st, seat)
		pick := legal[0]
		for _, mv := range legal {
			switch mv.Action {
			case "skip", "cw", "first":
				pick = mv
			}
		}
		res, err := state.Apply(g, st, seat, pick)
		if err != nil {
			t.Fatal(err)
		}
		st = res.NewState
	}
	d := data(t, st)
	if d.Winner != cc {
		t.Fatalf("winner %d (doom %v)", d.Winner, d.Doom)
	}
}

func TestElderSignsStayHiddenInPublicViews(t *testing.T) {
	g := game(t)
	p := pos{
		units: []u{{gc, "cultist", "south-pacific", 1}},
		gates: map[string]int{"south-pacific": gc}, power: powers(8, 8, 8, 8),
		signs: map[int][]int{2: {3, 2}, 1: {1}},
	}
	st := p.build()
	pub := state.Public(g, st, 1)
	raw, err := json.Marshal(pub)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Ext struct {
			RNG   uint64  `json:"rng"`
			Bag   []int   `json:"bag"`
			Signs [][]int `json:"signs"`
			View  struct {
				Prompt string `json:"prompt"`
				Seats  map[string]struct {
					Counters []struct {
						ID    string `json:"id"`
						Value int    `json:"value"`
					} `json:"counters"`
				} `json:"seats"`
			} `json:"view"`
		} `json:"ext"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Ext.RNG != 0 {
		t.Error("the generator leaked")
	}
	if doc.Ext.Signs[1][0] != 1 {
		t.Errorf("a player sees their own Elder Signs: %v", doc.Ext.Signs)
	}
	for _, v := range doc.Ext.Signs[2] {
		if v != 0 {
			t.Errorf("another player's Elder Sign value leaked: %v", doc.Ext.Signs[2])
		}
	}
	if len(doc.Ext.Signs[2]) != 2 {
		t.Error("the number of Elder Signs is public")
	}
	for _, c := range doc.Ext.View.Seats["2"].Counters {
		if c.ID == "signs" && c.Value != 2 {
			t.Errorf("signs counter %d", c.Value)
		}
	}
}
