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
	return GenerateHexHexSides(size, size)
}

// GenerateHexHexSides builds a hexagon whose sides are not all the same
// length: four of them have long cells and the two between them short, which
// makes a hexagon with **no middle cell** (its centre of symmetry falls
// between cells). Meridians is played on the 6/7 board of 114 points; with
// short == long it is the ordinary hexagon of GenerateHexHex.
//
// The shape is the same axial picture as GenerateHexHex, with the bounds no
// longer symmetric: short + long - 1 columns, 2*long - 1 rows, and the
// diagonal band long-1 <= q+r <= (columns-1) + (rows-1) - (long-1).
func GenerateHexHexSides(short, long int) (*Graph, error) {
	if short < 2 || long < 2 {
		return nil, fmt.Errorf("board: a hexhex board needs sides >= 2, got %d and %d", short, long)
	}
	if short > 13 || long > 13 {
		return nil, fmt.Errorf("board: hexhex boards support sides up to 13, got %d and %d", short, long)
	}
	if long < short {
		return nil, fmt.Errorf("board: the second side of a hexhex board (%d) cannot be shorter than the first (%d)", long, short)
	}
	width := short + long - 1
	height := 2*long - 1
	lo := long - 1
	hi := (width - 1) + (height - 1) - lo
	inside := func(q, r int) bool {
		return q >= 0 && q < width && r >= 0 && r < height && q+r >= lo && q+r <= hi
	}
	name := func(q, r int) PositionID {
		return PositionID(string(rune('a'+q)) + strconv.Itoa(r+1))
	}

	g := newGraph()
	g.kind = "hex"
	g.width, g.height = width, height

	for r := 0; r < height; r++ {
		for q := 0; q < width; q++ {
			if !inside(q, r) {
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
			if !inside(nq, nr) {
				continue
			}
			g.addEdge(id, name(nq, nr))
		}
	}

	var edge, corners, top, bottom, ul, ll, ur, lr []PositionID
	var cornerOf [6]PositionID
	for _, id := range g.order {
		c := g.coords[id]
		q, r := c.X, c.Y
		onTop, onBottom := r == 0, r == height-1
		onLeft, onRight := q == 0, q == width-1
		onUL, onLR := q+r == lo, q+r == hi
		onEdge := onTop || onBottom || onLeft || onRight || onUL || onLR
		if !onEdge {
			continue
		}
		edge = append(edge, id)
		// A corner lies on two sides at once.
		sides := 0
		for _, on := range []bool{onTop, onBottom, onUL, onLeft, onRight, onLR} {
			if on {
				sides++
			}
		}
		if sides >= 2 {
			corners = append(corners, id)
			switch {
			case onTop && onUL:
				cornerOf[0] = id
			case onTop && onRight:
				cornerOf[1] = id
			case onRight && onLR:
				cornerOf[2] = id
			case onBottom && onLR:
				cornerOf[3] = id
			case onBottom && onLeft:
				cornerOf[4] = id
			default:
				cornerOf[5] = id
			}
			continue
		}
		switch {
		case onTop:
			top = append(top, id)
		case onBottom:
			bottom = append(bottom, id)
		case onUL:
			ul = append(ul, id)
		case onLeft:
			ll = append(ll, id)
		case onRight:
			ur = append(ur, id)
		case onLR:
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
