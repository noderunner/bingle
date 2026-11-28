# 0162. Games with people go first: one CPU budget, a reserve background work never takes, and batch records written together

Status: accepted (2026-11-28), for issue #316. The sister of [ADR 0161](0161-limits-on-batch-jobs.md), which limits
how much a batch may take; this one settles who goes first when a batch and a real game want the same CPU, or the same
database writer. Builds on the practice batches (ADR 0034), the bot ladder ([ADR 0160](0160-a-bot-ladder.md)) and the
pools' metrics ([ADR 0064](0064-metrics-on-a-second-listener.md)).

## Context

Work that costs CPU ran in pools each sized on its own against the CPUs: a batch's workers (three quarters of them by
default, all of them at the workers ceiling), live bots' replies (`botSlots`, twice the CPUs), hints (`hintSlots`, the
CPUs), `match_bots` (`matchSlots`, the CPUs), the training environments (8 at once) and a WebAssembly ruleset's
instances (twice the CPUs). The pools knew nothing of each other, and Go gives no goroutine priority over another.

The maintainer was away when this was settled, so each of the issue's eight questions took its first suggestion, and
where the issue had none the simplest safe answer was taken. Any of it may be reversed.

## Decision

1. **What is live, by the work.** *Live*: everything about a room: a request, a bot's reply there (a bot playing for
   an absent player included), a hint, and the rooms of `POST /api/bot-matches`, which people watch and which stop
   after two minutes unwatched. *Background*: batches, the bot ladder, `match_bots` (`envs.PlayMatch`), the training
   environments' opponents, and tuning (offline, through `batch.Run`). **A script playing a seat through the API or
   MCP is a player**: the server cannot tell it from a person, and it waits for a bot's reply as a person does. There
   is no check of who is seated or watching: a room is live whoever is in it.
2. **One budget, in one file** (`internal/cpu`), of `GOMAXPROCS` slots:
   - *Live work never waits for it.* A live search counts itself while it runs (`Budget.Live`) and keeps its own
     pool's bound (`botSlots`, `hintSlots`) as before.
   - *Background work takes a slot* (`Budget.Background`) from a first-come queue before it searches: never more than
     the size less the reserve, and none while live and background together would pass the size.
   - *It gives way between plies* (`Slot.GiveWay`): when live work has gone past the reserve, or the share has shrunk,
     it hands its slot back and queues again, keeping its game in memory, and plays on when its turn comes; and once
     it has held a slot for `Turn` (100 ms) while another background job waits, it does the same, so background jobs
     take turns. Nothing is stopped in the middle of a search and no game is dropped half played.
   - A slot is taken and given back only in that file, and nothing waits for a slot while holding one, so the budget
     cannot deadlock. A waiter whose context ends leaves the queue; a slot handed to it at that moment is given back
     once. `matchSlots` is replaced by the budget; the WebAssembly instances are unchanged, being memory, and are used
     inside the slot of whatever calls them.
   - **The bounds.** A live job waits for the budget never. The CPUs it shares with background work beyond the
     reserve are handed back within one ply of each background game: a second for MCTS on time, up to ADR 0161's
     search ceilings otherwise. With no live work, background gets everything but the reserve at once; a background
     job waits behind other background jobs for at most about `Turn` plus a ply each ahead of it. Background waits for
     as long as live work fills the machine: that is the point, and a batch's time limit (ADR 0161) counts that wait.
3. **Enough, as a number.** The target was a person's move at the 99th percentile, while a batch runs on every CPU,
   within twice the idle figure or the idle figure and 10 ms. It is met for the database and not for the CPU on the
   machine measured: see *The measurements*.
4. **The database.** Measured, a batch of quick games (Hex 7×7 between random bots, 700 games a second) held a person's
   move up for a second at the 99th percentile, before and after the CPU budget: one transaction a game kept the one
   writer from live play's moves, which wait for it by SQLite's busy timeout. So **a batch's records are written
   together** (`store.AddBatchGames`): in one transaction a quarter of a second after the last write (one timer at most
   waits to make it), or at once when 256 are waiting, and the rest when the batch ends, after which nothing is
   written. No record waits longer than that, so a slow batch's game is written within a quarter of a second of its
   end; the `done` a batch shows players is read from the database, so it is as late. The move's
   99th percentile under that batch fell from 844–1,154 ms to 44–64 ms, and the batch itself, no longer bound by the
   writer, played 240,000 games a minute instead of 42,000. Live writes are not given priority of their own.
5. **Memory.** The reserve is of CPUs only. Memory stays bounded by ADR 0161's ceilings (workers times a bot's
   search); background searches at once are now at most the background share, so a batch of more workers than that no
   longer searches with all of them at once (a waiting game keeps its position, not a search tree). `GOMEMLIMIT` stays
   the operator's.
