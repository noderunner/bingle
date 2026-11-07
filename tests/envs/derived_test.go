package envs_test

import (
	"context"
	"testing"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/envs"
	"github.com/noderunner/bingle/tests/testdata/council"
)

// A table of other houses than the game's own is a game of its own, named by
// its id: training environments take it like any other game.
func TestDerivedGamesWorkAsEnvironments(t *testing.T) {
	cat := catalog.NewCatalog()
	if err := cat.RegisterModule(council.Module()); err != nil {
		t.Fatal(err)
	}
	m := envs.NewManager(cat)
	ctx := context.Background()
	const id = "test-council~owl-stag-wolf-crow-court"

	spec, err := m.Spec(id, false, 0)
	if err != nil {
		t.Fatalf("spec of %s: %v", id, err)
	}
	if !spec.Actions.Fixed || spec.Actions.Count != 13 {
		t.Errorf("the action space of %s: %+v", id, spec.Actions)
	}
	obs, err := m.Reset(ctx, envs.ResetOptions{GameDefID: id, Opponent: "random", Seed: 5, Planes: true, ActionIDs: true})
	if err != nil {
		t.Fatalf("reset %s: %v", id, err)
	}
	// The Owl (seat 1) begins: it may claim the North or go to its three neighbours.
	if !obs.YourImpulse || len(obs.LegalMoves) != 4 {
		t.Fatalf("first decision: your turn %v, %d moves %v", obs.YourImpulse, len(obs.LegalMoves), obs.LegalMoves)
	}
	for i := 0; i < 40 && !obs.Done; i++ {
		if len(obs.LegalActionIDs) == 0 {
			t.Fatalf("no legal action ids at step %d", i)
		}
		obs, err = m.StepAction(ctx, obs.EnvID, obs.LegalActionIDs[0])
		if err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
	}
	if obs.Planes == nil {
		t.Error("no planes in the observation")
	}

	// An unknown combination is an unknown game.
	if _, err := m.Reset(ctx, envs.ResetOptions{GameDefID: "test-council~crow-crow-wolf-ram"}); err == nil {
		t.Error("a house twice must be refused")
	}
}
