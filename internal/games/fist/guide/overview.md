# Fist

A two-player game from Alysha Wishingrad's novel *The Verdigris Pawn* (Crab Fragment Labs, open beta rules 1.1). A **challenger** with a mage, an ace and a few guards storms the Keep of a **king** with a whole army — and a single neutral pawn, the **verdigris pawn**, may be seized by whichever side sees it first.

## The goal

| Side | Colour | Wins by |
|---|---|---|
| ![](seat:1) | moves first | capturing the **king** |
| ![](seat:2) | moves second | capturing all **three blue guards** of the challenger |

Whoever reaches their goal wins at once. A side with no legal move draws, and so does a game that reaches 400 plies.

## The board

Fist is played on an 8×8 board. Two squares **touch** if they share an *edge* (never only a corner). A piece **sees** along all eight lines — straight and diagonal — until another piece blocks its view.

The four central squares **d4 e4 d5 e5** are **the Keep**. The king starts there (on e5), and any piece standing in the Keep can always see the verdigris pawn.

## Setup

::: note
You do not have to remember the diagram: the board starts in the right position. In words:
:::

- **The king and three yellow guards** hold the Keep (the king on e5).
- The **king's side** also has seven blue guards and nine more yellow guards, spread round the top and right edges and the lower right, guarding the corners that hold the mage and the pawn.
- The **challenger** crowds the a1 corner: the **ace** on a1, three blue guards and six yellow guards around it — while the **mage** sits far away in the a8 corner, ringed by the king's guards.
- The **verdigris pawn** waits in the h1 corner, belonging to nobody.

## Values at a glance

Everything in Fist is about *value*: it decides what a piece may jump over and how strongly it pins.

| Piece | Value | Moves | Captures by |
|---|---|---|---|
| ![](piece:yellow) [Yellow guard](tab:pieces) | 1 | 1 square | pinning |
| ![](piece:blue) [Blue guard](tab:pieces) | 2 | up to 2 squares | jumping |
| ![](piece:ace) [Ace](tab:pieces) | 3 | up to 3 squares | jumping (and Convert) |
| ![](piece:mage) [Mage](tab:pieces) | 3 | up to 3 squares | jumping (and Empower) |
| ![](piece:king) [King](tab:pieces) | 4 | 1 square | pinning |
| ![](piece:pawn:green) [Verdigris pawn](tab:pieces) | 4 | up to 4 squares | pinning *or* jumping |

Read [the pieces](tab:pieces) for each one in detail and [moving and capturing](tab:moves) for the two ways to take a piece, with examples.

## How a turn goes

1. Pick one of your pieces (or the verdigris pawn, if you can see it — see below).
2. Either **step** it in a straight line over empty squares, **or jump** with it. Never both in one turn.
3. Steps may end in a **pin** (yellow guards, the king and the pawn); jumps may **capture** the pieces they leap.

In the game you click the piece, then where it goes. Pins and jump choices show as buttons: *Capture*, *Spare*, *Convert*, *End*.
