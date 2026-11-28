# 0158. Voice chat is for a game's players only

Status: accepted (2026-11-27), for issue #244. Builds on [ADR 0049](0049-voice-chat.md) (voice chat).

## Context

Voice chat was built for the seated players of a game, and `docs/history/webrtc-tls-plan.md` listed listen-only
spectators as not done. Issue #244 asked whether players could let spectators listen, per game or otherwise.

## Decision

- **A spectator cannot listen to, or join, a game's voice chat.** Voice stays for the game's players, as ADR 0049
  built it. Decided by the maintainer on 2026-11-27 (#244): "No support for spectators in voice chat, which makes all
  other decisions moot."
- So there is no setting, per game or per player, to let spectators listen, and no listen-only mode.

## Consequences

Nothing changes in the code or the API. A change to this needs a new ADR that supersedes this one.
