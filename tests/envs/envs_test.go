// Package envs_test covers the in-memory training environments directly (the
// MCP tools over them are exercised in tests/app).
package envs_test

import (
	"context"
	"errors"
	"testing"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/envs"
	"github.com/noderunner/bingle/internal/games/classics"
	"github.com/noderunner/bingle/internal/games/tak"
	"github.com/noderunner/bingle/tests/testdata"
)

func manager(t testing.TB) *envs.Manager {
	t.Helper()
	cat := catalog.NewCatalog()
	mods := append([]catalog.Module{tak.Module(), testdata.TicTacToe(), testdata.ConnectFour()}, classics.Modules()...)
	for _, m := range mods {
		if err := cat.RegisterModule(m); err != nil {
			t.Fatal(err)
		}
	}
	return envs.NewManager(cat)
}

func TestResetValidatesInput(t *testing.T) {
	m := manager(t)
	ctx := context.Background()
	if _, err := m.Reset(ctx, envs.ResetOptions{GameDefID: "nope"}); !errors.Is(err, envs.ErrUnknownGame) {
		t.Errorf("unknown game: %v", err)
	}
	if _, err := m.Reset(ctx, envs.ResetOptions{GameDefID: "tak-5x5", Opponent: "gpt"}); !errors.Is(err, envs.ErrUnknownBot) {
		t.Errorf("unknown bot: %v", err)
	}
	if _, err := m.Reset(ctx, envs.ResetOptions{GameDefID: "tak-5x5", Seat: 3}); err == nil {
		t.Error("seat 3 should be rejected")
	}
	if _, err := m.Reset(ctx, envs.ResetOptions{GameDefID: "tak-5x5", StartTPS: "garbage"}); err == nil {
		t.Error("bad TPS should be rejected")
	}
}

func TestSeatTwoLetsTheBotOpen(t *testing.T) {
	m := manager(t)
	obs, err := m.Reset(context.Background(), envs.ResetOptions{GameDefID: "tak-5x5", Seat: 2, Opponent: "random", Seed: 3})
	if err != nil {
		t.Fatal(err)
	}
	if obs.Ply != 1 || len(obs.History) != 1 || len(obs.LastMoves) != 1 || !obs.YourTurn || obs.ToMove != 2 {
		t.Errorf("obs = ply %d history %v last %v yourTurn %v toMove %d", obs.Ply, obs.History, obs.LastMoves, obs.YourTurn, obs.ToMove)
	}
}

func TestStepRejectsIllegalMovesWithoutChangingState(t *testing.T) {
	m := manager(t)
	ctx := context.Background()
	obs, _ := m.Reset(ctx, envs.ResetOptions{GameDefID: "tak-3x3", Opponent: "random", Seed: 1})
	before := obs.Ply
	if _, err := m.Step(ctx, obs.EnvID, "Sa1"); err == nil {
		t.Error("a wall during the opening should be illegal")
	}
	if _, err := m.Step(ctx, obs.EnvID, "zz9"); err == nil {
		t.Error("garbage notation should be an error")
	}
	again, _ := m.Observe(obs.EnvID)
	if again.Ply != before {
		t.Errorf("ply changed from %d to %d after rejected moves", before, again.Ply)
	}
}

func TestSelfPlayControlsBothSeats(t *testing.T) {
	m := manager(t)
	ctx := context.Background()
	obs, err := m.Reset(ctx, envs.ResetOptions{GameDefID: "tic-tac-toe", Opponent: "self"})
	if err != nil {
		t.Fatal(err)
	}
	// X wins on the top row: a3, b3, c3 vs O's a2, b2.
	for _, mv := range []string{"a3", "a2", "b3", "b2", "c3"} {
		if obs.Done {
			t.Fatal("finished early")
		}
		if obs, err = m.Step(ctx, obs.EnvID, mv); err != nil {
			t.Fatalf("step %s: %v", mv, err)
		}
	}
	if !obs.Done || obs.WinnerSeat == nil || *obs.WinnerSeat != 1 || obs.Reward != 1 || obs.Result != "win" {
		t.Errorf("obs = done %v winner %v reward %v result %q", obs.Done, obs.WinnerSeat, obs.Reward, obs.Result)
	}
	if _, err := m.Step(ctx, obs.EnvID, "c1"); !errors.Is(err, envs.ErrGameOver) {
		t.Errorf("step after done: %v, want ErrGameOver", err)
	}
}

