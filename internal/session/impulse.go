package session

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/trace"
)

// ErrStalePly is returned for a query or a move made against a ply the game has
// moved on from (ADR 0114): a move may take minutes to build, and a resignation,
// a vote-kick or a timeout can come in between.
var ErrStalePly = errors.New("session: the game has moved on")

// SelectionsView is what the impulse seat may select next (ADR 0114).
type SelectionsView struct {
	Seat int
	// Ply is the ply the answer was made at: the impulses played.
	Ply int
	// Made are the selections made so far, in the game's notation.
	Made []string
	// Options are the next selections; empty when Complete.
	Options []SelectionOption
	// Complete: the selections made are a move, ready to submit. MayEnd: they
	// may be submitted as they are although more could follow (a game for one).
	Complete bool
	MayEnd   bool
	// State is the position after the selections made as the seat may see it;
	// empty when Complete, or when the asker may not see the seat's choices.
	State string
}

// Selections answers the selections query for roomCode's impulse seat: the
// options after the selections made (text, joined by "; "; empty for the
// first). It is answered on the same terms as legal moves (ADR 0112) and plays
// nothing that ends the impulse, so it cannot show a roll or a draw early.
func (m *Manager) Selections(ctx context.Context, roomCode, made string, ply *int) (SelectionsView, error) {
	roomCode = canonRoom(roomCode)
	game, err := m.loadGame(ctx, roomCode, false)
	if err != nil {
		return SelectionsView{}, err
	}
	if game.Status != "active" || game.ImpulseSeat == nil {
		return SelectionsView{}, ErrGameNotActive
	}
	if ply != nil && *ply != game.Ply {
		return SelectionsView{}, fmt.Errorf("%w: it is at ply %d, not %d", ErrStalePly, game.Ply, *ply)
	}
	compiled, err := m.compiledFor(game)
	if err != nil {
		return SelectionsView{}, err
	}
	seat := *game.ImpulseSeat
	view := SelectionsView{Seat: seat, Ply: game.Ply, Made: []string{}, Options: []SelectionOption{}}
	if ok, err := m.mayListMoves(ctx, compiled, game, seat); err != nil {
		return SelectionsView{}, err
	} else if !ok {
		return view, nil
	}
	var sels []state.Selection
	if made != "" {
		if sels, err = state.ParseImpulse(compiled, made); err != nil {
			return SelectionsView{}, err
		}
	}
	bs := state.NewBoardState()
	if err := bs.UnmarshalJSON([]byte(game.BoardState)); err != nil {
		return SelectionsView{}, fmt.Errorf("session: unmarshal board state for room %q: %w", roomCode, err)
	}
	_, span := trace.Start(ctx, "engine.NextSelections")
	start := time.Now()
	next, err := state.NextSelections(compiled, bs, seat, sels)
	m.mx.Legal(gameFamily(compiled), time.Since(start), len(next.Options))
	span.End()
	if err != nil {
		return SelectionsView{}, err
	}
	for _, mv := range sels {
		view.Made = append(view.Made, state.FormatMove(compiled, mv))
	}
	view.Complete, view.MayEnd = next.Complete, next.MayEnd
	for _, mv := range next.Options {
		view.Options = append(view.Options, SelectionOption{Selection: mv, Notation: state.FormatMove(compiled, mv), Gestures: state.Gestures(compiled, mv)})
	}
	if next.Position != nil {
		doc, err := state.Public(compiled, next.Position, seat).MarshalJSON()
		if err != nil {
			return SelectionsView{}, err
		}
		view.State = string(doc)
	}
	return view, nil
}
