# 0130. Badges: few, declared, earned during play or at the end, and never taken back

Status: accepted (2026-11-13). Decisions 1 to 19 of the achievements plan
([docs/history/achievements-plan.md](../history/achievements-plan.md)), as they are to be built. It builds on
[ADR 0129](0129-the-rules-report-facts.md) (facts), [0080](0080-rated-games-and-results.md) (rated games and their
pending results), [0124](0124-the-server-says-what-things-are-clients-decide-how-they-look.md) (a client's look) and
[0123](0123-tutorials-come-in-series-of-lessons.md) (tutorials).

## Context

Players asked for badges for accomplishing things across the games. Three things shape how:

- **Retention deletes games.** Anything that works badges out from history stops being true when
  `BINGLE_RETENTION_DAYS` deletes a game. An award has to be written when it is earned, and a count that spans
  games has to be kept as a count.
- **Farming.** A badge for games played is a badge for games played against a friend who resigns at once. What a
  badge counts decides which badges are possible.
- **Speed.** A move must never wait for a badge.

The rated games of ADR 0080 already solve part of this: a result is written after the game, outside the room lock,
from a row marked pending, and it comes with guards against farming (both seats moved, a daily limit per pair).
Only rated games leave anything durable behind, though, and the maintainer wants unrated games and larger tables
to count.

## Decision

### What a badge is

**Few and meaningful: a club, not a game platform.** A badge marks something a player would mention, not an
hour spent. One award per badge: no tiers, and no rarity shown.

**Badges are declared, never coded.** Platform badges are one embedded YAML file. A game's are in its definition
under `badges`, so a Go ruleset's YAML carries them too and an overlay game ships its own. A game's badge ids are
namespaced by family (`cthulhu-wars.deep-call`) and a badge with the same id in two versions of a family is the
same badge. Each badge has an `id`, a `name`, a `description`, `when`, and its condition. **Its icon is the
client's**: the web page's is in the game's `web.yaml` under a top-level `badges`, and the platform's in the page's
own file. `display.badges`, the marks drawn on a unit, keep their name: they are a different thing in a
different place.

### What a condition can say

A condition is built from a small fixed set of parts, never a general expression language. Every part present must
hold:

| part | says |
|---|---|
| `when` | `during` (awarded as soon as it holds) or `end` (checked once the game has finished) |
| `count` | `{fact, at_least, within}`: a fact about the earning seat, at least *N* times. `within` is `game` (the default), `[open, close]` (between two facts, such as `[battle_start, battle_end]`, counted afresh in each window), or `games` (a lasting count, below) |
| `distinct` | `{of: opponents \| families, at_least}`: different accounts played, or different families played, across games |
| `won` | `true`: the earning seat won |
| `rated` | `true`: only rated games whose result was written, not voided |
| `bot` | `{at_least: <bot id>}`: a bot badge, below |
| `seat` | `{faction}` or `{side}`: the earning seat's faction, or the seat as its definition numbers it |
| `game` | `{family}` or `{variant}`: for a platform badge, one family; for any badge, one variant |

`won`, `rated`, `distinct`, `bot` and `within: games` depend on the end, so they need `when: end`. The catalog
refuses a badge that breaks this, names a fact its game does not declare, or reuses an id. A window's facts are
matched by name whatever seat they name, and a `count` of `played`, `won`, `drawn` or `lost` with `within: games`
counts the platform's results, in one family or across all of them. A condition the parts cannot say gets a new part
by ADR. A condition over a game's state was rejected (ADR 0129).

```yaml
badges:
  - id: deep-call
    name: The Deep Calls
    description: Earn two spellbooks in a single battle as Great Cthulhu.
    when: during
    seat: {faction: great-cthulhu}
    count: {fact: spellbook, at_least: 2, within: [battle_start, battle_end]}
```

### Which games count

- **For player badges, a game counts when every seat is held by a different account**, rated or not, at any size of
  table. A badge checked at the `end` also needs every seat to have moved, the rated games' no-show rule.
- **A `rated` badge** counts only rated games with a written result, so it inherits their guards. It is the badge
  for anything collusion between friends would spoil.
- **Variety counts distinct opponents or distinct families**, never games played.
- **Bot badges are their own kind.** They count a game of one account against bots only, need the win, and name the
  weakest bot that will do: `at_least: mcts` is any bot at or after `mcts` in the bot list, whose order is kept
  from weakest to strongest. A bot earns nothing.
- **A seat held by a guest, or a tutorial, makes the game count for nobody.** A game of two accounts and a bot
  counts for nothing either. An account playing through the API or MCP is an account like any other. Practice
  batches are played in memory and count for nothing.

### When it is worked out

**Nothing runs on a move.** A move records its facts and goes. Two workers outside the room lock do the rest, and
each has a metric and a log line for every failure:

- **The `during` worker** follows a watermark per game (the last move it looked at). When a game has moves past it,
  the worker reads that game's facts from its start, checks the `during` badges of the seats that can earn, writes
  the new awards and moves the watermark. The watermark says where to look, not what to remember, so a crash only
  repeats work, and an award is written once (below).
- **The `end` check** is the pending-results worker of ADR 0080, generalised from rated games to every game that
  counts. A finished game is marked pending in the transaction that ends it, as now. The worker writes the rated
  result first, if any, then the counts below and the `end` awards in one transaction with clearing the mark, so a
  retry never counts twice. A game abandoned before its end keeps whatever `during` awards it gave.

**Lasting counts live in `stats(account_id, key, value)`**, which the tournaments plan built and owns, under keys
of their own. The rated keys (`played:<family>`, `won:<family>` and the rest) keep their meaning. Every game that
counts adds to `game.played:<family>` and to `game.won`, `game.drawn` or `game.lost`, to `fact.<name>:<family>` for
every fact about the seat, and to `met:<account id>` for each opponent (`met.rated:<account id>` in a rated game).
Distinct opponents and families are a count of those rows. A badge added later is checked against its count the
next time that count moves, and there is **no backfill**: everyone starts at zero on the release that brings
badges.

### Awards

**An award is a row that never changes**, apart from becoming visible: account, badge id, the badge's name and
description as they were, when, the game's id and family, a detail, and when it became visible. There is one row
per account and badge (unique), so a worker that runs twice awards once. The game's id is kept and not tied to the
game, which retention may delete. A badge that is retired or redefined keeps the awards already given, and an id
is never reused for a different badge. A deleted account's awards display under the `deleted-<uid>` rule of the
accounts plan.

**A secret award waits for the game's end.** An award that depends on a secret fact is written at once and shown
to its holder, and to everyone else, the game's chat included, only when the game ends.

**An award is announced** to its holder by a hub event (a toast on the page) and a notification, never an email,
and to the room by a line from the server in the game's chat.

**Every registered user can see every account's badges**, on the profile page, at
`GET /api/accounts/{handle}/badges` and through an MCP tool. Guests cannot. There is no setting to hide them.

## Consequences

- The plan's order of work follows: facts (ADR 0129); every counting game recorded at its end; definitions, the
  awards table and the `end` check, with the first platform badges proposed in that pull request for the
  maintainer to choose from; the `during` worker; announcements; the API, MCP tool and profile page; then game
  badges, starting with Cthulhu Wars' in the overlay.
- The pending-results worker now runs for most finished games, not only rated ones. Its work per game is a few
  rows, but it is the one place a counting bug becomes a lasting one, since counts are never re-derived.
- The bot list's order becomes a promise: a new bot is placed by strength, not appended.
- A badge's condition is fixed at the time it is earned. Changing a badge's meaning means a new id.
