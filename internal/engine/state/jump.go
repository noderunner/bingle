package state

import (
	"fmt"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
)

// The "jump" action implements capture-by-leaping as in draughts: a piece
// hops over an adjacent enemy piece onto the empty square directly beyond,
// capturing it. With chain enabled the same piece keeps hopping in one move
// (a chain must be played to its end, and a promotion ends it), and with
// mandatory enabled a jump must be taken whenever one is available.
//
// A move is {from, path: [landing squares...]}; each hop's captured square
// is the midpoint between consecutive squares.

// jumpDirs returns the directions rp may jump in for the mover.
func (c *rctx) jumpDirs(rp catalog.ResolvedPiece) []board.Direction {
	if dirs, ok := rp.Dirs[c.seat]; ok {
		return dirs
	}
	return c.game.Directions
}

// hop describes one possible single hop from a square.
type hop struct {
	over board.PositionID
	land board.PositionID
}

// hopsFrom lists the single hops available from cur for a piece of rp,
// given squares already captured earlier in the chain and the square the
// piece started on (which it has vacated).
func (c *rctx) hopsFrom(rp catalog.ResolvedPiece, cur, origin board.PositionID, captured map[board.PositionID]bool) []hop {
	g := c.game.Board
	cc, ok := g.Coord(cur)
	if !ok {
		return nil
	}
	var out []hop
	for _, d := range c.jumpDirs(rp) {
		overID, ok1 := g.At(board.Coord{X: cc.X + d.DX, Y: cc.Y + d.DY})
		landID, ok2 := g.At(board.Coord{X: cc.X + 2*d.DX, Y: cc.Y + 2*d.DY})
		if !ok1 || !ok2 {
			continue
		}
		if captured[overID] {
			continue // a piece may not be jumped twice
		}
		top, occupied := c.st.Get(overID)
		if !occupied || top.Color == c.color {
			continue
		}
		if _, occ := c.st.Get(landID); occ && landID != origin {
			continue
		}
		// Squares captured earlier still hold their piece until the move
		// ends, so they block landing too (handled by the occupied check).
		out = append(out, hop{over: overID, land: landID})
	}
	return out
}

// validateJump checks a jump move and returns its executor.
func (c *rctx) validateJump(act catalog.ResolvedAction, mv Move) (executor, error) {
	g := c.game.Board
	if mv.Piece != "" || mv.Count != 0 || len(mv.Drops) > 0 {
		return nil, fmt.Errorf("state: a jump takes only from and a path of landing squares")
	}
	if !g.HasPosition(mv.From) {
		return nil, fmt.Errorf("state: unknown position %q", mv.From)
	}
	top, ok := c.st.Get(mv.From)
	if !ok {
		return nil, fmt.Errorf("state: no piece at %s", mv.From)
	}
	if top.Color != c.color {
		return nil, fmt.Errorf("state: piece at %s belongs to color %q, not seat %d's color %q", mv.From, top.Color, c.seat, c.color)
	}
	rp, ok := c.game.Pieces[top.Piece]
	if !ok {
		return nil, fmt.Errorf("state: unknown piece type %q at %s", top.Piece, mv.From)
	}
	if len(mv.Path) == 0 {
		return nil, fmt.Errorf("state: a jump needs at least one landing square")
	}
	if len(mv.Path) > 1 && !act.Chain {
		return nil, fmt.Errorf("state: this game does not allow chained jumps")
	}

	captured := map[board.PositionID]bool{}
	var captures []board.PositionID
	cur := mv.From
	for i, land := range mv.Path {
		var found *hop
		for _, h := range c.hopsFrom(rp, cur, mv.From, captured) {
			if h.land == land {
				hh := h
				found = &hh
				break
			}
		}
		if found == nil {
			return nil, fmt.Errorf("state: %s is not a legal jump from %s (a jump hops over an adjacent enemy piece onto the empty square beyond it)", land, cur)
		}
		captured[found.over] = true
		captures = append(captures, found.over)
		cur = land

		last := i == len(mv.Path)-1
		if c.promotesAt(rp, land) && !last {
			return nil, fmt.Errorf("state: a jump ends when the piece is promoted at %s", land)
		}
	}

	// A chain must be played to its end unless the piece was promoted.
	if act.Chain && !c.promotesAt(rp, cur) {
		if len(c.hopsFrom(rp, cur, mv.From, captured)) > 0 {
			return nil, fmt.Errorf("state: the jump must continue: another capture is available from %s", cur)
		}
	}

	from, dest := mv.From, cur
	caps := captures
	return func(ns *BoardState) []board.PositionID {
		stack := ns.Stack(from)
		piece := stack[len(stack)-1]
		if len(stack) == 1 {
			ns.Remove(from)
		} else {
			ns.SetStack(from, stack[:len(stack)-1])
		}
		for _, p := range caps {
			ns.Remove(p)
		}
		ns.Push(dest, piece)
		c.applyPromotion(ns, dest)
		return append([]board.PositionID(nil), caps...)
	}, nil
}

// enumerateJumps generates every legal jump (and chain) for the mover.
func (c *rctx) enumerateJumps(act catalog.ResolvedAction, keep func(catalog.ResolvedAction, Move, bool)) {
	for _, from := range c.game.Board.Positions() {
		if c.enough() {
			return
		}
		top, ok := c.st.Get(from)
		if !ok || top.Color != c.color {
			continue
		}
		rp, ok := c.game.Pieces[top.Piece]
		if !ok {
			continue
		}
		c.jumpChains(act, rp, from, from, nil, map[board.PositionID]bool{}, keep)
	}
}

// jumpChains depth-first expands hops from cur, emitting maximal chains.
func (c *rctx) jumpChains(act catalog.ResolvedAction, rp catalog.ResolvedPiece, origin, cur board.PositionID,
	path []board.PositionID, captured map[board.PositionID]bool, keep func(catalog.ResolvedAction, Move, bool)) {

	hops := c.hopsFrom(rp, cur, origin, captured)
	if len(hops) == 0 {
		if len(path) > 0 {
			keep(act, Move{Action: act.ID, From: origin, To: path[len(path)-1], Path: append([]board.PositionID(nil), path...)}, false)
		}
		return
	}
	for _, h := range hops {
		next := append(append([]board.PositionID(nil), path...), h.land)
		if !act.Chain || c.promotesAt(rp, h.land) {
			keep(act, Move{Action: act.ID, From: origin, To: h.land, Path: next}, false)
			continue
		}
		nextCaptured := make(map[board.PositionID]bool, len(captured)+1)
		for k := range captured {
			nextCaptured[k] = true
		}
		nextCaptured[h.over] = true
		c.jumpChains(act, rp, origin, h.land, next, nextCaptured, keep)
	}
}

// mustJump reports whether a mandatory jump is pending for the mover, which
// makes every non-jump action illegal.
func (c *rctx) mustJump() bool {
	if c.mustJumpKnown {
		return c.mustJumpVal
	}
	c.mustJumpKnown = true
	for _, act := range c.game.Actions {
		if act.Type != catalog.ActionJump || !act.Mandatory {
			continue
		}
		found := false
		c.enumerateJumps(act, func(catalog.ResolvedAction, Move, bool) { found = true })
		if found {
			c.mustJumpVal = true
			return true
		}
	}
	return false
}
