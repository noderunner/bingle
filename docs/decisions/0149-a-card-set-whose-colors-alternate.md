# 0149. A set of cards may share its colors, if they alternate warm and cool

Status: accepted (2026-11-25). It amends [ADR 0085](0085-card-sets-belong-to-the-table.md), whose sets give each
suit a color of its own.

## Context

A game with more than four suits had no set that looked like playing cards: the themed sets of ADR 0085 each have
eight suits in eight colors, so none of them says "red or black" at a glance. Where colors are the rule
(Solitaire's `alternate`), a card at such a table carries a mark of its classic suit (`.sol-color`) to say which it
counts as. And the rules already decide color by place: a suit at an odd place in `suit_order` is red, one at an
even place black (docs/dsl.md).

## Decision

**Classic Extended** (`classic-extended`) is the four suits of a pack of playing cards and four more, in this
order: hearts, spades, diamonds, clubs, clovers, moons, arrows (an arrowhead) and hourglasses. Hearts and diamonds
are red, spades and clubs black, clovers and arrows orange, moons and hourglasses purple. So the colors group the
suits in pairs four ways, and they alternate as the places do: the odd places are warm (red, orange) and the even
places cool (black, purple). A game that needs only two colors reads warm and cool, as Blooms' players do.

A set that does this says so (`alternates` in `js/card-sets.js`). Its suits may share a color two by two, never a
color at an odd place and an even one, and where colors are the rule its cards carry no mark of the classic suit,
since their own colors show it. Every other set still gives each suit its own color.

The order puts the classic suits where every game in the repository already lists them (hearts, spades, diamonds,
clubs), so their hearts are hearts. A suit's tooltip names the rules' suit only when it is another: *9 of Hearts*,
not *9 of Hearts (hearts)*.

## Consequences

Nyet! and every other game with a standard deck can be set up with it, as with any set. A game whose suits are in
another order still wears the set by place (ADR 0085), so its spades might wear hearts; no game here does.
