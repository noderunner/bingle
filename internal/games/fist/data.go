package fist

import (
	"encoding/json"
	"fmt"

	"github.com/noderunner/bingle/internal/engine/state"
)

// Seats: the challenger plays white and goes first, the king's side black.
const (
	seatChallenger = 1
	seatKing       = 2
)

// Piece kinds (the keys of "pieces" in fist.json).
const (
	kNone uint8 = iota
	kYellow
	kBlue
	kKing
	kAce
	kMage
	kPawn
)

var kindName = [...]string{"", "yellow", "blue", "king", "ace", "mage", "pawn"}

var kindLabel = [...]string{"", "yellow guard", "blue guard", "king", "ace", "mage", "verdigris pawn"}

func kindOf(name string) uint8 {
	for k := kYellow; k <= kPawn; k++ {
		if kindName[k] == name {
			return k
		}
	}
	return kNone
}

// Colours: the pawn is green, belonging to neither seat.
var sideColor = [3]string{"green", "white", "black"}

func sideOf(color string) (uint8, bool) {
	for s, c := range sideColor {
		if c == color {
			return uint8(s), true
		}
	}
	return 0, false
}

// Base value of each kind, and how many of each a side owns.
var baseValue = [...]int8{0, 1, 2, 4, 3, 3, 4}

// total[side][kind] is how many pieces a side starts with.
var total = [3][7]int{
	1: {kYellow: 6, kBlue: 3, kAce: 1, kMage: 1},
	2: {kYellow: 12, kBlue: 7, kKing: 1},
	0: {kPawn: 1},
}

// Data is the game-specific part of a Fist position; the pieces themselves
// are on the shared board.
type Data struct {
	// Turn is the seat to move (0 once the game is over).
	Turn int `json:"turn"`
	// PawnFrom is the square the verdigris pawn left on the previous turn: it
	// may not go straight back there.
	PawnFrom string `json:"pawn_from,omitempty"`
	// KingEmpower says the king's side captured the mage, so the king now
	// empowers its allies.
	KingEmpower bool `json:"king_empower,omitempty"`
	// Last move, for display: where it began and ended, what it took.
	LastFrom string   `json:"last_from,omitempty"`
	LastTo   string   `json:"last_to,omitempty"`
	LastTook []string `json:"last_took,omitempty"`
	// Result of a finished game.
	Winner int    `json:"winner,omitempty"`
	Reason string `json:"reason,omitempty"`
	// Events narrates what the last move did.
	Events []string `json:"events,omitempty"`
}

// CloneExt implements state.ExtClone.
func (d *Data) CloneExt() any { return d.clone() }

func (d *Data) clone() *Data {
	c := *d
	c.LastTook = append([]string(nil), d.LastTook...)
	c.Events = append([]string(nil), d.Events...)
	return &c
}

// publicExt is how a position serialises: the data with the derived view.
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
			return nil, fmt.Errorf("fist: bad position payload: %w", err)
		}
		return d, nil
	case nil:
		return nil, fmt.Errorf("fist: position has no game payload")
	default:
		return nil, fmt.Errorf("fist: position payload has type %T", v)
	}
}
