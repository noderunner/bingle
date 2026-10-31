# 0066. Mycelium: a confluence one spread at a time

Status: accepted (2026-10-31)

## Context

Mycelium is Miles Stevenson's own design (2024), on Bingle partly so that it can be playtested. Two
colonies place nodes on a 61-cell hex-hex board; whoever takes the centre starts a **confluence**,
in which every cluster spreads in turn and converts enemy nodes, and a colony left with nothing
loses. Its rules exist only as the designer's description, so this record keeps the readings the
implementation settled, for the designer to overturn.

A first implementation had the spore spread *move* the spreading node onto its target, allowed only
straight strands spreading along their own axis, and let a cluster spread again whenever its shape
changed. The designer's rulings: a spore spread converts the enemy node it sees and the spore stays
put; a strand may bend, and a hyphae spread may wind in any direction from either end; a full board
is no rule of its own.

## Decision

- **A Go `state.Ruleset`.** Sight lines, clusters and strands are beyond the JSON DSL.
- **A confluence is a run of decisions, one spread each**, and the seat to move is whoever chooses
  the next spread. After each, the ruleset hands the confluence to the next seat with something to
  spread (the other player first, then the same one), and moves the initiative on — then from the
  spore phase to the hyphae phase, and then to the end — while nobody has. A stored position is
  always at a decision, and `Validate` refuses one that is not.
- **A hyphae spread is one decision per conversion**, as Blooms' turn is one per stone (ADR 0038):
  the end of the strand and the first enemy node, then each further node, then `done` or the end
  of the strand's allowance. With winding spreads, listing every whole spread as one move would run
  to millions in a crowded position; one conversion at a time is at most six choices.
- **The length of a spread** is fixed when it starts: the longest strand of the cluster ending at the
  chosen node, and never more than the enemy cluster it enters (every node it converts belongs to
  that cluster). The longest path is a hard problem; the search is exact, ordered by the most
  constrained node and pruned by a block-cut bound, and on 80,000 random clusters it took 10ms at
  worst. A budget of 20,000 partial strands keeps a position nobody has met from stalling a game;
  past it, the longest strand found counts. Nothing in testing came near it.
- **Who has spread** is a list of cells, `Spent`, cleared when the phase changes. A cluster holding a
  spent cell has spread, whether it has since grown, merged or been cut; a cell that changes colour
  leaves the list, so a cluster made only of converted nodes is new and may spread if the initiative
  has not passed its size.
- **The bots** take the centre only when a greedy play-out of the confluence leaves them better off
  than a node placed elsewhere, convert as far as a spread allows, start from the end of the longest
  strand, and treat spreads that convert the same node as one.

## Interpretations

- A cluster with a spread available must make one; the player chooses which of their clusters goes
  next, and a hyphae spread may stop after its first conversion.
- A strand ends at the node chosen, which may be any node of the cluster; a single node is a strand
  of one.
- Kind (spore or hyphae) and size are read when a cluster's initiative comes, so conversions earlier
  in the confluence can change them.
- A game with no confluence to decide it is drawn after 500 plies.
