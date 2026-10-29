package session

import (
	"context"
	"runtime"
	"time"

	"github.com/noderunner/bingle/internal/bots"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/metrics"
	"github.com/noderunner/bingle/internal/store"
)

// maxBotPlies bounds how many consecutive bot moves one request may
// trigger (guards against two bots playing a never-ending game inside a
// single call).
const maxBotPlies = 1000

// botSlots bounds concurrent bot searches to twice the CPU count.
var botSlots = make(chan struct{}, 2*runtime.NumCPU())

// botMoveTimeout bounds how long bots may think for the replies triggered
// by a single human move.
const botMoveTimeout = 2 * time.Minute

// botSoftLimit is how long a request waits for the bots' replies before it
// answers: quick bots (random, tactical) reply within it, so clients get
// their moves in the response; a slow search, or a table of several bots, is
// carried on in the background and reaches clients as move events.
const botSoftLimit = 1500 * time.Millisecond

// botThinkLimit caps one bot decision: a search that is out of time answers
// with the best move found so far (the searching bots deepen ply by ply).
const botThinkLimit = 1500 * time.Millisecond

// botBackgroundLimit bounds one background run of bot play.
const botBackgroundLimit = 30 * time.Minute

// runBots lets bot-held seats play for as long as it is a bot's turn (up to
// botSoftLimit), and returns the moves played; the rest carries on in the
// background. The caller holds the room lock. It is detached from ctx's
// cancellation so a disconnecting client can't leave a bot's reply
// half-played.
func (m *Manager) runBots(ctx context.Context, roomCode string) []MoveOutcome {
	roomCode = canonRoom(roomCode)
	played, more := m.runBotsFor(ctx, roomCode, botSoftLimit, maxBotPlies)
	if more {
		m.continueBots(roomCode)
	}
	return played
}

// continueBots plays the remaining bot moves of a room in the background,
// one at a time under the room lock, so people can still read, undo or
// resign while the bots think. At most one such run exists per room.
func (m *Manager) continueBots(roomCode string) {
	m.mu.Lock()
	if m.botsBusy == nil {
		m.botsBusy = map[string]bool{}
	}
	if m.botsBusy[roomCode] {
		m.mu.Unlock()
		return
	}
	m.botsBusy[roomCode] = true
	m.mu.Unlock()
	go func() {
		defer func() {
			m.mu.Lock()
			delete(m.botsBusy, roomCode)
			m.mu.Unlock()
		}()
		deadline := time.Now().Add(botBackgroundLimit)
		for time.Now().Before(deadline) {
			// A background run belongs to no request, so nothing is
			// collecting and the span costs a context lookup.
			unlock := m.lockRoom(context.Background(), roomCode)
			_, more := m.runBotsFor(context.Background(), roomCode, 0, 1)
			unlock()
			if !more {
				return
			}
		}
	}()
}

// runBotsFor plays up to maxMoves bot moves, stopping after soft (when
// positive) has elapsed. more reports that it stopped for one of those
// reasons and a bot may still be due; false means it is a person's turn, or
// the game is over.
func (m *Manager) runBotsFor(ctx context.Context, roomCode string, soft time.Duration, maxMoves int) (played []MoveOutcome, more bool) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), botMoveTimeout)
	defer cancel()
	start := time.Now()
	for i := 0; i < maxMoves; i++ {
		if soft > 0 && i > 0 && time.Since(start) > soft {
			return played, true
		}
		game, err := m.store.GetGame(ctx, roomCode)
		if err != nil || game.Status != "active" || game.TurnSeat == nil {
			if err == nil && game.Status == "finished" {
				m.shuffles.forget(roomCode)
			}
			return played, false
		}
		players, err := m.store.ListPlayers(ctx, game.ID)
		if err != nil {
			return played, false
		}
		var seatPlayer *store.Player
		for idx := range players {
			if players[idx].Seat == *game.TurnSeat {
				seatPlayer = &players[idx]
			}
		}
		if seatPlayer == nil {
			return played, false
		}
		compiled, ok := m.catalog.Get(game.GameDefID)
		if !ok {
			return played, false
		}
		bs := state.NewBoardState()
		if err := bs.UnmarshalJSON([]byte(game.BoardState)); err != nil {
			return played, false
		}
		if seatPlayer.Bot == "" {
			// A person's decision: unless it is a reaction they chose to pass
			// automatically, the game waits for them.
			mv, pass := m.autoPass(ctx, compiled, game, bs, *seatPlayer)
			if !pass {
				return played, false
			}
			outcome, err := m.applyAndRecordAs(ctx, compiled, game, *seatPlayer, mv, passedForPlayer)
			if err != nil {
				return played, false
			}
			played = append(played, outcome)
			continue
		}
		botPlayer := seatPlayer
		bot, ok := m.bots.Get(botPlayer.Bot)
		if !ok {
			return played, false
		}
		legal := state.LegalMoves(compiled, bs, botPlayer.Seat)
		if len(legal) == 0 {
			return played, false
		}
		mv := m.botMove(ctx, bot, roomCode, compiled, bs, botPlayer.Seat, legal, i)
		outcome, err := m.applyAndRecord(ctx, compiled, game, *botPlayer, mv)
		if err != nil {
			return played, false
		}
		played = append(played, outcome)
	}
	return played, true
}

// botMove is the decision bot makes for seat among its legal moves. i varies
// the fallback when the bot cannot answer in time.
func (m *Manager) botMove(ctx context.Context, bot bots.Bot, roomCode string, compiled *catalog.CompiledGame, bs *state.BoardState, seat int, legal []state.Move, i int) state.Move {
	legal = m.botChoices(roomCode, compiled, bs, seat, legal)
	// Thinking is CPU; bound how many bots think at once.
	var mv state.Move
	var err error
	queued := time.Now()
	select {
	case botSlots <- struct{}{}:
		// Recorded inside the case, so it is the wait for a slot and not the
		// thinking that follows it: queue wait is what grows first as load
		// grows, because the pool stays full and each search takes as long
		// as it always did.
		m.mx.SlotWait(metrics.SlotBot, time.Since(queued))
		m.mx.SlotEnter(metrics.SlotBot)
		think, stop := context.WithTimeout(ctx, botThinkLimit)
		searched := time.Now()
		mv, err = bot.Choose(think, compiled, bs, seat, legal)
		m.mx.BotSearch(bot.ID(), gameFamily(compiled), time.Since(searched))
		stop()
		m.mx.SlotLeave(metrics.SlotBot)
		<-botSlots
	case <-ctx.Done():
		err = ctx.Err()
	}
	if err != nil {
		// Fall back to a move rather than stalling: not the first
		// (often a free action that changes nothing), but a varied one.
		mv = legal[(len(legal)-1+i*7)%len(legal)]
	}
	return mv
}
