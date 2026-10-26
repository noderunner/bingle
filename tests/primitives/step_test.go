// Tests of the step movement primitive: which squares a piece may step to, for each occupancy rule, board and direction set.

package primitives_test

import (
	"testing"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/primitives"
)

// fakeOccupancy is a minimal test-only stand-in for primitives.OccupancyView
// (since internal/engine/state.BoardState does not exist yet). It maps a
// board.PositionID to the color of the piece occupying it; absence from the
// map means unoccupied.
type fakeOccupancy map[board.PositionID]string

// Occupant satisfies primitives.OccupancyView.
func (f fakeOccupancy) Occupant(id board.PositionID) (color string, occupied bool) {
	c, ok := f[id]
	return c, ok
}

// --- fixtures ---------------------------------------------------------------

// adjacentGrid builds a small 3x3 orthogonal4 grid, giving us plenty of
// graph-adjacent pairs (e.g. "1,1"-"1,0") and plenty of non-adjacent pairs
// (e.g. the opposite corners "0,0" and "2,2").
func adjacentGrid(t *testing.T) *board.Graph {
	t.Helper()
	g, err := board.Generate(3, 3, "orthogonal4")
	if err != nil {
		t.Fatalf("board.Generate(3,3,orthogonal4) returned unexpected error: %v", err)
	}
	return g
}

// disconnectedGraph builds a graph with two positions and no edges at all
// between them, for the "no edge exists whatsoever" non-adjacency case.
func disconnectedGraph(t *testing.T) *board.Graph {
	t.Helper()
	g, err := board.FromExplicit(
		[]board.PositionID{"a", "b"},
		[][2]board.PositionID{},
	)
	if err != nil {
		t.Fatalf("board.FromExplicit returned unexpected error: %v", err)
	}
	return g
}

// --- Get: registry lookup ---------------------------------------------------

func TestGet_Step_ReturnsRegisteredFunction(t *testing.T) {
	fn, ok := primitives.Get("step")
	if !ok {
		t.Fatalf("Get(%q) ok = false, want true", "step")
	}
	if fn == nil {
		t.Fatalf("Get(%q) returned nil function, want non-nil", "step")
	}
}

func TestGet_UnknownName_ReturnsFalse(t *testing.T) {
	cases := []string{
		"nonexistent-primitive",
		"",
		"Step", // case-sensitivity: must not fuzzy-match "step"
		"jump",
		"capture-or-empty", // an occupancy mode, not a primitive name
	}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			fn, ok := primitives.Get(name)
			if ok {
				t.Errorf("Get(%q) ok = true, want false", name)
			}
			if fn != nil {
				t.Errorf("Get(%q) returned non-nil function, want nil when ok is false", name)
			}
		})
	}
}

// --- step, occupancy=empty-only: adjacent + unoccupied → allowed -----------

func TestStep_EmptyOnly_AdjacentAndUnoccupied_Allowed(t *testing.T) {
	g := adjacentGrid(t)
	step, ok := primitives.Get("step")
	if !ok {
		t.Fatalf("Get(%q) ok = false, want true", "step")
	}

	from := board.PositionID("1,1")
	to := board.PositionID("1,0")
	if !g.HasEdge(from, to) {
		t.Fatalf("fixture error: expected HasEdge(%s,%s) = true", from, to)
	}

	occ := fakeOccupancy{from: "black"} // only `from` is occupied; `to` is empty
	res := step(g, occ, primitives.OccupancyEmptyOnly, from, to, "black")

	if !res.Allowed {
		t.Errorf("Allowed = false, want true (reason: %q)", res.Reason)
	}
	if res.Captured != nil {
		t.Errorf("Captured = %v, want nil", *res.Captured)
	}
}