func TestForkIsIndependent(t *testing.T) {
	m := manager(t)
	ctx := context.Background()
	obs, _ := m.Reset(ctx, envs.ResetOptions{GameDefID: "tic-tac-toe", Opponent: "self"})
	if _, err := m.Step(ctx, obs.EnvID, "b2"); err != nil {
		t.Fatal(err)
	}
	fork, err := m.Fork(obs.EnvID)
	if err != nil {
		t.Fatal(err)
	}
	if fork.EnvID == obs.EnvID || fork.Ply != 1 {
		t.Fatalf("fork = %+v", fork)
	}
	// Moving in the original does not move the fork.
	if _, err := m.Step(ctx, obs.EnvID, "a1"); err != nil {
		t.Fatal(err)
	}
	if got, _ := m.Observe(fork.EnvID); got.Ply != 1 {
		t.Errorf("fork ply = %d, want 1", got.Ply)
	}
	m.Close(obs.EnvID)
	if _, err := m.Observe(obs.EnvID); !errors.Is(err, envs.ErrNotFound) {
		t.Errorf("closed env: %v", err)
	}
	if ids := m.List(); len(ids) != 1 || ids[0] != fork.EnvID {
		t.Errorf("List = %v", ids)
	}
}

func TestMaxPliesTruncates(t *testing.T) {
	m := manager(t)
	ctx := context.Background()
	obs, _ := m.Reset(ctx, envs.ResetOptions{GameDefID: "gomoku", Opponent: "random", Seed: 5, MaxPlies: 6})
	for i := 0; i < 5 && !obs.Done; i++ {
		var err error
		if obs, err = m.Step(ctx, obs.EnvID, obs.LegalMoves[0]); err != nil {
			t.Fatal(err)
		}
	}
	if !obs.Done || !obs.Truncated || obs.Reward != 0 || obs.EndReason != "max-plies" {
		t.Errorf("obs = done %v truncated %v reward %v reason %q", obs.Done, obs.Truncated, obs.Reward, obs.EndReason)
	}
}

func TestSeededEpisodesAreReproducible(t *testing.T) {
	play := func() []string {
		m := manager(t)
		ctx := context.Background()
		obs, _ := m.Reset(ctx, envs.ResetOptions{GameDefID: "tak-3x3", Opponent: "random", Seed: 11})
		for !obs.Done {
			var err error
			if obs, err = m.Step(ctx, obs.EnvID, obs.LegalMoves[0]); err != nil {
				t.Fatal(err)
			}
		}
		return obs.History
	}
	a, b := play(), play()
	if len(a) != len(b) {
		t.Fatalf("lengths differ: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("histories diverge at ply %d: %q vs %q", i, a[i], b[i])
		}
	}
}

