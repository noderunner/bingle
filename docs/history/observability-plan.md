# Plan: logging, metrics and a slow-request journal

Status: **Stage 1 done, stages 2–7 planned** (2026-10-28). The decisions in *What was decided*
below were taken with the owner before this was written; the rest is the reasoning and the
staging. What Stage 1 built is described in [docs/observability.md](../observability.md).

## Why, and the constraint that shapes everything

The server is growing: two dozen games (ten of them with their rules written in Go), a
session manager with locks and bot slots, a SQLite database with a dozen tables,
WebSockets, voice signalling, direct messages, batches. The owner wants four things from
it — performance and error tracing, an eye on the database as it grows, a way to find slow
or error-prone paths through the API, and metric history that survives into the backups.

What shapes every answer is the shape of bingle itself: **one binary, one container, one
SQLite file, run by one person**, with *no authentication anywhere* and a room code as the
only thing between a stranger and a game. So:

1. A design that needs a collector, a time-series database and Grafana running beside it to
   be useful would break the property the architecture is proudest of. Observability has to
   be useful with nothing else installed, and still be scrapeable by all of that later.
2. Every endpoint is world-readable unless something is done about it, and metric *labels*
   are a leak channel: a label carrying a room code publishes the key to a game.

## What exists today

- **Logging:** 18 calls in total (`main.go` 7, `internal/session/notify.go` 5,
  `internal/httpserve` 3, `internal/app` 3), all `log.Printf`/`log.Fatalf`. No request log,
  no request ids, and **no recover middleware** — a panic in a handler is swallowed by
  `net/http` per connection and nothing records it. `config.Secret` already implements
  `slog.LogValuer` to keep the SMTP password out of logs, so there is a redaction pattern to
  follow.
- **Metrics:** none. No `/metrics`, no `expvar`, no `pprof`. `/healthz` pings the database
  and returns the engine version; the Containerfile's health check uses it.
- **Hooks that already exist and should be used rather than duplicated:**
  * the hub's `tap` sees every published event (`internal/session/hub.go`);
  * `sql.DB.Stats()` is free and answers most pool questions;
  * the CPU semaphores — `hintSlots` (NumCPU), `botSlots` (2×NumCPU), `envs.matchSlots`
    (NumCPU) and the environments' batch stepper (8) — are where queueing will first hurt,
    as is the batch runner's own `Workers` (three quarters of the CPUs);
  * batches already measure `games_per_minute`;
  * `state.RenderText`, `state.Public`, `state.Validate` and canonical notation make a
    position printable, redactable, checkable and replayable;
  * `tests/allgames` already plays every game at random and checks it stays consistent.
- **Dependencies** are deliberately few (7 direct). The project hand-rolls its Markdown
  renderer, its SVG diagrams and its charts.
- **Deployment** is `./bin/bingle` (or the container) with `BINGLE_*` environment variables;
  `scripts/run.example.sh` is the template.

## What was decided

| Decision | Chosen | Why, and what was rejected |
|---|---|---|
| Metrics library | **`prometheus/client_golang`** with a **custom registry** | Battle-tested, exemplars and native histograms if wanted later. Rejected: hand-rolling (~250 lines we would own for no gain now). The default registry is *not* used, so `/metrics` carries only what we chose to register. |
| Wire format | **Prometheus text** at `/metrics` | `curl`-able with nothing installed, and scrapeable by Prometheus, VictoriaMetrics, Netdata, telegraf *and* an OTEL collector. Rejected for now: OTLP metrics, which need a collector running before they are worth anything. |
| Exposure | **A second listener on `127.0.0.1`** | The public site has no authentication; metrics and the health page must not be on it. A token is the alternative for scraping across a network. |
| Tracing | **A slow-request journal**, span-shaped | Answers "which paths are slow or error-prone" on one box with no dependencies. The span API is shaped like OTEL's, so the OTEL SDK can be dropped in later without touching call sites. Revisit when traces need to leave the machine. |
| Metric history | **A separate `bingle-metrics.db`** | Keeps the measurements out of the database being measured: no write amplification on game data, no schema churn in the game schema, and the file can be dropped or truncated without touching a single game. It sits beside `bingle.db`, so the backup is one more file in the same directory. |
| Client errors | **Collected** at `POST /api/client-error` | The UI is where most recent work went and where there is no visibility at all today. |

## What this is *not*

Not an analytics system. Nothing here counts page views, tracks players between sessions or
measures people; it measures the server. There are no accounts to attach anything to, and
this plan does not create any.

## Design

### 1. Structured logging (`log/slog`)

