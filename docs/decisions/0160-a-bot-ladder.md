# 0160. The bot ladder: the bots that think are rated nightly in each game, apart from people

Status: accepted (2026-11-28), for issue #239. The last of the four formats the tournaments plan
([tournaments-and-rankings-plan.md](../history/tournaments-and-rankings-plan.md)) left for later. Builds on the
practice batches (ADR 0071's `run_batch`, `internal/batch`) and the Glicko-2 ratings of
[ADR 0080](0080-rated-games-and-results.md).

## Context

The plan keeps bots out of people's ratings, so that the first strong bot does not distort them, and notes that the
batches already measure how strong a bot is. The issue asked where a bot ladder shows, when it runs and which bots take
part. The maintainer decided: it shows in the Ratings tab and the bot guide only, not in a game's opponent list; it runs
on a schedule set by a server setting, every midnight UTC unless set otherwise; and only the basic Tactical, Minimax and
MCTS bots take part.

## Decision

- **Its own table.** `bot_ratings` holds one row per bot per game family: rating, deviation, volatility, games, won,
  drawn, lost. It has no account in it, and nothing that reads people's ratings (`ratings`, `rating_history`, the
  leaderboards, a profile, a seat's rating) reads it; nothing that writes a person's rating writes it. A bot therefore
  cannot appear among people, and a bot's game cannot move a person's rating.
- **Who plays.** Tactical, Minimax and MCTS, each at the strength players meet in a game (Minimax at its 30,000
  positions a move, MCTS at its second a move). Random is left out. A bot a game or a pack registers is not on it.
- **Which games.** Every game family of two seats that is not a prototype, played as the game of the family's own id
  (`hex-11` for Hex, `chess` for Chess; the first of the family where there is no such game), and only where a batch can
  set it up with nobody choosing anything (`batch.Spec.Validate`). The ladder knows nothing of any game: a game a pack or
  the private overlay adds is on it by the same rule. One rating per family, as people's ratings are; a variant is not
  rated apart. Tables of more than two are left out, as they are from rated play.
- **A run.** Each of the three pairings plays `BINGLE_BOT_LADDER_GAMES` games in each family (2 by default: one each way
  round, the batches' seat rotation), as `batch.PlayGame` plays a batch's game. A game is cut at 300 plies and then
  counts as a draw; a game the rules failed (no legal move, or an illegal one) is not counted. The run is one Glicko-2
  rating period: each bot's rating in a family moves once, by all its games there that run, against its opponents'
  ratings before the run. A bot starts at 1500 and is shown provisional above a deviation of 110, as a person is.
- **The schedule.** `BINGLE_BOT_LADDER` is a time of day in UTC, `HH:MM` (default `00:00`), or `off`. Once a day is
  enough for ratings that move slowly, and a time of day needs no cron syntax and no library. The setting is checked at
  start, as the others are.
- **What a run may cost.** A run plays only while no batch does: it takes the batches' turn (`batch.Manager.Hold`), so a
  batch waits for it and it waits for a batch. It plays on as many workers as a batch takes by default (three quarters of
  the CPUs). `BINGLE_BOT_LADDER_LIMIT` (1 hour by default) bounds the whole run, the wait for a batch included: what was
  played by then counts, and the rest is not played. A run plays the families whose ratings are oldest first (those
  never played before all), so a run its limit cuts short is followed by one that starts where it stopped. One run at a time: the next is worked out when one ends, so a run that
  overruns its next start skips that one rather than queueing it.
- **Where it shows.** `GET /api/bot-ratings` lists each family's bots, the highest first. The Ratings tab shows it below
  the people's leaderboard, a game at a time; the guide to the bots has a section on it ending in the same table. A
  game's setup does not show it.

## Consequences

- **What a run costs, measured** (2026-11-28, a machine of 16 CPUs, so 12 workers, at the bots' full strength). One
  family is 6 games. Yavalath took 2 minutes of game time (6 to 47 seconds a game), Hex 11×11 10 minutes (1 to 2 minutes
  a game) and Chess 14 minutes (1 to 3.3 minutes a game; one reached the 300-ply cap, at under 3 minutes); with 12
  workers the three families took 3 minutes 21 seconds of wall time. Most of it is MCTS's second a move and Minimax's
  30,000 positions, which is what the strength players meet costs. The default run on the public build is 18 families,
  108 games: on this machine it had not finished after 20 minutes, when it was stopped, so the bigger boards (Go,
  Havannah, Amazons) cost more than these. A server of two CPUs plays one game at a time and gets through about 4 to 6
  families of Hex's or Chess's size in the hour, so a full rotation of the 18 takes about four nights there, and one
  night on a large machine.
- When the priority for live games (#316) and the limits on batch jobs (#317) are built, the ladder is background work
  like a batch: it should take its CPU from the same shared budget, give way between games (it already plays a game at a
  time per worker, so a ply or a game is its natural point to ask again), and come under the same ceilings on workers,
  search and time. Its own bounds (games a pairing, the run's limit) are the kind #317 asks for, set by the operator.
- The bots' ratings say how strong each bot is against the others in that game, not against people: the scale is the
  same Glicko-2 scale, but the two pools never meet, so a bot's 1600 and a person's 1600 are not comparable.
- A ladder that should run more often, rate variants apart, or include more seats or other bots is a change to
  `internal/botladder` alone; the table and the route stay.
