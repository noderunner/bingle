# 0025 — Go rulesets, chance and hidden information

## Context

ADR 0001 promised a Go escape hatch for what the JSON DSL cannot express, but
the only hook that existed (`GameCallbacks`) can veto a move and name a
winner — nothing more. Courtfall needs far more: a per-unit condition track,
dice, a secret simultaneous allocation in every exchange, a turn made of a
dozen small decisions by both players, resources spent between decisions, and
a victory condition that is not a board pattern. It also raised questions the
platform had never faced: dice must be reproducible for replay yet unknowable
in advance, a committed choice must stay hidden from the opponent and from
spectators, and "undo" must not turn into a way to peek at the dice.

## Decision

**A `state.Ruleset` replaces the declarative action engine for one game.**
A definition says `"engine": "ruleset"` and its `catalog.Module` carries a Go
value implementing `Start`, `ToMove`, `Legal`, `Apply`, `Format`, `Parse`,
`Steps`, `Validate` and `Outcome`. Everything above the engine — session,
store, API, MCP, bots, environments, web UI — keeps calling `state.Apply`,
`state.LegalMoves`, `state.FormatMove`… which dispatch to the ruleset. The
definition still describes the board, seats, pieces, regions, `params` and a
`display` legend.

**A position carries a game payload.** `BoardState` gained an opaque `ext`
(JSON in the document, a typed value inside a ruleset). Units are still
ordinary stacks of one piece, so text rendering, planes and replays see them.

**A turn is a sequence of decisions, each one a move.** `ToMove` is a function
of the position, so the same seat may move several times in a row and the
other seat may be asked mid-turn (a strike's target). Undo, take-back, move
lists, clocks, hints and bots work on decisions without change.

**Chance is a seeded generator inside the position.** Dice are a pure function
of the position's `rng` field, so replays and `simulate` are exact. A game
whose start carries a seed (`SeededRuleset`) has its start document recorded
with the game, like a custom start position.

**Hidden information has three parts.**

1. *Public views.* Everything leaving the server goes through
   `state.Public(game, position, viewer)`: the generator is zeroed, an
   unanswered commit is masked for everyone but its owner, and the ruleset's
   derived **view** (tints, badges, scoreboards, the pending decision, the
   dice on the table) is attached under `ext.view` so every client — and
   history navigation — gets it for free. Move records pass through
   `PublicMove` (`Qxd4/?`). Identity stays name-only (ADR 0006): this hides
   secrets from the wire, not from someone who claims another player's name.
2. *Determinization.* Searching bots, hints and `simulate` run on
   `state.Determinize(...)`: the generator is re-drawn and the opponent's
   commit is sampled, so a search cannot peek at the dice or at a secret.
   Forked training environments are determinized for the same reason.
3. *No rewinding across chance.* `UndoGuard.CanTakeBack` refuses undo and
   take-back requests that would cross a die roll.

**Optional interfaces** let a ruleset opt in to a fixed action space,
its own planes, a text renderer, a search heuristic and client labels
(`ActionLister`, `PlaneEncoder`, `TextRenderer`, `Heuristic`, `Describer`).
Search bots use the heuristic as their leaf value; MCTS uses it to score
unfinished playouts.

## Consequences

* Courtfall is ~2,000 lines of Go plus a 100-line JSON document and plugs into
  the lobby, UI, API, MCP, bots and environments without per-game branches
  beyond "is this a ruleset game".
* Rulesets are not covered by the DSL's brute-force enumeration tests; the
  Courtfall suite instead checks every legal move against `Apply`, mutated
  moves against rejection, notation and document round-trips, a reference
  movement implementation and fixed-dice scenarios.
* Puzzles (an exhaustive solver) and PTN/TPS are for deterministic,
  declarative games only. Import of move lists is refused for games with
  chance because a replay would roll different dice.
* A future game with chance or hidden information needs no engine work: write
  a ruleset, register it, add a `display` legend.
