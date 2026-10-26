package cthulhuwars_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/games/cthulhuwars"
)

// What the page needs to show a price and a card on every button, and where each
// unit of a cult is.

type cardsView struct {
	Costs map[string]int `json:"costs"`
	Seats map[string]struct {
		Units []struct {
			ID     string   `json:"id"`
			Cost   int      `json:"cost"`
			Total  int      `json:"total"`
			Pool   int      `json:"pool"`
			OnMap  int      `json:"on_map"`
			Enter  string   `json:"enter"`
			Places []string `json:"places"`
			Awaken []string `json:"awaken"`

			Abilities []struct {
				Name   string `json:"name"`
				Timing string `json:"timing"`
				Text   string `json:"text"`
			} `json:"abilities"`
		} `json:"units"`
		Counters []struct {
			ID    string `json:"id"`
			Value int    `json:"value"`
		} `json:"counters"`
	} `json:"seats"`
}

func publicViewOf(t *testing.T, tb *table, st *state.BoardState, viewer string) cardsView {
	t.Helper()
	raw, err := json.Marshal(state.Public(tb.g, st, tb.seat(viewer)))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Ext struct {
			View cardsView `json:"view"`
		} `json:"ext"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	return doc.Ext.View
}

func TestTheViewPricesEveryActionButton(t *testing.T) {
	tb := newTable(t, "cthulhu-wars", []string{"gc", "cc", "bg", "ys"}, true)
	st := tb.build(xp{
		units: []xu{{"gc", "cultist", "south-pacific", 2}, {"gc", "cultist", "new-zealand", 1}, {"cc", "cultist", "asia", 1}},
		gates: map[string]string{"south-pacific": "gc", "asia": "cc"},
		power: map[string]int{"gc": 10}, turn: "gc",
	})
	v := publicViewOf(t, tb, st, "gc")
	want := map[string]int{
		"recruit":             1,
		"recruit-high-priest": 3,
		"gate":                3, // the Cultist in New Zealand builds one
		"summon-deep-one":     1,
		"summon-shoggoth":     2,
		"summon-starspawn":    3,
		"awaken-cthulhu":      10,
		"pass":                0,
	}
	for id, cost := range want {
		if got, ok := v.Costs[id]; !ok || got != cost {
			t.Errorf("cost of %s = %d (listed %v), want %d (all: %v)", id, got, ok, cost, v.Costs)
		}
	}
	// Only the buttons that are on offer are priced: the Black Goat's are not.
	if _, ok := v.Costs["summon-ghoul"]; ok {
		t.Errorf("a monster of another cult is priced: %v", v.Costs)
	}
	// The Ice Age surcharge is part of the price of what ends in its area.
	st = tb.build(xp{
		units: []xu{{"gc", "cultist", "south-pacific", 2}, {"gc", "cultist", "new-zealand", 1}, {"cc", "cultist", "asia", 1}},
		gates: map[string]string{"south-pacific": "gc", "asia": "cc"},
		power: map[string]int{"gc": 10}, turn: "gc",
		edit: func(d *cthulhuwars.Data) { d.Ice = "new-zealand" },
	})
	if v := publicViewOf(t, tb, st, "gc"); v.Costs["gate"] != 4 {
		t.Errorf("a Gate in the Ice Age area costs %d, want 4", v.Costs["gate"])
	}
}

func TestEveryGreatOldOneHasAnAwakenButtonWithItsOwnSteps(t *testing.T) {
	tb := newTable(t, "cthulhu-wars", []string{"gc", "cc", "bg", "ys"}, false)
	st := tb.build(xp{
		units: []xu{{"gc", "cultist", "south-pacific", 1}},
		gates: map[string]string{"south-pacific": "gc"}, power: map[string]int{"gc": 10}, turn: "gc",
	})
	mv, err := state.ParseMove(tb.g, "awaken:cthulhu@south-pacific")
	if err != nil {
		t.Fatal(err)
	}
	steps := state.Steps(tb.g, mv)
	if len(steps) != 2 || steps[0].Kind != state.StepAction || steps[0].Value != "awaken-cthulhu" || steps[1].Kind != state.StepPos || steps[1].Value != "south-pacific" {
		t.Fatalf("steps = %+v", steps)
	}
	// The button has a name and a rule (the card shown on hover).
	info, _ := state.DescribeRuleset(tb.g)
	var found bool
	for _, a := range info.Actions {
		if a.ID == "awaken-cthulhu" {
			found = true
			if a.Label != "Awaken Cthulhu" || !strings.Contains(a.Hint, "starting area") {
				t.Errorf("label %q hint %q", a.Label, a.Hint)
			}
		}
	}
	if !found {
		t.Fatal("no awaken-cthulhu action is described")
	}
	for _, id := range []string{"awaken-nyarlathotep", "awaken-shub-niggurath", "awaken-king-in-yellow", "awaken-hastur", "awaken-rhan-tegoth", "awaken-ithaqua", "awaken-tsathoggua"} {
		ok := false
		for _, a := range info.Actions {
			ok = ok || a.ID == id
		}
		if !ok {
			t.Errorf("no %s action is described", id)
		}
	}
	// The same position offers the move under that action.
	if got := tb.can(st, "gc"); !has(got, "awaken:cthulhu@south-pacific") {
		t.Fatalf("moves: %v", got)
	}
}

func TestEveryActionButtonCarriesItsRule(t *testing.T) {
	tb := newTable(t, "cthulhu-wars-5p", []string{"gc", "cc", "ww", "sl", "bg"}, true)
	info, _ := state.DescribeRuleset(tb.g)
	hints := map[string]string{}
	for _, a := range info.Actions {
		hints[a.ID] = a.Hint
	}
	// The ones the player asked about by name, and a sample of the abilities.
	for _, id := range []string{
		"gate", "recruit", "recruit-high-priest", "battle", "capture", "submerge", "dreams", "forms", "ghroth", "shriek",
		"iceage", "hibernate", "sorcery", "slumber", "wake", "boon", "bane", "oath", "reveal", "ritual", "avatar",
		"book-absorb", "book-ice-age", "book-energy-nexus",
	} {
		h, ok := hints[id]
		if !ok || len(strings.TrimSpace(h)) < 20 {
			t.Errorf("action %q has no rule text: %q", id, h)
		}
	}
	// Names the player asked for.
	for _, a := range info.Actions {
		switch a.ID {
		case "gate":
			if a.Label != "Build Gate" || !strings.Contains(a.Hint, "one Gate to an area") {
				t.Errorf("gate: %q / %q", a.Label, a.Hint)
			}
		case "recruit-high-priest":
			if a.Label != "Recruit High Priest" || !strings.Contains(a.Hint, "Unspeakable Oath") {
				t.Errorf("high priest: %q / %q", a.Label, a.Hint)
			}
		}
	}
}

func TestEachUnitThatIsNotInThePoolSaysWhereItIs(t *testing.T) {
	tb := newTable(t, "cthulhu-wars", []string{"gc", "cc", "bg", "ys"}, true)
	st := tb.build(xp{
		units: []xu{{"gc", "cultist", "south-pacific", 3}, {"gc", "cultist", "asia", 1}, {"gc", "deep-one", "south-pacific", 1}, {"cc", "cultist", "asia", 1}},
		gates: map[string]string{"south-pacific": "gc"}, power: map[string]int{"gc": 5}, turn: "gc",
		pool: map[string]int{"gc|cultist": 1},
		edit: func(d *cthulhuwars.Data) {
			d.Holders = map[string]string{"south-pacific": "cultist"}
			d.Captured[2][1] = 1 // Crawling Chaos holds a Cultist of Great Cthulhu
		},
	})
	v := publicViewOf(t, tb, st, "gc")
	var got []string
	for _, u := range v.Seats["1"].Units {
		switch u.ID {
		case "cultist":
			got = u.Places
			if u.Total != 6 || u.Pool != 1 || u.OnMap != 4 {
				t.Errorf("cultists: total %d pool %d on map %d", u.Total, u.Pool, u.OnMap)
			}
			if !strings.Contains(u.Enter, "already have a unit") {
				t.Errorf("enter = %q", u.Enter)
			}
		case "deep-one":
			if len(u.Places) != 1 || u.Places[0] != "in South Pacific" {
				t.Errorf("deep one places %v", u.Places)
			}
			if !strings.Contains(u.Enter, "Gate you control") {
				t.Errorf("enter = %q", u.Enter)
			}
		}
	}
	// Five away: the one on the Gate first, then the rest of the map, then the
	// one the enemy holds.
	want := []string{"in Asia", "on the Gate in South Pacific", "in South Pacific", "in South Pacific", "captured by Crawling Chaos"}
	// (areas are listed in the order of the map)
	if len(got) != len(want) {
		t.Fatalf("places = %v", got)
	}
	has := func(s string) int {
		n := 0
		for _, g := range got {
			if g == s {
				n++
			}
		}
		return n
	}
	if has("on the Gate in South Pacific") != 1 || has("in South Pacific") != 2 || has("in Asia") != 1 || has("captured by Crawling Chaos") != 1 {
		t.Errorf("places = %v, want %v", got, want)
	}
}

// Every Great Old One's card lists the ability it has besides fighting, with
// its timing: Desecrate and Vengeance among them.
func TestEveryGreatOldOneCardListsItsAbilities(t *testing.T) {
	want := map[string][2]string{
		"cthulhu":        {"Devour", "Pre-Battle"},
		"nyarlathotep":   {"The Harbinger", "Post-Battle"},
		"shub-niggurath": {"Avatar", "Action: Cost 1"},
		"king-in-yellow": {"Desecrate", "Action: Cost 2"},
		"hastur":         {"Vengeance", "Post-Battle"},
		"rhan-tegoth":    {"Eternal", "Post-Battle"},
		"ithaqua":        {"Ferox", "Ongoing"},
		"tsathoggua":     {"Lethargy", "Action: Cost 0"},
	}
	found := map[string]bool{}
	for _, keys := range [][]string{{"gc", "cc", "bg", "ys", "ww"}, {"sl", "cc", "bg", "ys", "ww"}} {
		tb := newTable(t, "cthulhu-wars-5p", keys, false)
		st := tb.build(xp{turn: keys[0]})
		for i := range keys {
			v := publicViewOf(t, tb, st, keys[0])
			for _, u := range v.Seats[fmt.Sprint(i+1)].Units {
				w, isGOO := want[u.ID]
				if !isGOO {
					if len(u.Abilities) != 0 {
						t.Errorf("%s lists abilities: %+v", u.ID, u.Abilities)
					}
					continue
				}
				found[u.ID] = true
				if len(u.Abilities) != 1 || u.Abilities[0].Name != w[0] || u.Abilities[0].Timing != w[1] || len(u.Abilities[0].Text) < 30 {
					t.Errorf("%s: abilities %+v, want %s (%s)", u.ID, u.Abilities, w[0], w[1])
				}
				if len(u.Awaken) < 2 {
					t.Errorf("%s: no steps of awakening: %v", u.ID, u.Awaken)
				}
			}
		}
	}
	for id := range want {
		if !found[id] {
			t.Errorf("%s is on no card", id)
		}
	}
}

// The King's rule is the card's: a roll against the units in his area, and a
// Monster or Cultist of cost 2 or less placed whatever the roll.
func TestTheDesecrateCardSaysWhatTheCardSays(t *testing.T) {
	tb := newTable(t, "cthulhu-wars", []string{"gc", "cc", "bg", "ys"}, false)
	v := publicViewOf(t, tb, tb.start(1), "ys")
	for _, u := range v.Seats["4"].Units {
		if u.ID != "king-in-yellow" {
			continue
		}
		txt := u.Abilities[0].Text
		for _, w := range []string{"roll 1 die", "including the King", "Whether you succeed or fail", "cost of 2 or less"} {
			if !strings.Contains(txt, w) {
				t.Errorf("Desecrate lacks %q: %s", w, txt)
			}
		}
		return
	}
	t.Fatal("no King in Yellow")
}

// Yellow Sign counts its Desecration tokens, and only Yellow Sign does.
func TestOnlyYellowSignCountsDesecrationTokens(t *testing.T) {
	tb := newTable(t, "cthulhu-wars", []string{"gc", "cc", "bg", "ys"}, false)
	st := tb.build(xp{
		units: []xu{{"ys", "king-in-yellow", "asia", 1}},
		turn:  "ys", power: map[string]int{"ys": 4},
		edit: func(d *cthulhuwars.Data) { d.Desec = map[string]bool{"asia": true, "europe": true} },
	})
	v := publicViewOf(t, tb, st, "ys")
	first := func(seat string) (string, int) {
		c := v.Seats[seat].Counters
		if len(c) == 0 {
			return "", 0
		}
		return c[0].ID, c[0].Value
	}
	if id, n := first("4"); id != "desecrations" || n != 2 {
		t.Errorf("Yellow Sign's first number is %q = %d, want desecrations = 2", id, n)
	}
	for _, seat := range []string{"1", "2", "3"} {
		if id, _ := first(seat); id != "power" {
			t.Errorf("seat %s starts with %q", seat, id)
		}
	}
}
