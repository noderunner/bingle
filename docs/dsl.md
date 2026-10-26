# Writing a game in JSON

A game is one JSON document (embedded with `go:embed` and registered with
`catalog.Module`). Everything below is optional unless marked **required**.
`internal/games/tak/tak.json` is the fullest worked example; the
`internal/games/classics/*.json` files (Tic-Tac-Toe is the smallest; Amazons,
Ataxx, Pente and Brandubh show the newer actions) cover the rest.

```
id, name                      required
description, rules[]          shown in the lobby, rules dialog and MCP get_game_info
version, designers[], publisher   the implementation's semver (raise it with every change to the
                              definition or Go code; below 1.0.0 shows a Beta mark), and who made the game
family, variants[]            group sibling games / derive them from this document
options[]                     setup choices a creator makes (bool, seat-choice, book-swap)
board                         required: generator or explicit graph, regions, style
seats[]                       required
pieces{}                      required
reserves{}                    starting "hand", per seat, by reserve pool
setup[]                       pieces on the board at the start
preview[]                     a sample game in play for the list of games (for a game that starts empty)
turn_order                    required: round-robin, optional opening phase
actions[]                     place / move-stack / jump / slide / leap / step / pass
                              (pieces with a movement primitive also get "step")
win_condition                 required: none | callback | rules
notation                      generic (default) | ptn
cards                         suits, decks and zones of a card game
```

## board

```json
"board": {
  "generator": { "type": "grid", "width": 5, "height": 5,
                 "adjacency": "orthogonal4", "labels": "algebraic" },
  "style": "squares"
}
```

* `generator.type` is `grid` (rectangular; `adjacency` is `orthogonal4`,
  `orthogonal8` or `diagonal4` — diagonal links only, as on a draughts board),
  `hex` (a `width` × `height` parallelogram of hexagons as in
  Hex: cells `a1` … from the top-left, six neighbours each; no stack moves on
  hex boards) or `hexhex` (a hexagon with `size` cells per side — 5 for
  Yavalath, 8 for Havannah; cells keep their position in the enclosing
  `2·size−1` square, so some IDs such as `a1` do not exist). Hexhex boards
  have the regions `edge`, `corners`, `corner-0`…`corner-5` and the six sides
  without corners: `top`, `bottom`, `upper-left`, `lower-left`,
  `upper-right`, `lower-right`. A board has at most 4,096 positions.
* `labels`: `xy` (default, positions are `"x,y"` with y growing downwards) or
  `algebraic` (`a1` is bottom-left, files are letters, ranks numbers; up to
  26 columns).
* `style`: `squares` (default; filled checkered cells) or `points` (pieces sit
  on line intersections, as in Go).
* Every grid and parallelogram hex board has the regions `left`, `right`,
  `top`, `bottom`.
  Add your own with `"regions": {"home-1": ["a1","b1"]}`.
* `"borders": {"top": "red", "left": "blue"}` tints those edges in the UI.
* Instead of a generator you can list `positions` (optionally with `x`/`y`
  layout) and `edges`. Explicit boards draw as node/edge graphs.

## seats, pieces, setup

```json
"seats": [ {"seat": 1, "color": "white", "label": "White"},
           {"seat": 2, "color": "black"} ],
"pieces": {
  "flat": { "label": "Flat stone", "code": "F", "shape": "square",
            "pool": "stone", "tags": ["road", "flat"] },
  "wall": { "label": "Standing stone", "code": "S", "shape": "wall",
            "pool": "stone", "tags": ["blocker"],
            "stack": { "coverable": false, "flattenable_by": ["cap"], "flatten_to": "flat" } }
}
```

* Colors are CSS-ish names; the UI has palettes for `white`, `black`, `red`,
  `blue`, `green` and `yellow`.
* `shape`: `disc` (default), `square`/`flat`, `wall`, `cap`, `ring`,
  `diamond`, `triangle`, `cross`.
* `code` is the short letter used by notation, TPS and the text board.
* `tags` classify pieces for win rules and scores.
* `pool` names the reserve pool a *placed* piece is drawn from (default: the
  piece type itself). Pieces sharing a pool share a supply — Tak's flat and
  standing stones.
* `stack.coverable: false` means nothing may be moved onto a square topped by
  this piece, except a piece listed in `flattenable_by`, which turns it into
  `flatten_to` (see the `move-stack` action's `flatten` options).
* A piece with `"movement": {"primitive": "step", "occupancy": "capture-or-empty"}`
  gets the implicit `step` action: move one graph step, optionally capturing.
  `occupancy` is `empty-only`, `capture-or-empty` or `capture-only`.
  `"directions"` restricts the steps (and jumps) to compass names (`N`, `NE`,
  …) or, relative to the seat's `"forward"` (`N`/`E`/`S`/`W` on its seat
  entry), `forward`, `backward`, `left`, `right`, `forward-left`, … .
