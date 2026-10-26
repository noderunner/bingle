# 0039. Hive: tiles on a large hex field

Status: accepted (2026-10-24); the drawn field and the cell notation were replaced by [0041](0041-hive-without-a-board.md)

## Context

Hive (John Yianni) has no board: tiles are placed against each other and the hive moves as it grows. Each player has a Queen Bee, Beetles, Spiders, Grasshoppers and Soldier Ants, plus (expansions) a Mosquito, a Ladybug and a Pillbug. A player wins by surrounding the enemy Queen. Bingle's engine needs a fixed set of cells.

## Decision

- A Go `state.Ruleset` (`internal/games/hive`) played on a hexhex field of size 10 (271 cells). White's first tile goes in the middle cell; the field is far wider than any hive grows, and a move off its edge is simply not offered. A decision is one move: a placement (tile, then cell), a move (cell, then cell) or a pass (only when nothing else is legal).
- Stacks use the engine's own stacks: a Beetle (or a Mosquito on top of the hive) sits on top and its colour owns the stack. The tiles in each supply are the engine's reserves, so the generic hand panel shows the counts; the tile pictures are a new `bug-<kind>` piece shape (a hexagon with an insect).
- The geometry is checked in one place: slides need contact with the hive and a gap that fits (exactly one of the two cells beside the step is occupied); climbing and descending use the "freedom to move" height rule (blocked when both cells beside the step stand higher than the tile). The One Hive rule is a connectivity test of the table with the lifted tile removed.
- The Mosquito copies the kinds of the top tiles it touches (not another Mosquito) and is a Beetle on top of the hive. The Pillbug, or a Mosquito touching one, may instead throw a lone adjacent tile over itself to an empty cell beside it, under the Beetle's height rule for the lift and the drop. A throw and a walk with the same start and end are one move, so a move is only its two cells.
- `hive-base` is a variant with the three expansion supplies at zero.
- Bots: the heuristic is how far each Queen is surrounded plus a little for free tiles; `BotGuide` takes a winning move, otherwise drops moves that surround the bot's own Queen.

## Interpretations

The Pillbug restriction on the tile that was "just moved" applies to a tile moved or thrown by the previous move, not one that was merely placed. A tile in a stack of more than one cannot be thrown, and a covered Pillbug cannot throw. A player with no legal move passes; two passes in a row, or 300 plies, draw the game. Surrounding both Queens with one move is a draw; surrounding only your own loses.
