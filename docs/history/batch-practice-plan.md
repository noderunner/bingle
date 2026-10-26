# Bot practice batches, metrics and reports (Cthulhu Wars) — plan

Handoff document: a fresh Claude session should be able to continue from here.
Status log is at the bottom — update it as stages complete. No git commits unless
the user asks (repo is untracked). Never kill/restart the user's running
`./bin/bingle` on :8080; build `bin/bingle` and tell them.

## Goal

The user wants to tune the Cthulhu Wars bots (existing kinds: random / tactical /
minimax / mcts, plus the `state.BotGuide` scored move filter in
`internal/games/cthulhuwars/guide.go`) so they play harder and differently per
faction. Win/loss alone is a noisy signal (3–5 seats, dice), so first collect
rich per-game metrics over hundreds of simulated bot games, look at them in a
report UI, then tune guide weights / bot knobs against them (that tuning is a
LATER phase, "option 1" in the discussion; this plan covers metrics + batch
runner + wizard + reports). Longer-term idea (not in scope now): imitation +
self-play RL using the same records (planes are not available for areas maps,
so a feature encoder would be needed).

## Decisions already made with the user

* NOT Prometheus/OTel for this: batch, in-process, per-game questions. Use one
  structured record per game (+ per-ply events for a sample), stored in SQLite
  (`bingle.db`, new tables), aggregated by the server for the reports.
  Optional LATER: a low-cardinality `/metrics` endpoint (labels: faction, action
  kind only) for the live server. Not now.
* The engine stays pure/deterministic: metrics come from an OBSERVER in the
  match runner that diffs state before/after each move. No hooks inside the
  engine.
* Rotate faction-to-seat assignments across games (first-seat bias would
  pollute results). Seeded and reproducible: a batch = parameters + base seed.
* Worker pool ≈ 12 of the 16 cores so the live server stays responsive.
* Full per-ply event data only for the first 20 games of a batch; summary
  record for every game (bounds DB size).
* Charts: inline SVG, hand-written in the existing plain-JS UI style (no new
  dependencies). Machine: 16 cores, 31 GB RAM, RTX 2080 Ti (not needed here).
* Measure games/minute EARLY and show an estimate in the wizard (MCTS at 1.5 s
  per move would make hundreds of games slow; tactical/low-depth minimax fast).

## Stages (each usable on its own)

### Stage 1 — observer, records, batch runner (server)

* Look first at `internal/envs/match.go` (`match_bots`), `internal/bots`,
  `internal/session` (bot choices, `BotGuide`), `internal/games/cthulhuwars`
  (`Data`, `Data.Log`, `view.go`), `docs/mcp.md`, ADR 0019.
