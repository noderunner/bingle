package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/metrics"
	"github.com/noderunner/bingle/internal/store"
	"github.com/noderunner/bingle/internal/trace"
)

// ErrGameNotActive is returned by MakeMove when roomCode's game isn't
// currently "active" (either still "waiting" for seats to fill, or already
// "finished").
var ErrGameNotActive = errors.New("session: game is not active")

// ErrRulesetPanic is returned when a ruleset panicked while applying a move.
// It is the server's fault and not the client's, so internal/api maps it to a
// 500 rather than to the 400 an illegal move gets.
var ErrRulesetPanic = errors.New("session: the game's rules panicked")

// ErrNotYourImpulse is returned by MakeMove when name doesn't hold the seat
// whose turn it currently is — whether because name holds no seat in this
// game at all, or because it's the other seat's turn.
var ErrNotYourImpulse = errors.New("session: not your turn")

// ErrNotAPlayer is returned by operations that need a seated player (undo,
// resign) when name holds no seat in the game.
var ErrNotAPlayer = errors.New("session: not a player in this game")

// MoveOutcome is MakeMove's return value: the persisted move itself, plus
// the game's resulting turn/status/board state — sparing callers (REST/WS) a
// second GetState round-trip.
type MoveOutcome struct {
	Seq int
	// Ply is the impulse the move (the last selection recorded) is part of,
	// and Plies the impulses the game has played after it.
	Ply      int
	Plies    int
	Seat     int
	Color    string
	From     string
	To       string
	Captured *string

	Action   string
	Piece    string
	Count    int
	Path     []string
	Drops    []int
	Aim      string
	Notation string

	NextImpulseSeat int
	GameEnded       bool
	WinnerSeat      *int
	EndReason       string
	EndDetail       string
	Status          string // "active" or "finished" after this move
	BoardState      string // the resulting v1 flat board state JSON
	State           string // the resulting full state document JSON
	// Auto says the server played this move for a person: a reaction they
	// chose to decline automatically. AutoPlay says a tactical bot played it
	// for a person whose Auto-Play Timeout ran out.
	Auto     bool
	AutoPlay bool
	// Locked says the move rolled dice: nothing can be taken back across it.
	Locked bool

	// BotMoves are the replies bots played immediately afterwards.
	BotMoves []MoveOutcome
}

// MakeMove applies a single legacy step move by name (who must hold the seat
// whose turn it currently is) from `from` to `to`; see Play.
func (m *Manager) MakeMove(ctx context.Context, roomCode, name string, from, to board.PositionID) (MoveOutcome, error) {
	return m.Play(ctx, roomCode, name, state.Selection{From: from, To: to})
}

// PlayNotation parses text in the game's move notation and plays it; see
// PlayNotationAt.
func (m *Manager) PlayNotation(ctx context.Context, roomCode, name, text string) (MoveOutcome, error) {
	return m.PlayNotationAt(ctx, roomCode, name, text, nil)
}

// PlayNotationAt plays text: one selection, or a whole impulse of several joined
// by "; " (ADR 0114), which is applied all or nothing and must end the impulse.
// With ply, it is refused with ErrStalePly unless the game is still at that ply.
// A single selection may still leave the impulse open, as the page sends them
// one at a time until ADR 0114's first phase is complete.
func (m *Manager) PlayNotationAt(ctx context.Context, roomCode, name, text string, ply *int) (MoveOutcome, error) {
	roomCode = canonRoom(roomCode)
	game, err := m.loadGame(ctx, roomCode, false)
	if err != nil {
		return MoveOutcome{}, err
	}
	compiled, err := m.compiledFor(game)
	if err != nil {
		return MoveOutcome{}, err
	}
	sels, err := state.ParseImpulse(compiled, text)
	if err != nil {
		return MoveOutcome{}, err
	}
	return m.playSelections(ctx, roomCode, name, sels, ply, len(sels) > 1)
}

// Play applies a move by name (who must hold the seat whose turn it
// currently is), persists it, publishes it to the room, lets any bot
// opponents reply, and returns the resulting MoveOutcome. See
// docs/history/plan.md section 6.4d for the original contract:
//
//   - The game must be "active" (ErrGameNotActive otherwise).
//   - name must match an existing player whose seat is the game's current
//     impulse_seat (ErrNotYourImpulse otherwise — covering both "name holds no
//     seat here" and "it's the other seat's turn").
//   - The move itself must be legal per state.Apply; an illegal move's
//     error is returned as-is, with nothing persisted.
func (m *Manager) Play(ctx context.Context, roomCode, name string, mv state.Selection) (MoveOutcome, error) {
	return m.PlayAt(ctx, roomCode, name, mv, nil)
}

