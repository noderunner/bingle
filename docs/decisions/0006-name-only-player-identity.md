# 0006. Name-only player identity, no session token

Status: Superseded by the accounts plan: [ADR 0067](0067-principals-and-guest-keys.md) (a seat is its owner's) and [ADR 0068](0068-accounts-sessions-and-passwords.md)–[0073](0073-account-only-features.md). Only a seat taken by an anonymous client is still acted for by name.

## Context
The brief explicitly rules out accounts/RBAC ("just ask users to provide a name"). Within that, identity could still be backed by a server-issued session token (cookie or client-stored) to survive reconnects/refreshes, or be purely name-based with no token at all.

## Decision
Identity is name-only, re-entered per session. There is no session token; "reconnecting" means re-entering the same room code and the same name, which the server matches to your existing seat.

## Consequences
- No token issuance, storage, or validation code needed anywhere.
- A player who refreshes mid-game must re-type their name to resume their seat; if someone else uses the same name in the same room, seat assignment is ambiguous — acceptable given the brief's low-security, casual framing, but worth flagging to users in the UI (e.g. "don't reuse another player's name").
- This is a real usability tradeoff accepted deliberately, not an oversight — do not "fix" it by quietly adding tokens without raising it first.
