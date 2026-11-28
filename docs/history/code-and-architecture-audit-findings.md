# Findings: the code and architecture audit, measured

Status: **the first deliverable of issue #249**, measured on `main` at d8c5d4d (2026-11-28). It
changes no code. The plan is [code-and-architecture-audit-plan.md](code-and-architecture-audit-plan.md);
the maintainer's answers to its open questions bind what follows:

1. No JavaScript build step.
2. Moving internal packages is fine where it notably improves organisation and readability without
   costing performance or security.
3. (Answered earlier: the commercial games are in the private overlay, ADR 0127.)
4. Loosely: functions under 50 lines, files between 200 and 500 lines.
5. Documentation is out of scope; overly verbose comments in the code are in. A comment says intent
   only where the names and a skim of the statements do not.

## How the numbers were made

Every number here comes from two scripts that read the tree and the CI logs, and can be run again:

```sh
go run ./scripts/audit [-out DIR] [-top N] [sizes funcs comments churn deps dead dup]
sh scripts/audit/ci-logs.sh DIR 60    # needs BINGLE_GH_REMOTE and the app; downloads logs
sh scripts/audit/ci-report.sh DIR
```

`scripts/audit` prints the tables below and writes the full lists as tab-separated files (by default
into `bingle-audit` in the temporary directory). It uses the standard library only and takes about a
second. What each section does:

- **sizes, funcs**: every tracked `.go` and `.js` file. A test is a `_test.go` file or anything under
  `tests/` (the fixture games included). Go functions are `go/ast` declarations, from `func` to its
  closing brace; a closure counts in the function around it. JavaScript functions come from a small
  lexer in the script (comments, strings, template literals, regular expressions) that pairs brackets
  and calls a `{` a function body when it follows `=>`, `function name(…)` or a method head. Every
  JavaScript file's brackets balanced, and spot checks of the longest functions land on the right
  closing brace. It counts nested functions too (the "nested in" column), so a nested function's lines
  are also its parent's.
- **comments**: a comment line holds a comment and no code. *Doc* lines are the package comment, the
  doc comment of an exported top-level name and the doc comment of a struct's exported field (schemagen
  turns those into the API's and the definitions' JSON schema descriptions, unexported request types
  included); in JavaScript, a `/** */` block. The density is the other comment lines per 100 code lines.
- **churn**: `git log --since=30.days` and `90.days --name-only`. Every pull request is squashed into
  one commit on `main`, so a commit is a pull request; there were 268 in 30 days and 294 in 90.
- **deps**: `go list` of the module's packages and their own imports (tests left out), and the
  JavaScript lexer's `import` and `import("…")` of a string.
- **dead**: by `go/ast` over every Go file, whatever its build tags. An unexported function, type,
  variable or constant is *unused* when no identifier of its name appears in its directory apart from
  its declaration; an exported one is *used nowhere outside* when no other directory writes
  `pkg.Name` with that package's import, or, for a method, `.Name` at all. Methods that satisfy the
  standard library's interfaces, names the templates use and functions with `//go:wasmexport` are left
  out. It over-counts uses (a local variable of the same name hides a dead function), so it misses some
  dead code and never invents it. **An unused export is a candidate, not a verdict**: the private
  overlay builds with `-tags private` against these packages and uses names this tree cannot see.
  JavaScript exports are matched by name against every other module, page script and template, and
  against the browser tests' sources.
- **dup**: Go function bodies compared token for token, once as written and once with every name and
  literal blanked (same shape); and runs of six or more lines (indentation, blank lines, comments,
  bracket-only lines and imports left out) found in two files, per area.
- **CI** (`ci-logs.sh`, `ci-report.sh`): the last 60 completed runs of the `CI` workflow on pushes to
  `main` and the last 60 on pull requests (2026-11-24 to 2026-11-28), every attempt of a rerun, through
  the GitHub App. Job times are the API's; package times are the `ok` lines of `go test`; failed tests
  are the `--- FAIL` lines.

## The numbers

### Sizes

| | files | lines | over 500 | under 200 | functions | over 50 | over 100 | over 200 |
|---|---|---|---|---|---|---|---|---|
| Go, code | 360 | 86,008 | 43 | 208 | 2,924 | 212 | 46 | 7 |
| Go, tests | 336 | 82,572 | 29 | 168 | 2,460 | 271 | | |
| JavaScript | 66 | 24,349 | 10 | 34 | 1,594 | 84 | 19 | 6 |

The small files cluster where a package is cut by concern, which is fine: `internal/session` (28 of 59
under 200), `internal/api` (17 of 26), `internal/store` (16 of 31), `tests/app` (73 of 140) and
`internal/web/static/js` (32 of 64). Nothing below asks for a file to grow.

The largest code files:

