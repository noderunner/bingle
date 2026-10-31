# Plan: authenticated accounts, capabilities and a secure transport

Status: **stages 1–4 done, stages 5–8 planned** (2026-10-29; guests amended 2026-10-31). Stage 1
(secure transport) settled [ADR 0065](../decisions/0065-the-connection-must-be-secure.md), stage 2 (principals and guest keys) [ADR 0067](../decisions/0067-principals-and-guest-keys.md), stage 3 (accounts, sessions, passwords) [ADR 0068](../decisions/0068-accounts-sessions-and-passwords.md), stage 4 (the second factor) [ADR 0069](../decisions/0069-second-factor-and-lockout.md). The decisions in *What was decided*
were taken with the owner before this was written; the rest is the reasoning, the diagrams
and the staging. The 2026-10-31 amendment makes a guest a principal of its own, with an ID
and a portable key (§3a), before stage 2 begins.

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

None of the three owns a seat. The seat token is checked only by the notify endpoints, and the
browser secret only by direct messages and who-is-here.

Also true today, and relevant:

- A player is a `player_name` the API is given; `players` has `UNIQUE(game_id, name COLLATE
  NOCASE)`, so names are unique **per game** and owned by nobody. **A move is authorised by
  the name alone** (`POST /api/games/{code}/moves` with `player_name`): whoever knows the room
  code and a seated player's name can move for them, and resuming a seat means typing the name
  again (ADR 0006).
- "My games" is `bingle.games` in the browser's storage, for everybody: a game does not follow
  its player to another browser, and is lost with the browser's storage.
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
| Guest identity *(2026-10-31)* | **A guest is a principal with a server-issued ID**, like an account, and proves it with a **guest key**: a random secret, hashed at rest, disposable (rotate it, or start over as a new guest) and portable (the same key on another browser is the same guest, in every game). Seats, chat lines and games reference the principal, never a name. |
| Moving a guest *(2026-10-31)* | **The guest can copy the key**, as text or as a link, and use it on another device to resume every game there. |
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
- **Guest** — a guest key (§3a), sent as the `bingle_guest` cookie by a browser or as
  `Authorization: Bearer bgg_…` by a script. It replaces the `bingle.session` browser secret
  and the seat tokens. Unused in `closed`.
- **Token** — `Authorization: Bearer bgl_…`, hashed in `api_tokens`, scoped, revocable,
  created on the account page. Agents and scripts use these, never passwords. Admin actions
  are never available to a token.

Every principal, of either kind, is a row in `principals` with a stable opaque ID
(`p_` and 16 random bytes in base32). Everything that belongs to somebody refers to that ID:
`players.principal_id`, `games.owner_principal_id`, chat lines, notify contacts and
`auth_events`. Code below the middleware asks "which principal?", never "which name?" or
"guest or account?", except where a capability check needs the class.

**CSRF**: a state-changing request authenticated *by cookie* must carry `X-Bingle-Session`
(the page sends it; token clients are unaffected). WebSocket upgrades check `Origin`.
`/ws-dm` becomes session-authenticated and loses its hello-secret frame — a simplification
that falls out of direct messages being account-only.

### 3a. Guests: an ID and a disposable, portable key *(amended 2026-10-31)*

A guest has two things, and they must not be confused:

| | The guest **ID** | The guest **key** |
|---|---|---|
| Is | `principals.id`, e.g. `p_7kq2…` | `bgg_` and 32 random bytes, base64url |
| Secret? | No: the server's name for the guest. Never shown in the interface, and logged only as a hash (ADR 0063) | Yes: a bearer credential. Whoever holds it *is* the guest |
| Stored as | itself | SHA-256 in `guest_keys` (high-entropy, so no Argon2id needed) |
| Lifetime | forever, so history keeps its owner | until rotated, discarded or idle for `BINGLE_GUEST_IDLE` (90 days) |

