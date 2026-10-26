package courtfall_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/envs"
	"github.com/noderunner/bingle/internal/games/courtfall"
)

func envManager(t testing.TB) *envs.Manager {
	t.Helper()
	cat := catalog.NewCatalog()
	if err := cat.RegisterModule(courtfall.Module()); err != nil {
		t.Fatal(err)
	}
	return envs.NewManager(cat)
}

func TestEnvironmentsPlayCourtfallWithPlanesAndActionIDs(t *testing.T) {
	m := envManager(t)
	ctx := context.Background()
	obs, err := m.Reset(ctx, envs.ResetOptions{GameDefID: "courtfall", Opponent: "random", Seed: 5, Planes: true, ActionIDs: true})
	if err != nil {
		t.Fatal(err)
	}
	if obs.Planes == nil || obs.Planes.Shape != [3]int{56, 7, 7} {
		t.Fatalf("planes = %+v", obs.Planes)
	}
	if obs.NumActions != 1823 || len(obs.LegalActionIDs) != len(obs.LegalMoves) || len(obs.LegalMoves) == 0 {
		t.Fatalf("actions: num %d legal %d ids %d", obs.NumActions, len(obs.LegalMoves), len(obs.LegalActionIDs))
	}
	spec, err := m.Spec("courtfall", false, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !spec.Actions.Fixed || spec.Actions.Count != 1823 || spec.Planes == nil || spec.Planes.Shape != [3]int{56, 7, 7} {
		t.Errorf("spec = %+v / %+v", spec.Actions, spec.Planes)
	}
	// Play a hundred decisions through the environment.
	for i := 0; i < 100 && !obs.Done; i++ {
		obs, err = m.Step(ctx, obs.EnvID, obs.LegalMoves[0])
		if err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
		if !obs.Done && !obs.YourTurn {
			t.Fatalf("step %d: the agent should be asked to move again", i)
		}
	}
	// The observation never carries the generator.
	var doc struct {
		Ext struct {
			RNG uint64 `json:"rng"`
		} `json:"ext"`
	}
	raw, _ := json.Marshal(obs.State)
	if err := json.Unmarshal(raw, &doc); err != nil || doc.Ext.RNG != 0 {
		t.Errorf("state leaks the generator: %v %d", err, doc.Ext.RNG)
	}
	if !strings.Contains(obs.BoardText, "Phase:") {
		t.Errorf("board text = %q", obs.BoardText)
	}
}

func TestSeededEnvironmentsReplayExactly(t *testing.T) {
	m := envManager(t)
	ctx := context.Background()
	run := func() []string {
		obs, err := m.Reset(ctx, envs.ResetOptions{GameDefID: "courtfall", Opponent: "tactical", Seed: 42})
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 80 && !obs.Done; i++ {
			// Always the last legal move: deterministic given the position.
			obs, err = m.Step(ctx, obs.EnvID, obs.LegalMoves[len(obs.LegalMoves)-1])
			if err != nil {
				t.Fatal(err)
			}
		}
		return obs.History
	}
	a, b := run(), run()
	if strings.Join(a, " ") != strings.Join(b, " ") {
		t.Fatalf("seeded runs diverged:\n%v\n%v", a, b)
	}
}

func TestForkedEnvironmentsDoNotShareDice(t *testing.T) {
	m := envManager(t)
	ctx := context.Background()
	obs, err := m.Reset(ctx, envs.ResetOptions{GameDefID: "courtfall", Opponent: "self", Seed: 9})
	if err != nil {
		t.Fatal(err)
	}
	// Reach a strike: play first legal non-contest moves until strike is on offer.
	for i := 0; i < 400; i++ {
		var strike string
		for _, n := range obs.LegalMoves {
			if strings.Contains(n, "x") && strings.HasSuffix(n, "/1") && n[0] >= 'B' && n[0] <= 'R' {
				strike = n
			}
		}
		if strike != "" {
			obs, err = m.Step(ctx, obs.EnvID, strike)
			if err != nil {
				t.Fatal(err)
			}
			break
		}
		pick := obs.LegalMoves[len(obs.LegalMoves)/2]
		for _, n := range obs.LegalMoves {
			if !strings.ContainsAny(n, "x*^") && n != "clear" {
				pick = n
				break
			}
		}
		obs, err = m.Step(ctx, obs.EnvID, pick)
		if err != nil {
			t.Fatal(err)
		}
	}
	if !strings.Contains(strings.Join(obs.LegalMoves, " "), "respond/") {
		t.Skip("no strike was reached")
	}
	// Two forks answering the same way roll independent dice, so across a
	// few forks the results are not all identical.
	results := map[string]bool{}
	for i := 0; i < 8; i++ {
		f, err := m.Fork(obs.EnvID)
		if err != nil {
			t.Fatal(err)
		}
		out, err := m.Step(ctx, f.EnvID, "respond/0")
		if err != nil {
			t.Fatal(err)
		}
		results[strings.Join(out.History[len(out.History)-1:], "")+out.BoardText[strings.Index(out.BoardText, "Last move"):]] = true
	}
	if len(results) < 2 {
		t.Errorf("eight forks all rolled identical dice: %d distinct outcomes", len(results))
	}
}
