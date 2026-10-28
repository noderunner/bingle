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
- **Rules/engine tests** — `tests/tak` and `tests/classics` drive the engine directly: rule-by-rule Tak tests built from TPS positions, a perft check against the published Tak node counts, and randomized consistency tests that cross-check `LegalMoves` against `Apply`, mutated moves, notation and TPS round trips and piece conservation.
- **Engine hardening** — `tests/tak/fuzz_test.go` holds native Go fuzz targets (move and TPS parsers, structured-move application, the definition compiler: none may panic); run one with `go test ./tests/tak -run xxx -fuzz FuzzParseMove -fuzztime 30s`. `tests/allgames` adds `FuzzApplyAnyMoveToAnyGame`: arbitrary structured moves (including `aim`) against a mid-game position of every game, asserting that anything `Apply` accepts is one `LegalMoves` lists. `tests/board/limits_test.go` checks the board-size cap. `tests/classics/amazons_test.go` brute-forces every from/to/aim triple against the slide enumerator, and `tests/tak/completeness_test.go` does the same for stack moves.
- **Cross-game invariants** — `tests/allgames` runs one battery over every shipped game and variant (every enumerated move applies and round-trips through notation to the same position, the position document and planes keep their shape, action IDs exist, the start is playable); a new game JSON is covered the moment it is registered.
- **Training support** — `tests/envs` (planes, fixed action IDs, batch stepping, recorded matches) and `tests/puzzles` (solver correctness and a re-proof of every shipped puzzle; `go run scripts/genpuzzles` regenerates them).
- **Smoke test** — `scripts/smoke-test.sh`. Builds the real binary, runs it as a real OS process bound to a real port, and curls `/healthz`. This is the only check that exercises `main.go` and `internal/httpserve`'s actual `net.Listen` path end to end; everything else in `internal/app` is deliberately built to never bind a real listener itself, so it can be tested in-process (see `docs/history/plan.md` section 6.7).

### Courtfall

`tests/courtfall` pins the Go ruleset: the draft and its constraints (every
random draft places all ten tiles), turn structure, movement against an
independent reference implementation on random boards, exchanges with
fixed dice (`courtfall.RollDice` finds a generator state that rolls what a
scenario needs), shots and shoves, pushes and chains, every Court, secrecy
(public/own views, move-list masking, determinization) and a random-play
contract check of `Legal`/`Apply`/notation/planes/action space. REST-level
secrecy and the undo guard are in `tests/app/courtfall_rest_test.go`.

### Cthulhu Wars

