package cthulhuwars_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

// viewDoc is the part of a public position these tests read: the derived
// view, the private and public Elder Sign lists and the event log.
type viewDoc struct {
	Ext struct {
		Signs  [][]int  `json:"signs"`
		Shown  [][]int  `json:"shown"`
		Events []string `json:"events"`
		Log    []struct {
			Ply   int      `json:"ply"`
			Seat  int      `json:"seat"`
			Lines []string `json:"lines"`
		} `json:"log"`
		View struct {
			Prompt string `json:"prompt"`
			Phases []struct {
				ID    string `json:"id"`
				Label string `json:"label"`
				State string `json:"state"`
			} `json:"phases"`
			Order struct {
				CW  []int `json:"cw"`
				CCW []int `json:"ccw"`
			} `json:"order"`
			Bag struct {
				Left  int `json:"left"`
				Total int `json:"total"`
			} `json:"bag"`
			Seats map[string]struct {
				Counters []struct {
					ID    string `json:"id"`
					Value int    `json:"value"`
					Note  string `json:"note"`
				} `json:"counters"`
				Hidden int   `json:"hidden_signs"`
				Shown  []int `json:"shown_signs"`
				Signs  []int `json:"signs"`
				Units  []struct {
					ID       string `json:"id"`
					Name     string `json:"name"`
					Class    string `json:"class"`
					Cost     int    `json:"cost"`
					Combat   string `json:"combat"`
					Total    int    `json:"total"`
					Pool     int    `json:"pool"`
					OnMap    int    `json:"on_map"`
					Away     int    `json:"away"`
					Captured int    `json:"captured"`
					Enhanced bool   `json:"enhanced"`
					Books    []struct {
						ID       string `json:"id"`
						Name     string `json:"name"`
						Unlocked bool   `json:"unlocked"`
					} `json:"books"`
				} `json:"units"`
			} `json:"seats"`
		} `json:"view"`
	} `json:"ext"`
}

