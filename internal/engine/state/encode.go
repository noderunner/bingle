package state

import (
	"fmt"
	"sort"
	"sync"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
)

// Planes is a fixed-shape numeric encoding of a position, for feeding a
// model. Data is row-major [channel][y][x] and Channels names each channel.
//
// The encoding is relative to a viewer seat ("me" is the viewer, "opp1",
// "opp2" … the other seats in turn order after the viewer), so one network
// can play either side. Board channels hold 1 where the piece is at that
// depth of a stack (top, 2nd, …); scalar channels (reserves, side to move,
// ply) are constant over the whole plane.
type Planes struct {
	Shape    [3]int    `json:"shape"` // channels, height, width
	Channels []string  `json:"channels"`
	Data     []float32 `json:"data"`
}

// PlaneOptions tunes EncodePlanes.
type PlaneOptions struct {
	// Depth is how many stack levels (from the top) get their own piece
	// channels; 0 chooses 1 for single-occupant games and 4 when the game
	// has stack moves.
	Depth int
}

// planeLayout describes the channels for a game, independent of any
// position, so env_spec can publish it.
type planeLayout struct {
	depth  int
	width  int
	height int
	seats  []int
	colors []string
	pieces []string
	pools  []string
}

// channelNames lists the channels in order: piece one-hots per depth level,
// the stack height, reserve scalars, then side-to-move and ply.
func (l *planeLayout) channelNames() []string {
	var names []string
	rels := relativeNames(len(l.seats))
	for d := 0; d < l.depth; d++ {
		level := "top"
		if d > 0 {
			level = fmt.Sprintf("depth%d", d+1)
		}
		for _, rel := range rels {
			for _, piece := range l.pieces {
				names = append(names, level+"/"+rel+"/"+piece)
			}
		}
	}
	names = append(names, "height")
	for _, rel := range rels {
		for _, pool := range l.pools {
			names = append(names, "reserve/"+rel+"/"+pool)
		}
	}
	return append(names, "to-move", "ply")
}

// PlaneChannels returns the channel names EncodePlanes uses for game as seen
// by seat 1 (the names are viewer-relative, so they do not depend on the
// seat), plus the plane height and width.
func PlaneChannels(game *catalog.CompiledGame, opts PlaneOptions) (names []string, height, width int, err error) {
	if rs, ok := RulesetOf(game); ok {
		pe, ok := rs.(PlaneEncoder)
		if !ok {
			return nil, 0, 0, fmt.Errorf("state: game %q has no plane encoding", game.Definition.ID)
		}
		names, height, width = pe.PlaneLayout(game)
		return names, height, width, nil
	}
	lay, err := newPlaneLayout(game, opts)
	if err != nil {
		return nil, 0, 0, err
	}
	return lay.channelNames(), lay.height, lay.width, nil
}

// PlaneCells lists the board position at every plane cell, row-major (y*W+x);
// "" marks a cell that is not part of the board (hex boards are fully
// populated; this exists for completeness and for irregular boards).
func PlaneCells(game *catalog.CompiledGame) ([]string, error) {
	if rs, ok := RulesetOf(game); ok {
		pe, ok := rs.(PlaneEncoder)
		if !ok {
			return nil, fmt.Errorf("state: game %q has no plane encoding", game.Definition.ID)
		}
		return pe.PlaneCells(game), nil
	}
	lay, err := newPlaneLayout(game, PlaneOptions{})
	if err != nil {
		return nil, err
	}
	out := make([]string, lay.width*lay.height)
	for _, p := range game.Board.Positions() {
		if c, ok := game.Board.Coord(p); ok && c.X >= 0 && c.X < lay.width && c.Y >= 0 && c.Y < lay.height {
			out[c.Y*lay.width+c.X] = string(p)
		}
	}
	return out, nil
}

func newPlaneLayout(game *catalog.CompiledGame, opts PlaneOptions) (*planeLayout, error) {
	g := game.Board
	if g.Width() <= 0 || g.Height() <= 0 {
		return nil, fmt.Errorf("state: planes need a grid or hex board; %q has an explicit board", game.Definition.ID)
	}
	depth := opts.Depth
	if depth <= 0 {
		depth = 1
		for _, a := range game.Actions {
			if a.Type == catalog.ActionMoveStack {
				depth = 4
			}
		}
	}
	if depth > 8 {
		depth = 8
	}
	lay := &planeLayout{depth: depth, width: g.Width(), height: g.Height(), pieces: game.PieceTypes}
	for _, s := range game.Definition.Seats {
		lay.seats = append(lay.seats, s.Seat)
		lay.colors = append(lay.colors, s.Color)
	}
	// Reserve pools in a stable order.
	pools := map[string]bool{}
	for p := range game.Definition.Reserves {
		pools[p] = true
	}
	for p := range pools {
		lay.pools = append(lay.pools, p)
	}
	sort.Strings(lay.pools)
	return lay, nil
}

