// Tests of a grid generated as several boards side by side (Alice chess).

package board_test

import (
	"testing"

	"github.com/noderunner/bingle/internal/engine/board"
)

func TestGridOfTwoBoards(t *testing.T) {
	g, err := board.GenerateGrid(board.GridOptions{Width: 8, Height: 8, Mode: "orthogonal8", Labels: board.LabelAlgebraic, Boards: 2})
	if err != nil {
		t.Fatal(err)
	}
	if g.Width() != 16 || g.Height() != 8 || g.Len() != 128 {
		t.Fatalf("got %dx%d with %d positions, want 16x8 with 128", g.Width(), g.Height(), g.Len())
	}
	boards := g.Boards()
	if len(boards) != 2 || len(boards[0]) != 64 || len(boards[1]) != 64 {
		t.Fatalf("boards: got %d, want two of 64", len(boards))
	}
	for i, want := range []map[board.PositionID]bool{{"a1": true, "h8": true}, {"i1": true, "p8": true}} {
		on := map[board.PositionID]bool{}
		for _, id := range boards[i] {
			on[id] = true
		}
		for id := range want {
			if !on[id] {
				t.Errorf("board %d has no %s", i, id)
			}
		}
	}
	if c, _ := g.Coord("i1"); c != (board.Coord{X: 8, Y: 7}) {
		t.Errorf("i1 is at %v, want the ninth column of the bottom row", c)
	}
	// Each board's squares are neighbours on it, never across to the other.
	assertSameSet(t, g.Neighbors("h4"), []board.PositionID{"h5", "h3", "g4", "g5", "g3"})
	assertSameSet(t, g.Neighbors("i4"), []board.PositionID{"i5", "i3", "j4", "j5", "j3"})
	if g.HasEdge("h4", "i4") {
		t.Error("h4 and i4 are on different boards")
	}
	// The grid's edges are the whole graph's.
	if left, _ := g.Region("left"); left[0] != "a8" {
		t.Errorf("left begins %s, want a8", left[0])
	}
	if right, _ := g.Region("right"); right[0] != "p8" {
		t.Errorf("right begins %s, want p8", right[0])
	}
}

func TestGridOfOneBoardHasNoBoards(t *testing.T) {
	for _, n := range []int{0, 1} {
		g, err := board.GenerateGrid(board.GridOptions{Width: 8, Height: 8, Mode: "orthogonal8", Boards: n})
		if err != nil {
			t.Fatal(err)
		}
		if g.Boards() != nil || g.Width() != 8 {
			t.Errorf("boards %d: got %v boards and width %d, want none and 8", n, len(g.Boards()), g.Width())
		}
	}
}

func TestGridBoardsAreBounded(t *testing.T) {
	for _, opts := range []board.GridOptions{
		{Width: 8, Height: 8, Mode: "orthogonal8", Boards: -1},
		{Width: 8, Height: 8, Mode: "orthogonal8", Boards: 1_000_000},
		{Width: 14, Height: 8, Mode: "orthogonal8", Labels: board.LabelAlgebraic, Boards: 2},
	} {
		if g, err := board.GenerateGrid(opts); err == nil || g != nil {
			t.Errorf("%+v: got graph=%v err=%v, want an error", opts, g != nil, err)
		}
	}
}
