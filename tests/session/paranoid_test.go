// Tests of paranoid mode: the engine checked against itself after every move.
//
// It is for development and CI, so what matters is that it catches the bug it
// exists for. The way to prove that is a ruleset that really does lie, built
// here by wrapping a real one.

package session_test

import (
	"bytes"
	"context"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/games/fist"
	"github.com/noderunner/bingle/internal/logging"
	"github.com/noderunner/bingle/internal/session"
	"github.com/noderunner/bingle/internal/store"
)

// lyingRuleset is a real ruleset with one thing wrong with it: its Legal
// leaves out a move its Apply still accepts. That is the inconsistency
// paranoid mode exists to catch, and the one that otherwise reaches a player
// as a game that cannot be continued.
type lyingRuleset struct {
	state.Ruleset
	// dropFirst makes Legal leave out its first move.
	dropFirst bool

	mu sync.Mutex
	// dropped is the notation of the move Legal last left out. The test needs
	// it because asking the manager for the legal moves goes through this
	// Legal too, so the omitted move cannot be found that way — which is
	// exactly why a client can end up offering one.
	dropped string
}

func (r *lyingRuleset) Legal(game *catalog.CompiledGame, st *state.BoardState, seat int) []state.Move {
	moves := r.Ruleset.Legal(game, st, seat)
	if !r.dropFirst || len(moves) < 2 {
		return moves
	}
	r.mu.Lock()
	// Format is the wrapped ruleset's: this type only lies about Legal.
	r.dropped = r.Format(game, moves[0])
	r.mu.Unlock()
	return moves[1:]
}

func (r *lyingRuleset) droppedMove() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.dropped
}

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

// newParanoidManager returns a manager over Fist, optionally with its ruleset
// made to lie, with paranoid mode on and the log captured.
func newParanoidManager(t *testing.T, lying bool) (*session.Manager, *logBuf, string, *lyingRuleset) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "paranoid.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	cat := catalog.NewCatalog()
	mod := fist.Module()
	var liar *lyingRuleset
	if lying {
		base, ok := mod.Rules.(state.Ruleset)
		if !ok {
			t.Fatalf("the Fist module's rules are not a state.Ruleset (%T)", mod.Rules)
		}
		liar = &lyingRuleset{Ruleset: base, dropFirst: true}
		mod.Rules = liar
	}
	if err := cat.RegisterModule(mod); err != nil {
		t.Fatal(err)
	}
	// The id of the game the module registered, whatever it is called.
	var id string
	for _, cg := range cat.List() {
		if cg.Definition.Family == "fist" || cg.Definition.ID == "fist" {
			id = cg.Definition.ID
			break
		}
	}
	if id == "" {
		t.Fatal("the Fist module registered no game")
	}

	buf := &logBuf{}
	m := session.NewManager(st, cat)
	m.SetLogger(logging.New(buf, slog.LevelDebug, logging.FormatText))
	m.SetDebug(session.DebugOptions{Paranoid: true})
	return m, buf, id, liar
}

// playOneMove creates a game, fills its seats and plays one move: the move
// the liar leaves out of Legal when there is one, and an ordinary legal move
// otherwise.
func playOneMove(t *testing.T, m *session.Manager, gameID string, liar *lyingRuleset) (string, error) {
	t.Helper()
	ctx := context.Background()
	created, err := m.CreateGameWithOptions(ctx, session.CreateOptions{GameDefID: gameID, CreatorName: "Alice"})
	if err != nil {
		t.Fatalf("creating a %s game: %v", gameID, err)
	}
	if _, err := m.JoinGame(ctx, created.RoomCode, "Bob", session.RolePlayer); err != nil {
		t.Fatalf("Bob joins: %v", err)
	}
	// Asking for the legal moves also runs Legal, which is what makes the
	// liar record the move it is hiding.
	_, legal, err := m.LegalMoves(ctx, created.RoomCode, 0)
	if err != nil {
		t.Fatalf("legal moves: %v", err)
	}
	if len(legal) == 0 {
		t.Fatal("the opening position offers no moves")
	}
	notation := legal[0].Notation
	if liar != nil {
		if dropped := liar.droppedMove(); dropped != "" {
			notation = dropped
		} else {
			t.Fatal("the liar hid nothing, so there is nothing to catch")
		}
	}
	_, err = m.PlayNotation(ctx, created.RoomCode, "Alice", notation)
	return created.RoomCode, err
}

// The bug paranoid mode is for: Apply accepted a move Legal did not list.
func TestParanoid_CatchesAMoveLegalDidNotList(t *testing.T) {
	m, buf, gameID, liar := newParanoidManager(t, true)

	if _, err := playOneMove(t, m, gameID, liar); err != nil {
		t.Fatalf("the move was refused, so there is nothing to check: %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "paranoid mode found an inconsistency") {
		t.Fatalf("paranoid mode said nothing about a ruleset that lies:\n%s", got)
	}
	if !strings.Contains(got, "the applied move was not in Legal's list") {
		t.Errorf("the complaint does not name the check that failed:\n%s", got)
	}
	if !strings.Contains(got, "level=ERROR") {
		t.Errorf("the complaint is not an error:\n%s", got)
	}
	// And it says enough to find the game again, without the room code
	// itself (ADR 0063).
	if !strings.Contains(got, "game=fist") {
		t.Errorf("the complaint does not name the game:\n%s", got)
	}
}

// An honest ruleset is left in peace: paranoid mode must not cry wolf, or it
// will be turned off and stay off.
func TestParanoid_SaysNothingAboutAnHonestRuleset(t *testing.T) {
	m, buf, gameID, _ := newParanoidManager(t, false)

	if _, err := playOneMove(t, m, gameID, nil); err != nil {
		t.Fatalf("playing a legal move: %v", err)
	}
	if got := buf.String(); strings.Contains(got, "paranoid mode found an inconsistency") {
		t.Errorf("paranoid mode complained about a ruleset that is fine:\n%s", got)
	}
}

// With paranoid mode off the checks do not run at all — it doubles Legal on
// every move, which is why it is not for production.
func TestParanoid_OffMeansTheChecksDoNotRun(t *testing.T) {
	m, buf, gameID, liar := newParanoidManager(t, true)
	m.SetDebug(session.DebugOptions{}) // paranoid off

	if _, err := playOneMove(t, m, gameID, liar); err != nil {
		t.Fatalf("the move was refused: %v", err)
	}
	if got := buf.String(); strings.Contains(got, "paranoid mode") {
		t.Errorf("the checks ran with paranoid mode off:\n%s", got)
	}
}
