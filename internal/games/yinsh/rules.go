package yinsh

import (
	"fmt"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

// Ruleset is Yinsh's state.Ruleset. Every decision is one click-built move:
// a ring placed in the setup, a marker dropped in a ring and the ring moved,
// a row chosen to be removed, a ring chosen to go with it, or a pass.
type Ruleset struct{}

// New returns the ruleset.
func New() *Ruleset { return &Ruleset{} }

const (
	actPlace  = "place"  // To: a ring goes on an empty point (the setup)
	actMove   = "move"   // From, To: a marker drops in the ring on From, which moves to To
	actRow    = "row"    // From, To: the two ends of the five markers to remove
	actRemove = "remove" // From: the ring to take off the board
	actPass   = "pass"
)

// What stands on a point.
const (
	empty  = 0
	ring   = 1
	marker = 2
)

// eng is a decoded position: kind[i] and owner[i] say what is on point i and
// whose colour it is.
type eng struct {
	g      *geo
	d      *Data
	kind   []uint8
	owner  []uint8
	colors [3]string
	game   *catalog.CompiledGame
	ev     []string
}

func newEng(game *catalog.CompiledGame, st *state.BoardState, d *Data) *eng {
	g := geoOf(game)
	e := &eng{g: g, d: d, kind: make([]uint8, len(g.ids)), owner: make([]uint8, len(g.ids)), game: game}
	for _, s := range game.Definition.Seats {
		if s.Seat >= 1 && s.Seat <= 2 {
			e.colors[s.Seat] = s.Color
		}
	}
	for _, id := range st.Occupied() {
		i, ok := g.index[id]
		if !ok {
			continue
		}
		o, _ := st.Get(id)
		if o.Piece == "ring" {
			e.kind[i] = ring
		} else {
			e.kind[i] = marker
		}
		for s := 1; s <= 2; s++ {
			if e.colors[s] == o.Color {
				e.owner[i] = uint8(s)
			}
		}
	}
	return e
}

func (e *eng) save(st *state.BoardState) {
	for _, id := range st.Occupied() {
		st.Remove(id)
	}
	for i, k := range e.kind {
		switch k {
		case ring:
			st.Set(e.g.ids[i], state.Occupant{Piece: "ring", Color: e.colors[e.owner[i]]})
		case marker:
			st.Set(e.g.ids[i], state.Occupant{Piece: "marker", Color: e.colors[e.owner[i]]})
		}
	}
}

func other(seat int) int { return 3 - seat }

func seatLabel(seat int) string {
	if seat == 1 {
		return "White"
	}
	return "Black"
}

func (e *eng) count(kind uint8, seat int) int {
	n := 0
	for i, k := range e.kind {
		if k == kind && int(e.owner[i]) == seat {
			n++
		}
	}
	return n
}

func (e *eng) markersOnBoard() int {
	n := 0
	for _, k := range e.kind {
		if k == marker {
			n++
		}
	}
	return n
}

func (e *eng) target() int    { return e.game.ParamInt("rings_to_win", 3) }
func (e *eng) supply() int    { return e.game.ParamInt("markers", 51) }
func (e *eng) ringsEach() int { return e.game.ParamInt("rings", 5) }

// ringDests lists where the ring on from can move: along a line over empty
// points, or over a group of markers (turning each over) to the first empty
// point beyond it. A ring cannot pass over or land on another ring.
func (e *eng) ringDests(from int) []int {
	var out []int
	for d := 0; d < 6; d++ {
		jumped := false
		for c := e.g.nbr[from][d]; c >= 0; c = e.g.nbr[c][d] {
			switch e.kind[c] {
			case ring:
				c = -2 // blocked
			case marker:
				jumped = true
				continue
			default:
				out = append(out, c)
				if jumped {
					c = -2 // must stop on the first empty point after the markers
				}
			}
			if c == -2 {
				break
			}
		}
	}
	return out
}

// dirBetween is the direction from a to b if they lie on one line.
func (e *eng) dirBetween(a, b int) int {
	for d := 0; d < 6; d++ {
		for c := e.g.nbr[a][d]; c >= 0; c = e.g.nbr[c][d] {
			if c == b {
				return d
			}
		}
	}
	return -1
}

// rows lists every five markers of seat's colour in a row, by their two ends.
func (e *eng) rows(seat int) [][2]int {
	var out [][2]int
	isMine := func(i int) bool { return i >= 0 && e.kind[i] == marker && int(e.owner[i]) == seat }
	for i := range e.kind {
		if !isMine(i) {
			continue
		}
		for _, d := range []int{0, 2, 4} {
			if isMine(e.g.nbr[i][d+1]) {
				continue // not the start of the run
			}
			run := []int{i}
			for c := e.g.nbr[i][d]; isMine(c); c = e.g.nbr[c][d] {
				run = append(run, c)
			}
			for k := 0; k+5 <= len(run); k++ {
				out = append(out, [2]int{run[k], run[k+4]})
			}
		}
	}
	return out
}

// legal lists the decisions of the seat that decides.
func (e *eng) legal() []state.Move {
	d := e.d
	if d.Turn == 0 {
		return nil
	}
	seat := uint8(d.Turn)
	var out []state.Move
	switch d.Stage {
	case stageSetup:
		for i := range e.kind {
			if e.kind[i] == empty {
				out = append(out, state.Move{Action: actPlace, To: e.g.ids[i]})
			}
		}
	case stageRow:
		for _, r := range e.rows(d.Turn) {
			out = append(out, state.Move{Action: actRow, From: e.g.ids[r[0]], To: e.g.ids[r[1]]})
		}
	case stageRing:
		for i := range e.kind {
			if e.kind[i] == ring && e.owner[i] == seat {
				out = append(out, state.Move{Action: actRemove, From: e.g.ids[i]})
			}
		}
	default:
		for i := range e.kind {
			if e.kind[i] == ring && e.owner[i] == seat {
				for _, to := range e.ringDests(i) {
					out = append(out, state.Move{Action: actMove, From: e.g.ids[i], To: e.g.ids[to]})
				}
			}
		}
		if len(out) == 0 {
			out = append(out, state.Move{Action: actPass})
		}
	}
	return out
}

func (e *eng) say(format string, args ...any) { e.ev = append(e.ev, fmt.Sprintf(format, args...)) }

func (e *eng) finish(winner int, reason string) {
	e.d.Turn, e.d.Winner, e.d.Reason = 0, winner, reason
}

// execute plays a legal decision.
func (e *eng) execute(mv state.Move, seat int) {
	d := e.d
	switch mv.Action {
	case actPlace:
		i := e.g.index[mv.To]
		e.kind[i], e.owner[i] = ring, uint8(seat)
		e.say("%s places a ring on %s.", seatLabel(seat), mv.To)
		if e.count(ring, 1) >= e.ringsEach() && e.count(ring, 2) >= e.ringsEach() {
			d.Stage, d.Mover, d.Turn = stagePlay, 1, 1
			return
		}
		d.Turn = other(seat)
	case actMove:
		from, to := e.g.index[mv.From], e.g.index[mv.To]
		dir := e.dirBetween(from, to)
		d.Flipped = nil
		for c := e.g.nbr[from][dir]; c != to; c = e.g.nbr[c][dir] {
			if e.kind[c] == marker {
				e.owner[c] = uint8(other(int(e.owner[c])))
				d.Flipped = append(d.Flipped, string(e.g.ids[c]))
			}
		}
		e.kind[from], e.owner[from] = marker, uint8(seat)
		e.kind[to], e.owner[to] = ring, uint8(seat)
		d.Last = []string{string(mv.From), string(mv.To)}
		if len(d.Flipped) > 0 {
			e.say("%s drops a marker on %s and moves the ring to %s, turning over %d marker(s).", seatLabel(seat), mv.From, mv.To, len(d.Flipped))
		} else {
			e.say("%s drops a marker on %s and moves the ring to %s.", seatLabel(seat), mv.From, mv.To)
		}
		e.settle()
	case actRow:
		from, to := e.g.index[mv.From], e.g.index[mv.To]
		dir := e.dirBetween(from, to)
		for c := from; ; c = e.g.nbr[c][dir] {
			e.kind[c], e.owner[c] = empty, 0
			if c == to {
				break
			}
		}
		d.Stage = stageRing
		e.say("%s removes the row %s–%s.", seatLabel(seat), mv.From, mv.To)
	case actRemove:
		i := e.g.index[mv.From]
		e.kind[i], e.owner[i] = empty, 0
		d.Score[seat-1]++
		d.Stage = stagePlay
		e.say("%s removes the ring on %s (%d of %d).", seatLabel(seat), mv.From, d.Score[seat-1], e.target())
		if d.Score[seat-1] >= e.target() {
			e.finish(seat, "rings")
			e.say("%s has removed %d rings and wins!", seatLabel(seat), d.Score[seat-1])
			return
		}
		e.settle()
	case actPass:
		e.say("%s cannot move a ring and passes.", seatLabel(seat))
		e.endTurn()
	}
}

// settle hands the next decision on after a ring has moved or a row has gone:
// to the mover while they have rows to remove, then to the opponent, and when
// none is left the turn ends.
func (e *eng) settle() {
	d := e.d
	for _, s := range []int{d.Mover, other(d.Mover)} {
		if len(e.rows(s)) > 0 {
			d.Stage, d.Turn = stageRow, s
			return
		}
	}
	e.endTurn()
}

// endTurn passes the turn, or ends the game when the markers are all on the
// board.
func (e *eng) endTurn() {
	d := e.d
	d.Stage, d.Mover = stagePlay, other(d.Mover)
	d.Turn = d.Mover
	if e.markersOnBoard() >= e.supply() {
		e.say("The last marker is on the board: the game is over.")
		switch {
		case d.Score[0] > d.Score[1]:
			e.finish(1, "markers")
		case d.Score[1] > d.Score[0]:
			e.finish(2, "markers")
		default:
			e.finish(0, "markers")
		}
	}
}

// --- ruleset interface ----------------------------------------------------

// Start returns the empty board: White places the first ring.
func (rs *Ruleset) Start(game *catalog.CompiledGame, seed uint64) (*state.BoardState, error) {
	st := state.NewBoardState()
	st.SetExt(&Data{Turn: 1, Mover: 1, Stage: stageSetup})
	return st, nil
}

// ToMove is the seat that decides next, or 0 once the game is over.
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
	out := state.Outcome{Ended: true, Reason: d.Reason, Detail: fmt.Sprintf("%d–%d rings removed", d.Score[0], d.Score[1])}
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
	if d.Turn < 0 || d.Turn > 2 || d.Mover < 0 || d.Mover > 2 {
		return fmt.Errorf("yinsh: bad turn %d/%d", d.Turn, d.Mover)
	}
	switch d.Stage {
	case stageSetup, stagePlay, stageRow, stageRing:
	default:
		return fmt.Errorf("yinsh: unknown stage %q", d.Stage)
	}
	e := newEng(game, st, d)
	for _, id := range st.Occupied() {
		o, _ := st.Get(id)
		if (o.Piece != "ring" && o.Piece != "marker") || e.colors[1] != o.Color && e.colors[2] != o.Color {
			return fmt.Errorf("yinsh: bad piece %s/%s at %s", o.Color, o.Piece, id)
		}
		if st.Height(id) > 1 {
			return fmt.Errorf("yinsh: two pieces on %s", id)
		}
	}
	return nil
}