6. **Small machines.** Background always keeps one slot. On one CPU there is no reserve: background runs on it and gives
   way between plies to any live work. On two, one is kept and one is background's, which is what a batch's default
   workers already were.
7. **The setting.** `BINGLE_LIVE_CPUS` (1 to 1024, [build.md](../build.md)): the CPUs background work never takes. The
   default is what three quarters left over, a quarter rounded up (4 of 16, 1 of 2), and a batch's default workers
   (`batch.DefaultWorkers`) and the ladder's are now the background share, so nothing changes at the default.
8. **Seen.** `bingle_slot_*` gains the pools `live` (live searches running; size the reserve, which they may pass) and
   `background` (slots held; size the share; its wait is every wait for a slot, giving way included). The
   administrator's health page does not show the slots, so it is unchanged and no words were added.

### The measurements

`BINGLE_LOAD=1 go test ./tests/load -run TestPeopleFirst -v` (about 100 seconds; [test.md](../test.md)) plays people's
moves through the session layer in three phases of 30 seconds: four people against the random bot (*move*: the move
itself, whose reply costs nothing) and four against the tactical bot (*reply*: a live search in the answer), each
thinking 50–150 ms between moves, while nothing else runs, while a batch of MCTS bots (150 iterations a move, Hex 11×11)
plays on 16 workers, and while a batch of random bots on Hex 7×7 does. The test file alone builds on a checkout from
before this change, which is how *before* was run. The machine: an 8-core, 16-thread i9-9900K, `GOMAXPROCS` 16, shared
with another agent rendering a 3D client, its load average 7 to 19 throughout. **That made the figures noisy**: the idle
reply's 99th percentile ranged from 102 ms to 821 ms between runs, so a single run says little and only differences
seen in every run are claimed. The 99th percentile in milliseconds, each run (*after* is the budget; the cheap batch's
*after* is with the records grouped, its runs before that matching *before*):

| Phase | Move, before | Move, after | Reply, before | Reply, after |
|---|---|---|---|---|
| idle | 3.5, 3.6, 4.2 | 3.8, 38.8, 4.1 | 104, 102, 127 | 104, 821, 122 |
| searching batch | 21, 17, 22 | 188, 15, 19 | 547, 471, 575 | 1,592, 436, 488 |
| cheap batch | 1,131, 844, 1,154 | 64 | 1,078, 1,367, 1,249 | 512 |

The idle move's 50th percentile was 1.4–1.6 ms and the idle reply's 62–69 ms in every quiet run. The two *after* runs
with a figure several times the others (38.8 idle, 188 searching) are the same runs whose idle reply was also slow,
when the other process was busiest.

With eight more people playing MCTS (live searches past the reserve, so background gives way): searching batch move
40 → 24 ms and reply 681 → 605 ms; cheap batch move 1,069 → 44 ms.

What this shows:

- **The database was the large effect**, and grouping the records removed it.
- **The CPU budget does what it says** (its tests show the reserve is never taken, live never waits, background gives
  way and is not starved), but on this machine it bought little for a live search: a tactical reply was three to five
  times slower under any batch, before and after. The reserve is of logical CPUs, and on a machine of hyperthreads the
  four kept are siblings of cores the batch is using, besides sharing the memory bus and the turbo budget; another
  process was taking several CPUs as well. The reply under the cheap batch got slower *after* only because that batch,
  no longer waiting for the writer, now uses its whole share.
- An operator for whom live searches matter more than batch throughput raises `BINGLE_LIVE_CPUS`. A run at 8 was too
  noisy here to give a figure.

## Consequences

- A person's move no longer waits behind a quick batch's writes; a batch of quick games runs several times faster.
- Background work never takes the CPUs kept for live play, and gives them back within a ply when live play needs more.
  A spec's `workers` above the background share take turns rather than all searching at once; `games_per_minute`
  reflects that.
- `match_bots` and the training environments' opponents now wait behind live play and take turns with a batch, where
  they used to share the CPUs on equal terms; a `match_bots` call that waits past its time budget reports what it
  played, as before.
- Not done: priority for live writes over background ones beyond grouping; a reserve counted in physical cores; a
  pre-emption inside a search (a long background search gives way only at its next ply).
- The shared interfaces that changed: none of their signatures. New: the package `internal/cpu`,
  `store.Store.AddBatchGames`, `metrics.SlotLive`, `metrics.SlotBackground` and `metrics.Metrics.SlotInUse`,
  `config.Config.LiveCPUs`. Changed in behaviour: `batch.PlayGame` (and so `batch.Run` and the ladder) waits for a
  background slot and gives way between plies; `batch.DefaultWorkers` is the background share; `envs.PlayMatch` takes a
  background slot in place of `matchSlots`.
