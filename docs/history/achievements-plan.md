# Plan: achievements and badges

Status: **planning in progress** (opened 2026-10-28). Initial thoughts written before the
design discussion, not settled decisions. Nothing is implemented.

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
