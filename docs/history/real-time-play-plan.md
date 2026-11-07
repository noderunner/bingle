# Plan: real-time play

Status: **settled**, ready to implement (opened and settled 2026-11-01). Nothing is implemented.
[Tournaments](tournaments-and-rankings-plan.md) depend on the clock work here, and real-time
tournaments on all of it, but every game can use it without a tournament.

## What was asked for

- **Real-time** as a setup option on any game, in or out of a tournament. The players are expected
  to stay with the game until it ends.
- The server watches each player's connection. A player whose connection fails or who leaves shows
  as disconnected until the server hears from their client again.
- With a game timer, nothing more is needed: a player who runs out of time forfeits.
- Without a timer, the slow player is warned after a configurable time, and after a second
  configurable time the other players may have a bot play the turn.
- A player can be voted out and replaced by a bot.

## What already exists that this builds on

- **The game clock** (ADR 0057), per-move limits (ADR 0020) and lazy expiry in `Manager.loadGame`.
- **Auto-Play** (`session/autoplay.go`): a tactical bot takes every decision of a turn, including
  reactions, and marks the moves as auto-play. Its 20-second watcher is already a background
  worker, and it does not count time the server was down.
- **The room's WebSocket**, held open by the game page, and the hub that publishes to it.
- **Seat ownership by principal** (ADR 0067) and the `moves.autoplay` marking.

## What is missing today

- **The server never pings a socket**, so a connection that dies silently goes unnoticed.
- **Presence is an opt-in mark** with a 150-second expiry, renewed by the page. It says nothing
  about the connection, and it is shown and does nothing else.
- **The clock counts through a server restart**, so downtime can make a flag fall. `autoPlaySince`
  already has the rule the clock needs.
- **The clock has no increment**, so a player low on time can lose to network lag alone.
- **The clock is limited to two seats** by one check in `newGameClock`, and running out of time at a
  table of three or more ends the game with no winner, because `Opponent` finds nobody to award it to.
- **A bot can only be seated before a game starts** (`AddBotAt`).

## Decisions (settled 2026-11-01)

### The setting

1. **Real-time is a property of a game**, chosen at setup and fixed from then on. It is stored like
   the clock, in one JSON column (`games.real_time`, empty when off), and offered through the API
   and MCP as well as the page. It is independent of the timer: a game may be real-time with a
   clock, with a per-move limit or with neither, and a clocked game may be correspondence.
2. **Tutorial rooms are never real-time.** At a table with no other human, the setting does nothing:
   there is nobody to warn on behalf of, or to vote.

### Connection

3. **Connection replaces presence, in every game.** The opt-in mark, its expiry and its toggle go.
   A seat is **connected** while at least one socket authenticated for that seat is open, so a second
   tab or device is harmless and a spectator's socket does not count. The display is shown in every
   game; only real-time games attach consequences to it.
4. **WebSocket pings, not new polling.** The server pings each game socket about every 15 seconds
   and closes one that does not answer. A reconnect marks the seat connected at once.
5. **A seat shows as disconnected after a grace period of about 20 seconds**, so a reload or a brief
   drop does not flag anyone. Closing the page and a failed ping take the same path.
6. **The page reconnects faster in a real-time game.** Its backoff reaches about 45 seconds today;
   there it stops at a few seconds.
7. **The label says "disconnected", never "away".** A connection is all the server can observe.
   Phones suspend background tabs, so switching apps can show a player as disconnected, and that is
   accepted.
8. **Clients without a socket are left for later.** MCP agents and future clients choose how to show
   they are connected when they are built; until then, one without a socket shows as disconnected.

### A game without a timer

These rules apply to a real-time game with neither a clock nor a per-move limit, at a table with
at least one other human. With a timer, none of them do: time running out is the only consequence.

9. **Two warnings, set at setup:** the first after 2 minutes by default, and the second 5 minutes
   later, 7 minutes in all. The first tells the slow player. The second tells the table and offers
   Auto-Play.
10. **The wait restarts at every decision the game accepts**, as Auto-Play's does, so a long
    multi-step turn is not punished for its length. It applies to whichever seat the game is waiting
    on, reactions included.
11. **Auto-Play is an offer, never automatic.** Any one other connected human may trigger it once
    the second warning has passed, and the table may choose to wait. It uses the existing tactical
    bot, not a random move, so that triggering it does not hand the table an advantage. It plays one
    turn, marked as auto-play. Afterwards the timing starts again.
12. **Auto-Play continues while the player stays disconnected.** If the seat was disconnected when
    Auto-Play was triggered, the bot plays that seat's later turns without a wait until the player
    reconnects, when normal timing resumes. Without this, an absent opponent in a two-player game
    costs 7 minutes on every turn, and that table has no vote-kick to end it.
13. **A player's own Auto-Play Timeout keeps working**: whichever comes first plays the turn.

### Vote-kick

14. **A vote-kick passes when more than half of those counted vote for it.** Those counted are the
    connected humans plus the target, who is always counted and cannot vote. Bots and other
    disconnected players are not counted. The person opening the vote has voted yes.

    | Connected humans, target included | Votes needed |
    |---|---|
    | 2 | impossible: only one person can vote |
    | 3 | 2 (both others) |
    | 4 | 3 (all three others) |
    | 5 | 3 |

    The target counts whether connected or not. With four humans, where the target and one other have
    gone, 3 are counted and both who remain must agree.
