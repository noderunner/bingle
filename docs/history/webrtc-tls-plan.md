# Plan: TLS (Let's Encrypt) and in-game voice/video (WebRTC)

Status: **Stage 1 (TLS) is implemented** (2026-10-25, native `autocert`, optional; see
`docs/tls.md` and ADR 0048). Stages 2 to 4 (voice, audio only) are implemented too
(2026-10-25; see `docs/voice.md` and ADR 0049). Update the
status log at the bottom as stages land.

## Why

Players want to talk to each other (and maybe see each other in small video tiles)
while playing, in the game page itself. Discord was considered: its voice and video
cannot be embedded in or captured by a web page, short of becoming a Discord
Activity. So the chosen route is **WebRTC inside Bingle**, with Discord kept as an
optional extra (a "join our voice channel" link, turn notifications) for later.

Browsers only allow `getUserMedia` (microphone/camera) on a **secure context**:
HTTPS, or `http://localhost`. Players reaching the server over a LAN address or a
domain need real TLS, which is the groundwork stage below.

## What exists today

- `internal/httpserve` already serves TLS when `BINGLE_TLS_CERT` and `BINGLE_TLS_KEY`
  are both set (static files, no renewal). Otherwise plain HTTP.
- `BINGLE_PUBLIC_URL` (validated in `internal/config`) is used for invite links.
- The game page opens one WebSocket per viewer: `GET /ws/{code}?name=&role=`
  (`internal/api`, `github.com/coder/websocket`). The page builds the URL from
  `location.protocol`, so it follows HTTPS to `wss:` without a code change.
- There are no user accounts. A player is a name in a room; the WebSocket trusts the
  `name` query parameter. Signalling must not make that worse (see Security).
- Container: `Containerfile` exposes `:8080`, volume `/data`.

## Stage 1: TLS groundwork

Goal: `https://games.example.com` works, with automatic certificate renewal.

Decision to make first: **who terminates TLS?**

| Option | How | Notes |
|---|---|---|
| A. Bingle itself, `autocert` | `golang.org/x/crypto/acme/autocert`; `BINGLE_TLS_DOMAIN` (+ optional `BINGLE_TLS_EMAIL`) | One binary, no extra moving parts. Needs ports 80 and 443 reachable from the internet for the HTTP-01/TLS-ALPN-01 challenge, and a persistent cache dir (under `/data/certs`). |
| B. Reverse proxy in front (Caddy, nginx + certbot, Traefik) | Proxy gets the certificate and forwards to `:8080` | Least code, best for a k8s/homelab setup. Bingle needs to trust `X-Forwarded-Proto/For` and the proxy must pass WebSocket upgrades. |
| C. DNS-01 via a client (`lego`, Caddy with a DNS plugin) | Certificate for a name not reachable from the internet (LAN-only play) | The only way to get a trusted certificate for a LAN-only host. Needs a domain at a DNS provider with an API. |

Recommendation: do **A** (it fits "bingle is one binary") but keep static cert files
working, and document **B/C** in the README for people who already run a proxy or
play on a LAN. Ask the user which network shape they have before building:
public internet, or home LAN only (which points to C).

Work items:

1. `internal/config`: `BINGLE_TLS_DOMAIN`, `BINGLE_TLS_EMAIL`, `BINGLE_TLS_CACHE`
   (default `<db dir>/certs`), `BINGLE_HTTP_ADDR` for the redirect/challenge listener
   (default `:80`). Validate that autocert and static cert files are not both set.
2. `internal/httpserve`: an autocert path next to the existing static-file path.
   Second listener on `:80` that answers ACME HTTP-01 and redirects everything else
   to HTTPS. Use the manager's `TLSConfig()`; set `MinVersion: tls.VersionTLS12`.
   Graceful shutdown must stop both listeners. `Serve` stays the only place that
   binds (as its comment demands).
3. Security headers on HTTPS responses: `Strict-Transport-Security` (start with a
   short `max-age`, raise once it has run for a while), and a
   `Permissions-Policy: microphone=(self), camera=(self)` on the game page.
4. `BINGLE_PUBLIC_URL` should default to `https://<domain>` when a domain is set.
5. `Containerfile`: `EXPOSE 80 443`, keep `/data` as the volume (certs live there),
   health check keeps using an HTTP address that works (a loopback listener, or
   `-k` against HTTPS).
6. Tests: unit-test config validation; test the redirect and the ACME challenge
   path with a fake `autocert.Manager` host policy; test that static cert mode is
   unchanged. A real issuance cannot run in CI; document a manual check with the
   Let's Encrypt **staging** directory (`BINGLE_TLS_ACME_URL` for tests only).
7. ADR (`docs/decisions/0036-...`) recording the termination choice.

Done when: a browser on another machine gets a trusted certificate, `getUserMedia`
prompts on the game page, WebSockets connect over `wss:`, and renewal is seen to
work (certificate cache survives a restart).

## Stage 2: WebRTC signalling

Goal: players in a room can set up direct audio connections.

- Reuse the room WebSocket. Add message types, relayed by the server to one named
  peer in the same room: `rtc-offer`, `rtc-answer`, `rtc-ice`, plus presence:
  `rtc-join` / `rtc-leave` so peers know who is in the call. The server never looks
  inside the SDP; it only checks that sender and target are both connected to this
  room and forwards. Cap message size and rate.
