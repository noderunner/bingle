# 0034 — Practice batches: a metrics observer, stored per-game records, reports

## Context

Tuning the Cthulhu Wars bots on win or lose alone is slow: a 3–5 seat game with
dice gives about one bit per game. We want hundreds of simulated games with
what each cult did (units built, abilities used, Gates, spellbooks, areas held)
so that strategies can be correlated with winning and tuning has a denser
signal. Prometheus/OTel were considered and rejected: the runs are batch,
in-process and the questions are per game, which counters with labels answer
badly.

## Decision

* **Observer, not hooks.** `cthulhuwars.Observer` (metrics.go) reads the
  positions before and after each decision and produces one `GameMetrics` per
  game. The rules never call it, so the engine stays pure and deterministic.
  Units built/lost come from pool deltas, Gates from `Data.Gates`, spellbooks
  from `Data.Books`, battles from the moment a battle's dice are rolled, areas
  from a sample at the end of every round and at the end of the game
  (Gate / presence / control per seat and area).
* **Batch runner** (`internal/batch`): a spec (layout, High Priests, a cult and
  bot per seat, games, seed, rotate, ply cap) plays games on a worker pool
  (default three quarters of the CPUs). Game *i* uses seed+*i* and, with
  `rotate`, seats the line-up shifted by *i* (a derived table id), so the order
  of play does not favour a cult. The same spec replays the same games.
* **Storage:** tables `batches`, `batch_games` (metrics JSON per game); the
  decisions of the first 20 games are kept. A batch is one at a time (queue);
  batches a restart interrupted are marked `interrupted`.
* **Reports** are aggregated in Go (`BuildReport`) and drawn client-side with
  inline SVG: win rate with Wilson intervals, doom/gates/units/power by round,
  length, spellbook timing, and "what goes with winning" tables that split a
  cult's games at its own average of a unit, ability, spellbook or area and
  compare the win rates (z-test flagged; correlation, not proof).
* UI: a wizard card on the lobby page when Cthulhu Wars is selected
  (`practice.js`), `/batches` and `/batches/{id}` (`batches.js`, `charts.js`).
  REST `/api/batches…` and MCP `start_batch`, `get_batch`, `list_batches`,
  `cancel_batch`.

## Consequences

* Measured speed (4 workers, 4 seats, ~500 plies per game): tactical 28
  games/min, minimax 25, MCTS at 20 iterations 2.6. The wizard's estimate is
  built from these.
* The report is computed on demand from the stored metrics (cached once a batch
  has finished); thousands of games are fine, millions are not.
* The tuning harness (hill-climbing the bot guide's weights against these
  reports) is not built yet; it will consume the same records.