| file | lines | commits 30d | functions over 50 |
|---|---|---|---|
| `internal/web/static/js/game.js` | 6,257 | 83 | 22 |
| `internal/web/static/js/home.js` | 1,706 | 35 | 7 |
| `internal/session/tournaments.go` | 1,429 | 5 | 4 |
| `internal/games/courtfall/v1.0.0/rules.go` | 1,388 | 1 | 7 |
| `internal/config/config.go` | 1,280 | 16 | 1 |
| `internal/auth/handlers.go` | 1,218 | 17 | 2 |
| `internal/web/static/js/render.js` | 1,197 | 20 | 4 |
| `internal/games/chess/rules.go` | 1,125 | 13 | 3 |
| `internal/mcpserver/tools_v2.go` | 989 | 16 | 2 |
| `internal/web/static/js/themes.js` | 936 | 7 | 1 |

The longest functions in code:

| function | lines | where |
|---|---|---|
| `registerEnvTools` | 321 | `internal/mcpserver/tools_v2.go:640` |
| `Manager.CreateGameWithOptions` | 307 | `internal/session/create.go:261` |
| `registerPlayTools` | 305 | `internal/mcpserver/tools_v2.go:21` |
| `installTopbar` (JS) | 304 | `internal/web/static/js/topbar.js:105` |
| `drawBoard` (JS) | 301 | `internal/web/static/js/render.js:411` |
| `Manager.applyAndRecordAs` | 292 | `internal/session/moves.go:236` |
| `refreshOptions` (JS) | 262 | `internal/web/static/js/game.js:5018` |
| `mountDirectMessages` (JS) | 262 | `internal/web/static/js/dm.js:307` |
| `buildGameInfo` | 261 | `internal/session/info.go:274` |
| `app.New` | 213 | `internal/app/app.go:156` |
| `compileWinRules` | 204 | `internal/engine/catalog/winrules.go:7` |
| `state.ParseMove` | 199 | `internal/engine/state/notation.go:176` |

Functions over 50 lines per area (code): `internal/session` 45, `internal/engine/state` 29,
`internal/games/courtfall` 16, `internal/engine/catalog` 11, `internal/mcpserver` 9 (1,302 lines, more
than half the package), `internal/api` 8, `internal/web/static/js` 84.

### Churn beside size

Lines times commits in the last 30 days, divided by 100, code only:

| file | lines | commits 30d | score |
|---|---|---|---|
| `internal/web/static/js/game.js` | 6,257 | 83 | 5,193 |
| `internal/web/static/js/home.js` | 1,706 | 35 | 597 |
| `internal/app/app.go` | 747 | 32 | 239 |
| `internal/web/static/js/render.js` | 1,197 | 20 | 239 |
| `internal/auth/handlers.go` | 1,218 | 17 | 207 |
| `internal/config/config.go` | 1,280 | 16 | 204 |
| `internal/engine/dsl/dsl.go` | 842 | 24 | 202 |
| `internal/web/static/js/topbar.js` | 738 | 24 | 177 |
| `internal/api/games.go` | 746 | 23 | 171 |
| `internal/session/info.go` | 769 | 22 | 169 |
| `internal/mcpserver/tools_v2.go` | 989 | 16 | 158 |
| `internal/web/static/js/api.js` | 337 | 46 | 155 |

Per area, file-commits in 30 days: the web client 486, `internal/session` 332, `internal/api` 178,
`internal/store` 120, `internal/engine/state` 98, `internal/auth` 83. The game packages are quiet:
6 to 24 each. `internal/version/version.go` (214) is the version bump, not a finding.

### Coupling

Fan-in is the module's packages that import a package (tests and scripts included), fan-out the
module's packages it imports:

| package | fan-in | fan-out | transitive internal deps |
|---|---|---|---|
| `internal/engine/catalog` | 31 | 5 | 5 |
| `internal/engine/dsl` | 20 | 0 | 0 |
| `internal/engine/state` | 19 | 6 | 7 |
| `internal/engine/board` | 17 | 0 | 0 |
| `internal/web/look` | 11 | 3 | 6 |
| `internal/session` | 5 | 20 | 28 |
| `internal/api` | 2 | 19 | 37 |
| `internal/app` | 2 | 26 | 59 |

The engine is the stable core with nothing above it, as it should be. `internal/app` is the
composition root. `internal/session` imports 20 of the module's packages, from `auth`, `mail` and
`translate` to `tournament`, `rating` and `badges`.

Two packages exist to avoid an import cycle: `internal/engine/callback` and
`internal/engine/primitives` (their comments at `callback.go:9` and `primitives.go:8`), so that
`catalog` can call into `state` without importing it; `dsl/parse.go:28` records the same constraint.
They are small (35 and 66 code lines) and do their job; the audit leaves them.

