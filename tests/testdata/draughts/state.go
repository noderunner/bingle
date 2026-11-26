package draughts

import (
	"errors"
	"fmt"

	"github.com/noderunner/bingle/internal/engine/wasmrules/guest"
)

// The rules played on an array of the dark squares (#322): what the search
// inside a module plays (guest.Searchable), and what Legal and Apply play
// too, so that there is one copy of the rules. A position is turned into a
// state when a call comes in, and back when it goes out.

// diagonals are the four ways, in the order of near and far.
var diagonals = [4]xy{{-1, -1}, {1, -1}, {-1, 1}, {1, 1}}

// maxSquares is the most dark squares a board may have.
const maxSquares = 64

// A square holds nothing, or a seat's man or king: seat<<1, plus 1 for a king.
func code(seat int, isKing bool) int8 {
	c := int8(seat << 1)
	if isKing {
		c++
	}
	return c
}

func seatOfCode(c int8) int { return int(c >> 1) }
func kingCode(c int8) bool  { return c&1 == 1 }

// index works out what the array needs of the board.
func (r *rules) index() {
	n := len(r.cells)
	r.idx = make(map[string]int, n)
	r.row = make([]int, n)
	r.near = make([][4]int, n)
	r.far = make([][4]int, n)
	for i, id := range r.cells {
		r.idx[id] = i
	}
	for i, id := range r.cells {
		c := r.where[id]
		r.row[i] = c.y
		for d, w := range diagonals {
			r.near[i][d], r.far[i][d] = -1, -1
			if to, ok := r.at[xy{c.x + w.x, c.y + w.y}]; ok {
				r.near[i][d] = r.idx[to]
			}
			if to, ok := r.at[xy{c.x + 2*w.x, c.y + 2*w.y}]; ok {
				r.far[i][d] = r.idx[to]
			}
		}
	}
	// The ways of dirs as diagonals: forward first, the left one first.
	way := func(dx, dy int) int {
		for d, w := range diagonals {
			if w.x == dx && w.y == dy {
				return d
			}
		}
		return -1
	}
	for seat := 1; seat <= 2; seat++ {
		f := r.forward[seat]
		r.ways[seat][0] = []int{way(-1, f), way(1, f)}
		r.ways[seat][1] = []int{way(-1, f), way(1, f), way(-1, -f), way(1, -f)}
	}
	r.men = r.rows * r.width / 2
}

// state is a position of draughts as an array of the dark squares.
type state struct {
	r      *rules
	sq     [maxSquares]int8
	ply    int
	turn   int
	chain  int // the square of a chain of jumps, -1 for none
	quiet  int
	winner int
	reason string
	// src is the position it was made from, whose counters and reserves a
	// position made of it carries over.
	src *guest.Position
	// legal is what the seat to move may do, once it has been worked out.
	legal  []guest.Selection
	listed bool
}

var _ guest.Searchable = (*rules)(nil)

// State turns a position into a state.
func (r *rules) State(pos *guest.Position) (guest.State, error) {
	d, err := dataOf(pos)
	if err != nil {
		return nil, err
	}
	return r.stateOf(pos, d)
}

func (r *rules) stateOf(pos *guest.Position, d *data) (*state, error) {
	if len(r.cells) > maxSquares {
		return nil, fmt.Errorf("draughts: %d squares do not fit an array of %d", len(r.cells), maxSquares)
	}
	st := &state{r: r, ply: pos.Ply, turn: d.Turn, chain: -1, quiet: d.Quiet, winner: d.Winner, reason: d.Reason, src: pos}
	for i, id := range r.cells {
		if p, held := top(pos, id); held {
			if seat := r.seatOf(p.Color); seat != 0 {
				st.sq[i] = code(seat, p.Piece == king)
			}
		}
	}
	if d.Chain != "" {
		i, ok := r.idx[d.Chain]
		if !ok {
			return nil, fmt.Errorf("draughts: no square %s", d.Chain)
		}
		st.chain = i
	}
	return st, nil
}

// position is the state as a position, with the counters and reserves of the
// position it came from.
func (st *state) position() (*guest.Position, error) {
	r := st.r
	pos := &guest.Position{Cells: make(map[string][]guest.Piece, 2*r.men), Ply: st.ply}
	for i, c := range st.sq[:len(r.cells)] {
		if c == 0 {
			continue
		}
		piece := man
		if kingCode(c) {
			piece = king
		}
		pos.Cells[r.cells[i]] = []guest.Piece{{Piece: piece, Color: r.colors[seatOfCode(c)]}}
	}
	if src := st.src.Clone(); src != nil {
		pos.Reserves, pos.Counters = src.Reserves, src.Counters
	}
	return pos, setData(pos, st.data())
}

func (st *state) data() *data {
	d := &data{Turn: st.turn, Quiet: st.quiet, Winner: st.winner, Reason: st.reason}
	if st.chain >= 0 {
		d.Chain = st.r.cells[st.chain]
	}
	return d
}

func (st *state) Turn() int   { return st.turn }
func (st *state) Winner() int { return st.winner }

// Value is the seat's pieces less the other seat's, as Heuristic.
func (st *state) Value(seat int) float64 {
	mine, theirs := 0, 0
	for _, c := range st.sq[:len(st.r.cells)] {
		switch seatOfCode(c) {
		case 0:
		case seat:
			mine++
		default:
			theirs++
		}
	}
	return st.r.worth(mine, theirs)
}

