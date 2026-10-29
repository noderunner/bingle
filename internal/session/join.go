package session

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/noderunner/bingle/internal/store"
)

// Role distinguishes a JoinGame caller who wants a seat ("player") from one
// who wants read-only access ("spectator").
type Role string

// Valid Role values.
const (
	RolePlayer    Role = "player"
	RoleSpectator Role = "spectator"
)

// JoinResult is JoinGame's return value: the caller's resulting role, seat
// assignment (0 for a spectator), color ("" for a spectator), and the game's
// status after this join.
type JoinResult struct {
	Role   Role
	Seat   int    // 0 for a spectator
	Color  string // "" for a spectator
	Status string // the game's status after this join
}

// ErrSeatsFull is returned by JoinGame when role == RolePlayer, name doesn't
// match any existing seat, and every declared seat is already taken.
var ErrSeatsFull = errors.New("session: seats full")

// ErrNameIsABot is returned when a human tries to take, or act as, a seat
// held by a built-in bot by typing the bot's name.
var ErrNameIsABot = errors.New("session: that name belongs to a bot in this game")

// JoinGame attaches name to roomCode as either a spectator or a player. See
// docs/history/plan.md section 6.4c ("Join semantics") for the full contract:
//
//   - role == RoleSpectator: always succeeds, no seat, no DB write.
//   - role == RolePlayer, name matches an existing seat's stored name
//     (case-insensitive): reattaches to that seat (the reconnect mechanism —
//     no token), no DB write.
//   - role == RolePlayer, name unmatched, an open seat remains: assigns the
//     next open seat (in the game definition's declared seat order); if that
//     fills the last seat, the game's status moves from "waiting" to
//     "active".
//   - role == RolePlayer, name unmatched, no open seat: returns ErrSeatsFull,
//     no DB write.
//
// Errors from the underlying store (e.g. store.ErrNotFound for an unknown
// room code) are propagated unwrapped, as GetState does.
func (m *Manager) JoinGame(ctx context.Context, roomCode, name string, role Role) (JoinResult, error) {
	return m.JoinGameAt(ctx, roomCode, name, role, 0)
}

// JoinGameAt is JoinGame with a choice of seat: a player taking a new seat sits
// at seat (when it is open) instead of the first open one. A player already
// seated keeps their seat.
func (m *Manager) JoinGameAt(ctx context.Context, roomCode, name string, role Role, wantSeat int) (JoinResult, error) {
	roomCode = canonRoom(roomCode)
	unlock := m.lockRoom(ctx, roomCode)
	defer unlock()

	game, err := m.loadGame(ctx, roomCode, true)
	if err != nil {
		return JoinResult{}, err
	}

	if role == RoleSpectator {
		return JoinResult{Role: RoleSpectator, Status: game.Status}, nil
	}

	players, err := m.store.ListPlayers(ctx, game.ID)
	if err != nil {
		return JoinResult{}, err
	}

	compiled, ok := m.catalog.Get(game.GameDefID)
	if !ok {
		return JoinResult{}, fmt.Errorf("session: game definition %q not found for room %q", game.GameDefID, roomCode)
	}

	playersBySeat := make(map[int]store.Player, len(players))
	for _, p := range players {
		if strings.EqualFold(p.Name, name) {
			if p.Bot != "" {
				return JoinResult{}, ErrNameIsABot
			}
			return JoinResult{Role: RolePlayer, Seat: p.Seat, Color: p.Color, Status: game.Status}, nil
		}
		playersBySeat[p.Seat] = p
	}

	var openSeat int
	var openSeatColor string
	var foundOpenSeat bool
	for _, seatDef := range compiled.Definition.Seats {
		if _, taken := playersBySeat[seatDef.Seat]; taken {
			continue
		}
		if wantSeat != 0 && seatDef.Seat != wantSeat {
			continue
		}
		openSeat = seatDef.Seat
		openSeatColor = seatDef.Color
		foundOpenSeat = true
		break
	}
	if !foundOpenSeat {
		if wantSeat != 0 {
			return JoinResult{}, fmt.Errorf("%w: seat %d is not free", ErrSeatsFull, wantSeat)
		}
		return JoinResult{}, ErrSeatsFull
	}

	newStatus := game.Status
	var newTurnSeat *int
	if len(players)+1 >= len(compiled.Definition.Seats) && !game.Lobby {
		newStatus = "active"
		firstTurnSeat := firstSeatOf(compiled, game)
		newTurnSeat = &firstTurnSeat
	}

	player, err := m.store.AddPlayer(ctx, game.ID, openSeat, name, openSeatColor, newStatus, newTurnSeat)
	if err != nil {
		return JoinResult{}, err
	}

	m.publishState(ctx, roomCode, "join")
	// A bot that moves first gets to play now that the game is active.
	if newStatus == "active" {
		m.runBots(ctx, roomCode)
	}

	return JoinResult{Role: RolePlayer, Seat: player.Seat, Color: player.Color, Status: newStatus}, nil
}
