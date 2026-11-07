# Plan: a patience vocabulary for the DSL

Status: **planning in progress** (opened 2026-11-06). This is addition F of the DSL coverage audit
(`dsl-coverage-audit-plan.md`, *The additions*), which gave it a plan of its own as the largest
addition proposed. The open questions were settled on 2026-11-06 (*What was decided*, below).
Status update (2026-11-07): **done**. All five stages are built, and Solitaire is played from its
declaration (#153).

## What was asked for

The audit found that Klondike, Spider and FreeCell, three of the four games in Solitaire, can be
declared once the DSL can say what patience games say, and that one well-made patience vocabulary
opens up hundreds of known games: the best value for plugins in the catalog. Bowling Solitaire is
unique and stays in Go.

## Where things stand

- **Additions A–E are built** (ADRs 0115–0120). The relevant ones are:
  - D, a seeded deal at the start (ADR 0119): a declared game holds `cards` in zones and deals them
    before the first move. Players and bots see only what the zones show. Drawing during play was
    left for this plan.
  - E, move cards (ADR 0120): a card can be played from a zone. A patience move is a different
    thing: it moves cards between piles.
- **Solitaire today** is one ruleset of 1,657 lines. `rules.go` (366) is the shared frame: the start,
  the end, the take-back guard, determinizing and the hint order. `patience.go` (541) holds Klondike,
  Spider and FreeCell, switched by `params.mode`. `bowling.go` (417) is Bowling.
- **Its four modes share one definition.** The zones are listed once, the variants differ by `params`
  and by deck patches, and the page draws them with `static/js/solitaire.js` from the ruleset's view
  (`view.table` plus `view.solitaire`, the mode).

What the three patience modes do, read from `patience.go`:

| | Klondike | Spider | FreeCell |
|---|---|---|---|
| Deck | 52 | 104 (1, 2 or 4 suits) | 52 |
| Columns | 7, sizes 1–7, top card up | 10, sizes 6/5, top card up | 8, sizes 7/6, all up |
| Build in a column | down, alternating colour | down, any suit | down, alternating colour |
| What moves | an ordered run | a run of one suit | a run as long as the free cells and empty columns allow |
| Empty column takes | a king | anything | anything |
| Foundations | 4, by suit, ace up | none: a whole king-to-ace run of one suit leaves the table | 4, by suit, ace up |
| Free cells | none | none | 4 |
| Stock | draw 1 or 3 to the waste; the waste turns over without limit | deals one card to every column, never while one is empty | none |
| From the foundations | the top card may come back to a column | — | — |
| Ends | all home; no move left | all suits off; no move left | all home; no move left |
| Also | `finish` when everything can go home | | |

## Initial thinking on the design

### A vocabulary of piles and rules, not a move language

Every patience game moves cards between **piles**, and the rules are always the same few
questions about each pile:

- What may go on it, and in what order.
- What may be taken off it: the top card, or a run, and which runs.
- What happens when it empties.
- How the stock refills the others.

A vocabulary that answers those questions as parameters covers the families below. A general
language of zone moves with conditions would be larger, harder to read than the Go it replaces (the
audit's second test), and still need these same words. I'd recommend the parameters.

A sketch, to give the discussion something concrete:

```yaml
patience:
  tableau:
    columns: 7
    deal: {sizes: [1, 2, 3, 4, 5, 6, 7], up: top}   # or up: all
    build: {order: down, suits: alternate}          # down|up|either; alternate|same|any
    move: ordered-run                                # top | ordered-run | same-suit-run | any-face-up (Yukon)
    empty: king                                      # any | king | none
    run_limit: free-cells                            # FreeCell's (cells+1)·2^empty
  foundations: {count: 4, by: suit, from: ace, build: up, takes_back: true}
  cells: 4
  stock:
    to: waste                 # draw to the waste ...
    draw: 1                   # ... draw cards at a time
    redeals: unlimited        # or a number
  # or, Spider:  stock: {to: columns, unless_empty: true}
  complete: {run: king-to-ace, suits: same, off: true}   # Spider's finished suits
  win: all-home               # or all-off
  finish: true                # offer "finish" when every card can go home
```

It would compile to one more action type, as `card-move` did, that lists and validates the patience
moves of the position. The position is the `cards` table of ADR 0119, with nothing new stored.

### Keep the notation

The notation Solitaire writes today stays, so the differential harness can compare by notation and
recorded games still read:

- piles are `t1`…`t10`, `w`, `c1`…`c4` and `f1`…`f4`;
- a run of several cards says how many: `t3.2-t5`;
- a card going home is `-f`, and a card going to a free cell is `-c`;
- the other moves are `draw`, `recycle`, `deal` and `finish`.

A game that declares other piles names them in the same pattern.

### What the engine needs besides the vocabulary

- **Chance during play ends the impulse.** Drawing from the stock and uncovering a face-down card
  show the player a card they could not see. ADR 0114 says such a selection ends the impulse, and
  Solitaire's `CanTakeBack` refuses to take it back. A declared game needs the same rule, worked out
  from the zones: a selection that makes a hidden card visible to the mover cannot be taken back.
  ADR 0119 left this for here. It is general, not patience's own, so it would be an ADR of its own.
- **The colour of a suit.** Klondike and FreeCell alternate red and black. Solitaire reads colour from
  a suit's place in `suit_order` (odd and even), because a themed set of cards wears the suits by
  place. A declared suit needs a colour class, or the vocabulary keeps the rule by place.
- **Hints and search.** Solitaire orders its moves for the hint button (going home first, then moves
  that turn a card or empty a column) and tells search bots when a decision is not worth searching.
  A declaration either gets a generic order from the vocabulary, which knows these things, or loses
  them.

### Bowling Solitaire

Bowling stays in Go, but it is a variant of the same definition today, and a variant cannot change
engine. Either Bowling becomes a game of its own id, which changes its id and its record, or the
catalog lets a variant name a ruleset when the base is declared. This has to be decided before
Solitaire converts.

### The families it would open

| Family | Examples | Needs beyond Klondike, Spider and FreeCell |
|---|---|---|
| Builders with a stock | Klondike, Thoughtful, Westcliff | nothing |
| Spider-likes | Spider, Scorpion, Spiderette | `move: any-face-up` (Scorpion) |
| Cell games | FreeCell, Baker's Game, Eight Off, Seahaven | `build.suits: same`, more cells |
| Yukon-likes | Yukon, Russian Solitaire | `move: any-face-up` |
| Two-deck builders | Forty Thieves, Napoleon's Square | eight foundations, `empty: any` |
| Open builders | Baker's Dozen, Beleaguered Castle | `empty: none`, no stock |

**Golf, TriPeaks and Pyramid** are a different shape. They play onto one waste by rank (one up or
one down) or remove pairs that add up to 13. They are popular, but they would be a second, smaller
vocabulary later, not part of this one.

## What was decided (2026-11-06)

1. **Parameters.** A `patience:` block of parameters, compiled to one action type.
2. **The cheap neighbours come in the first ADR.** Yukon and Scorpion (`move: any-face-up`),
   Baker's Dozen (`empty: none`) and the two-deck Forty Thieves are added alongside Klondike, Spider
   and FreeCell.
3. **Bowling Solitaire becomes a game of its own**, in Go, before Solitaire converts.
4. **The take-back rule is settled first**, as a general ADR: a declared selection that shows the
   mover a card they could not see cannot be taken back.
5. **Suit colour stays by place** in `suit_order`, as now, so themed card sets keep working.
6. **A generic hint order** comes from the vocabulary: going home first, then moves that turn a card
   or empty a column, then the rest, with the stock last.
7. **The page:** a declared patience game sends the view shape Solitaire sends today, and
   `solitaire.js` stays as it is.

## Open questions for the discussion

These were the questions put to the maintainer; the answers are above.


1. **Parameters or primitives?** Is a `patience:` block of parameters, compiled to one action type,
   the right shape? I'd recommend it.
2. **Which families are in the first ADR?** Klondike, Spider and FreeCell are needed. Should Yukon
   and Scorpion (`any-face-up`), Baker's Dozen (`empty: none`) and the two-deck Forty Thieves come
   in the same ADR? They are small and cheap to add while the vocabulary is new.
3. **Bowling:** a game of its own, or a ruleset variant of a declared base?
4. **The take-back rule** for a selection that turns up a hidden card: agreed as a general ADR before
   the vocabulary? I'd recommend it, as it is the part of ADR 0119 that was left open.
5. **Suit colour:** a `color_class` on suits, or kept by place?
6. **Hints:** a generic hint order from the vocabulary, or none for declared patience?
7. **The page:** it draws Solitaire from the ruleset's view. A declared patience game would send the
   same `view.table` and `view.solitaire` shape, so `solitaire.js` can stay as it is. That would be
   the second declared view, after the card table Onitama's step 2 needs.

## Proposed stages, once the questions are settled

1. An ADR for the take-back rule on revealed cards, built and tested on a small fixture.
2. An ADR for the patience vocabulary: Klondike, Spider and FreeCell, with Yukon, Scorpion, Baker's
   Dozen and Forty Thieves, suit colour by place, and the generic hint order.
3. Build the vocabulary, with a fixture per family in `tests/`.
4. Bowling Solitaire moved to a game of its own.
5. Klondike, Spider and FreeCell declared beside the ruleset and checked by `tests/difftest`
   (conversion step 1), as Blooms and Onitama were. Step 2 follows one release later.

## Dependencies and cross-plan effects

- **The DSL coverage audit:** this plan is its addition F, and its order puts F last.
- **Game plugins:** patience games are the first data-only pack the audit thinks worth shipping on its
  own.
- **The page:** the declared card table that Onitama's step 2 needs comes first, and this plan reuses
  it.

## Status log

- 2026-11-06: opened. Read Solitaire's three patience modes into a table, sketched a parameter
  vocabulary with today's notation kept, and listed what the engine needs besides it: the take-back
  rule on revealed cards, the colour of a suit, and hints. Also listed the families it would open,
  the Bowling question and seven open questions. Awaiting the maintainer's review.
- 2026-11-06: the maintainer settled the seven questions: a parameter vocabulary; the cheap
  neighbours (Yukon, Scorpion, Baker's Dozen, Forty Thieves) in the first ADR; Bowling as a game of
  its own; the take-back rule as a general ADR first; suit colour by place; a generic hint order; and
  the page's patience view unchanged. Next: stage 1, the take-back ADR.
- 2026-11-06: stage 1, ADR 0121 (#145, built in #146): a card turned up cannot be taken back.
  Stage 2, ADR 0122 (#147), and stage 3, its build (#148). The build's test turns each of
  Solitaire's six patience games into a declaration and holds it to the ruleset with the harness,
  hints included. Yukon, Scorpion, Baker's Dozen and Forty Thieves are tested as fixtures. Two
  details were added to the ADR: `win_condition: {type: patience}`, and the position's `counters`
  for limited redeals. Stage 4, Bowling as a game of its own under its old id, is #149. Next:
  stage 5, Solitaire's declaration beside the ruleset.
- 2026-11-06: stage 5, Solitaire declared beside the ruleset (#150), then step 2 (#153): Klondike,
  Spider and FreeCell are played from the declaration, their ruleset is deleted and stored games are
  converted at start-up. The page draws them with the patience view it already had, so the declared
  card table this plan expected to reuse from Onitama was not needed. Done.
