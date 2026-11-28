# 0159. No media server until growth needs one; then an SFU

Status: accepted (2026-11-28), for issue #245. Builds on [ADR 0049](0049-voice-chat.md) (voice chat).

## Context

Voice chat connects every player to every other, a mesh (`docs/history/webrtc-tls-plan.md`). That suits two to five
seats, but it grows as the square of the number of people, and each player uploads one stream per other player. The
plan listed a selective forwarding unit (SFU), which relays each stream once, as not done, and issue #245 asked
whether one is needed at all given today's table sizes, whether it would be built into the binary or run by
operators as TURN is, and when the server would switch from the mesh to it.

## Decision

- **No media server is built now**, and none will be until growth brings an actual need for it. The mesh stays as
  ADR 0049 built it. Decided by the maintainer on 2026-11-27 (#245): "This will not be added until there is an actual
  need for it due to growth."
- **If larger tables are ever needed, an SFU is the default strategy.** The maintainer: "implementing SFU will be the
  default strategy if larger tables are ever needed."
- Which SFU, whether it is built in or run by operators, when the server switches to it, and what it costs to host are
  not answered here. They are left for the day it is needed.

## Consequences

Nothing changes in the code or the API. When growth brings the need, the work starts from an SFU and settles those
questions then, in a new ADR.
