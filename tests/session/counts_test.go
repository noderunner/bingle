// Every game that counts adds to its players' lasting counts when it ends
// (ADR 0130): results and facts per family and the opponents met, rated or
// not, and only when every seat is a different account that moved.

package session_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/session"
	"github.com/noderunner/bingle/internal/store"
)

func (w *ratedWorld) stats(p player) map[string]int {
	w.t.Helper()
	s, err := w.st.Stats(context.Background(), p.id)
	if err != nil {
		w.t.Fatal(err)
	}
	return s
}

func (w *ratedWorld) countStatus(room string) string {
	w.t.Helper()
	g, err := w.st.GetGame(context.Background(), room)
	if err != nil {
		w.t.Fatal(err)
	}
	var status string
	if err := w.st.DB().QueryRow(`SELECT count_status FROM games WHERE id = ?`, g.ID).Scan(&status); err != nil {
		w.t.Fatal(err)
	}
	return status
}

// unrated makes an unrated game of def between two accounts.
func (w *ratedWorld) unrated(def string, a, b player) string {
	w.t.Helper()
	created, err := w.m.CreateGameWithOptions(a.ctx, session.CreateOptions{GameDefID: def, CreatorName: a.handle})
	if err != nil {
		w.t.Fatal(err)
	}
	if _, err := w.m.JoinGame(b.ctx, created.RoomCode, b.handle, session.RolePlayer); err != nil {
		w.t.Fatal(err)
	}
	return created.RoomCode
}

func (w *ratedWorld) resign(room string, p player) {
	w.t.Helper()
	if _, err := w.m.Resign(p.ctx, room, p.handle); err != nil {
		w.t.Fatal(err)
	}
	w.m.RecordResults(context.Background())
}

func TestAnUnratedGameCountsForBothPlayers(t *testing.T) {
	w := newRatedWorld(t)
	ana, bea := w.account("ana"), w.account("bea")
	room := w.unrated("yavalath", ana, bea)
	w.moveEach(room, ana, bea)
	w.resign(room, bea)

	if got, want := w.stats(ana), map[string]int{"game.played:yavalath": 1, "game.won:yavalath": 1, store.MetKey(bea.id, false): 1}; !reflect.DeepEqual(got, want) {
		t.Errorf("ana's counts = %v, want %v", got, want)
	}
	if got, want := w.stats(bea), map[string]int{"game.played:yavalath": 1, "game.lost:yavalath": 1, store.MetKey(ana.id, false): 1}; !reflect.DeepEqual(got, want) {
		t.Errorf("bea's counts = %v, want %v", got, want)
	}
	if got := w.countStatus(room); got != store.CountCounted {
		t.Errorf("count status = %q, want %q", got, store.CountCounted)
	}
	// Looking again adds nothing.
	w.m.RecordResults(context.Background())
	if got := w.stats(ana)["game.played:yavalath"]; got != 1 {
		t.Errorf("after a second look ana has played %d, want 1", got)
	}
}

// A rated game keeps its own keys as they were, and adds the opponent to the
// rated ones too, once its result is written.
func TestARatedGameCountsBesideItsRatedKeys(t *testing.T) {
	w := newRatedWorld(t)
	ana, bea := w.account("ana"), w.account("bea")
	w.decisive(ana, bea)

	got := w.stats(ana)
	for key, want := range map[string]int{
		"played:yavalath": 1, "won:yavalath": 1,
		"game.played:yavalath": 1, "game.won:yavalath": 1,
		store.MetKey(bea.id, false): 1, store.MetKey(bea.id, true): 1,
	} {
		if got[key] != want {
			t.Errorf("ana's %s = %d, want %d (all: %v)", key, got[key], want, got)
		}
	}
}

// A game somebody never moved in counts for nobody.
func TestANoShowCountsForNobody(t *testing.T) {
	w := newRatedWorld(t)
	ana, bea := w.account("ana"), w.account("bea")
	room := w.unrated("yavalath", ana, bea)
	if _, err := w.m.PlayNotation(ana.ctx, room, ana.handle, "e5"); err != nil {
		t.Fatal(err)
	}
	w.resign(room, bea)
	if got := w.stats(ana); len(got) != 0 {
		t.Errorf("ana's counts = %v, want none", got)
	}
	if got := w.countStatus(room); got != store.CountSkipped {
		t.Errorf("count status = %q, want %q", got, store.CountSkipped)
	}
}

// A guest at a seat makes the game count for nobody, the account included.
func TestAGuestAtASeatCountsForNobody(t *testing.T) {
	w := newRatedWorld(t)
	ana := w.account("ana")
	created, err := w.m.CreateGameWithOptions(ana.ctx, session.CreateOptions{GameDefID: "yavalath", CreatorName: ana.handle})
	if err != nil {
		t.Fatal(err)
	}
	guest := context.Background()
	if _, err := w.m.JoinGame(guest, created.RoomCode, "Gus", session.RolePlayer); err != nil {
		t.Fatal(err)
	}
	if _, err := w.m.PlayNotation(ana.ctx, created.RoomCode, ana.handle, "e5"); err != nil {
		t.Fatal(err)
	}
	if _, err := w.m.PlayNotation(guest, created.RoomCode, "Gus", "d4"); err != nil {
		t.Fatal(err)
	}
	w.resign(created.RoomCode, ana)
	if got := w.stats(ana); len(got) != 0 {
		t.Errorf("ana's counts = %v, want none", got)
	}
	if got := w.countStatus(created.RoomCode); got != store.CountSkipped {
		t.Errorf("count status = %q, want %q", got, store.CountSkipped)
	}
}

// The facts about a seat are added to its player's counts, per family.
func TestTheFactsOfAGameAreCountedForTheirSeats(t *testing.T) {
	w := newRatedWorld(t)
	if err := w.cat.Register("fixture-facts", []byte(factsFixtureDefJSON), nil); err != nil {
		t.Fatal(err)
	}
	ana, bea := w.account("ana"), w.account("bea")
	room := w.unrated("fixture-facts", ana, bea)
	for _, mv := range []struct {
		p     player
		piece string
		to    board.PositionID
	}{{ana, "black", "a1"}, {bea, "white", "b2"}, {ana, "black", "c3"}} {
		if _, err := w.m.Play(mv.p.ctx, room, mv.p.handle, state.Selection{Action: "place", Piece: mv.piece, To: mv.to}); err != nil {
			t.Fatal(err)
		}
	}
	w.resign(room, bea)
	if got := w.stats(ana)["fact.placed:fixture-facts"]; got != 2 {
		t.Errorf("ana placed %d, want 2", got)
	}
	if got := w.stats(bea)["fact.placed:fixture-facts"]; got != 1 {
		t.Errorf("bea placed %d, want 1", got)
	}
}
