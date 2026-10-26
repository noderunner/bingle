package cthulhuwars_test

import (
	"fmt"
	"testing"

	"github.com/noderunner/bingle/internal/engine/state"
)

// Random and guided games at tables with the Opener of the Way and the
// Tcho-Tcho stay valid and finish.
func TestRandomGamesWithOpenerAndTchoTcho(t *testing.T) {
	tables := []struct {
		base string
		keys []string
		hp   bool
	}{
		{"cthulhu-wars", []string{"gc", "ow", "tt", "ys"}, false},
		{"cthulhu-wars", []string{"ow", "tt", "bg", "cc"}, true},
		{"cthulhu-wars-5p", []string{"ow", "tt", "ww", "sl", "gc"}, false},
		{"cthulhu-wars-3p", []string{"tt", "ow", "gc"}, true},
	}
	seeds := 3
	if testing.Short() {
		seeds = 1
	}
	for _, c := range tables {
		g := derived(t, c.base, c.keys, c.hp)
		for seed := int64(1); seed <= int64(seeds); seed++ {
			name := fmt.Sprintf("%s seed %d", g.Definition.ID, seed)
			st, ply, done := randomGame(t, g, seed, 9000, func(ply int, st *state.BoardState, seat int, mv state.Move) {
				if err := state.Validate(g, st); err != nil {
					t.Fatalf("%s ply %d after %s: %v\n%s", name, ply, state.FormatMove(g, mv), err, state.RenderText(g, st))
				}
			})
			if !done {
				t.Errorf("%s: no end after %d plies\n%s", name, ply, state.RenderText(g, st))
			}
		}
	}
}

func TestGuidedGamesWithOpenerAndTchoTcho(t *testing.T) {
	for _, keys := range [][]string{{"gc", "ow", "tt", "ys"}, {"ow", "tt", "bg", "cc"}} {
		g := derived(t, "cthulhu-wars", keys, false)
		finished := 0
		for seed := int64(1); seed <= 4; seed++ {
			if playGuided(t, g, seed, 8000).ended {
				finished++
			}
		}
		if finished < 2 {
			t.Errorf("%v: only %d of 4 guided games finished", keys, finished)
		}
	}
}
