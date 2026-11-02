# 0082. Correspondence tournaments

Status: accepted (2026-11-02). [ADR 0083](0083-a-move-stands.md) replaces what this says about taking moves back.

## Context

Step 3 of the tournaments plan (`docs/history/tournaments-and-rankings-plan.md`, on the plans branch),
whose decisions this follows. Real-time tournaments are step 4. These are the questions the plan left
open, and where building it changed something.

## Decision

- **A sweep and not a scheduler.** A round has no deadline of its own: its games' clocks bound it. A
  game that ends starts the sweep at once, and a timer runs it each minute, for a flag that fell with
  nobody looking and for what a crash interrupted. Reading a game is what ends it on time, so the sweep
  just reads the unfinished ones. One lock moves tournaments on, always taken before a room's, and a game
  ending never waits for it. A round's games and its row are written in one transaction.
- **The result is read from the game, not marked on it.** A line of `tournament_games` with no score whose
  game has finished is all the sweep needs, so nothing was added to the transaction that ends a game. The
  score is credited by account and not by seat, because a pie-rule swap changes who sits where.
- **A game is ordinary**, between two accounts on equal clocks, with `games.tournament_id` set. It need
  not be rated, so `Game.Competitive()` (rated, or in a tournament) is what refuses Auto-Play.
- **Standings are worked out on reading** from the tournament's record. There are too few games for this
  to cost anything, and nothing can drift out of step.
- **A no-show in correspondence is the clock.** The first player's clock runs from the game's start, so
  they lose on time. The plan's separate first-move deadline, and both players scoring zero, come with
  real-time tournaments, where the first-move deadline is shorter than the clock.
- **Turn mail is on by default** for an account with a confirmed address, because a correspondence game
  is unplayable without it, and the player can turn it off per game or with the mail's stop link. It goes
  through a contact row belonging to the account, which no browser holds the secret of, so nobody can
  repoint it at another address. There is no separate mail for a round starting: the first player's turn
  mail says it, and only fixed text is ever mailed, so a tournament's name never is.
- **A new capability on an older database.** The matrix is seeded once, so a capability added later would
  be missing, and missing means no. On start, any row missing is added as the file seeds it, and nothing
  the admin page changed is touched.
- **`enter_tournament` is also checked in the session layer.** MCP reaches it through one route that only
  needs `play`. Leaving needs no capability, so an account shut out can still leave.
- **Swiss rounds are capped** at what can be played without a rematch. A search that finds no pairing
  without one, in a late round of a small field, falls back to allowing them rather than leaving someone
  out, and a budget bounds that search.
- **The organiser may remove a player**, which the plan did not list, as the only remedy for one who has
  gone quiet other than waiting for their clocks.

## Consequences

- `real-time or not` is not a setting yet. Step 4 adds it, with start times, check-in and the no-show rule.
- There is no rating correction, so the fair-play rule is still stated and not enforced by any tool.
- An unplayed game forfeited because a player withdrew scores for the opponent, and counts towards
  tie-breaks like a game played.
