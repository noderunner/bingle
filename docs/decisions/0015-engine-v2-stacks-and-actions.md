# 0015 — Engine v2: stacks, reserves and declarative actions

## Context

v1's engine modelled a move as `from → to` of a single piece onto a square
holding at most one piece. Tak — the reference for "more complex abstract
games" — needs pieces in hand, placement, stacks of several pieces, moves
that carry part of a stack across several squares, an opening swap rule, and
road / flat-count win conditions. The v1 decision (ADR 0001) was JSON first with
a Go escape hatch; implementing Tak as Go callbacks would have left the DSL
unchanged but made every future stacking/placement game a Go project.

## Decision

Grow the engine, not the escape hatch:

* **State** holds, per square, a stack of pieces (bottom first), per-color
  **reserve pools**, and a ply counter. Pieces draw from a *pool* (default: their
  own type), so Tak's flat and standing stones share one supply.
* **Actions** are declared in JSON: `place` (reserve or unlimited supply,
  region, gravity) and `move-stack` (carry limit, straight line, ≥1 drop per
  square, bottom-first). Pieces with a movement primitive keep the v1 implicit
  `step` action.
* **Piece stacking attributes** (`coverable`, `flattenable_by`, `flatten_to`)
  plus per-action flatten conditions express walls and capstones.
* **Opening phase** on the turn order restricts the first plies and can make
  placed pieces take the opponent's color (the Tak swap).
* **Win rules** are a small ordered list (`connect`, `line`, `end`) with
  priorities (the mover wins ties) and an `end_reason`.
* **Variants** via JSON merge patch give board-size families.
* Legal-move enumeration and move application share one validation path per
  action, so "listed as legal" and "accepted" cannot drift apart.

Everything v1 did still works unchanged: `Apply` with a `{from,to}` move is
the old algorithm, and the Go callback interface is unchanged (it only gained
optional fields on `MoveContext`).

## Consequences

* Tak is 100 % JSON and passes perft against published node counts.
* Tic-Tac-Toe, Connect Four and Gomoku fell out of the same primitives.
* Rules that still need Go: capture by custody, flipping, hidden information,
  multi-step turns. New *action types* in `internal/engine/state` are the
  preferred way to add such families.
* Enumeration validates every candidate, which is plenty fast for Tak (a full
  depth-4 perft in ~2 s) but is the first place to optimise for deep search.
