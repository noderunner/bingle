package tak_test

import (
	"testing"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/state"
)

func TestTakRegistersAllBoardSizes(t *testing.T) {
	cases := []struct {
		id        string
		size      int
		stones    int
		capstones int
	}{
		{"tak-3x3", 3, 10, 0},
		{"tak-4x4", 4, 15, 0},
		{"tak-5x5", 5, 21, 1},
		{"tak-6x6", 6, 30, 1},
		{"tak-7x7", 7, 40, 2},
		{"tak-8x8", 8, 50, 2},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			g := loadGame(t, tc.id)
			if g.Board.Width() != tc.size || g.Board.Height() != tc.size {
				t.Errorf("board = %dx%d, want %dx%d", g.Board.Width(), g.Board.Height(), tc.size, tc.size)
			}
			st := state.InitialState(g)
			for _, color := range []string{"white", "black"} {
				if got := st.Reserve(color, "stone"); got != tc.stones {
					t.Errorf("%s stone reserve = %d, want %d", color, got, tc.stones)
				}
				if got := st.Reserve(color, "cap"); got != tc.capstones {
					t.Errorf("%s cap reserve = %d, want %d", color, got, tc.capstones)
				}
			}
			if g.Definition.ID != tc.id {
				t.Errorf("Definition.ID = %q, want %q", g.Definition.ID, tc.id)
			}
			act, ok := g.Action("move")
			if !ok || act.CarryLimit != tc.size {
				t.Errorf("move carry limit = %d, want %d (the board size)", act.CarryLimit, tc.size)
			}
		})
	}
}

func TestOpeningPlacesOpponentFlat(t *testing.T) {
	g := loadGame(t, "tak-5x5")
	st := state.InitialState(g)

	// White's first move places a BLACK flat, taken from black's reserve.
	res := play(t, g, st, 1, "c3")
	occ, ok := res.NewState.Get("c3")
	if !ok || occ.Color != "black" || occ.Piece != "flat" {
		t.Fatalf("after white's opening c3, occupant = %+v ok=%v, want black flat", occ, ok)
	}
	if got := res.NewState.Reserve("black", "stone"); got != 20 {
		t.Errorf("black stone reserve = %d, want 20", got)
	}
	if got := res.NewState.Reserve("white", "stone"); got != 21 {
		t.Errorf("white stone reserve = %d, want 21 (untouched)", got)
	}
	if res.NextTurnSeat != 2 {
		t.Errorf("NextTurnSeat = %d, want 2", res.NextTurnSeat)
	}

	// Black's first move places a WHITE flat.
	res2 := play(t, g, res.NewState, 2, "a1")
	if occ, _ := res2.NewState.Get("a1"); occ.Color != "white" {
		t.Errorf("black's opening stone color = %q, want white", occ.Color)
	}

	// Walls, capstones and stack moves are not allowed in the opening.
	reject(t, g, state.InitialState(g), 1, "Sc3")
	reject(t, g, state.InitialState(g), 1, "Cc3")

	// Exactly 25 moves in the opening: one flat per square.
	if n := len(state.LegalMoves(g, state.InitialState(g), 1)); n != 25 {
		t.Errorf("opening legal moves = %d, want 25", n)
	}
}

func TestPlacementAfterOpening(t *testing.T) {
	g := loadGame(t, "tak-5x5")
	st := startAfterOpening(t, g)

	res := play(t, g, st, 1, "Sc3")
	occ, _ := res.NewState.Get("c3")
	if occ.Piece != "wall" || occ.Color != "white" {
		t.Errorf("c3 = %+v, want white wall", occ)
	}
	// White's e5 flat (placed by black) plus this wall: 21 - 2 = 19. A wall
	// and a flat draw from the same stone pool.
	if got := res.NewState.Reserve("white", "stone"); got != 19 {
		t.Errorf("white stone pool = %d, want 19", got)
	}

	res = play(t, g, res.NewState, 2, "Cd4")
	occ, _ = res.NewState.Get("d4")
	if occ.Piece != "cap" || occ.Color != "black" {
		t.Errorf("d4 = %+v, want black cap", occ)
	}
	if got := res.NewState.Reserve("black", "cap"); got != 0 {
		t.Errorf("black cap reserve = %d, want 0", got)
	}
}

