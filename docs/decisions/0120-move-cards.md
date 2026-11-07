# 0120. Move cards

Status: accepted (2026-11-06). It is addition E of the DSL coverage audit (`docs/history/dsl-coverage-audit-plan.md`
on `more-plans`), after [ADR 0119](0119-a-seeded-deal-at-the-start.md)'s deal at the start.

## Context

Since ADR 0119 a declared game can deal cards into zones, but no card does anything once dealt. In **Onitama** a
card *is* the move: each Move card shows the squares a piece may reach, as offsets from the piece, read from the
point of view of the player holding it (so the second player's cards are read upside down). On your turn you choose
one of your two cards and move one of your pieces by one of its offsets, capturing an enemy piece you land on. The card
you used then goes to the side of the mat, and you take the card that was waiting there. When no card moves any piece,
you still choose a card and exchange it.

Card-driven movement is common beyond Onitama (the audit counts it "moderate"). What a card *is* stays the game's: a
name and a set of offsets. What playing it means is the same each time: choose a card in your zone, move a piece as it
says, and pass the card on.

## Decision

### A card's moves

A card type may list **`moves`**: offsets as `[right, forward]` from the moving piece, as its owner sees the board.

```yaml
cards:
  decks:
    move:
      extra:
        - {id: tiger, label: Tiger, moves: [[0, 2], [0, -1]]}
        - {id: crab, label: Crab, moves: [[0, 1], [-2, 0], [2, 0]]}
```

The offsets are turned by the moving seat's **`forward`** (the seat field the movement directions already use): with
`forward: N` forward is up the board, with `forward: S` everything is read upside down. A game with moves on its cards
gives each seat a `forward`.

### Playing a card

A new action type, **`card-move`**:

```yaml
actions:
  - id: move
    type: card-move
    hand: hand            # a seat zone: the cards the mover may play
    pieces: [master, student]
    pass_on: side         # the used card goes here, and the mover takes what lay there
    stuck: spend          # when no card moves a piece, a card is still played, without a move
```

- A selection names the **card** and the piece's squares. Its notation is `tiger:c1-c3`, and `tiger:pass` for a card
  spent without a move. The selection carries the card in a new field, `card`.
- The piece **leaps**: it lands on the offset's square whatever lies between. It may not leave the board or land on a
  piece of its own side (ADR 0117), and an opposing piece it lands on is captured.
- **`pass_on`** is a table zone. Once the piece has moved, the mover takes every card in that zone into its `hand`, then
  the used card goes into `pass_on`, face up. With Onitama's single side card that is the exchange.
- **`stuck: spend`** offers each card in the hand as `card:pass` when no card moves any piece, and the card is passed
  on as if it had been used. Without `stuck` the seat simply has no selection then, and the turn order's rules for a
  stuck seat apply.
- A gesture for a card is new, `card`: the page shows the mover's hand, a card chosen first, then the piece and the
  square, as Onitama's page does today. The page learns it when a declared game with move cards is first played, at
  Onitama's conversion, since until then no shipped game declares one.

### A piece of one seat's gone

Onitama is won by capturing the other Master, and both Masters are the same piece type. The `absent` win rule fires
when a piece type has left the board altogether; it gains **`of_seat`** (`of` was taken: connect rules list regions under it), a seat: the rule fires when no piece of that type
**in that seat's colours** is left.

```yaml
- {id: stone-red, type: absent, piece: master, of_seat: 2, seat: 1, reason: master-captured}
```

### What stays as it is

- Cards that do something other than move a piece (Nyet!'s, a trick's) are not declared by this.
- Drawing a card from a deck during play is still not declared (ADR 0119).

## Consequences

- With D and E, Onitama can be declared: sixteen cards with their moves, three zones, the deal, a `card-move` action,
  two `absent` and two `reach` rules and a ply-limit. Its declaration goes beside the ruleset under the harness, as
  Blooms' did, after its ruleset first deals through `cards.RNG` (ADR 0119).
- The schema, `docs/dsl.md`, the compiler (moves on a card without a `forward` on every seat, a `hand` that is not a
  seat zone or a `pass_on` that is not a table zone, `of_seat` naming an unknown seat), the selection (`card`), notation and
  gestures, the engine and the page's card table for declared games change together.
- Declared games without `card-move` play as now; the harness holds every shipped game to it.

## Addendum (2026-11-07): Onitama stays in Go

Onitama's conversion stops after step 1. The maintainer means to add Onitama's expansions, which may
need rules the DSL does not have, so its Go ruleset stays the one the server plays and its declaration
is deleted rather than kept in step with a ruleset that will move on. Move cards stay in the DSL as
decided here, held to their rules by the small fixture in `tests/cardmove`; the page's card table for
declared games waits for a game that uses them.
