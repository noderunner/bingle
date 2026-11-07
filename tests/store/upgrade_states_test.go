package store_test

import (
	"context"
	"testing"

	"github.com/noderunner/bingle/internal/store"
)

// UpgradeStates rewrites the positions of the games named and of the games
// derived from them, the board, the initial state and every move, and leaves
// the other games alone.
func TestUpgradeStatesRewritesTheGamesNamed(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	old := `{"old":true}`
	games := map[string]store.Game{}
	for i, def := range []string{"blooms", "blooms~pie", "bloomsday", "gomoku"} {
		g, _, err := s.CreateGame(ctx, "ROOM"+string(rune('A'+i)), def, old, "Ana", "red")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.RecordMove(ctx, g.ID, store.MoveInput{Seat: 1, ResultingBoardState: old, NextImpulseSeat: 1, Notation: "x"}); err != nil {
			t.Fatal(err)
		}
		games[def] = g
	}
	n, err := s.UpgradeStates(ctx, []string{"blooms"}, func(def string, doc []byte) ([]byte, bool) {
		if string(doc) != old {
			return doc, false
		}
		return []byte(`{"new":"` + def + `"}`), true
	})
	if err != nil {
		t.Fatal(err)
	}
	if n != 4 { // two games: a board and a move each (their initial state is empty)
		t.Fatalf("%d documents changed, want 4", n)
	}
	for def, g := range games {
		got, err := s.GetGame(ctx, g.RoomCode)
		if err != nil {
			t.Fatal(err)
		}
		moves, err := s.ListMoves(ctx, g.ID, 0)
		if err != nil || len(moves) != 1 {
			t.Fatalf("%s: %d moves, %v", def, len(moves), err)
		}
		want := old
		if def == "blooms" || def == "blooms~pie" {
			want = `{"new":"` + def + `"}`
		}
		if got.BoardState != want || moves[0].ResultingState != want {
			t.Errorf("%s: board %s, move %s, want %s", def, got.BoardState, moves[0].ResultingState, want)
		}
	}
	// Run again: nothing is old any more.
	if n, err := s.UpgradeStates(ctx, []string{"blooms"}, func(def string, doc []byte) ([]byte, bool) { return doc, string(doc) == old }); err != nil || n != 0 {
		t.Fatalf("the second run changed %d (%v)", n, err)
	}
}
