// Package turns_test covers declared turns of several selections (ADR 0115):
// the parts of a turn, done, the turn in progress in the position, and that
// the ply, the opening and the end triggers count whole turns.
package turns_test

import (
	"encoding/json"
	"math/rand"
	"slices"
	"strings"
	"testing"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

func grid(n int) map[string]any {
	return map[string]any{"generator": map[string]any{"type": "grid", "width": n, "height": n, "adjacency": "orthogonal4", "labels": "algebraic"}}
}

func seats() []any {
	return []any{map[string]any{"seat": 1, "color": "black"}, map[string]any{"seat": 2, "color": "white"}}
}

// six is Connect6 on a small board with a short line: one stone to open, two
// a turn after that, and four in a row wins.
func six(edit func(map[string]any)) map[string]any {
	def := map[string]any{
		"id": "six", "name": "Six", "version": "0.1.0",
		"board": grid(9), "seats": seats(),
		"pieces": map[string]any{"stone": map[string]any{}},
		"turn_order": map[string]any{
			"type": "round-robin", "seats": []int{1, 2},
			"turn":    []any{map[string]any{"actions": []string{"place"}, "min": 2}},
			"opening": map[string]any{"plies": 1, "turn": []any{map[string]any{"actions": []string{"place"}}}},
		},
		"actions": []any{map[string]any{"id": "place", "type": "place", "pieces": []string{"stone"}, "source": "supply"}},
		"win_condition": map[string]any{"type": "rules", "rules": []any{
			map[string]any{"id": "four", "type": "line", "length": 4, "reason": "four"},
			map[string]any{"id": "full", "type": "end", "when": []string{"board-full"}, "decide": map[string]any{"type": "draw"}, "reason": "draw"},
		}},
	}
	if edit != nil {
		edit(def)
	}
	return def
}

// fence is a turn of two parts: a slide of your rook if you like, then a wall
// (written as its square).
func fence() map[string]any {
	return map[string]any{
		"id": "fence", "name": "Fence", "version": "0.1.0",
		"board": grid(5), "seats": seats(),
		"pieces": map[string]any{"rook": map[string]any{}, "wall": map[string]any{}},
		"setup":  []any{map[string]any{"position": "a1", "piece": "rook", "color": "black"}, map[string]any{"position": "e5", "piece": "rook", "color": "white"}},
		"turn_order": map[string]any{
			"type": "round-robin", "seats": []int{1, 2},
			"turn": []any{map[string]any{"actions": []string{"slide"}, "min": 0}, map[string]any{"actions": []string{"wall"}}},
		},
		"actions": []any{
			map[string]any{"id": "slide", "type": "slide", "pieces": []string{"rook"}},
			map[string]any{"id": "wall", "type": "place", "pieces": []string{"wall"}, "source": "supply"},
		},
		"win_condition": map[string]any{"type": "rules", "rules": []any{
			map[string]any{"id": "full", "type": "end", "when": []string{"board-full"}, "decide": map[string]any{"type": "draw"}, "reason": "draw"},
		}},
	}
}

// pair is one or two stones a turn, of different shapes, as in Blooms.
func pair() map[string]any {
	return map[string]any{
		"id": "pair", "name": "Pair", "version": "0.1.0",
		"board": grid(5), "seats": seats(),
		"pieces": map[string]any{"round": map[string]any{}, "square": map[string]any{}},
		"turn_order": map[string]any{
			"type": "round-robin", "seats": []int{1, 2},
			"turn": []any{map[string]any{"actions": []string{"place"}, "min": 1, "max": 2, "distinct": "piece"}},
		},
		"actions": []any{map[string]any{"id": "place", "type": "place", "pieces": []string{"round", "square"}, "source": "supply"}},
		"win_condition": map[string]any{"type": "rules", "rules": []any{
			map[string]any{"id": "limit", "type": "end", "when": []string{"ply-limit"}, "plies": 3, "decide": map[string]any{"type": "draw"}, "reason": "limit"},
		}},
	}
}

func compile(t *testing.T, def map[string]any) (*catalog.CompiledGame, error) {
	t.Helper()
	raw, err := json.Marshal(def)
	if err != nil {
		t.Fatal(err)
	}
	cat := catalog.NewCatalog()
	if err := cat.Register(def["id"].(string), raw, nil); err != nil {
		return nil, err
	}
	g, _ := cat.Get(def["id"].(string))
	return g, nil
}

func mustCompile(t *testing.T, def map[string]any) *catalog.CompiledGame {
	t.Helper()
	g, err := compile(t, def)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func legal(g *catalog.CompiledGame, st *state.BoardState, seat int) []string {
	var out []string
	for _, mv := range state.LegalSelections(g, st, seat) {
		out = append(out, state.FormatMove(g, mv))
	}
	return out
}

func play(t *testing.T, g *catalog.CompiledGame, st *state.BoardState, seat int, text string) *state.MoveResult {
	t.Helper()
	mv, err := state.ParseMove(g, text)
	if err != nil {
		t.Fatalf("%s: %v", text, err)
	}
	res, err := state.Apply(g, st, seat, mv)
	if err != nil {
		t.Fatalf("%s: %v", text, err)
	}
	return res
}

// A turn of two stones keeps the seat after the first, and ends after the
// second; the opening's turn is one stone. The ply counts turns.
func TestATurnOfTwoStones(t *testing.T) {
	g := mustCompile(t, six(nil))
	st := state.InitialState(g)
	res := play(t, g, st, 1, "e5")
	if res.NextImpulseSeat != 2 || res.NewState.Ply() != 1 || res.NewState.Turn() != nil {
		t.Fatalf("the opening stone: next %d, ply %d, turn %+v", res.NextImpulseSeat, res.NewState.Ply(), res.NewState.Turn())
	}
	if !state.ImpulseEnds(g, st, 1, res.Move, res) {
		t.Error("the opening stone does not end the impulse")
	}
	st = res.NewState
	res = play(t, g, st, 2, "a1")
	if res.NextImpulseSeat != 2 || res.NewState.Ply() != 1 {
		t.Fatalf("White's first stone: next %d, ply %d", res.NextImpulseSeat, res.NewState.Ply())
	}
	if state.ImpulseEnds(g, st, 2, res.Move, res) {
		t.Error("the first of two stones ends the impulse")
	}
	if tp := res.NewState.Turn(); tp == nil || tp.Part != 0 || tp.Made != 1 {
		t.Errorf("the turn in progress: %+v", tp)
	}
	if l := legal(g, res.NewState, 2); slices.Contains(l, "done") || slices.Contains(l, "a1") || len(l) != 79 {
		t.Errorf("after one of two stones: %d selections, done offered %v", len(l), slices.Contains(l, "done"))
	}
	if _, err := state.Apply(g, res.NewState, 2, state.Selection{Action: "done"}); err == nil {
		t.Error("done is accepted before the turn has its two stones")
	}
	st = res.NewState
	res = play(t, g, st, 2, "b2")
	if res.NextImpulseSeat != 1 || res.NewState.Ply() != 2 || res.NewState.Turn() != nil {
		t.Errorf("White's second stone: next %d, ply %d, turn %+v", res.NextImpulseSeat, res.NewState.Ply(), res.NewState.Turn())
	}

	// A whole move goes as one impulse, in notation joined by "; ".
	sels, err := state.ParseImpulse(g, "c3; d4")
	if err != nil {
		t.Fatal(err)
	}
	imp, err := state.ApplyImpulse(g, res.NewState, 1, sels)
	if err != nil || imp.Last().NextImpulseSeat != 2 || state.FormatImpulse(g, sels) != "c3; d4" {
		t.Fatalf("the impulse c3; d4: %v %+v", err, imp)
	}
	if _, err := state.ApplyImpulse(g, res.NewState, 1, sels[:1]); err == nil {
		t.Error("half a turn is accepted as a whole impulse")
	}
}

// The game can end halfway through a turn, and the impulse with it.
func TestAWinHalfwayThroughATurn(t *testing.T) {
	g := mustCompile(t, six(nil))
	st := state.InitialState(g)
	for _, s := range []struct {
		seat int
		at   string
	}{{1, "a1"}, {2, "a9"}, {2, "b9"}, {1, "b1"}, {1, "c1"}, {2, "c9"}, {2, "e9"}} {
		st = play(t, g, st, s.seat, s.at).NewState
	}
	res := play(t, g, st, 1, "d1")
	if !res.GameEnded || res.EndReason != "four" || !state.ImpulseEnds(g, st, 1, res.Move, res) {
		t.Errorf("four in a row with the first stone of the turn: ended %v (%s)", res.GameEnded, res.EndReason)
	}
}

// An optional part is passed over by choosing the next part's action, and a
// turn that reaches its last part's max ends.
func TestAnOptionalPartThenARequiredOne(t *testing.T) {
	g := mustCompile(t, fence())
	st := state.InitialState(g)
	l := legal(g, st, 1)
	if !slices.Contains(l, "a1-a3") || !slices.Contains(l, "c3") || slices.Contains(l, "done") {
		t.Fatalf("at the start: %v", l)
	}
	res := play(t, g, st, 1, "c3")
	if res.NextImpulseSeat != 2 || res.NewState.Ply() != 1 {
		t.Errorf("a wall alone is a whole turn: next %d", res.NextImpulseSeat)
	}
	res = play(t, g, res.NewState, 2, "e5-e3")
	if res.NextImpulseSeat != 2 {
		t.Fatalf("a slide keeps the seat for the wall")
	}
	l = legal(g, res.NewState, 2)
	if slices.Contains(l, "e3-e1") || !slices.Contains(l, "b4") || slices.Contains(l, "done") {
		t.Errorf("after the slide only a wall: %v", l)
	}
	if res = play(t, g, res.NewState, 2, "b4"); res.NextImpulseSeat != 1 || res.NewState.Ply() != 2 {
		t.Errorf("the wall ends the turn: next %d, ply %d", res.NextImpulseSeat, res.NewState.Ply())
	}
}

// distinct: piece keeps the second stone of a turn to the other shape, and
// done stops after one; the ply limit counts turns.
func TestDistinctPiecesAndDone(t *testing.T) {
	g := mustCompile(t, pair())
	st := state.InitialState(g)
	res := play(t, g, st, 1, "round@a1")
	l := legal(g, res.NewState, 1)
	if !slices.Contains(l, "done") || !slices.Contains(l, "square@b1") || slices.Contains(l, "round@b1") {
		t.Fatalf("after a round stone: %v", l)
	}
	if gs := state.Gestures(g, state.Selection{Action: "done"}); len(gs) != 1 || gs[0].Kind != state.GestureAction || gs[0].Value != "done" {
		t.Errorf("done's gestures: %+v", gs)
	}
	res = play(t, g, res.NewState, 1, "done")
	if res.NextImpulseSeat != 2 || res.NewState.Ply() != 1 || res.NewState.Turn() != nil {
		t.Fatalf("done: next %d, ply %d", res.NextImpulseSeat, res.NewState.Ply())
	}
	st = res.NewState
	st = play(t, g, st, 2, "square@c1").NewState
	res = play(t, g, st, 2, "round@d1")
	if res.GameEnded || res.NewState.Ply() != 2 {
		t.Fatalf("two turns: ended %v, ply %d", res.GameEnded, res.NewState.Ply())
	}
	res = play(t, g, res.NewState, 1, "round@e1")
	if res.GameEnded {
		t.Fatal("the ply limit ended the game halfway through the third turn")
	}
	res = play(t, g, res.NewState, 1, "done")
	if !res.GameEnded || res.EndReason != "limit" {
		t.Errorf("three turns: ended %v (%s)", res.GameEnded, res.EndReason)
	}
}

// The turn in progress is kept in the position document, and only then.
func TestTheTurnInProgressIsStored(t *testing.T) {
	g := mustCompile(t, pair())
	st := play(t, g, state.InitialState(g), 1, "round@a1").NewState
	raw, _ := st.MarshalJSON()
	if !strings.Contains(string(raw), `"turn":{"part":0,"made":1,"pieces":["round"]}`) {
		t.Fatalf("mid-turn document: %s", raw)
	}
	var back state.BoardState
	if err := json.Unmarshal(raw, &back); err != nil || state.Validate(g, &back) != nil {
		t.Fatalf("round trip: %v", err)
	}
	if l := legal(g, &back, 1); slices.Contains(l, "round@b1") {
		t.Error("the stored turn forgot the shape used")
	}
	end, _ := play(t, g, st, 1, "done").NewState.MarshalJSON()
	if strings.Contains(string(end), `"turn"`) {
		t.Errorf("between turns: %s", end)
	}
}

// The catalog refuses what a turn cannot mean.
func TestTheCatalogRefusesABadTurn(t *testing.T) {
	part := func(p map[string]any) func(map[string]any) {
		return func(def map[string]any) { def["turn_order"].(map[string]any)["turn"] = []any{p} }
	}
	for name, c := range map[string]struct {
		edit func(map[string]any)
		want string
	}{
		"an unknown action":   {part(map[string]any{"actions": []string{"move"}}), `unknown action "move"`},
		"no actions":          {part(map[string]any{"actions": []string{}}), "names no actions"},
		"min above max":       {part(map[string]any{"actions": []string{"place"}, "min": 3, "max": 2}), "from 3 to 2"},
		"an unknown distinct": {part(map[string]any{"actions": []string{"place"}, "distinct": "colour"}), `distinct "colour"`},
		"an action named done": {func(def map[string]any) {
			def["actions"] = append(def["actions"].([]any), map[string]any{"id": "done", "type": "pass"})
		}, "may not declare an action of that id"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := compile(t, six(c.edit)); err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("got %v, want %q", err, c.want)
			}
		})
	}
}

