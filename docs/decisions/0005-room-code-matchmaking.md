# 0005. Room codes over a public lobby

Status: Accepted

## Context
Two participants need a way to end up in the same game. Options: a public lobby list of open games anyone can join, shareable room codes/links, or both. A lobby adds UI/API surface (listing, filtering open games) that a casual, low-security app doesn't need yet.

## Decision
Creating a game returns a room code (and shareable link); a second participant joins using that code. No public lobby/discovery list in v1.

## Consequences
- Simpler API surface: no "list open games" endpoint needed.
- AI/MCP clients can join deterministically given a code, which is what automated playtesting needs anyway.
- Two humans who want to play must exchange a code/link out of band (no in-app discovery) — acceptable per the brief's casual scope.
