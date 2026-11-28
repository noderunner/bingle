// Package load measures how promptly a person's move is answered while bot
// batches run (ADR 0162). It is not in the default suite: it takes a couple
// of minutes and loads every CPU. Run it with
//
//	BINGLE_LOAD=1 go test ./tests/load -run TestPeopleFirst -v -timeout 10m
//
// It plays people's moves through the session layer (each against a quick
// bot, whose reply is in the answer) in three phases, each BINGLE_LOAD_SECONDS
// long (30 by default): with nothing else running, while a batch of searching
// bots plays on every CPU, and while a batch of cheap games does (hundreds of
// records a second for the database's one writer). BINGLE_LOAD_PLAYERS sets the
// people against each of the random and the tactical bot (4), and BINGLE_LOAD_SEARCHERS adds rooms where a person plays MCTS,
// whose replies are live searches of their own (0). It reports each phase's
// answer at the 50th and 99th percentile, the slowest, and the batch's games a
// minute. It only uses what existed before the budget, so the same file runs on
// an older checkout for a before-and-after.
package load

import (
	"context"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/noderunner/bingle/internal/app"
	"github.com/noderunner/bingle/internal/batch"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
	_ "github.com/noderunner/bingle/internal/games/all"
	"github.com/noderunner/bingle/internal/session"
	"github.com/noderunner/bingle/internal/store"
)

func setting(name string, def int) int {
	if n, err := strconv.Atoi(os.Getenv(name)); err == nil && n >= 0 {
		return n
	}
	return def
}

// sample is what one phase measured: moves against a random bot (the move
// itself, with a reply that costs nothing), against a tactical one (a live
// search in the answer), and a page read after each.
type sample struct {
	moves, replies, pages []time.Duration
}

func pct(ds []time.Duration, p float64) time.Duration {
	if len(ds) == 0 {
		return 0
	}
	s := slices.Clone(ds)
	slices.Sort(s)
	return s[min(len(s)-1, int(float64(len(s))*p))]
}

func ms(d time.Duration) string { return fmt.Sprintf("%.1f", float64(d.Microseconds())/1000) }

