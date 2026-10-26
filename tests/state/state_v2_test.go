package state_test

import (
	"encoding/json"
	"testing"

	"github.com/noderunner/bingle/internal/engine/state"
)

func TestV2JSON_RoundTripsStacksReservesAndPly(t *testing.T) {
	s := state.NewBoardState()
	s.Push("c3", state.Occupant{Piece: "flat", Color: "white"})
	s.Push("c3", state.Occupant{Piece: "flat", Color: "black"})
	s.Push("c3", state.Occupant{Piece: "cap", Color: "white"})
	s.Set("a1", state.Occupant{Piece: "wall", Color: "black"})
	s.SetReserve("white", "stone", 17)
	s.SetReserve("black", "cap", 1)
	s.SetPly(9)

	data, err := s.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		V int `json:"v"`
	}
	if err := json.Unmarshal(data, &doc); err != nil || doc.V != 2 {
		t.Fatalf("document = %s (v=%d, err=%v), want a v2 document", data, doc.V, err)
	}

	back := state.NewBoardState()
	if err := back.UnmarshalJSON(data); err != nil {
		t.Fatal(err)
	}
	stack := back.Stack("c3")
	if len(stack) != 3 || stack[0].Color != "white" || stack[1].Color != "black" || stack[2].Piece != "cap" {
		t.Errorf("c3 stack = %+v", stack)
	}
	if back.Reserve("white", "stone") != 17 || back.Reserve("black", "cap") != 1 || back.Ply() != 9 {
		t.Errorf("reserves/ply lost: white stone %d black cap %d ply %d", back.Reserve("white", "stone"), back.Reserve("black", "cap"), back.Ply())
	}
	if top, _ := back.Get("c3"); top.Piece != "cap" {
		t.Errorf("Get returns %+v, want the top piece", top)
	}
}

func TestUnmarshal_AcceptsTheLegacyFlatShape(t *testing.T) {
	s := state.NewBoardState()
	if err := s.UnmarshalJSON([]byte(`{"0,0":{"Piece":"stone","Color":"black"},"v":{"Piece":"stone","Color":"white"}}`)); err != nil {
		t.Fatal(err)
	}
	// A position that happens to be called "v" must not be mistaken for the
	// version marker.
	if occ, ok := s.Get("v"); !ok || occ.Color != "white" {
		t.Errorf("v = %+v ok=%v", occ, ok)
	}
	if occ, ok := s.Get("0,0"); !ok || occ.Color != "black" {
		t.Errorf("0,0 = %+v ok=%v", occ, ok)
	}
	if s.Ply() != 0 {
		t.Errorf("legacy ply = %d, want 0", s.Ply())
	}
}

func TestLegacyJSON_IsTheTopOfStackView(t *testing.T) {
	s := state.NewBoardState()
	s.Push("c3", state.Occupant{Piece: "flat", Color: "white"})
	s.Push("c3", state.Occupant{Piece: "wall", Color: "black"})
	s.SetReserve("white", "stone", 3)

	data, err := s.LegacyJSON()
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]struct{ Piece, Color string }
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got["c3"].Piece != "wall" || got["c3"].Color != "black" {
		t.Errorf("legacy view = %s, want only c3 -> black wall", data)
	}
}

func TestClone_IsDeepForStacksAndReserves(t *testing.T) {
	s := state.NewBoardState()
	s.Push("a1", state.Occupant{Piece: "flat", Color: "white"})
	s.SetReserve("white", "stone", 5)

	c := s.Clone()
	c.Push("a1", state.Occupant{Piece: "wall", Color: "white"})
	c.SetReserve("white", "stone", 1)
	c.SetPly(4)

	if s.Height("a1") != 1 || s.Reserve("white", "stone") != 5 || s.Ply() != 0 {
		t.Errorf("mutating the clone changed the original: height %d reserve %d ply %d", s.Height("a1"), s.Reserve("white", "stone"), s.Ply())
	}
	// Mutating the original's returned stack copy must not leak either.
	st := s.Stack("a1")
	st[0].Color = "black"
	if top, _ := s.Get("a1"); top.Color != "white" {
		t.Error("Stack() returned a slice aliasing internal state")
	}
}

func TestSetOccupant_RecolorsTheTopPiece(t *testing.T) {
	s := state.NewBoardState()
	s.Push("a1", state.Occupant{Piece: "flat", Color: "white"})
	s.Push("a1", state.Occupant{Piece: "cap", Color: "white"})
	s.SetOccupant("a1", "black")
	stack := s.Stack("a1")
	if stack[0].Color != "white" || stack[1].Color != "black" || stack[1].Piece != "cap" {
		t.Errorf("stack = %+v, want only the top piece recolored", stack)
	}
}

func TestUnsupportedVersionIsRejected(t *testing.T) {
	s := state.NewBoardState()
	if err := s.UnmarshalJSON([]byte(`{"v":99,"cells":{}}`)); err == nil {
		t.Error("an unknown document version should be rejected")
	}
}
