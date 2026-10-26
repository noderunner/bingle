# 0001. Hybrid game definition: JSON DSL + Go escape hatch

Status: Accepted

## Context
Custom games need a way to describe board, pieces, and turn order. Options considered: pure declarative DSL (JSON/YAML) backed by a fixed library of primitives; pure Go modules per game; or a hybrid.

A pure DSL is easiest to author but can't express arbitrary rules without the primitive library growing without bound. Pure Go modules are fully expressive but require writing and building Go code for every game, which is heavier than a "casual" platform needs.

## Decision
Games are defined declaratively (board, pieces, movement primitives, turn order), with an optional Go callback interface a game can implement for logic the DSL can't express (custom win conditions, special captures, etc.).

## Consequences
- Most games, including the demo game, should need no Go code at all.
- The engine must define a stable callback interface early, since it's the pressure-release valve for everything the DSL primitive library doesn't cover yet.
- The primitive library will grow incrementally as real games are added; don't try to anticipate every case up front.
