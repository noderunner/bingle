# 0099. Impulses: the seat to act, and the card that shows it

Status: accepted (2026-11-04). Its line "Each step is an impulse of its own" is replaced by
[ADR 0114](0114-impulses-selections-moves-and-gestures.md), which says where an impulse ends.

## Context

The games call what a player does a turn, a round, an action or a reaction, and the code had picked up as many words
for the seat being waited on: `turn_seat`, `to_move`, `your_turn`. A game such as Cthulhu Wars also waits on a player
who is not taking a turn at all, to decide on a reaction. The card that holds a player's buttons said *Your move*, and
to anyone else *Actions*, which told them nothing of what the game was waiting for.

## Decision

**An impulse is the seat or player being asked to act**, regardless of the game's terms. A game waiting on a reaction
is waiting on an impulse. Each step is an impulse of its own: choosing *Build Gate* and then an area are two.

**The name is used everywhere, including what clients and the database see.** `turn_seat` and `to_move` are
`impulse_seat`, `your_turn` is `your_impulse`, `turn_deadline` is `impulse_deadline`, and the `to-move` plane is
`on-impulse`, in Go, the page, REST, MCP, the training environments and the columns of `games`. There are no aliases:
the old names are gone, so a client has to change with it. An older database has its columns renamed when it opens.

**Some things keep their names**, because they are not the seat being waited on or could not change for free:

- `turn_order` and a game's own turn: the rotation, and a game's rules, in the game's own words.
- The position data inside `board_state`, such as a game's `Turn` field. Renaming it would mean rewriting every stored
  position, and it is a game's own data.
- What a player reads about their own moment, such as the *Your turn* mark, the notification and the mail.
- The ADRs and plans, which record what was decided at the time.

**The impulse card has the seat's name as its title and what it is asked to choose under that**, for everyone: the
player on impulse, the rest of the table and anyone watching. Somebody playing several seats also sees which seat.

**The words come from the game.** Every Go ruleset's view has `impulse`, a sentence with no name in it. `prompt` stays
as the sentence with the name, for clients that read only text, so it keeps what the card shows elsewhere, such as
Cthulhu Wars' two orders of play and the Power to spend. A game on the generic engine says what its
actions are. A step uses the action's `impulse` if it has one, or else the action's label and the kind of choice left.
*Choose an action.* is only for when a player has a list of actions to pick from.

**A step shows only for the player taking it.** A choice is held in that player's browser until the move is sent, and
telling the table would give away what they are considering and might take back. The rest see the new impulse once the
move reaches the server, which includes every forced choice, reaction and step of a battle.

## Consequences

- A client of the REST or MCP API breaks until it uses the new names.
- A server rolled back to before this change finds the columns under names it does not know, so a database that has
  been opened by this version needs its columns renamed back first.
- A sentence is addressed to the player named over it ("where your cult begins"), and everyone reads the same words.
