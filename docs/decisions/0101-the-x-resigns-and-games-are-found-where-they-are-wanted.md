# 0101. The ✕ of an active game resigns, and games are found where they are wanted

Status: accepted (2026-11-04). Replaces the *All games* tab of [ADR 0058](0058-all-games-who-is-here-and-the-icons.md).

## Context

The ✕ on the active games list only forgot a game in this browser. The seat stayed taken, so the table waited on somebody who
had walked away. The *All games* tab listed every unfinished game on the server, in one list, which was not much help to
somebody who wanted a seat, or a game of one kind to watch.

## Decision

**The ✕ of a game in play resigns from it, then takes it off the list, after asking.** A game that has not started has nothing
to resign: it leaves the list after asking, and the seat stays. A finished game goes without asking. The question says what
resigning costs, because a resignation ends the game for the whole table, whether that is two seats or five.

**Each game's games are in its setup**, in an *Active Games* tab, each with *Watch*. **The tables looking for players are in
Join Game**: games that have not started with a seat free, each with *Join*, without the viewer's own. `GET /games` takes
`family` and `open_seats`, so the list is not limited to the newest games of the server. It looks through at most 1000.

## Consequences

- There is no list of every game on the server any more in the page. `GET /games` still gives it.
- A rated game joined from the list still needs what it always did: the server's refusal shows in the window.
