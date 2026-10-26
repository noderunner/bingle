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

// A game created with a lobby waits for its host to start it: the players
// gather through the invite link, choose their seats (or take bots for them)
// and nobody plays until the host is satisfied with the table.

// ErrNotHost is returned when someone other than the host tries a host-only
// action of a lobby (starting the game, removing a player).
var ErrNotHost = errors.New("session: only the host of this game can do that")

// ErrAlreadyStarted is returned for lobby actions on a game that began.
var ErrAlreadyStarted = errors.New("session: the game has already started")

// ErrSeatsOpen is returned when the host starts a game with seats still open.
var ErrSeatsOpen = errors.New("session: some seats are still open — wait for players or add bots")

// hostOf is the seat of the person who created the game: the first human to
// sit down. It is 0 when no human is seated.
func hostOf(players []store.Player) int {
	var best *store.Player
	for i := range players {
		if players[i].Bot != "" {
			continue
		}
		if best == nil || players[i].ID < best.ID {
			best = &players[i]
		}
	}
	if best == nil {
		return 0
	}
	return best.Seat
}

// lobbyContext loads a waiting game, its players and the caller's seat.
func (m *Manager) lobbyContext(ctx context.Context, roomCode, name string) (store.Game, []store.Player, *store.Player, error) {
	game, err := m.loadGame(ctx, roomCode, true)
	if err != nil {
		return store.Game{}, nil, nil, err
	}
	players, err := m.store.ListPlayers(ctx, game.ID)
	if err != nil {
		return store.Game{}, nil, nil, err
	}
	var me *store.Player
	for i := range players {
		if players[i].Bot == "" && strings.EqualFold(players[i].Name, name) {
			me = &players[i]
		}
	}
	if me == nil {
		return store.Game{}, nil, nil, ErrNotAPlayer
	}
	if game.Status != "waiting" {
		return store.Game{}, nil, nil, ErrAlreadyStarted
	}
	return game, players, me, nil
}

// StartGame begins a waiting game. Only its host may, and every seat must be
// filled (the host can fill open seats with bots first).
func (m *Manager) StartGame(ctx context.Context, roomCode, name string) (StateSnapshot, error) {
	roomCode = canonRoom(roomCode)
	unlock := m.lockRoom(roomCode)
	defer unlock()
	game, players, me, err := m.lobbyContext(ctx, roomCode, name)
	if err != nil {
		return StateSnapshot{}, err
	}
	if hostOf(players) != me.Seat {
		return StateSnapshot{}, ErrNotHost
	}
	compiled, ok := m.catalog.Get(game.GameDefID)
	if !ok {
		return StateSnapshot{}, fmt.Errorf("session: game definition %q not found for room %q", game.GameDefID, roomCode)
	}
	if len(players) < len(compiled.Definition.Seats) {
		return StateSnapshot{}, ErrSeatsOpen
	}
	if err := m.store.StartGame(ctx, game.ID, firstSeatOf(compiled, game)); err != nil {
		return StateSnapshot{}, err
	}
	m.publishState(ctx, roomCode, "start")
	m.runBots(ctx, roomCode)
	return m.readState(ctx, roomCode, nil)
}

// ChooseSeat moves the player called name to the open seat wanted before the
// game starts.
func (m *Manager) ChooseSeat(ctx context.Context, roomCode, name string, wanted int) (StateSnapshot, error) {
	roomCode = canonRoom(roomCode)
	unlock := m.lockRoom(roomCode)
	defer unlock()
	game, players, me, err := m.lobbyContext(ctx, roomCode, name)
	if err != nil {
		return StateSnapshot{}, err
	}
	compiled, ok := m.catalog.Get(game.GameDefID)
	if !ok {
		return StateSnapshot{}, fmt.Errorf("session: game definition %q not found for room %q", game.GameDefID, roomCode)
	}
	sd, ok := compiled.Seat(wanted)
	if !ok {
		return StateSnapshot{}, fmt.Errorf("%w: game %q has no seat %d", ErrInvalidSeat, game.GameDefID, wanted)
	}
	for _, p := range players {
		if p.Seat == wanted && p.Seat != me.Seat {
			return StateSnapshot{}, fmt.Errorf("%w: seat %d is taken", ErrSeatsFull, wanted)
		}
	}
	if wanted != me.Seat {
		if err := m.store.MovePlayerSeat(ctx, game.ID, me.Seat, wanted, sd.Color); err != nil {
			return StateSnapshot{}, err
		}
		m.publishState(ctx, roomCode, "seat")
	}
	return m.readState(ctx, roomCode, nil)
}

// ErrSeatTaken is returned when the cult of a seat that a player or bot holds
// is asked to change.
var ErrSeatTaken = errors.New("session: that seat is taken: only the cult of an open seat can change")

// ErrNoCultChoice is returned for a game whose seats are not cults to choose.
var ErrNoCultChoice = errors.New("session: the seats of this game have no cult to choose")

