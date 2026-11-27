# Report: Zèrtz as a WebAssembly install pack

Status: **decided** (2026-11-27): the maintainer agreed with both recommendations. Written 2026-11-26,
phase 8 of #294 (#300). The design is
[ADR 0147](../decisions/0147-rules-as-webassembly-in-an-install-pack.md). Its two strategies are judged apart:
the **rules** as a WebAssembly module, and every **client's files** carried in the pack and served.

## In short

| Strategy | Recommendation | Why |
|---|---|---|
| Rules as WebAssembly | **Keep as it is**: packs for small games, the overlay for the rest | It works and Zèrtz plays well. A pack's rules stay 27 to 66 times slower a call, nothing needs a pack today, and most commercial games would need much more of the ABI |
| Client files in a pack | **Keep and extend** to the overlay's games | It costs about 500 lines and no dependency, it was free to run at Zèrtz's size, and the client needed no code of its own for Zèrtz |

## How it was measured

On 2026-11-26, on public `main` at e59f0a2 and the overlay at 2a7a550: one Intel i9-9900K (8 cores, 16
threads), Linux, Go 1.27.1, wazero 1.12.0 and TinyGo 0.44.0. Zèrtz is on its 37-ring board unless said otherwise.
**Go** and **TinyGo** are the module each toolchain builds, with the bots' search inside it.

- Rerun: both reports (`BINGLE_WASM_REPORT=1`, `tests/wasmrules` and the overlay's `tests/zertz`) and
  `go test ./tests/wasmrules -run xxx -bench .`.
- Measured once for this report, with throwaway code that is not committed: Zèrtz's calls, memory and practice
  batch; the server's start-up, memory, selections and file routes; and the slower links.
- Timings are single runs, except the benchmarks' own averages and the medians of the server and link figures.
  Expect them to move by a tenth or so, and a 20-game match by much more.

### What "linked in" means

The native figure is the same rules package linked into the server through `wasmrules.Native`, searched by the
host's bots. **It is not the fastest these rules could be.** The adapter converts the position at every call. On
Draughts, the guest's search over the same rules, natively and on its own state, looks at 1,493,000 positions a
second in minimax and plays 29,500 MCTS playouts a second, against 90,700 and 2,340 linked in. A ruleset written
straight against the engine would fall somewhere else again, and was not built. So where a pack's bots come near
"linked in" below, the module is still about 20 times slower than the same search compiled natively.

## The rules

### A call across the boundary

| | Linked in | Go | TinyGo |
|---|---|---|---|
| Draughts `Legal` | 4.0 µs | 267 µs (66×) | 157 µs (39×) |
| Draughts `Apply` | 8.7 µs | 479 µs (55×) | 265 µs (30×) |
| Zèrtz `Legal` | 11 µs | 580 µs (52×) | 399 µs (36×) |
| Zèrtz `Apply` | 27 µs | 1,063 µs (39×) | 721 µs (27×) |

A player's move is a handful of calls. The one place a player could feel it is the list of selections, which makes
three calls for each option. Over the API, for a Zèrtz placement:

| | Options | Go | TinyGo |
|---|---|---|---|
| 37 rings | 111 | 147 ms | 128 ms |
| 61 rings | 183 | 322 ms | 210 ms |

Phase 3 found about 1.5 ms for the same listing inside the engine, which was not measured again.

### Positions searched

`across` is the host's bots searching through `legal` and `apply`, as before phase 4.

| Draughts | Linked in | Across, Go | Inside, Go | Inside, TinyGo |
|---|---|---|---|---|
| tactical, a decision | 0.38 ms | 21 ms | 1.8 ms | 0.80 ms |
| minimax, positions/s | 90,700 | 1,800 | 39,000 | 73,600 |
| MCTS, playouts/s | 2,340 | 37 | 633 | 1,240 |

| Zèrtz | Linked in | Across, Go | Inside, Go | Inside, TinyGo |
|---|---|---|---|---|
| tactical, a decision | 24 ms | 930 ms | 146 ms | 87 ms |
| minimax, positions/s | 33,000 | 910 | 7,800 | 14,000 |
| MCTS, playouts/s | 706 | 9 | 165 | 373 |

Zèrtz's move to a state of its own (#326) made it faster linked in too, about twice, so the module gained less on
native than on itself.

### Strength at equal time

MCTS at 250 ms a move, 20 games with the seats alternated: wins, losses and draws of the side named first.

| | Draughts | Zèrtz |
|---|---|---|
| across, Go, against linked in | 0–20–0 | 0–20–0 |
| inside, Go, against linked in | 2–11–7 | 7–13–0 |
| inside, TinyGo, against linked in | 6–10–4 | 7–13–0 |
| inside, Go, against across | 20–0–0 | 19–1–0 |
| inside, TinyGo, against inside, Go | 15–1–4 | 19–1–0 |

**A pack's bot is still the weaker one.** Phase 4c recorded 5–5–10 for TinyGo against linked in on Draughts and
called it level. This run gave 6–10–4. On Zèrtz, phase 4c's follow-up recorded 9–11, and this run 7–13. Twenty
games cannot tell small differences apart, and neither run shows a pack's bot ahead.

### Practice batches

MCTS at 30 iterations against minimax at 500 positions, 16 games on 8 workers.

| | Linked in | Across, Go | Inside, Go | Inside, TinyGo |
|---|---|---|---|---|
| Draughts, games a minute | 282 | 6.5 | 87 | 160 |
| Zèrtz, games a minute | 76 | not run | 82 | 186 |
| Zèrtz, plies a second | 61 | not run | 66 | 148 |

On Zèrtz a batch through a TinyGo module is faster than linked in. That is the adapter's cost showing (*What
"linked in" means*), since inside the module the search converts nothing. The sides also search differently, so
their games are not the same games.

