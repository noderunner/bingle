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
kind: rules
title: Placing stones and capturing a bloom
summary: How a turn works, what a bloom is, and how one is captured.
order: 1
game: blooms-4
seat: 1
steps:
  - text: |
      Place a red stone on the highlighted cell.
    play: [red@d4]
    hint: Click the highlighted cell.
    highlight: [d4]
    reply: [blue@a4, black@g1]
  - text: A bloom with no empty cell beside it is fenced.
    pieces: [blue@g1, blue@f2, black@f3, black@f4, yellow@g4, red@g2, red@g3]
    highlight: [g2, g3]
  - text: You can't place two stones of one colour in a turn.
    pieces: [red@d4]
    highlight: [e4]
    illegal: true
```

### Every key of a tutorial

| Key | Required | What it is |
|---|---|---|
| `id` | yes | The tutorial's name, unique among the game's tutorials. Name the file after it. |
| `kind` | yes | `rules`, `strategy` or `variant`. |
| `title` | yes | What the list of tutorials shows. |
| `summary` | no | One line under the title. |
| `order` | no | Sorts the game's tutorials of one kind, lowest first. |
| `game` | yes | The game or variant it is played on, of the family whose files hold it (`blooms-4`). |
| `variant_only` | no | `true` when it teaches only that game, so that it is offered only with it (`bowling-basics`). Without it the tutorial is offered for the whole family, and `game` is merely the board it is played on, as Blooms is taught on its small board. |
| `seat` | no | The learner's seat, 1 by default. The page plays every other seat. |
| `seed` | for a game with chance | Fixes the dice or draws, so every roll of the script comes out the same. Not needed with `start`. |
| `setup` | no | Moves played by whichever seat is to move, before the first step. |
| `start` | no | A position document to begin from instead of the game's start, for a game well into play. Usually `!include positions/name.yaml`. |
| `start_seat` | no | Who moves first from `start`, in a game whose position does not say (one without a Go ruleset). |
| `steps` | yes | The pages of the tutorial, in order. |

### Every key of a step

| Key | What it is | Rules |
|---|---|---|
| `text` | The page's prose, in Markdown. The game's guide images may be used. | Required. |
| `play` | The moves the learner must make, in order, written as the move list writes them. The page offers only these. | A step with none waits for **Next**. |
| `hint` | Shown when the learner asks for a hint. On a step without `play`, it can hold the answer to a question the text asks. | |
| `reply` | Moves played for the other seats after the learner's. | |
| `highlight` | Squares or areas to ring. | Must be on the board. |
| `arrow` | An arrow, as `[from, to]`. | Exactly two squares, both on the board. |
| `illegal` | `true` draws the step's `arrow` and `highlight` red with a cross: what must not be done. | Needs one of them. No `play` or `reply`. |
| `pieces` | A picture: the board with just these pieces on it. | Only one of `pieces`, `show` and `position`. No `play` or `reply`. |
| `show` | A picture: the position these moves reach from the game's start. | As above. Every move must be legal, in turn. |
| `position` | A picture: a position written out as a document. | As above. |

`play`, `reply`, `setup` and `show` all use the game's move notation, the same text as the move list.
A square in `highlight`, `arrow` or `pieces` is a position id of the board (`d4`, or `south-pacific` on
a map).

### The game, and pictures

**The game is reached by moves.** The room really plays `setup` and each step's `play` and `reply` from
the start, and the engine checks each one, so a rule change can never leave a tutorial teaching rules the
engine no longer has. A step without any of these is just a page to read.

**A picture is not part of the game.** A step with `pieces`, `show` or `position` draws that position in
place of the room's for that one step, and asks for no moves. The room's game is untouched, and the next
step without a picture shows it again. The status line says it is an example, and there is no turn in it.
Pick whichever of the three is easiest:

- **`pieces`** is for an example. It lists what is on the board, and nothing asks whether play could get
  there, so it can show a position that could never arise. Write `piece@square`, or
  `piece:color@square` for a piece not named after its colour (`cultist:green@asia`). List a stack bottom
  first. Everything else, such as captures and reserves, stays as it is at the start of the tutorial.
- **`show`** is for a real game, such as a finished one. The moves must be a legal game from the start, every
  seat in its turn: in Blooms, Warm's one stone first, with `done` ending any later turn of one
  stone. Taking them from bots playing a game out is easiest.
- **`position`** is for what `pieces` cannot say, such as the units, Power and phase of a game with more
  state than its board. It is the whole document, including the game's own `ext`:

  ```yaml
  position:
    v: 2
    ext: {turn: 2, turns: 3}
    cells:
      g1: [{piece: blue, color: blue}]
  ```

**A game can still refuse a picture.** `pieces` and `position` pass through the game's check of a custom
start, which refuses what it could not draw: an unknown piece, a square not on the board, or two stones on
one cell of Blooms. Reachability is not part of that check, so a bloom that should already have been
captured is fine.

**Showing an illegal move:** draw the position with `pieces`, then mark the move with `arrow` or
`highlight` and `illegal: true`. Nothing is played, so the move does not have to be legal, and Verify only
checks that its squares exist.

**Starting from a written-out position.** Where reaching the game's position by moves is impractical — a
four-player game in its second round — a tutorial can begin from `start`. It is checked as any custom start
is, carries its own state of chance, so no `seed` is needed, and every move after it is still replayed
against the engine. Keep a long one in a file of its own:

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
arrow drawn (`.illegal-marks`, with `.illegal-cross` over them, for an `illegal` step), and a picture in
place of the game's, with `#status-message` saying it is an example. The chat and the voice call are left out:
there is nobody to talk to.

**What to select is a signal, not a page of its own.** The tutorial the API serves carries a `guide`
for each step: the selections each of the learner's moves is made of (`piece red`, then `pos d4`) and
the position a step shows ([api.md](api.md#games)). A client shows them its own way. The web page
keeps every control where it always is — the colour buttons, *End* — picks out the one to press,
makes the others inert, and lets only the square to click be clicked. *Need a hint?* shows the step's
`hint`. When the learner's moves are made, the page
plays the step's `reply` for the other seats, a moment apart so they can be followed, and moves on.
A step without `play` waits for **Next** (**Finish** on the last).

**Back** (`#tutorial-back`) returns to any earlier step, to read it again with its board as it stood. Looking
back changes nothing: the moves of the room stand ([ADR 0083](decisions/0083-a-move-stands.md)), the board cannot be
played on, no reply is played, and **Next** comes forward again to where the learner was.

Where the learner is comes from the
moves the room has played, so a reload, another tab or a phone find the same step; moves that are not
the script's (a step taken back) leave the tutorial *off script*, with a way to start again.

At the end the tutorial is marked done in the browser, and **Finish this game against a bot** starts
an ordinary game from the position on the board.

## The guarantee

`go test ./tests/tutorials` replays every tutorial of every game as the page will: the setup, then
each step's moves for the learner's seat and its replies for the others. A move that is not legal,
made by the wrong seat, a square that is not on the board, or a picture its game refuses fails the test — so a rule change can
never leave a tutorial teaching rules the engine no longer has.

The replay proves the moves, not the prose: when a step says "the blue stone is captured", play the
line once and look.
