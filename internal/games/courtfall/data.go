package courtfall

import (
	"encoding/json"
	"fmt"

	"github.com/noderunner/bingle/internal/engine/state"
)

// Phases of a game.
const (
	phaseTerrain = "terrain" // alternating tile placement
	phaseCourt   = "court"   // each player picks a Court
	phaseDeploy  = "deploy"  // alternating unit placement
	phasePlay    = "play"
	phaseOver    = "over"
)

// Stages of the play phase: what the position is waiting for.
const (
	stageRecover = "recover" // choose whose stone the free recovery takes
	stageMain    = "main"    // spend start-of-turn Momentum, then activate / exert / clear
	stageAct     = "act"     // the active unit spends its actions
	stageRespond = "respond" // the target of a Strike splits its dice
	stageReroll  = "reroll"  // a side may spend Momentum to reroll a failed die
	stagePush    = "push"    // the winner picks a direction to push a unit
	stageShift   = "shift"   // Hit and Run's free shift
	stageSeize   = "seize"   // optionally take another turn
)

// Tile colors.
const (
	tileGreen = "green"
	tileBlue  = "blue"
	tileRed   = "red"
)

var tileColors = [3]string{tileGreen, tileBlue, tileRed}

func tileIndex(c string) int {
	for i, t := range tileColors {
		if t == c {
			return i
		}
	}
	return -1
}

// Conditions of a unit.
const (
	condFresh     = 0
	condFatigued  = 1 // white stone
	condExhausted = 2 // black stone
)

// Courts.
const (
	courtBastion = "bastion"
	courtGale    = "gale"
	courtStone   = "stone"
	courtCinder  = "cinder"
	courtVeiled  = "veiled"
)

var courts = []string{courtBastion, courtGale, courtStone, courtCinder, courtVeiled}

func validCourt(c string) bool {
	for _, k := range courts {
		if k == c {
			return true
		}
	}
	return false
}

// Units, in deployment order.
var unitTypes = []string{"pawn", "knight", "bishop", "rook", "queen"}

var unitCode = map[string]string{"pawn": "P", "knight": "N", "bishop": "B", "rook": "R", "queen": "Q"}

var unitFromCode = map[string]string{"P": "pawn", "N": "knight", "B": "bishop", "R": "rook", "Q": "queen"}

var unitName = map[string]string{"pawn": "Warden", "knight": "Outrider", "bishop": "Arbalest", "rook": "Juggernaut", "queen": "Champion"}

// Combat value and move allowance of each unit.
var (
	unitCV   = map[string]int{"pawn": 3, "knight": 3, "bishop": 2, "rook": 3, "queen": 4}
	unitMove = map[string]int{"pawn": 3, "knight": 4, "bishop": 3, "rook": 3, "queen": 4}
)

// Activation is the unit currently spending its actions.
type Activation struct {
	// Unit is the piece type of the active unit (it belongs to the seat
	// whose turn it is).
	Unit string `json:"unit"`
	// Left counts the actions it may still take.
	Left int `json:"left"`
	// Exert marks an Exert (a Fatigued unit spending its one action).
	Exert bool `json:"exert,omitempty"`
	// Charge is the hex a Juggernaut last charged from: its previous action
	// was a straight move of two or more hexes, so a Strike on a unit it
	// was not adjacent to there holds the Charge Edge.
	Charge string `json:"charge,omitempty"`
}

// Contest is a Strike, shot or shove being resolved.
type Contest struct {
	// Kind is "strike", "shoot" or "shove".
	Kind string `json:"kind"`
	// A is the initiating unit's key and T the target's.
	A string `json:"a"`
	T string `json:"t"`
	// ASeat and TSeat are their owners.
	ASeat int `json:"a_seat"`
	TSeat int `json:"t_seat"`
	// AAtk is the initiator's committed attack dice — secret until the
	// target has answered (-1 in public views).
	AAtk int `json:"a_atk"`
	// TAtk is the target's answer (-1 before it).
	TAtk int `json:"t_atk"`
	// Charge says the initiator holds the Charge Edge.
	Charge bool `json:"charge,omitempty"`
	// Rolled says the dice are on the table.
	Rolled bool `json:"rolled,omitempty"`
	// The dice (values 1-6): the initiator's attack and defense dice and the
	// target's, with each pool's success number.
	AD  []int `json:"ad,omitempty"`
	AF  []int `json:"af,omitempty"`
	TD  []int `json:"td,omitempty"`
	TF  []int `json:"tf,omitempty"`
	ADN int   `json:"ad_need,omitempty"`
	AFN int   `json:"af_need,omitempty"`
	TDN int   `json:"td_need,omitempty"`
	TFN int   `json:"tf_need,omitempty"`
	// Auto marks a shove the target loses automatically (Exhausted).
	Auto bool `json:"auto,omitempty"`
	// Step is the next side to be offered a reroll: 0 initiator, 1 target,
	// 2 none left. Used marks sides that already rerolled.
	Step int     `json:"step,omitempty"`
	Used [2]bool `json:"used,omitempty"`
}

