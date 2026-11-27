# 0117. Groups by color, enemies by seat

Status: accepted (2026-11-06). It is addition C of the DSL coverage audit (`docs/history/dsl-coverage-audit-plan.md`
on `more-plans`), after [ADR 0115](0115-a-declared-turn-of-several-selections.md)'s turns of several selections and
[ADR 0116](0116-effects-at-the-end-of-a-turn.md)'s effects at the end of a turn.

## Context

In a declared game a seat plays one color, its `color`. Every piece it places takes that color, and the engine
answers two different questions with the same comparison:

- **Is this piece the mover's?** To move it, to tell a capture from a block, to flank for `custodian`, for `absent`.
- **Is this piece in the same group?** For `captures`, a group is the connected pieces of one color.

With one color per seat the answers agree. **Blooms** is where they part: Warm plays red and yellow, Cool blue and
black. A bloom is one color, so a red group and a yellow group touching are two blooms, each with its own liberties;
but a fenced yellow bloom is Warm's, so Warm does not capture it and Cool does. Today Blooms' Go ruleset handles this,
and its YAML lists yellow and black in `extra_colors` and in each seat's `blend` only so they can be drawn.

Other games give a seat several colors too: four-color Go variants played by two, partnership games where a pair of
seats share a side. The audit found this addition small and common enough to declare.

## Decision

### A seat's colors

A seat may list **`colors`**, every color it plays, its `color` first:

```yaml
seats:
  - {seat: 1, color: red, colors: [red, yellow], label: Warm, blend: [red, yellow]}
  - {seat: 2, color: blue, colors: [blue, black], label: Cool, blend: [black, blue]}
pieces:
  red: {label: Red stone, code: R, shape: disc, color: red}
  yellow: {label: Yellow stone, code: Y, shape: disc, color: yellow}
```

- `colors` defaults to `[color]`, so a game that does not declare it plays exactly as now.
- A color belongs to one seat at most, and a seat's color is not also listed in `extra_colors`.
- `color` stays the seat's own: its reserves, its pools, its swatch, and the color notation and the API name it by.
- `blend` stays a drawing instruction and nothing more.

### Which color a piece is placed in

A piece type with a fixed `color` that is one of the placing seat's colors is placed in that color. Any other piece
is placed in the seat's `color`, as now. A piece type whose fixed `color` is another seat's is that seat's alone to
place, so Warm is never offered a blue stone. With an opening that places for the opponent (`owner: opponent`), the
opponent's colors are the ones that count. A piece keeps its color when it moves. A `flips` placement turns the
flipped pieces to the color of the piece placed.

### Mine and theirs, by seat

Wherever the rules ask whether a piece is the mover's (which pieces it may move, whether a destination is a capture
or a block, the flanking piece for `custodian` and `flips`, `absent` and `reach`), a piece is the mover's when its
color is one of the mover's seat's `colors`. An **opponent's** piece is one whose color belongs to another seat. A
piece in an `extra_colors` color is nobody's, as now.

### Groups by color

A group is still the connected pieces of **one** color. So:

- **`captures`** removes the groups an opponent owns that the placed piece leaves without liberties. A group of the
  mover's other color is never captured by the mover's placement, with or without liberties. The suicide test, as now,
  looks only at the group of the piece placed.
- **`remove-groups`** (ADR 0116) reads `whose` through the same ownership: `opponents` removes groups of any color an
  opponent owns, `mover` any of the mover's colors.

### Reserves and pools

Reserves and pools belong to the seat and are kept under its `color`, whatever color a piece is placed in. Placing a
yellow stone from reserve draws on Warm's reserve; `count_in` credits Warm's pool with every stone removed, of either of
Cool's colors.

### What a client sees

A piece's color in the state is the color it was placed in, which may be a seat's second color. The seats the
session describes carry `colors`, so a client can tell whose a piece is without knowing the game.

## Consequences

- With A, B and C, Blooms can be declared: its YAML gains `colors`, a turn (ADR 0115), an `end_of_turn` effect (ADR
  0116), a capture pool and a `pool` win, and its Go ruleset goes, kept for one release behind the differential harness.
- No shipped declared game gives a seat more than one color, so all of them play as now; the harness holds them to it.
- The schema, `docs/dsl.md`, the compiler (a color owned twice, a seat color also in `extra_colors`, `color` not
  first in `colors`), the engine's ownership checks, the state's validation and the session's seat description change
  together.
- The engine's ownership checks become one question, "whose is this color", asked in one place, rather than a
  comparison with the seat's color repeated across the actions.
