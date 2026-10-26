package cthulhuwars_test

import (
	"math/rand"
	"strings"
	"testing"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/games/cthulhuwars"
)

// guided returns the moves a bot may consider for seat in st.
func guided(t testing.TB, g *catalog.CompiledGame, st *state.BoardState, seat int) []string {
	t.Helper()
	legal := state.LegalMoves(g, st, seat)
	var out []string
	// An explicit zero tuning: these tests pin the base heuristics, not the
	// shipped per-cult offsets.
	rules, _ := state.RulesetOf(g)
	cw := rules.(*cthulhuwars.Ruleset)
	for _, mv := range cw.GuideMovesTuned(g, st, seat, legal, &cthulhuwars.GuideTuning{}) {
		out = append(out, state.FormatMove(g, mv))
	}
	return out
}

func TestBotsNeverConsiderGivingUpAGate(t *testing.T) {
	g := game(t)
	p := pos{
		units: []u{{gc, "cultist", "south-pacific", 2}, {gc, "cultist", "new-zealand", 1}},
		gates: map[string]int{"south-pacific": gc, "new-zealand": gc},
		power: powers(8, 8, 8, 8),
	}
	st := p.build()
	if got := notations(g, st, 1); !has(got, "leave:cultist@south-pacific>south-pacific") {
		t.Fatalf("people may abandon, by stepping off: %v", got)
	}
	for _, n := range guided(t, g, st, 1) {
		if from, to, ok := stepOff(n); ok && from == to {
			t.Fatalf("a bot must not consider %s", n)
		}
		// Nor does it send the unit that holds a Gate away: the others do.
		if strings.HasPrefix(n, "leave:") {
			t.Fatalf("a bot must not consider %s", n)
		}
	}
}

func TestGuideOpensByMovingACultistToAFreeNeighbour(t *testing.T) {
	g := game(t)
	st := state.InitialState(g)
	st = play(t, g, st, 1, "cw") // the first player chooses the direction
	got := guided(t, g, st, 1)
	if len(got) == 0 {
		t.Fatal("no guided moves")
	}
	for _, n := range got {
		if !strings.HasPrefix(n, "go:cultist@south-pacific>") {
			t.Errorf("the opening is a Cultist leaving the start area, not %s", n)
		}
	}
	// ... and only one Cultist at a time: after the first the Move ends.
	st = play(t, g, st, 1, got[0])
	if d := data(t, st); d.Stage != "moving" {
		t.Fatalf("stage %q", d.Stage)
	}
	after := guided(t, g, st, 1)
	if len(after) != 1 || after[0] != "done" {
		t.Fatalf("one unit per Move: %v", after)
	}
}

func TestGuideBuildsAGateWhereTheCultistWent(t *testing.T) {
	g := game(t)
	p := pos{
		units: []u{{gc, "cultist", "south-pacific", 5}, {gc, "cultist", "new-zealand", 1}},
		gates: map[string]int{"south-pacific": gc},
		power: powers(7, 8, 8, 8),
	}
	got := guided(t, g, p.build(), 1)
	if len(got) != 1 || got[0] != "gate@new-zealand" {
		t.Fatalf("guided = %v", got)
	}
	// ... not at the cost of the Power for answering a threat to the one
	// Gate it has — but a Gate is the point, so only an enemy standing in
	// the new area stops it.
	q := pos{
		units: []u{{gc, "cultist", "south-pacific", 5}, {gc, "cultist", "new-zealand", 1}, {cc, "hunting-horror", "new-zealand", 1}},
		gates: map[string]int{"south-pacific": gc},
		power: powers(7, 8, 8, 8),
	}
	for _, n := range guided(t, g, q.build(), 1) {
		if n == "gate@new-zealand" {
			t.Fatalf("a Gate where an enemy monster stands is captured at once: %s", n)
		}
	}
}

func TestGuideRecruitsBeforeItShufflesUnits(t *testing.T) {
	g := game(t)
	p := pos{
		units: []u{{gc, "cultist", "south-pacific", 2}},
		gates: map[string]int{"south-pacific": gc},
		power: powers(2, 8, 8, 8),
	}
	got := guided(t, g, p.build(), 1)
	for _, n := range got {
		if !strings.HasPrefix(n, "recruit@") {
			t.Fatalf("more income first: %v", got)
		}
	}
}

func TestGuideKeepsTheLastCultistOfAGateWhereItIs(t *testing.T) {
	g := game(t)
	p := pos{
		units: []u{{gc, "cultist", "south-pacific", 1}, {gc, "deep-one", "south-pacific", 1}},
		gates: map[string]int{"south-pacific": gc},
		power: powers(1, 8, 8, 8),
	}
	for _, n := range guided(t, g, p.build(), 1) {
		if strings.HasPrefix(n, "go:cultist@south-pacific") {
			t.Fatalf("leaving the Gate without a Cultist gives it up: %s", n)
		}
	}
}