The JavaScript: `util.js` is imported by 51 of the 66 modules, `i18n.js` by 45, `api.js` by 32,
`nav.js` by 12, `paint.js` by 10. Six modules call `fetch(` themselves rather than through `api.js`:
`api.js` (2), `health.js` (2), `errors.js` (1, a keepalive report), `i18n.js` (1, the catalog),
`nav.js` (1, a page's HTML) and `notify.js` (1, the unsubscribe page). Each has a reason (they run
before `api.js` could, or fetch something that is not the API), so there is no seam broken here.

### Unused names

- **Unexported Go names used nowhere**: 8, all constants in `internal/games/solitaire/data.go`
  (`modeKlondike`, `modeSpider`, `modeFreeCell` at 13–15, `reasonCleared`, `reasonStuck` at 21–22,
  `zoneStock`, `zoneWaste`, `zoneDone` at 31–33): left from the three games converted to declarations.
  golangci-lint's `unused` does not report them because it treats a `const` group with one used member
  as used.
- **Exported Go names used nowhere at all**: 15, among them `app.WithArgon2` (`app.go:637`),
  `batch.LiftOf` and `batch.Wilson` (`report.go:265`, `:290`), `batch.ErrQueueFull`, `batch.ErrNotFound`,
  `store.CountSessions`, `store.CapsSeeded`, `store.RenamePlayer` (`lobby.go:273`),
  `session.ErrNoReactions` (`reactions.go:70`), `cards.Zone.FlipAll`, `state.SelectionCaptures`,
  `journal.CanonicalLabels`, `metrics.SlotEnvBatch`, `translate.ErrQueueFull`.
- **Exported Go names no other package uses**: 435 used only inside their own package, and 210 used
  outside it only by tests. The first group is mostly in `internal/auth` (114) and `internal/session`
  (78); they are candidates for unexporting at best, and not worth a pull request of their own.
- **JavaScript exports no other file names**: 39; 11 used nowhere at all, among them `setupZoom`
  (100 lines, `render.js:1098`), `addPieceShapes` (`render.js:32`), `headingsOf` (`markdown.js:47`),
  `chatLoaded` (`chat.js:186`), `offerTour` (`welcome-tour.js:36`), `cardOpen`, `peopleIcon`,
  `myHereAddress`, `suitIcon`, `setSuitArt`; 26 used only inside their own module (the `export` could
  go). Page scripts can import from `/static/js`, and the overlay's page scripts are not here, so
  `addPieceShapes` and `setupZoom` in particular may be the overlay's.

### Duplication

Go functions of eight lines or more: 3 groups identical, 41 groups of the same shape. The ones that
matter:

| shape | lines each | where |
|---|---|---|
| `dataOf`: the position's payload as the game's `Data`, from three forms | 18 | chess `data.go:122`, courtfall `data.go:242`, fendo `data.go:48`, fist `data.go:103`, mycelium `data.go:101` |
| `Ruleset.Redact` (identical) | 9 | fendo `view.go:44`, meridians `view.go:45`, mycelium `view.go:44` |
| `Ruleset.Outcome` (identical) | 12 | chess `rules.go:892`, fist `rules.go:416` |
| `Module` (registration) | 11 | chess `module.go:34`, meridians `module.go:26`, mycelium `module.go:33` |
| `geoOf` (identical) | 8 | meridians `geometry.go:29`, mycelium `geometry.go:30` |
| seat handlers | 20 | `api/lobby.go:78`, `:152`, `:175` (choose, release, remove a seat) |
| a store update by game id | 12 | `store/game.go:269`, `:282`, `:477`, `store/lobby.go:233` |
| a limiter of recent times | 19 | `session/chat.go:64` `chatLimiter.allow`, `session/voice.go:144` `voiceTracker.allow` |
| folding samples | 36 | `journal/compact.go:75`, `:112` |

Runs of six lines or more repeated across files, per area (each pair counted from both sides):

| area | pairs of files | lines | the largest pairs |
|---|---|---|---|
| games (Go) | 103 | 1,182 | fendo/mycelium `view.go` 40 lines; fendo/fist `view.go` 39; meridians/mycelium `geometry.go` 35 (a run of 21) |
| `api` and `mcpserver` | 12 | 154 | `api/extra.go:401` and `mcpserver/tools_v2.go:414`, 42 lines (a run of 21) |
| definitions (YAML) | 48 | 348 | the draughts fixture and its pack, 20; `gomoku.yaml`/`hex.yaml` 15; `go.yaml`/`pente.yaml` 10 |
| web client | 3 | 26 | `dm.js:286`/`voice.js` 10; `chat.js:286`/`global-chat.js` 9 |
| browser tests | 138 | 1,321 | `rest_join_test.go`/`rest_make_move_test.go` 38; `mcp_join_state_test.go`/`rest_join_test.go` 26 |

The front-end JavaScript repeats itself least of all the areas measured.