- Who may call: seated players and, if wanted, spectators in a listen-only mode
  (a room setting; default players only).
- Topology: **mesh** (each player connects to each other player). Fine for up to
  about 6 players for audio and small video. Cost per player is N-1 uploads, which
  is the reason for the SFU note in Stage 4.
- Server side lives next to the existing hub code in `internal/api` (the WebSocket
  handler); keep it free of any WebRTC library, since it only relays JSON.

## Stage 3: client UI

- Voice panel on the game page: "Join voice" button, mute / deafen, per-player
  speaking indicator (from `AudioContext` analysers), volume slider, push-to-talk
  option (a key, default off).
- Optional video tiles: small, draggable or docked row above the players' panels,
  off by default, camera toggle per player. Audio-only is the default.
- Device pickers (`enumerateDevices`), remembered in `localStorage`.
- Failure states: no secure context (explain that HTTPS is needed), permission
  denied, no devices, peer connection failed (show "retry").
- Everything in plain JS in `internal/web/static/js` (a new `voice.js`), with an
  `h()`-built panel like the rest of the UI. No new dependency.
- Do not start media automatically; the user clicks to join (browsers require a
  gesture for audio playback anyway).

## Stage 4: NAT traversal

Most home networks work with a STUN server alone; strict NATs (corporate, some
mobile, symmetric NAT) need a TURN relay.

- Start with public STUN (`stun:stun.l.google.com:19302`) configurable via
  `BINGLE_RTC_STUN`.
- Add TURN when a connection fails: run **coturn** next to Bingle (container),
  with a static shared secret and **time-limited credentials** minted by Bingle
  (`GET /api/rtc/config` returns `iceServers` with a username/password valid for a
  few hours). TURN also needs UDP and TCP/TLS ports open (3478, 5349 and a relay
  port range); say so in the README.
- If rooms ever need more than about 6 voices, or recording, replace the mesh by an
  SFU (LiveKit is the usual open-source choice). Keep the client behind a small
  interface in `voice.js` so that swap is local.

## Security and privacy

- The WebSocket identity is a name from a query parameter. For signalling that means
  anyone who joins a room under a name could receive offers addressed to that name.
  Mitigations, in order of effort: (1) only relay between connections that already
  passed the room's join check; (2) refuse a second connection claiming a name that
  is already connected; (3) later, bind names to a cookie/token issued at join (this
  is also the foundation for Discord login or any account work).
- Never log SDP or ICE candidates (they contain IP addresses).
- Disclose in the UI that peers see each other's IP addresses in a mesh call; with a
  TURN relay they do not.
- Rate-limit signalling messages per connection; drop oversized frames.
- Camera/microphone indicators are the browser's; do not try to hide them.

## Testing

- Go: signalling relay unit tests (room isolation, unknown target, oversize, rate
  limit); config and TLS tests from Stage 1.
- UI (chromedp): launch Chrome with `--use-fake-ui-for-media-stream` and
  `--use-fake-device-for-media-stream`, open two pages in one room, join voice on
  both, assert both peers reach `connected` and the speaking/mute UI works. The
  existing UI tests serve plain HTTP on localhost, which counts as a secure context,
  so the tests do not need certificates.
- Manual: two real machines over HTTPS, one behind a strict NAT (checks TURN).

## Order of work and rough size

1. Stage 1, TLS: 1 to 2 days with option A, a few hours with option B.
2. Stage 2, signalling relay: half a day.
3. Stage 3, voice UI with fake-device tests: 1 to 2 days (audio first, video after).
4. Stage 4, STUN config then coturn and credentials: half a day to a day.

## Open questions for the user

- Where does the server run: public internet, or home LAN only? (Decides A/B/C.)
- Domain name and DNS provider (needed for Let's Encrypt in all three options).
- Audio only first, with video as a follow-up, or both from the start?
- Should spectators hear or join the call?
- Is a push-to-talk key wanted, or open mic with mute?

## Status log

- 2026-10-17: plan written; no code yet.
- 2026-10-25: Stage 1 built as option A (native `autocert`), optional and off by default:
  `BINGLE_TLS_DOMAIN`/`EMAIL`/`CACHE`, `BINGLE_HTTP_ADDR`, `BINGLE_TLS_ACME_URL`,
  `BINGLE_TLS_HSTS_SECONDS`; `internal/config`, `internal/httpserve`, tests in
  `tests/httpserve/acme_test.go` and `internal/config/config_test.go`. Not done from the
  Stage 1 list: the `Permissions-Policy` header (add it with the voice UI) and a real
  issuance test (needs the internet: follow the staging check in `docs/tls.md`). The
  deployment (domain at register.com, router forwarding) is the user's to set up.
- 2026-10-25: Stages 2 to 4 built, audio only. Signalling is REST in (`POST .../voice/signal`) and
  WebSocket out, delivered to the addressed seat only (a hub room per seat) rather than new
  client-to-server WebSocket messages, so the socket stays server-push. Mesh with the higher seat
  offering; `js/voice.js`; STUN by default, TURN with expiring credentials
  (`BINGLE_RTC_STUN`/`TURN_URLS`/`TURN_SECRET`); `Permissions-Policy` on the game page. Decisions on the
  open questions (the owner said to go ahead): audio first, video later; players only (no spectators);
  open microphone with mute, push to talk optional. Not done: video, listen-only spectators, an SFU,
  a test of a real TURN relay.
