package mycelium

import (
	"fmt"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

// Ruleset is Mycelium's state.Ruleset. A normal turn is one placement; a
// confluence is a run of decisions, one spread at a time, in which the seat to
// move is whoever chooses the next spread.
type Ruleset struct{}

// New returns the ruleset.
func New() *Ruleset { return &Ruleset{} }

const (
	actPlace  = "place"
	actSpore  = "spore"
	actHyphae = "hyphae"
	actDone   = "done"
)

// eng is a decoded position: cells[i] is the colour on cell i ("" for empty).
type eng struct {
	g     *geo
	d     *Data
	cells []string
	game  *catalog.CompiledGame
	ev    []string
}

func newEng(game *catalog.CompiledGame, st *state.BoardState, d *Data) *eng {
	g := geoOf(game)
	e := &eng{g: g, d: d, cells: make([]string, len(g.ids)), game: game}
	for _, id := range st.Occupied() {
		if i, ok := g.index[id]; ok {
			occ, _ := st.Get(id)
			e.cells[i] = occ.Color
		}
	}
	return e
}

func (e *eng) save(st *state.BoardState) {
	for _, id := range st.Occupied() {
		st.Remove(id)
	}
	for i, c := range e.cells {
		if c != "" {
			st.Set(e.g.ids[i], state.Occupant{Piece: c, Color: c})
		}
	}
}

// count is the number of nodes of each seat (index seat-1).
func (e *eng) count() [2]int {
	var n [2]int
	for _, c := range e.cells {
		if s := ownerOf(c); s != 0 {
			n[s-1]++
		}
	}
	return n
}

// legal lists the moves of the seat to move.
func (e *eng) legal() []state.Move {
	d := e.d
	if d.Turn == 0 {
		return nil
	}
	if d.Confluence != nil {
		return e.spreads(d.Turn)
	}
	color := seatColors[d.Turn]
	var out []state.Move
	for i, c := range e.cells {
		if c == "" {
			out = append(out, state.Move{Action: actPlace, Piece: color, To: e.g.ids[i]})
		}
	}
	return out
}

// --- ruleset interface ----------------------------------------------------

// Start returns the empty board; Black (player 1) moves first.
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
	out := state.Outcome{Ended: true, Reason: d.Reason, Detail: detail(newEng(game, st, d).count())}
	if d.Winner != 0 {
		w := d.Winner
		out.Winner = &w
	}
	return out
}

func detail(n [2]int) string { return fmt.Sprintf("%d–%d nodes", n[0], n[1]) }

// Validate checks a position document.
func (rs *Ruleset) Validate(game *catalog.CompiledGame, st *state.BoardState) error {
	d, err := dataOf(st)
	if err != nil {
		return err
	}
	if d.Turn < 0 || d.Turn > 2 {
		return fmt.Errorf("mycelium: bad turn %d", d.Turn)
	}
	for _, id := range st.Occupied() {
		occ, _ := st.Get(id)
		if ownerOf(occ.Color) == 0 || occ.Piece != occ.Color {
			return fmt.Errorf("mycelium: bad node %s/%s at %s", occ.Color, occ.Piece, id)
		}
		if st.Height(id) > 1 {
			return fmt.Errorf("mycelium: two nodes on %s", id)
		}
	}
	cf := d.Confluence
	if cf == nil || d.Turn == 0 {
		return nil
	}
	if cf.Initiator != 1 && cf.Initiator != 2 {
		return fmt.Errorf("mycelium: bad confluence initiator %d", cf.Initiator)
	}
	if cf.Phase != phaseSpore && cf.Phase != phaseHyphae {
		return fmt.Errorf("mycelium: bad confluence phase %q", cf.Phase)
	}
	if cf.Initiative < 1 {
		return fmt.Errorf("mycelium: bad confluence initiative %d", cf.Initiative)
	}
	for _, c := range cf.Spent {
		if !game.Board.HasPosition(board.PositionID(c)) {
			return fmt.Errorf("mycelium: unknown spent cell %q", c)
		}
	}
	if cf.Head != "" {
		occ, ok := st.Get(board.PositionID(cf.Head))
		if !ok || ownerOf(occ.Color) != d.Turn || cf.Left < 1 {
			return fmt.Errorf("mycelium: bad hyphae spread at %q (%d left)", cf.Head, cf.Left)
		}
	}
	// A stored confluence is always resolved up to a decision: the seat to
	// move has a spread to choose.
	if len(newEng(game, st, d).legal()) == 0 {
		return fmt.Errorf("mycelium: %s has no spread to choose in the confluence", seatLabel(d.Turn))
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

// Apply plays one decision: a placement, or one step of a confluence.
func (rs *Ruleset) Apply(game *catalog.CompiledGame, st *state.BoardState, seat int, mv state.Move) (*state.MoveResult, error) {
	d, err := dataOf(st)
	if err != nil {
		return nil, err
	}
	if d.Turn == 0 {
		return nil, fmt.Errorf("mycelium: the game is over")
	}
	if seat != d.Turn {
		return nil, fmt.Errorf("mycelium: it is %s's turn", seatLabel(d.Turn))
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
		return nil, fmt.Errorf("mycelium: %q is not a legal move now", want)
	}
	nd := d.clone()
	e.d = nd
	e.ev = nil
	ns := st.Clone()
	e.execute(legal, seat)
	ns.SetPly(st.Ply() + 1)
	if nd.Turn != 0 && nd.Confluence == nil && ns.Ply() >= game.ParamInt("max_plies", 500) {
		e.finish(0, "ply-limit")
		e.say("The move limit is reached; the game is drawn.")
	}
	nd.Events = e.ev
	e.save(ns)
	ns.SetExt(nd)
	res := &state.MoveResult{NewState: ns, Move: legal, Notation: rs.Format(game, legal), NextTurnSeat: nd.Turn}
	if nd.Turn == 0 {
		res.GameEnded = true
		res.NextTurnSeat = seat
		res.EndReason = nd.Reason
		res.EndDetail = detail(e.count())
		if nd.Winner != 0 {
			w := nd.Winner
			res.WinnerSeat = &w
		}
	}
	return res, nil
}

func (e *eng) say(format string, args ...any) { e.ev = append(e.ev, fmt.Sprintf(format, args...)) }

func (e *eng) finish(winner int, reason string) {
	e.d.Turn, e.d.Winner, e.d.Reason = 0, winner, reason
}

// execute plays a legal move on e.
func (e *eng) execute(mv state.Move, seat int) {
	d := e.d
	if mv.Action != actPlace {
		e.spread(mv, seat)
		return
	}
	i := e.g.index[mv.To]
	e.cells[i] = mv.Piece
	d.Last = string(mv.To)
	e.say("%s places a node on %s.", seatLabel(seat), mv.To)
	d.Turn = other(seat)
	if i == e.g.centre {
		d.Confluence = &Confluence{Initiator: seat, Phase: phaseSpore, Initiative: 1}
		e.say("%s takes the centre: a confluence begins.", seatLabel(seat))
		e.resolve(other(seat))
	}
}