func viewFor(t testing.TB, g *catalog.CompiledGame, st *state.BoardState, viewer int) viewDoc {
	t.Helper()
	raw, err := json.Marshal(state.Public(g, st, viewer))
	if err != nil {
		t.Fatal(err)
	}
	var doc viewDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

// --- Gates are taken by themselves ----------------------------------------------

func TestACultistMovingIntoAnAbandonedGateTakesControlAtOnce(t *testing.T) {
	g := game(t)
	p := pos{
		units: []u{{gc, "cultist", "south-pacific", 2}},
		gates: map[string]int{"south-pacific": gc, "new-zealand": 0},
		power: powers(8, 8, 8, 8),
	}
	st := play(t, g, p.build(), 1, "go:cultist@south-pacific>new-zealand")
	d := data(t, st)
	if d.Gates["new-zealand"] != gc {
		t.Fatalf("the Gate should be ours: %v", d.Gates)
	}
	if d.Power[gc] != 7 {
		t.Fatalf("taking a Gate is free: power %d", d.Power[gc])
	}
	if got := strings.Join(d.Events, "\n"); !strings.Contains(got, "takes control of the Gate in New Zealand") {
		t.Fatalf("events: %v", d.Events)
	}
	// The Gate left behind still has a Cultist.
	if d.Gates["south-pacific"] != gc {
		t.Fatalf("gates %v", d.Gates)
	}
}

func TestAMonsterArrivingAtAnAbandonedGateLeavesItAlone(t *testing.T) {
	g := game(t)
	p := pos{
		units: []u{{gc, "deep-one", "south-pacific", 1}, {gc, "cultist", "south-pacific", 1}},
		gates: map[string]int{"south-pacific": gc, "new-zealand": 0},
		power: powers(8, 8, 8, 8),
	}
	st := play(t, g, p.build(), 1, "go:deep-one@south-pacific>new-zealand")
	if got := data(t, st).Gates["new-zealand"]; got != 0 {
		t.Fatalf("monsters do not control Gates: %d", got)
	}
}

func TestRecruitingAtAnAbandonedGateTakesControlToo(t *testing.T) {
	g := game(t)
	p := pos{
		units: []u{{gc, "cultist", "new-zealand", 1}},
		gates: map[string]int{"new-zealand": 0},
		power: powers(8, 8, 8, 8),
	}
	st := play(t, g, p.build(), 1, "recruit@new-zealand")
	if got := data(t, st).Gates["new-zealand"]; got != gc {
		t.Fatalf("gate controller %d", got)
	}
}

func TestAGateGivenUpThisTurnIsNotTakenBackByArriving(t *testing.T) {
	g := game(t)
	p := pos{
		units: []u{{gc, "cultist", "new-zealand", 1}, {gc, "cultist", "south-pacific", 1}},
		gates: map[string]int{"new-zealand": gc, "south-pacific": gc},
		power: powers(8, 8, 8, 8),
	}
	st := play(t, g, p.build(), 1, "leave:cultist@new-zealand>new-zealand")
	st = play(t, g, st, 1, "leave:cultist@south-pacific>new-zealand")
	if got := data(t, st).Gates["new-zealand"]; got != 0 {
		t.Fatalf("a Gate abandoned on purpose stays abandoned this turn: %d", got)
	}
}

func TestDreamsAndEmergeTakeAnAbandonedGateToo(t *testing.T) {
	g := game(t)
	// Dreams: the enemy's only Cultist is replaced, the Gate falls to us.
	p := pos{
		units: []u{{gc, "cultist", "south-pacific", 1}, {cc, "cultist", "asia", 1}},
		gates: map[string]int{"south-pacific": gc, "asia": cc},
		power: powers(8, 8, 8, 8), books: [5]uint8{0, books(bkDreams)},
	}
	st := play(t, g, p.build(), 1, "dreams@asia/cc")
	d := data(t, st)
	if d.Gates["asia"] != gc || count(st, gc, "cultist", "asia") != 1 || count(st, cc, "cultist", "asia") != 0 {
		t.Fatalf("gates %v", d.Gates)
	}
	// Emerge into an area with an abandoned Gate.
	q := pos{
		units: []u{{gc, "cultist", "south-pacific", 1}},
		gates: map[string]int{"new-zealand": 0},
		power: powers(8, 8, 8, 8), books: [5]uint8{0, books(bkSubmerge)},
		away: map[string]int{"cthulhu": 1, "cultist": 1},
	}
	st = play(t, g, q.build(), 1, "emerge@new-zealand")
	if got := data(t, st).Gates["new-zealand"]; got != gc {
		t.Fatalf("gate controller %d", got)
	}
}

// --- Elder Signs ----------------------------------------------------------------------

func TestRevealTurnsEveryElderSignOverAtOnce(t *testing.T) {
	g := game(t)
	p := pos{
		units: []u{{gc, "cultist", "south-pacific", 1}},
		gates: map[string]int{"south-pacific": gc},
		power: powers(8, 8, 8, 8),
		signs: map[int][]int{1: {2, 3, 1}, 2: {1}},
	}
	st := p.build()
	got := notations(g, st, 1)
	if !has(got, "reveal") || hasPrefix(got, "reveal/") {
		t.Fatalf("one action reveals them all: %v", got)
	}
	st = play(t, g, st, 1, "reveal")
	d := data(t, st)
	if d.Doom[1] != 6 || len(d.Signs[1]) != 0 {
		t.Fatalf("doom %d signs %v", d.Doom[1], d.Signs[1])
	}
	if len(d.Shown[1]) != 3 {
		t.Fatalf("revealed signs are kept face up: %v", d.Shown[1])
	}
	if hasPrefix(notations(g, st, 1), "reveal") {
		t.Fatal("nothing left to reveal")
	}
	// Everyone sees what was revealed; the other seat's hidden sign stays
	// hidden.
	v := viewFor(t, g, st, 3)
	s1 := v.Ext.View.Seats["1"]
	if len(s1.Shown) != 3 || s1.Hidden != 0 {
		t.Fatalf("public view of seat 1: %+v", s1)
	}
	s2 := v.Ext.View.Seats["2"]
	if s2.Hidden != 1 || len(s2.Signs) != 0 {
		t.Fatalf("public view of seat 2: %+v", s2)
	}
	if got := strings.Join(d.Events, " "); !strings.Contains(got, "reveals 3 Elder Signs: 2, 3, 1 — 6 Doom") {
		t.Fatalf("events: %v", d.Events)
	}
	// The legacy "reveal/N" notation still means the same thing.
	if mv, err := state.ParseMove(g, "reveal/2"); err != nil || state.FormatMove(g, mv) != "reveal" {
		t.Fatalf("legacy notation: %+v %v", mv, err)
	}
}

func TestEveryPlayerSeesTheirOwnElderSignValuesAndNobodyElses(t *testing.T) {
	g := game(t)
	p := pos{
		units: []u{{gc, "cultist", "south-pacific", 1}},
		gates: map[string]int{"south-pacific": gc},
		power: powers(8, 8, 8, 8),
		signs: map[int][]int{1: {3, 1}, 2: {2}},
	}
	st := p.build()
	own := viewFor(t, g, st, 1).Ext.View.Seats["1"]
	if len(own.Signs) != 2 || own.Signs[0] != 3 || own.Signs[1] != 1 || own.Hidden != 2 {
		t.Fatalf("own view: %+v", own)
	}
	other := viewFor(t, g, st, 2).Ext.View.Seats["1"]
	if len(other.Signs) != 0 || other.Hidden != 2 {
		t.Fatalf("seat 2 must see only the count of seat 1's signs: %+v", other)
	}
	spectator := viewFor(t, g, st, 0).Ext.View.Seats["2"]
	if len(spectator.Signs) != 0 || spectator.Hidden != 1 {
		t.Fatalf("spectator view: %+v", spectator)
	}
}

func TestTheBagCountIsPublicAndFallsAsSignsAreDrawn(t *testing.T) {
	g := game(t)
	st := state.InitialState(g)
	b := viewFor(t, g, st, 0).Ext.View.Bag
	if b.Left != 36 || b.Total != 36 {
		t.Fatalf("bag %+v", b)
	}
	p := inDoom(pos{
		units: []u{{gc, "cthulhu", "south-pacific", 1}, {gc, "cultist", "south-pacific", 1}},
		gates: map[string]int{"south-pacific": gc},
		power: powers(9, 0, 0, 0), reqs: [5]uint8{0, 0xff},
	})
	st = play(t, g, p.build(), 1, "ritual")
	if b := viewFor(t, g, st, 0).Ext.View.Bag; b.Left != 35 || b.Total != 36 {
		t.Fatalf("one Elder Sign was drawn: %+v", b)
	}
}

// --- the round, the order of play --------------------------------------------------------

func phaseStates(v viewDoc) string {
	var out []string
	for _, p := range v.Ext.View.Phases {
		out = append(out, p.ID+"="+p.State)
	}
	return strings.Join(out, " ")
}

func TestThePhaseBarListsTheRoundInRuleBookOrder(t *testing.T) {
	g := game(t)
	st := state.InitialState(g)
	v := viewFor(t, g, st, 0)
	// Before the first Action phase the first player chooses the direction of
	// play; the Doom phase is skipped in that first round.
	if got := phaseStates(v); got != "action=todo gather=done first=current doom=todo" {
		t.Fatalf("phases at the start: %s", got)
	}
	st = play(t, g, st, 1, "cw")
	v = viewFor(t, g, st, 0)
	if got := phaseStates(v); got != "action=current gather=todo first=todo doom=todo" {
		t.Fatalf("phases in the first Action phase: %s", got)
	}
	labels := ""
	for _, p := range v.Ext.View.Phases {
		labels += p.Label + ";"
	}
	if labels != "Action;Gather Power;First Player;Doom;" {
		t.Fatalf("labels %q", labels)
	}

	// The Action phase ends: Power is gathered at once and the players wait
	// at the choice of the direction.
	p := lastPass(pos{
		units: []u{{bg, "cultist", "africa", 6}, {ys, "cultist", "europe", 1}},
		gates: map[string]int{"africa": bg, "europe": ys},
	})
	st = play(t, g, p.build(), 1, "pass")
	if got := phaseStates(viewFor(t, g, st, 0)); got != "action=done gather=done first=current doom=todo" {
		t.Fatalf("waiting for the direction: %s", got)
	}
	st = play(t, g, st, bg, "cw")
	if got := phaseStates(viewFor(t, g, st, 0)); !strings.HasPrefix(got, "action=done gather=done first=done doom=current") {
		t.Fatalf("in the Doom phase: %s", got)
	}
}

func TestTheDirectionChoiceNamesWhoPlaysNextEachWay(t *testing.T) {
	g := game(t)
	p := lastPass(pos{
		units: []u{{bg, "cultist", "africa", 6}, {ys, "cultist", "europe", 1}},
		gates: map[string]int{"africa": bg, "europe": ys},
	})
	st := play(t, g, p.build(), 1, "pass")
	v := viewFor(t, g, st, bg)
	prompt := v.Ext.View.Prompt
	for _, want := range []string{"clockwise", "Black Goat → Yellow Sign → Great Cthulhu → Crawling Chaos", "counter-clockwise", "Black Goat → Crawling Chaos → Great Cthulhu → Yellow Sign"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt lacks %q: %s", want, prompt)
		}
	}
	o := v.Ext.View.Order
	if len(o.CW) != 4 || o.CW[0] != bg || o.CW[1] != ys || o.CW[2] != gc || o.CW[3] != cc {
		t.Errorf("clockwise order %v", o.CW)
	}
	if len(o.CCW) != 4 || o.CCW[0] != bg || o.CCW[1] != cc || o.CCW[2] != gc || o.CCW[3] != ys {
		t.Errorf("counter-clockwise order %v", o.CCW)
	}
	st = play(t, g, st, bg, "ccw")
	if got := strings.Join(data(t, st).Events, "\n"); !strings.Contains(got, "chooses to play counter-clockwise: Black Goat → Crawling Chaos → Great Cthulhu → Yellow Sign") {
		t.Fatalf("events: %v", got)
	}
}

