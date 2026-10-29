// Tests of repro bundles: a forced inconsistency writes one, the bundle
// replays to the same fault, and nothing else in the suite writes one.

package repro_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/noderunner/bingle/internal/app"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/games/fist"
	"github.com/noderunner/bingle/internal/logging"
	"github.com/noderunner/bingle/internal/repro"
	"github.com/noderunner/bingle/internal/session"
	"github.com/noderunner/bingle/internal/store"
)

// logBuf is a buffer the manager's goroutines can share.
type logBuf struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *logBuf) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *logBuf) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// lyingRuleset is a real ruleset with one thing wrong with it: its Legal
// leaves out a move its Apply still accepts. It is the same fixture
// tests/session uses, for the same reason — the only way to prove the
// machinery catches a fault is to build one.
type lyingRuleset struct {
	state.Ruleset
	mu      sync.Mutex
	dropped string
}

func (r *lyingRuleset) Legal(game *catalog.CompiledGame, st *state.BoardState, seat int) []state.Move {
	moves := r.Ruleset.Legal(game, st, seat)
	if len(moves) < 2 {
		return moves
	}
	r.mu.Lock()
	r.dropped = r.Format(game, moves[0])
	r.mu.Unlock()
	return moves[1:]
}

func (r *lyingRuleset) droppedMove() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.dropped
}

// bundlesIn reads every bundle in a directory.
func bundlesIn(t *testing.T, dir string) []repro.Bundle {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}
	var out []repro.Bundle
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		b, err := repro.Load(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("loading %s: %v", e.Name(), err)
		}
		out = append(out, b)
	}
	return out
}

// builtinCatalog is what scripts/replay builds: every game this build ships.
func builtinCatalog(t *testing.T) *catalog.Catalog {
	t.Helper()
	cat := catalog.NewCatalog()
	for _, mod := range app.BuiltinModules() {
		if err := cat.RegisterModule(mod); err != nil {
			t.Fatalf("registering %q: %v", mod.ID, err)
		}
	}
	return cat
}

