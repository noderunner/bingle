# 0078. A hex board is drawn as hexagons or as a grid of points, and the player chooses

Status: accepted (2026-11-01)

## Context

Some hex games are printed as a lattice of points rather than a field of hexagons: Meridians' own
board is a hexagon of triangles with the stones on the intersections. The definition of Meridians
already said `style: points`, but the hex renderer ignored the style and drew hexagons, whatever the
rules text said. The maintainer asked for that look in Meridians, and for any hex game to offer it.

## Decision

- **Two styles, one board.** `renderHexBoard` draws either hexagons or a *hex grid*: dots at the
  points, joined by the lines between neighbours. It is the same picture with the same click areas,
  position names, coordinates and orientation, so nothing about a game, its notation or its tutorials
  depends on the style.
- **The player chooses, and the definition only sets the default.** A hex board with
  `board.style: points` starts as the grid (Meridians); any other starts as hexagons. No new field
  was needed, and `points` now means on a hex board what it already means on a grid.
- **The choice is kept in the browser, per game family** (`bingle.hexStyle.<family>`), like the other
  display settings. It is not global, because a game's own default must survive a choice made in
  another game; the variants of a game share it.
- **Hive is left out.** Its `free` style has no board to draw: the tiles are the board.

## Consequences

- Guest and account players alike have the setting, and it does not follow them to another browser.
- The home page's picture of a game shows the game's default, not the player's choice.
- The board is not turned to match a print that stands on a corner. Turning it would make "rows
  from the top" in the rules and the row labels wrong in one style only.
