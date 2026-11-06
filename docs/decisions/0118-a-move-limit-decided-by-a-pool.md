# 0118. A move limit decided by a pool

Status: accepted (2026-11-06). Built with the Blooms declaration (#138), the first conversion of the DSL coverage
audit (`docs/history/dsl-coverage-audit-plan.md` on `more-plans`).

## Context

An `end` rule says when a game stops (`ply-limit`, `passes`, `board-full`, …) and `decide` says who has won then.
`most` gave the win to the seat with the most matching pieces on top of the board. Blooms ends at a move limit too, but
the player with more **captures** wins, and captures are not on the board: since [ADR 0116](0116-effects-at-the-end-of-a-turn.md)
a declared game counts them in a reserve pool (`count_in`). The same is true of any game that scores into a pool,
Pente's captured pairs among them.

## Decision

`decide: {type: most}` takes an optional **`pool`**: each seat's count is then its reserve pool of that name, not its
pieces on the board.

```yaml
- {id: limit, type: end, when: [ply-limit], plies: 250, decide: {type: most, pool: captured}, reason: ply-limit}
```

- A tie is a draw, as for `most` by pieces.
- The pool must be declared in `reserves`, as the `pool` win rule already requires.
- A pool and a piece filter (`tag`, `piece`) do not go together: the catalog refuses both at once.

It is a parameter of an existing rule rather than a new rule: "who has more" is the same question whether the count is on
the board or in a pool.

## Consequences

- Blooms' move limit is declared, and the differential harness holds the declaration to the ruleset's results there.
- No other game changes: `pool` is absent from every definition shipped before it.
