# 0007. Read-only spectators via room code

Status: Accepted

## Context
The brief calls out AI playtesting as a core use case, which benefits from being able to watch a game (e.g. AI vs AI) without occupying a player seat. Considered restricting rooms to exactly the two seated players versus allowing anyone with the code to observe read-only.

## Decision
Any client with a room's code/link beyond the first two (player) joiners connects as a read-only spectator: they receive the same WebSocket state broadcasts but cannot submit moves.

## Consequences
- The engine must distinguish player actions (moves) from spectator connections (state read + WebSocket subscribe only) at the API layer.
- No additional access control beyond "do you have the room code" — consistent with the brief's low-security scope.
