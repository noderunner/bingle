// Tests of the pie rule as a setup choice (ADR 0056): a definition declares
// whether the game is played with it, the catalog brings the swap action and
// the check box, and the choice of a table is a derived game.

package catalog_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/dsl"
)

// pieDefJSON is a two-seat placement game with the given pie_rule value (and
// any extra lines of the document).
func pieDefJSON(pie, extra string) string {
	rule := ""
	if pie != "" {
		rule = fmt.Sprintf("\"pie_rule\": %q,", pie)
	}
	return fmt.Sprintf(`{
  "id": "pietest",
  "name": "Pie test",
  %s
  %s
  "board": { "generator": { "type": "grid", "width": 5, "height": 5, "adjacency": "orthogonal4" } },
  "seats": [ { "seat": 1, "color": "black" }, { "seat": 2, "color": "white" } ],
  "pieces": { "stone": { "shape": "disc" } },
  "actions": [ { "id": "place", "type": "place", "pieces": ["stone"], "source": "supply" } ],
  "turn_order": { "type": "round-robin", "seats": [1, 2] },
  "win_condition": { "type": "none" }
}`, rule, extra)
}

// pieCatalog registers the document as a module (so derived games can be built
// from it) and returns the catalog.
func pieCatalog(t *testing.T, def string) *catalog.Catalog {
	t.Helper()
	cat := catalog.NewCatalog()
	if err := cat.RegisterModule(catalog.Module{ID: "pietest", Definition: []byte(def)}); err != nil {
		t.Fatalf("register: %v", err)
	}
	return cat
}

func hasSwap(cg *catalog.CompiledGame) bool {
	for _, a := range cg.Actions {
		if a.Type == catalog.ActionSwap {
			return true
		}
	}
	return false
}

func pieOption(cg *catalog.CompiledGame) *dsl.OptionDef {
	for i, o := range cg.Definition.Options {
		if o.ID == catalog.PieOption {
			return &cg.Definition.Options[i]
		}
	}
	return nil
}

// A game played with the pie rule gets the swap action without declaring one,
// and offers its table the choice of leaving it out.
func TestPieRuleStandardBringsTheSwapActionAndTheChoice(t *testing.T) {
	cat := pieCatalog(t, pieDefJSON(catalog.PieStandard, ""))
	cg, ok := cat.Get("pietest")
	if !ok {
		t.Fatal("the game is not in the catalog")
	}
	if !hasSwap(cg) {
		t.Error("a game played with the pie rule has no swap action")
	}
	opt := pieOption(cg)
	if opt == nil {
		t.Fatal("the game offers no pie-rule option")
	}
	if opt.Kind != "bool" || opt.Default != true {
		t.Errorf("the option is %+v, want a check box that starts on", opt)
	}
}

// A game that only offers the rule is played without it until a table asks.
func TestPieRuleOptionalOffersTheChoiceButPlaysWithoutIt(t *testing.T) {
	cat := pieCatalog(t, pieDefJSON(catalog.PieOptional, ""))
	cg, _ := cat.Get("pietest")
	if hasSwap(cg) {
		t.Error("a game that only offers the pie rule starts with the swap action")
	}
	if opt := pieOption(cg); opt == nil || opt.Default == true {
		t.Errorf("the option is %+v, want a check box that starts off", opt)
	}
}

// The table's choice is a game of its own, and a game stored under that id is
// loaded again from the id alone.
func TestPieRuleChoiceMakesADerivedGame(t *testing.T) {
	for _, tc := range []struct {
		declared string
		want     bool
		id       string
	}{
		{catalog.PieStandard, false, "pietest~nopie"},
		{catalog.PieOptional, true, "pietest~pie"},
	} {
		cat := pieCatalog(t, pieDefJSON(tc.declared, ""))
		id, err := cat.Derive("pietest", map[string]any{catalog.PieOption: tc.want})
		if err != nil {
			t.Fatalf("%s: derive: %v", tc.declared, err)
		}
		if id != tc.id {
			t.Fatalf("%s: derived %q, want %q", tc.declared, id, tc.id)
		}
		cg, ok := cat.Get(id)
		if !ok {
			t.Fatalf("%s: %s is not in the catalog", tc.declared, id)
		}
		if hasSwap(cg) != tc.want {
			t.Errorf("%s: %s has swap=%v, want %v", tc.declared, id, hasSwap(cg), tc.want)
		}
		// Nothing is left to choose in a derived game.
		if opt := pieOption(cg); opt != nil {
			t.Errorf("%s: %s still offers the choice", tc.declared, id)
		}
		// A fresh catalog builds it from the id, as a stored game needs.
		fresh := pieCatalog(t, pieDefJSON(tc.declared, ""))
		cg2, ok := fresh.Get(id)
		if !ok || hasSwap(cg2) != tc.want {
			t.Errorf("%s: a fresh catalog could not rebuild %s (ok=%v)", tc.declared, id, ok)
		}
		if _, ok := fresh.Get("pietest~half-pie"); ok {
			t.Error("an unknown suffix was accepted")
		}
	}
}

