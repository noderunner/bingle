# Proposal: games as services, in process or out

Status: **proposal, deferred** (written 2026-11-07). The maintainer chose the private overlay of the
plugins plan (`game-plugins-plan.md`, decisions 1 and 2) for now. This records an alternative to
revisit if the number of compiled-in games keeps growing and the refactor starts to look worth its
cost. Nothing here is decided or built.

## Why this exists

The plugins plan rejected two ways of loading game code at run time:

- **`.so` plugins** (`plugin.Open`): they need cgo, which ends the static binary (ADR 0014). They load
  only into the exact binary they were built against. They can never be unloaded. They run with the
  server's full access.
- **WebAssembly or a subprocess at the rules boundary**: every `Legal` and `Apply` call would cross a
  serialization boundary, and bots, practice batches and training environments make millions of them.

Both kept Go games compiled in, and the commercial ones went to a private overlay: a gitignored
`internal/games/private/` folder built in by a build tag. The plan noted one way out of the
performance objection: put the bot search inside the plugin, so the boundary is crossed once per move
decision. This proposal takes that all the way. **Cut between the platform and the game at the level
of a player's action, not a rules call.** The same game can then run inside the server or in a
process of its own, at no cost to the games that stay inside.

## When to revisit

Any one of these would make it worth reopening:

- **Several private games, maintained apart.** Keeping them checked out into one overlay folder,
  in step with the public engine, becomes the friction.
- **A game needs its own release cycle**, or its own repository and licence, beyond what a build tag
  gives.
- **A game's crash or a slow search hurts other games.** Today a panic or a runaway goroutine in one
  ruleset is in the same process as every room.
- **Two versions of a Go game need to run at once**, or one game must be upgraded without restarting
  the server. Prototypes already keep each rules version as a game of its own (ADR 0098), which shows
  the wish.
- **Sharing game code between servers becomes a goal.** Packs are data-only today (plugins decision 5).
  A separate process can be confined by the operating system, which a `.so` never could.
- **A game written in another language.**
- **Build and test times** grow with every compiled-in game, and each test run carries all of them.

As of writing there are 12 Go rulesets, and 5 of them are headed for the overlay.

## The cut

**The core** stays one program and owns everything that spans games:

- accounts, rooms and the lobby;
- chat, presence and direct messages;
- clocks, ratings, tournaments and notifications;
- SQLite;
- the event hub;
- the public REST, WebSocket and MCP API, so clients still see one API;
- the web page's shell.

**A game service** owns everything about one game family:

