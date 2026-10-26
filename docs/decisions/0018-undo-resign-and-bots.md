# 0018 — Undo, resign and built-in bots

## Context

Async casual play wants take-backs and the ability to play alone; model
training wants fast, always-available opponents. There is no auth (ADR 0006).

## Decision

* **Undo**: a player may take back their latest move provided no other *human*
  has moved since; replies by built-in bots are removed with it. The game
  returns to active with the same turn. A game that ended by resignation cannot
  be undone. No opponent consent is required (casual, trust-based).
* **Resign** finishes the game (`end_reason: resignation`, opponent wins).
* **Bots** (`random`, `tactical`, `mcts`) only use the generic
  `LegalMoves`/`Apply` API, so they work for every game. A room seat can be held
  by a bot (`players.bot`); bots reply inside the request that triggered them
  (detached from client cancellation) and their moves are published like any
  other.
* Live change events other than moves are broadcast as `state` events carrying
  the whole position (`reason: join | undo | resign`).

## Consequences

* Solo play and self-play need no second client.
* Request latency grows by the bot's think time (MCTS ≈ 1 s) while the room is
  locked; acceptable for casual use, and training uses envs instead.
* Undo semantics are intentionally simple; a negotiated take-back would need
  new persistent request state and UI.