func TestCapstoneSupplyExhausted(t *testing.T) {
	g := loadGame(t, "tak-5x5")
	st := startAfterOpening(t, g)
	st = play(t, g, st, 1, "Cc3").NewState
	st = play(t, g, st, 2, "b2").NewState
	// White already used its only capstone.
	reject(t, g, st, 1, "Cd4")

	// 3x3 has no capstones at all.
	g3 := loadGame(t, "tak-3x3")
	st3 := state.InitialState(g3)
	st3 = play(t, g3, st3, 1, "a1").NewState
	st3 = play(t, g3, st3, 2, "c3").NewState
	reject(t, g3, st3, 1, "Cb2")
}

func TestCannotPlaceOnOccupiedSquare(t *testing.T) {
	g := loadGame(t, "tak-5x5")
	st := startAfterOpening(t, g)
	reject(t, g, st, 1, "a1")
	reject(t, g, st, 1, "Se5")
}

func TestStackMovementDropsBottomFirst(t *testing.T) {
	g := loadGame(t, "tak-5x5")
	// a3 = white, black, white (top). White to move, move 3 (no opening).
	st, seat := position(t, g, "x5/x5/121,x4/x5/x5 1 3")

	t.Run("one each", func(t *testing.T) {
		res := play(t, g, st, seat, "3a3>111")
		ns := res.NewState
		if ns.Height("a3") != 0 {
			t.Errorf("a3 height = %d, want 0", ns.Height("a3"))
		}
		want := map[string]string{"b3": "white", "c3": "black", "d3": "white"}
		for sq, color := range want {
			occ, ok := ns.Get(boardID(sq))
			if !ok || ns.Height(boardID(sq)) != 1 || occ.Color != color {
				t.Errorf("%s = %+v height %d, want single %s", sq, occ, ns.Height(boardID(sq)), color)
			}
		}
		if res.Notation != "3a3>111" {
			t.Errorf("Notation = %q, want 3a3>111", res.Notation)
		}
	})

	t.Run("one then two", func(t *testing.T) {
		res := play(t, g, st, seat, "3a3>12")
		ns := res.NewState
		if h := ns.Height("b3"); h != 1 {
			t.Errorf("b3 height = %d, want 1", h)
		}
		if h := ns.Height("c3"); h != 2 {
			t.Fatalf("c3 height = %d, want 2", h)
		}
		stack := ns.Stack("c3")
		if stack[0].Color != "black" || stack[1].Color != "white" {
			t.Errorf("c3 stack = %+v, want black under white", stack)
		}
	})

	t.Run("single top piece", func(t *testing.T) {
		res := play(t, g, st, seat, "a3+")
		if h := res.NewState.Height("a3"); h != 2 {
			t.Errorf("a3 height = %d, want 2", h)
		}
		if occ, _ := res.NewState.Get("a4"); occ.Color != "white" {
			t.Errorf("a4 = %+v, want white", occ)
		}
	})

	t.Run("whole stack in one drop", func(t *testing.T) {
		res := play(t, g, st, seat, "3a3>")
		if h := res.NewState.Height("b3"); h != 3 {
			t.Errorf("b3 height = %d, want 3", h)
		}
	})

	t.Run("rejections", func(t *testing.T) {
		reject(t, g, st, seat, "4a3>")     // only 3 in the stack
		reject(t, g, st, seat, "a3<")      // off the board
		reject(t, g, st, seat, "3a3>1111") // 4 drops for 3 pieces: runs 4 squares, sum 4
		reject(t, g, st, seat, "2a3>3")    // drops disagree with the count
		reject(t, g, st, 2, "3a3>")        // black does not control this stack
		reject(t, g, st, seat, "3a3>201")  // zero drop
	})
}