- its definitions, variants and derived games;
- rules, notation and gestures;
- hidden information;
- views, and its guide, tutorials and web assets (ADR 0124's `web.yaml` and scripts);
- bots and their search;
- practice batches, tuning, training environments and puzzles;
- the conversion of stored positions.

**A game service does not implement the public API.** A game that served the whole API would need
accounts, rooms, clocks, ratings and tournaments as a library, and a share of one database. Features
that span games (the lobby, friends, ratings, tournaments, the home page) would need a central service
anyway. The narrow protocol gets the modularity without a copy of the platform in every game.

**A game service keeps no state.** The core stores every position, as today. It passes the position in
with each call and stores what comes back. This is what `Legal` and `Apply` already are: pure and
deterministic, with no I/O and no `context` (CLAUDE.md). A stateless service is those functions with a
socket in front. It also keeps SQLite to one writer.

## One interface, two transports

The protocol is a Go interface, `GameService`. It has two implementations, and a game never knows which
one it is behind:

- **In process**: the service is a value in the server, and the calls are function calls. The engine
  and the built-in games stay linked in as now. The default build remains one static binary, and
  nothing gets slower.
- **Out of process**: the same calls go over a Unix socket (or stdin and stdout) to a separate binary
  that the core starts and supervises. The encoding is JSON or `net/rpc` from the standard library, so
  no dependency is added. gRPC and HashiCorp's `go-plugin` do this too, but they would add far more
  than they give here.

A game binary is a small `main` that wraps the same package in the server side of the transport. One
helper in the engine module gives every game that `main` for free.

## The protocol, from today's interfaces

Today's `state.Ruleset` has 9 methods and about 17 optional interfaces. The engine is called from
`session`, `bots`, `envs`, `batch`, `tune`, `puzzles`, `tutorial`, `repro` and `store`. Most of those
calls stay inside the game service. Only the coarse ones cross:

| Today | Called from | In the protocol |
|---|---|---|
| `catalog.Module` (definition, assets, tutorials), `Describer`, `ListLabeler`, `Sampler`, `SetupCatalogs` | catalog, session, api | **`Describe`**: the family's games, variants, setup catalogs, list labels and asset index, fetched once at start-up and on upgrade. Assets are fetched by path |
| `catalog.Deriver` (derived games), `SeatCults`, `OwnSeatCults` | catalog, session | **`Derive(options)`** and **`SeatOptions(...)`**: setup questions, asked when a game is created |
| `Start`, `Seeded` | session | **`Start(game, options, seed)`** → the first position, impulse seat and views |
| `Legal`, `Gestures`, `Format`, `Parse`, `LegalSelections` | session, api, mcp | **`Selections(game, position, seat, made)`** → the next selections with notation and gestures (the selections query of ADR 0114) |
| `Apply`, `ImpulseSeat`, `Outcome`, `PendingReaction`, `RepeatKey` | session | **`Submit(game, position, seat, move)`**: one impulse applied atomically. Returns the new position, the next impulse seat, any reaction window, the outcome, and every seat's and the public view. One round trip per impulse |
| `Redact`, `RedactMove`, `Text` | session, api, mcp | **`View(game, position, viewer)`**, for a late joiner or a spectator. `Submit` already returns the views for the seats at the table |
| `CanTakeBack` | session (undo) | **`CanTakeBack(game, before, after)`** |
| `Validate` | api (custom starts) | **`Validate(game, position)`** |
| `Searchable`, `Heuristic`, `BotGuide`, `Determinize` | bots | **stay inside**. The protocol has **`Bot(game, position, seat, level, budget)`** → a move |
| `PlaneEncoder`, `ActionLister` | envs | **stay inside**. A training environment runs in the game service, and the core relays one `EnvStep` per agent step, or the trainer talks to the game binary directly |
| practice batches, tuning (`batch`, `tune`) | batch, tune | **stay inside**. **`RunBatch(spec)`** streams progress and returns the report, its statistics described by the game (plugins step 3) |
| puzzles, tutorial replay, `repro` | puzzles, tutorial | **stay inside**, behind `Puzzle(...)` and `CheckTutorial(...)` |
| `Module.Upgrade` | store at start-up | **`Upgrade(document)`**, called on each stored position before the game is served |

Declared games, data packs included, are served by a **built-in declared-game service**: the DSL
interpreter as one more in-process game service. A pack stays data, and the engine stays the only code
that runs it.

## Performance

The one thing that crosses the boundary in play is one `Submit` per impulse, plus `Selections` while a
player builds a move and `View` for a late joiner. That is a few calls a second at a human table, and
each is one local-socket round trip and the encoding of a position. This is measured in microseconds to
a millisecond, far below a WebSocket push to a browser. It has **not been measured**. The API-only play
test (#110) would measure it, through the out-of-process transport, before anything moves.

Everything that makes millions of calls stays inside one process: bot search, practice batches,
training environments and puzzles. A bot-against-bot game at full speed is the case to watch. It
crosses once per move, as a human game does, with the search inside. A Cthulhu Wars position is the
largest to encode. If that shows up, the core can pass the previous position by reference and keep a
small cache in the game service. That cache can be dropped at any time, so the service stays stateless.

## Lifecycle

- **Start-up**: the core starts each configured game binary, checks the protocol version in a
  handshake, and calls `Describe`. A game that fails is logged and left out. Its rooms show the game
  as unavailable, and nothing else is affected.
- **Failure**: every call has a deadline. A crashed service is restarted with backoff. While it is
  down, its rooms refuse moves and say why. Whether clocks stop meanwhile is for the design.
- **Upgrade**: replace the binary and restart that service alone. Two versions of a game are two
  services with different ids, which is how prototypes are versioned already.
- **Isolation**: an out-of-process game can run as its own user, with no network and no access to the
  database. That would make shared code packs possible one day, though this proposal does not propose
  them.

## What it costs

- **The engine becomes public.** A game in another repository must import `state`, `dsl`, `catalog`,
  `board` and `cards`, which live under `internal/` today. They would move to a public module with a
  promise of source stability. That is the step the overlay was chosen to avoid. Protocol versioning
  removes the `.so` problem of rebuilding everything against one binary, but the engine's Go API still
  becomes something games depend on.
- **The protocol is the real work**: folding the interfaces above into a few versioned calls, and
  deciding what each call returns so the core never needs a second round trip.
- **Operations**: supervising child processes, health checks, deadlines, the "game unavailable" state,
  and logs and metrics that name the game service (observability plan).
- **One static binary stops being the only shape.** It stays the default, but a server with external
  games is several processes and some configuration.
- **Tests**: the suite would run each game through both transports, or at least the out-of-process
  one in CI, so the two cannot drift.

## The path, if it is taken

Each stage is useful alone:

1. **Draw the interface in process.** Gather today's calls from `session`, `bots`, `envs`, `batch`
   and the rest behind `GameService`, implemented directly over the engine. No behaviour changes, and
   it is a good refactor in its own right: the platform would stop reaching into rulesets.
2. **The out-of-process transport**, with one public game run through it in the test suite, and the
   performance of the API-only play test measured.
3. **The engine as a public module**, with its stability promise.
4. **The private games become binaries of their own**, replacing the overlay, Cthulhu Wars first.

## What to do now, so the door stays open

These cost little and help the overlay too:

- **Plugins step 2** (games register themselves) should register a family through one value, close
  to `GameService`, not scattered hooks.
- **Keep `Legal` and `Apply` pure.** It is already a rule, and it is what makes a stateless service
  possible.
- **New platform code calls the engine through the session layer**, not straight into a ruleset's
  optional interfaces.
- **ADR 0124 already helps**: a game's look is a file the web page reads, which a game service can
  serve like any other asset.

## Open questions, for when it is revisited

1. Socket transport: JSON (debuggable, slower) or a binary encoding (`gob` or `net/rpc`)?
2. Does a training environment go through the core, or does a trainer talk to the game binary directly?
3. Is a game binary started by the core, or run separately (by systemd or a container) and only
   connected to?
4. Do private games ever run on another machine, or always on the same host?
5. What does the core show for a game that is configured but not running: hidden, or listed as
   unavailable?

## Status log

- 2026-11-07: written at the maintainer's request as a deferred alternative to the private overlay,
  after asking whether games could be separate Go binaries behind a lighter API acting as a router. The
  maintainer chose to keep the internal plan for now.
