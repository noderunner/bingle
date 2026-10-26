package tak_test

import (
	"math/rand"
	"testing"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

// bruteForceCandidates builds every syntactically conceivable move for the
// mover — all placements of every piece type on every square, and every
// stack move (any count, any of the four directions, any number of squares,
// any composition) — without using the engine's own enumeration.
func bruteForceCandidates(g *catalog.CompiledGame, st *state.BoardState) []state.Move {
	var out []state.Move
	positions := g.Board.Positions()

	for _, piece := range []string{"flat", "wall", "cap"} {
		for _, to := range positions {
			out = append(out, state.Move{Action: "place", Piece: piece, To: to})
		}
	}

	dirs := [][2]int{{0, -1}, {1, 0}, {0, 1}, {-1, 0}}
	for _, from := range positions {
		h := st.Height(from)
		if h == 0 {
			continue
		}
		fc, _ := g.Board.Coord(from)
		// count may exceed both the stack and the carry limit: those must
		// be rejected too.
		for count := 1; count <= h+1 && count <= g.Board.Size()+1; count++ {
			for _, d := range dirs {
				for squares := 1; squares <= count; squares++ {
					path := make([]board.PositionID, 0, squares)
					onBoard := true
					for i := 1; i <= squares; i++ {
						p, ok := g.Board.At(board.Coord{X: fc.X + i*d[0], Y: fc.Y + i*d[1]})
						if !ok {
							onBoard = false
							break
						}
						path = append(path, p)
					}
					if !onBoard {
						continue
					}
					var rec func(i, left int, drops []int)
					rec = func(i, left int, drops []int) {
						if i == squares-1 {
							dd := append(append([]int(nil), drops...), left)
							out = append(out, state.Move{Action: "move", From: from, To: path[squares-1], Count: count, Path: path, Drops: dd})
							return
						}
						for v := 1; v <= left-(squares-1-i); v++ {
							rec(i+1, left-v, append(drops, v))
						}
					}
					rec(0, count, nil)
				}
			}
		}
	}
	return out
}

// TestLegalMovesAreExactlyTheAcceptedMoves checks, on many random positions,
// that the enumerated legal moves are precisely the brute-force candidates
// that Apply accepts: nothing legal is missing and nothing illegal is
// listed. It guards the enumerator's pruning shortcuts.
func TestLegalMovesAreExactlyTheAcceptedMoves(t *testing.T) {
	cases := []struct {
		id    string
		games int
	}{{"tak-3x3", 150}, {"tak-4x4", 30}}
	if testing.Short() {
		cases[0].games, cases[1].games = 6, 2
	}
	for _, tc := range cases {
		g := loadGame(t, tc.id)
		rng := rand.New(rand.NewSource(99))
		checked := 0
		for n := 0; n < tc.games; n++ {
			st, seat := state.InitialState(g), 1
			for ply := 0; ply < 80; ply++ {
				legal := map[string]bool{}
				for _, mv := range state.LegalMoves(g, st, seat) {
					legal[mv.Key()] = true
				}
				for _, cand := range bruteForceCandidates(g, st) {
					norm, err := cand.Normalize(g)
					if err != nil {
						t.Fatalf("normalize %+v: %v", cand, err)
					}
					_, applyErr := state.Apply(g, st, seat, norm)
					accepted := applyErr == nil
					if accepted != legal[norm.Key()] {
						t.Fatalf("%s ply %d: move %s accepted=%v but listed=%v\n%s",
							tc.id, ply, state.FormatMove(g, norm), accepted, legal[norm.Key()], state.RenderText(g, st))
					}
					checked++
				}
				moves := state.LegalMoves(g, st, seat)
				res, err := state.Apply(g, st, seat, moves[rng.Intn(len(moves))])
				if err != nil {
					t.Fatal(err)
				}
				if res.GameEnded {
					break
				}
				st, seat = res.NewState, res.NextTurnSeat
			}
		}
		t.Logf("%s: cross-checked %d candidate moves", tc.id, checked)
	}
}
