# 0040. Fendo: fences travel in the view

Status: accepted (2026-10-24)

## Context

Fendo (Dieter Stein, 2014): pieces and fences on a 7×7 board. A turn either moves a piece (straight, with at most one right-angle turn, never over a piece or fence) and builds a fence on a free side of the square it ends on, or adds a piece one move away from a friendly piece. A fence may not wall off an empty area or a second area with several pieces. A region closed round exactly one piece belongs to that piece's owner; when the last open region is closed the larger territory wins. Fences are walls on the edges between squares, which the board model (cells and pieces) has no place for.

## Decision

- A Go `state.Ruleset` (`internal/games/fendo`). The fences are a list of touching cell pairs in the position's payload; the board holds only pieces. A decision is one whole move: the piece, where it goes (left out when it stays) and the side of the fence (`d1-d4+n`, `d4+e`), an added piece (`add@c3`) or a pass. In the click flow the fence sides are action buttons after the piece and square.
- The view carries the walls and tints closed areas by owner (`area1`, `area2`). The square board renderer draws `view.walls` as thick lines on the shared edges, which any ruleset can use.
- Legal fences are tested by walling the edge and recomputing the regions: no region may hold no piece, and at most one may hold several.
- Variants `fendo-9` and `fendo-11` use the published 9×9 and 11×11 boards, where the first player to own a majority (41 of 81, 61 of 121) wins at once.
- Bots: the heuristic and `BotGuide` score a position by the squares each side owns plus the open squares its pieces reach first.

## Interpretations

A piece may stay put and only build a fence. The published text says a selection is illegal when no legal fence can be built at the destination (and mentions the origin in an example); only the destination is checked. Ties are a draw. Two passes in a row, or 400 plies, end the game and count the closed areas.
