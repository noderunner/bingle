# Moving and notation

## The One Hive rule

The hive must always be a single connected group. A tile may not move if lifting it off would split the hive in two — even if the move would join it up again. A tile that is the only link between two groups is **pinned**.

## Freedom to move

A tile that **slides** along the ground must:

- keep touching the hive as it goes (it never drifts off into empty space), and
- be able to **fit through**: it cannot slide between two tiles that are both next to its path — a gap with a tile on each side is too narrow.

A Beetle (or a Mosquito acting as one) that climbs or descends faces the same squeeze rule applied by **height**: it cannot step between two stacks that are both taller than the level at which it travels.

The Grasshopper jumps and ignores this rule; the Ladybug flies over the hive and applies the height version.

## Stacks

A Beetle (or Mosquito) on top of others forms a **stack**. Only the top tile may move. The stack counts as the colour of its top tile when you place new tiles. A **number** on the tile shows how many tiles are stacked.

## Draws

- If one move surrounds both Queens, the game is drawn.
- Two passes in a row, or 300 plies, also draw the game.

## Notation

Moves are written in the **Universal Hive Protocol** notation, the same as on hivegame.com.

A tile is its **colour** (`w` white, `b` black), its **kind** (`Q` Queen Bee, `B` Beetle, `S` Spider, `G` Grasshopper, `A` Soldier Ant, `M` Mosquito, `L` Ladybug, `P` Pillbug) and, for kinds you have several of, a **number**: `wQ`, `bA2`. The number is the tile's number in your supply and stays with it as it moves.

A move is the tile you play, a space, and a tile **already on the table** that it ends beside — with a symbol for the side:

| Written | Means |
|---|---|
| `wA1` | the first move of the game (no neighbour yet) |
| `bQ wA1-` | `bQ` to the **right** of `wA1` |
| `bQ wA1/` | `bQ` to the **top right** of `wA1` |
| `bQ wA1\` | `bQ` to the **bottom right** of `wA1` |
| `bQ -wA1` | `bQ` to the **left** of `wA1` |
| `bQ \wA1` | `bQ` to the **top left** of `wA1` |
| `bQ /wA1` | `bQ` to the **bottom left** of `wA1` |
| `wB1 bQ` | `wB1` climbs **on top of** `bQ` (no symbol) |
| `pass` | you have no move |

A symbol **after** the name means the right side of the tile; **before** it, the left side. Both placing a new tile and moving a tile on the table are written the same way.
