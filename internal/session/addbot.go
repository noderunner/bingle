package session

import (
	"context"
	"fmt"
	"strings"
)

// AddBotResult is what AddBot reports: the seat the bot took and whether the
// game started.
type AddBotResult struct {
	Seat   int    `json:"seat"`
	Name   string `json:"name"`
	Color  string `json:"color"`
	Bot    string `json:"bot"`
	Status string `json:"status"`
}

// AddBot seats the built-in bot botID in the lowest open seat of a game that
// is still waiting for players, so mixed tables of people and computers can
// be set up in games with more than two seats. by names a player already in
// the game (only people at the table may fill it).
func (m *Manager) AddBot(ctx context.Context, roomCode, by, botID string) (AddBotResult, error) {
	return m.AddBotAt(ctx, roomCode, by, botID, 0)
}

// AddBotAt is AddBot for a chosen seat (0 = the lowest open one). The bot is
// named after the faction or colour of its seat and its kind.
func (m *Manager) AddBotAt(ctx context.Context, roomCode, by, botID string, wantSeat int) (AddBotResult, error) {
	roomCode = canonRoom(roomCode)
	unlock := m.lockRoom(ctx, roomCode)
	defer unlock()

	game, err := m.loadGame(ctx, roomCode, true)
	if err != nil {
		return AddBotResult{}, err
	}
	bot, ok := m.bots.Get(botID)
	if !ok {
		return AddBotResult{}, fmt.Errorf("%w: %q", ErrUnknownBot, botID)
	}
	compiled, err := m.compiledFor(game)
	if err != nil {
		return AddBotResult{}, err
	}
	players, err := m.store.ListPlayers(ctx, game.ID)
	if err != nil {
		return AddBotResult{}, err
	}
	seated := false
	taken := map[int]bool{}
	for _, p := range players {
		taken[p.Seat] = true
		if p.Bot == "" && strings.EqualFold(p.Name, by) {
			if err := m.mayActFor(ctx, &p); err != nil {
				return AddBotResult{}, err
			}
			seated = true
		}
	}
	if !seated {
		return AddBotResult{}, ErrNotAPlayer
	}
	if game.Status != "waiting" {
		return AddBotResult{}, ErrSeatsFull
	}
	open, color := 0, ""
	var openDef = compiled.Definition.Seats[0]
	for _, sd := range compiled.Definition.Seats {
		if taken[sd.Seat] || wantSeat != 0 && sd.Seat != wantSeat {
			continue
		}
		open, color, openDef = sd.Seat, sd.Color, sd
		break
	}
	if open == 0 {
		return AddBotResult{}, ErrSeatsFull
	}
	status := game.Status
	var turn *int
	if len(players)+1 >= len(compiled.Definition.Seats) && !game.Lobby {
		status = "active"
		first := firstSeatOf(compiled, game)
		turn = &first
	}
	name := botName(bot, openDef, len(compiled.Definition.Seats))
	if _, err := m.store.AddBotPlayer(ctx, game.ID, open, name, color, bot.ID(), status, turn); err != nil {
		return AddBotResult{}, fmt.Errorf("session: seating bot: %w", err)
	}
	m.publishState(ctx, roomCode, "join")
	if status == "active" {
		m.runBots(ctx, roomCode)
	}
	return AddBotResult{Seat: open, Name: name, Color: color, Bot: bot.ID(), Status: status}, nil
}
