# 0048. Optional native TLS with automatic Let's Encrypt certificates

Status: accepted (2026-10-25)

## Context

`docs/history/webrtc-tls-plan.md` (Stage 1) left open who terminates TLS. The owner will point a name under their own domain at their home server and wants to run bingle with and without TLS while that is being set up.

## Decision

- Option A of the plan: bingle does it itself with `golang.org/x/crypto/acme/autocert`, **off unless `BINGLE_TLS_DOMAIN` is set**. Plain HTTP stays the default, and the certificate-file mode (`BINGLE_TLS_CERT`/`KEY`) is unchanged; the two cannot be combined.
- With a domain set the address defaults to `:443`, and a second plain listener (`BINGLE_HTTP_ADDR`, default `:80`) serves HTTP-01 challenges, `/healthz`, and a permanent redirect to HTTPS (to the public URL's port when it has one). Losing that listener is logged, not fatal: TLS-ALPN-01 on the HTTPS port still works.
- Only the configured names get a certificate (`HostWhitelist`); certificates and the account key are cached in `BINGLE_TLS_CACHE` (next to the database by default). Certificates are fetched at start (a loopback handshake per name) so problems show in the log and the first visitor does not wait; renewal is autocert's.
- `BINGLE_TLS_ACME_URL` points at another directory (the Let's Encrypt staging one for a first try). `BINGLE_TLS_HSTS_SECONDS` is opt-in, default off, because HSTS locks browsers out of plain HTTP for the name.
- `BINGLE_PUBLIC_URL` defaults to `https://<first domain>`.
- DNS-01 is not built in (autocert does not do it): names that cannot be reached from the internet use certificate files made by `lego`/`certbot`.

## Consequences

New dependencies `golang.org/x/crypto` and `golang.org/x/net` (and newer `x/sys`, `x/text`). Real issuance cannot run in CI, so `tests/httpserve/acme_test.go` covers everything around it (host policy, redirect, challenge path, shutdown, HSTS) against an unreachable authority; the staging check in `docs/tls.md` is the manual test. The `Permissions-Policy` header of the plan waits for the voice UI.
