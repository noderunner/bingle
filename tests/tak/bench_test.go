package tak_test

import (
	"encoding/json"
	"math/rand"
	"testing"

	"github.com/noderunner/bingle/internal/engine/state"
)

// midGame plays n random plies from the start and returns the position and
// the seat to move.
func midGame(tb testing.TB, id string, n int, seed int64) (*state.BoardState, int) {
	tb.Helper()
	g := loadGame(tb, id)
	rng := rand.New(rand.NewSource(seed))
	st, seat := state.InitialState(g), 1
	for i := 0; i < n; i++ {
		moves := state.LegalMoves(g, st, seat)
		res, err := state.Apply(g, st, seat, moves[rng.Intn(len(moves))])
		if err != nil {
			tb.Fatal(err)
		}
		if res.GameEnded {
			break
		}
		st, seat = res.NewState, res.NextTurnSeat
	}
	return st, seat
}

func BenchmarkLegalMoves5x5Midgame(b *testing.B) {
	g := loadGame(b, "tak-5x5")
	st, seat := midGame(b, "tak-5x5", 30, 1)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = state.LegalMoves(g, st, seat)
	}
}

func BenchmarkLegalMoves8x8Midgame(b *testing.B) {
	g := loadGame(b, "tak-8x8")
	st, seat := midGame(b, "tak-8x8", 60, 1)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = state.LegalMoves(g, st, seat)
	}
}

func BenchmarkRandomPlayout5x5(b *testing.B) {
	g := loadGame(b, "tak-5x5")
	rng := rand.New(rand.NewSource(1))
	b.ResetTimer()
	plies := 0
	for i := 0; i < b.N; i++ {
		st, seat := state.InitialState(g), 1
		for {
			moves := state.LegalMoves(g, st, seat)
			res, err := state.Apply(g, st, seat, moves[rng.Intn(len(moves))])
			if err != nil {
				b.Fatal(err)
			}
			plies++
			if res.GameEnded {
				break
			}
			st, seat = res.NewState, res.NextTurnSeat
		}
	}
	b.ReportMetric(float64(plies)/float64(b.N), "plies/game")
}

// TestLegalMoveListSizeIsReasonable documents how big the full legal-move
// payload gets on the largest board mid-game (what the UI fetches each turn).
func TestLegalMoveListSizeIsReasonable(t *testing.T) {
	g := loadGame(t, "tak-8x8")
	st, seat := midGame(t, "tak-8x8", 80, 7)
	moves := state.LegalMoves(g, st, seat)
	type dto struct {
		Notation string
		Steps    []state.Step
		Move     state.Move
	}
	out := make([]dto, len(moves))
	for i, m := range moves {
		out[i] = dto{state.FormatMove(g, m), state.Steps(g, m), m}
	}
	data, _ := json.Marshal(out)
	t.Logf("8x8 mid-game: %d legal moves, %d KiB of JSON", len(moves), len(data)/1024)
	if len(data) > 4<<20 {
		t.Errorf("legal-move payload is %d bytes, too large for a per-turn fetch", len(data))
	}
}
