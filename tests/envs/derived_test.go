package envs_test

import (
	"context"
	"testing"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/envs"
	"github.com/noderunner/bingle/internal/games/cthulhuwars"
)

// A table of other cults than the layout's own is a game of its own, named by
// its id: training environments take it like any other game.
func TestDerivedGamesWorkAsEnvironments(t *testing.T) {
	cat := catalog.NewCatalog()
	if err := cat.RegisterModule(cthulhuwars.Module()); err != nil {
		t.Fatal(err)
	}
	m := envs.NewManager(cat)
	ctx := context.Background()
	const id = "cthulhu-wars-5p~ww-sl-gc-cc-ys-hp"

	spec, err := m.Spec(id, false, 0)
	if err != nil {
		t.Fatalf("spec of %s: %v", id, err)
	}
	if !spec.Actions.Fixed || spec.Actions.Count == 0 {
		t.Errorf("the action space of %s is not fixed: %+v", id, spec.Actions)
	}
	obs, err := m.Reset(ctx, envs.ResetOptions{GameDefID: id, Opponent: "random", Seed: 5, Planes: true, ActionIDs: true})
	if err != nil {
		t.Fatalf("reset %s: %v", id, err)
	}
	// The Windwalker (seat 1 here) begins: it chooses a sigil area.
	if !obs.YourTurn || len(obs.LegalMoves) != 2 {
		t.Fatalf("first decision: your turn %v, %d moves %v", obs.YourTurn, len(obs.LegalMoves), obs.LegalMoves)
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
	if _, err := m.Reset(ctx, envs.ResetOptions{GameDefID: "cthulhu-wars-5p~gc-gc-bg-ys-ww"}); err == nil {
		t.Error("a cult twice must be refused")
	}
}