// SetSeatCult changes the cult of an open seat of a game that has not started
// (a cult that another open seat plays changes places with it). Any person at
// the table may; the cults of seats a player or bot holds stay. The game takes
// the definition of the new table and a fresh starting position.
func (m *Manager) SetSeatCult(ctx context.Context, roomCode, by string, seat int, cult string) (StateSnapshot, error) {
	roomCode = canonRoom(roomCode)
	unlock := m.lockRoom(roomCode)
	defer unlock()
	game, players, _, err := m.lobbyContext(ctx, roomCode, by)
	if err != nil {
		return StateSnapshot{}, err
	}
	compiled, ok := m.catalog.Get(game.GameDefID)
	if !ok {
		return StateSnapshot{}, fmt.Errorf("session: game definition %q not found for room %q", game.GameDefID, roomCode)
	}
	if _, ok := compiled.Seat(seat); !ok {
		return StateSnapshot{}, fmt.Errorf("%w: game %q has no seat %d", ErrInvalidSeat, game.GameDefID, seat)
	}
	taken := map[int]bool{}
	for _, p := range players {
		taken[p.Seat] = true
	}
	if taken[seat] {
		return StateSnapshot{}, ErrSeatTaken
	}
	options, ok, err := state.SeatCultOptions(compiled, seat, cult, taken)
	if !ok {
		return StateSnapshot{}, ErrNoCultChoice
	}
	if errors.Is(err, state.ErrCultHeld) {
		return StateSnapshot{}, fmt.Errorf("%w: %v", ErrSeatTaken, err)
	}
	if err != nil {
		return StateSnapshot{}, fmt.Errorf("%w: %v", ErrInvalidOptions, err)
	}
	if options == nil {
		return m.readState(ctx, roomCode, nil) // already so
	}
	base, _, _ := strings.Cut(game.GameDefID, catalog.DerivedSep)
	newID, err := m.catalog.Derive(base, options)
	if err != nil {
		return StateSnapshot{}, fmt.Errorf("%w: %v", ErrInvalidOptions, err)
	}
	if newID == game.GameDefID {
		return m.readState(ctx, roomCode, nil)
	}
	next, ok := m.catalog.Get(newID)
	if !ok {
		return StateSnapshot{}, fmt.Errorf("%w: %q", ErrGameDefNotFound, newID)
	}
	start := state.InitialState(next)
	doc, err := start.MarshalJSON()
	if err != nil {
		return StateSnapshot{}, fmt.Errorf("session: marshal initial board state: %w", err)
	}
	if err := m.store.ChangeLobbyGame(ctx, game.ID, newID, string(doc)); err != nil {
		return StateSnapshot{}, err
	}
	// A game whose start carries a seed keeps it on record, as at creation.
	if state.NeedsStartRecord(next) {
		if err := m.store.SetInitialPosition(ctx, game.ID, string(doc), state.FirstSeat(next)); err != nil {
			return StateSnapshot{}, fmt.Errorf("session: recording start position: %w", err)
		}
	}
	m.publishState(ctx, roomCode, "cult")
	return m.readState(ctx, roomCode, nil)
}

// LeaveSeat takes the player called name out of a game that has not started.
// The host's role passes to the next person at the table.
func (m *Manager) LeaveSeat(ctx context.Context, roomCode, name string) (StateSnapshot, error) {
	roomCode = canonRoom(roomCode)
	unlock := m.lockRoom(roomCode)
	defer unlock()
	game, _, me, err := m.lobbyContext(ctx, roomCode, name)
	if err != nil {
		return StateSnapshot{}, err
	}
	if err := m.store.RemovePlayer(ctx, game.ID, me.Seat); err != nil {
		return StateSnapshot{}, err
	}
	m.publishState(ctx, roomCode, "leave")
	return m.readState(ctx, roomCode, nil)
}

// RemoveSeat lets the host clear another seat — a player who should not be
// there, or a bot to be replaced — before the game starts.
func (m *Manager) RemoveSeat(ctx context.Context, roomCode, by string, seat int) (StateSnapshot, error) {
	roomCode = canonRoom(roomCode)
	unlock := m.lockRoom(roomCode)
	defer unlock()
	game, players, me, err := m.lobbyContext(ctx, roomCode, by)
	if err != nil {
		return StateSnapshot{}, err
	}
	if hostOf(players) != me.Seat {
		return StateSnapshot{}, ErrNotHost
	}
	if seat == me.Seat {
		return StateSnapshot{}, fmt.Errorf("session: leave your own seat with leave, not remove")
	}
	found := false
	for _, p := range players {
		if p.Seat == seat {
			found = true
		}
	}
	if !found {
		return StateSnapshot{}, store.ErrNotFound
	}
	if err := m.store.RemovePlayer(ctx, game.ID, seat); err != nil {
		return StateSnapshot{}, err
	}
	m.publishState(ctx, roomCode, "leave")
	return m.readState(ctx, roomCode, nil)
}

// Host reports the name of the host of a game (empty when nobody human sits).
func (m *Manager) Host(ctx context.Context, roomCode string) (int, error) {
	roomCode = canonRoom(roomCode)
	game, err := m.store.GetGame(ctx, roomCode)
	if err != nil {
		return 0, err
	}
	players, err := m.store.ListPlayers(ctx, game.ID)
	if err != nil {
		return 0, err
	}
	return hostOf(players), nil
}
