# 0157. A moderator removes a rated game from the ratings

Status: accepted (2026-11-27), for issue #240. Gives the `moderate` capability of [ADR 0071](0071-capabilities-modes-and-tokens.md)
its first use, and settles what [ADR 0080](0080-rated-games-and-results.md) left out: a tool to remove a rating.

## Context

The fair-play rule of rated games is enforced by moderators, not by detection (the tournaments plan, decision 19), and
there was no tool for them. Ratings are written as games end and kept in `ratings` and `rating_history`, because
retention deletes old games. The maintainer settled the issue's questions: a moderator removes a game's effect on the
ratings at their discretion; ratings are recomputed when possible; tournament standings are annotated, for everybody,
with the change and the moderator who removed it; the audit log records it and the player is told; it lives on the admin
page.

## Decision

- **What is removed is one rated game, for both its players.** The game's effect on the ratings is the change it made to
  each of them, and a recomputation of the other player's rating was the issue's own question, answered yes. The two
  `rating_history` rows stay, marked by a row in `rating_removals` (room, moderator, when, recomputed or not), and every
  later replay skips them. The game, if retention has kept it, gets `rating_status = removed`, and its page says *Not
  counted*. A game is removed once; there is no undoing it.
- **Recomputing is possible when the history accounts for the ratings.** A rated game is replayed from its history rows
  alone: who played whom, the score, and when, which is all `recordResult` used, since a rating ages by the time between
  games. So the family's ratings are replayed from its first game in the order the rows were written, from new players'
  ratings, skipping removed games. *Possible* means that this replay, before the removal, reproduces every stored
  rating of the family: the same accounts, games counted, and rating, deviation and volatility within one part in a
  million. Then the family is replayed without the game, and every rating it changes is written: both players', and those
  of anybody whose later games were against them or against somebody who was. A player left with no rated game in the
  family loses their rating there. What is written is itself a replay, so the next removal can recompute too.
- **Otherwise only the game's own change is reversed.** When the history does not reproduce the ratings (an earlier
  removal that had to reverse, or a family a rename merged), each player's current rating loses the game's change (after
  minus before); deviation, volatility, the time of the last game and every other rating stay as they are. That is the
  one change the stored numbers can be trusted to say the game made.
- **Either way** each player's games and won, drawn or lost counters lose the game, and their count of opponents is
  counted again without it. Badges already earned stay. The rule is deterministic: the same history and ratings give the
  same result, and no result is written while the ratings are rewritten.
- **A tournament's result stands.** Its points and places are its own record, unchanged. Each affected player's line of
  the standings carries the change removed and the moderator's handle (`rating_removals`), shown to everybody under the
  player's handle. The maintainer chose to name the moderator publicly; no earlier decision keeps staff names private.
- **The audit log** gets a `rating-removed` entry under the moderator's account: the game, the room, each player's rating
  before and after, and whether it was recomputed (with how many others moved) or reversed.
- **The players are told by mail**, the channel a friend request and a tournament use to tell an account something that
  is not chat: each of the two, at a confirmed address, when the server sends mail. It names the moderator, the game and
  the opponent, and links the player's page. There is no other notice: an account without a confirmed address, or a
  server without mail, tells nobody.
- **Who.** `GET /api/rated-games?player=` lists an account's rated games and `POST /api/rated-games/{code}/remove`
  removes one; both need `moderate`, which the admin group holds and no token scope carries. An administrator who still
  owes a second factor off the server's machine is held back on both, the read too, since it says when and against whom.
  The session layer checks the capability as well, so a future door obeys.
- **Where.** A *Ratings* card on `/admin`: a player's handle, their rated games, and *Remove*, which asks first. The card
  shows to whoever holds `moderate`; a moderator who is not an administrator sees it alone, without the refusal the rest of
  the page gives them. That widens ADR 0071's "the admin page is the admin group's" by this one card.

## Consequences

- The capability matrix is per class, so a moderator who is not an administrator exists only if the whole `user` class is
  given `moderate`. Moderators as a group of their own would be a change to ADR 0071.
- `RecordRatedGame` no longer counts a removed game when it decides whether an opponent is new, so the replay and the
  stored count of opponents agree.
- A removal replays the whole family's history, which is a few thousand rows for a busy game: a moderator's rare action,
  under the results lock.
