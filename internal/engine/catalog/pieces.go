package catalog

import (
	"fmt"

	"github.com/noderunner/bingle/internal/engine/dsl"
	"github.com/noderunner/bingle/internal/engine/primitives"
)

// resolvePieces resolves every piece type in def.Pieces against the
// primitives registry, returning an error naming both the piece type and
// the unknown primitive name if any movement primitive is not registered.
// A piece that declares no movement at all (empty primitive and occupancy)
// is allowed: it simply has no step moves.
func resolvePieces(def *dsl.GameDefinition) (map[string]ResolvedPiece, error) {
	pieces := make(map[string]ResolvedPiece, len(def.Pieces))
	for pieceType, pieceDef := range def.Pieces {
		rp := ResolvedPiece{
			Tags:      make(map[string]bool, len(pieceDef.Tags)),
			Coverable: pieceDef.Stack.Coverable == nil || *pieceDef.Stack.Coverable,
			Pool:      pieceDef.Pool,
		}
		if rp.Pool == "" {
			rp.Pool = pieceType
		}
		for _, tag := range pieceDef.Tags {
			rp.Tags[tag] = true
		}

		mv := pieceDef.Movement
		if mv.Primitive != "" || mv.Occupancy != "" {
			fn, ok := primitives.Get(mv.Primitive)
			if !ok {
				return nil, fmt.Errorf("catalog: piece %q references unknown movement primitive %q", pieceType, mv.Primitive)
			}
			rp.Primitive = fn
			rp.Occupancy = primitives.Occupancy(mv.Occupancy)
		}
		pieces[pieceType] = rp
	}

	for pieceType, pieceDef := range def.Pieces {
		for _, by := range pieceDef.Stack.FlattenableBy {
			if _, ok := def.Pieces[by]; !ok {
				return nil, fmt.Errorf("catalog: piece %q is flattenable_by unknown piece %q", pieceType, by)
			}
		}
		if len(pieceDef.Stack.FlattenableBy) > 0 {
			if _, ok := def.Pieces[pieceDef.Stack.FlattenTo]; !ok {
				return nil, fmt.Errorf("catalog: piece %q flatten_to %q is not a piece type", pieceType, pieceDef.Stack.FlattenTo)
			}
		}
	}
	return pieces, nil
}
