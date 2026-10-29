# Watching bingle run

Bingle is one binary on one machine, so what it says about itself has to be useful with nothing
else installed. [docs/history/observability-plan.md](history/observability-plan.md) stages the
whole of that work; **this page covers what is built: the log, the metrics, the database, the
slow-request journal, the games, the history behind the Health page, and what goes wrong in
browsers.** Repro bundles are Stage 7 and are not here yet.

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
| `bingle_db_queries_total{stmt,result}` | Statements run, by name and outcome |
| `bingle_db_query_duration_seconds{stmt}` | How long each took (a histogram) |
| `bingle_db_rows_total{stmt}` | Rows read or changed |
| `bingle_db_tx_duration_seconds` | How long transactions were open |
| `bingle_db_busy_total` | Statements that waited out the busy timeout and still lost |
| `bingle_db_file_bytes`, `bingle_db_wal_bytes` | The database and its write-ahead log, on disk |
| `bingle_db_page_count`, `bingle_db_freelist_pages`, `bingle_db_page_bytes` | What SQLite says about its pages |
| `bingle_db_table_rows{table}` | Rows in each table |
| `bingle_db_pool_*` | `database/sql`'s own account of the connection pool |
| `bingle_db_snapshot_ok` | 1 when the figures above could be read, 0 when they could not |
| `bingle_game_moves_total{game,result}` | Moves applied, refused by the rules, or failed |
| `bingle_game_illegal_moves_total{game}` | Moves a ruleset refused. A spike means a rules change or a stale client |
| `bingle_games_finished_total{game,end_reason}` | Games that ended, and why. A spike in `ply-limit` or `no-legal-moves` means a rule stopped working |
| `bingle_game_apply_duration_seconds{game}`, `bingle_game_legal_duration_seconds{game}` | How long the engine took |
| `bingle_game_legal_moves{game}` | How many moves a position offered |
| `bingle_bot_search_duration_seconds{bot,game}`, `bingle_bot_nodes_total{bot}` | How long and how hard a bot thought |
| `bingle_slot_wait_seconds{pool}`, `bingle_slot_in_use{pool}`, `bingle_slot_size{pool}` | The CPU pools: `hint`, `bot`, `match`. **Wait is what grows first under load** |
| `bingle_batch_workers` | Games a practice batch is playing at once |
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

## The database

Every statement in `internal/store` carries a **name**, and the name is what the metrics group by.
A statement that fails, finds nothing, or cannot get its lock is told apart from one that worked:

```
bingle_db_queries_total{result="ok",stmt="RecordMove.insertMove"} 412
bingle_db_queries_total{result="no-rows",stmt="GetGame"} 7
bingle_db_queries_total{result="busy",stmt="RecordMove.updateGame"} 0
bingle_db_queries_total{result="error",stmt="GetGame"} 0
```

`no-rows` is deliberately **not** an error: asking whether a row exists is what half of bingle's
statements are for. All four outcomes are published from the first time a statement runs, so a
`rate()` over errors works from the start rather than the series appearing only once something has
gone wrong.

`busy` is the one to watch. `store.Open` sets `busy_timeout(10000)`, so a statement counted here
waited ten whole seconds for the write lock and still did not get it — the first thing to look at
when writes start failing under load.

### The rule for new code

**Nothing in `internal/store` reaches for the database directly.** Use the wrapper:

```go
res, err := s.exec(ctx, "SetGameClock", `UPDATE games SET clock = ? WHERE id = ?`, clock, id)
row := s.queryRow(ctx, "GetGame", `SELECT ... WHERE room_code = ?`, code)
rows, err := s.query(ctx, "ListMoves", `SELECT ... WHERE game_id = ?`, id)
tx, err := s.begin(ctx, "RecordMove")   // then tx.exec / tx.queryRow
```

The name is the method the statement belongs to (`GetGame`), with a short suffix when a method
runs more than one (`RecordMove.insertMove`). **A name is never built from data** — it is a metric
label, and a label is a closed set.