// PlayAt is Play, refused with ErrStalePly unless the game is still at ply.
func (m *Manager) PlayAt(ctx context.Context, roomCode, name string, mv state.Selection, ply *int) (MoveOutcome, error) {
	return m.playSelections(ctx, roomCode, name, []state.Selection{mv}, ply, false)
}

// playSelections plays sels for name under the room lock. whole says sels are an
// impulse, checked as one with state.ApplyImpulse before anything is recorded;
// each selection is then recorded as a move of its own, as stored games still
// are until ADR 0114's first phase regroups them.
func (m *Manager) playSelections(ctx context.Context, roomCode, name string, sels []state.Selection, ply *int, whole bool) (MoveOutcome, error) {
	roomCode = canonRoom(roomCode)
	unlock := m.lockRoom(ctx, roomCode)
	defer unlock()

	game, err := m.loadGame(ctx, roomCode, true)
	if err != nil {
		return MoveOutcome{}, err
	}

	if game.Status != "active" {
		return MoveOutcome{}, ErrGameNotActive
	}

	players, err := m.store.ListPlayers(ctx, game.ID)
	if err != nil {
		return MoveOutcome{}, err
	}

	var player store.Player
	var found bool
	for _, p := range players {
		if strings.EqualFold(p.Name, name) {
			player = p
			found = true
			break
		}
	}
	if !found || player.Bot != "" || game.ImpulseSeat == nil || player.Seat != *game.ImpulseSeat {
		return MoveOutcome{}, ErrNotYourImpulse
	}
	if err := m.mayActFor(ctx, &player); err != nil {
		return MoveOutcome{}, err
	}

	compiled, err := m.compiledFor(game)
	if err != nil {
		return MoveOutcome{}, err
	}

	if ply != nil && *ply != game.Ply {
		return MoveOutcome{}, fmt.Errorf("%w: it is at ply %d, not %d", ErrStalePly, game.Ply, *ply)
	}
	if whole {
		bs := state.NewBoardState()
		if err := bs.UnmarshalJSON([]byte(game.BoardState)); err != nil {
			return MoveOutcome{}, fmt.Errorf("session: unmarshal board state for room %q: %w", game.RoomCode, err)
		}
		if _, err := state.ApplyImpulse(compiled, bs, player.Seat, sels); err != nil {
			return MoveOutcome{}, err
		}
	}

	var outcome MoveOutcome
	var notations []string
	for i, mv := range sels {
		if i > 0 {
			if game, err = m.loadGame(ctx, roomCode, true); err != nil {
				return MoveOutcome{}, err
			}
		}
		if outcome, err = m.applyAndRecord(ctx, compiled, game, player, mv); err != nil {
			return MoveOutcome{}, err
		}
		notations = append(notations, outcome.Notation)
	}
	// The move is the whole impulse: its notation is all of its selections'.
	outcome.Notation = strings.Join(notations, state.SelectionSeparator)

	outcome.BotMoves = m.runBots(ctx, roomCode)
	return outcome, nil
}

// applyAndRecord validates and applies mv for player against game's current
// state, persists the result and publishes the "move" event. The caller
// holds the room lock and has already checked turn ownership.
func (m *Manager) applyAndRecord(ctx context.Context, compiled *catalog.CompiledGame, game store.Game, player store.Player, mv state.Selection) (MoveOutcome, error) {
	return m.applyAndRecordAs(ctx, compiled, game, player, mv, madeByPlayer)
}

// moveHow says how a move came about when the seat's player did not make it
// themselves.
type moveHow int

const (
	madeByPlayer    moveHow = iota
	passedForPlayer         // a reaction declined for them (the server)
	playedForPlayer         // played by a tactical bot when their Auto-Play Timeout ran out
)