**Issued lazily.** A guest principal and its key are made on the first request that needs a
principal — create, join, chat, voice — not on every page view, so crawlers and passers-by
leave no rows. The server sets the key in the `bingle_guest` cookie (`HttpOnly; Secure;
SameSite=Lax; Path=/`, with `Secure` dropped on loopback as for accounts) and never in a URL.
A REST or MCP client gets the key in the response body of `POST /api/guest` and sends it as a
bearer token, so an anonymous agent becomes a real guest too, rather than a name.

**Disposable.** The profile window offers, for a guest:

- **Copy my guest key** — see *Moving to another device* below.
- **New key, same me** — rotate: the old key stops working everywhere (a key copied somewhere
  it should not be), the ID and every seat stay.
- **Start over as a new guest** — the key is revoked and the next action makes a new principal.
  The old guest's seats stay held under the old ID, so the confirmation says what is lost.

A key idle for `BINGLE_GUEST_IDLE` stops working; the principal stays so history still
displays.

**Moving to another device.** "Copy my guest key" fetches the key from `GET /api/guest/key`
(cookie and CSRF header required; the page never keeps the key in its own storage) and offers
it as text and as a link, `https://<host>/#guest=bgg_…`. The key travels in the fragment, so it
never reaches a server log, a proxy or a `Referer`. On the phone the link, or the key pasted into
"Use a guest key from another device", is posted to `POST /api/guest/adopt`, which sets the
phone's cookie. From then the phone is the same guest: the same seats, and the same server-side
"My games" (§6), so every game the guest is in resumes there.

- **What the phone had before** is merged into the adopted guest: its seats and games move
  over and its own principal is marked `merged_into`. The phone loses nothing by adopting.
- **Adoption is rate-limited** per address and answers uniformly, so keys cannot be guessed by
  trying them.
