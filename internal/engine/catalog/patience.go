package catalog

import (
	"fmt"
	"strconv"

	"github.com/noderunner/bingle/internal/engine/dsl"
)

// Patience is a compiled patience block (ADR 0122).
type Patience struct {
	Columns    int
	Sizes      []int // cards dealt to each column
	Down       []int // of them face down
	KingsUnder bool
	RankUp     bool   // build up rather than down
	Suit       string // "alternate", "same" or "any"
	Lift       string // "top", "ordered", "same-suit" or "any-face-up"
	LiftCells  bool   // FreeCell's run limit
	Empty      string // "any", "king" or "none"

	Foundations int
	Back        bool
	Cells       int

	Draw    int // cards a draw turns onto the waste; 0 for no waste
	Redeals int // times the waste may go back; -1 for no limit
	Row     int // columns a row is dealt to; 0 for no rows
	// RowUnlessEmpty refuses a row while a column is empty.
	RowUnlessEmpty bool

	Off    bool
	Finish bool

	// Deck is the deck dealt; Total its cards.
	Deck  string
	Total int
}

// Patience pile names, as Solitaire's notation writes them.
const (
	PileStock = "stock"
	PileWaste = "waste"
	PileDone  = "done"
)

// Column, Foundation and Cell name the piles of a game of patience.
func Column(i int) string     { return "t" + strconv.Itoa(i) }
func Foundation(i int) string { return "f" + strconv.Itoa(i) }
func Cell(i int) string       { return "c" + strconv.Itoa(i) }

