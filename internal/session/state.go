package session

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/store"
)

// StateSnapshot is GetState's return value: a game's current (or
// historical, via atSeq) state, plus every seat's occupancy.
type StateSnapshot struct {
	RoomCode  string
	GameDefID string
	Status    string
	// BoardState is the v1 flat view: JSON object position -> top piece.
	BoardState string
	// State is the complete position document (stacks, reserves, ply).
	State      string
	TurnSeat   *int
	MoveSeq    int
	WinnerSeat *int
	EndReason  string
	// EndDetail adds specifics to EndReason (e.g. the final score).
	EndDetail string
	Seats     []SeatInfo // one entry per seat the game definition declares, filled or not

	// Lobby says the game waits for its host to start it; HostSeat is the
	// seat of the person who created it (0 when nobody human sits).
	Lobby    bool
	HostSeat int

	// MoveTimeLimit is the per-move allowance in seconds (0 = untimed).
	MoveTimeLimit int
	// Clock is the game clock, nil for a game played without one: what each
	// seat has left (the seat to move counts down from TurnDeadline).
	Clock *ClockSnapshot
	// TurnDeadline is when the player to move runs out of time; nil when
	// the game is untimed or not in progress.
	TurnDeadline *time.Time
	// ServerTime lets clients correct for clock skew when showing a
	// countdown.
	ServerTime time.Time

	// DrawOfferSeat is the seat currently offering a draw (nil if none).
	DrawOfferSeat *int

	// TakebackSeat is the seat asking to take back its last move (nil if
	// none), and TakebackToSeq the number of moves that would remain.
	TakebackSeat  *int
	TakebackToSeq int

	// StartState is the starting position document when the game began
	// from a custom position (empty for the standard start), and
	// StartTurnSeat who moved first from it.
	StartState    string
	StartTurnSeat int
}

// snapshot assembles a StateSnapshot from a game row, its players and a
// board-state document (the game's current one, or a historical one).
func (m *Manager) snapshot(compiled *catalog.CompiledGame, game store.Game, players []store.Player, stateDoc string) (StateSnapshot, error) {
	bs := state.NewBoardState()
	if err := bs.UnmarshalJSON([]byte(stateDoc)); err != nil {
		return StateSnapshot{}, fmt.Errorf("session: unmarshal board state for room %q: %w", game.RoomCode, err)
	}
	// Clients see the public view: secrets (dice generator, a committed
	// choice) are masked and the ruleset's derived view is attached.
	bs = state.Public(compiled, bs, 0)
	legacy, err := bs.LegacyJSON()
	if err != nil {
		return StateSnapshot{}, err
	}
	full, err := bs.MarshalJSON()
	if err != nil {
		return StateSnapshot{}, err
	}

	playersBySeat := make(map[int]store.Player, len(players))
	for _, p := range players {
		playersBySeat[p.Seat] = p
	}
	seats := make([]SeatInfo, 0, len(compiled.Definition.Seats))
	for _, seatDef := range compiled.Definition.Seats {
		info := SeatInfo{Seat: seatDef.Seat, Color: seatDef.Color, Label: seatDef.Label}
		if p, ok := playersBySeat[seatDef.Seat]; ok {
			info.Name = p.Name
			info.Bot = p.Bot
			info.PrincipalID = p.PrincipalID
		}
		seats = append(seats, info)
	}

	return StateSnapshot{
		RoomCode:      game.RoomCode,
		GameDefID:     game.GameDefID,
		Status:        game.Status,
		BoardState:    string(legacy),
		State:         string(full),
		TurnSeat:      game.TurnSeat,
		MoveSeq:       game.MoveSeq,
		WinnerSeat:    game.WinnerSeat,
		EndReason:     game.EndReason,
		EndDetail:     game.EndDetail,
		Seats:         seats,
		Lobby:         game.Lobby,
		HostSeat:      hostOf(players),
		MoveTimeLimit: game.MoveTimeLimit,
		Clock:         clockSnapshot(game),
		TurnDeadline:  m.deadline(game),
		ServerTime:    m.now().UTC(),
		DrawOfferSeat: game.DrawOfferSeat,

		TakebackSeat:  game.TakebackSeat,
		TakebackToSeq: game.TakebackSeq,
		StartState:    publicDoc(compiled, game.InitialState),
		StartTurnSeat: game.InitialTurnSeat,
	}, nil
}

