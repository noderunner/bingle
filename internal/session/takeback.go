package session

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/store"
)

// ErrTakebackNotAllowed is returned by Takeback for requests that make no
// sense in the current state.
var ErrTakebackNotAllowed = errors.New("session: takeback request not allowed")

// Takeback handles take-back requests between human players. Undo takes back
// your own last move on its own as long as no other human has moved since;
// once your opponent has replied, rewinding needs their consent:
//
//   - "request": ask to take back your last move (and everything after it);
//   - "cancel": withdraw your own request;
//   - "accept" / "decline": answer the opponent's request. Accepting rewinds
//     the game to the position before the requester's move, with the
//     requester to move.
//
// Any move (or undo) withdraws an outstanding request.
func (m *Manager) Takeback(ctx context.Context, roomCode, name, action string) (StateSnapshot, error) {
	roomCode = canonRoom(roomCode)
	unlock := m.lockRoom(ctx, roomCode)
	defer unlock()

	game, err := m.loadGame(ctx, roomCode, true)
	if err != nil {
		return StateSnapshot{}, err
	}
	if game.Status != "active" {
		return StateSnapshot{}, ErrGameNotActive
	}
	players, err := m.store.ListPlayers(ctx, game.ID)
	if err != nil {
		return StateSnapshot{}, err
	}
	var player *store.Player
	botSeats := map[int]bool{}
	for i := range players {
		if strings.EqualFold(players[i].Name, name) {
			player = &players[i]
		}
		if players[i].Bot != "" {
			botSeats[players[i].Seat] = true
		}
	}
	if player == nil || player.Bot != "" {
		return StateSnapshot{}, ErrNotAPlayer
	}
	compiled, ok := m.catalog.Get(game.GameDefID)
	if !ok {
		return StateSnapshot{}, fmt.Errorf("session: game definition %q not found for room %q", game.GameDefID, roomCode)
	}

	asked := game.TakebackSeat
	switch action {
	case "request":
		if len(botSeats) > 0 {
			return StateSnapshot{}, fmt.Errorf("%w: your opponent is a bot; just undo", ErrTakebackNotAllowed)
		}
		if asked != nil {
			return StateSnapshot{}, fmt.Errorf("%w: a take-back request is already open", ErrTakebackNotAllowed)
		}
		moves, err := m.store.ListMoves(ctx, game.ID, 0)
		if err != nil {
			return StateSnapshot{}, err
		}
		idx := -1
		for i := len(moves) - 1; i >= 0; i-- {
			if moves[i].Seat == player.Seat {
				idx = i
				break
			}
		}
		if idx < 0 {
			return StateSnapshot{}, fmt.Errorf("%w: you have not moved yet", ErrTakebackNotAllowed)
		}
		if idx == len(moves)-1 {
			return StateSnapshot{}, fmt.Errorf("%w: nobody has replied to your move, so you can undo it directly", ErrTakebackNotAllowed)
		}
		if state.IsRuleset(compiled) {
			before := ""
			if idx == 0 {
				before, err = startDoc(compiled, game)
				if err != nil {
					return StateSnapshot{}, err
				}
			} else {
				before = moves[idx-1].ResultingState
			}
			if err := m.checkTakeBack(compiled, before, game.BoardState); err != nil {
				return StateSnapshot{}, fmt.Errorf("%w: %v", ErrTakebackNotAllowed, err)
			}
		}
		seat := player.Seat
		if err := m.store.SetTakebackRequest(ctx, game.ID, &seat, moves[idx].Seq-1); err != nil {
			return StateSnapshot{}, err
		}
		m.publishState(ctx, roomCode, "takeback-request")

	case "cancel":
		if asked == nil || *asked != player.Seat {
			return StateSnapshot{}, fmt.Errorf("%w: you have no take-back request to withdraw", ErrTakebackNotAllowed)
		}
		if err := m.store.SetTakebackRequest(ctx, game.ID, nil, 0); err != nil {
			return StateSnapshot{}, err
		}
		m.publishState(ctx, roomCode, "takeback-cancelled")

	case "decline":
		if asked == nil || *asked == player.Seat {
			return StateSnapshot{}, fmt.Errorf("%w: there is no take-back request from your opponent", ErrTakebackNotAllowed)
		}
		if err := m.store.SetTakebackRequest(ctx, game.ID, nil, 0); err != nil {
			return StateSnapshot{}, err
		}
		m.publishState(ctx, roomCode, "takeback-declined")

	case "accept":
		if asked == nil || *asked == player.Seat {
			return StateSnapshot{}, fmt.Errorf("%w: there is no take-back request from your opponent", ErrTakebackNotAllowed)
		}
		keepSeq := game.TakebackSeq
		moves, err := m.store.ListMoves(ctx, game.ID, 0)
		if err != nil {
			return StateSnapshot{}, err
		}
		if keepSeq < 0 || keepSeq >= len(moves) {
			// The request is stale (cannot normally happen: any move
			// clears it); drop it rather than corrupt the game.
			_ = m.store.SetTakebackRequest(ctx, game.ID, nil, 0)
			return StateSnapshot{}, fmt.Errorf("%w: the request no longer applies", ErrTakebackNotAllowed)
		}
		var doc string
		if keepSeq == 0 {
			doc, err = startDoc(compiled, game)
			if err != nil {
				return StateSnapshot{}, err
			}
		} else {
			doc = moves[keepSeq-1].ResultingState
		}
		if err := m.store.UndoToSeq(ctx, game.ID, keepSeq, doc, *asked); err != nil {
			return StateSnapshot{}, err
		}
		removed := make([]int, 0, len(moves)-keepSeq)
		for _, mv := range moves[keepSeq:] {
			removed = append(removed, mv.Seq)
		}
		m.publishUndo(ctx, roomCode, removed)

	default:
		return StateSnapshot{}, fmt.Errorf("%w: unknown action %q (use request, cancel, accept or decline)", ErrTakebackNotAllowed, action)
	}
	return m.readState(ctx, roomCode, nil)
}
