# 0010. Async turn timing, no enforced clock

Status: Accepted

## Context
The brief's open questions mention "game timers" as a possible basic feature. Options considered: an enforced per-player countdown clock (like chess clocks, with persistence across disconnects), a simple "it's your turn" notification with no enforcement, or fully async with no time pressure at all.

## Decision
V1 is fully async: the server tracks and notifies whose turn it is (via WebSocket) but does not enforce or persist any time limit.

## Consequences
- No clock state to persist, restore, or reconcile across disconnects/reconnects — meaningfully less complexity for v1.
- A game can sit open indefinitely waiting on one player; no timeout/forfeit mechanism exists yet.
- Enforced clocks are a plausible v2 feature but are explicitly deferred, not designed around, in v1.
