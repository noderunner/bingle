package classics_test

import (
	"math/rand"
	"sort"
	"testing"

	"github.com/noderunner/bingle/internal/engine/state"
)

func TestAmazonsOpeningMoveCount(t *testing.T) {
	g := load(t, "amazons")
	moves := state.LegalMoves(g, state.InitialState(g), 1)
	// The well-known branching factor of the first Amazons move.
	if len(moves) != 2176 {
		t.Errorf("White's first move has %d options, want 2176", len(moves))
	}
}

func TestAmazonsMoveSlidesAndShoots(t *testing.T) {
	g := load(t, "amazons")
	st := state.InitialState(g)

	mv, err := state.ParseMove(g, "d1-d5/f5")
	if err != nil {
		t.Fatal(err)
	}
	res, err := state.Apply(g, st, 1, mv)
	if err != nil {
		t.Fatal(err)
	}
	if res.Notation != "d1-d5/f5" {
		t.Errorf("notation = %q", res.Notation)
	}
	ns := res.NewState
	if _, ok := ns.Get(boardID("d1")); ok {
		t.Error("the queen should have left d1")
	}
	if top, ok := ns.Get(boardID("d5")); !ok || top.Piece != "queen" || top.Color != "white" {
		t.Errorf("d5 = %+v", top)
	}
	if top, ok := ns.Get(boardID("f5")); !ok || top.Piece != "arrow" {
		t.Errorf("f5 = %+v, want an arrow", top)
	}

	// The origin square is free for the shot, so shooting back is legal.
	back, _ := state.ParseMove(g, "d1-d5/d1")
	if _, err := state.Apply(g, st, 1, back); err != nil {
		t.Errorf("shooting at the vacated square: %v", err)
	}

	for _, bad := range []struct{ text, why string }{
		{"d1-d5", "no aim"},
		{"d1-e3/e4", "not a queen move"},
		{"d1-d10/d9", "blocked by the opposing queen? d10 is occupied"},
		{"d1-d5/a1", "shot not in line"},
		{"d1-d5/d1x", "garbage"},
		{"a4-a8/a9", "slides over a7's queen"},
	} {
		m, err := state.ParseMove(g, bad.text)
		if err != nil {
			continue // unparseable counts as rejected
		}
		if _, err := state.Apply(g, st, 1, m); err == nil {
			t.Errorf("%s (%s) was accepted", bad.text, bad.why)
		}
	}

	// Nothing may pass over an arrow, and an arrow cannot be landed on.
	m1, _ := state.ParseMove(g, "d1-d5/d6")
	r1, err := state.Apply(g, st, 1, m1)
	if err != nil {
		t.Fatal(err)
	}
	over, _ := state.ParseMove(g, "d10-d4/d5")
	if _, err := state.Apply(g, r1.NewState, 2, over); err == nil {
		t.Error("black slid over the arrow on d6")
	}
}

// Enumeration and validation must agree: every enumerated move applies, and
// brute force over every from/to/aim triple accepts exactly the listed ones.
func TestAmazonsEnumerationMatchesValidation(t *testing.T) {
	g := load(t, "amazons-6")
	rng := rand.New(rand.NewSource(9))
	st, seat := state.InitialState(g), 1
	for ply := 0; ply < 14; ply++ {
		legal := state.LegalMoves(g, st, seat)
		if len(legal) == 0 {
			break
		}
		listed := map[string]bool{}
		for _, mv := range legal {
			listed[state.FormatMove(g, mv)] = true
			if _, err := state.Apply(g, st, seat, mv); err != nil {
				t.Fatalf("ply %d: listed move %s rejected: %v", ply, state.FormatMove(g, mv), err)
			}
		}
		// Brute force: every square triple.
		ids := g.Board.Positions()
		accepted := 0
		for _, from := range ids {
			for _, to := range ids {
				for _, aim := range ids {
					mv := state.Move{Action: "move", From: from, To: to, Aim: aim}
					if _, err := state.Apply(g, st, seat, mv); err == nil {
						accepted++
						if !listed[state.FormatMove(g, mv)] {
							t.Fatalf("ply %d: %s accepted but not listed", ply, state.FormatMove(g, mv))
						}
					}
				}
			}
		}
		if accepted != len(legal) {
			t.Fatalf("ply %d: brute force accepted %d moves, enumeration listed %d", ply, accepted, len(legal))
		}
		res, err := state.Apply(g, st, seat, legal[rng.Intn(len(legal))])
		if err != nil {
			t.Fatal(err)
		}
		st, seat = res.NewState, res.NextTurnSeat
	}
}

func TestAmazonsLastPlayerToMoveWins(t *testing.T) {
	g := load(t, "amazons-6")
	rng := rand.New(rand.NewSource(4))
	finished := 0
	for n := 0; n < 20; n++ {
		st, seat := state.InitialState(g), 1
		for ply := 0; ply < 60; ply++ {
			legal := state.LegalMoves(g, st, seat)
			if len(legal) == 0 {
				t.Fatalf("game %d: seat %d stuck but the game did not end", n, seat)
			}
			res, err := state.Apply(g, st, seat, legal[rng.Intn(len(legal))])
			if err != nil {
				t.Fatal(err)
			}
			if res.GameEnded {
				if res.WinnerSeat == nil || res.EndReason != "no-moves" {
					t.Fatalf("game %d ended oddly: winner=%v reason=%q", n, res.WinnerSeat, res.EndReason)
				}
				// The loser really has no move.
				loser := 3 - *res.WinnerSeat
				if len(state.LegalMoves(g, res.NewState, loser)) != 0 {
					t.Fatalf("game %d: loser still has moves", n)
				}
				finished++
				break
			}
			st, seat = res.NewState, res.NextTurnSeat
		}
	}
	if finished < 18 {
		t.Errorf("only %d of 20 random 6x6 games finished within 60 plies", finished)
	}
}

func TestAmazonsStepsBuildTheCompoundMove(t *testing.T) {
	g := load(t, "amazons")
	mv, _ := state.ParseMove(g, "d1-d5/f5")
	steps := state.Steps(g, mv)
	var got []string
	for _, s := range steps {
		got = append(got, s.Kind+":"+s.Value)
	}
	sort.Strings(got)
	want := []string{"pos:d1", "pos:d5", "pos:f5"}
	if len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Errorf("steps = %v, want %v", got, want)
	}
}
