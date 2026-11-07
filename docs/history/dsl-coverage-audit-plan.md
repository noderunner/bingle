# Plan: auditing how much of each game the DSL can express

Status: **planning in progress** (opened 2026-10-28). The goal, the scope and the guideline for
what belongs in the DSL were settled on 2026-11-06 (*What was decided*, below); the audit itself is
next. Nothing is implemented.

## What was asked for

- Audit the simpler games (everything except Cthulhu Wars) to see how much of each is definable
  in the DSL.
- Work out whether the DSL's capabilities can be extended so more games are fully covered.

## Where things actually stand, measured

The split is cleaner than "DSL games and Go games" suggests. **Every** game uses the DSL for its
board, pieces, display, guide and variants. What the ten Go games do not use is the DSL's **move
generation and win conditions** — their definitions declare `win_condition.type: "ruleset"` and
contain *zero* `actions`:

| Game | Go rules (lines) | DSL actions | Win condition |
|---|---|---|---|
| Blooms | 714 | 0 | ruleset |
| Meridians | 813 | 0 | ruleset |
| Yinsh | 927 | 0 | ruleset |
| Fendo | 940 | 0 | ruleset |
| Onitama | 959 | 0 | ruleset |
| Fist | 1,205 | 0 | ruleset |
| Hive | 1,292 | 0 | ruleset |
| Nyet | 1,654 | 0 | ruleset (uses the DSL `cards` block) |
| Courtfall | 3,786 | 0 | ruleset |
| Cthulhu Wars | 17,628 | 0 | ruleset (out of scope) |

The fifteen fully-declared games (Tak and the fourteen classics) are 1.5–5 KB of definition
each, and they are not toys: **Go is pure DSL**, with captures, ko, two passes ending the game
and area scoring with per-seat komi. Tak is pure DSL with stacks, flattening and carry limits.
That is the real measure of how much reach the vocabulary has.

The vocabulary today: action types `place`, `move-stack`, `jump`, `slide`, `leap`, `swap`,
`step` and `pass`; win rules `connect`, `reach`, `most`, `score`, `pool`, `ring`, `absent`,
`board-full`, `reserve-empty`, `no-legal-moves`, `passes`, `ply-limit`, `mover-wins`,
`mover-loses` and `callback`; plus captures, custodian capture, flips, flattening, ko, chains,
carry limits, forbidden regions, clone distance, infection, gravity, promotion, stacks,
reserves, cards, regions, square and hex generators, merge-patch variants, the pie rule and
options.

## What was decided (2026-11-06)

The maintainer's answers to the open questions below. They come before the initial thinking, which
is kept for its reasoning but loses where it disagrees.

1. **The goal is plugins.** Grow the DSL so that more games can ship as data-only packs, and rank
   each addition by the games it unlocks. Less Go code is a welcome side effect, not the aim. A game
   that is mostly declared but keeps a Go hook (the `callback` win rule, say) is still compiled in,
   so for this goal a game is either fully declared or stays in Go, and the audit says which and why.
2. **The scope is every game with a Go ruleset except Cthulhu Wars**, the private ones included:
   Yinsh, Onitama, Hive and Nyet! as well as Blooms, Meridians, Fendo, Mycelium, Fist, Courtfall and
   Solitaire. A private game that is declared can be a private pack instead of compiled code, and
   all of them show which additions pay off.
3. **The line is how common a mechanic is, not a category such as dice or hidden information.**
   In the maintainer's words: mechanics that are common and used in a lot of games should be
   expressible in the DSL; those that are more unique, or need more complex logic, are out of
   scope. Examples given: shuffling a deck and rolling a group of d6, counting 5+ as a hit, are
   common, so they belong in the DSL. Magic: The Gathering's *scry* (draw X from the top, keep one,
   put the rest back in any order) and dice-pool manipulation (turning a die to its opposite face,
   and anything more elaborate) are uncommon, so they belong in Go. It is a guideline, not a
   precise rule. The audit applies it as two tests:
   - **Is it common?** A mechanic earns a primitive when it appears in several games, in the
     catalog or well-known outside it, and can be stated as one rule with a few parameters. One
     that needs a script of steps stays in Go.
   - **Is the declaration simpler than the Go?** Being expressible is not enough. When a game's
     declaration would be larger and harder to follow than its Go ruleset, because it builds a
     complex game out of simple primitives, Go is the right home even if every mechanic is
     supported. The audit compares the two for each game, and asks whether someone could check the
     declaration against the rulebook line by line.
   Consequences: seeded dice and shuffles are in scope (the engine is already seeded and
   deterministic), and with shuffled decks the common kinds of hidden information come in too, such
   as a hand or a face-down pile. Simultaneous secret choice (Fist) is common in card and bluffing
   games but changes the turn model, so the audit costs it rather than ruling on it now.
4. **A converted game keeps its Go ruleset for one release**, as the reference for the differential
   harness (the same legal moves, states and outcomes over a random battery), and a follow-up pull
   request deletes it.
5. **Every accepted DSL addition gets an ADR**, since the vocabulary is a public contract for anyone
   who authors a game; and there is no size cap on the DSL as such: test 1 above is the brake.

