package chess

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

// A move is written in long algebraic notation, which needs no position to
// read (ADR 0144): the piece's letter (none for a pawn), the square it
// leaves, - or x for a capture, the square it reaches and = with the piece
// a pawn becomes. Castling is O-O or O-O-O, and a Crazyhouse drop the
// piece's letter, @ and the square (ADR 0145).
//
//	e2-e4  Ng1-f3  e4xd5  e5xd6 (en passant)  e7-e8=Q  O-O  N@f3
//
// Alice's second board is files i to p, and a move names the square the
// piece moves to on its own board, as in chess (ADR 0146): Ng1-f3 ends on
// n3, the twin of f3 on the other board. The older form, naming the twin
// (Ng1-n3), is read too.
//
// Parse also reads short algebraic notation (e4, Nf3, exd5, Nbd7, e8=Q, with
// or without + and #) and UCI (e2e4, e7e8q); Apply finds the one legal move
// it names, and the game records that in the long form.

// Format renders mv.
func (rs *Ruleset) Format(game *catalog.CompiledGame, mv state.Selection) string {
	switch mv.Action {
	case actCastle:
		if len(mv.To) > 0 && fold(mv.To[0]) == 'c' {
			return "O-O-O"
		}
		return "O-O"
	case actDrop:
		letter := "P"
		if k := kindOf(mv.Piece); k > kPawn {
			letter = kindLetter[k]
		}
		return letter + "@" + string(mv.To)
	}
	var b strings.Builder
	if k := kindOf(mv.Piece); k != kNone {
		b.WriteString(kindLetter[k])
	}
	b.WriteString(string(mv.From))
	switch {
	case mv.Aim != "":
		b.WriteByte('x')
	case len(mv.From) == 2:
		b.WriteByte('-')
	}
	b.WriteString(string(mv.To))
	if k := promoOf(mv.Action); k != kNone {
		b.WriteString("=" + kindLetter[k])
	}
	return b.String()
}

var (
	longForm  = regexp.MustCompile(`^([KQRBNP]?)([a-p][1-8])([-x])([a-p][1-8])(?:=?([KQRBNkqrbn]))?$`)
	uciForm   = regexp.MustCompile(`^([a-p][1-8])([a-p][1-8])([kqrbnKQRBN]?)$`)
	shortForm = regexp.MustCompile(`^([KQRBNP]?)([a-p]?[1-8]?)(x?)([a-p][1-8])(?:=?([KQRBNkqrbn]))?$`)
	dropForm  = regexp.MustCompile(`^([QRBNP]?)@([a-h][1-8])$`)
)

// Parse reads a move in long or short algebraic notation, or UCI.
func (rs *Ruleset) Parse(game *catalog.CompiledGame, text string) (state.Selection, error) {
	t := strings.TrimSpace(text)
	t = strings.TrimSpace(strings.TrimSuffix(t, "e.p."))
	t = strings.TrimRight(t, "+#!?")
	switch strings.ReplaceAll(strings.ToUpper(t), "0", "O") {
	case "O-O":
		return state.Selection{Action: actCastle, Piece: "king", To: "g"}, nil
	case "O-O-O":
		return state.Selection{Action: actCastle, Piece: "king", To: "c"}, nil
	}
	piece := func(letter string) string {
		if letter == "" || letter == "P" {
			return kindName[kPawn]
		}
		return kindName[kindOfLetter(letter[0])]
	}
	action := func(promo string) string {
		if promo == "" {
			return actMove
		}
		return kindName[kindOfLetter(strings.ToUpper(promo)[0])]
	}
	if m := dropForm.FindStringSubmatch(t); m != nil {
		return state.Selection{Action: actDrop, Piece: piece(m[1]), To: board.PositionID(m[2])}, nil
	}
	if m := longForm.FindStringSubmatch(t); m != nil {
		mv := state.Selection{Action: action(m[5]), Piece: piece(m[1]), From: board.PositionID(m[2]), To: board.PositionID(m[4])}
		if m[3] == "x" {
			mv.Aim = mv.To
		}
		return mv, nil
	}
	if m := uciForm.FindStringSubmatch(t); m != nil {
		return state.Selection{Action: action(m[3]), From: board.PositionID(m[1]), To: board.PositionID(m[2])}, nil
	}
	if m := shortForm.FindStringSubmatch(t); m != nil {
		mv := state.Selection{Action: action(m[5]), Piece: piece(m[1]), From: board.PositionID(m[2]), To: board.PositionID(m[4])}
		if m[3] == "x" {
			mv.Aim = mv.To
		}
		return mv, nil
	}
	return state.Selection{}, fmt.Errorf("chess: cannot read %q as a move (try e2-e4, Nf3 or O-O)", text)
}

