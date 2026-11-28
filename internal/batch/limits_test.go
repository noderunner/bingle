package batch_test

// The limits on batches (ADR 0161): ceilings a spec is refused over, a
// player's allowances, who may raise a batch's limits, and the time limit that
// ends a batch.

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/noderunner/bingle/internal/app"
	"github.com/noderunner/bingle/internal/auth"
	"github.com/noderunner/bingle/internal/batch"
	"github.com/noderunner/bingle/internal/engine/catalog"
	_ "github.com/noderunner/bingle/internal/games/all"
	"github.com/noderunner/bingle/internal/store"
)

func newLimitedManager(t *testing.T, l batch.Limits) *batch.Manager {
	t.Helper()
	cat := catalog.NewCatalog()
	for _, m := range app.BuiltinModules() {
		if err := cat.RegisterModule(m); err != nil {
			t.Fatal(err)
		}
	}
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return batch.NewManager(st, cat, batch.WithLimits(l))
}

func hex7(games int) batch.Spec {
	return batch.Spec{Base: "hex-7", Games: games, Seed: 1, Players: []batch.Player{{Bot: "random"}, {Bot: "random"}}}
}

// as is ctx acting for a signed-in account, an administrator's own session
// when admin, or for a guest when id has no account.
func as(id string, admin bool) context.Context {
	return auth.WithPrincipal(context.Background(), auth.Principal{ID: id, Account: &store.Account{Admin: admin}})
}

// held keeps every batch waiting in the queue, so that a test counts what is
// queued without anything being played.
func held(t *testing.T, m *batch.Manager) {
	t.Helper()
	release, err := m.Hold(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)
}

func wantLimit(t *testing.T, what string, err error, limit string, max int) {
	t.Helper()
	var over *batch.LimitError
	if !errors.As(err, &over) || over.Limit != limit || over.Max != max {
		t.Errorf("%s: %v, want a refusal over %s at %d", what, err, limit, max)
	}
}

func wantAllowance(t *testing.T, what string, err error, limit string) {
	t.Helper()
	var no *batch.AllowanceError
	if !errors.As(err, &no) || no.Limit != limit {
		t.Errorf("%s: %v, want an allowance spent: %s", what, err, limit)
	}
}

// A spec over any ceiling is refused with the limit and its value, whatever
// it is over, and nothing is queued; what the operator sets is what counts.
func TestASpecOverACeilingIsRefused(t *testing.T) {
	m := newLimitedManager(t, batch.Limits{Ceilings: batch.Ceilings{Workers: 2, MCTSIterations: 50, MinimaxNodes: 400, MaxPlies: 100, Games: 20, EventGames: 3}})
	ctx := as("p_ann", false)
	for name, c := range map[string]struct {
		edit  func(*batch.Spec)
		limit string
		max   int
	}{
		"games":           {func(s *batch.Spec) { s.Games = 21 }, "games", 20},
		"workers":         {func(s *batch.Spec) { s.Workers = 3 }, "workers", 2},
		"max_plies":       {func(s *batch.Spec) { s.MaxPlies = 101 }, "max_plies", 100},
		"event_games":     {func(s *batch.Spec) { s.EventGames = 4 }, "event_games", 3},
		"mcts_iterations": {func(s *batch.Spec) { s.Players[1].MCTSIterations = 51 }, "mcts_iterations", 50},
		"minimax_nodes":   {func(s *batch.Spec) { s.Players[0].MinimaxNodes = 401 }, "minimax_nodes", 400},
	} {
		spec := hex7(2)
		c.edit(&spec)
		_, err := m.Start(ctx, "", spec)
		wantLimit(t, name, err, c.limit, c.max)
	}
	_, err := m.Start(ctx, string(make([]rune, batch.NameRunes+1)), hex7(2))
	wantLimit(t, "a long name", err, "name", batch.NameRunes)
	if list, _ := m.List(ctx); len(list) != 0 {
		t.Errorf("%d batches were queued by refused specs", len(list))
	}
	// At the ceilings is allowed.
	spec := hex7(20)
	spec.Workers, spec.MaxPlies, spec.EventGames = 2, 100, 3
	spec.Players = []batch.Player{{Bot: "mcts", MCTSIterations: 50}, {Bot: "minimax", MinimaxNodes: 400}}
	if _, err := m.Start(ctx, "", spec); err != nil {
		t.Errorf("a spec at its ceilings: %v", err)
	}
}

