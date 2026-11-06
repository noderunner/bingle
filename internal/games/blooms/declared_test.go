package blooms

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"slices"
	"testing"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/dsl"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/tests/difftest"
)

// Step 1 of converting Blooms to a declaration (docs/dsl.md, "Converting a
// Go ruleset"): declared.yaml, which only this test compiles, plays exactly
// as the ruleset does over a random battery, on every board and with the pie
// rule.

func catalogs(t *testing.T) (ref, cand *catalog.Catalog) {
	return catalogsWith(t, nil, nil)
}

// catalogsWith registers the ruleset and the declaration, each after its
// edit, if any, has changed its definition.
func catalogsWith(t *testing.T, editRef, editCand func(map[string]any)) (ref, cand *catalog.Catalog) {
	t.Helper()
	edit := func(raw []byte, f func(map[string]any)) []byte {
		if f == nil {
			return raw
		}
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatal(err)
		}
		f(m)
		out, err := json.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	ref, cand = catalog.NewCatalog(), catalog.NewCatalog()
	mod := Module()
	mod.Definition = edit(mod.Definition, editRef)
	if err := ref.RegisterModule(mod); err != nil {
		t.Fatal(err)
	}
	guide, err := fs.Sub(guideFS, "guide")
	if err != nil {
		t.Fatal(err)
	}
	def, err := dsl.Load(os.DirFS("."), "declared.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err := cand.RegisterModule(catalog.Module{ID: "blooms", Definition: edit(def, editCand), Assets: guide}); err != nil {
		t.Fatal(err)
	}
	return ref, cand
}

// The extras compared are each seat's captures (the ruleset's data, the
// declaration's pool) and the fenced blooms tinted (the ruleset's view, the
// declaration's end_of_turn tint).
func rulesetExtra(game *catalog.CompiledGame) func(*state.BoardState) string {
	return func(st *state.BoardState) string {
		d, err := dataOf(st)
		if err != nil {
			return err.Error()
		}
		var fenced []string
		for id, tint := range newEng(game, st, d).buildView().Tints {
			if tint == "fenced" {
				fenced = append(fenced, id)
			}
		}
		slices.Sort(fenced)
		return fmt.Sprintf("captured %d–%d, fenced %v", d.Captured[0], d.Captured[1], fenced)
	}
}

func declaredExtra(game *catalog.CompiledGame) func(*state.BoardState) string {
	return func(st *state.BoardState) string {
		var fenced []string
		for id, tint := range state.EndOfTurnTints(game, st) {
			if tint == "fenced" {
				fenced = append(fenced, string(id))
			}
		}
		slices.Sort(fenced)
		return fmt.Sprintf("captured %d–%d, fenced %v", st.Reserve("red", "captured"), st.Reserve("blue", "captured"), fenced)
	}
}

func TestTheDeclarationPlaysAsTheRuleset(t *testing.T) {
	ref, cand := catalogs(t)
	ids := []string{"blooms", "blooms-4", "blooms-6", "blooms-8", "blooms-10"}
	for _, base := range []string{"blooms", "blooms-4"} {
		r, err := ref.Derive(base, map[string]any{catalog.PieOption: true})
		if err != nil {
			t.Fatal(err)
		}
		c, err := cand.Derive(base, map[string]any{catalog.PieOption: true})
		if err != nil || c != r {
			t.Fatalf("derive %s: %q %q %v", base, r, c, err)
		}
		ids = append(ids, r)
	}
	for _, id := range ids {
		t.Run(id, func(t *testing.T) {
			rg, ok := ref.Get(id)
			cg, ok2 := cand.Get(id)
			if !ok || !ok2 {
				t.Fatalf("%s: ruleset %v, declaration %v", id, ok, ok2)
			}
			opt := difftest.Options{Games: 12, Plies: 600, Sample: 6, Seed: 11}
			if testing.Short() || rg.Board.Len() > 100 {
				opt = difftest.Options{Games: 3, Plies: 120, Sample: 3, Seed: 11}
			}
			if err := difftest.Compare(difftest.Side{Game: rg, Extra: rulesetExtra(rg)}, difftest.Side{Game: cg, Extra: declaredExtra(cg)}, opt); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// Random games end on captures long before the move limit, so a battery with
// an unreachable target and a short limit compares the limit's results.
func TestTheMoveLimitIsDecidedAlike(t *testing.T) {
	ref, cand := catalogsWith(t,
		func(d map[string]any) { d["params"] = map[string]any{"capture_target": 1000, "max_plies": 30} },
		func(d map[string]any) {
			rules := d["win_condition"].(map[string]any)["rules"].([]any)
			rules[0].(map[string]any)["at_least"] = 1000
			rules[1].(map[string]any)["plies"] = 30
		})
	rg, _ := ref.Get("blooms")
	cg, _ := cand.Get("blooms")
	opt := difftest.Options{Games: 12, Plies: 200, Sample: 4, Seed: 5}
	if err := difftest.Compare(difftest.Side{Game: rg, Extra: rulesetExtra(rg)}, difftest.Side{Game: cg, Extra: declaredExtra(cg)}, opt); err != nil {
		t.Fatal(err)
	}
}
