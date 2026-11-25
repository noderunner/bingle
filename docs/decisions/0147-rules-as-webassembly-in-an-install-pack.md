# 0147. An install pack: rules as WebAssembly, and every client's files from the server (proof of concept)

Status: proposed (2026-11-24), phase 0 of #294. A proof of concept, not a direction: it ends with the maintainer's
verdict (#300), and if that verdict is to drop it, what it built is removed again. It has two halves, judged
apart: a game's **rules** as a WebAssembly module, and a game's **files for every client** carried in the pack and
handed out by the server. It reopens decision 1 of the game plugins plan
([docs/history/game-plugins-plan.md](../history/game-plugins-plan.md)) and the withdrawn
[ADR 0128](0128-data-packs.md), settles for packs what that plan left open about a non-web client's assets, and is
a third transport for the cut of the game services proposal
([docs/history/game-services-proposal.md](../history/game-services-proposal.md), #241). It relies on ADR
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

The second half is about what a player needs beside the server. The web page gets a game's look from the server
(`web.yaml`, ADR 0124). The native client, [bingle-3d](https://github.com/noderunner/bingle-3d), compiles its
models, textures and sounds in, and reads a private game's look from a folder on the player's own machine, so every
player of a private game needs that folder as well as the client. If the pack carries each client's files and the
server hands them out, **one generic build of a client can play a private game on any server that offers it**.

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
- **Plugins plan, on a non-web client's assets**: "How assets reach a non-web client is deliberately not decided.
  Whether the Bevy client downloads assets from the server, ships its own, or both, is for the discussion when
  that client is built." ADR 0128 left it open again. The client is built now, and its own plan says assets ship
  with it and "downloading from the server waits until a data pack needs it". This ADR decides it for packs: both.
  A client ships what every game shares, and downloads what a pack brings.
- **The services proposal** cuts at a player's action (`Submit` per impulse, `Bot` per decision), so that nothing
  fine-grained crosses. Phase 1 deliberately cuts lower, at `state.Ruleset`, because then the session, API, MCP,
  bots and web page work unchanged and the cost of the fine-grained boundary is measured rather than argued.
  Phase 4 adds the proposal's coarse `Bot` call. If the verdict is to keep packs, a later ABI should move to the
  proposal's other coarse calls, so that one interface has three transports: in process, a socket and
  WebAssembly.

## Decision

### The runtime

**[wazero](https://github.com/tetratelabs/wazero)**, pinned to a tagged release (1.12.0). It is pure Go, and the
one module it requires, `golang.org/x/sys`, was in the build already, so `CGO_ENABLED=0` and the single binary
survive (ADR 0014). It adds one direct dependency and no indirect ones. A module is compiled once, ahead of time, where wazero has a compiler (amd64 and arm64), and is
interpreted elsewhere. It is compiled when its pack loads and kept in memory. The proof of concept keeps no
on-disk cache.

### The guest

A pack's rules are **standard Go**: `GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared`, exporting its calls
with `//go:wasmexport`. That makes it a reactor module: the host calls `_initialize` once and then calls the
exports. No other toolchain is needed. TinyGo stays an option if module size or start-up time proves to be the
problem (the last phase reports both).

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
| `bingle_alloc` | `(size u32) → u32` | Memory for a request, which the host writes into. The host may keep it and write its next request there: a guest keeps nothing of a request |
| `bingle_free` | `(ptr u32, size u32)` | Frees a request or a reply |
| `bingle_init` and one export per call | `(ptr u32, len u32) → u64` | Reads a JSON request at `ptr`, returns its JSON reply as `ptr << 32 \| len`. The host copies the reply and frees it |

The calls are `init`, `start`, `impulse_seat`, `legal`, `apply`, `format`, `parse`, `gestures`, `validate` and
`outcome`, one for each method of `state.Ruleset`. Phase 4 adds `bot`. Only the Ruleset's nine methods are
bridged. Every optional interface (`HiddenInfo`, `Reactor`, `Heuristic`, `PlaneEncoder` and the rest) is left
out until a phase needs it, and a phase that adds one records it here. Phase 1 adds `SeededRuleset`, which costs
no call: the adapter says that a start is recorded with its game, because what `start` does with its seed is the
module's own business, and a replay must not ask for the start again.

- **Every reply is an envelope**: `{"ok": …}` or `{"error": "…"}`. An error is the call's error (an illegal
  selection, unparsable notation, a bad custom start). `ok` is never null: no selections is `[]`, and `validate`
  replies `true`. A reply that is not JSON, or not an envelope, counts as a failure of the instance (see below),
  and so does one that makes no sense for the game, such as a next seat the game does not have.
- **`init` carries games**: `{"abi": 0, "games": […]}`, each game with its `id`, its `definition` as the catalog
  compiled it, and its `board` as the catalog built it (`kind`, `width`, `height`, each of its `positions` with its
  `id`, its `xy` coordinate where it has one and its `neighbors`, and the `regions`). A guest never compiles a
  definition or a board itself. No other call carries the definition. **An instance is told of a game before the
  first call that names it** (changed in phase 1, which first read "once per instance, every game of the pack"):
  `init` adds its games to those the instance knows, so nobody lists a pack's games ahead, a game derived from
  one of them by a setup option is told like any other, and rules that refuse a game fail that game alone.
- **Every other request names its game** by id, and carries what the Ruleset method takes:
  `{"game", "position", "seat", "selection", "text", "seed"}`, each present where the call takes it.
- **A position crosses as its document**, exactly as `BoardState.MarshalJSON` writes it (`v: 2`: cells, reserves,
  ply, turn, counters and `ext`), and comes back the same way. The host reads it with the engine's own decoder.
  **The guest's own data rides in `ext`, which the host never decodes**: it stays raw JSON in the `BoardState`, is
  stored as it is, and is handed back on the next call.
- **Selections and gestures** cross with the JSON of `state.Selection` and `state.Gesture`.
- **`apply` replies with the result**: the new position and the fields of `state.MoveResult` (`captures`,
  `next_seat`, `notation`, `facts`, `ended`, `winner`, `drawn`, `all_lost`, `reason`, `detail`). `outcome` replies
  with the fields of `state.Outcome`. A `winner` or a `next_seat` of 0, or none, is no seat. Where an impulse ends is worked out by the host from the reply, as for any
  ruleset (`state.ImpulseEnds`). The guest does not report it.

### The sandbox

- **No host functions but WASI's**, and of WASI only what the Go runtime needs: the sixteen functions that a Go
  module which opens no file and no connection imports. A module that imports anything else is refused when it
  loads. There are no preopened directories, no environment and no arguments, and there are no sockets (wazero
  has none unless a host adds them). Stdout and stderr go to the server's log at debug level, cut short, and the
  start of what a failed call wrote is in its error, which is where a Go panic says what it was.
- **No real clock and no real randomness.** Wall time and the monotonic clock are fake ones, not the
  machine's, and `random_get` reads from a fixed seed. The clocks move a millisecond with every call into an
  instance and a nanosecond with every reading, so nearly all of an instance's time passes between its calls. A
  clock that moves at each reading instead, which is wazero's default, must not be used with a Go guest: see the
  status log. All of this is set explicitly rather than left to wazero's defaults. A
  game's chance comes only from the `seed` that `start` is given, kept in its position, as for a compiled-in
  ruleset. Go randomises map order inside the guest from that seed, so a guest must not let map order reach a
  reply. Phase 1's tests compare a fresh instance with a reused one to catch it.
- **A memory cap per instance**, by default 2048 pages (128 MiB). Phase 1 measured the fixture's instance at 3.6
  MiB once started and 8 MiB after 20,000 small calls, so rules alone would fit in a quarter of it. The cap stays until
  phase 4 has put a search inside.
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
  blank. **`ImpulseSeat` answers 0** (settled in phase 1). The engine does not read that as the end of the game,
  which only `Apply`'s result and `Outcome` decide. Its two callers read it as nothing to say: `state.StartSeat`
  then names the first seat, and `state.ImpulseAfter` the seat that last moved. At worst a new game waits for a
  seat its module is not waiting for, and has to be made again. Phase 1 tests each of these.

### A pack

A pack is a **directory**:

```text
zertz/
  manifest.yaml   name, version, abi, private
  game.yaml       an ordinary definition with engine: ruleset, and its variants
  web.yaml        optional: its look (ADR 0124)
  rules.wasm      the rules
  guide/          the guide's Markdown and pictures
  clients/        optional: a folder of files for each client that is not the web page
    bingle-3d/    that client's look, laid out as it lays out its own assets
```

- **The definition needs no new field.** `engine: ruleset` says the rules are a ruleset, and the loader supplies
  the adapter as the module's `Rules`.
- **Names and versions are ADR 0128's.** The family is `pack-<name>`, each version is a game of its own
  `pack-<name>-<version>` with variants `pack-<name>-<version>-<word>` (ADR 0098), and the family's name means the
  newest. The loader sets `id`, `family`, `version` and `prototype` from the manifest and refuses a definition that
  sets them. A variant is named `<name>-<word>` in `game.yaml` and `web.yaml`, as ADR 0128 had it (phase 2). A pack's game is a prototype, so it is not rated. No built-in game may use the prefix, and
  `tests/registry` holds that.
- **Two versions can sit side by side** as two directories. New games use the newest, and a game in progress
  finishes on the rules it started with.
- **The guide is the game's files**, rooted at `guide/` as a built-in game's guide folder is, so a definition and a
  look name a file by its path inside it. Nothing else of the pack is served: not its manifest, definition, look
  or rules (phase 2).
- **A pack can carry no page script, and no look's `script` or `style`.** What it draws is what the page already
  draws (ADR 0128's checks, below). A game that needs its own drawing still needs a build.

### Every client's files

**A pack carries the files each client draws its game with, and the server hands them out.** A client ships what
every game shares and fetches the rest.

- **A folder per client**, `clients/<client>/`, named by the client. What is inside is that client's own: for
  bingle-3d its appearance file, models, textures, sounds and pictures, laid out as its own `assets/` is. The web
  page's files stay where they are (`web.yaml` and `guide/`), because the server reads and checks those.
- **The server does not read a client's files.** ADR 0124 stands: the server says what things are, and each
  client decides how they look. A look in a pack is still the client's, written in the client's format by whoever
  made the pack. The server only keeps it and delivers it.
- **What the server checks without reading them**: no path or link that leaves the folder; a size limit per
  file, per folder and on the number of files (by default 64 MiB, 512 MiB and 4,096, until phase 5 has measured);
  and a list of kinds it will serve at all: TOML, JSON, plain text and Markdown, binary glTF, KTX2, PNG, JPEG and
  Ogg, each sniffed where its format has a signature. Nothing else is accepted: no script, shader, HTML, SVG or
  program. A pack with a client folder that fails is refused whole, as for any other check.
- **An index, then files.**
  - `GET /api/game-defs/{id}/clients/{client}` lists the client's files for a game: each one's path, size,
    SHA-256 and content type.
  - `GET /api/game-defs/{id}/clients/{client}/files/{path}` sends one. Its hash is its `ETag`, `If-None-Match`
    is honoured, and byte ranges are served, so a large download can be resumed.
  - A game's info names the clients it has files for (`clients: ["bingle-3d"]`), so a client knows from the list
    of games, without asking game by game.
  - Every game of a pack (its variants) offers the same files.
- **Files are served from disk and never held in memory.** Each is hashed when the pack loads and opened through
  a root confined to the pack. They are sent inert, as a guide's pictures are (`nosniff`, a sandboxing
  `Content-Security-Policy`), and as attachments, so a browser never renders one.
- **A client fetches what it lacks and keeps it by its hash.** It asks only for a game the player opens, or lists
  with a picture. A file two variants, two versions of a pack or two servers share is downloaded once.
- **A client treats what it downloads as hostile**, since a player may connect to a server they have no reason
  to trust. Each file is checked against its hash and the client's own limits. A look is data and never code. It
  adds to the client and never replaces a file the client ships or another game's. A look the client cannot read
  (one made for a later format, say) is left out, and the game is drawn from the defaults that every game can be
  drawn from. All of this is the client's to hold, in phase 6 (#303).
- **The API speaks of games, not packs.** The files belong to a `catalog.Module`, as its guide and its `web.yaml`
  do, so a compiled-in game or one of the overlay's could offer client files the same way. The proof of concept
  wires only packs. The additions are a minor `version.API`, made in phase 5 (#302).

### Loading

- **`BINGLE_PACKS`** names a directory of packs, read once at start-up. Configuration comes from the environment
  (`internal/config`), so there is no flag. Unset means no packs and nothing changes. To change a pack, change the
  directory and restart.
- **The operator chooses what is in the directory, but its contents are checked as though they came from anywhere**,
  since a pack is meant to be handed from one server to another. The checks are ADR 0128's, less what only a zip
  needed: the manifest and ABI version, a definition that compiles, a look that fits it (`look.Check`), no `script`
  or `style`, colours that are colours, pictures sniffed, text that is text, no path or link that leaves the pack,
  size limits on what is really read, and a short random trial of each game, in which a pack that panics or thinks
  far too long is refused. A client's folder gets the checks of *Every client's files*.
- **A pack that fails is logged with its reason and left out.** The server starts without it.
- **A pack that was there and is gone** leaves its games as a retired game's (ADR 0076). They stay readable, and
  nothing new is started. **Its unfinished games are not ended** (settled in phase 2), unlike a retired game's: a
  pack may be back at the next start, after a fix or a rebuild for a new ABI, and its games then play on. The
  server keeps each pack game's name for this (`pack_games`).

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
   brings no dependency of its own, and is the only way to run WebAssembly without cgo. If the verdict drops the rules half, it
   goes again. Phase 1 updates the count where CLAUDE.md, the README and `docs/build.md` give it.
3. **A directory at start-up, or more.** *Recommended: a directory at start-up is enough.* Zips, an upload, an
   admin page and installing on a running server are what ADR 0128 was withdrawn for. The verdict may propose them
   only with the numbers behind them.
4. **How a pack's games are marked.** *Recommended: `private: true` in the manifest sets `catalog.Module.Private`*,
   and so `private: true` in the API (ADR 0140). A client deciding what to hide needs the same answer for a
   commercial game whether it came from the overlay or a pack. A pack without it is public. Marking adds no API field. The only
   API additions of the proof of concept are those of *Every client's files*.
5. **Which rulebook settles Zèrtz's rules.** *Settled by the maintainer: the 2016 Rio Grande Games edition.* Its
   rulebook settles every rule, and the game's guide names the edition, as Cthulhu Wars is settled by one named
   rulebook. The 48- and 61-ring boards and Blitz follow it where it gives them, and the GIPF project's published
   tournament rules where it does not.

6. **Who may fetch a private game's client files.** *This needs the maintainer.* The recommendation is the same
   standing as the game's guide pictures and `web.yaml`, which the API already gives to anyone who can reach it:
   who can reach a private server is its operator's responsibility. The plugins plan asked for it to be written
   down that "bingle ships no mechanism for distributing someone else's game", and a server that hands a
   commercial game's models to every client that asks comes closer to one than the page's pictures did. The
   stricter choice is that a game marked `private` gives its client files only to a signed-in account.
7. **Whether the web page's files move under `clients/web/`.** *Recommended: no.* The server reads and checks
   `web.yaml` and the guide, and nothing reads a client folder. Two kinds of file with two kinds of checking are
   clearer in two places.

### How it ends

The last phase (#300) reports. For the rules, each against the native figure for the same rules:

- the cost per call of `Legal` and `Apply`;
- positions searched per second through the adapter and inside the guest, and bot strength at equal time;
- practice batch throughput;
- module size, start-up time with a pack, and memory per instance and for a full pool;
- what it cost: lines of host code, test time, and anything that had to change in the engine or the page;
- what did not fit: anything Zèrtz needed that a pack could not do.

For the client files:

- the size of a pack's client folder, and how long a first game takes to open over a home connection and a slow
  one, against opening it from the client's own assets;
- what a second open, another variant and another server cost, with the files already kept;
- the server's memory and disk for it, and what serving a file costs;
- what the client had to refuse or limit, and anything a look needed that data alone could not give.

The report closes with a recommendation for each half: keep and extend, keep as is, or drop. The halves stand
apart. Client files could be kept for compiled-in and overlay games with the rules dropped, and the rules kept
with each client still shipping its looks. **To drop the rules** is to remove `internal/engine/wasmrules` and
wazero and to move Zèrtz into the overlay through the in-process adapter. **To drop the client files** is to
remove the index, the file route and `clients` from a game's info, and for bingle-3d to go back to its private
folder. To drop both removes the pack loader and `BINGLE_PACKS` too, and marks this ADR withdrawn.

## Consequences

- Phases 1 (#296) to 4 (#299) build the rules half, and phases 5 (#302) to 7 (#304) the client files, each recording
  here anything it changes. The last phase (#300) sets this ADR's status.
- Until the verdict, the overlay stays the way a commercial game is built. Nothing moves out of it to become a pack.
- While the proof of concept runs, the plugins plan's decision 1 and ADR 0128's withdrawal stand for every game but
  Zèrtz.
- A pack's game shows what the guest's calls cost each time a page asks for its selections. Bots and practice
  batches on a pack's game are expected to be slower than on a compiled-in game until phase 4, and phase 4 may
  show that they stay so.
- **A game's look now travels with the game, and has to follow the client's format.** A pack made for an older
  format of a client's look, or a newer one, is drawn from the defaults until it is rebuilt. A client that changes
  its format often makes packs stale often.
- **The server becomes a host for large files**: tens of megabytes a game, where a guide's pictures are tens of
  kilobytes. They cost disk and bandwidth, though not memory.
- **A client reads files from servers it does not know.** Its decoders for models, pictures and sound become
  something a hostile server can aim at. Hashes, limits and data-only formats narrow that and do not remove it.
- bingle-3d's plan changes with this: assets still ship with the client (its decision 1), and a look may now come
  from the server as well. Its private folder (decision 2) stays for the overlay's games and for making a look.
- ABI v0 promises nothing. A pack is rebuilt when the ABI changes, and only when this ADR is accepted does an ABI
  version become a promise.

## Status log

- 2026-11-24, **phase 1** (#296): the host (`internal/engine/wasmrules`), the guest glue (`wasmrules/guest`),
  `wasmrules.Native` and a public fixture, English draughts with each jump of a chain a selection of its own
  (`tests/testdata/draughts`). Nothing loads a module yet. What it changed is marked above: an instance is told of
  a game before the first call about it, `ImpulseSeat` answers 0 when its call fails, a start is always recorded,
  and the list of WASI functions.
  - **What a call costs**, on the fixture's 8×8 game twenty selections in, one Intel i9-9900K core, Go 1.27.1 and
    wazero 1.12.0 (`go test ./tests/wasmrules -run xxx -bench . -benchmem`). *Linked in* is `wasmrules.Native`.
    *JSON only* is the same requests and replies written and read in process, with no module. *No deadline* is
    the module run without the checks that let a call be stopped. The last column is the module as it was first
    built, with `encoding/json` in the guest.

    | Call | Linked in | JSON only | Module | Module, no deadline | Module, `encoding/json` in the guest |
    |---|---|---|---|---|---|
    | `Legal` | 6.1 µs | 22 µs | 326 µs | 113 µs | 830 µs |
    | `Apply` | 11.3 µs | 45 µs | 563 µs | 205 µs | 1,288 µs |
    | `Format` (carries next to nothing) | 0.06 µs | 1.6 µs | 29 µs | 10 µs | 91 µs |

    A call through the module costs about 50 times the same call linked in, and the plugins plan's objection
    holds as it was written: a search that makes a million `Legal` calls takes five and a half minutes where it
    took six seconds. A person's move is a handful of calls, and a third of a millisecond each is nothing to them.
  - **The guest reads and writes its JSON by hand**, which is where the last column went. With `encoding/json`,
    which works by reflection, a call cost 115 to 135 times the same call linked in, and nearly all of that was
    the guest's side of the JSON. A reader and a writer for the wire's few shapes (`guest/codec.go`, held to
    `encoding/json` by a fuzz target) make a call 2.3 to 3.1 times faster. The wire is the same, and `init`,
    which an instance hears once for a game, still uses `encoding/json`.
  - **Where the time goes now.** Entering a module is cheap: an export that does nothing returns in 24 ns, or
    290 ns with a deadline to watch. Of a `Legal` call, about half is reading the request, nearly all of that the
    position; two fifths is the fixture's rules; a tenth is writing the reply. All of it is
    ordinary Go that runs ten to twenty times slower compiled to WebAssembly under wazero than natively: the
    garbage collector is not the cause (turning it off saved a twentieth). **The deadline costs a factor of 2.8.**
    wazero stops a call by having the module's code leave for the host at the top of every loop to ask whether
    it has been closed.
  - **What that leaves for later phases to try**, none of it done here: TinyGo, which generates different code;
    a position that crosses as something cheaper to build than a map; and a game reading its own data by hand,
    which was tried on the fixture and gave 7%. Phase 4's search inside the module will pay the module's slowness
    and the deadline's factor on every node, so its own measurement matters more than this one.
  - **The module**: 4.6 MB of WebAssembly for the fixture, 1.4 s to compile when it loads, 3.8 ms for each further
    instance. A compiled module is kept in memory only, and shared by every runtime of the process that loads it.
  - **A fake clock can starve a Go guest**, found after the first push and fixed. The clocks first moved a
    millisecond at each reading, as wazero's own fake clocks do. Go's collector reads the clock several times in a
    cycle and the code between cycles never does, so the collector saw itself using the whole processor and
    stopped making the allocating code help it. Nothing else runs it inside a call, so the heap then grew to the
    memory cap: after some tens of thousands of calls an instance that had held 8 MiB held all it was allowed,
    and died. The pool replaced it, so no test noticed. With clocks that move between calls an instance stays at 8 MiB,
    and a test now churns two megabytes a call through one that may hold sixteen.
  - **In the engine**, one addition: `BoardState.Counters`, so that the linked-in adapter can hand a position's
    counts to rules.
  - **Open, for phase 2**: `state.InitialState` turns a start that fails into an empty position, on the ground
    that a registered ruleset's start cannot fail. A module's can (a deadline under load). The session should
    refuse to create the game instead.
- 2026-11-25, **phase 2** (#297): packs load from `BINGLE_PACKS` at start-up (`internal/pack`, documented in
  [docs/packs.md](../packs.md)). Each is read through an `os.Root` confined to its directory, checked, played a
  trial game and registered beside the built-in modules. What it settled is marked above: a variant's name in a
  pack, the guide as the game's files, and a gone pack's unfinished games left as they stand.
  - **The checks** are ADR 0128's, kept as its reader had them where a directory needs the same: the limits per
    file (8 MiB), in all (40 MiB) and on the number of files (300), with `rules.wasm` allowed 64 MiB apart; text
    and pictures sniffed; colours held to a colour's form; a look with no `script` or `style`; at most 64
    variants and 1 MiB of definition. A directory adds no link anywhere, and no file or folder beside those of the
    layout. Names starting with a dot (`.git`, `.DS_Store`) are neither read nor served.
  - **The trial** is 300 random selections from a fixed seed within ten seconds. A call into a module that fails
    answers with nothing rather than an error (*Instances and failures*), so the trial reads the module's count of
    thrown-away instances (`Module.Faults`) to tell rules that failed from rules with nothing to offer.
  - **A start that fails** now fails the game's creation (`state.Start`, `session.ErrStartFailed`, a 500), which
    closes phase 1's open point.
  - **Nothing new in the API**: a pack's game is listed with `prototype` and, from `private: true`, `private`,
    which the API already sent.
