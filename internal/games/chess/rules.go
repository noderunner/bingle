package chess

import (
	"fmt"
	"hash/fnv"
	"strconv"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

// Ruleset is the state.Ruleset of chess and its variants (the definition's
// param "variant"). A selection is one whole move: a piece from one square
// to another, a castling, or a Crazyhouse drop.
type Ruleset struct{}

// New returns the ruleset.
func New() *Ruleset { return &Ruleset{} }

// Seeded reports that a start position may carry a seed to record: a
// Chess960 table is drawn from it (state.SeededRuleset).
func (*Ruleset) Seeded() bool { return true }

// variant is which game of the family the rules play.
type variant uint8

const (
	vStandard variant = iota
	v960
	vCrazyhouse
	vAntichess
	vAtomic
	vAlice
)

var variantByName = map[string]variant{
	"standard": vStandard, "960": v960, "crazyhouse": vCrazyhouse, "antichess": vAntichess, "atomic": vAtomic, "alice": vAlice,
}

func variantOf(game *catalog.CompiledGame) variant {
	if game == nil {
		return vStandard
	}
	return variantByName[game.ParamString("variant", "standard")]
}

// Selection actions: an ordinary move, castling, a drop from the hand, or a
// pawn's move to the last rank named by the piece it becomes.
const (
	actMove   = "move"
	actCastle = "castle"
	actDrop   = "drop"
)

// promoKinds are the pieces a pawn may become, in the order offered; in
// Antichess it may become a king too.
var promoKinds = []uint8{kQueen, kRook, kBishop, kKnight}

func (v variant) promotions() []uint8 {
	if v == vAntichess {
		return append(append([]uint8(nil), promoKinds...), kKing)
	}
	return promoKinds
}

// promoOf is the kind an action promotes to (kNone for any other action).
func promoOf(action string) uint8 {
	switch k := kindOf(action); k {
	case kQueen, kRook, kBishop, kKnight, kKing:
		return k
	}
	return kNone
}

// pc is one piece: its kind and side.
type pc struct{ kind, side uint8 }

// Squares are numbered board*64 + rank*8 + file: a1 = 0 and h8 = 63 on the
// board everybody plays on, and i1 = 64 to p8 = 127 on Alice's second board.
func cellName(g int) string {
	return string([]byte{byte('a' + g/64*8 + g%8), byte('1' + g%64/8)})
}

func cellOf(s string) (int, bool) {
	if len(s) != 2 || s[0] < 'a' || s[0] > 'p' || s[1] < '1' || s[1] > '8' {
		return 0, false
	}
	f := int(s[0] - 'a')
	return f/8*64 + int(s[1]-'1')*8 + f%8, true
}

// fold names a file of either board by its letter on the first: i is a.
func fold(file byte) byte {
	if file >= 'i' && file <= 'p' {
		return file - 8
	}
	return file
}

// step is the square df files and dr ranks from g on the same board.
func step(g, df, dr int) (int, bool) {
	f, r := g%8+df, g%64/8+dr
	if f < 0 || f > 7 || r < 0 || r > 7 {
		return 0, false
	}
	return g&^63 + r*8 + f, true
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
	v    variant
	b    [128]pc
	side uint8 // to move
	// rook is the square of the rook each castling right is for, by side and
	// wing (0 the king's side, 1 the queen's), or -1.
	rook [3][2]int
	ep   int // the en passant square, or -1
	// hand counts each side's pieces in hand (Crazyhouse), and promoted marks
	// the pieces that were pawns.
	hand     [3][7]int
	promoted [128]bool
}

// move is one move.
type move struct {
	from int // -1 for a drop
	to   int // where the piece ends (Alice: on the other board)
	land int // where it moves on its own board (to, but for Alice)
	kind uint8
	// promo is what a pawn becomes, or kNone.
	promo uint8
	// capture is the square of the piece taken, or -1.
	capture          int
	castle, drop     bool
	rookFrom, rookTo int
}

func noRights() [3][2]int { return [3][2]int{{-1, -1}, {-1, -1}, {-1, -1}} }

func decode(st *state.BoardState, d *Data, v variant) *pos {
	p := &pos{v: v, side: sideOfSeat(d.Turn), ep: -1}
	for _, id := range st.Occupied() {
		g, ok := cellOf(string(id))
		if !ok {
			continue
		}
		occ, _ := st.Get(id)
		p.b[g] = pc{kind: kindOf(occ.Piece), side: sideOf(occ.Color)}
	}
	if g, ok := cellOf(d.EnPassant); ok {
		p.ep = g
	}
	p.rook, _ = p.parseCastling(d.Castling)
	if v == vCrazyhouse {
		for side := uint8(1); side <= 2; side++ {
			for k := kPawn; k <= kQueen; k++ {
				p.hand[side][k] = st.Reserve(sideColor[side], kindName[k])
			}
		}
		for _, s := range d.Promoted {
			if g, ok := cellOf(s); ok {
				p.promoted[g] = true
			}
		}
	}
	return p
}

// parseCastling reads castling rights in X-FEN: K and Q the outermost rook
// on each side of the king, or a rook's file letter. It reports a right
// that names no rook of that side on its first rank.
func (p *pos) parseCastling(s string) ([3][2]int, error) {
	rook := noRights()
	for i := 0; i < len(s); i++ {
		c := s[i]
		side, home := uint8(seatWhite), 0
		if c >= 'a' && c <= 'z' {
			side, home, c = seatBlack, 56, c-'a'+'A'
		}
		kf := -1
		for f := 0; f < 8; f++ {
			if p.b[home+f] == (pc{kKing, side}) {
				kf = f
			}
		}
		found := -1
		switch {
		case kf < 0:
		case c == 'K':
			for f := 7; f > kf && found < 0; f-- {
				if p.b[home+f] == (pc{kRook, side}) {
					found = f
				}
			}
		case c == 'Q':
			for f := 0; f < kf && found < 0; f++ {
				if p.b[home+f] == (pc{kRook, side}) {
					found = f
				}
			}
		case c >= 'A' && c <= 'H':
			if f := int(c - 'A'); f != kf && p.b[home+f] == (pc{kRook, side}) {
				found = f
			}
		}
		if found < 0 {
			return rook, fmt.Errorf("chess: castling right %c names no rook beside the king on its first rank", s[i])
		}
		wing := 0
		if found < kf {
			wing = 1
		}
		rook[side][wing] = home + found
	}
	return rook, nil
}

// castling writes the rights in X-FEN: K and Q for rooks in the corners,
// the file otherwise.
func (p *pos) castling() string {
	var out []byte
	for side := uint8(1); side <= 2; side++ {
		for wing := 0; wing < 2; wing++ {
			r := p.rook[side][wing]
			if r < 0 {
				continue
			}
			c := byte('A' + r%8)
			if (wing == 0 && r%8 == 7) || (wing == 1 && r%8 == 0) {
				c = "KQ"[wing]
			}
			if side == seatBlack {
				c += 'a' - 'A'
			}
			out = append(out, c)
		}
	}
	return string(out)
}

// save writes the pieces and the hands onto the shared board.
func (p *pos) save(st *state.BoardState) {
	for _, id := range st.Occupied() {
		st.Remove(id)
	}
	for g, q := range p.b {
		if q.kind != kNone {
			st.Set(board.PositionID(cellName(g)), state.Occupant{Piece: kindName[q.kind], Color: sideColor[q.side]})
		}
	}
	if p.v == vCrazyhouse {
		for side := uint8(1); side <= 2; side++ {
			for k := kPawn; k <= kQueen; k++ {
				st.SetReserve(sideColor[side], kindName[k], p.hand[side][k])
			}
		}
	}
}

func (p *pos) promotedSquares() []string {
	var out []string
	for g, on := range p.promoted {
		if on && p.b[g].kind != kNone {
			out = append(out, cellName(g))
		}
	}
	return out
}

func (p *pos) king(side uint8) int {
	for g, q := range p.b {
		if q.kind == kKing && q.side == side {
			return g
		}
	}
	return -1
}

func (p *pos) count(side uint8) int {
	n := 0
	for _, q := range p.b {
		if q.kind != kNone && q.side == side {
			n++
		}
	}
	return n
}

// attacked reports whether a piece of side by attacks square sq on sq's
// board; kings counts the king's attack (not in Atomic, where a king cannot
// capture).
func (p *pos) attacked(sq int, by uint8, kings bool) bool {
	// A pawn attacks diagonally forward, so it stands one rank behind.
	for _, df := range [2]int{-1, 1} {
		if c, ok := step(sq, df, -forward(by)); ok && p.b[c] == (pc{kPawn, by}) {
			return true
		}
	}
	for _, s := range knightSteps {
		if c, ok := step(sq, s[0], s[1]); ok && p.b[c] == (pc{kKnight, by}) {
			return true
		}
	}
	if kings {
		for _, s := range kingSteps {
			if c, ok := step(sq, s[0], s[1]); ok && p.b[c] == (pc{kKing, by}) {
				return true
			}
		}
	}
	slides := func(dirs [4][2]int, kind uint8) bool {
		for _, d := range dirs {
			for n := 1; ; n++ {
				c, ok := step(sq, d[0]*n, d[1]*n)
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

func adjacent(a, b int) bool {
	if a == b || a&^63 != b&^63 {
		return false
	}
	df, dr := a%8-b%8, a%64/8-b%64/8
	return df >= -1 && df <= 1 && dr >= -1 && dr <= 1
}

// safeAt reports whether side's king would be out of check on sq. In
// Atomic a king beside the other king is never in check, since taking it
// would blow up the taker's own king.
func (p *pos) safeAt(sq int, side uint8) bool {
	enemy := other(side)
	if p.v == vAtomic {
		if k := p.king(enemy); k >= 0 && adjacent(k, sq) {
			return true
		}
		return !p.attacked(sq, enemy, false)
	}
	return !p.attacked(sq, enemy, true)
}

// inCheck reports whether side's king is attacked (never in Antichess,
// where the king is an ordinary piece).
func (p *pos) inCheck(side uint8) bool {
	if p.v == vAntichess {
		return false
	}
	k := p.king(side)
	return k >= 0 && !p.safeAt(k, side)
}

// add appends a move of the piece on from to land (taking capture), unless
// Alice's square on the other board is taken or an Atomic king would
// capture.
func (p *pos) add(out []move, from, land int, kind, promo uint8, capture int) []move {
	to := land
	if p.v == vAlice {
		to = land ^ 64
		if p.b[to].kind != kNone {
			return out
		}
	}
	if p.v == vAtomic && kind == kKing && capture >= 0 {
		return out
	}
	return append(out, move{from: from, to: to, land: land, kind: kind, promo: promo, capture: capture, rookFrom: -1, rookTo: -1})
}

// pseudo lists the moves of the side to move that obey how the pieces move,
// whether or not they leave its own king in check.
func (p *pos) pseudo() []move {
	side := p.side
	var out []move
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
				to, ok := step(from, s[0], s[1])
				switch {
				case !ok:
				case p.b[to].kind == kNone:
					out = p.add(out, from, to, q.kind, kNone, -1)
				case p.b[to].side != side:
					out = p.add(out, from, to, q.kind, kNone, to)
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
					to, ok := step(from, d[0]*n, d[1]*n)
					if !ok || (p.b[to].kind != kNone && p.b[to].side == side) {
						break
					}
					if p.b[to].kind != kNone {
						out = p.add(out, from, to, q.kind, kNone, to)
						break
					}
					out = p.add(out, from, to, q.kind, kNone, -1)
				}
			}
		}
	}
	return p.drops(p.castles(out))
}

func (p *pos) pawnMoves(from int, out []move) []move {
	side := p.side
	fwd := forward(side)
	last, start := 7, 1
	if side == seatBlack {
		last, start = 0, 6
	}
	push := func(to, capture int) {
		if to%64/8 == last {
			for _, k := range p.v.promotions() {
				out = p.add(out, from, to, kPawn, k, capture)
			}
			return
		}
		out = p.add(out, from, to, kPawn, kNone, capture)
	}
	if to, ok := step(from, 0, fwd); ok && p.b[to].kind == kNone {
		push(to, -1)
		if to2, ok := step(from, 0, 2*fwd); ok && from%64/8 == start && p.b[to2].kind == kNone {
			push(to2, -1)
		}
	}
	for _, df := range [2]int{-1, 1} {
		to, ok := step(from, df, fwd)
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

// castles adds the castling moves, Chess960's included: the king goes to
// the g or c file and the rook beside it, on the f or d file. Both must
// still have their right, every square between their starts and their ends
// must be empty but for the two of them, and the king may not be in check
// nor cross or land on an attacked square. In Alice both then go to the
// other board, where their squares must be empty.
func (p *pos) castles(out []move) []move {
	side := p.side
	if p.v == vAntichess || p.rook[side] == [2]int{-1, -1} {
		return out
	}
	home := 0
	if side == seatBlack {
		home = 56
	}
	k := -1
	for g := home; g < home+8; g++ {
		if p.b[g] == (pc{kKing, side}) {
			k = g
		}
	}
	if k < 0 || p.inCheck(side) {
		return out
	}
	// The squares the king crosses are judged without it on its square.
	bare := *p
	bare.b[k] = pc{}
	for wing, r := range p.rook[side] {
		if r < 0 || p.b[r] != (pc{kRook, side}) {
			continue
		}
		kd, rd := home+6, home+5
		if wing == 1 {
			kd, rd = home+2, home+3
		}
		lo, hi := min(k, kd, r, rd), max(k, kd, r, rd)
		ok := true
		for g := lo; g <= hi && ok; g++ {
			ok = g == k || g == r || p.b[g].kind == kNone
		}
		dir := 1
		if kd < k {
			dir = -1
		}
		for g := k; ok && g != kd; {
			g += dir
			ok = bare.safeAt(g, side)
		}
		to, rookTo := kd, rd
		if p.v == vAlice {
			to, rookTo = kd^64, rd^64
			ok = ok && p.b[to].kind == kNone && p.b[rookTo].kind == kNone
		}
		if ok {
			out = append(out, move{from: k, to: to, land: kd, kind: kKing, capture: -1, castle: true, rookFrom: r, rookTo: rookTo})
		}
	}
	return out
}

// drops adds the Crazyhouse drops: a piece from the hand onto any empty
// square, a pawn not on the first or last rank.
func (p *pos) drops(out []move) []move {
	if p.v != vCrazyhouse {
		return out
	}
	for k := kPawn; k <= kQueen; k++ {
		if p.hand[p.side][k] == 0 {
			continue
		}
		for g := 0; g < 64; g++ {
			if p.b[g].kind != kNone || (k == kPawn && (g/8 == 0 || g/8 == 7)) {
				continue
			}
			out = append(out, move{from: -1, to: g, land: g, kind: k, capture: -1, drop: true, rookFrom: -1, rookTo: -1})
		}
	}
	return out
}

// play returns the position after m: pieces moved, taken (to the hand in
// Crazyhouse) or blown up (Atomic), rights and the en passant square
// updated, the other side to move.
func (p *pos) play(m move) *pos {
	n := *p
	side := p.side
	if m.drop {
		n.b[m.to], n.promoted[m.to] = pc{m.kind, side}, false
		n.hand[side][m.kind]--
	} else {
		q := n.b[m.from]
		n.b[m.from] = pc{}
		wasPawn := n.promoted[m.from]
		n.promoted[m.from] = false
		if m.capture >= 0 {
			if p.v == vCrazyhouse {
				k := n.b[m.capture].kind
				if n.promoted[m.capture] {
					k = kPawn
				}
				n.hand[side][k]++
			}
			n.b[m.capture], n.promoted[m.capture] = pc{}, false
		}
		if m.castle {
			r := n.b[m.rookFrom]
			n.b[m.rookFrom] = pc{}
			n.b[m.to], n.b[m.rookTo] = q, r
		} else {
			if m.promo != kNone {
				q.kind = m.promo
			}
			n.b[m.to], n.promoted[m.to] = q, wasPawn || m.promo != kNone
		}
		if p.v == vAtomic && m.capture >= 0 {
			// The capture explodes: the piece that took, and every piece but a
			// pawn on the squares around.
			n.b[m.to], n.promoted[m.to] = pc{}, false
			for _, s := range kingSteps {
				if c, ok := step(m.to, s[0], s[1]); ok && n.b[c].kind != kPawn {
					n.b[c] = pc{}
				}
			}
		}
		if q.kind == kKing {
			n.rook[side] = [2]int{-1, -1}
		}
	}
	// A right goes with its rook: moved, taken or blown up.
	for s := uint8(1); s <= 2; s++ {
		for w, r := range n.rook[s] {
			if r >= 0 && n.b[r] != (pc{kRook, s}) {
				n.rook[s][w] = -1
			}
		}
	}
	n.ep = -1
	if m.kind == kPawn && !m.drop && (m.land-m.from == 16 || m.from-m.land == 16) {
		n.ep = (m.from + m.land) / 2
		if p.v == vAlice {
			n.ep ^= 64
		}
	}
	n.side = other(side)
	return &n
}

// legal lists the moves of the side to move, in a stable order. A move may
// not leave its own king in check; in Atomic it may not blow it up, and
// blowing up the other king is legal whatever else it does; in Alice a move
// must also be legal on its own board before the piece crosses; in
// Antichess the king is an ordinary piece, and a capture, if any can be
// made, must be.
func (p *pos) legal() []move {
	pseudo := p.pseudo()
	var out []move
	if p.v == vAntichess {
		capture := false
		for _, m := range pseudo {
			capture = capture || m.capture >= 0
		}
		for _, m := range pseudo {
			if !capture || m.capture >= 0 {
				out = append(out, m)
			}
		}
		return out
	}
	for _, m := range pseudo {
		n := p.play(m)
		if p.v == vAtomic {
			if n.king(p.side) < 0 {
				continue
			}
			if n.king(other(p.side)) < 0 {
				out = append(out, m)
				continue
			}
		}
		if p.v == vAlice {
			here := m
			here.to = m.land
			if m.castle {
				here.rookTo = m.rookTo ^ 64
			}
			h := p.play(here)
			if k := h.king(p.side); k >= 0 && k&64 == m.land&64 && !h.safeAt(k, p.side) {
				continue
			}
		}
		if !n.inCheck(p.side) {
			out = append(out, m)
		}
	}
	return out
}

// insufficient reports a position where neither side can ever win: in
// chess and Chess960, kings alone, or with one minor piece, or with bishops
// all on squares of one color; in Atomic and Alice, kings alone. Drops
// keep Crazyhouse alive, and Antichess is won by losing everything.
func (p *pos) insufficient() bool {
	switch p.v {
	case vCrazyhouse, vAntichess:
		return false
	case vAtomic, vAlice:
		for _, q := range p.b {
			if q.kind != kNone && q.kind != kKing {
				return false
			}
		}
		return true
	}
	minors, knights := 0, 0
	bishopColors := [2]bool{}
	for g, q := range p.b {
		switch q.kind {
		case kPawn, kRook, kQueen:
			return false
		case kKnight:
			minors++
			knights++
		case kBishop:
			minors++
			bishopColors[(g/8+g%8)%2] = true
		}
	}
	if minors <= 1 {
		return true
	}
	return knights == 0 && (!bishopColors[0] || !bishopColors[1])
}

// key hashes what makes two positions the same for repetition: the pieces,
// the side to move, the castling rights, an en passant capture that can
// really be made (legal are the moves of the side to move), and in
// Crazyhouse the hands and which pieces were pawns.
func (p *pos) key(legal []move) string {
	h := fnv.New64a()
	var b []byte
	for g, q := range p.b {
		v := q.kind<<2 | q.side
		if p.promoted[g] {
			v |= 0x80
		}
		b = append(b, v)
	}
	b = append(b, p.side)
	for s := 1; s <= 2; s++ {
		for _, r := range p.rook[s] {
			b = append(b, byte(r))
		}
	}
	ep := byte(0xff)
	for _, m := range legal {
		if m.kind == kPawn && !m.drop && m.land == p.ep {
			ep = byte(p.ep)
			break
		}
	}
	b = append(b, ep)
	if p.v == vCrazyhouse {
		for s := 1; s <= 2; s++ {
			for k := kPawn; k <= kQueen; k++ {
				b = append(b, byte(p.hand[s][k]))
			}
		}
	}
	_, _ = h.Write(b)
	return strconv.FormatUint(h.Sum64(), 36)
}

// selection is m as clients see it. Its square is the one the piece moves
// to on its own board; in Alice it then crosses to the twin square (ADR
// 0146).
func (m move) selection() state.Selection {
	sel := state.Selection{Action: actMove, Piece: kindName[m.kind], To: board.PositionID(cellName(m.land))}
	switch {
	case m.drop:
		sel.Action = actDrop
		return sel
	case m.castle:
		sel.Action = actCastle
		sel.Aim = board.PositionID(cellName(m.rookFrom))
	case m.promo != kNone:
		sel.Action = kindName[m.promo]
	}
	sel.From = board.PositionID(cellName(m.from))
	if m.capture >= 0 {
		sel.Aim = board.PositionID(cellName(m.capture))
	}
	return sel
}

// matches reports whether a parsed selection names m. A selection with both
// squares (from the list of legal moves, long notation or UCI) is matched by
// them and the promotion: the square reached is the one moved to on the
// piece's own board, or the twin where it ends (Alice's older records), or
// for castling the rook's. One from
// short notation (Nf3, exd5, O-O, N@f3) is matched by what it gives: the
// piece, the square it goes to and any file or rank it names.
func matches(s state.Selection, m move) bool {
	to, land := cellName(m.to), cellName(m.land)
	switch {
	case s.Action == actDrop || m.drop:
		return s.Action == actDrop && m.drop && kindOf(s.Piece) == m.kind && string(s.To) == to
	case s.Action == actCastle:
		return m.castle && len(s.To) > 0 && fold(s.To[0]) == fold(land[0])
	case promoOf(s.Action) != m.promo:
		return false
	}
	from := cellName(m.from)
	if len(s.From) == 2 {
		if string(s.From) != from {
			return false
		}
		if m.castle {
			// Squares name a castling by the king taking its rook, or by the
			// king going two files or more (e1g1): a Chess960 king one file
			// from its square may step there too, and that is the step.
			apart := int(from[0]) - int(land[0])
			return string(s.To) == cellName(m.rookFrom) || ((string(s.To) == to || string(s.To) == land) && (apart >= 2 || apart <= -2))
		}
		return string(s.To) == to || string(s.To) == land
	}
	if m.castle || len(s.To) != 2 || fold(s.To[0]) != fold(land[0]) || s.To[1] != land[1] || kindOf(s.Piece) != m.kind {
		return false
	}
	for i := 0; i < len(s.From); i++ {
		if c := s.From[i]; fold(c) != fold(from[0]) && c != from[1] {
			return false
		}
	}
	return true
}

// --- the ruleset interface ---------------------------------------------

// backRank is the pieces of White's first rank: the usual one, or for
// Chess960 the one numbered seed mod 960 (518 is the usual one).
func backRank(v variant, seed uint64) [8]uint8 {
	if v != v960 {
		return [8]uint8{kRook, kKnight, kBishop, kQueen, kKing, kBishop, kKnight, kRook}
	}
	n := int(seed % 960)
	var r [8]uint8
	r[2*(n%4)+1] = kBishop
	n /= 4
	r[2*(n%4)] = kBishop
	n /= 4
	place := func(i int, k uint8) {
		for f := range r {
			if r[f] == kNone {
				if i == 0 {
					r[f] = k
					return
				}
				i--
			}
		}
	}
	place(n%6, kQueen)
	n /= 6
	knights := [10][2]int{{0, 1}, {0, 2}, {0, 3}, {0, 4}, {1, 2}, {1, 3}, {1, 4}, {2, 3}, {2, 4}, {3, 4}}[n]
	place(knights[1], kKnight)
	place(knights[0], kKnight)
	place(0, kRook)
	place(0, kKing)
	place(0, kRook)
	return r
}

// Start returns the opening position: White moves first.
func (rs *Ruleset) Start(game *catalog.CompiledGame, seed uint64) (*state.BoardState, error) {
	v := variantOf(game)
	p := &pos{v: v, side: seatWhite, ep: -1, rook: noRights()}
	for f, k := range backRank(v, seed) {
		p.b[f] = pc{k, seatWhite}
		p.b[8+f] = pc{kPawn, seatWhite}
		p.b[48+f] = pc{kPawn, seatBlack}
		p.b[56+f] = pc{k, seatBlack}
	}
	if v != vAntichess {
		p.rook, _ = p.parseCastling("KQkq")
	}
	st := state.NewBoardState()
	p.save(st)
	st.SetExt(&Data{Turn: seatWhite, Castling: p.castling(), Fullmove: 1, Seen: []string{p.key(p.legal())}})
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
		return "neither side has the pieces left to win"
	case "king-exploded":
		return "the king was blown up"
	case "no-pieces":
		return "the winner has lost all their pieces"
	case "no-moves":
		return "the winner has no legal move"
	}
	return reason
}

// Validate checks a position document: known pieces on the board's squares,
// one king each (any number in Antichess), no pawn on the first or last
// rank, castling rights that name rooks, a sensible en passant square and
// hand, and the side that just moved not left in check.
func (rs *Ruleset) Validate(game *catalog.CompiledGame, st *state.BoardState) error {
	d, err := dataOf(st)
	if err != nil {
		return err
	}
	v := variantOf(game)
	if d.Turn < 0 || d.Turn > 2 {
		return fmt.Errorf("chess: bad turn %d", d.Turn)
	}
	for _, id := range st.Occupied() {
		occ, _ := st.Get(id)
		g, ok := cellOf(string(id))
		switch {
		case !ok || !game.Board.HasPosition(id):
			return fmt.Errorf("chess: no square %s", id)
		case kindOf(occ.Piece) == kNone || sideOf(occ.Color) == 0:
			return fmt.Errorf("chess: unknown piece %s/%s at %s", occ.Color, occ.Piece, id)
		case occ.Piece == "pawn" && (g%64/8 == 0 || g%64/8 == 7):
			return fmt.Errorf("chess: a pawn cannot stand on %s", id)
		}
	}
	turn := d.Turn
	if turn == 0 {
		turn = seatWhite
	}
	p := decode(st, &Data{Turn: turn, EnPassant: d.EnPassant, Promoted: d.Promoted}, v)
	if v != vAntichess {
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
	}
	if v == vAntichess && d.Castling != "" {
		return fmt.Errorf("chess: there is no castling in Antichess")
	}
	if _, err := p.parseCastling(d.Castling); err != nil {
		return err
	}
	for side := uint8(1); side <= 2; side++ {
		for k := kPawn; k <= kQueen; k++ {
			if p.hand[side][k] < 0 {
				return fmt.Errorf("chess: %s holds %d %ss", sideColor[side], p.hand[side][k], kindName[k])
			}
		}
	}
	if d.EnPassant != "" {
		g, ok := cellOf(d.EnPassant)
		if want := map[int]int{seatWhite: 5, seatBlack: 2}[turn]; !ok || g%64/8 != want || !game.Board.HasPosition(board.PositionID(d.EnPassant)) {
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
	moves := decode(st, d, variantOf(game)).legal()
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
		return nil, fmt.Errorf("chess: it is %s's move", sideColor[sideOfSeat(d.Turn)])
	}
	v := variantOf(game)
	p := decode(st, d, v)
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
	nd.Castling = np.castling()
	nd.EnPassant = ""
	if np.ep >= 0 {
		nd.EnPassant = cellName(np.ep)
	}
	nd.Promoted = nil
	if v == vCrazyhouse {
		nd.Promoted = np.promotedSquares()
	}
	nd.Halfmove++
	if m.kind == kPawn || m.capture >= 0 {
		nd.Halfmove = 0
	}
	if seat == seatBlack {
		nd.Fullmove++
	}
	// A position after a capture, a pawn move or a lost right can never come
	// back, so the earlier ones need not be kept; in Crazyhouse a piece taken
	// can be dropped back, so only a lost right is for good.
	if (v != vCrazyhouse && nd.Halfmove == 0) || np.rook != p.rook {
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
	case v == vAtomic && np.king(np.side) < 0:
		nd.Turn, nd.Winner, nd.Reason = 0, seat, "king-exploded"
	case v == vAntichess && len(replies) == 0:
		nd.Turn, nd.Winner, nd.Reason = 0, int(np.side), "no-moves"
		if np.count(np.side) == 0 {
			nd.Reason = "no-pieces"
		}
	case len(replies) == 0 && np.inCheck(np.side):
		nd.Turn, nd.Winner, nd.Reason = 0, seat, "checkmate"
	case len(replies) == 0:
		nd.Turn, nd.Reason = 0, "stalemate"
	case np.insufficient():
		nd.Turn, nd.Reason = 0, "insufficient-material"
	case seen >= 3:
		nd.Turn, nd.Reason = 0, "repetition"
	case v != vCrazyhouse && nd.Halfmove >= 100:
		nd.Turn, nd.Reason = 0, "fifty-moves"
	}

	ns := st.Clone()
	np.save(ns)
	ns.SetExt(nd)
	ns.SetPly(st.Ply() + 1)
	sel := m.selection()
	res := &state.MoveResult{NewState: ns, Move: sel, Notation: rs.Format(game, sel), NextImpulseSeat: nd.Turn}
	if m.capture >= 0 {
		taken := []int{m.capture}
		if v == vAtomic {
			// The piece that took and the pieces around go up with it.
			taken = append(taken, m.from)
			for g, q := range p.b {
				if q.kind != kNone && g != m.from && g != m.capture && np.b[g].kind == kNone && adjacent(g, m.to) {
					taken = append(taken, g)
				}
			}
		}
		for _, g := range taken {
			res.Captures = append(res.Captures, board.PositionID(cellName(g)))
		}
		res.Captured = &res.Captures[0]
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
