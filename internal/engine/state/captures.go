package state

import (
	"slices"
	"strings"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
)

// SelectionEffects lists what mv would do to the pieces on the board if seat
// made it in st, so that a client can show it before the move is confirmed:
// captures are the positions whose pieces it takes off the board (a draughts
// chain's jumped men, the stones a placement leaves without liberties, the end
// of the turn's captures), converts those whose pieces keep their place and
// change colour (Othello's flips, Fist's ace converting what it jumps), with
// the colour each turns to. It
// applies mv to a copy, so it says exactly what Apply would. Both are nil when
// mv is not legal, in a game with secrets (what a selection does may depend on
// what the seat may not see), and when mv would show the seat something it
// cannot know yet (a roll, a draw), which a query must not.
func SelectionEffects(game *catalog.CompiledGame, st *BoardState, seat int, mv Selection) (captures []board.PositionID, converts []Conversion) {
	if keepsSecrets(game) {
		return nil, nil
	}
	res, err := Apply(game, st, seat, mv)
	if err != nil || !CanTakeBack(game, st, res.NewState) {
		return nil, nil
	}
	if len(res.Captures) > 0 {
		captures = res.Captures
	}
	return captures, conversions(st, res.NewState, res.Captures)
}

// SelectionCaptures is the captures of SelectionEffects.
func SelectionCaptures(game *catalog.CompiledGame, st *BoardState, seat int, mv Selection) []board.PositionID {
	captures, _ := SelectionEffects(game, st, seat, mv)
	return captures
}

// A Conversion is a position whose piece a selection turns over, and the
// colour the top piece there turns to.
type Conversion struct {
	At    board.PositionID
	Color string
}

// conversions are the positions, sorted, whose stack has the same height and
// the same kinds of piece before and after but not the same colours: a piece
// turned over where it stands. A capture by a piece of the kind it takes (a
// pawn taking a pawn) looks the same, so the positions captured are left out;
// a promotion changes the kind and a placement fills an empty position, so
// neither is a conversion.
func conversions(before, after *BoardState, captured []board.PositionID) []Conversion {
	var out []Conversion
	for id, was := range before.cells {
		now := after.cells[id]
		if len(was) == 0 || len(now) != len(was) || slices.Contains(captured, id) {
			continue
		}
		turned := false
		for i := range was {
			if was[i].Piece != now[i].Piece {
				turned = false
				break
			}
			turned = turned || was[i].Color != now[i].Color
		}
		if turned {
			out = append(out, Conversion{At: id, Color: now[len(now)-1].Color})
		}
	}
	slices.SortFunc(out, func(a, b Conversion) int { return strings.Compare(string(a.At), string(b.At)) })
	return out
}
