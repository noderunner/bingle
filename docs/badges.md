# Badges

A badge marks something a player would mention, not an hour spent
([ADR 0130](decisions/0130-badges.md)). An account earns each badge once, and keeps it. This page is
for whoever writes one: the platform's are in `internal/badges/platform.yaml`, and a game's are in
its definition under `badges`, where its id becomes `family.id`.

```yaml
badges:
  - id: deep-call
    name: The Deep Calls
    description: Earn two spellbooks in a single battle as Great Cthulhu.
    when: during
    seat: {faction: gc}
    count: {fact: spellbook, at_least: 2, within: [battle_start, battle_end]}
```

## The parts

Every part a badge has must hold. There is no other way to say a condition: one the parts cannot
say needs a new part, by ADR.

| part | says |
|---|---|
| `when` | **required.** `during`: awarded as soon as it holds, from what happens in play. `end`: checked once the game has finished |
| `count` | `{fact, at_least, within}`: a fact about the earning seat, at least *N* times. `within` is `game` (the default), `games` (a lasting count), or `[open, close]`, two facts between which the count starts afresh each time, whoever they name |
| `distinct` | `{of: opponents \| families, at_least}`: different accounts, or different families, played across games that count |
| `won` | `true`: the earning seat won |
| `rated` | `true`: only rated games whose result was written. A `count` of results and a `distinct` then count rated games only |
| `bot` | `{at_least: <bot id>}`: a bot badge, below |
| `seat` | `{faction}` (the faction's key in the game) or `{side}` (the seat's number in the definition) |
| `game` | `{family}` (a platform badge only) or `{variant}` (a game's id) |

A badge may `count` only the facts its game declares (`facts` in the definition, see
[dsl.md](dsl.md)), or the platform's results, `played`, `won`, `drawn` and `lost`, with
`within: games`. A platform badge counts results only, since it knows no game's facts. A count
across games is per family for a game's badge or a platform badge that names a family, and across
every family otherwise.

`won`, `rated`, `distinct`, `bot` and a count across games are known only at the end, so they need
`when: end`. The catalog refuses a badge that breaks a rule here, so a mistake stops the server
from starting rather than a badge from ever being earned.

## Which games count

- **Player badges** count a game when every seat is held by a different account and every seat
  has moved, rated or not, at any size of table.
- **Bot badges** count a game of one account against bots only. They need `won: true`, and name
  the weakest bot that will do: `at_least: tactical` is the tactical bot or any after it in the
  server's list of bots, which is kept weakest first. A game against bots earns no other badge.
- A guest at a seat, a tutorial or a game somebody never moved in counts for nobody. Practice
  batches are played in memory and count for nothing.

## The platform's badges

| id | name | condition |
|---|---|---|
| `first-win` | First Win | win a game against another player |
| `well-met` | Well Met | play ten different people |
| `wide-reader` | Wide Reader | play five different games against other people |
| `contender` | Contender | win twenty-five rated games |
| `machine-breaker` | Machine Breaker | beat the strongest computer opponent (`mcts`) |

An id is never reused for a different badge. A badge that is retired or redefined keeps the
awards already given, under the name and description they were given with.

## Where awards are kept

`badge_awards`, one row per account and badge, never changed but to become visible. Awards for
badges checked at the `end` are written by the results worker, in the same transaction as the
game's counts (`bingle_game_counts_total` says when that fails). Nothing shows them yet; the
profile page, the API and MCP come with step 7 of the
[achievements plan](history/achievements-plan.md), and `during` badges with step 5.
