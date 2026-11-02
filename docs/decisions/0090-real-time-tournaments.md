# 0090. Real-time tournaments

Status: accepted (2026-11-02).

## Context

Step 4 of the tournaments plan (`docs/history/tournaments-and-rankings-plan.md`, on the plans branch), on top of
[ADR 0082](0082-correspondence-tournaments.md) and the real-time games of [ADR 0089](0089-real-time-games.md). Pairing,
clocks and standings are shared with a correspondence tournament. These are the questions the plan left open.

## Decision

- **It starts by itself.** The organiser sets `starts_at`, and there is no Start button: a tournament that keeps its
  time is the point of having one. The start must be in the future when it opens.
- **A `check_in` state**, for the last 15 minutes by default. Entries close when it begins, because a new entry at that
  point has nothing to wait for. Whoever has not checked in by the start is deleted from the tournament rather than
  withdrawn, since they played nothing.
- **Too few checked in abandons it**, saying why, and it stays in the list so that those who entered can see what became
  of it.
- **The sweep runs every 5 seconds for real-time tournaments**, and still every minute for the rest. A game ending still
  starts it at once. The watcher in ADR 0089 lets a game's own flag fall, and the games are real-time games.
- **The second game of a pairing is made when the first ends**, not ahead of time. Its line waits with no room code, so
  the schedule can show it as still to come, and a withdrawal in the meantime settles it as it would in a new round.
- **A break of a minute by default between rounds, from 0 to an hour**, measured from the end of the round's last game
  and stored, so a restart does not shorten it. A round nobody played has none.
- **The first-move deadline is per player**: two minutes by default, or half the clock if that is shorter, and always
  shorter than the clock. The first player's counts from the game's start and the second player's from the first
  player's move. Once both have moved only the clock matters. A restart gives it back, like the clock.
- **"Both miss it" means the first player missed it and the other seat shows as disconnected.** The second player has no
  move to miss while the first has not moved, so a connection is the only evidence that they came. Then nobody scores,
  with `end_reason: "no_shows"`. Otherwise the player who missed it forfeits (`no_show`). A rated game with a seat that
  never moved already leaves both ratings unchanged (ADR 0080).
- **A player on their finished game's page is told of the next game**, by an event in that room: nothing else reaches
  them, and their first-move deadline has started. A player on the tournament's page sees a link, as that page reads
  itself every 5 seconds.
- **Turn mail stays on as in a correspondence tournament.** It takes no socket open, so a player between pages may get a
  mail for a game that is waiting on them, which is what it is for.

## Consequences

- A server down across a tournament's whole check-in opens it and starts it at once when it comes back, with nobody
  checked in, and so abandons it. Moving the start would be guessing what the organiser wants.
- A second player with a page open only on the tournament page shows as disconnected, so if the first player misses the
  deadline too, neither scores. They have the 5-second link and the mail to bring them to the game.
- Nothing here changes a correspondence tournament.
