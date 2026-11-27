# 0142. What a selection captures is worked out by applying it to a copy

Status: accepted (2026-11-22), for issue #269.

## Context

A client wants to show which pieces a selection would take off the board while the move waits for its confirming
click (bingle-3d makes them see-through). A selection names only where a piece goes: a draughts chain gives its
landings, a Go stone or a Pente placement its point, a Fist jump its landings and a word per leg. The pieces it takes
follow from the rules, which a client must not re-derive (ADR 0124). The engine already works them out when a move
is played (`MoveResult.Captures`), but a played move sent only `captured`, the first of them.

## Decision

- **Each legal selection carries `captures`**, the positions whose pieces it takes off the board, in the
  selections query, simulate's `legal_moves` and MCP's `list_selections` and `simulate` (API 1.10.0). Absent for none.
- **It is worked out by applying the selection to a copy** (`state.SelectionCaptures`), so it says exactly what
  playing it would: end-of-turn captures and Go rulesets included, with nothing to keep in step. It costs one apply
  per option, on the listings a person's client asks for, not on bots' searches.
- **It is left out where applying would tell the seat too much**: in a game with secrets (a ruleset that only
  attaches a view says it hides nothing, `state.Open`), and for a selection that rolls, draws or turns up a card
  (`CanTakeBack`). That keeps ADR 0114's rule that a query never shows a roll early.
- **A conversion is not a capture**: Fist's ace and Reversi's flips leave a piece where it was, in another color,
  so they are not in `captures`. A field for them can follow.
- **A played move carries `captures` too**, beside `captured` (still the first, for the clients that read it),
  kept in `moves.captures`; a row from before has its `captured` alone. A move of several selections lists all of
  theirs.
