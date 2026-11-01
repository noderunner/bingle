package hive

import (
	"encoding/json"
	"fmt"

	"github.com/noderunner/bingle/internal/engine/state"
)

// Insect kinds (the keys of "pieces" in hive.yaml).
const (
	kQueen = iota
	kBeetle
	kSpider
	kHopper
	kAnt
	kMosquito
	kLadybug
	kPillbug
	numKinds
)

var kindName = [numKinds]string{"queen", "beetle", "spider", "grasshopper", "ant", "mosquito", "ladybug", "pillbug"}

var kindLabel = [numKinds]string{"Queen Bee", "Beetle", "Spider", "Grasshopper", "Soldier Ant", "Mosquito", "Ladybug", "Pillbug"}

func kindOf(name string) (int, bool) {
	for k, n := range kindName {
		if n == name {
			return k, true
		}
	}
	return 0, false
}

// Data is the game-specific part of a position; the pieces are on the board.
type Data struct {
	// Turn is the seat to move (0 once the game is over).
	Turn int `json:"turn"`
	// Turns counts the turns each seat has taken (index seat-1): the Queen
	// must be placed by a player's fourth turn.
	Turns [2]int `json:"turns"`
	// Moved is the cell where the piece the last move moved now stands: the
	// Pillbug may not move it.
	Moved string `json:"moved,omitempty"`
	// Passes counts consecutive passes: two in a row draw the game.
	Passes int `json:"passes,omitempty"`
	// Nums numbers the tiles of each stack of the table (cell → bottom-first
	// numbers) for the notation: the second Ant is A2, whichever cell it is on.
	Nums map[string][]uint8 `json:"nums,omitempty"`
	// Last lists the cells the last move left and reached.
	Last []string `json:"last_move,omitempty"`
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
	c.Nums = make(map[string][]uint8, len(d.Nums))
	for k, v := range d.Nums {
		c.Nums[k] = append([]uint8(nil), v...)
	}
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
			return nil, fmt.Errorf("hive: bad position payload: %w", err)
		}
		return d, nil
	case nil:
		return nil, fmt.Errorf("hive: position has no game payload")
	default:
		return nil, fmt.Errorf("hive: position payload has type %T", v)
	}
}
