package session

import (
	"context"
	"fmt"
	"strings"
)

// AddBotResult is what AddBot reports: the seat the bot took and whether the
// game started.
type AddBotResult struct {
	// Seat is the seat the bot took.
	Seat int `json:"seat"`
	// Name is the bot's name at the table, after the seat's faction or colour
	// and its kind (Black Goat (MCTS)).
	Name string `json:"name"`
	// Color is the seat's colour.
	Color string `json:"color"`
	// Bot is the bot's id, as asked for.
	Bot string `json:"bot"`
	// Status is the game's afterwards: active when the bot filled the last
	// seat of a game without a lobby, waiting otherwise.
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
	if game.Competitive() {
		return AddBotResult{}, fmt.Errorf("%w: bots do not play rated games", ErrRatedGame)
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
	reserved, err := m.reservedFor(ctx, game.ID)
	if err != nil {
		return AddBotResult{}, err
	}
	open, color := 0, ""
	var openDef = compiled.Definition.Seats[0]
	for _, sd := range compiled.Definition.Seats {
		if taken[sd.Seat] || wantSeat != 0 && sd.Seat != wantSeat {
			continue
		}
		// A seat kept for a friend gets a bot only if the host asks for that seat, which gives it up.
		if _, kept := reserved[sd.Seat]; kept && (wantSeat == 0 || !m.isHostNamed(players, by)) {
			continue
		}
		open, color, openDef = sd.Seat, sd.Color, sd
		break
	}
	if open == 0 {
		return AddBotResult{}, ErrSeatsFull
	}
	if _, kept := reserved[open]; kept {
		if err := m.store.ReleaseSeat(ctx, game.ID, open); err != nil {
			return AddBotResult{}, err
		}
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