15. **A vote expires after 2 minutes.** Only one may be open against a player. It lives in memory, as
    presence did; a restart forgets it. Whoever opened a failed vote may not reopen it against the same
    player for 5 minutes (a default chosen here to stop badgering, not discussed separately).
16. **A kick is immediate and final.** A tactical bot takes the seat, the game resumes from that
    position, and the player cannot take a seat in that game again, though they may watch. The ban is
    held by principal, or by name for a seat nobody owns.
17. **No vote-kicks in rated games**, which covers tournaments.
18. **The vote is available in the API and MCP**, not only on the page, like every other action.

### Clocks

19. **The clock works at any number of seats**, and a seat's clock runs while the game is waiting on
    that seat. It already follows the next seat the game names, which should cover reactions; a test
    with a multi-seat game must establish it before the limit goes.
20. **Out of time at a table of three or more seats, a tactical bot takes the seat**, by the same path
    as a kick, and the game goes on. Ending the game would punish the whole table, and removing the
    seat would mean teaching every ruleset about elimination. This covers per-move limits too. **Two
    seats keep the forfeit.** If no human remains, the game ends with no winner.
21. **A bot's clock does not run** once it has taken a seat.
22. **A restart costs nobody time.** A turn in progress is measured from the later of its start and
    the server's start, the rule Auto-Play already uses. The current player gets back whatever they
    had used of that turn, which is a small gift and far simpler than journalling the time.
23. **The clock gets a Fischer increment**, one number per game in the same JSON, added when the turn
    passes to another seat, offered at setup. A game without one is unaffected.
24. **A flag must fall with nobody looking.** Expiry stays lazy for correspondence play, and a watcher
    also looks at running real-time and tournament games. It may share Auto-Play's loop at a shorter
    interval; a query limited to those games keeps it cheap.

### The rules engine

25. **Nothing here reaches a ruleset.** `Legal` and `Apply` stay pure. Connection, warnings, votes and
    takeovers live in `internal/session`, and every consequence is an ordinary move or a change of who
    holds a seat.

## Replacing a player, in the data

A takeover turns the seat's `players` row into a bot seat and records who was replaced, when and why
(`kicked`, `timeout`). The record of the game must still show who played the moves before it, and a
bot seat that goes on to win is credited to the bot, not the person. A kicked player's game leaves
their active list. Where the existing move marking and player row cannot carry this, the smallest new
table does.

## Order of work

Each step is its own pull request, with an ADR where it settles a question:

1. **The clock's fixes:** restart credit, the increment, and the test of who is charged during a
   reaction. Useful alone, and needed by correspondence tournaments.
2. **Connection:** pings, the grace period, faster reconnects, and presence replaced.
3. **The setting, warnings and Auto-Play offers.**
4. **Taking over a seat:** the vote-kick, then the clock at any number of seats, which both use it.

## Dependencies and cross-plan effects

- **Tournaments** need step 1 for any tournament, and steps 2 and 3 for real-time ones.
- **ADR 0010 and ADR 0020** said no enforced clock and no background worker. Auto-Play already moved
  on from the second; the ADRs here should say so.
- **Observability:** the watcher needs a metric and a log, like Auto-Play's. Connection counts stay
  in memory and must not add per-room metric labels.
- **Bevy client:** must be able to do everything here through the API. Its connection signal is
  decision 8.

## Status log

- 2026-11-01: opened and settled in discussion with the maintainer, as the foundation for real-time
  tournaments. Vote-kick by more than half; tactical bot, never random; a bot takes the seat when time
  runs out at three or more seats; WebSocket pings; presence merged into connection.
- 2026-11-01: step 1 built (ADR 0079, branch `clock-fixes`): restart credit on clocks and per-move
  limits, and the increment. Found along the way: taking a move back must remove the increment it
  earned, or undoing earns time without limit, and undoing a pie-rule swap left the clocks swapped;
  both fixed. A move is only accepted from the seat the game waits on and a reaction window names the
  reacting seat, so decision 19's question is answered for the code as it stands. Decided: the server
  playing a move for someone earns no increment, and a take-back never runs a clock out.
- 2026-11-02: step 2 built (ADR 0088, branch `connection`): pings, the grace period and presence replaced.
  Found along the way: a game socket's handler only noticed a closed socket at the room's next event, now
  fixed; a connection must follow its player through a pie-rule swap or a lobby seat change, which step 4's
  takeover should remember. Turn mail now shares the record and takes no grace period. Decision 6's faster
  reconnects read `real_time` from the game, which step 3 adds, and step 3 owes that its test.
- 2026-11-02: step 2 merged (#75), and step 3 built (ADR 0089, branch `real-time-setting`): the setting, warnings,
  Auto-Play offers and the flag watcher of decision 24. Decided here: each warning 30 seconds to an hour; asking for
  the bot takes a seat that shows as connected, so an MCP agent with no socket is refused. Left for step 4: the
  takeover, the vote-kick and clocks at larger tables. Real-time tournaments can now start.
