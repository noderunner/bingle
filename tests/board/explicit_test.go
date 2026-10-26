// Tests of boards built from an explicit list of positions and edges: adjacency, and the errors for empty, duplicate or dangling definitions.

package board_test

import (
	"testing"

	"github.com/noderunner/bingle/internal/engine/board"
)

// --- The AC itself: 3 positions, 2 edges, exact adjacency -------------------

func TestFromExplicit_ThreePositionsTwoEdges_ExactAdjacency(t *testing.T) {
	positions := []board.PositionID{"p1", "p2", "p3"}
	edges := [][2]board.PositionID{
		{"p1", "p2"},
		{"p2", "p3"},
	}

	g, err := board.FromExplicit(positions, edges)
	if err != nil {
		t.Fatalf("FromExplicit returned unexpected error: %v", err)
	}
	if g == nil {
		t.Fatalf("FromExplicit returned nil graph with nil error")
	}

	// All given positions present.
	for _, p := range positions {
		if !g.HasPosition(p) {
			t.Errorf("HasPosition(%s) = false, want true", p)
		}
	}

	// Nothing extra.
	for _, other := range []board.PositionID{"p4", "nonexistent", ""} {
		if g.HasPosition(other) {
			t.Errorf("HasPosition(%s) = true, want false (not in input positions)", other)
		}
	}

	// Exactly the given edges, both directions (undirected).
	if !g.HasEdge("p1", "p2") {
		t.Errorf("HasEdge(p1,p2) = false, want true")
	}
	if !g.HasEdge("p2", "p1") {
		t.Errorf("HasEdge(p2,p1) = false, want true (undirected)")
	}
	if !g.HasEdge("p2", "p3") {
		t.Errorf("HasEdge(p2,p3) = false, want true")
	}
	if !g.HasEdge("p3", "p2") {
		t.Errorf("HasEdge(p3,p2) = false, want true (undirected)")
	}

	// Not directly connected: no inferred/extra edges.
	if g.HasEdge("p1", "p3") {
		t.Errorf("HasEdge(p1,p3) = true, want false (not a given edge)")
	}
	if g.HasEdge("p3", "p1") {
		t.Errorf("HasEdge(p3,p1) = true, want false (not a given edge)")
	}

	// Neighbor sets match exactly (order-independent).
	assertNeighborSet(t, g, "p1", []board.PositionID{"p2"})
	assertNeighborSet(t, g, "p2", []board.PositionID{"p1", "p3"})
	assertNeighborSet(t, g, "p3", []board.PositionID{"p2"})
}

// assertNeighborSet fails the test if g.Neighbors(id) doesn't contain
// exactly the elements of want, order-independent.
func assertNeighborSet(t *testing.T, g *board.Graph, id board.PositionID, want []board.PositionID) {
	t.Helper()
	got := g.Neighbors(id)
	if len(got) != len(want) {
		t.Errorf("Neighbors(%s) = %v (len %d), want %v (len %d)", id, got, len(got), want, len(want))
		return
	}
	wantSet := make(map[board.PositionID]bool, len(want))
	for _, w := range want {
		wantSet[w] = true
	}
	for _, g := range got {
		if !wantSet[g] {
			t.Errorf("Neighbors(%s) = %v, contains unexpected member %s (want %v)", id, got, g, want)
		}
	}
}

// --- Validation: zero positions ---------------------------------------------

func TestFromExplicit_ZeroPositions_ReturnsErrorAndNilGraph(t *testing.T) {
	g, err := board.FromExplicit(nil, nil)
	if err == nil {
		t.Errorf("FromExplicit(nil, nil) expected non-nil error, got nil (graph %v)", g)
	}
	if g != nil {
		t.Errorf("FromExplicit(nil, nil) expected nil graph on error, got %v", g)
	}
}

func TestFromExplicit_EmptyPositionsSlice_ReturnsErrorAndNilGraph(t *testing.T) {
	g, err := board.FromExplicit([]board.PositionID{}, [][2]board.PositionID{})
	if err == nil {
		t.Errorf("FromExplicit(empty slice, empty edges) expected non-nil error, got nil (graph %v)", g)
	}
	if g != nil {
		t.Errorf("FromExplicit(empty slice, empty edges) expected nil graph on error, got %v", g)
	}
}

// --- Validation: duplicate position id --------------------------------------

func TestFromExplicit_DuplicatePosition_ReturnsErrorAndNilGraph(t *testing.T) {
	positions := []board.PositionID{"p1", "p2", "p1"}
	edges := [][2]board.PositionID{{"p1", "p2"}}

	g, err := board.FromExplicit(positions, edges)
	if err == nil {
		t.Errorf("FromExplicit with duplicate position %q expected non-nil error, got nil (graph %v)", "p1", g)
	}
	if g != nil {
		t.Errorf("FromExplicit with duplicate position expected nil graph on error, got %v", g)
	}
}

// --- Validation: edge references unknown position ---------------------------

func TestFromExplicit_EdgeReferencesUnknownPosition_ReturnsErrorAndNilGraph(t *testing.T) {
	positions := []board.PositionID{"p1", "p2", "p3"}
	edges := [][2]board.PositionID{
		{"p1", "p2"},
		{"p2", "ghost"}, // "ghost" not in positions
	}

	g, err := board.FromExplicit(positions, edges)
	if err == nil {
		t.Errorf("FromExplicit with edge referencing unknown position %q expected non-nil error, got nil (graph %v)", "ghost", g)
	}
	if g != nil {
		t.Errorf("FromExplicit with edge referencing unknown position expected nil graph on error, got %v", g)
	}
}