func TestPeopleFirst(t *testing.T) {
	if os.Getenv("BINGLE_LOAD") == "" {
		t.Skip("set BINGLE_LOAD=1 to measure (a few minutes on every CPU)")
	}
	phase := time.Duration(setting("BINGLE_LOAD_SECONDS", 30)) * time.Second
	players := setting("BINGLE_LOAD_PLAYERS", 4)
	searchers := setting("BINGLE_LOAD_SEARCHERS", 0)

	cat := catalog.NewCatalog()
	for _, m := range app.BuiltinModules() {
		if err := cat.RegisterModule(m); err != nil {
			t.Fatal(err)
		}
	}
	st, err := store.Open(filepath.Join(t.TempDir(), "load.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	mgr := session.NewManager(st, cat)

	// people play until stop closes, each in a room of its own against
	// opponent, and record into rec while it is set.
	var rec atomic.Pointer[sample]
	var mu sync.Mutex
	stop := make(chan struct{})
	var wg sync.WaitGroup
	person := func(i int, opponent string) {
		defer wg.Done()
		ctx := context.Background()
		r := rand.New(rand.NewPCG(uint64(i), 7))
		name := fmt.Sprintf("p%d", i)
		room := ""
		for {
			select {
			case <-stop:
				return
			default:
			}
			if room == "" {
				res, err := mgr.CreateGameWithOptions(ctx, session.CreateOptions{GameDefID: "hex-11", CreatorName: name, Opponent: opponent})
				if err != nil {
					t.Error(err)
					return
				}
				room = res.RoomCode
			}
			game, err := st.GetGame(ctx, room)
			if err != nil || game.Status != "active" || game.ImpulseSeat == nil {
				room = ""
				continue
			}
			compiled, _ := cat.Get(game.GameDefID)
			bs := state.NewBoardState()
			if err := bs.UnmarshalJSON([]byte(game.BoardState)); err != nil {
				t.Error(err)
				return
			}
			legal := state.LegalSelections(compiled, bs, *game.ImpulseSeat)
			if len(legal) == 0 {
				room = ""
				continue
			}
			mv := legal[r.IntN(len(legal))]
			began := time.Now()
			out, err := mgr.Play(ctx, room, name, mv)
			took := time.Since(began)
			if err != nil {
				// A searcher's bot may still be thinking in the background.
				time.Sleep(50 * time.Millisecond)
				continue
			}
			pageBegan := time.Now()
			_, _ = mgr.GetState(ctx, room, nil)
			page := time.Since(pageBegan)
			if s := rec.Load(); s != nil && opponent != "mcts" {
				mu.Lock()
				if opponent == "random" {
					s.moves = append(s.moves, took)
				} else {
					s.replies = append(s.replies, took)
				}
				s.pages = append(s.pages, page)
				mu.Unlock()
			}
			if out.GameEnded {
				room = ""
			}
			// A person thinks before the next move.
			time.Sleep(time.Duration(50+r.IntN(100)) * time.Millisecond)
		}
	}
	for i := range players {
		wg.Add(2)
		go person(i, "random")
		go person(500+i, "tactical")
	}
	for i := range searchers {
		wg.Add(1)
		go person(1000+i, "mcts")
	}

	// running queues a batch on every CPU (the workers ceiling) with the
	// batch manager, which writes its games as the server does, and returns
	// what stops it and counts its games.
	batches := batch.NewManager(st, cat, batch.WithLimits(batch.Limits{Ceilings: batch.Ceilings{Games: 1_000_000}, GamesPerDay: 100_000_000}))
	running := func(spec batch.Spec) (stop func(), done func() int) {
		ctx := context.Background()
		id, err := batches.Start(ctx, "load", spec)
		if err != nil {
			t.Fatal(err)
		}
		stop = func() {
			_ = batches.Cancel(ctx, id)
			for {
				if b, err := batches.Get(ctx, id); err != nil || b.Status != "running" {
					return
				}
				time.Sleep(20 * time.Millisecond)
			}
		}
		done = func() int {
			b, _ := batches.Get(ctx, id)
			return b.Done
		}
		return stop, done
	}
	searching := batch.Spec{Base: "hex-11", Games: 1_000_000, Seed: 1, Workers: runtime.NumCPU(), Rotate: true,
		Players: []batch.Player{{Bot: "mcts", MCTSIterations: 150}, {Bot: "mcts", MCTSIterations: 150}}}
	cheap := batch.Spec{Base: "hex-7", Games: 1_000_000, Seed: 1, Workers: runtime.NumCPU(),
		Players: []batch.Player{{Bot: "random"}, {Bot: "random"}}}

	phases := []struct {
		name string
		spec *batch.Spec
	}{{"idle", nil}, {"searching batch", &searching}, {"cheap batch", &cheap}}
	results := make([]sample, len(phases))
	time.Sleep(2 * time.Second) // let the rooms start
	for i, ph := range phases {
		s := &results[i]
		end, done := func() {}, func() int { return 0 }
		if ph.spec != nil {
			end, done = running(*ph.spec)
			time.Sleep(2 * time.Second) // let it fill the CPUs
		}
		rec.Store(s)
		began, before := time.Now(), done()
		time.Sleep(phase)
		rec.Store(nil)
		games := done() - before
		mins := time.Since(began).Minutes()
		end()
		t.Logf("%-16s move p50 %5s p99 %6s (%4d) | with tactical's reply p50 %6s p99 %6s (%4d) | page p99 %5s ms | batch %6.0f games/min",
			ph.name, ms(pct(s.moves, .5)), ms(pct(s.moves, .99)), len(s.moves), ms(pct(s.replies, .5)), ms(pct(s.replies, .99)), len(s.replies),
			ms(pct(s.pages, .99)), float64(games)/mins)
	}
	close(stop)
	wg.Wait()
	t.Logf("%d CPUs (GOMAXPROCS %d), %d people, %d people against MCTS, %s a phase", runtime.NumCPU(), runtime.GOMAXPROCS(0), players, searchers, phase)
}
