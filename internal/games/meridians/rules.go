// Package meridians ships Meridians (Kanare Kato, 2021): a placement game on
// the intersections of a triangular grid where a group of stones that can see
// no other group of its colour dies. You win by taking every stone of your
// opponent's off the board.
//
// The rules are a Go state.Ruleset — line of sight, paths between groups and
// the removal of dead groups are none of them expressible in the JSON DSL —
// and meridians.json describes the board (a hexagon with no middle point),
// the two seats and the one kind of stone. There are no secrets and no chance.
package meridians

import (
	"fmt"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

// Ruleset is Meridians' state.Ruleset.
type Ruleset struct{}

// New returns the ruleset.
func New() *Ruleset { return &Ruleset{} }

// The actions of a turn: a stone, or nothing when there is nowhere to put one.
const (
	actPlace = "place"
	actPass  = "pass"
)

// eng is a decoded position: stones[i] is the seat whose stone is on point i
// (0 for an empty point).
type eng struct {
	g      *geo
	d      *Data
	stones []int
	game   *catalog.CompiledGame
	ev     []string
}

func newEng(game *catalog.CompiledGame, st *state.BoardState, d *Data) *eng {
	g := geoOf(game)
	e := &eng{g: g, d: d, stones: make([]int, len(g.ids)), game: game}
	for _, id := range st.Occupied() {
		if i, ok := g.index[id]; ok {
			occ, _ := st.Get(id)
			e.stones[i] = seatOf(occ.Color)
		}
	}
	return e
}

func (e *eng) save(st *state.BoardState) {
	for _, id := range st.Occupied() {
		st.Remove(id)
	}
	for i, seat := range e.stones {
		if seat != 0 {
			st.Set(e.g.ids[i], state.Occupant{Piece: "stone", Color: seatColor(seat)})
		}
	}
}

func (e *eng) count(seat int) int {
	n := 0
	for _, s := range e.stones {
		if s == seat {
			n++
		}
	}
	return n
}

// ---- groups, sight and paths -------------------------------------------------

// groups returns the groups of seat: the sets of its stones that touch one
// another, each as a sorted list of points.
func (e *eng) groups(seat int) [][]int {
	seen := make([]bool, len(e.stones))
	var out [][]int
	for i, s := range e.stones {
		if s != seat || seen[i] {
			continue
		}
		group := []int{i}
		seen[i] = true
		for n := 0; n < len(group); n++ {
			for _, j := range e.g.nbr[group[n]] {
				if e.stones[j] == seat && !seen[j] {
					seen[j] = true
					group = append(group, j)
				}
			}
		}
		out = append(out, group)
	}
	return out
}

// seesGroup reports whether the stone on point i has a path to a stone of the
// same colour that is not in its own group: a straight line of one or more
// *empty* points ending on such a stone.
func (e *eng) seesGroup(i int, group map[int]bool, seat int) bool {
	for d := range dirVec {
		for n, j := range e.g.ray[i][d] {
			if e.stones[j] == 0 {
				continue // an empty point: the line goes on
			}
			// The first stone along the line: a path needs at least one empty
			// point before it, the stone to be ours, and it to belong to
			// another group of ours.
			if n > 0 && e.stones[j] == seat && !group[j] {
				return true
			}
			break
		}
	}
	return false
}

// deadGroups returns the groups of seat that have no path to another group of
// theirs: the groups that come off the board when their owner's turn ends.
func (e *eng) deadGroups(seat int) [][]int {
	groups := e.groups(seat)
	if len(groups) < 2 {
		// A player with one group (or none) has nothing to see: a single group
		// is dead by the rules, which is why the opening stones are exempt.
		return groups
	}
	var dead [][]int
	for _, group := range groups {
		members := make(map[int]bool, len(group))
		for _, i := range group {
			members[i] = true
		}
		alive := false
		for _, i := range group {
			if e.seesGroup(i, members, seat) {
				alive = true
				break
			}
		}
		if !alive {
			dead = append(dead, group)
		}
	}
	return dead
}

// ---- the moves of a turn -----------------------------------------------------

// placements lists the empty points seat may lay a stone on.
func (e *eng) placements(seat int) []int {
	// A player with no stones begins anywhere.
	if e.count(seat) == 0 {
		var out []int
		for i, s := range e.stones {
			if s == 0 {
				out = append(out, i)
			}
		}
		return out
	}
	// Otherwise: a point one of our stones can see along a straight line, with
	// no stone of the other colour in the way. (One of ours in the way makes no
	// difference: that stone sees the same point along the same line.)
	seen := make([]bool, len(e.stones))
	for i, s := range e.stones {
		if s != seat {
			continue
		}
		for d := range dirVec {
			for _, j := range e.g.ray[i][d] {
				if e.stones[j] == other(seat) {
					break
				}
				if e.stones[j] == 0 {
					seen[j] = true
				}
			}
		}
	}
	// The second stone of a player must not touch their first.
	var forbidden map[int]bool
	if e.d.Turns[seat-1] == 1 {
		if first, ok := e.g.index[board.PositionID(e.d.First[seat-1])]; ok {
			forbidden = map[int]bool{}
			for _, j := range e.g.nbr[first] {
				forbidden[j] = true
			}
		}
	}
	var out []int
	for i, ok := range seen {
		if ok && !forbidden[i] {
			out = append(out, i)
		}
	}
	return out
}

func (e *eng) legal(seat int) []state.Move {
	points := e.placements(seat)
	if len(points) == 0 {
		// No stone can be laid: the turn passes, which is the only time it may.
		return []state.Move{{Action: actPass}}
	}
	out := make([]state.Move, 0, len(points))
	for _, i := range points {
		out = append(out, state.Move{Action: actPlace, Piece: "stone", To: e.g.ids[i]})
	}
	return out
}

// ---- the Ruleset ------------------------------------------------------------

// Start is the empty board, Light to move.
func (rs *Ruleset) Start(game *catalog.CompiledGame, seed uint64) (*state.BoardState, error) {
	st := state.NewBoardState()
	st.SetExt(&Data{Turn: 1})
	return st, nil
}

// ToMove is the seat to move, or 0 once the game is over.
func (rs *Ruleset) ToMove(game *catalog.CompiledGame, st *state.BoardState) int {
	d, err := dataOf(st)
	if err != nil {
		return 0
	}
	return d.Turn
}

// Outcome reports a finished game.
func (rs *Ruleset) Outcome(game *catalog.CompiledGame, st *state.BoardState) state.Outcome {
	d, err := dataOf(st)
	if err != nil || d.Turn != 0 {
		return state.Outcome{}
	}
	out := state.Outcome{Ended: true, Reason: d.Reason}
	if d.Winner != 0 {
		w := d.Winner
		out.Winner = &w
		out.Detail = fmt.Sprintf("%s has no stones left", seatLabel(other(w)))
	}
	return out
}

// Validate checks a position document.
func (rs *Ruleset) Validate(game *catalog.CompiledGame, st *state.BoardState) error {
	d, err := dataOf(st)
	if err != nil {
		return err
	}
	if d.Turn < 0 || d.Turn > 2 {
		return fmt.Errorf("meridians: bad turn %d", d.Turn)
	}
	for seat := 1; seat <= 2; seat++ {
		if d.Turns[seat-1] < 0 {
			return fmt.Errorf("meridians: %s has taken %d turns", seatLabel(seat), d.Turns[seat-1])
		}
	}
	g := geoOf(game)
	for _, id := range st.Occupied() {
		occ, _ := st.Get(id)
		if seatOf(occ.Color) == 0 || occ.Piece != "stone" {
			return fmt.Errorf("meridians: bad stone %s/%s at %s", occ.Color, occ.Piece, id)
		}
		if st.Height(id) > 1 {
			return fmt.Errorf("meridians: two stones on %s", id)
		}
		if _, ok := g.index[id]; !ok {
			return fmt.Errorf("meridians: %s is not a point of this board", id)
		}
	}
	return nil
}

// Legal lists the moves of seat.
func (rs *Ruleset) Legal(game *catalog.CompiledGame, st *state.BoardState, seat int) []state.Move {
	d, err := dataOf(st)
	if err != nil || d.Turn == 0 || seat != d.Turn {
		return nil
	}
	return newEng(game, st, d).legal(seat)
}

// Apply lays a stone (or passes), takes off the mover's dead groups and hands
// the turn over.
func (rs *Ruleset) Apply(game *catalog.CompiledGame, st *state.BoardState, seat int, mv state.Move) (*state.MoveResult, error) {
	d, err := dataOf(st)
	if err != nil {
		return nil, err
	}
	if d.Turn == 0 {
		return nil, fmt.Errorf("meridians: the game is over")
	}
	if seat != d.Turn {
		return nil, fmt.Errorf("meridians: it is %s's turn", seatLabel(d.Turn))
	}
	e := newEng(game, st, d)
	want := rs.Format(game, mv)
	var legal state.Move
	found := false
	for _, cand := range e.legal(seat) {
		if rs.Format(game, cand) == want {
			legal, found = cand, true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("meridians: %q is not a legal move now", want)
	}

	nd := d.clone()
	e.d = nd
	e.ev = nil
	ns := st.Clone()
	e.play(legal, seat)
	nd.Events = e.ev
	ns.SetExt(nd)
	e.save(ns)
	ns.SetPly(st.Ply() + 1)
	if nd.Turn != 0 && ns.Ply() >= game.ParamInt("max_plies", 1200) {
		e.finish(0, "ply-limit")
		nd.Events = e.ev
	}
	res := &state.MoveResult{NewState: ns, Move: legal, Notation: rs.Format(game, legal), NextTurnSeat: nd.Turn}
	if nd.Turn == 0 {
		res.GameEnded = true
		res.NextTurnSeat = seat
		res.EndReason = nd.Reason
		if nd.Winner != 0 {
			w := nd.Winner
			res.WinnerSeat = &w
			res.EndDetail = fmt.Sprintf("%s has no stones left", seatLabel(other(w)))
		}
		nd.Events = e.ev
	}
	return res, nil
}

func (e *eng) say(format string, args ...any) { e.ev = append(e.ev, fmt.Sprintf(format, args...)) }

func (e *eng) finish(winner int, reason string) {
	e.d.Turn, e.d.Winner, e.d.Reason = 0, winner, reason
	switch reason {
	case "ply-limit":
		e.say("The move limit is reached: the game is a draw.")
	case "stuck":
		e.say("Neither player can lay a stone: the game is a draw.")
	case "wiped-out":
		e.say("%s has no stones left: %s wins.", seatLabel(other(winner)), seatLabel(winner))
	}
}

// play carries out one turn: the stone, then the mover's dead groups, then the
// turn passes.
func (e *eng) play(mv state.Move, seat int) {
	d := e.d
	d.Turns[seat-1]++
	switch mv.Action {
	case actPlace:
		i := e.g.index[mv.To]
		e.stones[i] = seat
		d.Last = string(mv.To)
		if d.Turns[seat-1] == 1 {
			d.First[seat-1] = string(mv.To)
		}
		d.Passes = 0
		e.say("%s lays a stone on %s.", seatLabel(seat), mv.To)
	case actPass:
		d.Passes++
		d.Last = ""
		e.say("%s has nowhere to lay a stone and passes.", seatLabel(seat))
		if d.Passes >= 2 {
			e.finish(0, "stuck")
			return
		}
	}
	// A player's own dead groups come off when their turn ends — but not
	// before they have had two turns, so that the opening stones stand.
	if d.Turns[seat-1] >= 2 {
		if dead := e.deadGroups(seat); len(dead) > 0 {
			n := 0
			for _, group := range dead {
				for _, i := range group {
					e.stones[i] = 0
					n++
				}
			}
			e.say("%d of %s's stones can see no friend and come off (%d group%s).", n, seatLabel(seat), len(dead), plural(len(dead)))
		}
	}
	// A player whose turn would begin with no stones has lost. (The rules put
	// it at the start of their turn; the game is over either way, and ending it
	// here spares everybody a move that cannot matter.)
	nextSeat := other(seat)
	if d.Turns[nextSeat-1] >= 1 && e.count(nextSeat) == 0 {
		e.finish(seat, "wiped-out")
		return
	}
	if e.count(seat) == 0 && d.Turns[seat-1] >= 2 {
		// The mover killed their own last group: they lose at the start of
		// their next turn, which is to say now.
		e.finish(nextSeat, "wiped-out")
		return
	}
	d.Turn = nextSeat
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