// compilePatience resolves a patience block: one seat, one deck, a deal the
// deck can make, the piles' zones (added when the definition leaves them
// out), the patience action and win_condition.type "patience".
func compilePatience(cg *CompiledGame) error {
	def := cg.Definition
	pd := def.Patience
	if pd == nil {
		if def.WinCondition.Type == "patience" {
			return fmt.Errorf("catalog: win_condition.type \"patience\" needs a patience block")
		}
		return nil
	}
	fail := func(format string, args ...any) error {
		return fmt.Errorf("catalog: patience: "+format, args...)
	}
	if len(def.Seats) != 1 {
		return fail("a game of patience is for one seat")
	}
	if def.WinCondition.Type != "patience" {
		return fail("win_condition.type must be \"patience\"")
	}
	if len(def.Deal) > 0 {
		return fail("the columns deal the cards; leave out deal")
	}
	cc := cg.Cards
	if cc == nil || len(cc.Instances) != 1 {
		return fail("needs cards with one deck")
	}
	p := &Patience{Finish: pd.Finish, Cells: pd.Cells}
	for name, ids := range cc.Instances {
		p.Deck, p.Total = name, len(ids)
	}
	c := pd.Columns
	if c.Count < 1 {
		return fail("columns.count must be at least 1")
	}
	p.Columns = c.Count
	if n := len(c.Deal.Sizes); n != 1 && n != c.Count {
		return fail("columns.deal.sizes needs one number or %d", c.Count)
	}
	if n := len(c.Deal.Down); n > 1 && n != c.Count {
		return fail("columns.deal.down needs one number or %d", c.Count)
	}
	dealt := 0
	for i := 0; i < c.Count; i++ {
		size := c.Deal.Sizes.At(i)
		down := size - 1
		if len(c.Deal.Down) > 0 {
			down = c.Deal.Down.At(i)
		}
		if size < 0 || down < 0 || (size > 0 && down > size) || (size == 0 && down > 0) {
			return fail("column %d is dealt %d cards, %d of them down", i+1, size, down)
		}
		if size == 0 {
			down = 0
		}
		p.Sizes, p.Down = append(p.Sizes, size), append(p.Down, down)
		dealt += size
	}
	if dealt > p.Total {
		return fail("the columns take %d cards and the deck has %d", dealt, p.Total)
	}
	p.KingsUnder = c.Deal.KingsUnder
	switch c.Build.Rank {
	case "", "down":
	case "up":
		p.RankUp = true
	default:
		return fail("build.rank %q is not down or up", c.Build.Rank)
	}
	switch p.Suit = c.Build.Suit; p.Suit {
	case "alternate", "same", "any":
	case "":
		p.Suit = "any"
	default:
		return fail("build.suit %q is not alternate, same or any", c.Build.Suit)
	}
	switch p.Lift = c.Lift; p.Lift {
	case "top", "ordered", "same-suit", "any-face-up":
	case "":
		p.Lift = "ordered"
	default:
		return fail("lift %q is not top, ordered, same-suit or any-face-up", c.Lift)
	}
	switch c.LiftLimit {
	case "":
	case "cells":
		p.LiftCells = true
	default:
		return fail("lift_limit %q is not cells", c.LiftLimit)
	}
	switch p.Empty = c.Empty; p.Empty {
	case "any", "king", "none":
	case "":
		p.Empty = "any"
	default:
		return fail("empty %q is not any, king or none", c.Empty)
	}
	if f := pd.Foundations; f != nil {
		if f.Count < 1 || f.Count%len(cc.Suits) != 0 {
			return fail("foundations.count %d is not a multiple of the %d suits", f.Count, len(cc.Suits))
		}
		p.Foundations, p.Back = f.Count, f.Back
	}
	if p.Cells < 0 {
		return fail("cells must not be negative")
	}
	if s := pd.Stock; s != nil {
		switch {
		case s.Draw > 0 && s.Deal == "":
			if s.To != "waste" {
				return fail("stock.to must be waste")
			}
			p.Draw = s.Draw
			if r := s.Redeals; r != nil {
				switch {
				case r.Keyword == "unlimited":
					p.Redeals = -1
				case r.Keyword == "" && r.N >= 0:
					p.Redeals = r.N
				default:
					return fail("stock.redeals must be a number or unlimited")
				}
			}
		case s.Deal == "row" && s.Draw == 0:
			p.Row = p.Columns
			if q := s.Columns; q != nil && q.Keyword != "all" {
				if q.Keyword != "" || q.N < 1 || q.N > p.Columns {
					return fail("stock.columns must be all or 1 to %d", p.Columns)
				}
				p.Row = q.N
			}
			p.RowUnlessEmpty = s.UnlessEmpty
		default:
			return fail("stock is either {draw, to: waste} or {deal: row}")
		}
	}
	if o := pd.Off; o != nil {
		if o.Run != "king-to-ace" || (o.Suit != "" && o.Suit != "same") {
			return fail("off is {run: king-to-ace, suit: same}")
		}
		p.Off = true
	}
	if p.Foundations == 0 && !p.Off {
		return fail("a game needs foundations or off to be won")
	}
	if p.Finish && p.Foundations == 0 {
		return fail("finish needs foundations")
	}
	// The piles' zones, as the page and redaction read them.
	zone := func(id, label, vis string, pile, counts bool) {
		if _, ok := cc.Zones[id]; !ok {
			z := dsl.ZoneDef{ID: id, Label: label, Scope: "table", Visibility: vis, Pile: pile, Counts: counts}
			cc.Zones[id] = z
			def.Cards.Zones = append(def.Cards.Zones, z)
		}
	}
	for i := 1; i <= p.Columns; i++ {
		zone(Column(i), "Column", "flag", false, false)
	}
	for i := 1; i <= p.Foundations; i++ {
		zone(Foundation(i), "Foundation", "all", true, false)
	}
	for i := 1; i <= p.Cells; i++ {
		zone(Cell(i), "Free cell", "all", true, false)
	}
	zone(PileStock, "Stock", "none", true, true)
	if p.Draw > 0 {
		zone(PileWaste, "Waste", "all", true, false)
	}
	if p.Off {
		zone(PileDone, "Finished suits", "all", true, false)
	}
	cg.Patience = p
	cg.Actions = append(cg.Actions, ResolvedAction{ID: ActionPatience, Type: ActionPatience, Label: "Move"})
	return nil
}
