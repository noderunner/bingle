// Tests of a moderator taking a rated game out of the ratings (ADR 0157): who
// may, when the family is replayed and when only the game's change is
// reversed, and what a tournament's standings then say.

package session_test

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/noderunner/bingle/internal/auth"
	"github.com/noderunner/bingle/internal/rating"
	"github.com/noderunner/bingle/internal/session"
	"github.com/noderunner/bingle/internal/store"
)

// moderatorOf makes an account in the admin group, which holds every
// capability, signed in by hand.
func (w *ratedWorld) moderatorOf(handle string) player {
	w.t.Helper()
	ctx := context.Background()
	id, err := w.st.CreateAccount(ctx, store.NewAccount{
		Handle: handle, DisplayName: handle, Email: handle + "@example.test", PrincipalID: "p-" + handle, NewPrincipal: true, Verified: true,
		Groups: []string{store.GroupAdmin},
	})
	if err != nil {
		w.t.Fatal(err)
	}
	acc, err := w.st.AccountByID(ctx, id)
	if err != nil {
		w.t.Fatal(err)
	}
	return player{ctx: auth.WithPrincipal(ctx, auth.Principal{ID: acc.PrincipalID, Kind: store.KindAccount, Account: &acc}), handle: handle, id: id}
}

// later moves the clock on, so that games are some time apart and a rating ages
// between them. The database stamps its rows with the real clock, so the games'
// clocks have to outlast the jump.
func (w *ratedWorld) later() {
	w.seconds = session.MaxClockSeconds
	w.clock.Advance(3 * 24 * time.Hour)
}

