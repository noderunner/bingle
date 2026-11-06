package onitama

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/dsl"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/tests/difftest"
)

// Step 1 of converting Onitama to a declaration (docs/dsl.md, "Converting a
// Go ruleset"): declared.yaml, which only this test compiles, plays exactly as
// the ruleset does over a random battery, its cards included.

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
	def, err := dsl.Load(os.DirFS("."), "declared.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err := cand.RegisterModule(catalog.Module{ID: "onitama", Definition: edit(def, editCand), Assets: newGuideAssets()}); err != nil {
		t.Fatal(err)
	}
	return ref, cand
}

// hands says where the five cards in play are: each hand (in any order) and
// the side.
func hands(red, blue []string, side string) string {
	slices.Sort(red)
	slices.Sort(blue)
	return fmt.Sprintf("red %s, blue %s, side %s", strings.Join(red, " "), strings.Join(blue, " "), side)
}

func rulesetCards(st *state.BoardState) string {
	d, err := dataOf(st)
	if err != nil {
		return err.Error()
	}
	return hands(d.Hands[0][:], d.Hands[1][:], d.Side)
}

func declaredCards(st *state.BoardState) string {
	ids := func(zone string, owner int) []string {
		var out []string
		for _, c := range st.Cards().Zone(zone, owner).Cards {
			out = append(out, c.ID)
		}
		return out
	}
	return hands(ids("hand", 1), ids("hand", 2), strings.Join(ids("side", 0), " "))
}

func TestTheDeclarationPlaysAsTheRuleset(t *testing.T) {
	ref, cand := catalogs(t)
	rg, _ := ref.Get("onitama")
	cg, _ := cand.Get("onitama")
	opt := difftest.Options{Games: 30, Plies: 320, Sample: 6, Seed: 3}
	if testing.Short() {
		opt = difftest.Options{Games: 6, Plies: 120, Sample: 3, Seed: 3}
	}
	if err := difftest.Compare(difftest.Side{Game: rg, Extra: rulesetCards}, difftest.Side{Game: cg, Extra: declaredCards}, opt); err != nil {
		t.Fatal(err)
	}
}

// Random games are won long before the move limit, so a battery with a limit
// of 12 compares the draw it ends in.
func TestTheMoveLimitIsDecidedAlike(t *testing.T) {
	ref, cand := catalogsWith(t,
		func(d map[string]any) { d["params"] = map[string]any{"max_plies": 12} },
		func(d map[string]any) {
			rules := d["win_condition"].(map[string]any)["rules"].([]any)
			rules[len(rules)-1].(map[string]any)["plies"] = 12
		})
	rg, _ := ref.Get("onitama")
	cg, _ := cand.Get("onitama")
	opt := difftest.Options{Games: 20, Plies: 40, Sample: 4, Seed: 21}
	if err := difftest.Compare(difftest.Side{Game: rg, Extra: rulesetCards}, difftest.Side{Game: cg, Extra: declaredCards}, opt); err != nil {
		t.Fatal(err)
	}
}
