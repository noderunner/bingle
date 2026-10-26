package fendo

import (
	"strconv"
	"sync"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
)

// Sides of a cell are numbered 0 to 3: north (towards the top of the screen,
// the higher rank), east, south, west.
var sideLetter = [4]string{"n", "e", "s", "w"}
var sideName = [4]string{"north", "east", "south", "west"}
var sideStep = [4][2]int{{0, 1}, {1, 0}, {0, -1}, {-1, 0}}

func opposite(d int) int { return (d + 2) % 4 }

func sideOf(letter string) (int, bool) {
	for d, l := range sideLetter {
		if l == letter {
			return d, true
		}
	}
	return 0, false
}

type geo struct {
	ids   []board.PositionID
	index map[board.PositionID]int
	// nbr[i][d] is the cell beside i on side d, or -1 at the edge of the board.
	nbr  [][4]int
	w, h int
}

var geoCache sync.Map // *catalog.CompiledGame → *geo

// parseCell reads an algebraic id such as "c4" as column 2, rank 4.
func parseCell(id board.PositionID) (col, rank int, ok bool) {
	s := string(id)
	if len(s) < 2 || s[0] < 'a' || s[0] > 'z' {
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
	g := &geo{ids: ids, index: make(map[board.PositionID]int, len(ids)), nbr: make([][4]int, len(ids))}
	at := map[[2]int]int{}
	for i, id := range ids {
		g.index[id] = i
		c, r, _ := parseCell(id)
		at[[2]int{c, r}] = i
		if c+1 > g.w {
			g.w = c + 1
		}
		if r > g.h {
			g.h = r
		}
	}
	for i, id := range ids {
		c, r, _ := parseCell(id)
		for d, v := range sideStep {
			g.nbr[i][d] = -1
			if j, ok := at[[2]int{c + v[0], r + v[1]}]; ok {
				g.nbr[i][d] = j
			}
		}
	}
	v, _ := geoCache.LoadOrStore(game, g)
	return v.(*geo)
}
