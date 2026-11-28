# 0163. A link opens the 3D client at a game

Status: accepted (2026-11-28), for #312, with the maintainer away: each choice below is one the maintainer may
reverse. It builds on [ADR 0151](0151-the-server-offers-the-3d-client-and-a-setup-file.md) (the server offers the
client, and a setup file with a sign-in code), [0138](0138-a-native-client-signs-in-with-a-session-token.md) (a native
client's session), [0067](0067-principals-and-guest-keys.md) (guest keys) and
[0071](0071-capabilities-modes-and-tokens.md) (tokens, modes). The client's side is bingle-3d #77.

## Context

A player in the browser wants to go on with a game, or watch one, in bingle-3d, which already starts at a game from
its command line (`--server`, `--join`, `--watch`) and keeps its own identity for each server. The page can hand the
system a link in a scheme of the client's own, which the client registers. Three things had to be settled: the
link's exact form, which two repositories keep; how the client comes to be the same player as the browser without
a credential in the link, which ends up in histories and logs and which any page can forge, to any server; and what
the page shows.

## Decision

### The link

```
bingle-3d://join?server=https%3A%2F%2Fgames.example.com&room=K7MQ2X&code=bgc_AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8
bingle-3d://watch?server=http%3A%2F%2F127.0.0.1%3A8080&room=K7MQ2X
```

- **The scheme is `bingle-3d`**, the program's own name, which no other program's is. The host is the action: `join`
  takes an open seat, or the one the player already holds, else watches (`--join`); `watch` watches (`--watch`).
- **Everything else is in the query**, each value percent-encoded as `encodeURIComponent` does, in the order
  `server`, `room`, `code`; a client percent-decodes each one and depends on no order.
  - `server` (required) is the page's own origin: `https://host` or `https://host:port`, or `http://` only when the
    host is this machine (`localhost`, `127.0.0.0/8`, `[::1]`): the client's transport rules. No path (not even
    `/`), query, fragment or user info; a port is 1 to 65535.
  - `room` (required) is the room's code: the server writes six of `23456789ABCDEFGHJKMNPQRSTUVWXYZ`, and a client
    accepts 4 to 16 of `A-Z0-9`.
  - `code` (optional) matches `^bg[cj]_[A-Za-z0-9_-]{43}$`.
- **A client rejects the whole link** for another scheme or action; user info, a port or a path after the action;
  a fragment; a parameter it does not know, or one given twice; a missing `server` or `room`; a value that fails
  its pattern; more than 1024 characters. Nothing in a link becomes a command-line option.
- **A link carries no certificate fingerprint**, unlike the setup file: a fingerprint from a forged link would make
  the client trust an impostor. A server's own certificate is compared on the client's login screen, as it is today.
- **A link without a code works**: the client joins as whoever it already is on that server.

### The code

- **What is in a link is a code good once, for a minute**, bound to the player who asked and to nothing more than
  "be this player on this server": not to the room, and never the session's token, a guest key or an API token.
- **It is minted only by `POST /api/client/code`** from the player's own session: the page's request, so a cookie
  needs `X-Bingle-Session` (ADR 0067) and the gates of a sign-in hold (a password to change, an administrator's
  second factor). A `GET` mints nothing. Thirty an hour per player.
- **An account's code is a sign-in code, as the setup file's is**: the same row of `account_tokens`, kept by its
  hash, spent by one `UPDATE` that also checks it is unspent and in date, ended with the account's other sessions and
  by a new password, and audited as `sign-in-code link`. Only its life differs, a minute instead of a day. No second
  way to mint a credential for an account was added.
- **A guest's code is a row of `guest_codes`**, kept by its hash the same way, since a guest has no account to hang
  a sign-in code on. A new key for the guest, or giving the guest up, ends the guest's codes not yet traded, and so
  does the guest's becoming an account.
