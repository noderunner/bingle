// Package endturn_test covers effects at the end of a turn (ADR 0116):
// remove-groups takes the opponents' groups without liberties when the
// mover's turn ends, credits a pool, and tints every such group.
package endturn_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

// blooms is a small Blooms: two colours a seat, one or two stones of
// different colours a turn, fenced enemy blooms taken when the turn ends,
// and a win at target captures.
func blooms(setup []any, target int, edit func(map[string]any)) map[string]any {
	disc := func(c string) map[string]any { return map[string]any{"color": c, "code": strings.ToUpper(c[:2])} }
	def := map[string]any{
		"id": "bl", "name": "Bl", "version": "0.1.0",
		"board": map[string]any{"generator": map[string]any{"type": "grid", "width": 5, "height": 5, "adjacency": "orthogonal4", "labels": "algebraic"}},
		"seats": []any{
			map[string]any{"seat": 1, "color": "red", "colors": []string{"red", "yellow"}},
			map[string]any{"seat": 2, "color": "blue", "colors": []string{"blue", "black"}},
		},
		"pieces":   map[string]any{"red": disc("red"), "yellow": disc("yellow"), "blue": disc("blue"), "black": disc("black")},
		"setup":    setup,
		"reserves": map[string]any{"captured": 0},
		"turn_order": map[string]any{"type": "round-robin", "seats": []int{1, 2},
			"turn": []any{map[string]any{"actions": []string{"place"}, "min": 1, "max": 2, "distinct": "piece"}}},
		"actions":     []any{map[string]any{"id": "place", "type": "place", "pieces": []string{"red", "yellow", "blue", "black"}, "source": "supply"}},
		"end_of_turn": []any{map[string]any{"effect": "remove-groups", "condition": "no-liberties", "count_in": "captured", "tint": "fenced"}},
		"display":     map[string]any{"tints": map[string]any{"fenced": map[string]any{"label": "Fenced", "color": "#e8a898"}}},
		"win_condition": map[string]any{"type": "rules", "rules": []any{
			map[string]any{"id": "target", "type": "pool", "pool": "captured", "at_least": target, "reason": "target"},
		}},
	}
	if edit != nil {
		edit(def)
	}
	return def
}

func at(pos, piece string) map[string]any {
	return map[string]any{"position": pos, "piece": piece, "color": piece}
}

func compile(def map[string]any) (*catalog.CompiledGame, error) {
	raw, err := json.Marshal(def)
	if err != nil {
		return nil, err
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
	g, err := compile(def)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

func play(t *testing.T, g *catalog.CompiledGame, st *state.BoardState, seat int, sel state.Selection) *state.MoveResult {
	t.Helper()
	res, err := state.Apply(g, st, seat, sel)
	if err != nil {
		t.Fatalf("%+v: %v", sel, err)
	}
	return res
}

func stone(piece, to string) state.Selection {
	return state.Selection{Action: "place", Piece: piece, To: board.PositionID(to)}
}

var done = state.Selection{Action: catalog.ActionDone}

func occupied(st *state.BoardState, pos string) bool {
	_, ok := st.Get(board.PositionID(pos))
	return ok
}

// A bloom fenced by the first stone of a turn stays until the turn ends, and
// is taken then, with its stones counted in the mover's pool.
func TestAFencedBloomGoesWhenTheTurnEnds(t *testing.T) {
	g := mustCompile(t, blooms([]any{at("a1", "blue"), at("a2", "red")}, 10, nil))
	st := state.InitialState(g)
	res := play(t, g, st, 1, stone("yellow", "b1"))
	if !occupied(res.NewState, "a1") {
		t.Fatal("the blue stone went before the turn ended")
	}
	res = play(t, g, res.NewState, 1, done)
	if occupied(res.NewState, "a1") {
		t.Fatal("the fenced blue stone is still there after the turn")
	}
	if got := res.NewState.Reserve("red", "captured"); got != 1 {
		t.Fatalf("captured = %d, want 1", got)
	}
	if len(res.Captures) != 1 || res.Captures[0] != "a1" {
		t.Fatalf("captures %v", res.Captures)
	}
}

// The mover's own fenced bloom stays at the end of its turn, and its
// opponent takes it at the end of theirs unless they free it.
func TestTheMoversOwnBloomWaitsForTheOpponent(t *testing.T) {
	g := mustCompile(t, blooms([]any{at("a1", "red"), at("a2", "blue")}, 10, nil))
	st := state.InitialState(g)
	res := play(t, g, st, 1, stone("yellow", "b1")) // fences its own red stone
	res = play(t, g, res.NewState, 1, done)
	if !occupied(res.NewState, "a1") {
		t.Fatal("the mover's own fenced stone was taken at the end of its turn")
	}
	tints := state.EndOfTurnTints(g, res.NewState)
	if tints["a1"] != "fenced" {
		t.Fatalf("the fenced red stone is not tinted: %v", tints)
	}
	pub := state.Public(g, res.NewState, 0)
	raw, err := pub.MarshalJSON()
	if err != nil || !strings.Contains(string(raw), `"tints":{"a1":"fenced"}`) {
		t.Fatalf("the public state carries no tint: %s (%v)", raw, err)
	}
	res = play(t, g, res.NewState, 2, stone("black", "e5"))
	res = play(t, g, res.NewState, 2, done)
	if occupied(res.NewState, "a1") {
		t.Fatal("the opponent did not take the fenced stone at the end of its turn")
	}
	if got := res.NewState.Reserve("blue", "captured"); got != 1 {
		t.Fatalf("the opponent's captured = %d, want 1", got)
	}
}

// A pool win counts the captures of the turn that reaches the target.
func TestTheCapturesOfATurnWinIt(t *testing.T) {
	g := mustCompile(t, blooms([]any{at("a1", "blue"), at("a2", "red")}, 1, nil))
	res := play(t, g, state.InitialState(g), 1, stone("yellow", "b1"))
	res = play(t, g, res.NewState, 1, stone("red", "e5"))
	if !res.GameEnded || res.WinnerSeat == nil || *res.WinnerSeat != 1 {
		t.Fatalf("ended %v, winner %v", res.GameEnded, res.WinnerSeat)
	}
}

// The catalog refuses an unknown effect, condition or whose, and a tint the
// display does not declare.
func TestTheCatalogRefusesABadEffect(t *testing.T) {
	eff := func(k, v string) func(map[string]any) {
		return func(d map[string]any) { d["end_of_turn"].([]any)[0].(map[string]any)[k] = v }
	}
	for name, edit := range map[string]func(map[string]any){
		"effect":    eff("effect", "flip-groups"),
		"condition": eff("condition", "surrounded"),
		"whose":     eff("whose", "neighbours"),
		"tint":      eff("tint", "nowhere"),
		"pool":      eff("count_in", "taken"),
	} {
		if _, err := compile(blooms(nil, 10, edit)); err == nil {
			t.Errorf("%s: compiled", name)
		}
	}
}
