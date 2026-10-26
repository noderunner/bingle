package board

import (
	"fmt"
	"strconv"
)

// hexSteps are the six axial neighbor offsets (dq, dr) of a hex cell on a
// parallelogram ("rhombus") board such as the classic Hex board.
var hexSteps = []orthogonalStep{
	{dx: 1, dy: 0},
	{dx: -1, dy: 0},
	{dx: 0, dy: 1},
	{dx: 0, dy: -1},
	{dx: 1, dy: -1},
	{dx: -1, dy: 1},
}

// GenerateHex builds a width x height parallelogram of hexagonal cells in
// axial coordinates (q across, r down). Position IDs are a column letter plus
// a 1-based row number counted from the top ("a1" is the top-left corner),
// the notation Hex players use. Coord{X: q, Y: r} is recorded for every
// position, and the board carries the four edge regions "left" and "right"
// (columns), "top" and "bottom" (rows).
//
// Adjacency: (q±1, r), (q, r±1), (q+1, r-1) and (q-1, r+1), so interior cells
// have six neighbours, edge cells four, the acute corners (top-left and
// bottom-right) two and the obtuse corners (top-right and bottom-left) three.
func GenerateHex(width, height int) (*Graph, error) {
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("board: hex board dimensions must be positive, got %dx%d", width, height)
	}
	if width > 26 {
		return nil, fmt.Errorf("board: hex boards support at most 26 columns, got %d", width)
	}
	if height > MaxCells || width*height > MaxCells {
		return nil, fmt.Errorf("board: a %dx%d hex board exceeds the %d-position limit", width, height, MaxCells)
	}

	name := func(q, r int) PositionID {
		return PositionID(string(rune('a'+q)) + strconv.Itoa(r+1))
	}

	g := newGraph()
	g.kind = "hex"
	g.width, g.height = width, height

	for r := 0; r < height; r++ {
		for q := 0; q < width; q++ {
			id := name(q, r)
			g.addPosition(id)
			g.SetCoord(id, Coord{X: q, Y: r})
		}
	}
	for q := 0; q < width; q++ {
		for r := 0; r < height; r++ {
			for _, st := range hexSteps {
				nq, nr := q+st.dx, r+st.dy
				if nq < 0 || nq >= width || nr < 0 || nr >= height {
					continue
				}
				g.addEdge(name(q, r), name(nq, nr))
			}
		}
	}

	var left, right, top, bottom []PositionID
	for r := 0; r < height; r++ {
		left = append(left, name(0, r))
		right = append(right, name(width-1, r))
	}
	for q := 0; q < width; q++ {
		top = append(top, name(q, 0))
		bottom = append(bottom, name(q, height-1))
	}
	g.setRegion("left", left)
	g.setRegion("right", right)
	g.setRegion("top", top)
	g.setRegion("bottom", bottom)

	return g, nil
}

// GenerateHexHex builds a hexagon-shaped board with size cells along each
// side (size 5 = 61 cells, the Yavalath board; size 8 = 169 cells, the
// Havannah board). It uses the same axial layout as GenerateHex, translated
// so every coordinate is non-negative; the cells of the enclosing
// (2*size-1) square that fall outside the hexagon simply do not exist.
// Position IDs are a column letter plus a 1-based row number, as for the
// parallelogram board.
//
// Regions: "edge" (the outer ring), "corners" (its six corners), each corner
// on its own ("corner-0" top-left, then clockwise: top-right, right,
// bottom-right, bottom-left, left) and the six sides without their corners:
// "top", "bottom", "upper-left", "lower-left", "upper-right" and
// "lower-right".
func GenerateHexHex(size int) (*Graph, error) {
	if size < 2 {
		return nil, fmt.Errorf("board: a hexhex board needs size >= 2, got %d", size)
	}
	if size > 13 {
		return nil, fmt.Errorf("board: hexhex boards support at most size 13, got %d", size)
	}
	n := size - 1
	span := 2*size - 1
	abs := func(v int) int {
		if v < 0 {
			return -v
		}
		return v
	}
	// (cq, cr) are centred axial coordinates in [-n, n].
	inside := func(cq, cr int) bool { return abs(cq) <= n && abs(cr) <= n && abs(cq+cr) <= n }
	name := func(q, r int) PositionID {
		return PositionID(string(rune('a'+q)) + strconv.Itoa(r+1))
	}

	g := newGraph()
	g.kind = "hex"
	g.width, g.height = span, span

	for r := 0; r < span; r++ {
		for q := 0; q < span; q++ {
			if !inside(q-n, r-n) {
				continue
			}
			id := name(q, r)
			g.addPosition(id)
			g.SetCoord(id, Coord{X: q, Y: r})
		}
	}
	for _, id := range g.order {
		c := g.coords[id]
		for _, st := range hexSteps {
			nq, nr := c.X+st.dx, c.Y+st.dy
			if nq < 0 || nq >= span || nr < 0 || nr >= span || !inside(nq-n, nr-n) {
				continue
			}
			g.addEdge(id, name(nq, nr))
		}
	}

	var edge, corners, top, bottom, ul, ll, ur, lr []PositionID
	var cornerOf [6]PositionID
	for _, id := range g.order {
		c := g.coords[id]
		cq, cr := c.X-n, c.Y-n
		ring := abs(cq) == n || abs(cr) == n || abs(cq+cr) == n
		if !ring {
			continue
		}
		edge = append(edge, id)
		// A corner lies on two sides at once.
		sides := 0
		for _, on := range []bool{cr == -n, cr == n, cq+cr == -n, cq == -n, cq == n, cq+cr == n} {
			if on {
				sides++
			}
		}
		if sides >= 2 {
			corners = append(corners, id)
			switch {
			case cr == -n && cq == 0:
				cornerOf[0] = id
			case cr == -n && cq == n:
				cornerOf[1] = id
			case cq == n && cr == 0:
				cornerOf[2] = id
			case cr == n && cq == 0:
				cornerOf[3] = id
			case cq == -n && cr == n:
				cornerOf[4] = id
			default:
				cornerOf[5] = id
			}
			continue
		}
		switch {
		case cr == -n:
			top = append(top, id)
		case cr == n:
			bottom = append(bottom, id)
		case cq+cr == -n:
			ul = append(ul, id)
		case cq == -n:
			ll = append(ll, id)
		case cq == n:
			ur = append(ur, id)
		case cq+cr == n:
			lr = append(lr, id)
		}
	}
	g.setRegion("edge", edge)
	g.setRegion("corners", corners)
	g.setRegion("top", top)
	g.setRegion("bottom", bottom)
	g.setRegion("upper-left", ul)
	g.setRegion("lower-left", ll)
	g.setRegion("upper-right", ur)
	g.setRegion("lower-right", lr)
	for i, id := range cornerOf {
		g.setRegion("corner-"+strconv.Itoa(i), []PositionID{id})
	}
	return g, nil
}

// IsHex reports whether the graph is a hexagonal board.
func (g *Graph) IsHex() bool { return g.kind == "hex" }
