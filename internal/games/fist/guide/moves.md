# Moving and capturing

On your turn you move **one piece**: it either steps or it jumps, never both. There are two ways to capture, and each piece uses one or the other (the verdigris pawn uses both).

## Stepping

A piece steps in a **straight line** in any of the eight directions, over **empty squares only**, as far as its range allows (yellow guard 1, blue guard 2, ace 3, mage 3, king 1, pawn 4). It stops anywhere along the line, but it can never pass over another piece: any piece in the way blocks the line.

## Capturing by pinning

*Yellow guards, the king and the verdigris pawn.*

After stepping **at least one square**, the piece may capture an enemy piece that *touches its new square* (an edge, not a corner). It does **not** move onto the captured piece's square — it captures it where it stands.

Whether the pin works is a matter of **strength**:

1. **Your strength** = the piece's own value + the value of every ally that touches the target. (An ally touching *any* of the targets counts; each ally counts once.)
2. **Their resistance** = the total value of all the enemy pieces that touch your piece's new square.
3. The pin succeeds only if your strength is **at least** their resistance — and then it takes **just one** of them.

This is the **divided attention** rule: surrounded by many enemies, your piece must beat all of them together, even though it can only capture one.

::: example
A white yellow guard (value 1) steps next to a black yellow guard (value 1). Only that one enemy touches it, so resistance is 1; strength is 1: it pins and captures. If a black blue guard (value 2) also touched the square, resistance would be 3: the yellow guard would need two friends (each worth at least 1) touching its target to reach 3.
:::

The verdigris pawn pins like a guard, with the help of its mover's allies, but it can never be the *ally* that adds strength to another piece's pin.

## Capturing by jumping

*Blue guards, the ace, the mage and the verdigris pawn.*

A jump goes **exactly two squares** in a straight line (any of the eight directions): over an adjacent piece, onto the **empty** square just beyond. The piece jumped must have a value **equal to or lower than** the jumper's value. You may chain jumps up to the piece's range (blue guard 2, ace 3, mage 3, pawn 4), but you may never land on a square you have already landed on in the same turn — including the one you started from.

- For **each piece jumped** you choose: *capture it*, or *spare it*. You can even jump (and spare) your own pieces to get across the board.
- The **ace** may also *convert* a captured guard (see [the ace](tab:pieces)).
- The verdigris pawn can be jumped (and spared) but can never be captured.
- A captured piece is removed at once, so later jumps in the same chain see the changed board.
- If a jump captures the **king**, the chain stops: the challenger has won.

::: example
A blue guard on a1 with an enemy yellow guard on b1 and c1 empty: it jumps to c1 and may capture the guard on b1. If another enemy yellow guard stands on d1 and e1 is empty, it can continue — but a jump over a *king* (value 4) is not allowed, since the blue guard's value is only 2.
:::

## How a game is won

- **The challenger wins** when the king is captured — by a pin, or by a jump of a piece worth 4 or more (the verdigris pawn, or an Empowered ace or mage).
- **The king's side wins** when all three of the challenger's blue guards have been captured, and not replaced by Convert.

If a side has no legal move, the game is a draw.
