package onitama

import (
	"fmt"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

// Ruleset is Onitama's state.Ruleset: one move is a card, a piece and a
// destination (or, when nothing can move, a pass that still spends a card).
type Ruleset struct{}

// New returns the ruleset.
func New() *Ruleset { return &Ruleset{} }

const (
	actMove = "move"
	actPass = "pass"
)

// Piece kinds.
const (
	pMaster  = "master"
	pStudent = "student"
)

func cellName(c int) string { return string([]byte{byte('a' + c%5), byte('1' + c/5)}) }

func cellOf(id board.PositionID) (int, bool) {
	s := string(id)
	if len(s) != 2 || s[0] < 'a' || s[0] > 'e' || s[1] < '1' || s[1] > '5' {
		return 0, false
	}
	return int(s[1]-'1')*5 + int(s[0]-'a'), true
}

// arch is each seat's Temple Arch: the middle of its home rank.
func arch(seat int) int {
	if seat == seatRed {
		return 2
	}
	return 4*5 + 2
}

func other(seat int) int { return 3 - seat }

type piece struct {
	master bool
	seat   int
}

// eng is a decoded position.
type eng struct {
	d *Data
	b [25]*piece
}

func newEng(st *state.BoardState, d *Data) *eng {
	e := &eng{d: d}
	for _, id := range st.Occupied() {
		c, ok := cellOf(id)
		if !ok {
			continue
		}
		occ, _ := st.Get(id)
		seat := seatRed
		if occ.Color == seatColor[seatBlue] {
			seat = seatBlue
		}
		e.b[c] = &piece{master: occ.Piece == pMaster, seat: seat}
	}
	return e
}

func (e *eng) save(st *state.BoardState) {
	for _, id := range st.Occupied() {
		st.Remove(id)
	}
	for c, p := range e.b {
		if p == nil {
			continue
		}
		kind := pStudent
		if p.master {
			kind = pMaster
		}
		st.Set(board.PositionID(cellName(c)), state.Occupant{Piece: kind, Color: seatColor[p.seat]})
	}
}

// offset turns a card's (right, forward) step into a (file, rank) step for
// seat: Blue faces the other way.
func offset(seat int, m [2]int) (int, int) {
	if seat == seatRed {
		return m[0], m[1]
	}
	return -m[0], -m[1]
}

func (e *eng) legal(seat int) []state.Move {
	if e.d.Turn == 0 {
		return nil
	}
	hand := e.d.Hands[seat-1]
	var out []state.Move
	for _, id := range hand {
		card, ok := cardByID(id)
		if !ok {
			continue
		}
		for c, p := range e.b {
			if p == nil || p.seat != seat {
				continue
			}
			for _, m := range card.Moves {
				df, dr := offset(seat, m)
				f, r := c%5+df, c/5+dr
				if f < 0 || f > 4 || r < 0 || r > 4 {
					continue
				}
				to := r*5 + f
				if q := e.b[to]; q != nil && q.seat == seat {
					continue
				}
				out = append(out, state.Move{Action: actMove, Piece: id, From: board.PositionID(cellName(c)), To: board.PositionID(cellName(to))})
			}
		}
	}
	if len(out) > 0 {
		return out
	}
	// Nothing can move: the turn passes, but a card is still exchanged.
	for _, id := range hand {
		out = append(out, state.Move{Action: actPass, Piece: id})
	}
	return out
}

// --- ruleset interface ----------------------------------------------------

// Seeded says the start position carries a random deal that replays reuse.
func (*Ruleset) Seeded() bool { return true }

func splitmix(s *uint64) uint64 {
	*s += 0x9e3779b97f4a7c15
	z := *s
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	return z ^ (z >> 31)
}

// Start deals five cards and sets up the pieces. Red (seat 1) always starts: the
// rules let the stamp on the side card choose, but the lobby and the clients
// take seat 1 as the first player.
func (rs *Ruleset) Start(game *catalog.CompiledGame, seed uint64) (*state.BoardState, error) {
	if seed == 0 {
		seed = 0x2545f4914f6cdd1d
	}
	deck := make([]string, len(cards))
	for i, c := range cards {
		deck[i] = c.ID
	}
	for i := len(deck) - 1; i > 0; i-- {
		j := int(splitmix(&seed) % uint64(i+1))
		deck[i], deck[j] = deck[j], deck[i]
	}
	d := &Data{Hands: [2][2]string{{deck[0], deck[1]}, {deck[2], deck[3]}}, Side: deck[4]}
	d.Turn = seatRed
	st := state.NewBoardState()
	for f := 0; f < 5; f++ {
		kind := pStudent
		if f == 2 {
			kind = pMaster
		}
		st.Set(board.PositionID(cellName(f)), state.Occupant{Piece: kind, Color: seatColor[seatRed]})
		st.Set(board.PositionID(cellName(20+f)), state.Occupant{Piece: kind, Color: seatColor[seatBlue]})
	}
	st.SetExt(d)
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
	out := state.Outcome{Ended: true, Reason: d.Reason, Detail: reasonText(d.Reason)}
	if d.Winner != 0 {
		w := d.Winner
		out.Winner = &w
	}
	return out
}

func reasonText(reason string) string {
	switch reason {
	case "stone":
		return "the Master was captured (Way of the Stone)"
	case "stream":
		return "a Master reached the opponent's Temple Arch (Way of the Stream)"
	case "ply-limit":
		return "move limit reached"
	}
	return reason
}

// Validate checks a position document.
func (rs *Ruleset) Validate(game *catalog.CompiledGame, st *state.BoardState) error {
	d, err := dataOf(st)
	if err != nil {
		return err
	}
	if d.Turn < 0 || d.Turn > 2 {
		return fmt.Errorf("onitama: bad turn %d", d.Turn)
	}
	seen := map[string]bool{}
	for _, id := range []string{d.Hands[0][0], d.Hands[0][1], d.Hands[1][0], d.Hands[1][1], d.Side} {
		if _, ok := cardByID(id); !ok {
			return fmt.Errorf("onitama: unknown card %q", id)
		}
		if seen[id] {
			return fmt.Errorf("onitama: card %q is dealt twice", id)
		}
		seen[id] = true
	}
	masters := [3]int{}
	students := [3]int{}
	e := newEng(st, d)
	for _, p := range e.b {
		if p == nil {
			continue
		}
		if p.master {
			masters[p.seat]++
		} else {
			students[p.seat]++
		}
	}
	for s := seatRed; s <= seatBlue; s++ {
		if masters[s] > 1 || students[s] > 4 {
			return fmt.Errorf("onitama: too many %s pieces", seatColor[s])
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
	return newEng(st, d).legal(seat)
}

// Apply plays one turn.
func (rs *Ruleset) Apply(game *catalog.CompiledGame, st *state.BoardState, seat int, mv state.Move) (*state.MoveResult, error) {
	d, err := dataOf(st)
	if err != nil {
		return nil, err
	}
	if d.Turn == 0 {
		return nil, fmt.Errorf("onitama: the game is over")
	}
	if seat != d.Turn {
		return nil, fmt.Errorf("onitama: it is %s's turn", seatColor[d.Turn])
	}
	e := newEng(st, d)
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
		return nil, fmt.Errorf("onitama: %q is not a legal move", want)
	}
	nd := d.clone()
	e.d = nd
	ns := st.Clone()
	e.execute(legal, seat)
	ns.SetExt(nd)
	e.save(ns)
	ns.SetPly(st.Ply() + 1)
	if nd.Turn != 0 && ns.Ply() >= game.ParamInt("max_plies", 300) {
		nd.Turn, nd.Winner, nd.Reason = 0, 0, "ply-limit"
		e.say("The move limit is reached: the game is drawn.")
	}
	res := &state.MoveResult{NewState: ns, Move: legal, Notation: rs.Format(game, legal), NextTurnSeat: nd.Turn}
	if nd.Turn == 0 {
		res.GameEnded = true
		res.NextTurnSeat = seat
		res.EndReason = nd.Reason
		res.EndDetail = reasonText(nd.Reason)
		if nd.Winner != 0 {
			w := nd.Winner
			res.WinnerSeat = &w
		}
	}
	nd.Events = e.ev()
	return res, nil
}

func (e *eng) say(format string, args ...any) {
	e.d.Events = append(e.d.Events, fmt.Sprintf(format, args...))
}

func (e *eng) ev() []string { return e.d.Events }

func (e *eng) execute(mv state.Move, seat int) {
	d := e.d
	d.Events = nil
	card, _ := cardByID(mv.Piece)
	d.LastCard, d.LastFrom, d.LastTo = card.ID, string(mv.From), string(mv.To)
	if mv.Action == actPass {
		d.LastFrom, d.LastTo = "", ""
		e.say("%s has no legal move and passes, using the %s card.", seatName(seat), card.Name)
	} else {
		from, _ := cellOf(mv.From)
		to, _ := cellOf(mv.To)
		p := e.b[from]
		who := "student"
		if p.master {
			who = "Master"
		}
		victim := e.b[to]
		e.b[from], e.b[to] = nil, p
		if victim != nil {
			vk := "student"
			if victim.master {
				vk = "Master"
			}
			e.say("%s's %s takes the %s on %s with the %s card (%s → %s).", seatName(seat), who, vk, mv.To, card.Name, mv.From, mv.To)
		} else {
			e.say("%s's %s moves %s → %s with the %s card.", seatName(seat), who, mv.From, mv.To, card.Name)
		}
		switch {
		case victim != nil && victim.master:
			d.Turn, d.Winner, d.Reason = 0, seat, "stone"
			e.say("%s wins the Way of the Stone.", seatName(seat))
		case p.master && to == arch(other(seat)):
			d.Turn, d.Winner, d.Reason = 0, seat, "stream"
			e.say("%s wins the Way of the Stream.", seatName(seat))
		}
	}
	// Exchange: the used card goes to the side, the waiting one to the hand.
	h := &d.Hands[seat-1]
	for i := range h {
		if h[i] == card.ID {
			h[i], d.Side = d.Side, card.ID
			break
		}
	}
	if d.Turn != 0 {
		d.Turn = other(seat)
	}
}

func seatName(seat int) string {
	if seat == seatRed {
		return "Red"
	}
	return "Blue"
}
