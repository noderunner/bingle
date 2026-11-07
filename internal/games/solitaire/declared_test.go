package solitaire

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/noderunner/bingle/internal/engine/cards"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/dsl"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/tests/difftest"
)

// The patience vocabulary (ADR 0122) against the ruleset it is to replace:
// each of Solitaire's patience games, declared, plays exactly as the ruleset
// does over a random battery.

// patienceOf is the patience block of each of the ruleset's games.
var patienceOf = map[string]map[string]any{
	"solitaire":        klondike(1),
	"solitaire-draw-3": klondike(3),
	"spider":           spider(),
	"spider-2":         spider(),
	"spider-1":         spider(),
	"freecell": {
		"columns": map[string]any{"count": 8, "deal": map[string]any{"sizes": []int{7, 7, 7, 7, 6, 6, 6, 6}, "down": 0},
			"build": map[string]any{"suit": "alternate"}, "lift_limit": "cells"},
		"foundations": map[string]any{"count": 4},
		"cells":       4,
		"finish":      true,
	},
}

func klondike(draw int) map[string]any {
	return map[string]any{
		"columns": map[string]any{"count": 7, "deal": map[string]any{"sizes": []int{1, 2, 3, 4, 5, 6, 7}},
			"build": map[string]any{"suit": "alternate"}, "empty": "king"},
		"foundations": map[string]any{"count": 4, "back": true},
		"stock":       map[string]any{"draw": draw, "to": "waste", "redeals": "unlimited"},
		"finish":      true,
	}
}

func spider() map[string]any {
	return map[string]any{
		"columns": map[string]any{"count": 10, "deal": map[string]any{"sizes": []int{6, 6, 6, 6, 5, 5, 5, 5, 5, 5}}, "lift": "same-suit"},
		"stock":   map[string]any{"deal": "row", "columns": "all", "unless_empty": true},
		"off":     map[string]any{"run": "king-to-ace", "suit": "same"},
	}
}

// declared turns each of the ruleset's games into a declaration.
func declared(t *testing.T) map[string]*catalog.CompiledGame {
	t.Helper()
	docs, err := dsl.ExpandVariants(definition())
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]*catalog.CompiledGame{}
	for _, doc := range docs {
		var d map[string]any
		if err := json.Unmarshal(doc, &d); err != nil {
			t.Fatal(err)
		}
		id := d["id"].(string)
		block, ok := patienceOf[id]
		if !ok {
			continue // Bowling
		}
		delete(d, "engine")
		delete(d, "params")
		delete(d, "variants")
		d["patience"] = block
		d["win_condition"] = map[string]any{"type": "patience"}
		raw, err := json.Marshal(d)
		if err != nil {
			t.Fatal(err)
		}
		cat := catalog.NewCatalog()
		if err := cat.Register(id, raw, nil); err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		g, _ := cat.Get(id)
		out[id] = g
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

func TestTheVocabularyPlaysAsTheRuleset(t *testing.T) {
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