// Gestures decomposes a move into clicks: the piece, the square it goes to
// and, for a promotion, the piece the pawn becomes. Castling is the king and
// its square, or in Chess960, where the king may not move at all, the king
// and the rook. A drop is the piece in hand and its square.
func (rs *Ruleset) Gestures(game *catalog.CompiledGame, mv state.Selection) []state.Gesture {
	pos := func(id board.PositionID) state.Gesture {
		return state.Gesture{Kind: state.GesturePos, Value: string(id)}
	}
	switch {
	case mv.Action == actDrop:
		return []state.Gesture{{Kind: state.GesturePiece, Value: mv.Piece}, pos(mv.To)}
	case mv.Action == actCastle && variantOf(game) == v960:
		return []state.Gesture{pos(mv.From), pos(mv.Aim)}
	}
	out := []state.Gesture{pos(mv.From), pos(mv.To)}
	if promoOf(mv.Action) != kNone {
		out = append(out, state.Gesture{Kind: state.GestureAction, Value: mv.Action})
	}
	return out
}

// Describe labels the decisions for clients.
func (rs *Ruleset) Describe(game *catalog.CompiledGame) state.RulesetInfo {
	v := variantOf(game)
	info := state.RulesetInfo{
		WinRules: []string{
			"Checkmate the other king to win.",
			"The game is drawn by stalemate, by the same position three times, by fifty moves each without a capture or a pawn move, and when neither side has the pieces left to checkmate.",
		},
		NotationHelp: []string{
			"A move is the piece's letter (none for a pawn), the square it leaves, - or x for a capture, and the square it reaches: e2-e4, Ng1-f3, e4xd5.",
			"A pawn reaching the last rank names what it becomes: e7-e8=Q. Castling is O-O (king's side) or O-O-O (queen's side).",
			"Short algebraic notation (e4, Nf3, exd5, e8=Q) and UCI (e2e4, e7e8q) are read too.",
			"Pieces: K king, Q queen, R rook, B bishop, N knight.",
		},
	}
	switch v {
	case v960:
		info.NotationHelp = append(info.NotationHelp, "To castle, click the king and then the rook.")
	case vCrazyhouse:
		info.WinRules[1] = "The game is drawn by stalemate and by the same position three times (the hands included)."
		info.NotationHelp = append(info.NotationHelp, "A drop is the piece's letter, @ and the square: N@f3, P@e4.")
	case vAntichess:
		info.WinRules = []string{
			"Lose all your pieces, or be left with no legal move, to win. A capture, if any can be made, must be.",
			"The king is an ordinary piece: there is no check and no castling. The game is drawn by the same position three times and by fifty moves each without a capture or a pawn move.",
		}
	case vAtomic:
		info.WinRules = []string{
			"Blow up the other king, or checkmate it. A capture explodes: the piece taken, the piece taking and every piece but a pawn on the squares around are removed.",
			"A king may not capture, nor blow up its own king. The game is drawn by stalemate, by the same position three times, by fifty moves each without a capture or a pawn move, and when only the kings are left.",
		}
	case vAlice:
		info.WinRules[1] = "The game is drawn by stalemate, by the same position three times, by fifty moves each without a capture or a pawn move, and when only the kings are left."
		info.NotationHelp = append(info.NotationHelp, "The second board is files i to p. A move is made on the piece's own board, as in chess (Ng1-f3), and the piece then crosses to the twin square of the other board (n3).")
	}
	if st, err := rs.Start(game, 518); err == nil {
		if moves := rs.Legal(game, st, seatWhite); len(moves) > 0 {
			info.NotationHelp = append(info.NotationHelp, "Example of a legal first move: '"+rs.Format(game, moves[0])+"'.")
		}
	}
	for _, k := range v.promotions() {
		info.Actions = append(info.Actions, state.RulesetAction{
			ID: kindName[k], Label: strings.ToUpper(kindName[k][:1]) + kindName[k][1:], Hint: "The pawn becomes a " + kindName[k] + ".",
		})
	}
	return info
}