// relativeNames names seats relative to the viewer: "me", then "opp1"… (a
// two-seat game has just "me" and "opp").
func relativeNames(n int) []string {
	if n == 2 {
		return []string{"me", "opp"}
	}
	out := []string{"me"}
	for i := 1; i < n; i++ {
		out = append(out, fmt.Sprintf("opp%d", i))
	}
	return out
}

// EncodePlanes encodes st from viewerSeat's point of view. toMoveSeat is the
// seat whose turn it is (feeds the to-move scalar).
func EncodePlanes(game *catalog.CompiledGame, st *BoardState, viewerSeat, toMoveSeat int, opts PlaneOptions) (*Planes, error) {
	if rs, ok := RulesetOf(game); ok {
		pe, ok := rs.(PlaneEncoder)
		if !ok {
			return nil, fmt.Errorf("state: game %q has no plane encoding", game.Definition.ID)
		}
		if _, ok := game.Seat(viewerSeat); !ok {
			return nil, fmt.Errorf("state: game %q has no seat %d", game.Definition.ID, viewerSeat)
		}
		names, h, w := pe.PlaneLayout(game)
		return &Planes{Shape: [3]int{len(names), h, w}, Channels: names, Data: pe.EncodePlanes(game, st, viewerSeat)}, nil
	}
	lay, err := newPlaneLayout(game, opts)
	if err != nil {
		return nil, err
	}
	// Viewer-relative colour order.
	idx := -1
	for i, s := range lay.seats {
		if s == viewerSeat {
			idx = i
		}
	}
	if idx < 0 {
		return nil, fmt.Errorf("state: game %q has no seat %d", game.Definition.ID, viewerSeat)
	}
	rel := map[string]int{} // colour → relative index
	relColors := make([]string, len(lay.colors))
	for k := range lay.colors {
		c := lay.colors[(idx+k)%len(lay.colors)]
		rel[c] = k
		relColors[k] = c
	}
	pieceIdx := map[string]int{}
	for i, p := range lay.pieces {
		pieceIdx[p] = i
	}

	names := lay.channelNames()
	w, h := lay.width, lay.height
	cells := w * h
	data := make([]float32, len(names)*cells)
	chPerLevel := len(lay.colors) * len(lay.pieces)
	heightCh := lay.depth * chPerLevel

	for _, pos := range st.Occupied() {
		c, ok := game.Board.Coord(pos)
		if !ok || c.X < 0 || c.X >= w || c.Y < 0 || c.Y >= h {
			continue
		}
		stack := st.cells[pos]
		cell := c.Y*w + c.X
		for d := 0; d < lay.depth && d < len(stack); d++ {
			occ := stack[len(stack)-1-d]
			r, okc := rel[occ.Color]
			pi, okp := pieceIdx[occ.Piece]
			if !okc || !okp {
				continue
			}
			ch := d*chPerLevel + r*len(lay.pieces) + pi
			data[ch*cells+cell] = 1
		}
		data[heightCh*cells+cell] = float32(len(stack))
	}

	// Scalars.
	ch := heightCh + 1
	for _, color := range relColors {
		for _, pool := range lay.pools {
			initial := float32(game.Definition.Reserves[pool])
			v := float32(st.Reserve(color, pool))
			if initial > 0 {
				v /= initial
			}
			fill(data, ch, cells, v)
			ch++
		}
	}
	if toMoveSeat == viewerSeat {
		fill(data, ch, cells, 1)
	}
	ch++
	fill(data, ch, cells, float32(st.Ply())/100)

	return &Planes{Shape: [3]int{len(names), h, w}, Channels: names, Data: data}, nil
}

func fill(data []float32, ch, cells int, v float32) {
	if v == 0 {
		return
	}
	for i := 0; i < cells; i++ {
		data[ch*cells+i] = v
	}
}

// --- fixed action space ---------------------------------------------------

// ActionSpace is every move a game can express on its board, as canonical
// notation, in a stable order. A move's index is its fixed action ID, so a
// policy head can have one logit per index and mask with the legal moves.
type ActionSpace struct {
	// Moves lists the notation of every action; empty when !Fixed.
	Moves []string
	// Fixed is false for games whose moves cannot be enumerated ahead of
	// time (chained jumps have unbounded paths); use legal-move lists then.
	Fixed bool

	index map[string]int
}

// ID returns the fixed action ID of a move's notation.
func (a *ActionSpace) ID(notation string) (int, bool) {
	id, ok := a.index[notation]
	return id, ok
}

var actionSpaces sync.Map // *catalog.CompiledGame → *ActionSpace

// ActionSpaceOf returns (and caches) the fixed action space of game.
func ActionSpaceOf(game *catalog.CompiledGame) *ActionSpace {
	if v, ok := actionSpaces.Load(game); ok {
		return v.(*ActionSpace)
	}
	as := buildActionSpace(game)
	v, _ := actionSpaces.LoadOrStore(game, as)
	return v.(*ActionSpace)
}

// maxActionSpace guards against a definition whose space is impractically
// large; such games fall back to dynamic legal-move lists.
const maxActionSpace = 200_000

