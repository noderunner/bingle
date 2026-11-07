# Plan: a game as an installable plugin

Status: **planning in progress** (opened 2026-10-28). These are initial thoughts written
before the design discussion, not settled decisions. Nothing is implemented, and the open
questions at the end are expected to change the shape of this.

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

## Open questions for the discussion

1. How far up the tiers do we actually want to go? My recommendation is Tier 0 plus private
   builds; Tier 1 only when something needs it.
2. Does **Cthulhu Wars leave the public repository**? It is 17,628 lines — half the games tree
   — and it is the commercial game that motivated this. Moving it is the real test of the
   private-build path, and it would also halve what the code audit has to read.
3. Where does install live: the admin area from the auth plan, or a separate page? Who may do
   it in hybrid mode — admins only, I would say.
4. Do plugins ever ship JavaScript? I lean strongly no.
5. Is there a story for *sharing* plugins between friends' servers (a signed file, a trust
   list), or is it strictly "a file I install on my own server"?

## Dependencies and cross-plan effects

- **Needs the auth plan** (merged, not implemented): `install_plugins`, the admin group and the
  admin UI. This is why auth was planned first.
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