- **The key is a password with no username.** The copy dialog says so ("anyone with this key
  can play as you"), and "New key, same me" is one click away from it.
- **Considered and left for later:** a short one-time transfer code (eight characters, ten
  minutes) that is easier to type on a phone than a 47-character key and harmless once used.
  It can be added beside the key without changing anything below it.

**Becoming an account.** Signing up, or signing in, from a guest browser **upgrades the
principal in place**: the row gains an `account_id`, its keys are revoked, and the session cookie
takes over. Every seat, game and chat line keeps its owner without being rewritten. This is the
reason seats point at a principal, not at an account or a guest.

**Seats from before this plan** have no principal. In stage 2 the first principal to act on such
a seat by its name binds it, and from then the seat is that principal's. That is ADR 0006's
"re-enter your name" for one last time, per seat. After that a name proves nothing.

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
  "log out everywhere", and API tokens. For a guest it holds the guest-key actions of §3a
  (copy, use a key from another device, new key, start over) and *Sign up to keep your games*.
- **"My games" moves server-side for every principal**, listed from `players.principal_id`.
  A guest's games follow their key to the phone; an account's follow the account to any
  device without a key to carry, which is still a reason to sign up. `bingle.games` survives
  only as the offline cache of the list.
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
  │              bingle_guest cookie or Bearer bgg_… ─► guest   (hybrid only)
  │              Bearer bgl_…   ─► token
  │              none, and the route needs one ─► a new guest   (hybrid only)
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
 join     POST /api/games/{code}/join          join       the principal owns the seat
 move     POST /api/games/{code}/moves         play       only the seat's principal; the name
                                                          in the body is no longer enough
 chat     POST /api/games/{code}/chat          chat_game
 live     GET  /ws/{code}                      Origin checked; session or guest cookie
 list     GET  /api/games                      join       (every unfinished game, ADR 0058)
 mine     GET  /api/me/games                   —          the principal's own games
```

### Moving a guest to another device

```
 desktop                                server                              phone
   │ GET /api/guest/key  (cookie + CSRF) ─►│
   │◄── bgg_…  shown as text and as  https://host/#guest=bgg_…
   │ ··· copied, AirDropped, messaged to oneself ··· ························►│
   │                                       │◄── POST /api/guest/adopt {key} ──│  fragment read by
   │                                       │    rate-limited, uniform answer  │  the page, never sent
   │                                       │    the phone's old guest merged  │  in a URL
   │                                       │──► Set-Cookie: bingle_guest ────►│
   │                                       │◄── GET /api/me/games ────────────│  every game resumes
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
     hybrid:  a token is optional — an anonymous agent is a guest, with a key from POST /api/guest
     closed:  a token is required
     scopes:  play · create_game · read_metrics        (never admin)
     BINGLE_DEV=1 seeds a deterministic token so CI can exercise closed mode
```

## Schema sketch

```sql
principals(id, kind, account_id, created_at, last_seen_at, merged_into)  -- kind: guest|account
guest_keys(id, principal_id, key_hash UNIQUE, created_at, last_used_at, revoked_at)
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

Everything is additive: new tables plus `players.principal_id` and
`games.owner_principal_id`, both nullable, so existing rows keep meaning what they mean (a
`NULL` principal is a seat from before stage 2, bound by name once, §3a). They point at a
principal rather than an account so a guest who signs up keeps everything without a rewrite.
`seat_tokens` is retired once the notify endpoints check the principal instead.

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
| `BINGLE_GUEST_IDLE` | `90d` | a guest key unused this long stops working (the guest's history stays) |
| `BINGLE_RTC_RELAY` | `all` | `all`, `accounts` or `none`; plus force-relay-for-guests |

## Staged plan

Each stage is one pull request that leaves a working server.

**1. Secure transport.** *Done.* The start-up rule, self-signed generation and caching,
`BINGLE_TRUST_PROXY` with forwarded-header handling, the `BINGLE_DEV` gate, HSTS review.
Tests: a non-loopback bind with no TLS refuses to start; a trusted proxy's `http` request is
refused while a forwarded `https` one is served; a generated certificate is reused across
restarts; the in-process suites still run on loopback HTTP.

Notes. **The default bind had to change from `:8080` to `127.0.0.1:8080`**: the rule exempts a
loopback bind, so the one bind nobody chose has to be the exempt one or `./bin/bingle` would
refuse to start out of the box. That is the most disruptive thing in this plan and it arrived in
its first stage — a server someone reaches from their LAN today stops answering until it is told
how. `internal/transport` is new and owns the three questions (scheme, peer, locality), with the
"a trusted proxy makes nothing local" rule enforced in one place. `clientIP` moved onto it, which
the plan did not call for but had to happen in this stage: without it `BINGLE_TRUST_PROXY` would
have made every request in the world share one address and the rate limiters would have throttled
everybody at once. HSTS now applies behind a trusted proxy and is deliberately **not** sent with a
generated certificate.

**2. Principals and guest keys.** The `principals` and `guest_keys` tables, the lazily issued
`bingle_guest` cookie and `POST /api/guest` for scripts, one principal per request carried in
the context, and every endpoint moved onto it: seats, games, chat lines and notify contacts
record `principal_id`, a move or a seat change is refused unless it comes from the seat's
principal, and a seat from before this stage is bound by name on first use. The notify
endpoints check the principal and `seat_tokens` goes. The web client stops sending names as
proof and drops `bingle.tokens`; `bingle.session` stays only for direct messages until stage 8.
The interface looks the same, except that typing a seated player's name no longer takes
their seat. Tests: every route resolves a principal; a move with the right name from the
wrong principal is refused; a pre-stage-2 seat binds once, then refuses a second principal;
a guest key never appears in a log line or a URL; a bearer guest key and a cookie guest key
are the same guest; a contract test fails when a new endpoint reads `player_name` without a
principal.

Notes. *Done* (ADR 0067). One departure, forced by the plan's own wording: an anonymous request
is not made a guest, because every client without a cookie jar would then be a new guest per
request and lose its seat on the second. It stays anonymous and keeps unowned seats only, acted
for by name; whatever a browser or a keyed script takes is out of its reach. Private views show a
non-owner a spectator's view instead of refusing, and the page's first state-changing request is
sent alone so that two concurrent ones cannot make two guests.


**3. Accounts, sessions and password login.** The schema, Argon2id, server-side sessions, the
cookie and CSRF rules, the seeded admin with its forced password change, the login and signup
pages, email confirmation through the existing mailer, and the `bingle account` commands.
Signing up or in from a guest browser upgrades that principal in place (§3a).
Tests: Argon2id parameters survive a round trip; a session is rotated on privilege change;
a guest who signs up keeps every seat and game, and their guest key stops working;
`Secure` is set off-loopback and omitted on it; signup is refused with no SMTP; enumeration
responses are uniform; a CSRF-less cookie POST is refused.

Notes. *Done* (ADR 0068). A guest who signs up gives the account its principal, and signing in
from another guest browser merges that guest in, so "in place" holds either way. The password
rules are length only (10–256). Invitations wait for the admin pages of stage 6, so
`BINGLE_SIGNUP` is `open` or `closed` for now; handles are validated and reserved, and keeping
guests off a registered handle is stage 7's.

**4. TOTP, recovery codes and lockout.** Enrolment, verification, the ten codes, the admin
group requirement with its loopback exemption, and `bingle account reset`. Tests: RFC 6238
vectors; a used recovery code cannot be reused; the loopback exemption does not apply behind
a trusted proxy; brute-force backoff.

Notes. *Done* (ADR 0069). The QR code stays out: the key and the `otpauth://` link are shown as
text, which every app accepts. Resetting a forgotten password by mail, which §9 assumes and no
stage named, landed here, with a second factor asked for too. Backoff and waiting sign-ins are in
memory, so a restart clears them.

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
sessions, tokens), server-side "my games" for every principal (`GET /api/me/games`), the
guest-key actions in the profile window — copy as text or link, use a key from another device,
new key, start over — with `GET /api/guest/key` and `POST /api/guest/adopt`, and the
audit-log view. Moving a guest needs nothing from stages 3–6, so it can land right after
stage 2 if it is wanted sooner.
Tests: a guest cannot take a registered handle; a renamed handle is held; "my games" follows
a second session; the account page never shows another account's address; a guest key copied
from one browser and adopted in another resumes every game there, and the second browser's own
games are merged in; a rotated key stops working everywhere while the seats stay; "start over"
leaves the old seats with the old ID; adoption is rate-limited and answers a wrong key exactly as
an expired one; the `#guest=` fragment is removed from the address bar once read.

**8. Migrating the account-only features.** Direct messages and who-is-here onto sessions
(guests dropped from the list, and `bingle.session` retired with them), `chat_global` and
`notify` gated, confirmed contacts grandfathered and offered the upgrade path, and the documentation sweep that retires ADR 0006.
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
  accounts can be held to anything. The documentation should say so where it matters. A guest
  key identifies a browser that kept it, not a person, and "start over" makes a new guest at
  will.
- **A guest key is a bearer credential with no second factor.** Whoever holds it plays as that
  guest until it is rotated. It lives in an `HttpOnly` cookie, is shown only on request, travels
  in a fragment rather than a URL path, and is never logged; the copy dialog says what it is.
- **Losing the key loses the seats.** Once a name no longer reclaims a seat, a guest whose
  browser storage is cleared, and who never copied the key, cannot resume. That is the price of
  nobody else being able to take the seat by typing the name. Handing an abandoned seat to
  someone else (for example by the game's creator) is a separate, later decision.
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
- 2026-10-31: guests amended before stage 2, at the owner's request. A guest becomes a principal
  with a stable server-side ID and a disposable, portable guest key (§3a), so guest sessions can
  be tracked across browsers and games; seats, games and chat refer to principals, not names or
  accounts, and a guest who signs up keeps everything in place. The guest can copy the key, as text
  or as a `#guest=` link, to resume their games on another device (stage 7, or straight after
  stage 2). Found while reviewing: a move is authorised today by the player's name alone, which
  stage 2 now closes. Nothing implemented.
- 2026-10-31: stage 2 done (ADR 0067): principals and guest keys, seats owned by their principal,
  seat tokens retired; anonymous requests keep unowned seats only.
- 2026-10-31: stage 3 done (ADR 0068): accounts, server-side sessions, Argon2id, signing up with a
  confirmed address, the seeded administrator and `bingle account`.
- 2026-10-31: stage 4 done (ADR 0069): TOTP with replay protection, recovery codes, the admin
  group's second factor, backoff, and password reset by mail.
