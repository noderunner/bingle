# Plan: authenticated accounts, capabilities and a secure transport

Status: **planned** (2026-10-28). Nothing implemented. The decisions in *What was decided*
were taken with the owner before this was written; the rest is the reasoning, the diagrams
and the staging.

This plan **supersedes [ADR 0006](../decisions/0006-name-only-player-identity.md)** (name-only
identity, no session token) and changes the "explicitly out of scope: accounts, auth, RBAC" line
in [architecture.md](../architecture.md). Every document that says "there are no accounts"
— `ui.md`, `api.md`, the direct-message and who-is-here sections, ADR 0058 and ADR 0059 —
needs a pass as part of the work, not afterwards. (ADRs 0056–0062 arrive with the pending
"land the batch" pull request; this plan assumes they are on `main` by the time stage 1 starts.)

## Why, and the constraint that shapes everything

Bingle is about to grow two features that cannot live on an unauthenticated server: a plugin
system whose install path is code execution, and an observability surface that reveals the
whole server. It already has features that are awkward without identity — direct messages
between people who are only "a name in a browser", email verification that any visitor can
trigger, and a global chat nobody can be held to.

The constraint that shapes the design is the one that has always shaped bingle: **friends
must be able to click a link and play, with no signup**. Everything else follows from
keeping that true while making everything *else* require an account.

Hence two modes and three classes, and a capability for every power, rather than a login
wall.

## What exists today

Three partial identity mechanisms, which accounts must **subsume** rather than sit beside:

| Mechanism | Where it lives | What it proves |
|---|---|---|
| **Seat token** | `bingle.tokens` in the browser, hashed in `seat_tokens` | this browser took that seat (only the notify endpoints use it) |
| **Browser secret → address** | `bingle.session`, published as the first 8 bytes of its SHA-256 | this browser sent that direct message, or is "here" |
| **Verified email contact** | `notify` tables: an address, its hash, a confirmation token, a suppression list | this browser controls that mailbox |

Also true today, and relevant:

- A player is a `player_name` the API is given; `players` has `UNIQUE(game_id, name COLLATE
  NOCASE)`, so names are unique **per game** and owned by nobody.
- A room code is the only access control, and since ADR 0058 every unfinished game is
  **listed** to everyone, so the code is not even obscure.
- `golang.org/x/crypto` is already a direct dependency, so Argon2id costs nothing new.
- TLS is optional: files, ACME (`BINGLE_TLS_DOMAIN`, with the `:80` challenge listener) or
  nothing. `BINGLE_TLS_HSTS_SECONDS` exists.
- Mail works through any SMTP server, with rate limits, a suppression list and hashed
  addresses (ADR 0050), and `internal/mail/fakesmtp` is the in-process fake the tests use —
  the model for a fake OIDC provider.
- `tests/app` runs ~100 browser tests against `httptest.NewServer`, which is **plain HTTP on
  127.0.0.1**. Voice works there because a loopback origin is "trustworthy" to the browser.
- MCP is mounted at `/mcp` and the REST API is open; agents identify themselves with a name.

## What was decided

| Decision | Chosen |
|---|---|
| Modes | **`hybrid` (default) and `closed`**. "Open" (no auth at all) is removed: it no longer makes sense once every deployment has an admin. |
| Classes | **Admin, User, Guest.** Admin is a **group**, so accounts can be added to it later; it implicitly holds every capability. |
| Capabilities | One row per (class, capability). **Seeded from a YAML file on a fresh database**, then owned by the database and managed in the admin UI. |
| Transport | **The connection must be secure** — TLS at bingle (files, ACME or self-signed) *or* plain HTTP behind a proxy with `BINGLE_TRUST_PROXY=1`. Not "bingle must terminate TLS". |
| Dev transport | **Self-signed certificates generated at startup** on request, cached, so CI and debugging stay trivial. |
| Credentials | **Password (Argon2id) + TOTP + recovery codes + generic OIDC**, with a "Sign in with Google" button on a real login/signup page. |
| Admin 2FA | **Required for the admin group**, except on a loopback connection. |
| Signup | **Email required**, and therefore **signup is disabled when SMTP is not configured** (the admin creates accounts instead). |
| First admin | Seeded when the accounts table is empty. **Deterministic `admin`/`bingle123` only under `BINGLE_DEV=1` or on loopback**; otherwise a random password printed once. Either way the first login forces a change. |
| Guests | `play`, `join`, `chat_game`, `voice_game` and — by default, operator-configurable — `create_game`. |
| Accounts only | `dm`, `voice_direct`, `chat_global`, `notify`, `run_batch`. |
| Operator only | `view_metrics`, `moderate`, `install_plugins`, `admin`. |
| TURN relay | **Startup config, not a capability**: `BINGLE_RTC_RELAY=all\|accounts\|none`, with an option to force relay for guests so their addresses never reach a stranger. |
| Live calls | **Capability changes never touch a call in progress.** Capabilities are checked when an action starts. |
| Who is here | **Accounts only.** Guests do not appear. |
| Direct messages | **An account-only system**, to grow a friends list later. |
| Deleted accounts | Renamed to `deleted-<uid>` and referenced by that, so history still displays. |
| API and MCP | **A token is required in `closed`**; a deterministic dev token exists under the same `BINGLE_DEV` gate for CI. |
| Roles | Deferred. Twelve named capabilities and three classes; revisit if the list grows. |

