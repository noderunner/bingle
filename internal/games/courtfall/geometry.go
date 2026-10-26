package courtfall

import (
	"sync"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
)

// dirNames names the six hex directions in the order geo.nbr uses them.
// E and W run along a row; NE/NW go up the screen and SE/SW down it.
var dirNames = [6]string{"E", "NE", "NW", "W", "SW", "SE"}

// dirVec is each direction's axial (dq, dr) step.
var dirVec = [6][2]int{{1, 0}, {1, -1}, {0, -1}, {-1, 0}, {-1, 1}, {0, 1}}

func dirIndex(name string) int {
	for i, n := range dirNames {
		if n == name {
			return i
		}
	}
	return -1
}

// geo is the arena's geometry in integer form.
type geo struct {
	ids   []board.PositionID
	index map[board.PositionID]int
	// nbr[c][d] is the hex next to c in direction d, or -1 off the board.
	nbr [][6]int
	// ray[c][d] lists the hexes along direction d from c, nearest first
	// (at most 4: the longest shot).
	ray  [][6][]int
	dist [][]int8
	// zone[c] is the seat (1 or 2) whose deployment zone holds c, else 0.
	zone []int8
	eye  int
}

var geoCache sync.Map // *catalog.CompiledGame → *geo

func geoOf(game *catalog.CompiledGame) *geo {
	if v, ok := geoCache.Load(game); ok {
		return v.(*geo)
	}
	g := buildGeo(game)
	v, _ := geoCache.LoadOrStore(game, g)
	return v.(*geo)
}

func buildGeo(game *catalog.CompiledGame) *geo {
	b := game.Board
	ids := b.Positions()
	g := &geo{ids: ids, index: make(map[board.PositionID]int, len(ids)), eye: -1}
	for i, id := range ids {
		g.index[id] = i
	}
	n := len(ids)
	g.nbr = make([][6]int, n)
	g.ray = make([][6][]int, n)
	g.dist = make([][]int8, n)
	g.zone = make([]int8, n)
	coords := make([]board.Coord, n)
	for i, id := range ids {
		c, _ := b.Coord(id)
		coords[i] = c
	}
	for i := range ids {
		for d := 0; d < 6; d++ {
			g.nbr[i][d] = -1
			c := coords[i]
			cur := i
			for step := 1; step <= 4; step++ {
				cc := board.Coord{X: coords[cur].X + dirVec[d][0], Y: coords[cur].Y + dirVec[d][1]}
				id, ok := b.At(cc)
				if !ok {
					break
				}
				j := g.index[id]
				if step == 1 {
					g.nbr[i][d] = j
				}
				g.ray[i][d] = append(g.ray[i][d], j)
				cur = j
			}
			_ = c
		}
		g.dist[i] = make([]int8, n)
		for j := range ids {
			dq := coords[j].X - coords[i].X
			dr := coords[j].Y - coords[i].Y
			g.dist[i][j] = int8(max3(abs(dq), abs(dr), abs(dq+dr)))
		}
	}
	for z, region := range map[int8]string{1: "zone-1", 2: "zone-2"} {
		if members, ok := b.Region(region); ok {
			for _, id := range members {
				if i, ok := g.index[id]; ok {
					g.zone[i] = z
				}
			}
		}
	}
	if members, ok := b.Region("eye"); ok && len(members) == 1 {
		g.eye = g.index[members[0]]
	}
	return g
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func max3(a, b, c int) int {
	m := a
	if b > m {
		m = b
	}
	if c > m {
		m = c
	}
	return m
}

// dirBetween is the direction from cell a to an adjacent cell b, or -1.
func (g *geo) dirBetween(a, b int) int {
	for d := 0; d < 6; d++ {
		if g.nbr[a][d] == b {
			return d
		}
	}
	return -1
}
