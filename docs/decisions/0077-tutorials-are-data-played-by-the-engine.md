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
  code. A game in a private build, or later a data pack, brings its own.
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
  to the learner.

## Consequences

- A rule change that breaks a tutorial fails the build instead of shipping a tutorial that lies.
- The replay proves moves, not prose: what a step *says* happens is checked by its author.
- The tutorial page (playing in real rooms, refusing other moves, finishing against a bot) builds on
  this format; nothing here is visible to players yet.
