package solitaire

import (
	"fmt"
	"io/fs"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/noderunner/bingle/internal/engine/cards"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/dsl"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/tests/difftest"
)

// Step 1 of converting Solitaire's games of patience to a declaration
// (docs/dsl.md, "Converting a Go ruleset"): declared.yaml, which only this
// test compiles, plays each of them exactly as the ruleset does over a random
// battery, hints included.

// declared compiles declared.yaml with its variants, by id.
func declared(t *testing.T) map[string]*catalog.CompiledGame {
	t.Helper()
	def, err := dsl.Load(os.DirFS("."), "declared.yaml")
	if err != nil {
		t.Fatal(err)
	}
	guide, err := fs.Sub(guideFS, "guide")
	if err != nil {
		t.Fatal(err)
	}
	cat := catalog.NewCatalog()
	if err := cat.RegisterModule(catalog.Module{ID: "solitaire", Definition: def, Assets: guide}); err != nil {
		t.Fatal(err)
	}
	out := map[string]*catalog.CompiledGame{}
	for _, g := range cat.List() {
		out[g.Definition.ID] = g
	}
	if len(out) != 6 {
		t.Fatalf("the declaration has %d games, want Solitaire's six", len(out))
	}
	return out
}

// table writes every pile with cards, in name order, face-down cards marked.
func table(t *cards.Table) string {
	if t == nil {
		return "no table"
	}
	var keys []string
	for k, z := range t.Zones {
		if z.Len() > 0 {
			keys = append(keys, k)
		}
	}
	slices.Sort(keys)
	var b strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&b, "%s:", k)
		for _, c := range t.Zones[k].Cards {
			if !c.Up {
				b.WriteByte('~')
			}
			b.WriteString(c.ID)
			b.WriteByte(' ')
		}
		b.WriteString("; ")
	}
	return b.String()
}

func rulesetTable(st *state.BoardState) string {
	d, err := dataOf(st)
	if err != nil {
		return err.Error()
	}
	return table(d.Table)
}

func declaredTable(st *state.BoardState) string { return table(st.Cards()) }

func TestTheDeclarationPlaysAsTheRuleset(t *testing.T) {
	ref := catalog.NewCatalog()
	if err := ref.RegisterModule(Module()); err != nil {
		t.Fatal(err)
	}
	for id, cg := range declared(t) {
		t.Run(id, func(t *testing.T) {
			rg, _ := ref.Get(id)
			opt := difftest.Options{Games: 8, Plies: 250, Sample: 6, Seed: 5}
			if testing.Short() {
				opt = difftest.Options{Games: 2, Plies: 80, Sample: 3, Seed: 5}
			}
			if err := difftest.Compare(difftest.Side{Game: rg, Extra: rulesetTable}, difftest.Side{Game: cg, Extra: declaredTable}, opt); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// The hint order the vocabulary gives (ADR 0122) picks the moves Solitaire's
// does, at every position of random games.
func TestTheHintsAgree(t *testing.T) {
	ref := catalog.NewCatalog()
	if err := ref.RegisterModule(Module()); err != nil {
		t.Fatal(err)
	}
	notations := func(g *catalog.CompiledGame, ms []state.Selection) []string {
		var out []string
		for _, m := range ms {
			out = append(out, state.FormatMove(g, m))
		}
		slices.Sort(out)
		return out
	}
	for id, cg := range declared(t) {
		rg, _ := ref.Get(id)
		for seed := uint64(1); seed <= 4; seed++ {
			rs, _ := state.InitialStateSeeded(rg, seed)
			cs, _ := state.InitialStateSeeded(cg, seed)
			for ply := 0; ply < 120; ply++ {
				rl, cl := state.LegalSelections(rg, rs, 1), state.LegalSelections(cg, cs, 1)
				if len(rl) == 0 {
					break
				}
				rh, ch := notations(rg, state.GuidedMoves(rg, rs, 1, rl)), notations(cg, state.GuidedMoves(cg, cs, 1, cl))
				if !slices.Equal(rh, ch) {
					t.Fatalf("%s seed %d ply %d: hints %v against %v", id, seed, ply, rh, ch)
				}
				// Play the first hint on both.
				rm, _ := state.ParseMove(rg, rh[0])
				cm, _ := state.ParseMove(cg, rh[0])
				rr, err1 := state.Apply(rg, rs, 1, rm)
				cr, err2 := state.Apply(cg, cs, 1, cm)
				if err1 != nil || err2 != nil {
					t.Fatalf("%s: %v / %v", id, err1, err2)
				}
				if rr.GameEnded {
					break
				}
				rs, cs = rr.NewState, cr.NewState
			}
		}
	}
}