### Comments

| | code lines | comment lines | of them doc | other comment lines per 100 code lines |
|---|---|---|---|---|
| Go, code | 66,971 | 12,737 | 7,391 | 8.0 |
| JavaScript | 19,817 | 2,889 | 1,317 | 7.9 |

Densest areas (other lines per 100 code lines): `internal/web` 26.4, `internal/api` 15.6, `internal/app`
15.3, `internal/httpserve` 13.0, `internal/engine/wasmrules` 10.9, `internal/session` 9.1. Densest files
of 50 code lines or more: `courtfall/v1.0.0/arena.go` 56.1, `session/movelog.go` 48.0,
`session/paranoid.go` 47.9, `api/clienterror.go` 44.1, `store/store.go` 39.2,
`catalog/pierule.go` 38.9, `state/turn.go` 30.8, `api/ws.go` 29.1, `js/errors.js` 29.4.

**A hand-read sample** of the non-doc comments of twelve of the densest files (the nine above, with
`web/router.go`, `session/botturns.go` and `session/clock.go`: 132 comment blocks, about 590 lines),
each block sorted by what most of it does:

| class | blocks | share | examples |
|---|---|---|---|
| says intent the code does not | 93 | 70% | why the DSN carries the pragmas (`store.go:46`); why a clock's start is kept to the nanosecond (`clock.go:18`); why the endpoint answers 204 whatever happens (`clienterror.go:126`) |
| restates the code | 29 | 22% | `// marshal is the clock as it is stored.` (`clock.go:104`); `// clientErrorPage keeps the page label a closed set.` (`clienterror.go:183`); the swap's ownership said three times in `pierule.go` (9–22, 49–54, 79–80) |
| history or narrative for an ADR or a commit | 8 | 6% | the file headers of `movelog.go` (14–28), `paranoid.go` (14–26), `pierule.go` (9–22) and `errors.js` (1–12: "the UI is where most of bingle's recent work went") |
| wrong | 2 | 2% | see *Bugs met on the way* |

By lines the narrative weighs more than by blocks, since it comes in long headers: about 70 of the 590
lines. The sample is of the densest files, so it overstates the rest; what it supports is that in such a
file roughly a quarter of the non-doc comment lines could go or move, not that a quarter of all comments
could. Separately, 51 comment lines in 24 code files still name the two games now in the private
overlay (for example `movelog.go:108–110`, and the example `"Alice · …"` at `api/games.go:66`).

### Tests, from CI

120 runs, 2026-11-24 to 2026-11-28. Successful jobs:

| job | runs | median | max |
|---|---|---|---|
| browser (one of four shards) | 476 | 318 s | 556 s |
| unit | 124 | 160 s | 269 s |
| container | 129 | 56 s | 69 s |
| lint | 128 | 51 s | 78 s |
| sims (short on pull requests) | 129 | 25 s | 75 s |

The wall time of a run is the slowest browser shard, about six to nine minutes. In the unit job, by
median: `tests/apiplay` 111 s, `tests/bots` 40 s, `tests/session` 39 s (130 s at most), `tests/allgames`
24 s, `tests/wasmrules` 24 s, `tests/difftest` 20 s, `tests/store` 20 s. The packages run in parallel,
so `tests/apiplay` is the unit job's long pole.

**13 of the 60 runs on `main` failed** (22%), each on commits that had passed on their pull request,
so most likely flakes. The failures, by shape (one pull request's own failure of every shard, on
`spell-color`, is left out):

