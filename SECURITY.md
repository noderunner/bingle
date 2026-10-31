# Security

## The model

Bingle is built for a group of friends that may grow ([the accounts plan](docs/history/auth-and-accounts-plan.md)):

- **Guests play without signing up.** A seat belongs to the guest (a browser, by the key in its
  cookie, or a script with a guest key) or the account that took it (ADR 0067); anyone with the
  game's link can join an open seat or watch. A guest is anybody who holds its key.
- **Accounts** sign in with a password (Argon2id) or an OpenID Connect provider, and may add TOTP;
  administrators must (ADR 0068–0070). Direct messages, the global chat, email notifications and
  practice batches are accounts' (ADR 0071, 0073).
- **`BINGLE_AUTH_MODE=closed`** makes a server for its members only; API tokens give scripts and
  agents an account's scoped powers, never an administrator's.
- An anonymous client — no key, no token — can still play seats nobody owns, on a `hybrid` server.
- Voice chat sends audio directly between the players' browsers; players in a call can see each
  other's network addresses unless a TURN relay is used (docs/voice.md).

So: put it on a private network or behind a VPN or a reverse proxy with its own authentication if you do
not want strangers using it, and use HTTPS (`BINGLE_TLS_DOMAIN`, docs/tls.md) if it is reachable
from the internet. The server limits message sizes and rates where it can, but it is not hardened
against a determined abuser.

## Reporting a vulnerability

Please do **not** open a public issue for a security problem. Use GitHub's private reporting:
the **Security** tab of the repository, then **Report a vulnerability**. You will get an answer
within a few days. There is no bounty, but you will be credited if you like.

Secrets never belong in the repository: `.gitignore` keeps out certificates, keys, `.env` files and
databases. If you find one that was committed, tell us the same way.
