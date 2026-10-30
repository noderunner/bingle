# Mycelium

Mycelium is a two-player game of competing fungal colonies invented by Miles Stevenson in 2024.

## Components

- A size-5 hex-hex board (61 cells).
- Black and green nodes (stones).

## Setup

Black plays first. The board begins empty.

## Normal turns

On your turn, place one node of your colour on any empty cell.

If your placement occupies the centre cell (e5), you trigger a **confluence turn**. Normal turns pause while the confluence is resolved.

## Clusters

A **cluster** is a connected group of nodes of one colour.

- A **spore cluster** has no enemy neighbours.
- A **hyphae cluster** touches at least one enemy node.
- A **strand** is a straight line of nodes inside a hyphae cluster.

## Confluence turn

A confluence has two phases, both resolved in **initiative order**:

1. **Spore phase**: every spore cluster spreads.
2. **Hyphae phase**: every hyphae cluster spreads.

Initiative starts at 1 and counts upward. Only clusters whose size equals the current initiative may spread. If both players have eligible clusters, the player who did **not** start the confluence chooses first, and the players alternate until every cluster of that size has spread.

### Spore spread

Select any node in the spore cluster as the source. Trace a straight line across empty cells to the first enemy node. The source node moves to the enemy node's cell; the enemy node is removed.

### Hyphae spread

Select one strand in the hyphae cluster. Convert up to as many adjacent enemy nodes as the strand has cells, in a straight line touching the strand. Only one strand per hyphae cluster may spread in a single confluence turn.

### New clusters

Clusters created by conversion may also spread during the same confluence, but only if their size's initiative order has not yet finished.

## End of confluence

After all spreads are complete:

- Any node remaining in the centre is removed.
- If one player has no nodes left, that player loses.
- If both players have no nodes left, the player who **started** the confluence loses.
- Otherwise, the player who did not start the confluence takes the next normal turn.

## End of the game

The game ends immediately when a confluence eliminates one player's colony. If the board fills or 500 plies pass without a confluence ending the game, the game is drawn.