// publicDoc is a stored position document as a spectator may see it: for
// games with secrets (a seed, a committed choice) they are masked; other
// games' documents are returned unchanged.
func publicDoc(compiled *catalog.CompiledGame, doc string) string {
	if doc == "" || !state.IsRuleset(compiled) {
		return doc
	}
	bs := state.NewBoardState()
	if err := bs.UnmarshalJSON([]byte(doc)); err != nil {
		return doc
	}
	out, err := state.Public(compiled, bs, 0).MarshalJSON()
	if err != nil {
		return doc
	}
	return string(out)
}

// parseTimestamp reads a database timestamp (RFC 3339, or SQLite's
// "YYYY-MM-DD HH:MM:SS").
func parseTimestamp(ts string) (time.Time, bool) {
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05"} {
		if t, err := time.Parse(layout, ts); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}

// ClockSnapshot is a game clock as clients draw it: one entry per seat, and
// the seat whose clock is running now (0 when none is).
type ClockSnapshot struct {
	Seats   []SeatClock `json:"seats"`
	Running int         `json:"running,omitempty"`
}

// SeatClock is one seat's clock: what it started with and what is left of it
// (not counting a turn in progress — the seat that is running counts down from
// the snapshot's TurnDeadline).
type SeatClock struct {
	Seat    int     `json:"seat"`
	Initial int     `json:"initial_seconds"`
	Left    float64 `json:"seconds_left"`
}

// clockSnapshot describes a game's clock for clients, or nil without one.
func clockSnapshot(game store.Game) *ClockSnapshot {
	c := parseGameClock(game.Clock)
	if c == nil {
		return nil
	}
	running := 0
	if game.Status == "active" && game.TurnSeat != nil {
		running = *game.TurnSeat
	}
	return c.snapshot(running)
}

// deadline returns when the player to move runs out of time, or nil: the
// earlier of the per-move allowance and what is left on their game clock (a
// game is created with one or the other, never both).
func (m *Manager) deadline(game store.Game) *time.Time {
	if game.Status != "active" || game.TurnSeat == nil {
		return nil
	}
	started, ok := parseTimestamp(game.UpdatedAt)
	if !ok {
		return nil
	}
	var out *time.Time
	if game.MoveTimeLimit > 0 {
		d := started.Add(time.Duration(game.MoveTimeLimit) * time.Second)
		out = &d
	}
	if c := parseGameClock(game.Clock); c != nil {
		d := c.startedAt(started).Add(time.Duration(c.LeftFor(*game.TurnSeat) * float64(time.Second)))
		if out == nil || d.Before(*out) {
			out = &d
		}
	}
	return out
}

// loadGame reads a game and, when its per-move clock has run out, ends it
// on time (the player to move loses) before returning it. locked says the
// caller already holds the room lock.
func (m *Manager) loadGame(ctx context.Context, roomCode string, locked bool) (store.Game, error) {
	game, err := m.store.GetGame(ctx, roomCode)
	if err != nil {
		return game, err
	}
	if d := m.deadline(game); d == nil || !m.now().After(*d) {
		return game, nil
	}
	if !locked {
		unlock := m.lockRoom(ctx, roomCode)
		defer unlock()
		if game, err = m.store.GetGame(ctx, roomCode); err != nil {
			return game, err
		}
		if d := m.deadline(game); d == nil || !m.now().After(*d) {
			return game, nil
		}
	}

	compiled, ok := m.catalog.Get(game.GameDefID)
	if !ok || game.TurnSeat == nil {
		return game, nil
	}
	var winner *int
	if opp, ok := compiled.Opponent(*game.TurnSeat); ok {
		winner = &opp
	}
	if err := m.store.FinishGame(ctx, game.ID, winner, "timeout"); err != nil {
		return game, err
	}
	m.mx.Finished(gameFamily(compiled), "timeout")
	if game, err = m.store.GetGame(ctx, roomCode); err != nil {
		return game, err
	}
	m.publishState(ctx, roomCode, "timeout")
	return game, nil
}

// GetState returns roomCode's current state, or the historical state at
// atSeq (if non-nil), along with every seat the game definition declares.
// atSeq 0 is the game's starting position. Errors from the underlying store
// (e.g. store.ErrNotFound for an unknown room code or an out-of-range atSeq)
// are propagated unwrapped so callers can errors.Is against them directly.
func (m *Manager) GetState(ctx context.Context, roomCode string, atSeq *int) (StateSnapshot, error) {
	roomCode = canonRoom(roomCode)
	if _, err := m.loadGame(ctx, roomCode, false); err != nil {
		return StateSnapshot{}, err
	}
	return m.readState(ctx, roomCode, atSeq)
}

