// Tests of what can be taken back: a step of the turn still being played, and
// nothing else, in any kind of game (ADR 0083).

package session_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/games/blooms"
	"github.com/noderunner/bingle/internal/games/classics"
	"github.com/noderunner/bingle/internal/session"
	"github.com/noderunner/bingle/internal/store"
)

func undoManager(t *testing.T) *session.Manager {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "undo.db"))
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

// A move that hands the turn on stands, whoever has it next.
func TestUndo_AMoveStandsOnceTheTurnHasPassed(t *testing.T) {
	m := undoManager(t)
	ctx := context.Background()
	room := twoPlayers(t, m, session.CreateOptions{GameDefID: "yavalath"})
	if _, err := m.Undo(ctx, room, "Alice"); !errors.Is(err, session.ErrNothingToUndo) {
		t.Errorf("before any move: %v", err)
	}
	playMove(t, m, room, "Alice", "e5")
	if _, err := m.Undo(ctx, room, "Alice"); !errors.Is(err, session.ErrCannotUndo) {
		t.Errorf("Alice, with the turn passed: %v", err)
	}
	if _, err := m.Undo(ctx, room, "Bob"); !errors.Is(err, session.ErrNothingToUndo) {
		t.Errorf("Bob, who has not moved: %v", err)
	}
	playMove(t, m, room, "Bob", "d4")
	if _, err := m.Undo(ctx, room, "Bob"); !errors.Is(err, session.ErrCannotUndo) {
		t.Errorf("Bob, with the turn passed: %v", err)
	}
	if _, err := m.Undo(ctx, room, "Alice"); !errors.Is(err, session.ErrCannotUndo) {
		t.Errorf("Alice, with her move answered: %v", err)
	}
	if _, err := m.Resign(ctx, room, "Alice"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Undo(ctx, room, "Bob"); !errors.Is(err, session.ErrCannotUndo) {
		t.Errorf("after the game: %v", err)
	}
}

// A bot's reply passes the turn as well as a person's does.
func TestUndo_ABotsReplyDoesNotComeBack(t *testing.T) {
	m := undoManager(t)
	ctx := context.Background()
	made, err := m.CreateGameWithOptions(ctx, session.CreateOptions{GameDefID: "yavalath", CreatorName: "Alice", Opponent: "random"})
	if err != nil {
		t.Fatal(err)
	}
	playMove(t, m, made.RoomCode, "Alice", "e5")
	if _, err := m.Undo(ctx, made.RoomCode, "Alice"); !errors.Is(err, session.ErrCannotUndo) {
		t.Errorf("after the bot's reply: %v", err)
	}
}

// A swap hands the turn on though the seat's number may not change.
func TestUndo_ASwapStands(t *testing.T) {
	m := undoManager(t)
	ctx := context.Background()
	room := twoPlayers(t, m, session.CreateOptions{GameDefID: "yavalath", Options: map[string]any{"pie-rule": true}})
	playMove(t, m, room, "Alice", "e5")
	playMove(t, m, room, "Bob", "swap")
	for _, name := range []string{"Alice", "Bob"} {
		if _, err := m.Undo(ctx, room, name); !errors.Is(err, session.ErrCannotUndo) && !errors.Is(err, session.ErrNothingToUndo) {
			t.Errorf("%s, after the swap: %v", name, err)
		}
	}
	snap, err := m.GetState(ctx, room, nil)
	if err != nil || snap.MoveSeq != 2 {
		t.Errorf("after trying: move %d, %v", snap.MoveSeq, err)
	}
}

// The steps of a turn of several: each comes back, one at a time, until the turn
// is over.
func TestUndo_AStepOfTheTurnComesBack(t *testing.T) {
	m := undoManager(t)
	ctx := context.Background()
	room := twoPlayers(t, m, session.CreateOptions{GameDefID: "blooms"})
	// Blooms: the first player opens with one stone, then a turn is one or two.
	playMove(t, m, room, "Alice", "red@e5")
	playMove(t, m, room, "Bob", "blue@d4")
	snap, err := m.GetState(ctx, room, nil)
	if err != nil || snap.ImpulseSeat == nil || *snap.ImpulseSeat != 2 {
		t.Fatalf("after one stone of two: %+v, %v", snap.ImpulseSeat, err)
	}
	out, err := m.Undo(ctx, room, "Bob")
	if err != nil {
		t.Fatalf("the first stone: %v", err)
	}
	if len(out.Removed) != 1 || out.Snapshot.MoveSeq != 1 || *out.Snapshot.ImpulseSeat != 2 {
		t.Errorf("after taking it back: %+v at move %d", out.Removed, out.Snapshot.MoveSeq)
	}
	// Nothing of Alice's comes back with a second press.
	if _, err := m.Undo(ctx, room, "Bob"); !errors.Is(err, session.ErrNothingToUndo) {
		t.Errorf("past the start of the turn: %v", err)
	}

	playMove(t, m, room, "Bob", "blue@d4")
	playMove(t, m, room, "Bob", "black@f6")
	if _, err := m.Undo(ctx, room, "Bob"); !errors.Is(err, session.ErrCannotUndo) {
		t.Errorf("with the turn over: %v", err)
	}
	if snap, err = m.GetState(ctx, room, nil); err != nil || snap.MoveSeq != 3 {
		t.Errorf("after trying: move %d, %v", snap.MoveSeq, err)
	}
}

// Rated or in a tournament, the rule is the same.
func TestUndo_ARatedGameHasTheSameRule(t *testing.T) {
	w := newRatedWorld(t)
	if err := w.cat.RegisterModule(blooms.Module()); err != nil {
		t.Fatal(err)
	}
	ana, bea := w.account("ana"), w.account("bea")
	created, err := w.m.CreateGameWithOptions(ana.ctx, session.CreateOptions{GameDefID: "blooms", CreatorName: ana.handle, ClockSeconds: []int{600, 600}, Rated: true})
	if err != nil {
		t.Fatal(err)
	}
	room := created.RoomCode
	if _, err := w.m.JoinGame(bea.ctx, room, bea.handle, session.RolePlayer); err != nil {
		t.Fatal(err)
	}
	if _, err := w.m.PlayNotation(ana.ctx, room, ana.handle, "red@e5"); err != nil {
		t.Fatal(err)
	}
	if _, err := w.m.PlayNotation(bea.ctx, room, bea.handle, "blue@d4"); err != nil {
		t.Fatal(err)
	}
	if _, err := w.m.Undo(bea.ctx, room, bea.handle); err != nil {
		t.Errorf("a step of the turn: %v", err)
	}
	if _, err := w.m.Undo(ana.ctx, room, ana.handle); !errors.Is(err, session.ErrCannotUndo) {
		t.Errorf("a move with the turn passed: %v", err)
	}
}

func TestMoveIsFinal(t *testing.T) {
	m := undoManager(t)
	ctx := context.Background()
	room := twoPlayers(t, m, session.CreateOptions{GameDefID: "blooms"})
	final := func(name, notation string) bool {
		t.Helper()
		got, err := m.MoveIsFinal(ctx, room, name, notation)
		if err != nil {
			t.Fatalf("%s %s: %v", name, notation, err)
		}
		return got
	}
	// The opening stone ends the turn; the next turn's first stone does not, and
	// its second does.
	if !final("Alice", "red@e5") {
		t.Error("the opening stone hands the turn on")
	}
	playMove(t, m, room, "Alice", "red@e5")
	if final("Bob", "blue@d4") {
		t.Error("the first stone of two can come back")
	}
	playMove(t, m, room, "Bob", "blue@d4")
	if !final("Bob", "black@f6") {
		t.Error("the second stone hands the turn on")
	}
	if !final("Bob", "done") {
		t.Error("ending the turn hands it on")
	}
	// Asking changes nothing.
	snap, err := m.GetState(ctx, room, nil)
	if err != nil || snap.MoveSeq != 2 {
		t.Errorf("after asking: move %d, %v", snap.MoveSeq, err)
	}
	if _, err := m.MoveIsFinal(ctx, room, "Alice", "red@a1"); !errors.Is(err, session.ErrNotYourImpulse) {
		t.Errorf("out of turn: %v", err)
	}
	if _, err := m.MoveIsFinal(ctx, room, "Bob", "black@e5"); err == nil {
		t.Error("an illegal move has no answer")
	}

	generic := twoPlayers(t, m, session.CreateOptions{GameDefID: "yavalath"})
	if got, err := m.MoveIsFinal(ctx, generic, "Alice", "e5"); err != nil || !got {
		t.Errorf("a game of one move a turn: %v, %v", got, err)
	}
}
