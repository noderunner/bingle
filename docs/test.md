# Testing bingle

## Prerequisites

- Go (matching `go.mod`)
- `golangci-lint`, for `make lint` (optional locally — CI always runs it; `make lint` skips it gracefully if it's not installed and just does the `gofmt` check)
- A Chrome or Chromium binary on `$PATH` (as `google-chrome`, `chromium`, or similar), or `BINGLE_BROWSER=/path/to/it`, for the browser-driven web UI tests. Without one those tests are skipped and the rest still runs; `make test-short` (or `go test -short ./...`) skips them on purpose, with the slowest simulations, and takes under a minute

## Running the tests

```sh
make test
```

This runs `go test ./...` — the entire suite, across every package, in one command. It's the single thing to run before opening a PR.

```sh
make lint
```

Runs `gofmt -l .` (fails if anything's unformatted — fix with `make fmt`) and `golangci-lint run` if it's installed.

## What's actually being tested

The suite has these kinds of tests, all driven by the one `go test ./...` command:

- **Unit tests** — small, isolated packages (`internal/engine/...`, `internal/session/roomcode`, etc.), no I/O.
- **Integration tests** — most of `tests/*`. These build a real `*app.App` (with a real, temp-file-backed SQLite database) and drive it in-process via `httptest`, exercising the real REST, WebSocket, and MCP handlers without binding an actual network port. This is the bulk of the suite.
- **Browser-driven tests** — `tests/app/web_*.go`. These use [`chromedp`](https://github.com/chromedp/chromedp) to drive a real headless Chrome instance against the running app, so they verify what actually renders and updates in the DOM (SVG board rendering, live WebSocket updates across two tabs, etc.), not just what the server returns.
- **Rules/engine tests** — `tests/tak` and `tests/classics` drive the engine directly: rule-by-rule Tak tests built from TPS positions, a perft check against the published Tak node counts, and randomized consistency tests that cross-check `LegalSelections` against `Apply`, mutated moves, notation and TPS round trips and piece conservation.
- **Engine hardening** — `tests/tak/fuzz_test.go` holds native Go fuzz targets (move and TPS parsers, structured-move application, the definition compiler: none may panic); run one with `go test ./tests/tak -run xxx -fuzz FuzzParseMove -fuzztime 30s`. `tests/allgames` adds `FuzzApplyAnyMoveToAnyGame`: arbitrary structured moves (including `aim`) against a mid-game position of every game, asserting that anything `Apply` accepts is one `LegalSelections` lists. `tests/board/limits_test.go` checks the board-size cap. `tests/classics/amazons_test.go` brute-forces every from/to/aim triple against the slide enumerator, and `tests/tak/completeness_test.go` does the same for stack moves.
- **Cross-game invariants** — `tests/allgames` runs one battery over every shipped game and variant (every enumerated move applies and round-trips through notation to the same position, the position document and planes keep their shape, action IDs exist, the start is playable); a new game JSON is covered the moment it is registered.
- **Two implementations of one game** — `tests/difftest` plays a reference and a candidate side by side from the same seeds, and reports the first selection where their legal selections, positions, notation, results or the end of an impulse (ADR 0114) differ, with the seed and the moves that led there. It is what a game converted from a Go ruleset to a declaration is checked against (see [dsl.md](dsl.md#converting-a-go-ruleset)); its own tests play every shipped game against a second compiled copy of itself, which also holds every ruleset to being deterministic for a seed.
- **API-only play** — `tests/apiplay` plays every game in `GET /game-defs` to a finish over HTTP, with no browser and no knowledge of any game: random moves from `GET /moves/legal`, played by notation and by their fields in turn. A new game is covered the moment it is registered. Patience games, which random play need not win, play 500 moves and then resign. Every preset in a game's setup catalog plays out as well, and the game must really be the one the options asked for. `-short` plays one game of each family and no presets.
- **Training support** — `tests/envs` (planes, fixed action IDs, batch stepping, recorded matches) and `tests/puzzles` (solver correctness and a re-proof of every shipped puzzle; `go run scripts/genpuzzles` regenerates them).
- **Smoke test** — `scripts/smoke-test.sh`, or `make smoke`. `make test` runs it after the Go
  suites, because it is the only check that exercises `main.go` and so the only one that would
  notice a start-up rule the Go tests never reach (the secure-connection rule of ADR 0065 broke it
  without a single Go test failing). Builds the real binary, runs it as a real OS process bound to a real port, and curls `/healthz`. This is the only check that exercises `main.go` and `internal/httpserve`'s actual `net.Listen` path end to end; everything else in `internal/app` is deliberately built to never bind a real listener itself, so it can be tested in-process (see `docs/history/plan.md` section 6.7).

### The fixture games

`tests/testdata` holds games only the tests play: the server never registers them, and
`testdata.Fixtures()` adds them to a test's server. They are there so that every shared feature
is tested through a public game, now that the commercial games are built apart (ADR 0127). Each
has its own tests (`tests/council`, `tests/tricks`, `tests/table`), so the tests that lean on it
lean on something sound.

- **Council** (`test-council`, a Go ruleset): a table of four, reactions, a choice of house per
  seat with a pool and a derived game per table, a setup catalog, tutorials, a map of areas, a
  page script and a stylesheet, and a practice study.
- **Tricks** (`test-tricks`, `-3`, `-5`; a Go ruleset): tables of three to five, hands only their
  owner sees, a board of choices crossed off in turn, tricks, and a plain deck that can wear a set
  of cards. Its page script draws the board's pictures and its players (`choiceIcon`,
  `seatIcon`).
- **Table** (`test-table`, a declaration): tiles laid from a hand onto a free board that grows,
  and the ring and marker shapes.
- **Freeplay**, **Tic-Tac-Toe** and **Connect Four**: the smallest games, kept for the tests that
  play one to the end.

### Courtfall

`tests/courtfall` pins the Go ruleset: the draft and its constraints (every
random draft places all ten tiles), turn structure, movement against an
independent reference implementation on random boards, exchanges with
fixed dice (`courtfall.RollDice` finds a generator state that rolls what a
scenario needs), shots and shoves, pushes and chains, every Court, secrecy
(public/own views, move-list masking, determinization) and a random-play
contract check of `Legal`/`Apply`/notation/planes/action space. REST-level
secrecy and the undo guard are in `tests/app/courtfall_rest_test.go`.

## Running a subset

Standard `go test` flags all work:

```sh
go test ./tests/session/...              # just one package
go test ./... -run TestMakeMove           # by test name, across all packages
go test ./tests/session/... -race -v      # race detector + verbose output
```

The concurrency-sensitive tests (`tests/session/make_move_concurrency_test.go`, `tests/app/ws_disconnect_test.go`) are worth running with `-race` after touching `internal/session/manager.go` or `internal/session/hub.go`.

## Browser tests

The browser-driven tests need a real Chrome/Chromium binary. `chromedp`'s default allocator looks for common names (`chromium`, `google-chrome`, etc.) on `$PATH`.

If you don't have one of those directly on `$PATH` — for example, if your Chromium is installed as a Flatpak (`org.chromium.Chromium`) rather than a native package, as on some immutable-OS setups — a small wrapper script makes it discoverable:

```sh
mkdir -p ~/.local/bin
cat > ~/.local/bin/chromium <<'EOF'
#!/bin/bash
exec flatpak run org.chromium.Chromium "$@"
EOF
chmod +x ~/.local/bin/chromium
```

Make sure `~/.local/bin` is on your `$PATH`. This is local machine setup only — it's not part of the repo, and CI runners have a real Chrome/Chromium on `PATH` already with no wrapper needed.

The shared helper `newChromedpContext` (`tests/app/web_create_join_test.go`) gives every browser its own uniquely named profile directory and, when a test ends, kills any leftover process holding it and deletes the directory — so the quirk below no longer accumulates across runs on its own.

### Looking at the UI

`scripts/shot/main.go` (build-tagged `ignore`, run with `go run`) drives a running server with headless Chrome and saves screenshots of the lobby and a mid-game Tak position (desktop, mobile, dark, history, mid-move):

```sh
BINGLE_ADDR=127.0.0.1:18099 BINGLE_DB_PATH=/tmp/shot.db ./bin/bingle &
go run scripts/shot/main.go http://127.0.0.1:18099 /tmp/shots
```

### Known quirk: orphaned Chrome processes filling `/tmp`

On at least one Flatpak-Chromium setup we hit during development, killing the browser at the end of a test (via context cancellation) didn't reliably terminate the real Chrome process tree underneath the `flatpak run`/`bwrap` sandbox layer — the kill signal doesn't always propagate through that extra layer of indirection. Left unchecked, this silently accumulates orphaned headless-Chrome processes across many test runs and can fill `/run/user/<uid>` (a tmpfs), which then makes *every* subsequent browser test fail with a confusing "no space left on device" error that has nothing to do with your code.

If browser tests start failing that way, check before assuming it's a real regression:

```sh
pgrep -c chromedp-runner
df -h /run/user/$(id -u)
```

If there's a large process count or high disk usage, clean it up (safe — this is disposable per-run browser profile data, not project or user data):

```sh
pkill -9 -f 'chromedp-runne[r]'   # the [r] keeps pkill from matching its own command line
find /run/user/$(id -u)/.flatpak/org.chromium.Chromium/tmp -maxdepth 1 -name 'chromedp-runner*' -exec rm -rf {} +
```

This is purely a local-dev-machine concern with this specific Flatpak setup — CI runners start clean every time and never see it.

### The store's wrapper contract

`tests/store/measure_test.go` reads `internal/store`'s own source and fails if a statement reaches
for the database directly instead of going through the wrapper in `measure.go` — so every
statement stays named and counted. If it fails on code you have just written, the fix is to call
`s.exec` / `s.query` / `s.queryRow` / `s.begin` (or `tx.exec` / `tx.queryRow`) with a statement
name; [observability.md](observability.md) says how names are chosen. Provoking `SQLITE_BUSY`
means waiting out the ten-second busy timeout, so that one test is skipped by `-short` and runs in
`make test` and on CI.

### Known quirk: the ports `tests/httpserve` binds

`httpserve.Config` names a listen address, and `httpserve.Serve` binds it — there is no way to
hand it a listener that is already open, which is deliberate (only `main` binds; see
`internal/app`'s doc comment). So a test there has to name a port *before* the server binds it,
and the usual trick — listen on `:0`, read the port, close the listener, pass the address on —
is a race these tests lose. The port comes out of the kernel's ephemeral range, and between
the close and the bind the kernel can hand that very port to one of the many outgoing
connections the ACME tests make. That is not theoretical: it is what made this package fail in
CI, as `bind: address already in use` on the redirect listener or as a listener that never came
up.

`freeAddr` therefore takes ports from a fixed window (`portWindowBase`, `portWindowSize` in
`tests/httpserve/serve_test.go`) **below** the ephemeral range, one at a time, binding each
candidate once to check it is free. A test asserts the window really is below this machine's
`/proc/sys/net/ipv4/ip_local_port_range`, so a tuned-down range is reported rather than left to
flake. **A new test in this package should call `freeAddr` and never `net.Listen` on `:0`.**

The other half of that flake was the shutdown budget. `net/http`'s `Server.Shutdown` waits a
little over five seconds before it will abandon a connection that was accepted but never sent a
request header (Go issue 22682), and these tests leave exactly those behind — the certificate
warm-up handshakes against the TLS port, and the refusal test's dials. `shutdownBudget` allows
30 seconds for that, where it used to allow 5.

## CI

`.github/workflows/ci.yml` runs on every push and pull request, and nightly at 06:17 UTC. The
commands are the same ones described above; they are split across jobs that run at the same time,
because the runners have two cores and the suite is dominated by two packages:

| Job | What it runs | About |
|---|---|---|
| `lint` | `gofmt -l`, `go vet ./...`, `golangci-lint run` | |
| `unit` | `go build ./...`, every package except `tests/app` and `tests/batch`, then the smoke test | ~1 minute of tests |
| `browser` | `tests/app`, split four ways by `$BINGLE_TEST_SHARD` | ~2 minutes per shard |
| `sims` | `tests/batch` — short on a pull request, in full on main and nightly | 1 or 6 minutes |
| `container` | the container image and `/healthz` against it | |
| `ci` | nothing: it is green only when all of the above are | |

Measured on two pinned cores, which is what a free runner gives: `tests/app` takes **494 s**,
`tests/batch` **367 s**, and all 23 other packages about **50 s together**. So the browser suite is
split over four runners, the simulations get a runner of their own, and everything else shares one.
A pull request finishes in the time of its slowest job rather than the sum of all of them.

Two things follow from this that are worth knowing:

- **A pull request does not play the long bot simulations.** `tests/batch` runs with `-short`, which
  keeps the batch machinery, determinism and metrics tests and skips the tuning search on the Council
  fixture (`TestATuningSearchOnAStudy`). They measure throughput rather than
  correctness, and they run in full on every push to main and every night. Run them yourself with
  `make test-sims` when touching the bots, the batch runner or the tuner.
- **Chrome is no longer competing with the bot simulations.** In one job the browser tests and the
  simulations ran side by side on two cores, which is the condition that made the browser tests
  flaky; each browser shard now has a runner to itself.

Pushing again to a pull request cancels the run in progress.

### The private overlay

Games built apart from the public repository live in the private overlay (ADR 0127), a checkout at
`internal/games/private` compiled in with `-tags private`. Without the tag nothing of it is
compiled, and CI here never sees it. With it checked out:

```sh
make test-private     # the whole suite with the overlay's games, and their own tests
```

Its browser tests share the public helpers of `tests/app`, so `make test-private` copies them in as
`private_*_test.go` (which git ignores) while it runs. Its own CI does the same against this
repository.

### Sharding the browser suite

`$BINGLE_TEST_SHARD` is `i/n`: the run keeps the browser tests whose name hashes into bucket `i` of
`n` and skips the others. The check lives in `newChromedpContextWith`, which every browser test
calls, so a new test joins a shard on its own and the buckets stay even — `tests/app/shard_test.go`
asserts that they do, and prints the distribution:

```sh
make test-browser SHARD=2/4            # one shard, as CI runs it
make test-browser                      # all of them
go test ./tests/app -run TestShardOfIsStableAndBalanced -v
```

A test's subtests travel with it, and a malformed spec fails the tests rather than quietly running
everything. Only the browser tests are sharded; the other tests in `tests/app` are quick and every
shard runs them.