* `"promote": [{"seat": 1, "region": "top", "to": "king"}]` turns a piece
  that ends a move in the region into another type.
* `setup` entries (`{"position","piece","color"}`) naming the same position
  build a stack, first entry at the bottom.

## reserves and actions

```json
"reserves": { "stone": 21, "cap": 1 },
"actions": [
  { "id": "place", "type": "place", "pieces": ["flat","wall","cap"], "source": "reserve" },
  { "id": "move", "type": "move-stack", "carry_limit": "board-size",
    "flatten": { "final_square_only": true, "alone_only": true } }
]
```

**`place`** puts a piece on an empty square.

| field | meaning |
|---|---|
| `pieces` | placeable piece types (the first is the default) |
| `source` | `reserve` (default; decrements the pool) or `supply` (unlimited) |
| `region` | only squares of this region |
| `gravity` | direction name (`S`): only squares supported by a piece or the board edge |
| `captures` | `{}`: after placement, opposing orthogonal groups with no empty neighbour are removed; a placement leaving its own group without liberties is illegal unless it captures (`{"allow_suicide": true}` lifts that) — Go |
| `ko` | `true` (needs `captures`): may not recreate the position as it was before the opponent's last move |
| `flips` | `{"require": true}`: the placed piece flips every straight run of opposing pieces it brackets against a piece of its own color; with `require` the placement is only legal if it flips something (Reversi) |
| `custodian` | `{"lengths": [2], "count_in": "captured"}`: runs of opposing pieces flanked by the new piece and one of yours are removed (Pente); see *custodian captures* |

**`pass`**: `{"id": "pass", "type": "pass"}` — a move that does nothing (it
still ends the turn). Combine with the `passes` end trigger below.

**`move-stack`** (grid boards): pick up the top *n* pieces of a stack you
control (top piece is yours), carry them in a straight line without turning
back, and drop at least one piece on every square entered; the bottom of the
carried group is dropped first.

| field | meaning |
|---|---|
| `carry_limit` | integer, or `board-size` / `board-width` / `board-height` |
| `flatten.final_square_only` | flattening is only allowed on the last square |
| `flatten.alone_only` | the flattener must be the only piece dropped there |

**`step`** (explicit): when per-direction rules differ — Breakthrough's
straight moves must not capture but its diagonal moves may — declare steps as
actions instead of per-piece movement:

```json
{ "id": "advance",  "type": "step", "pieces": ["pawn"], "directions": ["forward"],
  "occupancy": "empty-only" },
{ "id": "diagonal", "type": "step", "pieces": ["pawn"],
  "directions": ["forward-left", "forward-right"], "occupancy": "capture-or-empty" }
```

**`jump`** (grid boards): capture by hopping over an adjacent enemy piece onto
the empty square directly beyond. The piece's `movement.directions` limit the
hop directions.

| field | meaning |
|---|---|
| `mandatory` | while any jump is available, only jumps are legal |
| `chain` | the same piece keeps hopping in one move; a chain must be played to its end, and a promotion ends it |

A jump move is `from` plus the list of landing squares; the squares hopped over
are removed. Notation: `c1xe3xg5`.

The turn can also *start* with a restricted phase:

```json
"turn_order": {
  "type": "round-robin", "seats": [1, 2],
  "opening": { "plies": 2, "actions": ["place"], "pieces": ["flat"], "owner": "opponent" }
}
```

`"pass_when_stuck": true` on `turn_order` skips a seat that has no legal move
(Reversi); when nobody can move an `end` rule with `no-legal-moves` finishes
the game.

During the first `plies` plies only the listed actions/pieces are allowed;
`owner: "opponent"` makes placed pieces take the opponent's color and come out
of the opponent's reserve (Tak's swap rule).

