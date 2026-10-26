package board

import "fmt"

// FromExplicit builds a Graph from a hand-authored list of positions and
// edges, per docs/history/plan.md section 5 step 3 and docs/history/backlog.md item 03. This
// lets boards be defined explicitly (irregular shapes, hand-authored
// layouts) rather than generated procedurally like Generate does for grids.
//
// FromExplicit returns a non-nil error (and a nil *Graph) if:
//   - positions is empty.
//   - positions contains a duplicate PositionID.
//   - an edge references a PositionID not present in positions.
//   - an edge is a self-loop (both PositionIDs in the pair are equal).
//
// Duplicate edges (the same pair given twice, in either order) are not an
// error; they are silently deduplicated.
func FromExplicit(positions []PositionID, edges [][2]PositionID) (*Graph, error) {
	if len(positions) == 0 {
		return nil, fmt.Errorf("board: explicit board must have at least one position")
	}

	seen := make(map[PositionID]bool, len(positions))
	for _, id := range positions {
		if seen[id] {
			return nil, fmt.Errorf("board: duplicate position %q", id)
		}
		seen[id] = true
	}

	for _, edge := range edges {
		a, b := edge[0], edge[1]
		if !seen[a] {
			return nil, fmt.Errorf("board: edge references unknown position %q", a)
		}
		if !seen[b] {
			return nil, fmt.Errorf("board: edge references unknown position %q", b)
		}
		if a == b {
			return nil, fmt.Errorf("board: edge is a self-loop on position %q", a)
		}
	}

	g := newGraph()
	for _, id := range positions {
		g.addPosition(id)
	}
	for _, edge := range edges {
		g.addEdge(edge[0], edge[1])
	}

	return g, nil
}
