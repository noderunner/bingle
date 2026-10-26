package yinsh

import (
	"encoding/json"
	"fmt"

	"github.com/noderunner/bingle/internal/engine/state"
)

// Stages of the game.
const (
	stageSetup = "setup" // the players place their five rings
	stagePlay  = ""      // a turn: place a marker and move its ring
	stageRow   = "row"   // the seat to move chooses which row of five to remove
	stageRing  = "ring"  // …and then which of its rings goes with it
)

// Data is the game-specific part of a position; rings and markers are on the
// board.
type Data struct {
	// Turn is the seat that decides next (0 once the game is over). It is the
	// seat whose turn it is, except while the other seat removes its rows.
	Turn int `json:"turn"`
	// Mover is the seat whose turn it is.
	Mover int    `json:"mover"`
	Stage string `json:"stage,omitempty"`
	// Score is the number of rings each seat has removed (index seat-1).
	Score [2]int `json:"score"`
	// Last lists the points the last ring left and reached; Flipped the
	// markers it turned over.
	Last    []string `json:"last_move,omitempty"`
	Flipped []string `json:"flipped,omitempty"`
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
	c.Last = append([]string(nil), d.Last...)
	c.Flipped = append([]string(nil), d.Flipped...)
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
			return nil, fmt.Errorf("yinsh: bad position payload: %w", err)
		}
		return d, nil
	case nil:
		return nil, fmt.Errorf("yinsh: position has no game payload")
	default:
		return nil, fmt.Errorf("yinsh: position payload has type %T", v)
	}
}
