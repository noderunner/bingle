# 0147. Rules as WebAssembly in an install pack (proof of concept)

Status: proposed (2026-11-24), phase 0 of #294. A proof of concept, not a direction: it ends in phase 5 (#300) with
the maintainer's verdict, and if that verdict is to drop it, the host and its dependency are removed again. It
reopens decision 1 of the game plugins plan ([docs/history/game-plugins-plan.md](../history/game-plugins-plan.md))
and the withdrawn [ADR 0128](0128-data-packs.md), and is a third transport for the cut of the game services
proposal ([docs/history/game-services-proposal.md](../history/game-services-proposal.md), #241). It relies on ADR
0014 (one static binary), 0076 (a retired game), 0098 (a version is a game of its own), 0114 (impulses and
selections), 0124 (a game's look is data), 0127 (the private overlay) and 0140 (private games are marked).

## Context

A game with Go rules is part of a build. A commercial one is compiled in from the private overlay (ADR 0127), so
running it takes a private build of the whole server, and its code runs with the server's full access. That is
cheap and fast, and it is still the plan. This proof of concept asks whether a game's rules can instead be a
WebAssembly module in a pack that an unmodified public binary loads, and what that costs. Zèrtz is the test case
because it is small, commercial and not yet written. It has a board that shrinks, a shared pool of marbles,
compulsory captures that chain (several selections in one impulse), isolation captures and three ways to win.

If it works, a pack would answer several of the services proposal's reasons to revisit: a game with its own
release cycle, a game whose crash or runaway search cannot hurt other games, a private game run by a public build,
and (one day) a game written in another language. The proof of concept has to show whether it works, and at what
price.

### What it reopens, in the earlier words

- **Plugins plan, decision 1**: "Go code is compiled in, never loaded at run time… a WebAssembly or subprocess Tier
  2 are **not planned**: every `Legal`/`Apply` call would cross a serialization boundary, and bots and practice
  batches make millions of them. They would only come back if sharing *code* between servers becomes a goal, and
  then with the bot search inside the plugin so the boundary is crossed once per move decision." The proof of
  concept measures that boundary first (phase 1, on the same rules both ways). Then, in phase 4, it moves the search
  inside the module, as the plan said it would have to.
- **ADR 0128, withdrawn 2026-11-09**: packs "did not pay for what they cost: the most dangerous button in the
  product, a reader for hostile zips, an admin flow with its reauthentication and audit, a namespace, versions and
  uninstalling". The proof of concept builds none of the button, the zip reader or the admin flow. Packs are
  directories that the operator names in the environment, read at start-up, which is close to the plan's own "a
  development-only folder of definitions loaded from disk, which is trusted, would serve … at a fraction of the
  cost". The namespace and versions are ADR 0128's as written, and uninstalling is ADR 0076's retired game.
- **The services proposal** cuts at a player's action (`Submit` per impulse, `Bot` per decision), so that nothing
  fine-grained crosses. Phase 1 deliberately cuts lower, at `state.Ruleset`, because then the session, API, MCP,
  bots and web page work unchanged and the cost of the fine-grained boundary is measured rather than argued.
  Phase 4 adds the proposal's coarse `Bot` call. If the verdict is to keep packs, a later ABI should move to the
  proposal's other coarse calls, so that one interface has three transports: in process, a socket and
  WebAssembly.

## Decision

### The runtime

**[wazero](https://github.com/tetratelabs/wazero)**, pinned to a tagged release. It is pure Go and its own module
requires nothing, so `CGO_ENABLED=0` and the single binary survive (ADR 0014). It adds one direct dependency and no
indirect ones. A module is compiled once, ahead of time, where wazero has a compiler (amd64 and arm64), and is
interpreted elsewhere. It is compiled when its pack loads and kept in memory. The proof of concept keeps no
on-disk cache.

### The guest

A pack's rules are **standard Go**: `GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared`, exporting its calls
with `//go:wasmexport`. That makes it a reactor module: the host calls `_initialize` once and then calls the
exports. No other toolchain is needed. TinyGo stays an option if module size or start-up time proves to be the
problem (phase 5 reports both).

A game implements a plain Go interface in **`internal/engine/wasmrules/guest`**. The interface has start, impulse
seat, legal, apply, format, parse, gestures, validate and outcome, over types of its own whose JSON is the
engine's. The package's exports, memory handling and dispatch compile only for `wasip1`. **The guest package
imports nothing from the engine**, so a module carries only its game and the standard library, and the JSON on the
wire, not Go types, is what a pack depends on. The host package `internal/engine/wasmrules` also offers an
**in-process adapter** for the same interface (`wasmrules.Native`). The same rules package can therefore be linked
natively, which gives a like-for-like benchmark, plain `go test` for a pack's rules, and a way back to the overlay
if the proof of concept is dropped.

### ABI v0

ABI v0 is the proof of concept's ABI, and **it may change without notice until this ADR is accepted**. A pack is
rebuilt when it changes. Each export is prefixed `bingle_`:

| Export | Signature | What it does |
|---|---|---|
| `bingle_abi` | `() → i32` | The ABI version the module speaks: `0`. The host refuses a module whose version it does not speak, or which lacks the export |
| `bingle_alloc` | `(size u32) → u32` | Memory for a request, which the host writes into |
| `bingle_free` | `(ptr u32, size u32)` | Frees a request or a reply |
| `bingle_init` and one export per call | `(ptr u32, len u32) → u64` | Reads a JSON request at `ptr`, returns its JSON reply as `ptr << 32 \| len`. The host copies the reply and frees it |

The calls are `init`, `start`, `impulse_seat`, `legal`, `apply`, `format`, `parse`, `gestures`, `validate` and
`outcome`, one for each method of `state.Ruleset`. Phase 4 adds `bot`. Only the Ruleset's nine methods are
bridged. Every optional interface (`HiddenInfo`, `Reactor`, `Heuristic`, `PlaneEncoder` and the rest) is left
out until a phase needs it, and a phase that adds one records it here.

- **Every reply is an envelope**: `{"ok": …}` or `{"error": "…"}`. An error is the call's error (an illegal
  selection, unparsable notation, a bad custom start). A reply that is not JSON, or not an envelope, counts as a
  failure of the instance (see below).
- **`init`, once per instance, carries the games**: every game the pack's definition compiles to (its base game and
  each variant), each with its id, its definition as the catalog compiled it, and its board as the catalog built it
  (each position's id, coordinates and neighbours, and the regions). A guest never compiles a definition or a
  board itself. No other call carries the definition.
- **Every other request names its game** by id, and carries what the Ruleset method takes: the `position`, a
  `seat`, a `selection`, a `text` or a `seed`.
- **A position crosses as its document**, exactly as `BoardState.MarshalJSON` writes it (`v: 2`: cells, reserves,
  ply, turn, counters and `ext`), and comes back the same way. The host reads it with the engine's own decoder.
  **The guest's own data rides in `ext`, which the host never decodes**: it stays raw JSON in the `BoardState`, is
  stored as it is, and is handed back on the next call.
- **Selections and gestures** cross with the JSON of `state.Selection` and `state.Gesture`.
- **`apply` replies with the result**: the new position and the fields of `state.MoveResult` (`captures`,
  `next_seat`, `notation`, `facts`, `ended`, `winner`, `drawn`, `all_lost`, `reason`, `detail`). `outcome` replies
  with the fields of `state.Outcome`. Where an impulse ends is worked out by the host from the reply, as for any
  ruleset (`state.ImpulseEnds`). The guest does not report it.

### The sandbox

- **No host functions but WASI's**, and of WASI only what the Go runtime needs. There are no preopened directories,
  no environment and no arguments, and there are no sockets (wazero has none unless a host adds them). Stdout and
  stderr go to the server's log at debug level, cut short.
- **No real clock and no real randomness.** Wall time and the monotonic clock are fake ones, not the
  machine's, and `random_get` reads from a fixed seed. All of this is set explicitly rather than left to wazero's defaults. A
  game's chance comes only from the `seed` that `start` is given, kept in its position, as for a compiled-in
  ruleset. Go randomises map order inside the guest from that seed, so a guest must not let map order reach a
  reply. Phase 1's tests compare a fresh instance with a reused one to catch it.
- **A memory cap per instance**, by default 2048 pages (128 MiB), until phase 1 has measured what a Go guest
  needs.
- **A deadline per call**, by default two seconds, enforced by wazero closing the module when its context ends.
  The context is the adapter's own: `Legal` and `Apply` still take none, and do no I/O. A call into an in-memory
  module is as pure as the rules inside it, which is what CLAUDE.md asks.

### Instances and failures

- **A pool of instances per pack.** One instance runs one call at a time. Instances are made as they are needed,
  each with `init`, up to a cap the size of the bot and batch slots. A call waits, within its deadline, for a free
  one.
- **A guest keeps nothing between calls.** Everything a call needs is in its request, so any instance can answer
  it.
- **An instance that traps, runs out of time, outgrows its memory or replies with something malformed is thrown
  away** and replaced, and the call is tried once more on the new instance. A second failure is that call's
  failure, and it is logged with the pack, the call and the game.
- **A failed call never ends a game or loses a move.** `Start`, `Apply`, `Parse` and `Validate` return it as their
  error. The methods without an error (`ImpulseSeat`, `Legal`, `Format`, `Gestures`, `Outcome`) answer with
  something that leaves the game as it stands: no legal selections, an outcome that is not over, notation left
  blank. `ImpulseSeat` has no such answer, since `0` means the game is over. Phase 1 settles it, for instance by
  answering from the seats that `start` and `apply` replied with, and tests each of these.

### A pack

A pack is a **directory**:

```text
zertz/
  manifest.yaml   name, version, abi, private
  game.yaml       an ordinary definition with engine: ruleset, and its variants
  web.yaml        optional: its look (ADR 0124)
  rules.wasm      the rules
  guide/          the guide's Markdown and pictures
```

- **The definition needs no new field.** `engine: ruleset` says the rules are a ruleset, and the loader supplies
  the adapter as the module's `Rules`.
- **Names and versions are ADR 0128's.** The family is `pack-<name>`, each version is a game of its own
  `pack-<name>-<version>` with variants `pack-<name>-<version>-<word>` (ADR 0098), and the family's name means the
  newest. The loader sets `id`, `family`, `version` and `prototype` from the manifest and refuses a definition that
  sets them. A pack's game is a prototype, so it is not rated. No built-in game may use the prefix, and
  `tests/registry` holds that.
- **Two versions can sit side by side** as two directories. New games use the newest, and a game in progress
  finishes on the rules it started with.
- **A pack can carry no page script, and no look's `script` or `style`.** What it draws is what the page already
  draws (ADR 0128's checks, below). A game that needs its own drawing still needs a build.

### Loading

- **`BINGLE_PACKS`** names a directory of packs, read once at start-up. Configuration comes from the environment
  (`internal/config`), so there is no flag. Unset means no packs and nothing changes. To change a pack, change the
  directory and restart.
- **The operator chooses what is in the directory, but its contents are checked as though they came from anywhere**,
  since a pack is meant to be handed from one server to another. The checks are ADR 0128's, less what only a zip
  needed: the manifest and ABI version, a definition that compiles, a look that fits it (`look.Check`), no `script`
  or `style`, colours that are colours, pictures sniffed, text that is text, no path or link that leaves the pack,
  size limits on what is really read, and a short random trial of each game, in which a pack that panics or thinks
  far too long is refused.
- **A pack that fails is logged with its reason and left out.** The server starts without it.
- **A pack that was there and is gone** leaves its games as a retired game's (ADR 0076). They stay readable, and
  nothing new is started.

### The questions #294 left open, with recommended answers

The maintainer settles these by reviewing this ADR. Each is a recommendation until then.

1. **Where Zèrtz's source lives.** *Recommended: the existing private overlay repository*, under `packs/zertz/`. A
   checkout of the overlay sits inside bingle's module (ADR 0127), so its packages may import
   `internal/engine/wasmrules/guest` and the engine can stay under `internal/`. A repository of its own would be a
   module of its own, and the guest package would then have to move to a public import path with a promise of
   stability. That is the step the overlay was chosen to avoid, and it is premature for a proof of concept. Nothing
   in the server imports the pack: its rules package carries `//go:build private || wasip1` and its `main` carries
   `//go:build wasip1`, so a public build compiles neither, and a private build compiles the rules only for its
   native tests.
2. **wazero as a twelfth direct dependency.** *Recommended: accept it for the proof of concept.* It is pure Go,
   brings no dependency of its own, and is the only way to run WebAssembly without cgo. If phase 5 drops packs, it
   goes again. Phase 1 updates the count where CLAUDE.md, the README and `docs/build.md` give it.
3. **A directory at start-up, or more.** *Recommended: a directory at start-up is enough.* Zips, an upload, an
   admin page and installing on a running server are what ADR 0128 was withdrawn for. Phase 5 may propose them
   only with the numbers behind them.
4. **How a pack's games are marked.** *Recommended: `private: true` in the manifest sets `catalog.Module.Private`*,
   and so `private: true` in the API (ADR 0140). A client deciding what to hide needs the same answer for a
   commercial game whether it came from the overlay or a pack. A pack without it is public. No new API field is
   added during the proof of concept, so `version.API` does not move.
5. **Which rulebook settles Zèrtz's rules.** *Settled by the maintainer: the 2016 Rio Grande Games edition.* Its
   rulebook settles every rule, and the game's guide names the edition, as Cthulhu Wars is settled by one named
   rulebook. The 48- and 61-ring boards and Blitz follow it where it gives them, and the GIPF project's published
   tournament rules where it does not.

### How it ends

Phase 5 reports, each against the native figure for the same rules:

- the cost per call of `Legal` and `Apply`;
- positions searched per second through the adapter and inside the guest, and bot strength at equal time;
- practice batch throughput;
- module size, start-up time with a pack, and memory per instance and for a full pool;
- what it cost: lines of host code, test time, and anything that had to change in the engine or the page;
- what did not fit: anything Zèrtz needed that a pack could not do.

The report closes with a recommendation to keep and extend, keep as is, or drop. **To drop** is to remove
`internal/engine/wasmrules`, the pack loader, `BINGLE_PACKS` and wazero, to move Zèrtz into the overlay through the
in-process adapter, and to mark this ADR withdrawn.

## Consequences

- Phases 1 (#296) to 4 (#299) build what this describes, each recording here anything it changes. Phase 5 (#300)
  sets this ADR's status.
- Until the verdict, the overlay stays the way a commercial game is built. Nothing moves out of it to become a pack.
- While the proof of concept runs, the plugins plan's decision 1 and ADR 0128's withdrawal stand for every game but
  Zèrtz.
- A pack's game shows what the guest's calls cost each time a page asks for its selections. Bots and practice
  batches on a pack's game are expected to be slower than on a compiled-in game until phase 4, and phase 4 may
  show that they stay so.
- ABI v0 promises nothing. A pack is rebuilt when the ABI changes, and only when this ADR is accepted does an ABI
  version become a promise.
