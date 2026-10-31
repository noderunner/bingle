# 0059. Direct messages, their voice calls, and one voice at a time

Status: accepted (2026-10-27); direct messages and calls are between signed-in accounts since [ADR 0073](0073-account-only-features.md), with no browser secret

## Context

`bingle_tasks.md` asked for "a new icon to the top nav bar for direct messaging and a list of all
currently 'active' unique users with active browser sessions where users can be clicked to start
new direct messages", for voice chat "within a direct message session", and for an active user to
"only be connected to a single voice chat session at a time, automatically disconnecting for any
current session if they join a new one".

Bingle has no accounts (ADR 0006): a name is whatever somebody typed. ADR 0058 gave a browser an
identity of the only kind that can exist here — a **secret** it makes for itself, published as the
**address** that secret hashes to — so that *who is here* could be listed. Direct messages need
exactly that: somewhere to send to, and proof of who is sending.

The owner was asked how far to take it and chose: a per-browser id with **ephemeral** messages, not
stored in SQLite, with the interface saying plainly that names are self-chosen.

## Decision

**Addressed to a browser, signed by its secret.** A message is sent with the sender's own secret
and addressed to the recipient's address. The server derives the sender's address from the secret,
so **the sender of a message cannot be faked** while anybody who is here can be written to.
What a message proves is that it came from the same browser as the last one — not whose browser
that is, which the window says in as many words.

**Nothing is stored.** The messages live in memory: 200 per pair, 2000 pairs, a pair forgotten two
hours after its last message, and all of it gone at a restart. A message to somebody who is not
here is refused (`404`) rather than queued: this is a way to say "fancy a game?", not a mailbox.
Writing without having said who you are is a `403`, and a flood a `429` (the chat's own limiter,
keyed by address).

**Delivery is the hub, over a socket of its own.** `/ws-dm` carries one browser's messages, the
ringing of a call and the WebRTC messages of one. The secret cannot travel in the URL — it would be
in every access log — so it comes as the socket's **first frame** (`{"type":"hello","secret":…}`)
and nothing else is ever read from it. From there on it is the same shape as everything else here:
REST in, WebSocket out.

**The call is a module of its own** (`js/dm-call.js`): one `RTCPeerConnection`, the lower of the two
addresses making the offer, the microphone and the ICE servers borrowed from `js/voice.js`
(`micStream`, `iceServers`) so that both kinds of call ask the browser for the same thing.
Generalising `voice.js` — 900 lines built around seats, a room's socket and a mesh — to carry a
second kind of identity would have put the game calls at risk for no gain; the two share what is
worth sharing and nothing else. A direct call outlives the page, as a game's call does (nav.js).

**One voice at a time, kept by the server.** A browser that sends its secret when joining a call —
a game's (`POST /games/{code}/voice`) or a direct one — is held in a small registry of "where this
browser's voice is". Joining a new call takes it out of the old one and tells that call: the room
gets the ordinary `voice` leave event for the seat, and the other end of a direct call gets
`dm-call` with `dropped`. The page that was dropped takes its own call down and says why. Doing it
on the server rather than in the page is what makes it hold when somebody opens a second tab, and
what stops a browser that crashes mid-call from being counted as still talking. An API client that
sends no secret is not tracked and keeps the old free-for-all.

**One thing had to change underneath.** `nav.js` kept a *single* predicate for "something must
outlive this page", which voice.js set; a second caller would have taken its answer away and a
game's call would have died on the next link. It now keeps every caller and a soft navigation
happens while any of them says so — which a test caught at once.

## Consequences

* The list of who is here is read every eight seconds while a window that shows it is open, and
  every minute otherwise: somebody arriving appears while you are looking at them.
* The profile window's list of people gained a *Message* button, so the two windows meet.
* A person's messages are readable by anybody holding their browser's secret, which is to say by
  that browser. There is nothing to take from the server: no history is kept anywhere else.
* Voice is still refused to spectators in a game (that is the seat rule), but a direct call needs
  no seat at all: two people who are merely here can talk.