`tests/cthulhuwars` pins the ruleset with scenarios built from a small
position builder (`pos{units, gates, power, books, …}`) and fixed dice
(`seedDice` finds a generator state that rolls a given sequence): setup,
costs and legality of every action, Flight, Gates and abandonment, battle
(kills, pains, retreat, Devour, every combat formula), each spellbook and
faction action, gathering Power with the half-the-leader floor, first player
ties, the Doom phase, the Ritual track, Instant Death, humanity and Elder Sign
secrecy. `wiki_rules_test.go` pins the corrections of the wiki rules pass: the
Ritual track by player count (the marker, the rising cost, Instant Death and the
end of that Doom phase), the Elder Signs of Immortal and of the first and last
book, Cthulhu's start area, the kill requirements, Avatar, the Red Sign, Share
Areas, paying 10 at once, Thousand Forms, Harbinger, Madness and Necrophagy in
battles the owner is not in, the Devolve reaction windows, Regenerate,
Invisibility on enemies and the Yellow Sign grant. `contract_test.go` plays random four-seat games and checks that
every legal move applies and round-trips through notation, `Apply` never
mutates its input, the action space is fixed and complete, planes are in
range, no decision leaves the position unchanged, only the seat to move has moves, public views hide the generator and
other players' signs, runs are reproducible by seed and take-back is refused
across a roll; `FuzzParseMove` fuzzes the parser. `playtest_test.go` pins
what the playtests asked for: Gates taken by arriving Cultists, revealing all
Elder Signs at once and keeping them private, the bag count, the phases of the
round and the order of play, the event log, unit cards and the spellbooks that
enhance units, and Submerge's choice of units; `guide_test.go` pins the bots'
strategy tips (never abandon, the opening, one unit per Move, reply
summoning…) and plays whole guided games to check they stay legal and end.
`expansion_test.go` derives tables (`cat.Derive`: the cults of each seat,
the High Priests; refused options) and plays random games of six kinds of
table; `expansion_rules_test.go` pins the Windwalker (setup choice,
Hibernate, Ice Age, Arctic Wind, Gnoph-Keh's price, Rhan-Tegoth and Ithaqua,
requirements, Herald, Howl, Berserkergang, Cannibalism, Eternal, Ferox), the
Sleeper (the 3-Power requirement actions, Lethargy, Death from Below, Ancient
Sorcery, Burrow, Capture Monster, Cursed Slumber, the Formless Spawn and
Tsathoggua dice, Demand Sacrifice, Energy Nexus) and the High Priests
(recruiting, Gather Power, capture, the Oath on one's own turn, after every
action of another player, and in the Doom phase) on positions built by cult key on
derived games; `gates_test.go` the occupant of a Gate, told apart from the
units beside it: leaving it (in place it is free, once a turn), the Gate
abandoned even with company, a unit put on it for free in the turn and in the middle of a Move,
swapping, the hand-over when the occupant is lost against its owner's will, the
High Priest and the Dark Young as occupants; `cards_test.go` the view behind the
cards — the price of every action button, the button and rule of every Great
Old One, the rule text of the ability buttons, and where each unit that is not in
the pool is; `expansion_test.go` also plays guided (bot) games on every kind of
table. `tests/envs/derived_test.go` runs a derived id as a training
environment. `tests/app` adds
`cthulhuwars_rest_test.go` (area maps, layouts and guide in the API, inert
assets, generated guides, four players over REST, bots filling a table and
named after their faction and kind, `seat_bots`, slow bots playing on in the
background, MCP guide and `add_bot`, the view's phases, order, bag, Ritual
track and private signs), `cthulhuwars_ui_test.go`
(map, tokens and cult panels, the Ritual track in the round bar,
summon / move / battle by clicking from a custom position, the rules
window's tabs, tables, images, keyboard and search, and the Markdown
renderer's safety), `cthulhuwars_playtest_ui_test.go` (unit cards, the
direction choice and phase bar, own Elder Signs, enhanced units, the log in
colour) and `cthulhuwars_round4_ui_test.go` (the corner star and class-sized
icons, the reaction settings (also before the spellbook is earned: "When I'm
targeted", its ? modal, the player list, the shortcuts on the question, and
changing the choice with real clicks while the bots play and the page is
redrawn many times a second), Devolve by clicking an
Acolyte, the lobby with its invite link, the chat, and the home page's seat
arrangement — bot kinds — surviving a change of layout) and
`cthulhuwars_round5_ui_test.go` (the home page's cult lists — swaps, the
High Priests box, memory across layouts, the `options` that reach the server and
that an unchanged table keeps its plain id — and the game page of a derived
table: panels in their colours, the Windwalker's choice of where to begin),
`cthulhuwars_options_rest_test.go` (derived tables over REST: options, seats,
lobby, a bot Windwalker beginning by itself), `cthulhuwars_round6_ui_test.go`
(a Gate with its occupant — one on the Gate, the rest in the stack — picked up and
stepped off, a unit put back on it, the game options window with its
sections and layers, a button and hover card for every monster, the pictograms
and pool icons of the cult cards, one card with the Moves and History tabs and
Undo among the actions, a game against oneself from the home page),
`cthulhuwars_round7_ui_test.go` (the units of a cult all listed with those away
greyed out and their place on hover, the price and rule card of the action
buttons, a button for each Great Old One, the words above the buttons, the
headed spellbook choice), `cthulhuwars_round8_ui_test.go` (spellbook
pictograms instead of ticks in the cult cards, on the buttons of spellbook
abilities and — greyed out — of the requirement actions, the Captured zone, a
free step-off that asks for nothing, the steps of awakening in every card of a
Great Old One and in the rules window), `cthulhuwars_round9_ui_test.go` (Desecrate
rolling at once and asking for its unit after, no Undo across a roll — in the page
and over REST, where the move is marked `locked` and the undo is a 409 — Yellow
Sign's Desecration token count first among its numbers, and the abilities of every
Great Old One in its cards and in the rules), `cthulhuwars_round10_ui_test.go`
(Great Cthulhu in seat 3 choosing the direction first, the History tab through
quiet stretches and earlier moves, the Black Goat's Done after one summon, the
Auto-Play Timeout in the options and its marks in the Moves and History tabs,
the cult list of an open seat in the lobby, the chat's emoji),
`cthulhuwars_round10_rest_test.go` (the first player of many sample tables, the
cult of an open seat changed over REST, the Auto-Play Timeout played by a bot
once the wait has run out and restarted by every move) and `solo_games_test.go`
(seats played by their creator, over REST).
`lobby_chat_reactions_test.go` covers the three features at the REST level —
lobbies (host, seats, start, remove, bots, the invite link), chat (history,
limits, spectators, the burst limiter) and reaction preferences — reactions
passed for a person unless they asked to be asked (when targeted, after chosen
players or anyone), the session's passes, which Undo ignores, and the rows saved
by the earlier opposite choice — and `mcp_v2_test.go` the matching MCP tools
(`start_game`, `set_seat_cult`, `send_chat`, `get_chat`, `get_reactions`,
`set_reactions`). The engine side is `tests/cthulhuwars/targeted_test.go` (what
an action is aimed at), `first_player_test.go` (the first player of a table) and
the Doom-log and Black Goat tests of `round_test.go` and `actions_test.go`.

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
BINGLE_ADDR=:18099 BINGLE_DB_PATH=/tmp/shot.db ./bin/bingle &
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

## CI

`.github/workflows/ci.yml` runs on every push and pull request, and nightly at 06:17 UTC. The
commands are the same ones described above; they are split across jobs that run at the same time,
because the runners have two cores and the suite is dominated by two packages:

| Job | What it runs | About |
|---|---|---|
| `lint` | `gofmt -l`, `go vet ./...`, `golangci-lint run` | |
| `unit` | `go build ./...`, every package except `tests/app` and `tests/batch`, then the smoke test | ~1 minute of tests |
| `browser` | `tests/app`, split four ways by `$BINGLE_TEST_SHARD`; the Cthulhu Wars interface tests only on a full run | ~2 minutes per shard |
| `sims` | `tests/batch` — short on a pull request, in full on main and nightly | 1 or 6 minutes |
| `container` | the container image and `/healthz` against it | |
| `ci` | nothing: it is green only when all of the above are | |

Measured on two pinned cores, which is what a free runner gives: `tests/app` takes **494 s**,
`tests/batch` **367 s**, and all 23 other packages about **50 s together**. So the browser suite is
split over four runners, the simulations get a runner of their own, and everything else shares one.
A pull request finishes in the time of its slowest job rather than the sum of all of them.

Three things follow from this that are worth knowing:

- **A pull request does not play the long bot simulations.** `tests/batch` runs with `-short`, which
  keeps the batch machinery, determinism and metrics tests and skips `TestBatchThroughput` (24 whole
  games of Cthulhu Wars) and `TestTunerRunsEndToEnd`. They measure throughput rather than
  correctness, and they run in full on every push to main and every night. Run them yourself with
  `make test-sims` when touching the bots, the batch runner or the tuner.
- **A pull request does not run the Cthulhu Wars interface tests.** See below: they are 56 of the
  172 browser tests, they drive one game's interface, and both CI failures the new workflow has
  seen so far came from one of them. `make test-cw-ui` runs them, and so do main, the nightly run
  and a manual run.
- **Chrome is no longer competing with the bot simulations.** In one job the browser tests and the
  simulations ran side by side on two cores, which is the condition that made the browser tests
  flaky; each browser shard now has a runner to itself.

Pushing again to a pull request cancels the run in progress.

### The Cthulhu Wars interface tests

`$BINGLE_TEST_CW_UI` turns them on; they are skipped without it:

```sh
make test-cw-ui                        # just those
make test                              # the whole suite sets it for you
BINGLE_TEST_CW_UI=1 make test-browser   # the browser suite with them in
```

Run them before pushing a change to Cthulhu Wars, or to anything they share with the rest of the
interface — the game page, the options and rules windows, the dock, the seat strips, the home
page's tables.

**The rule is a test's name, not its file.** A `TestWebCthulhuWars...` test is gated wherever it
lives, which is how the phone-layout test in `mobile_ui_test.go` is covered; and ten browser tests
that live in the `cthulhuwars_*_ui_test.go` files are *not* gated, because what they exercise is
shared code reached through a Cthulhu Wars table: the Markdown renderer's safety, the rules window,
the lobby, and the home page's layouts and seat choices. A new Cthulhu Wars interface test has to
be named `TestWebCthulhuWars...` to be covered by the gate.

Everything else about the game runs everywhere, which is most of what pins its rules: the 276 tests
in `tests/cthulhuwars`, and its REST and MCP surface in `tests/app`.

### Sharding the browser suite

`$BINGLE_TEST_SHARD` is `i/n`: the run keeps the browser tests whose name hashes into bucket `i` of
`n` and skips the others. It composes with the gate above — the Cthulhu Wars tests spread over the
shards on the runs that play them. The check lives in `newChromedpContextWith`, which every browser test
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
