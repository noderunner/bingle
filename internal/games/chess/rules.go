package chess

import (
	"fmt"
	"hash/fnv"
	"strconv"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

// Ruleset is the chess state.Ruleset. A selection is one whole move: a
// piece from one square to another, with the piece a pawn promotes to.
type Ruleset struct{}

// New returns the ruleset.
func New() *Ruleset { return &Ruleset{} }

// Selection actions: an ordinary move, castling, or a pawn's move to the
// last rank named by the piece it becomes.
const (
	actMove   = "move"
	actCastle = "castle"
)

// promoKinds are the pieces a pawn may become, in the order offered.
var promoKinds = [4]uint8{kQueen, kRook, kBishop, kKnight}

// promoOf is the kind an action promotes to (kNone for any other action).
func promoOf(action string) uint8 {
	for _, k := range promoKinds {
		if kindName[k] == action {
			return k
		}
	}
	return kNone
}

// Castling rights, one bit each.
const (
	castleWK uint8 = 1 << iota
	castleWQ
	castleBK
	castleBQ
)

const castleLetters = "KQkq"

func castleBits(s string) uint8 {
	var b uint8
	for i := range castleLetters {
		for j := 0; j < len(s); j++ {
			if s[j] == castleLetters[i] {
				b |= 1 << i
			}
		}
	}
	return b
}

func castleString(b uint8) string {
	var out []byte
	for i := range castleLetters {
		if b&(1<<i) != 0 {
			out = append(out, castleLetters[i])
		}
	}
	return string(out)
}

// pc is one piece: its kind and side.
type pc struct{ kind, side uint8 }

// Squares are numbered rank*8+file, a1 = 0 and h8 = 63.
func cellName(c int) string { return string([]byte{byte('a' + c%8), byte('1' + c/8)}) }

func cellOf(s string) (int, bool) {
	if len(s) != 2 || s[0] < 'a' || s[0] > 'h' || s[1] < '1' || s[1] > '8' {
		return 0, false
	}
	return int(s[1]-'1')*8 + int(s[0]-'a'), true
}

// at is the square df files and dr ranks from c, if it is on the board.
func at(c, df, dr int) (int, bool) {
	f, r := c%8+df, c/8+dr
	if f < 0 || f > 7 || r < 0 || r > 7 {
		return 0, false
	}
	return r*8 + f, true
}

func other(side uint8) uint8 { return 3 - side }

// forward is the rank step of side's pawns.
func forward(side uint8) int {
	if side == seatWhite {
		return 1
	}
	return -1
}

var (
	knightSteps = [8][2]int{{1, 2}, {2, 1}, {2, -1}, {1, -2}, {-1, -2}, {-2, -1}, {-2, 1}, {-1, 2}}
	kingSteps   = [8][2]int{{1, 0}, {1, 1}, {0, 1}, {-1, 1}, {-1, 0}, {-1, -1}, {0, -1}, {1, -1}}
	rookDirs    = [4][2]int{{1, 0}, {0, 1}, {-1, 0}, {0, -1}}
	bishopDirs  = [4][2]int{{1, 1}, {-1, 1}, {-1, -1}, {1, -1}}
)

// pos is a position decoded for move generation.
type pos struct {
	b      [64]pc
	side   uint8 // to move
	castle uint8
	ep     int // the en passant square, or -1
}

// move is one legal move.
type move struct {
	from, to int
	kind     uint8 // the piece moving
	promo    uint8 // what a pawn becomes, or kNone
	capture  int   // the square of the piece taken, or -1
	castle   bool
}

func decode(st *state.BoardState, d *Data) *pos {
	p := &pos{side: sideOfSeat(d.Turn), castle: castleBits(d.Castling), ep: -1}
	for _, id := range st.Occupied() {
		c, ok := cellOf(string(id))
		if !ok {
			continue
		}
		occ, _ := st.Get(id)
		p.b[c] = pc{kind: kindOf(occ.Piece), side: sideOf(occ.Color)}
	}
	if c, ok := cellOf(d.EnPassant); ok {
		p.ep = c
	}
	return p
}

// save writes the pieces onto the shared board.
func (p *pos) save(st *state.BoardState) {
	for _, id := range st.Occupied() {
		st.Remove(id)
	}
	for c, q := range p.b {
		if q.kind != kNone {
			st.Set(board.PositionID(cellName(c)), state.Occupant{Piece: kindName[q.kind], Color: sideColor[q.side]})
		}
	}
}

func (p *pos) king(side uint8) int {
	for c, q := range p.b {
		if q.kind == kKing && q.side == side {
			return c
		}
	}
	return -1
}

// attacked reports whether a piece of side by attacks square sq.
func (p *pos) attacked(sq int, by uint8) bool {
	// A pawn attacks diagonally forward, so it stands one rank behind.
	for _, df := range [2]int{-1, 1} {
		if c, ok := at(sq, df, -forward(by)); ok && p.b[c] == (pc{kPawn, by}) {
			return true
		}
	}
	for _, s := range knightSteps {
		if c, ok := at(sq, s[0], s[1]); ok && p.b[c] == (pc{kKnight, by}) {
			return true
		}
	}
	for _, s := range kingSteps {
		if c, ok := at(sq, s[0], s[1]); ok && p.b[c] == (pc{kKing, by}) {
			return true
		}
	}
	slides := func(dirs [4][2]int, kind uint8) bool {
		for _, d := range dirs {
			for n := 1; ; n++ {
				c, ok := at(sq, d[0]*n, d[1]*n)
				if !ok {
					break
				}
				q := p.b[c]
				if q.kind == kNone {
					continue
				}
				if q.side == by && (q.kind == kind || q.kind == kQueen) {
					return true
				}
				break
			}
		}
		return false
	}
	return slides(rookDirs, kRook) || slides(bishopDirs, kBishop)
}

// inCheck reports whether side's king is attacked.
func (p *pos) inCheck(side uint8) bool {
	k := p.king(side)
	return k >= 0 && p.attacked(k, other(side))
}

// pseudo lists the moves of the side to move that obey how the pieces move,
// whether or not they leave its own king in check.
func (p *pos) pseudo() []move {
	side := p.side
	var out []move
	add := func(from, to int, kind uint8) {
		capture := -1
		if p.b[to].kind != kNone {
			capture = to
		}
		out = append(out, move{from: from, to: to, kind: kind, capture: capture})
	}
	for from, q := range p.b {
		if q.kind == kNone || q.side != side {
			continue
		}
		switch q.kind {
		case kPawn:
			out = p.pawnMoves(from, out)
		case kKnight, kKing:
			steps := knightSteps
			if q.kind == kKing {
				steps = kingSteps
			}
			for _, s := range steps {
				if to, ok := at(from, s[0], s[1]); ok && (p.b[to].kind == kNone || p.b[to].side != side) {
					add(from, to, q.kind)
				}
			}
		default:
			var dirs [][2]int
			if q.kind != kBishop {
				dirs = append(dirs, rookDirs[:]...)
			}
			if q.kind != kRook {
				dirs = append(dirs, bishopDirs[:]...)
			}
			for _, d := range dirs {
				for n := 1; ; n++ {
					to, ok := at(from, d[0]*n, d[1]*n)
					if !ok || (p.b[to].kind != kNone && p.b[to].side == side) {
						break
					}
					add(from, to, q.kind)
					if p.b[to].kind != kNone {
						break
					}
				}
			}
		}
	}
	return p.castles(out)
}

func (p *pos) pawnMoves(from int, out []move) []move {
	side := p.side
	fwd := forward(side)
	last := 7
	start := 1
	if side == seatBlack {
		last, start = 0, 6
	}
	push := func(to, capture int) {
		if to/8 == last {
			for _, k := range promoKinds {
				out = append(out, move{from: from, to: to, kind: kPawn, promo: k, capture: capture})
			}
			return
		}
		out = append(out, move{from: from, to: to, kind: kPawn, capture: capture})
	}
	if to, ok := at(from, 0, fwd); ok && p.b[to].kind == kNone {
		push(to, -1)
		if to2, ok := at(from, 0, 2*fwd); ok && from/8 == start && p.b[to2].kind == kNone {
			push(to2, -1)
		}
	}
	for _, df := range [2]int{-1, 1} {
		to, ok := at(from, df, fwd)
		if !ok {
			continue
		}
		switch {
		case p.b[to].kind != kNone && p.b[to].side != side:
			push(to, to)
		case to == p.ep:
			// The pawn taken en passant stands beside the one taking it.
			push(to, to-8*fwd)
		}
	}
	return out
}

// castles adds the castling moves: the king and rook unmoved (the rights),
// the squares between them empty, and the king neither in check nor
// passing over or landing on an attacked square.
func (p *pos) castles(out []move) []move {
	side := p.side
	home := 0
	short, long := castleWK, castleWQ
	if side == seatBlack {
		home, short, long = 56, castleBK, castleBQ
	}
	king := home + 4
	if p.b[king] != (pc{kKing, side}) || p.castle&(short|long) == 0 || p.attacked(king, other(side)) {
		return out
	}
	empty := func(cs ...int) bool {
		for _, c := range cs {
			if p.b[c].kind != kNone {
				return false
			}
		}
		return true
	}
	safe := func(cs ...int) bool {
		for _, c := range cs {
			if p.attacked(c, other(side)) {
				return false
			}
		}
		return true
	}
	if p.castle&short != 0 && p.b[home+7] == (pc{kRook, side}) && empty(home+5, home+6) && safe(home+5, home+6) {
		out = append(out, move{from: king, to: home + 6, kind: kKing, capture: -1, castle: true})
	}
	if p.castle&long != 0 && p.b[home] == (pc{kRook, side}) && empty(home+1, home+2, home+3) && safe(home+3, home+2) {
		out = append(out, move{from: king, to: home + 2, kind: kKing, capture: -1, castle: true})
	}
	return out
}

// play returns the position after m: pieces moved, rights and the en
// passant square updated, the other side to move.
func (p *pos) play(m move) *pos {
	n := *p
	q := n.b[m.from]
	n.b[m.from] = pc{}
	if m.capture >= 0 {
		n.b[m.capture] = pc{}
	}
	if m.promo != kNone {
		q.kind = m.promo
	}
	n.b[m.to] = q
	if m.castle {
		rank := m.to / 8 * 8
		if m.to%8 == 6 {
			n.b[rank+5], n.b[rank+7] = n.b[rank+7], pc{}
		} else {
			n.b[rank+3], n.b[rank] = n.b[rank], pc{}
		}
	}
	// A right is lost when the king or that rook moves, or the rook is taken.
	for _, c := range [2]int{m.from, m.to} {
		switch c {
		case 4:
			n.castle &^= castleWK | castleWQ
		case 60:
			n.castle &^= castleBK | castleBQ
		case 0:
			n.castle &^= castleWQ
		case 7:
			n.castle &^= castleWK
		case 56:
			n.castle &^= castleBQ
		case 63:
			n.castle &^= castleBK
		}
	}
	n.ep = -1
	if m.kind == kPawn && (m.to-m.from == 16 || m.from-m.to == 16) {
		n.ep = (m.from + m.to) / 2
	}
	n.side = other(p.side)
	return &n
}

// legal lists the moves of the side to move that do not leave its king in
// check, in a stable order.
func (p *pos) legal() []move {
	var out []move
	for _, m := range p.pseudo() {
		if !p.play(m).inCheck(p.side) {
			out = append(out, m)
		}
	}
	return out
}

// insufficient reports a position where neither side can ever checkmate:
// kings alone, or with one minor piece, or with bishops all on squares of
// one colour.
func (p *pos) insufficient() bool {
	minors, knights := 0, 0
	bishopColors := [2]bool{}
	for c, q := range p.b {
		switch q.kind {
		case kPawn, kRook, kQueen:
			return false
		case kKnight:
			minors++
			knights++
		case kBishop:
			minors++
			bishopColors[(c/8+c%8)%2] = true
		}
	}
	if minors <= 1 {
		return true
	}
	return knights == 0 && (!bishopColors[0] || !bishopColors[1])
}

// key hashes what makes two positions the same for repetition: the pieces,
// the side to move, the castling rights and an en passant capture that can
// really be made (legal are the moves of the side to move).
func (p *pos) key(legal []move) string {
	h := fnv.New64a()
	var b [68]byte
	for c, q := range p.b {
		b[c] = q.kind<<2 | q.side
	}
	b[64], b[65], b[66] = p.side, p.castle, 0xff
	for _, m := range legal {
		if m.kind == kPawn && m.to == p.ep {
			b[66] = byte(p.ep)
			break
		}
	}
	_, _ = h.Write(b[:67])
	return strconv.FormatUint(h.Sum64(), 36)
}

func (m move) selection() state.Selection {
	sel := state.Selection{Action: actMove, Piece: kindName[m.kind], From: board.PositionID(cellName(m.from)), To: board.PositionID(cellName(m.to))}
	switch {
	case m.castle:
		sel.Action = actCastle
	case m.promo != kNone:
		sel.Action = kindName[m.promo]
	}
	if m.capture >= 0 {
		sel.Aim = board.PositionID(cellName(m.capture))
	}
	return sel
}

// matches reports whether a parsed selection names m. A selection with both
// squares (from the list of legal moves, long notation or UCI) is matched by
// them and the promotion; one from short notation (Nf3, exd5, O-O) by what it
// gives: the piece, the square it goes to and any file or rank it names.
func matches(s state.Selection, m move) bool {
	if s.Action == actCastle {
		return m.castle && len(s.To) > 0 && s.To[0] == cellName(m.to)[0]
	}
	if promoOf(s.Action) != m.promo {
		return false
	}
	from := cellName(m.from)
	if len(s.From) == 2 {
		return string(s.From) == from && string(s.To) == cellName(m.to)
	}
	if m.castle || string(s.To) != cellName(m.to) || kindOf(s.Piece) != m.kind {
		return false
	}
	for i := 0; i < len(s.From); i++ {
		if c := s.From[i]; c != from[0] && c != from[1] {
			return false
		}
	}
	return true
}

// --- the ruleset interface ---------------------------------------------

func startPosition() *pos {
	p := &pos{side: seatWhite, castle: castleWK | castleWQ | castleBK | castleBQ, ep: -1}
	back := [8]uint8{kRook, kKnight, kBishop, kQueen, kKing, kBishop, kKnight, kRook}
	for f, k := range back {
		p.b[f] = pc{k, seatWhite}
		p.b[8+f] = pc{kPawn, seatWhite}
		p.b[48+f] = pc{kPawn, seatBlack}
		p.b[56+f] = pc{k, seatBlack}
	}
	return p
}

// Start returns the opening position: White moves first.
func (rs *Ruleset) Start(game *catalog.CompiledGame, seed uint64) (*state.BoardState, error) {
	p := startPosition()
	st := state.NewBoardState()
	p.save(st)
	st.SetExt(&Data{Turn: seatWhite, Castling: castleString(p.castle), Fullmove: 1, Seen: []string{p.key(p.legal())}})
	return st, nil
}

// ImpulseSeat is the seat to move, or 0 once the game is over.
func (rs *Ruleset) ImpulseSeat(game *catalog.CompiledGame, st *state.BoardState) int {
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
	case "checkmate":
		return "checkmate"
	case "stalemate":
		return "stalemate: the side to move has no legal move and is not in check"
	case "repetition":
		return "the same position came up three times"
	case "fifty-moves":
		return "fifty moves each without a capture or a pawn move"
	case "insufficient-material":
		return "neither side has the pieces left to checkmate"
	}
	return reason
}

