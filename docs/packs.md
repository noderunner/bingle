# Install packs

An install pack is a game whose rules are a WebAssembly module, in a directory that a server reads
when it starts. An unmodified public build of bingle then offers the game like any other. Packs are
for small games ([ADR 0147](decisions/0147-rules-as-webassembly-in-an-install-pack.md)): the way
other commercial games are built is still the private overlay
([ADR 0127](decisions/0127-commercial-games-in-a-private-overlay.md)).

**The ABI, version 0, promises nothing.** It may change in any release, so rebuild a pack with the
server it runs on.

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
  clients/        optional: every other client's files, a folder each
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
and every `color` and `paint` must be a color (`#rgb` to `#rrggbbaa`, a name, or `rgb()`/`hsl()`). It must fit
the game (`look.Check`).

### guide/

The guide's Markdown and its pictures: PNG, JPEG, GIF, WebP and SVG. They are the game's files, as a
built-in game's guide folder is, so the definition and the look name them by their path inside
`guide/` (`file: rules.md`, not `guide/rules.md`). Nothing else of the pack is served but `clients/`.

### clients/

The files each client other than the web page draws the game with, in a folder named after the
client: `clients/bingle-3d/` for bingle-3d's look, models, textures and sounds, laid out as that
client wants them. The server never reads them: it checks them as below, hashes each once when the
pack loads, and hands them to any client that asks, as it does a guide's pictures (ADR 0147).

```text
clients/
  bingle-3d/
    look.toml
    models/man.glb
    textures/board.ktx2
```

- A client's name is lower-case words joined by dashes, as a pack's is; at most 8 clients.
- A path is at most 200 bytes, each part of it letters, digits, dots, dashes and underscores.
- At most 64 MiB a file, 512 MiB and 4,096 files a client. An empty folder is refused.
- The kinds, by extension, each held to its signature or, for text, to UTF-8 without a NUL:

  | Kind | Extensions | Sent as |
  |---|---|---|
  | TOML | `.toml` | `application/toml` |
  | JSON | `.json` | `application/json` |
  | Text | `.txt` | `text/plain` |
  | Markdown | `.md` | `text/markdown` |
  | Binary glTF 2.0 | `.glb` (its header's length must be the file's) | `model/gltf-binary` |
  | KTX2 | `.ktx2` | `image/ktx2` |
  | PNG | `.png` | `image/png` |
  | JPEG | `.jpg`, `.jpeg` | `image/jpeg` |
  | Ogg | `.ogg` | `audio/ogg` |

Every game of the pack, its variants too, offers the same files. A client finds them from the list
of games (`clients` in `GET /api/game-defs`), lists them, and fetches only those it lacks:

```sh
curl -s localhost:8080/api/game-defs/pack-draughts-0.1.0/clients/made-up   # the fixture's client
# {"game":"pack-draughts-0.1.0","client":"made-up","files":[{"path":"look.toml","size":182,"sha256":"b025c2…","content_type":"application/toml"}, …]}
curl -s -H 'If-None-Match: "b025c2…"' localhost:8080/api/game-defs/pack-draughts-0.1.0/clients/made-up/files/look.toml
# 304, nothing sent: the client has it
```

A file's SHA-256 is its strong `ETag`, so `If-None-Match` and `If-Range` are answered, and so are byte
ranges, for a download that is resumed. Files are sent from the disk as they are read, inert
(`nosniff`, a sandboxing `Content-Security-Policy`) and as attachments. One changed on the disk after
the pack loaded is not sent (500) until the server is restarted, so a file never goes out under
another's hash. A client checks what it downloads against its hash and treats it as hostile.

## Checks

A pack is meant to be handed from one server to another, so the server checks it as though it came
from anywhere:

| Check | Refused |
|---|---|
| Paths | anything but the files and folders above; a link; the same name twice, ignoring case |
| Sizes, on what is really read | a file over 8 MiB, `rules.wasm` over 64 MiB, over 40 MiB in all, over 300 files, not counting `clients/` |
| Kinds | text that is not UTF-8 or holds a NUL; a picture that is not what its name says; another kind of file |
| Clients | a file of `clients/` itself; a client's name, path, size, count or kind out of the limits of [clients/](#clients); an empty client |
| Manifest | a missing or malformed name, version or ABI; an ABI this server does not speak; any other field |
| Definition | one that sets its id, family, version or prototype; that is not a ruleset; that does not compile; over 1 MiB with its aliases expanded |
| Look | a `script` or `style`; a color that is not one; one that does not fit the game |
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

