package session

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/store"
)

// The Auto-Play Timeout: a person who is slow to act can have a tactical bot
// play for them. When it is their turn to act (a turn, or a reaction waiting
// for their answer) and nothing has happened in the game for the time they
// chose, the bot takes the decisions of that turn, until play passes to
// someone else. The moves are recorded as the person's own, marked as
// auto-play, so the move list and the history say who really played them.
//
// The wait is counted from the last change to the game — the move that made
// it the person's turn, or their own previous decision — so it starts afresh
// after every move, by the person or by the bot. Time the server spent down
// does not count: nobody could play then.

const (
	// MinAutoPlayTimeout and MaxAutoPlayTimeout bound the setting, in seconds.
	MinAutoPlayTimeout = 60
	MaxAutoPlayTimeout = 365 * 24 * 60 * 60

	// autoPlayEvery is how often the watcher looks for a timeout that ran out.
	autoPlayEvery = 20 * time.Second
	// maxAutoPlayMoves bounds the decisions a bot takes in one turn.
	maxAutoPlayMoves = 400
	// autoPlayBot is the bot that plays.
	autoPlayBot = "tactical"
)

// ErrBadAutoPlayTimeout is returned for a timeout outside the allowed range.
var ErrBadAutoPlayTimeout = fmt.Errorf("session: the Auto-Play Timeout must be between a minute and a year")

// AutoPlaySettings is one person's Auto-Play Timeout in a game.
type AutoPlaySettings struct {
	Seat int `json:"seat"`
	// Enabled says the option is on; TimeoutSeconds is the wait before a bot
	// plays, DefaultTimeoutSeconds the wait the option starts with.
	Enabled               bool `json:"enabled"`
	TimeoutSeconds        int  `json:"timeout_seconds"`
	DefaultTimeoutSeconds int  `json:"default_timeout_seconds"`
	// Deadline (RFC 3339) is when a bot plays, while the option is on and it
	// is the person's turn to act; ServerTime is the server's clock.
	Deadline   string `json:"deadline,omitempty"`
	ServerTime string `json:"server_time"`
}

// AutoPlaySettings returns the Auto-Play Timeout of the person called name.
func (m *Manager) AutoPlaySettings(ctx context.Context, roomCode, name string) (AutoPlaySettings, error) {
	roomCode = canonRoom(roomCode)
	game, err := m.loadGame(ctx, roomCode, false)
	if err != nil {
		return AutoPlaySettings{}, err
	}
	return m.autoPlaySettings(ctx, game, name)
}

// personSeat finds the person called name (not a bot) among the players.
func (m *Manager) personSeat(ctx context.Context, game store.Game, name string) (store.Player, error) {
	players, err := m.store.ListPlayers(ctx, game.ID)
	if err != nil {
		return store.Player{}, err
	}
	for _, p := range players {
		if p.Bot == "" && strings.EqualFold(p.Name, name) {
			return p, nil
		}
	}
	return store.Player{}, ErrNotAPlayer
}

// autoPlaySince is when the wait of the player to move began: the game's last
// change, or the moment the watcher came up if that is later (the server was
// down in between).
func (m *Manager) autoPlaySince(updatedAt string) (time.Time, bool) {
	since, ok := parseTimestamp(updatedAt)
	if !ok {
		return time.Time{}, false
	}
	m.mu.Lock()
	from := m.autoPlayFrom
	m.mu.Unlock()
	if from.After(since) {
		since = from
	}
	return since, true
}

func (m *Manager) autoPlaySettings(ctx context.Context, game store.Game, name string) (AutoPlaySettings, error) {
	me, err := m.personSeat(ctx, game, name)
	if err != nil {
		return AutoPlaySettings{}, err
	}
	prefs, err := m.store.AutoPlay(ctx, game.ID, me.Seat)
	if err != nil {
		return AutoPlaySettings{}, err
	}
	out := AutoPlaySettings{
		Seat: me.Seat, Enabled: prefs.Enabled, TimeoutSeconds: prefs.TimeoutSeconds,
		DefaultTimeoutSeconds: store.DefaultAutoPlayTimeout,
		ServerTime:            m.now().UTC().Format(time.RFC3339),
	}
	if prefs.Enabled && game.Status == "active" && game.TurnSeat != nil && *game.TurnSeat == me.Seat {
		if since, ok := m.autoPlaySince(game.UpdatedAt); ok {
			out.Deadline = since.Add(time.Duration(prefs.TimeoutSeconds) * time.Second).UTC().Format(time.RFC3339)
		}
	}
	return out, nil
}