### Size, start-up and memory

| | Draughts, Go | Draughts, TinyGo | Zèrtz, Go | Zèrtz, TinyGo |
|---|---|---|---|---|
| module | 4,733 KB | 776 KB | 4,843 KB | 823 KB |
| load (compile and first instance) | 1.55 s | 0.29 s | 1.57 s | 0.31 s |
| a further instance | 56 ms | 10 ms | 59 ms | 10 ms |
| an instance after a game | 6.0 MiB | 0.4 MiB | 8.2 MiB | 0.8 MiB |
| a full pool of 32 instances | not run | not run | 282 MiB | 24 MiB |
| the process's growth to fill that pool | not run | not run | 699 MiB | 52 MiB |

The pool holds up to twice the processor's threads. The last row is resident memory after 96 games at once, over
what the process held before it loaded the module, so it includes wazero's compiled code and Go's own heap.

The server as a whole, median of five starts, until `/healthz` answers, and resident memory a second later:

| | No packs | Zèrtz, Go | Zèrtz, TinyGo |
|---|---|---|---|
| start-up | 137 ms | 2,170 ms | 870 ms |
| idle memory | 64 MiB | 161 MiB | 110 MiB |

wazero's own code is about 1.2 MB of the 45 MB binary.

### What it cost

Lines added by the proof of concept's own pull requests (#305, #311, #320 and #323 to #327), counted from their
diffs. The first four rows are the files as they stand.

| | Lines |
|---|---|
| the host, `internal/engine/wasmrules` | 1,470 |
| the guest glue, `wasmrules/guest` (812 of them its JSON codec) | 2,010 |
| the pack loader, `internal/pack` less the client files | 750 |
| the client files, server side | 500 |
| elsewhere in the engine, session, store, bots, app and scripts (added) | about 440 |
| tests (added) | about 3,700 |
| test fixtures (added) | about 1,360 |
| Zèrtz in the overlay: rules and pack code, and its tests | 1,480 and 1,420 |

- **Dependency:** wazero, the twelfth direct one. It brings no indirect one that was not there already.
- **Test time:** `tests/wasmrules` takes 18 s and `tests/pack` 14 s here with a warm build cache, run side by
  side, and the overlay's Zèrtz tests 20 s. The smoke test also builds and loads the fixture pack. CI has no
  TinyGo, so **nothing in CI tests a TinyGo module**.
- **Changed in the engine:** a position's counters are readable (`BoardState.Counters`); a start that fails now
  fails the game's creation; one optional interface, `state.Searcher`, with the bots handing it their decision; the
  store remembers pack games' names, so a gone pack's games stay readable.
