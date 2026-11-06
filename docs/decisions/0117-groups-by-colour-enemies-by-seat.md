# 0117. Groups by colour, enemies by seat

Status: accepted (2026-11-06). It is addition C of the DSL coverage audit (`docs/history/dsl-coverage-audit-plan.md`
on `more-plans`), after [ADR 0115](0115-a-declared-turn-of-several-selections.md)'s turns of several selections and
[ADR 0116](0116-effects-at-the-end-of-a-turn.md)'s effects at the end of a turn.

## Context

In a declared game a seat plays one colour, its `color`. Every piece it places takes that colour, and the engine
answers two different questions with the same comparison:

- **Is this piece the mover's?** To move it, to tell a capture from a block, to flank for `custodian`, for `absent`.
- **Is this piece in the same group?** For `captures`, a group is the connected pieces of one colour.

With one colour per seat the answers agree. **Blooms** is where they part: Warm plays red and yellow, Cool blue and
black. A bloom is one colour, so a red group and a yellow group touching are two blooms, each with its own liberties;
but a fenced yellow bloom is Warm's, so Warm does not capture it and Cool does. Today Blooms' Go ruleset handles this,
and its YAML lists yellow and black in `extra_colors` and in each seat's `blend` only so they can be drawn.

Other games give a seat several colours too: four-colour Go variants played by two, partnership games where a pair of
seats share a side. The audit found this addition small and common enough to declare.

## Decision

### A seat's colours

A seat may list **`colors`**, every colour it plays, its `color` first:

```yaml
seats:
  - {seat: 1, color: red, colors: [red, yellow], label: Warm, blend: [red, yellow]}
  - {seat: 2, color: blue, colors: [blue, black], label: Cool, blend: [black, blue]}
pieces:
  red: {label: Red stone, code: R, shape: disc, color: red}
  yellow: {label: Yellow stone, code: Y, shape: disc, color: yellow}
```

- `colors` defaults to `[color]`, so a game that does not declare it plays exactly as now.
- A colour belongs to one seat at most, and a seat's colour is not also listed in `extra_colors`.
- `color` stays the seat's own: its reserves, its pools, its swatch, and the colour notation and the API name it by.
- `blend` stays a drawing instruction and nothing more.

### Which colour a piece is placed in

A piece type with a fixed `color` that is one of the placing seat's colours is placed in that colour. Any other piece
is placed in the seat's `color`, as now. A piece type whose fixed `color` is another seat's is that seat's alone to
place, so Warm is never offered a blue stone. With an opening that places for the opponent (`owner: opponent`), the
opponent's colours are the ones that count. A piece keeps its colour when it moves. A `flips` placement turns the
flipped pieces to the colour of the piece placed.

### Mine and theirs, by seat

Wherever the rules ask whether a piece is the mover's (which pieces it may move, whether a destination is a capture
or a block, the flanking piece for `custodian` and `flips`, `absent` and `reach`), a piece is the mover's when its
colour is one of the mover's seat's `colors`. An **opponent's** piece is one whose colour belongs to another seat. A
piece in an `extra_colors` colour is nobody's, as now.

### Groups by colour

A group is still the connected pieces of **one** colour. So:

- **`captures`** removes the groups an opponent owns that the placed piece leaves without liberties. A group of the
  mover's other colour is never captured by the mover's placement, with or without liberties. The suicide test, as now,
  looks only at the group of the piece placed.
- **`remove-groups`** (ADR 0116) reads `whose` through the same ownership: `opponents` removes groups of any colour an
  opponent owns, `mover` any of the mover's colours.

### Reserves and pools

Reserves and pools belong to the seat and are kept under its `color`, whatever colour a piece is placed in. Placing a
yellow stone from reserve draws on Warm's reserve; `count_in` credits Warm's pool with every stone removed, of either of
Cool's colours.

### What a client sees

A piece's colour in the state is the colour it was placed in, which may be a seat's second colour. The seats the
session describes carry `colors`, so a client can tell whose a piece is without knowing the game.

## Consequences

- With A, B and C, Blooms can be declared: its YAML gains `colors`, a turn (ADR 0115), an `end_of_turn` effect (ADR
  0116), a capture pool and a `pool` win, and its Go ruleset goes, kept for one release behind the differential harness.
- No shipped declared game gives a seat more than one colour, so all of them play as now; the harness holds them to it.
- The schema, `docs/dsl.md`, the compiler (a colour owned twice, a seat colour also in `extra_colors`, `color` not
  first in `colors`), the engine's ownership checks, the state's validation and the session's seat description change
  together.
- The engine's ownership checks become one question, "whose is this colour", asked in one place, rather than a
  comparison with the seat's colour repeated across the actions.
