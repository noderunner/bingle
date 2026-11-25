package wasmrules_test

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sync"
	"testing"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/state"
	"github.com/noderunner/bingle/internal/engine/wasmrules"
	"github.com/noderunner/bingle/internal/engine/wasmrules/guest"
	"github.com/noderunner/bingle/tests/testdata"
	"github.com/noderunner/bingle/tests/testdata/draughts"
)

func sameGame(t *testing.T, what string, got, want []step) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: %d selections, want %d", what, len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s: selection %d is\n%+v\nwant\n%+v", what, i, got[i], want[i])
		}
	}
}

// The same seed gives the same game, to the byte of every stored position:
// twice on one module, whose instances have answered other calls by then, on
// a module with a single instance, and on a module that was never asked
// anything. A guest that let the order of a Go map into an answer, or kept
// something from one call to the next, would differ somewhere here. The
// linked rules give that game too.
func TestTheSameSeedGivesTheSameGame(t *testing.T) {
	mod := draughtsModule(t)
	used := games(t, mod.Rules())
	native := nativeGames(t)
	for _, id := range []string{draughts.ID, draughts.SmallID, draughts.BallotID} {
		for _, seed := range []uint64{1, 1<<63 + 12345} {
			t.Run(fmt.Sprintf("%s/%d", id, seed), func(t *testing.T) {
				want := playOut(t, native[id], seed, 3, 300)
				sameGame(t, "the module", playOut(t, used[id], seed, 3, 300), want)
				sameGame(t, "the module again", playOut(t, used[id], seed, 3, 300), want)

				one := load(t, newRuntime(t, wasmrules.Options{Instances: 1}), testdata.DraughtsWasm)
				sameGame(t, "a new module with one instance", playOut(t, games(t, one.Rules())[id], seed, 3, 300), want)
				if n := one.Instances(); n != 1 {
					t.Fatalf("the module has %d instances, want 1", n)
				}
			})
		}
	}
}

// The seed crosses whole: the ballot draws a different start from seeds that
// differ only beyond what a JSON number keeps in other languages.
func TestTheSeedCrossesWhole(t *testing.T) {
	game := games(t, draughtsModule(t).Rules())[draughts.BallotID]
	native := nativeGames(t)[draughts.BallotID]
	starts := map[string]bool{}
	for seed := uint64(1 << 62); seed < 1<<62+40; seed++ {
		st, err := state.InitialStateSeeded(game, seed)
		if err != nil {
			t.Fatal(err)
		}
		ref, err := state.InitialStateSeeded(native, seed)
		if err != nil {
			t.Fatal(err)
		}
		got, _ := st.MarshalJSON()
		want, _ := ref.MarshalJSON()
		if string(got) != string(want) {
			t.Fatalf("seed %d: the module starts from\n%s\nthe linked rules from\n%s", seed, got, want)
		}
		if n := len(st.Occupied()); n != 22 {
			t.Fatalf("seed %d: %d men on the board, want 11 a side", seed, n)
		}
		starts[string(got)] = true
	}
	if len(starts) < 20 {
		t.Fatalf("40 seeds gave %d different starts", len(starts))
	}
}

// Many games at once on a pool smaller than their number play as they do one
// at a time. Run it with -race.
func TestConcurrentCallsShareThePool(t *testing.T) {
	const instances, players = 3, 12
	mod := load(t, newRuntime(t, wasmrules.Options{Instances: instances}), testdata.DraughtsWasm)
	wasm, native := games(t, mod.Rules()), nativeGames(t)
	ids := []string{draughts.ID, draughts.SmallID, draughts.BallotID}
	plies := 200
	if testing.Short() {
		plies = 40
	}
	want := make([][]step, players)
	for i := range want {
		want[i] = playOut(t, native[ids[i%len(ids)]], uint64(i), int64(i), plies)
	}
	var wg sync.WaitGroup
	got := make([][]step, players)
	for i := range got {
		wg.Go(func() { got[i] = playOut(t, wasm[ids[i%len(ids)]], uint64(i), int64(i), plies) })
	}
	wg.Wait()
	for i := range want {
		sameGame(t, fmt.Sprintf("game %d", i), got[i], want[i])
	}
	if n := mod.Instances(); n < 1 || n > instances {
		t.Fatalf("the module holds %d instances, want at most %d", n, instances)
	}
}

