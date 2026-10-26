package batch_test

import (
	"context"
	"reflect"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/noderunner/bingle/internal/batch"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/games/cthulhuwars"
)

func newCatalog(t *testing.T) *catalog.Catalog {
	t.Helper()
	cat := catalog.NewCatalog()
	if err := cat.RegisterModule(cthulhuwars.Module()); err != nil {
		t.Fatal(err)
	}
	return cat
}

func spec(bot string, games int) batch.Spec {
	return batch.Spec{
		Base: "cthulhu-wars", Games: games, Seed: 7, Rotate: true, EventGames: 1, Workers: 4,
		Players: []batch.Player{
			{Faction: "gc", Bot: bot, MCTSIterations: 20}, {Faction: "cc", Bot: bot, MCTSIterations: 20},
			{Faction: "bg", Bot: bot, MCTSIterations: 20}, {Faction: "ys", Bot: bot, MCTSIterations: 20},
		},
	}
}

func collect(t *testing.T, cat *catalog.Catalog, s batch.Spec) []batch.Record {
	t.Helper()
	var mu sync.Mutex
	var recs []batch.Record
	err := batch.Run(context.Background(), cat, s, func(r batch.Record) {
		mu.Lock()
		recs = append(recs, r)
		mu.Unlock()
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	sort.Slice(recs, func(i, j int) bool { return recs[i].Index < recs[j].Index })
	return recs
}

// The same spec replays the same games, and the records are filled in.
func TestBatchIsDeterministicAndRecordsMetrics(t *testing.T) {
	cat := newCatalog(t)
	s := spec("tactical", 4)
	a, b := collect(t, cat, s), collect(t, cat, s)
	if len(a) != 4 {
		t.Fatalf("played %d games, want 4", len(a))
	}
	if !reflect.DeepEqual(a, b) {
		t.Fatal("the same seed produced different records")
	}
	for _, r := range a {
		m := r.Metrics
		if m.Plies < 20 || len(m.Seats) != 4 || m.EndReason == "" {
			t.Fatalf("game %d: odd record %+v", r.Index, m)
		}
		for _, sm := range m.Seats {
			if len(sm.Actions) == 0 || sm.Built["cultist"] == 0 && sm.GatesBuilt == 0 {
				t.Errorf("game %d seat %d: nothing recorded: %+v", r.Index, sm.Seat, sm)
			}
			if len(sm.Areas) == 0 {
				t.Errorf("game %d seat %d: no areas sampled", r.Index, sm.Seat)
			}
		}
	}
	if len(a[0].Events) == 0 || len(a[1].Events) != 0 {
		t.Errorf("only the first game keeps events: %d, %d", len(a[0].Events), len(a[1].Events))
	}
	if a[0].Seating[0] == a[1].Seating[0] {
		t.Errorf("seats were not rotated: %v %v", a[0].Seating, a[1].Seating)
	}
}

// Throughput is logged so the wizard's estimates can be based on it.
func TestBatchThroughput(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	cat := newCatalog(t)
	for _, bot := range []string{"tactical", "minimax", "mcts"} {
		s := spec(bot, 8)
		start := time.Now()
		recs := collect(t, cat, s)
		el := time.Since(start)
		plies, wins := 0, 0
		for _, r := range recs {
			plies += r.Metrics.Plies
			if r.Metrics.Winner != 0 {
				wins++
			}
		}
		t.Logf("%-8s %d games in %v (%.1f games/min on %d workers), %d plies/game, %d decided",
			bot, len(recs), el.Round(time.Millisecond), float64(len(recs))/el.Minutes(), s.Workers, plies/len(recs), wins)
	}
}

// Spelling out a cult's shipped tuning plays the same games as leaving it unset.
func TestZeroTuningChangesNothing(t *testing.T) {
	cat := newCatalog(t)
	plain := spec("tactical", 3)
	tuned := spec("tactical", 3)
	for i := range tuned.Players {
		shipped := cthulhuwars.ShippedTuning(tuned.Players[i].Faction)
		tuned.Players[i].Tuning = &shipped
	}
	a, b := collect(t, cat, plain), collect(t, cat, tuned)
	for i := range a {
		if !reflect.DeepEqual(a[i].Metrics, b[i].Metrics) {
			t.Fatalf("game %d differs with the shipped tuning spelled out", i)
		}
	}
	// A real shift plays differently.
	for i := range tuned.Players {
		tuned.Players[i].Tuning = &cthulhuwars.GuideTuning{EndTurn: -40, Battle: 30}
	}
	c := collect(t, cat, tuned)
	same := true
	for i := range a {
		same = same && reflect.DeepEqual(a[i].Metrics.Plies, c[i].Metrics.Plies)
	}
	if same {
		t.Error("a large shift changed nothing")
	}
}
