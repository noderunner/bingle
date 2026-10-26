package board_test

import (
	"testing"

	"github.com/noderunner/bingle/internal/engine/board"
)

// A definition is untrusted input once custom games can be supplied, so board
// generation must refuse sizes that would exhaust memory.
func TestGeneratorsRejectOversizedBoards(t *testing.T) {
	for _, tc := range []struct {
		name string
		gen  func() (*board.Graph, error)
	}{
		{"grid huge", func() (*board.Graph, error) { return board.Generate(1_000_000, 1_000_000, "orthogonal4") }},
		{"grid wide", func() (*board.Graph, error) { return board.Generate(board.MaxCells+1, 1, "orthogonal4") }},
		{"grid product", func() (*board.Graph, error) { return board.Generate(100, 100, "orthogonal4") }},
		{"hex tall", func() (*board.Graph, error) { return board.GenerateHex(11, 1_000_000) }},
	} {
		g, err := tc.gen()
		if err == nil || g != nil {
			t.Errorf("%s: got graph=%v err=%v, want an error and a nil graph", tc.name, g != nil, err)
		}
	}
	g, err := board.Generate(19, 19, "orthogonal4")
	if err != nil || g.Len() != 361 {
		t.Fatalf("19x19 must still be allowed, got err %v", err)
	}
	if _, err := board.Generate(64, 64, "orthogonal4"); err != nil {
		t.Fatalf("64x64 (4096 cells) must be allowed: %v", err)
	}
}
