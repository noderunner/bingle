# Courtfall

Courtfall (Third Edition) is a two-player arena skirmish: five exiles per side
on a 37-hex arena, fatigue as both action economy and health, a secret
simultaneous dice split in every exchange, Momentum as a spendable resource
and a personal *Clear Turn* that cashes the crowd's favor into victory
points. The rulebook is [courtfall-rulebook.md](courtfall-rulebook.md); this
page says how bingle implements it and where it had to interpret.

Courtfall is the first game whose rules are **written in Go** (a
`state.Ruleset`, see [ADR 0025](decisions/0025-go-rulesets-and-hidden-information.md))
because it needs things the declarative DSL does not have: per-unit
conditions, dice, hidden choices and turns made of many small decisions. Its
definition (`internal/games/courtfall/courtfall.json`) still declares the
board, seats, pieces and the display legend, so the lobby, the web UI, the
REST/WebSocket API, the MCP server, the bots and the training environments
treat it like any other game.

## Playing it

* **Seats.** Light is seat 1 and Dark seat 2. They stand in for the rules'
  opening roll-off: Light (the "loser") drafts terrain and deploys first,
  Dark (the "winner") picks a Court first and takes the first turn.
* **Setup** is part of the game record: `tile:green@d3` (green Overgrowth,
  blue Mire, red Hazard), `court:bastion`, `deploy:Q@c6`.
* **A turn** is a sequence of decisions, each one a move: free recovery
  (when there is a choice), then optional start-of-turn spends (`surge`,
  `rouse`, `tiles`), then `activate:Q` / `exert:Q` / `clear`, then the unit's
  actions (`Qd4` move, `Qxd4/2` strike, `B*e5` shoot, `R^d4` shove, `end`),
  any exchange sub-decisions (`respond/1`, `reroll/3`, `keep`, `push>NE`,
  `shift>d6`, `stay`) and finally `seize` or `done`.
* **Secrecy.** A strike commits the attacker's split (`Qxd4/2`) in secret: the
  position, the move list, the WebSocket events and exports show
  `Qxd4/?` until the target has answered. The dice come from a seeded
  generator that is never sent to clients, and a game cannot be taken back
  across a die roll — otherwise a player could look at the dice and choose
  again.
* **Notation** is documented in `get_game_info` / the rules dialog; every move
  has a fixed action ID (`env_spec`) and the observation planes are in
  [mcp.md](mcp.md).

## How the rules text was read

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
