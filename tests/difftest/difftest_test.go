package difftest_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/noderunner/bingle/internal/app"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/tests/difftest"
	"github.com/noderunner/bingle/tests/testdata"
)

func builtin(t *testing.T) *catalog.Catalog {
	t.Helper()
	cat := catalog.NewCatalog()
	for _, m := range app.BuiltinModules() {
		if err := cat.RegisterModule(m); err != nil {
			t.Fatalf("RegisterModule(%s): %v", m.ID, err)
		}
	}
	return cat
}

// Every game agrees with a second, separately compiled copy of itself. That is
// the floor a conversion stands on: from the same seed, the same moves give
// the same positions, every time, or no comparison would mean anything.
func TestEveryGameAgreesWithItself(t *testing.T) {
	one, two := builtin(t), builtin(t)
	for _, g := range one.List() {
		id := g.Definition.ID
		t.Run(id, func(t *testing.T) {
			other, ok := two.Get(id)
			if !ok {
				t.Fatalf("%s is in one catalog only", id)
			}
			opt := difftest.Options{Games: 3, Plies: 60, Sample: 4, Seed: 7}
			if g.Board.Len() > 150 || testing.Short() {
				opt = difftest.Options{Games: 1, Plies: 25, Sample: 2, Seed: 7}
			}
			if err := difftest.Compare(difftest.Side{Game: g}, difftest.Side{Game: other}, opt); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// ticTacToe compiles the Tic-Tac-Toe fixture as id, after edit has changed its
// definition, so a test can make a candidate that differs in one known way.
func ticTacToe(t *testing.T, id string, edit func(def map[string]any)) *catalog.CompiledGame {
	t.Helper()
	var def map[string]any
	if err := json.Unmarshal(testdata.TicTacToe().Definition, &def); err != nil {
		t.Fatal(err)
	}
	def["id"] = id
	if edit != nil {
		edit(def)
	}
	raw, err := json.Marshal(def)
	if err != nil {
		t.Fatal(err)
	}
	cat := catalog.NewCatalog()
	if err := cat.Register(id, raw, nil); err != nil {
		t.Fatalf("register %s: %v", id, err)
	}
	g, _ := cat.Get(id)
	return g
}

// The harness finds each kind of disagreement, and says where it happened.
func TestItFindsEachKindOfDisagreement(t *testing.T) {
	ref := difftest.Side{Game: ticTacToe(t, "ref", nil)}
	opt := difftest.Options{Games: 5, Seed: 1}
	if err := difftest.Compare(ref, difftest.Side{Game: ticTacToe(t, "same", nil)}, opt); err != nil {
		t.Fatalf("two copies of one game disagree: %v", err)
	}

	for name, c := range map[string]struct {
		cand difftest.Side
		want []string
	}{
		"a square the candidate never offers": {
			cand: difftest.Side{Game: ticTacToe(t, "no-centre", func(def map[string]any) {
				def["board"].(map[string]any)["regions"] = map[string]any{"rim": []string{"a1", "a2", "a3", "b1", "b3", "c1", "c2", "c3"}}
				def["actions"].([]any)[0].(map[string]any)["region"] = "rim"
			})},
			want: []string{"seed 1, at the start", "legal moves differ", "only the reference has b2"},
		},
		"a game the candidate ends early": {
			cand: difftest.Side{Game: ticTacToe(t, "short", func(def map[string]any) {
				wc := def["win_condition"].(map[string]any)
				wc["rules"] = append([]any{map[string]any{"id": "short", "type": "end", "when": []string{"ply-limit"}, "plies": 3, "decide": map[string]any{"type": "draw"}, "reason": "short"}}, wc["rules"].([]any)...)
			})},
			want: []string{"after ", "the results differ", "play goes on in the reference, a draw (short) in the candidate"},
		},
		"something off the board": {
			cand: difftest.Side{Game: ticTacToe(t, "extra", nil), Extra: func(st *state.BoardState) string {
				if st.Ply() > 1 {
					return "a capture"
				}
				return ""
			}},
			want: []string{"positions after it differ", `the reference reports "", the candidate "a capture"`},
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := difftest.Compare(ref, c.cand, opt)
			if err == nil {
				t.Fatal("no disagreement found")
			}
			for _, w := range c.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("the report does not say %q:\n%v", w, err)
				}
			}
		})
	}
}

// A different board is refused before anything is played.
func TestItRefusesADifferentBoard(t *testing.T) {
	ref := difftest.Side{Game: ticTacToe(t, "three", nil)}
	cand := difftest.Side{Game: ticTacToe(t, "four", func(def map[string]any) {
		def["board"].(map[string]any)["generator"].(map[string]any)["width"] = 4
	})}
	if err := difftest.Compare(ref, cand, difftest.Options{}); err == nil || !strings.Contains(err.Error(), "the boards differ") {
		t.Fatalf("got %v", err)
	}
}
