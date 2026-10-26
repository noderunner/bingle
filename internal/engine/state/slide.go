package state

import (
	"fmt"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/primitives"
)

// --- slide --------------------------------------------------------------

// slideDirs returns the directions a slide may use for the mover: the
// action's own restriction, or every direction the board offers.
func (c *rctx) slideDirs(dirs map[int][]board.Direction) []board.Direction {
	if d, ok := dirs[c.seat]; ok && d != nil {
		return d
	}
	return c.game.Directions
}

// walkRay walks a straight line from `from` in direction (dx, dy) over
// adjacent squares (graph edges), for at most max squares (0 = to the edge).
// It calls visit for every square reached, with whether isEmpty says the
// square is free; visit returns false to stop early. The walk ends at the
// first square that is not free (after visiting it).
func (c *rctx) walkRay(from board.PositionID, dx, dy, max int, isEmpty func(board.PositionID) bool, visit func(n int, p board.PositionID, empty bool) bool) {
	g := c.game.Board
	fc, ok := g.Coord(from)
	if !ok {
		return
	}
	prev := from
	for n := 1; max == 0 || n <= max; n++ {
		p, ok := g.At(board.Coord{X: fc.X + n*dx, Y: fc.Y + n*dy})
		if !ok || !g.HasEdge(prev, p) {
			return
		}
		empty := isEmpty(p)
		if !visit(n, p, empty) || !empty {
			return
		}
		prev = p
	}
}

