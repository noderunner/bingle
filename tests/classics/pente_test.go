package classics_test

import (
	"math/rand"
	"testing"

	"github.com/noderunner/bingle/internal/engine/state"
)

func TestPenteCapturesExactlyFlankedPairs(t *testing.T) {
	g := load(t, "pente")
	st := state.InitialState(g)
	put := func(id, color string) { st.Set(boardID(id), state.Occupant{Piece: "stone", Color: color}) }

	// Black at d4, white pair e4 f4; black plays g4 and captures the pair.
	put("d4", "black")
	put("e4", "white")
	put("f4", "white")
	mv, _ := state.ParseMove(g, "g4")
	res, err := state.Apply(g, st, 1, mv)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"e4", "f4"} {
		if _, ok := res.NewState.Get(boardID(id)); ok {
			t.Errorf("%s should have been captured", id)
		}
	}
	if got := res.NewState.Reserve("black", "captured"); got != 2 {
		t.Errorf("black's captured count = %d, want 2", got)
	}
	if got := res.NewState.Reserve("white", "captured"); got != 0 {
		t.Errorf("white's captured count = %d, want 0", got)
	}
	if len(res.Captures) != 2 {
		t.Errorf("MoveResult.Captures = %v", res.Captures)
	}

	// A run of three is not a pair: nothing is captured.
	st2 := state.InitialState(g)
	put2 := func(id, color string) { st2.Set(boardID(id), state.Occupant{Piece: "stone", Color: color}) }
	put2("d4", "black")
	put2("e4", "white")
	put2("f4", "white")
	put2("g4", "white")
	mv2, _ := state.ParseMove(g, "h4")
	r2, err := state.Apply(g, st2, 1, mv2)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := r2.NewState.Get(boardID("e4")); !ok {
		t.Error("a run of three must not be captured")
	}

	// Moving into a flanked position is safe: white plays between black
	// stones and nothing happens to it.
	st3 := state.InitialState(g)
	put3 := func(id, color string) { st3.Set(boardID(id), state.Occupant{Piece: "stone", Color: color}) }
	put3("d4", "black")
	put3("g4", "black")
	put3("e4", "white")
	mv3, _ := state.ParseMove(g, "f4")
	r3, err := state.Apply(g, st3, 2, mv3)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := r3.NewState.Get(boardID("f4")); !ok {
		t.Error("a stone placed into a flank is safe")
	}
	if _, ok := r3.NewState.Get(boardID("e4")); !ok {
		t.Error("the safe stone must remain")
	}
}

func TestPenteTenCapturedStonesWin(t *testing.T) {
	g := load(t, "pente")
	st := state.InitialState(g)
	st.SetReserve("black", "captured", 8)
	put := func(id, color string) { st.Set(boardID(id), state.Occupant{Piece: "stone", Color: color}) }
	put("a1", "black")
	put("b1", "white")
	put("c1", "white")
	mv, _ := state.ParseMove(g, "d1")
	res, err := state.Apply(g, st, 1, mv)
	if err != nil {
		t.Fatal(err)
	}
	if !res.GameEnded || res.WinnerSeat == nil || *res.WinnerSeat != 1 || res.EndReason != "captures" {
		t.Errorf("ended=%v winner=%v reason=%q, want seat 1 by captures", res.GameEnded, res.WinnerSeat, res.EndReason)
	}
}

func TestPenteFiveInARowAndRandomGames(t *testing.T) {
	g := load(t, "pente")
	res := playAll(t, g, "g7", "a1", "h7", "a3", "i7", "a5", "j7", "a8", "k7")
	if !res.GameEnded || *res.WinnerSeat != 1 || res.EndReason != "five-in-a-row" {
		t.Errorf("five in a row: ended=%v winner=%v reason=%q", res.GameEnded, res.WinnerSeat, res.EndReason)
	}

	rng := rand.New(rand.NewSource(12))
	for n := 0; n < 10; n++ {
		st, seat := state.InitialState(g), 1
		for ply := 0; ply < 300; ply++ {
			legal := state.LegalMoves(g, st, seat)
			if len(legal) == 0 {
				break
			}
			r, err := state.Apply(g, st, seat, legal[rng.Intn(len(legal))])
			if err != nil {
				t.Fatal(err)
			}
			// Captured counts never exceed what was removed from the board.
			stones := len(r.NewState.Occupied())
			captured := r.NewState.Reserve("black", "captured") + r.NewState.Reserve("white", "captured")
			if stones+captured != ply+1 {
				t.Fatalf("game %d ply %d: %d stones on board + %d captured != %d placed", n, ply, stones, captured, ply+1)
			}
			if r.GameEnded {
				break
			}
			st, seat = r.NewState, r.NextTurnSeat
		}
	}
}
