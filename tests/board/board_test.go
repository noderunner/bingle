// Tests of generated grid boards: neighbour counts of interior, edge and corner squares for each adjacency mode.

package board_test

import (
	"fmt"
	"sort"
	"testing"

	"github.com/noderunner/bingle/internal/engine/board"
)

// posID builds a PositionID from 0-based x,y coordinates, matching the
// "x,y" format mandated by docs/history/plan.md section 6.1's setup entries
// ("0,0" / "8,8").
func posID(x, y int) board.PositionID {
	return board.PositionID(fmt.Sprintf("%d,%d", x, y))
}

// sortedIDs returns a sorted copy of ids as strings, for order-independent
// comparison of neighbor sets.
func sortedIDs(ids []board.PositionID) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = string(id)
	}
	sort.Strings(out)
	return out
}

// assertSameSet fails the test if got and want don't contain exactly the
// same elements (order-independent, duplicates matter).
func assertSameSet(t *testing.T, got, want []board.PositionID) {
	t.Helper()
	gs, ws := sortedIDs(got), sortedIDs(want)
	if len(gs) != len(ws) {
		t.Fatalf("neighbor set size mismatch: got %v (len %d), want %v (len %d)", gs, len(gs), ws, len(ws))
	}
	for i := range gs {
		if gs[i] != ws[i] {
			t.Fatalf("neighbor set mismatch: got %v, want %v", gs, ws)
		}
	}
}

// --- Generate: orthogonal4 on a 9x9 grid ---------------------------------

func TestGenerate_Orthogonal4_9x9_InteriorHasExactly4Neighbors(t *testing.T) {
	g, err := board.Generate(9, 9, "orthogonal4")
	if err != nil {
		t.Fatalf("Generate(9,9,orthogonal4) returned unexpected error: %v", err)
	}

	got := g.Neighbors(posID(4, 4))
	if len(got) != 4 {
		t.Errorf("Neighbors(4,4) len = %d, want 4 (got %v)", len(got), got)
	}
	want := []board.PositionID{posID(3, 4), posID(5, 4), posID(4, 3), posID(4, 5)}
	assertSameSet(t, got, want)
}

func TestGenerate_Orthogonal4_9x9_CornerHasExactly2Neighbors(t *testing.T) {
	g, err := board.Generate(9, 9, "orthogonal4")
	if err != nil {
		t.Fatalf("Generate(9,9,orthogonal4) returned unexpected error: %v", err)
	}

	got := g.Neighbors(posID(0, 0))
	if len(got) != 2 {
		t.Errorf("Neighbors(0,0) len = %d, want 2 (got %v)", len(got), got)
	}
	want := []board.PositionID{posID(1, 0), posID(0, 1)}
	assertSameSet(t, got, want)
}

func TestGenerate_Orthogonal4_9x9_AllFourCornersHaveExactly2Neighbors(t *testing.T) {
	g, err := board.Generate(9, 9, "orthogonal4")
	if err != nil {
		t.Fatalf("Generate(9,9,orthogonal4) returned unexpected error: %v", err)
	}

	corners := []board.PositionID{posID(0, 0), posID(8, 0), posID(0, 8), posID(8, 8)}
	for _, c := range corners {
		if got := len(g.Neighbors(c)); got != 2 {
			t.Errorf("Neighbors(%s) len = %d, want 2", c, got)
		}
	}
}

func TestGenerate_Orthogonal4_9x9_NonCornerEdgeHasExactly3Neighbors(t *testing.T) {
	g, err := board.Generate(9, 9, "orthogonal4")
	if err != nil {
		t.Fatalf("Generate(9,9,orthogonal4) returned unexpected error: %v", err)
	}

	got := g.Neighbors(posID(0, 4))
	if len(got) != 3 {
		t.Errorf("Neighbors(0,4) len = %d, want 3 (got %v)", len(got), got)
	}
	want := []board.PositionID{posID(1, 4), posID(0, 3), posID(0, 5)}
	assertSameSet(t, got, want)
}

// --- Generate: orthogonal8 on a 9x9 grid ---------------------------------

func TestGenerate_Orthogonal8_9x9_InteriorHasExactly8Neighbors(t *testing.T) {
	g, err := board.Generate(9, 9, "orthogonal8")
	if err != nil {
		t.Fatalf("Generate(9,9,orthogonal8) returned unexpected error: %v", err)
	}

	got := g.Neighbors(posID(4, 4))
	if len(got) != 8 {
		t.Errorf("Neighbors(4,4) len = %d, want 8 (got %v)", len(got), got)
	}
	want := []board.PositionID{
		posID(3, 3), posID(4, 3), posID(5, 3),
		posID(3, 4), posID(5, 4),
		posID(3, 5), posID(4, 5), posID(5, 5),
	}
	assertSameSet(t, got, want)
}

