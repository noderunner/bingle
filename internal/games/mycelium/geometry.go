package mycelium

import (
	"sync"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
)

// dirVec is the axial step to each of the six neighbours of a hex.
var dirVec = [6][2]int{{1, 0}, {1, -1}, {0, -1}, {-1, 0}, {-1, 1}, {0, 1}}

type geo struct {
	ids     []board.PositionID
	index   map[board.PositionID]int
	nbr     [][]int
	center  board.PositionID
	centerI int
}

var geoCache sync.Map // *catalog.CompiledGame → *geo

func geoOf(game *catalog.CompiledGame) *geo {
	if v, ok := geoCache.Load(game); ok {
		return v.(*geo)
	}
	b := game.Board
	ids := b.Positions()
	g := &geo{ids: ids, index: make(map[board.PositionID]int, len(ids)), nbr: make([][]int, len(ids))}
	for i, id := range ids {
		g.index[id] = i
	}
	for i, id := range ids {
		c, _ := b.Coord(id)
		for _, d := range dirVec {
			if nid, ok := b.At(board.Coord{X: c.X + d[0], Y: c.Y + d[1]}); ok {
				g.nbr[i] = append(g.nbr[i], g.index[nid])
			}
		}
	}
	// The centre of a hex-hex board is its middle axial coordinate. Mycelium
	// uses size 5, whose centre is e5.
	cx, cy := b.Width()/2, b.Height()/2
	if cid, ok := b.At(board.Coord{X: cx, Y: cy}); ok {
		g.center = cid
		g.centerI = g.index[cid]
	}
	v, _ := geoCache.LoadOrStore(game, g)
	return v.(*geo)
}
