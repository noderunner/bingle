# Plan: guided game tutorials

Status: **settled**, ready to implement (opened 2026-10-28, settled 2026-11-01). The sections
before *Decisions* are the initial thinking, kept as written. Nothing is implemented.

## What was asked for

- Guided tutorials that walk a player through the rules and click through examples.
- Forced bot moves, to illustrate a mechanic.
- The option to finish the game against the bots afterwards if the player wants to.

## What already exists that this builds on

This is the most nearly-free of the new features, because every piece exists:

- **Positions are documents.** `games.initial_state` and `initial_turn_seat` already let a game
  start from an arbitrary position, and `Ruleset.Validate` checks one.
- **Puzzles already are this, minus the prose.** Each puzzle is a position, a seat to move, a
  list of every winning first move and one full solution line including the defender's replies,
  and there is a solver that proves them. A tutorial step is a puzzle step with a paragraph
  attached.
- **The click-by-click machinery is already game-agnostic.** `Ruleset.Steps` decomposes a legal
  move into its selections and `MoveBuilder` in the browser walks that tree, which is why no
  game needs bespoke input code. A tutorial can therefore accept only the intended move and
  explain gently when something else is clicked, in any game, with no per-game JavaScript.
- **The server already plays moves on a player's behalf**: `moves.auto` and `moves.autoplay`
  exist for reactions and auto-play timeouts. A scripted opponent reply is the same mechanism.
- Guides are markdown with tabs, rendered by `markdown.js` with an asset base, so prose and
  diagrams already have a home next to the game.
- Bots exist for every game, so "finish it against the bot" is a seat assignment.

## Initial thinking

**A tutorial should be data, not code** — one document per game, shipped in the game module's
assets so that a plugin can bring its own. Something like a list of steps, each with: markdown
prose; optionally a position to set; optionally squares, pieces or regions to highlight (the
display vocabulary already has tints, badges and overlays); the move or moves accepted; and the
opponent's scripted reply. "Free play from here" ends the tutorial by converting the position
into a real game with a bot in the other seat, which is the existing custom-start path.

**The runner is a page, not a new engine concept.** It talks to the normal endpoints, so the
rules shown are the rules that are implemented — there is no second copy of the rules to drift.

**Every tutorial must be verified in CI.** This is the part I would insist on: a test that
replays every tutorial's scripted line against the real engine and fails if any step's expected
move is no longer legal. Without it, the first rule change silently produces a tutorial that
lies, and the puzzle solver already sets the precedent for proving content against the engine.

**Authoring.** Hand-writing position documents is miserable. The interesting option is a
**recording mode**: play the sequence in a real game, then export the move list and positions
as a draft tutorial to annotate. The move history and export endpoints already give almost
everything needed, and this is probably the difference between three tutorials and twenty.

**Guests should be allowed.** A tutorial is the lowest-barrier thing on the platform and should
need no account; progress is per-browser unless signed in, which is a decent nudge towards
signing up.

## Decisions (settled 2026-11-01)

1. **The first games are Blooms, Meridians and Courtfall** — three shapes: a two-stage turn on
   a hex board, a placement game with its own geometry, and a ruleset game with dice and secret
   choices. All three stay in the public repository. Tutorials for the games going to the
   private build (Cthulhu Wars, Yinsh, Onitama, Hive, Nyet!) come later and travel with them,
   because a tutorial is data in the game's own folder.
2. **Tutorials are YAML**, hand-written, in the game's folder (`tutorials/*.yaml`), with a JSON
   Schema generated like the definition's. **A step's position is given as moves**, from the
   start or from the previous step, not as a hand-written position document; the engine builds
   every position by playing them, which is also what verifies the tutorial. A "copy this game
   as a tutorial draft" export from a played game may come later; moves keep their notation, so
   it is small.
3. **A game has any number of tutorials, of three kinds**: `rules` (how to play), `strategy`
   (why a move is good) and `variant` (what a variant changes). Rules tutorials are split by
   topic where a game is big enough to need it — Cthulhu Wars would have combat, spellbooks,
   gates and so on — so a player can come back to one small tutorial as a refresher rather than
   replay everything. Each tutorial has an id, a title, a kind, a one-line summary, an order
   within the game and the game id (or variant id) it is played on; the list of a game's
   tutorials is served with it, grouped by kind.
4. **No achievements for tutorials**, and none planned. Which tutorials a browser has finished
   is kept in the browser, only to mark them in the list.
5. **A tutorial belongs to a game family and names the variant it is played on**, so one
   tutorial serves Hex on every size unless a `variant` tutorial says otherwise.
