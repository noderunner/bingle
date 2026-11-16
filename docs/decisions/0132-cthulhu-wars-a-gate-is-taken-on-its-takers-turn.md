# 0132. Cthulhu Wars: a Gate is taken only on its taker's turn, and a lost holder is replaced only by its own kind

Status: accepted (2026-11-16). Supersedes two sentences of
[ADR 0030](0030-gate-occupants-are-units-and-action-cards.md); the rest of 0030 stands.

## Context

ADR 0030 said that a unit arriving at an abandoned Gate takes it by itself (except in the turn the Gate was given
up), and that when the unit on a Gate is removed against its owner's will, "another unit that can hold the Gate steps
on". An audit of the game against its rulebook found both too generous. Control is an Unlimited Action, and "Unlimited
Actions must occur on your turn. You cannot Control an Abandoned Gate with a Cultist except when it is your turn to
take an Action. If two players both have Cultists in an Area with an Abandoned Gate, the first player to take an
Action will be able to Control the Gate" (printed p. 19). A Gate is Controlled by *the* Cultist on top of it and is
Abandoned "as indicated by the lack of a Cultist on top" (p. 9). Capture was already settled by ADRs 0051 and 0131.

## Decision

- **Arriving** (`mayControlNow`): a unit that arrives at an abandoned Gate (by Move, Recruit, retreat, Emerge,
  Dreams, or one an enemy moves or places) takes it by itself only in its own cult's turn, and not in the turn the
  Gate changed hands. Off its turn the Gate stays abandoned until a cult takes it with the free `control` action in
  its own turn; the first to act gets it.
- **Losing the holder** (`syncGate`): when the unit on a Gate is removed and another unit of the **same kind** stands
  there, that one keeps the Gate: the engine counts units by type, so the removed unit is taken to be one from beside
  the Gate, as an owner choosing its loss would choose. When the holder was the **last of its kind** there, the Gate
  is abandoned. Another kind of unit (the High Priest beside an Acolyte, a Dark Young under the Red Sign) steps on
  only in its controller's own turn, as the `control` action.

## Consequences

A battle, a Dreams or a retreat in another cult's turn can leave a Gate open for whoever acts first. In that cult's
own turn nothing changes for the player: arriving still takes the Gate with no extra click. Hastur's Vengeance still
cannot send a result to the unit on a Gate, because results are assigned by unit type; that is left open. The engine
change is in the private overlay, which carries the Cthulhu Wars code (ADR 0127).
