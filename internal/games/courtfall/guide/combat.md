# Combat — the Exchange

Every unit has a **Combat Value (CV)**: its dice pool, after condition and Mire modifiers (cap 4, floor 1).

When a Strike is declared, both players **secretly and simultaneously** split their unit's current CV between **attack dice (light)** and **defense dice (dark)** — dice into a closed hand, reveal together, roll everything at once.

**A die succeeds on a 6.** The tactical situation improves that threshold through **Edges**: each Edge you hold for a given pool lowers its success number by 1, to a maximum of **4+** (two Edges is the most that can ever count).

**Attack Edges** (apply to your attack dice):
- **Flanking** — a non-Exhausted friendly unit is adjacent to the enemy
- **Worn Down** — the enemy is Fatigued or Exhausted
- **Charge** — Juggernaut trait (§10)
- **Isolation** — Veiled Court (§11)

**Defense Edges** (apply to your defense dice):
- **Overgrowth** — versus ranged attacks only
- **Shieldwall** — Warden trait / Bastion Court

**Resolution** — count successes, both directions simultaneously:
1. Initiator's attack successes vs. target's defense successes: if the initiator has strictly more, the target suffers a Hit.
2. Target's attack successes vs. initiator's defense successes: same — the counter lands in the same instant.

Ties go to defense. A pool with zero dice scores zero successes — total commitment to attack means any enemy attack success hits you automatically. **Winning by 2 or more successes** also pushes the loser 1 hex, direction of the winner's choice, after the Hit.

**Shooting:** the Arbalest rolls 3 attack dice at 6+ against a target 2–4 hexes away in a straight hex line (blocked by intervening units or Overgrowth; no adjacent targets). The defender rolls 2 dice if Fresh, 1 if Fatigued or Exhausted, at 6+ (5+ with the Overgrowth Edge). No allocation, no counter-fire.

**Shoving:** an opposed roll of full current CV, no allocation — the shover succeeds on **5+** (moving someone is easier than wounding them; Flanking still improves this to 4+), the defender resists on 6+ (Shieldwall applies). If the shover has strictly more successes, push the target 1 hex directly away. A shove never inflicts a Hit — the terrain does that. Exhausted units automatically lose shove contests.

**Rerolls:** after any roll, each player may spend 1 Momentum to reroll one of their own dice (once per contest per player).

The arithmetic is the message: an unsupported attack at 6+ barely lands, a flank at 5+ is credible, and a flank against a worn unit at 4+ is a genuine threat. You don't roll your way into hits in this game — you *maneuver* your way into them.

## The odds

What a pool is worth, in percent. The row is the success number your Edges have earned you, the
column how many dice you are rolling, and the figures in a cell are the chance of **at least** one
success, then at least two, and so on — so `88/50/12` means three dice at 4+ land one success 88%
of the time, two 50%, and all three 12%.

| | 1 die | 2 dice | 3 dice | 4 dice |
|---|---|---|---|---|
| **4+** | 50 | 75/25 | 88/50/12 | 94/69/31/6 |
| **5+** | 33 | 56/11 | 70/26/4 | 80/41/11/1 |
| **6+** | 17 | 31/3 | 42/7/<1 | 52/13/2/<1 |

Four dice is the cap, and Mire takes one away. Read it in pairs, because that is how an Exchange
works: a Hit needs **strictly more** successes than the other pool, and ties go to defence. Three
attack dice at 6+ (42% for one success) against two defence dice at 6+ (31% for one, 3% for two)
is nearly an even fight that mostly produces nothing; the same three dice at 4+ (88/50/12) against
those two is a different game altogether.

Two things the table does not show, and they both matter. A pool of **zero** dice scores nothing at
all, so an all-in attack hands the enemy any success they roll. And a **reroll** bought with
Momentum is worth most where the odds are worst: a sixth of a die at 6+, a half at 4+ — one
Momentum turns a 42% pool of three into roughly a 52% one.

---

## 7. Hits

When a unit suffers a Hit, advance its condition one step:

- **Fresh → Fatigued.** Attacker gains **1 Momentum**.
- **Fatigued → Exhausted.** Attacker gains **1 Momentum**.
- **Already Exhausted:** no change; instead the unit is **pushed 1 hex**, attacker's choice of direction, and the attacker gains **no Momentum**. The crowd doesn't cheer for kicking someone who's already down — but they might cheer for where they land.

Bullying an exhausted unit earns nothing except board position. Chain-pushing a helpless enemy toward a Hazard still works — Hazard hits push again — but it burns your turns while the rest of their squad acts freely, and every stone you put on *their* side of the board is one their next Clear Turn wipes away for free.