## Design

### 1. Modes, classes and capabilities

```
                     hybrid (default)                 closed
guest   play, join, chat_game, voice_game,     nothing but /healthz, the
        create_game (configurable)            login and signup pages, and
                                              the static assets they need
user    + dm, voice_direct, chat_global,       the same as hybrid
          notify, run_batch
admin   every capability, by definition        every capability
```

Twelve capabilities: `play`, `join`, `create_game`, `chat_game`, `chat_global`, `voice_game`,
`voice_direct`, `dm`, `notify`, `run_batch`, `view_metrics`, `moderate`, plus the two that
arrive with later features, `install_plugins` and `admin`. Every one of them is shown in the
admin UI with a toggle for **User** and for **Guest**; Admin has no column because it always
holds all of them.

**Where the defaults come from.** `bingle-capabilities.yaml` (path from
`BINGLE_CAPS_FILE`, with a sensible built-in default if absent) seeds the database the first
time it is initialised, and is **ignored from then on**: the database is the truth and the UI
is how it changes. `bingle caps reset --from-file` re-applies it deliberately, and the admin
page shows a badge when the database has drifted from the file, so CI can rely on the file
while a live server is managed by hand.

```yaml
# bingle-capabilities.yaml — defaults for a fresh database only
mode: hybrid            # hybrid | closed
signup: open            # open | invite | closed
classes:
  guest:  [play, join, create_game, chat_game, voice_game]
  user:   [play, join, create_game, chat_game, chat_global, voice_game,
           voice_direct, dm, notify, run_batch]
limits:                 # per class, per minute
  guest:  { chat: 10, create_game: 3, login: 5 }
  user:   { chat: 30, create_game: 10, dm: 20 }
```

### 2. Transport: the connection must be secure

Bingle refuses to start unless one of these is true:

1. TLS is configured — certificate files, ACME (`BINGLE_TLS_DOMAIN`), or
   `BINGLE_TLS_SELFSIGNED=1`, which generates a P-256 certificate for the configured host
   plus `localhost` and `127.0.0.1`, caches it in the TLS cache directory so it is stable
   across restarts, and prints its fingerprint; or
2. `BINGLE_TRUST_PROXY=1`, in which case bingle serves plain HTTP, honours
   `X-Forwarded-Proto` and `X-Forwarded-For` from the proxy, and **refuses any request that
   arrives as `http`**.

Enforcement lives in `main`/`internal/httpserve`, **not in the router**, so `app.Router()`
stays transport-agnostic and the existing in-process tests keep using loopback HTTP — which
also keeps the voice tests working, since a bypassed self-signed certificate is a murkier
"secure context" to a browser than `127.0.0.1` is.

The session cookie is `HttpOnly; Secure; SameSite=Lax; Path=/`, with **`Secure` omitted only
for a loopback request**, so logged-in browser tests work against `httptest`.

**The loopback exemptions are a trap, and must be computed from the real peer address.**
Admin 2FA is skipped on loopback, and the deterministic dev password is accepted on loopback
— so with `BINGLE_TRUST_PROXY=1` a proxy on the same host would make *every* request look
local. Rule: when `BINGLE_TRUST_PROXY=1`, **no loopback exemption applies at all**.

### 3. Principals, sessions and tokens

One principal per request, resolved once by middleware and carried in the context:

