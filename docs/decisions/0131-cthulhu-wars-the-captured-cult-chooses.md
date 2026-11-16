# 0131. Cthulhu Wars: the captured cult chooses which of its Cultists is taken

Status: accepted (2026-11-16). Supersedes the **Capture** and **Bots** parts of
[ADR 0051](0051-cthulhu-wars-capture-choice-and-neutral-window.md); the rest of 0051 stands.

## Context

ADR 0051 let the captor choose, when the enemy had more than one Cultist in the area, between one from the stack
and the Cultist on the Gate, and the bots always took the one on the Gate. That came from a playtest request, not
from the rules. An audit of the game against its rulebook found that the rulebook gives that choice to the other
side: "If there is more than one Cultist in the Area, the player whose Cultist is being Captured chooses which one
is Captured. (Thus, you do not have to allow a Gate-Controlling Cultist to be Captured, unless it is the last
remaining Cultist in the Area.)" (printed p. 16). The maintainer chose to follow the rulebook.

## Decision

- **Capture** (`captureMoves`, `capture` in `actions.go`): the captor names only the area and the enemy,
  `capture@area/faction`. When the enemy has more than one Cultist there (an Acolyte, a High Priest, the one on the
  Gate), a `captive` prompt asks the **enemy** which is taken: `lose:cultist@area`, `lose:high-priest@area`, or the
  occupant, `lose:gate:cultist@area` / `lose:gate:high-priest@area`, as Devour asks its victim. A lone Cultist is
  taken with no question. The two `lose:gate:…` notations are appended to `ActionNotations`; the old
  `capture:<piece>` notations still apply when a saved game replays them but are no longer offered.
- **The Gate** is unchanged from ADR 0051: capturing its occupant abandons it, and the captor's own Cultist in the
  area takes it at once.
- **Bots** (`guide.go`) give up an Acolyte before the High Priest, and one beside the Gate before the occupant.

## Consequences

A cult can keep its Gate against Capture while it has another Cultist in the area, so capturing a Gate's last
Cultist is the only way Capture takes a Gate. A capture with a choice costs the game one more impulse: the enemy's
answer to the prompt. The engine change is in the private overlay, which carries the Cthulhu Wars code
(ADR 0127).
