# 0144. Chess: long algebraic notation, and draws that end the game by themselves

Status: accepted (2026-11-23).

## Context

Chess joins the games as a Go `state.Ruleset` (ADR 0025): check, castling through unattacked squares and en
passant are not things the DSL says, and the FIDE rules leave two questions that the platform must answer.

- **Notation.** Players write short algebraic notation (*SAN*: `e4`, `Nf3`, `exd5+`), but SAN depends on the
  position: whether `Nd2` needs a file letter, whether a move is a capture, and the check mark. A ruleset's `Format`
  and `Parse` see only the selection, and the platform holds every game to that: the record keeps the notation, and
  replays, repro bundles and the paranoid check read it back with `Parse` and require `Format` to give the same text.
- **Draws by repetition and the fifty-move rule.** Over the board these are *claimed* (FIDE 9.2, 9.3), and only
  the fivefold repetition and seventy-five-move rule end a game by themselves. The platform has draw offers but
  nothing a player claims.

## Decision

- **A move is recorded in long algebraic notation**, which needs no position: the piece's letter (none for a
  pawn), the square left, `-` or `x`, the square reached, and `=` with the promoted piece: `e2-e4`, `Ng1-f3`,
  `e5xd6` (en passant), `e7-e8=Q`. Castling is `O-O` and `O-O-O`. The selection carries what the text says: `piece`
  is the piece moving, `aim` the square of the piece taken (beside `to` for en passant), and the action is `move`,
  `castle` or the piece a pawn becomes (`queen`, `rook`, `bishop`, `knight`). There are no check marks.
- **Parse also reads SAN and UCI** (`Nf3`, `Nbd7`, `exd5`, `e8=Q+`, `e2e4`, `e7e8q`), giving a selection with only
  what the text names; `Apply` finds the one legal move it fits, says when the text fits two, and records the long
  form. So a person or a model may type the notation they know, and the record stays readable without a board.
- **Promotion is a third gesture**: the pawn, its square on the last rank, then one of four actions naming the
  piece. Castling is the king's two squares.
- **The draws end the game at once**: the third time a position comes up (the same pieces, side to move, castling
  rights and a possible en passant capture), the hundredth ply with no capture or pawn move, stalemate, and too
  little material to mate (kings alone, a single minor piece, or bishops all on squares of one color). This is
  what most online play does, and it means bots and clocks never run on in a dead game. Agreeing a draw is the
  platform's draw offer.
- **The position keeps what FEN keeps** in its `ext`: the side to move, castling rights, the en passant square,
  the half-move and full-move counts, and hashes of the positions since the last capture, pawn move or lost right,
  for repetition.

## Consequences

- The move list reads `Ng1-f3` rather than `Nf3`, and shows check on the board (a tinted king) rather than with
  `+`. A client that wants SAN in its own move list can work it out from the positions.
- A custom start (`start_state`) is a position document with the FEN fields in `ext`; there is no FEN import yet.
- Achievements and tutorials for Chess are separate issues.
