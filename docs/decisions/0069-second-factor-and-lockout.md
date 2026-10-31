# 0069. A second factor, recovery codes and lockout

Status: accepted (2026-10-31)

## Context

Stage 4 of [the accounts plan](../history/auth-and-accounts-plan.md) (§4, §9): TOTP, recovery
codes, the administrators' second factor with its loopback exemption, backoff on repeated
failures, and `bingle account reset` for the person who lost both the phone and the codes.

## Decision

- **TOTP** is RFC 6238 on the standard library (HMAC-SHA1, six digits, thirty seconds, a step of
  skew either side) — no dependency. A step once used is spent (the credential keeps the last one),
  so a code read over a shoulder cannot be replayed. Setting it up asks for the password, shows the
  base32 key and the `otpauth://` link as text (no QR code yet: the plan's one open micro-decision
  stays open), and is on only once a code from the app confirms it.
- **Ten recovery codes** (two groups of five, about fifty bits each), hashed, shown once, each
  usable once instead of a code, renewable with the password.
- **Signing in** with a second factor is two requests: the password answers
  `{second_factor: true}` and a short `bingle_login` cookie (five minutes, five wrong codes), and
  `POST /auth/second` with a code or a recovery code finishes it. Waiting sign-ins are kept in
  memory; a restart only means typing the password again. Turning the factor on or off ends every
  other session, like a password change.
- **Administrators** (the `admin` group) sign in with a second factor. One without, off this
  machine, may do nothing but `/auth` and reads until one is set up; on a genuine loopback
  connection the requirement is waived, and behind `BINGLE_TRUST_PROXY` nothing is loopback.
- **Backoff.** Five failures within a quarter of an hour lock the key for a minute after the last,
  doubling with each further failure, an hour at most; a success clears it. Keys are a handle (any
  handle, so a lock does not say whether an account exists), an account's codes and
  re-authentications, and an account's reset links. In memory: a restart forgets them.
- **A forgotten password** is reset by mail: `POST /auth/forgot` (handle or address; one answer
  whether or not there is such an account), a link that works once for an hour, and a new password
  that ends every session. An account with a second factor gives its code too, so a mailbox alone
  does not get past it; the link survives a missing code. Reset mail has its own kind, so it is not
  held back by a confirmation sent a moment before.
- **`bingle account reset HANDLE`** now also removes the second factor: whoever can run it on the
  server is the way back for somebody who lost everything.

## Consequences

The TOTP secret is stored as it is (it has to be read to check a code); the database is as
sensitive as the plan already takes it to be. The lockout can be used to keep somebody out of
their account for up to an hour by failing on purpose; that is the usual trade, and the
per-address limits bound it.
