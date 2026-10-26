package nyet

import (
	"encoding/json"
	"fmt"

	"github.com/noderunner/bingle/internal/engine/cards"
	"github.com/noderunner/bingle/internal/engine/state"
)

// Phases of a round.
const (
	phaseNyet    = "nyet"    // the players cross choices off the board
	phaseTeam    = "team"    // the first player picks the team
	phaseBonus   = "bonus"   // (five players) the first player gives out the bonus card
	phaseDiscard = "discard" // cards are discarded or passed
	phaseTrick   = "trick"   // the tricks
)

// The five lines of the board.
const (
	lineFirst   = "first"
	lineDiscard = "discard"
	lineTrump   = "trump"
	lineSuper   = "super"
	linePoints  = "points"
)

// Play is a card played into the trick by a seat.
type Play struct {
	Seat int    `json:"seat"`
	Card string `json:"card"`
}

// Data is the game-specific part of a position. The cards are in Table.
type Data struct {
	// Phase is the stage of the round, Turn the seat that decides (0 once the
	// game is over).
	Phase string `json:"phase"`
	Turn  int    `json:"turn"`
	// Round counts from 1; Dealer deals the round and crosses first.
	Round  int `json:"round"`
	Dealer int `json:"dealer"`
	// RNG is the shuffling generator's state: secret in public views.
	RNG uint64 `json:"rng,omitempty"`
	// Table holds every card (hands, deck, trick, tricks won, loot).
	Table *cards.Table `json:"table,omitempty"`

	// Crossed maps a crossed box ("line:option") to the seat that crossed it.
	Crossed map[string]int `json:"crossed,omitempty"`
	// The terms of the round, once every line has one box left.
	First  int    `json:"first,omitempty"`
	Disc   string `json:"disc,omitempty"` // none, 1, 2, except1, passleft
	Trump  string `json:"trump,omitempty"`
	Super  string `json:"super,omitempty"` // a colour, or "none"
	Points int    `json:"points,omitempty"`
	// Team is the first player's team (including them); Bonus holds the x2 card.
	Team  []int `json:"team,omitempty"`
	Bonus int   `json:"bonus,omitempty"`
	// DiscardDone counts the discard decisions made; Pending are the cards
	// chosen to pass left (seat → card), secret until everyone has chosen.
	DiscardDone int            `json:"discard_done,omitempty"`
	Pending     map[int]string `json:"pending,omitempty"`

	// Plays are the cards of the trick being played; LastTrick the previous one.
	Plays      []Play `json:"plays,omitempty"`
	LastTrick  []Play `json:"last_trick,omitempty"`
	LastWinner int    `json:"last_winner,omitempty"`
	// Tricks and Loot count what each seat's won this round (index seat).
	Tricks [6]int `json:"tricks"`
	Loot   [6]int `json:"loot"`
	// Scores are the points of each seat over the game (index seat).
	Scores [6]int `json:"scores"`
	// Result of a finished game.
	Winner int    `json:"winner,omitempty"`
	Reason string `json:"reason,omitempty"`
	// Events narrates what the last decision did.
	Events []string `json:"events,omitempty"`
}

// CloneExt implements state.ExtClone.
func (d *Data) CloneExt() any { return d.clone() }

func (d *Data) clone() *Data {
	c := *d
	c.Table = d.Table.Clone()
	c.Crossed = make(map[string]int, len(d.Crossed))
	for k, v := range d.Crossed {
		c.Crossed[k] = v
	}
	c.Team = append([]int(nil), d.Team...)
	c.Pending = make(map[int]string, len(d.Pending))
	for k, v := range d.Pending {
		c.Pending[k] = v
	}
	c.Plays = append([]Play(nil), d.Plays...)
	c.LastTrick = append([]Play(nil), d.LastTrick...)
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
			return nil, fmt.Errorf("nyet: bad position payload: %w", err)
		}
		if d.Table == nil {
			d.Table = cards.NewTable()
		}
		return d, nil
	case nil:
		return nil, fmt.Errorf("nyet: position has no game payload")
	default:
		return nil, fmt.Errorf("nyet: position payload has type %T", v)
	}
}
