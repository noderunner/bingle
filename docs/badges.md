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
  server's list of bots, which is kept weakest first. A game against bots earns no other badge,
  and adds to no count, so a bot badge counts only within its game.
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

A `during` badge counts a game of accounts only, as a player badge does, but needs no seat to have
moved: it is awarded in the game, so a game abandoned afterwards keeps it.

## Secret awards

A badge that counts a secret fact, or opens or closes its window on one, would tell what the fact
keeps hidden. Its award is written at once and shown to its holder, and to everyone else only when
the game ends: when the results worker counts the game, or skips it.

## Where awards are kept

`badge_awards`, one row per account and badge, never changed but to become visible (`visible_at`).
Awards for badges checked at the `end` are written by the results worker, in the same transaction
as the game's counts (`bingle_game_counts_total` says when that fails).

Badges checked `during` play are written by a worker of their own, away from the moves. A move that
reports facts moves its game's `facts_seq` on and wakes the worker, which reads that game's facts
from its start, checks the badges of its seats, and moves the game's `badges_seq` up to it, in one
transaction with the awards. The watermark says where to look, not what was found, so a crash only
repeats a check; an undo takes both back no further than the move it keeps. The worker also looks
once a minute, in case a check failed, and `bingle_badge_checks_total{outcome}` counts its checks:
`checked`, `skipped` (no `during` badges, or a guest or bot at a seat) and `failed`.

## Announcements

Each award a worker writes is announced once, after its transaction commits:

- **to its holder**, by a `badge` event on the account's `/ws-dm` socket (`{badge, name,
  description, room, game, hidden}`). The page shows it in a toast and, when the page is out of
  sight and the browser allows this site's notifications, raises a browser notification. Never an
  email. An account without the `dm` capability has no such socket, and is not told.
- **to the room**, by a line from the server in the game's chat (`server: true`, named for the
  instance): "@ana earned the badge First Win: Win a game against another player."

An award that waits for its game's end is told to its holder at once, marked `hidden`, and to
the room when it is shown. An announcement that fails, such as a chat line for a game retention
has deleted, is logged and dropped: the award stands.

The profile page, the API and MCP come with step 7 of the
[achievements plan](history/achievements-plan.md).