- **The client trades it with `POST /auth/client {code}`**, to the server named in the link, over the same transport
  rules it already keeps. An account's code answers `{token, expires, me}`, exactly as `POST /auth/login {code}`
  does: a session of the client's own, listed in the account's sessions under the client's name and ended on its
  own, no stronger than the browser's session (an account for the same account). A guest's code answers
  `{guest_key}`: a new key of the same guest, in the same transaction as the spending, not a copy of the browser's
  key. A second trade fails, and so does one after the minute (`401`, one answer for every reason).
- **A guest the client brings goes with it**, as on signing in (ADR 0138): a guest key sent as the bearer is merged
  into the code's player and stops working. A client that wants to keep that guest apart leaves the key off.
- **An administrator gets no code** (`403`), and an API token none. The setup file gives an administrator's account
  its sign-in code, behind the second factor's gate; but a link is clicked many times a day, from pages on many
  servers, and the admin group's powers should not ride on one. The page opens the link without a code, and the
  client joins as whoever it is, or signs in with the password and the second factor.
- **The trade is under `/auth`**, so a closed server (ADR 0071) lets a client that is not signed in yet reach it,
  as it lets one reach the sign-in. It is always there; a server that offers no client mints no codes, and refuses
  every one. Trades of guests' codes are limited to thirty an hour per address, and an account's count as sign-in
  tries.
- **Nothing on the server trusts the link's server address**: the server never sees the link. It mints a code for
  the session that asks and trades it for whoever brings it, which is what "once, for a minute" is for.
- **Neither the code nor the link is recorded.** The access log carries route patterns, not paths; the code travels
  only in a request's and an answer's bodies; the audit log names what a code was for, not the code; and a metric's
  labels are routes and statements.

### The page

- **Beside *Join* and *Watch* wherever the page offers them, and on a game the player already sits in**: the public
  lobbies, the games going, the player's own games that are not over, the game's page (its join card, and the seated
  player's controls), and a game invitation, which the link accepts first.
- **Not on a phone, or on a platform with no build**: the link is shown in the desktop layout on Windows or Linux,
  as the quick setup page tells them apart, when the server offers a build for it. Steam Frame has no page to click
  a link on.
- **Always tried; if not found, the download**: a page cannot ask whether a scheme has a program behind it. A click
  mints a code and opens the link; if the page has neither lost focus nor been hidden a moment later, *Download Game
  Client*, the logo menu's own words, appears beside it, a link to the quick setup page.
- **The words** are *Join with Bingle3D* and *Watch with Bingle3D*: watching has its own, since *Join* beside *Watch*
  would say the wrong thing.
- **A seat held in both stays connected in both.** The server already lets one seat be held from several
  connections, as two tabs are: a seat counts its sockets and shows connected while any is open, and any of them may
  act. The link adds no rule.
- **A server the client cannot reach as the browser does** (a certificate it has not compared, an API it does not
  go with, a server it cannot reach) is shown on the client's own login screen with the reason it already gives; a
  code gone stale meanwhile is refused, and the client joins as whoever it is, and says so.

## Considered

- *The session's token, or the guest's key, in the link*: nothing to trade, but a live credential in every history,
  log and crash report the link reaches.
- *A code bound to the room as well*: no use, since the code is spent before the client joins, and a client that
  is the player may sit wherever the player may.
- *One table for both kinds of code*: one statement fewer to read, but a second way to mint an account's sign-in
  beside the setup file's, with its own ending to keep in line with the sessions'.
- *A narrower session for an administrator*: there is none to give. A code that asks for the second factor again
  would be a second sign-in flow for one case.
- *The trade under `/api/client`, beside the code's minting*: a closed server would refuse it to a client that has
  not signed in yet, which is every client that needs it.
- *A link the server builds*: one encoder in Go, but the page would still build a link without a code when no code
  is given.

## Consequences

- `version.API` 1.30.0: `POST /api/client/code` and `POST /auth/client` in the schema.
- bingle-3d #77 registers the scheme and builds to the contract above, in `docs/client-builds.md` too.
- The link works only once the client registers it; until then the page offers the download after every click.
