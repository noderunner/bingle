# 0079. Clock increments, and a restart costs nobody time

Status: accepted (2026-11-01)

## Context

Step 1 of the real-time plan (`docs/history/real-time-play-plan.md`, on the plans branch), which
tournaments also need. Three things stood in the way of a clock anyone could compete on: a restart
could lose a player the game, a player short of time could lose to network lag alone, and a take-back
across a pie-rule swap left each player holding the other's clock.

## Decision

- **A turn costs time from the later of its start and this server's start**
  (`Manager.turnStart`), on a clock and on a per-move limit. Nobody can play while the server is down.
  A player gets back what they had used of the interrupted turn: writing the time down as it passes
  would be far more machinery to take back a few seconds. A crash and a clean restart are the same.
- **An increment**, one number per game in the clock's JSON (`clock_increment_seconds`, at most a day),
  Fischer style: added when a player hands the turn on, after the time is charged. So:
  - a turn of several decisions earns it **once**, on the decision that ends it;
  - a move the server makes for a player (a reaction passed, Auto-Play) earns nothing, as the player
    did not act;
  - the move that ends the game earns nothing, as there is no clock left to run;
  - a pie-rule swap always earns it, because the turn passes to the other person even when the seat
    number does not change;
  - a clock can pass what it started with.
- **Taking a move back takes back what it earned.** Otherwise moving and undoing again and again would
  earn time without limit. ADR 0057's rule stands: time *spent* is never refunded, so a clock can hold
  less than the moves earned. Taking a move back never runs a clock out, which would lose the game for
  it: at least a second of what the player has stays.
- **Taking back a swap swaps the clocks back**, in `UndoToSeq`'s transaction with the seats. This was
  a bug: the seats changed back and the clocks did not.
- **The seat charged is always the seat the game was waiting on.** A move is refused from anyone else
  (`ErrNotYourTurn`), and a Cthulhu Wars reaction window names the reacting seat as next, which a test
  now holds to. That is what the clock at larger tables will rely on.

## Consequences

- A game created before this has no increment and plays as it did.
- A server that restarts repeatedly hands the player to move more thinking time. That is a fair price
  and an unusual case.
- The last move of a game ended by an agreed draw or on time does not get its increment removed if it
  is then undone, because the store does not say whether that move ended the game. A few seconds at
  most, in a game that was already over.