- **Account** — a session cookie, looked up in `sessions` (server-side, so logging out
  everywhere is a delete). Idle and absolute lifetimes; rotated when privileges change.
- **Guest** — the `bingle.session` browser secret that already exists, which keeps owning
  guest seats so a guest can still resume a game in their own browser. Unused in `closed`.
- **Token** — `Authorization: Bearer bgl_…`, hashed in `api_tokens`, scoped, revocable,
  created on the account page. Agents and scripts use these, never passwords. Admin actions
  are never available to a token.

**CSRF**: a state-changing request authenticated *by cookie* must carry `X-Bingle-Session`
(the page sends it; token clients are unaffected). WebSocket upgrades check `Origin`.
`/ws-dm` becomes session-authenticated and loses its hello-secret frame — a simplification
that falls out of direct messages being account-only.

### 4. Credentials

- **Password**: Argon2id (`x/crypto/argon2`) tuned to ~100 ms, parameters stored with the
  hash so they can be raised later.
- **TOTP**: RFC 6238 on stdlib HMAC and base32 — no dependency. Enrolment shows the
  `otpauth://` URI and the secret for manual entry; a QR image is a later nicety (the one
  genuinely open micro-decision in this plan: a small dependency or ~400 lines of encoder).
- **Recovery codes**: ten single-use codes, hashed, shown once, regenerable.
- **OIDC**: generic, so Google, Authentik, Keycloak and Entra all work from one
  implementation (`golang.org/x/oauth2` + `github.com/coreos/go-oidc/v3`, authorisation code
  with PKCE and `state`). Configured per provider; absent unless configured.
- **Credentials are polymorphic from day one** — `credentials(account_id, kind, …)`, not a
  `password_hash` column — so passkeys can be added without a migration.

**The linking rule, because this is the classic takeover vector.** A provider sign-in links
to an existing account **only** when the provider asserts `email_verified: true` *and* that
address is already verified locally. Otherwise the person signs in with their password first
and links the provider from the account page.

**Admin 2FA** is required for every member of the admin group, skipped only on a genuine
loopback connection. Lost TOTP *and* recovery codes is an operator problem:
`bingle account reset <handle>` on the command line.

### 5. Signup, bootstrap and invitations

Signup takes a handle, an email address and a password, and the address must be confirmed by
the existing mail machinery before the account is usable. **With no SMTP configured, signup
is disabled** and says so plainly; `bingle account create` is always available.

The first admin is seeded when `accounts` is empty: handle from
`BINGLE_ADMIN_HANDLE` (default `admin`), address from `BINGLE_ADMIN_EMAIL` or
`<handle>@<host of BINGLE_PUBLIC_URL>`, and a password that is **`bingle123` only under
`BINGLE_DEV=1` or on a loopback bind** and otherwise random and printed once. The first login
with a seeded password forces a change, and the audit log records it. `signup: invite` adds
an `invites` table and an admin page to mint links.

In `closed` mode an invite link to a game leads to the login page with a **path-only** return
target (never an absolute URL — open redirects are not a feature).

### 6. Identity in the interface

- Accounts have a **handle** (`@ana`, 3–24 characters of `[a-z0-9_-]`, case-insensitively
  unique, a reserved list including `admin`, `bingle`, `system` and `support`) and a display
  name. **Handles are globally reserved**, so no guest can type one as their name; renames
  are allowed but the old handle is held for 30 days.
- A seat shows `@ana` for an account and `Ana (guest)` for a guest, in the seat strips, chat,
  the move list and the who-is-here window.
- The **profile window** (ADR 0058) gains *Sign in* / *Signed in as @ana* and a link to the
  full account page: email, password, TOTP, recovery codes, linked providers, sessions with
  "log out everywhere", and API tokens.
- **"My games" moves server-side** for accounts, which is the reason a friend would bother
  signing up: today `bingle.games` is per-browser, so your games do not follow you to your
  phone.
- The admin area holds accounts, the capability matrix, invites, the audit log and — when the
  plugin plan lands — installation.

### 7. The account-only features, and the people already using them

`dm`, `voice_direct`, `chat_global` and `notify` become account capabilities. Three
consequences to handle rather than discover:

1. **Who-is-here becomes accounts only**, so `/api/here` and `js/here.js` move from the
   browser secret to the session. A quiet server shows an empty list rather than a crowd of
   anonymous names.
2. **Existing confirmed contacts are grandfathered**: they keep receiving turn mail, but no
   new contact can be created by a guest. Without this, every player using notifications
   silently loses them on upgrade.