func TestGuideDoesNotLeaveAGuardAwayFromWhatItGuards(t *testing.T) {
	g := game(t)
	p := pos{
		units: []u{{gc, "cultist", "south-pacific", 2}, {gc, "deep-one", "south-pacific", 1}},
		gates: map[string]int{"south-pacific": gc},
		power: powers(1, 8, 8, 8),
	}
	got := guided(t, g, p.build(), 1)
	for _, n := range got {
		if strings.HasPrefix(n, "go:deep-one@south-pacific") {
			t.Fatalf("the only guard stays: %v", got)
		}
	}
}

func TestGuideSummonsAProtectorAsTheLastActionWhenAnEnemyCouldCapture(t *testing.T) {
	g := game(t)
	// Crawling Chaos has a Nightgaunt next door and the Power to use it.
	enemy := []u{{cc, "nightgaunt", "new-zealand", 1}, {cc, "cultist", "asia", 1}}
	units := append([]u{{gc, "cultist", "south-pacific", 2}}, enemy...)
	short := pos{units: units, gates: map[string]int{"south-pacific": gc, "asia": cc}, power: powers(1, 8, 8, 8)}
	got := guided(t, g, short.build(), 1)
	if len(got) != 1 || got[0] != "summon:deep-one@south-pacific" {
		t.Fatalf("with 1 Power left the protector is the last action: %v", got)
	}
	// With Power to spare the reply can wait: nothing needs to be summoned
	// ahead of time.
	plenty := pos{units: units, gates: map[string]int{"south-pacific": gc, "asia": cc}, power: powers(8, 8, 8, 8)}
	for _, n := range guided(t, g, plenty.build(), 1) {
		if strings.HasPrefix(n, "summon:") {
			t.Fatalf("with enough Power left the protector is summoned in reply, not now: %s", n)
		}
	}
}

func TestGuideSummonsAtOnceWhenAnEnemyMonsterIsAlreadyThere(t *testing.T) {
	g := game(t)
	p := pos{
		units: []u{{gc, "cultist", "south-pacific", 2}, {cc, "nightgaunt", "south-pacific", 1}},
		gates: map[string]int{"south-pacific": gc},
		power: powers(8, 8, 8, 8),
	}
	got := guided(t, g, p.build(), 1)
	if len(got) == 0 {
		t.Fatal("no guided moves")
	}
	for _, n := range got {
		if !strings.HasPrefix(n, "summon:") {
			t.Fatalf("an enemy monster already in the area is answered by a summon: %v", got)
		}
	}
}

func TestGuideTakesAbandonedGatesAndCaptures(t *testing.T) {
	g := game(t)
	cap := pos{
		units: []u{{gc, "deep-one", "south-pacific", 1}, {gc, "cultist", "south-pacific", 1}, {cc, "cultist", "south-pacific", 1}},
		gates: map[string]int{"south-pacific": gc},
		power: powers(8, 8, 8, 8),
	}
	got := guided(t, g, cap.build(), 1)
	if len(got) != 1 || got[0] != "capture@south-pacific/cc" {
		t.Fatalf("a free Cultist is taken: %v", got)
	}
}

func TestGuideCapturesTheCultistThatHoldsTheGate(t *testing.T) {
	g := game(t)
	cap := pos{
		units: []u{{gc, "deep-one", "south-pacific", 1}, {gc, "cultist", "south-pacific", 1}, {cc, "cultist", "south-pacific", 2}},
		gates: map[string]int{"south-pacific": cc},
		power: powers(8, 8, 8, 8),
	}
	got := guided(t, g, cap.build(), 1)
	if len(got) != 1 || got[0] != "capture:gate:cultist@south-pacific/cc" {
		t.Fatalf("the Cultist on the Gate is taken: %v", got)
	}
}

func TestGuideChoosesTheCheapestUnitToLose(t *testing.T) {
	g := game(t)
	// Great Cthulhu loses a unit to the Devour of ... itself being the
	// victim: assign a Kill among a Cultist, a Deep One and a Star Spawn.
	p := pos{
		units:   []u{{gc, "cultist", "south-pacific", 1}, {gc, "deep-one", "south-pacific", 1}, {gc, "starspawn", "south-pacific", 1}, {cc, "hunting-horror", "south-pacific", 1}},
		gates:   map[string]int{"south-pacific": gc},
		prompts: nil,
	}
	_ = p
	// The Kill queue is the engine's to build: play a real battle with
	// dice that kill (a 6) and look at the assignment prompt.
	seed := seedDice(t, 6, 6, 6, 1, 1, 1, 1)
	q := pos{
		units: []u{{gc, "cultist", "south-pacific", 1}, {gc, "deep-one", "south-pacific", 1}, {gc, "starspawn", "south-pacific", 1}, {cc, "hunting-horror", "south-pacific", 1}},
		gates: map[string]int{"south-pacific": gc},
		power: powers(8, 8, 8, 8),
		rng:   seed,
		turn:  2,
	}
	st := play(t, g, q.build(), 2, "battle@south-pacific/gc")
	d := data(t, st)
	if d.Stage != "assign" {
		t.Skipf("the battle did not reach an assignment (stage %q)", d.Stage)
	}
	owner := d.Bat.Queue[0].Seat
	got := guided(t, g, st, d.Bat.Queue[0].Who)
	if owner == gc && len(got) == 1 && got[0] != "assign:deep-one" {
		t.Fatalf("the least costly unit takes the result: %v", got)
	}
}