// applyAndRecordAs is applyAndRecord; how marks a move the server played on
// the player's behalf: a reaction passed for them, or a turn a bot took after
// their Auto-Play Timeout.
func (m *Manager) applyAndRecordAs(ctx context.Context, compiled *catalog.CompiledGame, game store.Game, player store.Player, mv state.Selection, how moveHow) (MoveOutcome, error) {
	ctx, funnel := trace.Start(ctx, "session.ApplyMove")
	funnel.Set("game", gameFamily(compiled))
	defer funnel.End()

	auto, autoPlay := how == passedForPlayer, how == playedForPlayer
	bs := state.NewBoardState()
	if err := bs.UnmarshalJSON([]byte(game.BoardState)); err != nil {
		return MoveOutcome{}, fmt.Errorf("session: unmarshal board state for room %q: %w", game.RoomCode, err)
	}

	// Apply takes no context and does no I/O (ADR 0025): the span goes
	// around the call, never inside the ruleset.
	family := gameFamily(compiled)
	_, applySpan := trace.Start(ctx, "engine.Apply")
	applyStart := time.Now()
	result, err := m.applyWatched(compiled, bs, player.Seat, mv)
	applyTook := time.Since(applyStart)
	applySpan.End()
	if err != nil {
		// A refusal here is usually the ruleset's answer and not a fault: the
		// client offered a move the rules do not allow. A spike in these
		// means a rules change or a stale client, which is why they are
		// counted apart from everything else.
		m.mx.Move(family, metrics.MoveIllegal, applyTook)
		m.logMove(ctx, compiled, game, player, mv, moveLine{err: err, dur: applyTook})
		// Unless Legal listed it, in which case the engine has contradicted
		// itself and that is worth a bundle. Running Legal again costs
		// something, so it only happens on this path and only when bundles
		// are switched on at all.
		m.reproRefusal(ctx, compiled, game, bs, player.Seat, mv, err)
		return MoveOutcome{}, err
	}

	newStateJSON, err := result.NewState.MarshalJSON()
	if err != nil {
		return MoveOutcome{}, fmt.Errorf("session: marshal resulting board state: %w", err)
	}
	// A move that rolled dice (or drew a hidden token) locks the game in: it
	// can be taken back no more, and the page knows to stop offering it.
	locked := state.Guarded(compiled) && !state.CanTakeBack(compiled, bs, result.NewState)
	// What the same seat decides straight after a roll — the unit Desecrate
	// places, the results it assigns — is part of it: nobody takes back a
	// choice made on seeing the die.
	if state.Guarded(compiled) && !locked && game.MoveSeq > 0 {
		if prev, err := m.store.ListMoves(ctx, game.ID, game.MoveSeq-1); err == nil && len(prev) == 1 && prev[0].Seat == player.Seat && prev[0].Locked {
			locked = true
		}
	}
	// Whether the selection ends its impulse, and so the ply (ADR 0114). In a
	// game for one seat each selection is sent as a move of its own, and a
	// reaction the session declined for the seat is that seat's whole impulse
	// (the dummy submits it), even where the seat's own turn comes next.
	endsImpulse := auto || state.AnySelectionMayEnd(compiled) || state.ImpulseEnds(compiled, bs, player.Seat, result.Move, result)
	plies := game.Ply
	if endsImpulse {
		plies++
	}
	// Only a game for one seat takes moves back, so only there is a lock news.
	shownLocked := locked && state.AnySelectionMayEnd(compiled)
	// What leaves the server is the public view of the position.
	pub := state.Public(compiled, result.NewState, 0)
	publicJSON := newStateJSON
	if pub != result.NewState {
		publicJSON, err = pub.MarshalJSON()
		if err != nil {
			return MoveOutcome{}, fmt.Errorf("session: marshal resulting board state: %w", err)
		}
	}
	legacyJSON, err := pub.LegacyJSON()
	if err != nil {
		return MoveOutcome{}, fmt.Errorf("session: marshal resulting board state: %w", err)
	}
	moveJSON, err := json.Marshal(result.Move)
	if err != nil {
		return MoveOutcome{}, fmt.Errorf("session: marshal move: %w", err)
	}

	// The facts the rules report go with the selection (ADR 0129). A fact the
	// game does not declare is a bug in its rules: it is dropped and said,
	// and the move goes on.
	facts, undeclared := state.DeclaredFacts(compiled, result.Facts)
	if len(undeclared) > 0 {
		m.mx.FactsUndeclared(family, len(undeclared))
		m.logger().Error("the rules reported facts the game does not declare", "game", compiled.Definition.ID, "facts", undeclared)
	}
	var factsJSON []byte
	if len(facts) > 0 {
		if factsJSON, err = json.Marshal(facts); err != nil {
			return MoveOutcome{}, fmt.Errorf("session: marshal facts: %w", err)
		}
	}

	var capturedStr *string
	if result.Captured != nil {
		v := string(*result.Captured)
		capturedStr = &v
	}

	// The pie rule (ADR 0056): a swap leaves the board untouched and changes
	// who sits in which seat — recorded with the move, so the two can never
	// disagree.
	pieSwap := isPieSwap(compiled, result.Move)

	// The clock of the seat to move has been running since the game row was
	// last touched (its updated_at, which every turn change bumps): charge it
	// in the transaction that records the move, so the two cannot disagree.
	var clockJSON *string
	charged := parseGameClock(game.Clock)
	if charged != nil {
		if started, ok := parseTimestamp(game.UpdatedAt); ok {
			charged.charge(player.Seat, m.now().Sub(charged.startedAt(m.turnStart(started))))
		}
		if earnsIncrement(how, result.GameEnded, pieSwap, player.Seat, result.NextImpulseSeat) {
			charged.gain(player.Seat)
		}
		if pieSwap {
			// Each player's time moves with them to the other seat.
			charged.swapSeats()
		}
		charged.start(m.now())
		stored, err := charged.marshal()
		if err != nil {
			return MoveOutcome{}, err
		}
		clockJSON = &stored
	}

	recorded, err := m.store.RecordMove(ctx, game.ID, store.MoveInput{
		Seat:                player.Seat,
		Color:               player.Color,
		From:                string(result.Move.From),
		To:                  string(result.Move.To),
		Captured:            capturedStr,
		ResultingBoardState: string(newStateJSON),
		NextImpulseSeat:     result.NextImpulseSeat,
		GameEnded:           result.GameEnded,
		WinnerSeat:          result.WinnerSeat,
		EndReason:           result.EndReason,
		EndDetail:           result.EndDetail,
		Notation:            result.Notation,
		MoveJSON:            string(moveJSON),
		Auto:                auto,
		AutoPlay:            autoPlay,
		Locked:              locked,
		Clock:               clockJSON,
		SwapSeats:           pieSwap,
		EndsImpulse:         endsImpulse,
		Facts:               string(factsJSON),
	})
	if err != nil {
		m.mx.Move(family, metrics.MoveFailed, applyTook)
		return MoveOutcome{}, err
	}
	move := recorded
	if pieSwap {
		m.seatsSwapped(canonRoom(game.RoomCode), 1, 2)
	}

	finalStatus := "active"
	if result.GameEnded {
		finalStatus = "finished"
	}

	path := make([]string, len(result.Move.Path))
	for i, p := range result.Move.Path {
		path[i] = string(p)
	}

	var eventImpulseSeat *int
	var eventDeadline string
	var eventClock *ClockSnapshot
	if !result.GameEnded {
		eventImpulseSeat = &result.NextImpulseSeat
		deadline := time.Time{}
		if game.MoveTimeLimit > 0 {
			deadline = m.now().UTC().Add(time.Duration(game.MoveTimeLimit) * time.Second)
		}
		if charged != nil {
			// The next player's clock starts running now.
			d := m.now().UTC().Add(time.Duration(charged.LeftFor(result.NextImpulseSeat) * float64(time.Second)))
			if deadline.IsZero() || d.Before(deadline) {
				deadline = d
			}
		}
		if !deadline.IsZero() {
			eventDeadline = deadline.Format(time.RFC3339)
		}
	}
	if charged != nil {
		eventClock = charged.snapshot(result.NextImpulseSeat)
		if result.GameEnded {
			eventClock.Running = 0
		}
	}
	// A move that commits a secret (a strike's dice) is announced without it.
	shownMove, shownNotation := state.PublicMove(compiled, result.NewState, result.Move, result.Notation)
	_, publishSpan := trace.Start(ctx, "ws.publish")
	m.hub.publish(game.RoomCode, Event{Type: "move", Data: moveEventData{
		Seq:         move.Seq,
		Seat:        player.Seat,
		Color:       player.Color,
		From:        string(shownMove.From),
		To:          string(shownMove.To),
		Captured:    capturedStr,
		Action:      shownMove.Action,
		Piece:       shownMove.Piece,
		Count:       shownMove.Count,
		Path:        path,
		Drops:       shownMove.Drops,
		Aim:         string(shownMove.Aim),
		Notation:    shownNotation,
		ImpulseSeat: eventImpulseSeat,
		GameEnded:   result.GameEnded,
		WinnerSeat:  result.WinnerSeat,
		EndReason:   result.EndReason,
		EndDetail:   result.EndDetail,
		Status:      finalStatus,
		BoardState:  json.RawMessage(legacyJSON),
		State:       json.RawMessage(publicJSON),

		ImpulseDeadline: eventDeadline,
		Clock:           eventClock,
		ServerTime:      m.now().UTC().Format(time.RFC3339),
		Auto:            auto,
		AutoPlay:        autoPlay,
		Locked:          shownLocked,
		Ply:             move.Ply,
		EndsImpulse:     endsImpulse,
	}})
	publishSpan.End()

	if pieSwap {
		// The seats changed hands: a state event makes every client re-read
		// who sits where (and which seat is theirs).
		m.publishState(ctx, game.RoomCode, "swap")
	}

	m.mx.Move(family, metrics.MoveApplied, applyTook)
	if result.GameEnded {
		m.finished(game, family, result.EndReason)
	}
	m.logMove(ctx, compiled, game, player, result.Move, moveLine{
		seq: move.Seq, notation: result.Notation, dur: applyTook, locked: locked,
		ended: result.GameEnded, endReason: result.EndReason, state: result.NewState,
	})
	if m.debug.Paranoid {
		m.checkParanoid(ctx, compiled, game, bs, player.Seat, result, move.Seq)
	}

	return MoveOutcome{
		Seq:             move.Seq,
		Seat:            player.Seat,
		Color:           player.Color,
		From:            string(result.Move.From),
		To:              string(result.Move.To),
		Captured:        capturedStr,
		Action:          shownMove.Action,
		Piece:           shownMove.Piece,
		Count:           shownMove.Count,
		Path:            path,
		Drops:           shownMove.Drops,
		Aim:             string(shownMove.Aim),
		Notation:        shownNotation,
		NextImpulseSeat: result.NextImpulseSeat,
		GameEnded:       result.GameEnded,
		WinnerSeat:      result.WinnerSeat,
		EndReason:       result.EndReason,
		EndDetail:       result.EndDetail,
		Status:          finalStatus,
		BoardState:      string(legacyJSON),
		State:           string(publicJSON),
		Auto:            auto,
		AutoPlay:        autoPlay,
		Locked:          shownLocked,
		Ply:             move.Ply,
		Plies:           plies,
	}, nil
}

