# Plan: competitive play, tournaments and rankings

Status: **planning in progress** (opened 2026-10-28). Initial thoughts written before the
design discussion, not settled decisions. Nothing is implemented. One item — the prize pool —
I think we should talk about before anything is built, for the reasons below.

## What was asked for

- Tournament formats and scheduling.
- Player rankings.
- Maybe: donations towards a prize pool.
- Requires authenticated accounts.

## What already exists that this builds on

- Finished games are already recorded with a winner, an end reason, a detail and a full move
  list, and `game_versions` records which version of the game's rules was played.
- The **game clock** (ADR 0057) with per-seat handicaps, move time limits, auto-play timeouts
  and forfeits is already in place — the hard part of running a timed competition exists.
- Email notifications, presence and the away/turn mail make **asynchronous** play work today.
- Practice batches already run many bot-versus-bot games and report win rates with Wilson
  intervals in the Analysis tab, which is most of a bot ladder already.

## Initial thinking

**Ratings, and what they are per.** Bingle has 25 games with wildly different shapes. One
global rating would be meaningless, so a rating is per **game family** and seat count. I would
use **Glicko-2** rather than Elo: it carries a rating deviation, which is the right model for
bingle's actual usage — a friend who plays four games a year should not move a rating as much
as someone playing weekly, and should be shown as provisional. It is about 200 lines of pure
Go, no dependency.

**Ratings must be stored, not derived.** `RunRetention` deletes games with no activity for
`BINGLE_RETENTION_DAYS`, so any design that recomputes a rating by scanning game history is
wrong the first time retention runs. Ratings and a rating history table are written when a game
ends, and the game record is then expendable.

**Asymmetric games break the assumption.** Courtfall factions and Cthulhu Wars factions are not
mirror images, and first-player advantage varies by game. Two mitigations already exist: the
**pie rule** as a setup option, and handicap clocks. My initial line would be that ranked play
defaults the pie rule on where the game supports it, and that strongly asymmetric games are
either rated per faction or left unrated at first rather than pretending.

**Formats, chosen to fit what bingle actually is.** Bingle is a correspondence platform:
people take a turn when they get the mail. Live brackets with scheduled rounds are the worst
fit and the most machinery. I would start with **asynchronous round robin and Swiss**, with a
per-round deadline enforced by the existing clock and automatic forfeits, then an **ongoing
ladder** (challenge anyone near you, no rounds at all), which suits a small group of friends
far better than a bracket. Single and double elimination can come later; they are mostly a
pairing function plus a bye rule once the deadline machinery exists.

**Scheduling** is then: rounds with deadlines, byes for odd counts, forfeit on timeout,
drop-outs that do not corrupt the standings, and a tournament that can be abandoned without
leaving zombie games.

**Bots belong on their own ladder**, never mixed into human ratings — otherwise the first
person to grind the weakest bot owns the leaderboard. The batch machinery already measures bot
strength, so a bot ladder is cheap and genuinely useful for tuning.

**Anti-abuse matters more than it sounds.** Accounts will be free, so two accounts can feed
each other wins all day. Provisional ratings, a minimum number of distinct opponents before a
rating is published, "rated" as an explicit game setting, and the `moderate` capability for
corrections are the minimum.

## On the prize pool — my recommendation is to separate it, or skip it

This is the one item where I think the request should change shape. Taking donations and paying
them out as prizes for a competition is not a feature, it is a regulated activity: handling
other people's money needs a payment processor and its compliance obligations, and "entry fees
or donations funding cash prizes for a contest" lands near gambling law, which varies by
jurisdiction and does not care that it is a hobby server for friends. Chargebacks, refunds when
a tournament is abandoned, and the tax treatment of payouts are all real work with no
game-design payoff.

Safer shapes, in increasing order of involvement:

1. A **link** to an existing tip jar or donation page on the tournament's page. Bingle holds no
   money and makes no promises. This is nearly free and probably sufficient.
2. **Prizes funded entirely by a sponsor, with no entry fee** and prizes awarded off-platform —
   bingle records who won, a human sends the prize.
3. Actual custody of funds inside bingle. I would not build this, and if we do it should be its
   own plan with its own threat model, not a line item here.

My suggestion: design the tournament schema with an opaque `sponsor_note` and no money
concepts at all, and revisit if it ever matters.

## Open questions for the discussion

1. Asynchronous only to start, or do live timed tournaments matter to you?
2. Ladder first, or formal tournaments first? I lean ladder — it is less machinery and more fun
   for a handful of friends.
3. Who can create a tournament: admins only, or a `create_tournament` capability for accounts?
4. Are ratings public? Per-game leaderboards on the home page, or only on your own profile?
5. Is "rated" a property of a game you choose when creating it, or of a tournament only?
6. Prize pool: shall we take the link-out option and close the question?

## Dependencies and cross-plan effects

- **Needs accounts** (auth plan): a rating belongs to an account, and `deleted-<uid>` accounts
  must still appear in historic standings.
- **Needs achievements to be decided alongside it**: both want a durable per-account statistics
  table written at game-end, and building two of those would be a mistake. Whichever is
  implemented first should own `stats` and the other should read it.
- **Interacts with retention**: see above; this is the constraint most likely to be missed.
- **Interacts with the plugin plan**: a plugin game needs a stable identity and version to be
  ranked at all, and an uninstalled game must not break standings.
- **Interacts with observability**: tournament scheduling is the first background scheduler in
  bingle, and the slow-request journal and the metrics database are how we will see it misbehave.
- **Tutorials** feed this: a tutorial is how a new account becomes a player who enters a
  tournament.

## Status log

- 2026-10-28: opened with initial thoughts. Glicko-2 per family, async round robin/Swiss plus a
  ladder, bots on a separate ladder, ratings stored at game-end because retention deletes
  games, and a recommendation to keep money off the platform. Planning in progress.