func TestStep_EmptyOnly_AdjacentAndUnoccupied_AllDirections(t *testing.T) {
	// Table-driven sweep over every adjacent pair in a 3x3 grid's center,
	// confirming the empty-only allow path is not direction-specific.
	g := adjacentGrid(t)
	step, ok := primitives.Get("step")
	if !ok {
		t.Fatalf("Get(%q) ok = false, want true", "step")
	}

	center := board.PositionID("1,1")
	dests := []board.PositionID{"1,0", "1,2", "0,1", "2,1"}
	for _, to := range dests {
		t.Run(string(to), func(t *testing.T) {
			if !g.HasEdge(center, to) {
				t.Fatalf("fixture error: expected HasEdge(%s,%s) = true", center, to)
			}
			occ := fakeOccupancy{center: "white"}
			res := step(g, occ, primitives.OccupancyEmptyOnly, center, to, "white")
			if !res.Allowed {
				t.Errorf("Allowed = false, want true for %s->%s (reason: %q)", center, to, res.Reason)
			}
			if res.Captured != nil {
				t.Errorf("Captured = %v, want nil for %s->%s", *res.Captured, center, to)
			}
		})
	}
}

// --- step, occupancy=empty-only: adjacent + occupied → rejected ------------

func TestStep_EmptyOnly_AdjacentButOccupiedByOpponent_Rejected(t *testing.T) {
	g := adjacentGrid(t)
	step, ok := primitives.Get("step")
	if !ok {
		t.Fatalf("Get(%q) ok = false, want true", "step")
	}

	from := board.PositionID("1,1")
	to := board.PositionID("1,0")
	occ := fakeOccupancy{from: "black", to: "white"}

	res := step(g, occ, primitives.OccupancyEmptyOnly, from, to, "black")

	if res.Allowed {
		t.Errorf("Allowed = true, want false (destination occupied by opponent)")
	}
	if res.Captured != nil {
		t.Errorf("Captured = %v, want nil (empty-only never captures)", *res.Captured)
	}
	if res.Reason == "" {
		t.Errorf("Reason = %q, want non-empty rejection reason", res.Reason)
	}
}

func TestStep_EmptyOnly_AdjacentButOccupiedBySameColor_Rejected(t *testing.T) {
	g := adjacentGrid(t)
	step, ok := primitives.Get("step")
	if !ok {
		t.Fatalf("Get(%q) ok = false, want true", "step")
	}

	from := board.PositionID("1,1")
	to := board.PositionID("1,0")
	occ := fakeOccupancy{from: "black", to: "black"}

	res := step(g, occ, primitives.OccupancyEmptyOnly, from, to, "black")

	if res.Allowed {
		t.Errorf("Allowed = true, want false (destination occupied by mover's own color)")
	}
	if res.Captured != nil {
		t.Errorf("Captured = %v, want nil (empty-only never captures)", *res.Captured)
	}
	if res.Reason == "" {
		t.Errorf("Reason = %q, want non-empty rejection reason", res.Reason)
	}
}

func TestStep_EmptyOnly_AdjacentAndOccupied_RejectedRegardlessOfColor(t *testing.T) {
	// Table-driven: any occupying color at an adjacent destination is
	// rejected under empty-only, including colors unrelated to movingColor.
	g := adjacentGrid(t)
	step, ok := primitives.Get("step")
	if !ok {
		t.Fatalf("Get(%q) ok = false, want true", "step")
	}

	from := board.PositionID("1,1")
	to := board.PositionID("1,0")

	cases := []struct {
		name          string
		occupantColor string
		movingColor   string
	}{
		{"opponent_color", "white", "black"},
		{"same_color", "black", "black"},
		{"third_unrelated_color", "red", "black"},
		{"empty_string_color", "", "black"}, // occupied with a zero-value color string is still occupied
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			occ := fakeOccupancy{from: tc.movingColor, to: tc.occupantColor}
			res := step(g, occ, primitives.OccupancyEmptyOnly, from, to, tc.movingColor)
			if res.Allowed {
				t.Errorf("Allowed = true, want false (occupant color %q, mover color %q)", tc.occupantColor, tc.movingColor)
			}
			if res.Captured != nil {
				t.Errorf("Captured = %v, want nil", *res.Captured)
			}
			if res.Reason == "" {
				t.Error("Reason = \"\", want non-empty rejection reason")
			}
		})
	}
}

// --- step, occupancy=empty-only: non-adjacent → rejected regardless -------

