package session

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/store"
)

// MoveRecord is one entry of ListMoves' return value, matching the REST
// table's GET /api/games/{code}/moves array entry shape.
type MoveRecord struct {
	Seq      int
	Seat     int
	Color    string
	From     string
	To       string
	Captured *string

	Action    string
	Piece     string
	Count     int
	Path      []string
	Drops     []int
	Aim       string
	Notation  string
	CreatedAt string
	// Auto: the server played it for the seat (an auto-passed reaction).
	// AutoPlay: a tactical bot played it for the seat's person after their
	// Auto-Play Timeout ran out.
	Auto     bool
	AutoPlay bool
	// Locked: the move rolled dice (or drew a hidden token) — it, and
	// everything before it, can no longer be taken back.
	Locked bool
}

// ListMoves returns every move recorded for roomCode with seq > sinceSeq, in
// ascending seq order. Errors from the underlying store (e.g.
// store.ErrNotFound for an unknown room code) are propagated unwrapped, as
// GetState does. Always returns a non-nil (possibly empty) slice, matching
// store.ListMoves' convention.
func (m *Manager) ListMoves(ctx context.Context, roomCode string, sinceSeq int) ([]MoveRecord, error) {
	roomCode = canonRoom(roomCode)
	game, err := m.loadGame(ctx, roomCode, false)
	if err != nil {
		return nil, err
	}

	moves, err := m.store.ListMoves(ctx, game.ID, sinceSeq)
	if err != nil {
		return nil, err
	}

	// Games with secrets hide what the latest, unresolved move committed.
	var current *state.BoardState
	if compiled, ok := m.catalog.Get(game.GameDefID); ok && state.IsRuleset(compiled) && game.Status == "active" {
		bs := state.NewBoardState()
		if err := bs.UnmarshalJSON([]byte(game.BoardState)); err == nil {
			current = bs
		}
	}
	records := make([]MoveRecord, 0, len(moves))
	for _, mv := range moves {
		rec := MoveRecord{
			Seq:       mv.Seq,
			Seat:      mv.Seat,
			Color:     mv.Color,
			From:      mv.From,
			To:        mv.To,
			Captured:  mv.Captured,
			Notation:  mv.Notation,
			CreatedAt: mv.CreatedAt,
			Auto:      mv.Auto,
			AutoPlay:  mv.AutoPlay,
			Locked:    mv.Locked,
		}
		if mv.MoveJSON != "" {
			var sm state.Move
			if err := json.Unmarshal([]byte(mv.MoveJSON), &sm); err == nil {
				rec.Action, rec.Piece, rec.Count, rec.Drops, rec.Aim = sm.Action, sm.Piece, sm.Count, sm.Drops, string(sm.Aim)
				for _, p := range sm.Path {
					rec.Path = append(rec.Path, string(p))
				}
				if current != nil && mv.Seq == game.MoveSeq {
					compiled, _ := m.catalog.Get(game.GameDefID)
					hidden, note := state.PublicMove(compiled, current, sm, mv.Notation)
					rec.Count, rec.Notation = hidden.Count, note
				}
			}
		}
		records = append(records, rec)
	}

	return records, nil
}

