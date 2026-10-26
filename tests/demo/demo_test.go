// Tests of the demo game module: it registers, has two seats and resolves its stone piece.

package demo_test

import (
	"testing"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/callback"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/primitives"
	"github.com/noderunner/bingle/internal/games/demo"
)

// noopOccupancyView is a primitives.OccupancyView with no occupants
// anywhere — used to invoke a resolved piece's movement primitive directly
// against two empty, adjacent board positions (test 4's end-to-end
// resolution check), without pulling in internal/engine/state (which this
// tester package must not depend on to test catalog/demo behavior).
type emptyOccupancyView struct{}

func (emptyOccupancyView) Occupant(id board.PositionID) (string, bool) {
	return "", false
}

// TestModule_Shape covers AC 1: demo.Module() returns the expected ID, a
// non-empty embedded Definition, and a nil Callbacks (the catalog is
// responsible for substituting NoopCallbacks — see TestRegisteredGame_Callbacks).
func TestModule_Shape(t *testing.T) {
	mod := demo.Module()

	if mod.ID != "demo-freeplay" {
		t.Errorf("mod.ID = %q, want %q", mod.ID, "demo-freeplay")
	}
	if len(mod.Definition) == 0 {
		t.Error("mod.Definition is empty, want the embedded demo.json contents")
	}
	if mod.Callbacks != nil {
		t.Errorf("mod.Callbacks = %#v, want nil (catalog substitutes NoopCallbacks)", mod.Callbacks)
	}
}

// TestRegister_Succeeds covers AC 2: registering demo.Module()'s fields into
// a fresh Catalog returns a nil error.
func TestRegister_Succeeds(t *testing.T) {
	mod := demo.Module()
	cat := catalog.NewCatalog()

	if err := cat.Register(mod.ID, mod.Definition, mod.Callbacks); err != nil {
		t.Fatalf("cat.Register(%q, ...) returned error: %v, want nil", mod.ID, err)
	}
}

// mustRegisterDemo is a shared test helper: registers demo.Module() into a
// fresh Catalog and returns the resulting CompiledGame, failing the test
// immediately (via t.Fatalf) if registration or lookup fails.
func mustRegisterDemo(t *testing.T) *catalog.CompiledGame {
	t.Helper()

	mod := demo.Module()
	cat := catalog.NewCatalog()

	if err := cat.Register(mod.ID, mod.Definition, mod.Callbacks); err != nil {
		t.Fatalf("cat.Register(%q, ...) returned error: %v, want nil", mod.ID, err)
	}

	cg, ok := cat.Get("demo-freeplay")
	if !ok {
		t.Fatalf("cat.Get(%q) returned ok=false, want true", "demo-freeplay")
	}
	if cg == nil {
		t.Fatalf("cat.Get(%q) returned nil *CompiledGame with ok=true", "demo-freeplay")
	}
	return cg
}

// TestGet_ReturnsCompiledGameWithTwoSeats is the backlog item's AC itself:
// Catalog.Get("demo-freeplay") returns a compiled game with two seats,
// colored black and white (order-independent).
func TestGet_ReturnsCompiledGameWithTwoSeats(t *testing.T) {
	cg := mustRegisterDemo(t)

	if cg.Definition == nil {
		t.Fatal("cg.Definition is nil, want a parsed *dsl.GameDefinition")
	}

	seats := cg.Definition.Seats
	if len(seats) != 2 {
		t.Fatalf("len(cg.Definition.Seats) = %d, want 2 (got %#v)", len(seats), seats)
	}

	colors := map[string]bool{}
	for _, s := range seats {
		colors[s.Color] = true
	}
	want := map[string]bool{"black": true, "white": true}
	if len(colors) != len(want) {
		t.Fatalf("seat colors = %v, want set {black, white}", colors)
	}
	for c := range want {
		if !colors[c] {
			t.Errorf("seat colors %v missing %q", colors, c)
		}
	}
}

// TestStonePiece_ResolvesAndAllowsEmptyAdjacentStep covers AC 4: cg.Pieces
// has a "stone" entry with OccupancyCaptureOrEmpty, and invoking its
// Primitive directly on two adjacent, empty board positions returns
// Allowed: true — proving full end-to-end resolution (board generation +
// primitive registry lookup working together), not just structural presence
// of the map entry.
func TestStonePiece_ResolvesAndAllowsEmptyAdjacentStep(t *testing.T) {
	cg := mustRegisterDemo(t)

	stone, ok := cg.Pieces["stone"]
	if !ok {
		t.Fatal(`cg.Pieces["stone"] missing, want a resolved piece for the "stone" type`)
	}
	if stone.Occupancy != primitives.OccupancyCaptureOrEmpty {
		t.Errorf("stone.Occupancy = %q, want %q", stone.Occupancy, primitives.OccupancyCaptureOrEmpty)
	}
	if stone.Primitive == nil {
		t.Fatal("stone.Primitive is nil, want a resolved MovementPrimitive func")
	}

	from := board.PositionID("0,0")
	to := board.PositionID("1,0")

	if !cg.Board.HasPosition(from) {
		t.Fatalf("cg.Board does not contain position %q, want it present on the 9x9 grid", from)
	}
	if !cg.Board.HasPosition(to) {
		t.Fatalf("cg.Board does not contain position %q, want it present on the 9x9 grid", to)
	}
	if !cg.Board.HasEdge(from, to) {
		t.Fatalf("cg.Board has no edge between %q and %q, want them orthogonally adjacent", from, to)
	}

	result := stone.Primitive(cg.Board, emptyOccupancyView{}, stone.Occupancy, from, to, "black")
	if !result.Allowed {
		t.Errorf("stone.Primitive(%q -> %q, both empty) = %+v, want Allowed: true", from, to, result)
	}
	if result.Captured != nil {
		t.Errorf("stone.Primitive(%q -> %q, both empty) reported Captured=%v, want nil", from, to, result.Captured)
	}
}