func TestStep_EmptyOnly_NonAdjacent_OppositeCorners_RejectedEvenWhenDestinationEmpty(t *testing.T) {
	g := adjacentGrid(t)
	step, ok := primitives.Get("step")
	if !ok {
		t.Fatalf("Get(%q) ok = false, want true", "step")
	}

	from := board.PositionID("0,0")
	to := board.PositionID("2,2")
	if g.HasEdge(from, to) {
		t.Fatalf("fixture error: expected HasEdge(%s,%s) = false (opposite corners)", from, to)
	}

	occ := fakeOccupancy{from: "black"} // to is unoccupied
	res := step(g, occ, primitives.OccupancyEmptyOnly, from, to, "black")

	if res.Allowed {
		t.Errorf("Allowed = true, want false (from/to not graph-adjacent)")
	}
	if res.Captured != nil {
		t.Errorf("Captured = %v, want nil", *res.Captured)
	}
	if res.Reason == "" {
		t.Error("Reason = \"\", want non-empty rejection reason")
	}
}

func TestStep_EmptyOnly_NonAdjacent_OppositeCorners_RejectedWhenDestinationOccupied(t *testing.T) {
	g := adjacentGrid(t)
	step, ok := primitives.Get("step")
	if !ok {
		t.Fatalf("Get(%q) ok = false, want true", "step")
	}

	from := board.PositionID("0,0")
	to := board.PositionID("2,2")

	occ := fakeOccupancy{from: "black", to: "white"}
	res := step(g, occ, primitives.OccupancyEmptyOnly, from, to, "black")

	if res.Allowed {
		t.Errorf("Allowed = true, want false (from/to not graph-adjacent, regardless of occupancy)")
	}
	if res.Captured != nil {
		t.Errorf("Captured = %v, want nil", *res.Captured)
	}
	if res.Reason == "" {
		t.Error("Reason = \"\", want non-empty rejection reason")
	}
}

func TestStep_EmptyOnly_NoEdgeAtAllBetweenPositions_Rejected(t *testing.T) {
	// A graph where the two positions have no edge whatsoever (not merely
	// "far apart" on a grid, but truly disconnected).
	g := disconnectedGraph(t)
	step, ok := primitives.Get("step")
	if !ok {
		t.Fatalf("Get(%q) ok = false, want true", "step")
	}

	from := board.PositionID("a")
	to := board.PositionID("b")
	if g.HasEdge(from, to) {
		t.Fatalf("fixture error: expected HasEdge(%s,%s) = false (no edges in graph)", from, to)
	}

	t.Run("destination_empty", func(t *testing.T) {
		occ := fakeOccupancy{from: "black"}
		res := step(g, occ, primitives.OccupancyEmptyOnly, from, to, "black")
		if res.Allowed {
			t.Errorf("Allowed = true, want false (no edge between positions)")
		}
		if res.Reason == "" {
			t.Error("Reason = \"\", want non-empty rejection reason")
		}
	})

	t.Run("destination_occupied", func(t *testing.T) {
		occ := fakeOccupancy{from: "black", to: "white"}
		res := step(g, occ, primitives.OccupancyEmptyOnly, from, to, "black")
		if res.Allowed {
			t.Errorf("Allowed = true, want false (no edge between positions)")
		}
		if res.Reason == "" {
			t.Error("Reason = \"\", want non-empty rejection reason")
		}
	})
}

// --- step, occupancy=empty-only: from itself (no self-move) ----------------

func TestStep_EmptyOnly_FromEqualsTo_Rejected(t *testing.T) {
	// board.HasEdge treats a==b as false (never its own neighbor, per
	// board.go's documented self-loop behavior), so from==to must be
	// rejected as a non-adjacency case, not silently allowed.
	g := adjacentGrid(t)
	step, ok := primitives.Get("step")
	if !ok {
		t.Fatalf("Get(%q) ok = false, want true", "step")
	}

	pos := board.PositionID("1,1")
	occ := fakeOccupancy{pos: "black"}
	res := step(g, occ, primitives.OccupancyEmptyOnly, pos, pos, "black")

	if res.Allowed {
		t.Errorf("Allowed = true, want false (from == to is not a valid move)")
	}
	if res.Reason == "" {
		t.Error("Reason = \"\", want non-empty rejection reason")
	}
}