// ListLegalMoves returns every position `from` can legally move to right
// now, per docs/history/plan.md section 6.8: if `from` has no occupant (including
// when `from` isn't even a valid position on the board — that naturally has
// no occupant either), it's not an error — this is an AI convenience query,
// not a move attempt — so an empty slice is returned. Otherwise it returns
// the destinations of every legal move of the stack at `from` (for the
// seat that owns its top piece). Errors from the underlying store (e.g.
// store.ErrNotFound for an unknown room code) are propagated unwrapped, as
// GetState does. Always returns a non-nil (possibly empty) slice, matching
// ListMoves' convention.
func (m *Manager) ListLegalMoves(ctx context.Context, roomCode string, from board.PositionID) ([]board.PositionID, error) {
	roomCode = canonRoom(roomCode)
	game, err := m.loadGame(ctx, roomCode, false)
	if err != nil {
		return nil, err
	}

	compiled, ok := m.catalog.Get(game.GameDefID)
	if !ok {
		return nil, fmt.Errorf("session: game definition %q not found for room %q", game.GameDefID, roomCode)
	}

	bs := state.NewBoardState()
	if err := bs.UnmarshalJSON([]byte(game.BoardState)); err != nil {
		return nil, fmt.Errorf("session: unmarshal board state for room %q: %w", roomCode, err)
	}

	occupant, ok := bs.Get(from)
	if !ok {
		return []board.PositionID{}, nil
	}
	if _, known := compiled.Pieces[occupant.Piece]; !known {
		return nil, fmt.Errorf("session: piece %q not found in game definition %q", occupant.Piece, game.GameDefID)
	}
	seat, ok := compiled.SeatOfColor(occupant.Color)
	if !ok {
		return []board.PositionID{}, nil
	}

	legal := []board.PositionID{}
	seen := map[board.PositionID]bool{}
	for _, mv := range state.LegalMoves(compiled, bs, seat) {
		if mv.From != from || seen[mv.To] {
			continue
		}
		seen[mv.To] = true
		legal = append(legal, mv.To)
	}

	return legal, nil
}

// LegalMove is one legal move in the full listing: the structured move, its
// canonical notation, and the interactive selection steps that build it.
type LegalMove struct {
	Move     state.Move
	Notation string
	Steps    []state.Step
}

// LegalMoves lists every legal move for seat in roomCode's current position
// (seat 0 means whoever is to move). The game must be active. Moves are in
// the engine's deterministic order.
func (m *Manager) LegalMoves(ctx context.Context, roomCode string, seat int) (int, []LegalMove, error) {
	roomCode = canonRoom(roomCode)
	game, err := m.loadGame(ctx, roomCode, false)
	if err != nil {
		return 0, nil, err
	}
	if game.Status != "active" || game.TurnSeat == nil {
		return 0, nil, ErrGameNotActive
	}
	compiled, ok := m.catalog.Get(game.GameDefID)
	if !ok {
		return 0, nil, fmt.Errorf("session: game definition %q not found for room %q", game.GameDefID, roomCode)
	}
	if seat == 0 {
		seat = *game.TurnSeat
	}
	bs := state.NewBoardState()
	if err := bs.UnmarshalJSON([]byte(game.BoardState)); err != nil {
		return 0, nil, fmt.Errorf("session: unmarshal board state for room %q: %w", roomCode, err)
	}
	return seat, legalMoveList(compiled, bs, seat), nil
}

func legalMoveList(compiled *catalog.CompiledGame, bs *state.BoardState, seat int) []LegalMove {
	moves := state.LegalMoves(compiled, bs, seat)
	out := make([]LegalMove, len(moves))
	for i, mv := range moves {
		out[i] = LegalMove{Move: mv, Notation: state.FormatMove(compiled, mv), Steps: state.Steps(compiled, mv)}
	}
	return out
}

// Simulation is the result of Simulate: a position reached by applying
// moves to a (possibly historical) position without persisting anything.
type Simulation struct {
	// BoardState is the v1 flat view; State is the full document.
	BoardState string
	State      string
	TurnSeat   *int // nil when the simulated game ended
	GameEnded  bool
	WinnerSeat *int
	EndReason  string
	// Applied lists the canonical notation of each applied move.
	Applied []string
	// Legal lists the legal moves for the seat to move (empty when ended).
	Legal []LegalMove
	// TPS is the position in TPS for PTN games (empty otherwise or when
	// the simulated game ended).
	TPS string
}

