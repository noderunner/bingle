package session

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/store"
)

// ErrNothingToUndo is returned by Undo when the caller has no move to take
// back.
var ErrNothingToUndo = errors.New("session: nothing to undo")

// ErrCannotUndo is returned by Undo when the caller's last move can no
// longer be taken back because someone else has moved since.
var ErrCannotUndo = errors.New("session: cannot undo: another player has moved since")

// UndoOutcome describes the state after an Undo.
type UndoOutcome struct {
	// Removed lists the sequence numbers of the moves taken back.
	Removed  []int
	Snapshot StateSnapshot
}

// Undo takes back name's most recent move, along with any replies bots made
// to it. It fails with ErrCannotUndo once another human has moved since, and
// with ErrNothingToUndo when name has not moved. A game that ended in
// resignation cannot be undone.
func (m *Manager) Undo(ctx context.Context, roomCode, name string) (UndoOutcome, error) {
	roomCode = canonRoom(roomCode)
	unlock := m.lockRoom(ctx, roomCode)
	defer unlock()

	game, err := m.loadGame(ctx, roomCode, true)
	if err != nil {
		return UndoOutcome{}, err
	}
	players, err := m.store.ListPlayers(ctx, game.ID)
	if err != nil {
		return UndoOutcome{}, err
	}
	var player *store.Player
	botSeats := map[int]bool{}
	names := map[int]string{}
	for i := range players {
		if strings.EqualFold(players[i].Name, name) {
			player = &players[i]
		}
		if players[i].Bot != "" {
			botSeats[players[i].Seat] = true
		}
		names[players[i].Seat] = players[i].Name
	}
	if player == nil || player.Bot != "" {
		return UndoOutcome{}, ErrNotAPlayer
	}
	// The seats a person plays besides their own (a game against oneself) are
	// theirs to take back too.
	for seat, n := range names {
		if seat != player.Seat && sameOwner(player.Name, n) {
			botSeats[seat] = true
		}
	}
	compiled, ok := m.catalog.Get(game.GameDefID)
	if !ok {
		return UndoOutcome{}, fmt.Errorf("session: game definition %q not found for room %q", game.GameDefID, roomCode)
	}
	if game.Status == "finished" && game.EndReason == "resignation" {
		return UndoOutcome{}, ErrCannotUndo
	}

	moves, err := m.store.ListMoves(ctx, game.ID, 0)
	if err != nil {
		return UndoOutcome{}, err
	}
	// A pie-rule swap exchanged the seats' occupants: a move at or before it
	// belongs to whoever now holds the other seat.
	swapSeq := swapSeqOf(compiled, moves)
	idx := -1
	for i := len(moves) - 1; i >= 0; i-- {
		if moverSeatNow(moves[i], swapSeq) == player.Seat {
			idx = i
			break
		}
	}
	if idx < 0 {
		return UndoOutcome{}, ErrNothingToUndo
	}
	for _, later := range moves[idx+1:] {
		if !botSeats[moverSeatNow(later, swapSeq)] && !later.Auto {
			return UndoOutcome{}, ErrCannotUndo
		}
	}
	// The move rolled a die, or answers one that did: the die is cast.
	if moves[idx].Locked {
		return UndoOutcome{}, ErrDiceRolled
	}

	keepSeq := moves[idx].Seq - 1
	var doc string
	if idx == 0 {
		start, err := startDoc(compiled, game)
		if err != nil {
			return UndoOutcome{}, err
		}
		doc = start
	} else {
		doc = moves[idx-1].ResultingState
	}

	if state.IsRuleset(compiled) {
		if err := m.checkTakeBack(compiled, doc, game.BoardState); err != nil {
			return UndoOutcome{}, err
		}
	}

	// Taking back across the swap puts the players back in their first seats:
	// the seat to move is the undoer's old one.
	undoSwap := swapSeq > 0 && swapSeq > keepSeq
	turnSeat := player.Seat
	if undoSwap {
		turnSeat = 3 - turnSeat
	}
	if err := m.store.UndoToSeq(ctx, game.ID, keepSeq, doc, turnSeat, undoSwap); err != nil {
		return UndoOutcome{}, err
	}

	removed := make([]int, 0, len(moves)-idx)
	for _, mv := range moves[idx:] {
		removed = append(removed, mv.Seq)
	}

	m.publishUndo(ctx, roomCode, removed)

	snap, err := m.GetState(ctx, roomCode, nil)
	if err != nil {
		return UndoOutcome{}, err
	}
	return UndoOutcome{Removed: removed, Snapshot: snap}, nil
}

// ErrDiceRolled is returned when taking moves back would rewind across a die
// roll (see state.UndoGuard).
var ErrDiceRolled = fmt.Errorf("%w: dice have been rolled since that move, and they cannot be rolled again", ErrCannotUndo)

// checkTakeBack asks the game's ruleset whether the game may return from
// the position after to the one before (both position documents).
func (m *Manager) checkTakeBack(compiled *catalog.CompiledGame, before, after string) error {
	b, a := state.NewBoardState(), state.NewBoardState()
	if err := b.UnmarshalJSON([]byte(before)); err != nil {
		return nil
	}
	if err := a.UnmarshalJSON([]byte(after)); err != nil {
		return nil
	}
	if !state.CanTakeBack(compiled, b, a) {
		return ErrDiceRolled
	}
	return nil
}
