# 0081. Clock minutes, and a default time for each game

Status: accepted (2026-11-02)

## Context

The setup offered a fixed list of clock times, and the same list for every game, though a bigger board
makes for a longer game. Tournament setup will need the same control.

## Decision

- **The setup takes any whole number of minutes from 1 to 60**, for both players or for each one. A
  value outside that is moved to the nearest one the box allows, where the player can see it.
- **2 hours and 1 day stay on the list.** Correspondence play is a settled option in the tournaments
  plan, and 60 minutes cannot express it. 1 hour is now just 60 minutes. The API's limits do not change:
  any time up to 30 days.
- **`default_clock_minutes` in a definition**, 1 to 60, is the time the setup suggests. A variant sets
  its own in its merge patch. A value out of range stops the game loading. Without one the page suggests
  10 minutes, the time a rated game already fell back on.
- **The default does not turn a clock on.** A clock forfeits a player who walks away, so a player who
  did not choose one should not get one by surprise. It is only the time the setup starts from, and the
  API never applies it. A rated game, which must have a clock, uses it when none is chosen.
- **A time the player typed stays** when they switch game or board size. An untouched box follows the
  game.

## Consequences

- A game is free to declare a default whatever it seats, as a clock is only offered at two seats today.
  The real-time plan's clocks for bigger tables can use the same field.
- Only Blooms names a time so far (10, 15 and 25 minutes by board size). These are estimates, not
  measurements, and other games suggest 10 until they name their own.
