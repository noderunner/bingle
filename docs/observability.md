# Watching bingle run

Bingle is one binary on one machine, so what it says about itself has to be useful with nothing
else installed. [docs/history/observability-plan.md](history/observability-plan.md) stages the
whole of that work; **this page covers what is built: the log and the metrics.** The slow-request
journal, the metrics database and the Health page are later stages and are not here yet.

## The log

Everything the server says goes through one logger, built in `main` and handed to the
application, the session manager and the listener. The standard library's `log` package is
pointed at the same handler, so a line from chi, the WebSocket library or the SQLite driver
appears in the same stream at the same level instead of bypassing both.

| Variable | Default | What |
|---|---|---|
| `BINGLE_LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error`. `debug` is the verbose mode |
| `BINGLE_LOG_FORMAT` | `text` | `text` is `key=value` lines for a person; `json` is one object per line for a log collector |

At `info` — the default — a working server is quiet: it says what it is listening on, and then
nothing until something goes wrong. A request is logged at `debug`, and at `info` only when the
server itself failed it (a 5xx). So:

```
BINGLE_LOG_LEVEL=debug ./bin/bingle
```

is how you watch traffic, and

```
time=2026-10-28T19:02:11.004Z level=DEBUG msg=request method=POST route=/api/games/{code}/chat \
  status=200 bytes=113 dur=856.792µs id=367aaf254039bf3a
```

is what one request looks like.

### Request ids

Every request gets an id — eight random bytes in hex — which every line about it carries as `id`
and which the response echoes in `X-Request-Id`. An inbound `X-Request-Id` is kept when it is
short and printable, so a proxy in front of bingle keeps its own id across the hop, and replaced
when it is neither.

### A panic is now recorded

A panic in a handler used to vanish: `net/http` dropped the connection per request and nothing
was written down. It is now one `ERROR` line with the stack, the route and the request id, and
the client gets a 500.

The recover middleware sits **inside** the access log rather than outside it. A panic unwinds past
every middleware outside the one that recovers, so with the recover outermost a panicking request
would be the one request never logged as a request — and, from Stage 2 on, never counted either.

### WebSockets

A room socket lives as long as the player is on the page, so it is not logged as a request that
finished: an accepted upgrade is logged when it is accepted (`websocket open`) and again when it
closes (`websocket closed`, with how long it lasted). A rejected upgrade never became a
connection and is logged as the ordinary request it was.

The closing line comes when the handler notices, which for a room socket is the next time the
room has anything to say — the handler is parked on the room's event channel, not on the
connection.

## What a log line may never carry

The redaction policy is [ADR 0063](decisions/0063-what-a-log-line-may-say.md), and it is tested
rather than trusted: `tests/app/logging_test.go` plays a game, talks in it, writes a direct
message, opens a socket and offers an address with the level turned up to `debug`, then fails if
the log holds any of it.

**Never logged, at any level:** seat tokens, browsers' secrets, chat and direct-message bodies,
email addresses, WebRTC payloads, and the paths requests came to (a path carries the room code).

**Logged only as a reference:**

| Thing | How it appears | Helper |
|---|---|---|
| a room code | the first two characters and twelve hex characters of its hash, `AB-3f9a1c22d4e5` | `logging.Room` |
| an email address | twelve hex characters of the hash of the lower-cased address | `logging.Addr` |
| a player's name | the same, and omitted altogether unless the line would say nothing without it | `logging.Name` |
| the route | chi's pattern, `/api/games/{code}/chat`, never the path | `logging.Route` |

A credential held in configuration is a `config.Secret`, which prints as `[redacted]` through
every formatter including `slog`, so it cannot reach a line by accident.

New code that wants to log one of these things uses the helper. New code that wants to log
something not in the table above, and not obviously harmless, belongs in the ADR first.

## The metrics

Bingle serves Prometheus' text format at `/metrics` on a **second listener**, separate from the
public site. `curl` it with nothing else installed, or point Prometheus, VictoriaMetrics, Netdata,
telegraf or an OTEL collector at it.