// SetAutoPlay turns the Auto-Play Timeout of the person called name on or
// off, and sets the wait. Arguments that are nil keep their value. It is for
// people: a bot does not need it.
func (m *Manager) SetAutoPlay(ctx context.Context, roomCode, name string, enabled *bool, timeoutSeconds *int) (AutoPlaySettings, error) {
	roomCode = canonRoom(roomCode)
	unlock := m.lockRoom(roomCode)
	defer unlock()
	game, err := m.loadGame(ctx, roomCode, true)
	if err != nil {
		return AutoPlaySettings{}, err
	}
	me, err := m.personSeat(ctx, game, name)
	if err != nil {
		return AutoPlaySettings{}, err
	}
	prefs, err := m.store.AutoPlay(ctx, game.ID, me.Seat)
	if err != nil {
		return AutoPlaySettings{}, err
	}
	if enabled != nil {
		prefs.Enabled = *enabled
	}
	if timeoutSeconds != nil {
		if *timeoutSeconds < MinAutoPlayTimeout || *timeoutSeconds > MaxAutoPlayTimeout {
			return AutoPlaySettings{}, ErrBadAutoPlayTimeout
		}
		prefs.TimeoutSeconds = *timeoutSeconds
	}
	if err := m.store.SetAutoPlay(ctx, game.ID, me.Seat, prefs); err != nil {
		return AutoPlaySettings{}, err
	}
	return m.autoPlaySettings(ctx, game, name)
}

// RunAutoPlay watches for Auto-Play Timeouts that ran out until ctx ends, and
// lets a bot play the turns that were not taken. every is how often it looks
// (a default of a few seconds when zero).
func (m *Manager) RunAutoPlay(ctx context.Context, every time.Duration) {
	if every <= 0 {
		every = autoPlayEvery
	}
	m.mu.Lock()
	m.autoPlayFrom = m.now()
	m.mu.Unlock()
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.AutoPlayDue(ctx)
		}
	}
}

// AutoPlayDue lets a tactical bot play, in every game, the turn of the person
// whose Auto-Play Timeout has run out. It returns the rooms where a bot
// played.
func (m *Manager) AutoPlayDue(ctx context.Context) []string {
	cands, err := m.store.AutoPlayCandidates(ctx)
	if err != nil {
		return nil
	}
	var rooms []string
	for _, c := range cands {
		since, ok := m.autoPlaySince(c.UpdatedAt)
		if !ok || m.now().Before(since.Add(time.Duration(c.TimeoutSeconds)*time.Second)) {
			continue
		}
		if m.autoPlayRoom(ctx, c.RoomCode, c.Seat) {
			rooms = append(rooms, c.RoomCode)
		}
	}
	return rooms
}

// autoPlayRoom plays the turn of seat's person in the room, if their timeout
// has really run out (it is checked again under the room lock: they may have
// moved in the meantime). It reports whether a bot played.
func (m *Manager) autoPlayRoom(ctx context.Context, roomCode string, seat int) bool {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), botBackgroundLimit)
	defer cancel()
	roomCode = canonRoom(roomCode)
	unlock := m.lockRoom(roomCode)
	defer unlock()

	game, err := m.loadGame(ctx, roomCode, true)
	if err != nil || game.Status != "active" || game.TurnSeat == nil || *game.TurnSeat != seat {
		return false
	}
	prefs, err := m.store.AutoPlay(ctx, game.ID, seat)
	if err != nil || !prefs.Enabled {
		return false
	}
	since, ok := m.autoPlaySince(game.UpdatedAt)
	if !ok || m.now().Before(since.Add(time.Duration(prefs.TimeoutSeconds)*time.Second)) {
		return false
	}
	compiled, ok := m.catalog.Get(game.GameDefID)
	if !ok {
		return false
	}
	bot, ok := m.bots.Get(autoPlayBot)
	if !ok {
		return false
	}
	players, err := m.store.ListPlayers(ctx, game.ID)
	if err != nil {
		return false
	}
	var person *store.Player
	for i := range players {
		if players[i].Seat == seat && players[i].Bot == "" {
			person = &players[i]
		}
	}
	if person == nil {
		return false
	}

	played := 0
	for i := 0; i < maxAutoPlayMoves; i++ {
		game, err = m.store.GetGame(ctx, roomCode)
		if err != nil || game.Status != "active" || game.TurnSeat == nil || *game.TurnSeat != seat {
			break
		}
		bs := state.NewBoardState()
		if err := bs.UnmarshalJSON([]byte(game.BoardState)); err != nil {
			break
		}
		legal := state.LegalMoves(compiled, bs, seat)
		if len(legal) == 0 {
			break
		}
		mv := m.botMove(ctx, bot, roomCode, compiled, bs, seat, legal, i)
		if _, err := m.applyAndRecordAs(ctx, compiled, game, *person, mv, playedForPlayer); err != nil {
			break
		}
		played++
	}
	if played == 0 {
		return false
	}
	// The bots and the passes the others asked for carry on from here.
	m.runBots(ctx, roomCode)
	return true
}
