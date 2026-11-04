# 0103. One history card, written out in words or as notation

Status: accepted (2026-11-04). Replaces the two tabs of the history card.

## Context

A game that narrates its turns had a *Moves* tab and a *History* tab, and on a map those sat inside a *History* tab of the rail. The
buttons that step through the game were only under *Moves*, so reading the story and stepping through it were at odds.

## Decision

**One card, one set of buttons, and a switch between *Friendly* and *Notation*.** Both write out the same moves. The choice is
remembered in the browser, Friendly is the default, and a game with no narration has only the notation and no switch.

**Friendly shows the whole story, with the move in view highlighted.** It used to stop at the position in view, so a step back
hid the lines after it. A line goes to its move when clicked. *Since your last move* is in both, and goes to that move.

**A move that told nothing shows its notation in the story only while it is in view.** Moves and lines are not one to one: a Done
says nothing. Listing every silent move would clutter the story, and leaving them out would make a step look as if it did nothing.

**The impulse card tells the move in words in either format,** so the formal list can be open beside what each move did.