## Initial thinking on the method

**The output should be a ranked list of candidate DSL features, not a coverage percentage.**
For each game, classify every rule into three buckets: expressible today, expressible with a
named small addition, or genuinely needs code. Then rank the proposed additions by *games
unlocked per unit of added vocabulary*. A feature that unlocks three games is worth it; one that
unlocks half of Yinsh is how a DSL turns into a bad programming language.

**My prior on each game, to be tested rather than trusted:**

- **Onitama** — the DSL already has a `cards` block (Nyet uses it) and `leap` actions. Move
  cards that pass to the opponent are a card-rotation rule, not new geometry. Most promising.
- **Blooms** — placement of one or two stones per turn plus group capture by liberties. Go
  already proves liberties and captures work on a square board; this is mostly "N placements per
  turn" on a hex board. Second most promising.
- **Fendo** — placing fences on edges. The DSL has edges in its board model but pieces live on
  positions; this needs a real new concept (an occupant on an edge).
- **Meridians** — sight lines along rays, and removal deferred to the end of the mover's turn
  with a two-turn exemption. The geometry is expressible; the *deferred, conditional* removal is
  the interesting gap, and a general "end of turn, evaluate this" hook might unlock several games.
- **Hive** — one-hive connectivity and freedom-to-move. Connectivity constraints on *movement*
  (rather than as a win condition, which exists) is a plausible general addition.
- **Yinsh** — moving a ring, flipping the markers jumped, removing a row of five and then a ring.
  A chain of distinct effects in one decision; the DSL's `then` is the seed of this.
- **Fist** — simultaneous hidden commitment. Not a geometry gap: it needs hidden information and
  a commit-reveal turn structure, which is `HiddenInfo` territory. I would call this "needs code"
  and not try.
- **Courtfall** — dice, momentum, conditions, programmed combat. Out of scope, same as Cthulhu
  Wars.

**So the honest framing is that full coverage is the wrong target.** There is a real line —
hidden information, simultaneous choice, dice, resource economies — where a Go ruleset is the
right answer, and the DSL is valuable precisely because 4,992 lines of `state` serve fifteen
games well rather than twenty-five games badly. The audit should find that line and say where it
is, which is a useful document in itself.

**The engineering that matters is a differential harness.** When a game is converted, the old Go
ruleset and the new declaration both exist, so we can play them against each other: for every
position in a large random battery, assert identical legal-move sets, identical applied states
and identical outcomes. `tests/allgames` already runs a random battery, so this is an extension
of something real, and it is the only way to convert a game with confidence.

## Open questions for the discussion

1. Is reducing Go code actually a goal, or is the goal "more games can be plugins"? They point
   at different work. If it is the plugin goal, Onitama and Blooms are the whole project.
   (Context from 2026-10-31: the plugin plan settled that installable packs are data only and
   Go code is always compiled in, so this audit is now the only route to an installable game.)
2. How much vocabulary growth is acceptable? The DSL struct is ~670 lines today; at what size
   does it stop being a DSL?
3. Do converted games keep their Go ruleset for a release as a differential reference, then drop
   it, or do we keep both permanently for the ones that are borderline?
4. Where do we draw the "needs code" line — I would draw it at hidden information, simultaneous
   decisions and dice. Agreed?
5. Does each accepted DSL addition get an ADR? I think yes; the vocabulary is a public contract
   for anyone authoring a game.

## Dependencies and cross-plan effects

- **Plugin plan**: this is the lever that makes plugins matter. Every game that becomes fully
  declared is a game that can be installed as data with no sandbox.
- **YAML plan**: this audit *adds* fields to the DSL, so doing the format change first means
  authoring new vocabulary once. Cheap first, expensive second.
- **Tutorials**: highlights and prompts need things in the definition to point at, so some of
  what the audit adds will be display vocabulary, not rules vocabulary.
- **Code audit**: converting a 900-line ruleset to a declaration is the largest simplification
  available anywhere in the codebase, so these two plans should agree on order.
- **Tournaments**: a converted game is a *different implementation* of the same rules; the
  `game_versions` record and any ratings must survive the conversion honestly.

## Status log

- 2026-10-28: opened with initial thoughts, including a measured table of where each game stands
  and a per-game prior. Recommendation: rank candidate DSL features by games unlocked, accept
  that full coverage is the wrong goal, and build a differential replay harness before
  converting anything. Planning in progress.
- 2026-10-31: noted the plugin decision under question 1: packs are data only, so this
  audit is the only route to an installable game.
- 2026-11-06: the maintainer settled the goal (plugins), the scope (every Go game but Cthulhu Wars,
  the private ones too), the guideline for what belongs in the DSL (how common a mechanic is, and a
  declaration no larger or harder to follow than the Go it replaces), one release of overlap for a
  converted game, and an ADR per addition. Since the table above was measured, Solitaire (1,657
  lines of Go) and Mycelium (1,242) have arrived, and Yinsh, Onitama, Hive and Nyet! are headed for
  the private build. Next: the audit itself, rule by rule.
