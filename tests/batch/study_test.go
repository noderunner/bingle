package batch_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/noderunner/bingle/internal/batch"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/tune"
	"github.com/noderunner/bingle/tests/testdata/council"
)

func councilCatalog(t *testing.T) *catalog.Catalog {
	t.Helper()
	cat := catalog.NewCatalog()
	if err := cat.RegisterModule(council.Module()); err != nil {
		t.Fatal(err)
	}
	return cat
}

func councilSpec(games int) batch.Spec {
	return batch.Spec{
		Base: council.ID, Options: []string{"court"}, Games: games, Seed: 3, Rotate: true, Workers: 2,
		Players: []batch.Player{{Pick: "owl", Bot: "random"}, {Pick: "stag", Bot: "random"}, {Pick: "wolf", Bot: "random"}, {Pick: "crow", Bot: "random"}},
	}
}

// A game's study (Council's, tests/testdata/council) gives its batches their
// table, measures each game and adds its own part to the report.
func TestAStudyPicksMeasuresAndReports(t *testing.T) {
	cat := councilCatalog(t)
	recs := collect(t, cat, councilSpec(4))
	if len(recs) != 4 {
		t.Fatalf("%d games", len(recs))
	}
	for _, r := range recs {
		if !strings.HasPrefix(r.GameID, council.ID+catalog.DerivedSep) || !strings.HasSuffix(r.GameID, "-court") || len(r.Seating) != 4 {
			t.Fatalf("game %d: %s, seating %v", r.Index, r.GameID, r.Seating)
		}
		var m council.Metrics
		if err := json.Unmarshal(r.Metrics, &m); err != nil || len(m.Claims) != 4 {
			t.Fatalf("game %d: metrics %s (%v)", r.Index, r.Metrics, err)
		}
	}
	// The seats are rotated: the Owl does not always sit first.
	if recs[0].Seating[0] == recs[1].Seating[0] {
		t.Errorf("seats were not rotated: %v %v", recs[0].Seating, recs[1].Seating)
	}
	g, _ := cat.Get(council.ID)
	rep, err := batch.BuildReport(g, recs)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(rep)
	var out struct {
		Games  int `json:"games"`
		Houses []struct {
			Key   string `json:"key"`
			Games int    `json:"games"`
		} `json:"houses"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || out.Games != 4 || len(out.Houses) != 4 || out.Houses[0].Games != 4 {
		t.Fatalf("report %s (%v)", raw, err)
	}
	// What the study refuses, the batch refuses.
	for name, edit := range map[string]func(*batch.Spec){
		"an unknown house":  func(s *batch.Spec) { s.Players[0].Pick = "bear" },
		"a house twice":     func(s *batch.Spec) { s.Players[1].Pick = "owl" },
		"an unknown option": func(s *batch.Spec) { s.Options = []string{"dragons"} },
	} {
		s := councilSpec(1)
		s.Players = append([]batch.Player(nil), s.Players...)
		edit(&s)
		if err := s.Validate(cat); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}

// A tuning search runs on any study with knobs: Council's one.
func TestATuningSearchOnAStudy(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	cat := councilCatalog(t)
	res, err := tune.Run(context.Background(), cat, tune.Config{
		Base: council.ID, Lineup: []string{"owl", "stag", "wolf", "crow"}, Pick: "owl", Bot: "random",
		Generations: 1, Population: 2, GamesPerEval: 4, ConfirmGames: 4, ValidateGames: 4, Seed: 3, Workers: 2,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Steps) != 1 || res.Validation == nil || res.Validation.Tuned.Games != 4 {
		t.Fatalf("result = %+v", res)
	}
	for k := range res.Best {
		if k != "claim_first" {
			t.Errorf("a knob the study does not have: %s", k)
		}
	}
}
