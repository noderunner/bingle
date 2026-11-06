package session

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/store"
)

// RegroupMoves numbers the rows of games stored before impulses were counted
// (ADR 0114): each row gets the impulse it was a selection of, and each game
// the number of impulses it has ended, by the same test a move is recorded
// with (state.ImpulseEnds). It runs once at start-up and is a no-op after.
func (m *Manager) RegroupMoves(ctx context.Context) error {
	rooms, err := m.store.GamesToRegroup(ctx)
	if err != nil {
		return fmt.Errorf("session: games to regroup: %w", err)
	}
	for _, room := range rooms {
		game, err := m.store.GetGame(ctx, room)
		if err != nil {
			return fmt.Errorf("session: regroup %s: %w", room, err)
		}
		moves, err := m.store.ListMoves(ctx, game.ID, 0)
		if err != nil {
			return fmt.Errorf("session: regroup %s: %w", room, err)
		}
		compiled, _ := m.compiledFor(game) // a retired game is numbered by its seats alone
		plies, played := numberPlies(compiled, game, moves)
		if err := m.store.NumberPlies(ctx, game.ID, plies, played); err != nil {
			return fmt.Errorf("session: regroup %s: %w", room, err)
		}
	}
	if len(rooms) > 0 {
		slog.Info("session: moves regrouped by impulse", "games", len(rooms))
	}
	return nil
}

// numberPlies gives each stored row (by seq) the impulse it is part of, and
// says how many impulses ended. An impulse still open at the end of an
// unfinished game, from before the page sent moves whole, stays open.
func numberPlies(compiled *catalog.CompiledGame, game store.Game, moves []store.Move) (map[int]int, int) {
	plies := make(map[int]int, len(moves))
	ply, played := 1, 0
	before := ""
	if compiled != nil {
		before, _ = startDoc(compiled, game)
	}
	for i, mv := range moves {
		plies[mv.Seq] = ply
		if storedRowEndsImpulse(compiled, game, moves, i, before) {
			played = ply
			ply++
		}
		before = mv.ResultingState
	}
	return plies, played
}

// storedRowEndsImpulse replays row i from the position before it and asks
// state.ImpulseEnds; a reaction the session declined for the seat always ends
// its impulse, as when it is recorded. Where the rules can no longer replay it (a retired game,
// a definition that has changed), the row ends its impulse when the next row
// is another seat's, the game ended there, it was the pie-rule swap or it
// rolled or drew (locked).
func storedRowEndsImpulse(compiled *catalog.CompiledGame, game store.Game, moves []store.Move, i int, before string) bool {
	row := moves[i]
	if row.Auto || (compiled != nil && state.AnySelectionMayEnd(compiled)) {
		return true // a declined reaction is the seat's whole impulse
	}
	if compiled != nil && before != "" {
		bs := state.NewBoardState()
		var mv state.Selection
		var err error
		if err = bs.UnmarshalJSON([]byte(before)); err == nil {
			if row.MoveJSON != "" {
				err = json.Unmarshal([]byte(row.MoveJSON), &mv)
			} else {
				mv, err = state.ParseMove(compiled, row.Notation)
			}
		}
		if err == nil {
			if res, err := state.Apply(compiled, bs, row.Seat, mv); err == nil {
				return state.ImpulseEnds(compiled, bs, row.Seat, mv, res)
			}
		}
	}
	if i == len(moves)-1 {
		return game.Status != "active" || game.ImpulseSeat == nil || *game.ImpulseSeat != row.Seat
	}
	return moves[i+1].Seat != row.Seat || row.Notation == "swap" || row.Locked
}
