# 0056. The pie rule as a setup choice, not a fixed rule

Status: accepted (2026-10-27)

## Context

`bingle_tasks.md` asked for "a setup option to enable the pie rule for all 2 player abstract games
where it would work", and for "a DSL/Json attribute for game definitions `supports_pie_rule`
(change the actual name of the attribute if it makes sense to do so)".

The engine has had the move since the v2 engine work: a `swap` action recolours every piece on the
board, exchanges the reserves and gives the turn back to the first player, and Hex and Gomoku each
declared one in their document. So the rule worked — but as a fixed property of those two games,
with no way for a table to leave it out and no way for any other game to offer it.

## Decision

**One attribute, `pie_rule`, with the default in it.** `supports_pie_rule` would have said only
that the rule is possible, and every game that offers it would then have needed a second attribute
to say whether a table starts with it on: Hex and Gomoku are *normally* played with the pie rule,
while Yavalath or Amazons are normally played without it. So the attribute carries both:

| value | meaning |
|---|---|
| `"standard"` | played with the rule; the table may turn it off (Hex, Gomoku) |
| `"optional"` | played without it; the table may turn it on |
| `"on"` / `"off"` | settled — what a derived document carries, nothing left to choose |
| absent | the game has no pie rule |

**The catalog expands it** (`internal/engine/catalog/pierule.go`, called before the rules are
compiled): when the rule is in play it appends the `swap` action, and when the creator may still
choose it appends a `bool` setup option, `pie-rule`, whose `default` is the game's own practice.
Hex and Gomoku therefore *dropped* their hand-written swap action and declare `"pie_rule":
"standard"` instead — the compiled game is identical to what it was, which matters because games
already stored under `hex-11` keep their rules and their move histories replay.

**A table's choice is a derived game** (ADR 0028), `hex-11~nopie` or `yavalath~pie`, built by a
`pieDeriver` in the catalog itself rather than by the game: a JSON-only game has no Go code to
implement `catalog.Deriver`, and this is the first option that is the platform's rather than a
game's. `Catalog.Derive` now also accepts an **empty suffix** from a Deriver, meaning "that is the
game itself", so ticking the box on Hex — the rule it plays with anyway — keeps the plain id and
the plain link. The id stays the only record of the choice; nothing new is stored per game.

**It is refused where it cannot work**, when the game is registered, so a mistake is a build-time
error and not a broken room: a game of other than two seats; a game whose seats have a `forward`
direction (swapping recolours the pieces where they stand, so Draughts and Breakthrough would come
out with every man facing the wrong way); a game that declares a `swap` action of its own; and a Go
ruleset, which builds its own moves and would have to offer swapping sides itself.

**Which games offer it.** Where it would work means two seats, the generic engine, no chance, no
direction of play, and a start that is symmetric between the colours:

* `"standard"`: **Hex**, **Gomoku** — as before, but now the table may leave it out.
* `"optional"`: **Yavalath**, **Havannah**, **Pente**, **Connect Four**, **Amazons**, **Ataxx**,
  **Reversi**, **Go**.
* nothing: **Tic-Tac-Toe** (a drawn game: there is no advantage to even out), **Tak** (its opening
  already hands the first stone to the opponent, and two balancing rules would fight),
  **Brandubh** (the two sides are not the same army, so recolouring is meaningless),
  **Draughts** and **Breakthrough** (a direction of play), and every Go ruleset — Blooms, Fendo,
  Fist, Hive, Onitama, Yinsh, Courtfall, Cthulhu Wars, Nyet!. A ruleset that wants the pie rule can
  offer a swap of its own later; the attribute deliberately does not pretend to give it one.

**The page.** A game's `bool` options were drawn only under the seats of a table of three or more,
which no two-player game has, so they now also appear in a field of their own
(`#setup-options`) for a game of two, from the same builder. An option is sent in the create call
only when its box differs from the game's default, which is what keeps `hex-11` out of
`hex-11~pie`.

## Consequences

* Every game that offers the rule says so in its own rules page and in `rules[]`, so a player
  reads it where they read everything else about the game.
* The versions of the ten games that changed went up a minor step; the engine's did too, as the
  DSL grew an attribute.
* A game created before this change is untouched: `hex-11` still means Hex with the swap.
