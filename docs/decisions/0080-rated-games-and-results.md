# 0080. Rated games, and a result written from a mark

Status: accepted (2026-11-02). [ADR 0083](0083-a-move-stands.md) replaces what this says about taking moves back.

## Context

Step 2 of the tournaments plan (`docs/history/tournaments-and-rankings-plan.md`, on the plans branch):
ratings, worth having before any tournament exists. Its decisions 14 to 24 are the design; this says
how they were built and where the building settled something the plan left open.

## Decision

- **Glicko-2 in `internal/rating`**, pure and with no dependency, held to the worked example in
  Glickman's paper. A game is a rating period of its own, and a deviation grows by the time since the
  player's last game, at one period's worth a week, up to a new player's.
- **A rated game is chosen at setup and fixed** (`games.rated`). It takes a signed-in account, two
  seats, no bots, the same game clock for both and the standard start, and a joiner must be a second
  account. Equal clocks and the standard start go beyond the plan: a handicap or a chosen opening
  would make the result mean less, and would be an easy way to arrange one.
- **Undo, take-backs and a player's own Auto-Play are refused** in the session layer, so every door
  obeys. Undo can reopen a finished game, and a rating is written once.
- **A result is written from a mark.** `RecordMove` and `FinishGame` set `rating_status = 'pending'`
  on a rated game in the transaction that ends it. `Manager.finished`, which every ending that has a
  result now goes through, starts `RecordResults`, away from the room lock, and `RunResults` looks
  again at start-up and every minute. So a move never waits for a rating, and a crash or a failed write
  delays a result and cannot lose one. A retired game has no result and is never marked.
- **One transaction writes a result**: both ratings, both history rows, the counters and the game's
  status, and it writes nothing unless the game is still pending, so a result cannot count twice.
- **What does not count.** A game unless both seats made a move of their own (`no_moves`): that is the
  plan's no-show rule, applied to every rated game, because resigning at once is the cheapest way to
  hand somebody a win. More than 5 games in 24 hours between the same two accounts in a family
  (`daily_limit`). A game whose seats are not two accounts (`void`). None of these touch a rating, the
  history or the counters.
- **`stats` counts rated games only**, narrower than the plan's wording. An ordinary game can be undone
  after it ends and finished again, which would count it twice, and a counter anybody can inflate
  against a bot is no use to the achievements plan either. Achievements can add unrated counters when it
  has settled what it needs them for.
- **A leaderboard is public and a record is not.** `GET /ratings/{family}` shows the handle, rating and
  number of games, and `rating_history` has no reader that says when or against whom. A profile adds
  won, drawn and lost. A rating below the bar of 5 games against 3 opponents is visible only to its
  owner, except at the seat of a rated game, where the opponent sees it.
- **The numbers** are the plan's defaults, as constants in `internal/rating`.

## Consequences

- **The fair-play rule is stated, not enforced.** Nothing here detects an outside engine, and no tool
  yet removes a rating. The `moderate` capability still waits for the feature that uses it.
- **Left for the tournaments step:** `rating_history` has no tournament column until one exists, and
  the pie rule defaulting on is a tournament setting; an ordinary rated game offers it as any game does.
- A rated game in a family that later leaves the build keeps its rating and its name, which is why
  `family_name` is stored. Reading such a game's room is ADR 0076's business.
- Every state read of a rated game costs a few more queries. An unrated game costs nothing, not even the
  check on a move beyond one `CASE` in an `UPDATE`.
