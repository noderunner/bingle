# Plan: guided game tutorials

Status: **planning in progress** (opened 2026-10-28). Initial thoughts written before the
design discussion, not settled decisions. Nothing is implemented.

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

## Open questions for the discussion

1. How many games to begin with? I would do three with different shapes — something like
   Blooms, Onitama and Hive — and only generalise once the format has survived them.
2. Hand-authored JSON, or is the recording mode worth building first?
3. Does a tutorial teach the *rules* or the *strategy*? The guide pages already explain rules;
   the thing people actually want is "why was that move good", which is a different document.
4. Is tutorial completion an achievement (and therefore account-gated for the badge, though not
   for the tutorial itself)?
5. Does a tutorial belong to a game family or to a variant? Hex on different board sizes should
   share one, I think.

## Dependencies and cross-plan effects

- **Plugin plan**: tutorials must be data in the module's assets, or a plugin game can never
  have one. That constraint comes from here and should be in the plugin manifest.
- **Achievements**: "finished the tutorial" is the obvious first badge, and tutorials are how a
  new player reaches the achievements that need skill.
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
