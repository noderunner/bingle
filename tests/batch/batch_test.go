package batch_test

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/noderunner/bingle/internal/app"
	"github.com/noderunner/bingle/internal/batch"
	"github.com/noderunner/bingle/internal/cpu"
	"github.com/noderunner/bingle/internal/engine/catalog"
	_ "github.com/noderunner/bingle/internal/games/all" // the studies, with the games
)

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

// A batch's game is background work (ADR 0162): while live play fills every
// CPU it waits, and it plays as soon as live play leaves room.
func TestABatchGameWaitsForLivePlay(t *testing.T) {
	cat := catalog.NewCatalog()
	for _, m := range app.BuiltinModules() {
		if err := cat.RegisterModule(m); err != nil {
			t.Fatal(err)
		}
	}
	spec := batch.Spec{Base: "hex-7", Games: 1, Seed: 1, Workers: 1, Players: []batch.Player{{Bot: "random"}, {Bot: "random"}}}
	if err := spec.Validate(cat); err != nil {
		t.Fatal(err)
	}
	budget := cpu.Shared()
	var live []func()
	for range budget.Size() {
		live = append(live, budget.Live())
	}
	played := make(chan error, 1)
	go func() {
		_, err := batch.PlayGame(context.Background(), cat, spec, 0)
		played <- err
	}()
	select {
	case <-played:
		t.Fatal("a batch's game played while live play held every CPU")
	case <-time.After(100 * time.Millisecond):
	}
	for _, done := range live {
		done()
	}
	select {
	case err := <-played:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the game did not play once live play was done")
	}
	if st := budget.Stats(); st != (cpu.Stats{}) {
		t.Errorf("the budget holds %+v after the game", st)
	}
	if got, want := batch.DefaultWorkers(), budget.BackgroundShare(); got != want {
		t.Errorf("a batch's default workers are %d, the background share %d", got, want)
	}
}
