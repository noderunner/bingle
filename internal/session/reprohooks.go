package session

import (
	"context"
	"fmt"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/repro"
	"github.com/noderunner/bingle/internal/store"
)

// The two things the move funnel can notice on its own, as opposed to the four
// paranoid mode goes looking for.

// applyWatched is state.Apply with a panic turned into an error and a bundle.
//
// A ruleset is a pure function, and a pure function that panics takes the
// request's goroutine with it: the HTTP recover middleware turns that into a
// 500 and a stack, which says where it broke but not what it broke on. The
// position is what is needed, so it is caught here, where the position is
// still in hand.
func (m *Manager) applyWatched(compiled *catalog.CompiledGame, bs *state.BoardState, seat int, mv state.Move) (result *state.MoveResult, err error) {
	defer func() {
		v := recover()
		if v == nil {
			return
		}
		err = fmt.Errorf("%w: %v", ErrRulesetPanic, v)
		result = nil
		if m.repro.On() {
			b := repro.Of(repro.RulesetPanicked, compiled, compiled.Definition.ID, bs, seat, &mv, err)
			b.Note = fmt.Sprintf("the ruleset panicked in Apply: %v", v)
			m.repro.Write(b)
		}
	}()
	return state.Apply(compiled, bs, seat, mv)
}

// reproRefusal writes a bundle when Apply has refused a move that Legal lists.
//
// Legal and Apply disagreeing is the fault that reaches a player as a game
// that cannot be continued: the page offers a move, the server refuses it, and
// nothing anybody can see explains why. Paranoid mode catches the same
// disagreement from the other side; this catches it in production, on a path
// that is already an error, and only when there is somewhere to write.
func (m *Manager) reproRefusal(ctx context.Context, compiled *catalog.CompiledGame, game store.Game, bs *state.BoardState, seat int, mv state.Move, cause error) {
	if !m.repro.On() {
		return
	}
	want := mv.Key()
	listed := false
	for _, candidate := range state.LegalMoves(compiled, bs, seat) {
		if candidate.Key() == want {
			listed = true
			break
		}
	}
	if !listed {
		// The ordinary case: the client offered something the rules do not
		// allow, and the rules said so. Nothing impossible about it.
		return
	}
	b := repro.Of(repro.ApplyRejectedALegalMove, compiled, game.GameDefID, bs, seat, &mv, cause)
	b.Note = "Legal listed this move and Apply refused it"
	// The move was refused, so it was never recorded: the whole history is
	// the path to this position.
	m.addHistory(ctx, &b, game, 0)
	m.repro.Write(b)
}
