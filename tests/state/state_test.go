// Tests of the board state: set, get, remove and clone.

package state_test

import (
	"encoding/json"
	"testing"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/callback"
	"github.com/noderunner/bingle/internal/engine/primitives"
	"github.com/noderunner/bingle/internal/engine/state"
)

// --- compile-time interface assertions ---------------------------------------

var _ primitives.OccupancyView = (*state.BoardState)(nil)
var _ callback.BoardView = (*state.BoardState)(nil)

// --- 2. NewBoardState is empty ------------------------------------------------

func TestNewBoardState_IsEmpty(t *testing.T) {
	s := state.NewBoardState()
	if s == nil {
		t.Fatalf("NewBoardState() = nil, want non-nil *BoardState")
	}

	occ, ok := s.Get("0,0")
	if ok {
		t.Errorf("Get(0,0) on empty state ok = true, want false")
	}
	if occ != (state.Occupant{}) {
		t.Errorf("Get(0,0) on empty state = %+v, want zero-value Occupant{}", occ)
	}

	color, occupied := s.Occupant("0,0")
	if occupied {
		t.Errorf("Occupant(0,0) on empty state occupied = true, want false")
	}
	if color != "" {
		t.Errorf("Occupant(0,0) on empty state color = %q, want \"\"", color)
	}
}

// --- 3. Set then Get ----------------------------------------------------------

func TestSet_ThenGet_ReturnsExactOccupant(t *testing.T) {
	s := state.NewBoardState()
	want := state.Occupant{Piece: "stone", Color: "black"}
	s.Set("0,0", want)

	got, ok := s.Get("0,0")
	if !ok {
		t.Fatalf("Get(0,0) ok = false, want true after Set")
	}
	if got != want {
		t.Errorf("Get(0,0) = %+v, want %+v", got, want)
	}
}

// --- 4. Remove after Set -------------------------------------------------------

func TestRemove_AfterSet_MakesGetReturnFalse(t *testing.T) {
	s := state.NewBoardState()
	s.Set("0,0", state.Occupant{Piece: "stone", Color: "black"})
	s.Remove("0,0")

	got, ok := s.Get("0,0")
	if ok {
		t.Errorf("Get(0,0) after Remove ok = true, want false")
	}
	if got != (state.Occupant{}) {
		t.Errorf("Get(0,0) after Remove = %+v, want zero-value Occupant{}", got)
	}
}

// --- 5. Clone is a deep, independent copy -------------------------------------

func TestClone_MutatingCloneDoesNotAffectOriginal(t *testing.T) {
	s := state.NewBoardState()
	s.Set("0,0", state.Occupant{Piece: "stone", Color: "black"})
	s.Set("1,0", state.Occupant{Piece: "stone", Color: "white"})

	clone := s.Clone()
	clone.Set("0,0", state.Occupant{Piece: "stone", Color: "white"}) // overwrite
	clone.Remove("1,0")
	clone.Set("2,0", state.Occupant{Piece: "stone", Color: "black"}) // new entry

	// Original must be unaffected by any of the clone's mutations.
	orig00, ok := s.Get("0,0")
	if !ok || orig00 != (state.Occupant{Piece: "stone", Color: "black"}) {
		t.Errorf("original Get(0,0) after clone mutation = (%+v, %v), want ({stone black}, true)", orig00, ok)
	}
	orig10, ok := s.Get("1,0")
	if !ok || orig10 != (state.Occupant{Piece: "stone", Color: "white"}) {
		t.Errorf("original Get(1,0) after clone Remove = (%+v, %v), want ({stone white}, true)", orig10, ok)
	}
	if _, ok := s.Get("2,0"); ok {
		t.Errorf("original Get(2,0) after clone Set = ok true, want false (clone-only addition)")
	}
}

func TestClone_MutatingOriginalDoesNotAffectClone(t *testing.T) {
	s := state.NewBoardState()
	s.Set("0,0", state.Occupant{Piece: "stone", Color: "black"})
	s.Set("1,0", state.Occupant{Piece: "stone", Color: "white"})

	clone := s.Clone()

	s.Set("0,0", state.Occupant{Piece: "stone", Color: "white"}) // overwrite original
	s.Remove("1,0")
	s.Set("2,0", state.Occupant{Piece: "stone", Color: "black"}) // new entry on original

	clone00, ok := clone.Get("0,0")
	if !ok || clone00 != (state.Occupant{Piece: "stone", Color: "black"}) {
		t.Errorf("clone Get(0,0) after original mutation = (%+v, %v), want ({stone black}, true)", clone00, ok)
	}
	clone10, ok := clone.Get("1,0")
	if !ok || clone10 != (state.Occupant{Piece: "stone", Color: "white"}) {
		t.Errorf("clone Get(1,0) after original Remove = (%+v, %v), want ({stone white}, true)", clone10, ok)
	}
	if _, ok := clone.Get("2,0"); ok {
		t.Errorf("clone Get(2,0) after original Set = ok true, want false (original-only addition)")
	}
}