**`swap`** (two seats): the pie rule — `{"id": "swap", "type": "swap", "plies": [1]}`
(default: only the second player's first move). Taking it recolours every
piece on the board to the other side (and exchanges the reserves), and play
continues with the first player. Notation `swap`; the UI offers a *Swap
sides* button. Hex and Gomoku use it.

### slide (and compound moves)

```json
{ "id": "move", "type": "slide", "pieces": ["queen"],
  "occupancy": "empty-only", "max_distance": 0,
  "then": { "type": "shoot", "piece": "arrow" } }
```

A piece in `pieces` slides any distance (up to `max_distance` when set) along
a straight line without passing over other pieces; `directions` restricts the
lines (default: every direction the board's `adjacency` offers, so
`orthogonal8` gives queen moves and `orthogonal4` rook moves) and `occupancy`
is `empty-only` (default), `capture-only` or `capture-or-empty`.
`forbid_region` names squares the action's pieces may pass over but never stop
on (Hnefatafl's corners and throne, for everyone but the king, who gets his own
slide action). A slide may also carry `custodian` captures (below).

`then` makes the move compound: after sliding, the mover must also *shoot* — put
a `piece` from the unlimited supply on any empty square reachable by a slide
from the destination (the square just left counts as empty). Notation is
`from-to/aim` (`d1-d5/f5`), structured moves carry `aim`, and the UI builds it
as three clicks (piece, destination, target). Amazons is this
and nothing else (`internal/games/classics/amazons.json`).

### custodian captures

```json
{ "id": "place", "type": "place", "pieces": ["stone"], "source": "supply",
  "custodian": { "lengths": [2], "count_in": "captured" } }
```

After a placement or slide, along each direction a run of exactly `length`
opposing pieces closed off by another piece of the mover's colour — or by an
empty square of an `anvils` region (corners, empty throne) — is removed
(Pente's captured pairs, Hnefatafl's sandwiches; a piece moved *into* a flank
is safe). `count_in` credits a reserve pool with the number of pieces
taken — declare it in `reserves` with 0 and win with a `pool` rule. Pools no
piece draws from are shown as per-seat counters next to the player's name.

### leap

```json
{ "id": "move", "type": "leap", "pieces": ["blob"],
  "max_distance": 2, "clone_distance": 1, "infect": true }
```

A piece lands on an empty square at most `max_distance` board steps away
(steps follow the board's adjacency, ignoring what is in between, so on an
`orthogonal8` grid it is the Chebyshev distance and on a hex board the hex
distance). Within `clone_distance` the piece stays where it was — a clone —
and further out it jumps, vacating its origin. `infect` converts every
opposing piece adjacent to the landing square (Ataxx).

## win_condition

```json
"win_condition": { "type": "rules", "rules": [
  { "id": "road", "type": "connect", "tag": "road",
    "regions": [["left","right"],["top","bottom"]], "reason": "road" },
  { "id": "flats", "type": "end", "when": ["reserve-empty","board-full"],
    "decide": { "type": "most", "tag": "flat" }, "reason": "flat-win" }
]}
```

Rules are evaluated in order after every move; the first that fires decides.

* `connect`: a seat wins when a connected group of its pieces (matched by the
  **top** piece of each square: `tag` and/or `piece`) touches both regions of
  any pair. If several seats connect at once the mover wins. Add `"seat": N` to
  restrict a rule to one seat — in Hex each player connects a different pair
  of edges.
  Instead of `regions`, `"of": [...regions], "touch": N` fires for a group
  touching at least N of the listed regions — Havannah's bridge (two of the
  six `corner-*`) and fork (three of the six sides).
* `line`: `length` of the seat's matching pieces in a row (grid: orthogonal
  and diagonal; hex: along the three hex axes).
* `absent`: `piece` and `seat` — `seat` wins as soon as no piece of that type
  is left on the board (Hnefatafl: the king is captured).
* `pool`: `pool` and `at_least` — a seat wins when its reserve pool holds at
  least that many (the capture counter of Pente; see *custodian captures*).
* `ring`: a seat's matching pieces enclose at least one square (hex boards;
  Havannah). The enclosed square may be empty or hold anything.
* Any of `connect`/`line`/`ring`/`reach` may say `"result": "lose"`: the seat
  that satisfies the rule loses, so its opponent wins (Yavalath's "three in a
  row loses"; put the rule after the "four wins" rule so a four that
  contains a three wins).
* `reach`: a seat wins when one of its matching pieces stands in `region`
  (`"seat": N` restricts the rule to one seat) — Breakthrough's far row.
* `end`: `when` any of `board-full`, `reserve-empty` (some seat has an empty
  hand), `no-legal-moves` (the seat to move), `ply-limit` (with `"plies": N`)
  or `passes` (with `"count": N` consecutive passes); `decide` is `draw`,
  `most` (count of matching top pieces; tie = draw), `score` (area scoring —
  stones plus empty regions bordering only that color, plus `komi` per seat
  number, e.g. `{"2": 6.5}`; the final tally is reported as the game's
  `end_detail`), `mover-wins` or `mover-loses`. "The seat to move has no legal move" is
  `no-legal-moves` + `mover-wins` (draughts).
* `reason` is stored as the game's `end_reason`.

