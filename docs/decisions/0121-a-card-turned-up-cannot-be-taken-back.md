# 0121. A card turned up cannot be taken back

Status: proposed (2026-11-06). It is stage 1 of the patience vocabulary plan (`docs/history/patience-vocabulary-plan.md`
on `more-plans`), and settles what [ADR 0119](0119-a-seeded-deal-at-the-start.md) left open: drawing during play.

## Context

A player may take back a selection, or go Back within an impulse, while nothing has happened that they could not
know before (ADR 0083). Chance ends that: rewinding across a die roll lets a player see the dice and choose again,
and [ADR 0114](0114-impulses-selections-moves-and-gestures.md) makes such a selection end the impulse, through
`CanTakeBack`.

A ruleset says when it cannot take back with `state.UndoGuard`. Solitaire's says: a play stands once it shows the
player a card they could not see before, such as a card from the stock or a card uncovered in a column. A declared
game has no `UndoGuard`, so its `CanTakeBack` is always true. Since ADR 0119 a declared game can hold hidden cards.
The patience vocabulary will turn them up during play, so a declared game needs Solitaire's rule.

## Decision

In a declared game with `cards`, a selection **cannot be taken back** when the position after it lets some seat see
the face of a card that the position before it hid from that seat.

- "See" is the zones' `visibility` and each card's face, exactly as redaction works out what a seat is sent
  (`Table.Redact`): the rule compares, seat by seat, the cards each could see before and after.
- A card that only changes place, or turns face down, takes nothing back from anyone, and the selection may be taken
  back as before.
- With ADR 0114 this ends the impulse at that selection. The player sees the card, and the next choice is a new
  impulse.
- It is general, not patience's own: a draw to a hidden hand, a card flipped on the table and a card uncovered in a
  column all fall under it.

A ruleset keeps its own `UndoGuard`; this is what a declared game gets without one.

## Consequences

- `state.CanTakeBack` gains the rule for declared games with cards, so `ImpulseEnds`, the page's Back and the
  session's take-back all follow it with no change of their own.
- Every declared game shipped today either has no cards or shows them all from the deal on (Onitama's declaration),
  so nothing changes in play; the differential harness, which compares where impulses end, holds them to it.
- The patience vocabulary can move cards out of the stock and turn up column cards, and the declaration still agrees
  with Solitaire's ruleset on where the impulse ends.