// Validate checks a position document: known pieces, one king each, no pawn
// on the first or last rank, castling rights that fit the pieces, and the
// side that just moved not left in check.
func (rs *Ruleset) Validate(game *catalog.CompiledGame, st *state.BoardState) error {
	d, err := dataOf(st)
	if err != nil {
		return err
	}
	if d.Turn < 0 || d.Turn > 2 {
		return fmt.Errorf("chess: bad turn %d", d.Turn)
	}
	for _, id := range st.Occupied() {
		occ, _ := st.Get(id)
		c, ok := cellOf(string(id))
		switch {
		case !ok:
			return fmt.Errorf("chess: no square %s", id)
		case kindOf(occ.Piece) == kNone || sideOf(occ.Color) == 0:
			return fmt.Errorf("chess: unknown piece %s/%s at %s", occ.Color, occ.Piece, id)
		case occ.Piece == "pawn" && (c/8 == 0 || c/8 == 7):
			return fmt.Errorf("chess: a pawn cannot stand on %s", id)
		}
	}
	turn := d.Turn
	if turn == 0 {
		turn = seatWhite
	}
	p := decode(st, &Data{Turn: turn, Castling: d.Castling, EnPassant: d.EnPassant})
	for side := uint8(1); side <= 2; side++ {
		n := 0
		for _, q := range p.b {
			if q == (pc{kKing, side}) {
				n++
			}
		}
		if n != 1 {
			return fmt.Errorf("chess: %s has %d kings, not one", sideColor[side], n)
		}
	}
	for i := 0; i < len(d.Castling); i++ {
		bit := castleBits(d.Castling[i : i+1])
		if bit == 0 {
			return fmt.Errorf("chess: castling rights %q: only K, Q, k and q", d.Castling)
		}
		home, side := 0, uint8(seatWhite)
		if bit&(castleBK|castleBQ) != 0 {
			home, side = 56, seatBlack
		}
		rook := home + 7
		if bit&(castleWQ|castleBQ) != 0 {
			rook = home
		}
		if p.b[home+4] != (pc{kKing, side}) || p.b[rook] != (pc{kRook, side}) {
			return fmt.Errorf("chess: castling right %c needs the king and rook on their first squares", d.Castling[i])
		}
	}
	if d.EnPassant != "" {
		c, ok := cellOf(d.EnPassant)
		if want := map[int]int{seatWhite: 5, seatBlack: 2}[turn]; !ok || c/8 != want {
			return fmt.Errorf("chess: no en passant on %s with %s to move", d.EnPassant, sideColor[turn])
		}
	}
	if d.Turn != 0 && p.inCheck(other(p.side)) {
		return fmt.Errorf("chess: %s is in check with %s to move", sideColor[other(p.side)], sideColor[p.side])
	}
	return nil
}

