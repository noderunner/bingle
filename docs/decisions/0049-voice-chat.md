# 0049. Voice chat: WebRTC mesh, signalling over REST and the room socket

Status: accepted (2026-10-25)

## Context

`docs/history/webrtc-tls-plan.md` planned in-game voice with WebRTC after TLS (ADR 0048). Its open questions were left to defaults: audio first, players only, open microphone with mute plus optional push to talk.

## Decision

- **Peer to peer, a full mesh, audio only.** One `RTCPeerConnection` per pair of seats; the higher seat of a pair makes the offer, so offers never collide, and a lower seat asks for a new call with a `retry` message. No WebRTC library on the server.
- **Signalling: REST in, WebSocket out.** The plan had the clients send signalling over the room WebSocket, which is server-push only (`CloseRead`). Rather than make it bidirectional, a message is `POST /api/games/{code}/voice/signal`; the server checks that both seats are in the call, limits size (16 KB) and rate (300 per 10 s per seat), and publishes it into a **hub room of its own per seat** (`<room>#rtc#<seat>`). `handleWS` subscribes a player's socket to its own seat's room only, so offers and ICE candidates (which carry network addresses) are not seen by other seats, by spectators or by anything that follows the room's events. They are not logged.
- **The call list** is in memory (`session/voice.go`): a seat joins and renews with a 15 s heartbeat and lapses after 45 s, the room hears `voice` join/leave events, and only seated humans can join (a name is the identity, as everywhere in bingle; documented in `docs/voice.md`).
- **Network helpers**: `GET /api/rtc/config` gives STUN (default Google's) and an optional coturn relay with credentials in coturn's REST scheme (HMAC-SHA1 of an expiry-bearing user name; 6 hours).
- **The page**: `js/voice.js` keeps the call in module state because the page redraws on every move; the card is rebuilt from it and refreshed in place. The microphone opens only on a click. `Permissions-Policy: microphone=(self), camera=()` is set on the game page.

## Consequences

Players see each other's addresses unless a relay is used (stated in the card). The mesh is fine for the seats of a game, not for large rooms. Video, spectators and an SFU are later work. Tests run two and three real browsers with Chrome's fake microphone; the 45 s expiry and TURN are not covered.