// Choosing what the game does anyway is the game itself: no derived id, so a
// link to it is the ordinary one.
func TestPieRuleChoosingTheGamesOwnPracticeKeepsThePlainID(t *testing.T) {
	cat := pieCatalog(t, pieDefJSON(catalog.PieStandard, ""))
	id, err := cat.Derive("pietest", map[string]any{catalog.PieOption: true})
	if err != nil || id != "pietest" {
		t.Errorf("derived %q (%v), want the plain id", id, err)
	}
}

// A declaration that cannot work is refused when the game is registered, with
// a message that says what to do instead.
func TestPieRuleIsRefusedWhereItCannotWork(t *testing.T) {
	// The document's own line for the seats and the actions, so that a case
	// can put something else in their place (a repeated key would be ignored).
	const (
		seatsLine   = `"seats": [ { "seat": 1, "color": "black" }, { "seat": 2, "color": "white" } ],`
		actionsLine = `"actions": [ { "id": "place", "type": "place", "pieces": ["stone"], "source": "supply" } ],`
	)
	instead := func(line, with string) string {
		def := pieDefJSON(catalog.PieStandard, "")
		if !strings.Contains(def, line) {
			t.Fatalf("the test document no longer holds %s", line)
		}
		return strings.Replace(def, line, with, 1)
	}
	for name, tc := range map[string]struct{ def, says string }{
		"an unknown value":    {pieDefJSON("sometimes", ""), "optional, standard, on or off"},
		"a swap of its own":   {instead(actionsLine, `"actions": [ { "id": "place", "type": "place", "pieces": ["stone"], "source": "supply" }, { "id": "swap", "type": "swap" } ],`), "brings the action itself"},
		"a direction of play": {instead(seatsLine, `"seats": [ { "seat": 1, "color": "black", "forward": "N" }, { "seat": 2, "color": "white", "forward": "S" } ],`), "facing the wrong way"},
		"three seats":         {instead(seatsLine, `"seats": [ { "seat": 1, "color": "black" }, { "seat": 2, "color": "white" }, { "seat": 3, "color": "red" } ],`), "the pie rule is for two"},
	} {
		cat := catalog.NewCatalog()
		err := cat.RegisterModule(catalog.Module{ID: "pietest", Definition: []byte(tc.def)})
		if err == nil {
			t.Errorf("%s was accepted", name)
			continue
		}
		if !strings.Contains(err.Error(), tc.says) {
			t.Errorf("%s says %q, want it to mention %q", name, err, tc.says)
		}
		if _, ok := cat.Get("pietest"); ok {
			t.Errorf("%s left the game in the catalog", name)
		}
	}
}

// A ruleset game may declare the pie rule too (Blooms does): the catalog
// brings the choice and the derived games, but not the swap action — a ruleset
// builds its own moves, and offering the swap is its own work.
func TestPieRuleARulesetBringsItsOwnSwap(t *testing.T) {
	cat := catalog.NewCatalog()
	// Registration only asks that a ruleset is there; what it is plays no part
	// in what the catalog brings.
	err := cat.RegisterModule(catalog.Module{ID: "pietest", Definition: []byte(pieDefJSON(catalog.PieOptional, `"engine": "ruleset",`)), Rules: true})
	if err != nil {
		t.Fatalf("a ruleset declaring the pie rule was refused: %v", err)
	}
	cg, _ := cat.Get("pietest")
	if hasSwap(cg) {
		t.Error("the catalog gave a ruleset game the generic swap action")
	}
	if opt := pieOption(cg); opt == nil || opt.Default == true {
		t.Errorf("the option is %+v, want a check box that starts off", opt)
	}
	id, err := cat.Derive("pietest", map[string]any{catalog.PieOption: true})
	if err != nil || id != "pietest~pie" {
		t.Fatalf("derive: %q, %v", id, err)
	}
	cg, ok := cat.Get(id)
	if !ok || !catalog.PieRuleOn(cg.Definition) {
		t.Fatalf("%s is not a game played with the pie rule (ok=%v)", id, ok)
	}
	if hasSwap(cg) {
		t.Error("the derived ruleset game has the generic swap action")
	}
	if opt := pieOption(cg); opt != nil {
		t.Errorf("%s still offers the choice", id)
	}
}

// An option the game does not have, or a value that is not true or false, is
// refused with a message rather than quietly ignored.
func TestPieRuleRefusesOptionsItDoesNotKnow(t *testing.T) {
	cat := pieCatalog(t, pieDefJSON(catalog.PieOptional, ""))
	if _, err := cat.Derive("pietest", map[string]any{"komi": 6.5}); err == nil || !strings.Contains(err.Error(), "komi") {
		t.Errorf("an unknown option gave %v", err)
	}
	if _, err := cat.Derive("pietest", map[string]any{catalog.PieOption: "yes"}); err == nil || !strings.Contains(err.Error(), "true or false") {
		t.Errorf("a value that is not a flag gave %v", err)
	}
	// A game with no pie rule at all takes no options.
	plain := pieCatalog(t, pieDefJSON("", ""))
	if _, err := plain.Derive("pietest", map[string]any{catalog.PieOption: true}); err == nil {
		t.Error("a game without a pie rule accepted the option")
	}
}
