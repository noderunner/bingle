# FreeCell

Paul Alfille's 1978 game, popularised by Windows. No card is hidden, so there is almost no luck: nearly every deal can be won, and a lost game is usually a mistake you can look for.

## The aim

Move all 52 cards to the four **foundations**, one per suit, each built up from ace to king.

## The table

Eight **columns**, all cards face up: seven cards in the first four and six in the others. Four **free cells** stand empty, along with the foundations.

## A move

* **In a column** cards go down in alternating colours: a black 6 goes on a red 7.
* **An empty column** takes any card.
* **A free cell** holds one card, any card, until it is played to a column or a foundation.
* **A run in order** can be moved at once if there is room to move it card by card: with F empty cells and E empty columns, (F + 1) × 2^E cards, half that when going to an empty column.
* **A card on a foundation** stays there.

Click a card, then where it should go, or drag it. Click a selected card again to send it to its foundation. **Finish** appears once everything can go home by itself, and **← Back** takes a move back.

## Advice

Empty cells are your room to manoeuvre, so fill them with care. Look for the aces and twos first, and which cards bury them. An empty column is worth more than a free cell: it doubles what you can move.
