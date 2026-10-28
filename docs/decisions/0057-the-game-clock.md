# 0057. The game clock: a store of time per seat, charged by the move

Status: accepted (2026-10-27)

## Context

`bingle_tasks.md` asked for "a game clock (chess clock) function that can be used for all 2-player
games (or when 2 players are selected for games that support a range) with supporting timer
countdown information in the player status windows", where "the first player to run out of time
automatically forfeits", and for a setup option to turn it on with "configurable time for each
players clock (allow differences for handicap play)", off by default.

Bingle already had a **per-move** limit: `games.move_time_limit` seconds, with the turn's start
taken from the row's `updated_at`, no timer goroutine, and the game ended on time whenever anybody
reads or moves in it (`Manager.loadGame`). A chess clock is a different shape — a store of time per
player that only runs on that player's turn — but it wants the same enforcement.

## Decision

**One JSON column, `games.clock`** (an additive migration, like every column since the first
schema), holding `{"initial": {"1": 300, "2": 600}, "left": {"1": 241.7, "2": 600}, "since":
"…"}`. A game played without a clock has an empty string there, so nothing changes for the games
that exist.

**The mover is charged in the transaction that records their move.** `store.MoveInput` carries an
optional clock, written by `RecordMove`'s own transaction, so a move and the time it cost can never
disagree; `applyAndRecordAs` is the one funnel every move goes through (a person's, a bot's, a
tactical bot playing for someone whose Auto-Play Timeout ran out), so one place does it.

**The turn's start is in the clock, not `updated_at`.** `updated_at` would nearly have done — every
statement that bumps it is a turn passing — but SQLite's `CURRENT_TIMESTAMP` is truncated to whole
seconds, which would quietly cost the mover up to a second of their store on *every* move. The
clock therefore carries `since` to the nanosecond, written by the same transaction, and a turn is
measured from the later of `since` and `updated_at` (so an undo, which rewinds the game and bumps
the row, starts the new turn then rather than charging the wait to the player who undid).

**One deadline for the page.** `Manager.deadline` returns the earlier of the per-move allowance and
what is left on the player's clock, so everything already built on it — the countdown on the page,
`seconds_left` in an MCP observation, and above all `loadGame` ending the game with
`end_reason = "timeout"` and the opponent as the winner — works for a clock with no new machinery.
There is still no timer goroutine: a flag falls the moment anybody looks at the game, and the page
asks the server when its own countdown reaches zero.

**A clock or a per-move limit, never both** (`ErrInvalidClock`): two deadlines on one turn is a
riddle, not a rule. The clock is for a game of **two seats**, which covers "when 2 players are
selected for games that support a range" by itself, because each seat count of such a game is a
layout of its own (`nyet-2`, `cthulhu-wars-3p`). Each seat's time is given separately — a number in
the API gives both the same, a list gives them their own — so odds are just two different numbers.

**The page.** Every seat strip of a game with a clock carries that seat's own clock
(`.clock[data-clock="<seat>"]`), the running one marked and counting down to the turn's deadline
while the others hold still; the per-move countdown keeps its own chip (`data-clock="turn"`). The
setup has a *Game clock* field for a game of two, off by default, with a check box that opens two
times for handicap play; choosing a clock clears the time per move and the other way round.

## Consequences

* The clock is **never refunded**: an undo or a take-back rewinds the board, not the time, which is
  what a clock is for. A game created before this change has none and behaves exactly as it did.
* A bot's reply is charged to the bot's own clock, which is all but free — bots answer at once.
* No increment (Fischer) and no delay yet: `initial` and `left` per seat is the whole model, and an
  increment would be another number in the same JSON when somebody wants it.
