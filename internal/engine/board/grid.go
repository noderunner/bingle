package board

import (
	"fmt"
	"strconv"
)

// orthogonalStep is a single (dx, dy) offset used to find a position's
// in-bounds neighbors on a grid.
type orthogonalStep struct{ dx, dy int }

// orthogonal4Steps are the four cardinal (up/down/left/right) neighbor
// offsets used by the "orthogonal4" adjacency mode.
var orthogonal4Steps = []orthogonalStep{
	{dx: 0, dy: -1},
	{dx: 0, dy: 1},
	{dx: -1, dy: 0},
	{dx: 1, dy: 0},
}

// diagonalSteps are the four additional diagonal neighbor offsets added by
// the "orthogonal8" adjacency mode on top of orthogonal4Steps.
var diagonalSteps = []orthogonalStep{
	{dx: -1, dy: -1},
	{dx: 1, dy: -1},
	{dx: -1, dy: 1},
	{dx: 1, dy: 1},
}

// Labeling selects how a generated grid names its positions.
type Labeling string

const (
	// LabelXY names positions "x,y" with 0-based integers (the default).
	LabelXY Labeling = "xy"
	// LabelAlgebraic names positions like chess squares: file letter
	// ("a" = leftmost column) followed by rank number (1 = bottom row).
	LabelAlgebraic Labeling = "algebraic"
)

// Direction is a named grid direction with its (dx, dy) offset. Y grows
// downwards, so North has dy = -1 (towards a higher algebraic rank).
type Direction struct {
	Name   string
	DX, DY int
}

// Orthogonal4Directions lists the four cardinal directions in a fixed order.
var Orthogonal4Directions = []Direction{
	{Name: "N", DX: 0, DY: -1},
	{Name: "E", DX: 1, DY: 0},
	{Name: "S", DX: 0, DY: 1},
	{Name: "W", DX: -1, DY: 0},
}

// Diagonal4Directions lists the four diagonal directions.
var Diagonal4Directions = []Direction{
	{Name: "NE", DX: 1, DY: -1},
	{Name: "SE", DX: 1, DY: 1},
	{Name: "SW", DX: -1, DY: 1},
	{Name: "NW", DX: -1, DY: -1},
}

// Orthogonal8Directions adds the four diagonals to Orthogonal4Directions.
var Orthogonal8Directions = []Direction{
	{Name: "N", DX: 0, DY: -1},
	{Name: "NE", DX: 1, DY: -1},
	{Name: "E", DX: 1, DY: 0},
	{Name: "SE", DX: 1, DY: 1},
	{Name: "S", DX: 0, DY: 1},
	{Name: "SW", DX: -1, DY: 1},
	{Name: "W", DX: -1, DY: 0},
	{Name: "NW", DX: -1, DY: -1},
}

// GridOptions configures GenerateGrid.
type GridOptions struct {
	Width, Height int
	// Mode is the adjacency mode: "orthogonal4" or "orthogonal8".
	Mode string
	// Labels selects position naming; empty means LabelXY.
	Labels Labeling
}

// Generate builds a Graph of width*height positions laid out on a grid,
// one Position per (x, y) for x in [0, width) and y in [0, height), with
// PositionID formatted as "x,y" (0-based, comma-separated — see
// docs/history/plan.md section 6.1). mode selects which cells are connected:
//
//   - "orthogonal4" connects each position to its in-bounds up/down/left/
//     right neighbors only.
//   - "orthogonal8" additionally connects the four in-bounds diagonal
//     neighbors.
//   - "diagonal4" connects only the four diagonal neighbors (as on a
//     draughts board, where pieces never move orthogonally).
//
// Generate returns a non-nil error (and a nil *Graph) if width <= 0,
// height <= 0, or mode is not one of the two modes above.
func Generate(width, height int, mode string) (*Graph, error) {
	return GenerateGrid(GridOptions{Width: width, Height: height, Mode: mode})
}

