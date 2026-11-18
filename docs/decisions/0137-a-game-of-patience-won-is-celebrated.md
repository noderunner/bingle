# 0137. A game of patience won is celebrated, in the spirit of its cards

Status: accepted (2026-11-18), for issue #220. It builds on [ADR 0085](0085-card-sets-belong-to-the-table.md) (the card sets) and
[0122](0122-a-patience-vocabulary.md) (the patience vocabulary).

## Context

The maintainer asked for an animation when a game of patience is solved. It should first show the cards
sorting into their stacks, then put a simple picture of three to five seconds over the page: fireworks for
the game's own cards, and something of its own for each themed set.

## Decision

- **It is the page's alone.** The server sends nothing new: a game is won when `view.home` reaches
  `view.total`. Bowling Solitaire, which is scored by pins rather than solved, has no celebration.
- **It is shown only as it happens**: when the live position goes from not won to won in front of the player.
  A finished game opened later, a position looked back at in the history, and analysis show nothing. A
  spectator watching live sees it too.
- **The cards go home first.** The page draws the table anew for every change, so `js/solitaire.js` keeps
  where each face-up card was after each drawing of a live game. When the game is won, each card that was
  not yet on a foundation flies there from that place, the low ranks first (`.sol-flyer`). Spider's last
  finished suit flies to the count of suits finished. The foundations keep what they held until the
  cards land.
- **Then the picture** (`js/victory.js`, `#victory`) covers the page for 4 s and fades out over its last
  half second, with "Solved in N moves!" over it. A click or a key ends it early, and it never takes a
  click itself. It is drawn on a canvas, and a themed set's own suit pictures fly about as sprites:

  | cards | picture |
  |---|---|
  | the game's own | fireworks |
  | Ancient Civilizations | a golden sun rising in a burst of rays, the suits rising with it |
  | Amphibians | frogs hopping across a pond, ripples where they land, bubbles |
  | Fantasy Creatures | sparkles spiralling out of the middle, the creatures flying out with them |
  | Lovecraftian | tentacles rising from the deep, and an eye that opens and blinks |
  | Atomic Age | an atom spinning up, starbursts, rockets and saucers streaking past with smoke trails |

- **A player who asks for less motion** (`prefers-reduced-motion`) sees neither the flight nor the picture.
  The game-over message still says the game was won.

## Consequences

- A new card set should bring a scene of its own in `victory.js`. Until it does, it gets the fireworks.
- Nothing is added for other card games. A game that wants a show at its end would be a decision of its own.
