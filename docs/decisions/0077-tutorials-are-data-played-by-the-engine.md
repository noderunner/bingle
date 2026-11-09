# 0077. Tutorials are data, and the engine plays every one in the tests

Status: accepted (2026-11-01)

## Context

The tutorials plan (`docs/history/game-tutorials-plan.md`, on the plans branch, settled 2026-11-01)
wants guided tutorials that walk a player through a game on the board, with scripted replies and the
option to finish the game against a bot. Two things decide whether that stays maintainable: where a
tutorial's knowledge of the game lives, and what stops a tutorial from going stale when a rule changes.

## Decision

- **A tutorial is a YAML document in the game's own files** (`tutorials/<id>.yaml`, embedded as
  `catalog.Module.Tutorials`), read by `internal/tutorial` with the definition's loader
  (`dsl.LoadAs`): includes, anchors, field names checked with file and line. No game has tutorial
  code. A game in a private build brings its own.
- **A game has any number of tutorials, of kind `rules`, `strategy` or `variant`**, sorted by kind
  and `order`. Big games split their rules by topic so one part can be replayed as a refresher.
- **Positions are given as moves** — `setup`, then each step's `play` (the learner's) and `reply`
  (the other seats') in the game's notation — never as position documents.
- **A game with chance names a `seed`** (`state.NeedsStartRecord`), so a scripted line plays the
  same every time.
- **`tutorial.Verify` replays a tutorial as the page will**, and a test runs it over every shipped
  tutorial: every move legal for the seat that makes it, every square pointed at on the board.
- **No achievements** for tutorials.
- `docs/tutorial.schema.json` is generated beside the definition's schema.

Added 2026-11-01, after the first tutorials were played:

- **A step can show another position** (`show`: moves from the start, for that step only), so a
  rules tutorial can begin with how a game ends. Rules tutorials follow one order: what you are
  doing, how it ends, the board, the pieces, the turn, then each phase ([tutorials.md](../tutorials.md)).
- **What the learner is to select is a signal**: the served tutorial's `guide` gives each expected
  move's selections and each shown position, and a client presents them its own way. The web page
  keeps its controls where they always are, picks out the expected one and makes the rest inert,
  rather than offering only the expected move.

- **A tutorial can begin from a written-out position** (`start`) and a step can picture one
  (`position`), checked by the game's own validation of a custom start; the moves after it are
  replayed as ever. It is for a game well into play — the Cthulhu Wars High Priests tutorial starts in
  round 2 of a four-player game — where the moves to reach it would be hundreds long.
- **Every reaction in a tutorial room is put to the script**: the server answers none (a room where
  one person holds every seat is asked everything), and a scripted reply may decline a reaction put
  to the learner. That choice is fixed for the room, whatever is saved for the seat: the learner
  cannot change it (`PUT /reactions` is refused, `GET` says `fixed`), so no reaction is passed behind
  the script's back. It is fixed for the whole tutorial rather than step by step, because every
  reaction of a scripted line is part of the script.

Added 2026-11-02:

- **Back looks back, and takes nothing back.** The learner can return to any earlier step, shown with the board from
  when that step began, but cannot play it again. The moves are real moves in a real room, and a move stands
  ([ADR 0083](0083-a-move-stands.md)). Rewinding the room would need an exception to that rule, and replaying into a new
  room would leave a room behind at every press. *Start again* still replays the tutorial from the top.

Added 2026-11-06: steps are named lessons in series, the end's buttons changed, and an account keeps the tutorials it
has finished ([ADR 0123](0123-tutorials-come-in-series-of-lessons.md)).

## Consequences

- A rule change that breaks a tutorial fails the build instead of shipping a tutorial that lies.
- The replay proves moves, not prose: what a step *says* happens is checked by its author.
- The tutorial page (playing in real rooms, refusing other moves, finishing against a bot) builds on
  this format; nothing here is visible to players yet.
