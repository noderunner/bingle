// Package primitives implements the built-in movement/occupancy primitive
// library described in docs/history/plan.md section 6.2. Primitives are resolved by
// name (see the DSL's "movement.primitive" field, docs/history/plan.md section 6.1)
// via the Get registry below.
//
// Primitives depend only on board.Graph and the small OccupancyView
// interface defined here — never on internal/engine/state or
// internal/engine/catalog directly — to avoid an import cycle (state depends
// on catalog, catalog depends on primitives to resolve primitive names by
// string; see docs/history/plan.md section 6.2's correction).
package primitives

import "github.com/noderunner/bingle/internal/engine/board"

// Occupancy names the occupancy rule a movement primitive enforces against a
// move's destination.
type Occupancy string

const (
	// OccupancyEmptyOnly requires the destination position be unoccupied.
	OccupancyEmptyOnly Occupancy = "empty-only"
	// OccupancyCaptureOrEmpty additionally allows landing on an
	// opponent-colored piece, which is captured.
	OccupancyCaptureOrEmpty Occupancy = "capture-or-empty"
	// OccupancyCaptureOnly requires the destination hold an opponent piece,
	// which is captured.
	OccupancyCaptureOnly Occupancy = "capture-only"
)

// OccupancyView answers "is there a piece at this position, and if so what
// color owns it" — the only thing a movement primitive needs to know about
// board state. internal/engine/state.BoardState implements this; primitives
// itself has no dependency on the state or catalog packages.
type OccupancyView interface {
	Occupant(id board.PositionID) (color string, occupied bool)
}

// Result is the outcome of evaluating a movement primitive for a candidate
// move.
type Result struct {
	Allowed  bool
	Captured *board.PositionID // non-nil if this move captures an occupant at `to`
	Reason   string            // human-readable rejection reason when !Allowed
}

// MovementPrimitive evaluates whether moving movingColor's piece from `from`
// to `to` is allowed, given the board graph `g`, the current occupancy `occ`,
// and the occupancy rule `occupancy`.
type MovementPrimitive func(g *board.Graph, occ OccupancyView, occupancy Occupancy, from, to board.PositionID, movingColor string) Result

// registry holds the built-in movement primitives by name.
var registry = map[string]MovementPrimitive{
	"step": step,
}

// Get looks up a registered movement primitive by exact, case-sensitive
// name. ok is false if name is not registered, in which case the returned
// function is nil.
func Get(name string) (MovementPrimitive, bool) {
	fn, ok := registry[name]
	if !ok {
		return nil, false
	}
	return fn, true
}
