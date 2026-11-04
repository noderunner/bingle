# COURTFALL — Third Edition
*A tactical skirmish game for two players.*

Five exiles from a fallen court meet in the arena. Nobody dies here — the crowd pays for *moments*, not corpses. Favor is earned in the exchange and spent at the rail, and the fighter who knows when to catch their breath is the one who walks out champion.

**Design pillars:** positioning over attrition, tempo as a spendable resource, hidden-information combat, a personal clock instead of a shared one — and zero bookkeeping.

## 1. Components

- **10 chess pieces** — one Pawn, Knight, Bishop, Rook, and Queen per player (light and dark sets)
- **37 hex tiles in 4 colors** — 27 tan (Open), 4 green (Overgrowth), 3 blue (Mire), 3 red (Hazard)
- **6-sided dice** — each player has 4 light dice (attack) and 4 dark dice (defense)
- **Stones in 4 colors:**
  - **White** — Fatigue markers (beside units)
  - **Black** — Exhaustion markers (beside units)
  - **Blue** — Momentum (a pool at your board edge)
  - **Red** — Victory Points (kept separately; never spent once earned)

A unit carries at most one stone: none = **Fresh**, white = **Fatigued**, black = **Exhausted**. Units are never removed from the board. Nothing is ever written down.

---

## 2. Building the Arena

Lay out a hexagonal field of 37 tiles (a center tile plus three rings), all tan. **Mark the center tile — the Eye of the Arena.**

Draft the terrain: starting with the player who lost the opening roll-off, alternate replacing one tan tile with a colored tile (4 green, 3 blue, 3 red) until all ten are placed. Constraints: none in either deployment zone (each player's back two rows); the Eye is always Open; no two red tiles adjacent; no more than two special tiles adjacent to any single tile.

| Tile | Name | Effect |
|---|---|---|
| Tan | Open | No effect. |
| Green | Overgrowth | Blocks ranged line of sight *through* it. A unit in Overgrowth gains a **defensive Edge** (§6) against ranged attacks. |
| Blue | Mire | Entering ends the Move action immediately. A unit in Mire rolls −1 die in all contests. |
| Red | Hazard | A unit that enters or is pushed into a Hazard immediately **suffers one Hit** (§7) and stays there. |

**The arena's edge:** a unit pushed off the edge suffers one Hit and is placed back in the hex it left.

**Deployment:** alternate placing one unit at a time in your back two rows, the roll-off loser placing first. The roll-off winner takes the first turn.

**The standard arena.** Ticking *Standard arena* when the game is made skips the draft and lays a
fixed terrain instead — Overgrowth in two pairs in opposite corners, two Hazards beside the Eye, the
odd Mire and Hazard on the middle row — which obeys every rule of the draft above and maps onto
itself under a half turn of the board, so neither player has the better ground. Ten decisions fewer
before the first move, and the arena that batches of bot games need: the same ground every time, so
two batches can be compared.

---

## 3. Conditions

Fatigue is your action economy and your health track at once — acting wears you down and getting hit wears you down, and they wear down the *same* thing.

| State | Marker | Dice | Can act? |
|---|---|---|---|
| **Fresh** | none | Full CV | Full activation — 2 actions |
| **Fatigued** | white | −1 die | Exert only — 1 action |
| **Exhausted** | black | 1 die, defense only | Cannot act |

**Recovery:**
- **Start of every one of your turns**, free and automatic: remove one white stone from any one friendly unit.
- **Black stones** are removed only by your **Clear Turn** (§5) or the Rouse spend (§8).
- There is no other recovery. There are no rounds. Nothing resets on its own — your squad wears down until *you* choose to stop and breathe.