func TestCarryLimitIsBoardSize(t *testing.T) {
	g := loadGame(t, "tak-3x3")
	// A 4-high white stack on a 3x3 board: at most 3 may be carried.
	st, seat := position(t, g, "x3/x3/1111,x2 1 4")
	reject(t, g, st, seat, "4a1+")
	play(t, g, st, seat, "3a1+")
}

func TestWallBlocksAndCapFlattens(t *testing.T) {
	g := loadGame(t, "tak-5x5")

	// Black wall on b3; white flat on a3.
	st, seat := position(t, g, "x5/x5/1,2S,x3/x5/x5 1 3")
	reject(t, g, st, seat, "a3>")

	// A white capstone alone may flatten it.
	st, seat = position(t, g, "x5/x5/1C,2S,x3/x5/x5 1 3")
	res := play(t, g, st, seat, "a3>")
	stack := res.NewState.Stack("b3")
	if len(stack) != 2 || stack[0].Piece != "flat" || stack[0].Color != "black" || stack[1].Piece != "cap" {
		t.Errorf("b3 = %+v, want black flat (flattened wall) under white cap", stack)
	}

	// A capstone riding on a flat may NOT flatten when carried together...
	st, seat = position(t, g, "x5/x5/11C,2S,x3/x5/x5 1 3")
	reject(t, g, st, seat, "2a3>")
	// ... but the capstone alone (count 1) may, leaving its partner behind.
	res = play(t, g, st, seat, "a3>")
	if got := res.NewState.Stack("a3"); len(got) != 1 || got[0].Piece != "flat" {
		t.Errorf("a3 = %+v, want the single remaining white flat", got)
	}
	if got := res.NewState.Stack("b3"); len(got) != 2 || got[0].Piece != "flat" || got[1].Piece != "cap" {
		t.Errorf("b3 = %+v, want flattened wall under the capstone", got)
	}
}

func TestCapstoneFlattensOnlyOnFinalSquare(t *testing.T) {
	g := loadGame(t, "tak-5x5")
	// A wall sits on the FIRST square of a two-square move: nothing may be
	// dropped there except by flattening, which is final-square only.
	st, seat := position(t, g, "x5/x5/11C,2S,x3/x5/x5 1 3")
	reject(t, g, st, seat, "2a3>11")
}

func TestCapstoneCannotBeCovered(t *testing.T) {
	g := loadGame(t, "tak-5x5")
	st, seat := position(t, g, "x5/x5/1,2C,x3/x5/x5 1 3")
	reject(t, g, st, seat, "a3>")
}

func TestFlatsCanBeCovered(t *testing.T) {
	g := loadGame(t, "tak-5x5")
	st, seat := position(t, g, "x5/x5/1,2,x3/x5/x5 1 3")
	res := play(t, g, st, seat, "a3>")
	if h := res.NewState.Height("b3"); h != 2 {
		t.Errorf("b3 height = %d, want 2", h)
	}
}

func TestHorizontalRoadWins(t *testing.T) {
	g := loadGame(t, "tak-5x5")
	st, seat := position(t, g, "x5/x5/1,1,1,1,x/x5/x5 1 6")
	res := play(t, g, st, seat, "e3")
	if !res.GameEnded || res.WinnerSeat == nil || *res.WinnerSeat != 1 || res.EndReason != "road" {
		t.Errorf("result = ended %v winner %v reason %q, want seat 1 by road", res.GameEnded, res.WinnerSeat, res.EndReason)
	}
}

func TestVerticalRoadWithCapstone(t *testing.T) {
	g := loadGame(t, "tak-5x5")
	// Black owns c5,c4,c2,c1 as flats and places its capstone on c3.
	st, seat := position(t, g, "x2,2,x2/x2,2,x2/x5/x2,2,x2/x2,2,x2 2 7")
	res := play(t, g, st, seat, "Cc3")
	if !res.GameEnded || res.WinnerSeat == nil || *res.WinnerSeat != 2 || res.EndReason != "road" {
		t.Errorf("ended=%v winner=%v reason=%q, want seat 2 by a vertical road through the capstone", res.GameEnded, res.WinnerSeat, res.EndReason)
	}
}