`"type": "none"` means free play (the demo); `"type": "callback"` defers to a
Go `GameCallbacks.CheckWinner` for rules JSON cannot express.

## variants

```json
"variants": [
  { "id": "tak-3x3", "name": "Tak (3×3)",
    "patch": { "board": {"generator": {"width": 3, "height": 3}},
               "reserves": {"stone": 10, "cap": 0} } }
]
```

`patch` is an RFC 7386 JSON merge patch against the base document; register
with `Catalog.RegisterModule` and every variant becomes a game of its own
(grouped by `family` in the lobby).

## options

```json
"options": [
  { "id": "factions", "label": "Factions", "kind": "seat-choice",
    "text": "Which cult sits in each seat.",
    "choices": [ {"id": "gc", "label": "Great Cthulhu", "color": "green"}, … ],
    "default": ["gc", "cc", "bg", "ys", "ww"] },
  { "id": "high-priests", "label": "High Priests", "kind": "bool", "default": false },
  { "id": "book-swaps", "label": "Neutral spellbook instead of a timer book", "kind": "book-swap",
    "slots":   [ {"id": "bg", "label": "Ghroth"}, … ],
    "choices": [ {"id": "shrivel", "label": "Shriveling"}, … ] }
]
```

Options describe the choices of a creator for clients: `bool` is a check box,
`seat-choice` a list per seat (each `choices[].id` at most once, `color` is the
seat colour it brings), `book-swap` a list of `choices` for every `slots[]`
entry whose cult (the slot's `id`) sits at the table. The values travel in the create call (`options`) and
are turned into a **derived game**, `<id>~<suffix>`, by the game's
`catalog.Deriver` — a ruleset implements `Derive`/`FromSuffix` and returns a
merge patch of this document; see
[ADR 0028](decisions/0028-derived-games-and-setup-options.md). JSON-only games
have no `Deriver`: a create call that passes `options` for one is refused with
400 (*invalid setup options*).

## notation

* Generic: step, slide and leap `from-to` (`2,0-2,1`); slide with a follow-up
  `from-to/aim` (`d1-d5/f5`); place `piece@square` (just `square` when only one
  piece type can be placed); stack move
  `from/count>square:drops>square:drops`; jump `fromxlandxland`.
* `"notation": "ptn"` (needs an algebraic grid): placements `a1`, `Sa1`, `Ca1`;
  stack moves `[count]<square><+|-|<|>>[drops]` such as `3c3>111`. Positions
  can be exchanged as TPS (`x3,12,2S/x5/x5/x5/x5 1 2`).

## Go rulesets (`engine: "ruleset"`)

Some games need more than declarative actions: hidden choices, dice,
per-unit conditions, turns made of many decisions. Such a game keeps its
JSON document for what *is* data — board, seats, pieces, regions, text — and
says who implements the rules:

```json
{
  "engine": "ruleset",
  "params": { "vp_to_win": 5, "momentum_cap": 5 },
  "turn_order": { "type": "round-robin", "seats": [1, 2] },
  "win_condition": { "type": "ruleset" },
  "display": {
    "tints":  { "green": { "label": "Overgrowth", "color": "#5f9e5b", "glyph": "♣", "text": "…" } },
    "badges": { "fatigued": { "label": "Fatigued", "color": "#f4f1e8", "text": "…" } }
  }
}
```

The module registers a Go value implementing `state.Ruleset` next to the
JSON: `catalog.Module{ID, Definition, Rules: courtfall.New()}`. `actions` and
`win_condition.rules` are not used. A ruleset

* generates and validates every decision (`Legal`, `Apply`) — a turn is a
  sequence of decisions, and `ToMove` says whose the position waits for;
* keeps its data in the position's `ext` payload and publishes a derived
  `ext.view` (`tints`, `badges`, `seats[].counters`, `prompt`, `contest`…)
  that the generic UI draws using the `display` legend;
* may hide information (`Redact`, `RedactMove`, `Determinize`), forbid
  taking moves back across chance (`CanTakeBack`), publish a fixed action
  space and its own observation planes, and give search bots a heuristic.

See [courtfall.md](courtfall.md), [cthulhu-wars.md](cthulhu-wars.md) and ADR 0025. `params` are plain numbers and
strings the ruleset reads (`CompiledGame.ParamInt`); variants change them with
a merge patch like anything else.

## Area maps (`board.style: "areas"`)

A board of irregular areas — continents, oceans, a wrapped sea — instead of a
grid. Positions are declared with their graph edges, so movement and
adjacency are ordinary; the polygons only say how to draw them:

```json
"board": {
  "type": "explicit",
  "style": "areas",
  "canvas": { "width": 1000, "height": 600 },
  "positions": [
    { "id": "asia", "name": "Asia", "anchor": [800, 140], "tags": ["land", "glyph-dragon"],
      "polygons": [[[690,60],[842,60],[915,245],[690,230]]] },
    { "id": "north-pacific", "name": "North Pacific", "tags": ["ocean"],
      "polygons": [[[0,60],[130,60],[130,320],[0,320]], [[915,60],[1000,60],[1000,320],[915,320]]] }
  ],
  "edges": [{ "from": "asia", "to": "north-pacific" }]
}
```

* `polygons` — one or more outlines (`[x, y]` corners, at least three) on the
  `canvas`; several polygons draw an area that wraps around the edge.
* `anchor` — where the name and unit tokens are centred (default: the
  centroid of the largest polygon).
* `tags` — free labels; `ocean`/`land` pick the tint and `glyph-<name>` draws
  a small symbol. Games read tags through `mapData` in their ruleset.
* The catalog rejects a missing canvas or a position without an outline.

## Rules guide (`guide`)

```json
"guide": { "tabs": [
  { "id": "overview", "title": "Overview", "file": "guide/overview.md" },
  { "id": "battle",   "title": "Battle",   "file": "guide/battle.md" }
] }
```

The files are embedded with the module (`catalog.Module.Assets`, an `fs.FS`;
`//go:embed guide`) and each tab must exist. They are plain Markdown with a
few extras (callouts `> [!NOTE]`, `::: details Title` … `:::`, `[[Key]]`
keycaps, tables, `![](piece:cthulhu:blue)` piece icons, `![|600](board.svg)`
images relative to the game's assets, `[text](tab:battle#dice)` links). A
game with no `guide` gets one generated *Rules* page (description, rules, how it
ends, notation). The abstract games ship a single `rules.md` with diagrams
(`scripts/guide_diagrams.py` draws the SVGs); add a second tab only where it earns
its place (Fist's pieces, Onitama's cards, drawn by Go from the card table). `display.no_hands: true` hides the generic reserve panels of a
ruleset that draws its own.

## cards

A card game defines its cards in `cards` (see ADR 0043). A deck is a
template of every suit × every rank, plus explicit cards:

```json
"cards": {
  "suits": { "red": { "label": "Red", "color": "#c0392b", "symbol": "●" } },
  "suit_order": ["red", "blue"],
  "decks": { "main": {
    "ranks": [ { "id": "1", "value": 1, "count": 3 }, { "id": "2", "value": 2 } ],
    "extra": [ { "id": "joker", "label": "Joker", "text": "Wild", "symbols": ["★"], "count": 2 } ],
    "remove": ["red-2"]
  } },
  "back": { "color": "#5b4a8a", "text": "NYET!" },
  "zones": [
    { "id": "deck",  "scope": "table", "visibility": "none",  "layout": "stack", "counts": true },
    { "id": "hand",  "scope": "seat",  "visibility": "owner", "layout": "fan" },
    { "id": "trick", "scope": "table", "visibility": "flag",  "layout": "row" }
  ]
}
```

* A card type is `<suit>-<rank>` (or an extra card's `id`); a `count` above one
  makes identical copies, named `blue-1#0`, `blue-1#1`, … (`catalog.TypeID`
  strips the `#n`). Suit, rank, value, text, symbols and tags travel to clients
  in the game info; a position names a card by its id only.
* A zone is a place cards can be: `scope` `seat` (one per seat: a hand, tricks
  won) or `table`. `visibility` says who sees faces: `owner`, `all`, `flag`
  (face-up cards only) or `none`. `layout` is how clients draw it: `fan`, `row`,
  `stack` (top card and count), `splay` or `grid`; `counts: true` sends a hidden
  pile as a count only.
* A variant strips cards with a merge patch of `cards.decks.<deck>.remove`.
* Board `style: "none"` draws no board (the cards are the table).
* In Go, `internal/engine/cards` holds a game's cards in a `Table` of zones: it
  shuffles with a seedable `RNG`, deals, moves and flips cards, and `Redact` /
  `View` give each viewer only the cards they may see. The client side is
  `static/js/cards.js` (the card table, with flip and deal animations) and a
  generic board of choices (`view.grid`).

## What JSON can't express

Anything outside the above — hidden information, randomness, rules that depend
on long position history — needs either a new action type in
`internal/engine/state` (preferred when the idea is reusable: it then works
for every game; ADR 0022 shows how `slide` and `leap` were added), or a Go
ruleset as described above (ADR 0025; Courtfall and Cthulhu Wars are). The small
`callback.GameCallbacks` hook (veto a move, name a winner) remains for
declarative games that need only that.
