package session

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"github.com/noderunner/bingle/internal/auth"
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
	// everything before it, can no longer be taken back. Set in a game for
	// one seat only, the one that takes moves back.
	Locked bool
	// Ply is the impulse the move is a selection of (ADR 0114).
	Ply int
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
	if compiled, ok := m.catalog.Get(game.GameDefID); ok && state.Guarded(compiled) && game.Status == "active" {
		bs := state.NewBoardState()
		if err := bs.UnmarshalJSON([]byte(game.BoardState)); err == nil {
			current = bs
		}
	}
	alone := false
	if compiled, ok := m.catalog.Get(game.GameDefID); ok {
		alone = state.AnySelectionMayEnd(compiled)
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
			Locked:    mv.Locked && alone,
			Ply:       mv.Ply,
		}
		if mv.MoveJSON != "" {
			var sm state.Selection
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

// SelectionOption is one legal selection: the structured selection, its
// canonical notation, and the gestures that build it.
type SelectionOption struct {
	Selection state.Selection
	Notation  string
	Gestures  []state.Gesture
}

// mayListMoves reports whether the request may see what seat can do. In a game with secrets the
// list can name the seat's hand, so it goes to whoever holds the seat (ADR 0112); a seat nobody owns
// is acted for by name, by anybody, so it hides nothing. Unlike mayView this only reads: it must not
// make a seat nobody owns the property of whoever happened to ask.
func (m *Manager) mayListMoves(ctx context.Context, compiled *catalog.CompiledGame, game store.Game, seat int) (bool, error) {
	if !state.HasSecrets(compiled) {
		return true, nil
	}
	players, err := m.store.ListPlayers(ctx, game.ID)
	if err != nil {
		return false, err
	}
	for i := range players {
		if players[i].Seat == seat && players[i].Bot == "" {
			return players[i].PrincipalID == "" || players[i].PrincipalID == auth.IDFrom(ctx), nil
		}
	}
	return false, nil
}

func legalMoveList(compiled *catalog.CompiledGame, bs *state.BoardState, seat int) []SelectionOption {
	moves := state.LegalSelections(compiled, bs, seat)
	out := make([]SelectionOption, len(moves))
	for i, mv := range moves {
		out[i] = SelectionOption{Selection: mv, Notation: state.FormatMove(compiled, mv), Gestures: state.Gestures(compiled, mv)}
	}
	return out
}

// Simulation is the result of Simulate: a position reached by applying
// moves to a (possibly historical) position without persisting anything.
type Simulation struct {
	// BoardState is the v1 flat view; State is the full document.
	BoardState  string
	State       string
	ImpulseSeat *int // nil when the simulated game ended
	GameEnded   bool
	WinnerSeat  *int
	EndReason   string
	// Applied lists the canonical notation of each applied move.
	Applied []string
	// Legal lists the legal moves for the seat to move (empty when ended).
	Legal []SelectionOption
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
	compiled, err := m.compiledFor(game)
	if err != nil {
		return Simulation{}, err
	}

	// After atSeq moves (impulses), or the position as it is.
	seq, live := game.MoveSeq, atSeq == nil || *atSeq == game.Ply
	var doc string
	if atSeq == nil {
		doc, err = m.stateAtSeq(ctx, compiled, game, seq)
	} else {
		doc, seq, err = m.stateAtPly(ctx, compiled, game, *atSeq)
	}
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
	if live && game.Status == "finished" {
		return Simulation{}, ErrGameNotActive
	}
	if ts := m.impulseSeatAt(ctx, compiled, game, seq); ts != nil {
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
		bs, seat = res.NewState, res.NextImpulseSeat
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
		sim.ImpulseSeat = &seat
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
			// A position's log counts the engine's plies, one per row; the
			// story is told by move (impulse, ADR 0114), the parts of one
			// move together.
			if e.Ply != l.Seq || len(e.Lines) == 0 {
				continue
			}
			e.Ply = l.Ply
			if n := len(out); n > 0 && out[n-1].Ply == e.Ply && out[n-1].Seat == e.Seat {
				out[n-1].Lines = append(out[n-1].Lines, e.Lines...)
				continue
			}
			out = append(out, e)
		}
	}
	return out, nil
}
