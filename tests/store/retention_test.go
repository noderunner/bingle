package store_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/noderunner/bingle/internal/store"
)

// Games nobody has touched for longer than the cutoff are deleted along with
// their players and moves; recently active games stay.
func TestDeleteInactiveGames(t *testing.T) {
	path := filepath.Join(t.TempDir(), "retention.db")
	s, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	ctx := context.Background()

	old, _, err := s.CreateGame(ctx, "OLDONE", "tak-5x5", "{}", "Alice", "white")
	if err != nil {
		t.Fatal(err)
	}
	fresh, _, err := s.CreateGame(ctx, "FRESH1", "tak-5x5", "{}", "Bob", "white")
	if err != nil {
		t.Fatal(err)
	}

	// Age the first game's row directly (the store stamps rows with "now").
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()
	week := time.Now().UTC().Add(-10 * 24 * time.Hour).Format("2006-01-02 15:04:05")
	if _, err := raw.Exec(`UPDATE games SET updated_at = ? WHERE id = ?`, week, old.ID); err != nil {
		t.Fatal(err)
	}

	n, err := s.DeleteInactiveGames(ctx, time.Now().Add(-7*24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("deleted %d games, want 1", n)
	}
	if _, err := s.GetGame(ctx, "OLDONE"); err == nil {
		t.Error("the old game should be gone")
	}
	if _, err := s.GetGame(ctx, "FRESH1"); err != nil {
		t.Errorf("the fresh game must stay: %v", err)
	}
	// Its players went with it (cascade).
	var players int
	if err := raw.QueryRow(`SELECT COUNT(*) FROM players WHERE game_id = ?`, old.ID).Scan(&players); err != nil {
		t.Fatal(err)
	}
	if players != 0 {
		t.Errorf("%d orphaned players", players)
	}
	_ = fresh
}
