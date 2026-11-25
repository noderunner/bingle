# Install packs

An install pack is a game whose rules are a WebAssembly module, in a directory that a server reads
when it starts. An unmodified public build of bingle then offers the game like any other. It is a
proof of concept ([ADR 0147](decisions/0147-rules-as-webassembly-in-an-install-pack.md)): the way a
commercial game is built is still the private overlay
([ADR 0127](decisions/0127-commercial-games-in-a-private-overlay.md)).

## Loading packs

Name a directory of packs in `BINGLE_PACKS`. Each pack is a directory of its own inside it:

```sh
BINGLE_PACKS=/srv/bingle/packs ./bin/bingle
```

The packs are read once, at start-up. To add, change or remove one, change the directory and
restart. A pack that fails any check below is logged with its reason and left out, and the server
starts without it:

```
ERROR pack: left out dir=/srv/bingle/packs/draughts error="pack: web.yaml names a script: …"
INFO pack: loaded dir=/srv/bingle/packs/zertz games="[pack-zertz-1.0.0 pack-zertz-1.0.0-48]" …
```

Anything in the directory that is not a directory is left out too, and so are names starting with a
dot. Unset, `BINGLE_PACKS` loads nothing and nothing changes.

## Layout

```text
draughts/
  manifest.yaml   name, version, abi, private
  game.yaml       the definition (engine: ruleset), and its variants
  web.yaml        optional: how the web page draws it (ADR 0124)
  rules.wasm      the rules
  guide/          the guide's Markdown and pictures
  clients/        optional: the files of other clients (not read yet: #302)
```

Nothing else may be in a pack: no other file or folder at its top, and no link anywhere. Files are
read through a root confined to the pack. [`tests/testdata/draughts/pack`](../tests/testdata/draughts/pack)
is a complete pack less its `rules.wasm`, which the tests build.

### manifest.yaml

```yaml
name: draughts   # lower-case words joined by dashes, at most 40 characters
version: 0.1.0   # major.minor.patch
abi: 0           # the version of the calls its rules speak; the server refuses any other
private: false   # true marks a commercial game, as the overlay's are (ADR 0140)
```

### game.yaml

An ordinary definition ([dsl.md](dsl.md)) with `engine: ruleset`, but
**without `id`, `family`, `version` or `prototype`**, which the server sets from the manifest:

- The game is `pack-<name>-<version>` and its family `pack-<name>`. The family's name means its
  newest version, as for any prototype ([ADR 0098](decisions/0098-prototype-games-and-rules-versions.md)).
- A variant is named after the pack, `<name>-<word>`, and becomes `pack-<name>-<version>-<word>`:
  `draughts-6` in Draughts 0.1.0 is `pack-draughts-0.1.0-6`.
- A pack's game is a prototype, so it is not rated.

It may have no callbacks, no `callback` win condition and no setup `options` (use variants), and at
most 64 variants. Its rules' own end reasons are worded in its `endings` (see [dsl.md](dsl.md), *win_condition*), since a pack has no
page script to word them.

### web.yaml

A look as any game's ([ADR 0124](decisions/0124-the-server-says-what-things-are-clients-decide-how-they-look.md)), with a variant's
named as in `game.yaml`. A pack runs nothing in a player's browser: **no `script` and no `style`**,
and every colour must be a colour (`#rgb` to `#rrggbbaa`, a name, or `rgb()`/`hsl()`). It must fit
the game (`look.Check`).

### guide/

The guide's Markdown and its pictures: PNG, JPEG, GIF, WebP and SVG. They are the game's files, as a
built-in game's guide folder is, so the definition and the look name them by their path inside
`guide/` (`file: rules.md`, not `guide/rules.md`). Nothing else of the pack is served.

## Checks

A pack is meant to be handed from one server to another, so the server checks it as though it came
from anywhere:

| Check | Refused |
|---|---|
| Paths | anything but the files and folders above; a link; the same name twice, ignoring case |
| Sizes, on what is really read | a file over 8 MiB, `rules.wasm` over 64 MiB, over 40 MiB in all, over 300 files |
| Kinds | text that is not UTF-8 or holds a NUL; a picture that is not what its name says; another kind of file |
| Manifest | a missing or malformed name, version or ABI; an ABI this server does not speak; any other field |
| Definition | one that sets its id, family, version or prototype; that is not a ruleset; that does not compile; over 1 MiB with its aliases expanded |
| Look | a `script` or `style`; a colour that is not one; one that does not fit the game |
| Rules | a module that does not load (an import beyond WASI, a missing export, another ABI) |
| Trial | a game of 300 random selections, played with a fixed seed, in which the rules fail, panic, refuse a selection they offered or take over ten seconds |

The rules then run sandboxed: no files, network or real clock, a memory cap and a deadline on every
call (ADR 0147, *The sandbox*).

## A pack that is gone

A server started without a pack it once had starts as usual. The pack's games are kept as a
retired game's ([ADR 0076](decisions/0076-retiring-games.md)): `GET /api/games/{code}` answers
`retired: true` with the game's name, the moves stay listed, and nothing more can be done in them
(410). Unlike a retired game's, **its unfinished games are not ended**: when the pack is back, they
play on. No new game of it can be made meanwhile.

## Building a pack

The rules are standard Go, written against `internal/engine/wasmrules/guest` (ADR 0147, *The
guest*), and built for `wasip1` as a module whose exports are called:

```sh
GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -trimpath \
  -o packs/draughts/rules.wasm ./tests/testdata/draughts/wasm
cp -R tests/testdata/draughts/pack/. packs/draughts/
BINGLE_PACKS=$PWD/packs ./bin/bingle
```

The same rules package links natively through `wasmrules.Native`, so it is tested with plain
`go test`. `scripts/smoke-test.sh` builds and loads the fixture pack this way.

## Bots

A module built with the guest glue answers the `bot` call (ADR 0147, *ABI v0*), so the built-in bots
(tactical, minimax and MCTS) run their search inside the module and cross into it once a decision,
rather than at every position they look at. A game gets them by writing its rules alone. A game whose
`Rules` also implement `guest.Heuristic`, a value of a position for a seat from -1 to 1, is searched
with it: minimax values its leaves with it, and MCTS a playout it had to cut short. Without one,
minimax counts the pieces on top of the board, and MCTS scores a playout it cut short as a draw. A
module built before the call was added is still played, with the host's bots searching through
`legal` and `apply`, which is much slower.

The search is still the rules' own speed in WebAssembly, which is far below the same rules linked
in. On the Draughts fixture, against the same rules compiled into the server:

| | Linked in | Through the module | Inside the module |
|---|---|---|---|
| minimax | 67,000 positions/s | 1,500 positions/s | 2,700 positions/s |
| MCTS | 1,700 playouts/s | 31 playouts/s | 51 playouts/s |
| a practice batch | 223 games/min | 5.4 games/min | 10.6 games/min |

`tests/wasmrules/report_test.go` measures them (`BINGLE_WASM_REPORT=1`).
