// Tests of a game whose id changed (catalog.Module.Formerly): Reversi became
// Othello (#310).

package app_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/noderunner/bingle/internal/app"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/session"
)

// A database from before the rename opens with its Reversi games, ratings and
// versions under Othello, and the old id still finds the game and makes new
// games under the new one.
func TestRenamedGame_ReversiBecomesOthello(t *testing.T) {
	cfg := newTestConfig(t)
	ctx := context.Background()

	// A game played before the rename: made as Othello, then put back under
	// the old id, as an older server stored it.
	a, err := app.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	res, err := a.Manager().CreateGameWithOptions(ctx, session.CreateOptions{GameDefID: "othello", CreatorName: "Alice"})
	if err != nil {
		t.Fatal(err)
	}
	_ = a.Close()

	db, err := sql.Open("sqlite", cfg.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		`UPDATE games SET game_def_id = 'reversi' WHERE room_code = '` + res.RoomCode + `'`,
		`DELETE FROM game_versions WHERE family = 'othello'`,
		`INSERT INTO game_versions (family, version) VALUES ('reversi', '0.3.2')`,
		`INSERT INTO accounts (handle, handle_lower, display_name, email, email_lower, status, principal_id)
		 VALUES ('ana', 'ana', 'ana', 'ana@example.com', 'ana@example.com', 'active', 'p-ana')`,
		`INSERT INTO ratings (account_id, family, family_name, rating, deviation, volatility, games, opponents, last_game_at)
		 VALUES ((SELECT id FROM accounts WHERE handle = 'ana'), 'reversi', 'Reversi', 1600, 80, 0.06, 3, 1, CURRENT_TIMESTAMP)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	_ = db.Close()

	a, err = app.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Close() })

	db, err = sql.Open("sqlite", cfg.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var def, family, name string
	if err := db.QueryRow(`SELECT game_def_id FROM games WHERE room_code = ?`, res.RoomCode).Scan(&def); err != nil || def != "othello" {
		t.Errorf("the old game is stored under %q (%v)", def, err)
	}
	if err := db.QueryRow(`SELECT family, family_name FROM ratings`).Scan(&family, &name); err != nil || family != "othello" || name != "Othello" {
		t.Errorf("the rating is under %q, %q (%v)", family, name, err)
	}
	var left int
	if err := db.QueryRow(`SELECT (SELECT COUNT(*) FROM game_versions WHERE family = 'reversi') + (SELECT COUNT(*) FROM games WHERE game_def_id = 'reversi')`).Scan(&left); err != nil || left != 0 {
		t.Errorf("%d rows still under reversi (%v)", left, err)
	}

	// The old id finds the game, and what is made with it is Othello.
	cg, ok := a.Catalog().Get("reversi")
	if !ok || cg.Definition.ID != "othello" {
		t.Fatalf("reversi finds %v, %v", cg, ok)
	}
	made, err := a.Manager().CreateGameWithOptions(ctx, session.CreateOptions{GameDefID: "reversi", CreatorName: "Bob", Options: map[string]any{catalog.PieOption: true}})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT game_def_id FROM games WHERE room_code = ?`, made.RoomCode).Scan(&def); err != nil || def != "othello~pie" {
		t.Errorf("a game made as reversi with the pie rule is stored under %q (%v)", def, err)
	}
}