// Legal lists the moves of seat.
func (rs *Ruleset) Legal(game *catalog.CompiledGame, st *state.BoardState, seat int) []state.Selection {
	d, err := dataOf(st)
	if err != nil || d.Turn == 0 || seat != d.Turn {
		return nil
	}
	moves := decode(st, d).legal()
	out := make([]state.Selection, len(moves))
	for i, m := range moves {
		out[i] = m.selection()
	}
	return out
}

// Apply plays one move.
func (rs *Ruleset) Apply(game *catalog.CompiledGame, st *state.BoardState, seat int, mv state.Selection) (*state.MoveResult, error) {
	d, err := dataOf(st)
	if err != nil {
		return nil, err
	}
	if d.Turn == 0 {
		return nil, fmt.Errorf("chess: the game is over")
	}
	if seat != d.Turn {
		return nil, fmt.Errorf("chess: it is %s's move", sideColor[d.Turn])
	}
	p := decode(st, d)
	var found []move
	for _, m := range p.legal() {
		if matches(mv, m) {
			found = append(found, m)
		}
	}
	switch {
	case len(found) == 0:
		return nil, fmt.Errorf("chess: %q is not a legal move", rs.Format(game, mv))
	case len(found) > 1:
		return nil, fmt.Errorf("chess: %q could be %s or %s: say which", rs.Format(game, mv),
			rs.Format(game, found[0].selection()), rs.Format(game, found[1].selection()))
	}
	m := found[0]
	np := p.play(m)
	replies := np.legal()

	nd := d.clone()
	nd.Castling = castleString(np.castle)
	nd.EnPassant = ""
	if np.ep >= 0 {
		nd.EnPassant = cellName(np.ep)
	}
	nd.Halfmove++
	if m.kind == kPawn || m.capture >= 0 {
		nd.Halfmove = 0
	}
	if seat == seatBlack {
		nd.Fullmove++
	}
	// A position after a capture, a pawn move or a lost right can never come
	// back, so the earlier ones need not be kept.
	if nd.Halfmove == 0 || np.castle != p.castle {
		nd.Seen = nil
	}
	key := np.key(replies)
	nd.Seen = append(nd.Seen, key)
	nd.Turn = int(np.side)
	seen := 0
	for _, k := range nd.Seen {
		if k == key {
			seen++
		}
	}
	switch {
	case len(replies) == 0 && np.inCheck(np.side):
		nd.Turn, nd.Winner, nd.Reason = 0, seat, "checkmate"
	case len(replies) == 0:
		nd.Turn, nd.Reason = 0, "stalemate"
	case np.insufficient():
		nd.Turn, nd.Reason = 0, "insufficient-material"
	case seen >= 3:
		nd.Turn, nd.Reason = 0, "repetition"
	case nd.Halfmove >= 100:
		nd.Turn, nd.Reason = 0, "fifty-moves"
	}

	ns := st.Clone()
	np.save(ns)
	ns.SetExt(nd)
	ns.SetPly(st.Ply() + 1)
	sel := m.selection()
	res := &state.MoveResult{NewState: ns, Move: sel, Notation: rs.Format(game, sel), NextImpulseSeat: nd.Turn}
	if m.capture >= 0 {
		c := board.PositionID(cellName(m.capture))
		res.Captured, res.Captures = &c, []board.PositionID{c}
	}
	if nd.Turn == 0 {
		res.GameEnded = true
		res.NextImpulseSeat = seat
		res.EndReason = nd.Reason
		res.EndDetail = reasonText(nd.Reason)
		if nd.Winner != 0 {
			w := nd.Winner
			res.WinnerSeat = &w
		}
	}
	return res, nil
}