// isPieSwap reports whether mv is the pie-rule swap of a game played with the
// rule. Then the move is also a change of who sits in which seat: the board
// is untouched, the turn stays with the mover's seat (the engine gives it
// back), and the seats' occupants exchange.
func isPieSwap(compiled *catalog.CompiledGame, mv state.Selection) bool {
	return state.IsPieSwap(compiled, mv)
}

// swapSeqOf is the seq of the game's pie-rule swap, 0 when the game has none.
// The seats' occupants exchange at the swap, so a move at or before it was
// made by whoever now holds the other seat.
func swapSeqOf(compiled *catalog.CompiledGame, moves []store.Move) int {
	if !catalog.PieRuleOn(compiled.Definition) {
		return 0
	}
	for _, mv := range moves {
		if mv.Notation == string(catalog.ActionSwap) {
			return mv.Seq
		}
	}
	return 0
}

// moverSeatNow is the seat whose present occupant made mv: the seat that made
// it, or the other one when mv is the pie-rule swap or predates it.
func moverSeatNow(mv store.Move, swapSeq int) int {
	if swapSeq > 0 && mv.Seq <= swapSeq {
		return 3 - mv.Seat
	}
	return mv.Seat
}

// publishState broadcasts the room's current state to every subscriber as a
// "state" event; reason says what changed ("join", "undo", "resign").
func (m *Manager) publishState(ctx context.Context, roomCode, reason string) {
	snap, err := m.readState(ctx, roomCode, nil)
	if err != nil {
		return
	}
	m.hub.publish(roomCode, Event{Type: "state", Data: stateEventFrom(snap, reason, nil)})
}

func (m *Manager) publishUndo(ctx context.Context, roomCode string, removed []int) {
	snap, err := m.readState(ctx, roomCode, nil)
	if err != nil {
		return
	}
	m.hub.publish(roomCode, Event{Type: "state", Data: stateEventFrom(snap, "undo", removed)})
}
