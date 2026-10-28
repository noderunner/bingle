package meridians

import (
	"encoding/json"
	"fmt"

	"github.com/noderunner/bingle/internal/engine/state"
)

// Data is the game-specific part of a position: whose turn it is, how many
// turns each player has taken (the opening rules and the removal of dead
// groups both depend on it) and how the game ended. The stones themselves are
// on the board.
type Data struct {
	// Turn is the seat to move, 0 once the game is over.
	Turn int `json:"turn"`
	// Turns counts the turns each seat has taken (index seat-1).
	Turns [2]int `json:"turns"`
	// First is each seat's first stone, while that matters: a player's second
	// stone may not touch it.
	First [2]string `json:"first,omitempty"`
	// Last is where the last stone was laid, for the page to mark.
	Last string `json:"last,omitempty"`
	// Passes counts the turns passed in a row: two in a row and nobody can
	// move, which ends the game.
	Passes int `json:"passes,omitempty"`

	Winner int    `json:"winner,omitempty"`
	Reason string `json:"reason,omitempty"`
	// Events narrates what the last move did.
	Events []string `json:"events,omitempty"`
}

// CloneExt implements state.ExtClone.
func (d *Data) CloneExt() any { return d.clone() }

// publicExt is a position with the derived view attached, as clients get it.
type publicExt struct {
	*Data
	View *View `json:"view"`
}

// CloneExt implements state.ExtClone for positions with a view.
func (p *publicExt) CloneExt() any { return &publicExt{Data: p.clone(), View: p.View} }

func (d *Data) clone() *Data {
	c := *d
	c.Events = append([]string(nil), d.Events...)
	return &c
}

// dataOf reads the payload of a position.
func dataOf(st *state.BoardState) (*Data, error) {
	switch v := st.Ext().(type) {
	case *Data:
		return v, nil
	case *publicExt:
		return v.Data, nil
	case json.RawMessage:
		d := &Data{}
		if err := json.Unmarshal(v, d); err != nil {
			return nil, fmt.Errorf("meridians: %w", err)
		}
		return d, nil
	case map[string]any:
		raw, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("meridians: %w", err)
		}
		d := &Data{}
		if err := json.Unmarshal(raw, d); err != nil {
			return nil, fmt.Errorf("meridians: %w", err)
		}
		return d, nil
	case nil:
		return nil, fmt.Errorf("meridians: the position carries no game data")
	default:
		return nil, fmt.Errorf("meridians: the position payload is %T", st.Ext())
	}
}

// seatLabel names a seat the way the rules do.
func seatLabel(seat int) string {
	if seat == 1 {
		return "Light"
	}
	return "Dark"
}

// seatColor is the colour of a seat's stones, as the board records them.
func seatColor(seat int) string {
	if seat == 1 {
		return "white"
	}
	return "black"
}

// seatOf is the seat that owns stones of a colour (0 for neither).
func seatOf(color string) int {
	switch color {
	case "white":
		return 1
	case "black":
		return 2
	}
	return 0
}

func other(seat int) int {
	if seat == 1 {
		return 2
	}
	return 1
}
