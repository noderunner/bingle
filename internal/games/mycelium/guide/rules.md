# Mycelium

Mycelium is a game of two competing fungal colonies for two players, designed by Miles Stevenson
in 2024. It is still being playtested, and Bingle is one of the places it is tested.

## Components

- A hex-hex board with five cells to a side (61 cells).
- Black nodes for the first player, green nodes for the second.

## Normal turns

The board starts empty and Black places first. On your turn, place one node of your colour on any
empty cell.

A node placed on the **centre** (e5) starts a **confluence**. Normal turns stop until it is over.

## Clusters and strands

A **cluster** is a connected group of nodes of one colour.

- A **spore cluster** touches no enemy node.
- A **hyphae cluster** touches at least one enemy node.
- A **strand** is a path through a cluster's nodes, each touching the next. It may bend as it
  likes, but it visits no node twice. Its length is the number of nodes on it.

## The confluence

A confluence has two phases:

1. **The spore phase**: the spore clusters spread.
2. **The hyphae phase**: the hyphae clusters spread.

Each phase goes in **initiative order**: first the clusters of one node, then those of two, then
three, and so on, up to the largest cluster on the board. At each size, the player who did **not**
take the centre chooses first. The players then take turns, each spreading one of their clusters of
that size, until neither has one left to spread. If one player runs out first, the other spreads
the rest of theirs.

A cluster with a legal spread must spread. Whether a cluster is a spore or a hyphae cluster, and its
size, are looked at when its turn comes, so conversions earlier in the confluence can change them.

### Spore spread

Choose a node of the spore cluster and a straight line from it across empty cells. The first enemy
node along that line becomes your colour. The node that spreads stays where it is.

### Hyphae spread

Choose a node of the hyphae cluster as the end of a strand, and convert an enemy node that touches
it. You may then convert an enemy node touching the one you just converted, and so on, up to as
many nodes as the longest strand that ends at the node you chose. The converted nodes need not
follow a straight line. You may stop early.

On Bingle each conversion of a hyphae spread is a move of its own: click the end of the strand and
the first enemy node, then each further node, and **Done** to stop early.

### Which clusters may spread

Each cluster spreads at most once in each phase. A cluster that grows or merges, or is cut in two by
the other player's spreads, still counts as having spread if it contains a node of a cluster that
has spread. A cluster made only of new nodes, such as a node just converted from the enemy, may
spread in the same phase, as long as the initiative has not already passed its size.

During a confluence, the board tints the nodes that have spread in the current phase.

## The end of a confluence

When neither phase has anything left to spread:

- The node on the centre is removed, whichever colour it is.
- A player with no nodes left loses.
- If neither player has any nodes left, the player who took the centre loses.
- Otherwise, the player who did not take the centre places the next node.

## The end of the game

The game ends when a confluence wipes out a colony. A game that has not ended after 500 plies is a
draw.