func closeTo(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestRatingRemoval_OnlyAModerator(t *testing.T) {
	w := newRatedWorld(t)
	ana, bea, mod := w.account("ana"), w.account("bea"), w.moderatorOf("mod")
	room := w.decisive(ana, bea)

	refused := func(who string, ctx context.Context) {
		t.Helper()
		if _, err := w.m.RemoveRatedGame(ctx, room, ""); !errors.Is(err, session.ErrNotModerator) {
			t.Errorf("%s removes it: %v", who, err)
		}
		if _, err := w.m.RatedGamesOf(ctx, "ana"); !errors.Is(err, session.ErrNotModerator) {
			t.Errorf("%s reads Ana's rated games: %v", who, err)
		}
	}
	refused("a player of the game", ana.ctx)
	refused("nobody signed in", context.Background())
	// An administrator's token never carries the admin group's powers.
	acc, err := w.st.AccountByID(context.Background(), mod.id)
	if err != nil {
		t.Fatal(err)
	}
	refused("a token", auth.WithPrincipal(context.Background(), auth.Principal{ID: acc.PrincipalID, Kind: store.KindAccount, Account: &acc, Scopes: []string{auth.ScopePlay}}))
	// With the capability check of a server, its answer is the one that counts.
	w.m.SetMayModerate(func(context.Context) bool { return false })
	refused("an administrator the check refuses", mod.ctx)
	w.m.SetMayModerate(func(ctx context.Context) bool {
		p, ok := auth.From(ctx)
		return ok && p.Account != nil && p.Account.Handle == "bea"
	})
	refused("the administrator, now", mod.ctx)
	if _, err := w.m.RatedGamesOf(bea.ctx, "ana"); err != nil {
		t.Errorf("whoever the check allows: %v", err)
	}
	w.m.SetMayModerate(nil)

	if r, _ := w.rating(ana); r.Games != 1 {
		t.Fatalf("a refusal changed her rating: %+v", r)
	}
	if _, err := w.m.RemoveRatedGame(mod.ctx, room, ""); err != nil {
		t.Fatalf("the administrator removes it: %v", err)
	}
	if _, err := w.m.RemoveRatedGame(mod.ctx, room, ""); !errors.Is(err, session.ErrAlreadyRemoved) {
		t.Errorf("twice: %v", err)
	}
	if _, err := w.m.RemoveRatedGame(mod.ctx, "NOSUCH", ""); !errors.Is(err, session.ErrNotRatedGame) {
		t.Errorf("a room with no rated result: %v", err)
	}
}

// When the family's history reproduces its ratings, it is replayed without the
// game: both players lose it, and so do the later results that depended on it.
func TestRatingRemoval_RecomputesWhenTheHistoryAllows(t *testing.T) {
	w := newRatedWorld(t)
	ana, bea, cyd, mod := w.account("ana"), w.account("bea"), w.account("cyd"), w.moderatorOf("mod")
	first := w.decisive(ana, bea) // Ana beats Bea
	w.later()
	w.decisive(bea, cyd) // Bea beats Cyd
	w.later()
	w.decisive(ana, cyd) // Ana beats Cyd
	cydBefore, _ := w.rating(cyd)

	list, err := w.m.RatedGamesOf(mod.ctx, "@bea")
	if err != nil || len(list) != 2 || list[1].RoomCode != first || list[1].Opponent != "ana" || list[1].Score != 0 || list[1].Removed != nil {
		t.Fatalf("Bea's rated games: %+v, %v", list, err)
	}

	out, err := w.m.RemoveRatedGame(mod.ctx, first, "")
	if err != nil {
		t.Fatal(err)
	}
	if !out.Recomputed || len(out.Players) != 2 || out.Players[0].Handle != "ana" || out.Players[1].Handle != "bea" || out.Others != 1 {
		t.Errorf("the removal: %+v", out)
	}

	// Bea's first game is now her win over Cyd, as two new players.
	fresh := rating.Update(rating.New(), []rating.Result{{Opponent: rating.New(), Score: 1}})
	rb, _ := w.rating(bea)
	if rb.Games != 1 || rb.Opponents != 1 || !closeTo(rb.Rating, fresh.Rating) || !closeTo(rb.Deviation, fresh.Deviation) {
		t.Errorf("Bea's rating: %+v, want %+v", rb, fresh)
	}
	ra, _ := w.rating(ana)
	if ra.Games != 1 || ra.Opponents != 1 {
		t.Errorf("Ana's rating: %+v", ra)
	}
	// Cyd lost to a Bea who had not lost first, so his rating moved too.
	rc, _ := w.rating(cyd)
	if rc.Games != 2 || closeTo(rc.Rating, cydBefore.Rating) {
		t.Errorf("Cyd's rating did not move: %+v against %+v", rc, cydBefore)
	}
	stats, err := w.st.Stats(context.Background(), bea.id)
	if err != nil || stats["played:yavalath"] != 1 || stats["lost:yavalath"] != 0 || stats["won:yavalath"] != 1 {
		t.Errorf("Bea's counters: %v, %v", stats, err)
	}
	if got := w.status(first); got != store.RatingRemoved {
		t.Errorf("the game's status: %q", got)
	}
	if snap, err := w.m.GetState(ana.ctx, first, nil); err != nil || len(snap.RatingChanges) != 0 {
		t.Errorf("the game still shows its change: %+v, %v", snap.RatingChanges, err)
	}
	list, _ = w.m.RatedGamesOf(mod.ctx, "bea")
	if r := list[1].Removed; r == nil || r.By != "mod" || !r.Recomputed {
		t.Errorf("the list after: %+v", list[1])
	}

	events, err := w.st.AuthEvents(context.Background(), 5)
	if err != nil || len(events) == 0 || events[0].Kind != "rating-removed" || events[0].AccountID != mod.id ||
		!strings.Contains(events[0].Detail, "@ana") || !strings.Contains(events[0].Detail, "@bea") || !strings.Contains(events[0].Detail, "recomputed") {
		t.Errorf("the audit log: %+v, %v", events, err)
	}

	// What was written is itself a replay, so the next removal can be one too,
	// and a game played afterwards counts as usual.
	w.later()
	w.decisive(cyd, ana)
	if again, err := w.m.RemoveRatedGame(mod.ctx, list[0].RoomCode, ""); err != nil || !again.Recomputed {
		t.Errorf("a second removal: %+v, %v", again, err)
	}
	if _, ok := w.rating(bea); ok {
		t.Errorf("Bea has no rated game left in Yavalath, and still a rating")
	}
}

// When the history does not reproduce the stored ratings, the game's own change
// is reversed for both players, and nobody else's rating moves.
func TestRatingRemoval_ReversesWhenTheHistoryDoesNot(t *testing.T) {
	w := newRatedWorld(t)
	ana, bea, cyd, mod := w.account("ana"), w.account("bea"), w.account("cyd"), w.moderatorOf("mod")
	room := w.decisive(ana, bea)
	w.later()
	w.decisive(bea, cyd)
	w.later()
	w.decisive(ana, bea)
	// A rating the history cannot account for, as one written before a
	// change to how results were kept would be.
	if _, err := w.st.DB().Exec(`UPDATE ratings SET rating = rating + 7 WHERE account_id = ?`, cyd.id); err != nil {
		t.Fatal(err)
	}
	ra, _ := w.rating(ana)
	rb, _ := w.rating(bea)
	rc, _ := w.rating(cyd)
	changes, err := w.st.RatingChanges(context.Background(), room)
	if err != nil || len(changes) != 2 {
		t.Fatalf("the game's changes: %+v, %v", changes, err)
	}

	out, err := w.m.RemoveRatedGame(mod.ctx, room, "")
	if err != nil {
		t.Fatal(err)
	}
	if out.Recomputed || out.Others != 0 {
		t.Errorf("the removal: %+v", out)
	}
	ra2, _ := w.rating(ana)
	rb2, _ := w.rating(bea)
	rc2, _ := w.rating(cyd)
	if !closeTo(ra2.Rating, ra.Rating-(changes[0].After-changes[0].Before)) || ra2.Games != 1 || ra2.Opponents != 1 || ra2.Deviation != ra.Deviation {
		t.Errorf("Ana's rating: %+v, from %+v", ra2, ra)
	}
	if !closeTo(rb2.Rating, rb.Rating-(changes[1].After-changes[1].Before)) || rb2.Games != 2 || rb2.Opponents != 2 {
		t.Errorf("Bea's rating: %+v, from %+v", rb2, rb)
	}
	if rc2 != rc {
		t.Errorf("Cyd's rating moved: %+v, from %+v", rc2, rc)
	}
	events, _ := w.st.AuthEvents(context.Background(), 1)
	if len(events) != 1 || !strings.Contains(events[0].Detail, "reversed") {
		t.Errorf("the audit log: %+v", events)
	}
}

// A tournament's result stands, and its standings say, for everybody, what was
// taken out of the ratings and by whom.
func TestRatingRemoval_TournamentStandingsSayWhoRemovedWhat(t *testing.T) {
	w := newRatedWorld(t)
	org, mod := w.account("org"), w.moderatorOf("mod")
	ana, bea := w.account("ana"), w.account("bea")
	who := handles(ana, bea)
	id := w.openTournament(tournamentSettings("round_robin"), org, ana, bea)
	d, err := w.m.StartTournament(org.ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	game := openGames(d)[0]
	w.moveEachIn(game, who)
	w.finishGame(game, who, false)
	w.m.RecordResults(context.Background())
	before := w.detail(context.Background(), id)

	if _, err := w.m.RemoveRatedGame(mod.ctx, game.RoomCode, ""); err != nil {
		t.Fatal(err)
	}
	after := w.detail(context.Background(), id)
	for i, line := range after.Standings {
		if line.Score != before.Standings[i].Score || line.Place != before.Standings[i].Place {
			t.Errorf("the standings changed: %+v, from %+v", line, before.Standings[i])
		}
		if len(line.RatingRemovals) != 1 || line.RatingRemovals[0].By != "mod" || line.RatingRemovals[0].RoomCode != game.RoomCode {
			t.Errorf("@%s's line: %+v", line.Handle, line.RatingRemovals)
			continue
		}
		won := line.Handle == game.First
		if c := line.RatingRemovals[0].Change; (won && c <= 0) || (!won && c >= 0) {
			t.Errorf("@%s's removed change: %d", line.Handle, c)
		}
	}
}
