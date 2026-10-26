# COURTFALL — Third Edition
### A tactical skirmish game for two players

Five exiles from a fallen court meet in the arena. Nobody dies here — the crowd pays for *moments*, not corpses. Favor is earned in the exchange and spent at the rail, and the fighter who knows when to catch their breath is the one who walks out champion.

**Design pillars:** positioning over attrition, tempo as a spendable resource, hidden-information combat, a personal clock instead of a shared one — and zero bookkeeping.

---

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

---

## 4. The Flow of Play

Players simply alternate turns for the whole game. At the start of each of your turns, take your free white-stone recovery, then do exactly one of the following:

1. **Activate** a Fresh unit — 2 actions, then it gains a white stone.
2. **Exert** a Fatigued unit — 1 action, then its white stone is swapped for black.
3. **Take a Clear Turn** (§5).

That's the entire structure. No rounds, no passing, no shared clock — each player runs their own engine and decides for themselves when to cash out and reset.

---

## 5. The Clear Turn

A Clear Turn is your whole turn: no unit acts. Resolve in order:

1. **The Eye pays out:** if a non-Exhausted friendly unit stands on the Eye, gain 1 Momentum.
2. **Cash the crowd's favor:** buy VP with Momentum at escalating cost — **the first VP costs 2 Momentum, the second costs 3, the third costs 4**, and so on. Buy as many as you can afford and want. The price ladder resets at your *next* Clear Turn.
3. **Catch your breath:** remove **all** white and black stones from **all** friendly units. Your entire squad is Fresh.
4. **Check victory** (§9).

This is the game's central dilemma. Clearing often means cashing often — VP arrives faster, and the price ladder resets before it gets steep. Clearing rarely means each Clear Turn recovers more stones per turn spent — better action efficiency — but your favor sits at risk, your squad degrades, and your opponent gets to pick fights with your worn units in the meantime. There is no correct cadence; there is only reading the board.

A Clear Turn with no stones on your units and only 2 Momentum in your pool is legal — sometimes you just need the point.

---

## 6. Combat — the Exchange

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

---

## 7. Hits

When a unit suffers a Hit, advance its condition one step:

- **Fresh → Fatigued.** Attacker gains **1 Momentum**.
- **Fatigued → Exhausted.** Attacker gains **1 Momentum**.
- **Already Exhausted:** no change; instead the unit is **pushed 1 hex**, attacker's choice of direction, and the attacker gains **no Momentum**. The crowd doesn't cheer for kicking someone who's already down — but they might cheer for where they land.

Bullying an exhausted unit earns nothing except board position. Chain-pushing a helpless enemy toward a Hazard still works — Hazard hits push again — but it burns your turns while the rest of their squad acts freely, and every stone you put on *their* side of the board is one their next Clear Turn wipes away for free.

---

## 8. Momentum

Blue stones, capped at **5**; excess is simply never gained.

**Gain 1 Momentum when:**
- Your Hit advances an enemy unit's condition
- You push an enemy into a Hazard or off the edge
- You hold the Eye at the start of your Clear Turn
- The enemy Champion becomes Exhausted by your Hit (+1 *in addition* to the normal gain)

**Spend Momentum:**

| Cost | Effect | Timing |
|---|---|---|
| 1 | **Reroll** one of your own dice | After any roll; once per contest |
| 1 | **Surge** — shift any non-Exhausted friendly unit 1 hex | Start of your turn |
| 2 | **Seize the Tempo** — take another turn immediately (never twice in succession) | After completing your turn |
| 2 | **Rouse** — remove a black stone from one friendly unit | Start of your turn |

Every blue stone is a fork in three directions: banked VP at the next Clear, a tactical spend now, or lost to the cap. And since the price ladder climbs within a single Clear Turn, a big hoard converts *worse* than steady cashing — favor spoils.

---

## 9. Victory

The moment a player reaches **5 VP**, they win. Since VP is only bought during Clear Turns, the game always ends on a Clear — the champion steps back, raises a hand, and the crowd decides it's over.

---

## 10. The Units

Identity comes from Move, CV, and one signature trait. Baseline mobility is high on purpose — a 37-hex arena crossed at Move 3–4, plus Surges, pushes, and Hit and Run, keeps the board churning turn to turn.

### ♟ Pawn — the Warden
- **Move 3 · CV 3**
- **Shieldwall:** while adjacent to a non-Exhausted friendly unit, the Warden holds a permanent defensive Edge and cannot be pushed.

The anchor. A Warden in formation defends at 5+ and can't be moved — parked on the Eye with a friend beside it, it's a problem that must be solved with Hits, not shoves.

