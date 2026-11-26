package wasmrules_test

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/noderunner/bingle/internal/batch"
	"github.com/noderunner/bingle/internal/bots"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/engine/wasmrules"
	"github.com/noderunner/bingle/tests/testdata"
	"github.com/noderunner/bingle/tests/testdata/draughts"
)

// What bots and practice batches cost through the boundary of ADR 0147 (#299),
// on the Draughts fixture with the same rules both ways. It is a development
// aid, not an assertion, and takes some minutes: run it with
//
//	BINGLE_WASM_REPORT=1 go test ./tests/wasmrules -run Report -v -timeout 60m
//
// BINGLE_WASM_REPORT_GAMES sets the games of the strength match (default 20).
// The sides built with TinyGo need it (testdata.TinyGo), and are left out
// without it.
func TestReport(t *testing.T) {
	if os.Getenv("BINGLE_WASM_REPORT") == "" {
		t.Skip("set BINGLE_WASM_REPORT=1 to measure bots and batches through a module")
	}
	t.Run("Module", reportModule)
	t.Run("Search", reportSearch)
	t.Run("Batch", reportBatch)
	t.Run("Strength", reportStrength)
}

// reportSide is one way of running the fixture's rules.
type reportSide struct {
	name string
	cat  *catalog.Catalog
	game *catalog.CompiledGame
	mod  *wasmrules.Module // nil for rules linked in
}

// The sides: the rules linked in and searched by the host's bots; the guest's
// search over the rules linked in, by way of the JSON (in process); the
// host's bots searching across the module's boundary, with a module built
// without the bot call (across); the search inside the module (inside); and
// inside a module whose calls have no deadline (inside, no deadline). The
// TinyGo sides are across and inside with a module TinyGo built.
const (
	sideLinked       = "linked in"
	sideInProcess    = "in process"
	sideAcross       = "across"
	sideInside       = "inside"
	sideNoDeadline   = "inside, no deadline"
	sideAcrossTinyGo = "across, TinyGo"
	sideInsideTinyGo = "inside, TinyGo"
)

// tinyGoSides are names with the TinyGo sides left out when there is no TinyGo.
func tinyGoSides(names ...string) []string {
	if _, ok := testdata.TinyGo(); ok {
		return names
	}
	var out []string
	for _, name := range names {
		if !strings.HasSuffix(name, ", TinyGo") {
			out = append(out, name)
		}
	}
	return out
}

func reportSides(t *testing.T, instances int, names ...string) []reportSide {
	t.Helper()
	module := func(opt wasmrules.Options, wasm []byte) *wasmrules.Module {
		opt.Instances, opt.Log = instances, quietLog
		mod, err := newRuntime(t, opt).Load(context.Background(), "draughts", wasm)
		if err != nil {
			t.Fatal(err)
		}
		return mod
	}
	var out []reportSide
	for _, name := range names {
		var rules state.Ruleset
		var mod *wasmrules.Module
		switch name {
		case sideLinked:
			rules = wasmrules.Native(draughts.New)
		case sideInProcess:
			rules = wasmrules.Encoded(draughts.New)
		case sideAcross:
			mod = module(wasmrules.Options{}, testdata.BuildWasm(t, testdata.DraughtsWasm, testdata.NoBot))
		case sideInside:
			mod = module(wasmrules.Options{}, testdata.BuildWasm(t, testdata.DraughtsWasm))
		case sideNoDeadline:
			mod = module(wasmrules.Options{Deadline: -1}, testdata.BuildWasm(t, testdata.DraughtsWasm))
		case sideAcrossTinyGo:
			mod = module(wasmrules.Options{}, testdata.BuildWasmTinyGo(t, testdata.DraughtsWasm, testdata.NoBot))
		case sideInsideTinyGo:
			mod = module(wasmrules.Options{}, testdata.BuildWasmTinyGo(t, testdata.DraughtsWasm))
		default:
			t.Fatalf("no side %q", name)
		}
		if mod != nil {
			rules = mod.Rules()
		}
		cat := catalog.NewCatalog()
		if err := cat.RegisterModule(testdata.DraughtsWith(rules)); err != nil {
			t.Fatal(err)
		}
		game, _ := cat.Get(draughts.ID)
		out = append(out, reportSide{name: name, cat: cat, game: game, mod: mod})
	}
	return out
}

