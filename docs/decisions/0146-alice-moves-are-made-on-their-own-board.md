# 0146. An Alice move is clicked and written on its own board

Status: accepted (2026-11-23). It supersedes the point of [ADR 0145](0145-chess-variants.md) that an Alice move is
clicked and written by the square where the piece ends. Its line between the boards is superseded by [ADR 0148](0148-a-game-on-several-boards.md).

## Context

ADR 0145 had an Alice move name the square where the piece ends, on the other board: `Ng1-n3` for the knight's move to
f3. A capture was then clicked on the empty twin of the piece taken, not on the piece itself, and a player who
clicked the knight they meant to take found no such move.

## Decision

- **A move names the square it moves to on its own board**, as in chess, in its gestures and its notation:
  `Ng1-f3`, and `Bj5xi6` for a bishop taking a knight on board B. The piece then crosses to the twin square, which
  must be empty, as before.
- **The older form is still read**: a selection naming the twin (`Ng1-n3`) is the same move, so games recorded
  before replay as they were.
- **The two boards are drawn apart**: the look's board takes `divide`, a thick line after that many columns, and
  Alice's is 8.

## Consequences

- Where the piece ends is not in the move's text; the board shows it. Every other chess game is unchanged, since
  there the two squares are the same.
