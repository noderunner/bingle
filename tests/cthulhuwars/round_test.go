package cthulhuwars_test

import (
	"strings"
	"testing"

	"github.com/noderunner/bingle/internal/engine/state"
)

// endOfActions: only seat 1 has Power left and passes, so the action phase
// ends and Power is gathered.
func lastPass(p pos) pos {
	p.power = powers(1, 0, 0, 0)
	return p
}

func TestGatherPowerCountsCultistsGatesAndAbandonedGates(t *testing.T) {
	g := game(t)
	p := lastPass(pos{
		units: []u{
			{gc, "cultist", "south-pacific", 3}, {gc, "deep-one", "south-pacific", 2},
			{cc, "cultist", "asia", 2},
			{bg, "cultist", "africa", 4},
			{ys, "cultist", "europe", 1},
		},
		gates: map[string]int{"south-pacific": gc, "asia": cc, "africa": bg, "europe": ys, "antarctica": 0},
	})
	st := play(t, g, p.build(), 1, "pass")
	d := data(t, st)
	// Cultists on the map + 2 per Gate + 1 per abandoned Gate (everyone).
	want := map[int]int{gc: 3 + 2 + 1, cc: 2 + 2 + 1, bg: 4 + 2 + 1, ys: 1 + 2 + 1}
	// The cults below half of the leader (bg: 7) are raised to 4.
	for seat, w := range want {
		if w < 4 {
			w = 4
		}
		if d.Power[seat] != w {
			t.Errorf("seat %d has %d Power, want %d", seat, d.Power[seat], w)
		}
	}
}

func TestCapturedCultistsGoHomeAndPayTheCaptor(t *testing.T) {
	g := game(t)
	p := lastPass(pos{
		units: []u{{gc, "deep-one", "asia", 1}, {cc, "cultist", "asia", 1}},
		gates: map[string]int{"asia": cc},
	})
	p.captured[gc][cc] = 2
	st := play(t, g, p.build(), 1, "pass")
	d := data(t, st)
	if d.Captured[gc][cc] != 0 {
		t.Fatalf("captured cultists must go home: %v", d.Captured)
	}
	if n := count(st, cc, "cultist", "asia"); n != 1 {
		t.Fatalf("the Cultist on the map: %d", n)
	}
	// Seat 1: 0 cultists + 0 gates + 2 returned captives.
	if d.Power[gc] < 2 {
		t.Fatalf("the captor earns 1 Power per returned Cultist: %d", d.Power[gc])
	}
	if err := state.Validate(g, st); err != nil {
		t.Fatal(err)
	}
}

func TestHalfTheLeadersPowerIsTheFloor(t *testing.T) {
	g := game(t)
	p := lastPass(pos{
		units: []u{{bg, "cultist", "africa", 6}, {ys, "cultist", "europe", 1}},
		gates: map[string]int{"africa": bg, "europe": ys},
	})
	st := play(t, g, p.build(), 1, "pass")
	d := data(t, st)
	// bg: 6 + 2 = 8 Power; the others are raised to 4.
	if d.Power[bg] != 8 || d.Power[gc] != 4 || d.Power[cc] != 4 || d.Power[ys] != 4 {
		t.Fatalf("power %v", d.Power)
	}
}

func TestMostPowerTakesFirstPlayerAndChoosesTheDirection(t *testing.T) {
	g := game(t)
	p := lastPass(pos{
		units: []u{{bg, "cultist", "africa", 6}, {ys, "cultist", "europe", 1}},
		gates: map[string]int{"africa": bg, "europe": ys},
	})
	st := play(t, g, p.build(), 1, "pass")
	d := data(t, st)
	if d.Stage != "direction" || d.First != bg {
		t.Fatalf("stage %q first %d", d.Stage, d.First)
	}
	if got := notations(g, st, bg); len(got) != 2 || !has(got, "cw") || !has(got, "ccw") {
		t.Fatalf("direction options = %v", got)
	}
	st = play(t, g, st, bg, "ccw")
	d = data(t, st)
	if d.Dir != -1 || d.Phase != "doom" {
		t.Fatalf("dir %d phase %q", d.Dir, d.Phase)
	}
}

func TestTiedPlayersLetThePreviousFirstPlayerChoose(t *testing.T) {
	g := game(t)
	p := lastPass(pos{
		units: []u{{gc, "cultist", "south-pacific", 3}, {cc, "cultist", "asia", 3}, {bg, "cultist", "africa", 1}, {ys, "cultist", "europe", 1}},
		gates: map[string]int{"south-pacific": gc, "asia": cc, "africa": bg, "europe": ys},
	})
	st := play(t, g, p.build(), 1, "pass")
	d := data(t, st)
	if d.Stage != "first" {
		t.Fatalf("stage %q, want a first-player choice", d.Stage)
	}
	got := notations(g, st, 1)
	if len(got) != 2 || !has(got, "first:gc") || !has(got, "first:cc") {
		t.Fatalf("options = %v", got)
	}
	st = play(t, g, st, 1, "first:cc")
	if d := data(t, st); d.First != cc || d.Stage != "direction" {
		t.Fatalf("first %d stage %q", d.First, d.Stage)
	}
}

