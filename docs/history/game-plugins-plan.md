# Plan: a game as an installable plugin

Status: **planning in progress** (opened 2026-10-28). The five open questions were settled on
2026-10-31 (see *Decisions*). Step 1 and the retirements are built; step 2 is next. The sections before *Decisions* are
the initial thinking, kept as written.

## What was asked for

- One file (a zip?) holds one game — the Go code, the UI extensions, the graphics, the rules.
- New games can be **installed through the bingle interface**.
- So that personal-use versions of commercial games can be developed without publishing them,
  while public-domain games (Amazons, Blooms) stay in the public repository.

## What already exists that this builds on

Much more than it first appears:

- A game is already a value: `catalog.Module{ID string, Definition []byte, Rules any, Assets
  fs.FS}`. `Catalog.RegisterModule` is mutex-guarded, so **registering a game while the server
  runs is already safe**.
- Assets are already served out of a module's `fs.FS` at `/api/game-defs/{id}/assets/*`, and
  guides at `/api/game-defs/{id}/guide`. `archive/zip`'s reader implements `fs.FS`, so a zip is
  a drop-in asset source with no new abstraction.
- **15 of the 25 games are already pure data**: Tak plus the 14 in `internal/games/classics`
  are a JSON definition and nothing else. Ten have Go rulesets, from Blooms at 714 lines to
  Cthulhu Wars at 17,628.
- `state.Ruleset` is 13 methods, and `Legal`/`Apply` **take no `context`, do no I/O and are
  deterministic**. For sandboxing that is a gift: there is nothing to mediate but CPU and
  memory.
- `callback.GameCallbacks` is only three methods (`ValidateMove`, `OnMoveApplied`,
  `CheckWinner`) and the DSL already has `win_condition: callback` to reach them.

## Initial thinking: three tiers, and they are not equally wise

**Tier 0 — a data-only plugin.** A zip containing `game.json`, assets and guide markdown. No
code executes. This covers every one of the 15 games that are already pure DSL, needs no new
engine concept, and is maybe a week of careful work: a manifest, validation through the
existing compiler, an install path, a store table, and a registry that merges installed
modules with built-in ones. **This is most of the value for a fraction of the risk**, and it
gets better every time the DSL coverage audit converts another game.

**Tier 1 — data plus a small sandboxed hook.** The three `GameCallbacks` methods, supplied as
WebAssembly and run with wazero (pure Go, no CGO, so the single static binary survives). The
interface is tiny and pure, which is exactly what makes it sandboxable.

**Tier 2 — a full ruleset from a plugin.** Three ways, and I do not like two of them:

- `plugin.Open` is **out**: it needs CGO and a byte-identical toolchain and dependency set.
  Bingle builds with `CGO_ENABLED=0` on purpose.
- WASM or a subprocess for the whole `Ruleset` is technically possible but has a **performance
  cliff**: bots and practice batches call `Legal` and `Apply` millions of times (batch workers
  run on three quarters of the CPUs, hint and bot slots on more), and every call would cross a
  serialization boundary. A game that is pleasant to play by hand and useless to analyse is a
  bad trade.
- **A private build** is the honest answer: the game lives in its own repository, is wired in
  with a build tag or a small `games_private.go`, and the owner builds their own binary. No
  runtime install, no sandbox, full speed, and it meets the actual goal — a commercial game
  that is never published.

So my initial recommendation is **Tier 0 now, Tier 1 if a real game needs it, and Tier 2 as a
private build rather than a runtime install** — with the DSL coverage audit as the lever that
moves games from Tier 2 into Tier 0 over time.

## Things that will bite, listed now so they are not discovered later

- **Installing code is privilege escalation.** This needs the `install_plugins` capability from
  the auth plan, a reauthentication step, and an audit entry. It is the single most dangerous
  button in the product.
- **Zip slip and friends**: entry paths must be validated, symlinks refused, sizes capped,
  content types sniffed rather than trusted from the extension.
- **Identifier collisions.** Built-in ids are bare (`blooms`), and derived games already use
  `base~suffix`. Installed games need their own namespace — `@author/game` or a reserved
  prefix — or a plugin can shadow a built-in and silently change a running game's rules.