### ♞ Knight — the Outrider
- **Move 4, leaping** — ignores units and all terrain while moving; only the destination matters (Hazard rules apply if it ends there)
- **CV 3**
- **Hit and Run:** after resolving an Exchange it initiated, the Outrider may immediately shift 1 hex for free.

Pure tempo. It reaches anything, never bogs down, delivers flanks on demand, and steps out of retaliation range after trading.

### ♝ Bishop — the Arbalest
- **Move 3 · CV 2**
- **Shoot** (§6) — the game's only ranged attack.

Weak up close, but the Arbalest turns sightlines into pressure: chipping a key enemy to Fatigued from safety hands every nearby ally the Worn Down Edge.

### ♜ Rook — the Juggernaut
- **Move 3, straight lines only** — each Move action travels in a single direction
- **CV 3**
- **Bulldoze:** its Shove pushes 2 hexes instead of 1, and it holds the **Charge** attack Edge in any Exchange it initiated after moving 2+ hexes in a straight line into contact this activation.

Slow to turn, brutal in a lane. Adjacent to a red tile, its 2-hex shove is the scariest threat in the game.

### ♛ Queen — the Champion
- **Move 4** (any path)
- **CV 4**
- **Bounty:** when the Champion becomes Exhausted by an enemy Hit, that opponent gains 1 additional Momentum.

The best fighter in the arena, and the crowd knows it. Four dice makes her allocations the deepest mind-game on the board — and makes her collapse the payout everyone else is playing toward.

---

## 11. The Five Courts

Choose your Court after the arena is built, before deployment.

**⛨ The Bastion Court** — *the wall that walks.*
Shieldwall's defensive Edge is contagious: all your units hold it while adjacent to a non-Exhausted friendly unit. (The Warden's un-pushable clause remains hers alone.)

**⇶ The Gale Court** — *always somewhere else.*
Your free start-of-turn recovery may instead downgrade one black stone to a white stone. The Gale Court alone can nurse exhaustion without stopping.

**⬡ The Stone Court** — *the arena is a weapon.*
At the start of your turn, before acting, you may swap two unoccupied tiles within 2 hexes of each other (setup constraints still apply; the Eye never moves). Once used, this cannot be used again until after your next Clear Turn.

**🔥 The Cinder Court** — *push them into the fire.*
Your Shoves push 1 extra hex, and you gain 1 additional Momentum whenever you push an enemy into a Hazard or off the edge.

**🗡 The Veiled Court** — *cut the strays from the herd.*
Your units hold the **Isolation** attack Edge in Exchanges against an enemy with no non-Exhausted friendly units adjacent to it.

With rounds gone, "once per round" abilities now key off your own Clear Turns — your reset is also your ability refresh, one more thing the clear cadence decision has to weigh.

---

## 12. Designer's Notes & Tuning Levers

- **The Clear Turn is the whole strategy layer.** Every other system feeds it: fatigue accrual sets how badly you need one, the price ladder punishes waiting too long, the Eye rewards clearing from a position of strength, and Seize the Tempo exists partly so you can punish an opponent the turn they stop to breathe. If one player clears every 4 turns and the other every 7, they are playing legitimately different strategies.
- **Rough success math** (before defense cancels): 3 dice at 6+ lands at least one success ~42% of the time; at 5+, ~70%; at 4+, ~87%. Unsupported attacks against Fresh, defended units are nearly futile *by design* — the Worn Down Edge means combat naturally escalates against weakened targets, so the first clean Hit you engineer opens the door to the second.
- **Pacing is knob one.** Victory at 5 VP with a Momentum cap of 5 and the 2/3/4 ladder means roughly three good Clear Turns to win. If games run long, drop to 4 VP or let the Eye also pay 1 Momentum when an *enemy* Clear Turn happens while you hold it. If they're too fast, raise to 6.
- **Stalling self-polices, mostly.** A leader who turtles still has to Clear to cash, gifting free turns; a pacifist can grind Eye income, but at 1 Momentum per Clear that's a VP every two Clears — anyone actually fighting outruns it. If turtling appears anyway, the lever is making the Eye contested: no payout if an enemy unit is adjacent to it.
- **The Exert decision got sharper.** With no Momentum tax, Exerting is priced entirely in recovery: a black stone can't trickle off and sits until your Clear Turn (Gale Court excepted). Exerting early in your cadence is cheap; exerting right after a Clear is mortgaging your freshest asset.
- **Zero-dice allocations stay legal.** All-in attack against a coward's full-defense split is the purest bluff in the game; with success-counting it's even sharper, since a full-defense pool at 4+ Shieldwall odds blanks almost anything.
