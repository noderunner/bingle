# 0115. A declared turn of several selections

Status: proposed (2026-11-06). It is addition A of the DSL coverage audit (`docs/history/dsl-coverage-audit-plan.md`
on `more-plans`) and the DSL step of [ADR 0114](0114-impulses-selections-moves-and-gestures.md)'s second phase.

## Context

In a declared game a turn is one selection. `turn_order` is round-robin, `opening` restricts the first plies, and
`then` (Amazons' shot) packs a second choice into the same selection, with a notation of its own (`d1-d5/f5`). Every
game whose turn is made of several choices is written in Go: Blooms' one or two stones of different colours, Onitama's
card and move, Fendo's move and then fence, Yinsh's marker and ring move. Outside the catalog the shape is everywhere:
Connect6's two stones, Arimaa's four steps, every move-then-act game.

ADR 0114 already gives the platform what this needs: an impulse of several selections, submitted as one move and
written with its selections joined by `; `, a selections query that walks it one selection at a time, and a pending
move in the page. Only the declaration and the engine's generic rules are missing.

## Decision

### The declaration

`turn_order` takes a **`turn`**: the parts of a turn, in order. Each part names the actions it may use and how many
selections it takes.

```yaml
turn_order:
  type: round-robin
  seats: [1, 2]
  turn:
    - {actions: [place], min: 1, max: 2, distinct: piece}   # Blooms: one or two stones, of different colours
  opening:
    plies: 1
    turn:
      - {actions: [place], max: 1}                          # Player 1's first turn is a single stone
```

| field | meaning |
|---|---|
| `actions` | the action ids this part may use (required) |
| `min` | selections the part must have, from 0 (default 1) |
| `max` | selections it may have (default `min`, or 1 when `min` is 0) |
| `distinct` | `piece`: no two selections of the part place or move the same piece type |

Without `turn`, a turn is one selection of any action, as today, so no existing definition changes. An `opening`
may carry a `turn` of its own for its plies, next to its `actions`, `pieces` and `owner`.

Some sample turns:

```yaml
turn: [{actions: [place], min: 2}]                             # Connect6, after a one-stone opening
turn: [{actions: [move]}, {actions: [fence]}]                  # move, then fence
turn: [{actions: [move], min: 0}, {actions: [fence]}]          # a move if you like, then a fence
```

### How a turn goes

- **The legal selections** are those of the current part's actions, filtered by `distinct` and by the part's `max`.
  Once the part has its `min`, the first actions of the next part are legal too, so a player moves on by choosing one,
  and a part with `min: 0` can be passed over the same way. If an action is in both parts, the selection counts in the
  current part.
- **`done`** is offered once every part that remains has its `min` and the turn may stop. It is a selection with the
  notation `done` and the gesture `action/done`, like Blooms'. A definition may not declare an action of that id.
- **The turn ends** after `done`, when the last part reaches its `max`, or when nothing is legal and nothing more is
  required. If a required part has nothing legal, the turn ends there too, as if the part were optional: the engine never
  leaves a seat with no selection and no way to stop. `pass` ends the turn wherever it comes.
- **A selection that does not end the turn keeps the seat**, so `NextImpulseSeat` is the mover's and, by ADR 0114's
  rule, the impulse goes on. The page builds it as a pending move, and bots and environments play it one selection at a
  time, as in Blooms today.

### What counts a turn, not a selection

- **The ply** advances once a turn ends, so a ply is one impulse (ADR 0114). `opening.plies`, the `ply-limit` end
  trigger and every `plies` list (`swap`'s, say) count turns.
- **Ko** compares with the position before the opponent's last turn, not before its last selection.
- **`passes`** counts turns that were a `pass`.
- **The position records the turn in progress**: the part, how many selections it has, and the pieces `distinct` has
  used. These go in a new optional field of the position document, `turn`, which is absent between turns, so every
  stored position stays valid and the document's version stays 2.

### Wins

The win rules are checked after every selection, as now. A turn may end the game halfway (Connect6's first stone of a
six), and the impulse ends with it. End triggers about the seat to move (`no-legal-moves`) and those that count plies
are looked at only when the turn ends.

### What stays as it is

- **`then` stays.** It is one selection with one notation, and Amazons' stored games and notation depend on it. A
  definition could write the same thing as two parts, but that is another game.
- **Capture rules attached to an action** fire on that selection. Effects at the end of a turn, such as Blooms' captures
  of the opponent only when the mover's turn ends, are addition B, with their own ADR.
- **No condition between parts**, such as "move the ring you just placed a marker in" (Yinsh) or "a push is two of
  your four steps" (Arimaa). Each is a rule of its own game. The audit reassesses Yinsh once this exists.

## Consequences

- A game with a turn of several choices can be declared, and Blooms needs this together with B and C. Connect6 needs
  this alone, and is the first declared game the tests can play with it.
- The schema, `docs/dsl.md`, the compiler's checks (unknown actions, `min` above `max`, an action named `done`), the
  engine and the action space (a fixed `done` id) change together. A game that declares no `turn` plays exactly as now:
  the differential harness holds every shipped game to a copy of itself before and after.
- Blooms converted later keeps its notation and stored games: its selections and `done` are the same, and its ply
  already counts turns (#129).
- Hidden or random selections in the middle of a turn wait for addition D. A roll or a draw would end the impulse there
  (ADR 0114), and the turn would go on in the same seat's next impulse.
