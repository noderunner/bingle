# The pieces

Every piece sees along the eight lines (until blocked), but each moves and captures in its own way. **Value** is the number that matters: a piece may only jump over pieces of equal or lower value, and a pinning piece is only as strong as its own value plus the help of its friends.

## Yellow guard

![](piece:yellow:white) ![](piece:yellow:black)

| Stat | Detail |
|---|---|
| **Value** | 1 |
| **Moves** | 1 square in any of the eight directions |
| **Captures by** | **pinning** only — it cannot jump |
| **Number** | 6 for the challenger, 12 for the king's side |

The humble foot-soldier. After stepping one square it may pin an enemy piece that touches its new square. Alone it is only worth 1, so it pins only a piece that is itself worth 1 and nothing else nearby — but yellow guards fight best in groups, because *every ally that touches the target adds its value to the pin*. Three yellow guards around a blue guard take it.

::: tip
A yellow guard that cannot step cannot pin: you must move *at least one square* first.
:::

## Blue guard

![](piece:blue:white) ![](piece:blue:black)

| Stat | Detail |
|---|---|
| **Value** | 2 |
| **Moves** | up to 2 squares in a straight line over empty squares, **or** jump up to 2 times |
| **Captures by** | **jumping** |
| **Number** | 3 for the challenger, 7 for the king's side |

The backbone of both armies. A blue guard may leap a piece of value 2 or less (yellow and blue guards) that stands next to it, landing on the empty square just beyond, and may chain a second jump. The challenger loses if all three of their blue guards fall, so they are the targets of the king's side.

## Ace

![](piece:ace:white)

| Stat | Detail |
|---|---|
| **Value** | 3 |
| **Moves** | up to 3 squares in a straight line, **or** jump up to 3 times |
| **Captures by** | **jumping** |
| **Special** | **Convert** |
| **Number** | 1, the challenger's |

The challenger's raider: it jumps over anything up to value 3 (so the guards and the mage, but not the king or the verdigris pawn).

**Convert.** When the ace captures an enemy *guard* by jumping, it may — instead of just removing it — put one of its own guards of that same kind in its place, if the challenger has one out of play. A captured yellow guard can become a challenger yellow guard; a blue one a challenger blue guard. This is also how the challenger gets blue guards back.

## Mage

![](piece:mage:white)

| Stat | Detail |
|---|---|
| **Value** | 3 |
| **Moves** | up to 3 squares in a straight line, **or** jump up to 3 times |
| **Captures by** | **jumping** |
| **Special** | **Empower** |
| **Number** | 1, the challenger's |

The challenger's engine of attack. It starts far from its army, in the a8 corner, ringed by the king's guards.

**Empower.** At the start of each of your turns the mage adds **1 to the value and the move range** of every one of your pieces it can see (the verdigris pawn is not affected). A yellow guard becomes a value-2 piece that moves two squares; a blue guard becomes value 3 and jumps three times. The new values are fixed for the whole turn, even if pieces move out of its sight.

::: warning
If the **king's side captures the mage**, the king gains Empower: from then on the king empowers every piece of the king's side that it sees. Guard your mage.
:::

## King

![](piece:king:black)

| Stat | Detail |
|---|---|
| **Value** | 4 |
| **Moves** | 1 square in any of the eight directions |
| **Captures by** | **pinning** only |
| **Number** | 1, the king's side's |

The king's side loses at once if the king is captured. It starts in the Keep, protected by yellow guards, and it is strong: a value-4 pinner that adds its help to nearby guards.

## Verdigris pawn

![](piece:pawn:green)

| Stat | Detail |
|---|---|
| **Value** | 4 |
| **Moves** | up to 4 squares in a straight line, **or** jump up to 4 times |
| **Captures by** | **pinning or jumping** (one of the two each turn) |
| **Special** | belongs to neither side; can never be captured |

The pawn is **controlled by whoever sees it**. At the start of your turn you may move it if at least one of your pieces sees it. A piece standing in the Keep always sees it, so the king's side usually has the first call — until the challenger's pieces get a view.

- It steps and pins, or jumps, but never does both in one turn.
- It can **never be captured**: pins and jumps over it can only spare it.
- Its pin counts the help of the allies of whoever moves it, but it gives none of its own (you cannot use it to help a pin by another piece).
- It **cannot go straight back** to the square it left on the previous turn.
- It is not empowered by the mage.

::: tip
Moving the pawn is a whole turn spent on it: nothing else of yours moves. It is worth doing when the pawn can pin the king, take a blue guard, or when it blocks your opponent's view of it for next turn.
:::
