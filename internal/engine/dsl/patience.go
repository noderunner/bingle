package dsl

import (
	"encoding/json"
	"fmt"
)

// PatienceDef is a game of patience (ADR 0122): its columns, foundations,
// free cells, stock and finished runs, answered as parameters. The catalog
// compiles it to one action, "patience", and win_condition.type "patience".
type PatienceDef struct {
	Columns     PatienceColumnsDef `json:"columns"`
	Foundations *FoundationsDef    `json:"foundations,omitempty"`
	// Cells is how many free cells, each holding one card.
	Cells int       `json:"cells,omitempty"`
	Stock *StockDef `json:"stock,omitempty"`
	// Off takes a complete run off its column into the finished pile.
	Off *OffDef `json:"off,omitempty"`
	// Finish offers "finish" once every card can go home with nothing hidden.
	Finish bool `json:"finish,omitempty"`
}

// PatienceColumnsDef is the tableau.
type PatienceColumnsDef struct {
	Count int              `json:"count"`
	Deal  ColumnDealDef    `json:"deal"`
	Build PatienceBuildDef `json:"build,omitempty"`
	// Lift is what may be moved off a column together: "top", "ordered"
	// (default), "same-suit" or "any-face-up".
	Lift string `json:"lift,omitempty"`
	// LiftLimit "cells" limits a run to (free cells + 1) × 2^(empty columns).
	LiftLimit string `json:"lift_limit,omitempty"`
	// Empty is what an empty column takes: "any" (default), "king" or "none".
	Empty string `json:"empty,omitempty"`
}

// ColumnDealDef is how the columns are dealt.
type ColumnDealDef struct {
	// Sizes is the cards each column is dealt: one number for all, or one
	// per column.
	Sizes Counts `json:"sizes"`
	// Down is how many of them lie face down (default: all but the top).
	Down Counts `json:"down,omitempty"`
	// KingsUnder puts each column's kings at its bottom (Baker's Dozen).
	KingsUnder bool `json:"kings_under,omitempty"`
}

// PatienceBuildDef is how cards build in a column.
type PatienceBuildDef struct {
	// Rank is "down" (default) or "up".
	Rank string `json:"rank,omitempty"`
	// Suit is "alternate" (colour, by place in suit_order), "same" or "any".
	Suit string `json:"suit,omitempty"`
}

// FoundationsDef is the piles built up by suit from the ace.
type FoundationsDef struct {
	Count int `json:"count"`
	// Back lets a foundation's top card come back to a column.
	Back bool `json:"back,omitempty"`
}

// StockDef is the stock: either drawn to the waste (Draw > 0), or dealt in a
// row onto the columns (Deal "row").
type StockDef struct {
	Draw int    `json:"draw,omitempty"`
	To   string `json:"to,omitempty"`
	// Redeals is how often the waste may go back to the stock: a number or
	// "unlimited" (default 0).
	Redeals *Quantity `json:"redeals,omitempty"`
	Deal    string    `json:"deal,omitempty"`
	// Columns is how many columns a row is dealt to: "all" or a number.
	Columns *Quantity `json:"columns,omitempty"`
	// UnlessEmpty refuses a row while a column is empty (Spider).
	UnlessEmpty bool `json:"unless_empty,omitempty"`
}

// OffDef is a run that leaves the table once built.
type OffDef struct {
	// Run is "king-to-ace"; Suit is "same".
	Run  string `json:"run"`
	Suit string `json:"suit,omitempty"`
}

// Counts is one number for every column, or one per column.
type Counts []int

// UnmarshalJSON accepts a number or a list of numbers.
func (c *Counts) UnmarshalJSON(data []byte) error {
	var n int
	if err := json.Unmarshal(data, &n); err == nil {
		*c = Counts{n}
		return nil
	}
	var list []int
	if err := json.Unmarshal(data, &list); err != nil {
		return fmt.Errorf("dsl: expected a number or a list of numbers")
	}
	*c = list
	return nil
}

// At is the count for column i (0-based) of n: the single number, or the
// list's entry.
func (c Counts) At(i int) int {
	if len(c) == 1 {
		return c[0]
	}
	if i < len(c) {
		return c[i]
	}
	return 0
}
