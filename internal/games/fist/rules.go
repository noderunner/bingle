package fist

import (
	"fmt"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

// Ruleset is Fist's state.Ruleset. A move is one whole turn: a piece's step
// (with an optional pin) or a chain of jumps with a choice at every jump.
type Ruleset struct{}

// New returns the ruleset.
func New() *Ruleset { return &Ruleset{} }

// Move actions.
const (
	actMove = "move"
	actJump = "jump"
)

// What a jump does to the piece it passes over.
const (
	jumpSpare   = 0
	jumpCapture = 1
	jumpConvert = 2 // the Ace takes the piece and replaces it with its own
)

// pc is one piece on the board: its kind and side (0 for the pawn).
type pc struct{ kind, side uint8 }

// The eight sight lines: file and rank steps.
var dirs = [8][2]int{{1, 0}, {1, 1}, {0, 1}, {-1, 1}, {-1, 0}, {-1, -1}, {0, -1}, {1, -1}}

// The Keep, the four squares in the middle: d4 e4 d5 e5.
var keep = [4]int{3*8 + 3, 3*8 + 4, 4*8 + 3, 4*8 + 4}

func cellName(c int) string { return string([]byte{byte('a' + c%8), byte('1' + c/8)}) }

func cellOf(id board.PositionID) (int, bool) {
	s := string(id)
	if len(s) != 2 || s[0] < 'a' || s[0] > 'h' || s[1] < '1' || s[1] > '8' {
		return 0, false
	}
	return int(s[1]-'1')*8 + int(s[0]-'a'), true
}

func step(c int, d [2]int, n int) (int, bool) {
	f, r := c%8+d[0]*n, c/8+d[1]*n
	if f < 0 || f > 7 || r < 0 || r > 7 {
		return 0, false
	}
	return r*8 + f, true
}

// touching lists the squares sharing an edge with c.
func touching(c int) []int {
	var out []int
	for _, d := range [4][2]int{{1, 0}, {0, 1}, {-1, 0}, {0, -1}} {
		if n, ok := step(c, d, 1); ok {
			out = append(out, n)
		}
	}
	return out
}

func other(side uint8) uint8 { return 3 - side }

// eng is a position decoded for one turn. val holds the values as they stood
// when the turn began: Empower is settled then and does not change mid-turn.
type eng struct {
	d   *Data
	b   [64]pc
	val [64]int8
	emp [64]bool
	ev  []string
}

func newEng(st *state.BoardState, d *Data) *eng {
	e := &eng{d: d}
	for _, id := range st.Occupied() {
		c, ok := cellOf(id)
		if !ok {
			continue
		}
		occ, _ := st.Get(id)
		side, _ := sideOf(occ.Color)
		e.b[c] = pc{kind: kindOf(occ.Piece), side: side}
	}
	e.freeze()
	return e
}

// save writes the pieces back onto the shared board.
func (e *eng) save(st *state.BoardState) {
	for _, id := range st.Occupied() {
		st.Remove(id)
	}
	for c, p := range e.b {
		if p.kind != kNone {
			st.Set(board.PositionID(cellName(c)), state.Occupant{Piece: kindName[p.kind], Color: sideColor[p.side]})
		}
	}
}

// firstSeen returns the first piece along direction d from c (-1 for none).
func (e *eng) firstSeen(c int, d [2]int) int {
	for n := 1; ; n++ {
		t, ok := step(c, d, n)
		if !ok {
			return -1
		}
		if e.b[t].kind != kNone {
			return t
		}
	}
}

// freeze settles every piece's value for the turn that begins now: the mage
// (and the king, once the king's side has taken the mage) adds 1 to each ally
// it sees. The pawn is never empowered.
func (e *eng) freeze() {
	for c, p := range e.b {
		e.val[c] = baseValue[p.kind]
		e.emp[c] = false
	}
	for side := uint8(1); side <= 2; side++ {
		src := -1
		for c, p := range e.b {
			switch {
			case side == seatChallenger && p.side == side && p.kind == kMage:
				src = c
			case side == seatKing && p.side == side && p.kind == kKing && e.d.KingEmpower:
				src = c
			}
		}
		if src < 0 {
			continue
		}
		for _, dir := range dirs {
			if t := e.firstSeen(src, dir); t >= 0 && e.b[t].side == side {
				e.val[t]++
				e.emp[t] = true
			}
		}
	}
}

// controls reports whether side can take the pawn this turn: one of its
// pieces sees it, or stands in the Keep.
func (e *eng) controls(side uint8) bool {
	pawn := e.pawnCell()
	if pawn < 0 {
		return false
	}
	for _, k := range keep {
		if e.b[k].kind != kNone && e.b[k].side == side {
			return true
		}
	}
	for _, dir := range dirs {
		if t := e.firstSeen(pawn, dir); t >= 0 && e.b[t].side == side {
			return true
		}
	}
	return false
}

func (e *eng) pawnCell() int {
	for c, p := range e.b {
		if p.kind == kPawn {
			return c
		}
	}
	return -1
}

func (e *eng) count(side, kind uint8) int {
	n := 0
	for _, p := range e.b {
		if p.side == side && p.kind == kind {
			n++
		}
	}
	return n
}

func rangeOf(kind uint8, val int8) int {
	if kind == kKing {
		return 1
	}
	return int(val)
}

func canPin(kind uint8) bool  { return kind == kYellow || kind == kKing || kind == kPawn }
func canJump(kind uint8) bool { return kind == kBlue || kind == kAce || kind == kMage || kind == kPawn }

// --- move generation ------------------------------------------------------

// legal lists every move of side, a turn each, in a stable order.
func (e *eng) legal(side uint8) []state.Move {
	if e.d.Turn == 0 {
		return nil
	}
	pawnOK := e.controls(side)
	var out []state.Move
	for c, p := range e.b {
		if p.kind == kNone || (p.side != side && (p.kind != kPawn || !pawnOK)) {
			continue
		}
		out = append(out, e.pieceMoves(c, side)...)
	}
	return out
}

func (e *eng) pieceMoves(from int, side uint8) []state.Move {
	p := e.b[from]
	rng := rangeOf(p.kind, e.val[from])
	var out []state.Move
	fromID := board.PositionID(cellName(from))
	pawnBack := -1
	if p.kind == kPawn && e.d.PawnFrom != "" {
		if c, ok := cellOf(board.PositionID(e.d.PawnFrom)); ok {
			pawnBack = c
		}
	}
	// Steps in a straight line over empty squares.
	for _, dir := range dirs {
		for n := 1; n <= rng; n++ {
			to, ok := step(from, dir, n)
			if !ok || e.b[to].kind != kNone {
				break
			}
			if to == pawnBack {
				continue
			}
			toID := board.PositionID(cellName(to))
			out = append(out, state.Move{Action: actMove, Piece: kindName[p.kind], From: fromID, To: toID})
			if canPin(p.kind) {
				for _, t := range e.pins(from, to, side) {
					out = append(out, state.Move{Action: actMove, Piece: kindName[p.kind], From: fromID, To: toID, Aim: board.PositionID(cellName(t))})
				}
			}
		}
	}
	if canJump(p.kind) {
		j := &jumper{e: e, side: side, kind: p.kind, power: e.val[from], max: rng, from: from, fromID: fromID, pawnBack: pawnBack}
		j.board = e.b
		j.val = e.val
		j.board[from] = pc{}
		j.visited[from] = true
		j.walk(from)
		out = append(out, j.out...)
	}
	return out
}

// pins lists the enemy pieces a piece moving from→to could pin: with several
// targets touching its new square it can take one only if it (with its
// allies) could take them all.
func (e *eng) pins(from, to int, side uint8) []int {
	var targets []int
	sum := 0
	for _, t := range touching(to) {
		if e.b[t].kind != kNone && e.b[t].side == other(side) {
			targets = append(targets, t)
			sum += int(e.val[t])
		}
	}
	if len(targets) == 0 {
		return nil
	}
	// Allies that touch any target add their value: the pawn lends none.
	strength := int(e.val[from])
	counted := map[int]bool{}
	for _, t := range targets {
		for _, a := range touching(t) {
			if a == from || counted[a] {
				continue
			}
			if a == to {
				continue
			}
			p := e.b[a]
			if p.kind != kNone && p.kind != kPawn && p.side == side {
				counted[a] = true
				strength += int(e.val[a])
			}
		}
	}
	if strength < sum {
		return nil
	}
	return targets
}

// jumper walks every chain of jumps a piece can make.
type jumper struct {
	e        *eng
	side     uint8
	kind     uint8
	power    int8
	max      int
	from     int
	fromID   board.PositionID
	pawnBack int
	board    [64]pc
	val      [64]int8
	visited  [64]bool
	path     []board.PositionID
	drops    []int
	out      []state.Move
	over     bool
}

func (j *jumper) count(side, kind uint8) int {
	n := 0
	for _, p := range j.board {
		if p.side == side && p.kind == kind {
			n++
		}
	}
	return n
}

func (j *jumper) walk(cur int) {
	if len(j.path) >= j.max || j.over {
		return
	}
	for _, dir := range dirs {
		mid, ok1 := step(cur, dir, 1)
		land, ok2 := step(cur, dir, 2)
		if !ok1 || !ok2 {
			continue
		}
		over := j.board[mid]
		if over.kind == kNone || j.board[land].kind != kNone || j.visited[land] || j.val[mid] > j.power {
			continue
		}
		options := []int{jumpSpare}
		if over.kind != kPawn {
			options = append(options, jumpCapture)
			// The Ace may replace an enemy guard with one of its own that is
			// out of play.
			if j.kind == kAce && over.side == other(j.side) && (over.kind == kYellow || over.kind == kBlue) &&
				j.count(j.side, over.kind) < total[j.side][over.kind] {
				options = append(options, jumpConvert)
			}
		}
		for _, opt := range options {
			saveMid, saveMidVal := j.board[mid], j.val[mid]
			switch opt {
			case jumpCapture:
				j.board[mid], j.val[mid] = pc{}, 0
			case jumpConvert:
				j.board[mid] = pc{kind: over.kind, side: j.side}
				j.val[mid] = baseValue[over.kind]
			}
			j.visited[land] = true
			j.path = append(j.path, board.PositionID(cellName(land)))
			j.drops = append(j.drops, opt)
			won := opt != jumpSpare && over.kind == kKing
			if land != j.pawnBack || j.kind != kPawn {
				j.out = append(j.out, state.Move{
					Action: actJump, Piece: kindName[j.kind], From: j.fromID, To: j.path[len(j.path)-1],
					Path: append([]board.PositionID(nil), j.path...), Drops: append([]int(nil), j.drops...),
				})
			}
			if !won {
				j.walk(land)
			}
			j.path, j.drops = j.path[:len(j.path)-1], j.drops[:len(j.drops)-1]
			j.visited[land] = false
			j.board[mid], j.val[mid] = saveMid, saveMidVal
		}
	}
}

// --- ruleset interface ----------------------------------------------------

// Start returns the opening position: the challenger moves first.
func (rs *Ruleset) Start(game *catalog.CompiledGame, seed uint64) (*state.BoardState, error) {
	st := state.NewBoardState()
	put := func(side uint8, kind uint8, cells ...string) {
		for _, c := range cells {
			st.Set(board.PositionID(c), state.Occupant{Piece: kindName[kind], Color: sideColor[side]})
		}
	}
	put(seatKing, kKing, "e5")
	put(seatKing, kBlue, "h8", "g8", "h7", "b8", "a7", "h2", "g1")
	put(seatKing, kYellow, "f8", "g7", "h6", "c8", "b7", "a6", "h3", "g2", "f1", "d5", "e4", "d4")
	put(seatChallenger, kAce, "a1")
	put(seatChallenger, kMage, "a8")
	put(seatChallenger, kBlue, "a2", "b1", "b2")
	put(seatChallenger, kYellow, "a3", "c1", "a4", "b3", "c2", "d1")
	put(0, kPawn, "h1")
	st.SetExt(&Data{Turn: seatChallenger})
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
	case "king-captured":
		return "the king was captured"
	case "guards-captured":
		return "all three blue guards were captured"
	case "stalemate":
		return "the side to move had no legal move"
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
		return fmt.Errorf("fist: bad turn %d", d.Turn)
	}
	e := newEng(st, d)
	for _, id := range st.Occupied() {
		occ, _ := st.Get(id)
		side, ok := sideOf(occ.Color)
		kind := kindOf(occ.Piece)
		switch {
		case !ok || kind == kNone:
			return fmt.Errorf("fist: unknown piece %s/%s at %s", occ.Color, occ.Piece, id)
		case (kind == kPawn) != (side == 0):
			return fmt.Errorf("fist: the verdigris pawn is green and nothing else is (%s at %s)", occ.Piece, id)
		case total[side][kind] == 0:
			return fmt.Errorf("fist: %s has no %s (at %s)", sideColor[side], occ.Piece, id)
		}
	}
	for side := uint8(0); side <= 2; side++ {
		for kind := kYellow; kind <= kPawn; kind++ {
			if n := e.count(side, kind); n > total[side][kind] {
				return fmt.Errorf("fist: %d %s %s on the board, only %d exist", n, sideColor[side], kindName[kind], total[side][kind])
			}
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
	return newEng(st, d).legal(uint8(seat))
}

// Apply plays one turn.
func (rs *Ruleset) Apply(game *catalog.CompiledGame, st *state.BoardState, seat int, mv state.Move) (*state.MoveResult, error) {
	d, err := dataOf(st)
	if err != nil {
		return nil, err
	}
	if d.Turn == 0 {
		return nil, fmt.Errorf("fist: the game is over")
	}
	if seat != d.Turn {
		return nil, fmt.Errorf("fist: it is the %s's turn", seatName(d.Turn))
	}
	e := newEng(st, d)
	want := rs.Format(game, mv)
	var legal state.Move
	found := false
	for _, cand := range e.legal(uint8(seat)) {
		if rs.Format(game, cand) == want {
			legal, found = cand, true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("fist: %q is not a legal move", want)
	}
	nd := d.clone()
	e.d = nd
	ns := st.Clone()
	e.execute(legal, uint8(seat))
	if nd.Turn != 0 {
		nd.Turn = int(other(uint8(seat)))
	}
	ns.SetExt(nd)
	e.save(ns)
	ns.SetPly(st.Ply() + 1)
	// The next player may be unable to move at all, or the game too long.
	if nd.Turn != 0 {
		next := newEng(ns, nd)
		switch {
		case len(next.legal(uint8(nd.Turn))) == 0:
			e.finish(nd, 0, "stalemate")
		case ns.Ply() >= game.ParamInt("max_plies", 400):
			e.finish(nd, 0, "ply-limit")
		}
	}
	nd.Events = e.ev
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
	return res, nil
}

func seatName(seat int) string {
	if seat == seatChallenger {
		return "challenger"
	}
	return "king's side"
}

func (e *eng) say(format string, args ...any) { e.ev = append(e.ev, fmt.Sprintf(format, args...)) }

func (e *eng) finish(d *Data, winner int, reason string) {
	d.Turn, d.Winner, d.Reason = 0, winner, reason
	switch reason {
	case "king-captured":
		e.say("The king is captured: the challenger wins!")
	case "guards-captured":
		e.say("All three blue guards are out of play: the king's side wins!")
	case "stalemate":
		e.say("The side to move has no legal move: the game is drawn.")
	case "ply-limit":
		e.say("The move limit is reached: the game is drawn.")
	}
}

// execute plays mv for side on the decoded board and checks the two goals.
func (e *eng) execute(mv state.Move, side uint8) {
	d := e.d
	from, _ := cellOf(mv.From)
	p := e.b[from]
	who := func() string { return fmt.Sprintf("%s's %s", seatName(int(side)), kindLabel[p.kind]) }
	if p.kind == kPawn {
		who = func() string { return fmt.Sprintf("The verdigris pawn (under the %s)", seatName(int(side))) }
	}
	d.LastFrom, d.LastTo, d.LastTook = string(mv.From), string(mv.To), nil
	e.b[from] = pc{}
	took := func(c int) pc {
		v := e.b[c]
		e.b[c] = pc{}
		d.LastTook = append(d.LastTook, cellName(c))
		if v.kind == kMage && side == seatKing {
			d.KingEmpower = true
			e.say("The king's side has taken the mage: the king now Empowers its allies.")
		}
		return v
	}
	switch mv.Action {
	case actMove:
		to, _ := cellOf(mv.To)
		e.b[to] = p
		if mv.Aim != "" {
			t, _ := cellOf(mv.Aim)
			v := took(t)
			e.say("%s moves %s → %s and pins the %s on %s.", who(), mv.From, mv.To, kindLabel[v.kind], mv.Aim)
		} else {
			e.say("%s moves %s → %s.", who(), mv.From, mv.To)
		}
	case actJump:
		cur := from
		for i, land := range mv.Path {
			to, _ := cellOf(land)
			mid := (cur/8+to/8)/2*8 + (cur%8+to%8)/2
			over := e.b[mid]
			switch mv.Drops[i] {
			case jumpCapture:
				took(mid)
				e.say("%s jumps %s → %s, capturing the %s on %s.", who(), cellName(cur), land, kindLabel[over.kind], cellName(mid))
			case jumpConvert:
				took(mid)
				e.b[mid] = pc{kind: over.kind, side: side}
				e.say("%s jumps %s → %s, capturing the %s on %s and replacing it with its own.", who(), cellName(cur), land, kindLabel[over.kind], cellName(mid))
			default:
				e.say("%s jumps %s → %s over %s.", who(), cellName(cur), land, cellName(mid))
			}
			cur = to
		}
		e.b[cur] = p
	}
	if p.kind == kPawn {
		d.PawnFrom = string(mv.From)
	} else {
		d.PawnFrom = ""
	}
	switch {
	case e.count(seatKing, kKing) == 0:
		e.finish(d, seatChallenger, "king-captured")
	case e.count(seatChallenger, kBlue) == 0:
		e.finish(d, seatKing, "guards-captured")
	}
}
