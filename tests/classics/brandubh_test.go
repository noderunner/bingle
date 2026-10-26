package classics_test

import (
	"math/rand"
	"testing"

	"github.com/noderunner/bingle/internal/engine/state"
)

func brandubhPosition(t testing.TB, stones map[string][2]string) *state.BoardState {
	t.Helper()
	st := state.NewBoardState()
	for id, pc := range stones {
		st.Set(boardID(id), state.Occupant{Piece: pc[0], Color: pc[1]})
	}
	return st
}

func TestBrandubhStartAndRookMoves(t *testing.T) {
	g := load(t, "brandubh")
	st := state.InitialState(g)
	if n := len(st.Occupied()); n != 13 {
		t.Fatalf("start has %d pieces, want 13", n)
	}
	// Attackers at the rim can slide along their edge but never onto the
	// corners; d2 is boxed in by d1/d3.
	moves := state.LegalMoves(g, st, 1)
	if len(moves) == 0 {
		t.Fatal("attackers have no moves")
	}
	for _, mv := range moves {
		for _, forbidden := range []string{"a1", "g1", "a7", "g7", "d4"} {
			if string(mv.To) == forbidden {
				t.Errorf("an attacker may not stop on %s (%s)", forbidden, state.FormatMove(g, mv))
			}
		}
	}
	// a4-a1 would land on the corner, a4-a2 is fine.
	for _, tc := range []struct {
		text string
		ok   bool
	}{{"a4-a1", false}, {"a4-a2", true}, {"a4-a8", false}, {"d2-e3", false}, {"d1-g1", false}} {
		mv, err := state.ParseMove(g, tc.text)
		if err != nil {
			if tc.ok {
				t.Errorf("%s: %v", tc.text, err)
			}
			continue
		}
		_, err = state.Apply(g, st, 1, mv)
		if (err == nil) != tc.ok {
			t.Errorf("%s: err=%v, want ok=%v", tc.text, err, tc.ok)
		}
	}
}

func TestBrandubhSandwichCaptures(t *testing.T) {
	g := load(t, "brandubh")
	// Attackers at c2 and a defender at e2... build: black a2 and e2 sandwich
	// a white defender at d2 when e2 arrives.
	st := brandubhPosition(t, map[string][2]string{
		"c2": {"attacker", "black"},
		"d2": {"defender", "white"},
		"e5": {"attacker", "black"},
		"d4": {"king", "white"},
		"g7": {"attacker", "black"}, // keep both sides alive
	})
	mv, _ := state.ParseMove(g, "e5-e2")
	res, err := state.Apply(g, st, 1, mv)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := res.NewState.Get(boardID("d2")); ok {
		t.Error("the defender at d2 should be captured between c2 and e2")
	}
	if res.Captured == nil || *res.Captured != "d2" {
		t.Errorf("Captured = %v", res.Captured)
	}

	// Moving between two enemies is safe.
	st2 := brandubhPosition(t, map[string][2]string{
		"c3": {"attacker", "black"},
		"e3": {"attacker", "black"},
		"d6": {"defender", "white"},
		"f6": {"king", "white"},
		"g7": {"attacker", "black"},
	})
	mv2, _ := state.ParseMove(g, "d6-d3")
	r2, err := state.Apply(g, st2, 2, mv2)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := r2.NewState.Get(boardID("d3")); !ok {
		t.Error("a piece moving between two enemies is safe (and may pass over the empty throne)")
	}

	// A corner is an anvil: an attacker next to an empty corner is captured
	// by a defender arriving on the other side.
	st3 := brandubhPosition(t, map[string][2]string{
		"a2": {"attacker", "black"},
		"d4": {"king", "white"},
		"a5": {"defender", "white"},
		"g7": {"attacker", "black"},
	})
	mv3, _ := state.ParseMove(g, "a5-a3")
	r3, err := state.Apply(g, st3, 2, mv3)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := r3.NewState.Get(boardID("a2")); ok {
		t.Error("the attacker at a2 is pinned against the corner a1 and should be captured")
	}
}

func TestBrandubhWins(t *testing.T) {
	g := load(t, "brandubh")

	// King escapes to a corner.
	st := brandubhPosition(t, map[string][2]string{
		"g4": {"king", "white"},
		"d6": {"attacker", "black"},
		"a7": {"attacker", "black"},
	})
	mv, _ := state.ParseMove(g, "g4-g1")
	res, err := state.Apply(g, st, 2, mv)
	if err != nil {
		t.Fatal(err)
	}
	if !res.GameEnded || res.WinnerSeat == nil || *res.WinnerSeat != 2 || res.EndReason != "escape" {
		t.Errorf("escape: ended=%v winner=%v reason=%q", res.GameEnded, res.WinnerSeat, res.EndReason)
	}

	// King captured between two attackers.
	st2 := brandubhPosition(t, map[string][2]string{
		"c5": {"king", "white"},
		"b5": {"attacker", "black"},
		"f5": {"attacker", "black"},
		"g1": {"defender", "white"},
	})
	mv2, _ := state.ParseMove(g, "f5-d5")
	r2, err := state.Apply(g, st2, 1, mv2)
	if err != nil {
		t.Fatal(err)
	}
	if !r2.GameEnded || r2.WinnerSeat == nil || *r2.WinnerSeat != 1 || r2.EndReason != "king-captured" {
		t.Errorf("king capture: ended=%v winner=%v reason=%q", r2.GameEnded, r2.WinnerSeat, r2.EndReason)
	}
}

func TestBrandubhRandomGamesTerminateSanely(t *testing.T) {
	g := load(t, "brandubh")
	rng := rand.New(rand.NewSource(5))
	for n := 0; n < 15; n++ {
		st, seat := state.InitialState(g), 1
		for ply := 0; ply < 400; ply++ {
			legal := state.LegalMoves(g, st, seat)
			if len(legal) == 0 {
				t.Fatalf("game %d: stuck seat %d without the game ending", n, seat)
			}
			res, err := state.Apply(g, st, seat, legal[rng.Intn(len(legal))])
			if err != nil {
				t.Fatal(err)
			}
			if res.GameEnded {
				break
			}
			st, seat = res.NewState, res.NextTurnSeat
		}
	}
}
