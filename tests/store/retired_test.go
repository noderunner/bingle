package store_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/noderunner/bingle/internal/store"
)

// EndRetiredGames ends the unfinished games of a retired game and of the
// games derived from it, with no winner; finished games and other games,
// even one whose id merely starts the same way, are left alone.
func TestEndRetiredGames(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "retired.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	ctx := context.Background()

	create := func(room, def string) store.Game {
		g, _, err := s.CreateGame(ctx, room, def, "{}", "Al", "red")
		if err != nil {
			t.Fatal(err)
		}
		return g
	}
	waiting := create("WAIT01", "connect-four")
	derived := create("PIE001", "connect-four~pie")
	similar := create("LOOK01", "connect-four-ish")
	other := create("GOMO01", "gomoku")
	done := create("DONE01", "connect-four")
	one := 1
	if err := s.FinishGame(ctx, done.ID, &one, "line"); err != nil {
		t.Fatal(err)
	}

	n, err := s.EndRetiredGames(ctx, []string{"connect-four"})
	if err != nil || n != 2 {
		t.Fatalf("EndRetiredGames = %d, %v; want 2 games ended", n, err)
	}
	for _, g := range []store.Game{waiting, derived} {
		got, _ := s.GetGame(ctx, g.RoomCode)
		if got.Status != "finished" || got.WinnerSeat != nil || got.EndReason != store.EndReasonRetired || got.TurnSeat != nil {
			t.Errorf("%s: %+v, want finished with no winner", g.RoomCode, got)
		}
	}
	for _, g := range []store.Game{similar, other} {
		if got, _ := s.GetGame(ctx, g.RoomCode); got.Status != "waiting" {
			t.Errorf("%s was touched: %s", g.RoomCode, got.Status)
		}
	}
	if got, _ := s.GetGame(ctx, done.RoomCode); got.WinnerSeat == nil || *got.WinnerSeat != 1 || got.EndReason != "line" {
		t.Errorf("a finished game lost its result: %+v", got)
	}
	if n, _ := s.EndRetiredGames(ctx, []string{"connect-four"}); n != 0 {
		t.Errorf("a second sweep ended %d more", n)
	}
}
