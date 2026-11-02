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
    play: [red@d4]         # what the learner must play, in order, as the move list writes it
    hint: Click the highlighted cell.   # shown when they ask for a hint
    highlight: [d4]        # squares (or areas) to pick out
    arrow: [d4, e5]        # an arrow from one square to another
    reply: [blue@a4, black@g1]   # then played for the other seats
  - text: This is how a game ends.
    show: [red@e5, blue@d4, ...]   # a picture: another position, for this step only
```

**Positions are moves, never documents.** Every position is reached by playing `setup` and the steps'
moves from the game's start, so nothing is written out by hand and the engine checks everything. A
step without `play` or `reply` is just a page to read.

**A step can show another position** with `show`: moves from the game's start, played for that step's
picture only — the room's game is untouched. It is how a tutorial shows the end of a game before
teaching the start. A step that shows a position asks for no moves. A finished game to show is easiest
taken from bots playing one out; the moves are then fixed in the file, and checked like the rest.

**Or write a position out.** Where reaching a position by moves is impractical — a four-player game in
its second round — a tutorial can begin from a position document instead, `start` (with `start_seat`
for a game whose position does not say who moves), and a step can picture one with `position`. A
document is checked as any custom start is (the game's own validation), carries its own state of
chance, so no `seed` is needed, and every move after it is still replayed against the engine. Keep a
long one in a file of its own:

```yaml
start: !include positions/round-2.yaml   # beside the tutorial, under tutorials/positions/
```

The easiest way to write one is to let bots play to roughly the right moment, take the position
(`bingle dsl convert -` turns its JSON into YAML) and edit it: who has which units, where, and how much
of what they spend. `internal/games/cthulhuwars/tutorials/` is the example: the High Priests tutorial
starts from a written-out round 2.

**Reactions.** In a game where a player can react on another's turn (Cthulhu Wars' Unspeakable Oath),
every reaction in a tutorial room is asked — none is answered by the server — so the script decides
each one: the other seats' answers are in `reply`, and so may be the learner's routine declines,
which the page plays for them. A reaction worth making is the learner's own `play`. The choice of
when to be asked is fixed for the whole room: `GET /reactions` reports `fixed`, `PUT` is refused, and
the page offers neither the reaction settings nor the "stop being asked" shortcuts under a question,
since a reaction passed for the learner would leave the script behind.

## Writing a rules tutorial

A rules tutorial teaches the whole game in this order, one step each unless a part needs more:

1. **What you are doing**, in one or two sentences.
2. **How the game ends**: `show` a finished game, and explain how it ends and what wins.
3. **The board**, and any other play areas or zones.
4. **The pieces**, and any cards or resources.
5. **The turn**, and the rounds if there are any — usually the learner's first move.
6. **Each phase of the game in turn**, one step each, with the rules and mechanics unique to it — the
   learner making the moves that show them, the replies answering.

A big game splits its rules across several rules tutorials by topic, each following the same order
for its part.

The usual YAML of a definition applies: includes, anchors, and every field name checked with its file
and line ([dsl.md](dsl.md#the-file)). `docs/tutorial.schema.json` gives an editor completion; it is
generated from `internal/tutorial` by `go run ./scripts/genschema`.

## Tutorial rooms

`POST /api/tutorials` opens a tutorial in a real room ([api.md](api.md#games)): the game it names,
from the game's start with its chance seeded by `seed` and the `setup` played, the learner in their
seat and in every other one (`Lia · Blue`, the solo seats of ADR 0029). The page plays the learner's
moves as the learner and each `reply` as the seat that makes it, through the ordinary move endpoint,
so the rules are the engine's. The room is marked as a tutorial (`tutorial: "family/id"` in
`GET /api/games/{code}`) and stays out of "My games", the open games and turn notifications.

## Where players find them

- **The game picker** marks the games that have a tutorial with a *Tutorial* badge, and the chosen
  game's setup lists its tutorials under **Learn to play**, started under the name in the form.
- **The rules window**, from the game picker or a game, has a **Tutorials** tab for a game that has
  any: by kind (*Learn the rules*, *Strategy*, *Variants*), each with its summary and a *✓ Done* mark
  once this browser has finished it.

## The page

A tutorial room's game page shows the step above the board, its highlighted squares ringed and its
arrow drawn, and a shown position in place of the game's. The chat and the voice call are left out:
there is nobody to talk to.

**What to select is a signal, not a page of its own.** The tutorial the API serves carries a `guide`
for each step: the selections each of the learner's moves is made of (`piece red`, then `pos d4`) and
the position a step shows ([api.md](api.md#games)). A client shows them its own way. The web page
keeps every control where it always is — the colour buttons, *End turn* — picks out the one to press,
makes the others inert, and lets only the square to click be clicked. *Need a hint?* shows the step's
`hint`. When the learner's moves are made, the page
plays the step's `reply` for the other seats, a moment apart so they can be followed, and moves on.
A step without `play` waits for **Next** (**Finish** on the last). Where the learner is comes from the
moves the room has played, so a reload, another tab or a phone find the same step; moves that are not
the script's (an undo) leave the tutorial *off script*, with a way to start again.

At the end the tutorial is marked done in the browser, and **Finish this game against a bot** starts
an ordinary game from the position on the board.

## The guarantee

`go test ./tests/tutorials` replays every tutorial of every game as the page will: the setup, then
each step's moves for the learner's seat and its replies for the others. A move that is not legal,
made by the wrong seat, or a square that is not on the board fails the test — so a rule change can
never leave a tutorial teaching rules the engine no longer has.

The replay proves the moves, not the prose: when a step says "the blue stone is captured", play the
line once and look.
