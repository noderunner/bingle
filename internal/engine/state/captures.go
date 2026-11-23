package state

import (
	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
)

// SelectionCaptures lists the positions whose pieces mv would take off the
// board if seat made it in st, so that a client can show them before the move
// is confirmed. It applies mv to a copy, so it says exactly what Apply would:
// a draughts chain's jumped men, the stones a placement leaves without
// liberties, the end of the turn's captures. It is nil when mv captures
// nothing or is not legal, in a game with secrets (what a selection takes may
// depend on what the seat may not see), and when mv would show the seat
// something it cannot know yet (a roll, a draw), which a query must not.
func SelectionCaptures(game *catalog.CompiledGame, st *BoardState, seat int, mv Selection) []board.PositionID {
	if keepsSecrets(game) {
		return nil
	}
	res, err := Apply(game, st, seat, mv)
	if err != nil || len(res.Captures) == 0 || !CanTakeBack(game, st, res.NewState) {
		return nil
	}
	return res.Captures
}
