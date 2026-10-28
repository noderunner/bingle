# 0061. Courtfall: the odds of a pool, and a standard arena

Status: accepted (2026-10-27)

## Context

`bingle_tasks.md` asked for two things for Courtfall: a chart in the Combat page of the rules
"showing approximate odds for number of success", with the dice along one axis and the success
threshold along the other and each cell separating the odds "with a forward slash"; and "a default
train tile placement as an option for new games and that can be used for bot training", the
arrangement being mine to choose ("that seems balanced").

Courtfall's dice are a small, closed arithmetic — a pool of 1 to 4 dice, each succeeding on 4+, 5+
or 6+ once Edges are counted — and the game is *about* that arithmetic: a player who cannot see
that three dice at 6+ is nearly nothing is playing a different game. The terrain draft, on the
other hand, is ten decisions before anybody moves.

## Decision

**The odds are a table, and the table is tested.** Rows are the success number (4+, 5+, 6+),
columns the dice (1 to 4, the Combat Value cap), and a cell gives the chance of at least one
success, then at least two, and so on — `88/50/12` for three dice at 4+. A new test reads the table
out of `combat.md` and recomputes every figure from the binomial, so the rules page cannot drift
from the dice; a figure below half a percent is written `<1` rather than `0`, which would be a lie.
Around it, two sentences of what the numbers mean in an Exchange (a Hit needs *strictly more*, so
read the table in pairs) and the two things it cannot show: a pool of zero scores nothing, and a
reroll is worth most where the odds are worst.

**The standard arena is a setup option**, `standard-arena`, which lays this terrain and starts the
game at the choice of Courts:

```
        G   G   ·   H   ·   M      row 3
      M   ·   ·   ◎   ·   ·   H    row 4   (◎ is the Eye)
        M   ·   H   ·   G   G      row 5
```

It obeys every constraint of the draft (nothing in either deployment zone, the Eye Open, no two
Hazards adjacent, no tile touching more than two special tiles) and **maps onto itself under a half
turn of the board about the Eye**, so neither player has the better ground. Three Mire and three
Hazard are odd numbers, so two tiles cannot have a partner of their own colour; those two sit on the
middle row — the one row equally far from both deployment zones — and are each other's partners.
Overgrowth comes in two pairs in opposite corners, because a pair is a screen worth shooting round;
the two Hazards beside the Eye make the centre cost something to hold.

A table that takes the option plays **`courtfall~arena`**, derived by the ruleset's own
`Derive`/`FromSuffix` from a patch of `params.arena` (ADR 0028). Choosing to draft — what the game
does anyway — returns no suffix, so the plain game keeps its plain id and nothing stored changes.
Nothing else in the ruleset knows about it: `Start` lays the tiles and sets the phase, and from
there it is a game whose draft happens to be over.

**Why it matters for training.** A batch of bot games on the drafted arena measures the bots *and*
ten random terrain decisions per game, and two batches of the same bots are not comparable because
they were played on different ground. `courtfall~arena` is the fixed board the Analysis tab (ADR
0060) and the training environments can hold still, and it is the id a batch names.

## Consequences

* Courtfall goes to 0.2.0: a new option and a new starting position, no change to any rule. The
  engine's own version does not move — nothing outside the game changed — so the notes join the
  newest entry of the changelog rather than starting one, which is what CONTRIBUTING.md asks for.
* The option appears in the setup by itself, through the `bool` options every two-seat game now
  draws (ADR 0056).
* A second layout could be added later as another value of `params.arena` without touching
  anything else; the option would become a list rather than a check box.
