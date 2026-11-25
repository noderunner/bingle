# 0148. A game on several boards says so, and each board is drawn as its own

Status: accepted (2026-11-25). It supersedes the points of [ADR 0145](0145-chess-variants.md) that Alice is one
16×8 grid drawn as one board without a gap, and of [ADR 0146](0146-alice-moves-are-made-on-their-own-board.md) that
a look divides it with a line.

## Context

Alice chess is played on two boards. ADR 0145 made them one 16×8 grid so that no renderer needed anything new, and
ADR 0146 had the web page's look draw a line after the eighth column. Nothing but that line said there were two
boards: the API sent one board 16 wide, a client other than the web page (bingle-3d) drew one board 16 wide, h1 and
i1 were neighbours in the graph, and the line was the web page's alone (`divide` in `web.yaml`), so every client had
to be told the trick on its own.

## Decision

- **A grid may be several boards side by side**: the generator's `boards` (Alice's is 2) makes that many grids of
  its `width` × `height`, as one graph `boards` × `width` wide. Their columns, and algebraic files, run on from one
  board to the next, so Alice's squares keep their ids (board B is still i to p), its notation is unchanged and
  recorded games replay. No square is a neighbour of a square on another board.
- **The rules still see one board.** Only the layout knows there are several: the graph lists each board's
  positions (`Graph.Boards`), and the rules of Alice keep their own geometry.
- **The API says it**: a game definition's `board.boards` (`GET /game-defs/{id}`) lists each board's `positions`, left to right, and is absent
  for one board (API 1.14, an addition). The positions keep their coordinates, so a client that does not read it
  still draws one wide board as before.
- **Each client draws each board as a board of its own**, with a space between them, as it sees fit (ADR 0124). The
  web page draws a frame for each; the text of a position (`board_text`) puts a bar between them. The look's
  `divide` is removed.
- Only a grid may be several boards. A hex board or an explicit graph that needs it is a later decision.

## Consequences

- A new game on two boards of squares is a definition and nothing else; every client draws it apart.
- Bughouse is still two games (ADR 0145), not one game on two boards: its boards are not one position.
