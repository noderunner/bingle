# 0003. Board modeled as a generic graph, not a fixed grid

Status: Accepted

## Context
Boards could be modeled as a fixed rectangular grid (simplest, covers Go/chess-like games) or as a generic graph of positions plus adjacency edges (covers grids, hex boards, and irregular boards uniformly). A grid-only model is less work now but would need a rewrite the first time a non-square game shows up.

## Decision
Boards are a generic graph: a set of positions plus adjacency edges (optionally typed, e.g. "orthogonal"/"diagonal"). Square and Go-style grids are just a regular special case of this graph.

## Consequences
- Movement primitives and the SVG renderer are written against the graph model, not grid coordinates — this is more upfront modeling work.
- Adding hex or irregular boards later needs no engine changes, only new board definitions.
