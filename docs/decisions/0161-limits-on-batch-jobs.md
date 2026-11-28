# 0161. Limits on batch jobs: ceilings a spec is refused over, a player's allowances, and a time limit that ends a batch

Status: accepted (2026-11-28), for issue #317. Builds on the practice batches (ADR 0034, `internal/batch`), their
capability ([ADR 0071](0071-capabilities-modes-api-tokens-and-invitations.md)'s `run_batch`) and the bot ladder
([ADR 0160](0160-a-bot-ladder.md)). Who goes first when a batch and a real game want the same CPU is issue #316's, not
this one's.

## Context

A batch plays its bots' games on a pool of workers, one batch at a time for the whole server. Before this, the only
limits were that: 256 batches in the queue and 5,000 games a batch. A spec could ask for any number of `workers`,
any `mcts_iterations` or `minimax_nodes`, any `max_plies` and `event_games`, a name of a megabyte, and nothing ended a
batch that would take a week; one player could fill the queue of 256.

The maintainer was away when this was settled, so each of the issue's seven questions took its suggestion, and what
the issue left to the plan (the numbers, and what happens at a limit) was decided as below. Any of it may be reversed.

## Decision

The seven questions, in order:

1. **What is limited.** For one batch, *ceilings*: `workers`, each bot's search (`mcts_iterations`, `minimax_nodes`), a
   game's length (`max_plies`), the batch's `games`, the games that keep decisions (`event_games`), and its wall-clock
   time (`minutes`); and the length of its name. A spec over a ceiling is refused up front. The caps that existed are
   kept (5,000 games, 256 queued) and made settings; the missing ones are the rest. *Time a move* is not a field of a
   spec (a bot without `mcts_iterations` searches for a second; with them it stops at them), so it is bounded by the
   search ceilings and, whatever a move costs, by the batch's time limit, which every bot honours at once through its
   context.
2. **A rate as well as a ceiling: not built.** A batch is not slowed. The simplest of the three that are honest to the
   player is a refusal up front for what is known when the spec arrives, and an end, marked with the limit, for what
   is known only while it runs (time). A limiter that slowed a batch would need a clock in the worker pool, would make
   `games_per_minute` and the time limit mean less, and would hold the one batch slot (and the ladder behind it) for
   longer to do the same work; what slowing is for (leaving CPU to others) is #316's. This is a choice the maintainer
   may reverse. *At a limit*: a spec over a ceiling is `400` with the limit in the answer; a full queue or a spent
   allowance is `429`; a batch that reaches its time limit is stopped, keeps the games it played and is marked.
3. **Each player, as well as each batch.** Three allowances per principal (the account, or the guest the request acts
   for; requests with none share one): batches *queued or running* (16), batches *kept* (100) and *games started in 24
   hours* (20,000). The games of a batch count in full while it is queued or running and as the games it played after,
   so a batch cancelled before it ran costs nothing. They are counted at the start, not while the batch runs: what a
   player may use is decided once, when they ask. An administrator's own session is not held to them. A batch
   remembers its owner (`batches.owner`; those made before this belong to nobody's allowance). The server's queue
   (256) stays whole-server and is refused up front now, with no row left behind (it used to leave a failed batch).
4. **Who sets the limits.** Defaults in the server (`batch.DefaultLimits`), an environment variable for each
   (`BINGLE_BATCH_*`, [build.md](../build.md)), and an administrator able to raise them for one batch: a spec's
   `limits` object holds the ceilings to use in place of the server's, and only a request that is an administrator's
   own session may send it. An administrator's API token has none of the group's powers (ADR 0071) and is refused, as
   is everybody else (`403`), through REST and MCP alike. The check is on the principal in the request's context, so
   both doors share it.
5. **The other bot jobs.** What each already limits, and what is left:
   - *The bot ladder* (ADR 0160) is a batch's kind of work and comes under the same settings where they apply: its
     workers are the batches' default within the workers ceiling and its game cap (300 plies) is kept within the plies
     ceiling. Its search is the strength players meet, not a spec's, so the search ceilings do not apply to it; its
     time limit, games a pairing and schedule are its own settings (ADR 0160); a player's allowances do not apply to a
     server's own scheduled run.
   - *Matches between bots* (`POST /api/bot-matches`) are rooms people watch: 12 at once, the creation rate limit of
     `create_game` and the bot pool. *`match_bots` and the training environments*: `matchSlots` (the CPUs), a time budget
     and a cap on recorded games, and 8 environments stepped at once, 128 steps a call. *Tuning* is an offline command.
     These are out of this issue: they are interactive or bounded by their own caps, and bringing them under the same
     settings would move them without a number to move them to. They are #316's to order.
6. **What the player is told.** The `400`, `403` and `429` answers carry `{error, limit, max}`, `limit` naming the
   ceiling or allowance; MCP's `start_batch` returns the same in an error result's structured content, with `status`.
   A batch ended at its time limit is `cancelled` with `limit: "time"` and shows "Time limit reached" in place of its
   status, in the Analysis tab and on its page. It is not a new status: a client that does not know the field still
   sees a stopped batch. The Analysis form's number of games follows the games ceiling.
7. **The API.** `version.API` 1.28.0 (minor): `ErrorResponse` gains `limit` and `max`, `GET /api/batches` gains
   `limits`, a batch gains `limit` and a spec `limits`. Only `ErrorResponse` is in `docs/api.schema.json`: batches are
   not in the schema (they are not among the index's endpoints), and putting them there needs `batch.Spec`'s own JSON
   form (it reads specs saved with `high_priests`) written out by hand; left for a change of its own. They are in
   [api.md](../api.md).

### The numbers

Each is a ceiling a little above what a batch a person would start today uses, measured on the machine of ADR 0160
(16 CPUs, 12 workers by default), so that nothing started now is refused or cut short.

| Limit | Default | Why |
|---|---|---|
| `workers` | the CPUs | the default stays three quarters of them; a spec may ask for all, no more than there are |
| `mcts_iterations` | 5,000 | the default MCTS searches a second, which is about 440 iterations on Hex 11×11's opening (200 iterations 0.52 s, 2,000 4.6 s, 20,000 61 s a move): 5,000 is about 11 seconds a move there, 11 times the default |
| `minimax_nodes` | 100,000 | the default is 30,000 (0.17 s a move on Hex 11×11); 300,000 took 1.6 s a move and allocated 3.1 GB in the move |
| `max_plies` | 5,000 | a spec's default is 3,000 and games of the shipped games end far sooner (Hex 11×11 about 100 plies, Chess about 350 at random) |
| `games` | 5,000 | what it already was, and what the Analysis form offers |
| `event_games` | 100 | the Analysis tab opens decisions for the first 20; a kept game is about 50 bytes a ply (47 measured), so 100 games of 5,000 plies is about 23 MB of database |
| `minutes` | 720 | on 12 workers Hex 11×11 tactical against tactical plays 24 games a minute and Chess 12, so 5,000 games take about 3.5 and 7 hours; 100 games, the form's default, take under 5 minutes, and on one worker (a server of two CPUs) about 50 minutes. 5,000 games of Hex between two MCTS bots at a second a move take about 12 hours on 12 workers and nearly a week on one: a batch this ends on a smaller server |
| queue | 256 | what it already was |
| queued per player | 16 | a person runs a handful; sixteen of them fill the 256 |
| kept per player | 100 | the list shows the newest 200 |
| games a day per player | 20,000 | four batches of the most games: reached only on purpose |

### Memory

Go has no memory limit for a goroutine, so none is enforced for a batch, and this says what bounds it rather than
claiming a limit. A running batch holds, per worker, a game's position and a bot's search; the search is what grows.
Measured as peak heap (garbage not yet collected included): MCTS at 200 iterations 33 MB, at 2,000 107 MB and at
20,000 415 MB for one worker; 12 workers at 2,000 iterations 1.46 GB, and a search of Minimax at 30,000 nodes on
Chess 1.46 GB for 12. So **a batch's memory is about its workers times its bots' search, which is why both are
ceilings**: at the defaults, 16 workers at 5,000 iterations is of the order of 3 GB. It also keeps in memory the
decisions of the games being played (at most `event_games`' worth, 50 bytes a ply) and, when a report is read, every
stored game's row (at most `games`), and the finished batches' reports (the newest 32 are cached, which used to be all
of them). The records themselves go to the database as each game ends. **Not limited**: the heap of the whole process
(an operator who needs a bound sets `GOMEMLIMIT`, which makes the collector work harder rather than refuse a batch),
the size of what a game's study measures for each game (a study's metrics were not measured here: they are public for
plain games, empty), and the CPU a batch takes while it plays, which is #316's.

### The database

A batch is one row and a row a game, each written as the game ends in its own transaction. Rows are bounded by
`games` times the batches a player keeps, events by `event_games` times `max_plies`, and the name by 200 characters. The
rate of writes is not limited (a batch of cheap games finishes over a thousand games a second, each its own write through the one writer); who
goes first there is #316's question 4.

## Consequences

- No spec that a person would start today is refused or cut short; a spec that was never reasonable (a megabyte name,
  a thousand workers, a week of MCTS) now is, with the limit in the answer.
- A batch that reaches its time limit is stopped by cancelling its context, so a game in flight is dropped, not
  recorded half played; the games finished are kept and the report counts them.
- `MCP start_batch` is not gated by `run_batch` (the capability is enforced by route, and `/mcp` needs `play`): any
  principal that may play may queue a batch there, within the allowances above. The limits hold either way; the
  capability's coverage is a separate question.
- The shared interfaces that changed: `store.Store.CreateBatch` takes the owner, `batch.NewManager` takes options (a
  call without them is unchanged), `botladder.Options` has two new fields and `auth.Principal` an `IsAdmin` method.
- A server whose operator sets none of the settings behaves as before, with the limits above.
