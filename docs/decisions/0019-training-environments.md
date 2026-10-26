# 0019 — In-memory gym-style environments for training

## Context

"MCP support for training models" needs thousands of short episodes. Rooms are
persisted, multi-party and name-addressed; each move is several SQLite
transactions plus broadcasts.

## Decision

Add a separate, in-memory **environment** abstraction (`internal/envs`) exposed
via `env_*` MCP tools: `reset` / `step` / `observe` / `fork` / `close`. An
environment is a single-agent view of a game; the opponent (bot or none, for
self-play) replies inside `step`. Observations carry the text board, TPS,
legal moves (a fixed per-step action list), history and reward. Environments
are seedable, expire after idle time, and are bounded in number.

## Consequences

* Training never touches the database or the room machinery; the engine
  guarantees determinism so runs are reproducible.
* `fork` makes tree search and branching cheap, with no persistence.
* Environments and rooms deliberately share only the engine; results from an
  environment cannot be "promoted" to a room (use `export_game`/`history` to
  move records).
