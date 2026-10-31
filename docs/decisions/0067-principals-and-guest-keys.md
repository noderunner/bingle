# 0067. Principals and guest keys: a seat is its owner's

Status: accepted (2026-10-31)

## Context

Stage 2 of [the accounts plan](../history/auth-and-accounts-plan.md) (§3, §3a). Until now a seat
belonged to nobody: `POST /api/games/{code}/moves` with `player_name` was the whole of the proof,
so anybody who knew a room code — and since ADR 0058 every unfinished game is listed — and a
seated player's name could move for them, read their hand, speak as them or resign their game.
"Resuming" was typing the name again (ADR 0006). The seat tokens of ADR 0050 proved a seat to the
notify endpoints only.

The plan makes the owner of every seat, game and chat line a **principal**: a guest now, an account
later, with one ID either way, so a guest who signs up keeps everything without a rewrite.

## Decision

- **`principals(id, kind, account_id, …)`** and **`guest_keys(principal_id, key_hash, …)`**. A
  principal's ID (`p_` and 16 random bytes in base32) is the server's name for it, not a secret,
  and is shown nowhere. A guest key (`bgg_` and 32 random bytes) is the proof; it is stored as
  SHA-256 (256 random bits need no slow hash) and logged never.
- **`internal/auth`** resolves the principal of every request to `/api`, `/ws*` and `/mcp` once,
  and puts it in the request's context. The session layer reads it from there: no signature in
  `internal/session` changed, and the REST API and MCP get the same rule from the same code.
- **The rule** (`session.mayActFor`): a seat owned by a principal is that principal's alone; a
  seat owned by nobody is acted for by name, and the first principal to do so becomes its owner;
  an anonymous request may act only for a seat nobody owns. Twenty-odd places that found a seat by
  name call it. Private views (`observe`, `state?player_name=`) show a non-owner what a spectator
  sees rather than refusing. A seat's email setting needs a real owner: an anonymous request
  cannot change it even for an unowned seat, which is what the seat token used to guarantee.
- **A browser** gets its guest from its first state-changing request carrying `X-Bingle-Session`:
  the answer sets `bingle_guest` (`HttpOnly`, `SameSite=Lax`, `Secure` unless the connection is
  loopback HTTP, 400 days — the key's own life is the server's business). The page sends that
  first request alone and holds the rest until it is answered, or two of them would make two
  guests and the second cookie would orphan a seat bound by the first.
- **A script** gets a key from `POST /api/guest` and sends it as `Authorization: Bearer`. A bearer
  key that does not work is `401`; a cookie that does not work is cleared and the browser starts
  over as a newcomer.
- **Cross-site requests.** A state-changing request proved by the cookie must carry
  `X-Bingle-Session`, which a cross-site form cannot send; `SameSite=Lax` already keeps the cookie
  off a cross-site POST, and the header holds even where it would not. WebSocket upgrades keep the
  library's same-origin check. The one `sendBeacon` became a `keepalive` fetch, because a beacon
  cannot carry the header.
- **Limits.** One address makes at most 60 guests an hour; past that a page goes on anonymously and
  `POST /api/guest` is `429`. A key unused for `BINGLE_GUEST_IDLE` (90 days) stops working; its
  last use is written at most hourly, so a busy guest is not a write per request.
- **Retired:** `seat_tokens` (the table is dropped), `seat_token` in the create and join answers,
  `X-Seat-Token`, and the browser's `bingle.tokens`. `bingle.session` stays for direct messages and
  who-is-here until stage 8 moves them onto accounts.
- **Contract test.** `TestGuest_EveryRouteThatActsForASeatChecksItsOwner` walks the API's routes
  and sends each, as another guest, a request for somebody else's seat; a new route that forgets
  the check fails it unless it is listed as acting for no seat.

## Why anonymous requests keep the old rule

The plan says an anonymous agent "is a guest". Read literally, every client that does not keep a
cookie — the REST and MCP agents, the scripts, most of the test suite — would become a new guest
on every request and be refused its own seat on the second. So an anonymous request stays
anonymous, and keeps exactly what it had: seats nobody owns, acted for by name. A seat taken by a
browser or by a keyed script is out of its reach. In `closed` mode (stage 6) anonymous requests
go away altogether.

## Consequences

- Typing a seated player's name no longer takes their seat, and a guest whose browser storage is
  cleared cannot resume. Stage 7 adds copying the key to another device; handing an abandoned seat
  to somebody else is a later decision.
- A seat bound before a pie-rule swap moves with its person, like the name.
- Two tabs of one browser are one guest, and own the same seats, which is what a person playing a
  game against themselves needs.