// reportPositions are positions of the 8×8 game with a choice to make: ten,
// twenty and thirty selections into three games.
func reportPositions(t *testing.T) []*state.BoardState {
	t.Helper()
	game := nativeGames(t)[draughts.ID]
	var out []*state.BoardState
	for seed := uint64(1); seed <= 3; seed++ {
		steps := playOut(t, game, seed, int64(seed), 30)
		for _, at := range []int{10, 20, 30} {
			if at >= len(steps) {
				continue
			}
			st := state.NewBoardState()
			if err := st.UnmarshalJSON([]byte(steps[at].Position)); err != nil {
				t.Fatal(err)
			}
			if len(state.LegalSelections(game, st, state.StartSeat(game, st))) > 1 {
				out = append(out, st)
			}
		}
	}
	return out
}

// reportModule is what a module costs to have: its size, the time to load
// it (compiling it and making its first instance), the time to make a further
// instance, and the memory of one instance once it has played a game.
func reportModule(t *testing.T) {
	builds := []struct {
		name string
		wasm []byte
	}{{"Go", testdata.BuildWasm(t, testdata.DraughtsWasm)}}
	if _, ok := testdata.TinyGo(); ok {
		builds = append(builds, struct {
			name string
			wasm []byte
		}{"TinyGo", testdata.BuildWasmTinyGo(t, testdata.DraughtsWasm)})
	}
	for _, b := range builds {
		start := time.Now()
		mod, err := newRuntime(t, wasmrules.Options{Log: quietLog}).Load(context.Background(), "draughts", b.wasm)
		if err != nil {
			t.Fatal(err)
		}
		loaded := time.Since(start)
		// A second load of the same module is not compiled again: what is left
		// is making its instance.
		start = time.Now()
		if _, err := newRuntime(t, wasmrules.Options{Log: quietLog}).Load(context.Background(), "draughts", b.wasm); err != nil {
			t.Fatal(err)
		}
		instance := time.Since(start)
		playOut(t, games(t, mod.Rules())[draughts.ID], 1, 1, 300)
		t.Logf("%-7s %5d KB, loads in %s, a further instance in %s, an instance holds %.1f MiB after a game",
			b.name, len(b.wasm)>>10, loaded.Round(time.Millisecond), instance.Round(10*time.Microsecond), float64(mod.Memory())/(1<<20))
	}
}

// reportSearch is positions searched per second, by each bot that searches.
func reportSearch(t *testing.T) {
	positions := reportPositions(t)
	type botRun struct {
		name string
		make func() bots.Bot
	}
	runs := []botRun{
		{"tactical", func() bots.Bot { return bots.NewTactical(1) }},
		{"minimax, 2000 positions", func() bots.Bot { return bots.NewMinimax(bots.MinimaxOptions{Seed: 1, NodeBudget: 2000}) }},
		{"mcts, 1 s (playouts)", func() bots.Bot { return bots.NewMCTS(bots.MCTSOptions{Seed: 1, Budget: time.Second}) }},
	}
	for _, side := range reportSides(t, 0, tinyGoSides(sideLinked, sideInProcess, sideAcross, sideInside, sideNoDeadline, sideAcrossTinyGo, sideInsideTinyGo)...) {
		for _, run := range runs {
			var took time.Duration
			visits := 0
			for _, st := range positions {
				seat := state.StartSeat(side.game, st)
				legal := state.LegalSelections(side.game, st, seat)
				b := run.make()
				start := time.Now()
				if ev, ok := b.(bots.Evaluator); ok {
					e, err := ev.Evaluate(context.Background(), side.game, st, seat, legal)
					if err != nil {
						t.Fatal(err)
					}
					visits += e.Visits
				} else if _, err := b.Choose(context.Background(), side.game, st, seat, legal); err != nil {
					t.Fatal(err)
				}
				took += time.Since(start)
			}
			per := took / time.Duration(len(positions))
			if visits == 0 {
				t.Logf("%-19s %-24s %10s a decision", side.name, run.name, per.Round(10*time.Microsecond))
				continue
			}
			t.Logf("%-19s %-24s %10s a decision, %8.0f a second", side.name, run.name,
				per.Round(10*time.Microsecond), float64(visits)/took.Seconds())
		}
	}
}

