# 0008. Full move-history persistence

Status: Accepted

## Context
Persistence could range from none (in-memory only, lost on restart — contradicts the brief's call for a database backend), to current-state-only (enough to resume after a restart, no replay), to full history of every move.

## Decision
Persist every move (with sequence number, seat, payload, and resulting state) alongside games and players, enabling replay/spectator scrollback and correct resume after a server restart.

## Consequences
- Schema needs a `moves` table, not just a `games.current_state` column.
- Replay and "watch an AI playtest afterward" become possible with no extra engine work beyond reading the move log.
- Slightly more write volume per move (one history row) — acceptable for a casual, turn-based (not real-time-action) app.
