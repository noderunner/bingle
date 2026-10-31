# 0068. Accounts, sessions and passwords

Status: accepted (2026-10-31)

## Context

Stage 3 of [the accounts plan](../history/auth-and-accounts-plan.md) (§3–§5): accounts that sign in
with a password, server-side sessions, the first administrator of an empty server, and signing up
with a confirmed address. Guests stay exactly as ADR 0067 left them; an account is the other kind
of principal.

## Decision

- **Tables:** `accounts`, `credentials` (by kind: `password` now; TOTP, recovery codes and
  providers are later kinds, no migration), `groups` (`admin` exists from the start),
  `account_groups`, `sessions`, `account_tokens` (one-time links) and `auth_events` (the audit
  log). An account has a principal like a guest does; it was the guest's, when a guest signed up.
- **Passwords** are Argon2id, OWASP's parameters (19 MiB, two passes, one lane), written into the
  hash in the PHC format so they can be raised later: a hash made with weaker ones checks, and is
  remade at its next sign-in. 10 to 256 characters, nothing else checked.
- **Sessions** are server-side: the `bingle_session` cookie holds 32 random bytes, the table their
  SHA-256, so signing out is a delete and so is "everywhere". Fourteen days unused or ninety at
  all end one (`BINGLE_SESSION_IDLE`, `BINGLE_SESSION_MAX`). A changed password ends every session
  of the account and gives the browser that changed it a new cookie. The cookie has the guest
  cookie's flags (`HttpOnly`, `SameSite=Lax`, `Secure` except on loopback HTTP) and the same
  cross-site rule (`X-Bingle-Session` on a state-changing request).
- **The resolver** reads the session cookie before any guest key; an account's principal is the
  account's.
- **Signing up** takes a handle, an address and a password, and mails a link (through the
  notifier's queue and limits); the account is unusable until the link is followed, and forgotten
  after 48 hours if it never is. Without mail, signing up is off, and says so; `BINGLE_SIGNUP=closed`
  turns it off with mail. An address already in use gets the answer a new one gets and no mail.
- **The guest becomes the account.** Signing up from a guest browser gives the account the guest's
  principal; confirming makes it an account's and retires the guest's keys. Signing in from a guest
  browser that is somebody else merges that guest into the account: its seats, games and lines move
  over, and it records where it went. Nothing is lost by signing in.
- **Signing in** is one answer for a handle nobody has and a wrong password, in the same time (a
  dummy hash is checked); 10 tries a minute per address and 10 per 15 minutes per handle.
- **The first administrator** of an empty server is `BINGLE_ADMIN_HANDLE` (`admin`) in the `admin`
  group, its password `BINGLE_ADMIN_PASSWORD`, or `bingle123` under `BINGLE_DEV` or on a loopback
  bind — never behind `BINGLE_TRUST_PROXY` — or else a random one printed once on the terminal,
  never in the log. Whatever it is, it must be changed first: until then the account may sign out,
  read, and change its password, and every other state-changing request is `403`.
- **The command line:** `bingle account list`, `create HANDLE EMAIL [--admin]` and `reset HANDLE`
  print a password that must be changed at the next sign-in; `reset` also signs the account out
  everywhere.
- **Pages:** `/login`, `/signup`, `/account` (who you are, a new password, sign out) and
  `/account/confirm`, whose button — not the link — confirms, so a mail scanner cannot. The
  profile page says whether the browser is a guest or signed in.

## Left for later stages

Handles are checked and reserved here, but a guest can still type a registered handle as a name
in a game until stage 7. Invitations (`BINGLE_SIGNUP=invite`) need the admin pages of stage 6.
TOTP, recovery codes and the admin group's second factor are stage 4.