// GetStateFor is GetState as the player called name sees it: in a game with
// secrets the player's own committed choices are visible, to the seat's owner
// (ADR 0067). An unknown or empty name, or a seat that is somebody else's, is a
// spectator.
func (m *Manager) GetStateFor(ctx context.Context, roomCode string, atSeq *int, name string) (StateSnapshot, error) {
	snap, err := m.GetState(ctx, roomCode, atSeq)
	if err != nil || name == "" {
		return snap, err
	}
	seat := 0
	for _, s := range snap.Seats {
		if s.Name != "" && strings.EqualFold(s.Name, name) && s.Bot == "" {
			seat = s.Seat
		}
	}
	if seat == 0 {
		return snap, nil
	}
	game, err := m.store.GetGame(ctx, canonRoom(roomCode))
	if err != nil {
		return snap, nil
	}
	if !m.mayViewSeat(ctx, game.ID, seat) {
		return snap, nil
	}
	compiled, ok := m.catalog.Get(game.GameDefID)
	if !ok || !state.IsRuleset(compiled) {
		return snap, nil
	}
	doc := game.BoardState
	if atSeq != nil && *atSeq != game.MoveSeq {
		if doc, err = m.stateAtSeq(ctx, compiled, game, *atSeq); err != nil {
			return snap, nil
		}
	}
	bs := state.NewBoardState()
	if err := bs.UnmarshalJSON([]byte(doc)); err != nil {
		return snap, nil
	}
	own := state.Public(compiled, bs, seat)
	if full, err := own.MarshalJSON(); err == nil {
		snap.State = string(full)
	}
	return snap, nil
}

// readState is GetState without the time-limit check.
func (m *Manager) readState(ctx context.Context, roomCode string, atSeq *int) (StateSnapshot, error) {
	game, err := m.store.GetGame(ctx, roomCode)
	if err != nil {
		return StateSnapshot{}, err
	}

	players, err := m.store.ListPlayers(ctx, game.ID)
	if err != nil {
		return StateSnapshot{}, err
	}

	compiled, err := m.compiledFor(game)
	if err != nil {
		return StateSnapshot{}, err
	}

	doc := game.BoardState
	if atSeq != nil {
		doc, err = m.stateAtSeq(ctx, compiled, game, *atSeq)
		if err != nil {
			return StateSnapshot{}, err
		}
		// A historical snapshot reports the turn and result as they were.
		hist := game
		if *atSeq != game.MoveSeq {
			hist.Status = "active"
			hist.WinnerSeat = nil
			hist.EndReason = ""
			hist.MoveSeq = *atSeq
			hist.TurnSeat = m.turnSeatAt(ctx, compiled, game, *atSeq)
		}
		game = hist
	}

	snap, err := m.snapshot(compiled, game, players, doc)
	if err == nil {
		m.identify(ctx, snap.Seats)
	}
	return snap, err
}

// stateAtSeq returns the persisted state document after move seq (0 = the
// initial position).
func (m *Manager) stateAtSeq(ctx context.Context, compiled *catalog.CompiledGame, game store.Game, seq int) (string, error) {
	if seq == 0 {
		return startDoc(compiled, game)
	}
	return m.store.GetStateAtSeq(ctx, game.ID, seq)
}

// turnSeatAt returns whose turn it was after move seq. It replays the
// turn rule on the position at that point, so skipped (passing) seats are
// accounted for.
func (m *Manager) turnSeatAt(ctx context.Context, compiled *catalog.CompiledGame, game store.Game, seq int) *int {
	if seq == 0 {
		first := firstSeatOf(compiled, game)
		return &first
	}
	moves, err := m.store.ListMoves(ctx, game.ID, seq-1)
	if err != nil || len(moves) == 0 {
		return nil
	}
	bs := state.NewBoardState()
	if err := bs.UnmarshalJSON([]byte(moves[0].ResultingState)); err != nil {
		return nil
	}
	next, err := state.TurnAfter(compiled, bs, moves[0].Seat)
	if err != nil {
		return nil
	}
	return &next
}
