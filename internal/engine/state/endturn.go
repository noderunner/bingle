package state

import (
	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
)

// Effects at the end of a turn (ADR 0116). A declared game may list them;
// they run whenever a turn ends, except after a swap, before the turn is
// judged.

// endOfTurn applies game's end_of_turn effects to ns, the position after the
// turn seat has just ended, and returns the positions it emptied.
func endOfTurn(game *catalog.CompiledGame, ns *BoardState, seat int) []board.PositionID {
	var removed []board.PositionID
	for _, e := range game.EndOfTurn {
		// Every group is decided on the position before any is taken.
		var taken []board.PositionID
		for _, grp := range noLibertyGroups(game, ns) {
			if takes(game, e.Whose, seat, grp.color) {
				taken = append(taken, grp.members...)
			}
		}
		for _, p := range taken {
			ns.Remove(p)
		}
		if e.CountIn != "" && len(taken) > 0 {
			if def, ok := game.Seat(seat); ok {
				ns.SetReserve(def.Color, e.CountIn, ns.Reserve(def.Color, e.CountIn)+len(taken))
			}
		}
		removed = append(removed, taken...)
	}
	return removed
}

// takes reports whether an effect of whose, at the end of seat's turn,
// removes a group of color.
func takes(game *catalog.CompiledGame, whose string, seat int, color string) bool {
	if whose == catalog.WhoseAll {
		return true
	}
	owner, ok := game.SeatOfColor(color)
	if whose == catalog.WhoseMover {
		return ok && owner == seat
	}
	// The opponents' groups: any colour the mover does not play.
	return !ok || owner != seat
}

type colorGroup struct {
	color   string
	members []board.PositionID
}

// noLibertyGroups is every group in st with no empty position beside it: the
// connected top pieces of one colour, through the board's adjacency.
func noLibertyGroups(game *catalog.CompiledGame, st *BoardState) []colorGroup {
	g := game.Board
	seen := map[board.PositionID]bool{}
	var out []colorGroup
	for _, start := range g.Positions() {
		top, ok := st.Get(start)
		if !ok || seen[start] {
			continue
		}
		members := []board.PositionID{start}
		seen[start] = true
		liberty := false
		for i := 0; i < len(members); i++ {
			for _, n := range g.Neighbors(members[i]) {
				occ, occupied := st.Get(n)
				switch {
				case !occupied:
					liberty = true
				case occ.Color == top.Color && !seen[n]:
					seen[n] = true
					members = append(members, n)
				}
			}
		}
		if !liberty {
			out = append(out, colorGroup{color: top.Color, members: members})
		}
	}
	return out
}

// EndOfTurnTints is the cells the board tints for game's end_of_turn effects
// in st: every group meeting an effect's condition, whoever's it is,
// position -> display.tints key. It is nil when no effect has a tint.
func EndOfTurnTints(game *catalog.CompiledGame, st *BoardState) map[board.PositionID]string {
	var out map[board.PositionID]string
	var groups []colorGroup
	for _, e := range game.EndOfTurn {
		if e.Tint == "" {
			continue
		}
		if out == nil {
			out = map[board.PositionID]string{}
			groups = noLibertyGroups(game, st)
		}
		for _, grp := range groups {
			for _, p := range grp.members {
				if _, set := out[p]; !set {
					out[p] = e.Tint
				}
			}
		}
	}
	return out
}