3. **A confirmed contact is the best signup path there is** — the address is already proven,
   so the flow is "choose a handle and a password and this becomes your account".

Global chat, now account-only and handle-attributed, stops being an anonymous spam surface;
`moderate` (delete a line, mute a handle) plus the per-class rate limits are the rest of the
answer.

### 8. Deleting an account

Seats, moves, chat lines and direct messages reference the player. Deletion therefore
**renames**: the account becomes `deleted-<uid>` with its address, credentials, tokens and
sessions destroyed, and every reference displays that name. Nothing is orphaned and no
history breaks — the same integrity rule the plugin plan needs for uninstalled games.

### 9. Audit log and limits

`auth_events` in the game database: sign-ins and failures, 2FA outcomes, password and email
changes, capability changes, token creation and use, group membership, admin actions. Per-class
rate limits from the YAML seed, enforced per principal *and* per address, with backoff on
login, TOTP and reset; uniform responses so neither handles nor addresses can be enumerated;
reset tokens single-use and short-lived.

## Diagrams

### Listeners, transport and stores

```
                     ┌──────────────────────────── the host ──────────────────────────────┐
 browser  ──TLS 443─►│ public listener            TLS: files | ACME | self-signed          │
 agent    ──TLS 443─►│   /  /api  /ws  /ws-dm  /mcp  /auth/*                               │
                     │   …or plain HTTP behind a proxy with BINGLE_TRUST_PROXY=1,          │
                     │      which must set X-Forwarded-Proto=https                         │
 ACME CA  ──HTTP 80─►│ challenge listener (ACME only) ─► redirects everything to HTTPS     │
                     │                                                                     │
 operator ──HTTP────►│ metrics listener  127.0.0.1:9095                                    │
 (loopback)          │   /metrics  /metrics/history  /metrics/health                       │
                     │                                                                     │
                     │   bingle.db          games, accounts, sessions, capabilities, audit  │
                     │   bingle-metrics.db  samples, slow requests, client errors           │
                     └─────────────────────────────────────────────────────────────────────┘
```

### The request pipeline

```
request
  ├─ secure?  TLS, or X-Forwarded-Proto=https from a trusted proxy ──── no ─► refuse
  ├─ request id · recover · access log                        (observability plan)
  ├─ principal:  session cookie ─► account
  │              bingle.session ─► guest            (hybrid only)
  │              Bearer bgl_…   ─► token
  ├─ CSRF:  cookie-authenticated and state-changing ─► X-Bingle-Session required
  ├─ mode:  closed ─► the principal must be an account
  ├─ capability for this route:  admin ─► all · user caps · guest caps
  └─ handler
```

### Signing in

```
password
  POST /auth/login {handle, password}
    └─ Argon2id ──► 2FA needed? ── no ──► Set-Cookie: session (HttpOnly, Secure, Lax)
                        │ yes
                        └─► POST /auth/totp {code | recovery_code} ──► Set-Cookie: session
  admin group: 2FA always, except a genuine loopback connection
  (and never exempt when BINGLE_TRUST_PROXY=1)

OIDC ("Sign in with Google")
  GET /auth/oidc/{provider}/start ──► provider   (PKCE + state + nonce)
  GET /auth/oidc/{provider}/callback
    ├─ email_verified && the address is verified here ──► link ──► session
    └─ otherwise ──► create an account (choose a handle)   or
                     "sign in with your password, then link the provider"
```

### Playing a game

```
 create   POST /api/games                      create_game
 join     POST /api/games/{code}/join          join       guest: bingle.session owns the seat
                                                          account: the account owns the seat
 move     POST /api/games/{code}/moves         play
 chat     POST /api/games/{code}/chat          chat_game
 live     GET  /ws/{code}                      Origin checked; cookie or guest secret
 list     GET  /api/games                      join       (every unfinished game, ADR 0058)
```

### Voice

```
 a game's call — seat-gated, voice_game, guests included
   A ──POST /api/games/{c}/voice (join)──► server ──"voice"──► B          who is in the call
   A ──POST /api/games/{c}/voice/signal─► server ──"rtc"────► B          SDP and ICE only
   A ◄════════════════ audio, peer to peer (SRTP) ══════════════════════► B
                        └ relay only if BINGLE_RTC_RELAY allows this principal

 a direct call — voice_direct, accounts only
   A ──POST /api/dm/call {to}──► server ──"dm-call" ring──► B    (B must answer)
   A ──POST /api/dm/signal─────► server ──"dm-rtc"────────► B
   A ◄═══════════════ audio, peer to peer ═══════════════════════► B

 the server never sees audio; one call at a time per principal; a capability change
 does not touch a call that is already up
```

