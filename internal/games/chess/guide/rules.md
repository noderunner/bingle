# Chess

Two armies of sixteen pieces on an 8×8 board. **White** moves first, then the players take turns, one move each.

## The goal

You win by **checkmate**: your opponent's king is attacked (in **check**) and no move can save it. You may never make a move that leaves your own king in check; when your king is in check, the square is tinted and you must get it out.

## The pieces

| Piece | Moves |
|---|---|
| ![](piece:king) King | one square in any direction |
| ![](piece:queen) Queen | any distance along a rank, a file or a diagonal |
| ![](piece:rook) Rook | any distance along a rank or a file |
| ![](piece:bishop) Bishop | any distance along a diagonal |
| ![](piece:knight) Knight | in an L: two squares one way and one the other; it jumps over anything in between |
| ![](piece:pawn) Pawn | one square forward, or two from its first square; it captures one square diagonally forward |

Every piece but the knight stops at the first piece in its way. A piece captures by moving onto an enemy piece, which is removed.

## Special moves

- **Castling.** The king moves two squares towards one of its rooks, and that rook jumps to the square the king crossed. You may castle only if neither piece has moved yet, the squares between them are empty, and the king is not in check, does not cross an attacked square and does not land on one. To castle, move the king two squares.
- **En passant.** A pawn that moves two squares and lands beside an enemy pawn may be captured by it *as if it had moved one*, but only on the very next move.
- **Promotion.** A pawn that reaches the last rank becomes a queen, rook, bishop or knight of your choice: after moving it, choose the piece.

## Draws

The game is drawn:

- by **stalemate**: the side to move has no legal move and is not in check;
- when the **same position** comes up three times, with the same player to move and the same moves possible;
- after **fifty moves** each with no capture and no pawn move;
- when neither side has the **pieces left to checkmate** (kings alone, or with one bishop or knight, or with bishops all on squares of one color).

A player may also offer a draw, which the other may accept, or resign.

## Move notation

- A move is the piece's letter (none for a pawn), the square it leaves, `-` (or `x` for a capture) and the square it reaches: `e2-e4`, `Ng1-f3`, `e4xd5`.
- The letters are **K** king, **Q** queen, **R** rook, **B** bishop and **N** knight.
- A promotion adds the new piece: `e7-e8=Q`. Castling is `O-O` on the king's side and `O-O-O` on the queen's.
- Short algebraic notation (`e4`, `Nf3`, `exd5`, `e8=Q`) and UCI (`e2e4`) are understood when you type a move.
