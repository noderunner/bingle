# 0098. Prototype games: each version of the rules is a game of its own

Status: accepted (2026-11-04)

## Context

Mycelium and Courtfall are still being designed. Their playtesters need to try one set of rules against
another, which the platform could not do: a game has one definition, and ADR 0053 says a game has one version.
A change to the rules also reaches a game under way on its next move. That is a fair price for a corrected
border (ADR 0097), and not for a playtest, whose result means nothing if the rules move during it.

## Decision

**`prototype: true` marks a game whose rules are not final.** The list shows a *Prototype* mark in place of
*Beta*. `version` on a prototype names its rules rather than its implementation, whatever the number, so the
rule of ADR 0053 that 1.0.0 ends the beta does not apply. Courtfall and Mycelium start at 1.0.0.

**Each version is a game of its own, with the id `family-version`** (`mycelium-1.0.0`), its variants
`family-version-name` and its derived games `family-version~suffix`. The catalog refuses a prototype whose id
does not say its `version`. Nothing else had to learn about versions. As ADR 0028 has it, the id is the only
record of the choice, so a game in progress, a tournament, a bot and a stored move list all stay on their rules
because their id does. The separator is a dash because `encodeURIComponent` leaves it alone, where an `@` would
reach the router as `%40`.

**A version is a folder holding the whole game**: `internal/games/mycelium/v1.0.0/` has the definition, the Go
ruleset, the guide and the tutorials, as a Go package of its own. `internal/games/mycelium` only lists them
(`Modules()`). Nothing is shared between versions, so a change to a new version cannot alter an old one. A new
version starts as a copy of the folder.

**The family's name means the newest version**, by number. `Catalog.Pin` turns `mycelium` into
`mycelium-1.0.0`, and `mycelium~pie` into `mycelium-1.0.0~pie`. A game or a tournament is stored under the pinned
id, so a client that only knows `mycelium` still gets a game that keeps its rules. `Catalog.Get` resolves the name
too, for reading. `GET /game-defs` lists every version, with `prototype` and `version`.

**The setup offers a *Rules version* list**, newest first and chosen, even when there is only one. It is separate
from the *Variant* list, which offers the variants of the version chosen, and it never lists the product of the
two. Every place that names one game says the version too (*Mycelium 1.0.0*), through `GameDefinition.Title`.

**A tutorial belongs to a version**, since it plays a fixed line under given rules. It lives in the version's
folder and names the version as its `game`, and `tutorial.Verify` refuses the family's name there, which would
start to mean another game when the next version ships.

**A prototype is not rated.** A rating says how well somebody plays a game, and rules that are still changing are
not one game. A rated game of a prototype is refused, the setup does not offer one, and a tournament of one is unrated.
A game rated before this may finish, and counts for nothing (`void`). Ratings already earned stay in the database and
are not shown on a leaderboard or a profile, so they are still there if the game comes out of prototype.

**Games stored before this keep their rules.** `store.Open` moves what was stored under `courtfall` and `mycelium`
to their `-1.0.0` ids: games, derived games, tournaments and tutorial rooms. It finds nothing to do afterwards.

## Interpretations

- **A new version is made only when the maintainer asks for one.** Otherwise a change, to the rules or not, is made
  in the folder of the version it belongs to, and the version stays as it is, since it is part of the id. Such a
  change reaches a game under way on its next move, as it does for any other game. What a game is pinned to is the
  version, and a version only stands still once a newer one has replaced it as the one being worked on.
- **The web client is shared by all versions.** It has no folder per version, and draws what a ruleset's view
  says, so an old version keeps working as long as the view keeps the shape the page reads. A new version that needs
  a new drawing adds to the page and leaves the old shape working.
- The metrics and the setup profiles use the family as before.

## Consequences

- A version costs a copy of its game in the repository and the binary: about 3,800 lines of Go for Courtfall and
  1,200 for Mycelium. A bug found in the code common to two versions has to be fixed in both. That is the price of
  not being able to change an old version by accident.
- Moving a game out of prototype is a later decision, and it would take a migration like this one in reverse.
- A version nobody plays can be retired by its id (ADR 0076), which keeps its games as a record.
- A server rolled back to before this change does not know the `-1.0.0` ids, and treats those games as unknown.