// --- 6. Occupant (OccupancyView) ----------------------------------------------

func TestOccupant_MatchesSetOccupantColor(t *testing.T) {
	s := state.NewBoardState()
	s.Set("0,0", state.Occupant{Piece: "stone", Color: "black"})

	color, occupied := s.Occupant("0,0")
	if !occupied {
		t.Fatalf("Occupant(0,0) occupied = false, want true")
	}
	if color != "black" {
		t.Errorf("Occupant(0,0) color = %q, want %q", color, "black")
	}
}

func TestOccupant_AbsentPosition_ReturnsEmptyFalse(t *testing.T) {
	s := state.NewBoardState()
	s.Set("0,0", state.Occupant{Piece: "stone", Color: "black"})

	color, occupied := s.Occupant("9,9")
	if occupied {
		t.Errorf("Occupant(9,9) occupied = true, want false")
	}
	if color != "" {
		t.Errorf("Occupant(9,9) color = %q, want \"\"", color)
	}
}

// --- 7. SetOccupant on an empty position --------------------------------------

func TestSetOccupant_OnEmptyPosition_CreatesOccupantWithColorAndEmptyPiece(t *testing.T) {
	s := state.NewBoardState()
	s.SetOccupant("0,0", "black")

	got, ok := s.Get("0,0")
	if !ok {
		t.Fatalf("Get(0,0) after SetOccupant on empty position ok = false, want true")
	}
	want := state.Occupant{Piece: "", Color: "black"}
	if got != want {
		t.Errorf("Get(0,0) after SetOccupant on empty position = %+v, want %+v", got, want)
	}
}

// --- 8. SetOccupant on an already-occupied position preserves Piece ----------

func TestSetOccupant_OnOccupiedPosition_PreservesPieceUpdatesColor(t *testing.T) {
	s := state.NewBoardState()
	s.Set("0,0", state.Occupant{Piece: "stone", Color: "black"})

	s.SetOccupant("0,0", "white")

	got, ok := s.Get("0,0")
	if !ok {
		t.Fatalf("Get(0,0) after SetOccupant on occupied position ok = false, want true")
	}
	want := state.Occupant{Piece: "stone", Color: "white"}
	if got != want {
		t.Errorf("Get(0,0) after SetOccupant on occupied position = %+v, want %+v (Piece preserved, Color updated)", got, want)
	}
}

// --- 9. RemoveOccupant ---------------------------------------------------------

func TestRemoveOccupant_Removes(t *testing.T) {
	s := state.NewBoardState()
	s.Set("0,0", state.Occupant{Piece: "stone", Color: "black"})

	s.RemoveOccupant("0,0")

	if got, ok := s.Get("0,0"); ok {
		t.Errorf("Get(0,0) after RemoveOccupant = (%+v, true), want ok=false", got)
	}
	if color, occupied := s.Occupant("0,0"); occupied {
		t.Errorf("Occupant(0,0) after RemoveOccupant = (%q, true), want occupied=false", color)
	}
}

// --- 10. MarshalJSON / UnmarshalJSON round-trip --------------------------------

func TestJSONRoundTrip_PreservesAllOccupants(t *testing.T) {
	s := state.NewBoardState()
	entries := map[board.PositionID]state.Occupant{
		"0,0": {Piece: "stone", Color: "black"},
		"1,0": {Piece: "stone", Color: "white"},
		"2,2": {Piece: "king", Color: "black"},
		"p1":  {Piece: "rock", Color: "white"},
	}
	for id, occ := range entries {
		s.Set(id, occ)
	}

	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("json.Marshal(s) error = %v, want nil", err)
	}

	fresh := state.NewBoardState()
	if err := json.Unmarshal(data, fresh); err != nil {
		t.Fatalf("json.Unmarshal(data, fresh) error = %v, want nil", err)
	}

	for id, want := range entries {
		got, ok := fresh.Get(id)
		if !ok {
			t.Errorf("after round-trip, Get(%s) ok = false, want true", id)
			continue
		}
		if got != want {
			t.Errorf("after round-trip, Get(%s) = %+v, want %+v", id, got, want)
		}
	}

	// And nothing extra should have appeared.
	if _, ok := fresh.Get("9,9"); ok {
		t.Errorf("after round-trip, Get(9,9) ok = true, want false (never set)")
	}
}

func TestJSONRoundTrip_EmptyBoardState(t *testing.T) {
	s := state.NewBoardState()

	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("json.Marshal(empty state) error = %v, want nil", err)
	}

	fresh := state.NewBoardState()
	if err := json.Unmarshal(data, fresh); err != nil {
		t.Fatalf("json.Unmarshal(empty data, fresh) error = %v, want nil", err)
	}

	if got, ok := fresh.Get("0,0"); ok {
		t.Errorf("after unmarshaling an empty snapshot, Get(0,0) = (%+v, true), want ok=false", got)
	}
}
