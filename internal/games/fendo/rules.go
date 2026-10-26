package fendo

import (
	"fmt"
	"sort"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

// Ruleset is Fendo's state.Ruleset. A turn is one decision: move a piece and
// build a fence beside it, add a piece, or (when nothing else is possible)
// pass.
type Ruleset struct{}

// New returns the ruleset.
func New() *Ruleset { return &Ruleset{} }

const (
	actMove = "move" // From, To (equal when the piece stays) and Piece = the fence side
	actAdd  = "add"  // To
	actPass = "pass"
)

// eng is a decoded position.
type eng struct {
	g      *geo
	d      *Data
	occ    []int8    // occ[i] is the seat whose piece stands on cell i (0 = empty)
	wall   [][4]bool // wall[i][d]: a fence on side d of cell i
	colors [3]string
	game   *catalog.CompiledGame
	ev     []string
}

func newEng(game *catalog.CompiledGame, st *state.BoardState, d *Data) *eng {
	g := geoOf(game)
	e := &eng{g: g, d: d, occ: make([]int8, len(g.ids)), wall: make([][4]bool, len(g.ids)), game: game}
	for _, s := range game.Definition.Seats {
		if s.Seat >= 1 && s.Seat <= 2 {
			e.colors[s.Seat] = s.Color
		}
	}
	for _, id := range st.Occupied() {
		if i, ok := g.index[id]; ok {
			o, _ := st.Get(id)
			for s := 1; s <= 2; s++ {
				if e.colors[s] == o.Color {
					e.occ[i] = int8(s)
				}
			}
		}
	}
	for _, w := range d.Walls {
		a, okA := g.index[boardID(w[0])]
		b, okB := g.index[boardID(w[1])]
		if okA && okB {
			e.setWall(a, b, true)
		}
	}
	return e
}

func (e *eng) save(st *state.BoardState) {
	for _, id := range st.Occupied() {
		st.Remove(id)
	}
	for i, s := range e.occ {
		if s != 0 {
			st.Set(e.g.ids[i], state.Occupant{Piece: "piece", Color: e.colors[s]})
		}
	}
}

// sideTo is the side of cell a that faces its neighbour b, or -1.
func (e *eng) sideTo(a, b int) int {
	for d, n := range e.g.nbr[a] {
		if n == b {
			return d
		}
	}
	return -1
}

func (e *eng) setWall(a, b int, on bool) {
	if d := e.sideTo(a, b); d >= 0 {
		e.wall[a][d] = on
		e.wall[b][opposite(d)] = on
	}
}

// step is the cell beside i on side d, or -1 at the edge of the board or
// across a fence.
func (e *eng) step(i, d int) int {
	if i < 0 || e.wall[i][d] {
		return -1
	}
	return e.g.nbr[i][d]
}

// reach lists the empty cells the piece on from can move to: straight along
// the rows and columns, with one right-angle turn, never across a fence or a
// piece.
func (e *eng) reach(from int) []int {
	var out []int
	seen := map[int]bool{from: true}
	add := func(c int) {
		if !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	for d1 := 0; d1 < 4; d1++ {
		for c := e.step(from, d1); c >= 0 && e.occ[c] == 0; c = e.step(c, d1) {
			add(c)
			for _, d2 := range []int{(d1 + 1) % 4, (d1 + 3) % 4} {
				for c2 := e.step(c, d2); c2 >= 0 && e.occ[c2] == 0; c2 = e.step(c2, d2) {
					add(c2)
				}
			}
		}
	}
	sort.Ints(out)
	return out
}

// area is a region the fences have cut off (or the open one).
type area struct {
	cells  []int
	pieces [3]int
}

func (a area) count() int { return a.pieces[1] + a.pieces[2] }

func (e *eng) areas() []area {
	seen := make([]bool, len(e.occ))
	var out []area
	for i := range e.occ {
		if seen[i] {
			continue
		}
		a := area{cells: []int{i}}
		seen[i] = true
		for k := 0; k < len(a.cells); k++ {
			c := a.cells[k]
			if e.occ[c] != 0 {
				a.pieces[e.occ[c]]++
			}
			for d := 0; d < 4; d++ {
				if n := e.step(c, d); n >= 0 && !seen[n] {
					seen[n] = true
					a.cells = append(a.cells, n)
				}
			}
		}
		out = append(out, a)
	}
	return out
}

// openArea is the region with more than one piece, or nil when none is left.
func (e *eng) openArea() []int {
	for _, a := range e.areas() {
		if a.count() > 1 {
			return a.cells
		}
	}
	return nil
}

// fenceLegal reports whether a fence on side d of cell i is allowed: the side
// is free and inside the board, no area it cuts off is empty, and at most one
// area is left holding more than one piece.
func (e *eng) fenceLegal(i, d int) bool {
	j := e.g.nbr[i][d]
	if j < 0 || e.wall[i][d] {
		return false
	}
	e.setWall(i, j, true)
	defer e.setWall(i, j, false)
	open := 0
	for _, a := range e.areas() {
		switch n := a.count(); {
		case n == 0:
			return false
		case n > 1:
			open++
		}
	}
	return open <= 1
}

// legal lists the moves of the seat to move.
func (e *eng) legal() []state.Move {
	d := e.d
	if d.Turn == 0 {
		return nil
	}
	seat := int8(d.Turn)
	inOpen := map[int]bool{}
	for _, c := range e.openArea() {
		inOpen[c] = true
	}
	var out []state.Move
	for i := range e.occ {
		if e.occ[i] != seat || !inOpen[i] {
			continue
		}
		for _, to := range append([]int{i}, e.reach(i)...) {
			e.occ[i], e.occ[to] = 0, seat
			for side := 0; side < 4; side++ {
				if e.fenceLegal(to, side) {
					out = append(out, state.Move{Action: actMove, From: e.g.ids[i], To: e.g.ids[to], Piece: sideLetter[side]})
				}
			}
			e.occ[to], e.occ[i] = 0, seat
		}
	}
	if d.Stock[seat-1] > 0 {
		seen := map[int]bool{}
		for i := range e.occ {
			if e.occ[i] != seat || !inOpen[i] {
				continue
			}
			for _, to := range e.reach(i) {
				if !seen[to] {
					seen[to] = true
					out = append(out, state.Move{Action: actAdd, To: e.g.ids[to]})
				}
			}
		}
	}
	if len(out) == 0 {
		out = append(out, state.Move{Action: actPass})
	}
	return out
}

// territory counts the squares of the areas closed round one piece.
func (e *eng) territory() [3]int {
	var t [3]int
	for _, a := range e.areas() {
		if a.count() == 1 {
			for s := 1; s <= 2; s++ {
				if a.pieces[s] == 1 {
					t[s] += len(a.cells)
				}
			}
		}
	}
	return t
}

func other(seat int) int { return 3 - seat }

func seatLabel(seat int) string {
	if seat == 1 {
		return "White"
	}
	return "Orange"
}

func (e *eng) say(format string, args ...any) { e.ev = append(e.ev, fmt.Sprintf(format, args...)) }

func (e *eng) majority() int { return e.game.ParamInt("majority", 0) }

// execute plays a legal move and settles whether the game is over.
func (e *eng) execute(mv state.Move, seat int) {
	d := e.d
	switch mv.Action {
	case actMove:
		from, to := e.g.index[mv.From], e.g.index[mv.To]
		side, _ := sideOf(mv.Piece)
		e.occ[from], e.occ[to] = 0, int8(seat)
		j := e.g.nbr[to][side]
		e.setWall(to, j, true)
		a, b := string(e.g.ids[to]), string(e.g.ids[j])
		if b < a {
			a, b = b, a
		}
		d.Walls = append(d.Walls, [2]string{a, b})
		d.Passes = 0
		if from == to {
			e.say("%s builds a fence on the %s side of %s.", seatLabel(seat), sideName[side], mv.To)
		} else {
			e.say("%s moves a piece from %s to %s and builds a fence on its %s side.", seatLabel(seat), mv.From, mv.To, sideName[side])
		}
	case actAdd:
		e.occ[e.g.index[mv.To]] = int8(seat)
		d.Stock[seat-1]--
		d.Passes = 0
		e.say("%s adds a piece on %s.", seatLabel(seat), mv.To)
	case actPass:
		d.Passes++
		e.say("%s has no action and passes.", seatLabel(seat))
	}
	t := e.territory()
	d.Score = [2]int{t[1], t[2]}
	switch {
	case e.majority() > 0 && t[seat] >= e.majority():
		e.finish(seat, "majority")
		e.say("%s owns %d squares, a majority, and wins!", seatLabel(seat), t[seat])
	case e.openArea() == nil || d.Passes >= 2:
		w := 0
		switch {
		case t[1] > t[2]:
			w = 1
		case t[2] > t[1]:
			w = 2
		}
		e.finish(w, "territory")
		if w == 0 {
			e.say("The areas are closed and both players own %d squares: a draw.", t[1])
		} else {
			e.say("The areas are closed: %s wins, %d squares to %d.", seatLabel(w), t[w], t[other(w)])
		}
	default:
		d.Turn = other(seat)
	}
}

func (e *eng) finish(winner int, reason string) {
	e.d.Turn, e.d.Winner, e.d.Reason = 0, winner, reason
}

// --- ruleset interface ----------------------------------------------------

// Start places each player's first piece on the middle of their side.
func (rs *Ruleset) Start(game *catalog.CompiledGame, seed uint64) (*state.BoardState, error) {
	g := geoOf(game)
	st := state.NewBoardState()
	mid := string(rune('a' + (g.w-1)/2))
	for s, rank := range map[int]int{1: 1, 2: g.h} {
		for _, def := range game.Definition.Seats {
			if def.Seat == s {
				st.Set(boardID(fmt.Sprintf("%s%d", mid, rank)), state.Occupant{Piece: "piece", Color: def.Color})
			}
		}
	}
	n := game.ParamInt("pieces", 7) - 1
	st.SetExt(&Data{Turn: 1, Stock: [2]int{n, n}})
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
	out := state.Outcome{Ended: true, Reason: d.Reason, Detail: fmt.Sprintf("%d–%d squares", d.Score[0], d.Score[1])}
	if d.Winner != 0 {
		w := d.Winner
		out.Winner = &w
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
		return fmt.Errorf("fendo: bad turn %d", d.Turn)
	}
	g := geoOf(game)
	for _, w := range d.Walls {
		a, okA := g.index[boardID(w[0])]
		b, okB := g.index[boardID(w[1])]
		if !okA || !okB || newEng(game, st, &Data{}).sideTo(a, b) < 0 {
			return fmt.Errorf("fendo: bad fence %v", w)
		}
	}
	for _, id := range st.Occupied() {
		if _, ok := g.index[id]; !ok {
			return fmt.Errorf("fendo: piece off the board at %s", id)
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
	return newEng(game, st, d).legal()
}

// Apply plays one decision.
func (rs *Ruleset) Apply(game *catalog.CompiledGame, st *state.BoardState, seat int, mv state.Move) (*state.MoveResult, error) {
	d, err := dataOf(st)
	if err != nil {
		return nil, err
	}
	if d.Turn == 0 {
		return nil, fmt.Errorf("fendo: the game is over")
	}
	if seat != d.Turn {
		return nil, fmt.Errorf("fendo: it is %s's turn", seatLabel(d.Turn))
	}
	e := newEng(game, st, d)
	want := rs.Format(game, mv)
	var legal state.Move
	found := false
	for _, cand := range e.legal() {
		if rs.Format(game, cand) == want {
			legal, found = cand, true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("fendo: %q is not a legal move now", want)
	}
	nd := d.clone()
	e.d = nd
	e.execute(legal, seat)
	nd.Events = e.ev
	ns := st.Clone()
	ns.SetExt(nd)
	e.save(ns)
	ns.SetPly(st.Ply() + 1)
	if nd.Turn != 0 && ns.Ply() >= game.ParamInt("max_plies", 400) {
		t := e.territory()
		w := 0
		switch {
		case t[1] > t[2]:
			w = 1
		case t[2] > t[1]:
			w = 2
		}
		e.finish(w, "ply-limit")
		nd.Score = [2]int{t[1], t[2]}
	}
	res := &state.MoveResult{NewState: ns, Move: legal, Notation: rs.Format(game, legal), NextTurnSeat: nd.Turn}
	if nd.Turn == 0 {
		res.GameEnded = true
		res.NextTurnSeat = seat
		res.EndReason = nd.Reason
		res.EndDetail = fmt.Sprintf("%d–%d squares", nd.Score[0], nd.Score[1])
		if nd.Winner != 0 {
			w := nd.Winner
			res.WinnerSeat = &w
		}
	}
	return res, nil
}
