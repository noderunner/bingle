# Plan: achievements and badges

Status: **settled**, ready to implement (opened 2026-10-28, settled 2026-11-13). The sections
before *Decisions* are the initial thinking, kept as written; where they differ, the decisions
win. Nothing is implemented. ADRs [0129](../decisions/0129-the-rules-report-facts.md) (facts) and
[0130](../decisions/0130-badges.md) (badges) record the decisions as they are to be built.

## What was asked for

- A system of badge awards for players accomplishing things across the games on the platform.
- Requires authenticated accounts.

## What already exists that this builds on

- The session layer already publishes an event stream through its hub (with a tap), so
  something can watch games finish without being wired into every call site.
- Games record a winner, an end reason and a detail, and `game_versions` records the rules
  version a game was played under.
- Practice batches record per-seat and per-bot results, so "beat the strongest bot" is
  measurable.

## Initial thinking

**Award at the moment it happens, and store it.** Retention deletes inactive games, so
anything that recomputes achievements by scanning history is wrong the moment
`BINGLE_RETENTION_DAYS` bites. An award is an immutable row — account, badge, when, which game,
a detail — written when the triggering event occurs.

**Keep running counters rather than re-deriving.** A small `stats(account_id, key, value)`
table makes every rule a constant-time check against a counter instead of a query over history,
which is both faster and the only version that survives retention. **This is the same table the
tournaments plan wants**, and the two plans must share one, not build two.

**Evaluate off the hot path.** Badge evaluation happens after a move is recorded and outside
the room lock. A move must never get slower because someone is three games away from a badge.

**Badges should be declarative data**, not Go code — a list of definitions with a counter or
event pattern, a threshold, a name, a description and an icon. Then new badges need no release,
and a plugin game can ship its own. The icon set already has a `data-icon` convention to hang
them on.

**Farming is the design problem, not the schema.** Badges keyed to "games played" are badges
keyed to "games played against a bot you beat in nine plies". The interesting anchors are
distinct human opponents, rated games, variety across game families, and difficulty of the bot
beaten. Worth deciding the anti-farming rule *before* the badge list, because it determines
which badges are possible.

**Privacy.** A public badge wall is a behavioural record — when someone plays, how much, with
whom. Displaying it should be the account holder's choice, consistent with the auth plan's
treatment of handles and names.

**Retroactive awards are a one-time chance.** If we want existing history to count, the
backfill has to run in the release that introduces the feature, before retention removes any
more games. After that the opportunity is gone. Worth a deliberate yes or no.

## Open questions for the discussion

1. What is the tone? Dozens of small badges (a game-platform feel) or a handful of meaningful
   ones (a club feel)? This changes the whole design.
2. What is the anti-farming anchor — distinct opponents, rated games only, or bot difficulty?
3. Platform-wide badges, per-game badges, or both? Per-game means plugins ship badges.
4. Public by default, private by default, or off unless the player opts in?
5. Backfill existing history at launch, or start everyone at zero?
6. Do bots or guests appear at all? My instinct: a guest earns nothing, and beating a bot counts
   only for explicitly bot-flavoured badges.

## Where things stood at settling (2026-11-13)

- `stats(account_id, key, value)` exists, but only a **rated** game writes it, through the
  pending-results worker (`recordResult` in `internal/session/ratings.go`): two seats, two
  accounts, no bots. Unrated games, games at larger tables and games against bots leave nothing
  durable behind.
- Rated games already carry anti-farming guards: both seats must have moved, a pair has a daily
  limit, and `ratings.opponents` counts distinct opponents.
- `rating_history`, `stats` and `tutorials_done` survive retention; games and their moves do not.
- Data packs are dropped (the plugins plan), so "a plugin game ships its own badges" now means a
  game's definition carries them, the private overlay's games included.
- ADR 0124: the server carries meaning, the client the look. A badge's icon is the client's.
- No profile has a visibility setting; friends lists exist.
- `MoveResult` says who won and why, and nothing else about what a move did.

## Decisions (settled 2026-11-13)

### What a badge is

