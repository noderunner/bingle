# 0154. A color is a name, and paint is each client's own

Status: accepted (2026-11-27), for issue #335. Says what follows from
[ADR 0124](0124-the-server-says-what-things-are-clients-decide-how-they-look.md)'s decision that color words stay
as identifiers, which stands.

## Context

Players at one table may each choose their own board theme, and the 3D client (bingle-3d) will offer looks of its
own. Chess and Go name their sides, and their notation, after colors. Two players who see different paints can
talk past each other: one says "the red stones" of stones the other sees as blue.

The code makes it easy to confuse the two things, because `color` means four things today:

- a seat's identifier: `seats[].color`, the keys of reserves, the API's `color`;
- a rule property of a piece: Blooms' four stones (ADR 0117), a pack's `extra_colors`;
- a suit's red or black, which patience's rules read (ADR 0122);
- a hex value to draw with: `web.yaml`'s tints and badges, and a few leftovers in `internal/engine/dsl`.

Both clients also paint by reading the identifier. The page turns the word into a CSS class (`cssColor`,
`--piece-<word>-fill`), and its board themes are keyed by twelve color words, so Go's first player, black, is
drawn cold in every theme. The 3D client does the same in its `look.rs`. ADR 0124 already says that a color word
is an identifier and that a client may draw it in any color, but nothing says what a style must keep, and nothing
helps players who see different paints.

## Decision

**Names are shared; paint is private.** A chess set works this way: "White" is the side's name whether the set is
ivory or red.

| Word | Meaning | Where it lives |
|---|---|---|
| **seat** | a place at the table: its turn in the order, its clock, its result | definition, API, state |
| **color** | a name the rules give a set of pieces (`white`, `yellow`, `grey`). A seat plays one or more; some are nobody's. An identifier, never a value to draw with | definition, API, state, notation |
| **label** | what players call a seat ("White", "Warm", "Attackers") | definition |
| **paint** | what one client draws a color as: a hex value, a gradient, a material | that client's files and code only |

1. **`color` keeps its name** in the definition, the API and stored positions. The drawing side is renamed
   instead: a field, variable or parameter called `color` holds a color (a name), and a value to draw with is
   called `paint`. CSS's own property names and a library's types (Bevy's `Color`) are what they are.
2. **Everything written about a game uses seats, colors and labels, never paint**: notation, the log, results,
   what a screen reader hears, a bot's name. The definition ties a color to its seat and the seat to the turn
   order, and the pie rule moves the people, not the colors
   ([ADR 0056](0056-the-pie-rule-as-a-setup-choice.md)). A seat gets no notation `code` until an export (SGF,
   FEN, PGN) needs one.
3. **Paint is each player's own.** A client may offer any styles, each player chooses alone, and the server is
   not told. A client's default is its **standard look**: a color whose name is a color word the client knows is
   painted as that word says, and any other gets a paint by seat, so that two unknown colors never look the same.
4. **The paint contract**, for every style a client ships, and tested: within a game, the paints of all the
   colors in play can be told apart, and the order of lightness is kept (`white` the lightest and `black` the
   darkest of the game's colors).
5. **The key.** Wherever a client names a seat or a color, it shows the viewer's own paint beside the name, so
   each player can turn their own picture into the shared names at a glance.
6. **One resolver per client** turns a color into paint; no other code reasons about hue.
7. **Not decided: references in chat** (a position or a seat's name in a chat line, drawn in each reader's own
   look). Deferred until players report that the confusion happens. It is deferred, not rejected.
8. **The word is spelled `color`** everywhere in both repositories.

The four words are in CLAUDE.md's Words and in the glossary ([docs/glossary/en.md](../glossary/en.md)). `seat` is
a word of the interface, with a catalog key, in the glossary's first table; `color`, `label` and `paint` are words
of the code and the docs that the interface shows nowhere, so they are in a second table, of terms and meanings
only, which the other languages' files do not repeat and the glossary's test does not hold to a catalog.

## Consequences

- Nothing stored or sent changes: positions, notation and the API keep their color words, as ADR 0124 decided.
  What changes is the clients' code and what their styles must keep.
- A style that keys its look by a color's name may paint a first player cold (Go's black in today's board themes).
  That is allowed; what a style must keep is the paint contract, not warm and cold by seat.
- **The work is staged, one issue and one pull request each.** In bingle:
  1. #335: this ADR, the Words and the glossary.
  2. #336: the word is spelled `color` everywhere, with a check that keeps it so. After #335.
  3. #337: a web file says `paint` for a value to draw with. After #336.
  4. #338: one place in the page turns a color into paint, and an unknown color is painted by seat. After #337.
  5. #339: every board theme is held to the paint contract. After #338.
  6. #340: the key, a seat's or a color's name with the viewer's paint beside it. After #338.

  In bingle-3d:
  1. bingle-3d #88: the four words, and `paint` in the appearance format. After #335.
  2. bingle-3d #89: the word is spelled `color` everywhere. After bingle-3d #88.
  3. bingle-3d #90: every shipped look is held to the paint contract. After bingle-3d #88.
  4. bingle-3d #91: the key. After bingle-3d #88.

  References in chat have no issue. The private overlay has none either: #336 and #337 say what to check there.
- Until #336 lands, existing text keeps the spelling it has; what is added from now on says `color`.
