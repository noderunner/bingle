# 0091. Taking over a seat

Status: accepted (2026-11-02).

## Context

Step 4 of the real-time plan (`docs/history/real-time-play-plan.md`, on the plans branch). A real-time table had no way
to go on without somebody who has stopped playing, short of having a bot play their turns one at a time. A game clock
was limited to two seats, because running out of time at a larger table ended the game for everyone: `Opponent` finds
nobody to award it to.

## Decision

- **A tactical bot can take a seat for good.** The row in `players` becomes the bot's, with the bot's name and no owner,
  and a new table, `seat_replacements`, records who held it, why (`kicked`, `timeout`) and at which move. Moves are
  recorded by seat, so the history reads that record to show who played the moves before it. A win after that belongs to
  the seat, whose holder is now the bot, and the game leaves the replaced person's list because they own no seat in it.
- **A vote passes with more than half of those counted**: the connected people plus the target, who always counts and
  cannot vote. It is for real-time games that are not rated. It takes two other connected people to open one, as one
  voter could never make a majority. A vote lapses after 2 minutes, only one may be open against a person, and whoever
  opened a failed vote waits 5 minutes to open it again. There is no vote against: not voting is how to vote against.
- **A person is voted on, not a seat.** Someone who plays several seats of one game has one vote, counts once, and loses
  all of them. Votes follow the person, so nothing else needs to know about seat numbers.
- **A vote counts while its voter is connected, and the count follows who is connected now.** The watcher looks at open
  votes every 5 seconds, so somebody leaving can pass a vote, and a voter who has gone does not decide anything. The plan
  did not say this; it follows from "those counted are the connected people".
- **Votes live in memory**, as a connection does: a restart forgets them.
- **The clock works at any number of seats from two up**, and runs for the seat the game waits on, which is the reacting
  seat during a reaction (tested with a Cthulhu Wars reaction). A game for one still has no clock.
- **Out of time at three seats or more, a bot takes the seat and the game goes on**, on a clock or a time per move. With
  no other person left at the table the game ends with no winner. Two seats keep the forfeit.
- **A seat a bot holds has no clock.** It is removed from the clock's JSON, and a seat not on the clock has no deadline.
  A bot seated from the start keeps its clock, which has always been so at two seats, and at a larger table simply loses it
  if it runs out.
- **A takeover restarts the turn only if the game was waiting on that seat.** The turn's start is read from `updated_at`,
  so bumping it for anybody else's seat would give the player to move their time back.
- **A takeover found on a read plays on in the background**, because a request that only asked to look must not wait for
  a bot to think.
- **An outstanding draw offer goes**, as a bot never answers one.
- **Replaced means no seat in this game again**: by principal, or by name for a seat nobody owned. They may watch.

## Consequences

- The vote's question comes up only with three or more people, so it can never reach a two-seat game, and rated games and
  tournaments are refused outright. Multi-seat tournaments, when they are designed, will have to say how a seat a bot took
  is scored.
- An agent with no socket shows as disconnected, so it can neither vote nor count, as with the turn on offer (ADR 0089).
- `clock_seconds` given as one number now means every seat, not two.
- The record is read with every snapshot of a table of three or more. Smaller tables skip the query, as they cannot have one.
- Nothing here reaches a ruleset.