### With TinyGo

[TinyGo](https://tinygo.org) builds the same rules into a module about twice as fast as Go's, a sixth
of the size, that loads five times sooner and holds a tenth of the memory (ADR 0147, phase 4b).
Nothing in the rules changes, and the server plays either module the same:

```sh
tinygo build -target=wasip1 -buildmode=c-shared -opt=2 -scheduler=none -no-debug   -o packs/draughts/rules.wasm ./tests/testdata/draughts/wasm
```

- **`-scheduler=none`** matters most: TinyGo's default scheduler slows every call three to six
  times. Without one, a module cannot start a goroutine, which rules have no use for.
- **`-opt=2`** is a quarter faster than the default `-opt=z`, for a module a quarter larger.
- Use a TinyGo that supports the Go the rules are written in (0.44.0 for Go 1.27). TinyGo does
  not have all of the standard library, and its `encoding/json` works by reflection and is
  slow, so a game that reads its own data on every call gains less.

The tests build a TinyGo module too where `BINGLE_TINYGO`, or a `tinygo` on the `PATH`, gives one,
and leave it out otherwise.

## Bots

A module built with the guest glue answers the `bot` call (ADR 0147, *ABI v0*), so the built-in bots
(tactical, minimax and MCTS) run their search inside the module and cross into it once a decision,
rather than at every position they look at. A game gets them by writing its rules alone. A game whose
`Rules` also implement `guest.Heuristic`, a value of a position for a seat from -1 to 1, is searched
with it: minimax values its leaves with it, and MCTS a playout it had to cut short. Without one,
minimax counts the pieces on top of the board, and MCTS scores a playout it cut short as a draw. A
module built before the call was added is still played, with the host's bots searching through
`legal` and `apply`, which is much slower.

### A game's own state

How fast the search is depends most on what a position costs the game. Through `Rules`, every
position the search looks at is a `guest.Position`: a map of cells to copy, and the game's own data
to read and write again as JSON. A game whose `Rules` also implement **`guest.Searchable`** gives
the search a position of its own instead, a `guest.State`, which the search makes once from the
`Position` it is asked about and plays from there:

```go
type Searchable interface {
    State(pos *Position) (State, error)
}

type State interface {
    Turn() int                         // the seat to move, 0 once the game is over
    Winner() int                       // of a finished game, 0 for a draw
    Legal() []Selection                // as Rules.Legal; the search does not change the list
    Play(sel Selection) (State, error) // a selection Legal listed, as Rules.Apply
    Value(seat int) float64            // from -1 to 1, as Heuristic
}
```

A `State` is never changed: `Play` returns another. It may keep the list `Legal` gave, and `Play`
need not check its selection, since it is only given one `Legal` listed. **Write `Rules.Legal` and
`Rules.Apply` through the same `State`**, turning the `Position` into one as a call comes in and
back as it goes out, as the Draughts fixture does (`tests/testdata/draughts/state.go`): the rules
are then written once, and every call across the boundary is cheaper too.

On the Draughts fixture, whose state is an array of its 32 dark squares, against the same rules
compiled into the server:

| | Linked in | Through the module | Inside, on `Position` | Inside, on a `State` | Inside, on a `State`, TinyGo |
|---|---|---|---|---|---|
| minimax, positions/s | 92,000 | 1,800 | 2,700 | 38,700 | 72,900 |
| MCTS, playouts/s | 2,300 | 36 | 53 | 635 | 1,250 |
| a practice batch, games/min | 283 | 6.6 | 7.2 | 87 | 161 |

At equal time, MCTS inside a TinyGo module playing on a `State` is as strong as MCTS on the rules
linked in. `tests/wasmrules/report_test.go` measures them (`BINGLE_WASM_REPORT=1`).
