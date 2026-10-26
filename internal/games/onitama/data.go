package onitama

import (
	"encoding/json"
	"fmt"

	"github.com/noderunner/bingle/internal/engine/state"
)

// Seat 1 is Red, whose pieces start on rank 1 and move up the board; seat 2
// is Blue, from rank 5 down. The first player is decided by the stamp of the
// card dealt to the side.
const (
	seatRed  = 1
	seatBlue = 2
)

var seatColor = [3]string{"", "red", "blue"}

// Data is the game-specific part of a position; the pieces are on the board.
type Data struct {
	// Turn is the seat to move (0 once the game is over).
	Turn int `json:"turn"`
	// Hands holds each seat's two cards (index seat-1); Side is the card at
	// the side of the mat, which the mover takes after moving.
	Hands [2][2]string `json:"hands"`
	Side  string       `json:"side"`
	// Last move, for display.
	LastCard string `json:"last_card,omitempty"`
	LastFrom string `json:"last_from,omitempty"`
	LastTo   string `json:"last_to,omitempty"`
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
	c.Events = append([]string(nil), d.Events...)
	return &c
}

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
			return nil, fmt.Errorf("onitama: bad position payload: %w", err)
		}
		return d, nil
	case nil:
		return nil, fmt.Errorf("onitama: position has no game payload")
	default:
		return nil, fmt.Errorf("onitama: position payload has type %T", v)
	}
}
