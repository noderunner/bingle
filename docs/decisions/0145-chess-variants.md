# 0145. Chess variants: one ruleset, Alice on one wide board, and no Bughouse yet

Status: accepted (2026-11-23). It builds on [ADR 0144](0144-chess-notation-and-draws.md) (chess's notation and
draws). Its drawing of Alice's boards as one is superseded by [ADR 0148](0148-a-game-on-several-boards.md).

## Context

Five variants join Chess: Chess960, Crazyhouse, Antichess, Atomic chess and Alice chess. Bughouse was asked for too.
Each changes a few rules of chess and keeps the rest, so the question is how they share the code, and what each
needs from the platform that chess did not.

## Decision

- **One ruleset, five variants of the definition.** Each is a merge patch of `chess.yaml` (`chess-960`,
  `chess-crazyhouse`, `chess-antichess`, `chess-atomic`, `chess-alice`) that sets the param `variant`, with its own
  rules text and a guide tab. The Go ruleset reads the param. Crazyhouse, Antichess and Atomic follow lichess's rules.
- **Chess960 draws its table from the start's seed** (the standard numbering: 518 is chess). The family's ruleset is
  therefore a `SeededRuleset` and every chess game records its start, which costs one document per game. A table
  is castled by clicking the king and then the rook, since the king may not move at all. Castling rights are written
  in X-FEN: `K`/`Q` for a rook in the corner, the rook's file otherwise.
- **A Crazyhouse hand is the position's reserves**, so the page's hand panels show it and a drop is the usual piece
  then square. A drop is written `N@f3`. Which pieces were pawns is kept in the position, since one taken returns to
  the hand as a pawn. There is no fifty-move rule, and repetition counts the hands; nothing but a lost castling right
  is irreversible, so the positions for repetition are kept from there.
- **Antichess** has no check and no castling; a capture, if any can be made, must be; a pawn may promote to a king;
  losing every piece or having no move wins.
- **Atomic**: a capture removes the piece taken, the piece taking it and every piece but a pawn around the square it
  reaches. A king may not capture or blow up its own king, blowing up the other king wins at once, and kings side by
  side cannot check each other. A played move's `captures` lists every piece the explosion removed.
- **Alice is one 16×8 grid**: board A is files a to h and board B files i to p, so the renderer needs nothing new
  and squares keep their ids. A move is clicked, and written, by the square where the piece ends, on the other board
  (`Ng1-n3`). Short notation folds the files (`Nf3`). A move must be legal on its own board, as if the other were not
  there, and must not leave the king in check on the board where it then stands; check comes only from the king's
  own board. Castling is on board A, and en passant is made by a pawn on the board where the pawn that moved two
  squares now stands.
- **Draws by too little material**: chess's rule in Chess960; kings alone in Atomic and Alice; never in Crazyhouse or
  Antichess.
- **Bughouse is not built.** It is two games played at once, by four players, with captured pieces passing between
  the boards as they happen. The engine waits on one seat at a time (ADR 0114 leaves several impulses outstanding at
  once for later), so Bughouse waits on that.

## Consequences

- A tutorial of a chess game must give a `seed` or a `start`, as for any seeded game.
- Alice's boards are drawn as one, without a gap, and a flipped board puts board B on the left.
- Move generation is checked against published perft counts for Chess960, Antichess and Atomic, and Crazyhouse's
  start; Alice has none to check against, and is covered by tests of its rules.