// Simulate starts from roomCode's position after move atSeq (nil = the
// current position) and applies each notation in turn, alternating seats per
// the turn order, without changing the game. It is a "what if" tool for
// analysis boards and for AI lookahead. An illegal move aborts the
// simulation with an error naming its index.
func (m *Manager) Simulate(ctx context.Context, roomCode string, atSeq *int, notations []string) (Simulation, error) {
	roomCode = canonRoom(roomCode)
	game, err := m.loadGame(ctx, roomCode, false)
	if err != nil {
		return Simulation{}, err
	}
	compiled, ok := m.catalog.Get(game.GameDefID)
	if !ok {
		return Simulation{}, fmt.Errorf("session: game definition %q not found for room %q", game.GameDefID, roomCode)
	}

	seq := game.MoveSeq
	if atSeq != nil {
		seq = *atSeq
	}
	if seq < 0 || seq > game.MoveSeq {
		return Simulation{}, store.ErrNotFound
	}
	doc, err := m.stateAtSeq(ctx, compiled, game, seq)
	if err != nil {
		return Simulation{}, err
	}
	bs := state.NewBoardState()
	if err := bs.UnmarshalJSON([]byte(doc)); err != nil {
		return Simulation{}, err
	}
	// A simulation must not see the dice or anyone's committed choice:
	// whatever the viewer cannot know is replaced by a random sample.
	bs = state.Determinize(compiled, bs, 0, rand.New(rand.NewSource(time.Now().UnixNano())))

	var seat int
	if seq == game.MoveSeq && game.Status == "finished" {
		return Simulation{}, ErrGameNotActive
	}
	if ts := m.turnSeatAt(ctx, compiled, game, seq); ts != nil {
		seat = *ts
	} else {
		return Simulation{}, ErrGameNotActive
	}

	sim := Simulation{Applied: []string{}}
	ended := false
	for i, text := range notations {
		if ended {
			return Simulation{}, fmt.Errorf("move %d (%q): the game is already over", i+1, text)
		}
		mv, err := state.ParseMove(compiled, text)
		if err != nil {
			return Simulation{}, fmt.Errorf("move %d (%q): %w", i+1, text, err)
		}
		res, err := state.Apply(compiled, bs, seat, mv)
		if err != nil {
			return Simulation{}, fmt.Errorf("move %d (%q): %w", i+1, text, err)
		}
		bs, seat = res.NewState, res.NextTurnSeat
		sim.Applied = append(sim.Applied, res.Notation)
		if res.GameEnded {
			ended = true
			sim.GameEnded = true
			sim.WinnerSeat = res.WinnerSeat
			sim.EndReason = res.EndReason
		}
	}

	shown := state.Public(compiled, bs, 0)
	full, err := shown.MarshalJSON()
	if err != nil {
		return Simulation{}, err
	}
	legacy, err := shown.LegacyJSON()
	if err != nil {
		return Simulation{}, err
	}
	sim.State, sim.BoardState = string(full), string(legacy)
	if !ended {
		sim.TurnSeat = &seat
		sim.Legal = legalMoveList(compiled, bs, seat)
		if compiled.Definition.Notation == "ptn" {
			if tps, err := state.FormatTPS(compiled, bs, seat); err == nil {
				sim.TPS = tps
			}
		}
	}
	return sim, nil
}

// HistoryEntry is what one move did, as the game narrated it.
type HistoryEntry struct {
	Ply   int      `json:"ply"`
	Seat  int      `json:"seat"`
	Lines []string `json:"lines"`
}

// History is the narration of every move of the game, oldest first: the part of
// each move's event log that the move itself added. A position only keeps its
// latest events, so the whole story is put together from the positions of all
// the moves.
func (m *Manager) History(ctx context.Context, roomCode string) ([]HistoryEntry, error) {
	game, err := m.loadGame(ctx, canonRoom(roomCode), false)
	if err != nil {
		return nil, err
	}
	logs, err := m.store.ListMoveLogs(ctx, game.ID)
	if err != nil {
		return nil, err
	}
	out := []HistoryEntry{}
	for _, l := range logs {
		if l.Log == "" {
			continue
		}
		var entries []HistoryEntry
		if err := json.Unmarshal([]byte(l.Log), &entries); err != nil {
			continue
		}
		for _, e := range entries {
			if e.Ply == l.Seq && len(e.Lines) > 0 {
				out = append(out, e)
			}
		}
	}
	return out, nil
}
