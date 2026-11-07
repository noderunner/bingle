// Package blooms_test covers Blooms as it is declared since 0.6.0 (ADRs
// 0115–0118): the turns, the captures at the end of a turn, the target and the
// pie rule, and the conversion of positions the Go ruleset stored.
package blooms_test

import (
	"encoding/json"
	"math/rand"
	"slices"
	"testing"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/games/blooms"
)

func catalogOf(t *testing.T) *catalog.Catalog {
	t.Helper()
	cat := catalog.NewCatalog()
	if err := cat.RegisterModule(blooms.Module()); err != nil {
		t.Fatal(err)
	}
	return cat
}

func gameFor(t *testing.T, id string) *catalog.CompiledGame {
	t.Helper()
	g, ok := catalogOf(t).Get(id)
	if !ok {
		t.Fatalf("%s is not in the catalog", id)
	}
	return g
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

func legal(g *catalog.CompiledGame, st *state.BoardState, seat int) []string {
	var out []string
	for _, m := range state.LegalSelections(g, st, seat) {
		out = append(out, state.FormatMove(g, m))
	}
	return out
}

// Each board has its size and its target.
func TestBoardSizesAndTargets(t *testing.T) {
	for id, want := range map[string][2]int{"blooms-4": {37, 15}, "blooms": {61, 20}, "blooms-6": {91, 25}, "blooms-8": {169, 40}, "blooms-10": {271, 50}} {
		g := gameFor(t, id)
		if n := g.Board.Len(); n != want[0] {
			t.Errorf("%s has %d cells, want %d", id, n, want[0])
		}
		if target := g.Definition.WinCondition.Rules[0].AtLeast; target != want[1] {
			t.Errorf("%s's target is %d, want %d", id, target, want[1])
		}
	}
}

// Warm opens with one stone of either colour; then a turn is one stone or two
// of different colours, and done ends it after one. A ply is a turn.
func TestOneStoneThenOneOrTwo(t *testing.T) {
	g := gameFor(t, "blooms-4")
	st := state.InitialState(g)
	cells := g.Board.Positions()
	res := play(t, g, st, 1, "yellow@"+string(cells[0]))
	if res.NextImpulseSeat != 2 || res.NewState.Ply() != 1 {
		t.Fatalf("the opening stone: next %d, ply %d", res.NextImpulseSeat, res.NewState.Ply())
	}
	if l := legal(g, res.NewState, 2); slices.Contains(l, "red@"+string(cells[1])) || !slices.Contains(l, "black@"+string(cells[1])) {
		t.Fatal("Cool is offered Warm's stones, or not its own")
	}
	res = play(t, g, res.NewState, 2, "blue@"+string(cells[18]))
	if res.NextImpulseSeat != 2 || res.NewState.Ply() != 1 {
		t.Fatalf("after one stone of two: next %d, ply %d", res.NextImpulseSeat, res.NewState.Ply())
	}
	l := legal(g, res.NewState, 2)
	if slices.Contains(l, "blue@"+string(cells[19])) || !slices.Contains(l, "black@"+string(cells[19])) || !slices.Contains(l, "done") {
		t.Fatalf("the second stone: %v", l[:3])
	}
	res = play(t, g, res.NewState, 2, "done")
	if res.NextImpulseSeat != 1 || res.NewState.Ply() != 2 {
		t.Fatalf("after done: next %d, ply %d", res.NextImpulseSeat, res.NewState.Ply())
	}
}

// With the pie rule Cool may take the opening stone instead of answering it.
func TestThePieRule(t *testing.T) {
	cat := catalogOf(t)
	id, err := cat.Derive("blooms", map[string]any{catalog.PieOption: true})
	if err != nil || id != "blooms~pie" {
		t.Fatalf("derive: %q %v", id, err)
	}
	plain, _ := cat.Get("blooms")
	pie, _ := cat.Get(id)
	cell := string(plain.Board.Positions()[30])
	for g, offered := range map[*catalog.CompiledGame]bool{plain: false, pie: true} {
		res := play(t, g, state.InitialState(g), 1, "red@"+cell)
		if got := slices.Contains(legal(g, res.NewState, 2), "swap"); got != offered {
			t.Errorf("%s offers the swap: %v", g.Definition.ID, got)
		}
		if offered {
			after := play(t, g, res.NewState, 2, "swap")
			if after.NextImpulseSeat != 2 {
				t.Errorf("after the swap seat %d moves", after.NextImpulseSeat)
			}
			if slices.Contains(legal(g, after.NewState, 2), "swap") {
				t.Error("the swap is offered twice")
			}
		}
	}
}

// Random games keep valid positions, read their notation back, and end.
func TestRandomGamesAreValidAndEnd(t *testing.T) {
	for _, id := range []string{"blooms-4", "blooms"} {
		g := gameFor(t, id)
		rng := rand.New(rand.NewSource(5))
		ended := 0
		for n := 0; n < 20; n++ {
			st, seat := state.InitialState(g), 1
			for ply := 0; ply < 1500; ply++ {
				moves := state.LegalSelections(g, st, seat)
				if len(moves) == 0 {
					t.Fatalf("%s: no move for seat %d", id, seat)
				}
				mv := moves[rng.Intn(len(moves))]
				text := state.FormatMove(g, mv)
				res := play(t, g, st, seat, text)
				if err := state.Validate(g, res.NewState); err != nil {
					t.Fatal(err)
				}
				st, seat = res.NewState, res.NextImpulseSeat
				if res.GameEnded {
					ended++
					break
				}
			}
		}
		if ended < 18 {
			t.Errorf("%s: only %d of 20 games ended", id, ended)
		}
	}
}

// A position the Go ruleset stored converts: its captures go to the pools, a
// turn halfway through goes on, and the game plays on from it.
func TestARulesetPositionConverts(t *testing.T) {
	g := gameFor(t, "blooms-4")
	cells := g.Board.Positions()
	old := map[string]any{
		"v":     2,
		"cells": map[string]any{string(cells[0]): []any{map[string]any{"Piece": "red", "Color": "red"}}, string(cells[5]): []any{map[string]any{"Piece": "blue", "Color": "blue"}}},
		"ply":   3,
		"ext":   map[string]any{"turn": 2, "stage": "second", "first": "blue", "turns": 3, "captured": []int{4, 2}},
	}
	raw, _ := json.Marshal(old)
	doc, changed := blooms.Upgrade(raw)
	if !changed {
		t.Fatal("nothing converted")
	}
	if again, ch := blooms.Upgrade(doc); ch || string(again) != string(doc) {
		t.Fatal("a converted position converts again")
	}
	st := state.NewBoardState()
	if err := st.UnmarshalJSON(doc); err != nil {
		t.Fatal(err)
	}
	if err := state.Validate(g, st); err != nil {
		t.Fatal(err)
	}
	if st.Reserve("red", "captured") != 4 || st.Reserve("blue", "captured") != 2 || st.Ply() != 3 {
		t.Fatalf("captured %d–%d, ply %d", st.Reserve("red", "captured"), st.Reserve("blue", "captured"), st.Ply())
	}
	l := legal(g, st, 2)
	if !slices.Contains(l, "done") || slices.Contains(l, "blue@"+string(cells[9])) || !slices.Contains(l, "black@"+string(cells[9])) {
		t.Fatal("the turn halfway through does not go on with the other colour")
	}
}
