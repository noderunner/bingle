# 0024 — Hints, game import and environments over HTTP

## Context

Three requests kept coming up around the training/analysis story: "what would
a bot play here?", "load this recorded game so I can look at it", and "I can
not speak MCP from my training script".

## Decision

* **Hints** (`POST /games/{code}/hint`, `suggest_move`) ask a built-in bot —
  MCTS by default, reporting its estimated win rate and the next-best moves.
  They are refused whenever another human plays in the room (and in waiting
  rooms): a hint is only ever a study aid against bots or over a finished
  game. Searches share a bounded number of CPU slots, as do bot replies and
  matches, so a burst of requests queues instead of starving the server.
* **Import** (`POST /games/import`, `import_game`) validates a PTN document or
  move list entirely in the engine first (naming the first illegal move),
  then replays it through the normal move path into a fresh room, so history,
  undo bookkeeping, export and analysis all behave as for a played game.
  Reviewing happens as a spectator.
* **Environments over HTTP** mirror the `env_*` MCP tools one-to-one
  (`/envs`, `/env-specs`, `/matches`) and share their implementation; a
  standard-library Python client (`scripts/bingle_env.py`) gives a
  Gymnasium-shaped `reset`/`step`/`fork` without any dependency, and a test
  keeps it working.

## Consequences

* There is exactly one implementation of environments; MCP and HTTP are thin
  adapters, which keeps behaviour (and bugs) identical.
* The hint rule is enforced on the server, not just hidden in the UI.
