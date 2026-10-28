# 0062. Meridians: a hexagon with no middle point, and death by loneliness

Status: accepted (2026-10-27)

## Context

`bingle_tasks.md` asked for one thing: "Implement the abstract game Meridians by Kanare Kato."

Meridians (2021) is a two-player placement game with an unusual death rule: nothing is captured
stone by stone, but a **group that can see no other group of its own colour is taken off the
board**. It won the Board Game Geek abstract forum's award for the best combinatorial two-player
game of its year. There is no single authoritative text online; the rules here follow the Board
Game Arena implementation's description (which that site says is more precise than its own rules
PDF), cross-read against abstractgames.org's account of the game.

## Decision

**The board is a new shape for the engine.** Meridians is played on a *centreless* hexagon: six
intersections along two sides and seven along the other four, so the board's centre of symmetry
falls between points and **no player can take the middle, because there isn't one**. The
`hexhex` generator made only regular hexagons, so it grew a second side length:

```json
"generator": { "type": "hexhex", "size": 6, "size2": 7 }
```

`board.GenerateHexHexSides(short, long)` builds `short + long - 1` columns by `2*long - 1` rows with
the diagonal band `long-1 ≤ q+r ≤ …`; with `short == long` it is exactly the old generator, so
Yavalath, Havannah, Blooms and Courtfall come out byte for byte as before (`GenerateHexHex` now
calls it). The standard board is 114 points; the 5/6 and 7/8 boards are variants.

**The rules are a Go ruleset.** Line of sight, paths between groups and the removal of dead groups
are none of them expressible in the JSON DSL. The position carries almost nothing of its own — the
stones are on the board, and `Data` holds whose turn it is, how many turns each player has taken
(the opening rules and the exemption both depend on it), each player's first stone and how the game
ended.

**Where the removal happens.** The source rules put it at the *start* of a turn: "remove all dead
groups of the opposing colour; skip this if your opponent has taken fewer than two turns". This
implementation removes **the mover's own dead groups when their turn ends**, which is the same thing
one step earlier — nothing happens between a player's placement and the next player's turn — and
leaves the board always settled, with no dead stones sitting on it for a turn. That matters for the
page, for the bots and for what a stored position means. The consequences the rules call for still
hold: a group the *opponent* cut off survives until its owner's next turn ends, so it can be saved
by giving it a new path, and the two-turn exemption keeps the opening stones (a lone group can see
nobody by definition).

**The opening.** A player's first stone goes anywhere; their second may not touch the first. Those
two rules are what make the exemption coherent: a second stone is laid in sight of the first along a
clear line, and not touching it, so the two groups have a path between them and neither is dead.

**Passing.** There is no passing while a player has a move; a turn with nowhere to play passes
(`pass`), and two passes in a row end the game as a draw, as does the 1200-ply limit. A win is
reported the moment the loser has nothing left, rather than waiting for the turn that would begin
with an empty board.

**The page needs no new code.** The board draws itself from the definition (`style: "points"`), a
move is one click, and the derived view marks the groups that will die (`badges.dead`), the last
stone (`tints.last`) and each player's stones and groups — all through the legend and view shapes
ruleset games already use.

## Consequences

* The **pie rule** (offered at some tables) is not here: it is the generic engine's `swap` action,
  which a Go ruleset does not use (ADR 0056). Meridians could offer a swap of its own later; it
  would be a rule of the ruleset, not an attribute.
* The minimax bot gets a heuristic (stones, less the groups about to die); MCTS needs none and is
  the better bot here, as in every game whose value is shape rather than material.
* `size2` is the first asymmetric board the generator makes. Any game wanting a hexagon with two
  side lengths now has one.