### Direct messages

```
   A ──POST /api/dm {to, text}──► server ──"dm"──► B's /ws-dm   (session-authenticated)
                                     └──────────► A's other sessions
   in memory only: 200 messages per pair, forgotten two hours after the last one
   who is here: accounts only — guests do not appear
```

### Agents and tokens

```
   agent ──Authorization: Bearer bgl_…──► /api/…   /mcp
     hybrid:  a token is optional — an anonymous agent is a guest
     closed:  a token is required
     scopes:  play · create_game · read_metrics        (never admin)
     BINGLE_DEV=1 seeds a deterministic token so CI can exercise closed mode
```

## Schema sketch

```sql
accounts(id, handle, handle_lower UNIQUE, display_name, email, email_verified_at,
         status, created_at, deleted_at)
credentials(id, account_id, kind, secret, params, created_at, last_used_at)  -- password|totp|recovery|oidc
groups(name)                                   -- "admin" first; roles later
account_groups(account_id, group_name)
capabilities(class, capability, allowed)        -- class: "user" | "guest"
sessions(id, account_id, created_at, last_seen_at, expires_at, ip_hash, agent)
api_tokens(id, account_id, name, hash, scopes, created_at, last_used_at, revoked_at)
invites(code_hash, created_by, expires_at, used_by, used_at)
auth_events(id, at, account_id, kind, detail, ip_hash)
```

Everything is additive: new tables plus `players.account_id` and `games.owner_account_id`,
both nullable, so existing rows keep meaning what they mean.

## Configuration

| Variable | Default | What |
|---|---|---|
| `BINGLE_AUTH_MODE` | `hybrid` | `hybrid` or `closed` |
| `BINGLE_CAPS_FILE` | `bingle-capabilities.yaml` | seeds a fresh database only |
| `BINGLE_TLS_SELFSIGNED` | off | generate and cache a certificate at startup |
| `BINGLE_TRUST_PROXY` | off | serve HTTP behind a terminating proxy; disables every loopback exemption |
| `BINGLE_DEV` | off | deterministic admin password and API token; loopback only |
| `BINGLE_ADMIN_HANDLE` | `admin` | the seeded account |
| `BINGLE_ADMIN_EMAIL` | `<handle>@<public host>` | its address |
| `BINGLE_ADMIN_PASSWORD` | — | explicit seed password (otherwise random, or `bingle123` under `BINGLE_DEV`) |
| `BINGLE_SIGNUP` | `open` | `open`, `invite` or `closed` |
| `BINGLE_OIDC_*` | — | one block per provider: issuer, client id, secret, scopes, button label |
| `BINGLE_SESSION_IDLE`, `BINGLE_SESSION_MAX` | `14d`, `90d` | session lifetimes |
| `BINGLE_RTC_RELAY` | `all` | `all`, `accounts` or `none`; plus force-relay-for-guests |

## Staged plan

Each stage is one pull request that leaves a working server.

**1. Secure transport.** The start-up rule, self-signed generation and caching,
`BINGLE_TRUST_PROXY` with forwarded-header handling, the `BINGLE_DEV` gate, HSTS review.
Tests: a non-loopback bind with no TLS refuses to start; a trusted proxy's `http` request is
refused while a forwarded `https` one is served; a generated certificate is reused across
restarts; the in-process suites still run on loopback HTTP.

**2. Principal plumbing.** One principal per request, resolved from what already exists
(guest secret, seat token), carried in the context, with every endpoint moved onto it. No
user-visible change, no credentials yet. Tests: every route resolves a principal; the seat
token path behaves exactly as before; a contract test fails when a new endpoint reads
`player_name` without a principal.

**3. Accounts, sessions and password login.** The schema, Argon2id, server-side sessions, the
cookie and CSRF rules, the seeded admin with its forced password change, the login and signup
pages, email confirmation through the existing mailer, and the `bingle account` commands.
Tests: Argon2id parameters survive a round trip; a session is rotated on privilege change;
`Secure` is set off-loopback and omitted on it; signup is refused with no SMTP; enumeration
responses are uniform; a CSRF-less cookie POST is refused.