// Random play: every listed selection is accepted, a turn's impulse ends only
// when the seat changes or the game ends, and the games come to an end.
func TestRandomPlayOfTurns(t *testing.T) {
	for _, def := range []map[string]any{six(nil), fence(), pair()} {
		g := mustCompile(t, def)
		rng := rand.New(rand.NewSource(3))
		for n := 0; n < 20; n++ {
			st, seat := state.InitialState(g), state.FirstSeat(g)
			ended := false
			for k := 0; k < 400 && !ended; k++ {
				sels := state.LegalSelections(g, st, seat)
				if len(sels) == 0 {
					t.Fatalf("%s: seat %d has nothing to do in %s", g.Definition.ID, seat, legal(g, st, seat))
				}
				mv := sels[rng.Intn(len(sels))]
				res, err := state.Apply(g, st, seat, mv)
				if err != nil {
					t.Fatalf("%s: a listed selection is refused: %v", g.Definition.ID, err)
				}
				if ends := state.ImpulseEnds(g, st, seat, mv, res); ends != (res.GameEnded || res.NextImpulseSeat != seat) {
					t.Fatalf("%s: the impulse ends %v with next seat %d", g.Definition.ID, ends, res.NextImpulseSeat)
				}
				if err := state.Validate(g, res.NewState); err != nil {
					t.Fatal(err)
				}
				st, seat, ended = res.NewState, res.NextImpulseSeat, res.GameEnded
			}
			if !ended {
				t.Errorf("%s: a game did not end", g.Definition.ID)
			}
		}
	}
}
