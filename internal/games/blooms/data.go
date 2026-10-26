package blooms

import (
	"encoding/json"
	"fmt"

	"github.com/noderunner/bingle/internal/engine/state"
)

// Player 1 has Red and Yellow, player 2 Blue and Black.
var seatColors = [3][2]string{{}, {"red", "yellow"}, {"blue", "black"}}

// ownerOf is the seat that owns stones of a colour (0 for none).
func ownerOf(color string) int {
	for s := 1; s <= 2; s++ {
		for _, c := range seatColors[s] {
			if c == color {
				return s
			}
		}
	}
	return 0
}

// Stages of a turn.
const (
	stageFirst  = ""       // the first stone of a turn
	stageSecond = "second" // a second stone of the other colour, or the end of the turn
)

// Data is the game-specific part of a position; the stones are on the board.
type Data struct {
	// Turn is the seat to move (0 once the game is over).
	Turn  int    `json:"turn"`
	Stage string `json:"stage,omitempty"`
	// First is the colour of the first stone placed this turn.
	First string `json:"first,omitempty"`
	// Turns counts completed turns: Player 1's first turn is one stone only.
	Turns int `json:"turns"`
	// Captured counts the enemy stones each seat has captured (index seat-1).
	Captured [2]int `json:"captured"`
	// Placed lists the cells stoned this turn, and Last the previous turn's.
	Placed []string `json:"placed,omitempty"`
	Last   []string `json:"last_placed,omitempty"`
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
	c.Placed = append([]string(nil), d.Placed...)
	c.Last = append([]string(nil), d.Last...)
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
			return nil, fmt.Errorf("blooms: bad position payload: %w", err)
		}
		return d, nil
	case nil:
		return nil, fmt.Errorf("blooms: position has no game payload")
	default:
		return nil, fmt.Errorf("blooms: position payload has type %T", v)
	}
}
