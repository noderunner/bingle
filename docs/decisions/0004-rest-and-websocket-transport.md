# 0004. REST + WebSocket transport

Status: Accepted

## Context
Clients (web UI, remote API clients, AI/MCP agents) need to create/join games, submit moves, and receive turn notifications. Considered: gRPC (strongly typed, but needs grpc-web proxying for browsers and is heavier for simple remote clients); REST-only with polling (simplest, but no live push); REST + WebSocket (REST for setup/moves, WebSocket for live state push).

## Decision
REST for game creation/join/move/state-fetch; WebSocket for live push of turn changes, applied moves, and join/leave events per room.

## Consequences
- Custom/remote clients only need a basic HTTP + WebSocket client, no protobuf tooling.
- The server must maintain per-room WebSocket subscriber lists and broadcast on every state change.
- Polling as a fallback is still possible for clients that don't want a WebSocket connection, since REST exposes current state too.