// jumps lists the jumps the piece on square i can make.
func (st *state) jumps(i int, out []guest.Selection) []guest.Selection {
	c := st.sq[i]
	seat := seatOfCode(c)
	for _, d := range st.r.ways[seat][b2i(kingCode(c))] {
		over, to := st.r.near[i][d], st.r.far[i][d]
		if over < 0 || to < 0 || seatOfCode(st.sq[over]) != other(seat) || st.sq[to] != 0 {
			continue
		}
		out = append(out, guest.Selection{Action: actJump, From: st.r.cells[i], To: st.r.cells[to]})
	}
	return out
}

// steps lists the moves without a capture the piece on square i can make.
func (st *state) steps(i int, out []guest.Selection) []guest.Selection {
	c := st.sq[i]
	for _, d := range st.r.ways[seatOfCode(c)][b2i(kingCode(c))] {
		if to := st.r.near[i][d]; to >= 0 && st.sq[to] == 0 {
			out = append(out, guest.Selection{Action: actMove, From: st.r.cells[i], To: st.r.cells[to]})
		}
	}
	return out
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// Legal is what the seat to move may do: the next jump of a chain; else every
// jump, since a capture must be made; else every move.
func (st *state) Legal() []guest.Selection {
	if st.listed {
		return st.legal
	}
	st.legal, st.listed = st.list(), true
	return st.legal
}

func (st *state) list() []guest.Selection {
	if st.turn == 0 {
		return nil
	}
	if st.chain >= 0 {
		return st.jumps(st.chain, nil)
	}
	var out []guest.Selection
	n := len(st.r.cells)
	for i, c := range st.sq[:n] {
		if seatOfCode(c) == st.turn {
			out = st.jumps(i, out)
		}
	}
	if len(out) > 0 {
		return out
	}
	for i, c := range st.sq[:n] {
		if seatOfCode(c) == st.turn {
			out = st.steps(i, out)
		}
	}
	return out
}

// Play makes one move or one jump that Legal listed.
func (st *state) Play(sel guest.Selection) (guest.State, error) {
	next, _, err := st.play(sel)
	if err != nil {
		return nil, err
	}
	return next, nil
}

// play is Play, which also says the square captured on (-1 for none). A jump
// that leaves its piece another jump keeps the seat; a man that reaches the
// far row is crowned, and its move ends there.
func (st *state) play(sel guest.Selection) (*state, int, error) {
	r := st.r
	from, okFrom := r.idx[sel.From]
	to, okTo := r.idx[sel.To]
	if !okFrom || !okTo || st.turn == 0 {
		return nil, -1, errors.New("draughts: not a move of this position")
	}
	seat := st.turn
	next := &state{r: r, sq: st.sq, ply: st.ply + 1, turn: seat, chain: -1, quiet: st.quiet + 1, src: st.src}
	c := next.sq[from]
	next.sq[from] = 0
	captured := -1
	if sel.Action == actJump {
		for d := range diagonals {
			if r.far[from][d] == to {
				captured = r.near[from][d]
			}
		}
		if captured < 0 {
			return nil, -1, errors.New("draughts: not a jump")
		}
		next.sq[captured] = 0
		next.quiet = 0
	}
	crowned := !kingCode(c) && r.row[to] == r.farRow(seat)
	if !kingCode(c) {
		next.quiet = 0
	}
	if crowned {
		c = code(seat, true)
	}
	next.sq[to] = c

	if sel.Action == actJump && !crowned && len(next.jumps(to, nil)) > 0 {
		next.chain = to
		return next, captured, nil
	}
	next.turn = other(seat)
	switch {
	case len(next.Legal()) == 0:
		next.turn, next.winner, next.reason = 0, seat, ReasonNoMoves
		next.legal = nil
	case next.quiet >= r.quiet:
		next.turn, next.reason = 0, ReasonQuiet
		next.legal = nil
	}
	return next, captured, nil
}

// Legal lists what seat may do.
func (r *rules) Legal(pos *guest.Position, seat int) []guest.Selection {
	d, err := dataOf(pos)
	if err != nil || seat == 0 || d.Turn != seat {
		return nil
	}
	st, err := r.stateOf(pos, d)
	if err != nil {
		return nil
	}
	return st.Legal()
}

// Apply makes one move or one jump, as play does, of a position.
func (r *rules) Apply(pos *guest.Position, seat int, sel guest.Selection) (*guest.Result, error) {
	d, err := dataOf(pos)
	if err != nil {
		return nil, err
	}
	if d.Turn == 0 {
		return nil, errors.New("draughts: the game is over")
	}
	if seat != d.Turn {
		return nil, fmt.Errorf("draughts: it is seat %d's move", d.Turn)
	}
	st, err := r.stateOf(pos, d)
	if err != nil {
		return nil, err
	}
	listed := false
	for _, l := range st.Legal() {
		if l.Action == sel.Action && l.From == sel.From && l.To == sel.To {
			listed = true
			break
		}
	}
	if !listed || !bare(sel) {
		return nil, fmt.Errorf("draughts: %s is not a legal move", r.Format(sel))
	}
	next, captured, err := st.play(sel)
	if err != nil {
		return nil, err
	}
	out, err := next.position()
	if err != nil {
		return nil, err
	}
	res := &guest.Result{Position: out, NextSeat: next.turn, Outcome: outcomeOf(next.data())}
	if captured >= 0 {
		res.Captures = []string{r.cells[captured]}
	}
	return res, nil
}