func TestPlayMatchTalliesAndAlternatesSeats(t *testing.T) {
	m := manager(t)
	res, err := m.PlayMatch(context.Background(), envs.MatchOptions{
		GameDefID: "tak-3x3", BotA: "tactical", BotB: "random", Games: 10, Seed: 4, KeepMoves: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Played != 10 || res.AWins+res.BWins+res.Draws+res.Unfinished != 10 {
		t.Fatalf("tally does not add up: %+v", res)
	}
	if res.AWins <= res.BWins {
		t.Errorf("tactical (%d) should beat random (%d)", res.AWins, res.BWins)
	}
	firstA := 0
	for _, g := range res.Games {
		if g.FirstSeatBot == "a" {
			firstA++
		}
		if len(g.Moves) != g.Plies {
			t.Errorf("game kept %d moves for %d plies", len(g.Moves), g.Plies)
		}
	}
	if firstA != 5 {
		t.Errorf("bot a had the first seat in %d of 10 games, want 5 (alternating)", firstA)
	}

	// Same seed, same match.
	again, _ := m.PlayMatch(context.Background(), envs.MatchOptions{GameDefID: "tak-3x3", BotA: "tactical", BotB: "random", Games: 10, Seed: 4})
	if again.AWins != res.AWins || again.BWins != res.BWins || again.Draws != res.Draws {
		t.Errorf("a seeded match is not reproducible: %+v vs %+v", again, res)
	}

	if _, err := m.PlayMatch(context.Background(), envs.MatchOptions{GameDefID: "tak-3x3", BotA: "x", BotB: "random", Games: 1}); err == nil {
		t.Error("an unknown bot should be an error")
	}
}

func TestIterationLimitedMCTSIsReproducibleAndFast(t *testing.T) {
	play := func() []string {
		m := manager(t)
		ctx := context.Background()
		obs, err := m.Reset(ctx, envs.ResetOptions{GameDefID: "tak-3x3", Opponent: "mcts", Seed: 9, BotIterations: 60})
		if err != nil {
			t.Fatal(err)
		}
		for !obs.Done {
			if obs, err = m.Step(ctx, obs.EnvID, obs.LegalMoves[0]); err != nil {
				t.Fatal(err)
			}
		}
		return obs.History
	}
	a, b := play(), play()
	if len(a) != len(b) {
		t.Fatalf("lengths differ: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("iteration-limited MCTS diverged at ply %d: %q vs %q", i, a[i], b[i])
		}
	}
}

func TestEncodingsAndFixedActionIDs(t *testing.T) {
	m := manager(t)
	ctx := context.Background()
	obs, err := m.Reset(ctx, envs.ResetOptions{
		GameDefID: "tak-3x3", Opponent: "random", Seed: 4, Planes: true, ActionIDs: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if obs.Planes == nil || obs.Planes.Shape[1] != 3 || obs.Planes.Shape[2] != 3 {
		t.Fatalf("planes = %+v", obs.Planes)
	}
	if obs.NumActions == 0 || len(obs.LegalActionIDs) != len(obs.LegalMoves) {
		t.Fatalf("action ids: num=%d ids=%d moves=%d", obs.NumActions, len(obs.LegalActionIDs), len(obs.LegalMoves))
	}

	spec, err := m.Spec("tak-3x3", true, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !spec.Actions.Fixed || spec.Actions.Count != obs.NumActions || len(spec.Actions.Moves) != obs.NumActions {
		t.Fatalf("spec actions = %+v", spec.Actions)
	}
	for i, id := range obs.LegalActionIDs {
		if spec.Actions.Moves[id] != obs.LegalMoves[i] {
			t.Errorf("action %d is %q in the spec but %q in the observation", id, spec.Actions.Moves[id], obs.LegalMoves[i])
		}
	}
	if spec.Planes == nil || spec.Planes.Shape != obs.Planes.Shape {
		t.Errorf("spec planes %+v vs observation %v", spec.Planes, obs.Planes.Shape)
	}

	// Stepping by action ID is the same as stepping by notation.
	byID, err := m.StepAction(ctx, obs.EnvID, obs.LegalActionIDs[0])
	if err != nil {
		t.Fatal(err)
	}
	if byID.History[0] != obs.LegalMoves[0] {
		t.Errorf("first move = %q, want %q", byID.History[0], obs.LegalMoves[0])
	}
	if _, err := m.StepAction(ctx, obs.EnvID, 1<<20); err == nil {
		t.Error("an out-of-range action id should be rejected")
	}
}

func TestDynamicActionSpaceGamesStillWork(t *testing.T) {
	m := manager(t)
	ctx := context.Background()
	obs, err := m.Reset(ctx, envs.ResetOptions{GameDefID: "draughts", Opponent: "random", Seed: 1, Planes: true, ActionIDs: true})
	if err != nil {
		t.Fatal(err)
	}
	if obs.NumActions != 0 || len(obs.LegalActionIDs) != 0 || len(obs.LegalMoves) == 0 {
		t.Errorf("draughts: num=%d ids=%d moves=%d", obs.NumActions, len(obs.LegalActionIDs), len(obs.LegalMoves))
	}
	if _, err := m.StepAction(ctx, obs.EnvID, 0); err == nil {
		t.Error("draughts has no fixed action space; step by id must fail")
	}
	spec, _ := m.Spec("draughts", false, 0)
	if spec.Actions.Fixed {
		t.Error("draughts action space must be reported as dynamic")
	}
}

func TestStepManyRunsEnvironmentsIndependently(t *testing.T) {
	m := manager(t)
	ctx := context.Background()
	var reqs []envs.StepRequest
	var firsts []string
	for i := 0; i < 6; i++ {
		obs, err := m.Reset(ctx, envs.ResetOptions{GameDefID: "tic-tac-toe", Opponent: "random", Seed: int64(i + 1)})
		if err != nil {
			t.Fatal(err)
		}
		reqs = append(reqs, envs.StepRequest{EnvID: obs.EnvID, Move: obs.LegalMoves[0]})
		firsts = append(firsts, obs.LegalMoves[0])
	}
	// One bad step in the middle must not disturb the rest.
	reqs[2].Move = "zz9"
	// Two steps on one environment run in order.
	again := reqs[0]
	again.Move = ""
	reqs = append(reqs, again) // no move or action: an error of its own

	out := m.StepMany(ctx, reqs)
	if len(out) != len(reqs) {
		t.Fatalf("got %d outcomes for %d steps", len(out), len(reqs))
	}
	for i, o := range out {
		bad := i == 2 || i == len(reqs)-1
		if bad != (o.Error != "") {
			t.Errorf("step %d: error=%q, want error=%v", i, o.Error, bad)
		}
		if !bad && (o.Observation == nil || o.Observation.History[0] != firsts[i]) {
			t.Errorf("step %d: bad observation %+v", i, o.Observation)
		}
	}
}

func TestMatchRecordsTrainingPositions(t *testing.T) {
	m := manager(t)
	res, err := m.PlayMatch(context.Background(), envs.MatchOptions{
		GameDefID: "tak-3x3", BotA: "tactical", BotB: "random", Games: 4, Seed: 3, MaxPlies: 60,
		Record: true, RecordPlanes: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Games) != 4 {
		t.Fatalf("games = %d", len(res.Games))
	}
	spec, _ := m.Spec("tak-3x3", true, 0)
	for gi, g := range res.Games {
		if len(g.Positions) != g.Plies {
			t.Errorf("game %d: %d positions for %d plies", gi, len(g.Positions), g.Plies)
		}
		var winner = -2 // -2 unknown, 0 draw
		for _, p := range g.Positions {
			if p.Result == 1 {
				winner = p.ToMove
			}
		}
		for i, p := range g.Positions {
			if p.Planes == nil || p.Planes.Shape != spec.Planes.Shape {
				t.Fatalf("game %d ply %d: planes missing or wrong shape", gi, i)
			}
			if p.ActionID < 0 || spec.Actions.Moves[p.ActionID] != p.Move {
				t.Errorf("game %d ply %d: action %d is %q, move was %q", gi, i, p.ActionID, spec.Actions.Moves[p.ActionID], p.Move)
			}
			if len(p.State) == 0 || p.ToMove == 0 {
				t.Errorf("game %d ply %d: incomplete record", gi, i)
			}
			// Results are consistent: the mover who wins sees +1, the
			// opponent -1.
			if g.Winner != "unfinished" && g.Winner != "draw" && winner > 0 {
				want := -1
				if p.ToMove == winner {
					want = 1
				}
				if p.Result != want {
					t.Errorf("game %d ply %d: result %d, want %d", gi, i, p.Result, want)
				}
			}
		}
	}

	// Recording is capped.
	if _, err := m.PlayMatch(context.Background(), envs.MatchOptions{GameDefID: "tak-3x3", BotA: "random", BotB: "random", Games: envs.MaxRecordedGames + 1, Record: true}); err == nil {
		t.Error("recording more than the cap should be refused")
	}
}