6. **Tutorials play in real rooms.** Starting one creates a room marked as a tutorial, kept out
   of "My games", the open games, notifications and retention's interest. The learner holds
   every seat (the solo seats of ADR 0029), plays their own side through the ordinary move
   builder, and the page plays each scripted reply as the other seat — through the same move
   endpoint, so the rules shown are the rules the engine applies. The page shows the position
   from the learner's seat, so a scripted opponent's secret choices stay secret.
   - **A move the step does not want is refused in the page**, before it is sent, with the
     step's hint; the move builder knows a move's notation before posting it.
   - **"Finish this game against a bot"** starts an ordinary game from the current position with
     a bot in the other seat — the existing custom-start path — and leaves the tutorial room.
   - **Games with chance fix their seed.** Courtfall keeps its dice generator in the position
     (`rng`), so a tutorial names a seed, the server builds the start from it, and every roll of
     the scripted line comes out the same each time. This is the one server addition the first
     three games need besides serving the files.
7. **What a step can point at** is a small vocabulary drawn by the board renderer for every
   game, with no per-game code: squares (or areas) to highlight, a move to suggest with an
   arrow, and the step's prose in Markdown (the guide's renderer, with the game's assets).
   Anything else a step needs to point at gets a name in the definition first.
8. **Every tutorial is verified by the tests**: a test replays every tutorial of every game
   against the engine — each step's moves legal, each scripted reply legal, each "the learner
   should play X" move legal — and fails when a rule change breaks one. A tutorial that lies is
   worse than none.
9. **Guests may take tutorials**; nothing needs an account.

## A sketch of the format

Illustrative, to be settled while building the first one:

```yaml
# yaml-language-server: $schema=../../../../docs/tutorial.schema.json
id: blooms-basics
kind: rules            # rules | strategy | variant
title: Placing and capturing
summary: How a turn works, and how a bloom is captured.
game: blooms-4         # the variant it is played on
order: 1
steps:
  - text: |
      Each turn you place one or two stones, of **different colours**.
    expect: [d4]               # the learner's move(s) this step accepts
    hint: Place on the highlighted space.
    highlight: [d4]
  - text: The opponent replies.
    reply: [e5, f5]            # played for the other seat
  - text: Now surround the red bloom.
    expect: [e3]
    arrow: e3
```

## The order of work

1. The tutorial format, its schema and the replay test, with one short Blooms tutorial.
2. Tutorial rooms: the marker, the seed, keeping them out of the lists.
3. The tutorial page: prose, highlights and arrows, refusing other moves, the scripted
   replies, finishing against a bot.
4. The tutorials of Blooms, Meridians and Courtfall, several per game where the rules divide.
5. The list of a game's tutorials, by kind, where the player picks a game and in its rules
   window.

## Dependencies and cross-plan effects

- **Plugin plan**: tutorials must be data in the module's assets, or a plugin game can never
  have one. That constraint comes from here and should be in the plugin manifest. It is also
  why the private games' tutorials can be written now and move with them.
- **Achievements**: decided 2026-11-01 that tutorials earn none. Tutorials are still how a new
  player reaches the achievements that need skill.
- **Tournaments**: the on-ramp. A ladder with no tutorial is a ladder for people who already
  know the games.
- **Auth plan**: guests may take tutorials; only accounts can keep progress across devices.
- **DSL coverage audit**: highlights, prompts and legal-move hints all come from the display
  vocabulary, and anything a tutorial needs to point at may need a name in the DSL.
- **YAML plan**: tutorial documents are exactly the kind of long hand-authored file that is
  nicer in YAML with comments.

## Status log

- 2026-10-28: opened with initial thoughts. Tutorials as data over the existing position,
  `Steps`/`MoveBuilder`, auto-move and guide machinery; a CI replay test as a hard requirement;
  a recording mode as the authoring idea worth discussing. Planning in progress.
- 2026-11-01: settled. Blooms, Meridians and Courtfall first; YAML with positions given as
  moves; any number of tutorials per game, of kind rules, strategy or variant, with big games'
  rules split by topic for refreshers; no achievements; real rooms with the learner in every
  seat and scripted replies through the move endpoint; fixed seeds for games with chance; a
  replay test of every tutorial.
- 2026-11-01: steps 1, 2, 3 and 5 built (ADR 0077). In step 4, Blooms and Meridians have rules
  tutorials; **Courtfall's waits until its rules are final**, and it gets no strategy tutorial
  until then. Decided along the way: rules tutorials follow one order (what you are doing; a shown
  finished game and how it ends; the board; the pieces; the turn; each phase), steps can `show`
  another position, the served tutorial carries a `guide` signal of what to select that each
  client presents its own way (the web page keeps its controls, picks out the expected one and
  makes the others inert), and tutorial rooms have no chat or voice. A minimax bot game on
  `courtfall~arena` with seed 1 ends 5–0 after 125 moves, an end state for Courtfall's step 2.
