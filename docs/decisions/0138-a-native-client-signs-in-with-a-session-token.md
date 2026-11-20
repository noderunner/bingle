# 0138. A native client signs in with a session token

Status: accepted (2026-11-19), for issue #227. It builds on [ADR 0068](0068-accounts-sessions-and-passwords.md)
(sessions), [0069](0069-second-factor-and-lockout.md) (the second factor), [0071](0071-capabilities-modes-and-tokens.md)
(API tokens) and [0134](0134-an-api-version-and-a-schema-of-its-messages.md) (the schema).

## Context

A client that is not a browser, such as bingle-3d, needs a signed-in account for friends, direct messages and
calls (ADR 0105). It had no settled way to be one. A guest key cannot use them. An API token (`bgl_`) acts
only within its scopes, none of which covers friends or direct messages, and it is made in a browser and
pasted in, so it is not a sign-in. The session cookie works with any HTTP client, but it brings a browser's
cookie handling and its cross-site header (`X-Bingle-Session`) into every native client.

## Decision

- **A sign-in can answer with a token instead of the cookie.** `POST /auth/login {handle, password, token:
  true}` answers `{token, expires, me}`. When the account has a second factor it answers `{second_factor:
  true, pending}`, and `POST /auth/second {pending, code | recovery_code}` answers the same way. These are the
  same calls as a browser's, so they have the same limits, lockouts and audit events. The pending value
  comes in the body because there is no `bingle_login` cookie to carry it.
- **The token is a session: `bgs_` and the session's secret.** It is the same row in `sessions` as a
  browser's cookie, so there is nothing new to store or to end. It is listed in `/auth/sessions` under the
  client's `User-Agent`, ended by `/auth/logout`, by ending the other sessions and by a new password, and it
  expires on `BINGLE_SESSION_IDLE` and `BINGLE_SESSION_MAX` as a browser's does. A password changed with a
  token answers with the new token.
- **It acts as the account fully**, as the cookie does, and unlike an API token it has no scopes. That
  includes an administrator's powers, which are those of the same account signed in from a browser, and
  the same gates: a password that must be changed, and an administrator's second factor. It is sent as
  `Authorization: Bearer`, which a cross-site form cannot send, so it needs no `X-Bingle-Session`.
  `/ws-dm` accepts it in the header, as `/ws/{code}` accepts a guest key.
- **The client's guest goes with it if the client sends it.** A guest key sent as the bearer of
  `/auth/login` merges that guest into the account, as a browser's guest cookie does (ADR 0068): its seats
  and games follow, and the key stops working. A client that wants a separate guest for games played
  signed out leaves the key off.
- **The schema covers it** (API 1.5.0): the token sign-in's two calls, `/auth/me`, `/auth/logout`, the friends
  calls, `/here`, the direct-message calls, and the `/ws-dm` events as `x-dm-events` and
  `$defs/DirectWebSocketEvent`, apart from `/ws/{code}`'s.

## Considered

- *Paths of their own* (`/auth/token/login`): two sets of the same steps to keep in line. A flag keeps one.
- *New scopes on API tokens* (`friends`, `dm`): smaller, but still made in a browser and pasted in, and not
  a sign-in that can show who is signed in or sign out.
- *The client keeps the cookie*: works today, but makes every native client handle cookies and send a
  browser's cross-site header.

## Not decided here

Signing in through a provider (OIDC), which needs a browser, and how calls' WebRTC works in a native client.