One logger built in `main`, handed to the app: text by default, JSON with
`BINGLE_LOG_FORMAT=json`, level from `BINGLE_LOG_LEVEL` (`info` default, `debug` is the
"verbose logging" of the request). The 18 existing calls move over, keeping their wording.

Three middlewares, in this order, in `app.newRouter`:

1. **Request id** — 8 random bytes, hex, or an inbound `X-Request-Id` if it is short and
   printable; put in the context and echoed in the response header.
2. **Recover** — a panic becomes one `ERROR` line with the stack and a 500, instead of
   vanishing. This is a bug fix the rest of the plan happens to need.
3. **Access log** — one line per request at `info` for errors and at `debug` otherwise:
   `method`, **chi's route pattern** (never the raw path: it carries room codes), status,
   bytes, duration, request id.

**Redaction policy**, written down once and tested:

- never logged at any level: seat tokens, browser secrets, DM or chat bodies, email
  addresses (hash them as `internal/session/notify.go` already does), WebRTC payloads;
- room codes appear only as the first two characters and a short hash, so sessions can be
  correlated without the log being a key to the game;
- player names are omitted by default and hashed where a line would be useless without one.

### 2. Metrics

`internal/metrics` owns a `prometheus.Registry`, registers the Go and process collectors
explicitly, and exposes typed constructors so call sites never touch label names directly.
Served by `promhttp.HandlerFor(reg, …)` on the metrics listener only.

**HTTP** — `bingle_http_requests_total{route,method,status}`,
`bingle_http_request_duration_seconds{route,method}` (histogram),
`bingle_http_requests_in_flight`, `bingle_http_response_bytes_total{route}`,
`bingle_http_errors_total{route,kind}` where `kind` comes from the existing
`api.statusForError` families.

