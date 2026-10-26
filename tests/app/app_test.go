// Tests of building the application: a real game is registered, and each call makes a fresh app that closes cleanly.

package app_test

import (
	"path/filepath"
	"testing"

	"github.com/noderunner/bingle/internal/app"
	"github.com/noderunner/bingle/internal/config"
	"github.com/noderunner/bingle/tests/testdata"
)

// registerFixtures adds the games only the tests use (see tests/testdata) to
// the app's catalog: the server registers none of them.
func registerFixtures(t *testing.T, a *app.App) {
	t.Helper()
	mod := testdata.Freeplay()
	if err := a.Catalog().Register(mod.ID, mod.Definition, mod.Callbacks); err != nil {
		t.Fatalf("registering the %s fixture: %v", mod.ID, err)
	}
}

// newTestConfig returns a config.Config pointing at a fresh, unique SQLite
// file under a per-call temp directory, so each call to app.New in these
// tests gets its own isolated store.
func newTestConfig(t *testing.T) config.Config {
	t.Helper()
	return config.Config{
		DBPath: filepath.Join(t.TempDir(), "test.db"),
	}
}

// TestNew_RegistersTicTacToeInCatalog covers the backlog item's AC
// itself: a fresh app.New(cfg) succeeds, and a.Catalog().Get("tic-tac-toe")
// returns a registered, compiled game.
func TestNew_RegistersTicTacToeInCatalog(t *testing.T) {
	cfg := newTestConfig(t)

	a, err := app.New(cfg)
	if err != nil {
		t.Fatalf("app.New(cfg) returned error: %v, want nil", err)
	}
	if a == nil {
		t.Fatal("app.New(cfg) returned nil *App with nil error")
	}
	t.Cleanup(func() { _ = a.Close() })

	cat := a.Catalog()
	if cat == nil {
		t.Fatal("a.Catalog() returned nil, want a non-nil *catalog.Catalog")
	}

	cg, ok := cat.Get("tic-tac-toe")
	if !ok {
		t.Fatal(`a.Catalog().Get("tic-tac-toe") returned ok=false, want true`)
	}
	if cg == nil {
		t.Fatal(`a.Catalog().Get("tic-tac-toe") returned nil *CompiledGame with ok=true`)
	}
}

// TestNew_TicTacToeHasTwoSeats is a light structural check (mirroring,
// without duplicating, tests/freeplay/freeplay_test.go's checks) that the
// "tic-tac-toe" entry registered into the App's catalog really is the
// compiled Tic-Tac-Toe game and not some placeholder: it must have exactly two
// seats.
func TestNew_TicTacToeHasTwoSeats(t *testing.T) {
	cfg := newTestConfig(t)

	a, err := app.New(cfg)
	if err != nil {
		t.Fatalf("app.New(cfg) returned error: %v, want nil", err)
	}
	t.Cleanup(func() { _ = a.Close() })

	cg, ok := a.Catalog().Get("tic-tac-toe")
	if !ok {
		t.Fatal(`a.Catalog().Get("tic-tac-toe") returned ok=false, want true`)
	}
	if cg.Definition == nil {
		t.Fatal("cg.Definition is nil, want a parsed *dsl.GameDefinition")
	}

	seats := cg.Definition.Seats
	if len(seats) != 2 {
		t.Fatalf("len(cg.Definition.Seats) = %d, want exactly 2 (got %#v)", len(seats), seats)
	}
}

// TestNew_RegistersFreshlyOnEachCall guards against an accidental
// package-level global catalog or sync.Once: calling app.New a second time,
// against a different fresh temp DB path, must produce an App whose own
// catalog ALSO has "tic-tac-toe" registered — i.e. registration is a
// per-App-instance side effect of New, not a one-time global side effect.
func TestNew_RegistersFreshlyOnEachCall(t *testing.T) {
	firstCfg := newTestConfig(t)
	first, err := app.New(firstCfg)
	if err != nil {
		t.Fatalf("app.New(firstCfg) returned error: %v, want nil", err)
	}
	t.Cleanup(func() { _ = first.Close() })

	if _, ok := first.Catalog().Get("tic-tac-toe"); !ok {
		t.Fatal(`first.Catalog().Get("tic-tac-toe") returned ok=false, want true`)
	}

	secondCfg := newTestConfig(t)
	if secondCfg.DBPath == firstCfg.DBPath {
		t.Fatalf("secondCfg.DBPath = %q, want a different path from firstCfg.DBPath (%q)", secondCfg.DBPath, firstCfg.DBPath)
	}

	second, err := app.New(secondCfg)
	if err != nil {
		t.Fatalf("app.New(secondCfg) returned error: %v, want nil", err)
	}
	t.Cleanup(func() { _ = second.Close() })

	secondCg, ok := second.Catalog().Get("tic-tac-toe")
	if !ok {
		t.Fatal(`second.Catalog().Get("tic-tac-toe") returned ok=false, want true — registration must happen freshly on every New call`)
	}
	if secondCg == nil {
		t.Fatal(`second.Catalog().Get("tic-tac-toe") returned nil *CompiledGame with ok=true`)
	}
}

// TestNew_CloseStillWorks is a light sanity check that adding catalog
// wiring to New hasn't broken App's existing Close lifecycle — not a full
// re-test of Close's behavior (see internal/app/app_test.go for that).
func TestNew_CloseStillWorks(t *testing.T) {
	cfg := newTestConfig(t)

	a, err := app.New(cfg)
	if err != nil {
		t.Fatalf("app.New(cfg) returned error: %v, want nil", err)
	}

	if err := a.Close(); err != nil {
		t.Errorf("a.Close() returned error: %v, want nil", err)
	}
}

func TestConfig_RetentionDaysFromEnvironment(t *testing.T) {
	t.Setenv("BINGLE_RETENTION_DAYS", "30")
	cfg, err := config.Load()
	if err != nil || cfg.RetentionDays != 30 {
		t.Fatalf("Load() = %+v, %v", cfg, err)
	}
	t.Setenv("BINGLE_RETENTION_DAYS", "-1")
	if _, err := config.Load(); err == nil {
		t.Error("a negative retention should be rejected")
	}
	t.Setenv("BINGLE_RETENTION_DAYS", "soon")
	if _, err := config.Load(); err == nil {
		t.Error("a non-numeric retention should be rejected")
	}
}

func TestPurgeInactiveKeepsFreshGames(t *testing.T) {
	cfg := config.Config{Addr: ":0", DBPath: filepath.Join(t.TempDir(), "purge.db")}
	a, err := app.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = a.Close() }()
	ctx := t.Context()
	if _, err := a.Manager().CreateGame(ctx, "tic-tac-toe", "Alice"); err != nil {
		t.Fatal(err)
	}
	if n, err := a.PurgeInactive(ctx, 0); err != nil || n != 0 {
		t.Errorf("PurgeInactive(0) = %d, %v; retention off must delete nothing", n, err)
	}
	if n, err := a.PurgeInactive(ctx, 30); err != nil || n != 0 {
		t.Errorf("PurgeInactive(30) = %d, %v; a fresh game must stay", n, err)
	}
}
