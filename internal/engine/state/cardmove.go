package state

import (
	"fmt"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/cards"
	"github.com/noderunner/bingle/internal/engine/catalog"
)

// Move cards (ADR 0120): a card in the mover's hand names [right, forward]
// offsets, read through the seat's forward; playing it leaps one of the
// mover's pieces by one of them, and the card is then passed on.

// cardTargets lists where the piece at from may go with the card of type t,
// for the mover: on the board, and not onto a piece of its own side.
func (c *rctx) cardTargets(t *catalog.CardType, from board.PositionID) []board.PositionID {
	g := c.game.Board
	fc, ok := g.Coord(from)
	if !ok {
		return nil
	}
	f := c.game.Forward[c.seat]
	right := [2]int{-f[1], f[0]}
	var out []board.PositionID
	for _, m := range t.Moves {
		dx, dy := m[0]*right[0]+m[1]*f[0], m[0]*right[1]+m[1]*f[1]
		to, ok := g.At(board.Coord{X: fc.X + dx, Y: fc.Y + dy})
		if !ok {
			continue
		}
		if top, occupied := c.st.Get(to); occupied && c.mine(top.Color) {
			continue
		}
		out = append(out, to)
	}
	return out
}

// hand is the mover's cards for act, or nil.
func (c *rctx) hand(act catalog.ResolvedAction) *cards.Zone {
	if c.st.table == nil {
		return nil
	}
	z, ok := c.st.table.Zones[cards.Key(act.Hand, c.seat)]
	if !ok {
		return nil
	}
	return z
}

// cardMoves calls yield with each move the mover's cards allow, until it
// returns false.
func (c *rctx) cardMoves(act catalog.ResolvedAction, yield func(Selection) bool) {
	hand := c.hand(act)
	if hand == nil {
		return
	}
	isPiece := map[string]bool{}
	for _, p := range act.Pieces {
		isPiece[p] = true
	}
	for _, card := range hand.Cards {
		t := c.game.Cards.Type(card.ID)
		if t == nil {
			continue
		}
		for _, from := range c.game.Board.Positions() {
			top, ok := c.st.Get(from)
			if !ok || !c.mine(top.Color) || !isPiece[top.Piece] {
				continue
			}
			for _, to := range c.cardTargets(t, from) {
				if !yield(Selection{Action: act.ID, Card: card.ID, From: from, To: to}) {
					return
				}
			}
		}
	}
}

// anyCardMove reports whether the mover has a card that moves a piece.
func (c *rctx) anyCardMove(act catalog.ResolvedAction) bool {
	found := false
	c.cardMoves(act, func(Selection) bool { found = true; return false })
	return found
}

func (c *rctx) enumerateCardMoves(act catalog.ResolvedAction, keep func(catalog.ResolvedAction, Selection, bool)) {
	moved := false
	c.cardMoves(act, func(mv Selection) bool {
		moved = true
		keep(act, mv, true)
		return !c.enough()
	})
	if moved || !act.StuckSpend {
		return
	}
	// Nothing moves: each card may be spent without a move.
	if hand := c.hand(act); hand != nil {
		for _, card := range hand.Cards {
			if c.enough() {
				return
			}
			keep(act, Selection{Action: act.ID, Card: card.ID}, true)
		}
	}
}

// validateCardMove checks a card-move: the card is in the mover's hand, and
// either the piece's leap is one the card allows, or (with stuck: spend) no
// card moves any piece and the card is spent.
func (c *rctx) validateCardMove(act catalog.ResolvedAction, mv Selection) (executor, error) {
	if mv.Piece != "" || mv.Count != 0 || len(mv.Path) > 0 || len(mv.Drops) > 0 || mv.Aim != "" {
		return nil, fmt.Errorf("state: a card move takes a card and two squares")
	}
	hand := c.hand(act)
	if mv.Card == "" || hand == nil || !hand.Has(mv.Card) {
		return nil, fmt.Errorf("state: %q is not a card in your hand", mv.Card)
	}
	t := c.game.Cards.Type(mv.Card)
	if t == nil {
		return nil, fmt.Errorf("state: unknown card %q", mv.Card)
	}
	if mv.From == "" && mv.To == "" {
		if !act.StuckSpend {
			return nil, fmt.Errorf("state: a card move needs a piece to move")
		}
		if c.anyCardMove(act) {
			return nil, fmt.Errorf("state: a card can be spent without a move only when no card moves a piece")
		}
		return c.passOn(act, mv.Card, "", ""), nil
	}
	top, ok := c.st.Get(mv.From)
	if !ok || !c.mine(top.Color) {
		return nil, fmt.Errorf("state: no piece of yours on %s", mv.From)
	}
	allowed := false
	for _, p := range act.Pieces {
		allowed = allowed || p == top.Piece
	}
	if !allowed {
		return nil, fmt.Errorf("state: a %s is not moved by cards", pieceLabel(c.game, top.Piece))
	}
	reach := false
	for _, to := range c.cardTargets(t, mv.From) {
		reach = reach || to == mv.To
	}
	if !reach {
		return nil, fmt.Errorf("state: the %s card does not move %s to %s", t.Label, mv.From, mv.To)
	}
	return c.passOn(act, mv.Card, mv.From, mv.To), nil
}

// passOn is the executor of a card-move: the piece leaps from from to to
// (capturing what it lands on), unless the card is spent without a move;
// then the mover takes the cards in pass_on, and the card goes there face up.
func (c *rctx) passOn(act catalog.ResolvedAction, card string, from, to board.PositionID) executor {
	seat := c.seat
	return func(ns *BoardState) []board.PositionID {
		var captured []board.PositionID
		if from != "" {
			piece, _ := ns.Get(from)
			stack := ns.Stack(from)
			if len(stack) <= 1 {
				ns.Remove(from)
			} else {
				ns.SetStack(from, stack[:len(stack)-1])
			}
			if _, occupied := ns.Get(to); occupied {
				ns.Remove(to)
				captured = append(captured, to)
			}
			ns.Push(to, piece)
			c.applyPromotion(ns, to)
		}
		t := ns.table
		hand := t.Zone(act.Hand, seat)
		used, _ := hand.Take(card)
		used.Up = true
		if act.PassOn != "" {
			side := t.Zone(act.PassOn, 0)
			for _, got := range side.Clear() {
				got.Up = true
				hand.Push(got)
			}
			side.Push(used)
		} else {
			hand.Push(used)
		}
		return captured
	}
}