// Legal lists the decisions of seat.
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
		return nil, fmt.Errorf("yinsh: the game is over")
	}
	if seat != d.Turn {
		return nil, fmt.Errorf("yinsh: it is %s's decision", seatLabel(d.Turn))
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
		return nil, fmt.Errorf("yinsh: %q is not a legal move now", want)
	}
	nd := d.clone()
	e.d = nd
	e.execute(legal, seat)
	nd.Events = e.ev
	ns := st.Clone()
	ns.SetExt(nd)
	e.save(ns)
	ns.SetPly(st.Ply() + 1)
	if nd.Turn != 0 && ns.Ply() >= game.ParamInt("max_plies", 1000) {
		e.say("The move limit is reached: the player with more rings removed wins.")
		switch {
		case nd.Score[0] > nd.Score[1]:
			e.finish(1, "ply-limit")
		case nd.Score[1] > nd.Score[0]:
			e.finish(2, "ply-limit")
		default:
			e.finish(0, "ply-limit")
		}
		nd.Events = e.ev
	}
	res := &state.MoveResult{NewState: ns, Move: legal, Notation: rs.Format(game, legal), NextTurnSeat: nd.Turn}
	if nd.Turn == 0 {
		res.GameEnded = true
		res.NextTurnSeat = seat
		res.EndReason = nd.Reason
		res.EndDetail = fmt.Sprintf("%d–%d rings removed", nd.Score[0], nd.Score[1])
		if nd.Winner != 0 {
			w := nd.Winner
			res.WinnerSeat = &w
		}
	}
	return res, nil
}