**4. TOTP, recovery codes and lockout.** Enrolment, verification, the ten codes, the admin
group requirement with its loopback exemption, and `bingle account reset`. Tests: RFC 6238
vectors; a used recovery code cannot be reused; the loopback exemption does not apply behind
a trusted proxy; brute-force backoff.

**5. Generic OIDC.** The start and callback endpoints with PKCE, state and nonce, the linking
rule, the provider buttons, and an in-process fake provider modelled on `fakesmtp`. Tests:
the fake provider drives a full sign-in; an unverified provider address never links; a
tampered state or a replayed code is refused.

**6. Capabilities and modes.** The YAML seed, the tables, enforcement at every call site, the
admin permissions UI, `closed` behaviour, API tokens for REST and MCP, per-class rate limits.
Tests: the YAML seeds once and is then ignored; a guest is refused each account-only
capability in hybrid and everything in closed; a token works and an admin action through a
token does not; `bingle caps reset` re-applies the file.

**7. Identity in the interface.** Handles and the reserved list, the rename hold, guest
markers everywhere a player is named, the account page (email, password, 2FA, providers,
sessions, tokens), server-side "my games", and the audit-log view.
Tests: a guest cannot take a registered handle; a renamed handle is held; "my games" follows
a second session; the account page never shows another account's address.

**8. Migrating the account-only features.** Direct messages and who-is-here onto sessions
(guests dropped from the list), `chat_global` and `notify` gated, confirmed contacts
grandfathered and offered the upgrade path, and the documentation sweep that retires ADR 0006.
Tests: the existing direct-message and notify suites, rewritten to log in; a grandfathered
contact still receives mail; a guest cannot create a contact; the upgrade path turns a
contact into an account without a second confirmation.

## How this changes the other two plans

- **Observability.** The localhost metrics listener **stays** as the operational path — it
  works before auth exists and keeps working when auth or the database is sick, which is
  exactly when it is needed, and scrapers want a token rather than a cookie. The Health page
  additionally appears on the public site behind `view_metrics`. No change to that plan's
  stages.
- **Plugins.** This plan comes **first**. With `install_plugins` and an admin area, installing
  moves into the normal interface with a reauthentication step, and the "admin listener only"
  compromise in the plugin discussion disappears. The recommended order is
  **observability → auth → plugins**.

## Residual risks, stated plainly

- **A seeded password is a loaded gun.** The loopback/`BINGLE_DEV` gate, the random fallback
  and the forced change are the whole defence. If someone sets `BINGLE_DEV=1` on a public
  box, they have published an admin account.
- **Loopback exemptions plus a proxy** would make every request local; the rule that
  `BINGLE_TRUST_PROXY=1` disables every exemption is load-bearing and needs a test, not a
  comment.
- **OIDC linking** is the likeliest account-takeover path if the verified-address rule is ever
  relaxed for convenience.
- **Mandatory TLS raises the floor for a first-time deployment.** Self-signed generation keeps
  it low, but a browser warning on first visit is now part of the home-server experience
  unless ACME or a proxy is used.
- **Losing the "no accounts" property costs simplicity.** Every endpoint gains a check, every
  test gains a session, and ADR 0006's simplicity argument was real. The trade is made
  deliberately: plugins and observability cannot exist safely without it.
- **Guests are still unauthenticated**, so a block or a mute against one is best-effort; only
  accounts can be held to anything. The documentation should say so where it matters.
- **Email becomes load-bearing.** Signup, reset and verification all need SMTP, so a
  misconfigured mail server now blocks onboarding as well as notifications.

## Status log

- 2026-10-28: plan written after a design discussion. Decisions settled: `hybrid` and
  `closed` modes with "open" removed; Admin/User/Guest with admin as a group; twelve
  capabilities seeded from YAML and then owned by the database and the admin UI; a secure
  connection required, with self-signed certificates and a trusted-proxy path; password,
  TOTP, recovery codes and generic OIDC; email required for signup and therefore no signup
  without SMTP; a seeded admin with a deterministic password only under `BINGLE_DEV`;
  guests keep `create_game` by default; `dm`, `voice_direct`, `chat_global` and `notify`
  are account-only; `voice_game` stays open to guests; TURN relay is startup config; live
  calls are never re-evaluated; deleted accounts are renamed `deleted-<uid>`. Nothing
  implemented.
