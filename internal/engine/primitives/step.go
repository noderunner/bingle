package primitives

import (
	"fmt"

	"github.com/noderunner/bingle/internal/engine/board"
)

// step implements the "step" movement primitive (docs/history/plan.md section 6.1):
// move a piece from an occupied position to one graph-adjacent position.
//
// occupancy=empty-only requires the destination be unoccupied.
// occupancy=capture-or-empty additionally allows moving onto an
// opponent-occupied destination, reporting it captured, while still
// rejecting a same-color-occupied destination.
func step(g *board.Graph, occ OccupancyView, occupancy Occupancy, from, to board.PositionID, movingColor string) Result {
	if !g.HasEdge(from, to) {
		return Result{Allowed: false, Reason: fmt.Sprintf("step: %s and %s are not graph-adjacent", from, to)}
	}

	switch occupancy {
	case OccupancyEmptyOnly:
		if _, occupied := occ.Occupant(to); occupied {
			return Result{Allowed: false, Reason: fmt.Sprintf("step: destination %s is occupied (occupancy=empty-only)", to)}
		}
		return Result{Allowed: true}
	case OccupancyCaptureOrEmpty:
		color, occupied := occ.Occupant(to)
		if !occupied {
			return Result{Allowed: true}
		}
		if color == movingColor {
			return Result{Allowed: false, Reason: fmt.Sprintf("step: destination %s is occupied by mover's own color %q (occupancy=capture-or-empty)", to, movingColor)}
		}
		captured := to
		return Result{Allowed: true, Captured: &captured}
	case OccupancyCaptureOnly:
		color, occupied := occ.Occupant(to)
		if !occupied {
			return Result{Allowed: false, Reason: fmt.Sprintf("step: destination %s is empty (occupancy=capture-only: a move there must capture)", to)}
		}
		if color == movingColor {
			return Result{Allowed: false, Reason: fmt.Sprintf("step: destination %s is occupied by mover's own color %q (occupancy=capture-only)", to, movingColor)}
		}
		captured := to
		return Result{Allowed: true, Captured: &captured}
	default:
		return Result{Allowed: false, Reason: fmt.Sprintf("step: occupancy mode %q is not yet supported", occupancy)}
	}
}