// --- the log -------------------------------------------------------------------------------

func TestTheLogKeepsEverythingSinceEachSeatsLastMove(t *testing.T) {
	g := game(t)
	p := pos{
		units: []u{
			{gc, "cultist", "south-pacific", 1}, {cc, "cultist", "asia", 1},
			{bg, "cultist", "africa", 1}, {ys, "cultist", "europe", 1},
		},
		gates: map[string]int{"south-pacific": gc, "asia": cc, "africa": bg, "europe": ys},
		power: powers(8, 8, 8, 8),
	}
	st := p.build()
	st = play(t, g, st, 1, "recruit@south-pacific") // ply 1
	st = play(t, g, st, 2, "recruit@asia")          // ply 2
	st = play(t, g, st, 3, "recruit@africa")        // ply 3
	// Black Goat may still summon for free, so its turn waits for "done".
	st = play(t, g, st, 3, "done") // ply 4

	log := viewFor(t, g, st, 0).Ext.Log
	if len(log) != 4 {
		t.Fatalf("until every seat has played a full turn nothing is old enough to drop: %d entries", len(log))
	}
	if log[0].Seat != 1 || len(log[0].Lines) == 0 || !strings.Contains(log[0].Lines[0], "recruits a Cultist") {
		t.Fatalf("first entry %+v", log[0])
	}
	st = play(t, g, st, 4, "recruit@europe")        // ply 5
	st = play(t, g, st, 1, "recruit@south-pacific") // ply 6

	log = viewFor(t, g, st, 0).Ext.Log
	// Seat 1's own turn is under way: what it has not seen yet is everything
	// since its decision at ply 1.
	if log[0].Ply != 1 || log[0].Seat != 1 || log[len(log)-1].Ply != 6 {
		t.Fatalf("log from %d to %d: %+v", log[0].Ply, log[len(log)-1].Ply, log)
	}
	st = play(t, g, st, 2, "recruit@asia") // ply 7
	log = viewFor(t, g, st, 0).Ext.Log
	// Now seat 1 has been followed by seat 2: its anchor is ply 6, seat 2's
	// is ply 2 — the oldest.
	if log[0].Ply != 2 || log[0].Seat != 2 || log[len(log)-1].Ply != 7 {
		t.Fatalf("log from %d to %d", log[0].Ply, log[len(log)-1].Ply)
	}
	for i := 1; i < len(log); i++ {
		if log[i].Ply <= log[i-1].Ply {
			t.Fatalf("log out of order: %+v", log)
		}
	}
	// Whoever's decision it was, it is the log of the position itself: a
	// replay of the same moves gives the same log.
	if got := data(t, st).Log; len(got) != len(log) {
		t.Fatalf("log in the data %d, in the view %d", len(got), len(log))
	}
}

