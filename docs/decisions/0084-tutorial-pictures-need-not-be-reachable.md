# 0084. A tutorial's picture need not be reachable

Status: accepted (2026-11-02)

## Context

ADR 0077 reached every position by playing moves, so that the engine proves the script. It later allowed a
written-out `position`, but that needs the game's own `ext` data. A picture for an example, like a fenced
bloom, then took either a long legal game or knowledge of the game's internals, and a tutorial could not
show a mistake at all.

## Decision

- **A step's `pieces` draws the board with just those pieces**, on top of the tutorial's starting position.
  Nothing asks whether play could reach it. The guarantee of ADR 0077 stays for the game itself: `setup`,
  `play` and `reply` are still replayed, and a picture changes nothing in the room.
- **The game's check of a custom start still applies** to `pieces` and `position`, because the page's
  drawing code relies on it. A position it could not draw, like two stones on one Blooms cell, is refused.
  Skipping that check would need each game's drawing proved against nonsense, which no tutorial needs yet.
- **`illegal: true` marks a step's arrow and highlights** as what must not be done: red, with a cross. A
  step that does so asks for no moves, so the mistake is never played.
- **A picture is labelled** as an example in the status line, with no turn or result of its own, in place of
  the "Your turn" a picture used to show.

## Consequences

- `pieces` can state something false about the rules, and no test will notice. The replay proves moves, not
  prose or pictures, so they need reading.
- `pieces` cannot set a game's state beyond its board. A picture that needs units' Power or a phase still uses
  `position`.
