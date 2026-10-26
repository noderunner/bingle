package classics_test

import (
	"math/rand"
	"testing"

	"github.com/noderunner/bingle/internal/engine/state"
)

// midGame plays n random plies of the named game and returns the position.
func midGame(b testing.TB, id string, n int) (*state.BoardState, int) {
	g := load(b, id)
	rng := rand.New(rand.NewSource(1))
	st, seat := state.InitialState(g), state.FirstSeat(g)
	for i := 0; i < n; i++ {
		legal := state.LegalMoves(g, st, seat)
		if len(legal) == 0 {
			break
		}
		res, err := state.Apply(g, st, seat, legal[rng.Intn(len(legal))])
		if err != nil {
			b.Fatal(err)
		}
		if res.GameEnded {
			break
		}
		st, seat = res.NewState, res.NextTurnSeat
	}
	return st, seat
}

// BenchmarkApplyHavannah8 measures one move application (state clone plus
// win-rule evaluation: ring, bridge, fork, board-full) on a mid-game
// position of the 169-cell board.
func BenchmarkApplyHavannah8(b *testing.B) {
	g := load(b, "havannah-8")
	st, seat := midGame(b, "havannah-8", 40)
	legal := state.LegalMoves(g, st, seat)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := state.Apply(g, st, seat, legal[i%len(legal)]); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkLegalMovesHavannah8(b *testing.B) {
	g := load(b, "havannah-8")
	st, seat := midGame(b, "havannah-8", 40)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = state.LegalMoves(g, st, seat)
	}
}
