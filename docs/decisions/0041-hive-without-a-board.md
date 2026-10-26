# 0041. Hive without a board, in the Universal Hive Protocol

Status: accepted (2026-10-24); supersedes the board and notation parts of [0039](0039-hive-on-a-large-hex-field.md)

## Context

[0039](0039-hive-on-a-large-hex-field.md) played Hive on a drawn hexagonal field with moves written as cells (`ant@j10`, `j10-k10`). hivegame.com shows no board at all: just the tiles on the table, a supply for each player, and moves in the Universal Hive Protocol (UHP): `wA1`, `bQ wA1-`, `wB1 bQ`.

## Decision

- Board style `free` (hex boards): the renderer draws only the tiles on the table plus the cells a tile may go to, and fits the picture to them (never smaller than a nine-cell-wide table, so the first tile is not huge). There is no frame, grid or coordinates. Stacks show the lower tiles peeking out below the top one, with a height number. The field behind the scenes stays a hexhex board of size 10; moves off its edge are simply not offered.
- Tiles are hexagons ringed in one colour per kind, bearing the insect, white or black.
- Notation is UHP. A tile is `w`/`b`, its kind letter (`Q B S G A M L P`) and, for kinds that come in several, a number: the number is the tile's place in its supply and travels with it, so the position payload records the numbers of each stack. A move is the tile and a neighbour tile with a side symbol (`bQ wA1-` right, `wA1/` top right, `wA1\` bottom right, `-wA1` left, `\wA1` top left, `/wA1` bottom left), a climb names just the tile underneath, and the first tile of the game stands alone. Every legal move carries its UHP text (in `Move.Aim`) and its cells; `Parse` reads UHP, and also the older cell forms, and a move is matched to the legal one by its text or by its cells.
- Hive ships its own rules guide (overview, a page per tile, moving and notation).

## Consequences

Any ruleset game can ask for the `free` style. The generated guide no longer shows an empty "Notes" column, and rules pages state each game's own ending (`RulesetInfo.WinRules`): the generated "How the game ends" used to print Courtfall's victory-point text for every Go ruleset game.
