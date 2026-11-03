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
- **The board and the cards share the whole width.** The board's column is as wide as the board can be drawn at that
  height, and the cards have the rest, so the space goes to them and not to a gap around a board that cannot fill its
  column. With room for it, the cards stand in two columns rather than stretching across the window.
- **On the player's turn the actions are pinned, not scrolled to.** The card sticks to the top and bottom edges of the
  column, so it is in view whatever the scroll position. Scrolling the column to it would pull away a player reading
  the chat, and taking the keyboard would interrupt one typing in it. The card stays in place on the other player's
  turn, or the cards below would jump at every turn.
- **No status bar on this page.** The highlighted seat and the actions card already say whose turn it is. The words stay
  for a screen reader, and the rated, real-time and tournament badges move to the players card. When there is something
  else to say or a button to press (the result and a rematch, looking through history, analysis, or a game still waiting
  for an opponent), that goes in the actions card, under a heading that says which it is.
- **Rollout is a list of games in `game.js`**, not a field of the game definition. Which page a game gets is a
  temporary fact about the client, and the definition language should not have to keep a field that goes away once all
  games have moved.
- **The lobby keeps its page.** It has no board to keep in view.

## Not done yet

- **Tabs for a large play area.** A hex board has one thing to show. The games that may need tabs, such as Cthulhu
  Wars with its map and its cult panels, are where that gets decided.
- **The other games.** Their panels (hands, card tables, cult sheets) live in the board column today and each will need
  a place in the column. Games of three or more seats, and the phone layouts of the area maps and card games, may
  replace or be replaced by this page.

## Consequences

Two code paths draw a game page until every game has moved. The cards themselves are the same functions in both, so
only the arrangement is duplicated. A card in the column can be scrolled out of sight, a seat's clock among them.
