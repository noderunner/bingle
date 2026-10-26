package cthulhuwars_test

import (
	"testing"

	"github.com/noderunner/bingle/internal/engine/state"
)

func TestSmokeRandomGamesFinishAndStayConsistent(t *testing.T) {
	g := game(t)
	for seed := int64(1); seed <= 12; seed++ {
		st, plies, ended := randomGame(t, g, seed, 20000, func(ply int, st *state.BoardState, seat int, mv state.Move) {
			if ply%7 == 0 {
				if err := state.Validate(g, st); err != nil {
					t.Fatalf("seed %d ply %d after %s: %v\n%s", seed, ply, state.FormatMove(g, mv), err, state.RenderText(g, st))
				}
			}
		})
		if !ended {
			t.Fatalf("seed %d: game unfinished after %d plies\n%s", seed, plies, state.RenderText(g, st))
		}
		t.Logf("seed %d ended after %d decisions", seed, plies)
	}
}