- **Changed for what Zèrtz needed:** grey pieces on the page (#313), and a game's own wording of its endings, in
  the definition, the API and the page (#314). Both help every game.

### What did not fit

| What | Where it stands | What it would take |
|---|---|---|
| Grey marbles | fixed, #313 | done |
| Zèrtz's own endings | fixed on the page, #314. bingle-3d still shows `won (marbles)` | an issue in bingle-3d |
| The selections view is slow, 0.1 to 0.3 s | open | a coarse `selections` call that formats, gives gestures and counts captures in one crossing |
| A free board shows no coordinates | open; the notation is read against the guide's picture | a page option, for any free board |
| A pool both players share has no place of its own | worked around: the pool is cells beside the board | a shared reserve in the engine and the page, if a second game needs one |
| Setup options, callbacks, callback win conditions | refused by the loader | options need a call that derives a game; callbacks are Go and stay out |
| Page scripts and styles | refused, on purpose | nothing: a game that needs them needs a build |
| Tutorials | the pack's layout has no place for them | a folder the loader checks and registers |
| Hidden information, reactions, training planes and action lists, and the other optional interfaces | not bridged; Zèrtz needs none of them | for each, a guest interface, a wire shape, an export and the adapter's side. Hidden information also needs its leak tests run through a module |
| Building a pack outside this module | not possible: the guest package is under `internal/` | moving it to a public path with a promise of stability. Today only a fork or the overlay can build a pack |
| Speed on other processors | not measured | wazero interprets where it has no compiler (anything but amd64 and arm64) |

What #294 set out to show did work: an unmodified public build plays Zèrtz, with no overlay compiled in.

### Against the private overlay, for the same game

| | Overlay | Pack |
|---|---|---|
| To run it | a private build of the server | any public build and a directory |
| Speed | native | 27 to 66 times slower a call; bots weaker at equal time |
| If the rules misbehave | they run with the server's full access, and a runaway search is the server's | no files, network or clock; a memory cap and a deadline on every call |
| What a game may use | all of the engine and the page, scripts and tutorials included | the nine `Ruleset` methods and the bots' search; a look that is data |
| Writing the rules | against the engine | against the guest's own types. For fast bots, also a state of its own |
| Two versions side by side | two packages in one build | two directories |
| Changing the engine | a compile error shows what broke | a change to the wire breaks old modules, which are rebuilt |

The overlay still needs everything it needed before: no game moved out of it.

### Against the out-of-process transport (#241)

The [services proposal](game-services-proposal.md) is not built, so there are no numbers for it.

| | Out of process (proposed) | WebAssembly (built) |
|---|---|---|
| Where it cuts | a player's action: `Submit`, `Selections`, `Bot` | the `Ruleset`'s methods, and `Bot` |
| Bots and batches | native, since the search stays in the game's process | inside the module: on Draughts, about 20 times slower than the same search natively |
| Optional interfaces | all of them, since they stay on the game's side | each one bridged by hand |
| Isolation | what the operating system gives, set up by the operator | the sandbox, with nothing to set up |
| Shipping a game | a binary for each platform | one module for every platform |
| Cost to build | the platform refactored behind `GameService`, a public engine module, process supervision | done, for the `Ruleset` |

The two are not rivals: the proposal's coarse calls could run over either. If packs grow, a later ABI should move
to those calls, which would also fix the selections view.

## The client files

### The numbers

Zèrtz's folder for bingle-3d, the same for its four games:

| File | Bytes |
|---|---|
| `models/zertz/marble.glb` | 147,980 |
| `pictures/zertz.jpg` | 44,531 |
| `appearance/zertz.toml` | 2,002 |
| **all** | **194,513** |

The index is 521 bytes. **Opening a game for the first time** fetches the index and the three files, one request
after another on one connection. The slower links were made with a small proxy that delays and paces the bytes.
It does not copy TCP's slow start, packet loss or TLS, so real links will be somewhat slower. These are the
transfers alone, without the client's reading of the files:

| Link | First open | Kept, one index request |
|---|---|---|
| loopback | 0.001 s | under 0.001 s |
| home: 100 Mbit/s down, 20 ms round trip | 0.10 s | 0.02 s |
| slow: 5 Mbit/s down, 150 ms round trip | 0.93 s | 0.15 s |

- **From the client's own assets** there is nothing to fetch. Phase 7 timed a whole first open on loopback, with a
  release build of bingle-3d, at 0.16 s. So nearly all of that is the client's own work, and the transfer is what a
  slower link adds. The client itself was not run through the proxy.
- **A second open** in the same run asks for nothing, since bingle-3d keeps the index too. **After a restart,
  another variant or another server** with the same pack costs the index and no file, since files are kept by their
  hash. Phase 7 checked the second server by hand.
- **The server:** the folder's 195 KB on disk. Files are hashed once as the pack loads and are never held in
  memory. Start-up with and without the folder took the same time (868 and 867 ms). Idle memory was 118 MiB without it and
  110 MiB with it, so the difference is noise. A file was served in about 0.3 ms on loopback, as timed by the script that
  asked for it, whether it was 2 KB or 148 KB.
- **Not measured:** anything near the limits. The server allows 512 MiB and 4,096 files a client, set "until phase
  5 has measured", and Zèrtz is a few thousandths of that. What hashing a very large folder adds to start-up, and
  what many clients downloading it at once cost, are unknown.

### What it cost

About 500 lines on the server (included in the table above), no dependency, a minor API version, and the fixture's
client folder. In bingle-3d, `src/downloads/` (checking, caching and fetching) and `format = 1` in appearance
files (bingle-3d #81). Its lines were not counted here.

### What did not fit

- **The client holds lower limits than the server**: 1,024 files and 256 MiB a game, textures at most 4,096 pixels
  a side, 500,000 vertices a model, sounds of 60 s, and a 2 GiB cache. Zèrtz comes near none of them.
- **A ring with a marble on it is not drawn.** The pack sends the marble alone, because the page's free board
  draws a stack of two as a pile. In 3D the marble then lies where its ring was. Data alone can fix it, either in
  the pack and the page (send the ring and the marble, and draw that stack as a marble in its ring) or in the
  client (draw the ring under a piece it is not told of). The choice is open.
- **Endings:** bingle-3d does not read a definition's `endings`.
- Nothing else about Zèrtz's look needed more than data, and the client has no code for Zèrtz. Zèrtz is a plain
  test of this, with one model of its own. A game with boards, cards or sounds of its own would test more.
- **Anyone who can reach the server can download a private game's files** (ADR 0147, question 6, settled that
  way). Extending this to the overlay's games would apply the same rule to their art.

## Recommendations

### The rules: keep as it is

Packs for small games, and the overlay for the rest.

- **For keeping them:** the proof of concept did what it set out to do. An unmodified public build plays a
  commercial game from a directory, sandboxed. With a state of its own and TinyGo, bots are usable: 14,000 minimax
  positions a second on Zèrtz, and Zèrtz batches no slower than linked in (Draughts batches reach a little over
  half). Away from bots, what a person might notice is the selections list.
- **Against extending them:** the next steps (Yinsh as a second pack, zip packs, admin install, hidden
  information) serve sharing games between servers, which nothing asks for yet. Admin install is what ADR 0128 was
  withdrawn over. The games most worth moving out of the overlay need the most bridging and gain the most from
  native speed.
- **What keeping costs:** about 4,200 lines of host, glue and loader, one dependency, half a minute of tests, and
  an ABI to keep working. If the ADR is accepted, it should say that **ABI 0 still promises nothing** and that a
  pack is rebuilt with the server it runs on. Otherwise acceptance would make it a promise, as the ADR now stands.
- **When to drop it instead:** if no second pack is likely, dropping is reasonable. Nothing depends on packs today
  but Zèrtz, which can move to the overlay through `wasmrules.Native`. That adapter lives in the package that would
  be removed, so it and the guest types would move with Zèrtz, and it keeps the cost shown in *What "linked in"
  means* until its rules are rewritten against the engine. If the client files are kept, the pack loader stays
  only for them, and they would need another home too.

The one follow-up worth considering while keeping it as it is: **the selections view's single call**, if Zèrtz's
players feel the delay.

### The client files: keep and extend

1. **Client files for the overlay's games**, in place of bingle-3d's private folder. The server side is already
   about games and not packs (`catalog.Module`), so what is left is for the overlay's modules to name their
   folders, and for bingle-3d's looks to move into them. A player of a private game would then need only the
   client. Measure start-up and a first open again with the first large folder. Say first that anyone who reaches
   the server may then download that art.
2. **The two findings in bingle-3d and the page:** the ring under a held marble, and `endings`.

A second client and stricter access are not recommended now. There is no second client, and access was settled.

If the rules are dropped and this is kept, it stands on its own: the overlay's modules would offer the files, and
Zèrtz would offer them from the overlay.

## The verdict

On 2026-11-27 the maintainer agreed with both: **the rules are kept as they are**, with ABI 0 promising nothing, and
**the client files are kept and extended**. [ADR 0147](../decisions/0147-rules-as-webassembly-in-an-install-pack.md)
is accepted. The follow-ups are #345 (client files for the overlay's games), #346 (a ring with a marble on it) and
noderunner/bingle-3d#96 (`endings`). The single `selections` call is not planned.
