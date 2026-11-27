# 0153. What a selection turns over is found by comparing the position before and after

Status: accepted (2026-11-27), for issue #293. Extends ADR 0142. Amended the same day: `converts_to` (API 1.20.0).

## Context

ADR 0142 sends with each legal selection the positions whose pieces it captures, and left conversions out: an
Othello placement flips every disc it outflanks and Fist's ace converts what it jumps, but those pieces stay where
they are, in another color. A client previewing a move (bingle-3d) cannot show them without re-deriving the rules,
which it must not do (ADR 0124).

## Decision

- **Each legal selection carries `converts`**, the positions whose pieces it turns over, wherever it carries
  `captures` (the selections query, simulate's `legal_moves`, MCP's `list_selections` and `simulate`; API 1.19.0),
  and left out where they are: in a game with secrets and for a selection that rolls, draws or turns up a card.
  Absent for none.
- **It is found from the same apply to a copy** (`state.SelectionEffects` gives both), by comparing each position
  occupied before with the same position after: a conversion is a stack of the same height with the same kind of
  piece at each level and not the same colors. No ruleset reports it, so every ruleset has it, the private
  overlay's and a WebAssembly pack's included.
- **What is not a conversion**: a position captured (a pawn taking a pawn leaves a pawn there in the other color),
  a piece replaced by another kind (a promotion), a position empty before (a placement, named or not) and a stack
  that grows or shrinks (Tak).
- **The name is `converts`**, Fist's word and ADR 0142's, beside `captures`.
- **Each also carries `converts_to`**, the color word the top piece at each of those positions turns to, by
  position (API 1.20.0). The client draws the piece as it will be. It must not guess the color: a move may
  turn over the mover's own pieces as well as another seat's (a Yinsh ring's), and in a game of more than two
  colors, "the other one" means nothing.
- **A played move does not carry it.** The preview is what asked for it; a client replaying a move has the
  positions before and after.