// --- what a unit's card says ------------------------------------------------------------------

func TestUnitCardsListThePoolAndTheBooksThatBearOnEachUnit(t *testing.T) {
	g := game(t)
	p := pos{
		units: []u{{gc, "cultist", "south-pacific", 2}, {gc, "shoggoth", "south-pacific", 1}},
		gates: map[string]int{"south-pacific": gc},
		power: powers(8, 8, 8, 8),
		books: [5]uint8{0, books(bkAbsorb)},
	}
	v := viewFor(t, g, p.build(), 0)
	type unit = struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Class    string `json:"class"`
		Cost     int    `json:"cost"`
		Combat   string `json:"combat"`
		Total    int    `json:"total"`
		Pool     int    `json:"pool"`
		OnMap    int    `json:"on_map"`
		Away     int    `json:"away"`
		Captured int    `json:"captured"`
		Enhanced bool   `json:"enhanced"`
		Books    []struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			Unlocked bool   `json:"unlocked"`
		} `json:"books"`
	}
	by := map[string]unit{}
	for _, x := range v.Ext.View.Seats["1"].Units {
		by[x.ID] = x
	}
	sh := by["shoggoth"]
	if sh.Class != "monster" || sh.Cost != 2 || sh.Combat != "2" || sh.Total != 2 || sh.Pool != 1 || sh.OnMap != 1 {
		t.Fatalf("shoggoth card %+v", sh)
	}
	if !sh.Enhanced || len(sh.Books) != 1 || sh.Books[0].ID != "absorb" || !sh.Books[0].Unlocked {
		t.Fatalf("Absorb is unlocked and enhances the Shoggoth: %+v", sh)
	}
	ss := by["starspawn"]
	if ss.Enhanced || len(ss.Books) != 1 || ss.Books[0].Unlocked {
		t.Fatalf("Regenerate is not unlocked: %+v", ss)
	}
	if c := by["cultist"]; c.Class != "cultist" || c.Pool != 4 || c.OnMap != 2 || c.Total != 6 {
		t.Fatalf("cultist card %+v", c)
	}
	if ct := by["cthulhu"]; ct.Class != "goo" || ct.Cost != 10 || ct.Combat != "6" {
		t.Fatalf("Cthulhu card %+v", ct)
	}
	// Computed Combat is explained with its current value.
	ny := v.Ext.View.Seats["2"].Units
	for _, x := range ny {
		if x.ID == "nyarlathotep" && !strings.Contains(x.Combat, "spellbook") {
			t.Fatalf("Nyarlathotep's Combat: %q", x.Combat)
		}
	}
}

func TestEveryEnhancingBookNamesRealUnitsOfItsFaction(t *testing.T) {
	g := game(t)
	v := viewFor(t, g, state.InitialState(g), 0)
	for seat, sv := range v.Ext.View.Seats {
		bookUnits := map[string]bool{}
		for _, u := range sv.Units {
			for _, b := range u.Books {
				bookUnits[b.ID] = true
			}
		}
		if len(bookUnits) < 5 {
			t.Errorf("seat %s: only %d spellbooks bear on a unit", seat, len(bookUnits))
		}
	}
}
