# 0065. The connection must be secure, and the default bind is loopback

Status: accepted (2026-10-29)

## Context

Stage 1 of [the accounts plan](../history/auth-and-accounts-plan.md) is the transport that
everything after it depends on. Bingle is about to start carrying passwords, session cookies,
two-factor secrets and API tokens, and none of those can cross a plain-HTTP connection to a
machine anyone else can reach.

What makes this a decision rather than a switch is the constraint the whole plan turns on:
**friends must be able to click a link and play, with no signup** — and, just as importantly,
somebody must be able to download one binary, run it, and have a server. TLS is currently
optional (ADR 0012, ADR 0048), the default bind is every interface, and ~100 browser tests run
against `httptest` over plain loopback HTTP. A rule that said "bingle must terminate TLS" would
break all three.

## Decision

**A client's connection to bingle must be secure, and bingle refuses to start otherwise.** Secure
means one of four things, checked in `config.Load` rather than at the first request — a server
that would hand out passwords over plain HTTP should not come up at all:

1. certificate files (`BINGLE_TLS_CERT`/`_KEY`);
2. automatic certificates (`BINGLE_TLS_DOMAIN`, ADR 0048);
3. **a certificate bingle generates for itself** (`BINGLE_TLS_SELFSIGNED=1`), cached in the TLS
   cache directory so it is the same one after a restart, with its SHA-256 fingerprint printed;
4. **a terminating proxy** (`BINGLE_TRUST_PROXY=1`), where bingle serves plain HTTP, believes the
   proxy's `X-Forwarded-Proto` and `X-Forwarded-For`, and refuses any request that did not arrive
   as `https`.

The rule is "the connection must be secure", **not** "bingle must terminate TLS". Most real
deployments put something in front, and telling them to stop would be both annoying and worse:
a proxy that already terminates TLS does it better than bingle will.

**The one exemption is a bind only this machine can reach.** A loopback listener may serve plain
HTTP, because the connection never leaves the host.

**So the default bind changes from `:8080` to `127.0.0.1:8080`.** This follows from the exemption
rather than being a separate decision: the one bind nobody chose has to be the one the rule
exempts, or `./bin/bingle` with nothing configured would refuse to start. Bingle out of the box is
now a server for the machine it runs on, and making it reachable is a deliberate act that comes
with a certificate.

**Enforcement is in `internal/httpserve`, never in the router.** `app.Router()` stays
transport-agnostic, so the in-process suites keep running over loopback HTTP — which also keeps
the voice tests working, since `127.0.0.1` is a *more* trustworthy origin to a browser than a
bypassed self-signed certificate is.

**A trusted proxy turns every loopback exemption off.** This is the load-bearing part.
Later stages exempt a local operator from admin two-factor and accept a deterministic development
password on loopback. A proxy on the same host makes *every* request arrive from `127.0.0.1`, so
`BINGLE_TRUST_PROXY=1` must mean nothing is local — enforced once, in
`transport.Policy.Loopback`, rather than remembered at each call site. `BINGLE_DEV` and
`BINGLE_TRUST_PROXY` are refused together for the same reason.

**HSTS is not sent with a generated certificate.** `BINGLE_TLS_HSTS_SECONDS` now applies behind a
trusted proxy as well as with real certificates, because that is where a browser can act on it.
With a self-signed one it is deliberately ignored and a warning is logged: HSTS plus a certificate
nothing trusts is a warning page with no way through, which would make `BINGLE_TLS_SELFSIGNED` a
trap instead of a convenience.

**Forwarded headers are read only from a trusted proxy, and only two of them.** `X-Forwarded-Proto`
and `X-Forwarded-For`; RFC 7239's `Forwarded` is not read, because two spellings of one claim is
two things to get wrong. With no proxy trusted they are ignored completely — anyone can send them.
`X-Forwarded-For` is read **rightmost-first**, assuming the one hop `BINGLE_TRUST_PROXY` describes:
a proxy appends the peer it saw, so whatever a client wrote is to the left of it.

## Consequences

* **A server reachable from a LAN stops working on upgrade until it is told how.** Somebody
  running `./bin/bingle` and reaching it from another machine on their network will find it
  loopback-only, with a startup error naming the four ways to fix it. That is the intended
  outcome and it is the most disruptive thing in this plan; the changelog says so plainly.
* **The rate limiters now see the real client behind a proxy.** `clientIP` moved onto the
  transport policy. Without that change, `BINGLE_TRUST_PROXY` would have made every request in
  the world share one address, and the first visitor to trip a limit would have tripped it for
  everybody — a bug this stage would otherwise have introduced.
* **A generated certificate is a convenience, not a security measure.** A browser will not trust
  it, the warning has to be clicked through once per browser, and anybody who can intercept the
  connection can present their own. It exists so that "the connection must be secure" does not
  make a laptop, a container on a private network, or a machine reached over a tunnel into a
  chore.
* **`BINGLE_DEV` exists from this stage but does nothing yet.** It is gated here — loopback only,
  never with a proxy — so that stages 3 and 4 inherit the gate rather than inventing it.
* **ADR 0012's "TLS is optional" is narrowed**, not reversed: it is still optional, on loopback or
  behind a proxy. ADR 0006 (name-only identity) is untouched until stage 8 retires it.
