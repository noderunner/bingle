# 0055. The order of a hand: sorted in the view, rearranged in the browser

Status: accepted (2026-10-27)

## Context

`bingle_tasks.md` asked for two things about hands of cards: that "the private view/hand of cards
for any card game should sort by suit then rank if possible", and that human players be able to
"click and drag cards in their hand to re-arrange them however they like".

The two pull in opposite directions: a sorted hand is the same for everyone and belongs to the
game, while an arrangement is one person's habit and must not be lost on the next redraw — and the
page is rebuilt on every move. Nyet! is the only card game today, but the card framework
(ADR 0043) is meant for every card game, so neither behaviour should live in Nyet!'s code.

## Decision

**The deal is sorted on the server, in the view.** `cards.Table.View` sorts the cards of a zone
when the zone is private to its owner (visibility `owner`), the owner is the viewer, the zone is
not a pile, and the viewer can see every card in it: by suit in the game's `suit_order`, then by
the rank's `value`, then by the card's type id; cards of no suit (jokers) come last, and two copies
of one card keep the order they were dealt in. The `Table` itself is untouched — the cards lie
where the deal left them, so positions, notation and replays do not change — and every client gets
the tidy order, including an agent reading the state over REST or MCP. Rulesets need no code for
this: Nyet! already builds its view with `d.Table.View(...)`.

**The arrangement is the browser's.** `js/cards.js` remembers the order a player drags their cards
into under `localStorage` `bingle.handOrder`, keyed by room, zone and seat (the last 40 keys).
Drawing a hand applies it: the cards the stored order knows come first, in it, and any card it does
not know follows in the order the server sent. A fresh deal therefore knows none of them and comes
out sorted, while playing or passing a card leaves the rest as they were and a drawn card joins at
the right-hand end.

It is not sent to the server. There are no accounts, the order means nothing to the rules, and
storing it would make a write on every drag and a per-seat row to migrate; the cost of this choice
is that the arrangement does not follow a player to another browser.

**The drag** is pointer events, not HTML5 drag and drop, so it works the same with a mouse, a pen
and a finger (`touch-action: pan-y` on the cards leaves the page scrolling vertically). A press
becomes a drag after 6 px of travel, so a tap still plays the card; the positions of the other
cards are measured once at that moment, because only transforms move after it (the cards the
dragged one would land in front of shift right to open a gap), which keeps the drop target from
oscillating as the cards move. The click that ends a drag is swallowed in the capture phase, so a
rearrangement never plays a card. Alt+← and Alt+→ move the card that has the focus, as the hand's
cards are buttons when they can be played and the drag alone would leave a keyboard player out.

## Interpretations

- "Sort by suit then rank if possible" is the order the *view* hands out, not a change to the cards
  on the table: a game whose rules care where a card sits in a hand keeps working.
- "However they like" is a free arrangement, with no re-sort button: a new deal sorts the hand
  again by itself, which is what a player wants after a deal.
