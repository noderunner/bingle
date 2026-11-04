// Tests that a prototype, whose rules are still changing, has no ratings.

package session_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	mycelium "github.com/noderunner/bingle/internal/games/mycelium/v1.0.0"
	"github.com/noderunner/bingle/internal/session"
	"github.com/noderunner/bingle/internal/store"
)

func newPrototypeWorld(t *testing.T) *ratedWorld {
	t.Helper()
	w := newRatedWorld(t)
	if err := w.cat.RegisterModule(mycelium.Module()); err != nil {
		t.Fatal(err)
	}
	return w
}

func TestPrototype_ARatedGameIsRefused(t *testing.T) {
	w := newPrototypeWorld(t)
	ana := w.account("ana")
	for _, id := range []string{"mycelium", "mycelium-1.0.0"} {
		opts := ratedOpts(ana)
		opts.GameDefID = id
		if _, err := w.m.CreateGameWithOptions(ana.ctx, opts); !errors.Is(err, session.ErrCannotBeRated) {
			t.Errorf("%s: %v, want ErrCannotBeRated", id, err)
		}
	}
}

// A game rated before the game became a prototype can finish, and moves nothing.
func TestPrototype_AGameAlreadyRatedCountsForNothing(t *testing.T) {
	w := newPrototypeWorld(t)
	ana, bo := w.account("ana"), w.account("bo")
	ctx := context.Background()
	created, err := w.m.CreateGameWithOptions(ana.ctx, session.CreateOptions{GameDefID: "mycelium", CreatorName: "ana", ClockSeconds: []int{600, 600}})
	if err != nil {
		t.Fatal(err)
	}
	room := created.RoomCode
	g := mustGame(t, w, room)
	if err := w.st.SetRated(ctx, g.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := w.m.JoinGame(bo.ctx, room, "bo", session.RolePlayer); err != nil {
		t.Fatal(err)
	}
	for _, mv := range []struct {
		p player
		n string
	}{{ana, "black@d4"}, {bo, "green@f6"}} {
		if _, err := w.m.PlayNotation(mv.p.ctx, room, mv.p.handle, mv.n); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := w.m.Resign(bo.ctx, room, "bo"); err != nil {
		t.Fatal(err)
	}
	w.m.RecordResults(ctx)
	if got := w.status(room); got != store.RatingVoid {
		t.Errorf("rating status %q, want %q", got, store.RatingVoid)
	}
	if _, ok, err := w.st.RatingOf(ctx, ana.id, "mycelium"); err != nil || ok {
		t.Errorf("the winner has a rating (%v, %v)", ok, err)
	}
}

// A tournament of a prototype is unrated unless somebody insists, which is refused.
func TestPrototype_ATournamentIsNotRated(t *testing.T) {
	w := newPrototypeWorld(t)
	org := w.account("org")
	s := tournamentSettings("swiss")
	s.GameDefID = "mycelium"
	made, err := w.m.CreateTournament(org.ctx, s)
	if err != nil {
		t.Fatal(err)
	}
	if made.Rated {
		t.Error("a tournament of a prototype is rated by default")
	}
	if made.GameDefID != "mycelium-1.0.0" {
		t.Errorf("the tournament is of %q, not of one version", made.GameDefID)
	}
	yes := true
	s.Rated = &yes
	if _, err := w.m.CreateTournament(org.ctx, s); !errors.Is(err, session.ErrInvalidTournament) {
		t.Errorf("a rated tournament of a prototype: %v", err)
	}
}

// Ratings earned before a game became a prototype stay stored and are not shown.
func TestPrototype_OldRatingsAreKeptAndNotShown(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rated.db")
	st, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	w := newPrototypeWorld(t)
	w.st, w.m = st, session.NewManager(st, w.cat)
	ana := w.account("ana")
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()
	for _, family := range []string{"mycelium", "yavalath"} {
		if _, err := raw.Exec(`INSERT INTO ratings (account_id, family, family_name, rating, deviation, volatility, games, opponents, last_game_at)
			VALUES (?, ?, ?, 1600, 60, 0.06, 20, 5, CURRENT_TIMESTAMP)`, ana.id, family, family); err != nil {
			t.Fatal(err)
		}
	}
	ctx := context.Background()
	families, err := w.m.RatedFamilies(ctx)
	if err != nil || len(families) != 1 || families[0].Family != "yavalath" {
		t.Errorf("the games with a leaderboard: %+v (%v)", families, err)
	}
	if rows, err := w.m.Leaderboard(ctx, "mycelium", 0); err != nil || len(rows) != 0 {
		t.Errorf("Mycelium's leaderboard: %+v (%v)", rows, err)
	}
	if rows, err := w.m.Leaderboard(ctx, "yavalath", 0); err != nil || len(rows) != 1 {
		t.Errorf("Yavalath's leaderboard: %+v (%v)", rows, err)
	}
	if rows, err := w.m.PlayerRatings(ana.ctx, "ana"); err != nil || len(rows) != 1 || rows[0].Family != "yavalath" {
		t.Errorf("ana's own profile: %+v (%v)", rows, err)
	}
	if _, ok, err := st.RatingOf(ctx, ana.id, "mycelium"); err != nil || !ok {
		t.Errorf("the stored rating is gone (%v, %v)", ok, err)
	}
}
