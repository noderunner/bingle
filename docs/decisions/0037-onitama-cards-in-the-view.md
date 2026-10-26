# 0037. Onitama: the cards travel in the view, Red always starts

Status: accepted (2026-10-24)

## Context

Onitama (Shimpei Sato, Arcane Wonders) is a perfect-information 5×5 game whose moves come from five Move cards (two per player and one at the side). Using a card passes it to the side and brings the waiting card into your hand.

## Decision

- A Go `state.Ruleset` (`internal/games/onitama`) with a JSON definition for board, seats and pieces. A move is the card, the piece and the destination (`tiger:c1-c3`), or a pass that still spends a card (`tiger:pass`) when nothing can move. The hands and the side card are in the position's own data; the sixteen cards are a table in Go.
- The UI steps are: the card (an action button, `card-tiger`), then the piece and the square on the board. A new `cardsPanel` in `game.js` draws both hands and the side card as 5×5 grids from `view.cards` (Blue's cards are drawn upside down as on the board) and makes the cards of the player to move clickable.
- The deal is random (the ruleset is seeded, so the start is recorded and replays reuse it).
- Red (seat 1) always starts. The printed game lets the stamp on the side card decide, but the lobby, the environments and the generic tests take seat 1 as the first player; the stamps stay on the card data.
- Bots play through `BotGuide`: take a winning move if there is one, otherwise avoid moves after which the opponent wins at once.

## Consequences

- Square boards draw ruleset tints (the Temple Arches, the last move), as of ADR 0036.
- A game whose first player depends on the deal would need the session layer to ask the ruleset who starts; that is not done here.
