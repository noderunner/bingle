# 0124. The server says what things are; each client decides how they look

Status: accepted (2026-11-07). It changes the game plugins plan's step 3 and decision 4
(`docs/history/game-plugins-plan.md` on `more-plans`), and moves the appearance half of the DSL's display
fields (`docs/dsl.md`) out of the game definition. [ADR 0085](0085-card-sets-belong-to-the-table.md) stands, as
described below.

## Context

Bingle is played through more than one client: the web page today, and a native client built on Bevy (and
perhaps others, in Java or elsewhere) to come. Each should be free to present a game in the way that suits its
medium: a 3D table, a phone, a screen reader. It should not have to design around choices made for the web page.

Most of what the server sends is already meaning: the rules, the legal selections, the position, who is to move,
and facts the rules derive, such as which cells hold a fenced bloom. The appearance has crept in alongside it,
though, mostly from the start, when the web page was the only client:

- **The game definition carries appearance**: a seat's `blend`, a piece's `shape` and `color`, `board.style` and
  `borders`, an area map's `polygons`, `anchor` and `canvas`, a card zone's `layout`, a card back's `color`, a
  sample `preview` position, and the `display` legend's hex colours, glyphs and `no_hands`. Game info passes
  most of it on to every client.
- **Go rulesets compute some**: Cthulhu Wars' `gateColor` returns hex colours for the Gate badges.
- **The plugins plan wanted more.** Its step 3 (2026-10-31) moved Cthulhu Wars' cult colours, sigils and icons
  *into* the definition, so that the web page names no game. Its decision 4 gave a data pack, which ships no
  JavaScript, its look through the definition's declarative display vocabulary.

The two goals of that step are sound: the web page should not hard-code games, and a data pack should be
drawn well. Putting the look on the server is the wrong way to meet them, because every other client would then
either reproduce the web page or work round it.

## Decision

**The game definition and the API carry what things are and what the rules say. How things look belongs to
each client.** A client can play and understand any game from the API alone (as the API-only play test, #110,
already checks). It needs no field that only describes a picture.

**Meaning stays on the server.** This covers:

- identifiers, names and words the rules or the players use: a seat's identifier and its `label` ("Warm"), a
  piece's `label` and notation `code`, a cult's name, an area's `name`;
- the board as the rules see it: positions, adjacency, regions, `tags` and a seat's `forward`;
- the position and the facts the rules derive from it. These are keyed by what they mean, never by how to draw
  them: the cells of a `fenced` bloom, a unit's `fatigued` badge, a pool's count and its target, a prompt;
- a tint's or badge's `label` and `text`, which explain a fact to a player in words;
- a property the rules read, even one named like a colour. A suit's red or black is a rule in patience
  (ADR 0122), and a seat's colour word (`red`) is the seat's identifier in positions, notation and the API;
- choices made at setup that players talk about: the table's card set (ADR 0085), as an id and its suit names;
- the rules text, the guide (Markdown with its illustrations, which are documentation, not the board), and
  tutorials.

**Appearance moves to the client.** This covers colours as values (hex), glyphs, shapes, gradients, board
style, border tints, map outlines and picture coordinates, card layouts and backs, the art of a card set, sample
pictures, icons, sigils, and which panels a page shows.

**A game may ship a separate appearance file for a client, which that client alone reads.** The web page's is
`web.yaml` beside the game's definition, served with the game's assets. It holds no rules and nothing a player
needs to understand the game. A client that does not know a file ignores it, and a client may ship its own
appearance for any game instead. A data pack may include a `web.yaml`, which is data and never script. A
compiled-in game (built in, or in the private overlay) may also give the web page its own scripts, as the
web client's, registered for that game through one generic hook, so that the shared page names no game.

