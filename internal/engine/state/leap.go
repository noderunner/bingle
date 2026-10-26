package state

import (
	"fmt"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
)

// --- leap ---------------------------------------------------------------

// leapReach returns the empty squares within max board steps of from (steps
// follow the board's adjacency, ignoring what stands in between, as when a
// piece hops over its neighbours) with their distances.
func (c *rctx) leapReach(from board.PositionID, max int) map[board.PositionID]int {
	g := c.game.Board
	dist := map[board.PositionID]int{from: 0}
	frontier := []board.PositionID{from}
	for d := 1; d <= max; d++ {
		var next []board.PositionID
		for _, p := range frontier {
			for _, n := range g.Neighbors(p) {
				if _, seen := dist[n]; seen {
					continue
				}
				dist[n] = d
				next = append(next, n)
			}
		}
		frontier = next
	}
	delete(dist, from)
	return dist
}

// validateLeap implements the "leap" action (Ataxx): a piece lands on an
// empty square within max_distance; within clone_distance it stays behind (a
// clone), further it leaves its origin (a jump); then adjacent opposing
// pieces may be infected.
func (c *rctx) validateLeap(act catalog.ResolvedAction, mv Move) (executor, error) {
	if mv.Piece != "" || mv.Count != 0 || len(mv.Path) > 0 || len(mv.Drops) > 0 || mv.Aim != "" {
		return nil, fmt.Errorf("state: a leap takes only from and to")
	}
	g := c.game.Board
	if !g.HasPosition(mv.From) || !g.HasPosition(mv.To) {
		return nil, fmt.Errorf("state: unknown position %q or %q", mv.From, mv.To)
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
	if _, occ := c.st.Get(mv.To); occ {
		return nil, fmt.Errorf("state: %s is occupied", mv.To)
	}
	d, reachable := c.leapReach(mv.From, act.MaxDistance)[mv.To]
	if !reachable {
		return nil, fmt.Errorf("state: %s is more than %d steps from %s", mv.To, act.MaxDistance, mv.From)
	}
	stays := d <= act.CloneDistance

	from, to := mv.From, mv.To
	return func(ns *BoardState) []board.PositionID {
		if !stays {
			stack := ns.Stack(from)
			if len(stack) <= 1 {
				ns.Remove(from)
			} else {
				ns.SetStack(from, stack[:len(stack)-1])
			}
		}
		ns.Push(to, Occupant{Piece: occupant.Piece, Color: occupant.Color})
		c.applyPromotion(ns, to)
		if act.Infect {
			for _, n := range g.Neighbors(to) {
				top, ok := ns.Get(n)
				if !ok || top.Color == occupant.Color {
					continue
				}
				stack := ns.Stack(n)
				stack[len(stack)-1] = Occupant{Piece: top.Piece, Color: occupant.Color}
				ns.SetStack(n, stack)
			}
		}
		return nil
	}, nil
}

func (c *rctx) enumerateLeaps(act catalog.ResolvedAction, keep func(catalog.ResolvedAction, Move, bool)) {
	isPiece := map[string]bool{}
	for _, p := range act.Pieces {
		isPiece[p] = true
	}
	for _, from := range c.game.Board.Positions() {
		if c.enough() {
			return
		}
		top, ok := c.st.Get(from)
		if !ok || top.Color != c.color || !isPiece[top.Piece] {
			continue
		}
		reach := c.leapReach(from, act.MaxDistance)
		// Board reading order keeps enumeration deterministic.
		for _, to := range c.game.Board.Positions() {
			if _, in := reach[to]; !in || len(c.st.cells[to]) > 0 {
				continue
			}
			keep(act, Move{Action: act.ID, From: from, To: to}, true)
		}
	}
}
