package session

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/noderunner/bingle/internal/bots"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/metrics"
	"github.com/noderunner/bingle/internal/store"
)

// hintSlots limits concurrent hint computations to the number of CPUs.
var hintSlots = make(chan struct{}, runtime.NumCPU())

// ErrHintNotAllowed is returned by Hint when asking would hand a player an
// edge over another human.
var ErrHintNotAllowed = errors.New("session: hints are only available against bots or after the game")

// Hint is a bot's suggestion for a position.
type Hint struct {
	Notation string
	Steps    []state.Step
	TurnSeat int
	Bot      string
	// WinRate is the bot's estimate of the side to move winning with the
	// suggested move (0 when the bot does not estimate); Alternatives are
	// the best few moves with their estimates.
	WinRate      float64
	Estimated    bool
	Alternatives []HintMove
}

// HintMove is one candidate move of a hint.
type HintMove struct {
	Notation string
	WinRate  float64
}

// defaultHintBot picks the bot that gives the better advice for a game:
// where counting pieces on top means something — stacks, flips, jumps,
// custodian captures — alpha-beta search with that heuristic is the stronger
// bot (see docs/mcp.md); elsewhere Monte-Carlo search is.
func defaultHintBot(compiled *catalog.CompiledGame) string {
	if _, ok := state.HeuristicValue(compiled, state.InitialState(compiled), 1); ok {
		// The game values positions itself: alpha-beta over that value is
		// the strongest advice (and bounded in time).
		return "minimax"
	}
	for _, a := range compiled.Actions {
		switch a.Type {
		case catalog.ActionMoveStack, catalog.ActionJump:
			return "minimax"
		case catalog.ActionPlace:
			if a.Flips || len(a.CustodianLengths) > 0 {
				return "minimax"
			}
		}
	}
	return "mcts"
}

// Hint asks a built-in bot (see defaultHintBot) which move it would play in
// roomCode's position after atSeq (nil = current) plus the given variation
// (notation, applied in order, nothing is stored). Only a player of the game
// may ask, and only when every other seat is a bot or the game is over, so a
// hint can never be used against another person.
func (m *Manager) Hint(ctx context.Context, roomCode, name string, atSeq *int, notations []string, botID string) (Hint, error) {
	roomCode = canonRoom(roomCode)
	game, err := m.loadGame(ctx, roomCode, false)
	if err != nil {
		return Hint{}, err
	}
	players, err := m.store.ListPlayers(ctx, game.ID)
	if err != nil {
		return Hint{}, err
	}
	var me *store.Player
	for i := range players {
		if strings.EqualFold(players[i].Name, name) && players[i].Bot == "" {
			me = &players[i]
		}
	}
	if me == nil {
		return Hint{}, ErrNotAPlayer
	}
	if err := m.mayActFor(ctx, me); err != nil {
		return Hint{}, err
	}
	if game.Status != "finished" {
		if game.Status == "waiting" {
			return Hint{}, ErrGameNotActive
		}
		for _, p := range players {
			if p.Seat != me.Seat && p.Bot == "" {
				return Hint{}, ErrHintNotAllowed
			}
		}
	}
	// A finished game has no turn to hint for at its end, but earlier
	// positions (atSeq) do.
	if game.Status == "finished" && atSeq == nil {
		return Hint{}, fmt.Errorf("%w: the game is over; choose an earlier position", ErrGameNotActive)
	}

	sim, err := m.Simulate(ctx, roomCode, atSeq, notations)
	if err != nil {
		return Hint{}, err
	}
	if sim.GameEnded || sim.TurnSeat == nil || len(sim.Legal) == 0 {
		return Hint{}, fmt.Errorf("%w: there is no move to suggest here", ErrGameNotActive)
	}
	compiled, err := m.compiledFor(game)
	if err != nil {
		return Hint{}, err
	}
	if botID == "" {
		botID = defaultHintBot(compiled)
	}
	bot, ok := m.bots.Get(botID)
	if !ok {
		return Hint{}, fmt.Errorf("%w: %q", ErrUnknownBot, botID)
	}
	bs := state.NewBoardState()
	if err := bs.UnmarshalJSON([]byte(sim.State)); err != nil {
		return Hint{}, err
	}
	legal := make([]state.Move, len(sim.Legal))
	for i, lm := range sim.Legal {
		legal[i] = lm.Move
	}
	// A hint follows the game's strategy guide like a computer player does.
	legal = state.GuidedMoves(compiled, bs, *sim.TurnSeat, legal)
	// A hint is a second or so of CPU; bound how many run at once so a
	// burst of requests queues instead of starving everything else.
	queued := time.Now()
	select {
	case hintSlots <- struct{}{}:
		m.mx.SlotWait(metrics.SlotHint, time.Since(queued))
		m.mx.SlotEnter(metrics.SlotHint)
		defer func() {
			m.mx.SlotLeave(metrics.SlotHint)
			<-hintSlots
		}()
	case <-ctx.Done():
		return Hint{}, ctx.Err()
	}

	var mv state.Move
	h := Hint{TurnSeat: *sim.TurnSeat, Bot: botID}
	if ev, ok := bot.(bots.Evaluator); ok {
		e, err := ev.Evaluate(ctx, compiled, bs, *sim.TurnSeat, legal)
		if err != nil {
			e.Move = legal[0]
			e.WinRate = 0
		} else if !e.Heuristic {
			h.Estimated, h.WinRate = true, e.WinRate
			for _, c := range e.Alternatives {
				h.Alternatives = append(h.Alternatives, HintMove{Notation: state.FormatMove(compiled, c.Move), WinRate: c.WinRate})
			}
		}
		mv = e.Move
	} else {
		var err error
		searched := time.Now()
		mv, err = bot.Choose(ctx, compiled, bs, *sim.TurnSeat, legal)
		m.mx.BotSearch(bot.ID(), gameFamily(compiled), time.Since(searched))
		if err != nil {
			mv = legal[0]
		}
	}
	h.Notation = state.FormatMove(compiled, mv)
	for _, lm := range sim.Legal {
		if lm.Notation == h.Notation {
			h.Steps = lm.Steps
			break
		}
	}
	return h, nil
}