func TestGenerate_Orthogonal8_9x9_CornerHasExactly3Neighbors(t *testing.T) {
	g, err := board.Generate(9, 9, "orthogonal8")
	if err != nil {
		t.Fatalf("Generate(9,9,orthogonal8) returned unexpected error: %v", err)
	}

	got := g.Neighbors(posID(0, 0))
	if len(got) != 3 {
		t.Errorf("Neighbors(0,0) len = %d, want 3 (got %v)", len(got), got)
	}
	want := []board.PositionID{posID(1, 0), posID(0, 1), posID(1, 1)}
	assertSameSet(t, got, want)
}

func TestGenerate_Orthogonal8_9x9_NonCornerEdgeHasExactly5Neighbors(t *testing.T) {
	g, err := board.Generate(9, 9, "orthogonal8")
	if err != nil {
		t.Fatalf("Generate(9,9,orthogonal8) returned unexpected error: %v", err)
	}

	got := g.Neighbors(posID(0, 4))
	if len(got) != 5 {
		t.Errorf("Neighbors(0,4) len = %d, want 5 (got %v)", len(got), got)
	}
	want := []board.PositionID{
		posID(0, 3), posID(0, 5),
		posID(1, 3), posID(1, 4), posID(1, 5),
	}
	assertSameSet(t, got, want)
}

// --- Total position count -------------------------------------------------

func TestGenerate_9x9_TotalPositionsIs81(t *testing.T) {
	for _, mode := range []string{"orthogonal4", "orthogonal8"} {
		t.Run(mode, func(t *testing.T) {
			g, err := board.Generate(9, 9, mode)
			if err != nil {
				t.Fatalf("Generate(9,9,%s) returned unexpected error: %v", mode, err)
			}
			count := 0
			for x := 0; x < 9; x++ {
				for y := 0; y < 9; y++ {
					if g.HasPosition(posID(x, y)) {
						count++
					}
				}
			}
			if count != 81 {
				t.Errorf("distinct positions found via HasPosition = %d, want 81", count)
			}
		})
	}
}

func TestGenerate_NonSquareGrid_TotalPositionsIsWidthTimesHeight(t *testing.T) {
	g, err := board.Generate(5, 3, "orthogonal4")
	if err != nil {
		t.Fatalf("Generate(5,3,orthogonal4) returned unexpected error: %v", err)
	}
	count := 0
	for x := 0; x < 5; x++ {
		for y := 0; y < 3; y++ {
			if g.HasPosition(posID(x, y)) {
				count++
			}
		}
	}
	if count != 15 {
		t.Errorf("distinct positions = %d, want 15 (5*3)", count)
	}
}

// --- HasPosition -----------------------------------------------------------

func TestHasPosition_AllInRangePositionsPresent(t *testing.T) {
	g, err := board.Generate(9, 9, "orthogonal4")
	if err != nil {
		t.Fatalf("Generate(9,9,orthogonal4) returned unexpected error: %v", err)
	}
	for x := 0; x < 9; x++ {
		for y := 0; y < 9; y++ {
			id := posID(x, y)
			if !g.HasPosition(id) {
				t.Errorf("HasPosition(%s) = false, want true", id)
			}
		}
	}
}

func TestHasPosition_OutOfRangeAbsent(t *testing.T) {
	g, err := board.Generate(9, 9, "orthogonal4")
	if err != nil {
		t.Fatalf("Generate(9,9,orthogonal4) returned unexpected error: %v", err)
	}
	outOfRange := []board.PositionID{
		posID(9, 9),
		posID(9, 0),
		posID(0, 9),
		posID(-1, 0),
		posID(0, -1),
		board.PositionID("not-a-position"),
		board.PositionID(""),
	}
	for _, id := range outOfRange {
		if g.HasPosition(id) {
			t.Errorf("HasPosition(%s) = true, want false", id)
		}
	}
}

// --- Neighbors on absent position ------------------------------------------

func TestNeighbors_AbsentPositionReturnsEmptySlice(t *testing.T) {
	g, err := board.Generate(9, 9, "orthogonal4")
	if err != nil {
		t.Fatalf("Generate(9,9,orthogonal4) returned unexpected error: %v", err)
	}
	got := g.Neighbors(posID(100, 100))
	if len(got) != 0 {
		t.Errorf("Neighbors(100,100) = %v, want empty slice", got)
	}
}

// --- HasEdge ---------------------------------------------------------------

func TestHasEdge_SymmetricAndConsistentWithNeighbors(t *testing.T) {
	g, err := board.Generate(9, 9, "orthogonal4")
	if err != nil {
		t.Fatalf("Generate(9,9,orthogonal4) returned unexpected error: %v", err)
	}

	a := posID(4, 4)
	for _, b := range g.Neighbors(a) {
		if !g.HasEdge(a, b) {
			t.Errorf("HasEdge(%s,%s) = false, want true (b is in Neighbors(a))", a, b)
		}
		if !g.HasEdge(b, a) {
			t.Errorf("HasEdge(%s,%s) = false, want true (symmetric edge)", b, a)
		}
	}
}