// A forced inconsistency writes a bundle, and that bundle replays to the same
// fault. This is the whole point of the stage: a game bug is diagnosed by
// replaying the position, not by reading a log.
func TestRepro_AForcedInconsistencyWritesABundleThatReplays(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "debug")
	buf := &logBuf{}
	lg := logging.New(buf, slog.LevelDebug, logging.FormatText)

	st, err := store.Open(filepath.Join(t.TempDir(), "repro.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	cat := catalog.NewCatalog()
	mod := fist.Module()
	base, ok := mod.Rules.(state.Ruleset)
	if !ok {
		t.Fatalf("the Fist module's rules are not a state.Ruleset (%T)", mod.Rules)
	}
	liar := &lyingRuleset{Ruleset: base}
	mod.Rules = liar
	if err := cat.RegisterModule(mod); err != nil {
		t.Fatal(err)
	}

	m := session.NewManager(st, cat)
	m.SetLogger(lg)
	m.SetDebug(session.DebugOptions{Paranoid: true})
	m.SetRepro(repro.NewWriter(dir, "9.9.9", lg))

	ctx := context.Background()
	created, err := m.CreateGameWithOptions(ctx, session.CreateOptions{GameDefID: "fist", CreatorName: "Alice"})
	if err != nil {
		t.Fatalf("creating a game: %v", err)
	}
	if _, err := m.JoinGame(ctx, created.RoomCode, "Bob", session.RolePlayer); err != nil {
		t.Fatalf("Bob joins: %v", err)
	}
	// Asking for the legal moves makes the liar record the move it hides.
	if _, _, err := m.LegalMoves(ctx, created.RoomCode, 0); err != nil {
		t.Fatalf("legal moves: %v", err)
	}
	hidden := liar.droppedMove()
	if hidden == "" {
		t.Fatal("the liar hid nothing, so there is nothing to catch")
	}
	if _, err := m.PlayNotation(ctx, created.RoomCode, "Alice", hidden); err != nil {
		t.Fatalf("playing the hidden move: %v", err)
	}

	// One bundle, and an ERROR line carrying its id so a log and a directory
	// can be matched up.
	bundles := bundlesIn(t, dir)
	if len(bundles) != 1 {
		t.Fatalf("%d bundles written, want 1", len(bundles))
	}
	b := bundles[0]
	if b.Kind != repro.LegalDidNotListAnAppliedMove {
		t.Errorf("kind = %q", b.Kind)
	}
	if b.GameDefID != "fist" {
		t.Errorf("game = %q", b.GameDefID)
	}
	if b.Bingle != "9.9.9" {
		t.Errorf("version = %q", b.Bingle)
	}
	if len(b.Position) == 0 {
		t.Error("the bundle carries no position, so it cannot reproduce")
	}
	if b.Board == "" {
		t.Error("the bundle carries no board, so nobody can read it before running it")
	}
	if b.Notation != hidden {
		t.Errorf("notation = %q, want the move that was hidden (%q)", b.Notation, hidden)
	}
	log := buf.String()
	if !strings.Contains(log, "a repro bundle was written") || !strings.Contains(log, b.ID) {
		t.Errorf("the log does not carry the bundle's id:\n%s", log)
	}
	if !strings.Contains(log, "scripts/replay") {
		t.Errorf("the log does not say how to replay it:\n%s", log)
	}

	// And it replays — against the lying catalog, which is the build that
	// wrote it.
	res, err := repro.Replay(cat, b)
	if err != nil {
		t.Fatalf("replaying: %v", err)
	}
	if !res.Reproduced {
		t.Errorf("the bundle did not reproduce: %s", res)
	}

	// Against an honest build it does not reproduce, and says why — which is
	// how the bundle answers "is it fixed?".
	res, err = repro.Replay(builtinCatalog(t), b)
	if err != nil {
		t.Fatalf("replaying against the real rules: %v", err)
	}
	if res.Reproduced {
		t.Errorf("the bundle reproduced against the real Fist rules: %s", res)
	}
	if len(res.Notes) == 0 {
		t.Error("the replay said nothing about why it did not reproduce")
	}
	t.Logf("against the real rules: %s", res)
}

// A bundle says how the position was reached, and the replay checks that the
// path still leads there — a divergence being its own finding.
func TestRepro_ABundleCarriesThePathToThePosition(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "debug")
	buf := &logBuf{}
	lg := logging.New(buf, slog.LevelDebug, logging.FormatText)

	st, err := store.Open(filepath.Join(t.TempDir(), "repro.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	cat := catalog.NewCatalog()
	mod := fist.Module()
	base := mod.Rules.(state.Ruleset)
	liar := &lyingRuleset{Ruleset: base}
	mod.Rules = liar
	if err := cat.RegisterModule(mod); err != nil {
		t.Fatal(err)
	}

	m := session.NewManager(st, cat)
	m.SetLogger(lg)
	m.SetDebug(session.DebugOptions{Paranoid: true})
	m.SetRepro(repro.NewWriter(dir, "9.9.9", lg))

	ctx := context.Background()
	created, err := m.CreateGameWithOptions(ctx, session.CreateOptions{GameDefID: "fist", CreatorName: "Alice"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.JoinGame(ctx, created.RoomCode, "Bob", session.RolePlayer); err != nil {
		t.Fatal(err)
	}

	// Several ordinary moves first, so the bundle has a path worth recording.
	// These are moves the liar does list, so nothing impossible happens yet.
	names := []string{"Alice", "Bob"}
	for i := 0; i < 3; i++ {
		seat, legal, err := m.LegalMoves(ctx, created.RoomCode, 0)
		if err != nil || len(legal) == 0 {
			t.Fatalf("move %d: %v", i, err)
		}
		if _, err := m.PlayNotation(ctx, created.RoomCode, names[(seat+1)%2], legal[0].Notation); err != nil {
			t.Fatalf("move %d: %v", i, err)
		}
	}
	if n := len(bundlesIn(t, dir)); n != 0 {
		t.Fatalf("%d bundles written for ordinary moves", n)
	}

	// Now the move the liar hides, which is the inconsistency.
	seat, _, err := m.LegalMoves(ctx, created.RoomCode, 0)
	if err != nil {
		t.Fatalf("legal moves: %v", err)
	}
	hidden := liar.droppedMove()
	if hidden == "" {
		t.Fatal("the liar hid nothing, so there is nothing to catch")
	}
	if _, err := m.PlayNotation(ctx, created.RoomCode, names[(seat+1)%2], hidden); err != nil {
		t.Fatalf("playing the hidden move: %v", err)
	}

	bundles := bundlesIn(t, dir)
	if len(bundles) == 0 {
		t.Fatal("no bundle was written")
	}
	b := bundles[len(bundles)-1]
	if len(b.Moves) == 0 {
		t.Error("the bundle records no path, so how the position was reached is lost")
	}
	for _, mv := range b.Moves {
		if mv == "" {
			t.Error("the path has an empty move in it")
		}
	}

	res, err := repro.Replay(cat, b)
	if err != nil {
		t.Fatalf("replaying: %v", err)
	}
	// The path must land on the recorded position exactly. It is the
	// bundle's own consistency check, and getting it wrong is easy: the move
	// the bundle is *about* has already been recorded in the database by the
	// time paranoid mode looks, so a path that included it would replay one
	// move too far.
	var exact bool
	for _, n := range res.Notes {
		if strings.Contains(n, "DIFFERENT position") {
			t.Errorf("the recorded path does not lead to the recorded position: %s", res)
		}
		if strings.Contains(n, "reaches the recorded position exactly") {
			exact = true
		}
	}
	if !exact {
		t.Errorf("the replay did not confirm the path leads to the position: %s", res)
	}
}

// With no directory nothing is written, which is what a server run without
// BINGLE_DEBUG_DIR does.
func TestRepro_NoDirectoryMeansNoBundles(t *testing.T) {
	if w := repro.NewWriter("", "9.9.9", nil); w != nil {
		t.Errorf("an empty directory gave a writer: %+v", w)
	}
	var w *repro.Writer
	if w.On() {
		t.Error("a nil writer says it is on")
	}
	if got := w.Write(repro.Bundle{Kind: repro.ValidateFailed}); got != "" {
		t.Errorf("a nil writer wrote %q", got)
	}
	if got := w.Dir(); got != "" {
		t.Errorf("a nil writer has directory %q", got)
	}
}

// A ruleset that has gone wrong goes wrong on every move, so one run cannot
// fill the disk.
func TestRepro_TheNumberOfBundlesIsCapped(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "debug")
	buf := &logBuf{}
	w := repro.NewWriter(dir, "9.9.9", logging.New(buf, slog.LevelDebug, logging.FormatText))

	for i := 0; i < 500; i++ {
		w.Write(repro.Bundle{Kind: repro.ValidateFailed, GameDefID: "fist", Position: json.RawMessage(`{}`)})
	}

	bundles := bundlesIn(t, dir)
	if len(bundles) == 0 {
		t.Fatal("nothing was written at all")
	}
	if len(bundles) >= 500 {
		t.Errorf("%d bundles were written: the cap did not hold", len(bundles))
	}
	if !strings.Contains(buf.String(), "no more repro bundles") {
		t.Errorf("the cap was reached without saying so:\n%s", buf.String())
	}
	if n := strings.Count(buf.String(), "no more repro bundles"); n != 1 {
		t.Errorf("the cap was announced %d times, want once", n)
	}
	t.Logf("%d bundles written before the cap", len(bundles))
}

// A bundle this build cannot place is a clear message, not a crash.
func TestRepro_AnUnknownGameIsReportedClearly(t *testing.T) {
	_, err := repro.Replay(builtinCatalog(t), repro.Bundle{
		Kind: repro.ValidateFailed, GameDefID: "a-game-from-the-future",
		Position: json.RawMessage(`{}`), Bingle: "99.0.0",
	})
	if err == nil {
		t.Fatal("replaying a bundle for an unknown game returned no error")
	}
	if !strings.Contains(err.Error(), "a-game-from-the-future") || !strings.Contains(err.Error(), "99.0.0") {
		t.Errorf("the error does not say which game or which build: %v", err)
	}
}

// Loading something that is not a bundle says so.
func TestRepro_LoadRefusesRubbish(t *testing.T) {
	dir := t.TempDir()
	for name, content := range map[string]string{
		"notjson.json":    `{`,
		"nogame.json":     `{"kind":"validate-failed","position":{}}`,
		"noposition.json": `{"kind":"validate-failed","game_def_id":"fist"}`,
	} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := repro.Load(path); err == nil {
			t.Errorf("%s was loaded as a bundle", name)
		}
	}
	if _, err := repro.Load(filepath.Join(dir, "nothing-here.json")); err == nil {
		t.Error("a missing file was loaded as a bundle")
	}
}

// Nothing else writes a bundle.
//
// A bundle means something impossible has happened, so one appearing during an
// ordinary test run would be a real finding — and a stream of them would make
// the mechanism worthless, which is how this sort of thing usually dies. This
// plays every game the build ships, at random, with paranoid mode on and
// bundles switched on, and fails if a single one is written.
func TestRepro_NothingElseWritesABundle(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "debug")
	buf := &logBuf{}
	lg := logging.New(buf, slog.LevelDebug, logging.FormatText)

	st, err := store.Open(filepath.Join(t.TempDir(), "allgames.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	cat := builtinCatalog(t)
	m := session.NewManager(st, cat)
	m.SetLogger(lg)
	m.SetDebug(session.DebugOptions{Paranoid: true})
	m.SetRepro(repro.NewWriter(dir, "9.9.9", lg))

	ctx := context.Background()
	played := 0
	for _, cg := range cat.List() {
		def := cg.Definition
		// Two-seat games only: filling more seats by hand is a different
		// test, and tests/allgames already plays everything at random.
		if len(def.Seats) != 2 {
			continue
		}
		created, err := m.CreateGameWithOptions(ctx, session.CreateOptions{GameDefID: def.ID, CreatorName: "Alice"})
		if err != nil {
			continue // a game that needs setup options it was not given
		}
		if _, err := m.JoinGame(ctx, created.RoomCode, "Bob", session.RolePlayer); err != nil {
			continue
		}
		names := []string{"Alice", "Bob"}
		for ply := 0; ply < 12; ply++ {
			seat, legal, err := m.LegalMoves(ctx, created.RoomCode, 0)
			if err != nil || len(legal) == 0 {
				break
			}
			if _, err := m.PlayNotation(ctx, created.RoomCode, names[(seat+1)%2], legal[0].Notation); err != nil {
				break
			}
			played++
		}
	}
	if played < 50 {
		t.Fatalf("only %d moves were played, so this test is not looking where it thinks", played)
	}

	if bundles := bundlesIn(t, dir); len(bundles) != 0 {
		for _, b := range bundles {
			t.Errorf("a bundle was written during an ordinary run: %s on %s — %s\n%s",
				b.Kind, b.GameDefID, b.Note, b.Board)
		}
		t.Fatalf("%d bundles written over %d moves; every one is a real finding", len(bundles), played)
	}
	if strings.Contains(buf.String(), "paranoid mode found an inconsistency") {
		t.Errorf("paranoid mode complained during an ordinary run:\n%s", buf.String())
	}
	t.Logf("%d moves across the built-in games wrote no bundles", played)
}
