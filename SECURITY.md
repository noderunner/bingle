# Security

## The model

Bingle is built for a group of friends, with a deliberately small trust model (ADR 0006):

- There are **no accounts or passwords**. A seat in a game belongs to whoever uses its name in that
  game; anyone with the game's link (the room code) can join an open seat or watch.
- The email-notification endpoints are protected by a per-seat token and a per-browser secret
  (ADR 0050). Nothing else is.
- The API, the WebSocket and the MCP server (`/mcp`) are open to anyone who can reach the server.
  The MCP server can create games and play any seat it is told to.
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
