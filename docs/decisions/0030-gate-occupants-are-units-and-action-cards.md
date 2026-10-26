# 0030. The unit on a Gate is a unit of its own; every action button has a card

## Context

ADR 0029's playtests came back with three kinds of complaint. A Gate showed the
figure that occupies it, but the figure could not be picked: the page moved "a
Cultist" of the stack, which could not tell the one on the Gate from the others,
so giving a Gate up needed a hidden step-off, and moving the occupant out of an
area with company quietly handed the Gate to somebody else. The unit icons of
the cult card listed only what was in the pool, so a player could not see where
the rest of the cult was. And the action buttons told the player little: "Create
Gate" and "Awaken Great Old One" did not say what they cost or where they work,
a spellbook choice was a row of unnamed buttons.

## Decision

**The occupant is one of the units of its type, told apart from the rest.** The
engine still counts units by area, seat and type, but `unitMoves` splits the
units of the occupant's type in the area of a Gate the seat holds: the others
move with `go:<unit>@a>b` (as many as stand beside the Gate), the occupant with
the new `leave:<unit>@a>b`, to any area it can reach or to its own. A `leave`
abandons the Gate, whoever else stands there — the player's rule, not the
rulebook's, which keeps control by presence. Moving it to another area is the
usual Move step; stepping off in place was one too, and became a free action
(ADR 0031).
Against the owner's will (a kill, a capture) nothing changes: another unit that
can hold the Gate steps on.

**Putting a unit on a Gate is a free action with a unit.** `control:<unit>@a`
puts a unit of the seat that stands in the area of an abandoned Gate onto it, or
swaps the occupant of a Gate of its own for a unit of another type. It does not
end the turn and is also offered in the middle of a Move action (abandoned Gates
only). The clicks are the ones of a move that goes nowhere — the unit, then the
Gate — and the occupant's piece value is `gate:<unit>`, so the page draws the
figure on the Gate and the stack beside it as two tokens and needs no new
interaction. A turn is held open for an abandoned Gate with a unit beside it,
never for a swap. A unit that *arrives* at an abandoned Gate still takes it by
itself, except in the turn the Gate was given up.

**The cult card shows every unit.** `units[].places` says where each unit that is
not in the pool is — the area (and "on the Gate"), captured by whom, held,
asleep, submerged — and the page greys those icons out; `units[].enter` says
where a unit may come into play.

**Every action button states its price and carries its rule.** The view has
`costs`: the Power each action id costs the player to move, from the legal moves
and `moveCost` (a Ritual's included). Great Old Ones get a button each
(`awaken-<unit>`) like monsters; *Create Gate* is *Build Gate*; the button text
carries the cost when there is one; a hover card shows the action's `hint`,
which `Describe` takes from the spellbook or faction ability that creates the
action, or from a summary written for it. A spellbook choice is headed
*Choose a spellbook to unlock:*.

## Consequences

* The action space of the agents grew at its end (`leave`, `control:<unit>`);
  `go:<unit>@a>a` and `control@a` stay in the list but are never legal.
* Saved positions need nothing: a Gate with no recorded occupant is given the
  one its controller would choose.
* The bots never `leave`, and never swap: `withoutStepOffs` drops the moves
  that give a Gate up for nothing.
* A new action needs a `Describe` entry with a rule text; the page does the
  rest.
