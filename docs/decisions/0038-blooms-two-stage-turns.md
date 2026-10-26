# 0038. Blooms: a turn of one or two placements

Status: accepted (2026-10-24)

## Context

Blooms (Nick Bentley, 2018) is a Go-like game on a hexhex board. Each player owns two colours of stone (Player 1 red and yellow, Player 2 blue and black). A turn places one stone, or two of different colours. When the turn ends the player captures every fenced enemy bloom (a connected group of one colour with no empty cell beside it); the first to capture the target number of stones wins. A player's own fenced blooms are not captured on their own turn, only at the end of the opponent's turn.

## Decision

- A Go `state.Ruleset` (`internal/games/blooms`). A turn is several decisions by the same seat: a first stone (colour and cell), then either a second stone of the other colour or `done`; captures are resolved when the turn ends. This keeps each decision to about 120 moves instead of 7000 for "every pair of stones". Player 1's first turn is a single stone.
- Stones are four piece types named after their colour (`red@d4`), so the stone colour buttons need no hand. Piece definitions gained an optional `color` so a button can be drawn in its own colour, and the definition's `neutral_colors` of ADR 0036 became `extra_colors` (a seat's second colour is not neutral).
- Boards: base 5 (61 cells, 20 captures) is the default; `blooms-4` (37 cells, 15) and `blooms-6` (91 cells, 25) are variants with the capture target as a parameter.
- The view tints fenced blooms, which are about to be captured, and the cells stoned this turn and last.
- The bots play through `BotGuide`: the placements that leave them best off by the captures the turn would bring, less the stones left fenced.

## Interpretations

The published rules do not define passing; there is none (a seat always has an empty cell to play). The game is decided on captures after 500 plies, a draw if equal. The newer "breakthrough" reformulation (one stone, then optionally one of the other colour) is the same decision structure.