| shape | tests | runs | state |
|---|---|---|---|
| a tutorial test (#382) | `TestWebTutorial_*` (7 tests), `TestWebFitScreen_TheTutorialIsBesideTheBoard` | 7 on main, 6 on PRs | the shape #388 fixed |
| work still running when the store closes: `TempDir RemoveAll … directory not empty`, `database is closed` | `TestAGuestAtASeatCountsForNobody`, `TestGameClock_ARestartCostsNobodyTime` | 1 main, 1 PR | fixed in #390 |
| a port taken before the shutdown test dials it | `TestServe_ShutsDownWithASilentConnectionInFlight` | 1 PR | fixed in #391 |
| Chrome not started within the test's own deadline: `could not dial …/devtools/browser`, `context deadline exceeded` | `TestAccountUI_TheAdminPage` (`account_ui_test.go:177`), `TestAccountUI_SignInWithAProvider` (`:140`) | 1 main, 1 PR | **open**: see finding 8 |
| a wait that timed out on a page that did not get there | `TestWebGame_AStalePageCatchesUp`, `TestTournaments_UI`, `TestWebGlobalChat_TheTabSaysHowManyAreUnread`, `TestWebFitScreen_TheTurnBringsTheActionsIntoView`, `TestAccountUI_AGuestKeyLinkResumesTheGames`, `TestWebCourtfall_PlayThroughSetupIntoTheGame` | 5 main, 1 PR | open, once each |

The raw `fetch` calls of the browser tests (#389) did not show as a failure in this window.

## Findings, ranked by risk times churn

Risk is how likely a change in the place goes wrong or is slowed; churn is how often one comes. The
order is the product, judged from the numbers above.

### 1. `game.js` is a quarter of the client and a third of the pull requests

**Evidence.** 6,257 lines (5,239 on 2026-10-28: it grew a fifth in a month), 83 of the 268 commits of
the last 30 days, 308 top-level functions, 22 functions over 50 lines (`refreshOptions` 262 at 5018,
`moveBar` 180 at 4382, `onEvent` 125 at 2137, `seatStrip` 123 at 3513). Its state is one object,
`S` (line 40), and 22 module-level `let`s (105–111, 564–565, 3638–3674, 3984–4051, 4299,
4596, 4930). It already marks its seams with ruled headings: Boot (114), the pending move (392),
derived state (535), ruleset games (841), actions (936), analysis (1403), history (1983), live updates
(2067), notifications (2264), the stack inspector (2285), rendering (2341–3652, 1,311 lines), the
reactions dock (3653), the Auto-Play Timeout and everything after it (3977–5872, 1,895 lines),
keyboard (5873), tutorials (5918–6257).

**What it costs.** Every change to the game page is a change to this file, so parallel pull requests
collide in it, and a reader looking for the seat strip has six thousand lines to search. #235 (a
module for a prototype's version) waits on it.

**Smallest change.** Move one section at a time into its own module, and the shared `S` into a
`game-state.js` that the others import. A module-level `let` another section assigns must move into
`S` first, since an imported binding cannot be assigned. Start with the sections that read state
but are not called back into: analysis, tutorials, the reactions dock, keyboard.

**What could go wrong.** Import cycles between sections (ES modules allow them, but a module used
before it has run fails at load); a section that relies on being evaluated after another. Each split
must load the page in every browser test it touches.

**Size.** Large in all; six to eight medium pull requests. ADR 0013 (no build step) stands, and nothing
here needs one.

### 2. `internal/session` is several packages in one

**Evidence.** 16,433 lines in 59 files, 332 file-commits in 30 days, fan-out 20. `Manager` has 40
fields and **349 methods**. Which members each file uses (`grep m\.` per file):

- `here.go`, `dm.go`, `voice.go` (with the call registry): in-memory only; they use `m.here`, `m.dms`,
  `m.calls`, `m.voice`, `m.hub` and `m.now`, never the room lock, and `voice.go` the store twice.
- `tournaments.go` and `schedule.go` (2,218 lines): `m.store` 109 times, `m.lockRoom` 8, `publishState`
  9, and their own mutex.
- `notify.go` and `translate.go` (1,186 lines): `m.store`, `m.notify` or `m.translation`, `m.detach`.
- `clock.go`, `connection.go`, `realtime.go`, `takeover.go`: the room's own business, tied to moves.

**What it costs.** One namespace of 349 methods; a reader cannot tell from a name whether it takes the
room lock, and the 20 imports make every change to session rebuild most of the server.

**Smallest change, in two shapes.** (a) New packages under `internal/session/`: `presence` (who is
here, direct messages, calls, voice) behind a small interface to publish an event, and
`tournaments` (tournaments and schedules) behind one to create and start a game. (b) Keep one package,
and only cut files and long functions. **Recommended: (a) for presence**, whose seam is already clean
and whose state is in memory, and **(b) for tournaments** for now: they lock rooms and publish state, so
an interface for them would be wide, and their churn is low (5 commits in 30 days). The clocks stay:
the plan's guess that they were separable is refuted, since they are part of applying a move.

**What could go wrong.** Lock order: presence must never be called with a room lock held if it ever
takes one. The capability checks (ADR 0071) and the account rules of ADR 0073 must move with the code
they guard.

**Size.** Medium for presence; large for tournaments, and not recommended yet.

### 3. The move funnel and the create path are each one long function

**Evidence.** `Manager.applyAndRecordAs` 292 lines (`moves.go:236`; moves.go 18 commits in 30 days),
`Manager.CreateGameWithOptions` 307 (`create.go:261`; 18), `buildGameInfo` 261 (`info.go:274`; 22).

**What it costs.** Every rules feature with a session side passes through these; a change reads three
hundred lines to find its place, and review cannot see the order of what runs under the lock.

**Smallest change.** Extract the named steps (validate, apply, record, notify, publish) as methods in
the same file, in the same order, with no change of lock or transaction. tests/session, tests/app and
tests/apiplay cover them.

**What could go wrong.** A step moved across the transaction's commit or out from under the room lock.
The extraction must leave every `defer` and the unlock where it was.

**Size.** Medium, one pull request each.

### 4. REST and MCP say some things twice, and one of them differently

**Evidence.** MCP's tools call `session.Manager` directly, as REST's handlers do, with no HTTP in
between, so it is not a parallel implementation of the game logic. The
repetition is at the edges:

- the observation: `observationResponse` (`api/extra.go:401`) and `observationResult`
  (`mcpserver/tools_v2.go:414`) are the same 21 fields with the same tags, each with its own converter.
  `internal/wire` exists to hold exactly such shapes ("so a move or a game definition looks the same to
  every client and is defined once").
- names: REST checks a player name with `checkName` (`api/errors.go:85`) at 29 call sites; MCP checks
  only that it is not blank (`mcpserver/tools.go:157`, `:282`, `:407`). See *Bugs met on the way*.
- the route of each tool: `mcpserver/guard.go:40` maps each tool to a REST route (#378), a third list
  beside the tools and the router.
- MCP's tool declarations are the longest Go functions: `registerEnvTools` 321 lines,
  `registerPlayTools` 305, `registerResourcesAndPrompts` 165, `registerLobbyTools` 151.

**Smallest change.** Move input rules that are the game's (a name's length and characters) into
`session`, where both doors get them; put the observation into `wire`; split the `register…`
functions one tool to a function. Projecting MCP through the REST handlers (the plan's guess) is
refuted: it would add an HTTP round trip inside the process for nothing ADR 0011 asks for.

**What could go wrong.** A REST error message clients match on changes wording when it moves into
session (`api: player_name is required`); keep the REST text where it is mapped. The API's schema
(ADR 0134) must not change: regenerate `docs/api.schema.json` and expect no difference.

**Size.** Small for each of the three.

### 5. The engine's long functions

**Evidence.** `state.ParseMove` 199 lines, `state.Apply` 170, `state.buildActionSpace` 173,
`rctx.validateSlide` 155, `state.RenderText` 143; `catalog.compileWinRules` 204, `compilePatience` 174,
`compileCards` 164. `internal/engine/state` has 29 functions over 50 lines and 98 file-commits in 30
days.

**What it costs.** These are where the DSL grows, and each addition makes a long function longer.

**Smallest change.** Split by case: `compileWinRules` is a switch on the rule's type and splits into a
function per type; `ParseMove` per notation form.

**What could go wrong.** The private overlay's games run through these functions; their signatures stay,
but behaviour is what matters, so these need the overlay build. tests/allgames, tests/difftest and the
fuzz targets cover the public games.

**Size.** Medium, one function per pull request.

### 6. The composition root and the configuration

**Evidence.** `app.New` 213 lines (`app.go:156`; app.go 32 commits in 30 days, the most of any Go
file), `config.Load` 178 (`config.go:447`; config.go 1,280 lines, 16 commits), `main` 177
(`main.go:55`), `api.NewRouter` 184 (`router.go:29`; 34 commits), `metrics.New` 186.

**What it costs.** Nearly every feature adds a line to each, so they collide between parallel pull
requests more than their size suggests.

**Smallest change.** `app.New` and `config.Load` as a sequence of named steps by area (store, sessions,
mail, TLS, …); `NewRouter` by group of routes, one function per group, in the order they are mounted
today.

**What could go wrong.** Route order in chi matters for the fallbacks; start-up order matters for what
reads the configuration. Behaviour-preserving if the order is kept.

**Size.** Small to medium each.

### 7. Comments: a quarter of the densest files' comments could go

**Evidence.** The sample above: 22% of the blocks restate the code, 6% are history; two are wrong; 51
lines name games no longer here.

**What it costs.** A reader reads two lines to learn one, and a restating comment is the one that goes
stale (both wrong comments found restate a parameter or a field).

**Smallest change.** One pull request per area, starting with the densest files listed above: cut what
restates, move what is history to the ADR it cites (most already do say it), correct what is wrong,
and drop the names of games now in the overlay. Leave doc comments on exported names and on struct
fields alone: schemagen publishes them.

**What could go wrong.** Little; a reviewer disagreeing about what is obvious. Keep each pull request
to one area so the review stays short.

**Size.** Small, several.

### 8. The browser suite: one open flake shape, and per-test times nobody can see

**Evidence.** Two failures in the window were Chrome failing to start inside the test's own deadline
(`account_ui_test.go:140`, `:177`): `newChromedpContext` (`browser_test.go:357`) wraps the browser's
start in the same `context.WithTimeout` as the test, so a slow start on a busy runner spends the test's
whole budget (`WSURLReadTimeout` is 90 s, but the test's 40 to 60 s come first). Six other tests timed
out once each waiting for the page, with no shared cause visible in the logs. The suite has 93
`time.Sleep` calls in 41 files of `tests/app` and 679 one-shot `eval…` reads beside 944 `waitJS`; the
known flake shapes (a one-shot read right after something the page does asynchronously) are of that
kind. CI runs `go test` without `-v` or `-json`, so the slowest tests cannot be found from its logs.

**What it costs.** 22% of the runs on `main` failed in five days; each costs a rerun and a look. Most
were the shapes fixed today, but the two open ones remain.

**Smallest change.** Start Chrome under a deadline of its own before the test's timeout begins (one
helper, one pull request). Add `-json` to the browser and unit jobs' `go test` and keep the output as an
artifact, so the next audit can rank tests by time. Then, by shape rather than by test, replace one-shot
reads that follow an action with waits.

**What could go wrong.** A longer start deadline makes a really broken Chrome fail later; that is fine.

**Size.** Small, small, then medium.

### 9. A shared kit for the game packages: a few helpers, not a framework

**Evidence.** The games are 11,365 lines of Go, quiet (6 to 24 file-commits each in 30 days), and the
repetition is small and exact: `dataOf` five times (18 lines), `Redact` three times identical (9),
`Outcome` twice identical (12), `Module` three times (11), `geoOf` twice. The view files repeat 25 to 40
lines pairwise.

**What it costs.** Little today: a fix to the shape of `dataOf` is five edits.

**Smallest change.** A generic helper in `internal/engine/state` for reading a ruleset's payload
(`dataOf`), and one for the common `Redact`. The rest stays per game, where each game's words belong.

**What could go wrong.** The overlay's games may have their own copies; an added helper breaks nothing,
and they can adopt it later.

**Size.** Small. Low value; do it when a game package is touched for another reason.

### 10. Dead code

**Evidence.** The eight solitaire constants; 15 exported Go names used nowhere; 11 JavaScript exports
used nowhere (`setupZoom` alone is 100 lines).

**Smallest change.** Delete the solitaire constants now (unexported, so safe). For the exported Go names
and the JavaScript exports, check them against the overlay's build and page scripts first, then delete
what nothing uses.

**Size.** Small.

### 11. Repeated blocks in the definitions: refuted

**Evidence.** The definitions are 1,668 lines (and 394 of web files). The largest repetition is the
two-seat stone game's seats, piece, turn order and `place` action, 9 to 15 lines in `gomoku.yaml`,
`hex.yaml`, `pente.yaml`, `go.yaml`, `havannah.yaml` and `yavalath.yaml`. ADR 0075 lets `!include` reach
only a game's own files, so sharing it would mean a file the six games include and a reader has to open
to read any of them.

**Verdict.** Not worth it: each definition reads whole on its own, which is worth more than the 60 lines.
The draughts fixture and its pack repeat 20 lines on purpose (the pack must stand alone, ADR 0147).

## The six candidates

| candidate | verdict |
|---|---|
| `internal/session` as several packages | **Confirmed in part**: presence (here, messages, calls, voice) is a clean split; tournaments and schedules are separable but wide; clocks are not separable (finding 2). |
| `game.js` split along its seams | **Confirmed**: the seams are already marked in the file, and no build step is needed (finding 1). |
| a shared kit for the game packages | **Confirmed small**: five helpers' worth of repetition in quiet packages (finding 9). |
| MCP as a thin projection of the API | **Refuted as stated**, confirmed in part: MCP already sits on the session layer; the repetition is in shapes and input checks, which belong in `wire` and `session` (finding 4). |
| repeated blocks in the definitions | **Refuted**: 60 lines across six files, against ADR 0075's own-files rule (finding 11). |
| the browser suite | **Confirmed**: one open flake shape with a one-helper fix, and no per-test timing to find what is slow (finding 8). |

## Proposed order of pull requests

Each is behaviour-preserving, green on its own, and touches neither the changelog nor a version (so
any two can run side by side unless the list says otherwise). *Overlay* marks the ones that touch what
the private overlay builds against, which need the coordinator's overlay build before merging.

1. **Delete the solitaire constants and correct the wrong comments** (*Bugs met on the way*). Sonnet,
   low. Touches `internal/games/solitaire/data.go`, `session/paranoid.go`, `session/clock.go`,
   `store/store.go`. Not: anything exported. Verified by `make lint` and tests/solitaire.
2. **Chrome starts under its own deadline in the browser tests.** Sonnet, medium. Touches
   `tests/app/browser_test.go` only. Not: any test's own timeout. Verified by the browser shards, twice.
3. **`-json` output of `go test` kept as a CI artifact.** Sonnet, low. Touches `.github/workflows/ci.yml`.
   Not: what runs. Verified by one run's artifact. Labelled `no-changelog`.
4. **A player's name is checked in `session`, for MCP too** (the bug below: its own fix with its own
   test, so it does take a changelog line). Sonnet, medium. Touches `internal/session` (create, join),
   `internal/api/errors.go`, `internal/mcpserver/tools.go`. Not: REST's error texts. Verified by a new
   MCP test that a 41-character name is refused, and tests/app's REST tests.
5. **The observation in `wire`.** Sonnet, low. Touches `internal/wire`, `api/extra.go`,
   `mcpserver/tools_v2.go`. Not: a field or tag. Verified by an unchanged `docs/api.schema.json`. After 4
   (both touch `mcpserver`).
6. **MCP's tools one to a function.** Sonnet, medium. Touches `internal/mcpserver` only. After 5.
   Verified by tests/app's MCP tests and the guard's test (every tool still has a route).
7. **`game.js`, first split: `game-state.js` and the analysis module.** Opus, high. Touches
   `internal/web/static/js/game.js` and new modules beside it. Not: any other module, the templates, the
   CSS. Verified by the whole browser suite (all four shards). Nothing else in `game.js` meanwhile.
8. **`game.js`, the next sections one pull request each**: tutorials, the reactions dock, keyboard, the
   Auto-Play Timeout and options, rendering last. Opus, high for rendering, Sonnet, high for the rest.
   Strictly one after another, after 7. #235 can follow.
9. **`internal/session/presence`.** Opus, high. Touches `session/here.go`, `dm.go`, `voice.go`, the
   `Manager` fields they own, and their callers in `internal/api`. Not: the room lock, the hub's API.
   Verified by tests/session and the browser tests of messages and voice. Side by side with 7 and 8;
   not with 10 or 11.
10. **The move funnel and the create path in named steps** (`applyAndRecordAs`, then
    `CreateGameWithOptions`, then `buildGameInfo`, one pull request each). Opus, high. Touches
    `internal/session` only. *Overlay* (its games' moves run through it). After 9.
11. **`app.New`, `config.Load`, `NewRouter` in named steps.** Sonnet, medium, one each. Touches
    `internal/app`, `internal/config`, `internal/api/router.go`. Collides with any feature pull request
    that wires something; do each on a quiet day.
12. **The engine's long functions**, one per pull request, `compileWinRules` first. Opus, high.
    *Overlay*. Side by side with the web client's.
13. **Comments, one area at a time**: `internal/session`, `internal/api`, `internal/web`, the client.
    Sonnet, medium. After the splits of the same area, so a comment is not cut and then moved. Labelled
    `no-changelog`.
14. **Unused exports**, after the overlay's build and page scripts are checked for them. Sonnet, low.
    *Overlay*.
15. **The game helpers** (`dataOf`, `Redact`). Sonnet, medium. *Overlay* (additive). Whenever.

**Not to do.** Moving `internal/engine/callback` and `primitives` (they are the price of no cycle, and
the overlay builds against the engine); sharing blocks between definitions (finding 11); splitting
`internal/session/tournaments.go` into a package now (finding 2); and unexporting the 435 names used
only in their own package, which is churn across the overlay's interface for nothing a reader gains.

## Bugs met on the way

- **MCP takes a player name REST refuses.** `checkName` (`internal/api/errors.go:85`) refuses a name
  over 40 characters or with a control character, but MCP's `create_game` and the other tools that take
  a `player_name` check only that it is not blank (`internal/mcpserver/tools.go:157`, `:282`, `:407`),
  and `session` does not check either (`mayUseName` checks reserved handles only). Found by reading;
  not run.
- **Three comments say something the code does not.** `internal/session/paranoid.go:34–37` describes a
  parameter `after` that is `pos`; `internal/session/clock.go:44–46` says the running clock started at
  the row's `updated_at`, where the file's header and `startedAt` take the later of that and `Since`;
  `internal/store/store.go:36` describes `closers` above `closeMu`.

## What could not be measured, and how far to trust the rest

- **The slowest tests.** CI runs `go test` without `-v` or `-json`, so its logs give times per package
  and per failed test only (pull request 3 would fix this). Not measured locally, by agreement.
- **Flakiness rates.** Five days of runs show which tests failed once, not how often they fail; a test
  seen once may be 1 in 10 or 1 in 200.
- **The overlay's uses.** Every unused-export number, and the overlay marks, are by what this tree can
  see.
- **JavaScript function lengths** depend on the script's lexer; brackets balanced in every file and the
  longest functions were checked by eye, but a regular expression after `)` or `}` would be misread
  (none was found). Methods of object literals count as functions; anonymous callbacks are listed as
  `(anonymous)`.
- **Dead code** counts names, not types: it can hide a dead function behind a local variable of the
  same name, and it judges methods by name across the whole tree.
- **Duplication** finds copies within a few tokens of each other, not two functions that do the same
  thing in different words; the REST and MCP findings beyond the observation were found by reading.
- **The comment sample** is twelve files chosen for being the densest: its shares hold for such files,
  and say nothing exact about the rest.
