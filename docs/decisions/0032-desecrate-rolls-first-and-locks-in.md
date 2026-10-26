# 0032. Desecrate rolls first; a rolled die locks the move in

## Context

A playtest read the King in Yellow's Desecrate as "roll a die against the units
in his area" and was surprised to be shown a list of units *before* anything
happened — and to find *Undo my move* offered after the die had been cast. The
card says: roll 1 die; at or under the number of your units in the area
(the King included), place a Desecration token; whether you succeed or fail,
place a Monster or Cultist with a cost of 2 or less. The engine had it right
but in the wrong order for the player: the unit was part of the action, chosen
first, and the roll came as a surprise behind a choice. The server refused to
take a rolled move back (the dice generator is part of what `CanTakeBack`
compares), but the page did not know and kept offering it.

## Decision

**Desecrate is one action that rolls, and the unit is chosen after the roll.**
`desecrate` pays 2 Power and rolls at once; the Monster or Cultist is a prompt
that follows (`desecrate:<unit>`), skipped when the pool holds one kind only or
nothing. The prompt is asked before a spellbook the glyph earns.

**A move that rolled a die is marked.** The session compares the dice state of
the position before and after each move of a Go ruleset (`CanTakeBack`) and
stores the answer with the move (`moves.locked`); the move list and the move
events carry `locked`, and the page does not offer *Undo my move* when the
player's last move, or any move after it, is locked. The refusal on the server
stays: a client that ignores the mark gets 409. What the same seat decides
straight after the roll (the unit Desecrate places, a result it assigns) is
locked with it: nobody takes back a choice made on seeing the die. Another
seat's move ends the chain.

**The cult cards carry what the cards carry.** Yellow Sign's card starts its row
of numbers with the Desecration tokens in play (a pictogram of the map's chip),
and every Great Old One's card lists its abilities as data
(`unitSpec.Abilities`): the rules window and the hover and click cards show
Desecrate, Vengeance, Devour, The Harbinger, Avatar, Eternal, Ferox and
Lethargy with their timings.

## Consequences

* `desecrate:<unit>` is no longer an action of the turn: it answers the prompt.
  The fixed action space (agents) is unchanged — both notations are in it.
* A game stored before the change has no locked flags: the page may offer Undo
  across an old roll, and the server refuses it.
