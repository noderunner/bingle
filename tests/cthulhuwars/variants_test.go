package cthulhuwars_test

import (
	"testing"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/games/cthulhuwars"
)

func variant(t testing.TB, id string) *catalog.CompiledGame {
	t.Helper()
	cat := catalog.NewCatalog()
	if err := cat.RegisterModule(cthulhuwars.Module()); err != nil {
		t.Fatal(err)
	}
	g, ok := cat.Get(id)
	if !ok {
		t.Fatalf("%s is not registered", id)
	}
	return g
}

func TestMapVariantsAreRegisteredWithTheirAreaCounts(t *testing.T) {
	for id, want := range map[string]int{"cthulhu-wars": 17, "cthulhu-wars-3-5": 17, "cthulhu-wars-3p": 13, "cthulhu-wars-5p": 21} {
		g := variant(t, id)
		if n := g.Board.Len(); n != want {
			t.Errorf("%s has %d areas, want %d", id, n, want)
		}
		// Every cult's start area exists and holds its Gate — but Windwalker's,
		// which it places itself in its first decision.
		d := data(t, state.InitialState(g))
		want := len(g.Definition.Seats)
		if id == "cthulhu-wars-5p" {
			want--
			if d.Stage != "setup" || d.Setup != 5 {
				t.Errorf("%s: stage %q, setup seat %d: Windwalker chooses where it begins", id, d.Stage, d.Setup)
			}
		}
		if len(d.Gates) != want {
			t.Errorf("%s: %d start Gates for %d cults: %v", id, len(d.Gates), len(g.Definition.Seats), d.Gates)
		}
	}
}

func TestThreePlayerGameLeavesYellowSignOut(t *testing.T) {
	g := variant(t, "cthulhu-wars-3p")
	if len(g.Definition.Seats) != 3 {
		t.Fatalf("seats %d", len(g.Definition.Seats))
	}
	for seed := int64(1); seed <= 4; seed++ {
		_, _, done := randomGame(t, g, seed, 1500, func(ply int, st *state.BoardState, seat int, mv state.Move) {
			if seat > 3 {
				t.Fatalf("seat %d acted in a three-player game", seat)
			}
			d := data(t, st)
			if d.Power[4] != 0 || len(d.Gates) > 0 && hasGateOf(d.Gates, 4) {
				t.Fatalf("the absent cult has Power %d / a Gate", d.Power[4])
			}
		})
		if !done {
			t.Errorf("seed %d did not finish", seed)
		}
	}
}

func hasGateOf(g map[string]int, seat int) bool {
	for _, s := range g {
		if s == seat {
			return true
		}
	}
	return false
}
