# 0129. The rules report facts about what a selection did

Status: accepted (2026-11-13). Decision 12 of the achievements plan
([docs/history/achievements-plan.md](../history/achievements-plan.md)), and the ground [ADR 0130](0130-badges.md)'s
badges stand on. It adds to the `Ruleset` contract of [ADR 0025](0025-go-rulesets-and-hidden-information.md) and keeps the
purity [CLAUDE.md](../../CLAUDE.md) asks of `Legal` and `Apply`.

## Context

A badge worth having is about something that happened in play: *earn two spellbooks in a single battle as Great
Cthulhu*. Nothing outside the rules can tell. `MoveResult` says who won and why and nothing else about what a
selection did, and the ways round that are all wrong:

- **A condition over the position** would tie every badge to how its ruleset happens to lay out `ext`, which is
  the ruleset's own business and changes with it.
- **Comparing positions** before and after a selection works for a count of pieces and for nothing a game keeps
  in its own words, such as a spellbook.
- **Reading the notation** is a second parser of every game's moves, and breaks when the notation changes.

The rules already know. They only have to say so.

## Decision

**`Apply` reports facts.** `MoveResult.Facts` is a list of `state.Fact{Name, Seat}`, in the order they happened:
`{spellbook, 2}`, `{battle_start, 0}`. The seat is the one the fact is about, which need not be the seat that
moved (a battle's loser can earn a spellbook too); `0` means the game rather than a seat. A thing that happens
twice is two facts. A selection that does nothing worth saying reports none, and most selections of most games
report none.

**Facts are output, not I/O.** They are worked out from the position and the selection alone, so `Apply` stays pure
and deterministic: the same position and selection give the same facts, a replay (history, an import, a regroup)
gives them again, and a search bot that applies a million selections in memory simply ignores them. An empty list
is nil, so a game without facts pays nothing.

**Every game lists the facts it can report**, at the top level of its definition, a Go ruleset's YAML included:

```yaml
facts:
  spellbook:    {description: A faction earns a spellbook.}
  battle_start: {description: A battle begins.}
  battle_end:   {description: A battle is over.}
  drew_elder:   {description: A seat draws an Elder Sign.,  secret: true}
```

A name is lower case with underscores and unique in its game; a variant may add facts by merge patch, like
anything else. `played`, `won`, `drawn` and `lost` are reserved: they are the platform's counts of results (ADR 0130),
not facts a game reports. A badge may name only facts on its game's list, and the catalog refuses one that names
anything else. A test plays every registered game and refuses a fact that is not on its list. If one gets through
anyway, the session drops it with a log line and a metric. A move never fails because of how it was reported.

**A declared game reports facts from its actions and effects.** An action or an `end_of_turn` effect takes
`fact: <name>`, reported for the mover each time the action is applied, or each time the effect removes something
(once for the effect, not once per piece). A count per piece, or a fact about another seat, waits for a game that
needs it, and comes with an addition to the DSL.

**Facts are stored with their selection**, in a `facts` column (JSON, NULL when there are none) of its row in
`moves`. Whatever reads them reads that, and never replays a game to find them. They go when retention deletes
the game. Only the counts that badges keep outlive it (ADR 0130).

**A secret fact stays secret.** `secret: true` marks a fact whose mere existence tells something hidden: a card
drawn, a choice committed. Facts are not in any event or API response for now. The badge workers read them from
the store. Anything that serves facts later holds the secret ones back from everyone but the seat they are about
until the game ends, as `RedactMove` holds back a move.

**Not facts:** the result, the end reason and the rated flag, which a badge reads from the game at its end; and
who moved and with which action, which the move already records.

## Consequences

- `Ruleset`'s methods keep their signatures. The contract grows: `Apply` reports the facts its game declares, and
  only those. A ruleset that reports nothing is still correct, and it gets no game badges.
- Cthulhu Wars' badge needs its rules (in the private overlay) to report `spellbook`, `battle_start` and
  `battle_end`. That is step 8 of the plan. No other game reports anything until a badge asks it to.
- The DSL's `fact` is one field on actions and effects. Its schema and `docs/dsl.md` say so when it is built.
- Tutorials and practice batches report facts like any game. Nothing counts them (ADR 0130).
