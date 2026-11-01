# Writing a tutorial

A tutorial teaches a game by playing it: a few pages of prose, each asking the learner for a move or
showing one, on a real board under the real rules. A game can have any number of them, of three
kinds:

- **rules** — how to play. A big game splits its rules by topic (Cthulhu Wars: combat, spellbooks,
  gates…), so a player can come back to one small tutorial as a refresher.
- **strategy** — why a move is good.
- **variant** — what a variant changes.

Tutorials earn no achievements; which ones a browser has finished is only remembered by the browser.
The design is [ADR 0077](decisions/0077-tutorials-are-data-played-by-the-engine.md).

## Where they live

One YAML file per tutorial in the game's `tutorials/` folder, named after the tutorial's `id`
(`internal/games/blooms/tutorials/blooms-basics.yaml`), embedded by the module as
`catalog.Module.Tutorials`. A tutorial belongs to the game's family and names the game or variant it
is played on, so a private game's tutorials travel with it.

## The format

```yaml
# yaml-language-server: $schema=../../../../docs/tutorial.schema.json
id: blooms-basics
kind: rules                # rules | strategy | variant
title: Placing stones and capturing a bloom
summary: How a turn works, what a bloom is, and how one is captured.
order: 1                   # among the game's tutorials of this kind
game: blooms-4             # the game or variant it is played on
seat: 1                    # the learner's seat (default 1); the page plays the others
# seed: 7                  # required for a game with chance (Courtfall's dice)
# setup: [red@d4, ...]     # moves played before the first step, to reach a position
steps:
  - text: |                # Markdown, as in the rules pages
      Place a red stone on the highlighted cell.
    play: [red@d4]         # what the learner must play, in order, in the game's notation
    hint: Choose red, then the highlighted cell.   # said when they try anything else
    highlight: [d4]        # squares (or areas) to pick out
    arrow: [d4, e5]        # an arrow from one square to another
    reply: [blue@a4, black@g1]   # then played for the other seats
```

**Positions are moves, never documents.** Every position is reached by playing `setup` and the steps'
moves from the game's start, so nothing is written out by hand and the engine checks everything. A
step without `play` or `reply` is just a page to read.

The usual YAML of a definition applies: includes, anchors, and every field name checked with its file
and line ([dsl.md](dsl.md#the-file)). `docs/tutorial.schema.json` gives an editor completion; it is
generated from `internal/tutorial` by `go run ./scripts/genschema`.

## The guarantee

`go test ./tests/tutorials` replays every tutorial of every game as the page will: the setup, then
each step's moves for the learner's seat and its replies for the others. A move that is not legal,
made by the wrong seat, or a square that is not on the board fails the test — so a rule change can
never leave a tutorial teaching rules the engine no longer has.

The replay proves the moves, not the prose: when a step says "the blue stone is captured", play the
line once and look.
