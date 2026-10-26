# 0031. A free step-off, the Oath asked after every action, spellbook marks

## Context

The eighth playtest list asked for four things that were not features but
corrections. Stepping a unit off its Gate in place had been made a Move step
(ADR 0030): it cost 1 Power and left a Move open, with *Done* and *Undo my move*
to click — for something the player thinks of as free, like putting the unit
back on. The Unspeakable Oath was asked only "when it could matter" (no Power
left, the High Priest threatened…), so a player who had recruited the High
Priest and checked none of the skip boxes was never asked — and read it as a
bug. The cult cards ticked books and requirements with green marks while the
buttons of the actions they give showed nothing of where they came from.
Units a cult held captive were not shown under the cult that held them.

## Decision

**Stepping off in place is a free action, once per Gate per turn.** `leave:<unit>
@a>a` is offered with the other unlimited actions (and in the middle of a Move);
it is no Move, uses no Power and no action, and opens nothing to confirm. A Gate
changes hands once in a turn (`Turn.Toggled`): after a step-off, or after a unit
was put on it, it cannot be stepped off or swapped again that turn, so the free
actions cannot go round for ever. Moving the occupant to *another* area is as it
was: a Move step, 1 Power, Done. A Move ends by itself when nothing is left to
pay for (`moveOver`) — free actions do not keep it open.

**The Oath is a reaction like the others.** The rulebook has it "at the end of
any player's Action", so the holder is asked after every action of another
player while the High Priest stands on the map, and in Gather Power; the
reactions settings (pass for everyone / for some players) are the way not to be
asked. The bots decide as before — they sacrifice only when it matters — so
what they do is unchanged.

**Spellbook marks.** One pictogram stands for "spellbook": coloured for a
spellbook that is unlocked or a requirement that is met, greyed out for one that
is not. It replaces the ticks in the cult cards, and marks the buttons of the
actions a spellbook gives (coloured) and the actions that meet a requirement
(greyed out): the definition of an action carries its `source` (`book` /
`requirement`), the page does the rest.

**Captured units are shown.** A cult's view lists the units it holds
(`seats[n].captured`: owner, type, fate); under its Units the page shows them as
icons in the owner's colour, which disappear when they go home in Gather Power.

**The rules say how each Great Old One wakes.** `UnitView.awaken` carries the
steps from the faction pages (what must stand on the map, the cost, where it
appears), named for this map; every card of a Great Old One shows them, and the
rules window lists all eight.

## Consequences

* `leave:<unit>@a>a` costs nothing now; agents that planned for 1 Power see a
  free action.
* The Oath produces up to one prompt per seat with a High Priest per action in
  a 5-player game; people who do not want them use the reactions settings,
  and a declined or auto-passed prompt is a recorded decision like Devolve's.
* The *next Ritual* line of the round bar is gone: the marker on the track
  says it.
