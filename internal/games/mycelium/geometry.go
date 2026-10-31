package mycelium

import (
	"sync"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
)

// The board is a hex-hex of cells, so every cell has six neighbours and lies
// on three straight lines. dirVec is the axial step to each neighbour, going
// round.
var dirVec = [6][2]int{{1, 0}, {1, -1}, {0, -1}, {-1, 0}, {-1, 1}, {0, 1}}

// geo is the board in integer form: the cells, who touches whom, the line of
// sight from every cell in every direction, and the centre.
type geo struct {
	ids   []board.PositionID
	index map[board.PositionID]int
	// nbr[i] are the cells that touch i.
	nbr [][]int
	// ray[i][d] are the cells along direction d from i, nearest first.
	ray [][6][]int
	// centre is the cell whose occupation starts a confluence.
	centre int
}

var geoCache sync.Map // *catalog.CompiledGame → *geo

func geoOf(game *catalog.CompiledGame) *geo {
	if v, ok := geoCache.Load(game); ok {
		return v.(*geo)
	}
	g := buildGeo(game)
	geoCache.Store(game, g)
	return g
}

func buildGeo(game *catalog.CompiledGame) *geo {
	b := game.Board
	ids := b.Positions()
	g := &geo{ids: ids, index: make(map[board.PositionID]int, len(ids)), nbr: make([][]int, len(ids)), ray: make([][6][]int, len(ids))}
	at := map[[2]int]int{}
	coord := make([][2]int, len(ids))
	for i, id := range ids {
		g.index[id] = i
		c, _ := b.Coord(id)
		coord[i] = [2]int{c.X, c.Y}
		at[coord[i]] = i
	}
	for i := range ids {
		for d, step := range dirVec {
			q, r := coord[i][0], coord[i][1]
			for {
				q, r = q+step[0], r+step[1]
				j, ok := at[[2]int{q, r}]
				if !ok {
					break
				}
				g.ray[i][d] = append(g.ray[i][d], j)
			}
			if len(g.ray[i][d]) > 0 {
				g.nbr[i] = append(g.nbr[i], g.ray[i][d][0])
			}
		}
	}
	// The centre is the one cell with the same number of cells along every
	// ray (e5 on the size-5 board).
	g.centre = -1
	for i := range ids {
		n, same := len(g.ray[i][0]), true
		for d := 1; d < 6; d++ {
			same = same && len(g.ray[i][d]) == n
		}
		if same && n > 0 {
			g.centre = i
		}
	}
	return g
}
