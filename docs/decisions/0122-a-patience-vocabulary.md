# 0122. A patience vocabulary

Status: proposed (2026-11-06). It is addition F of the DSL coverage audit, and stage 2 of the patience vocabulary plan
(`docs/history/patience-vocabulary-plan.md` on `more-plans`), whose seven questions the maintainer settled on
2026-11-06. It builds on [ADR 0119](0119-a-seeded-deal-at-the-start.md)'s cards and on
[ADR 0121](0121-a-card-turned-up-cannot-be-taken-back.md)'s take-back rule.

## Context

Solitaire plays Klondike, Spider and FreeCell from 541 lines of Go, switched by `params.mode`. Every patience game
moves cards between piles, and its rules answer the same few questions about each pile:

- what may be built on it, and in what order;
- what may be lifted off it: the top card, or a run, and which runs;
- what an emptied pile takes;
- how the stock gives out its cards;
- when a game is won.

The plan settled on answering those questions with parameters compiled to one action type, rather than a language of
zone moves with conditions. It also settled that Klondike, Spider and FreeCell come with their cheap neighbours:
Yukon, Scorpion, Baker's Dozen and Forty Thieves.

## Decision

### The declaration

A declared game with `cards` may give a **`patience`** block. The catalog then adds one action, `patience`, which
lists and plays the moves the block allows.

```yaml
patience:
  columns:
    count: 7
    deal: {sizes: [1, 2, 3, 4, 5, 6, 7]}   # face down but the top card (down: [...] counts them per column)
    build: {rank: down, suit: alternate}
    lift: ordered                           # top | ordered | same-suit | any-face-up
    empty: king                             # any | king | none
  foundations: {count: 4, back: true}       # each built up by suit from the ace
  stock: {draw: 1, to: waste, redeals: unlimited}
  finish: true
```

**Columns** (the tableau, `t1`…`tN`):

| field | meaning |
|---|---|
| `count` | how many columns |
| `deal.sizes` | the cards each column is dealt (one number for all, or a list) |
| `deal.down` | how many of them lie face down (default: all but the top card; `0` for all up; a list per column) |
| `deal.kings_under` | kings are put at the bottom of their column (Baker's Dozen) |
| `build.rank` | `down` (default) or `up`: a card goes on one a rank above (below) it |
| `build.suit` | `alternate` (colour), `same` or `any` |
| `lift` | what may be moved off a column together: `top` (one card), `ordered` (a run that builds by `build`), `same-suit` (an ordered run of one suit, Spider), `any-face-up` (any face-up card with everything on it, Yukon and Scorpion) |
| `lift_limit` | `cells`: a run is at most (free cells + 1) × 2^(empty columns), FreeCell's |
| `empty` | what an empty column takes: `any` (default), `king` (a king, or a run from a king) or `none` |

**Foundations** (`f1`…`fN`): `count` piles, each built up in one suit from the ace (`count: 8` for two decks). With
`back: true` the top card of a foundation may come back to a column (Klondike). A game without `foundations` wins
another way (`off`, below).

**Free cells** (`c1`…`cN`): `cells: N`, each holding one card.

**The stock**, which holds what the deal leaves:

- `{draw: N, to: waste, redeals: unlimited | M}`: turn N cards onto the waste (`w`). Only the waste's top card is
  played. An empty stock may take the waste back, turned over, `redeals` times (Klondike, Forty Thieves).
- `{deal: row, columns: all | N, unless_empty: true}`: deal one card face up to each column (or to the first N):
  Spider's, never while a column is empty, and Scorpion's three.

**Off**: `off: {run: king-to-ace, suit: same}` takes a full run off its column as soon as it is built, into a pile
of finished suits (Spider, Scorpion).

**Winning and the end:** the game is won when every card is home in the foundations, or off with `off`. It ends,
lost, when no move is left. `finish: true` offers `finish` once every card can go home from where it lies with nothing
hidden, and plays those moves at once.

### The notation is Solitaire's

The moves are written as Solitaire writes them now, so a recorded game reads the same and the differential harness
compares by notation:

- `t3-t5` moves a card, and `t3.2-t5` a run of two;
- `w-f` and `t1-f` send a card home (to its suit's foundation, the first that takes it with two decks);
- `t1-c` moves a card to a free cell, whichever is free;
- `c2-t4` and `f1-t3` play a card from a cell or a foundation;
- `draw`, `recycle` (the waste back to the stock), `deal` and `finish`.

The gestures are the pile to lift from (and, for a run, the card it starts at), then the pile it goes to, as the
page's patience table makes them today.

### Suit colour, hints and search

- **Colour** is a suit's place in `suit_order`: odd places are one colour and even places the other, as Solitaire
  reads it now. A themed set of cards wears the suits by place, so the rule holds whatever the cards look like.
- **A hint** comes from a generic order of the moves. `finish` comes first, then a card going home. After that, a move
  that turns a card over, then one that empties a column, then a move off a cell, then one that builds in suit. Other
  moves follow, then the stock, and a card back from a foundation or into a cell comes last. It is Solitaire's order,
  read from the vocabulary rather than from the mode.
- **Search** is not worth it for patience. A bot plays from the hint order, and the position is valued by the share of
  cards home or off.

### Take-back

A draw from the stock and a card turned over in a column show the player cards they could not see, so ADR 0121 ends
the impulse there. Every other move may be taken back, as now.

## Consequences

- Klondike (draw one or three), Spider (one, two or four suits) and FreeCell can be declared, and so can Yukon,
  Scorpion, Baker's Dozen and Forty Thieves, each in a few lines. The page's patience table needs the view it reads
  now (`view.table` and `view.solitaire`), which a declared patience game sends.
- Bowling Solitaire leaves Solitaire for a game of its own before Solitaire converts (the plan's decision 3).
- The schema, `docs/dsl.md`, the compiler, the engine (one action type, its notation, gestures, hints and value), and
  the declared view for patience change together. A fixture for each family goes into `tests/`.
- Golf, TriPeaks and Pyramid, which play onto one waste by rank or remove pairs, are not covered; they would be a
  smaller vocabulary of their own, later.
