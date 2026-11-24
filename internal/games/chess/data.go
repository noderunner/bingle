package chess

import (
	"encoding/json"
	"fmt"

	"github.com/noderunner/bingle/internal/engine/state"
)

// Seats: White moves first.
const (
	seatWhite = 1
	seatBlack = 2
)

// Piece kinds (the keys of "pieces" in chess.yaml).
const (
	kNone uint8 = iota
	kPawn
	kKnight
	kBishop
	kRook
	kQueen
	kKing
)

var kindName = [...]string{"", "pawn", "knight", "bishop", "rook", "queen", "king"}

// kindLetter is a piece's letter in the notation (a pawn has none).
var kindLetter = [...]string{"", "", "N", "B", "R", "Q", "K"}

func kindOf(name string) uint8 {
	for k := kPawn; k <= kKing; k++ {
		if kindName[k] == name {
			return k
		}
	}
	return kNone
}

func kindOfLetter(l byte) uint8 {
	for k := kKnight; k <= kKing; k++ {
		if kindLetter[k][0] == l {
			return k
		}
	}
	return kNone
}

var sideColor = [3]string{"", "white", "black"}

// sideOfSeat is the side a seat number plays, or 0 for anything but White's
// or Black's seat.
func sideOfSeat(seat int) uint8 {
	switch seat {
	case seatWhite:
		return seatWhite
	case seatBlack:
		return seatBlack
	}
	return 0
}

func sideOf(color string) uint8 {
	switch color {
	case "white":
		return seatWhite
	case "black":
		return seatBlack
	}
	return 0
}

// Data is the game-specific part of a chess position; the pieces themselves
// are on the shared board.
type Data struct {
	// Turn is the seat to move (0 once the game is over).
	Turn int `json:"turn"`
	// Castling lists the castling rights still held, as in FEN: K and Q for
	// White's king and queen sides, k and q for Black's ("" for none).
	Castling string `json:"castling,omitempty"`
	// EnPassant is the square a pawn passed over with a double step on the
	// last move, where an enemy pawn may take it.
	EnPassant string `json:"en_passant,omitempty"`
	// Halfmove counts the plies since the last capture or pawn move (the
	// fifty-move rule), and Fullmove the moves, from 1, rising after Black's.
	Halfmove int `json:"halfmove,omitempty"`
	Fullmove int `json:"fullmove"`
	// Seen holds a hash of every position since the last capture, pawn move
	// or loss of a castling right, the current one last: a position seen
	// three times is a draw.
	Seen []string `json:"seen,omitempty"`
	// The result of a finished game.
	Winner int    `json:"winner,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// CloneExt implements state.ExtClone.
func (d *Data) CloneExt() any { return d.clone() }

func (d *Data) clone() *Data {
	c := *d
	c.Seen = append([]string(nil), d.Seen...)
	return &c
}

// publicExt is how a position serialises for clients: the data with the
// derived view.
type publicExt struct {
	*Data
	View *View `json:"view"`
}

// CloneExt implements state.ExtClone for positions with a view.
func (p *publicExt) CloneExt() any { return &publicExt{Data: p.clone(), View: p.View} }

func dataOf(st *state.BoardState) (*Data, error) {
	switch v := st.Ext().(type) {
	case *Data:
		return v, nil
	case *publicExt:
		return v.Data, nil
	case json.RawMessage:
		d := &Data{}
		if err := json.Unmarshal(v, d); err != nil {
			return nil, fmt.Errorf("chess: bad position payload: %w", err)
		}
		return d, nil
	case nil:
		return nil, fmt.Errorf("chess: position has no game payload")
	default:
		return nil, fmt.Errorf("chess: position payload has type %T", v)
	}
}