**A game without an appearance file is still drawn.** Every client draws any game from its meaning alone, with
generic defaults (a palette by seat, a shape by piece type, a layout from the board's own coordinates). The
appearance file improves that; it is never required.

**Colour words stay as identifiers.** `red` keeps meaning "the seat that plays red" everywhere it is stored and
sent, because renaming every stored position and notation would cost much and gain nothing. A client may draw
that seat in any colour, or none.

## The audit: what moves

Today's appearance fields, and where each goes. "Web file" means the web page's `web.yaml`.

| Today | Used by | Goes to |
|---|---|---|
| `pieces.*.shape` | nearly every game | web file |
| `pieces.*.color` (a type in its own colour) | Blooms | **stays**: the rules read it (see the addendum) |
| `seats[].blend` | Blooms | web file |
| `board.style` (`squares`, `points`, `none`, `free`, `areas`) | classics, Tak, Fendo, Fist, Hive, Meridians, Nyet!, Onitama, Solitaire, Cthulhu Wars | web file |
| `board.borders` (edge tints by seat) | classics (Hex) | web file. The regions they mark stay, as rules |
| `positions[].polygons`, `anchor`, `board.canvas` | Cthulhu Wars | web file (with the game, into the overlay) |
| `positions[].x`, `y` | explicit boards | stays only where it is the board's geometry (a client's default layout). Otherwise web file |
| `cards` zone `layout`, card `back.color` | Nyet!, Solitaire | web file |
| suit `color` | Solitaire, Nyet! | stays where the rules read it (patience). Otherwise web file |
| suit and rank `symbol(s)` | card games | stays as text (a name players use), drawing to the client |
| `preview` (a sample position for the game list) | Blooms, classics, Courtfall, Meridians, Mycelium, Tak, Yinsh | web file |
| `display.tints/badges.*.color`, `glyph` | ten games | web file |
| `display.tints/badges.*.label`, `text` | ten games | stays: it explains the fact |
| `display.terms` | Courtfall | stays: it is words |
| `display.no_hands` | eight games | web file |
| `end_of_turn[].tint` (a key) | Blooms | stays, as the fact's name (`fenced`) |
| ruleset views' `tints`, `badges`, `counters`, `prompt` (keys and words) | Go rulesets | stays |
| Cthulhu Wars' `gateColor` (hex in the view) | Cthulhu Wars | web file. The view says which faction's Gate |
| Cthulhu Wars' cult colours, sigils, icons and the `cw-*.js` interface | the web page | stay in the web client, and travel with the game into the overlay |
| `card_set` (ADR 0085) | card games | stays as the setup's choice. Each set's art is the client's |
| `extra_colors`, `seats[].color`, `seats[].colors` | several | stay, as identifiers (above) |

## Consequences

- **The plugins plan's step 3 is rewritten**: the web page stops naming games by reading each game's web file
  and, for compiled-in games, its registered scripts. Nothing about a game's look moves to the server.
  Cthulhu Wars' interface stays web-side and goes with the game into the private overlay.
- **Decision 4 of that plan stands, narrowed**: a pack ships no script, and its look is its `web.yaml`, not
  its definition.
- **The work is staged, with no change for players at any stage**:
  1. The web page reads a game's `web.yaml` when there is one, and falls back to the fields in the definition.
  2. Each game's appearance fields move into its `web.yaml`, a few games at a time, and the definition's
     schema loses them. Game info and the views stop sending them, and Go rulesets' views send keys, not
     colours.
  3. A test holds every shipped game's definition, game info and views to the meaning side of this line, and
     draws every game with no `web.yaml` to show the defaults work.
- **The DSL reference** splits its display section into the facts (in `docs/dsl.md`) and the web file (a section
  of `docs/ui.md`), as each stage lands.
- **A new client** reads the rules, the position and the facts from the API. It reads nothing else from the
  server about how anything looks, unless it chooses to read the web file.

## Addendum (2026-11-07): a piece type's own colour is a rule

The audit put `pieces.*.color` with the appearance. It is not: a piece type with a colour of its own
(Blooms' four stones) is placed in that colour, and only the seat that plays it may place it
([ADR 0117](0117-groups-by-colour-enemies-by-seat.md)). Like a seat's colour word, it stays in the
definition as meaning. Stage 1 found it, when Blooms moved first.
