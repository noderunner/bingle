// Tests of the step primitive's capture rules (capture-or-empty and capture-only), for every colour pairing.

package primitives_test

import (
	"testing"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/primitives"
)

// --- step, occupancy=capture-or-empty: adjacent + unoccupied → allowed -----

func TestStep_CaptureOrEmpty_AdjacentAndUnoccupied_Allowed(t *testing.T) {
	g := adjacentGrid(t)
	step, ok := primitives.Get("step")
	if !ok {
		t.Fatalf("Get(%q) ok = false, want true", "step")
	}

	from := board.PositionID("1,1")
	to := board.PositionID("1,0")
	if !g.HasEdge(from, to) {
		t.Fatalf("fixture error: expected HasEdge(%s,%s) = true", from, to)
	}

	occ := fakeOccupancy{from: "black"} // `to` is unoccupied
	res := step(g, occ, primitives.OccupancyCaptureOrEmpty, from, to, "black")

	if !res.Allowed {
		t.Errorf("Allowed = false, want true (reason: %q)", res.Reason)
	}
	if res.Captured != nil {
		t.Errorf("Captured = %v, want nil (destination was empty, nothing to capture)", *res.Captured)
	}
}

// --- step, occupancy=capture-or-empty: adjacent + opponent-occupied → -----
// --- allowed, reports capture -----------------------------------------------

func TestStep_CaptureOrEmpty_AdjacentAndOpponentOccupied_AllowedAndCaptured(t *testing.T) {
	g := adjacentGrid(t)
	step, ok := primitives.Get("step")
	if !ok {
		t.Fatalf("Get(%q) ok = false, want true", "step")
	}

	from := board.PositionID("1,1")
	to := board.PositionID("1,0")
	occ := fakeOccupancy{from: "black", to: "white"}

	res := step(g, occ, primitives.OccupancyCaptureOrEmpty, from, to, "black")

	if !res.Allowed {
		t.Errorf("Allowed = false, want true (reason: %q)", res.Reason)
	}
	if res.Captured == nil {
		t.Fatalf("Captured = nil, want non-nil *board.PositionID equal to %s", to)
	}
	if *res.Captured != to {
		t.Errorf("Captured = %v, want %v", *res.Captured, to)
	}
}

// Sweep several distinct opponent-color pairs to make sure "different from
// movingColor" isn't hardcoded to one specific pair (e.g. "black"/"white").
func TestStep_CaptureOrEmpty_AdjacentAndOpponentOccupied_AllowedAndCaptured_ColorSweep(t *testing.T) {
	g := adjacentGrid(t)
	step, ok := primitives.Get("step")
	if !ok {
		t.Fatalf("Get(%q) ok = false, want true", "step")
	}

	from := board.PositionID("1,1")
	to := board.PositionID("1,0")

	cases := []struct {
		name        string
		movingColor string
		occupant    string
	}{
		{"black_vs_white", "black", "white"},
		{"white_vs_black", "white", "black"},
		{"red_vs_blue", "red", "blue"},
		{"gold_vs_silver", "gold", "silver"},
		{"p1_vs_p2", "p1", "p2"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			occ := fakeOccupancy{from: tc.movingColor, to: tc.occupant}
			res := step(g, occ, primitives.OccupancyCaptureOrEmpty, from, to, tc.movingColor)

			if !res.Allowed {
				t.Errorf("Allowed = false, want true (mover %q, occupant %q, reason: %q)", tc.movingColor, tc.occupant, res.Reason)
			}
			if res.Captured == nil {
				t.Fatalf("Captured = nil, want non-nil *board.PositionID equal to %s (mover %q, occupant %q)", to, tc.movingColor, tc.occupant)
			}
			if *res.Captured != to {
				t.Errorf("Captured = %v, want %v (mover %q, occupant %q)", *res.Captured, to, tc.movingColor, tc.occupant)
			}
		})
	}
}

// --- step, occupancy=capture-or-empty: adjacent + same-color-occupied → ---
// --- rejected ----------------------------------------------------------------

func TestStep_CaptureOrEmpty_AdjacentAndSameColorOccupied_Rejected(t *testing.T) {
	g := adjacentGrid(t)
	step, ok := primitives.Get("step")
	if !ok {
		t.Fatalf("Get(%q) ok = false, want true", "step")
	}

	from := board.PositionID("1,1")
	to := board.PositionID("1,0")
	occ := fakeOccupancy{from: "black", to: "black"}

	res := step(g, occ, primitives.OccupancyCaptureOrEmpty, from, to, "black")

	if res.Allowed {
		t.Errorf("Allowed = true, want false (destination occupied by mover's own color)")
	}
	if res.Captured != nil {
		t.Errorf("Captured = %v, want nil (same-color destination must never be reported as captured)", *res.Captured)
	}
	if res.Reason == "" {
		t.Error("Reason = \"\", want non-empty rejection reason")
	}
}

