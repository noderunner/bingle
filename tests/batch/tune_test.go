package batch_test

import (
	"context"
	"testing"

	"github.com/noderunner/bingle/internal/games/cthulhuwars"
	"github.com/noderunner/bingle/internal/tune"
)

// Every knob is an offset: the zero value of a tuning is the guide as written,
// and the knobs' ranges are sane.
func TestGuideKnobsAreOffsets(t *testing.T) {
	var zero cthulhuwars.GuideTuning
	seen := map[string]bool{}
	for _, k := range cthulhuwars.GuideKnobs {
		if seen[k.Name] {
			t.Errorf("knob %q listed twice", k.Name)
		}
		seen[k.Name] = true
		if k.Min >= 0 || k.Max <= 0 {
			t.Errorf("knob %q range [%v, %v] does not contain the default 0", k.Name, k.Min, k.Max)
		}
		tt := zero
		*k.Ptr(&tt) = 1
		if tt == zero {
			t.Errorf("knob %q does not point into the tuning", k.Name)
		}
	}
}

// A tiny run completes: it evaluates, confirms or keeps, and validates.
func TestTunerRunsEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	cat := newCatalog(t)
	res, err := tune.Run(context.Background(), cat, tune.Config{
		Base: "cthulhu-wars-3p", Lineup: []string{"gc", "cc", "bg"}, Faction: "cc",
		Generations: 1, Population: 2, GamesPerEval: 6, ConfirmGames: 6, ValidateGames: 6, Seed: 3, Workers: 6,
		Knobs: []string{"gate", "battle", "end_turn"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Steps) != 1 || res.Validation == nil || res.Validation.Tuned.Games != 6 || res.Validation.Shipped.Games != 6 {
		t.Fatalf("result = %+v", res)
	}
	if res.Steps[0].ParentEval.Games != 6 {
		t.Errorf("parent played %d games", res.Steps[0].ParentEval.Games)
	}
	// Only the chosen knobs may move.
	b := res.Best
	b.Gate, b.Battle, b.EndTurn = 0, 0, 0
	if b != (cthulhuwars.GuideTuning{}) {
		t.Errorf("other knobs moved: %+v", res.Best)
	}
	if _, err := tune.Run(context.Background(), cat, tune.Config{Base: "cthulhu-wars-3p", Lineup: []string{"gc", "cc", "bg"}, Faction: "ys"}, nil); err == nil {
		t.Error("a faction outside the line-up should be refused")
	}
}
