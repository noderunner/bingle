# 0021 — Custom start positions and take-backs with consent

## Context

Puzzles, endgame practice, "play from this analysis position" and curriculum
learning for agents all need a game that begins somewhere other than the
definition's setup. Undo (ADR 0018) deliberately stops once another human has
moved; a casual game still wants a polite way to say "may I take that back?".

## Decision

* A game may record its own start: `games.initial_state` (a position document)
  and `games.initial_turn_seat`. Empty means the definition's setup. Creation
  takes `start_tps` (PTN games) or `start_state` + `start_seat`; both go
  through `state.Validate` (squares, pieces, colours, reserves must exist) and
  are rejected if the position is already decided.
* Everything that used "the initial position" now asks the game: replay at
  `seq 0`, undo to the start, the first mover when the seats fill, export (the
  PTN gets a `[TPS]` header and carries the move number over).
* Take-backs: `takeback_seat` / `takeback_seq` on the game row. A player whose
  move has been answered by a human may *request* to rewind to before that
  move; the opponent *accepts* (the game is rewound with the requester to move)
  or *declines*. Any move, undo or ending clears the request. Against a bot
  plain undo already works, so requests are refused there.

## Consequences

* Rooms from puzzles need no special casing anywhere downstream; they are
  ordinary games whose row happens to carry a start.
* The consent flow mirrors draw offers, so clients, WebSocket events and MCP
  tools follow one pattern.
* A start position is validated but not proven reachable; that is by design
  (puzzles and handicaps are rarely reachable positions).
