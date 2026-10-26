package cthulhuwars_test

import (
	"encoding/json"
	"math"
	"math/rand"
	"strings"
	"testing"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

func marshal(t testing.TB, st *state.BoardState) string {
	t.Helper()
	b, err := st.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// Along random games the engine contract holds at every decision.
func TestRandomGamesKeepTheEngineContract(t *testing.T) {
	games, plies := 4, 600
	if testing.Short() {
		games, plies = 2, 300
	}
	checkEngineContract(t, game(t), games, plies)
}

// ... with every pack of Loyalty Cards in the box too: every legal move is in
// the action space, round-trips through its notation, has labelled steps and
// changes the position.
func TestRandomGamesWithEveryPackKeepTheEngineContract(t *testing.T) {
	games, plies := 5, 2500
	if testing.Short() {
		games, plies = 2, 1200
	}
	for _, c := range []struct {
		base string
		keys []string
	}{
		{"cthulhu-wars", []string{"gc", "cc", "bg", "ys"}},
		{"cthulhu-wars-5p", []string{"ww", "sl", "ow", "tt", "gc"}},
	} {
		checkEngineContract(t, packTable(t, c.base, c.keys, allCards), games, plies)
	}
}

func checkEngineContract(t *testing.T, g *catalog.CompiledGame, games, plies int) {
	t.Helper()
	nSeats := len(g.Definition.Seats)
	space := state.ActionSpaceOf(g)
	if !space.Fixed || len(space.Moves) == 0 {
		t.Fatalf("Cthulhu Wars should publish a fixed action space (%d moves)", len(space.Moves))
	}
	channels, h, w, err := state.PlaneChannels(g, state.PlaneOptions{})
	if err != nil {
		t.Fatal(err)
	}
	info, ok := state.DescribeRuleset(g)
	if !ok {
		t.Fatal("Cthulhu Wars should describe its actions")
	}
	labelled := map[string]bool{}
	for _, a := range info.Actions {
		labelled[a.ID] = true
	}

	for n := 0; n < games; n++ {
		rng := rand.New(rand.NewSource(int64(2000 + n)))
		st, err := state.InitialStateSeeded(g, uint64(900+n))
		if err != nil {
			t.Fatal(err)
		}
		seat := state.FirstSeat(g)
		for ply := 0; ply < plies; ply++ {
			legal := state.LegalMoves(g, st, seat)
			if len(legal) == 0 {
				t.Fatalf("game %d ply %d: seat %d has no moves\n%s", n, ply, seat, state.RenderText(g, st))
			}
			movers := 0
			for s := 1; s <= nSeats; s++ {
				if state.HasLegalMove(g, st, s) {
					movers++
				}
			}
			if movers != 1 {
				t.Fatalf("game %d ply %d: %d seats have moves", n, ply, movers)
			}
			if err := state.Validate(g, st); err != nil {
				t.Fatalf("game %d ply %d: position invalid: %v", n, ply, err)
			}
			doc := marshal(t, st)
			re := state.NewBoardState()
			if err := re.UnmarshalJSON([]byte(doc)); err != nil {
				t.Fatal(err)
			}
			if marshal(t, re) != doc {
				t.Fatalf("game %d ply %d: document does not round-trip", n, ply)
			}
			if got := state.LegalMoves(g, re, seat); len(got) != len(legal) {
				t.Fatalf("game %d ply %d: decoded position has %d moves, original %d", n, ply, len(got), len(legal))
			}

			seen := map[string]bool{}
			for _, mv := range legal {
				s := state.FormatMove(g, mv)
				if seen[s] {
					t.Fatalf("game %d ply %d: duplicate move %s", n, ply, s)
				}
				seen[s] = true
				if _, ok := space.ID(s); !ok {
					t.Fatalf("game %d ply %d: %s is not in the action space", n, ply, s)
				}
				steps := state.Steps(g, mv)
				if len(steps) == 0 {
					t.Fatalf("%s has no steps", s)
				}
				for _, sp := range steps {
					if sp.Kind == state.StepAction && !labelled[sp.Value] {
						t.Fatalf("%s uses action %q that has no label", s, sp.Value)
					}
				}
				back, err := state.ParseMove(g, s)
				if err != nil {
					t.Fatalf("%s does not parse: %v", s, err)
				}
				if state.FormatMove(g, back) != s {
					t.Fatalf("%s round-trips to %s", s, state.FormatMove(g, back))
				}
			}
			sample := legal
			if len(legal) > 6 {
				sample = nil
				for i := 0; i < 6; i++ {
					sample = append(sample, legal[rng.Intn(len(legal))])
				}
			}
			var chosen *state.MoveResult
			for i, mv := range sample {
				res, err := state.Apply(g, st, seat, mv)
				if err != nil {
					t.Fatalf("game %d ply %d: legal move %s rejected: %v", n, ply, state.FormatMove(g, mv), err)
				}
				if marshal(t, st) != doc {
					t.Fatalf("Apply mutated its input")
				}
				back, _ := state.ParseMove(g, res.Notation)
				res2, err := state.Apply(g, st, seat, back)
				if err != nil || marshal(t, res2.NewState) != marshal(t, res.NewState) {
					t.Fatalf("notation %s does not reproduce the move", res.Notation)
				}
				if !res.GameEnded {
					next, err := state.TurnAfter(g, res.NewState, seat)
					if err != nil || next != res.NextTurnSeat {
						t.Fatalf("TurnAfter = %d (%v), result says %d", next, err, res.NextTurnSeat)
					}
				}
				// A decision always changes the position: nothing may idle in
				// a cycle that spends no Power and goes nowhere.
				if canon(t, res.NewState) == canon(t, st) {
					t.Fatalf("game %d ply %d: %s changed nothing", n, ply, state.FormatMove(g, mv))
				}
				if i == 0 {
					chosen = res
				}
			}
			// A move that is not legal is refused.
			cells := g.Board.Positions()
			for k := 0; k < 4; k++ {
				orig := legal[rng.Intn(len(legal))]
				bad := orig
				if bad.To != "" {
					bad.To = cells[rng.Intn(len(cells))]
				} else {
					bad.Count += 1 + rng.Intn(3)
				}
				if state.FormatMove(g, bad) == state.FormatMove(g, orig) {
					continue // the notation does not carry the changed field
				}
				_, accepted := state.Apply(g, st, seat, bad)
				inList := false
				for _, mv := range legal {
					if state.FormatMove(g, mv) == state.FormatMove(g, bad) {
						inList = true
					}
				}
				if (accepted == nil) != inList {
					t.Fatalf("game %d ply %d: %s accepted=%v but in legal list=%v", n, ply, state.FormatMove(g, bad), accepted == nil, inList)
				}
			}
			// Another seat cannot move out of turn.
			other := seat%nSeats + 1
			if _, err := state.Apply(g, st, other, legal[0]); err == nil {
				t.Fatalf("game %d ply %d: seat %d moved out of turn", n, ply, other)
			}
			// Planes and the heuristic are well formed.
			pl, err := state.EncodePlanes(g, st, seat, seat, state.PlaneOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if pl.Shape != [3]int{len(channels), h, w} || len(pl.Data) != len(channels)*h*w {
				t.Fatalf("planes shape %v data %d", pl.Shape, len(pl.Data))
			}
			for _, v := range pl.Data {
				if math.IsNaN(float64(v)) || v < 0 || v > 1.0001 {
					t.Fatalf("plane value %v out of range", v)
				}
			}
			if hv, ok := state.HeuristicValue(g, st, seat); !ok || hv < -1 || hv > 1 {
				t.Fatalf("heuristic = %v %v", hv, ok)
			}
			if txt := state.RenderText(g, st); !strings.Contains(txt, "phase") {
				t.Fatalf("text render missing the phase line:\n%s", txt)
			}
			// Public views carry the derived view; the generator and
			// other players' Elder Sign values stay hidden.
			pub := state.Public(g, st, seat)
			var raw map[string]any
			if err := json.Unmarshal([]byte(marshal(t, pub)), &raw); err != nil {
				t.Fatal(err)
			}
			ext, _ := raw["ext"].(map[string]any)
			if ext == nil || ext["view"] == nil {
				t.Fatalf("public position has no view")
			}
			if rngv, _ := ext["rng"].(float64); rngv != 0 {
				t.Fatalf("public position leaks the dice generator")
			}
			d := data(t, st)
			for s := 1; s <= nSeats; s++ {
				if s == seat || len(d.Signs[s]) == 0 {
					continue
				}
				signs, _ := ext["signs"].([]any)
				if len(signs) <= s {
					t.Fatalf("public signs %v", signs)
				}
				row, _ := signs[s].([]any)
				if len(row) != len(d.Signs[s]) {
					t.Fatalf("seat %d's sign count must be public (%d of %d)", s, len(row), len(d.Signs[s]))
				}
				for _, v := range row {
					if f, _ := v.(float64); f != 0 {
						t.Fatalf("seat %d's Elder Sign value leaked to seat %d: %v", s, seat, row)
					}
				}
			}

			if chosen.GameEnded {
				break
			}
			st, seat = chosen.NewState, chosen.NextTurnSeat
		}
	}
}

func TestSameSeedSameMovesSamePositions(t *testing.T) {
	g := game(t)
	run := func() []string {
		rng := rand.New(rand.NewSource(77))
		st, err := state.InitialStateSeeded(g, 31337)
		if err != nil {
			t.Fatal(err)
		}
		seat := state.FirstSeat(g)
		var docs []string
		for ply := 0; ply < 160; ply++ {
			legal := state.LegalMoves(g, st, seat)
			res, err := state.Apply(g, st, seat, legal[rng.Intn(len(legal))])
			if err != nil {
				t.Fatal(err)
			}
			docs = append(docs, marshal(t, res.NewState))
			if res.GameEnded {
				break
			}
			st, seat = res.NewState, res.NextTurnSeat
		}
		return docs
	}
	a, b := run(), run()
	if len(a) != len(b) {
		t.Fatalf("lengths %d and %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("position %d differs between identical runs", i)
		}
	}
}

func TestDifferentSeedsGiveDifferentDice(t *testing.T) {
	g := game(t)
	a, _ := state.InitialStateSeeded(g, 1)
	b, _ := state.InitialStateSeeded(g, 2)
	if data(t, a).RNG == data(t, b).RNG || data(t, a).RNG == 0 {
		t.Fatal("seeds must give different generators")
	}
}

func TestNotationParsesCaseInsensitivelyAndRejectsGarbage(t *testing.T) {
	g := game(t)
	for _, s := range []string{"RECRUIT@ASIA", " recruit@asia ", "Move:cultist@europe>asia"} {
		if _, err := state.ParseMove(g, s); err != nil && !strings.Contains(s, "Move") {
			t.Errorf("%q: %v", s, err)
		}
	}
	for _, bad := range []string{"", "recruit", "recruit@atlantis", "go:cultist@asia", "battle@asia/zz", "fly", "retreat:cultist", "first:xx", "pay/x", "reveal/0"} {
		if mv, err := state.ParseMove(g, bad); err == nil {
			t.Errorf("%q parsed as %+v", bad, mv)
		}
	}
}

func FuzzParseMove(f *testing.F) {
	g := game(f)
	for _, s := range []string{"recruit@asia", "go:cultist@asia>europe", "battle@asia/cc", "pay/4", "book:absorb", "first:gc", "desecrate:cultist", "retreat:cultist>asia", "lose:cultist@asia", "zzz"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		mv, err := state.ParseMove(g, s)
		if err != nil {
			return
		}
		out := state.FormatMove(g, mv)
		back, err := state.ParseMove(g, out)
		if err != nil || state.FormatMove(g, back) != out {
			t.Fatalf("%q → %q does not round-trip", s, out)
		}
	})
}

func TestDeterminizeHidesTheBagAndKeepsTheOwnSigns(t *testing.T) {
	g := game(t)
	p := pos{
		units: []u{{gc, "cultist", "south-pacific", 1}},
		gates: map[string]int{"south-pacific": gc}, power: powers(8, 8, 8, 8),
		signs: map[int][]int{1: {3}, 2: {2, 1}}, rng: 55,
	}
	st := p.build()
	rng := rand.New(rand.NewSource(3))
	seenOther := map[int]bool{}
	for i := 0; i < 40; i++ {
		d := state.Determinize(g, st, 1, rng)
		dd := data(t, d)
		if len(dd.Signs[1]) != 1 || dd.Signs[1][0] != 3 {
			t.Fatalf("the viewer's own sign must stay: %v", dd.Signs[1])
		}
		if len(dd.Signs[2]) != 2 {
			t.Fatalf("other seats keep the same number of signs: %v", dd.Signs[2])
		}
		for _, v := range dd.Signs[2] {
			if v < 1 || v > 3 {
				t.Fatalf("a guessed sign is 1..3: %d", v)
			}
			seenOther[v] = true
		}
		if err := state.Validate(g, d); err != nil {
			t.Fatal(err)
		}
	}
	if len(seenOther) < 2 {
		t.Fatalf("a determinization must sample the unknown values: %v", seenOther)
	}
}

func TestUndoIsRefusedAfterTheDiceRoll(t *testing.T) {
	g := game(t)
	p := pos{
		units: []u{{gc, "deep-one", "asia", 1}, {cc, "cultist", "asia", 1}},
		power: powers(8, 0, 0, 0), rng: 1234,
	}
	before := p.build()
	after := play(t, g, before, 1, "battle@asia/cc")
	if state.CanTakeBack(g, before, after) {
		t.Fatal("a battle rolled dice: it cannot be taken back")
	}
	moved := play(t, g, before, 1, "recruit@asia")
	if !state.CanTakeBack(g, before, moved) {
		t.Fatal("a recruit changed no dice: it can be taken back")
	}
}

// canon is a position without its narration: units, Gates, Power, prompts…
func canon(t testing.TB, st *state.BoardState) string {
	t.Helper()
	d := *data(t, st)
	d.Events, d.Last = nil, nil
	doc, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	var cells strings.Builder
	for _, id := range st.Occupied() {
		cells.WriteString(string(id))
		for _, o := range st.Stack(id) {
			cells.WriteString("|" + o.Color + o.Piece)
		}
		cells.WriteString(";")
	}
	return string(doc) + cells.String()
}

func TestAMoveActionNeedsAUnitMovedAndABotCannotIdleInIt(t *testing.T) {
	g := game(t)
	p := pos{units: []u{{gc, "cultist", "south-pacific", 2}}, gates: map[string]int{"south-pacific": gc}, power: powers(8, 8, 8, 8)}
	st := p.build()
	for _, n := range notations(g, st, 1) {
		if n == "done" || n == "move" {
			t.Fatalf("%q at the root of a turn would be a way to idle", n)
		}
	}
	st = play(t, g, st, 1, "go:cultist@south-pacific>new-zealand")
	if got := notations(g, st, 1); !has(got, "done") {
		t.Fatalf("after moving, the player may finish: %v", got)
	}
}

func TestRepetitionGuardKnowsShufflesFromRealMoves(t *testing.T) {
	g := game(t)
	rs, ok := state.RulesetOf(g)
	if !ok {
		t.Fatal("no ruleset")
	}
	guard, ok := rs.(state.RepetitionGuard)
	if !ok {
		t.Fatal("Cthulhu Wars should guard against shuffles")
	}
	p := pos{units: []u{{gc, "cultist", "south-pacific", 2}}, gates: map[string]int{"south-pacific": gc}, power: powers(8, 8, 8, 8)}
	st := p.build()
	before := guard.RepeatKey(g, st, gc)
	moved := play(t, g, st, 1, "go:cultist@south-pacific>new-zealand")
	after := guard.RepeatKey(g, moved, gc)
	if before == after {
		t.Fatal("moving a unit changes where the pieces stand")
	}
	// The other cults' pieces do not matter to this seat's key.
	other := guard.RepeatKey(g, moved, cc)
	if other == after {
		t.Fatal("keys are per seat")
	}
	for _, c := range []struct {
		notation string
		want     bool
	}{
		{"go:cultist@south-pacific>new-zealand", true}, {"leave:cultist@south-pacific>new-zealand", true},
		{"control:cultist@asia", true}, {"control@asia", true}, {"abandon@asia", true},
		{"recruit@asia", false}, {"pass", false}, {"done", false}, {"battle@asia/cc", false}, {"ritual", false},
	} {
		mv, err := state.ParseMove(g, c.notation)
		if err != nil {
			t.Fatal(err)
		}
		if got := guard.Repositions(mv); got != c.want {
			t.Errorf("Repositions(%s) = %v, want %v", c.notation, got, c.want)
		}
	}
}
