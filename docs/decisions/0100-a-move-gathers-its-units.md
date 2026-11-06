# 0100. Cthulhu Wars: a Move gathers its units, and always waits for Done

Status: accepted (2026-11-04). Sending one move per unit goes with the first phase of
[ADR 0114](0114-impulses-selections-moves-and-gestures.md): the units of a queue will be sent as one move.

## Context

A Move action sends any number of units, each paying 1 Power. On the page that was one round of clicks per unit: the unit,
then its destination. Most moves send several units from one area to one place, and the card only said *Choose an area*
with a line of notation, so it was hard to see what the move would come to, or what it would cost.

## Decision

**The page gathers units in the impulse card, from one area, and clicking a destination sends them.** Clicking a unit adds it
to a queue, shown as icons with a count and the running Power, and takes it off the map as that player sees it (a unit on a
Gate leaves the Gate empty). The position on the server doesn't change until the units are sent. Clicking an icon takes one out, and *Back* empties the queue.
Units that can't be paid for, or that share no destination with the rest, can't be added.

**The queue belongs to the page.** The rules still have one step per unit (`go`, `leave`), so bots, the API, the notation and
stored games don't change. A destination sends the steps one by one, and the server judges each as it does any other. The
view adds what the page can't work out alone: how many units of a type are free to move (`move_stacks`), and the steps that
don't cost 1 Power (`move_costs`). If a step is refused, the ones before it stand and the player is told.

**A unit's own area can be its destination.** Stepping off a Gate, or putting a unit onto the Gate where it stands, is free and
goes on being chosen the same way: the unit, then the Gate. Only a queue of one unit can do it.

**One destination per queue, and more can follow.** Units in another area, or bound for another destination, are a new queue
in the same Move, which the rules have always allowed.

**A Move always waits for Done.** It used to end by itself when nothing else could move, such as at 0 Power, which would skip
the card that confirms it and take away *Back*. So there is now one more decision in a game, and bots make it as they do any
other. Done is a button even when it is the only thing left.

**The card that confirms says what moved**, from the position, so the whole table sees the same summary.

**Back there takes back the whole destination,** which the page knows only while it stays open: after a reload it takes back
one step at a time.

## Consequences

- A rated or tournament game can't take a move back (ADR 0083), so there the destination click is final.
- The running cost before a destination is the least the units can cost. A dearer destination (the Ice Age) is only offered if
  it can be paid for, and a free ride (a Shantak, Arctic Wind) shows in the summary.
- A step can still be refused in one case the page doesn't count: several Cultists on one Shantak's free seat, with too little
  Power for the rest.
- A tutorial's step is a single move, so it takes one unit at a time.
