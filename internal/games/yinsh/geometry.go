package yinsh

import (
	"strconv"
	"sync"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
)

// The points are named like the published Yinsh board: a column letter a–k
// and a row number 1–11 (a2–a5, b1–b7, … k7–k10). The lines of the board run
// in three directions, so a point touches up to six others: its column
// neighbours (row ±1), its row neighbours (column ±1) and the diagonal ones
// (column and row both ±1).
var dirVec = [6][2]int{{0, 1}, {0, -1}, {1, 0}, {-1, 0}, {1, 1}, {-1, -1}}

type geo struct {
	ids   []board.PositionID
	index map[board.PositionID]int
	// nbr[i][d] is the point beside i in direction d, or -1 at the edge.
	nbr [][6]int
}

var geoCache sync.Map // *catalog.CompiledGame → *geo

func parsePoint(id board.PositionID) (col, row int, ok bool) {
	s := string(id)
	if len(s) < 2 || s[0] < 'a' || s[0] > 'k' {
		return 0, 0, false
	}
	n, err := strconv.Atoi(s[1:])
	if err != nil {
		return 0, 0, false
	}
	return int(s[0] - 'a'), n, true
}

func geoOf(game *catalog.CompiledGame) *geo {
	if v, ok := geoCache.Load(game); ok {
		return v.(*geo)
	}
	ids := game.Board.Positions()
	g := &geo{ids: ids, index: make(map[board.PositionID]int, len(ids)), nbr: make([][6]int, len(ids))}
	at := map[[2]int]int{}
	for i, id := range ids {
		g.index[id] = i
		c, r, _ := parsePoint(id)
		at[[2]int{c, r}] = i
	}
	for i, id := range ids {
		c, r, _ := parsePoint(id)
		for d, v := range dirVec {
			g.nbr[i][d] = -1
			if j, ok := at[[2]int{c + v[0], r + v[1]}]; ok {
				g.nbr[i][d] = j
			}
		}
	}
	v, _ := geoCache.LoadOrStore(game, g)
	return v.(*geo)
}
