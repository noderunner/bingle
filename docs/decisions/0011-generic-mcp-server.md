# 0011. Single generic MCP server, not per-game

Status: Accepted

## Context
The brief wants AI models to "easily implement MCP for playtesting games and serving as an opponent." Considered: one generic MCP server exposing game-agnostic tools (list_games, get_state, make_move, ...) that wrap the same REST/engine calls a human client would use, versus letting each game module define its own tailored MCP tools.

## Decision
Ship one generic, in-process MCP server whose tools are game-agnostic and call the same engine functions the REST API uses. No per-game MCP tool definitions in v1.

## Consequences
- Any AI model can join or spectate any game (including future ones) with zero per-game MCP work.
- Move payloads exposed through MCP are whatever the generic move schema is (e.g. a position/target descriptor), not a game-specific vocabulary — an AI must interpret game-specific semantics itself (e.g. via get_state's board/piece data), same as a human client would via the UI.
- If a future game genuinely needs richer MCP semantics, that's a new decision to make deliberately, not a default to slide into.