- **Uninstall with games in the database.** `games.game_def_id` is a bare string. Removing a
  plugin must leave finished games displayable: the same tombstone discipline the auth plan
  uses for deleted accounts.
- **Version provenance.** The `game_versions` table already records which version of a game was
  played; a plugin's version has to flow into it, and an upgrade that changes rules mid-game
  must be refused for games in progress.
- **Client extensions are the risky part.** A plugin that ships an ES module into the page is
  cross-site scripting by design. My instinct is that plugins get *declarative* display — the
  existing `display`, themes, tints, badges and overlay vocabulary, plus images and markdown —
  and no JavaScript at all, at least to begin with.
- **Licensing is the whole point**: the public repository keeps public-domain games, and the
  install path is what lets a private game exist without being published. Worth writing down
  plainly, including that bingle ships no mechanism for distributing someone else's game.

## Decisions (settled 2026-10-31)

The five open questions were discussed on 2026-10-31, using Cthulhu Wars, the most complex game,
as the test case. These become an ADR when this plan lands on `main`.

1. **Go code is compiled in, never loaded at run time.** Installable packs are data only
   (Tier 0). A game with a Go ruleset is part of a build: the public binary, or a private
   build for a game that must not be published. Tier 1 (WebAssembly hooks) and a WebAssembly
   or subprocess Tier 2 are **not planned**: every `Legal`/`Apply` call would cross a
   serialization boundary, and bots and practice batches make millions of them. They would
   only come back if sharing *code* between servers becomes a goal, and then with the bot
   search inside the plugin so the boundary is crossed once per move decision, not once
   per searched position.

   `plugin.Open` (a `.so` per game) was considered in detail and rejected:
   - it needs cgo, which ends the static `CGO_ENABLED=0` binary (ADR 0014) and does not
     exist on Windows;
   - Go refuses to load a plugin unless every package it shares with the host was built from
     identical source with the same toolchain and flags. Cthulhu Wars imports
     `engine/state`, `catalog`, `board` and `dsl`, so any engine change, Go point release or
     dependency bump invalidates the `.so`. It is a piece of one particular binary shipped
     separately, not a pack. A narrower plugin API (the `plugin_foundation` experiment)
     reduces how often the game's *source* must change, not how often the `.so` must be
     rebuilt, because the check covers the boundary package and the standard library too;
   - a plugin can never be unloaded, so uninstall and upgrade need a restart and two versions
     of one game cannot coexist;
   - it runs inside the server with full access to the database and the network.

2. **Cthulhu Wars leaves the public repository**, into a private build. It is the real test
   of the private-build path. Because it imports `internal/...` packages, which another Go
   module may not import, the private build is an **overlay**: a gitignored
   `internal/games/private/` directory (or a private repository checked out there) compiled
   in by a build tag, rather than a separate module. The engine does not have to move to a
   public import path.

3. **Installing a data pack lives in the admin area**, behind the `install_plugins`
   capability, a fresh reauthentication and an audit entry. Admins only, in every mode.

4. **No JavaScript in a pack, ever.** A pack's look is data: its web file (`web.yaml`, ADR 0124,
   which moved the appearance half of `display` out of the definition), images and markdown. This is now a design
   principle as well as a security rule: see *Every game is playable through the API alone*,
   below.

5. **Data packs can be shared** as files between servers; the server validates a pack on
   install as if it were untrusted. Sharing code waits for the route in decision 1, if ever.

## Every game is playable through the API alone

A separate project, a native 3D client built on Bevy, is planned to play the same games
through the API. Whatever it turns out to need, a game must not depend on the JavaScript
interface to be played or understood. The same rule is what lets a private game leave the
repository cleanly. Today the rules side already holds: the web client fetches the full legal
move list (`api.legal`) and `MoveBuilder` only narrows it as the player clicks. The coupling
that remains, found 2026-10-31:

