// Package draughts is English draughts, a game only the tests play: the
// public fixture for rules that run as a WebAssembly module (ADR 0147). It is
// written against the guest interface and imports nothing of the engine, so
// the same package is linked natively (wasmrules.Native) and built into a
// module (the wasm folder), and the two are played against each other.
//
// It is here for what a pack's game needs of the host: captures that must be
// made, and a chain of jumps that is several selections in one impulse, each
// jump a selection of its own. Its variants are a second board and a start
// drawn by lot from the game's seed.
package draughts

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/noderunner/bingle/internal/engine/wasmrules/guest"
)

// The game's ids, as draughts.yaml names them.
const (
	ID       = "test-draughts"
	SmallID  = "test-draughts-6"
	BallotID = "test-draughts-ballot"
)

const (
	man  = "man"
	king = "king"

	actMove = "move"
	actJump = "jump"
)

// The ways a game ends.
const (
	ReasonNoMoves = "no-moves"
	ReasonQuiet   = "quiet"
)

// data is the game's part of a position.
type data struct {
	// Turn is the seat that moves, 0 once the game is over.
	Turn int `json:"turn"`
	// Chain is the cell of the piece that has jumped and must jump again.
	Chain string `json:"chain,omitempty"`
	// Quiet counts the selections since a capture or a man's move.
	Quiet int `json:"quiet,omitempty"`
	// Winner and Reason say how a finished game ended (no Winner: a draw).
	Winner int    `json:"winner,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// definition is what the rules read of the game's definition.
type definition struct {
	Seats []struct {
		Seat  int    `json:"seat"`
		Color string `json:"color"`
	} `json:"seats"`
	Params struct {
		// Rows of men each side starts with, the quiet selections that make
		// a draw, and whether a man of each side is drawn off by lot.
		Rows   int  `json:"rows"`
		Quiet  int  `json:"quiet"`
		Ballot bool `json:"ballot"`
	} `json:"params"`
}

type xy struct{ x, y int }

// rules are the rules of one game: its board's squares, and its seats.
type rules struct {
	width, height int
	// cells are the dark squares in the board's order; at and where turn a
	// coordinate into a square and back.
	cells []string
	at    map[xy]string
	where map[string]xy
	// colors is each seat's colour, by seat (1 and 2); forward is the way its
	// men move (seat 1 up the board, towards y = 0).
	colors  [3]string
	forward [3]int
	rows    int
	quiet   int
	ballot  bool
}

// New makes the rules of a game of draughts from its definition and board.
func New(g *guest.Game) (guest.Rules, error) {
	var def definition
	if err := json.Unmarshal(g.Definition, &def); err != nil {
		return nil, fmt.Errorf("draughts: the definition: %w", err)
	}
	if len(def.Seats) != 2 {
		return nil, fmt.Errorf("draughts: a game for two seats, not %d", len(def.Seats))
	}
	b := g.Board
	r := &rules{
		width: b.Width, height: b.Height,
		at: map[xy]string{}, where: map[string]xy{},
		forward: [3]int{0, -1, 1},
		rows:    def.Params.Rows, quiet: def.Params.Quiet, ballot: def.Params.Ballot,
	}
	for _, s := range def.Seats {
		if s.Seat != 1 && s.Seat != 2 {
			return nil, fmt.Errorf("draughts: seats 1 and 2, not %d", s.Seat)
		}
		r.colors[s.Seat] = s.Color
	}
	if b.Kind != "grid" || b.Width < 4 || b.Height < 4 || b.Height%2 != 0 {
		return nil, fmt.Errorf("draughts: a grid at least 4 wide and an even number of rows high, not a %s of %d by %d", b.Kind, b.Width, b.Height)
	}
	if r.rows < 1 || 2*r.rows >= b.Height {
		return nil, fmt.Errorf("draughts: %d rows of men do not fit %d rows", r.rows, b.Height)
	}
	if r.quiet < 1 {
		return nil, errors.New("draughts: params.quiet must say when a quiet game is drawn")
	}
	for _, p := range b.Positions {
		if p.XY == nil {
			return nil, fmt.Errorf("draughts: square %s has no coordinate", p.ID)
		}
		c := xy{p.XY[0], p.XY[1]}
		// The dark squares: the bottom left corner is one.
		if (c.x+(b.Height-1-c.y))%2 != 0 {
			continue
		}
		r.cells = append(r.cells, p.ID)
		r.at[c] = p.ID
		r.where[p.ID] = c
	}
	return r, nil
}

func (r *rules) seatOf(color string) int {
	switch color {
	case r.colors[1]:
		return 1
	case r.colors[2]:
		return 2
	}
	return 0
}

func other(seat int) int { return 3 - seat }

// farRow is the row a seat's men are crowned on.
func (r *rules) farRow(seat int) int {
	if r.forward[seat] < 0 {
		return 0
	}
	return r.height - 1
}

// Start sets each side's men on its nearest rows, Black (seat 1) at the
// bottom, to move first. With the ballot, a man of each side, drawn by lot
// from the seed, starts off the board.
func (r *rules) Start(seed uint64) (*guest.Position, error) {
	pos := guest.NewPosition()
	var men [3][]string
	for _, id := range r.cells {
		switch c := r.where[id]; {
		case c.y >= r.height-r.rows:
			men[1] = append(men[1], id)
		case c.y < r.rows:
			men[2] = append(men[2], id)
		}
	}
	if r.ballot {
		rng := seed
		for seat := 1; seat <= 2; seat++ {
			out := int(splitmix(&rng) % uint64(len(men[seat])))
			men[seat] = append(men[seat][:out:out], men[seat][out+1:]...)
		}
	}
	for seat := 1; seat <= 2; seat++ {
		for _, id := range men[seat] {
			pos.Cells[id] = []guest.Piece{{Piece: man, Color: r.colors[seat]}}
		}
	}
	return pos, setData(pos, &data{Turn: 1})
}

// splitmix is SplitMix64: the next number of the sequence that starts at
// *state.
func splitmix(state *uint64) uint64 {
	*state += 0x9e3779b97f4a7c15
	z := *state
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	return z ^ (z >> 31)
}

func dataOf(pos *guest.Position) (*data, error) {
	var d data
	if len(pos.Ext) == 0 {
		return nil, errors.New("draughts: the position has none of the game's data")
	}
	if err := json.Unmarshal(pos.Ext, &d); err != nil {
		return nil, fmt.Errorf("draughts: the game's data: %w", err)
	}
	return &d, nil
}

func setData(pos *guest.Position, d *data) error {
	raw, err := json.Marshal(d)
	if err != nil {
		return err
	}
	pos.Ext = raw
	return nil
}

// ImpulseSeat is the seat that moves.
func (r *rules) ImpulseSeat(pos *guest.Position) int {
	d, err := dataOf(pos)
	if err != nil {
		return 0
	}
	return d.Turn
}

// top is the piece on a square, and whether there is one.
func top(pos *guest.Position, id string) (guest.Piece, bool) {
	stack := pos.Cells[id]
	if len(stack) == 0 {
		return guest.Piece{}, false
	}
	return stack[len(stack)-1], true
}

// dirs are the ways a piece moves: a man forward, a king both ways.
func (r *rules) dirs(seat int, piece string) []xy {
	f := r.forward[seat]
	if piece == king {
		return []xy{{-1, f}, {1, f}, {-1, -f}, {1, -f}}
	}
	return []xy{{-1, f}, {1, f}}
}

// jumps lists the jumps the piece on from can make.
func (r *rules) jumps(pos *guest.Position, seat int, from string, out []guest.Selection) []guest.Selection {
	p, _ := top(pos, from)
	c := r.where[from]
	for _, d := range r.dirs(seat, p.Piece) {
		over, ok := r.at[xy{c.x + d.x, c.y + d.y}]
		if !ok {
			continue
		}
		if q, held := top(pos, over); !held || r.seatOf(q.Color) != other(seat) {
			continue
		}
		to, ok := r.at[xy{c.x + 2*d.x, c.y + 2*d.y}]
		if !ok {
			continue
		}
		if _, held := top(pos, to); held {
			continue
		}
		out = append(out, guest.Selection{Action: actJump, From: from, To: to})
	}
	return out
}

// steps lists the moves without a capture the piece on from can make.
func (r *rules) steps(pos *guest.Position, seat int, from string, out []guest.Selection) []guest.Selection {
	p, _ := top(pos, from)
	c := r.where[from]
	for _, d := range r.dirs(seat, p.Piece) {
		to, ok := r.at[xy{c.x + d.x, c.y + d.y}]
		if !ok {
			continue
		}
		if _, held := top(pos, to); held {
			continue
		}
		out = append(out, guest.Selection{Action: actMove, From: from, To: to})
	}
	return out
}

// Legal lists what seat may do: the next jump of a chain; else every jump,
// since a capture must be made; else every move.
func (r *rules) Legal(pos *guest.Position, seat int) []guest.Selection {
	d, err := dataOf(pos)
	if err != nil || seat == 0 || d.Turn != seat {
		return nil
	}
	return r.legal(pos, d, seat)
}

func (r *rules) legal(pos *guest.Position, d *data, seat int) []guest.Selection {
	if d.Chain != "" {
		return r.jumps(pos, seat, d.Chain, nil)
	}
	var out []guest.Selection
	for _, id := range r.cells {
		if p, held := top(pos, id); held && r.seatOf(p.Color) == seat {
			out = r.jumps(pos, seat, id, out)
		}
	}
	if len(out) > 0 {
		return out
	}
	for _, id := range r.cells {
		if p, held := top(pos, id); held && r.seatOf(p.Color) == seat {
			out = r.steps(pos, seat, id, out)
		}
	}
	return out
}

// Apply makes one move or one jump. A jump that leaves its piece another
// jump keeps the seat, so a chain is one impulse of several selections; a man
// that reaches the far row is crowned, and its move ends there.
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
	listed := false
	for _, l := range r.legal(pos, d, seat) {
		if l.Action == sel.Action && l.From == sel.From && l.To == sel.To {
			listed = true
			break
		}
	}
	if !listed || !bare(sel) {
		return nil, fmt.Errorf("draughts: %s is not a legal move", r.Format(sel))
	}

	next := pos.Clone()
	next.Ply++
	piece, _ := top(next, sel.From)
	delete(next.Cells, sel.From)
	res := &guest.Result{Position: next}
	nd := &data{Turn: seat, Quiet: d.Quiet + 1}
	if sel.Action == actJump {
		a, b := r.where[sel.From], r.where[sel.To]
		over := r.at[xy{(a.x + b.x) / 2, (a.y + b.y) / 2}]
		delete(next.Cells, over)
		res.Captures = []string{over}
		nd.Quiet = 0
	}
	crowned := piece.Piece == man && r.where[sel.To].y == r.farRow(seat)
	if piece.Piece == man {
		nd.Quiet = 0
	}
	if crowned {
		piece.Piece = king
	}
	next.Cells[sel.To] = []guest.Piece{piece}

	if sel.Action == actJump && !crowned && len(r.jumps(next, seat, sel.To, nil)) > 0 {
		nd.Chain = sel.To
	} else {
		nd.Turn = other(seat)
		switch {
		case len(r.legal(next, nd, nd.Turn)) == 0:
			nd.Turn, nd.Winner, nd.Reason = 0, seat, ReasonNoMoves
		case nd.Quiet >= r.quiet:
			nd.Turn, nd.Reason = 0, ReasonQuiet
		}
	}
	if err := setData(next, nd); err != nil {
		return nil, err
	}
	res.NextSeat = nd.Turn
	res.Outcome = outcomeOf(nd)
	return res, nil
}

// bare reports whether sel says nothing but its action and its two squares.
func bare(sel guest.Selection) bool {
	return sel.Piece == "" && sel.Count == 0 && len(sel.Path) == 0 && len(sel.Drops) == 0 && sel.Aim == "" && sel.Card == ""
}

func outcomeOf(d *data) guest.Outcome {
	if d.Turn != 0 {
		return guest.Outcome{}
	}
	return guest.Outcome{Ended: true, Winner: d.Winner, Reason: d.Reason}
}

// Outcome is how the position stands.
func (r *rules) Outcome(pos *guest.Position) guest.Outcome {
	d, err := dataOf(pos)
	if err != nil {
		return guest.Outcome{}
	}
	return outcomeOf(d)
}

// Format writes a move as "c3-d4" and a jump as "c3xe5".
func (r *rules) Format(sel guest.Selection) string {
	if sel.Action == actJump {
		return sel.From + "x" + sel.To
	}
	return sel.From + "-" + sel.To
}

// Parse reads what Format writes.
func (r *rules) Parse(text string) (guest.Selection, error) {
	text = strings.TrimSpace(text)
	for _, kind := range [][2]string{{"-", actMove}, {"x", actJump}} {
		action := kind[1]
		from, to, found := strings.Cut(text, kind[0])
		if !found {
			continue
		}
		if _, ok := r.where[from]; !ok {
			return guest.Selection{}, fmt.Errorf("draughts: %q is not a square men stand on", from)
		}
		if _, ok := r.where[to]; !ok {
			return guest.Selection{}, fmt.Errorf("draughts: %q is not a square men stand on", to)
		}
		return guest.Selection{Action: action, From: from, To: to}, nil
	}
	return guest.Selection{}, fmt.Errorf("draughts: %q is neither a move (c3-d4) nor a jump (c3xe5)", text)
}

// Gestures: the piece, then where it goes.
func (r *rules) Gestures(sel guest.Selection) []guest.Gesture {
	return []guest.Gesture{{Kind: guest.GesturePos, Value: sel.From}, {Kind: guest.GesturePos, Value: sel.To}}
}

// Validate checks a position that did not come from play: every piece is a
// man or a king of a seat's colour, alone on a dark square, with no man on
// the row it is crowned on, and the game's data fit it.
func (r *rules) Validate(pos *guest.Position) error {
	d, err := dataOf(pos)
	if err != nil {
		return err
	}
	for _, id := range slices.Sorted(maps.Keys(pos.Cells)) {
		stack := pos.Cells[id]
		if len(stack) == 0 {
			continue
		}
		c, ok := r.where[id]
		if !ok {
			return fmt.Errorf("draughts: %s is not a square men stand on", id)
		}
		if len(stack) > 1 {
			return fmt.Errorf("draughts: %s holds %d pieces", id, len(stack))
		}
		p := stack[0]
		seat := r.seatOf(p.Color)
		if seat == 0 {
			return fmt.Errorf("draughts: nobody plays %s", p.Color)
		}
		switch p.Piece {
		case king:
		case man:
			if c.y == r.farRow(seat) {
				return fmt.Errorf("draughts: the man on %s should have been crowned", id)
			}
		default:
			return fmt.Errorf("draughts: there is no piece %q", p.Piece)
		}
	}
	switch d.Turn {
	case 0:
		if d.Reason != ReasonNoMoves && d.Reason != ReasonQuiet {
			return errors.New("draughts: a finished game says how it ended")
		}
		return nil
	case 1, 2:
	default:
		return fmt.Errorf("draughts: there is no seat %d", d.Turn)
	}
	if d.Winner != 0 || d.Reason != "" || d.Quiet < 0 || d.Quiet >= r.quiet {
		return errors.New("draughts: the game's data are those of a finished game")
	}
	if d.Chain != "" {
		if p, held := top(pos, d.Chain); !held || r.seatOf(p.Color) != d.Turn {
			return fmt.Errorf("draughts: the piece in a chain of jumps is not on %s", d.Chain)
		}
	}
	if len(r.legal(pos, d, d.Turn)) == 0 {
		return fmt.Errorf("draughts: seat %d has no move, so the game is over", d.Turn)
	}
	return nil
}
