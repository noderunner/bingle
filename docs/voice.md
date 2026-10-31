# Voice chat

The players of a game can talk to each other from the game page: audio only, peer to
peer (WebRTC). Nothing is recorded and the audio never passes through the server.

## For players

* The **Voice** card appears for seated players when there is somebody to talk to (another
  person's seat, taken or open). Spectators and people playing only bots do not get it.
* **Join voice** asks for the microphone (the browser prompts) and puts you in the call.
  Everybody in the call hears everybody else. **Mute** closes your microphone, **Deafen**
  silences the others (and mutes you), **Leave** ends it. Each person has a volume slider.
* **Push to talk**: tick it, choose the key (default `V`); the microphone is open only while
  the key is held (not while typing in a text box).
* With several microphones or speakers the card offers pickers (speaker choice needs a
  browser that supports it; Chrome does). Choices are remembered in the browser.
* Voice needs a **secure connection**: `https://` (see [tls.md](tls.md)) or `localhost`.
  Over plain `http://` the card says so instead of offering the button.
* Players in the call see each other's network address (that is how a direct connection
  works). A TURN relay (below) hides it.
* Reloading the page leaves the call: join again (browsers want a click before they open
  the microphone).

## How it works

1. **The call list.** `POST /api/games/{code}/voice {player_name, active}` puts a seat in
   the call (or takes it out); the page repeats it every 15 s, and a seat that stops is
   dropped after 45 s. `GET /api/games/{code}/voice` lists the seats. The whole room hears
   `voice` events (`{kind: join|leave, seat, name}`) over its WebSockets.
2. **Finding each other.** Every pair of seats has one connection. The **higher seat makes
   the offer**, so the two never offer at once. The browsers exchange session descriptions
   and ICE candidates with `POST /api/games/{code}/voice/signal {player_name, to, kind,
   data}` (`kind` is `offer`, `answer`, `ice` or `retry`). The server checks that both seats
   are in the call, caps a message at 16 KB and a seat at 300 messages per 10 s, and
   delivers it as an `rtc` event **only to the call's WebSocket of the seat it is for**
   (`/ws/{code}?only=voice`, `session.SubscribeRTC`: a hub room per seat, so no other seat,
   spectator or follower of the game's events can see it; the socket a page uses never gets
   them). It never reads or logs what is inside. The call opens its socket *before* it
   announces itself and waits for the server's `joined` (sent after it listens), so the offers
   that follow are not lost.
5. **Across pages and games.** The call belongs to the game it was joined in and lives in
   `voice.js`, not in a page: while it is up, links to other pages of the site are followed
   without reloading the document (`js/nav.js`, ADR 0052), so the connections, the microphone and
   the sound go on when the player opens another game or the main page. A button in the top bar
   (`#voice-chip`) shows the call from every page, with the room code when it is in another game,
   and has Mute, Deafen, Leave and a way back. Reloading the page, or opening a game in
   another tab, still leaves the call (one call per tab).
3. **The network helpers.** `GET /api/rtc/config` returns the `iceServers` for the browser:
   STUN servers, and a TURN relay with time-limited credentials when one is configured.
4. **Trust.** A seat is its owner's — the guest or the account that took it (ADR 0067) — the
   same trust as making a move. Only seated people can join, and signalling only flows
   between seats in the call. A guest is anybody with the guest's key.

## Settings

| Variable | Default | Meaning |
|---|---|---|
| `BINGLE_RTC_STUN` | `stun:stun.l.google.com:19302` | Comma-separated STUN URLs; `none` for no STUN |
| `BINGLE_RTC_TURN_URLS` | *(unset)* | Comma-separated `turn:`/`turns:` URLs of your relay |
| `BINGLE_RTC_TURN_SECRET` | *(unset)* | coturn's `static-auth-secret`; required with the URLs |

Most home networks work with STUN alone. Players behind a strict NAT (some mobile and
corporate networks) need a relay.

### A TURN relay with coturn

```conf
# /etc/turnserver.conf
listening-port=3478
tls-listening-port=5349
realm=bingle.example.com
use-auth-secret
static-auth-secret=<the same long random string as BINGLE_RTC_TURN_SECRET>
cert=/path/fullchain.pem        # for turns: (optional)
pkey=/path/privkey.pem
no-multicast-peers
denied-peer-ip=10.0.0.0-10.255.255.255      # do not let the relay reach your LAN
denied-peer-ip=192.168.0.0-192.168.255.255
denied-peer-ip=172.16.0.0-172.31.255.255
```

With podman, host networking avoids mapping the relay range (put `external-ip=<public ip>`,
`min-port` and `max-port` in the config, and keep the file at mode 0600 outside the repo):

```sh
podman run -d --name bingle-coturn --restart=unless-stopped --network=host \
  -v ~/.config/bingle/turnserver.conf:/etc/coturn/turnserver.conf:ro,Z \
  docker.io/coturn/coturn:latest -c /etc/coturn/turnserver.conf
```

If the public address changes, update `external-ip` and restart the container.

Forward UDP and TCP 3478 (and 5349 for `turns:`) and coturn's relay range
(`min-port`/`max-port`, default 49152-65535; narrow it) to the machine, then start bingle
with `BINGLE_RTC_TURN_URLS=turn:bingle.example.com:3478,turns:bingle.example.com:5349
BINGLE_RTC_TURN_SECRET=...`. Bingle gives every client credentials that expire after 6 hours:
the user name is the expiry time, the password the HMAC-SHA1 of it under the secret
(coturn's REST scheme), so the relay needs no user list.

## Limits

Audio only, in a full mesh: each player uploads one stream per other player, which is fine
for the seats of a game (up to about 6). Video, spectators listening in and an SFU for
larger rooms are not built.

## Testing

`tests/app/voice_test.go` covers the endpoints, the relay's isolation (the right seat and
nobody else), limits and the TURN credentials; `tests/app/voice_ui_test.go` runs real
browsers with Chrome's fake microphone (`--use-fake-ui-for-media-stream
--use-fake-device-for-media-stream`) and checks two- and three-player calls, mute, deafen,
push to talk and the refusal messages. Not covered by tests: the 45-second expiry, and
connections through a TURN relay or a strict NAT (try two real machines).

## The call of a direct message

A call between two people who are not at the same table works the same way, with
a person's **address** in place of a seat (see [api.md](api.md) and ADR 0059):
`js/dm-call.js` keeps one `RTCPeerConnection`, the lower of the two addresses
makes the offer, and `POST /api/dm/signal` relays the offer, the answer and the
candidates to the other browser's `/ws-dm` socket. It borrows the microphone and
the ICE servers from `js/voice.js` (`micStream`, `iceServers`), so both kinds of
call ask for the same thing of the browser.

**A person talks in one place at a time.** A browser that sends its secret when
joining — a game's call or a direct one — is held to a single call: joining a new
one takes it out of the old and tells it (the room gets a `voice` leave for that
seat, the other end of a direct call a `dm-call` end marked `dropped`). The page
that was dropped takes its call down and says why. An API client that sends no
secret is not tracked.
