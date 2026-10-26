# Notes

## Designer's notes and tuning levers

- **The Clear Turn is the whole strategy layer.** Every other system feeds it: fatigue accrual sets how badly you need one, the price ladder punishes waiting too long, the Eye rewards clearing from a position of strength, and Seize the Tempo exists partly so you can punish an opponent the turn they stop to breathe. If one player clears every 4 turns and the other every 7, they are playing legitimately different strategies.
- **Rough success math** (before defense cancels): 3 dice at 6+ lands at least one success ~42% of the time; at 5+, ~70%; at 4+, ~87%. Unsupported attacks against Fresh, defended units are nearly futile *by design* — the Worn Down Edge means combat naturally escalates against weakened targets, so the first clean Hit you engineer opens the door to the second.
- **Pacing is knob one.** Victory at 5 VP with a Momentum cap of 5 and the 2/3/4 ladder means roughly three good Clear Turns to win. If games run long, drop to 4 VP or let the Eye also pay 1 Momentum when an *enemy* Clear Turn happens while you hold it. If they're too fast, raise to 6.
- **Stalling self-polices, mostly.** A leader who turtles still has to Clear to cash, gifting free turns; a pacifist can grind Eye income, but at 1 Momentum per Clear that's a VP every two Clears — anyone actually fighting outruns it. If turtling appears anyway, the lever is making the Eye contested: no payout if an enemy unit is adjacent to it.
- **The Exert decision got sharper.** With no Momentum tax, Exerting is priced entirely in recovery: a black stone can't trickle off and sits until your Clear Turn (Gale Court excepted). Exerting early in your cadence is cheap; exerting right after a Clear is mortgaging your freshest asset.
- **Zero-dice allocations stay legal.** All-in attack against a coward's full-defense split is the purest bluff in the game; with success-counting it's even sharper, since a full-defense pool at 4+ Shieldwall odds blanks almost anything.

## How the rules text was read here

| Rule | Implementation |
|---|---|
| Roll-off | Replaced by seats: Light drafts and deploys first, Dark takes the first turn and chooses its Court first. |
| "No more than two special tiles adjacent to any single tile" | Read literally (every tile, tan ones included) it leaves room for only **eight** of the ten tiles on this board with two-row deployment zones. It is applied to special tiles: no special tile touches more than two others. A placement is only legal if the remaining tiles can still all be placed, so every draft completes. |
| Court choice | Sequential (Dark first), not simultaneous. |
| Free recovery | Automatic when only one unit qualifies; otherwise a `recover:X` decision. A Gale Court may instead turn a black stone white. |
| Activation | 2 actions (Move, Strike, Shoot, Shove, in any order), ended early with `end`. The stone is earned at the end: Fresh → Fatigued; a unit already hit during its own activation steps on to Exhausted. A unit that becomes Exhausted mid-activation loses its remaining actions. |
| Exert | 1 action, then the white stone becomes black (Exhausted). |
| Moving | Through empty hexes only (the Outrider leaps). Mire and Hazard end the move. Entering a Hazard is a Hit with no attacker (no Momentum). |
| Strike | Attacker commits its split, target answers (an Exhausted target has one defense die), all dice roll at once. Both sides' hits land in the same instant; ties go to defense; a margin of 2+ pushes the loser (winner's choice of direction). |
| Edges | Flanking (another non-Exhausted friend adjacent to the target), Worn Down, Charge, Isolation for attack; Overgrowth (vs shots) and Shieldwall for defense. At most two count; the success number never goes below 4. Isolation (like Charge) is an Edge of Strikes only — "in Exchanges" — while shots and shoves get Flanking and Worn Down. |
| Charge | The Juggernaut's previous action was a straight move of 2+ hexes and the target was not adjacent to where it started. |
| Shoot | Arbalest only: 3 dice (−1 if Fatigued, −1 in Mire) at 6+ against 2 dice (1 if Fatigued/Exhausted) at 6+; line of sight is blocked by units and by Overgrowth *between* the two, not by the target's own hex. A shot won by 2+ pushes the target. |
| Shove | Full CV against full CV; the shover needs 5+ (attack Edges lower it, never below 4+), the target 6+ (Shieldwall helps). Exhausted units lose automatically. The Juggernaut pushes 2 hexes, the Cinder Court 1 more. Shoves that cannot move the target (a unit directly behind it, or an un-pushable Warden) are not offered. |
| Pushing | A push into an occupied hex is not possible. Off the edge is a Hit and the unit stays. Into a Hazard is a Hit and ends the push. A Hit on an Exhausted unit (terrain or combat) pushes it on, the attacker choosing the direction — a chain push may not go off the edge, so chains always end. |
| Momentum | +1 when your Hit advances a condition (+1 more for exhausting the Champion), +1 (+1 Cinder) for pushing an enemy into a Hazard or off the edge, +1 for the Eye at your Clear Turn. Hazard and edge Hits themselves pay nothing. Cap 5. |
| Rerolls | After the dice are shown the initiator, then the target, may spend 1 Momentum to reroll one **failed** die (rerolling a success can never help). |
| Hit and Run | Offered after the Outrider's own Strike if it is not Exhausted: shift one hex or stay. |
| Surge, Rouse, Seize | Surge/Rouse (and a Stone Court swap) come before the turn's main choice; Seize is offered when a turn completes (after a Clear Turn too) unless that turn was itself a Seize. Rouse leaves the unit Fresh. |
| Clear Turn | `clear` (buy 0) or `clear+N`. The Eye pays first (counting towards what you can afford), then VP at 2/3/4…, then every stone is removed and the Stone Court swap refreshes. |
| Victory | 5 VP at a Clear Turn. **Platform addition:** after 2000 plies the player with more VP wins (equal VP is a draw), so a game always ends. |

The numbers (VP to win, Momentum cap, ply limit) are `params` of the
definition, so a variant can change them with a JSON merge patch.
