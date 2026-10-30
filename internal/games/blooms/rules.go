package blooms

import (
	"fmt"
	"strings"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

// Ruleset is Blooms' state.Ruleset. A turn is a first stone, then either a
// second stone of the other colour or the end of the turn; captures happen
// when the turn ends.
type Ruleset struct{}

// New returns the ruleset.
func New() *Ruleset { return &Ruleset{} }

const (
	actPlace = "place"
	actDone  = "done"
	actSwap  = "swap"
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

func (e *eng) target() int { return e.game.ParamInt("capture_target", 20) }

// groups returns the groups (connected stones of one colour) of the given
// colour that have no empty cell beside them.
func (e *eng) fenced(owner int) [][]int {
	seen := make([]bool, len(e.cells))
	var out [][]int
	for i, c := range e.cells {
		if c == "" || seen[i] || ownerOf(c) != owner {
			continue
		}
		group, free := []int{i}, false
		seen[i] = true
		for k := 0; k < len(group); k++ {
			for _, n := range e.g.nbr[group[k]] {
				switch {
				case e.cells[n] == "":
					free = true
				case e.cells[n] == c && !seen[n]:
					seen[n] = true
					group = append(group, n)
				}
			}
		}
		if !free {
			out = append(out, group)
		}
	}
	return out
}

// legal lists the moves of the seat to move.
func (e *eng) legal() []state.Move {
	d := e.d
	if d.Turn == 0 {
		return nil
	}
	var out []state.Move
	for _, color := range seatColors[d.Turn] {
		if d.Stage == stageSecond && color == d.First {
			continue
		}
		for i, c := range e.cells {
			if c == "" {
				out = append(out, state.Move{Action: actPlace, Piece: color, To: e.g.ids[i]})
			}
		}
	}
	if d.Stage == stageSecond {
		out = append(out, state.Move{Action: actDone})
	}
	// The pie rule: the second player's first reply may be to take the
	// opening side. The catalog brings the setup choice (pie_rule); the swap
	// itself is the ruleset's own move.
	if e.pieOn() && !d.Swapped && d.Turn == 2 && d.Turns == 1 && d.Stage == stageFirst {
		out = append(out, state.Move{Action: actSwap})
	}
	return out
}

// pieOn reports whether this game is played with the pie rule.
func (e *eng) pieOn() bool { return catalog.PieRuleOn(e.game.Definition) }

// --- ruleset interface ----------------------------------------------------

// Start returns the empty board; Player 1 (Red and Yellow) moves first.
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
	out := state.Outcome{Ended: true, Reason: d.Reason, Detail: fmt.Sprintf("%d–%d stones captured", d.Captured[0], d.Captured[1])}
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
		return fmt.Errorf("blooms: bad turn %d", d.Turn)
	}
	if d.Stage != stageFirst && d.Stage != stageSecond {
		return fmt.Errorf("blooms: unknown stage %q", d.Stage)
	}
	for _, id := range st.Occupied() {
		occ, _ := st.Get(id)
		if ownerOf(occ.Color) == 0 || occ.Piece != occ.Color {
			return fmt.Errorf("blooms: bad stone %s/%s at %s", occ.Color, occ.Piece, id)
		}
		if st.Height(id) > 1 {
			return fmt.Errorf("blooms: two stones on %s", id)
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

// Apply plays one decision of a turn.
func (rs *Ruleset) Apply(game *catalog.CompiledGame, st *state.BoardState, seat int, mv state.Move) (*state.MoveResult, error) {
	d, err := dataOf(st)
	if err != nil {
		return nil, err
	}
	if d.Turn == 0 {
		return nil, fmt.Errorf("blooms: the game is over")
	}
	if seat != d.Turn {
		return nil, fmt.Errorf("blooms: it is %s's turn", seatLabel(d.Turn))
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
		return nil, fmt.Errorf("blooms: %q is not a legal move now", want)
	}
	nd := d.clone()
	e.d = nd
	e.ev = nil
	ns := st.Clone()
	e.execute(legal, seat)
	nd.Events = e.ev
	ns.SetExt(nd)
	e.save(ns)
	ns.SetPly(st.Ply() + 1)
	if nd.Turn != 0 && ns.Ply() >= game.ParamInt("max_plies", 500) {
		e.finish(0, "ply-limit")
		switch {
		case nd.Captured[0] > nd.Captured[1]:
			nd.Winner = 1
		case nd.Captured[1] > nd.Captured[0]:
			nd.Winner = 2
		}
		nd.Events = e.ev
	}
	res := &state.MoveResult{NewState: ns, Move: legal, Notation: rs.Format(game, legal), NextTurnSeat: nd.Turn}
	if nd.Turn == 0 {
		res.GameEnded = true
		res.NextTurnSeat = seat
		res.EndReason = nd.Reason
		res.EndDetail = fmt.Sprintf("%d–%d stones captured", nd.Captured[0], nd.Captured[1])
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
	if reason == "ply-limit" {
		e.say("The move limit is reached: the player with more captures wins.")
	}
}

func (e *eng) execute(mv state.Move, seat int) {
	d := e.d
	switch mv.Action {
	case actSwap:
		// The pie rule: the second player takes the opening side. The stones
		// stay exactly where and what they are — the players change seats
		// (the session layer exchanges them) — so the turn stays with the
		// mover's seat, held from now on by the player who opened.
		d.Swapped = true
		e.say("%s swaps seats (the pie rule): the opening stone changes hands, and the first player answers it.", seatLabel(seat))
		return
	case actPlace:
		i := e.g.index[mv.To]
		e.cells[i] = mv.Piece
		d.Placed = append(d.Placed, string(mv.To))
		e.say("%s places a %s stone on %s.", seatLabel(seat), mv.Piece, mv.To)
		// Player 1's first turn is a single stone.
		if d.Stage == stageFirst && d.Turns > 0 && e.hasEmpty() {
			d.Stage, d.First = stageSecond, mv.Piece
			return
		}
	}
	e.endTurn(seat)
}

func (e *eng) hasEmpty() bool {
	for _, c := range e.cells {
		if c == "" {
			return true
		}
	}
	return false
}

// endTurn captures the opponent's fenced blooms (never the mover's own, which
// stay until the opponent's turn ends) and passes the turn.
func (e *eng) endTurn(seat int) {
	d := e.d
	taken := 0
	var gone []string
	for _, group := range e.fenced(other(seat)) {
		taken += len(group)
		for _, i := range group {
			gone = append(gone, string(e.g.ids[i]))
			e.cells[i] = ""
		}
	}
	if taken > 0 {
		d.Captured[seat-1] += taken
		e.say("%s captures %d stone(s): %s. (%d of %d)", seatLabel(seat), taken, strings.Join(gone, " "), d.Captured[seat-1], e.target())
	}
	d.Last, d.Placed = d.Placed, nil
	d.Stage, d.First = stageFirst, ""
	d.Turns++
	if d.Captured[seat-1] >= e.target() {
		e.finish(seat, "captures")
		e.say("%s has captured %d stones and wins!", seatLabel(seat), d.Captured[seat-1])
		return
	}
	d.Turn = other(seat)
}

func other(seat int) int { return 3 - seat }
