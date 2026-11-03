# 0092. A game page that fits the screen

Status: accepted (2026-11-02).

## Context

The game page scrolled as one long column. The seat strips, the move bar and any tutorial were stacked around the
board, so a player could scroll the board off the screen, and had to scroll back and forth between the board and the
controls of a move. The maintainer asked for a page where the play area stays put and everything else sits beside it,
tried on one game before the others move.

## Decision

- **The page is as tall as the window and does not scroll.** The board takes the space left under the top bar, and
  shrinks to fit. Everything else is a card in a column beside it, which scrolls by itself. On an
  upright screen under 980px wide the column goes under the board. The footer is not shown: the version is also in the
  help window.
- **Two columns at most, with spare width outside them.** The board's column is as wide as the board can be drawn at
  that height, and the cards have what is left in a single column, up to 40rem. Width beyond that goes to the outer
  margins, so the board and the cards are always next to each other. A third column was tried on wide screens, and the
  page was harder to read for it.
- **On the player's turn the actions are pinned, not scrolled to.** The card sticks to the top and bottom edges of the
  column, so it is in view whatever the scroll position. Scrolling the column to it would pull away a player reading
  the chat, and taking the keyboard would interrupt one typing in it. The card stays in place on the other player's
  turn, or the cards below would jump at every turn.
- **No status bar on this page.** The highlighted seat and the actions card already say whose turn it is. The words stay
  for a screen reader, and the rated, real-time and tournament badges move to the players card. When there is something
  else to say or a button to press (the result and a rematch, looking through history, analysis, or a game still waiting
  for an opponent), that goes in the actions card, under a heading that says which it is.
- **Every game has this page.** Blooms came first, to be played and refined, and the rest followed. The old page
  remains only for the lobby, which has no board to keep in view.
- **What the player chooses from is an action.** A hand of pieces and Onitama's move cards go in the actions card and are
  pinned with it. Everything else about a seat stays in the players card.
- **The shape of the play area decides the columns.** A board takes the width its height allows, a board that grows
  keeps one shape so the columns stay put between turns, and a table of cards, which has no shape, takes all the width
  but a narrow column of cards.
- **Tabs are in the column of cards, not the play area.** On a map, the play area is one thing, and what crowds the page
  is the cards beside it: the cults, the talk and the history take turns, under the pinned actions. What belongs to the
  map (a battle, a unit's card) and the history go under it, in the height a wide map leaves.
- **The lobby keeps its page.** It has no board to keep in view.

## Consequences

A card in the column can be scrolled out of sight, a seat's clock among them, and on a map it can be behind a tab. Phones
keep their own pages for the area maps and the card games. On a phone a map is shown only with the phone on its side, and
the cards only with it upright: each gets the whole screen, and turning the phone is the way between them. A player with
rotation locked cannot reach the map until they unlock it.
