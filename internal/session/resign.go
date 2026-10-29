package session

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/noderunner/bingle/internal/store"
)

// Resign ends the game with name's opponent as the winner (in a two-seat
// game; with more seats the game simply ends without a winner).
func (m *Manager) Resign(ctx context.Context, roomCode, name string) (StateSnapshot, error) {
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
	for i := range players {
		if strings.EqualFold(players[i].Name, name) {
			player = &players[i]
		}
	}
	if player == nil || player.Bot != "" {
		return StateSnapshot{}, ErrNotAPlayer
	}
	compiled, ok := m.catalog.Get(game.GameDefID)
	if !ok {
		return StateSnapshot{}, fmt.Errorf("session: game definition %q not found for room %q", game.GameDefID, roomCode)
	}

	var winner *int
	if opp, ok := compiled.Opponent(player.Seat); ok {
		winner = &opp
	}
	// Counted here as well as in the move funnel: a game can end without a
	// move being played, and bingle_games_finished_total is meant to be every
	// game that ended, not only the ones that ended on the board.
	if err := m.store.FinishGame(ctx, game.ID, winner, "resignation"); err != nil {
		return StateSnapshot{}, err
	}
	m.mx.Finished(gameFamily(compiled), "resignation")

	m.publishState(ctx, roomCode, "resign")
	return m.GetState(ctx, roomCode, nil)
}

// ErrDrawNotAllowed is returned by Draw for requests that make no sense in
// the current state (no offer to answer, offering to a bot, ...).
var ErrDrawNotAllowed = errors.New("session: draw request not allowed")

// Draw handles draw offers. action is "offer" (propose a draw to the
// opponent), "cancel" (withdraw your own offer), "accept" or "decline" (answer
// the opponent's offer). Accepting ends the game as a draw
// (end_reason "agreement"). Any move withdraws an outstanding offer. Offers
// can't be made to bots, which never respond.
func (m *Manager) Draw(ctx context.Context, roomCode, name, action string) (StateSnapshot, error) {
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
	opponentIsBot := false
	for i := range players {
		if strings.EqualFold(players[i].Name, name) {
			player = &players[i]
		}
	}
	if player == nil || player.Bot != "" {
		return StateSnapshot{}, ErrNotAPlayer
	}
	for _, p := range players {
		if p.Seat != player.Seat && p.Bot != "" {
			opponentIsBot = true
		}
	}

	offered := game.DrawOfferSeat
	switch action {
	case "offer":
		if opponentIsBot {
			return StateSnapshot{}, fmt.Errorf("%w: your opponent is a bot and never answers draw offers", ErrDrawNotAllowed)
		}
		if offered != nil {
			return StateSnapshot{}, fmt.Errorf("%w: a draw has already been offered", ErrDrawNotAllowed)
		}
		seat := player.Seat
		if err := m.store.SetDrawOffer(ctx, game.ID, &seat); err != nil {
			return StateSnapshot{}, err
		}
		m.publishState(ctx, roomCode, "draw-offer")
	case "cancel":
		if offered == nil || *offered != player.Seat {
			return StateSnapshot{}, fmt.Errorf("%w: you have no draw offer to withdraw", ErrDrawNotAllowed)
		}
		if err := m.store.SetDrawOffer(ctx, game.ID, nil); err != nil {
			return StateSnapshot{}, err
		}
		m.publishState(ctx, roomCode, "draw-cancelled")
	case "decline":
		if offered == nil || *offered == player.Seat {
			return StateSnapshot{}, fmt.Errorf("%w: there is no draw offer from your opponent", ErrDrawNotAllowed)
		}
		if err := m.store.SetDrawOffer(ctx, game.ID, nil); err != nil {
			return StateSnapshot{}, err
		}
		m.publishState(ctx, roomCode, "draw-declined")
	case "accept":
		if offered == nil || *offered == player.Seat {
			return StateSnapshot{}, fmt.Errorf("%w: there is no draw offer from your opponent", ErrDrawNotAllowed)
		}
		if err := m.store.FinishGame(ctx, game.ID, nil, "agreement"); err != nil {
			return StateSnapshot{}, err
		}
		m.mx.Finished(m.familyOfGame(game.GameDefID), "agreement")
		m.publishState(ctx, roomCode, "draw-agreed")
	default:
		return StateSnapshot{}, fmt.Errorf("%w: unknown action %q (use offer, cancel, accept or decline)", ErrDrawNotAllowed, action)
	}
	return m.readState(ctx, roomCode, nil)
}