func TestDoomPhaseScoresGatesAndRitualsCostMoreEachTime(t *testing.T) {
	g := game(t)
	// Come into the Doom phase through a real Gather/First/direction.
	p := lastPass(pos{
		units: []u{
			{gc, "cultist", "south-pacific", 2}, {gc, "cthulhu", "south-pacific", 1},
			{cc, "cultist", "asia", 2}, {cc, "cultist", "africa", 1},
			{bg, "cultist", "europe", 1}, {ys, "cultist", "south-america-west", 1},
		},
		gates: map[string]int{"south-pacific": gc, "asia": cc, "africa": cc, "europe": bg, "south-america-west": ys},
	})
	st := play(t, g, p.build(), 1, "pass")
	d := data(t, st)
	// Power: gc 2+2=4, cc 3+4=7, bg 1+2=3→4, ys 1+2=3→4. Most Power: cc.
	if d.First != cc {
		t.Fatalf("first player %d", d.First)
	}
	st = play(t, g, st, cc, "cw")
	d = data(t, st)
	// One Doom per controlled Gate.
	want := map[int]int{gc: 1, cc: 2, bg: 1, ys: 1}
	for s, w := range want {
		if d.Doom[s] != w {
			t.Errorf("seat %d Doom %d, want %d", s, d.Doom[s], w)
		}
	}
	// Great Cthulhu receives a spellbook in the first Doom phase.
	if d.Stage != "prompt" || d.Prompts[0].Kind != "book" || d.Prompts[0].Seat != gc {
		t.Fatalf("stage %q prompts %+v", d.Stage, d.Prompts)
	}
	st = play(t, g, st, gc, "book:absorb")
	d = data(t, st)
	// ... and 1 Elder Sign with it.
	if len(d.Signs[gc]) != 1 {
		t.Fatalf("the first-Doom requirement gives an Elder Sign: %v", d.Signs[gc])
	}
	// Players decide in turn order starting with the first player (cc).
	if d.Stage != "doom" {
		t.Fatalf("stage %q", d.Stage)
	}
	got := notations(g, st, cc)
	if !has(got, "ritual") || !has(got, "skip") {
		t.Fatalf("options = %v", got)
	}
	st = play(t, g, st, cc, "ritual")
	d = data(t, st)
	// 5 Power paid, +2 Doom (2 Gates), no GOO so no Elder Signs.
	if d.Power[cc] != 2 || d.Doom[cc] != 4 || len(d.Signs[cc]) != 0 {
		t.Fatalf("after the ritual: power %d doom %d signs %v", d.Power[cc], d.Doom[cc], d.Signs[cc])
	}
	if d.Ritual != 1 {
		t.Fatalf("ritual marker %d", d.Ritual)
	}
	// Nobody else can afford a Ritual, but Great Cthulhu holds an Elder Sign it
	// may reveal: its decision comes last, and when it is made round 2
	// begins with the first player.
	if d.Phase != "doom" || d.Stage != "doom" {
		t.Fatalf("phase %q stage %q", d.Phase, d.Stage)
	}
	st = play(t, g, st, gc, "skip")
	d = data(t, st)
	if d.Phase != "action" || d.Round != 2 || d.Turn.Seat != cc {
		t.Fatalf("phase %q round %d turn %+v", d.Phase, d.Round, d.Turn)
	}
}

// The History log says what each Doom total is made of: a player who was
// given 3 Doom and then scores 3 Gates has 6, and the log shows both parts.
func TestTheDoomPhaseLogExplainsWhereTheDoomComesFrom(t *testing.T) {
	g := game(t)
	p := lastPass(pos{
		units: []u{
			{gc, "cultist", "south-pacific", 1}, {gc, "cultist", "new-zealand", 1}, {gc, "cultist", "antarctica", 1},
			{cc, "cultist", "asia", 1}, {bg, "cultist", "africa", 1}, {ys, "cultist", "europe", 1},
		},
		gates: map[string]int{"south-pacific": gc, "new-zealand": gc, "antarctica": gc, "asia": cc, "africa": bg, "europe": ys},
		doom:  [5]int{0, 3, 0, 0, 0},
	})
	st := p.build()
	// Yellow Sign's gift of 3 Doom was recorded when it was made.
	data(t, st).DoomBy[gc].Gifts = 3
	st = play(t, g, st, 1, "pass")
	d := data(t, st)
	st = play(t, g, st, d.First, "cw")
	log := lastLines(data(t, st))
	for _, want := range []string{
		"Great Cthulhu scores 3 Doom for 3 Gates: 3 → 6 on the Doom track (3 from Gates, 3 from gifts).",
		"Crawling Chaos scores 1 Doom for 1 Gate: 0 → 1 on the Doom track (1 from Gates).",
	} {
		if !strings.Contains(log, want) {
			t.Errorf("the log lacks %q:\n%s", want, log)
		}
	}
	if d := data(t, st); d.Doom[gc] != 6 {
		t.Errorf("Great Cthulhu has %d Doom, want 6", d.Doom[gc])
	}
	// The seat's Doom counter carries the same explanation.
	v := viewFor(t, g, st, 0)
	for _, c := range v.Ext.View.Seats["1"].Counters {
		if c.ID == "doom" && c.Note != "3 from Gates, 3 from gifts" {
			t.Errorf("the Doom counter says %q", c.Note)
		}
	}
}
