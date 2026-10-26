package hive

import (
	"sync"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
)

// dirVec is the axial step to each of the six neighbours of a hex, in order
// round the cell (so direction d+1 and d-1 flank direction d).
var dirVec = [6][2]int{{1, 0}, {1, -1}, {0, -1}, {-1, 0}, {-1, 1}, {0, 1}}

type geo struct {
	ids   []board.PositionID
	index map[board.PositionID]int
	// nbr[i][d] is the neighbour of cell i in direction d, or -1 at the edge.
	nbr    [][6]int
	center int
}

var geoCache sync.Map // *catalog.CompiledGame → *geo

func geoOf(game *catalog.CompiledGame) *geo {
	if v, ok := geoCache.Load(game); ok {
		return v.(*geo)
	}
	b := game.Board
	ids := b.Positions()
	g := &geo{ids: ids, index: make(map[board.PositionID]int, len(ids)), nbr: make([][6]int, len(ids))}
	maxX := 0
	for i, id := range ids {
		g.index[id] = i
		if c, _ := b.Coord(id); c.X > maxX {
			maxX = c.X
		}
	}
	for i, id := range ids {
		c, _ := b.Coord(id)
		for d, v := range dirVec {
			g.nbr[i][d] = -1
			if nid, ok := b.At(board.Coord{X: c.X + v[0], Y: c.Y + v[1]}); ok {
				g.nbr[i][d] = g.index[nid]
			}
		}
	}
	// The middle of a hexhex board: both axial coordinates are half the span.
	if id, ok := b.At(board.Coord{X: maxX / 2, Y: maxX / 2}); ok {
		g.center = g.index[id]
	}
	v, _ := geoCache.LoadOrStore(game, g)
	return v.(*geo)
}