This is not a convention to remember: `tests/store/measure_test.go`'s
`TestWrapperContract_NoStatementBypassesTheWrapper` reads the package's source and fails on a call
site that bypasses the wrapper, the way the DOM-contract tests pin the interface the browser tests
rely on. `internal/store/measure.go` is the one file allowed to call the database itself.

### The gauges about the file

These are read when the registry is gathered, not kept up to date as things happen, so what a
scrape shows is what was true when it was scraped. Counting the rows of `moves` is a table scan, so
a snapshot is **cached for ten seconds**: shorter than a normal scrape interval, so each scrape
gets its own figures, while a burst of scrapes costs one read. The snapshot's own statements are
deliberately not counted — they exist because something is watching.

If reading them fails (a closed database, a dropped table) `bingle_db_snapshot_ok` goes to 0 and
the figures are **left out** rather than published as zero, which would read as an empty database.
The pool numbers are still reported, because they are `database/sql`'s own counting and cannot
fail.

**On WAL mode:** `store.Open` sets `journal_mode(WAL)`, so a page that has been written lives in
the log until a checkpoint moves it into the file. `bingle_db_file_bytes` can therefore be far
smaller than `page_count × page_bytes` while `bingle_db_wal_bytes` holds the rest — watch the two
together. WAL growth is what the plan expects to bite first.

### What it costs

Measured on a read that answers from SQLite's cache, so the figure is the recording and almost
nothing else: **+0.46 µs per statement, and no extra allocations** (35.6 µs → 36.1 µs for a
`GetGame`). A move runs about ten statements, so that is about 5 µs on a move of roughly 900 µs:
**0.5%**, inside the plan's 1% budget.

Each statement name's metrics are resolved once and kept, because looking a label value up in a
Prometheus vector hashes it and takes a lock — doing that three times for each of ten statements
was, measured, 1.2% on a move all by itself.

## The slow-request journal

A request that was slow, or that the server failed, leaves one row saying where its time went. So
"the API is slow sometimes" becomes

```
WARN msg="a slow request" method=POST route=/api/games/{code}/moves status=200 dur=430ms
  id=b397ca0d phases="room.lock 410.2ms, store x9 12.1ms, session.ApplyMove 8.0ms"
```

and the row beside it holds the whole breakdown:

```
handler 0.83ms
  store GetGame 0.14 · room.lock 0.00 · store ListPlayers 0.06
  session.ApplyMove 0.30
    engine.Apply 0.02
    store.tx RecordMove 0.23
      readSeq 0.02 · insertMove 0.05 · updateGame 0.04 · readMove 0.03
    ws.publish 0.00
```

| Variable | Default | What |
|---|---|---|
| `BINGLE_SLOW_REQUEST` | `250ms` | How slow is slow. **0 turns the journal off**, collector and all |
| `BINGLE_METRICS_DB` | `bingle-metrics.db` beside the game database | Where the rows go. Empty logs the line and stores nothing |

**Everything else is thrown away** when the request finishes. Keeping every request would be a
trace store, and bingle has nowhere to put one. A request that panicked is always written down —
a 500 nobody can explain is what this is for — and the spans recorded before the panic say how far
it got. An accepted WebSocket is a connection, not a slow request, so it is never one.

The rows live in **a database of their own** beside the game database: measuring the server must
not write to what is being measured, and that file can be deleted or truncated without touching a
single game. Stage 5 puts the metric history in the same file.

### What it costs

Measured through the real REST API against SQLite, on a move of roughly 0.9 ms: **no difference in
time that the measurement can resolve** — the journal's medians land on both sides of the
no-journal ones across runs — and **+32 allocations on about 1020**, or 3.1%.

Those 32 are one `context.WithValue` per span: the parent chain is carried in the context, which
is how OpenTelemetry does it and the reason the SDK could replace this package without touching a
call site. Spans themselves are returned by value and allocate nothing. Measured as time, this is
inside the plan's 1% budget; measured as allocations it is not, and that trade is deliberate — the
alternative is giving up the shape that makes the journal replaceable. `BINGLE_SLOW_REQUEST=0`
removes it entirely.

