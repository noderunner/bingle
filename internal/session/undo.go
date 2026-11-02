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

// A move stands once the turn has passed (ADR 0083). The only thing that can be
// taken back is a step of the turn still being played: a stone of two, a unit of
// a Move, while it is still the same player's turn and nothing random has
// happened. That goes for every game: casual, rated or in a tournament.

// ErrNothingToUndo is returned by Undo when no step of this turn has been played.
var ErrNothingToUndo = errors.New("session: nothing to undo")

// ErrCannotUndo is returned by Undo once the moves stand.
var ErrCannotUndo = errors.New("session: cannot undo: the turn has passed, and moves stand")

// ErrDiceRolled is returned when the step rolled a die or drew a hidden token,
// or was decided on seeing one (see state.UndoGuard).
var ErrDiceRolled = fmt.Errorf("%w: dice have been rolled, and they cannot be rolled again", ErrCannotUndo)

// UndoOutcome describes the state after an Undo.
type UndoOutcome struct {
	// Removed lists the sequence numbers of the moves taken back: the step, and
	// any reaction passed for somebody straight after it.
	Removed  []int
	Snapshot StateSnapshot
}

// Undo takes back the last step of the turn name is playing. It fails with
// ErrNothingToUndo before any step, and with ErrCannotUndo once the turn has passed
// or the game has ended.
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
	for i := range players {
		if strings.EqualFold(players[i].Name, name) {
			player = &players[i]
		}
	}
	if player == nil || player.Bot != "" {
		return UndoOutcome{}, ErrNotAPlayer
	}
	if err := m.mayActFor(ctx, player); err != nil {
		return UndoOutcome{}, err
	}
	if game.Status != "active" || game.TurnSeat == nil {
		return UndoOutcome{}, ErrCannotUndo
	}
	// The seat to move has to be this person's, whichever of their seats of a game
	// against themselves they asked from.
	turn := *game.TurnSeat
	mine := player.Seat == turn
	for _, p := range players {
		if p.Seat == turn && p.Bot == "" && sameOwner(p.Name, player.Name) {
			mine = true
		}
	}
	if !mine {
		return UndoOutcome{}, ErrCannotUndo
	}
	compiled, err := m.compiledFor(game)
	if err != nil {
		return UndoOutcome{}, err
	}
	moves, err := m.store.ListMoves(ctx, game.ID, 0)
	if err != nil {
		return UndoOutcome{}, err
	}
	// The last move anybody chose. A reaction passed for somebody asked nobody, so
	// it does not hand the turn on.
	idx := len(moves) - 1
	for idx >= 0 && moves[idx].Auto {
		idx--
	}
	if idx < 0 {
		return UndoOutcome{}, ErrNothingToUndo
	}
	// A pie-rule swap exchanged the seats' occupants, so its move and those before
	// it belong to whoever now holds the other seat: the turn has passed.
	if moverSeatNow(moves[idx], swapSeqOf(compiled, moves)) != turn {
		if !movedAtSeat(moves, turn) {
			return UndoOutcome{}, ErrNothingToUndo
		}
		return UndoOutcome{}, ErrCannotUndo
	}
	if moves[idx].Locked {
		return UndoOutcome{}, ErrDiceRolled
	}

	keepSeq := moves[idx].Seq - 1
	var doc string
	if idx == 0 {
		if doc, err = startDoc(compiled, game); err != nil {
			return UndoOutcome{}, err
		}
	} else {
		doc = moves[idx-1].ResultingState
	}
	if state.IsRuleset(compiled) {
		if err := m.checkTakeBack(compiled, doc, game.BoardState); err != nil {
			return UndoOutcome{}, err
		}
	}
	if err := m.store.UndoToSeq(ctx, game.ID, keepSeq, doc, turn); err != nil {
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

func movedAtSeat(moves []store.Move, seat int) bool {
	for _, mv := range moves {
		if mv.Seat == seat {
			return true
		}
	}
	return false
}

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

// MoveIsFinal says whether a legal move could not be taken back once played: it
// hands the turn on, ends the game or brings in chance. A page asks this before
// sending a move, so that it asks for a Submit only when that matters. It says
// nothing about the position that the rules do not already show.
func (m *Manager) MoveIsFinal(ctx context.Context, roomCode, name, notation string) (bool, error) {
	roomCode = canonRoom(roomCode)
	game, err := m.loadGame(ctx, roomCode, false)
	if err != nil {
		return false, err
	}
	if game.Status != "active" || game.TurnSeat == nil {
		return false, ErrGameNotActive
	}
	players, err := m.store.ListPlayers(ctx, game.ID)
	if err != nil {
		return false, err
	}
	var player *store.Player
	for i := range players {
		if strings.EqualFold(players[i].Name, name) {
			player = &players[i]
		}
	}
	if player == nil || player.Bot != "" || player.Seat != *game.TurnSeat {
		return false, ErrNotYourTurn
	}
	if err := m.mayActFor(ctx, player); err != nil {
		return false, err
	}
	compiled, err := m.compiledFor(game)
	if err != nil {
		return false, err
	}
	mv, err := state.ParseMove(compiled, notation)
	if err != nil {
		return false, err
	}
	bs := state.NewBoardState()
	if err := bs.UnmarshalJSON([]byte(game.BoardState)); err != nil {
		return false, fmt.Errorf("session: unmarshal board state for room %q: %w", game.RoomCode, err)
	}
	result, err := m.applyWatched(compiled, bs, player.Seat, mv)
	if err != nil {
		return false, err
	}
	if result.GameEnded || result.NextTurnSeat != player.Seat || isPieSwap(compiled, result.Move) {
		return true, nil
	}
	return state.IsRuleset(compiled) && !state.CanTakeBack(compiled, bs, result.NewState), nil
}
