// Package board models a game board as a graph: a set of Positions plus an
// adjacency relation between them. This keeps the engine board-shape
// agnostic — grids, hex boards, and hand-authored irregular boards are all
// just graphs — per docs/history/plan.md section 5 step 3.
//
// Graphs may additionally carry layout metadata (grid coordinates, named
// regions) so rules and renderers that need geometry — stack moves along a
// straight line, "connect left to right" win conditions, drawing — can get
// it without parsing position IDs.
package board

import (
	"sort"
	"sync"
)

// PositionID uniquely identifies a Position within a Graph. Grid-generated
// boards use either the "x,y" format (0-based integers, comma-separated; see
// docs/history/plan.md section 6.1's setup entries, e.g. "0,0"/"8,8") or, when the
// generator asks for algebraic labels, chess-style names like "a1" (file
// letter + rank number, rank 1 at the bottom). Hand-authored explicit boards
// may use any non-empty string.
type PositionID string

// Position is a single node on the board graph.
type Position struct {
	ID PositionID
}

// Coord is a position's integer grid coordinate: X grows rightwards, Y grows
// downwards (screen convention), both 0-based.
type Coord struct{ X, Y int }

// Graph is an undirected adjacency graph of board Positions.
type Graph struct {
	positions map[PositionID]Position
	adjacency map[PositionID][]PositionID

	// order lists every position in insertion order, so iteration over the
	// board is deterministic (maps iterate randomly in Go).
	order []PositionID

	// coords/byCoord are populated for grid-generated graphs (and for
	// explicit graphs that declare coordinates); they let callers step in a
	// direction without parsing IDs.
	coords  map[PositionID]Coord
	byCoord map[Coord]PositionID

	// regions maps a region name (e.g. "left", "top") to its positions.
	regions map[string][]PositionID

	width, height int
	kind          string

	// Lazily built integer index tables (see index.go).
	indexOnce sync.Once
	idx       *graphIndex
	regionIdx regionCache
	rays      rayCache
}

// newGraph returns an empty Graph ready to have positions and edges added
// to it.
func newGraph() *Graph {
	return &Graph{
		positions: make(map[PositionID]Position),
		adjacency: make(map[PositionID][]PositionID),
		coords:    make(map[PositionID]Coord),
		byCoord:   make(map[Coord]PositionID),
		regions:   make(map[string][]PositionID),
		kind:      "explicit",
	}
}

// addPosition adds a Position with the given id to the graph, if not
// already present.
func (g *Graph) addPosition(id PositionID) {
	if _, exists := g.positions[id]; exists {
		return
	}
	g.positions[id] = Position{ID: id}
	g.order = append(g.order, id)
}

// addEdge connects a and b with an undirected edge. Both positions must
// already exist in the graph. addEdge is idempotent: adding the same edge
// twice does not duplicate it.
func (g *Graph) addEdge(a, b PositionID) {
	if a == b {
		return
	}
	if !g.HasEdge(a, b) {
		g.adjacency[a] = append(g.adjacency[a], b)
	}
	if !g.HasEdge(b, a) {
		g.adjacency[b] = append(g.adjacency[b], a)
	}
}

// HasPosition reports whether id names a Position in the graph.
func (g *Graph) HasPosition(id PositionID) bool {
	_, ok := g.positions[id]
	return ok
}

// Neighbors returns the positions adjacent to id. It returns an empty slice
// (never nil) if id is not present in the graph.
func (g *Graph) Neighbors(id PositionID) []PositionID {
	neighbors := g.adjacency[id]
	out := make([]PositionID, len(neighbors))
	copy(out, neighbors)
	return out
}

// HasEdge reports whether a and b are adjacent. It returns false if either
// position is absent from the graph, and false for a self-loop (a == b),
// since a position is never its own neighbor.
func (g *Graph) HasEdge(a, b PositionID) bool {
	if a == b {
		return false
	}
	if !g.HasPosition(a) || !g.HasPosition(b) {
		return false
	}
	for _, n := range g.adjacency[a] {
		if n == b {
			return true
		}
	}
	return false
}

// Positions returns every position ID in the graph in a stable order: for
// grids, reading order (left to right, top to bottom); for explicit boards,
// declaration order. The returned slice is a copy.
func (g *Graph) Positions() []PositionID {
	out := make([]PositionID, len(g.order))
	copy(out, g.order)
	return out
}

// Len returns the number of positions on the board.
func (g *Graph) Len() int { return len(g.order) }

// Kind reports how the graph was built: "grid" or "explicit".
func (g *Graph) Kind() string { return g.kind }

// IsGrid reports whether the graph is a regular grid with coordinates.
func (g *Graph) IsGrid() bool { return g.kind == "grid" }

// Width and Height are the grid's dimensions (0 for non-grid graphs).
func (g *Graph) Width() int  { return g.width }
func (g *Graph) Height() int { return g.height }

// Size returns the larger grid dimension (0 for non-grid graphs).
func (g *Graph) Size() int {
	if g.width > g.height {
		return g.width
	}
	return g.height
}

// Coord returns id's grid coordinate, if it has one.
func (g *Graph) Coord(id PositionID) (Coord, bool) {
	c, ok := g.coords[id]
	return c, ok
}

// At returns the position at grid coordinate c, if any.
func (g *Graph) At(c Coord) (PositionID, bool) {
	id, ok := g.byCoord[c]
	return id, ok
}

// Offset returns the position reached from id by moving (dx, dy) grid cells,
// provided both ends have coordinates and the target exists.
func (g *Graph) Offset(id PositionID, dx, dy int) (PositionID, bool) {
	c, ok := g.coords[id]
	if !ok {
		return "", false
	}
	return g.At(Coord{X: c.X + dx, Y: c.Y + dy})
}

// Region returns the named region's positions (a copy), and whether it
// exists. Grid graphs always define "left", "right", "top" and "bottom".
func (g *Graph) Region(name string) ([]PositionID, bool) {
	r, ok := g.regions[name]
	if !ok {
		return nil, false
	}
	out := make([]PositionID, len(r))
	copy(out, r)
	return out, true
}

// RegionNames returns every region name, sorted.
func (g *Graph) RegionNames() []string {
	names := make([]string, 0, len(g.regions))
	for n := range g.regions {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// setRegion stores (or replaces) a named region.
func (g *Graph) setRegion(name string, ids []PositionID) {
	cp := make([]PositionID, len(ids))
	copy(cp, ids)
	g.regions[name] = cp
	g.regionIdx.mu.Lock()
	g.regionIdx.m = nil
	g.regionIdx.mu.Unlock()
}

// SetCoord records id's coordinate (used by explicit boards that declare
// layout). It is a no-op for unknown positions.
func (g *Graph) SetCoord(id PositionID, c Coord) {
	if !g.HasPosition(id) {
		return
	}
	g.coords[id] = c
	g.byCoord[c] = id
}

// DefineRegion adds a named region to the graph. Every id must already be a
// position of the graph; unknown ids are reported by returning false and
// leaving the graph unchanged.
func (g *Graph) DefineRegion(name string, ids []PositionID) bool {
	for _, id := range ids {
		if !g.HasPosition(id) {
			return false
		}
	}
	g.setRegion(name, ids)
	return true
}
