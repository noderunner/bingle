# 0028. Derived games and setup options

## Context

The first Cthulhu Wars expansion brought two more cults (Windwalker and Sleeper),
a fifth seat and an optional unit for every cult (the High Priests). A table is
now a *choice*: three to five seats, which of six cults sit in them, and whether
an expansion is in. A game definition is a fixed document — seats with colours
and labels, a turn order, the pieces, the legend — and the whole platform (the
catalog, stored games, sessions, bots, the REST and MCP APIs, training
environments, the browser) addresses a game by the id of that document.

Making every combination a *variant* of the definition would be a few hundred
entries (6 cults in 5 seats, with or without the expansion, on three layouts),
none of which anyone wants to list. Special-casing the choice in the ruleset
alone would leave the seats, colours and labels of the document saying the
wrong thing, and every client would need to learn a second place to look.

## Decision

**Options.** A definition may declare `options` (`dsl.OptionDef`): a `bool` (a
check box) or a `seat-choice` (each seat picks one of the listed `choices`, none
twice, each with the colour it brings). They are descriptions for clients — the
home page draws them, `GET /api/game-defs/{id}` and the MCP `get_game_info`
list them — and the *value* of the options is a map in the create call
(`options` in `POST /api/games`, the MCP `create_game` argument, and
`CreateOptions.Options` in the session manager).

**Derived games.** A ruleset that takes options implements `catalog.Deriver`:

* `Derive(base, options)` validates the options against the base definition and
  returns a *suffix* naming the combination (stable, letters, digits and dashes:
  `ww-sl-gc-cc-ys-hp`) and a **merge patch** of the base document (seats,
  turn order, parameters, legend);
* `FromSuffix(base, suffix)` rebuilds the patch from a suffix, so that a game
  stored under its id can be loaded again.

The catalog registers the result on demand under `base~suffix` (`~` is
`catalog.DerivedSep`). From there on it is an ordinary game: same callbacks,
its own seats and colours. `Catalog.Get` derives a missing `base~suffix` id
itself, which is how stored games, API calls, bots and training environments
need no knowledge of any of this; `Catalog.List` leaves derived ids out so that
listings show the games a person can choose from, not their combinations. A
create call whose options equal the layout's own keeps the plain id.

The id is the only record of the choice: nothing else is stored for a game, and
a game stored before the options existed keeps meaning what it meant (the
five-player layout of those games has four cults — the position says so, `Data.Players`).

**The ruleset reads the table from the definition** (`config.go`:
`gameCfg` — the number of seats, the cult of each seat, the expansion flag and
the sigil areas), not from compile-time constants; every rule asks the cult of a
seat (`e.is(seat, facWW)`, `e.seatOf(facSL)`) instead of assuming a seat number.

## Consequences

* A combination costs nothing to add: the well over a thousand ways to seat
  the six cults, with or without the High Priests, are just ids. Tests derive tables (`tests/cthulhuwars/expansion_test.go`)
  and play random games on every kind.
* The seat count of a layout is fixed by its document; a table cannot have more
  seats than the layout, and every layout can seat any cult for which its map has
  the areas it needs (the Windwalker and Sleeper need their *sigil* areas, which
  `Derive` checks).
* Clients need nothing but `options` in the game info: the browser draws a
  list of the cults per seat and the check box, sends `options` only when the table
  differs from the layout's own, and shows derived games by their base name.
* A new game with choices of its own implements `Deriver` the same way; the
  platform side does not change.
* Because a derived document is built by merge patch, anything in the base that a
  derived game should *not* inherit has to be overwritten by the patch — for
  Cthulhu Wars the seats, the turn order (the Windwalker leads, because it makes
  the first decision of the game), the parameters and the gate legend.