// What a spec leaves to the defaults is kept within the ceilings, so an
// operator who sets one below a default does not refuse every spec.
func TestTheDefaultsStayWithinTheCeilings(t *testing.T) {
	m := newLimitedManager(t, batch.Limits{Ceilings: batch.Ceilings{Workers: 1, MaxPlies: 50}})
	held(t, m)
	id, err := m.Start(as("p_ann", false), "", hex7(2))
	if err != nil {
		t.Fatal(err)
	}
	info, _ := m.Get(context.Background(), id)
	if info.Spec.Workers != 1 || info.Spec.MaxPlies != 50 {
		t.Errorf("the spec was saved with %d workers and %d plies, want 1 and 50", info.Spec.Workers, info.Spec.MaxPlies)
	}
	if m.DefaultWorkers() != 1 {
		t.Errorf("the default workers are %d", m.DefaultWorkers())
	}
}

// Only an administrator's own session may send limits of a batch's own: a
// guest, an account, an anonymous request and an administrator's API token
// (which never has the group's powers) are refused, and queue nothing.
func TestOnlyAnAdministratorRaisesALimit(t *testing.T) {
	m := newLimitedManager(t, batch.Limits{Ceilings: batch.Ceilings{Games: 5, Minutes: 5}})
	held(t, m)
	raised := hex7(8)
	raised.Limits = &batch.Ceilings{Games: 8}
	token := auth.WithPrincipal(context.Background(), auth.Principal{ID: "p_root", Account: &store.Account{Admin: true}, Scopes: []string{"play"}})
	for name, ctx := range map[string]context.Context{
		"anonymous":        context.Background(),
		"a guest":          auth.WithPrincipal(context.Background(), auth.Principal{ID: "p_guest"}),
		"an account":       as("p_ann", false),
		"an admin's token": token,
	} {
		if _, err := m.Start(ctx, "", raised); !errors.Is(err, batch.ErrNotAdmin) {
			t.Errorf("%s: %v, want ErrNotAdmin", name, err)
		}
	}
	if list, _ := m.List(context.Background()); len(list) != 0 {
		t.Errorf("%d batches were queued by those who may not", len(list))
	}
	// An administrator without limits of the batch's own is held to the
	// server's, and with them is not; what the limits leave unset stays.
	root := as("p_root", true)
	_, err := m.Start(root, "", hex7(8))
	wantLimit(t, "an administrator's plain spec", err, "games", 5)
	id, err := m.Start(root, "", raised)
	if err != nil {
		t.Fatalf("an administrator's raised batch: %v", err)
	}
	if info, _ := m.Get(context.Background(), id); info.Spec.Limits == nil || info.Spec.Limits.Games != 8 || info.Games != 8 {
		t.Errorf("the batch: %+v", info)
	}
	tooMany := hex7(8)
	tooMany.Workers = 1000
	tooMany.Limits = &batch.Ceilings{Games: 8}
	_, err = m.Start(root, "", tooMany)
	wantLimit(t, "a ceiling the limits did not raise", err, "workers", batch.DefaultLimits().Workers)
}

