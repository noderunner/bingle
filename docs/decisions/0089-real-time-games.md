# 0089. Real-time games

Status: accepted (2026-11-02).

## Context

Step 3 of the real-time plan (`docs/history/real-time-play-plan.md`, on the plans branch), which real-time
tournaments also need. ADR 0088 made a connection something the server can see. This attaches consequences to a
game whose players are expected to stay.

## Decision

- **Real-time is chosen at setup and never changes**, stored as JSON in `games.real_time`, empty when off. It is
  independent of the timer, and offered through the API and MCP as well as the page. A tutorial room is never
  real-time, and at a table with no other person it does nothing.
- **With a timer, time running out is the only consequence.** The watcher only makes a flag fall on time. This takes
  back what ADR 0010 and ADR 0020 said about having no background worker, as Auto-Play already had.
- **Without one, two warnings**: 2 minutes by default, then 5 more, each 30 seconds to an hour. The first is for the
  player, the second for the table. They are measured from `updated_at`, which every accepted decision bumps and a
  draw offer does not, so a long turn of many steps is not punished and an offer cannot buy time. Time the server was
  down does not count.
- **A warning is an event, given once, and the snapshot also says the stage**, so a page that reconnects needs no
  history. The watcher looks every 5 seconds, which is as late as a warning or a flag can be.
- **Past the second warning, any other connected person may have the tactical bot play that one turn**, through
  `POST /games/{code}/auto-play-turn`. It is an offer and never automatic. The moves are the player's own, marked as
  auto-play, and the timing then starts again. A player's own Auto-Play Timeout goes on working, and whichever comes
  first plays the turn.
- **Asking takes a connected seat.** Someone with no open socket is not at the table in the only sense the server can
  see. The MCP tool exists for an agent that also holds a socket, and otherwise says why it is refused.
- **If the player's seat shows as disconnected when the bot is asked, the bot goes on playing their turns** with no
  wait, from the same loop as the seated bots, until a socket for that seat opens. Without this an absent opponent in a
  two-player game costs 7 minutes every turn. This lives in memory: a restart forgets it, and the warnings begin again.
- **A warning is painted in place**, as a connection is (ADR 0088): it arrives with nobody doing anything, and a
  render would close a card the viewer is reading.

## Consequences

- A rated or tournament game always has a clock, so none of the warnings or offers can reach one.
- At a table where the slow player is the only one connected, nobody can ask, and the game waits. That is decided: the
  bot is never automatic.
- The page's faster reconnect from ADR 0088 now has the setting it reads.
- Taking over a seat for good, the vote-kick and clocks at larger tables are step 4. A flag falling at three or more
  seats still ends the game with no winner until then.
- Bounds on the warnings were chosen here and not discussed: they are easy to change.
