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

// twoStones is a ruleset over the Tic-Tac-Toe fixture in which each seat places
// two marks a turn, so a seat's first mark keeps the seat. With rolls set, its
// guard says the first mark rolled a die: that ends the impulse although the
// same seat plays on, which is the one difference the harness must see there.
type twoStones struct {
	base  *catalog.CompiledGame
	rolls bool
}

func (r twoStones) seat(st *state.BoardState) int { return 1 + st.Ply()/2%2 }

func (r twoStones) Start(*catalog.CompiledGame, uint64) (*state.BoardState, error) {
	return state.InitialState(r.base), nil
}
func (r twoStones) ImpulseSeat(_ *catalog.CompiledGame, st *state.BoardState) int { return r.seat(st) }
func (r twoStones) Legal(_ *catalog.CompiledGame, st *state.BoardState, seat int) []state.Selection {
	if seat != r.seat(st) {
		return nil
	}
	return state.LegalSelections(r.base, st, seat)
}
func (r twoStones) Apply(_ *catalog.CompiledGame, st *state.BoardState, seat int, mv state.Selection) (*state.MoveResult, error) {
	res, err := state.Apply(r.base, st, seat, mv)
	if err == nil && !res.GameEnded {
		res.NextImpulseSeat = r.seat(res.NewState)
	}
	return res, err
}
func (r twoStones) Format(_ *catalog.CompiledGame, mv state.Selection) string {
	return state.FormatMove(r.base, mv)
}
func (r twoStones) Parse(_ *catalog.CompiledGame, text string) (state.Selection, error) {
	return state.ParseMove(r.base, text)
}
func (r twoStones) Gestures(_ *catalog.CompiledGame, mv state.Selection) []state.Gesture {
	return state.Gestures(r.base, mv)
}
func (r twoStones) Validate(_ *catalog.CompiledGame, st *state.BoardState) error {
	return state.Validate(r.base, st)
}
func (r twoStones) Outcome(*catalog.CompiledGame, *state.BoardState) state.Outcome {
	return state.Outcome{}
}
func (r twoStones) CanTakeBack(_ *catalog.CompiledGame, before, after *state.BoardState) bool {
	return !r.rolls || after.Ply()%2 == 0
}

func twoStonesGame(t *testing.T, id string, rolls bool) *catalog.CompiledGame {
	t.Helper()
	var def map[string]any
	if err := json.Unmarshal(testdata.TicTacToe().Definition, &def); err != nil {
		t.Fatal(err)
	}
	def["id"], def["engine"] = id, "ruleset"
	raw, err := json.Marshal(def)
	if err != nil {
		t.Fatal(err)
	}
	cat := catalog.NewCatalog()
	if err := cat.RegisterWith(id, raw, nil, twoStones{base: ticTacToe(t, id+"-base", nil), rolls: rolls}); err != nil {
		t.Fatalf("register %s: %v", id, err)
	}
	g, _ := cat.Get(id)
	return g
}

// The harness sees an impulse that ends in one and goes on in the other, though
// the same seat plays on in both (ADR 0114).
func TestItFindsAnImpulseThatEndsElsewhere(t *testing.T) {
	ref := difftest.Side{Game: twoStonesGame(t, "two", false)}
	opt := difftest.Options{Games: 3, Seed: 1}
	if err := difftest.Compare(ref, difftest.Side{Game: twoStonesGame(t, "two-again", false)}, opt); err != nil {
		t.Fatalf("two copies of one game disagree: %v", err)
	}
	err := difftest.Compare(ref, difftest.Side{Game: twoStonesGame(t, "rolls", true)}, opt)
	if err == nil || !strings.Contains(err.Error(), "seed 1, at the start: seat 1 plays") ||
		!strings.Contains(err.Error(), "does not end seat 1's impulse in the reference and ends it in the candidate") {
		t.Fatalf("got %v", err)
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
