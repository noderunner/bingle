package session

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/session/roomcode"
	"github.com/noderunner/bingle/internal/store"
)

// ErrTooManyBotMatches is returned when too many bot-versus-bot rooms are
// already being played.
var ErrTooManyBotMatches = errors.New("session: too many bot matches are running")

const (
	maxBotMatches     = 12
	minBotMatchDelay  = 150 * time.Millisecond
	maxBotMatchDelay  = 10 * time.Second
	botMatchIdleLimit = 2 * time.Minute
)

var runningBotMatches atomic.Int32

// BotMatchOptions describes a bot-versus-bot room to watch.
type BotMatchOptions struct {
	GameDefID string
	// BotA plays seat 1 and BotB seat 2 (two-seat games only).
	BotA, BotB string
	// Delay is the pause between moves (default 700 ms).
	Delay time.Duration
}

// StartBotMatch creates a room in which two built-in bots play each other at
// a watchable pace. Spectators follow it like any game; the match stops when
// it ends, after a long idle spell with nobody watching, or at the move cap.
func (m *Manager) StartBotMatch(ctx context.Context, opts BotMatchOptions) (CreateGameResult, error) {
	compiled, ok := m.catalog.Get(opts.GameDefID)
	if !ok {
		return CreateGameResult{}, fmt.Errorf("%w: %q", ErrGameDefNotFound, opts.GameDefID)
	}
	seats := compiled.Definition.Seats
	if len(seats) != 2 {
		return CreateGameResult{}, fmt.Errorf("session: bot matches need a two-player game")
	}
	botA, okA := m.bots.Get(strings.TrimSpace(opts.BotA))
	botB, okB := m.bots.Get(strings.TrimSpace(opts.BotB))
	if !okA || !okB {
		return CreateGameResult{}, fmt.Errorf("%w: %q / %q", ErrUnknownBot, opts.BotA, opts.BotB)
	}
	delay := opts.Delay
	if delay == 0 {
		delay = 700 * time.Millisecond
	}
	if delay < minBotMatchDelay {
		delay = minBotMatchDelay
	}
	if delay > maxBotMatchDelay {
		delay = maxBotMatchDelay
	}
	if runningBotMatches.Add(1) > maxBotMatches {
		runningBotMatches.Add(-1)
		return CreateGameResult{}, ErrTooManyBotMatches
	}
	started := false
	defer func() {
		if !started {
			runningBotMatches.Add(-1)
		}
	}()

	startState, firstSeat, customStart, err := resolveStart(compiled, CreateOptions{})
	if err != nil {
		return CreateGameResult{}, err
	}
	boardJSON, err := startState.MarshalJSON()
	if err != nil {
		return CreateGameResult{}, err
	}
	nameA, nameB := botA.Label(), botB.Label()
	if nameA == nameB {
		nameA, nameB = nameA+" 1", nameB+" 2"
	}

	var game store.Game
	var createErr error
	for attempt := 0; attempt < maxCreateGameAttempts; attempt++ {
		game, _, createErr = m.store.CreateGameAt(ctx, roomcode.New(), opts.GameDefID, string(boardJSON), nameA, seats[0].Color, seats[0].Seat)
		if createErr == nil {
			break
		}
	}
	if createErr != nil {
		return CreateGameResult{}, createErr
	}
	if err := m.store.SetPlayerBot(ctx, game.ID, seats[0].Seat, botA.ID()); err != nil {
		return CreateGameResult{}, err
	}
	first := firstSeat
	if _, err := m.store.AddBotPlayer(ctx, game.ID, seats[1].Seat, nameB, seats[1].Color, botB.ID(), "active", &first); err != nil {
		return CreateGameResult{}, err
	}

	if customStart {
		if err := m.store.SetInitialPosition(ctx, game.ID, string(boardJSON), firstSeat); err != nil {
			return CreateGameResult{}, err
		}
	}

	started = true
	go func() {
		defer runningBotMatches.Add(-1)
		m.autoplay(game.RoomCode, delay)
	}()
	return CreateGameResult{RoomCode: game.RoomCode, Seat: 0, Status: "active"}, nil
}

// autoplay plays roomCode's bots against each other, one move per delay.
func (m *Manager) autoplay(roomCode string, delay time.Duration) {
	ctx := context.Background()
	var unwatchedSince time.Time
	for i := 0; i < 2*maxBotPlies; i++ {
		time.Sleep(delay)

		if m.hub.count(roomCode) == 0 {
			if unwatchedSince.IsZero() {
				unwatchedSince = time.Now()
			} else if time.Since(unwatchedSince) > botMatchIdleLimit {
				return
			}
		} else {
			unwatchedSince = time.Time{}
		}

		unlock := m.lockRoom(ctx, roomCode)
		done := m.autoplayOne(ctx, roomCode)
		unlock()
		if done {
			return
		}
	}
}

// autoplayOne plays the bot move that is due in roomCode; it reports true
// when the match is over (or cannot continue).
func (m *Manager) autoplayOne(ctx context.Context, roomCode string) bool {
	ctx, cancel := context.WithTimeout(ctx, botMoveTimeout)
	defer cancel()

	game, err := m.store.GetGame(ctx, roomCode)
	if err != nil || game.Status != "active" || game.TurnSeat == nil {
		return true
	}
	players, err := m.store.ListPlayers(ctx, game.ID)
	if err != nil {
		return true
	}
	var mover *store.Player
	for i := range players {
		if players[i].Seat == *game.TurnSeat {
			mover = &players[i]
		}
	}
	if mover == nil || mover.Bot == "" {
		return true
	}
	bot, ok := m.bots.Get(mover.Bot)
	if !ok {
		return true
	}
	compiled, ok := m.catalog.Get(game.GameDefID)
	if !ok {
		return true
	}
	bs := state.NewBoardState()
	if err := bs.UnmarshalJSON([]byte(game.BoardState)); err != nil {
		return true
	}
	legal := state.LegalMoves(compiled, bs, mover.Seat)
	if len(legal) == 0 {
		return true
	}
	legal = m.botChoices(roomCode, compiled, bs, mover.Seat, legal)
	var mv state.Move
	select {
	case botSlots <- struct{}{}:
		mv, err = bot.Choose(ctx, compiled, bs, mover.Seat, legal)
		<-botSlots
	case <-ctx.Done():
		err = ctx.Err()
	}
	if err != nil {
		mv = legal[len(legal)-1]
	}
	outcome, err := m.applyAndRecord(ctx, compiled, game, *mover, mv)
	if err != nil {
		return true
	}
	return outcome.GameEnded
}
