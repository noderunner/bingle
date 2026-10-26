package board

import "sync"

// Integer indexes let hot loops (win-rule flood fills, bots' playouts) work
// over slices instead of string-keyed maps. A position's index is its place
// in Positions() order. The graph is immutable once built, so the tables are
// computed lazily on first use and shared; callers must not modify the
// slices they get back.

type graphIndex struct {
	pos      map[PositionID]int
	nbrs     [][]int
	coords   []Coord
	hasCoord []bool
	byCoord  map[Coord]int
}

type rayKey struct {
	from   PositionID
	dx, dy int
}

// rayCache memoises Ray results.
type rayCache struct {
	mu sync.RWMutex
	m  map[rayKey][]PositionID
}

type regionCache struct {
	mu sync.RWMutex
	m  map[string][]int
}

func (g *Graph) index() *graphIndex {
	g.indexOnce.Do(func() {
		n := len(g.order)
		ix := &graphIndex{
			pos:      make(map[PositionID]int, n),
			nbrs:     make([][]int, n),
			coords:   make([]Coord, n),
			hasCoord: make([]bool, n),
			byCoord:  make(map[Coord]int, n),
		}
		for i, id := range g.order {
			ix.pos[id] = i
		}
		for i, id := range g.order {
			for _, nb := range g.adjacency[id] {
				if j, ok := ix.pos[nb]; ok {
					ix.nbrs[i] = append(ix.nbrs[i], j)
				}
			}
			if c, ok := g.coords[id]; ok {
				ix.coords[i], ix.hasCoord[i] = c, true
				ix.byCoord[c] = i
			}
		}
		g.idx = ix
	})
	return g.idx
}

// Index returns the integer index of id (its place in Positions()).
func (g *Graph) Index(id PositionID) (int, bool) {
	i, ok := g.index().pos[id]
	return i, ok
}

// NeighborIndices returns the indexes of the positions adjacent to the one
// with index i. The slice is shared; do not modify it.
func (g *Graph) NeighborIndices(i int) []int { return g.index().nbrs[i] }

// IndexAt returns the index of the position at grid coordinate c.
func (g *Graph) IndexAt(c Coord) (int, bool) {
	i, ok := g.index().byCoord[c]
	return i, ok
}

// CoordOfIndex returns the coordinate of the position with index i.
func (g *Graph) CoordOfIndex(i int) (Coord, bool) {
	ix := g.index()
	return ix.coords[i], ix.hasCoord[i]
}

// RegionIndices returns the indexes of the named region's positions. The
// slice is shared; do not modify it.
func (g *Graph) RegionIndices(name string) ([]int, bool) {
	g.regionIdx.mu.RLock()
	idx, ok := g.regionIdx.m[name]
	g.regionIdx.mu.RUnlock()
	if ok {
		return idx, true
	}
	ids, ok := g.regions[name]
	if !ok {
		return nil, false
	}
	ix := g.index()
	idx = make([]int, 0, len(ids))
	for _, id := range ids {
		if i, ok := ix.pos[id]; ok {
			idx = append(idx, i)
		}
	}
	g.regionIdx.mu.Lock()
	if g.regionIdx.m == nil {
		g.regionIdx.m = map[string][]int{}
	}
	g.regionIdx.m[name] = idx
	g.regionIdx.mu.Unlock()
	return idx, true
}

// Ray returns the positions met walking from `from` in direction (dx, dy)
// over adjacent squares, in order, up to the board's edge (or the first step
// that is not a graph edge). The slice is shared; do not modify it.
func (g *Graph) Ray(from PositionID, dx, dy int) []PositionID {
	key := rayKey{from, dx, dy}
	g.rays.mu.RLock()
	r, ok := g.rays.m[key]
	g.rays.mu.RUnlock()
	if ok {
		return r
	}
	if c, has := g.coords[from]; has {
		prev := from
		for n := 1; ; n++ {
			p, ok := g.byCoord[Coord{X: c.X + n*dx, Y: c.Y + n*dy}]
			if !ok || !g.HasEdge(prev, p) {
				break
			}
			r = append(r, p)
			prev = p
		}
	}
	g.rays.mu.Lock()
	if g.rays.m == nil {
		g.rays.m = map[rayKey][]PositionID{}
	}
	g.rays.m[key] = r
	g.rays.mu.Unlock()
	return r
}