// Sweep several distinct same-color pairs (including a shared empty-string
// color) to make sure the same-color rejection isn't hardcoded to one value.
func TestStep_CaptureOrEmpty_AdjacentAndSameColorOccupied_Rejected_ColorSweep(t *testing.T) {
	g := adjacentGrid(t)
	step, ok := primitives.Get("step")
	if !ok {
		t.Fatalf("Get(%q) ok = false, want true", "step")
	}

	from := board.PositionID("1,1")
	to := board.PositionID("1,0")

	colors := []string{"black", "white", "red", "gold", ""}
	for _, c := range colors {
		t.Run(c, func(t *testing.T) {
			occ := fakeOccupancy{from: c, to: c}
			res := step(g, occ, primitives.OccupancyCaptureOrEmpty, from, to, c)

			if res.Allowed {
				t.Errorf("Allowed = true, want false (mover and occupant both color %q)", c)
			}
			if res.Captured != nil {
				t.Errorf("Captured = %v, want nil", *res.Captured)
			}
			if res.Reason == "" {
				t.Error("Reason = \"\", want non-empty rejection reason")
			}
		})
	}
}

// --- step, occupancy=capture-or-empty: non-adjacent → rejected regardless -

func TestStep_CaptureOrEmpty_NonAdjacent_OppositeCorners_RejectedWhenDestinationEmpty(t *testing.T) {
	g := adjacentGrid(t)
	step, ok := primitives.Get("step")
	if !ok {
		t.Fatalf("Get(%q) ok = false, want true", "step")
	}

	from := board.PositionID("0,0")
	to := board.PositionID("2,2")
	if g.HasEdge(from, to) {
		t.Fatalf("fixture error: expected HasEdge(%s,%s) = false (opposite corners)", from, to)
	}

	occ := fakeOccupancy{from: "black"} // to is unoccupied
	res := step(g, occ, primitives.OccupancyCaptureOrEmpty, from, to, "black")

	if res.Allowed {
		t.Errorf("Allowed = true, want false (from/to not graph-adjacent)")
	}
	if res.Captured != nil {
		t.Errorf("Captured = %v, want nil", *res.Captured)
	}
	if res.Reason == "" {
		t.Error("Reason = \"\", want non-empty rejection reason")
	}
}

func TestStep_CaptureOrEmpty_NonAdjacent_OppositeCorners_RejectedWhenDestinationOpponentOccupied(t *testing.T) {
	g := adjacentGrid(t)
	step, ok := primitives.Get("step")
	if !ok {
		t.Fatalf("Get(%q) ok = false, want true", "step")
	}

	from := board.PositionID("0,0")
	to := board.PositionID("2,2")

	occ := fakeOccupancy{from: "black", to: "white"} // would otherwise be a valid capture
	res := step(g, occ, primitives.OccupancyCaptureOrEmpty, from, to, "black")

	if res.Allowed {
		t.Errorf("Allowed = true, want false (from/to not graph-adjacent, regardless of occupancy or capture eligibility)")
	}
	if res.Captured != nil {
		t.Errorf("Captured = %v, want nil (rejected move must never report a capture)", *res.Captured)
	}
	if res.Reason == "" {
		t.Error("Reason = \"\", want non-empty rejection reason")
	}
}

func TestStep_CaptureOrEmpty_NoEdgeAtAllBetweenPositions_Rejected(t *testing.T) {
	// A graph where the two positions have no edge whatsoever (not merely
	// "far apart" on a grid, but truly disconnected) — re-verifying the
	// adjacency gate applies under capture-or-empty too, same as it does for
	// empty-only.
	g := disconnectedGraph(t)
	step, ok := primitives.Get("step")
	if !ok {
		t.Fatalf("Get(%q) ok = false, want true", "step")
	}

	from := board.PositionID("a")
	to := board.PositionID("b")
	if g.HasEdge(from, to) {
		t.Fatalf("fixture error: expected HasEdge(%s,%s) = false (no edges in graph)", from, to)
	}

	t.Run("destination_empty", func(t *testing.T) {
		occ := fakeOccupancy{from: "black"}
		res := step(g, occ, primitives.OccupancyCaptureOrEmpty, from, to, "black")
		if res.Allowed {
			t.Errorf("Allowed = true, want false (no edge between positions)")
		}
		if res.Captured != nil {
			t.Errorf("Captured = %v, want nil", *res.Captured)
		}
		if res.Reason == "" {
			t.Error("Reason = \"\", want non-empty rejection reason")
		}
	})

	t.Run("destination_opponent_occupied", func(t *testing.T) {
		occ := fakeOccupancy{from: "black", to: "white"}
		res := step(g, occ, primitives.OccupancyCaptureOrEmpty, from, to, "black")
		if res.Allowed {
			t.Errorf("Allowed = true, want false (no edge between positions, regardless of occupancy)")
		}
		if res.Captured != nil {
			t.Errorf("Captured = %v, want nil", *res.Captured)
		}
		if res.Reason == "" {
			t.Error("Reason = \"\", want non-empty rejection reason")
		}
	})

	t.Run("destination_same_color_occupied", func(t *testing.T) {
		occ := fakeOccupancy{from: "black", to: "black"}
		res := step(g, occ, primitives.OccupancyCaptureOrEmpty, from, to, "black")
		if res.Allowed {
			t.Errorf("Allowed = true, want false (no edge between positions)")
		}
		if res.Captured != nil {
			t.Errorf("Captured = %v, want nil", *res.Captured)
		}
		if res.Reason == "" {
			t.Error("Reason = \"\", want non-empty rejection reason")
		}
	})
}
