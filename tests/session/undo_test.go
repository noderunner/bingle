// Tests of plies and of what can be taken back (ADR 0114): a move is the
// selections of one impulse, numbered by its ply, and only a game for one seat
// takes a move back (tests/session/one_seat_test.go).

package session_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/games/blooms"
	"github.com/noderunner/bingle/internal/games/classics"
	"github.com/noderunner/bingle/internal/session"
	"github.com/noderunner/bingle/internal/store"
)

func undoManagerAt(t *testing.T, path string) *session.Manager {
	t.Helper()
	st, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	cat := catalog.NewCatalog()
	mods := append(classics.Modules(), blooms.Module())
	for _, mod := range mods {
		if err := cat.RegisterModule(mod); err != nil {
			t.Fatal(err)
		}
	}
	return session.NewManager(st, cat)
}

func undoManager(t *testing.T) *session.Manager {
	t.Helper()
	return undoManagerAt(t, filepath.Join(t.TempDir(), "undo.db"))
}

func twoPlayers(t *testing.T, m *session.Manager, opts session.CreateOptions) string {
	t.Helper()
	ctx := context.Background()
	opts.CreatorName = "Alice"
	made, err := m.CreateGameWithOptions(ctx, opts)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.JoinGame(ctx, made.RoomCode, "Bob", session.RolePlayer); err != nil {
		t.Fatal(err)
	}
	return made.RoomCode
}

func playMove(t *testing.T, m *session.Manager, room, name, notation string) {
	t.Helper()
	if _, err := m.PlayNotation(context.Background(), room, name, notation); err != nil {
		t.Fatalf("%s plays %s: %v", name, notation, err)
	}
}

func plies(t *testing.T, m *session.Manager, room string) (rows []int, played int) {
	t.Helper()
	ctx := context.Background()
	moves, err := m.ListMoves(ctx, room, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, mv := range moves {
		rows = append(rows, mv.Ply)
	}
	snap, err := m.GetState(ctx, room, nil)
	if err != nil {
		t.Fatal(err)
	}
	return rows, snap.Ply
}

func samePlies(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// The selections of one impulse share its ply, and the game counts impulses.
func TestPlies_AMoveIsTheSelectionsOfAnImpulse(t *testing.T) {
	m := undoManager(t)
	room := twoPlayers(t, m, session.CreateOptions{GameDefID: "blooms"})
	playMove(t, m, room, "Alice", "red@e5")
	playMove(t, m, room, "Bob", "blue@d4; black@f6")
	if rows, played := plies(t, m, room); !samePlies(rows, []int{1, 2, 2}) || played != 2 {
		t.Fatalf("plies %v, played %d; want [1 2 2], 2", rows, played)
	}
	// A selection sent on its own (a tutorial does) leaves the impulse open.
	playMove(t, m, room, "Alice", "red@c3")
	if rows, played := plies(t, m, room); !samePlies(rows, []int{1, 2, 2, 3}) || played != 2 {
		t.Fatalf("plies %v, played %d; want [1 2 2 3], 2", rows, played)
	}
	playMove(t, m, room, "Alice", "done")
	if rows, played := plies(t, m, room); !samePlies(rows, []int{1, 2, 2, 3, 3}) || played != 3 {
		t.Fatalf("plies %v, played %d; want [1 2 2 3 3], 3", rows, played)
	}
	// The ply guard names the impulses played.
	ply := 2
	if _, err := m.PlayNotationAt(context.Background(), room, "Bob", "blue@d6; done", &ply); !errors.Is(err, session.ErrStalePly) {
		t.Fatalf("a stale ply: %v", err)
	}
	ply = 3
	if _, err := m.PlayNotationAt(context.Background(), room, "Bob", "blue@d6; done", &ply); err != nil {
		t.Fatalf("the ply played: %v", err)
	}
}

// Nothing is taken back in a game for two, part of an impulse included.
func TestUndo_OnlyAGameForOneSeatTakesAMoveBack(t *testing.T) {
	m := undoManager(t)
	ctx := context.Background()
	room := twoPlayers(t, m, session.CreateOptions{GameDefID: "blooms"})
	playMove(t, m, room, "Alice", "red@e5")
	playMove(t, m, room, "Bob", "blue@d4")
	if _, err := m.Undo(ctx, room, "Bob"); !errors.Is(err, session.ErrCannotUndo) {
		t.Fatalf("Undo in a game for two: %v, want ErrCannotUndo", err)
	}
	if moves, _ := m.ListMoves(ctx, room, 0); len(moves) != 2 {
		t.Fatalf("%d moves after a refused Undo, want 2", len(moves))
	}
}

// A database from before plies were counted is numbered on start-up by the
// same test a move is recorded with, an impulse left open included.
func TestRegroupMoves_NumbersStoredGames(t *testing.T) {
	path := filepath.Join(t.TempDir(), "regroup.db")
	m := undoManagerAt(t, path)
	ctx := context.Background()
	room := twoPlayers(t, m, session.CreateOptions{GameDefID: "blooms"})
	for _, mv := range []struct{ who, what string }{
		{"Alice", "red@e5"}, {"Bob", "blue@d4"}, {"Bob", "black@f6"}, {"Alice", "red@c3"}, {"Alice", "done"}, {"Bob", "blue@d6"},
	} {
		playMove(t, m, room, mv.who, mv.what)
	}
	want, wantPlayed := []int{1, 2, 2, 3, 3, 4}, 3
	if rows, played := plies(t, m, room); !samePlies(rows, want) || played != wantPlayed {
		t.Fatalf("as recorded: plies %v, played %d; want %v, %d", rows, played, want, wantPlayed)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(`UPDATE moves SET ply = 0; UPDATE games SET ply = 0`); err != nil {
		t.Fatal(err)
	}
	if err := m.RegroupMoves(ctx); err != nil {
		t.Fatal(err)
	}
	if rows, played := plies(t, m, room); !samePlies(rows, want) || played != wantPlayed {
		t.Fatalf("regrouped: plies %v, played %d; want %v, %d", rows, played, want, wantPlayed)
	}
	// Bob finishes the impulse he left open, and it keeps its ply.
	playMove(t, m, room, "Bob", "done")
	if rows, played := plies(t, m, room); !samePlies(rows, append(want, 4)) || played != 4 {
		t.Fatalf("after: plies %v, played %d", rows, played)
	}
}