// MaxCells bounds the number of positions a generated board may have. The
// largest shipped board is 19x19 (361); the cap keeps a hostile or mistyped
// definition ("width": 1000000) from exhausting memory.
const MaxCells = 4096

// GenerateGrid is Generate with extra options (position labeling). Every
// grid it builds carries coordinates for each position and the four edge
// regions "left", "right", "top" and "bottom".
func GenerateGrid(opts GridOptions) (*Graph, error) {
	width, height := opts.Width, opts.Height
	if width <= 0 {
		return nil, fmt.Errorf("board: grid width must be positive, got %d", width)
	}
	if height <= 0 {
		return nil, fmt.Errorf("board: grid height must be positive, got %d", height)
	}

	if width > MaxCells || height > MaxCells || width*height > MaxCells {
		return nil, fmt.Errorf("board: a %dx%d grid exceeds the %d-position limit", width, height, MaxCells)
	}

	steps, ok := gridSteps(opts.Mode)
	if !ok {
		return nil, fmt.Errorf("board: unrecognized grid adjacency mode %q", opts.Mode)
	}

	labels := opts.Labels
	if labels == "" {
		labels = LabelXY
	}
	var name func(x, y int) PositionID
	switch labels {
	case LabelXY:
		name = gridPositionID
	case LabelAlgebraic:
		if width > 26 {
			return nil, fmt.Errorf("board: algebraic labels support at most 26 columns, got %d", width)
		}
		name = func(x, y int) PositionID {
			return PositionID(string(rune('a'+x)) + strconv.Itoa(height-y))
		}
	default:
		return nil, fmt.Errorf("board: unrecognized grid labeling %q", labels)
	}

	g := newGraph()
	g.kind = "grid"
	g.width, g.height = width, height

	// Reading order: top row first, left to right.
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			id := name(x, y)
			g.addPosition(id)
			g.SetCoord(id, Coord{X: x, Y: y})
		}
	}

	// Edges are added column-major (x outer, y inner), exactly as the
	// original generator did, so each position's Neighbors order is stable
	// across versions.
	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			from := name(x, y)
			for _, step := range steps {
				nx, ny := x+step.dx, y+step.dy
				if nx < 0 || nx >= width || ny < 0 || ny >= height {
					continue
				}
				g.addEdge(from, name(nx, ny))
			}
		}
	}

	var left, right, top, bottom []PositionID
	for y := 0; y < height; y++ {
		left = append(left, name(0, y))
		right = append(right, name(width-1, y))
	}
	for x := 0; x < width; x++ {
		top = append(top, name(x, 0))
		bottom = append(bottom, name(x, height-1))
	}
	g.setRegion("left", left)
	g.setRegion("right", right)
	g.setRegion("top", top)
	g.setRegion("bottom", bottom)

	return g, nil
}

// GridDirections returns the named directions available for an adjacency
// mode ("orthogonal4" or "orthogonal8").
func GridDirections(mode string) ([]Direction, bool) {
	switch mode {
	case "orthogonal4":
		return Orthogonal4Directions, true
	case "orthogonal8":
		return Orthogonal8Directions, true
	case "diagonal4":
		return Diagonal4Directions, true
	default:
		return nil, false
	}
}

// gridSteps returns the neighbor offsets for the given adjacency mode, and
// whether mode was recognized.
func gridSteps(mode string) ([]orthogonalStep, bool) {
	switch mode {
	case "orthogonal4":
		return orthogonal4Steps, true
	case "orthogonal8":
		steps := make([]orthogonalStep, 0, len(orthogonal4Steps)+len(diagonalSteps))
		steps = append(steps, orthogonal4Steps...)
		steps = append(steps, diagonalSteps...)
		return steps, true
	case "diagonal4":
		return diagonalSteps, true
	default:
		return nil, false
	}
}

// gridPositionID formats grid coordinates as the "x,y" PositionID format.
func gridPositionID(x, y int) PositionID {
	return PositionID(fmt.Sprintf("%d,%d", x, y))
}
