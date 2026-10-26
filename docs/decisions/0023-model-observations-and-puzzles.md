# 0023 — Model-ready observations and proven puzzles

## Context

Text boards and legal-move lists suit language models; networks want fixed
shapes. A benchmark also needs positions with a known right answer.

## Decision

* `state.EncodePlanes` produces a viewer-relative tensor: one-hot channels per
  stack depth, colour (me / opponent) and piece type, a stack-height plane,
  reserve fractions, side-to-move and ply. Channel names travel with the data
  and `env_spec` publishes the layout, so nothing is implicit.
* `state.ActionSpaceOf` enumerates every move a game can express on its board
  in a stable order; a move's index is its fixed action ID. Games whose moves
  cannot be enumerated (chained jumps) report a dynamic space instead.
* Both are opt-in per environment (`include: ["planes", "action_ids"]`) so
  language-model clients do not pay for tensors they do not read.
* Puzzles are generated offline by mining bot games for positions shortly
  before a decisive move and *proving* with an exhaustive solver that the side
  to move wins by force within N plies against every defence; only unique-ish,
  non-trivial positions are kept. A test re-proves every shipped puzzle.

## Consequences

* A reward of +1 on a puzzle environment really means "found a forced win".
* The solver is exact and exponential, so shipped puzzles are limited to a few
  plies; regenerating is a deterministic `go run scripts/genpuzzles`.
