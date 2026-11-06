package state

import (
	"slices"

	"github.com/noderunner/bingle/internal/engine/catalog"
)

// A turn of several selections (ADR 0115). A declared game may give its turn
// parts, each with its actions and how many selections it takes. The position
// records how far the turn has gone (BoardState.turn); a selection that does
// not end the turn keeps the seat, so the impulse goes on (ADR 0114), and the
// ply, ko and passes count turns.

// turnParts is the turn in force in st: the opening's during its plies, when it
// declares one, and otherwise the game's. Nil means a turn is one selection.
func turnParts(game *catalog.CompiledGame, st *BoardState) []catalog.TurnPart {
	if op := game.Opening; op != nil && op.Turn != nil && st.ply < op.Plies {
		return op.Turn
	}
	return game.Turn
}

// progress is the turn in progress in st, or its start.
func progress(st *BoardState) TurnProgress {
	if st.turn == nil {
		return TurnProgress{}
	}
	return *st.turn
}

// partFor says which part of the turn a selection of act, of piece, counts in:
// the current part while it has room, or a later one once every part between
// has its min. A swap is a whole turn of its own, so it comes only at a turn's
// start; done is not a part's (see canStop).
func partFor(parts []catalog.TurnPart, st *BoardState, act catalog.ResolvedAction, piece string) (int, bool) {
	p := progress(st)
	if act.Type == catalog.ActionSwap {
		return 0, p.Part == 0 && p.Made == 0
	}
	for k := p.Part; k < len(parts); k++ {
		part, made := parts[k], 0
		if k == p.Part {
			made = p.Made
		}
		used := part.DistinctPiece && k == p.Part && piece != "" && slices.Contains(p.Pieces, piece)
		if part.Actions[act.ID] && made < part.Max && !used {
			return k, true
		}
		if made < part.Min {
			return 0, false
		}
	}
	return 0, false
}

// canStop reports whether the turn may end now with done: every part left has
// its min.
func canStop(parts []catalog.TurnPart, st *BoardState) bool {
	p := progress(st)
	for k := p.Part; k < len(parts); k++ {
		made := 0
		if k == p.Part {
			made = p.Made
		}
		if made < parts[k].Min {
			return false
		}
	}
	return true
}

// pieceOf is the piece type a selection places or moves, for "distinct".
func pieceOf(st *BoardState, mv Selection) string {
	if mv.Piece != "" {
		return mv.Piece
	}
	if mv.From != "" {
		if occ, ok := st.Get(mv.From); ok {
			return occ.Piece
		}
	}
	return ""
}

// goesOn reports whether seat has a selection left in the turn in st other than
// done: when it has none, the turn ends, so the engine never leaves a seat with
// nothing to do but stop.
func goesOn(game *catalog.CompiledGame, st *BoardState, seat int) bool {
	// done is the game's last action, so a first selection found is another
	// unless done is all there is.
	first := legalSelections(game, st, seat, 1)
	return len(first) > 0 && first[0].Action != catalog.ActionDone
}
