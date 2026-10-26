# 0006. Name-only player identity, no session token

Status: Accepted

## Context
The brief explicitly rules out accounts/RBAC ("just ask users to provide a name"). Within that, identity could still be backed by a server-issued session token (cookie or client-stored) to survive reconnects/refreshes, or be purely name-based with no token at all.

## Decision
Identity is name-only, re-entered per session. There is no session token; "reconnecting" means re-entering the same room code and the same name, which the server matches to your existing seat.

## Consequences
- No token issuance, storage, or validation code needed anywhere.
- A player who refreshes mid-game must re-type their name to resume their seat; if someone else uses the same name in the same room, seat assignment is ambiguous — acceptable given the brief's low-security, casual framing, but worth flagging to users in the UI (e.g. "don't reuse another player's name").
- This is a real usability tradeoff accepted deliberately, not an oversight — do not "fix" it by quietly adding tokens without raising it first.