### Timing something new

```go
ctx, span := trace.Start(ctx, "store.RecordMove")
defer span.End()
span.Set("stmt", "RecordMove")
```

The API is shaped like OpenTelemetry's on purpose, so that if traces ever have to leave the
machine the OTEL SDK replaces `internal/trace` and no call site changes. **Outside a request
nothing is collected** — a bot search, a batch of five hundred games and every call in a test run
with no collector, and `Start` then returns a span whose methods are nil checks at zero
allocations. Attribute values are never player data: a span ends up in the journal, which is read
like a log.

The boundaries timed today are the handler, the session funnel, the room-lock wait, every store
statement and transaction, `Legal`, `Apply`, the template render and the publish to a room's
sockets. `Legal` and `Apply` still take no context and do no I/O — the spans go **around** the
calls, never inside a ruleset (ADR 0025).

## Watching the games

Every move in bingle goes through one funnel, so instrumenting it once covers every game, JSON
definition or Go ruleset alike, and **no logging call goes into any game's code**:

```
DEBUG msg="move applied" game=tak room=X4-567673f2e96b seq=3 seat=1 move=c3 dur=15.5µs action=place
```

At the default level a server says nothing per move. A move the rules refused is at `info`,
because that is worth seeing without turning the firehose on.

| Variable | Default | What |
|---|---|---|
| `BINGLE_DEBUG_GAMES` | *(every game)* | Comma-separated game families. `debug` across two dozen games is unreadable; this narrows it |
| `BINGLE_DEBUG_ROOM` | *(none)* | One room code, logged at `info` whatever else is set, for chasing a single report |
| `BINGLE_DEBUG_SECRETS` | off | Put **full** positions in the lines instead of the public view. **Development only** |
| `BINGLE_PARANOID` | off | Check the engine against itself after every move. **Development and CI only** |

A position in a debug line is the **public** view: Courtfall commits dice in secret, Cthulhu Wars
hides Elder Signs, Nyet! hides hands, and every position carries the game's random state.
`BINGLE_DEBUG_SECRETS` is the only way to the full one, and the server logs a warning at startup
when it is on.

### Paranoid mode

`BINGLE_PARANOID=1` makes the move funnel, after every move:

1. re-run `Legal` and check the applied move is in it — the bug that otherwise reaches a player as
   a game that cannot be continued;
2. run `state.Validate` on the new position;
3. check the notation survives `Format` → `Parse` → `Format`, or a replay will not reproduce it;
4. check the ruleset's public view is not simply the whole position.

Each failure is one `ERROR` line naming the check that failed. It runs `Legal` a second time on
every move, so it is for development and CI; the server warns loudly at startup when it is on.
Writing a repro bundle beside each failure is Stage 7.

## The history, and the Health page

`/metrics` is what is true now. The history is what was true at five past four last Tuesday, and
it lives in the same file as the slow-request journal, beside the game database — so a backup of
that directory takes it, and deleting it touches no game.

| Variable | Default | What |
|---|---|---|
| `BINGLE_METRICS_SAMPLE` | `5m` | How often a sample is written. **0 keeps no history**; the metrics are still served live |
| `BINGLE_METRICS_RETAIN_DAYS` | `14` | How long samples are kept at full resolution before they are compacted. 0 keeps everything, which grows without bound |

One goroutine, one small transaction per tick. Five minutes rather than fifteen seconds is
deliberate: the point is history that survives into the backup, not a time-series database. A
Prometheus, if anyone runs one, scrapes `/metrics` as often as it likes and none of this changes.

**Counters are kept as their monotonic total** and rates are worked out on read, exactly as
Prometheus does — so a restart shows as the total returning to zero rather than as a spike, and
the page drops that step instead of drawing a cliff. A histogram is kept as its `_count`, its
`_sum` and one row per bucket, which is enough to derive a quantile afterwards.

### How it stays small