func buildActionSpace(game *catalog.CompiledGame) *ActionSpace {
	as := &ActionSpace{index: map[string]int{}}
	if rs, ok := RulesetOf(game); ok {
		if al, ok := rs.(ActionLister); ok {
			as.Moves = al.ActionNotations(game)
			as.Fixed = true
			for i, m := range as.Moves {
				as.index[m] = i
			}
		}
		return as
	}
	g := game.Board
	var moves []string
	seen := map[string]bool{}
	add := func(mv Move) {
		s := FormatMove(game, mv)
		if !seen[s] {
			seen[s] = true
			moves = append(moves, s)
		}
	}

	for _, act := range game.Actions {
		switch act.Type {
		case catalog.ActionPlace:
			for _, piece := range act.Pieces {
				for _, to := range g.Positions() {
					if act.Region != nil && !act.Region[to] {
						continue
					}
					add(Move{Action: act.ID, Piece: piece, To: to})
				}
			}
		case catalog.ActionStep:
			for _, from := range g.Positions() {
				for _, to := range g.Neighbors(from) {
					add(Move{Action: act.ID, From: from, To: to})
				}
			}
		case catalog.ActionLeap:
			c := &rctx{game: game}
			for _, from := range g.Positions() {
				reach := c.leapReach(from, act.MaxDistance)
				for _, to := range g.Positions() {
					if _, ok := reach[to]; ok {
						add(Move{Action: act.ID, From: from, To: to})
					}
				}
			}
		case catalog.ActionPass, catalog.ActionSwap:
			add(Move{Action: act.ID})
		case catalog.ActionJump:
			if act.Chain {
				return as // dynamic
			}
			for _, from := range g.Positions() {
				fc, ok := g.Coord(from)
				if !ok {
					return as
				}
				for _, d := range game.Directions {
					if to, ok := g.At(board.Coord{X: fc.X + 2*d.DX, Y: fc.Y + 2*d.DY}); ok {
						add(Move{Action: act.ID, From: from, Path: []board.PositionID{to}, To: to})
					}
				}
			}
		case catalog.ActionSlide:
			dirs := slideDirUnion(game, act.SlideDirs)
			thenDirs := dirs
			if act.Then != nil {
				thenDirs = slideDirUnion(game, act.Then.Dirs)
			}
			ray := func(from board.PositionID, d board.Direction, max int) []board.PositionID {
				fc, ok := g.Coord(from)
				if !ok {
					return nil
				}
				var out []board.PositionID
				prev := from
				for n := 1; max == 0 || n <= max; n++ {
					p, ok := g.At(board.Coord{X: fc.X + n*d.DX, Y: fc.Y + n*d.DY})
					if !ok || !g.HasEdge(prev, p) {
						break
					}
					out = append(out, p)
					prev = p
				}
				return out
			}
			for _, from := range g.Positions() {
				for _, d := range dirs {
					for _, to := range ray(from, d, act.MaxDistance) {
						if act.Then == nil {
							add(Move{Action: act.ID, From: from, To: to})
							continue
						}
						max := act.Then.MaxDistance
						for _, sd := range thenDirs {
							for _, aim := range ray(to, sd, max) {
								add(Move{Action: act.ID, From: from, To: to, Aim: aim})
							}
						}
					}
				}
				if len(moves) > maxActionSpace {
					return as
				}
			}
		case catalog.ActionMoveStack:
			for _, from := range g.Positions() {
				fc, ok := g.Coord(from)
				if !ok {
					return as
				}
				for _, d := range game.Directions {
					var line []board.PositionID
					for i := 1; ; i++ {
						p, ok := g.At(board.Coord{X: fc.X + i*d.DX, Y: fc.Y + i*d.DY})
						if !ok || (i == 1 && !g.HasEdge(from, p)) {
							break
						}
						line = append(line, p)
					}
					for count := 1; count <= act.CarryLimit; count++ {
						for squares := 1; squares <= count && squares <= len(line); squares++ {
							forEachComposition(count, squares, func(drops []int) {
								add(Move{
									Action: act.ID, From: from, Count: count,
									To: line[squares-1], Path: line[:squares:squares], Drops: cloneInts(drops),
								})
							})
						}
					}
				}
				if len(moves) > maxActionSpace {
					return as
				}
			}
		}
		if len(moves) > maxActionSpace {
			return as
		}
	}
	as.Moves = moves
	for i, s := range moves {
		as.index[s] = i
	}
	as.Fixed = true
	return as
}

// slideDirUnion is the set of directions any seat may slide in.
func slideDirUnion(game *catalog.CompiledGame, per map[int][]board.Direction) []board.Direction {
	if per == nil {
		return game.Directions
	}
	seen := map[[2]int]bool{}
	var out []board.Direction
	for _, ds := range per {
		for _, d := range ds {
			k := [2]int{d.DX, d.DY}
			if !seen[k] {
				seen[k] = true
				out = append(out, d)
			}
		}
	}
	return out
}
