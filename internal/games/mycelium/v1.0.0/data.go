package mycelium

import (
	"encoding/json"
	"fmt"

	"github.com/noderunner/bingle/internal/engine/state"
)

// Player 1 plays black nodes, player 2 green nodes.
var seatColors = [3]string{"", "black", "green"}

// ownerOf is the seat that owns nodes of colour c (0 for none).
func ownerOf(c string) int {
	for s := 1; s <= 2; s++ {
		if seatColors[s] == c {
			return s
		}
	}
	return 0
}

func other(seat int) int { return 3 - seat }

func seatLabel(seat int) string {
	if seat == 1 {
		return "Black"
	}
	return "Green"
}

// The two phases of a confluence.
const (
	phaseSpore  = "spore"
	phaseHyphae = "hyphae"
)

// Confluence is a confluence under way. The seat that chooses the next spread
// is Data.Turn.
type Confluence struct {
	// Initiator is the seat that took the centre.
	Initiator int `json:"initiator"`
	// Phase is "spore" or "hyphae".
	Phase string `json:"phase"`
	// Initiative is the cluster size whose spreads are being resolved.
	Initiative int `json:"initiative"`
	// Spent lists the cells of the clusters that have spread in this phase: a
	// cluster holding one of them has had its spread. A cell leaves the list
	// when it changes colour.
	Spent []string `json:"spent,omitempty"`
	// Head and Left describe a hyphae spread under way: the node converted
	// last, which the next conversion must touch, and how many more
	// conversions the strand allows.
	Head string `json:"head,omitempty"`
	Left int    `json:"left,omitempty"`
}

func (c *Confluence) clone() *Confluence {
	if c == nil {
		return nil
	}
	d := *c
	d.Spent = append([]string(nil), c.Spent...)
	return &d
}

// Data is the game-specific part of a position; the nodes are on the board.
type Data struct {
	// Turn is the seat to move (0 once the game is over); during a
	// confluence, the seat that chooses the next spread.
	Turn int `json:"turn"`
	// Last is the cell of the last placement.
	Last string `json:"last,omitempty"`
	// Confluence is set while a confluence is being resolved.
	Confluence *Confluence `json:"confluence,omitempty"`
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
	c.Confluence = d.Confluence.clone()
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
			return nil, fmt.Errorf("mycelium: bad position payload: %w", err)
		}
		return d, nil
	case nil:
		return nil, fmt.Errorf("mycelium: position has no game payload")
	default:
		return nil, fmt.Errorf("mycelium: position payload has type %T", v)
	}
}
