package courtfall_test

import (
	"encoding/json"
	"math"
	"math/rand"
	"strings"
	"testing"

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

// Along random games, the engine contract holds at every ply.
func TestRandomGamesKeepTheEngineContract(t *testing.T) {
	g := game(t)
	space := state.ActionSpaceOf(g)
	if !space.Fixed || len(space.Moves) == 0 {
		t.Fatalf("Courtfall should publish a fixed action space (%d moves)", len(space.Moves))
	}
	channels, h, w, err := state.PlaneChannels(g, state.PlaneOptions{})
	if err != nil {
		t.Fatal(err)
	}
	info, ok := state.DescribeRuleset(g)
	if !ok {
		t.Fatal("Courtfall should describe its actions")
	}
	labelled := map[string]bool{}
	for _, a := range info.Actions {
		labelled[a.ID] = true
	}

	games, plies := 14, 450
	if testing.Short() {
		games, plies = 4, 200
	}
	for n := 0; n < games; n++ {
		rng := rand.New(rand.NewSource(int64(1000 + n)))
		st, err := state.InitialStateSeeded(g, uint64(500+n))
		if err != nil {
			t.Fatal(err)
		}
		seat := state.FirstSeat(g)
		for ply := 0; ply < plies; ply++ {
			legal := state.LegalMoves(g, st, seat)
			if len(legal) == 0 {
				t.Fatalf("game %d ply %d: seat %d has no moves\n%s", n, ply, seat, state.RenderText(g, st))
			}
			if other := state.LegalMoves(g, st, 3-seat); len(other) != 0 {
				t.Fatalf("game %d ply %d: both seats have moves", n, ply)
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
			// Apply a sample (all when the list is short).
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
				// Parsing the notation reaches the same position.
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
				if i == 0 {
					chosen = res
				}
			}
			// A move that is not in the legal list is refused: retarget a
			// legal move at a random hex or change its count.
			cells := g.Board.Positions()
			for k := 0; k < 4; k++ {
				bad := legal[rng.Intn(len(legal))]
				if bad.To != "" {
					bad.To = cells[rng.Intn(len(cells))]
				} else {
					bad.Count += 1 + rng.Intn(6)
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
			// Text rendering never panics and mentions the phase.
			if txt := state.RenderText(g, st); !strings.Contains(txt, "Phase:") {
				t.Fatalf("text render missing the phase line:\n%s", txt)
			}
			// Public views carry the derived view and hide the generator.
			pub := state.Public(g, st, 0)
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
		st, _ := state.InitialStateSeeded(g, 4242)
		seat := state.FirstSeat(g)
		var docs []string
		for i := 0; i < 250; i++ {
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
		t.Fatal("lengths differ")
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("position %d differs between identical runs", i)
		}
	}
}

func TestDifferentSeedsGiveDifferentDice(t *testing.T) {
	g := game(t)
	d1, _ := state.InitialStateSeeded(g, 1)
	d2, _ := state.InitialStateSeeded(g, 2)
	if data(t, d1).RNG == data(t, d2).RNG {
		t.Fatal("seeds were not applied")
	}
	// The unseeded start (InitialState) differs run to run.
	a, b := state.InitialState(g), state.InitialState(g)
	if data(t, a).RNG == data(t, b).RNG {
		t.Fatal("InitialState should draw a fresh random seed each time")
	}
	if !state.NeedsStartRecord(g) {
		t.Fatal("a seeded game must have its start recorded")
	}
}

func TestNotationParsesCaseInsensitivelyAndRejectsGarbage(t *testing.T) {
	g := game(t)
	for text, want := range map[string]string{
		"QXD4/2":        "Qxd4/2",
		"qd4":           "Qd4",
		"b*e5":          "B*e5",
		"R^C4":          "R^c4",
		"ACTIVATE:q":    "activate:Q",
		"Tile:Green@D2": "tile:green@d2",
		"PUSH>ne":       "push>NE",
		"clear+2":       "clear+2",
		"Clear":         "clear",
		"surge:n>d3":    "surge:N>d3",
		"tiles:d3<>c3":  "tiles:c3<>d3",
	} {
		mv, err := state.ParseMove(g, text)
		if err != nil {
			t.Errorf("%q: %v", text, err)
			continue
		}
		if got := state.FormatMove(g, mv); got != want {
			t.Errorf("%q → %q, want %q", text, got, want)
		}
	}
	for _, bad := range []string{"", "hello", "Qz9", "Xd4", "Qxd4", "Qxd4/x", "tile:purple@d2", "court:none", "push>UP", "deploy:Q", "reroll/-1", "clear+x"} {
		if _, err := state.ParseMove(g, bad); err == nil {
			t.Errorf("%q should not parse", bad)
		}
	}
}

func TestCommittedDiceAreHiddenFromEveryoneButTheirOwner(t *testing.T) {
	g := game(t)
	b := brawl{dice: []int{6, 6, 2, 1, 1, 2, 3}}
	st, res := play(t, g, b.start(t), light, "Qxd4/3")
	_ = res
	pubJSON := func(viewer int) map[string]any {
		var raw map[string]any
		if err := json.Unmarshal([]byte(marshal(t, state.Public(g, st, viewer))), &raw); err != nil {
			t.Fatal(err)
		}
		return raw["ext"].(map[string]any)
	}
	atk := func(ext map[string]any) float64 {
		return ext["contest"].(map[string]any)["a_atk"].(float64)
	}
	if got := atk(pubJSON(0)); got != -1 {
		t.Errorf("the public sees the committed split: %v", got)
	}
	if got := atk(pubJSON(dark)); got != -1 {
		t.Errorf("the opponent sees the committed split: %v", got)
	}
	if got := atk(pubJSON(light)); got != 3 {
		t.Errorf("the owner should see their own commit, got %v", got)
	}
	// The stored position still has it, and the legal answers do not depend
	// on it being visible.
	if data(t, st).Con.AAtk != 3 {
		t.Fatal("the engine's own copy must keep the commit")
	}
	// The move list hides the number until the exchange resolves.
	mv, _ := state.ParseMove(g, "Qxd4/3")
	mv, note := state.PublicMove(g, st, mv, "Qxd4/3")
	if note != "Qxd4/?" || mv.Count != -1 {
		t.Errorf("move record = %v %q", mv, note)
	}
	// After the answer the exchange is resolved and everything is public.
	st, _ = play(t, g, st, dark, "respond/1")
	_, note = state.PublicMove(g, st, mv, "Qxd4/3")
	if note != "Qxd4/3" {
		t.Errorf("a resolved strike's record should be public, got %q", note)
	}
	// The resolved contest shows both splits and every die.
	var raw map[string]any
	_ = json.Unmarshal([]byte(marshal(t, state.Public(g, st, 0))), &raw)
	view := raw["ext"].(map[string]any)["view"].(map[string]any)
	cv := view["contest"].(map[string]any)
	sides := cv["sides"].([]any)
	if cv["resolved"] != true || sides[0].(map[string]any)["attack"].(float64) != 3 {
		t.Errorf("resolved contest view = %v", cv)
	}
}

func TestDeterminizeHidesTheFutureAndTheOpponentsCommit(t *testing.T) {
	g := game(t)
	b := brawl{dice: []int{6, 6, 2, 1, 1, 2, 3}}
	st, _ := play(t, g, b.start(t), light, "Qxd4/3")
	rng := rand.New(rand.NewSource(1))
	seenAtk := map[int]bool{}
	seenRNG := map[uint64]bool{}
	for i := 0; i < 40; i++ {
		d := state.Determinize(g, st, dark, rng)
		dd := data(t, d)
		seenAtk[dd.Con.AAtk] = true
		seenRNG[dd.RNG] = true
		if dd.Con.AAtk < 0 || dd.Con.AAtk > 4 {
			t.Fatalf("sampled split %d outside the Champion's dice", dd.Con.AAtk)
		}
		if data(t, st).RNG == dd.RNG {
			t.Fatal("determinization kept the real generator")
		}
		if len(state.LegalMoves(g, d, dark)) != len(state.LegalMoves(g, st, dark)) {
			t.Fatal("determinization changed the legal answers")
		}
	}
	if len(seenAtk) < 3 || len(seenRNG) < 30 {
		t.Fatalf("determinization should sample: %d splits, %d generators", len(seenAtk), len(seenRNG))
	}
	// The committing side keeps its own knowledge.
	for i := 0; i < 5; i++ {
		d := state.Determinize(g, st, light, rng)
		if data(t, d).Con.AAtk != 3 {
			t.Fatal("the committer forgot its own split")
		}
	}
	// And the original is untouched.
	if data(t, st).Con.AAtk != 3 {
		t.Fatal("Determinize mutated the position")
	}
}

// FuzzParseMove: arbitrary text never panics the parser, and whatever parses
// formats to something that parses back to the same notation.
func FuzzParseMove(f *testing.F) {
	g := game(f)
	for _, s := range []string{"Qxd4/2", "B*e5", "R^c4", "Qd4", "tile:green@d3", "court:gale", "deploy:Q@c6",
		"activate:Q", "clear+2", "respond/3", "reroll/2", "push>NE", "shift>d4", "tiles:c3<>d4", "surge:N>d3", "end",
		"", "Q", "Qx", "Qx/", "tile:@", "clear+", "İ", "QK", "push>", "deploy:Q@"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, text string) {
		mv, err := state.ParseMove(g, text)
		if err != nil {
			return
		}
		canon := state.FormatMove(g, mv)
		back, err := state.ParseMove(g, canon)
		if err != nil {
			t.Fatalf("%q parsed to %q which does not parse: %v", text, canon, err)
		}
		if again := state.FormatMove(g, back); again != canon {
			t.Fatalf("%q → %q → %q", text, canon, again)
		}
	})
}
