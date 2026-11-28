// A store that closes while a game is being counted closes after the count
// stops: nothing the manager started is still using the store when the store
// is closed, and what the stop cut short is done at the next start (#385).

package session_test

import (
	"context"
	"log/slog"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/session"
	"github.com/noderunner/bingle/internal/store"
)

// shutdownLog keeps what the manager logs, and says when Close begins to wait
// and when the count of a finished game fails.
type shutdownLog struct {
	mu      sync.Mutex
	waiting chan struct{} // closed when Close says it is waiting for work
	counted chan struct{} // closed when a count fails
	closed  chan struct{} // closed by the test when the store has closed
	late    bool          // the count failed after the store had closed
	lines   []string
}

func (h *shutdownLog) Enabled(context.Context, slog.Level) bool { return true }
func (h *shutdownLog) WithAttrs([]slog.Attr) slog.Handler       { return h }
func (h *shutdownLog) WithGroup(string) slog.Handler            { return h }

func (h *shutdownLog) Handle(_ context.Context, r slog.Record) error {
	line := r.Message
	r.Attrs(func(a slog.Attr) bool {
		line += " " + a.String()
		return true
	})
	h.mu.Lock()
	defer h.mu.Unlock()
	h.lines = append(h.lines, line)
	switch r.Message {
	case "stopping: waiting for background work":
		close(h.waiting)
	case "counting a finished game for its players":
		select {
		case <-h.closed:
			h.late = true
		default:
		}
		select {
		case <-h.counted:
		default:
			close(h.counted)
		}
	}
	return nil
}

// countGate is a clock that holds the count of a finished game where it
// first asks the time, until it is let go.
type countGate struct {
	now     time.Time
	once    sync.Once
	entered chan struct{}
	release chan struct{}
}

func (g *countGate) Now() time.Time {
	pcs := make([]uintptr, 32)
	frames := runtime.CallersFrames(pcs[:runtime.Callers(2, pcs)])
	for {
		f, more := frames.Next()
		if strings.Contains(f.Function, "session.(*Manager).countGame") {
			g.once.Do(func() { close(g.entered) })
			<-g.release
			break
		}
		if !more {
			break
		}
	}
	return g.now
}

// within fails the test unless ch closes within a minute: a bound on a wait
// for something that must happen, not a pause.
func within(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(time.Minute):
		t.Fatalf("%s did not happen", what)
	}
}

func TestShutdown_TheStoreClosesAfterTheCountOfAGameEnding(t *testing.T) {
	cat := catalog.NewCatalog()
	if err := cat.Register(fixtureGameDefID, []byte(fixtureDefJSON), nil); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "shutdown.db")
	st, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	log := &shutdownLog{waiting: make(chan struct{}), counted: make(chan struct{}), closed: make(chan struct{})}
	gate := &countGate{now: time.Now(), entered: make(chan struct{}), release: make(chan struct{})}
	m := session.NewManager(st, cat)
	m.SetLogger(slog.New(log))
	m.SetClock(gate.Now)

	ctx := context.Background()
	created, err := m.CreateGame(ctx, fixtureGameDefID, "Alice")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.JoinGame(ctx, created.RoomCode, "Bob", session.RolePlayer); err != nil {
		t.Fatal(err)
	}
	// The game ends, and its count starts away from the request and stops
	// at the gate.
	if _, err := m.Resign(ctx, created.RoomCode, "Bob"); err != nil {
		t.Fatal(err)
	}
	within(t, gate.entered, "the count of the finished game")

	// The server stops while the count is under way. The count is let go
	// once the store has closed, or once the close is waiting for it.
	go func() {
		_ = st.Close()
		close(log.closed)
	}()
	select {
	case <-log.closed:
	case <-log.waiting:
	}
	close(gate.release)
	within(t, log.closed, "the store's close")
	within(t, log.counted, "the end of the count")

	log.mu.Lock()
	late, lines := log.late, strings.Join(log.lines, "\n")
	log.mu.Unlock()
	if late {
		t.Errorf("the count of a finished game was still running when the store closed:\n%s", lines)
	}
	if strings.Contains(lines, "database is closed") {
		t.Errorf("work the manager started used the store after it closed:\n%s", lines)
	}

	// What the stop cut short is still to do, and the next start does it.
	st2, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st2.Close() })
	next := session.NewManager(st2, cat)
	next.RecordResults(ctx)
	g, err := st2.GetGame(ctx, created.RoomCode)
	if err != nil {
		t.Fatal(err)
	}
	var status string
	if err := st2.DB().QueryRow(`SELECT count_status FROM games WHERE id = ?`, g.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != store.CountSkipped {
		t.Errorf("after the restart the game's count is %q, want %q (two guests count for nobody)", status, store.CountSkipped)
	}
}
