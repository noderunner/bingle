# 0020 — Optional per-move time limits, enforced lazily

## Context

ADR 0010 chose asynchronous play with no enforced clock. Casual correspondence
play still benefits from an optional "answer within a day" rule, and agents
benefit from knowing how long they have. There is no scheduler in the single
binary and no authentication to nudge players.

## Decision

* A game may carry `move_time_limit` (seconds; 0 = none). The clock for the
  current turn starts at the row's last update (a move, activation or undo) and
  only runs while the game is **active**.
* Expiry is **lazy**: whenever the game is read or acted on, if the deadline has
  passed the game is finished (`end_reason: timeout`, the player to move
  loses) under the room lock, so a move racing the deadline resolves one way
  or the other consistently, and a `state` event is broadcast.
* The UI shows a countdown and, at zero, re-reads the game, which makes the
  server apply the timeout and tell everyone; agents see `seconds_left` in
  observations.
* The manager's clock is injectable for tests.

## Consequences

* No background workers and no new failure modes; an abandoned timed game
  simply stays "active" in the database until someone looks at it.
* Time is judged by the server's clock; clients correct for skew with the
  `server_time` sent alongside every deadline.
* Bots move instantly and are never penalised.