// TestBoard_Is9x9Orthogonal4Grid covers AC 5: cg.Board is a 9x9 grid (81
// distinct positions) that is orthogonal4-connected (an interior position
// has exactly 4 neighbors).
func TestBoard_Is9x9Orthogonal4Grid(t *testing.T) {
	cg := mustRegisterDemo(t)

	count := 0
	for x := 0; x < 9; x++ {
		for y := 0; y < 9; y++ {
			id := board.PositionID(coord(x, y))
			if cg.Board.HasPosition(id) {
				count++
			} else {
				t.Errorf("cg.Board missing expected position %q", id)
			}
		}
	}
	if count != 81 {
		t.Errorf("cg.Board has %d of the expected 9x9=81 positions", count)
	}

	// Positions outside the 9x9 range must not be present.
	for _, outside := range []board.PositionID{"9,0", "0,9", "-1,0", "0,-1"} {
		if cg.Board.HasPosition(outside) {
			t.Errorf("cg.Board unexpectedly contains out-of-range position %q", outside)
		}
	}

	// An interior position (e.g. (4,4), the center of a 9x9 grid) must have
	// exactly 4 neighbors under orthogonal4 adjacency.
	interior := board.PositionID(coord(4, 4))
	neighbors := cg.Board.Neighbors(interior)
	if len(neighbors) != 4 {
		t.Errorf("cg.Board.Neighbors(%q) = %v (len %d), want exactly 4 (orthogonal4)", interior, neighbors, len(neighbors))
	}

	// A corner position must have exactly 2 neighbors (fewer than an
	// interior position), confirming this isn't e.g. a torus/wraparound grid.
	corner := board.PositionID(coord(0, 0))
	cornerNeighbors := cg.Board.Neighbors(corner)
	if len(cornerNeighbors) != 2 {
		t.Errorf("cg.Board.Neighbors(%q) = %v (len %d), want exactly 2 (corner, orthogonal4)", corner, cornerNeighbors, len(cornerNeighbors))
	}
}

// coord formats an (x, y) grid coordinate as the "x,y" PositionID string
// used by the grid board generator (docs/history/plan.md section 6.1's setup
// entries, e.g. "0,0"/"8,8").
func coord(x, y int) string {
	return itoa(x) + "," + itoa(y)
}

// itoa is a tiny local int-to-string helper so this test file needs no
// extra import beyond the standard testing/engine packages already listed.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if neg {
		return "-" + string(digits)
	}
	return string(digits)
}

// TestWinCondition_IsNone covers AC 6: cg.Definition.WinCondition.Type ==
// "none", per section 6.1's demo win-condition decision.
func TestWinCondition_IsNone(t *testing.T) {
	cg := mustRegisterDemo(t)

	if cg.Definition.WinCondition.Type != "none" {
		t.Errorf("cg.Definition.WinCondition.Type = %q, want %q", cg.Definition.WinCondition.Type, "none")
	}
}

// TestCallbacks_SubstitutedWithNoop covers AC 7: since demo.Module() passes
// nil Callbacks, the catalog must substitute callback.NoopCallbacks — proven
// here by checking cg.Callbacks is non-nil and behaves as a no-op (returns
// nil from ValidateMove given a zero-value MoveContext).
func TestCallbacks_SubstitutedWithNoop(t *testing.T) {
	cg := mustRegisterDemo(t)

	if cg.Callbacks == nil {
		t.Fatal("cg.Callbacks is nil, want catalog to have substituted callback.NoopCallbacks for the demo module's nil Callbacks")
	}

	err := cg.Callbacks.ValidateMove(callback.MoveContext{})
	if err != nil {
		t.Errorf("cg.Callbacks.ValidateMove(zero-value MoveContext) = %v, want nil (no-op)", err)
	}
}

// TestSetup_HasThreeStonesEachColor covers AC 8: cg.Definition.Setup has
// exactly 6 entries, 3 colored "black" and 3 colored "white" — deliberately
// not asserting exact position strings, so minor placement tweaks in the
// authored demo.json don't break this test.
func TestSetup_HasThreeStonesEachColor(t *testing.T) {
	cg := mustRegisterDemo(t)

	setup := cg.Definition.Setup
	if len(setup) != 6 {
		t.Fatalf("len(cg.Definition.Setup) = %d, want 6 (got %#v)", len(setup), setup)
	}

	var black, white, other int
	for _, entry := range setup {
		switch entry.Color {
		case "black":
			black++
		case "white":
			white++
		default:
			other++
		}
		if entry.Piece != "stone" {
			t.Errorf("setup entry %+v has Piece = %q, want %q", entry, entry.Piece, "stone")
		}
	}

	if black != 3 {
		t.Errorf("setup has %d black entries, want 3", black)
	}
	if white != 3 {
		t.Errorf("setup has %d white entries, want 3", white)
	}
	if other != 0 {
		t.Errorf("setup has %d entries with a color other than black/white, want 0", other)
	}
}
