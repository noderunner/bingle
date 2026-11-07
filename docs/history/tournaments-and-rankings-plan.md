# Plan: competitive play, tournaments and rankings

Status: **settled**, ready to implement (opened 2026-10-28, settled 2026-11-01). The sections
before *Decisions* are the initial thinking, kept as written; where they differ, the decisions
win. Nothing is implemented. Real-time play has a plan of its own,
[real-time-play-plan.md](real-time-play-plan.md).

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

## Decisions (settled 2026-11-01)

### Formats and time

1. **Tournaments first: round robin and Swiss.** A ladder and elimination brackets come later on the
   same machinery. This reverses the initial lean towards a ladder first.
2. **Every tournament game has a game clock**, which also bounds how long a round can take, so a
   round has no separate deadline: it ends when its last game does. A sweep ends games whose flag has
   fallen with nobody looking.
3. **A tournament is real-time or correspondence**, chosen by its organiser. Pairing, clocks and
   standings are shared; only start times, check-in and the no-show rule below are real-time's. A
   correspondence tournament is simply one with a long clock.
4. **Two seats only, at first.** That is every game but Nyet and Cthulhu Wars. Tournaments at larger
   tables need finishing positions (a game records only one winner today), table pairing and a rule
   for scoring a seat a bot has taken, and they get their own design later.

### Running a tournament

5. **`create_tournament` is a new capability, held by administrators by default** and grantable to
   all accounts from `/admin`. Entering is for accounts, under an `enter_tournament` capability seeded
   for `user`.
6. **A tournament passes through** draft, registration, check-in (real-time only), running, then
   finished or abandoned. Its settings are: the game and variant, format, clock and increment,
   real-time or not, games per pairing, rated or not, the player limits and the description. Swiss
   defaults to `ceil(log2(players))` rounds.
7. **A real-time tournament has a start time and a check-in window** before it. A player who has not
   checked in by the start is dropped from it. The next round starts after a short configurable break
   once the last game of a round ends; a correspondence round starts at once.
8. **No-shows:** a game has a first-move deadline, shorter than the clock in a real-time tournament.
   A player who misses it forfeits, and if both miss it both score zero. A game forfeited with no move
   from one player leaves both ratings unchanged.
9. **A pairing is two games by default, with sides swapped**, and the organiser may choose one. The
   games of a pairing run at the same time in a correspondence tournament and **one after the other
   in a real-time one**, because nobody can watch two boards at once. The pie rule is on by default
   where the game offers it.
10. **Scoring is 1, ½ and 0.** A bye scores as a win of the pairing's games. Ties break by
    Sonneborn-Berger in a round robin and by Buchholz, then Sonneborn-Berger, in Swiss. A tie that
    survives is a shared place, with no playoff.
11. **Pairing is deterministic given a stored seed.** Round robin uses Berger tables. Swiss pairs
    within score groups, seeded by rating, avoids rematches, balances sides, and gives the bye to the
    lowest-ranked player without one.
12. **A player who drops out** forfeits their games in progress and any unplayed round-robin games, is
    not paired again in Swiss, and earlier results stand. **An abandoned tournament** ends its games
    with no winner and no rating change, leaving no zombie games behind. A tournament whose game is
    retired or absent from the build is abandoned the same way (ADR 0076).
13. **Prizes are left out entirely.** There is no money and no sponsor or prize field. An organiser
    can say what they like in the description. This closes the question.

### Ratings

14. **Glicko-2, one rating per game family**, across its variants and board sizes, so that a small
    player base is not split. Pure Go, no new dependency. Bingle's games are too different for any
    global rating.
15. **Rated is chosen per game.** Any game with two seats, each held by a different account, may be
    marked rated at setup, provided it has a game clock. Tournament games are rated unless the
    organiser turns that off.
16. **In a rated game**, undo, take-backs, a player's own Auto-Play and vote-kicks are off. Undo can
    reopen a finished game today (only a resignation is refused, and `UndoToSeq` sets the game active
    again), and a rating must never be written for a result that can come back. Hints are already
    refused against a human during a game. A bot taking a seat makes a game unrated.
17. **Lopsided games are rated like any other, with one rating per player.** Playing both sides and
    the pie rule are the mitigation, rather than rating by faction or leaving games such as Courtfall
    and Brandubh out.
18. **Public leaderboards per game, and ratings on profiles.** A player appears only by choosing
    rated play and after enough games. A leaderboard shows the handle, rating and number of games;
    never when or against whom they played. Standings are public.
19. **Humans only.** Built-in bots cannot enter rated games or tournaments. API moves stay allowed,
    because the planned Bevy client plays that way, so using an outside engine is forbidden by a stated
    fair-play rule and enforced by moderators, not by detection. Someone can break it undetected, and
    that is accepted. A separate bot ladder comes later.