| Variable | Default | What |
|---|---|---|
| `BINGLE_METRICS_ADDR` | `127.0.0.1:9095` | The private listener. Empty turns the metrics off entirely — nothing is registered and nothing is recorded |
| `BINGLE_METRICS_TOKEN` | *(unset)* | A bearer token the listener requires. **Required** when `BINGLE_METRICS_ADDR` is not on the loopback interface, and at least 16 characters |

```sh
curl -s localhost:9095/metrics | grep '^bingle_'
```

### Why it is not on the public site

The public site has no authentication of any kind, so `/metrics` is **never** mounted on it — it
is a 404 there, and a test keeps it that way. What that listener serves is a running account of
which games are being played and how busy the server is.

The default is `127.0.0.1`, which nothing off this machine can reach. Put it anywhere a network
can reach it and bingle **refuses to start** without `BINGLE_METRICS_TOKEN`:

```sh
BINGLE_METRICS_ADDR=:9095 ./bin/bingle
# config: BINGLE_METRICS_ADDR is ":9095", which is not on this machine's loopback
# interface, so BINGLE_METRICS_TOKEN must be set: ...
```

Scrape it with `Authorization: Bearer <token>`. For a scraper on another machine, an SSH tunnel to
the loopback listener is the simpler answer and needs no token at all.

### In a container

`127.0.0.1` inside a container is the container, so the default listener is reachable from a shell
in it (`podman exec … curl -s localhost:9095/metrics`) and from nothing else. To scrape it from
outside, give it an address the container's network reaches **and** a token, and publish the port:

```sh
podman run -p 8080:8080 -p 9095:9095 \
  -e BINGLE_METRICS_ADDR=0.0.0.0:9095 \
  -e BINGLE_METRICS_TOKEN="$(cat ~/.config/bingle/metrics_token)" \
  -v bingle-data:/data bingle:latest
```

The container image does not publish 9095 by default, and the health check uses the public
`/healthz`, both unchanged.

### What is measured

| Series | What |
|---|---|
| `bingle_http_requests_total{route,method,status}` | Requests answered |
| `bingle_http_request_duration_seconds{route,method}` | How long they took (a histogram) |
| `bingle_http_requests_in_flight` | Requests being answered right now |
| `bingle_http_response_bytes_total{route}` | Response bodies written |
| `bingle_http_errors_total{route,kind}` | Refusals, by family: `bad-request`, `forbidden`, `not-found`, `conflict`, `rate-limited`, `client`, `server` |
| `bingle_ws_connections{kind}` | Sockets open now, by `page`, `voice`, `chat`, `dm` |
| `bingle_ws_connections_total{kind}` | Sockets accepted |
| `bingle_build_info{version}` | Always 1; the label is the engine's version |
| `go_*`, `process_*` | The Go runtime and process collectors, registered explicitly |

`route` is **chi's route pattern** — `/api/games/{code}/chat` — and never the path, for the same
reason the log does not carry one. A label is published and durable, so the whitelist in
[ADR 0064](decisions/0064-metrics-on-a-second-listener.md) is stricter than the log's: no room
code, no name, no address, no position, no move notation.

A socket is recorded as a connection starting, never as a request that finished — a room socket
lives as long as the player is on the page. The gauge of open sockets falls a little late, because
a room socket's handler notices a player has gone only when the room next has something to say.

### Two things not to change casually

**The histogram buckets.** They are chosen once, in ADR 0064. Once Stage 5 keeps history in them,
changing them makes earlier quantiles incomparable with later ones.

**The labels.** Add one through a method on `*metrics.Metrics`, never by passing a string from a
call site, and check the series-count ceiling test still passes — it is there to catch exactly
that.

### What it costs

Measured through the real REST API against SQLite: a move is about 0.9 ms, and the difference in
time with recording on is below the noise of the measurement. Allocations are the stable signal:
**992 to 1001 per move, +0.9%**, inside the plan's 1% budget. With `BINGLE_METRICS_ADDR` empty the
middleware returns the handler untouched and the cost is zero.

The dependency is not free: `prometheus/client_golang` is one new direct dependency, seven
indirect, and 3.6 MB on a 33.9 MB binary.