* Observer: after each applied move, diff before/after and record, per seat
  (faction/cult key `gc cc bg ys ww sl`):
  - action kind taken (move, battle, build gate, summon, awaken, ritual,
    capture, recruit, spellbook, ability ids…) — counts and ply of each
  - gates: built, lost, held per round, CONTROLLED (Round 7 model: a gate is
    controlled by its occupant, `Data.Holders`; abandoned gates are not)
  - **(addendum 1) units built/summoned/awakened, by unit type** (cultists,
    monsters, terrors, Great Old Ones), and units lost by type
  - **(addendum 1) abilities used**: faction spellbook abilities, per ability
    id (Devolve, Madness, Necrophagy, Submerge, Hibernate, Oath, Ice Age, …),
    counted when used (and unlock ply for each spellbook)
  - **(addendum 1) board areas controlled**: per area name (e.g. "North
    Atlantic", "West Africa"; use the ids/names in `cthulhuwars.json`), per
    seat: rounds in which the seat held a gate there / had units there /
    controlled it (define: owns the gate there, or has the only units).
    Sample at the end of each round (Doom phase) AND at game end. Reports must
    correlate area control (and unit/ability emphasis) with winning.
  - doom, power, Elder Signs, rituals performed, kills/pains in battle,
    battles won/lost, book unlock ply (each book), awakened GOOs and when
  - game result: winner seat/faction, end reason (ritual/Instant Death/
    timeout/ply cap), ply count, rounds
* Storage: new SQLite tables in `internal/store` (migrate in place, like other
  tables): `batches` (id, game_def_id, derived options, params JSON, base seed,
  n_games, status, created/finished, progress), `batch_games` (batch_id, index,
  seed, seating JSON, winner, end_reason, plies, summary JSON), and
  `batch_events` (batch_id, game index, ply, seat, action, details; first 20
  games only). Summary JSON holds the per-seat metrics above including area
  tables.
* Batch runner: background job manager (start, progress, cancel, resume-safe
  status after restart = mark `interrupted`), worker pool ~12 cores, per-game
  seed = base+index, seat rotation (a Latin-square style permutation of
  factions over seats), bot spec per seat (kind + knobs: `bot_iterations`,
  think time, later guide-weight preset), ply cap, `include_events` for the
  first 20.
* API (documented in `docs/api.md`): `POST /api/batches`, `GET /api/batches`,
  `GET /api/batches/{id}` (status + aggregated report data),
  `GET /api/batches/{id}/games`, `…/events/{n}`, `POST …/cancel`,
  `DELETE`. Also expose through MCP if cheap (`start_batch`, `get_batch`).
* Tests in `tests/` (same layout as the existing ones), including determinism
  (same seed ⇒ same records) and observer correctness on hand-built positions.
* Measure games/minute for tactical, minimax, mcts and write the numbers into
  the status log below (needed for the wizard's estimate).

### Stage 2 — "Practice batch" wizard (home page)

* Shown on the home page when Cthulhu Wars is selected, beside the normal game
  setup (see `internal/web` home.js: `renderSlots`, `slotMemory`,
  `factionMemory`, `#option-high-priests`; keep the arrangement-memory pitfalls
  in mind — see memory note project_cthulhu_wars).
* Steps: (1) table: layout/seat count, factions per seat, High Priests on/off;
  (2) bots per seat: kind + knobs (MCTS iterations, think time; later guide
  presets); (3) run: number of games, seed, rotate seats (default on), ply cap;
  summary with estimated runtime from the measured games/min; Start → goes to
  the batch's report page with live progress.
* Never give wizard controls a `data-seat` attribute on non-bot-kind selects
  (an old test selects `[data-seat]`).

### Stage 3 — report UI

* Batch list + batch detail page (+ compare view for two batches: did the
  tuning help?). Inline SVG charts, light/dark friendly like the rest of the UI.
* Charts: win rate per faction with confidence intervals (Wilson); doom over
  time (per round, per faction); how games end; gates held/controlled per
  round; spellbook unlock timing; game length distribution; battles won/lost;
  **(addendum 1) units built by type per faction, abilities used per faction,
  and area-control heatmap/table: per area, per faction, share of rounds
  controlled, with win-rate correlation (win rate when controlling the area vs
  not, with n and CI)** — plus the same lift view for units/abilities
  ("games where faction X built ≥k of unit U won p% vs q%").
* Drill into a game (events list for the 20 sampled games).

## Later (not now)

* Option 1 proper: tuning harness — hill-climb/evolve guide weights per
  faction and seat count against the reports' fitness (win rate + small
  bonus for doom margin), then raise MCTS iterations.
* Optional `/metrics` endpoint; imitation/RL (needs a feature encoder).

## Verify (as in earlier rounds)

`go test ./tests/cthulhuwars`, `go test ./tests/app -run '…'`, gofmt, go vet,
golangci-lint, `go test ./...`; build with `make build`. Add docs (this file,
`docs/api.md`, `docs/cthulhu-wars.md`, an ADR in `docs/decisions/`) and update
memory notes.

## Status log

* 2026-10-16 plan written.
* 2026-10-16 Stage 1 part 1 DONE: `internal/games/cthulhuwars/metrics.go` (Observer: Observe(before, after, seat, mv) / Finish → GameMetrics with per-seat Actions, Built/Lost units by type, gates built/lost, book events, per-area Gate/Presence/Control counts sampled at each round end + game end, RoundSample series, battle stats, final standing); `internal/batch/run.go` (Spec/Player/Record, Validate, Run worker pool, PlayGame; derived table id = base~keys[-hp], seats rotated cyclically per game); test `tests/batch/batch_test.go` (determinism + records + throughput log).
* TODO next: persistence (store tables + job manager with progress/cancel/interrupted), HTTP API + MCP, aggregation for reports, then Stage 2 (wizard) and Stage 3 (reports).
* 2026-10-16 Stage 1 part 2 DONE (code compiles, API next): store tables `batches`/`batch_games` + `internal/store/batch.go`; `internal/batch/manager.go` (queue, one batch at a time, cancel/delete, interrupted on restart, cached reports), `internal/batch/report.go` (BuildReport: per-faction win rate+Wilson CI, by seat, doom/power/gates by round, book timing, units built (+won/lost split), actions, areas control share, each with a mean-split win-rate `Lift`).
* MEASURED THROUGHPUT (4 workers, 4-seat `cthulhu-wars`, ~500 plies/game): tactical 28 games/min, minimax 25 games/min, mcts@20 iterations 2.6 games/min (10x slower). Expect ~3x more with 12 workers. Use these in the wizard's estimate (scale by workers/4; mcts roughly proportional to iterations/20 — re-measure).
* 2026-10-16 Stage 1 DONE: REST (`internal/api/batches.go`: POST/GET /api/batches, /{id}, /{id}/report, /{id}/games, /{id}/games/{n}/events, POST /{id}/cancel, DELETE /{id}) + MCP tools (`internal/mcpserver/tools_batch.go`: start_batch, get_batch, list_batches, cancel_batch); wired in `internal/app/app.go`; test `tests/app/batches_rest_test.go` passes. NOT yet: docs/api.md + ADR + docs for batch API (do at the end of Stage 3).
* NEXT: Stage 2 wizard on the home page (internal/web static home.js + templates), then Stage 3 report pages (new routes in internal/web router: /batches, /batches/{id}).
* 2026-10-16 Stages 2+3 DONE (first version): wizard (`internal/web/static/js/practice.js`, card `#practice-card` in `templates/index.html.tmpl`, shown by `syncPractice` from home.js `updateBlurb`; steps Table → Bots → Run, estimate from measured speeds), report pages (`/batches`, `/batches/{id}` via `templates/batches.html.tmpl`, `js/batches.js`, `js/charts.js`, `js/cw-names.js`; CSS appended to style.css): win rate + CI, game length, doom/gates/units/power by round, final standing, "What goes with winning?" lift tables (units, spellbooks, actions, areas) per cult, area heat table, spellbook timing, battles, games list with moves of the first games, compare-with another batch (win rates). Tests: `tests/app/batches_rest_test.go`, `tests/app/batches_ui_test.go` (set BINGLE_SHOTS=dir for screenshots), `tests/batch`. Docs: ADR 0034, docs/api.md section. The :8080 server was NOT restarted: tell the user to restart it (`make build` then restart their own process) to see the feature.
* IDEAS NOT DONE: compare view beyond win rates (deltas of doom/units); per-area lift on a map picture; a wizard control for guide-weight presets; tuning harness (hill-climb `BotGuide` weights per faction against reports; read `internal/games/cthulhuwars/guide.go`); optional `/metrics`; imitation/RL encoder.
* 2026-10-16 TUNING HARNESS built (needs a real run; the smoke run only proves the mechanics): `internal/games/cthulhuwars/tuning.go` (`GuideTuning` = 14 additive offsets to the guide's scores — gate, recruit, awaken, capture, summon_urgent/watch/idle, opening/guard/hunt move, battle, end_turn, reserve, slack; `GuideKnobs` ranges; `ShippedTuning(cult)` reads embedded `tuned.json` (currently `{}`), applied to every bot of the cult through `GuideMoves`; `GuideMovesTuned` for explicit tunings), `guide.go` wired (zero tuning = old behaviour, test `TestZeroTuningChangesNothing`), `batch.Player.Tuning`, `internal/tune/tune.go` ((1+λ) evolution strategy: parent + mutations (≈3 knobs each) on the same seeded games, best must beat the parent again on fresh confirm games, sigma adapts, final validation vs shipped tuning with z-score; fitness = win rate + 0.25*doom/30), CLI `go run scripts/tune/main.go -faction gc -generations 10 -pop 6 -games 120 -out FILE [-resume] [-ship]` (`-ship` merges into tuned.json — only if validation z ≥ 1.64). Limits: tunes the guide only, so it matters for tactical bots (MCTS/minimax playouts use the shipped tuning); tunings are per cult, not per seat count; others at the table play shipped tunings (co-evolve by shipping one cult at a time and re-tuning the rest). Cost: ~(pop+1)*games + 2*confirm games per generation (≈80 games/min with tactical on 12 workers).

### Status: first tuning run shipped (2026-10-16)
- `gc` tuned (8 gens, pop 6, 150 games, validate 600): `{"recruit":25.5,"awaken":-5,"capture":-12.5,"summon_watch":8.5,"summon_idle":4.5,"hunt_move":7,"battle":19.5,"end_turn":19}`. Validation vs previous shipped: 78.3% vs 24.2% wins, z=+18.8 (doom about equal). Now in `tuned.json`.
- Guide unit tests (`tests/cthulhuwars/guide_test.go`) use an explicit zero tuning so they do not depend on shipped numbers.
- Caveat: wins up with doom flat is a big jump; look at a few tuned games for a degenerate trick (e.g. end_turn +19) before trusting it. Next: tune cc/bg/ys/sl/ww one at a time against the shipped set.

### Status: spellbook requirements (2026-10-16)
- Why: ~28% (tactical) / 42% (MCTS) of batch games ended "humanity" (nobody held 6 books). YS (2.9 books) and BG (2.9) could not finish; the guide had no notion of requirements.
- Metrics: `SeatMetrics.Reqs` (`ReqEvent{req,ply,round}`), `cthulhuwars.Requirements(key)`, report `FactionReport.Reqs` (`ReqAvg`, every requirement, met or not), UI card "Spellbook requirements" in `batches.js` (rows under 50% highlighted).
- Guide: `guide_req.go`, `score()` = `scoreBase()` + `reqBonus()`; knob `req` (reqUnit 12 + offset) in `GuideTuning`. Covers gc, cc, bg, ys (ww/sl not yet).
- Measured (320 tactical games, gc already tuned): humanity 34/320 = 11% (was 28%); BG 3.8 books, YS 3.5 books. Still bottlenecks: BG "units in 8 areas" (3%), "6 areas" (42%); YS desecrate glyphs (Dragon 35%, Chevron 43%, Thorns 14%).
- Existing gc tuning in tuned.json was found before this bonus existed: re-validate/re-tune with `req` in the knob set.
- Next: tune bg and ys (req knob matters most), ww/sl requirement bonuses.

### Status: bg and ys tuned and shipped (2026-10-17)
- Tuner gained `BookWeight` / `-book-weight` (fitness += w*books/6) and prints books in validation. Runs used 0.4, 8 gens, pop 6, 150 games, 600 validation games.
- bg: wins 0.5% -> 5.2%, books 3.86 -> 4.55 (z=+4.9). ys: wins 0.2% -> 2.8%, books 3.63 -> 4.57 (z=+3.8). Both in tuned.json. Neither search moved the `req` knob.
- Still far from competitive: both are far below gc (~78%). Next: re-tune gc/cc against the new line-up, give bg/ys more generations, ww/sl req bonuses, structural help for the BG 8-area and YS glyph requirements.

### Status: YS/BG guide behaviours, ys retuned, phone layout (2026-10-17)
- Guide gained structural requirement behaviours for ys (desecrate, gifts, King/Scream/Shriek, walk to the King) and bg (sacrifice, spread to new areas, share new enemy); see `guide_req.go`. The older bg/ys tunings predate them (humanity wins rose to ~23% in a check).
- ys, 12 gens: wins 2.7% -> 8.3%, books 4.46 -> 4.62 (z=+4.3, 600 fresh games). Shipped in tuned.json.
- bg 12-gen run crashed with a Go runtime SIGSEGV in malloc at gen 10 (not engine code); rerun as `tune3-bg`. bg tuning in tuned.json is still the old one.
- Phone layout for CW shipped separately (ADR 0035).
