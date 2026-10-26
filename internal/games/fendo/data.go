package fendo

import (
	"encoding/json"
	"fmt"

	"github.com/noderunner/bingle/internal/engine/state"
)

// Data is the game-specific part of a position; the pieces are on the board.
type Data struct {
	// Turn is the seat to move (0 once the game is over).
	Turn int `json:"turn"`
	// Stock is the number of pieces each seat still has to add (index seat-1).
	Stock [2]int `json:"stock"`
	// Walls are the fences: each is a pair of cells that touch, with the fence
	// on the edge between them.
	Walls [][2]string `json:"walls,omitempty"`
	// Passes counts consecutive passes: two in a row end the game.
	Passes int `json:"passes,omitempty"`
	// Result of a finished game.
	Winner int    `json:"winner,omitempty"`
	Reason string `json:"reason,omitempty"`
	// Score is the territory each seat owns when the game ends.
	Score [2]int `json:"score"`
	// Events narrates what the last move did.
	Events []string `json:"events,omitempty"`
}

// CloneExt implements state.ExtClone.
func (d *Data) CloneExt() any { return d.clone() }

func (d *Data) clone() *Data {
	c := *d
	c.Walls = append([][2]string(nil), d.Walls...)
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
			return nil, fmt.Errorf("fendo: bad position payload: %w", err)
		}
		return d, nil
	case nil:
		return nil, fmt.Errorf("fendo: position has no game payload")
	default:
		return nil, fmt.Errorf("fendo: position payload has type %T", v)
	}
}