**WebSockets** — `bingle_ws_connections{kind}` (page, voice, chat, dm) as a gauge,
`bingle_ws_connections_total{kind}`, `bingle_ws_events_total{type}` (from the hub's `tap`),
`bingle_ws_dropped_total` when a subscriber's buffer overflows.

**Database** — every statement gets a *name*; the wrapper records
`bingle_db_queries_total{stmt,result}`, `bingle_db_query_duration_seconds{stmt}`,
`bingle_db_rows_total{stmt}`, `bingle_db_tx_duration_seconds`, `bingle_db_busy_total`.
Sampled gauges: `bingle_db_file_bytes`, `bingle_db_wal_bytes`,
`bingle_db_table_rows{table}`, `bingle_db_page_count`, `bingle_db_freelist_pages`, and the
pool numbers from `DB.Stats()`. WAL growth and the `moves` table are what will bite first;
both become visible before a player feels them.

**Games and the engine** — `bingle_game_moves_total{game,result}`,
`bingle_game_apply_duration_seconds{game}`, `bingle_game_legal_duration_seconds{game}`,
`bingle_game_legal_moves{game}` (histogram),
**`bingle_games_finished_total{game,end_reason}`** and
**`bingle_game_illegal_moves_total{game}`**. The last two are the most valuable series in
this plan: a spike in `ply-limit` or `no-legal-moves` means a rule stopped working, and a
spike in illegal moves means a rules change or a stale client.

**Bots and slots** — `bingle_bot_search_duration_seconds{bot,game}`,
`bingle_bot_nodes_total{bot}`, and for each semaphore
`bingle_slot_wait_seconds{pool}` + `bingle_slot_in_use{pool}` (`hint`, `bot`, `match`,
`env-batch`), and `bingle_batch_workers` for the batch runner. Queue *wait* is the number
that matters as load grows.

**Session and features** — `bingle_rooms_active`, `bingle_rooms_subscribed`,
`bingle_people_here`, `bingle_voice_calls`, `bingle_dm_threads`,
`bingle_batch_games_total{game,status}`, `bingle_mail_sent_total{kind,result}`,
`bingle_client_errors_total{page}`, and `bingle_build_info{version}` set to 1.

**Cardinality is a whitelist**, not a hope: route patterns (~60), game *families* (~25),
statement names (~60), bot kinds (4), status classes, pool names. **Never** room codes,
player names, addresses, board positions or move notation. A test asserts the series count
of a freshly started server stays under a stated ceiling, so a careless label is caught in
CI rather than six months later.

### 3. The slow-request journal (tracing, lightly)

`internal/trace` with an OTEL-shaped API:

```go
ctx, span := trace.Start(ctx, "store.RecordMove")
defer span.End()
span.Set("stmt", "RecordMove")
```

The HTTP middleware attaches a collector to the context: a capped slice (64) of
`{name, parent, start, duration, attrs}`, taken from a `sync.Pool`. Spans are added by the
boundaries that matter — handler, session funnel, **room-lock acquisition**, store
statement, `Legal`, `Apply`, bot search, template render, WS publish.

When the request finishes, the collector is **discarded** unless the request took longer
than `BINGLE_SLOW_REQUEST` (default 250 ms) or ended 5xx. Then it writes one row to
`slow_requests` in the metrics database and one `WARN` line with the request id and the
phase breakdown, so "the API is slow sometimes" becomes "`POST /api/games/{code}/moves`
spent 410 ms of 430 ms waiting for the room lock".

Because the API is span-shaped, swapping in the OTEL SDK later changes `internal/trace` and
nothing else.

### 4. History, in its own database

`bingle-metrics.db` (`BINGLE_METRICS_DB`, default beside the game database; empty disables
history), opened with the same WAL settings, with its own schema:

```sql
CREATE TABLE samples (                 -- one row per series per tick
  at     INTEGER NOT NULL,             -- unix seconds
  name   TEXT    NOT NULL,             -- "bingle_http_requests_total"
  labels TEXT    NOT NULL,             -- canonical "route=/api/games,method=POST", '' for none
  value  REAL    NOT NULL,
  PRIMARY KEY (name, labels, at)
) WITHOUT ROWID;

CREATE TABLE rollups (                 -- compacted older history
  bucket TEXT NOT NULL,                -- "hour" | "day"
  at INTEGER NOT NULL, name TEXT NOT NULL, labels TEXT NOT NULL,
  min REAL NOT NULL, mean REAL NOT NULL, max REAL NOT NULL, n INTEGER NOT NULL,
  PRIMARY KEY (bucket, name, labels, at)
) WITHOUT ROWID;

CREATE TABLE slow_requests (
  id INTEGER PRIMARY KEY AUTOINCREMENT, at INTEGER NOT NULL,
  request_id TEXT NOT NULL, route TEXT NOT NULL, method TEXT NOT NULL,
  status INTEGER NOT NULL, duration_ms REAL NOT NULL,
  spans TEXT NOT NULL,                 -- JSON: the phase breakdown
  err TEXT
);

CREATE TABLE client_errors (
  hash TEXT PRIMARY KEY,               -- message + stack + version
  first_seen INTEGER NOT NULL, last_seen INTEGER NOT NULL, count INTEGER NOT NULL,
  version TEXT NOT NULL, page TEXT NOT NULL, message TEXT NOT NULL, stack TEXT NOT NULL,
  browser TEXT NOT NULL
);
```

A sampler every `BINGLE_METRICS_SAMPLE` (default 5 min) gathers the registry and writes one
transaction: counters as their monotonic total (rates are computed on read, as Prometheus
does), gauges as their value, histograms as `_count`, `_sum` and one row per bucket (the
bucket in the labels), so quantiles can be derived later. Compaction in the same job: 5-minute
samples kept `BINGLE_METRICS_RETAIN_DAYS` (default 14), then hourly for a year, then daily.
Growth is bounded and needs no second process.

Reading it back: `GET /metrics/history?name=…&since=…` (JSON) and a CSV export, both on the
metrics listener, plus `GET /metrics/health` — a **Health page** drawn with the existing
`js/charts.js` (requests per minute, p95 by route, error rate, database and WAL size, the
slowest statements, active games, the client-error table). The embedded static filesystem is
served from the metrics listener too, so the page needs no new assets.

### 5. Client-side errors

`js/errors.js` hooks `window.onerror` and `unhandledrejection` — what the `collectJSErrors`
test helper already proves catches our failures — and posts
`{message, stack, page, version, browser}` where `page` is the route pattern, never the URL
(it carries room codes), and `version` is the `bingle-version` meta so a bad release is
obvious.

Shaped so it cannot become a nuisance: at most a few per session, rate-limited per address,
body capped, token-shaped strings scrubbed before sending, and **deduplicated at the
server** by `hash(message+stack+version)` — a broken release is one row with 400 hits, not
400 rows. It lands in the metrics database, counts in `bingle_client_errors_total{page}` and
shows on the Health page, newest and noisiest first. `BINGLE_CLIENT_ERRORS=off` turns the
endpoint off entirely.

### 6. Debugging the game implementations

Rulesets are **pure functions**: `Legal(game, st, seat)` and `Apply(game, st, seat, mv)`
take no `context`, do no I/O, and are deterministic given a seeded position. That is what
makes games testable, replayable and batchable, and this plan does not spend it. No logging
calls go into game code.

**a. The platform logs around the games.** Every move passes through one funnel
(`session.applyAndRecordAs`). Instrumented once, every game — JSON or Go ruleset — gets
the same line:

```
level=DEBUG msg="move applied" game=courtfall room=7f3a… seq=37 seat=2 phase=play
  stage=act action=strike move="Q d5>d4 2/2" legal=43 dur=1.8ms locked=true
```

**b. The narration is already the game's log.** Most of the ten rulesets produce player-facing events
("Light lays a stone on g7", "6 of Dark's stones can see no friend and come off"), shown in
the History tab. At `debug` the server mirrors `ext.events` to the logger with the same
attributes — no new code in any ruleset, and it rewards games that narrate well.

**c. Explanation on demand, instead of permanent verbosity.** The questions actually asked
of game code are "why is this move not legal?" and "why did that group die?". An optional
pure interface answers them:

```go
// Diagnostic is implemented by a ruleset that can explain itself.
type Diagnostic interface {
    Explain(game *catalog.CompiledGame, st *BoardState, seat int, mv Move) []string
}
```

called only from tests and from `POST /games/{code}/explain` on the metrics listener. Zero
cost in production, implementable game by game.

**d. Repro bundles — the highest-value piece.** Game bugs are diagnosed by replaying the
position, not by reading logs. Whenever something impossible happens — `Apply` rejects a
move `Legal` listed, a ruleset panics, `Validate` fails after a move, or a batch game ends
`illegal-move`/`no-legal-moves` unexpectedly — write a bundle to `BINGLE_DEBUG_DIR`:

```json
{ "bingle": "0.11.0", "game_def_id": "courtfall~arena", "seed": 1234,
  "start_state": {…}, "moves": ["…"], "seat": 2, "move": {…},
  "error": "…", "board": "<state.RenderText>" }
```

with one `ERROR` line carrying its id. `scripts/replay <bundle>` (and a `TestRepro` that
takes a file) reproduces it in-process, deterministically, with no server and no browser.
The batch runner emits the same shape, so a 500-game batch files its own repros.

**e. Paranoid mode.** `BINGLE_PARANOID=1` (dev and CI, never production) makes the move
funnel, after every move: re-run `Legal` and assert the applied move is in it; run
`state.Validate`; check `Format→Parse→Format` round-trips; and check the ruleset's
redaction against `state.Public`. One line each, and each failure writes a repro bundle.

**f. Two knobs, because `debug` across two dozen games is unreadable.**
`BINGLE_DEBUG_GAMES=courtfall,nyet` limits the firehose to named games;
`BINGLE_DEBUG_ROOM=AB12CD` logs everything about one room whatever the level.

**g. Secrets, which games make sharper.** Courtfall commits dice splits in secret, Cthulhu
Wars hides Elder Signs, Nyet! hides hands, and positions carry RNG state. Debug logs
therefore carry the **public** view (`state.Public`); full positions only with
`BINGLE_DEBUG_SECRETS=1`, documented as development-only. Repro bundles are full fidelity by
definition and go to a directory, never to the log stream.

## Configuration

| Variable | Default | What |
|---|---|---|
| `BINGLE_LOG_LEVEL` | `info` | `debug` is the verbose mode |
| `BINGLE_LOG_FORMAT` | `text` | `json` for a log collector |
| `BINGLE_METRICS_ADDR` | `127.0.0.1:9095` | empty disables metrics, the health page and the journal endpoints |
| `BINGLE_METRICS_TOKEN` | — | required as a bearer token when the listener is not on localhost |
| `BINGLE_METRICS_DB` | beside `bingle.db` | empty disables history (metrics still served live) |
| `BINGLE_METRICS_SAMPLE` | `5m` | how often history is written |
| `BINGLE_METRICS_RETAIN_DAYS` | `14` | before compaction to hourly, then daily after a year |
| `BINGLE_SLOW_REQUEST` | `250ms` | 0 disables the journal |
| `BINGLE_CLIENT_ERRORS` | `on` | the browser error endpoint |
| `BINGLE_DEBUG_DIR` | — | where repro bundles are written; empty disables them |
| `BINGLE_DEBUG_GAMES`, `BINGLE_DEBUG_ROOM` | — | narrow the debug firehose |
| `BINGLE_DEBUG_SECRETS`, `BINGLE_PARANOID` | off | development only |

## Staged plan

**Stage 1 — logging.** *Done (issue #28).* `slog`, the three middlewares, the redaction helpers,
the level and format settings, and the 18 call sites moved over. Tests: a panic returns 500 and
logs once with a stack; the request id travels to the response header; a log-capture test fails on
anything resembling a secret, an address, a token or a full room code. Two things came out
differently from the design above: the **recover middleware goes inside the access log, not
outside it** (a panic unwinds past every middleware outside the one that recovers, so with the
recover first a panicking request would be the one request never logged as a request — and, from
Stage 2, never counted), and a **WebSocket is logged when it is accepted and when it closes**
rather than as one request whose duration measures the whole visit. The redaction policy became
ADR 0063, which Stage 2's label whitelist inherits.

**Stage 2 — metrics and the listener.** `internal/metrics` with the custom registry, the
HTTP middleware, the Go and process collectors, the second listener (and its token path),
`/metrics`. Tests: the exposition parses and holds the expected families; **`/metrics` is
404 on the public listener**; the listener binds to localhost by default; the series ceiling
holds; `-race` on concurrent requests.

**Stage 3 — the database.** Named statements across the ~74 call sites in
`internal/store` (a thin wrapper, so the diffs are one line each), the busy counter, the
sampled gauges. Tests: a failing query counts as `result=error`; a contract test fails if a
new call site bypasses the wrapper (the same trick as the existing DOM-contract tests);
`PRAGMA` gauges read back sanely on a temporary database.

**Stage 4 — games, bots and the journal.** The move-funnel log and metrics, the end-reason
and illegal-move counters, slot waits, the `trace` package, the `slow_requests` writer, and
the paranoid-mode assertions. Tests: a deliberately slow handler writes exactly one journal
row with its spans in order; paranoid mode catches a ruleset that returns a move `Legal`
does not list; the public view never carries a secret under debug logging.

**Stage 5 — history and the Health page.** The metrics database, the sampler, compaction and
retention, `/metrics/history`, the CSV export and the page. Tests: a sample tick writes one
transaction; compaction preserves totals; retention deletes nothing newer than its window;
the page renders from a seeded history.

**Stage 6 — client errors.** `js/errors.js`, the endpoint, deduplication, limits, the table
on the Health page. Tests: a browser test throws on purpose and the row appears with a count
of 1, then 2; a flood is refused; no room code or token ever reaches the row.

**Stage 7 — repro bundles and `scripts/replay`.** The writer, the bundle format, the command
and `TestRepro`. Tests: a bundle written from a forced inconsistency replays to the same
error, and nothing else in the suite writes one.

Documentation goes with each stage: a new `docs/observability.md`, the variables in
`docs/build.md`, the endpoints in `docs/api.md`, the game-debugging section in
`docs/dsl.md`, and an ADR when Stage 2 lands.

## Performance budget

Target: **under 1% on a move, under 2% on a batch**. Measured with the project's own tools —
a 500-game `tactical` batch of Cthulhu Wars and of Hex, before and against each stage, plus
`go test -bench` on `Apply` for one JSON game and one ruleset. The journal's per-request
allocation (one pooled slice) and the sampler's transaction are the two costs to watch; if
either shows up, the span cap and the sample interval are the dials.

## Residual risks, stated plainly

- **Observability is a leak channel.** The listener, the label whitelist and the redaction
  tests are the whole defence. If the metrics listener is ever exposed without a token, game
  activity and the health page go with it.
- **Histogram buckets are hard to change** once history holds them: a bucket change makes
  older quantiles incomparable. They should be chosen once, deliberately, in Stage 2.
- **The metrics database shares the disk** with the game database. A separate file keeps the
  schemas and the write patterns apart, but not the I/O; the sampler writes one small
  transaction every five minutes, which is the reason for that interval.
- **Deduplicating client errors by message and stack** will occasionally merge two unrelated
  faults, and minified or inlined frames can make stacks unhelpful. Keeping the version in
  the key limits the damage.
- **The journal is not distributed tracing.** It has no UI beyond the Health page and
  cannot follow a request into another process. That is the trade accepted for now; the span
  API is shaped so OTEL can replace it without touching call sites.
- **Paranoid mode doubles `Legal`** on every move. It must stay off in production, and the
  config should say so loudly.

## Status log

- 2026-10-28: Stage 1 landed: `internal/logging` (the logger, the redaction helpers and the three
  middlewares), `BINGLE_LOG_LEVEL` and `BINGLE_LOG_FORMAT`, the 18 call sites, ADR 0063 and
  `docs/observability.md`. A panic in a handler is recorded for the first time.
- 2026-10-28: plan written after a design discussion. Decisions settled:
  `prometheus/client_golang` with a custom registry, a separate localhost metrics listener,
  client-side error collection, the slow-request journal instead of OTEL for now, and a
  separate `bingle-metrics.db`. Nothing implemented.
