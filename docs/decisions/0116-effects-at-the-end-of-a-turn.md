# 0116. Effects at the end of a turn

Status: accepted (2026-11-06). It is addition B of the DSL coverage audit (`docs/history/dsl-coverage-audit-plan.md`
on `more-plans`), after [ADR 0115](0115-a-declared-turn-of-several-selections.md)'s turns of several selections.

## Context

In a declared game everything happens when a piece lands: a `place` action's `captures` removes the opposing groups
its stone leaves without liberties, and `custodian` removes the runs it closes. Nothing can happen when a turn ends.
Some games need exactly that timing:

- **Blooms** removes fenced blooms, groups of one colour with no empty cell beside them, when the mover's turn ends, and
  only the opponent's. The mover's own fenced blooms stay until the opponent's turn ends, so the opponent can free them
  first. A turn has up to two stones, so capturing as each stone lands would decide too early.
- **Meridians** removes dead groups at the end of a turn. The audit keeps it in Go, since what "dead" means is its own,
  but the timing is the same.
- Go-like games with a turn of several stones would capture after the turn as a whole, not stone by stone.

Blooms' page also tints the fenced blooms, so a player sees what the end of the turn will take. The test that removes a
group is the one the tint shows.

## Decision

### The declaration

A game may list **`end_of_turn`** effects, which happen in order whenever a turn ends.

```yaml
end_of_turn:
  - effect: remove-groups
    condition: no-liberties   # a group with no empty position beside it
    whose: opponents          # the mover's opponents' groups, not the mover's own
    count_in: captured        # credit the mover's pool with the pieces removed
    tint: fenced              # mark such groups on the board, between turns and during them
```

| field | meaning |
|---|---|
| `effect` | `remove-groups`, the only effect for now |
| `condition` | `no-liberties`: no position adjacent to the group is empty |
| `whose` | `opponents` (default): every seat but the mover's; `mover`; or `all` |
| `count_in` | a pool of `reserves` to credit the mover with the pieces removed, as for `custodian` |
| `tint` | a key of `display.tints`: the board marks every group that meets the condition |

- A **group** is the pieces of one colour connected through the board's adjacency (hex boards included), as `captures`
  counts them. Only the top piece of a stack counts. Which seat a colour belongs to is the seat's `color`; addition C
  lets a seat have several colours, and this ADR uses whatever C decides.
- **A turn ends** as ADR 0115 says: after its last selection, after `done`, or after a `pass`. A game with no `turn`
  ends a turn after every selection, so the effects run after each one. A `swap` changes nothing on the board, so no
  effect runs after it.
- **The effects run before the turn is judged.** Pieces are removed, and pools credited, before the win rules look at
  the position, before `pass_when_stuck` and `no-legal-moves` ask who can move, and before the position is stored. A
  game won by a `pool` rule is won by the captures of the turn that reached the target.
- **Several groups** meeting the condition are removed together, all decided on the position before any is taken, so the
  order of removal never matters.

### The tint

A `tint` names a key of `display.tints`, with its label, colour and text, as Blooms declares `fenced` today. The state
the server sends for a declared game then carries the cells to tint, worked out from the position and never stored:
every group meeting the condition, whoever's they are. For Blooms that covers its own fenced blooms too, which the
opponent will take at the end of their turn unless they are freed first. During a turn of several selections the
tint follows the position after the selections made so far, so a player sees what ending the turn would take.

### What stays as it is

- `captures` and `custodian` still act as a piece lands. A game may use both kinds: a capture on landing and an effect
  at the end of the turn.
- A placement that leaves the mover's own group without liberties is legal unless a `captures` rule forbids suicide.
  `remove-groups` adds no rule of its own about it, and in Blooms such a group is simply removed at the end of the
  opponent's turn.
- No other conditions for now. A condition that only one game uses, such as Meridians' "no straight line of empty points
  to another of your groups", is that game's rule and stays in Go. A new condition earns its place the way any addition
  does: by being common.

## Consequences

- Blooms can be declared once C exists. Its captures, the counts on its scoreboards (a `count_in` pool), its fenced
  tint and the moment it captures all come from this effect.
- The schema, `docs/dsl.md`, the compiler (an unknown effect, condition or `whose`, a pool that does not exist, a tint that
  `display.tints` does not declare), the engine and the state the server sends for a declared game change together.
- A game that declares no `end_of_turn` plays exactly as now; the differential harness holds every shipped game to that.
- The page already draws a ruleset's tints, so it needs to read them from one more place, not learn to draw them.
