// Package tak_test exercises the JSON-defined Tak game end to end through
// the rules engine: registration of the board-size variants, the opening
// swap, placement, stack movement, flattening, road and flat-count wins,
// PTN/TPS notation, and engine self-consistency (legal-move enumeration vs.
// move validation) over many random games, plus a perft check against the
// published Tak node counts.
package tak_test

import (
	"testing"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/games/tak"
)

// loadGame registers the Tak module and returns the compiled game for id.
func loadGame(t testing.TB, id string) *catalog.CompiledGame {
	t.Helper()
	cat := catalog.NewCatalog()
	if err := cat.RegisterModule(tak.Module()); err != nil {
		t.Fatalf("RegisterModule(tak): %v", err)
	}
	g, ok := cat.Get(id)
	if !ok {
		t.Fatalf("catalog has no game %q", id)
	}
	return g
}

// position builds a state from TPS, failing the test on a parse error.
func position(t testing.TB, g *catalog.CompiledGame, tps string) (*state.BoardState, int) {
	t.Helper()
	st, seat, err := state.ParseTPS(g, tps)
	if err != nil {
		t.Fatalf("ParseTPS(%q): %v", tps, err)
	}
	return st, seat
}

// play parses ptn text as a move for seat and applies it, failing the test
// if it is rejected.
func play(t testing.TB, g *catalog.CompiledGame, st *state.BoardState, seat int, ptn string) *state.MoveResult {
	t.Helper()
	mv, err := state.ParseMove(g, ptn)
	if err != nil {
		t.Fatalf("ParseMove(%q): %v", ptn, err)
	}
	res, err := state.Apply(g, st, seat, mv)
	if err != nil {
		t.Fatalf("Apply(%q) by seat %d: %v", ptn, seat, err)
	}
	return res
}

// reject asserts that applying ptn for seat fails (either the notation does
// not parse or the engine refuses the move).
func reject(t testing.TB, g *catalog.CompiledGame, st *state.BoardState, seat int, ptn string) {
	t.Helper()
	mv, err := state.ParseMove(g, ptn)
	if err != nil {
		return
	}
	if _, err = state.Apply(g, st, seat, mv); err == nil {
		t.Fatalf("Apply(%q) by seat %d succeeded, want rejection", ptn, seat)
	}
}

// startAfterOpening returns a 5x5 position with the two opening plies done:
// white's stone (placed by black) on a1... see callers; it plays "a1"
// (white places a BLACK flat at a1) then "e5" (black places a WHITE flat).
func startAfterOpening(t testing.TB, g *catalog.CompiledGame) *state.BoardState {
	t.Helper()
	st := state.InitialState(g)
	st = play(t, g, st, 1, "a1").NewState
	st = play(t, g, st, 2, "e5").NewState
	return st
}
