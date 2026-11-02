# 0088. Connection replaces presence

Status: accepted (2026-11-02).

## Context

Step 2 of the real-time plan (`docs/history/real-time-play-plan.md`, on the plans branch). Real-time
games need to know whether a player is still there, and the only signal bingle had was an opt-in mark
a player set by clicking their own colour box. It said what the player claimed, not what the server
could see, and it had no effect on anything. Turn mail already kept a count of open sockets of its own.

## Decision

- **A seat is connected while a socket that joined for it is open.** A second tab or device is harmless,
  a spectator's socket does not count, and neither does the socket a voice call keeps open across pages.
  The opt-in mark, its expiry, its switch and `POST /games/{code}/presence` are gone. There was never an
  MCP tool for them.
- **The server pings each game socket every 15 seconds** and closes one that has not answered within 10.
  A closed socket and a failed ping then take the same path.
- **A seat with no socket left shows as disconnected after 20 seconds.** A socket coming back cancels
  that, and a seat that had gone is marked connected at once. A reload is therefore invisible to the
  others. At worst, a connection that dies silently shows after about 45 seconds.
- **The word is "disconnected", never "away".** A connection is all the server can observe. Phones suspend
  background tabs, so switching apps can show a player as disconnected, and that is accepted.
- **A connection belongs to the person, not the seat number.** A pie-rule swap and a change of seat in the
  lobby leave a player's socket open, so the tracker's record follows them, and a seat given up stops
  counting even if its socket stays open. Turn mail uses this same record, so it now has this right too.
- **Turn mail takes no grace period.** It asks whether a socket is open this moment, as it always has: a
  player whose page has just closed should get their mail.
- **The "joined" event says who is connected**, read after the socket's own seat is counted, so that a page
  has no gap between that list and the events that follow. `GET /games/{code}/connections` says the same for
  clients without a socket.
- **A connection changing does not re-render the page.** It is painted onto the seat strips in place. This now
  happens without anybody doing anything, and a render would close a card the viewer is reading.
- **A game socket's handler stops as soon as its socket closes.** It used to wait for the room's next event to
  fail to write, so a closed page counted as open until something happened in the room. This also makes a bot
  match's "nobody is watching" and the suppression of turn mail correct sooner.

## Consequences

- The display is in every game, so an opponent in a correspondence game normally shows as disconnected. That
  is true, and it is why the label is small. Only a real-time game will attach consequences to it.
- A client without a socket shows as disconnected, MCP agents included. They choose how to show that they are
  connected when they are built.
- The page's reconnection stops at 3 seconds where the game has `real_time` set. Nothing sets that until step 3,
  which is also where it gets its test.
- It all lives in memory: after a restart every seat is disconnected until its page reconnects, which it does by
  itself.
- Nothing here reaches a ruleset, and it adds no per-room metric labels.
