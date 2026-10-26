// Package puzzles holds a curated set of "win in N" positions and the solver
// that proves them. A puzzle is a start position plus the seat to move; the
// goal is to force a win within a number of plies against any defence. They
// double as a benchmark: an agent that cannot find a forced win in two moves
// is not playing the game yet.
package puzzles

import (
	"context"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

// Solver proves forced wins by exhaustive search (every defence is tried, so
// it is exact but exponential in plies; fine for the small depths puzzles
// use).
type Solver struct {
	game *catalog.CompiledGame
	// Nodes counts positions examined (for tests and generation budgets).
	Nodes int
}

// NewSolver returns a solver for game.
func NewSolver(game *catalog.CompiledGame) *Solver { return &Solver{game: game} }

// WinningMoves returns every move of seat in st that wins by force within
// plies plies (counting seat's own move as the first ply; so 1 = a move that
// wins at once, 3 = wins in two of its own moves whatever the opponent
// does). It stops early when ctx is done and returns what it found.
func (s *Solver) WinningMoves(ctx context.Context, st *state.BoardState, seat, plies int) []state.Move {
	var out []state.Move
	for _, mv := range state.LegalMoves(s.game, st, seat) {
		if ctx.Err() != nil {
			return out
		}
		res, err := state.Apply(s.game, st, seat, mv)
		s.Nodes++
		if err != nil {
			continue
		}
		if s.winsAfter(ctx, res, seat, plies-1) {
			out = append(out, mv)
		}
	}
	return out
}

// winsAfter reports whether target wins by force within plies more plies
// after a move produced res.
func (s *Solver) winsAfter(ctx context.Context, res *state.MoveResult, target, plies int) bool {
	if res.GameEnded {
		return res.WinnerSeat != nil && *res.WinnerSeat == target
	}
	if plies <= 0 || ctx.Err() != nil {
		return false
	}
	return s.solve(ctx, res.NewState, res.NextTurnSeat, target, plies)
}

// solve: with toMove to play in st, does target force a win within plies?
func (s *Solver) solve(ctx context.Context, st *state.BoardState, toMove, target, plies int) bool {
	legal := state.LegalMoves(s.game, st, toMove)
	if len(legal) == 0 {
		return false // stalemate: not a win for anyone we can prove
	}
	if toMove == target {
		for _, mv := range legal {
			res, err := state.Apply(s.game, st, toMove, mv)
			s.Nodes++
			if err != nil {
				continue
			}
			if s.winsAfter(ctx, res, target, plies-1) {
				return true
			}
		}
		return false
	}
	// The opponent picks the defence that hurts most: every reply must lose.
	for _, mv := range legal {
		res, err := state.Apply(s.game, st, toMove, mv)
		s.Nodes++
		if err != nil {
			continue
		}
		if !s.winsAfter(ctx, res, target, plies-1) {
			return false
		}
	}
	return true
}

// Line returns a principal variation: starting with mv, the attacker's moves
// and the defender's most resistant replies until the game ends, as
// notation. It assumes mv is a winning first move within plies.
func (s *Solver) Line(ctx context.Context, st *state.BoardState, seat int, mv state.Move, plies int) []string {
	var out []string
	cur, toMove, move := st, seat, mv
	for plies > 0 {
		res, err := state.Apply(s.game, cur, toMove, move)
		if err != nil {
			return out
		}
		out = append(out, res.Notation)
		plies--
		if res.GameEnded {
			return out
		}
		cur, toMove = res.NewState, res.NextTurnSeat
		legal := state.LegalMoves(s.game, cur, toMove)
		if len(legal) == 0 {
			return out
		}
		if toMove == seat {
			// Attacker: pick a move that still wins.
			found := false
			for _, m := range legal {
				r, err := state.Apply(s.game, cur, toMove, m)
				if err != nil {
					continue
				}
				if s.winsAfter(ctx, r, seat, plies-1) {
					move, found = m, true
					break
				}
			}
			if !found {
				return out
			}
		} else {
			// Defender: the reply that survives longest (the one needing
			// the most plies to lose; here: any reply that does not lose
			// immediately, else the first).
			move = legal[0]
			for _, m := range legal {
				r, err := state.Apply(s.game, cur, toMove, m)
				if err != nil {
					continue
				}
				if !r.GameEnded {
					move = m
					break
				}
			}
		}
	}
	return out
}

// Proves reports whether playing mv now wins by force for seat within plies
// plies (counting mv as the first).
func (s *Solver) Proves(ctx context.Context, st *state.BoardState, seat int, mv state.Move, plies int) bool {
	res, err := state.Apply(s.game, st, seat, mv)
	s.Nodes++
	if err != nil {
		return false
	}
	return s.winsAfter(ctx, res, seat, plies-1)
}
