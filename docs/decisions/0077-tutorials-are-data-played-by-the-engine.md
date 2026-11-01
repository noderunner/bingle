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

## Consequences

- A rule change that breaks a tutorial fails the build instead of shipping a tutorial that lies.
- The replay proves moves, not prose: what a step *says* happens is checked by its author.
- The tutorial page (playing in real rooms, refusing other moves, finishing against a bot) builds on
  this format; nothing here is visible to players yet.