- **Cthulhu Wars' presentation lives in the web client**: cult names and colours in
  `cw-names.js`, the sigils in `cw-glyphs.js` (drawn by `render.js`), icons in
  `game-icons.js` and `icons.js`, and a `gameID.startsWith("cthulhu-wars")` check in
  `practice.js`. (2026-10-31 said these move into the game's definition. ADR 0124 reverses that:
  they stay the web client's, registered per game, so the shared page names no game.)
- **The server names Cthulhu Wars outside its package**: `internal/app/app.go` registers it
  directly, `internal/batch/run.go` and `internal/batch/report.go` build CW-only practice
  reports, and `internal/tune/tune.go` and `scripts/tune/main.go` import it. Games should
  register themselves (selected by build tag), and a game should describe the statistics its
  practice report shows instead of `internal/batch` knowing them.

**How assets reach a non-web client is deliberately not decided.** Whether the Bevy client
downloads assets from the server, ships its own, or both, is for the discussion when that
client is built. Nothing here should assume one answer.

## Which games stay public

Decided 2026-10-31: **the public repository keeps public-domain games, openly licensed games,
games whose designers give general permission to implement them with credit, and the
maintainer's own designs.** A commercial game moves to the private overlay, and can come
back if its owner gives permission.

- **Private, for now: Cthulhu Wars, Yinsh, Onitama, Hive and Nyet!** No other game moves.
- **Everything else stays public**, including the designer-owned abstracts such as Blooms
  and Meridians. Their designers allow implementations that credit them, as on
  abstractplay.com, so **a public game must credit its designers**: the `designers` field
  every definition already carries is kept accurate and shown to players.
- **Reversi drops "(Othello)"** from its name; Othello is a trademark. The definition, its
  guide and `docs/games.md` change together.

**This reaches the public test suite.** Cthulhu Wars is named in about 60 test files, and
Hive, Yinsh, Onitama and Nyet! have their own test directories. Between them they are what
exercises the engine's hidden information, cards and reactions. So before a game moves, every
engine feature it exercised must be covered by a game that stays public, or by a small
**test-only fixture game** that lives in `tests/` and is never registered in the shipped
binary. Its own rules tests move with it into the private overlay.

## Retiring games

Decided 2026-10-31: **Tic-Tac-Toe and Connect Four are removed entirely.** The bots and the
other games now cover what they were there to show, and they are unlikely to be played.
Neither is a commercial question; they go, not to the private overlay.

- Tic-Tac-Toe is named in about 39 test files, as the smallest complete game. It becomes a
  **test-only fixture** under `tests/` rather than a shipped game, so those tests keep it.
  Connect Four's 6 test files are moved to another game or dropped.
- **Games already in a database must survive.** A finished game of a retired type stays in
  the player's history, readable as a move list, with the same tombstone discipline that
  uninstalling a pack needs. A game still in progress is ended at the upgrade with no result,
  and the changelog says so (confirmed 2026-10-31). This is the first real use of the tombstone, so it is built
  here and reused by data packs.
- Docs, guides, icons, the home page's lists and the changelog drop them.

Retirement needs nothing else in this plan and can land on its own, ahead of the rest.

## The order of work

1. **An API-only play test**: a scripted client plays every game to a finish using only the
   legal-moves and move endpoints, no browser. A game that cannot is a bug. It runs first
   because it shows the gaps before anything moves.
2. **Games register themselves**, and the binary's set of games is chosen by build tags.
3. **The web page stops naming games, and no look moves to the server** (ADR 0124, #157,
   which replaces the 2026-10-31 wording "presentation moves to the server"). The page reads a
   game's `web.yaml` and, for a compiled-in game, the scripts it registers through one generic
   hook. Cthulhu Wars' colours, sigils, icons and `cw-*.js` interface stay web-side and go with
   the game into the overlay. The practice report's shape is described by the game, as meaning.
4. **Fixture games** for every engine feature a departing game was the only public test of,
   Tic-Tac-Toe among them.
5. **The private overlay**, then **Cthulhu Wars, Yinsh, Onitama, Hive and Nyet! move into
   it**, Cthulhu Wars first. The acceptance test: the public repository builds, passes `make test`
   and plays every remaining game through the API with the private games absent.
6. **Data packs** (Tier 0): manifest, validation through the compiler, the store table, the
   admin install page, the namespace, uninstall tombstones and version provenance listed
   above.

Steps 1 to 5 need nothing from the other plans. Step 6 needs the auth plan's admin area,
which is now built.

## Dependencies and cross-plan effects

- **Needs the auth plan** (built on `main` by 2026-10-31): `install_plugins`, the admin group
  and the admin UI. Only step 6 depends on it.
- **Fed by the DSL coverage audit**: every game the audit moves into pure DSL becomes
  installable at Tier 0 with no further work. These two plans should be read together.
- **Fed by the YAML plan**: if a definition can be YAML, a plugin manifest and definition can
  be one file in one format.
- **Affects tutorials and achievements**: if a plugin can ship a game, it should be able to
  ship that game's tutorial and its badges, which means both of those must be data too.
- **Affects observability**: a plugin's compile and install deserve their own log events, and a
  plugin game's errors need to be attributable to the plugin.

## Status log

- 2026-10-28: opened with initial thoughts. Three tiers sketched, Tier 0 plus private builds
  recommended, five open questions. Planning in progress.
- 2026-10-31: the five questions settled with Cthulhu Wars as the test case. Go code stays
  compiled in (`.so` plugins and WebAssembly rulesets rejected); Cthulhu Wars moves to a
  private overlay build; packs are data only, with no JavaScript, installed by admins and
  shareable. Added the API-only principle, the coupling found in the web client and server,
  and an order of work. How a native client gets assets is left open.
- 2026-10-31: every commercial game goes private, not only Cthulhu Wars; the public
  repository keeps public-domain, openly licensed and the maintainer's own games. Added a
  proposed sorting of all 26 games for the maintainer to confirm, and fixture games to keep
  the engine's features tested once the games that exercised them leave.
- 2026-10-31: the private list settled at Cthulhu Wars, Yinsh, Onitama, Hive and Nyet!; every
  other game stays public with its designers credited. Tic-Tac-Toe and Connect Four are
  retired outright (Tic-Tac-Toe survives as a test fixture), with tombstones for games
  already played. Reversi drops "(Othello)".
- 2026-10-31: confirmed that games of a retired type still in progress end at the upgrade.
- 2026-11-07: step 1, the API-only play test, is built (#110), and so are the retirements (#53).
  The DSL coverage audit is done: Blooms, Klondike, Spider and FreeCell are data, and **Onitama
  stays in Go** (the maintainer's choice, so its expansions can be added), so it moves to the
  overlay as a ruleset, not as a private pack. Found while taking stock: **step 3 was sized for
  Cthulhu Wars' page as it was on 2026-10-31.** It is now about 5,500 lines in 23 `cw-*.js` files
  (map stage, battle, setup, art, tips), nine shared page files name it, and 83 test files do (not
  about 60). That interface cannot become the declarative display vocabulary. Decision 4 forbids
  JavaScript in *packs* only, so the overlay can carry the game's own scripts as module assets
  that the public client loads through a generic hook; step 3 should be rewritten to say so, and
  settled with the maintainer, before it is built. Step 4 should start by listing the engine
  features that only the departing games exercise.
- 2026-11-07: the maintainer set a strict line between meaning and appearance: the server says
  what things are, and each client decides how they look, so a Bevy or Java client never designs
  around the web page. ADR 0124 (proposed, #157) records it with an audit of today's appearance
  fields. Step 3 and decision 4 are rewritten to match: nothing about a game's look moves to the
  server, the web page reads a per-game `web.yaml` and registered scripts, and Cthulhu Wars' look
  goes with it into the overlay.
- 2026-11-07: an alternative to the private overlay is written up as a deferred proposal,
  `game-services-proposal.md`: a game family behind one coarse interface, run in process or as a
  binary of its own, with the boundary crossed once per impulse and the search inside the game.
  The overlay stays the plan. Step 2 should register a family through one value close to that
  interface, which keeps the door open.