// The calls no battery of games reaches: notation read back, the gestures of
// a selection, a position checked and the outcome of one, through the module
// and through its JSON alone, against the linked rules.
func TestEveryCallAnswersAsTheLinkedRulesDo(t *testing.T) {
	native := nativeGames(t)[draughts.ID]
	sides := map[string]state.Ruleset{
		"wasm":    draughtsModule(t).Rules(),
		"encoded": wasmrules.Encoded(draughts.New),
	}
	for name, rules := range sides {
		t.Run(name, func(t *testing.T) {
			game := games(t, rules)[draughts.ID]
			steps := playOut(t, native, 9, 9, 400)
			for i, s := range steps {
				st, ref := state.NewBoardState(), state.NewBoardState()
				if err := st.UnmarshalJSON([]byte(s.Position)); err != nil {
					t.Fatal(err)
				}
				_ = ref.UnmarshalJSON([]byte(s.Position))
				if err := state.Validate(game, st); err != nil {
					t.Fatalf("position %d does not validate: %v", i, err)
				}
				got, _ := state.Evaluate(game, st, 1)
				want, _ := state.Evaluate(native, ref, 1)
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("position %d: outcome %+v, want %+v", i, got, want)
				}
				seat := state.StartSeat(native, ref)
				if last := i == len(steps)-1; last != got.Ended {
					t.Fatalf("position %d of %d: ended is %v", i, len(steps)-1, got.Ended)
				} else if last {
					rs, _ := state.RulesetOf(game)
					if s := rs.ImpulseSeat(game, st); s != 0 {
						t.Fatalf("the finished game waits for seat %d", s)
					}
					break
				}
				for _, mv := range state.LegalSelections(game, st, seat) {
					text := state.FormatMove(game, mv)
					back, err := state.ParseMove(game, text)
					if err != nil || back.Key() != mv.Key() {
						t.Fatalf("%q reads back as %+v, %v", text, back, err)
					}
					if g, w := state.Gestures(game, mv), state.Gestures(native, mv); !reflect.DeepEqual(g, w) || len(g) != 2 {
						t.Fatalf("the gestures of %s are %v, want %v", text, g, w)
					}
				}
			}
			if _, err := state.ParseMove(game, "castles"); err == nil {
				t.Fatal("nonsense was read as a selection")
			}
			// A position that is not one of the game's: a man on a light square.
			odd := state.NewBoardState()
			_ = odd.UnmarshalJSON([]byte(steps[0].Position))
			odd.Set("a2", state.Occupant{Piece: "man", Color: "black"})
			if err := state.Validate(game, odd); err == nil {
				t.Fatal("a man on a light square was accepted")
			}
			// A selection the rules do not list is the call's own error.
			start := state.NewBoardState()
			_ = start.UnmarshalJSON([]byte(steps[0].Position))
			if _, err := state.Apply(game, start, 1, state.Selection{Action: "move", From: "a3", To: "a4"}); err == nil {
				t.Fatal("an illegal move was applied")
			}
			if _, err := state.Apply(game, start, 2, state.Selection{Action: "move", From: "b6", To: "a5"}); err == nil {
				t.Fatal("a move out of turn was applied")
			}
		})
	}
}

// The platform's own counts on a position cross and come back: a take-back
// counted on a position is still counted after the next selection.
func TestAPositionsCountersCross(t *testing.T) {
	for name, rules := range map[string]state.Ruleset{
		"native": wasmrules.Native(draughts.New), "encoded": wasmrules.Encoded(draughts.New), "wasm": draughtsModule(t).Rules(),
	} {
		game := games(t, rules)[draughts.ID]
		st, err := state.InitialStateSeeded(game, 1)
		if err != nil {
			t.Fatal(err)
		}
		later, _ := state.Apply(game, st, 1, state.Selection{Action: "move", From: "a3", To: "b4"})
		state.CountTakeBack(later.NewState, st)
		res, err := state.Apply(game, st, 1, state.Selection{Action: "move", From: "c3", To: "d4"})
		if err != nil {
			t.Fatal(err)
		}
		if got := state.Moves(res.NewState); got != 3 {
			t.Errorf("%s: %d moves used after a move, a take-back and a move, want 3", name, got)
		}
	}
}

// The guest's types are the engine's on the wire: a value written by one is
// read by the other and written back the same.
func TestTheGuestsTypesAreTheEnginesOnTheWire(t *testing.T) {
	roundTrip := func(name string, engine, guestValue any) {
		t.Helper()
		raw, err := json.Marshal(engine)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, guestValue); err != nil {
			t.Fatalf("%s: the guest cannot read %s: %v", name, raw, err)
		}
		back, err := json.Marshal(guestValue)
		if err != nil {
			t.Fatal(err)
		}
		if string(back) != string(raw) {
			t.Errorf("%s: the engine writes\n%s\nthe guest\n%s", name, raw, back)
		}
	}
	roundTrip("selection", state.Selection{
		Action: "a", Piece: "p", From: "f", To: "t", Count: 3,
		Path: []board.PositionID{"x", "y"}, Drops: []int{1, 2}, Aim: "m", Card: "c",
	}, &guest.Selection{})
	roundTrip("gesture", state.Gesture{Kind: state.GesturePos, Value: "a1"}, &guest.Gesture{})
	roundTrip("fact", state.Fact{Name: "n", Seat: 2}, &guest.Fact{})
	for kind, want := range map[string]string{
		guest.GesturePiece: state.GesturePiece, guest.GesturePos: state.GesturePos, guest.GestureCount: state.GestureCount,
		guest.GestureAction: state.GestureAction, guest.GestureCard: state.GestureCard,
	} {
		if kind != want {
			t.Errorf("the guest calls a gesture %q, the engine %q", kind, want)
		}
	}

	// A position, with everything a ruleset's position can hold.
	st := state.NewBoardState()
	st.SetStack("a1", []state.Occupant{{Piece: "man", Color: "black"}, {Piece: "king", Color: "white"}})
	st.SetReserve("black", "man", 4)
	st.SetPly(7)
	st.SetCounter("taken_back", 2)
	st.SetExt(json.RawMessage(`{"turn":1}`))
	raw, _ := st.MarshalJSON()
	var doc struct {
		V int `json:"v"`
		*guest.Position
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	back, _ := json.Marshal(doc)
	if string(back) != string(raw) || doc.V != state.StateVersion {
		t.Errorf("a position: the engine writes\n%s\nthe guest\n%s", raw, back)
	}
}
