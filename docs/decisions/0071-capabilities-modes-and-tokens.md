# 0071. Capabilities, modes, API tokens and invitations

Status: accepted (2026-10-31)

## Context

Stage 6 of [the accounts plan](../history/auth-and-accounts-plan.md) (§1, §3, §5): who may do what,
seeded from a YAML file and then owned by the database and an admin page; `hybrid` and `closed`;
API tokens for REST and MCP; per-class rate limits. Invitations, which stage 3 left for the admin
pages, land with them.

## Decision

- **The matrix** is `capabilities(class, capability, allowed)` for `guest` and `user` (the admin
  group holds everything and has no rows), and `cap_limits(class, kind, per_minute)`. A fresh
  database is seeded from `BINGLE_CAPS_FILE` (`bingle-capabilities.yaml`, or the built-in copy when
  there is no such file) and never reads it again; the admin page changes it, shows a badge when it
  has drifted from the file, and `bingle caps reset` applies the file deliberately. The file is read
  strictly: an unknown class, capability, limit or key is an error, not a silent no.
- **One more direct dependency:** `go.yaml.in/yaml/v3`, because the plan chose YAML and a
  hand-written parser for it would be the wrong saving. Eleven direct dependencies.
- **The mode and signing up are environment, not file:** `BINGLE_AUTH_MODE` (`hybrid`, `closed`) and
  `BINGLE_SIGNUP` (`open`, `invite`, `closed`). The plan's example file had both; two places to set
  one thing is one too many, so the file refuses them and says where they go.
- **Enforcement** is in the resolver's middleware, by route: making a game (`create_game`), joining
  (`join`, the game WebSocket too), a game's chat (`chat_game`), its voice (`voice_game`), batches
  (`run_batch`: starting, cancelling, deleting), and playing (`play`: every other action on a game,
  the training environments and MCP). A request without the capability is `403`, past its class's
  limit (chat lines, games made, direct messages, sign-ins, per minute; per principal, or per
  address when anonymous) `429`. Anonymous requests are guests.
- **Not yet enforced:** `dm`, `voice_direct`, `chat_global` and `notify` are in the matrix and seeded
  account-only, but the routes do not check them until stage 8 moves those features onto accounts and
  keeps the confirmed contacts working — checking them now would take direct messages and turn mail
  away from guests before their replacement exists. `view_metrics`, `moderate`, `install_plugins` and
  `admin` wait for the features that use them; the admin page itself is the admin group's.
- **Closed** serves members only: an API request from anybody else outside `/auth` is `401`, a page is
  a redirect to `/login?return=…` (the sign-in, sign-up, reset and confirmation pages, the static
  files and `/healthz` stay open), and a page's first request makes no guest.
- **API tokens** (`bgl_` and 32 bytes, SHA-256 at rest) are made, listed and revoked on the account
  page, and sent as `Authorization: Bearer`. Scopes: `play` (join, play, a game's chat and voice),
  `create_game`, `read_metrics`. A token acts within its account's class *and* its scopes, never with
  the admin group's powers, cannot make tokens, and skips the interactive gates (a password to change,
  an administrator's second factor) it has no use for. `BINGLE_DEV` (loopback only) seeds the first
  administrator a well-known token, `auth.DevAPIToken`, so CI can drive a closed server.
- **Invitations** (`BINGLE_SIGNUP=invite`): the admin page mints a link, `/signup?invite=…`, good for
  a week and once; a password sign-up and a provider's newcomer both need one.
- **The admin page** (`/admin`) shows the matrix with a box per class and capability, the limits, the
  drift badge and the invitations. It is for the admin group only, never through a token, and not for
  an administrator who still owes a second factor.

## Consequences

Tests run with `tests/app/testdata/capabilities.yaml`, the defaults with limits no test reaches,
because every test client shares 127.0.0.1; the defaults' own limits have a test of their own. The
fresh-server series ceiling is 350 (seeding the matrix is three more histograms). Agents that never
had a key keep working as guests within the guests' limits: three games a minute by default.