func (c *rctx) validateSlide(act catalog.ResolvedAction, mv Move) (executor, error) {
	if mv.Piece != "" || mv.Count != 0 || len(mv.Path) > 0 || len(mv.Drops) > 0 {
		return nil, fmt.Errorf("state: a slide takes only from, to and (when the action has a follow-up) aim")
	}
	g := c.game.Board
	if !g.HasPosition(mv.From) {
		return nil, fmt.Errorf("state: unknown position %q", mv.From)
	}
	if !g.HasPosition(mv.To) {
		return nil, fmt.Errorf("state: unknown position %q", mv.To)
	}
	occupant, ok := c.st.Get(mv.From)
	if !ok {
		return nil, fmt.Errorf("state: no piece at %s", mv.From)
	}
	if occupant.Color != c.color {
		return nil, fmt.Errorf("state: piece at %s belongs to color %q, not seat %d's color %q", mv.From, occupant.Color, c.seat, c.color)
	}
	listed := false
	for _, p := range act.Pieces {
		if p == occupant.Piece {
			listed = true
		}
	}
	if !listed {
		return nil, fmt.Errorf("state: action %q cannot move a %s", act.ID, pieceLabel(c.game, occupant.Piece))
	}
	if act.Then == nil && mv.Aim != "" {
		return nil, fmt.Errorf("state: action %q has no follow-up, so there is no aim", act.ID)
	}
	if act.Then != nil {
		if mv.Aim == "" {
			return nil, fmt.Errorf("state: action %q needs an aim: where does the %s go?", act.ID, pieceLabel(c.game, act.Then.Piece))
		}
		if !g.HasPosition(mv.Aim) {
			return nil, fmt.Errorf("state: unknown position %q", mv.Aim)
		}
	}

	// The slide must be a straight line along an allowed direction.
	fc, _ := g.Coord(mv.From)
	tc, ok := g.Coord(mv.To)
	if !ok {
		return nil, fmt.Errorf("state: %s has no coordinates", mv.To)
	}
	dx, dy := tc.X-fc.X, tc.Y-fc.Y
	if dx == 0 && dy == 0 {
		return nil, fmt.Errorf("state: a piece must move")
	}
	adx, ady := abs(dx), abs(dy)
	if dx != 0 && dy != 0 && adx != ady {
		return nil, fmt.Errorf("state: %s to %s is not a straight line", mv.From, mv.To)
	}
	sx, sy := sign(dx), sign(dy)
	n := adx
	if ady > n {
		n = ady
	}
	allowed := false
	for _, d := range c.slideDirs(act.SlideDirs) {
		if d.DX == sx && d.DY == sy {
			allowed = true
		}
	}
	if !allowed {
		return nil, fmt.Errorf("state: a %s cannot slide from %s to %s that way", pieceLabel(c.game, occupant.Piece), mv.From, mv.To)
	}
	if act.MaxDistance > 0 && n > act.MaxDistance {
		return nil, fmt.Errorf("state: a slide covers at most %d squares", act.MaxDistance)
	}

	// Every square in between must be empty; the destination follows the
	// occupancy rule.
	prev := mv.From
	var capture *board.PositionID
	for i := 1; i <= n; i++ {
		p, ok := g.At(board.Coord{X: fc.X + i*sx, Y: fc.Y + i*sy})
		if !ok || !g.HasEdge(prev, p) {
			return nil, fmt.Errorf("state: there is no square on that line past %s", prev)
		}
		prev = p
		top, occupied := c.st.Get(p)
		if i < n {
			if occupied {
				return nil, fmt.Errorf("state: the way is blocked at %s", p)
			}
			continue
		}
		switch act.SlideOccupancy {
		case primitives.OccupancyEmptyOnly:
			if occupied {
				return nil, fmt.Errorf("state: %s is occupied", p)
			}
		case primitives.OccupancyCaptureOnly, primitives.OccupancyCaptureOrEmpty:
			if occupied {
				if top.Color == c.color {
					return nil, fmt.Errorf("state: %s holds your own piece", p)
				}
				cp := p
				capture = &cp
			} else if act.SlideOccupancy == primitives.OccupancyCaptureOnly {
				return nil, fmt.Errorf("state: a slide here must capture, but %s is empty", p)
			}
		}
	}

	if act.ForbidLanding[mv.To] {
		return nil, fmt.Errorf("state: a %s may not stop on %s", pieceLabel(c.game, occupant.Piece), mv.To)
	}

	// Custodian captures: opposing pieces flanked by the moved piece.
	var sandwiched []board.PositionID
	if len(act.CustodianLengths) > 0 {
		sandwiched = c.custodianCaptures(act, mv.To, occupant.Color)
	}

	// The follow-up is checked against the position after the slide: the
	// origin is vacant and the destination holds the moved piece.
	if act.Then != nil {
		if err := c.checkShot(act, mv); err != nil {
			return nil, err
		}
	}

	from, to, aim := mv.From, mv.To, mv.Aim
	piece := occupant
	return func(ns *BoardState) []board.PositionID {
		stack := ns.Stack(from)
		if len(stack) <= 1 {
			ns.Remove(from)
		} else {
			ns.SetStack(from, stack[:len(stack)-1])
		}
		var captured []board.PositionID
		if capture != nil {
			ns.Remove(*capture)
			captured = append(captured, *capture)
		}
		ns.Push(to, Occupant{Piece: piece.Piece, Color: piece.Color})
		c.applyPromotion(ns, to)
		for _, p := range sandwiched {
			ns.Remove(p)
		}
		if len(sandwiched) > 0 {
			captured = append(captured, sandwiched...)
			if act.CountPool != "" {
				ns.SetReserve(piece.Color, act.CountPool, ns.Reserve(piece.Color, act.CountPool)+len(sandwiched))
			}
		}
		if act.Then != nil {
			ns.Push(aim, Occupant{Piece: act.Then.Piece, Color: piece.Color})
		}
		return captured
	}, nil
}

