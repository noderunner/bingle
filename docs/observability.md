# Watching bingle run

Bingle is one binary on one machine, so what it says about itself has to be useful with nothing
else installed. [docs/history/observability-plan.md](history/observability-plan.md) stages the
whole of that work; **this page covers what is built: the log.** Metrics, the slow-request
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