// A player may queue, keep and start only so much, another player is not
// held up by it, and an administrator is not held to it.
func TestAPlayersAllowances(t *testing.T) {
	m := newLimitedManager(t, batch.Limits{QueuedPerPlayer: 2, StoredPerPlayer: 3, GamesPerDay: 10, Queue: 6})
	held(t, m)
	ctx := context.Background()
	ann, bob, root := as("p_ann", false), as("p_bob", false), as("p_root", true)

	a1, err := m.Start(ann, "", hex7(2))
	if err != nil {
		t.Fatal(err)
	}
	a2, err := m.Start(ann, "", hex7(2))
	if err != nil {
		t.Fatal(err)
	}
	_, err = m.Start(ann, "", hex7(2))
	wantAllowance(t, "a third queued", err, "queued_per_player")
	if _, err := m.Start(bob, "", hex7(2)); err != nil {
		t.Errorf("another player: %v", err)
	}
	if _, err := m.Start(root, "", hex7(2)); err != nil {
		t.Errorf("an administrator: %v", err)
	}
	// Cancelled, a batch is no longer queued but is still kept.
	if err := m.Cancel(ctx, a1); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Start(ann, "", hex7(2)); err != nil {
		t.Errorf("after cancelling one: %v", err)
	}
	if err := m.Cancel(ctx, a2); err != nil {
		t.Fatal(err)
	}
	_, err = m.Start(ann, "", hex7(2))
	wantAllowance(t, "four kept", err, "stored_per_player")
	if err := m.Delete(ctx, a1); err != nil {
		t.Fatal(err)
	}
	// Games in a day: a batch not yet played counts in full, and cancelled
	// before it played, not at all.
	carl := as("p_carl", false)
	c1, err := m.Start(carl, "", hex7(6))
	if err != nil {
		t.Fatal(err)
	}
	_, err = m.Start(carl, "", hex7(6))
	wantAllowance(t, "twelve games in a day", err, "games_per_day")
	if err := m.Cancel(ctx, c1); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Start(carl, "", hex7(6)); err != nil {
		t.Errorf("after cancelling the batch that was never played: %v", err)
	}
	// The queue is the whole server's, and a refusal queues nothing.
	before, _ := m.List(ctx)
	_, err = m.Start(root, "", hex7(1))
	wantAllowance(t, "a full queue", err, "queue")
	if after, _ := m.List(ctx); len(after) != len(before) {
		t.Errorf("a batch refused for a full queue left %d rows behind", len(after)-len(before))
	}
}

// A batch that reaches its time limit stops, keeps the games it played and
// says which limit ended it; one that finishes in time, or is cancelled, does
// not.
func TestABatchEndsAtItsTimeLimit(t *testing.T) {
	m := newLimitedManager(t, batch.Limits{})
	m.SetWall(500 * time.Millisecond)
	ctx := as("p_ann", false)
	slow := batch.Spec{Base: "hex-11", Games: 5000, Seed: 1, Workers: 1, Players: []batch.Player{{Bot: "random"}, {Bot: "random"}}}
	id, err := m.Start(ctx, "", slow)
	if err != nil {
		t.Fatal(err)
	}
	info := waitStopped(t, m, id)
	if info.Status != store.BatchCancelled || info.Limit != batch.LimitTime {
		t.Errorf("a batch past its time: %q ending at %q", info.Status, info.Limit)
	}
	if info.Done < 1 || info.Done >= info.Games {
		t.Errorf("it played %d of %d games", info.Done, info.Games)
	}
	if rep, err := m.Report(ctx, id); err != nil || rep.Games != info.Done {
		t.Errorf("its report counts %d games, not the %d played: %v", rep.Games, info.Done, err)
	}

	m.SetWall(time.Minute)
	quick, _ := m.Start(ctx, "", hex7(3))
	if info := waitStopped(t, m, quick); info.Status != store.BatchDone || info.Limit != "" {
		t.Errorf("a batch in time: %q, limit %q", info.Status, info.Limit)
	}
	long, _ := m.Start(ctx, "", batch.Spec{Base: "hex-11", Games: 5000, Seed: 1, Workers: 1, Players: slow.Players})
	for deadline := time.Now().Add(20 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if info, _ := m.Get(ctx, long); info.Done > 0 {
			break
		}
	}
	_ = m.Cancel(ctx, long)
	if info := waitStopped(t, m, long); info.Status != store.BatchCancelled || info.Limit != "" {
		t.Errorf("a cancelled batch: %q, limit %q", info.Status, info.Limit)
	}
}

func waitStopped(t *testing.T, m *batch.Manager, id int64) batch.Info {
	t.Helper()
	for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		info, err := m.Get(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		if info.Status != store.BatchQueued && info.Status != store.BatchRunning {
			return info
		}
	}
	t.Fatalf("batch %d did not stop", id)
	return batch.Info{}
}