func (k *Contest) clone() *Contest {
	if k == nil {
		return nil
	}
	c := *k
	c.AD = append([]int(nil), k.AD...)
	c.AF = append([]int(nil), k.AF...)
	c.TD = append([]int(nil), k.TD...)
	c.TF = append([]int(nil), k.TF...)
	return &c
}

// Push is a pending push of a unit by a seat's choice of direction.
type Push struct {
	// Unit is the key of the unit to push.
	Unit string `json:"unit"`
	// By is the seat that chooses the direction.
	By int `json:"by"`
	// Chain forbids pushing off the edge (the push follows a terrain Hit on
	// an Exhausted unit).
	Chain bool `json:"chain,omitempty"`
}

// Data is the game-specific part of a Courtfall position: everything the
// shared board state (units on hexes, reserves, ply) does not hold.
type Data struct {
	Phase string `json:"phase"`
	// N counts decisions made within the current setup phase.
	N int `json:"n,omitempty"`
	// Tiles maps a hex to its special tile (tan hexes are absent).
	Tiles map[string]string `json:"tiles,omitempty"`
	// Left counts the tiles still to draft: Overgrowth, Mire, Hazard.
	Left [3]int `json:"left"`
	// Courts holds each seat's Court (index seat-1).
	Courts [2]string `json:"courts"`
	// Cond maps "color/piece" to a unit's condition (Fresh is absent).
	Cond map[string]int `json:"cond,omitempty"`
	// Mom and VP are each seat's Momentum and victory points.
	Mom [2]int `json:"mom"`
	VP  [2]int `json:"vp"`
	// StoneUsed marks a seat's Stone Court swap as spent until its next
	// Clear Turn.
	StoneUsed [2]bool `json:"stone_used,omitempty"`

	Stage  string      `json:"stage,omitempty"`
	Turn   int         `json:"turn,omitempty"`
	Streak int         `json:"streak,omitempty"`
	Act    *Activation `json:"act,omitempty"`
	Con    *Contest    `json:"contest,omitempty"`
	// Last is the most recently resolved contest, kept for display.
	Last   *Contest `json:"last,omitempty"`
	Pushes []Push   `json:"pushes,omitempty"`

	// RNG is the dice generator's state — secret in public views.
	RNG uint64 `json:"rng"`

	// Result of a finished game.
	Winner int    `json:"winner,omitempty"`
	Reason string `json:"reason,omitempty"`

	// Events narrates what the last move did (public).
	Events []string `json:"events,omitempty"`
}

// CloneExt implements state.ExtClone.
func (d *Data) CloneExt() any { return d.clone() }

func (d *Data) clone() *Data {
	c := *d
	if d.Tiles != nil {
		c.Tiles = make(map[string]string, len(d.Tiles))
		for k, v := range d.Tiles {
			c.Tiles[k] = v
		}
	}
	if d.Cond != nil {
		c.Cond = make(map[string]int, len(d.Cond))
		for k, v := range d.Cond {
			c.Cond[k] = v
		}
	}
	if d.Act != nil {
		a := *d.Act
		c.Act = &a
	}
	c.Con = d.Con.clone()
	c.Last = d.Last.clone()
	if d.Pushes != nil {
		c.Pushes = append([]Push(nil), d.Pushes...)
	}
	if d.Events != nil {
		c.Events = append([]string(nil), d.Events...)
	}
	return &c
}

// dataOf reads a position's Courtfall data. A position that came straight
// from a document is decoded on every call (never cached on the shared
// state); positions the ruleset produced hold the typed value.
func dataOf(st *state.BoardState) (*Data, error) {
	switch v := st.Ext().(type) {
	case *Data:
		return v, nil
	case *publicExt:
		return v.Data, nil
	case json.RawMessage:
		d := &Data{}
		if err := json.Unmarshal(v, d); err != nil {
			return nil, fmt.Errorf("courtfall: bad position payload: %w", err)
		}
		return d, nil
	case nil:
		return nil, fmt.Errorf("courtfall: position has no game payload")
	default:
		return nil, fmt.Errorf("courtfall: position payload has type %T", v)
	}
}

// publicExt is how a redacted position serialises: the (masked) data with
// the derived view for clients next to it.
type publicExt struct {
	*Data
	View *View `json:"view"`
}

// CloneExt implements state.ExtClone for redacted positions.
func (p *publicExt) CloneExt() any {
	return &publicExt{Data: p.clone(), View: p.View}
}

func unitKey(color, piece string) string { return color + "/" + piece }
