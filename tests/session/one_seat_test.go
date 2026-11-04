package session_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/games/solitaire"
	"github.com/noderunner/bingle/internal/session"
	"github.com/noderunner/bingle/internal/store"
)

func oneSeatManager(t *testing.T) *session.Manager {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "one.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	cat := catalog.NewCatalog()
	if err := cat.RegisterModule(solitaire.Module()); err != nil {
		t.Fatal(err)
	}
	return session.NewManager(st, cat)
}

// A game for one player has nobody to wait for: it starts as it is created, and its
// player can move, take moves back and give up (ADR 0086).
func TestOneSeat_StartsAtOnceAndPlays(t *testing.T) {
	m := oneSeatManager(t)
	ctx := context.Background()
	res, err := m.CreateGame(ctx, "solitaire", "Ana")
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "active" {
		t.Fatalf("status %q", res.Status)
	}
	snap, err := m.GetState(ctx, res.RoomCode, nil)
	if err != nil {
		t.Fatal(err)
	}
	if snap.ImpulseSeat == nil || *snap.ImpulseSeat != 1 {
		t.Fatalf("turn %v", snap.ImpulseSeat)
	}
	// Moves that show a new card each: they can be taken back all the same.
	playMove(t, m, res.RoomCode, "Ana", "draw")
	playMove(t, m, res.RoomCode, "Ana", "draw")
	for i := 0; i < 2; i++ {
		if _, err := m.Undo(ctx, res.RoomCode, "Ana"); err != nil {
			t.Fatalf("undo %d: %v", i, err)
		}
	}
	if _, err := m.Undo(ctx, res.RoomCode, "Ana"); !errors.Is(err, session.ErrNothingToUndo) {
		t.Errorf("undo with nothing to take back: %v", err)
	}
	// Giving up ends the game with nobody winning.
	snap, err = m.Resign(ctx, res.RoomCode, "Ana")
	if err != nil {
		t.Fatal(err)
	}
	if snap.Status != "finished" || snap.WinnerSeat != nil {
		t.Errorf("after giving up: status %q, winner %v", snap.Status, snap.WinnerSeat)
	}
}

// There is no second seat to sit in, and a game cannot be set up for two.
func TestOneSeat_NobodyElseCanSitDown(t *testing.T) {
	m := oneSeatManager(t)
	ctx := context.Background()
	res, err := m.CreateGame(ctx, "freecell", "Ana")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.JoinGame(ctx, res.RoomCode, "Bo", session.RolePlayer); !errors.Is(err, session.ErrSeatsFull) {
		t.Errorf("a second player: %v", err)
	}
	if _, err := m.JoinGame(ctx, res.RoomCode, "Bo", session.RoleSpectator); err != nil {
		t.Errorf("a spectator: %v", err)
	}
	// The whole-table clock is for two.
	_, err = m.CreateGameWithOptions(ctx, session.CreateOptions{GameDefID: "freecell", CreatorName: "Ana", ClockSeconds: []int{60}})
	if !errors.Is(err, session.ErrInvalidClock) {
		t.Errorf("a clock on a game for one: %v", err)
	}
}

// A hint is a legal move of the position, for every game of the family.
func TestOneSeat_HintsAreLegalMoves(t *testing.T) {
	m := oneSeatManager(t)
	ctx := context.Background()
	for _, id := range []string{"solitaire", "spider", "freecell", "bowling-solitaire"} {
		res, err := m.CreateGame(ctx, id, "Ana")
		if err != nil {
			t.Fatal(err)
		}
		h, err := m.Hint(ctx, res.RoomCode, "Ana", nil, nil, "")
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		if h.Notation == "" {
			t.Fatalf("%s: an empty hint", id)
		}
		playMove(t, m, res.RoomCode, "Ana", h.Notation)
	}
}
