// Package impulses_test checks ADR 0114's impulses on every shipped game: an
// impulse built selection by selection with NextSelections is exactly what
// ApplyImpulse accepts, it reaches the same position as its selections played
// one at a time, its notation reads back, and a part of one, or one too many,
// is refused (but in a game for one seat, which may stop after any selection).
package impulses_test

import (
	"errors"
	"math/rand"
	"testing"

	"github.com/noderunner/bingle/internal/app"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

func allGames(t *testing.T) []*catalog.CompiledGame {
	t.Helper()
	cat := catalog.NewCatalog()
	for _, m := range app.BuiltinModules() {
		if err := cat.RegisterModule(m); err != nil {
			t.Fatalf("RegisterModule(%s): %v", m.ID, err)
		}
	}
	return cat.List()
}

func doc(t *testing.T, st *state.BoardState) string {
	t.Helper()
	b, err := st.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestEveryGamePlaysByImpulses(t *testing.T) {
	impulses := 30
	if testing.Short() {
		impulses = 10
	}
	for _, g := range allGames(t) {
		g := g
		t.Run(g.Definition.ID, func(t *testing.T) {
			budget := impulses
			if g.Board.Len() > 150 {
				budget = 8
			}
			rng := rand.New(rand.NewSource(int64(len(g.Definition.ID)) * 17))
			st, err := state.InitialStateSeeded(g, 11)
			if err != nil {
				t.Fatal(err)
			}
			seat := state.FirstSeat(g)
			for n := 0; n < budget; n++ {
				var made []state.Move
				var sels state.Selections
				for {
					sels, err = state.NextSelections(g, st, seat, made)
					if err != nil {
						t.Fatalf("impulse %d after %q: %v", n, state.FormatImpulse(g, made), err)
					}
					if sels.Complete {
						break
					}
					if sels.MayEnd != (len(made) > 0 && state.AnySelectionMayEnd(g)) {
						t.Fatalf("impulse %d after %q: MayEnd = %v", n, state.FormatImpulse(g, made), sels.MayEnd)
					}
					// A game for one seat may stop anywhere; stop it at random.
					if sels.MayEnd && (rng.Intn(4) == 0 || len(made) >= 40) {
						break
					}
					if len(sels.Options) == 0 {
						return // the seat has nothing to choose: the game is stuck or over
					}
					if len(made) >= 200 {
						t.Fatalf("impulse %d runs past 200 selections: %q", n, state.FormatImpulse(g, made))
					}
					made = append(made, sels.Options[rng.Intn(len(sels.Options))])
				}

				// Part of the impulse is refused, and nothing is applied
				// (but in a game for one seat, which may stop anywhere).
				if state.AnySelectionMayEnd(g) {
					// nothing to refuse
				} else if _, err := state.ApplyImpulse(g, st, seat, made[:len(made)-1]); !errors.Is(err, state.ErrImpulseOpen) {
					t.Fatalf("impulse %d: submitting all but its last selection: %v, want ErrImpulseOpen", n, err)
				}
				res, err := state.ApplyImpulse(g, st, seat, made)
				if err != nil {
					t.Fatalf("impulse %d %q: %v", n, state.FormatImpulse(g, made), err)
				}

				// The same as its selections played one at a time.
				cur := st
				for _, mv := range made {
					r, err := state.Apply(g, cur, seat, mv)
					if err != nil {
						t.Fatal(err)
					}
					cur = r.NewState
				}
				if doc(t, cur) != doc(t, res.Last().NewState) {
					t.Fatalf("impulse %d: the whole impulse and its selections one by one reach different positions", n)
				}

				// Its notation reads back to the same impulse.
				text := state.FormatImpulse(g, made)
				back, err := state.ParseImpulse(g, text)
				if err != nil {
					t.Fatalf("impulse %d: %q does not read back: %v", n, text, err)
				}
				res2, err := state.ApplyImpulse(g, st, seat, back)
				if err != nil || doc(t, res2.Last().NewState) != doc(t, res.Last().NewState) {
					t.Fatalf("impulse %d: %q read back to a different impulse (%v)", n, text, err)
				}

				last := res.Last()
				if last.GameEnded {
					return
				}
				// One selection too many is refused, where the impulse really ended.
				if !sels.Complete {
					// a game for one seat stopped early: it could go on
				} else if next := state.LegalMoves(g, last.NewState, last.NextImpulseSeat); len(next) > 0 {
					if _, err := state.NextSelections(g, st, seat, append(append([]state.Move{}, made...), next[0])); !errors.Is(err, state.ErrImpulseOver) {
						t.Fatalf("impulse %d: a selection after its end: %v, want ErrImpulseOver", n, err)
					}
				}
				st, seat = last.NewState, last.NextImpulseSeat
			}
		})
	}
}

// Blooms' turn is one impulse: a stone, then a stone of the other colour or
// done; the first player's first turn is a single stone.
func TestBloomsTurnIsOneImpulse(t *testing.T) {
	var g *catalog.CompiledGame
	for _, c := range allGames(t) {
		if c.Definition.ID == "blooms" {
			g = c
		}
	}
	st, err := state.InitialStateSeeded(g, 1)
	if err != nil {
		t.Fatal(err)
	}
	parse := func(text string) []state.Move {
		t.Helper()
		sels, err := state.ParseImpulse(g, text)
		if err != nil {
			t.Fatal(err)
		}
		return sels
	}
	res, err := state.ApplyImpulse(g, st, 1, parse("red@e5"))
	if err != nil || res.Last().NextImpulseSeat != 2 {
		t.Fatalf("the opening stone: %v", err)
	}
	st = res.Last().NewState
	if _, err := state.ApplyImpulse(g, st, 2, parse("blue@d4")); !errors.Is(err, state.ErrImpulseOpen) {
		t.Errorf("one stone without done: %v, want ErrImpulseOpen", err)
	}
	for _, text := range []string{"blue@d4; black@f6", "blue@d4; done"} {
		if res, err := state.ApplyImpulse(g, st, 2, parse(text)); err != nil || res.Last().NextImpulseSeat != 1 {
			t.Errorf("%q: %v", text, err)
		}
	}
	if _, err := state.ApplyImpulse(g, st, 2, parse("blue@d4; black@f6; done")); !errors.Is(err, state.ErrImpulseOver) {
		t.Errorf("a third selection: %v, want ErrImpulseOver", err)
	}
}