Five-minute samples are kept for `BINGLE_METRICS_RETAIN_DAYS`, then folded into one hourly rollup
each; hourly rollups are kept for a year, then folded into daily ones. A rollup holds the `min`,
the `mean`, the `max` and the `n` of what went into it, so the extremes survive compaction and
nothing is silently averaged away. Compaction happens in the sampler's own tick — no second
process — and writes the rollups **before** deleting what they came from, so an interrupted run
loses no history. The slow-request journal is purged on the same window.

A year of one series is then about 4,000 samples for the fortnight, 24 rows a day for the rest of
the year, and 365 daily rows for ever.

**One trap worth knowing:** a rollup is stamped with the **start** of its bucket, so the day
bucket holding noon begins at midnight. A window that starts at noon will not include it. Ask for
a little more than you need when reading old history.

### Reading it back

All of this is on the **private** listener only, and a test keeps it that way.

```sh
curl -s localhost:9095/metrics/series | jq            # what is kept, and how much
curl -s 'localhost:9095/metrics/history?name=bingle_db_wal_bytes&hours=48' | jq
curl -s 'localhost:9095/metrics/history.csv?name=bingle_http_requests_total&hours=24' > requests.csv
curl -s -X POST localhost:9095/metrics/sample         # take a sample now
```

`GET /metrics/health` is the page: requests a minute by route, the slowest routes by 95th
percentile, the database and its write-ahead log over time, the busiest statements, how games
ended, and moves refused by the rules. It is drawn by `js/health.js` with the same inline-SVG
charts the batch reports use, and the private listener serves the embedded static files, so the
page needs no assets of its own.

## What goes wrong in browsers

The UI is where most of bingle's recent work went and where there was no visibility at all: a
script that threw on one person's machine left no trace anywhere. `js/errors.js` hooks `error` and
`unhandledrejection` on every page and posts to `POST /api/client-error`.

| Variable | Default | What |
|---|---|---|
| `BINGLE_CLIENT_ERRORS` | `on` | `off` removes the route altogether |

The faults are readable at `/metrics/client-errors` and on the Health page, newest first — **one
row per distinct fault**, keyed by a hash of the message, the stack and the release, so a bad
release is one row with four hundred hits rather than four hundred rows. Keeping the release in
the key is what makes "this started with 0.17" answerable; the cost is that two unrelated faults
with the same message and stack do merge, and a minified frame can make a stack unhelpful.

### This is the one observability route on the public listener

A browser has to be able to reach it, so everything about it is shaped by that:

- **it is write-only.** Nothing public reads a fault back — a test keeps it that way — and the
  endpoint answers `204` whatever it decided, because a browser can do nothing with a refusal and
  telling a stranger which reports were kept is information it has no reason to give out;
- **every field is checked against a closed set or a cap.** `page` becomes a metric label and a
  stranger is posting it, so an unknown page is `other` and an unlikely version is `unknown`,
  rather than a new series (ADR 0064);
- **it is rate limited per address** (20 an hour) and the body is capped at 8 KiB, so it cannot be
  used to fill the disk. The browser also sends at most three per page;
- **token-shaped and room-shaped text is scrubbed twice** — once in the browser, once on the
  server, because the endpoint is public and a report can come from anything.

### What a report may carry

`{message, stack, page, version, browser}` and nothing else. No name, no room code, no secret.
`page` is a **route pattern** worked out in the browser (`/game/{code}`), never the URL, which
carries the room code. The browser string is the engine and its major version plus `mobile` or
`desktop` — the parts of a user agent that a fault actually depends on, and not the parts that
identify a particular machine.

Scrubbing is deliberately blunt: a run of 24 or more base64url characters becomes `[redacted]`,
and a path naming a room or a batch loses that name. It occasionally eats a minified identifier or
a hash in a filename; losing a frame's name is a smaller harm than publishing a seat token.
**What it cannot catch** is a bare room code with nothing around it — six characters of letters
and digits is any short word — which is the residual risk and the reason `errors.js` strips its
own before sending.

A fault nobody has hit for `BINGLE_METRICS_RETAIN_DAYS` is purged with the rest of the history.
