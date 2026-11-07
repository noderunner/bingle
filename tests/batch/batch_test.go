package batch_test

import (
	"context"
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/noderunner/bingle/internal/app"
	"github.com/noderunner/bingle/internal/batch"
	"github.com/noderunner/bingle/internal/engine/catalog"
	_ "github.com/noderunner/bingle/internal/games/all" // the studies, with the games
	"github.com/noderunner/bingle/internal/games/cthulhuwars"
)

// metricsOf is a Cthulhu Wars record's measurements.
func metricsOf(t *testing.T, r batch.Record) cthulhuwars.GameMetrics {
	t.Helper()
	var m cthulhuwars.GameMetrics
	if err := json.Unmarshal(r.Metrics, &m); err != nil {
		t.Fatalf("game %d: metrics %s: %v", r.Index, r.Metrics, err)
	}
	return m
}

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
			{Pick: "gc", Bot: bot, MCTSIterations: 20}, {Pick: "cc", Bot: bot, MCTSIterations: 20},
			{Pick: "bg", Bot: bot, MCTSIterations: 20}, {Pick: "ys", Bot: bot, MCTSIterations: 20},
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
		m := metricsOf(t, r)
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
			plies += r.Plies
			if r.Winner != 0 {
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
		tuned.Players[i].Tuning = cthulhuwars.Study{}.Shipped(tuned.Players[i].Pick)
	}
	a, b := collect(t, cat, plain), collect(t, cat, tuned)
	for i := range a {
		if !reflect.DeepEqual(a[i].Metrics, b[i].Metrics) {
			t.Fatalf("game %d differs with the shipped tuning spelled out", i)
		}
	}
	// A real shift plays differently.
	for i := range tuned.Players {
		tuned.Players[i].Tuning = batch.Tuning{"end_turn": -40, "battle": 30}
	}
	c := collect(t, cat, tuned)
	same := true
	for i := range a {
		same = same && a[i].Plies == c[i].Plies
	}
	if same {
		t.Error("a large shift changed nothing")
	}
}

// A batch names no game: a game without a study is batched by its outcome,
// and picks, options and tunings, which only a study gives meaning to, are
// refused for it.
func TestAGameWithoutAStudy(t *testing.T) {
	cat := catalog.NewCatalog()
	for _, m := range app.BuiltinModules() {
		if err := cat.RegisterModule(m); err != nil {
			t.Fatal(err)
		}
	}
	plain := batch.Spec{Base: "hex-7", Games: 2, Seed: 1, Workers: 2, Players: []batch.Player{{Bot: "random"}, {Bot: "random"}}}
	recs := collect(t, cat, plain)
	if len(recs) != 2 || len(recs[0].Metrics) != 0 {
		t.Fatalf("records %+v", recs)
	}
	hex, _ := cat.Get("hex-7")
	rep, err := batch.BuildReport(hex, recs)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(rep)
	if strings.Contains(string(raw), `"factions"`) || rep.Games != 2 {
		t.Errorf("a plain game's report: %s", raw)
	}
	for name, edit := range map[string]func(*batch.Spec){
		"a pick":    func(s *batch.Spec) { s.Players[0].Pick, s.Players[1].Pick = "gc", "cc" },
		"an option": func(s *batch.Spec) { s.Options = []string{"hp"} },
		"a tuning":  func(s *batch.Spec) { s.Players[0].Tuning = batch.Tuning{"gate": 1} },
	} {
		s := plain
		s.Players = append([]batch.Player(nil), plain.Players...)
		edit(&s)
		if err := s.Validate(cat); err == nil {
			t.Errorf("%s was accepted for Hex", name)
		}
	}
}

// Cthulhu Wars' study checks its cults and options, and a spec saved with
// high_priests (before options were general) still asks for them.
func TestCthulhuWarsTables(t *testing.T) {
	cat := newCatalog(t)
	s := spec("random", 1)
	s.Options = []string{"hp"}
	if err := s.Validate(cat); err != nil {
		t.Fatalf("the High Priests: %v", err)
	}
	for name, edit := range map[string]func(*batch.Spec){
		"an unknown cult":    func(s *batch.Spec) { s.Players[0].Pick = "xx" },
		"a cult twice":       func(s *batch.Spec) { s.Players[1].Pick = s.Players[0].Pick },
		"an unknown option":  func(s *batch.Spec) { s.Options = []string{"azathoth"} },
		"some seats unnamed": func(s *batch.Spec) { s.Players[0].Pick = "" },
		"priests without cults": func(s *batch.Spec) {
			for i := range s.Players {
				s.Players[i].Pick = ""
			}
		},
	} {
		c := spec("random", 1)
		c.Options = []string{"hp"}
		edit(&c)
		if err := c.Validate(cat); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
	var old batch.Spec
	if err := json.Unmarshal([]byte(`{"base":"cthulhu-wars","high_priests":true,"players":[{"faction":"gc","bot":"random"}]}`), &old); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(old.Options, []string{"hp"}) || old.Players[0].Pick != "gc" {
		t.Errorf("an old spec reads as %+v", old)
	}
}