20. **Implementation defaults, not discussed separately** and easy to change: ratings start at 1500
    with a deviation of 350; a rating is provisional above a deviation of 110; a leaderboard needs 5
    rated games against at least 3 opponents; at most 5 rated games a day between the same two
    accounts change their ratings. Each game updates the rating at once, as a rating period of one.

### Storage

21. **Everything is written when a game ends**, and needs no game row afterwards, because retention
    deletes them. Three tables, all in `measuredTables`:
    - `ratings`: account, family, rating, deviation, volatility, games, opponents.
    - `rating_history`: immutable, one row per rated result: account, opponent, family, variant, seat,
      score, rating before and after, tournament if any, when. The family's name is stored too, for a
      game no longer in the build.
    - `stats(account_id, key, value)`: the counters shared with [achievements](achievements-plan.md),
      which this plan now owns. It starts with played, won, drawn and lost per family; achievements
      adds keys. There is no backfill of old games.
22. **One game-end hook.** Games end through `RecordMove`, `FinishGame` (resignation, agreement,
    timeout) and retirement. All must reach one function, after the commit and outside the room lock,
    so a move never waits for a rating. A write that fails is logged and counted.
23. **Retention must skip the games of an unfinished tournament.** A correspondence clock can be up to
    30 days a side, longer than `BINGLE_RETENTION_DAYS` may be, and `DeleteInactiveGames` would remove
    a game mid-round. Each result is copied to the tournament's own tables, so finished games become
    expendable like any other.
24. **Results refer to accounts**, so a deleted account appears as `deleted-<uid>` in old standings.

### Interface

25. **The home page gets a Tournaments tab and leaderboards.** Everything is in the API, and MCP gets
    the reading and entering tools. No rule of a tournament lives in the browser, so that a later
    client can do all of it.

## Order of work

Each step is its own pull request, with an ADR where it settles a question:

1. **Clock fixes**: step 1 of the real-time plan.
2. **Results and ratings**: the game-end hook, the three tables, Glicko-2, the rated option and its
   restrictions, then leaderboards. Worth having before any tournament exists.
3. **Correspondence tournaments**: the schema and states, capabilities, registration, round robin,
   Swiss, the sweep, retention, standings and notifications.
4. **Real-time tournaments**: start times, check-in and no-shows, once steps 2 and 3 of the real-time
   plan are in.

**Later, each with its own design:** the ladder, elimination brackets, multi-seat tournaments, and the
bot ladder.

## Dependencies and cross-plan effects

- **Needs accounts** (auth plan): a rating belongs to an account, and `deleted-<uid>` accounts
  must still appear in historic standings.
- **Lands before achievements and owns `stats`**: both want a durable per-account statistics table
  written at game-end, and building two would be a mistake. Achievements adds its keys to this one.
- **Needs the [real-time plan](real-time-play-plan.md)**: its clock fixes for any tournament, and its
  connection and setting work for real-time ones.
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
- 2026-11-01: settled, with real-time play split into its own plan. Round robin and Swiss first, in
  real-time and correspondence forms; every game clocked; two seats first; Glicko-2 per family; public
  leaderboards; rated by choice at setup or by tournament; humans only under a fair-play rule; no
  prizes. Tournaments now land before achievements and own the statistics table.
- 2026-11-02: step 1 (clock fixes) merged; step 2 built (ADR 0080, branch `results-and-ratings`).
  Departures: a rated game also needs equal clocks and the standard start; `stats` counts rated games
  only, because an ordinary game can be undone after it ends and counted twice; the no-show rule
  applies to every rated game. Left for step 3: a tournament column in `rating_history`, the pie rule
  defaulting on. Not built: any tool to remove a rating, which the `moderate` capability still waits for.
- 2026-11-02: step 2 merged (#64), and step 3 built (ADR 0082, branch `correspondence-tournaments`).
  Departures: a no-show in correspondence is simply the clock, with the first-move deadline and both
  players scoring zero left for step 4; the organiser may also remove a player; there is no separate mail
  for a round starting. Left for step 4: the real-time setting, start times, check-in and no-shows.
- 2026-11-02: ADR 0083 changes decision 16: a move stands once the turn has passed in every game, not only
  a rated one, so take-backs are gone everywhere. Back takes back a step of the turn still being played, in
  rated and tournament games too. ADR 0080's reason for `stats` counting rated games only has gone with it.
- 2026-11-02: real-time play steps 2 and 3 merged (#75, #76), and step 4 built (ADR 0090, branch
  `real-time-tournaments`). Decided: it starts by itself, with no Start button; entries close when check-in opens;
  the first-move deadline is per player; "both miss it" means the first player missed it and the other seat shows as
  disconnected. Left: the ladder, elimination, multi-seat tournaments and the bot ladder, each with its own design.
