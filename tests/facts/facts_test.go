// Package facts_test covers the facts a game's rules report about what a
// selection did (ADR 0129): a declared game reports them from its actions and
// end-of-turn effects, and the catalog holds a game to the facts it lists.
package facts_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

// fenced is a small game of stones placed from a supply, where a group left
// without liberties is taken when the turn ends. Placing reports "placed"
// and a turn that takes something reports "fenced".
func fenced(edit func(map[string]any)) map[string]any {
	def := map[string]any{
		"id": "fc", "name": "Fc", "version": "0.1.0",
		"board": map[string]any{"generator": map[string]any{"type": "grid", "width": 3, "height": 3, "adjacency": "orthogonal4", "labels": "algebraic"}},
		"seats": []any{map[string]any{"seat": 1, "color": "black"}, map[string]any{"seat": 2, "color": "white"}},
		"pieces": map[string]any{
			"black": map[string]any{"color": "black", "code": "B"},
			"white": map[string]any{"color": "white", "code": "W"},
		},
		"setup":       []any{map[string]any{"position": "a1", "piece": "white", "color": "white"}},
		"turn_order":  map[string]any{"type": "round-robin", "seats": []int{1, 2}},
		"actions":     []any{map[string]any{"id": "place", "type": "place", "pieces": []string{"black", "white"}, "source": "supply", "fact": "placed"}},
		"end_of_turn": []any{map[string]any{"effect": "remove-groups", "condition": "no-liberties", "fact": "fenced"}},
		"facts": map[string]any{
			"placed": map[string]any{"description": "A stone is placed."},
			"fenced": map[string]any{"description": "The mover takes a group.", "secret": true},
		},
		"win_condition": map[string]any{"type": "none"},
	}
	if edit != nil {
		edit(def)
	}
	return def
}

func compile(def map[string]any) (*catalog.CompiledGame, error) {
	raw, err := json.Marshal(def)
	if err != nil {
		return nil, err
	}
	cat := catalog.NewCatalog()
	if err := cat.Register(def["id"].(string), raw, nil); err != nil {
		return nil, err
	}
	g, _ := cat.Get(def["id"].(string))
	return g, nil
}

func place(t *testing.T, g *catalog.CompiledGame, st *state.BoardState, seat int, piece, to string) *state.MoveResult {
	t.Helper()
	res, err := state.Apply(g, st, seat, state.Selection{Action: "place", Piece: piece, To: board.PositionID(to)})
	if err != nil {
		t.Fatalf("place %s at %s: %v", piece, to, err)
	}
	return res
}

// An action reports its fact for the mover each time; an effect reports its
// fact only when it takes something, and once however much it takes.
func TestActionsAndEffectsReportTheirFacts(t *testing.T) {
	g, err := compile(fenced(nil))
	if err != nil {
		t.Fatal(err)
	}
	res := place(t, g, state.InitialState(g), 1, "black", "a2")
	if want := []state.Fact{{Name: "placed", Seat: 1}}; !reflect.DeepEqual(res.Facts, want) {
		t.Fatalf("a placement that takes nothing reported %+v, want %+v", res.Facts, want)
	}
	res = place(t, g, res.NewState, 2, "white", "c3")
	res = place(t, g, res.NewState, 1, "black", "b1")
	if want := []state.Fact{{Name: "placed", Seat: 1}, {Name: "fenced", Seat: 1}}; !reflect.DeepEqual(res.Facts, want) {
		t.Fatalf("a placement that fences a1 reported %+v, want %+v", res.Facts, want)
	}
}

// A game without facts reports none, and pays nothing for them.
func TestAGameWithoutFactsReportsNone(t *testing.T) {
	g, err := compile(fenced(func(d map[string]any) {
		delete(d, "facts")
		delete(d["actions"].([]any)[0].(map[string]any), "fact")
		delete(d["end_of_turn"].([]any)[0].(map[string]any), "fact")
	}))
	if err != nil {
		t.Fatal(err)
	}
	if res := place(t, g, state.InitialState(g), 1, "black", "a2"); res.Facts != nil {
		t.Fatalf("reported %+v", res.Facts)
	}
}

func TestTheCatalogHoldsAGameToItsFacts(t *testing.T) {
	facts := func(f map[string]any) func(map[string]any) {
		return func(d map[string]any) { d["facts"] = f }
	}
	for name, c := range map[string]struct {
		edit func(map[string]any)
		want string
	}{
		"undeclared action fact": {func(d map[string]any) { d["actions"].([]any)[0].(map[string]any)["fact"] = "dropped" }, `action "place": fact "dropped"`},
		"undeclared effect fact": {func(d map[string]any) { d["end_of_turn"].([]any)[0].(map[string]any)["fact"] = "taken" }, `end_of_turn 0: fact "taken"`},
		"reserved name":          {facts(map[string]any{"placed": map[string]any{"description": "x"}, "fenced": map[string]any{"description": "x"}, "won": map[string]any{"description": "x"}}), `"won" is the platform's`},
		"bad name":               {facts(map[string]any{"placed": map[string]any{"description": "x"}, "fenced": map[string]any{"description": "x"}, "Battle-Start": map[string]any{"description": "x"}}), "lower case"},
		"no description":         {facts(map[string]any{"placed": map[string]any{"description": "x"}, "fenced": map[string]any{"description": " "}}), "no description"},
	} {
		_, err := compile(fenced(c.edit))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: error %v, want one saying %q", name, err, c.want)
		}
	}
}

// A ruleset game lists its facts too, and the catalog checks the list.
func TestARulesetGamesFactsAreChecked(t *testing.T) {
	def := fenced(func(d map[string]any) {
		d["engine"] = "ruleset"
		d["win_condition"] = map[string]any{"type": "ruleset"}
		delete(d, "actions")
		delete(d, "end_of_turn")
		d["facts"] = map[string]any{"lost": map[string]any{"description": "x"}}
	})
	raw, _ := json.Marshal(def)
	if err := catalog.NewCatalog().RegisterModule(catalog.Module{ID: "fc", Definition: raw, Rules: struct{}{}}); err == nil || !strings.Contains(err.Error(), `"lost"`) {
		t.Fatalf("error %v, want the reserved fact refused", err)
	}
}

func TestDeclaredFactsSplitsOutTheUndeclared(t *testing.T) {
	g, err := compile(fenced(nil))
	if err != nil {
		t.Fatal(err)
	}
	got, bad := state.DeclaredFacts(g, []state.Fact{{Name: "placed", Seat: 1}, {Name: "spellbook", Seat: 2}, {Name: "fenced"}})
	if want := []state.Fact{{Name: "placed", Seat: 1}, {Name: "fenced"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("declared %+v, want %+v", got, want)
	}
	if !reflect.DeepEqual(bad, []string{"spellbook"}) {
		t.Errorf("undeclared %v, want [spellbook]", bad)
	}
	if f, ok := g.Fact("fenced"); !ok || !f.Secret {
		t.Errorf("fenced = %+v, %v; want a secret fact", f, ok)
	}
}