// checkShot verifies the follow-up of a slide: the aim must be reachable by
// a straight, unobstructed line from the destination, in the position after
// the slide.
func (c *rctx) checkShot(act catalog.ResolvedAction, mv Move) error {
	g := c.game.Board
	tc, _ := g.Coord(mv.To)
	ac, ok := g.Coord(mv.Aim)
	if !ok {
		return fmt.Errorf("state: %s has no coordinates", mv.Aim)
	}
	dx, dy := ac.X-tc.X, ac.Y-tc.Y
	if dx == 0 && dy == 0 {
		return fmt.Errorf("state: the shot cannot land where the piece stands")
	}
	adx, ady := abs(dx), abs(dy)
	if dx != 0 && dy != 0 && adx != ady {
		return fmt.Errorf("state: %s is not in a straight line from %s", mv.Aim, mv.To)
	}
	sx, sy := sign(dx), sign(dy)
	n := adx
	if ady > n {
		n = ady
	}
	allowed := false
	for _, d := range c.slideDirs(act.Then.Dirs) {
		if d.DX == sx && d.DY == sy {
			allowed = true
		}
	}
	if !allowed {
		return fmt.Errorf("state: cannot shoot from %s towards %s", mv.To, mv.Aim)
	}
	if act.Then.MaxDistance > 0 && n > act.Then.MaxDistance {
		return fmt.Errorf("state: a shot travels at most %d squares", act.Then.MaxDistance)
	}
	vacant := func(p board.PositionID) bool {
		if p == mv.From {
			return true
		}
		if p == mv.To {
			return false
		}
		_, occ := c.st.Get(p)
		return !occ
	}
	prev := mv.To
	for i := 1; i <= n; i++ {
		p, ok := g.At(board.Coord{X: tc.X + i*sx, Y: tc.Y + i*sy})
		if !ok || !g.HasEdge(prev, p) {
			return fmt.Errorf("state: there is no square on that line past %s", prev)
		}
		prev = p
		if !vacant(p) {
			if i == n {
				return fmt.Errorf("state: %s is occupied", p)
			}
			return fmt.Errorf("state: the shot is blocked at %s", p)
		}
	}
	return nil
}

// enumerateSlides lists every legal slide (with every legal follow-up).
func (c *rctx) enumerateSlides(act catalog.ResolvedAction, keep func(catalog.ResolvedAction, Move, bool)) {
	g := c.game.Board
	isPiece := map[string]bool{}
	for _, p := range act.Pieces {
		isPiece[p] = true
	}
	empty := func(p board.PositionID) bool { return len(c.st.cells[p]) == 0 }
	dirs := c.slideDirs(act.SlideDirs)

	for _, from := range g.Positions() {
		if c.enough() {
			return
		}
		top, ok := c.st.Get(from)
		if !ok || top.Color != c.color || !isPiece[top.Piece] {
			continue
		}
		for _, d := range dirs {
			if c.enough() {
				return
			}
			c.walkRay(from, d.DX, d.DY, act.MaxDistance, empty, func(_ int, to board.PositionID, isEmpty bool) bool {
				canLand := false
				if act.ForbidLanding[to] {
					return true // may be passed over, never stopped on
				}
				if isEmpty {
					canLand = act.SlideOccupancy != primitives.OccupancyCaptureOnly
				} else if act.SlideOccupancy != primitives.OccupancyEmptyOnly {
					if t, _ := c.st.Get(to); t.Color != c.color {
						canLand = true
					}
				}
				if !canLand {
					return true
				}
				if act.Then == nil {
					keep(act, Move{Action: act.ID, From: from, To: to}, true)
					return true
				}
				// Squares the piece could shoot at from `to`, in the
				// position after it moved.
				vacant := func(p board.PositionID) bool {
					if p == from {
						return true
					}
					if p == to {
						return false
					}
					return empty(p)
				}
				for _, sd := range c.slideDirs(act.Then.Dirs) {
					c.walkRay(to, sd.DX, sd.DY, act.Then.MaxDistance, vacant, func(_ int, aim board.PositionID, free bool) bool {
						if free {
							keep(act, Move{Action: act.ID, From: from, To: to, Aim: aim}, true)
						}
						return !c.enough()
					})
					if c.enough() {
						return false
					}
				}
				return !c.enough()
			})
		}
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func sign(v int) int {
	switch {
	case v > 0:
		return 1
	case v < 0:
		return -1
	}
	return 0
}
