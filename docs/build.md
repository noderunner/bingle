# Building bingle

## Prerequisites

- Go (matching the version in `go.mod`, currently 1.27.1; [go.dev/dl](https://go.dev/dl/))
- To build the container image: `docker` or `podman`

No other tools are required — both dependencies (`modernc.org/sqlite`, `github.com/coder/websocket`, etc.) are pure Go, so there's no CGO toolchain or system library to install.

## Building the binary

```sh
make build
```

This runs `go build -o bin/bingle .` and produces a single static binary at `bin/bingle`.

To build and run it in one step:

```sh
make run
```

## Running the binary locally

```sh
./bin/bingle
```

By default this listens on `:8080` (plain HTTP) and stores its SQLite database at `./bingle.db` in the current directory. Open <http://localhost:8080> in a browser to use the web UI.

### The connection must be secure

Bingle **refuses to start** on an address a network can reach unless the connection to it is
secure, because it is about to start carrying passwords and session cookies
([ADR 0065](decisions/0065-the-connection-must-be-secure.md)). One of these four:

```sh
BINGLE_TLS_DOMAIN=games.example.com   # automatic Let's Encrypt certificates (docs/tls.md)
BINGLE_TLS_CERT=… BINGLE_TLS_KEY=…    # your own certificate
BINGLE_TLS_SELFSIGNED=1               # one bingle generates and caches for itself
BINGLE_TRUST_PROXY=1                  # a proxy in front of bingle terminates TLS
```

**A loopback bind needs none of them**, which is why the default is `127.0.0.1:8080`: run
`./bin/bingle` and it serves the machine it is on, with nothing to configure. Making it reachable
from elsewhere is a deliberate act, and that act comes with a certificate.

The startup error names all four, so there is nothing to look up:

```
$ BINGLE_ADDR=:8080 ./bin/bingle
ERROR the configuration was refused err="config: bingle is listening on \":8080\", which a
network can reach, with no secure connection. Set BINGLE_TLS_DOMAIN for automatic certificates,
… A loopback address such as 127.0.0.1:8080 needs none of these"
```

### Configuration

All configuration is via environment variables — there's no config file.

| Variable | Default | Purpose |
|---|---|---|
| `BINGLE_ADDR` | `127.0.0.1:8080` | Address/port to listen on. **The default is loopback**: a bind a network can reach needs a secure connection as well (see below) |
| `BINGLE_DB_PATH` | `./bingle.db` | Path to the SQLite database file |
| `BINGLE_RETENTION_DAYS` | `0` (keep everything) | Delete games nobody has touched for this many days (checked hourly) |
| `BINGLE_LOG_LEVEL` | `info` | How much the server says: `debug`, `info`, `warn` or `error`. `debug` is the verbose mode and adds a line per request. See [observability.md](observability.md) |
| `BINGLE_LOG_FORMAT` | `text` | `text` is `key=value` lines for a person, `json` one object per line for a log collector |
| `BINGLE_METRICS_ADDR` | `127.0.0.1:9095` | The private listener serving Prometheus metrics at `/metrics`. Empty turns the metrics off. It is deliberately not the public listener, which has no authentication. See [observability.md](observability.md) |
| `BINGLE_METRICS_TOKEN` | *(unset)* | Bearer token for that listener; **required** (and at least 16 characters) when `BINGLE_METRICS_ADDR` is not on the loopback interface |
| `BINGLE_METRICS_DB` | `bingle-metrics.db` beside the database | The slow-request journal's database, kept separate from the game database. Empty stores nothing |
| `BINGLE_METRICS_SAMPLE` | `5m` | How often the metric history is written; `0` keeps no history. Reachable at `/metrics/health` on the private listener |
| `BINGLE_METRICS_RETAIN_DAYS` | `14` | Days of full-resolution history before it is compacted to hourly, then daily after a year; `0` keeps everything |
| `BINGLE_SLOW_REQUEST` | `250ms` | How slow a request must be before the journal writes it down; `0` turns the journal off. A failed request is written down however fast |
| `BINGLE_CLIENT_ERRORS` | `on` | `POST /api/client-error`, where a browser reports what went wrong in it. The only observability route on the public listener: write-only, validated against closed sets and rate limited. `off` removes it |
| `BINGLE_DEBUG_GAMES` | *(every game)* | Comma-separated game families to log moves for, so `debug` is readable |
| `BINGLE_DEBUG_ROOM` | *(unset)* | One room code logged at `info` whatever the level |
| `BINGLE_DEBUG_SECRETS` | `off` | Full positions in the debug lines instead of the public view. **Development only** |
| `BINGLE_DEBUG_DIR` | *(unset)* | Where repro bundles are written when something impossible happens; `scripts/replay` reproduces one. Unset writes none. A bundle carries a whole position, secrets and all — see [observability.md](observability.md) |
| `BINGLE_PARANOID` | `off` | Check the engine against itself after every move. **Development and CI only**; it runs `Legal` twice per move |
| `BINGLE_TLS_SELFSIGNED` | `off` | Generate and cache a certificate at startup, so a server reachable from a network can be run without obtaining one. **No browser will trust it**; the warning is clicked through once per browser |
| `BINGLE_TRUST_PROXY` | `off` | Serve plain HTTP behind a proxy that terminates TLS. The proxy **must** set `X-Forwarded-Proto: https`; anything arriving as `http` is refused. Turns every loopback exemption off |
| `BINGLE_DEV` | `off` | Deterministic development credentials (used from stage 3 of the accounts plan). Loopback binds only, and never with `BINGLE_TRUST_PROXY` |
| `BINGLE_GUEST_IDLE` | `90d` | How long a guest key may go unused before it stops working (`90d`, `720h`; an hour at least). The guest's history stays ([ADR 0067](decisions/0067-principals-and-guest-keys.md)) |
| `BINGLE_SIGNUP` | `open` | `open` or `closed`. Signing up also needs mail (`BINGLE_SMTP_*`): without it nobody can, and accounts are made with `bingle account create` |
| `BINGLE_SESSION_IDLE`, `BINGLE_SESSION_MAX` | `14d`, `90d` | A signed-in browser is signed out after this long unused, and after this long at all |
| `BINGLE_ADMIN_HANDLE` | `admin` | The first administrator, made when there are no accounts |
| `BINGLE_ADMIN_EMAIL` | the handle at the public URL's host | Its address |
| `BINGLE_ADMIN_PASSWORD` | — | Its first password; otherwise `bingle123` on loopback or under `BINGLE_DEV` (never behind `BINGLE_TRUST_PROXY`), and a random one printed once. It must be changed at the first sign-in |
| `BINGLE_OIDC_PROVIDERS` | — | Sign-in providers, by name: `google,corp` ([ADR 0070](decisions/0070-oidc.md)). Each needs `BINGLE_PUBLIC_URL` and a block of its own: |
| `BINGLE_OIDC_<ID>_ISSUER` | — | The issuer URL (`https://accounts.google.com`); its discovery document says the rest |
| `BINGLE_OIDC_<ID>_CLIENT_ID`, `_CLIENT_SECRET` or `_CLIENT_SECRET_FILE` | — | The client registered at the provider, whose redirect URI is `<public URL>/auth/oidc/<id>/callback` |
| `BINGLE_OIDC_<ID>_SCOPES`, `_LABEL` | `openid email profile`, the name | What is asked for, and the button's "Sign in with …" |
| `BINGLE_TLS_CERT` | *(unset)* | Path to a TLS certificate file |
| `BINGLE_TLS_KEY` | *(unset)* | Path to a TLS private key file |
| `BINGLE_PUBLIC_URL` | *(unset)* | Base URL players on other networks reach the server by, such as `https://games.example.com` (an absolute `http`/`https` URL, no query). Invite links in the lobby and `invite_url` are built from it; unset, the address the request came to is used |
| `BINGLE_TLS_DOMAIN` | *(unset)* | Comma-separated host names to get and renew **Let's Encrypt** certificates for. Turns on HTTPS with automatic certificates; the listen address then defaults to `:443`. Cannot be combined with `BINGLE_TLS_CERT`/`KEY` |
| `BINGLE_TLS_EMAIL` | *(unset)* | Contact address for the certificate authority (expiry warnings); optional. It is also the **sender address of email notifications** (see [email.md](email.md)) |
| `BINGLE_TLS_CACHE` | `certs` next to the database | Where certificates and the account key are kept; must survive restarts |
| `BINGLE_HTTP_ADDR` | `:80` | With `BINGLE_TLS_DOMAIN`: the plain-HTTP listener that answers certificate challenges and redirects to HTTPS |
| `BINGLE_TLS_ACME_URL` | Let's Encrypt production | Another ACME directory, e.g. the Let's Encrypt **staging** one while trying things out |
| `BINGLE_TLS_HSTS_SECONDS` | `0` (off) | Adds `Strict-Transport-Security: max-age=N` to HTTPS responses; raise it only once HTTPS works reliably |
| `BINGLE_RTC_STUN` | `stun:stun.l.google.com:19302` | Voice chat: comma-separated STUN URLs (`none` for no STUN) |
| `BINGLE_RTC_TURN_URLS` | *(unset)* | Voice chat: URLs of a TURN relay (coturn), for players behind strict NATs |
| `BINGLE_RTC_TURN_SECRET` | *(unset)* | coturn's `static-auth-secret`; bingle mints expiring credentials from it. See [voice.md](voice.md) |
| `BINGLE_SMTP_HOST` ... | *(unset = off)* | Email notifications: the SMTP server and login (`BINGLE_SMTP_PORT`, `_TLS`, `_USER`, `_PASSWORD_FILE`, `_CA_FILE`), mail limits (`BINGLE_MAIL_HOURLY_LIMIT`, `_DAILY_LIMIT`). The sender is `BINGLE_TLS_EMAIL`. See [email.md](email.md) |

TLS is **off by default** and optional: with none of the TLS variables set the server runs plain HTTP, which is what you want for local development. HTTPS turns on either with `BINGLE_TLS_DOMAIN` (automatic certificates) or when **both** `BINGLE_TLS_CERT` and `BINGLE_TLS_KEY` are set to valid, readable files. Setting up a domain, the router and a first test is in [docs/tls.md](tls.md).

### Accounts from the command line

An empty database gets its first administrator when the server starts: `@admin`, with the password
`bingle123` on a loopback bind or under `BINGLE_DEV`, and a random one printed once on the terminal
otherwise. Its first sign-in (at `/login`) asks for a new password.

```sh
./bin/bingle account list                              # handles, status, groups
./bin/bingle account create ana ana@example.com        # a confirmed account; prints its password
./bin/bingle account create ops ops@example.com --admin
./bin/bingle account reset ana                         # a new password (printed), no second factor, every session signed out
```

A password the command line prints must be changed at the next sign-in. The commands read the same
configuration as the server (`BINGLE_DB_PATH`), so run them where the server runs.

Example with a custom port and database location:

```sh
BINGLE_ADDR=127.0.0.1:9000 BINGLE_DB_PATH=/tmp/bingle-dev.db ./bin/bingle
```

## Building the container image

The `Containerfile` is a multi-stage build: a Go builder stage compiles a static binary (`CGO_ENABLED=0`), and the runtime stage is minimal Alpine with just that binary and `curl` (used by the container's own `HEALTHCHECK`).

```sh
docker build -f Containerfile -t bingle .
# or, with podman:
podman build -f Containerfile -t bingle .
```

## Running the container locally

The image defaults to listening on `:8080` inside the container and storing its database under `/data` (declared as a volume). To run it and reach it from your host browser:

```sh
docker run --rm -p 8080:8080 bingle
```

Then open **<http://localhost:8080>** in a browser — that's the create/join page. Create a game there, or open the same URL from a second tab/window to see live updates propagate between them once a game is underway.

To persist the database across container restarts, mount a host directory at `/data`:

```sh
docker run --rm -p 8080:8080 -v "$(pwd)/data:/data" bingle
```

To check the container is healthy without a browser:

```sh
curl -fsS http://localhost:8080/healthz
```

(swap `docker` for `podman` in any of the commands above if that's what you have installed — the image and flags are identical either way.)
