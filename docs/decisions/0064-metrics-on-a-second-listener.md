# 0064. Metrics on a second listener, from a registry of our own

Status: accepted (2026-10-28)

## Context

Stage 2 of [the observability plan](../history/observability-plan.md) gives bingle metrics. The
decisions below were settled with the owner before the plan was written; this records them with
what implementing them actually cost, and settles the two questions the plan deliberately left to
this stage — the histogram buckets and the shape of the private listener.

The constraint is the one the whole plan turns on: bingle is **one binary, one container, one
SQLite file, run by one person**, and **the public site has no authentication of any kind**. A room
code is all that stands between a stranger and a game (ADR 0005), and identity is a name somebody
types (ADR 0006).

## Decision

**`prometheus/client_golang`, with a registry of our own.** `prometheus.NewRegistry()`, never
`prometheus.DefaultRegisterer`, so `/metrics` carries exactly what bingle registered and not
whatever a dependency registered behind our back. The Go and process collectors are registered
explicitly, by name, for the same reason. A test registers a series into the default registry and
fails if it reaches the exposition.

Hand-rolling was rejected in the plan (~250 lines we would own for no gain). The cost of not
hand-rolling, stated plainly: **one new direct dependency, seven new indirect ones, and 3.6 MB on
a 33.9 MB binary (+10.7%)**. That is the price of exemplars, native histograms and an exposition
every scraper already understands, and it is the largest single dependency bingle has taken.

**Prometheus' text format at `/metrics`.** `curl`-able with nothing installed, and scrapeable by
Prometheus, VictoriaMetrics, Netdata, telegraf and an OTEL collector alike. OTLP was rejected for
now: it needs a collector running before it is worth anything.

**A second listener, on `127.0.0.1` by default.** `BINGLE_METRICS_ADDR`, default
`127.0.0.1:9095`; empty turns the whole thing off, registry and all. `/metrics` is **never**
mounted on the public router, and a test asserts it is a 404 there. Put the listener anywhere a
network can reach — `:9095`, `0.0.0.0:9095`, a routable address — and `BINGLE_METRICS_TOKEN`
becomes **required**, refused at startup rather than warned about, because what that listener
serves is a running account of which games are being played and how busy the server is. The token
is a `config.Secret`, compared in constant time over a SHA-256 of each side, and at least 16
characters.

**The listener binds before it serves.** `httpserve.ListenPrivate` returns something that has
already bound and can be asked `Addr()`, unlike `httpserve.Serve`, which is handed an address and
binds later. That is deliberate: the later shape is what made `tests/httpserve` flaky, because a
test had to guess a free port and then race the kernel for it. A test of this listener passes
`127.0.0.1:0` and is told what it got.

**Labels are a whitelist, and call sites never name one.** Every series is set through a method on
`*Metrics`, which decides the label values; nothing outside the package passes a label string.
The closed sets are route patterns (~60), refusal families (7), socket kinds (4) and status codes.
**Never** a room code, a player's name, an address, a board position or a move notation — this is
ADR 0063 applied to a second channel, and a published one: a label on `/metrics` is as readable as
a log line, and more durable, because it ends up in a time series.

The route label is **chi's pattern**, taken from `logging.Route` rather than a second copy of that
rule, so what a metric records and what the access log writes cannot drift apart.

**Refusals are counted by family, read back off the status.** `bingle_http_errors_total{route,kind}`
with `kind` in `bad-request`, `forbidden`, `not-found`, `conflict`, `rate-limited`, `client`,
`server` — the families `internal/api`'s `statusForError` already sorts errors into. Reading them
off the status rather than the error means the counter needs no cooperation from any handler, and
that a refusal written by something else (chi's own 404, a body too large) is counted the same way.

**The histogram buckets are chosen once, here.** Fourteen of them, a millisecond to thirty
seconds:

```
0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30
```

A millisecond is a static file off the embedded filesystem; a move is about a millisecond, so the
dense middle is where the answers are; thirty seconds is past the longest bot move, with `+Inf`
above it. **These are not to be changed casually.** Once the metrics database (Stage 5) holds
history in these buckets, a change makes quantiles from before it incomparable with those after —
which is the one mistake in this plan that cannot be undone by editing code.

**A `nil *Metrics` is the switched-off server.** Every method is nil-safe and `Middleware` returns
the handler untouched, so a deployment without `BINGLE_METRICS_ADDR` — and almost every test —
pays nothing and needs no branches at the call sites.

**A socket is not a request.** An accepted upgrade (101) is recorded as a connection starting,
in `bingle_ws_connections{kind}` and `bingle_ws_connections_total{kind}`, and never in the request
counter or the duration histogram: a room socket lives as long as the player is on the page, so
counting it as a request answered in that time would make every duration meaningless. This is the
metrics side of the same decision Stage 1 took for the access log.

## Consequences

* **The series count of a fresh server has a ceiling in CI.** 40 today, the test fails over 60.
  The headroom is for the runtime collector growing with a Go release; a new label is what the
  test is there to catch.
* **The cost on a move is inside the plan's 1% budget.** Measured through the real REST API against
  SQLite: a move is about 0.9 ms, and the difference in time is below the noise of the
  measurement. The stable signal is allocations — **992 to 1001 per move, +0.9%**. The synthetic
  figure for the middleware alone is +0.4 µs and 3 allocations on a request, which matters only to
  a handler that does nothing.
* **The gauge of open sockets falls late.** A room socket's handler is parked on the room's event
  channel rather than on the connection, so it learns a player has gone only when the room next
  has something to say. Stage 4 instruments the handlers and is where that is tightened.
* **`bingle_ws_events_total` and `bingle_ws_dropped_total` are not here.** They come from the
  hub's `tap`, which is Stage 4's work, as are the database, game, bot and session families.
* **Scrape instrumentation is included on purpose.** `promhttp_metric_handler_requests_total`
  comes from registering the handler into our own registry: it says whether scrapes are
  succeeding, which is worth three series.
* **Nothing is exposed by `/healthz`.** The public health check still answers status and engine
  version only, and the same handler is on the private listener so a scrape target and a health
  check can share a port.
* **The private listener is where Stages 5 and 6 land.** `/metrics/history`, the CSV export, the
  Health page and `POST /api/client-error`'s reporting view all belong behind it, and it already
  has the token path they need.
