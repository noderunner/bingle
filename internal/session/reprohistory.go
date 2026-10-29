package session

import (
	"context"

	"github.com/noderunner/bingle/internal/repro"
	"github.com/noderunner/bingle/internal/store"
)

// addHistory fills in how the position in a bundle was reached: the game's
// opening document and the notations played since.
//
// The bundle already carries the position itself, which is what makes it
// reproduce. This is the other half of the question — a ruleset usually goes
// wrong because of how a position was arrived at, not because of the position
// alone — and `repro.Replay` reports a divergence between replaying these and
// the recorded position as a finding of its own.
//
// beforeSeq is the sequence number of the move the bundle is *about*, which
// must not be part of the path: the bundle's position is the one before that
// move, so a path including it replays one move too far. Zero or less means
// the move was never recorded — Apply refused it — and the whole history is
// the path.
//
// It reads the database, so it is only ever called on a path where something
// has already gone wrong. A failure to read is not worth reporting: the
// bundle is still a reproduction without it.
func (m *Manager) addHistory(ctx context.Context, b *repro.Bundle, game store.Game, beforeSeq int) {
	if game.InitialState != "" {
		b.StartState = []byte(game.InitialState)
	}
	moves, err := m.store.ListMoves(ctx, game.ID, 0)
	if err != nil {
		return
	}
	for _, mv := range moves {
		if beforeSeq > 0 && mv.Seq >= beforeSeq {
			break
		}
		if mv.Notation == "" {
			// A game recorded before notation was stored, or a move with
			// none: the path cannot be replayed, so do not half-record it.
			b.Moves = nil
			return
		}
		b.Moves = append(b.Moves, mv.Notation)
	}
}
