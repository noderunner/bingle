package meridians

import (
	"sync"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
)

// The board is the intersections of a triangular grid, so every point has six
// neighbours and lies on three straight lines. dirVec is the axial step to
// each neighbour, going round; the first three and the last three are the two
// ways along the board's three directions.
var dirVec = [6][2]int{{1, 0}, {1, -1}, {0, -1}, {-1, 0}, {-1, 1}, {0, 1}}

// geo is the board in integer form: the points, who touches whom, and the
// line of sight from every point in every direction.
type geo struct {
	ids   []board.PositionID
	index map[board.PositionID]int
	// nbr[i] are the points that touch i.
	nbr [][]int
	// ray[i][d] are the points along direction d from i, nearest first.
	ray [][6][]int
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
	for i, id := range ids {
		g.index[id] = i
	}
	at := map[[2]int]int{}
	coord := make([][2]int, len(ids))
	for i, id := range ids {
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
	return g
}
