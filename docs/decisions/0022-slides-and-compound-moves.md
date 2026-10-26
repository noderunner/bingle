# 0022 — Slides and compound moves

## Context

The v2 actions (place, move-stack, jump, explicit step) cover Tak, Go,
Reversi, Draughts and Breakthrough, but not long-range sliders, and nothing
had a move made of two dependent choices. The Game of the Amazons needs both:
slide a queen any distance, then shoot an arrow from where it landed.

## Decision

* A new `slide` action: pieces, directions (default: all the board's
  adjacency offers, so `orthogonal8` gives queen lines), an occupancy rule and
  an optional `max_distance`. Pieces never pass over other pieces.
* A slide may carry `then: {type: "shoot", piece}`: a compulsory follow-up
  that puts a piece from the unlimited supply on any empty square reachable by
  a slide from the destination, judged in the position *after* the slide (the
  square just left is free).
* A move gains an `aim` field; notation is `from-to/aim`; the click-by-click
  `steps` are `pos(from), pos(to), pos(aim)`, so the generic UI needs no new
  code. Enumeration builds moves correct by construction, and a test
  brute-forces every from/to/aim triple against validation.
* Enumeration can stop early (`HasLegalMove`), which matters for games whose
  end rule is "the player to move has no legal move".

* A second, simpler addition in the same spirit: `leap` (clone within
  `clone_distance`, jump up to `max_distance`, optionally infecting adjacent
  opposing pieces) for Ataxx-style games. Distance is graph distance, so the
  same action works on square and hex boards.

## Consequences

* Amazons is a 40-line JSON document; the opening move count matches the
  published 2,176.
* The action space of the 10×10 game is 86,980 fixed IDs; clients that want a
  factored policy can split an ID into from / to / aim.
* Other compound moves (move-then-build as in Santorini, push moves) would add
  further `then` types rather than a new action each.
