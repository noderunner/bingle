package tak_test

import (
	"math/rand"
	"testing"
	"time"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

// perft counts the leaf nodes of the legal-move tree to the given depth.
func perft(g *catalog.CompiledGame, st *state.BoardState, seat, depth int) int {
	moves := state.LegalMoves(g, st, seat)
	if depth == 1 {
		return len(moves)
	}
	total := 0
	for _, mv := range moves {
		res, err := state.Apply(g, st, seat, mv)
		if err != nil {
			panic("legal move rejected by Apply: " + err.Error())
		}
		if res.GameEnded {
			total++ // a terminal node counts as a leaf
			continue
		}
		total += perft(g, res.NewState, res.NextTurnSeat, depth-1)
	}
	return total
}

// TestPerftMatchesPublishedTakCounts checks the move generator against the
// node counts every Tak engine reports for the 5x5 start position (the
// opening swap rule included): 25, 600, 43,320 and 2,999,784 at depths 1-4.
func TestPerftMatchesPublishedTakCounts(t *testing.T) {
	g := loadGame(t, "tak-5x5")
	want := []int{25, 600, 43320, 2999784}
	maxDepth := len(want)
	if testing.Short() {
		maxDepth = 3
	}
	for depth := 1; depth <= maxDepth; depth++ {
		start := time.Now()
		got := perft(g, state.InitialState(g), 1, depth)
		t.Logf("perft(%d) = %d in %v", depth, got, time.Since(start).Round(time.Millisecond))
		if got != want[depth-1] {
			t.Errorf("perft(%d) = %d, want %d", depth, got, want[depth-1])
		}
	}
}

// pieceCounts totals every piece (on the board plus in hand) per color.
func pieceCounts(g *catalog.CompiledGame, st *state.BoardState) map[string]int {
	out := map[string]int{}
	for _, id := range st.Occupied() {
		for _, o := range st.Stack(id) {
			out[o.Color]++
		}
	}
	for _, color := range g.Colors() {
		for pool := range g.Definition.Reserves {
			out[color] += st.Reserve(color, pool)
		}
	}
	return out
}

// TestRandomGamesStayConsistent plays many random games and, at every ply,
// checks that
//   - every enumerated legal move is accepted by Apply;
//   - mutated (mostly illegal) moves are accepted by Apply only if they are
//     enumerated;
//   - the notation round-trips (Parse(Format(m)) == m);
//   - pieces are conserved (board + reserve == starting supply);
//   - TPS round-trips the position.
func TestRandomGamesStayConsistent(t *testing.T) {
	games := []struct {
		id    string
		count int
	}{
		{"tak-3x3", 60},
		{"tak-4x4", 25},
		{"tak-5x5", 12},
	}
	if testing.Short() {
		games[0].count, games[1].count, games[2].count = 15, 6, 3
	}

	for _, gc := range games {
		g := loadGame(t, gc.id)
		rng := rand.New(rand.NewSource(20260711))
		initial := pieceCounts(g, state.InitialState(g))

		finished, draws := 0, 0
		for n := 0; n < gc.count; n++ {
			st := state.InitialState(g)
			seat := 1
			for ply := 0; ply < 400; ply++ {
				moves := state.LegalMoves(g, st, seat)
				if len(moves) == 0 {
					t.Fatalf("%s game %d ply %d: no legal moves but game not over\n%s", gc.id, n, ply, state.RenderText(g, st))
				}

				legal := make(map[string]bool, len(moves))
				for _, mv := range moves {
					legal[mv.Key()] = true
				}

				// Notation round trip and mutation checks on a sample.
				for i := 0; i < 6; i++ {
					mv := moves[rng.Intn(len(moves))]

					text := state.FormatMove(g, mv)
					parsed, err := state.ParseMove(g, text)
					if err != nil {
						t.Fatalf("%s: ParseMove(%q): %v", gc.id, text, err)
					}
					parsed, err = parsed.Normalize(g)
					if err != nil {
						t.Fatalf("%s: Normalize(%q): %v", gc.id, text, err)
					}
					if parsed.Key() != mv.Key() {
						t.Fatalf("%s: notation %q round-trips to %+v, want %+v", gc.id, text, parsed, mv)
					}

					mut := mutate(rng, mv)
					if legal[mut.Key()] {
						continue
					}
					if _, err := state.Apply(g, st, seat, mut); err == nil {
						t.Fatalf("%s game %d ply %d: Apply accepted %+v which LegalMoves did not enumerate\n%s", gc.id, n, ply, mut, state.RenderText(g, st))
					}
				}

				mv := moves[rng.Intn(len(moves))]
				res, err := state.Apply(g, st, seat, mv)
				if err != nil {
					t.Fatalf("%s game %d ply %d: legal move %+v rejected: %v\n%s", gc.id, n, ply, mv, err, state.RenderText(g, st))
				}

				got := pieceCounts(g, res.NewState)
				for color, want := range initial {
					if got[color] != want {
						t.Fatalf("%s: piece count for %s = %d, want %d after %s", gc.id, color, got[color], want, res.Notation)
					}
				}
				if res.NewState.Ply() != st.Ply()+1 {
					t.Fatalf("%s: ply went %d -> %d", gc.id, st.Ply(), res.NewState.Ply())
				}

				// TPS round trip.
				text, err := state.FormatTPS(g, res.NewState, res.NextTurnSeat)
				if err != nil {
					t.Fatalf("FormatTPS: %v", err)
				}
				back, backSeat, err := state.ParseTPS(g, text)
				if err != nil {
					t.Fatalf("ParseTPS(%q): %v", text, err)
				}
				if backSeat != res.NextTurnSeat || back.Ply() != res.NewState.Ply() {
					t.Fatalf("TPS %q: seat/ply = %d/%d, want %d/%d", text, backSeat, back.Ply(), res.NextTurnSeat, res.NewState.Ply())
				}
				again, _ := state.FormatTPS(g, back, backSeat)
				if again != text {
					t.Fatalf("TPS round trip: %q -> %q", text, again)
				}
				for _, color := range g.Colors() {
					for pool := range g.Definition.Reserves {
						if back.Reserve(color, pool) != res.NewState.Reserve(color, pool) {
							t.Fatalf("TPS %q: derived %s %s reserve = %d, want %d", text, color, pool, back.Reserve(color, pool), res.NewState.Reserve(color, pool))
						}
					}
				}

				st, seat = res.NewState, res.NextTurnSeat
				if res.GameEnded {
					finished++
					if res.WinnerSeat == nil {
						draws++
					}
					break
				}
			}
		}
		t.Logf("%s: %d/%d random games finished (%d draws)", gc.id, finished, gc.count, draws)
		if finished == 0 {
			t.Errorf("%s: no random game ever finished", gc.id)
		}
	}
}

// mutate perturbs a legal move into a (usually) illegal neighbour.
func mutate(rng *rand.Rand, mv state.Move) state.Move {
	out := mv
	out.Path = append(out.Path[:0:0], mv.Path...)
	out.Drops = append(out.Drops[:0:0], mv.Drops...)
	switch rng.Intn(4) {
	case 0:
		out.Count = mv.Count + 1
	case 1:
		if len(out.Drops) > 0 {
			out.Drops[0]++
		} else {
			out.Piece = "cap"
		}
	case 2:
		if len(out.Path) > 0 {
			out.Path = out.Path[:len(out.Path)-1]
			out.Drops = out.Drops[:len(out.Drops)-1]
		} else {
			out.To = out.From
		}
	default:
		out.From = mv.To
	}
	return out
}