func TestWallsDoNotMakeRoads(t *testing.T) {
	g := loadGame(t, "tak-5x5")
	st, seat := position(t, g, "x5/x5/1,1,1S,1,x/x5/x5 1 6")
	res := play(t, g, st, seat, "e3")
	if res.GameEnded {
		t.Errorf("game ended (%v) but a standing stone breaks the road", res.EndReason)
	}
	// Flattening the wall (cap) restores it: separate test above.
}

func TestOnlyTopPiecesCountForRoads(t *testing.T) {
	g := loadGame(t, "tak-5x5")
	// c3 is white under black: black on top, so no white road.
	st, seat := position(t, g, "x5/x5/1,1,12,1,x/x5/x5 1 6")
	res := play(t, g, st, seat, "e3")
	if res.GameEnded {
		t.Errorf("game ended (%v); a white piece buried under black must not carry the road", res.EndReason)
	}
}

func TestMoverWinsWhenBothCompleteRoads(t *testing.T) {
	g := loadGame(t, "tak-5x5")
	// Row 3: black flats a3 b3 d3 e3 with c3 = black under white.
	// Row 2: white flats a2 b2 d2 e2. White moves the top of c3 down to c2:
	// that completes white's row-2 road AND uncovers black's row-3 road.
	st, seat := position(t, g, "x5/x5/2,2,21,2,2/1,1,x,1,1/x5 1 6")
	res := play(t, g, st, seat, "c3-")
	if !res.GameEnded || res.WinnerSeat == nil || *res.WinnerSeat != 1 {
		t.Errorf("winner = %v ended=%v, want the mover (seat 1) to win", res.WinnerSeat, res.GameEnded)
	}
}

func TestFlatWinWhenBoardFull(t *testing.T) {
	g := loadGame(t, "tak-3x3")
	st, seat := position(t, g, "1S,2S,1/2,1S,2S/2,1,x 1 6")
	res := play(t, g, st, seat, "c1")
	if !res.GameEnded || res.WinnerSeat == nil || *res.WinnerSeat != 1 || res.EndReason != "flat-win" {
		t.Errorf("ended=%v winner=%v reason=%q, want seat 1 by flat-win (3 flats vs 2)", res.GameEnded, res.WinnerSeat, res.EndReason)
	}
}

func TestFlatCountTieIsDraw(t *testing.T) {
	g := loadGame(t, "tak-3x3")
	st, seat := position(t, g, "1S,2S,1/2,1S,2S/2,1,x 1 6")
	res := play(t, g, st, seat, "Sc1") // white fills the last square with a wall: 2 flats each
	if !res.GameEnded || res.WinnerSeat != nil {
		t.Errorf("ended=%v winner=%v, want a draw", res.GameEnded, res.WinnerSeat)
	}
}

func TestFlatWinWhenReserveExhausted(t *testing.T) {
	g := loadGame(t, "tak-3x3")
	// White has 9 of its 10 stones on the board (stacked), one in hand.
	st, seat := position(t, g, "11111,x,2/1111,x,x/x3 1 6")
	if got := st.Reserve("white", "stone"); got != 1 {
		t.Fatalf("white reserve = %d, want 1", got)
	}
	res := play(t, g, st, seat, "Sb2")
	if !res.GameEnded || res.WinnerSeat == nil || *res.WinnerSeat != 1 || res.EndReason != "flat-win" {
		t.Errorf("ended=%v winner=%v reason=%q, want seat 1 by flat-win once its reserve is empty", res.GameEnded, res.WinnerSeat, res.EndReason)
	}
}

func TestStackMoveGameContinues(t *testing.T) {
	g := loadGame(t, "tak-5x5")
	st := startAfterOpening(t, g)
	res := play(t, g, st, 1, "Sc3")
	if res.GameEnded {
		t.Error("game should not end after a plain placement")
	}
	if res.NextTurnSeat != 2 {
		t.Errorf("next seat = %d, want 2", res.NextTurnSeat)
	}
}

// boardID is a tiny helper for building PositionIDs inline in tests.
func boardID(s string) board.PositionID { return board.PositionID(s) }
