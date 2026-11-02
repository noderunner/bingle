package solitaire

import (
	"encoding/json"
	"fmt"

	"github.com/noderunner/bingle/internal/engine/cards"
	"github.com/noderunner/bingle/internal/engine/state"
)

// The games, by the param "mode" of the definition.
const (
	modeKlondike = "klondike"
	modeSpider   = "spider"
	modeFreeCell = "freecell"
	modeBowling  = "bowling"
)

// Why a game ended.
const (
	reasonCleared = "cleared" // every card is home
	reasonStuck   = "stuck"   // no move is left
	reasonScore   = "score"   // ten frames of bowling are done
)

// The one seat.
const player = 1

// The piles that are not numbered.
const (
	zoneStock = "stock"
	zoneWaste = "waste"
	zoneDone  = "done" // Spider's finished suits
	zoneGone  = "gone" // Bowling's cards out of play, there to be remembered
)

// Data is the game-specific part of a position. The cards are in Table.
type Data struct {
	Mode string `json:"mode"`
	// Turn is the seat that decides, 0 once the game is over.
	Turn int `json:"turn"`
	// Won says every card came home; Reason says why the game ended.
	Won    bool   `json:"won,omitempty"`
	Reason string `json:"reason,omitempty"`
	// RNG is the shuffling generator's state: secret in public views.
	RNG uint64 `json:"rng,omitempty"`
	// Table holds every card.
	Table *cards.Table `json:"table,omitempty"`
	// Bowling: the frame (from 1), the ball of it (from 1), what this ball has
	// knocked down so far, the pins of each ball of each frame, and whether no
	// play has been made yet on this rack of pins.
	Frame   int     `json:"frame,omitempty"`
	Ball    int     `json:"ball,omitempty"`
	Knocked int     `json:"knocked,omitempty"`
	Rolls   [][]int `json:"rolls,omitempty"`
	Fresh   bool    `json:"fresh,omitempty"`
	// Events are what the last move did, for the log.
	Events []string `json:"events,omitempty"`
}

func (d *Data) clone() *Data {
	c := *d
	if d.Table != nil {
		c.Table = d.Table.Clone()
	}
	c.Rolls = make([][]int, len(d.Rolls))
	for i, r := range d.Rolls {
		c.Rolls[i] = append([]int(nil), r...)
	}
	c.Events = nil
	return &c
}

// CloneExt implements state.ExtClone.
func (d *Data) CloneExt() any { return d.clone() }

// publicExt is what leaves the server: the position without its secrets, and the view.
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
		p := &publicExt{Data: d}
		if err := json.Unmarshal(v, p); err != nil {
			return nil, fmt.Errorf("solitaire: bad position payload: %w", err)
		}
		// A public position (what a hint is worked out from) has its cards in the view.
		if d.Table == nil {
			d.Table = cards.NewTable()
			if p.View != nil {
				for _, z := range p.View.Table {
					d.Table.Zone(z.ID, 0).Cards = z.Cards
				}
			}
		}
		return d, nil
	case nil:
		return nil, fmt.Errorf("solitaire: position has no game payload")
	default:
		return nil, fmt.Errorf("solitaire: position payload has type %T", v)
	}
}
