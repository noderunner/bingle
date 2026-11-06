// Package seatcolors_test covers seats that play several colours (ADR 0117):
// a piece type in one of its seat's colours is placed in it, a seat may not
// place another seat's colour, groups are one colour, and a placement never
// captures a group of the mover's other colour.
package seatcolors_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

// duo is Go on a 5×5 board with two colours a seat, as in Blooms.
func duo(setup []any, edit func(map[string]any)) map[string]any {
	disc := func(c string) map[string]any { return map[string]any{"color": c, "code": strings.ToUpper(c[:2])} }
	def := map[string]any{
		"id": "duo", "name": "Duo", "version": "0.1.0",
		"board": map[string]any{"generator": map[string]any{"type": "grid", "width": 5, "height": 5, "adjacency": "orthogonal4", "labels": "algebraic"}},
		"seats": []any{
			map[string]any{"seat": 1, "color": "red", "colors": []string{"red", "yellow"}},
			map[string]any{"seat": 2, "color": "blue", "colors": []string{"blue", "black"}},
		},
		"pieces":     map[string]any{"red": disc("red"), "yellow": disc("yellow"), "blue": disc("blue"), "black": disc("black")},
		"setup":      setup,
		"turn_order": map[string]any{"type": "round-robin", "seats": []int{1, 2}},
		"actions": []any{map[string]any{"id": "place", "type": "place", "pieces": []string{"red", "yellow", "blue", "black"},
			"source": "supply", "captures": map[string]any{}}},
		"win_condition": map[string]any{"type": "rules", "rules": []any{
			map[string]any{"id": "full", "type": "end", "when": []string{"board-full"}, "decide": map[string]any{"type": "draw"}, "reason": "draw"},
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

func place(g *catalog.CompiledGame, st *state.BoardState, seat int, piece, to string) (*state.MoveResult, error) {
	return state.Apply(g, st, seat, state.Selection{Action: "place", Piece: piece, To: board.PositionID(to)})
}

func colorAt(st *state.BoardState, pos string) string {
	occ, ok := st.Get(board.PositionID(pos))
	if !ok {
		return ""
	}
	return occ.Color
}

// A stone of the seat's second colour is placed in that colour, and a seat
// may not place the other seat's colours.
func TestAStoneTakesItsOwnColour(t *testing.T) {
	g := mustCompile(t, duo(nil, nil))
	st := state.InitialState(g)
	res, err := place(g, st, 1, "yellow", "c3")
	if err != nil {
		t.Fatal(err)
	}
	if got := colorAt(res.NewState, "c3"); got != "yellow" {
		t.Fatalf("a yellow stone is placed in %q", got)
	}
	if _, err := place(g, res.NewState, 2, "red", "a1"); err == nil {
		t.Fatal("the second seat placed a red stone")
	}
	for _, mv := range state.LegalSelections(g, res.NewState, 2) {
		if mv.Piece == "red" || mv.Piece == "yellow" {
			t.Fatalf("the second seat is offered %+v", mv)
		}
	}
}

// Groups are one colour: a red stone that takes the last liberty of a yellow
// group beside it captures nothing, since both are the first seat's; a
// yellow stone that takes the last liberty of a blue group captures it.
func TestAGroupIsOneColourAndEnemiesGoBySeat(t *testing.T) {
	g := mustCompile(t, duo([]any{at("a1", "yellow"), at("a2", "red"), at("e1", "blue"), at("e2", "yellow")}, nil))
	st := state.InitialState(g)
	res, err := place(g, st, 1, "red", "b1")
	if err != nil {
		t.Fatal(err)
	}
	if got := colorAt(res.NewState, "a1"); got != "yellow" {
		t.Fatalf("the mover's own fenced yellow stone was taken (a1 holds %q)", got)
	}
	st = res.NewState
	if res, err = place(g, st, 2, "black", "c5"); err != nil {
		t.Fatal(err)
	}
	if res, err = place(g, res.NewState, 1, "yellow", "d1"); err != nil {
		t.Fatal(err)
	}
	if got := colorAt(res.NewState, "e1"); got != "" {
		t.Fatalf("the fenced blue stone at e1 was not captured (%q)", got)
	}
}

// The catalog refuses colours that are not the seat's own first, a colour two
// seats play, and a seat's colour listed again in extra_colors.
func TestTheCatalogRefusesBadSeatColours(t *testing.T) {
	cases := map[string]func(map[string]any){
		"not first": func(d map[string]any) {
			d["seats"].([]any)[0].(map[string]any)["colors"] = []string{"yellow", "red"}
		},
		"twice": func(d map[string]any) {
			d["seats"].([]any)[1].(map[string]any)["colors"] = []string{"blue", "yellow"}
		},
		"extra": func(d map[string]any) { d["extra_colors"] = []string{"black"} },
	}
	for name, edit := range cases {
		if _, err := compile(duo(nil, edit)); err == nil {
			t.Errorf("%s: compiled", name)
		}
	}
}
