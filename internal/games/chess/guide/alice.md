# Alice Chess

Alice chess is played on **two boards**, side by side: **board A** (files a to h), where the pieces start, and **board B** (files i to p), empty at the start. Each square of one board has its twin on the other: e4 on board A is m4 on board B.

## Through the looking glass

- A piece moves as in chess, using only the pieces on **its own board**, as if the other were not there.
- Then it passes to the **twin square on the other board**, which must be empty. A move whose twin square is taken cannot be made.
- A capture takes the piece on the board where the move is made.

To move, click the piece and then the square where it will end, on the other board. In the notation the move names that square: `Ng1-n3` is the knight's move from g1 to f3, ending on board B.

## Check

A king is in check only from pieces on its own board. A move must be legal on its own board, and must not leave your king in check on the board where it then stands.

## Special moves

- **Castling** is made on board A, and the king and the rook then both pass to board B, whose squares must be empty.
- **En passant** is made by a pawn on the board where the pawn that moved two squares now stands.

The game is won by checkmate and drawn by stalemate, by the same position three times, after fifty moves each without a capture or a pawn move, and when only the kings are left.