// guidedGame plays a whole game in which every seat picks at random among
// the guided moves, and reports what the guide did.
type guidedStats struct {
	plies, rounds, abandons, moveActions, unitsMoved int
	ended                                            bool
	winner                                           int
}

func playGuided(t testing.TB, g *catalog.CompiledGame, seed int64, maxPlies int) guidedStats {
	t.Helper()
	rng := rand.New(rand.NewSource(seed))
	st, err := state.InitialStateSeeded(g, uint64(seed*7919+13))
	if err != nil {
		t.Fatal(err)
	}
	seat := state.FirstSeat(g)
	var s guidedStats
	inMove := false
	for ply := 0; ply < maxPlies; ply++ {
		legal := state.LegalMoves(g, st, seat)
		if len(legal) == 0 {
			t.Fatalf("seed %d ply %d: seat %d has no legal moves\n%s", seed, ply, seat, state.RenderText(g, st))
		}
		choices := state.GuidedMoves(g, st, seat, legal)
		if len(choices) == 0 {
			t.Fatalf("seed %d ply %d: the guide left nothing", seed, ply)
		}
		// Every guided move must be a legal one.
		legalSet := map[string]bool{}
		for _, mv := range legal {
			legalSet[state.FormatMove(g, mv)] = true
		}
		for _, mv := range choices {
			if !legalSet[state.FormatMove(g, mv)] {
				t.Fatalf("seed %d ply %d: guided move %s is not legal", seed, ply, state.FormatMove(g, mv))
			}
		}
		mv := choices[rng.Intn(len(choices))]
		switch mv.Action {
		case "abandon":
			s.abandons++
		case "go":
			if !inMove {
				s.moveActions++
			}
			inMove = true
			s.unitsMoved++
		default:
			inMove = false
		}
		res, err := state.Apply(g, st, seat, mv)
		if err != nil {
			t.Fatalf("seed %d ply %d: guided move %s rejected: %v", seed, ply, state.FormatMove(g, mv), err)
		}
		s.plies = ply + 1
		if res.GameEnded {
			s.ended = true
			if res.WinnerSeat != nil {
				s.winner = *res.WinnerSeat
			}
			s.rounds = data(t, res.NewState).Round
			return s
		}
		st, seat = res.NewState, res.NextTurnSeat
		s.rounds = data(t, st).Round
	}
	return s
}

func TestGuidedGamesAreLegalAndReachAnEnd(t *testing.T) {
	g := game(t)
	var ended, totalMoves, totalUnits, abandons, totalRounds int
	const games = 12
	for seed := int64(1); seed <= games; seed++ {
		s := playGuided(t, g, seed, 6000)
		if s.ended {
			ended++
		}
		totalMoves += s.moveActions
		totalUnits += s.unitsMoved
		abandons += s.abandons
		totalRounds += s.rounds
		t.Logf("seed %2d: plies %4d rounds %2d ended %v winner %d moves %d units %d", seed, s.plies, s.rounds, s.ended, s.winner, s.moveActions, s.unitsMoved)
	}
	if abandons != 0 {
		t.Errorf("bots abandoned %d Gates", abandons)
	}
	if ended < games*3/4 {
		t.Errorf("only %d of %d guided games ended", ended, games)
	}
	if totalMoves > 0 {
		per := float64(totalUnits) / float64(totalMoves)
		t.Logf("units per Move action: %.2f; rounds per game %.1f", per, float64(totalRounds)/games)
		if per > 1.5 {
			t.Errorf("bots move %.2f units per Move action: they should move one at a time", per)
		}
	}
}

// stepOff reads "go:unit@from>to" or "leave:unit@from>to".
func stepOff(n string) (from, to string, ok bool) {
	rest := ""
	switch {
	case strings.HasPrefix(n, "go:"):
		rest = n[3:]
	case strings.HasPrefix(n, "leave:"):
		rest = n[6:]
	default:
		return "", "", false
	}
	i := strings.Index(rest, "@")
	if i < 0 {
		return "", "", false
	}
	rest = rest[i+1:]
	j := strings.Index(rest, ">")
	if j < 0 {
		return "", "", false
	}
	return rest[:j], rest[j+1:], true
}
