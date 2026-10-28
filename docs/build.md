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

### Configuration

All configuration is via environment variables — there's no config file.

| Variable | Default | Purpose |
|---|---|---|
| `BINGLE_ADDR` | `:8080` | Address/port to listen on |
| `BINGLE_DB_PATH` | `./bingle.db` | Path to the SQLite database file |
| `BINGLE_RETENTION_DAYS` | `0` (keep everything) | Delete games nobody has touched for this many days (checked hourly) |
| `BINGLE_LOG_LEVEL` | `info` | How much the server says: `debug`, `info`, `warn` or `error`. `debug` is the verbose mode and adds a line per request. See [observability.md](observability.md) |
| `BINGLE_LOG_FORMAT` | `text` | `text` is `key=value` lines for a person, `json` one object per line for a log collector |
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

Example with a custom port and database location:

```sh
BINGLE_ADDR=:9000 BINGLE_DB_PATH=/tmp/bingle-dev.db ./bin/bingle
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
