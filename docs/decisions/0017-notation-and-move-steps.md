# 0017 — Canonical notation and click-by-click move steps

## Context

Humans click, agents type, and history should be portable. A structured move
(`action`, `piece`, `from`, `to`, `count`, `path`, `drops`) is unambiguous but
awkward for LLMs and unusable as a UI interaction model.

## Decision

* Every move has a **canonical notation string** per game: generic
  (`2,0-2,1`, `c3`, `c3/3>d3:1>e3:2`) or **PTN** for Tak. Notation is stored
  with the move, returned in every listing, and accepted anywhere a move is
  accepted. Structured and legacy `from`/`to` input keep working.
* Each legal move is also decomposed into **steps** — the selections that
  build it (`piece`, `pos`, `count`). A client turns the legal-move list into a
  decision tree and walks it click by click, so the UI needs no game-specific
  code.
* TPS is supported for Tak positions (observations, `env_reset start_tps`).

## Consequences

* A new game gets a working click UI and an agent interface for free.
* Notation round trip (`Parse(Format(m)) == m`) is property-tested over random
  games for every shipped game.
* Notation syntax is per game; ambiguity (`-` inside position IDs) is resolved
  by checking against the board.