1. **Few and meaningful: a club, not a game platform.** A badge marks something a player would
   mention, not an hour spent. The first list of badges is proposed in the pull request that
   brings the evaluator, for the maintainer to choose from; it is not settled here.
2. **Platform badges and per-game badges.** Platform badges live in one embedded YAML file; a
   game's badges live in its definition under `badges`, so a Go ruleset's YAML carries them too
   and an overlay game ships its own. A badge has an id (a game's are namespaced by family), a
   name, a description and its condition. **Its icon is the client's**, in `web.yaml` (ADR 0124).
3. **One award per badge.** No tiers, and no rarity shown (it would need a count across accounts
   and leans towards the platform feel).

### Which games count

4. **Unrated games and games at larger tables count.** A game counts when every seat is held by a
   distinct account. A badge checked at the end (decision 9) also needs every seat to have moved,
   the rated games' no-show rule.
5. **A badge may be marked `rated`.** It then counts only rated games whose result was written,
   not voided, so it inherits the rated games' guards (both moved, the daily limit per pair).
   This is the anchor for any badge where collusion between friends would matter.
6. **Variety badges count distinct opponents or distinct families**, never games played.
7. **Bot badges are their own kind.** They count a game of one account against bots, need the
   win, and name the strength of the bot beaten (its place in the game's list of bots), never a
   number of games. A bot earns nothing, and a bot at a seat makes the game count for bot badges
   only.
8. **Who earns:** accounts only. A guest earns nothing, and a seat held by a guest makes the game
   count for nobody. An account playing through the API or MCP is an account like any other.
   **Tutorials earn no badges**, as the plans branch's `plan-dependencies.md` already says.

### When a badge is earned

9. **A badge says when it is checked: `during` play or at the `end`.**
   - `during`: it depends only on things that happen in play. It is awarded as soon as it is
     earned, so an abandoned game keeps what was earned in it and the announcement lands in the
     moment.
   - `end`: it depends on the result, the end reason or the rated flag. It is awarded once the
     game has finished and, for a `rated` badge, once its rated result is written.
   A `rated` or winning condition forces `end`.
10. **Evaluation never touches a move.** It runs in a worker outside the room lock, reading moves
    already recorded since the last one it evaluated (a watermark per game), so it survives a crash
    and a move never waits for it. The `end` check extends the pending-results worker from rated
    games to every game that counts (decision 4), which also writes the counters of decision 14.
    The worker has a metric and a log line for every failure (the observability plan).
11. **Secrets stay secret.** An award that depends on a secret fact (decision 12) is recorded at
    once and shown to its holder, and to everyone else, the game's chat included, only when the
    game ends.

### What a condition can say

12. **The rules report facts.** `Apply` returns, beside the new state, a list of named facts about
    what the selection did: `{seat 2, spellbook}`, `battle_start`, `battle_end`. It stays pure and
    deterministic: facts are more output, not I/O. A DSL game declares facts on its actions and
    effects. **Every game lists the facts it can emit**, each with a description and whether it is
    secret, and a badge may name only facts on that list. Facts are stored with their move, so the
    worker reads them rather than replaying, and retention deletes them with the game. This changes
    the `Ruleset` contract and gets an ADR of its own.
13. **A condition is built from a small fixed set of parts**, never a general expression language:
    - **count** a named fact for the earning seat, **at least** *N* times;
    - **within** a window: the whole game, between two facts (`battle_start` to `battle_end`), or
      across games, which is a lasting counter;
    - **filters**: the seat's faction or side, the game's family or variant, `rated`, won.
    A condition the parts cannot say gets a new part by ADR. A condition over a game's state was
    rejected: it would tie badges to how each ruleset happens to lay out its state.

    The example the design was settled on, a Cthulhu Wars badge:

    ```yaml
    badges:
      - id: deep-call
        name: "…"
        description: Earn two spellbooks in a single battle as Great Cthulhu.
        when: during
        seat: { faction: great-cthulhu }
        count: { fact: spellbook, at_least: 2, within: [battle_start, battle_end] }
    ```
14. **Lasting counters live in `stats`**, which the tournaments plan built and owns. Every game that
    counts adds to played, won, drawn and lost per family, and to every declared fact per family,
    under keys of their own: the existing rated keys keep their meaning. A badge added later is
    checked against its counter the next time that counter moves.

### Awards and who sees them

15. **An award is a row that never changes**, apart from becoming visible (decision 11): account,
    badge, when, the game and a detail, with the badge's name and description as they were, the
    way `rating_history` keeps a family's name. A badge that is retired or redefined keeps the
    awards already given. A deleted account's awards display under the `deleted-<uid>` rule.
16. **Every registered user can see every account's badges.** Guests cannot. There is no
    visibility setting.
17. **An award is announced** to its holder by a hub event (a toast) and a notification, and to
    the room by a line in the game's chat, for the other players to see. No email.
18. **Where badges are seen:** the profile page, `GET /api/accounts/{handle}/badges` and an MCP
    tool, all behind an account.
19. **No backfill.** Everyone starts at zero on the release that brings badges. This closes the
    question of retroactive awards.

## Order of work

1. **ADRs**: one for badges (decisions 1 to 19), one for facts in the `Ruleset` contract
   (decision 12).
2. **Facts**: `MoveResult` carries them, moves store them, definitions declare them, the DSL emits
   them, and a secret fact never reaches a public event.
3. **Every game that counts is recorded at its end**: the pending-results worker generalised from
   rated games, writing the counters of decision 14.
4. **Badge definitions, the awards table and the `end` evaluator**, with the first platform list
   proposed for choosing.
5. **The `during` worker**, with its watermark and the secret delay.
6. **Announcements**: the hub event, the notification and the chat line.
7. **The API, the MCP tool and the profile page.**
8. **Game badges**, starting with Cthulhu Wars' spellbook badge in the private overlay, which also
   needs its rules to emit `spellbook`, `battle_start` and `battle_end`.

## Dependencies and cross-plan effects

- **Needs accounts** (auth plan), including the `deleted-<uid>` rule so an old award still
  displays after an account is deleted.
- **Shares the statistics table with the tournaments plan.** Whichever lands first owns it.
  This is the strongest coupling among the new plans.
- **Needs the observability plan's event discipline**: an evaluator consuming a live event
  stream is exactly the component that silently stops working, so it needs a metric and a log.
- **Plugin plan**: badge definitions must be data for a plugin game to have any.
- **Tutorials**: completion is the natural first badge.
- **Retention** is the constraint that shapes the storage model.

## Status log

- 2026-10-28: opened with initial thoughts. Event-time awards, declarative badge definitions, a
  shared counters table, evaluation off the move path, and farming as the real design problem.
  Planning in progress.
- 2026-11-01: tournaments are being built first and own `stats(account_id, key, value)`, starting with
  played, won, drawn and lost per family; add keys there. Their game-end hook is the place to attach
  evaluation. Rated games, per the tournaments plan, are one available anti-farming anchor.
- 2026-11-13: settled. Few and meaningful badges, platform and per game; unrated and larger games
  count, with `rated` badges for what collusion would spoil; a badge is checked during play or at
  the end; rules report declared facts, and conditions are built from a fixed set of parts; every
  registered user sees badges; awards are announced in the game's chat; no backfill.
- 2026-11-13: ADRs 0129 (facts) and 0130 (badges) written, step 1 of the order of work; the plan moves
  from the plans branch to `docs/history`.
- 2026-11-13: steps 2 to 4 built (#190, #191, #192): facts, every counting game recorded at its end,
  and badge definitions with the `end` check and the first five platform badges. Step 5, the `during`
  worker with its watermark and the secret delay, follows.
- 2026-11-14: step 5 built (#193): the `during` worker, its watermark per game, and secret awards
  shown when their game ends. Step 6, announcements, follows.
- 2026-11-14: step 6 built (#194): a badge is announced to its holder (a toast and a browser
  notification, on by default for an account since #195) and in the game's chat. Step 7 follows,
  at `GET /api/players/{handle}/badges` beside the ratings rather than `/api/accounts/…`.
