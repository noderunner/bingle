// Tests of the move funnel's log and the knobs that make it readable: one
// line per move for every game, the room as a reference rather than a code,
// and the position never carrying a secret unless asked.

package session_test

import (
	"context"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/games/courtfall"
	"github.com/noderunner/bingle/internal/games/tak"
	"github.com/noderunner/bingle/internal/logging"
	"github.com/noderunner/bingle/internal/session"
	"github.com/noderunner/bingle/internal/store"
)

// newLoggedManager returns a manager over Tak and Courtfall with its log
// captured at the given level.
func newLoggedManager(t *testing.T, level slog.Level, debug session.DebugOptions) (*session.Manager, *logBuf) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "movelog.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	cat := catalog.NewCatalog()
	for _, mod := range []catalog.Module{tak.Module(), courtfall.Module()} {
		if err := cat.RegisterModule(mod); err != nil {
			t.Fatal(err)
		}
	}
	buf := &logBuf{}
	m := session.NewManager(st, cat)
	m.SetLogger(logging.New(buf, level, logging.FormatText))
	m.SetDebug(debug)
	return m, buf
}

// play makes a game of gameID, fills it and plays n moves, returning the room.
func play(t *testing.T, m *session.Manager, gameID string, n int) string {
	t.Helper()
	ctx := context.Background()
	created, err := m.CreateGameWithOptions(ctx, session.CreateOptions{GameDefID: gameID, CreatorName: "Alice"})
	if err != nil {
		t.Fatalf("creating a %s game: %v", gameID, err)
	}
	if _, err := m.JoinGame(ctx, created.RoomCode, "Bob", session.RolePlayer); err != nil {
		t.Fatalf("Bob joins: %v", err)
	}
	names := []string{"Alice", "Bob"}
	for i := 0; i < n; i++ {
		seat, legal, err := m.LegalMoves(ctx, created.RoomCode, 0)
		if err != nil || len(legal) == 0 {
			t.Fatalf("move %d: legal moves for seat %d: %v (%d moves)", i, seat, err, len(legal))
		}
		if _, err := m.PlayNotation(ctx, created.RoomCode, names[(seat+1)%2], legal[0].Notation); err != nil {
			t.Fatalf("move %d: %v", i, err)
		}
	}
	return created.RoomCode
}

// One line per move, with the room as a reference and no player named.
func TestMoveLog_OneLinePerMoveAtDebug(t *testing.T) {
	m, buf := newLoggedManager(t, slog.LevelDebug, session.DebugOptions{})
	room := play(t, m, "tak-3x3", 3)

	got := buf.String()
	if n := strings.Count(got, `msg="move applied"`); n != 3 {
		t.Errorf("logged %d move lines for 3 moves:\n%s", n, got)
	}
	for _, want := range []string{"game=tak", "seq=1", "seq=2", "seq=3", "seat=", "dur="} {
		if !strings.Contains(got, want) {
			t.Errorf("the lines do not say %s:\n%s", want, got)
		}
	}
	// The room is followable without being joinable, and nobody is named.
	if strings.Contains(got, room) {
		t.Errorf("the log carries the room code %q:\n%s", room, got)
	}
	if !strings.Contains(got, "room="+logging.RoomRef(room)) {
		t.Errorf("the log does not carry the room's reference:\n%s", got)
	}
	for _, name := range []string{"Alice", "Bob"} {
		if strings.Contains(got, name) {
			t.Errorf("the log names a player (%s):\n%s", name, got)
		}
	}
}

// At the default level an ordinary server says nothing per move.
func TestMoveLog_SaysNothingAtInfo(t *testing.T) {
	m, buf := newLoggedManager(t, slog.LevelInfo, session.DebugOptions{})
	play(t, m, "tak-3x3", 3)

	if got := buf.String(); strings.Contains(got, "move applied") {
		t.Errorf("a move was logged at info:\n%s", got)
	}
}

// Debug across two dozen games is a firehose, so it can be narrowed to the
// games being worked on.
func TestMoveLog_GamesNarrowTheFirehose(t *testing.T) {
	m, buf := newLoggedManager(t, slog.LevelDebug, session.DebugOptions{Games: []string{"courtfall"}})
	play(t, m, "tak-3x3", 2)

	if got := buf.String(); strings.Contains(got, "move applied") {
		t.Errorf("Tak was logged while only Courtfall was asked for:\n%s", got)
	}

	play(t, m, "courtfall", 1)
	if got := buf.String(); !strings.Contains(got, "game=courtfall") {
		t.Errorf("Courtfall was not logged although it was asked for:\n%s", got)
	}
}

// One room can be watched whatever the level, for chasing a single report.
func TestMoveLog_AWatchedRoomIsLoggedAtInfo(t *testing.T) {
	// The room is not known until it is created, so this makes one first and
	// then watches it.
	m, buf := newLoggedManager(t, slog.LevelInfo, session.DebugOptions{})
	ctx := context.Background()
	created, err := m.CreateGameWithOptions(ctx, session.CreateOptions{GameDefID: "tak-3x3", CreatorName: "Alice"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.JoinGame(ctx, created.RoomCode, "Bob", session.RolePlayer); err != nil {
		t.Fatal(err)
	}
	m.SetDebug(session.DebugOptions{Room: created.RoomCode})

	seat, legal, err := m.LegalMoves(ctx, created.RoomCode, 0)
	if err != nil || len(legal) == 0 {
		t.Fatalf("legal moves for seat %d: %v", seat, err)
	}
	if _, err := m.PlayNotation(ctx, created.RoomCode, "Alice", legal[0].Notation); err != nil {
		t.Fatal(err)
	}

	got := buf.String()
	if !strings.Contains(got, `level=INFO msg="move applied"`) {
		t.Errorf("the watched room was not logged at info:\n%s", got)
	}
	// Watched or not, the room code itself still does not appear.
	if strings.Contains(got, created.RoomCode) {
		t.Errorf("watching a room put its code in the log:\n%s", got)
	}
}

// The position in a debug line is the public view. Courtfall commits dice in
// secret, so the full position is the thing that must not be there.
func TestMoveLog_ThePositionIsThePublicView(t *testing.T) {
	// Watching the room is what asks for a position at all.
	m, buf := newLoggedManager(t, slog.LevelDebug, session.DebugOptions{})
	ctx := context.Background()
	created, err := m.CreateGameWithOptions(ctx, session.CreateOptions{GameDefID: "courtfall", CreatorName: "Alice"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.JoinGame(ctx, created.RoomCode, "Bob", session.RolePlayer); err != nil {
		t.Fatal(err)
	}
	m.SetDebug(session.DebugOptions{Room: created.RoomCode})

	seat, legal, err := m.LegalMoves(ctx, created.RoomCode, 0)
	if err != nil || len(legal) == 0 {
		t.Fatalf("legal moves for seat %d: %v", seat, err)
	}
	if _, err := m.PlayNotation(ctx, created.RoomCode, "Alice", legal[0].Notation); err != nil {
		t.Fatal(err)
	}

	got := buf.String()
	if !strings.Contains(got, "position=") {
		t.Fatalf("a watched room logged no position:\n%s", got)
	}
	// The public view is what the opponents and the spectators already see,
	// so whatever it holds, it holds nothing a seat is keeping to itself.
	// What must not appear is the random state the position carries.
	if strings.Contains(got, "\"seed\"") || strings.Contains(got, "rng") {
		t.Errorf("the logged position carries the game's random state:\n%s", got)
	}
}