func TestFromExplicit_EdgeReferencesUnknownPositionAsFirstElement_ReturnsError(t *testing.T) {
	positions := []board.PositionID{"p1", "p2"}
	edges := [][2]board.PositionID{
		{"ghost", "p1"}, // unknown position as the first element of the pair
	}

	g, err := board.FromExplicit(positions, edges)
	if err == nil {
		t.Errorf("FromExplicit with unknown first-element edge expected non-nil error, got nil (graph %v)", g)
	}
	if g != nil {
		t.Errorf("FromExplicit with unknown first-element edge expected nil graph on error, got %v", g)
	}
}

// --- Validation: self-loop edge ----------------------------------------------

func TestFromExplicit_SelfLoopEdge_ReturnsErrorAndNilGraph(t *testing.T) {
	positions := []board.PositionID{"p1", "p2", "p3"}
	edges := [][2]board.PositionID{
		{"p1", "p2"},
		{"p3", "p3"}, // self-loop
	}

	g, err := board.FromExplicit(positions, edges)
	if err == nil {
		t.Errorf("FromExplicit with self-loop edge (p3,p3) expected non-nil error, got nil (graph %v)", g)
	}
	if g != nil {
		t.Errorf("FromExplicit with self-loop edge expected nil graph on error, got %v", g)
	}
}

// --- Duplicate edges (either order) are deduplicated, not an error ----------

func TestFromExplicit_DuplicateEdgeSameOrder_NoErrorAndNoDuplication(t *testing.T) {
	positions := []board.PositionID{"p1", "p2"}
	edges := [][2]board.PositionID{
		{"p1", "p2"},
		{"p1", "p2"}, // exact duplicate
	}

	g, err := board.FromExplicit(positions, edges)
	if err != nil {
		t.Fatalf("FromExplicit with duplicate identical edge returned unexpected error: %v", err)
	}
	if g == nil {
		t.Fatalf("FromExplicit returned nil graph with nil error")
	}

	if !g.HasEdge("p1", "p2") {
		t.Errorf("HasEdge(p1,p2) = false, want true")
	}
	if !g.HasEdge("p2", "p1") {
		t.Errorf("HasEdge(p2,p1) = false, want true")
	}
	if got := len(g.Neighbors("p1")); got != 1 {
		t.Errorf("Neighbors(p1) len = %d, want 1 (no duplication), got %v", got, g.Neighbors("p1"))
	}
	if got := len(g.Neighbors("p2")); got != 1 {
		t.Errorf("Neighbors(p2) len = %d, want 1 (no duplication), got %v", got, g.Neighbors("p2"))
	}
}

func TestFromExplicit_DuplicateEdgeReversedOrder_NoErrorAndNoDuplication(t *testing.T) {
	positions := []board.PositionID{"p1", "p2"}
	edges := [][2]board.PositionID{
		{"p1", "p2"},
		{"p2", "p1"}, // same edge, reversed order
	}

	g, err := board.FromExplicit(positions, edges)
	if err != nil {
		t.Fatalf("FromExplicit with duplicate reversed-order edge returned unexpected error: %v", err)
	}
	if g == nil {
		t.Fatalf("FromExplicit returned nil graph with nil error")
	}

	if !g.HasEdge("p1", "p2") {
		t.Errorf("HasEdge(p1,p2) = false, want true")
	}
	if !g.HasEdge("p2", "p1") {
		t.Errorf("HasEdge(p2,p1) = false, want true")
	}
	if got := len(g.Neighbors("p1")); got != 1 {
		t.Errorf("Neighbors(p1) len = %d, want 1 (no duplication from reversed-order duplicate), got %v", got, g.Neighbors("p1"))
	}
	if got := len(g.Neighbors("p2")); got != 1 {
		t.Errorf("Neighbors(p2) len = %d, want 1 (no duplication from reversed-order duplicate), got %v", got, g.Neighbors("p2"))
	}
}

// --- Single position, zero edges is valid -----------------------------------

func TestFromExplicit_SinglePositionZeroEdges_Valid(t *testing.T) {
	g, err := board.FromExplicit([]board.PositionID{"solo"}, nil)
	if err != nil {
		t.Fatalf("FromExplicit with single position and no edges returned unexpected error: %v", err)
	}
	if g == nil {
		t.Fatalf("FromExplicit returned nil graph with nil error")
	}
	if !g.HasPosition("solo") {
		t.Errorf("HasPosition(solo) = false, want true")
	}
	if got := len(g.Neighbors("solo")); got != 0 {
		t.Errorf("Neighbors(solo) len = %d, want 0, got %v", got, g.Neighbors("solo"))
	}
}

func TestFromExplicit_SinglePositionEmptyEdgesSlice_Valid(t *testing.T) {
	g, err := board.FromExplicit([]board.PositionID{"solo"}, [][2]board.PositionID{})
	if err != nil {
		t.Fatalf("FromExplicit with single position and empty edges slice returned unexpected error: %v", err)
	}
	if g == nil {
		t.Fatalf("FromExplicit returned nil graph with nil error")
	}
	if !g.HasPosition("solo") {
		t.Errorf("HasPosition(solo) = false, want true")
	}
	if got := len(g.Neighbors("solo")); got != 0 {
		t.Errorf("Neighbors(solo) len = %d, want 0, got %v", got, g.Neighbors("solo"))
	}
}