// reportBatch is a practice batch's throughput and memory, with the module's
// pool at the batch's worker count.
func reportBatch(t *testing.T) {
	workers := runtime.NumCPU() / 2
	spec := batch.Spec{
		Base: draughts.ID, Games: 2 * workers, Seed: 1, Rotate: true, MaxPlies: 200, Workers: workers,
		Players: []batch.Player{{Bot: "mcts", MCTSIterations: 30}, {Bot: "minimax", MinimaxNodes: 500}},
	}
	for _, side := range reportSides(t, workers, tinyGoSides(sideLinked, sideAcross, sideInside, sideInsideTinyGo)...) {
		base := rss()
		var peak atomic.Int64
		stop := make(chan struct{})
		var wg sync.WaitGroup
		wg.Go(func() {
			tick := time.NewTicker(50 * time.Millisecond)
			defer tick.Stop()
			for {
				select {
				case <-stop:
					return
				case <-tick.C:
					if r := rss(); r > peak.Load() {
						peak.Store(r)
					}
				}
			}
		})
		plies := 0
		var mu sync.Mutex
		start := time.Now()
		err := batch.Run(context.Background(), side.cat, spec, func(r batch.Record) {
			mu.Lock()
			plies += r.Plies
			mu.Unlock()
		}, nil)
		took := time.Since(start)
		close(stop)
		wg.Wait()
		if err != nil {
			t.Fatal(err)
		}
		line := fmt.Sprintf("%-14s %d games on %d workers in %s: %.1f games a minute, %.0f plies a second; process grew by %d MiB at most",
			side.name, spec.Games, workers, took.Round(time.Millisecond), float64(spec.Games)/took.Minutes(),
			float64(plies)/took.Seconds(), max(peak.Load()-base, 0)>>20)
		if side.mod != nil {
			line += fmt.Sprintf("; %d instances hold %d MiB", side.mod.Instances(), side.mod.Memory()>>20)
		}
		t.Log(line)
	}
}

// rss is the process's resident memory, in bytes.
func rss() int64 {
	b, err := os.ReadFile("/proc/self/statm")
	if err != nil {
		return 0
	}
	f := strings.Fields(string(b))
	if len(f) < 2 {
		return 0
	}
	pages, _ := strconv.ParseInt(f[1], 10, 64)
	return pages * int64(os.Getpagesize())
}

// reportStrength plays the MCTS bot on one side against the same bot on
// another, each with the same time a move, and the seats alternated. The
// game itself is played on the rules linked in.
func reportStrength(t *testing.T) {
	games := 20
	if n, err := strconv.Atoi(os.Getenv("BINGLE_WASM_REPORT_GAMES")); err == nil && n > 0 {
		games = n
	}
	const budget = 250 * time.Millisecond
	sides := reportSides(t, 0, tinyGoSides(sideLinked, sideAcross, sideInside, sideInsideTinyGo)...)
	referee := sides[0].game
	pairs := [][2]reportSide{{sides[1], sides[0]}, {sides[2], sides[0]}, {sides[2], sides[1]}}
	if len(sides) > 3 {
		pairs = append(pairs, [2]reportSide{sides[3], sides[0]}, [2]reportSide{sides[3], sides[2]})
	}
	for _, pair := range pairs {
		type result struct{ wins, losses, draws, plies int }
		results := make([]result, games)
		var wg sync.WaitGroup
		slots := make(chan struct{}, runtime.NumCPU()/2)
		for g := range games {
			wg.Go(func() {
				slots <- struct{}{}
				defer func() { <-slots }()
				// The first side takes seat 1 in even games and seat 2 in odd ones.
				firstSeat := 1 + g%2
				st, err := state.InitialStateSeeded(referee, uint64(g+1))
				if err != nil {
					t.Error(err)
					return
				}
				seat := state.StartSeat(referee, st)
				bot := map[int]bots.Bot{
					1: bots.NewMCTS(bots.MCTSOptions{Seed: int64(2*g + 1), Budget: budget}),
					2: bots.NewMCTS(bots.MCTSOptions{Seed: int64(2*g + 2), Budget: budget}),
				}
				r := &results[g]
				for ply := 0; ply < 200; ply++ {
					searched := pair[1].game
					if seat == firstSeat {
						searched = pair[0].game
					}
					legal := state.LegalSelections(referee, st, seat)
					mv, err := bot[seat].Choose(context.Background(), searched, st, seat, legal)
					if err != nil {
						t.Error(err)
						return
					}
					res, err := state.Apply(referee, st, seat, mv)
					if err != nil {
						t.Error(err)
						return
					}
					r.plies++
					if res.GameEnded {
						switch {
						case res.WinnerSeat == nil:
							r.draws++
						case *res.WinnerSeat == firstSeat:
							r.wins++
						default:
							r.losses++
						}
						return
					}
					st, seat = res.NewState, res.NextImpulseSeat
				}
				r.draws++ // unfinished
			})
		}
		wg.Wait()
		var sum result
		for _, r := range results {
			sum.wins += r.wins
			sum.losses += r.losses
			sum.draws += r.draws
			sum.plies += r.plies
		}
		t.Logf("mcts at %s a move, %s against %s: %d-%d-%d (wins, losses, draws or unfinished) in %d games, %.0f plies a game",
			budget, pair[0].name, pair[1].name, sum.wins, sum.losses, sum.draws, games, float64(sum.plies)/float64(games))
	}
}