func TestHasEdge_NonAdjacentPositionsFalse(t *testing.T) {
	g, err := board.Generate(9, 9, "orthogonal4")
	if err != nil {
		t.Fatalf("Generate(9,9,orthogonal4) returned unexpected error: %v", err)
	}

	a, b := posID(0, 0), posID(8, 8) // opposite corners: not adjacent
	if g.HasEdge(a, b) {
		t.Errorf("HasEdge(%s,%s) = true, want false (opposite corners are not adjacent)", a, b)
	}
	if g.HasEdge(b, a) {
		t.Errorf("HasEdge(%s,%s) = true, want false (opposite corners are not adjacent)", b, a)
	}
}

func TestHasEdge_SelfLoopFalse(t *testing.T) {
	g, err := board.Generate(9, 9, "orthogonal4")
	if err != nil {
		t.Fatalf("Generate(9,9,orthogonal4) returned unexpected error: %v", err)
	}
	a := posID(4, 4)
	if g.HasEdge(a, a) {
		t.Errorf("HasEdge(%s,%s) = true, want false (a position is not its own neighbor)", a, a)
	}
}

func TestHasEdge_EitherPositionAbsentReturnsFalse(t *testing.T) {
	g, err := board.Generate(9, 9, "orthogonal4")
	if err != nil {
		t.Fatalf("Generate(9,9,orthogonal4) returned unexpected error: %v", err)
	}

	inRange := posID(0, 0)
	absent := posID(50, 50)

	if g.HasEdge(inRange, absent) {
		t.Errorf("HasEdge(%s,%s) = true, want false (second position absent from graph)", inRange, absent)
	}
	if g.HasEdge(absent, inRange) {
		t.Errorf("HasEdge(%s,%s) = true, want false (first position absent from graph)", absent, inRange)
	}
	if g.HasEdge(absent, absent) {
		t.Errorf("HasEdge(%s,%s) = true, want false (both positions absent from graph)", absent, absent)
	}
}

// --- Generate error cases ---------------------------------------------------

func TestGenerate_InvalidArguments_ReturnErrorAndNilGraph(t *testing.T) {
	cases := []struct {
		name   string
		width  int
		height int
		mode   string
	}{
		{"zero_width", 0, 9, "orthogonal4"},
		{"zero_height", 9, 0, "orthogonal4"},
		{"zero_both", 0, 0, "orthogonal4"},
		{"negative_width", -1, 9, "orthogonal4"},
		{"negative_height", 9, -1, "orthogonal4"},
		{"negative_both", -3, -3, "orthogonal4"},
		{"unrecognized_mode", 9, 9, "diagonal-only"},
		{"empty_mode", 9, 9, ""},
		{"unrecognized_mode_and_invalid_dims", 0, 0, "diagonal-only"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g, err := board.Generate(tc.width, tc.height, tc.mode)
			if err == nil {
				t.Errorf("Generate(%d,%d,%q) expected non-nil error, got nil error and graph %v", tc.width, tc.height, tc.mode, g)
			}
			if g != nil {
				t.Errorf("Generate(%d,%d,%q) expected nil graph on error, got %v", tc.width, tc.height, tc.mode, g)
			}
		})
	}
}

func TestGenerate_ValidArguments_ControlCaseSucceeds(t *testing.T) {
	// Control: confirms the error-case failures above are attributable to
	// the specific bad argument under test, not some other setup mistake.
	if _, err := board.Generate(9, 9, "orthogonal4"); err != nil {
		t.Fatalf("control: Generate(9,9,orthogonal4) returned unexpected error: %v", err)
	}
	if _, err := board.Generate(9, 9, "orthogonal8"); err != nil {
		t.Fatalf("control: Generate(9,9,orthogonal8) returned unexpected error: %v", err)
	}
	if _, err := board.Generate(1, 1, "orthogonal4"); err != nil {
		t.Fatalf("control: Generate(1,1,orthogonal4) returned unexpected error: %v", err)
	}
}

// --- 1x1 grid degenerate edge case ------------------------------------------

func TestGenerate_1x1Grid_SinglePositionNoNeighbors(t *testing.T) {
	g, err := board.Generate(1, 1, "orthogonal4")
	if err != nil {
		t.Fatalf("Generate(1,1,orthogonal4) returned unexpected error: %v", err)
	}
	if !g.HasPosition(posID(0, 0)) {
		t.Fatalf("HasPosition(0,0) = false, want true for a 1x1 grid")
	}
	if got := len(g.Neighbors(posID(0, 0))); got != 0 {
		t.Errorf("Neighbors(0,0) on 1x1 grid len = %d, want 0", got)
	}
	if g.HasEdge(posID(0, 0), posID(0, 0)) {
		t.Errorf("HasEdge(0,0,0,0) on 1x1 grid = true, want false")
	}
}
