# Bingle — Roadmap & Progress Log

The v1 backlog (`docs/history/backlog.md`) shipped a working but minimal platform: one
demo game, single-occupant `from → to` moves, and a UI that dumped raw JSON.
This document tracks the **v2** push, loosely modelled on what
[play.abstractplay.com](https://play.abstractplay.com) offers for playing
abstract games, minus its social layer (no friends, chat, leaderboards).

Priorities (in order): a much better UI, support for more complex abstract
games (Tak with stacking as the reference), turn tracking, undo, and MCP
support for training/evaluating models.

Legend: ✅ done · 🟡 partial · ⬜ not started · ➖ deliberately out of scope

## Engine & game definition (DSL)

| | Item | Notes |
|---|---|---|
| ✅ | Stacks (several pieces per square) | `state.BoardState` holds a bottom-to-top stack per square |
| ✅ | Reserves / pools ("pieces in hand") | `reserves` + per-piece `pool`; Tak's flats and walls share the `stone` pool |
| ✅ | Placement action | `place`: source reserve/supply, region restriction, gravity (Connect-4 style) |
| ✅ | Stack-move action | `move-stack`: carry limit, straight line, ≥1 drop per square, bottom-first drops |
| ✅ | Piece stacking attributes | `coverable`, `flattenable_by`, `flatten_to`; flatten conditions per action |
| ✅ | Opening phase | `turn_order.opening`: restrict actions/pieces; place the *opponent's* piece (Tak swap) |
| ✅ | Declarative win rules | `connect` (region to region), `line` (n in a row), `end` (board full / reserve empty / no moves; decide: most / draw / mover wins) |
| ✅ | Mover wins ties | If one move completes roads for both players the mover wins |
| ✅ | Variants via JSON merge patch | `variants`: Tak 3×3 … 8×8 from one document |
| ✅ | Move notation | generic + PTN (Tak); TPS positions; `FormatMove`/`ParseMove` round-trip tested |
| ✅ | Legal-move enumeration | `state.LegalMoves`, validated by the same code as `Apply`; perft matches published Tak counts |
| ✅ | Board regions, coordinates, algebraic labels | grid generator `labels: "algebraic"`, auto `left/right/top/bottom` regions |
| ✅ | Hex boards | `generator: hex` (axial rhombus), hexagon rendering, per-seat connect rules, edge tints; Hex 7/9/11/13 |
| ✅ | Capture by jumping | `jump` action: mandatory captures, chained multi-jumps, promotion; English Draughts in JSON |
| ✅ | Directional movement & capture | per-seat `forward`, `directions`, explicit `step` actions with occupancy; Breakthrough in JSON |
| ✅ | `reach` and `ply-limit` rules | far-row wins, move-limit draws |
| ✅ | Flipping & passing | `place` + `flips`, `pass_when_stuck`; Reversi in JSON |
| ✅ | Liberty capture, ko, passing, area scoring | `captures` + `ko` on `place`, `pass` action, `passes` trigger, `score` decision; Go 9/13/19 in JSON |
| ✅ | Hexagon boards, ring / fork / bridge, "lose" rules | `generator: hexhex`; `ring`, `connect` with `of`/`touch`, `result: lose`; Yavalath and Havannah in JSON |
| ✅ | Pie rule | `swap` action (second player may take over the first move); Hex and Gomoku |
| ✅ | Slides and compound moves | `slide` action (queen/rook lines) with a `then: shoot` follow-up and `aim` in moves; Amazons in JSON |
| ✅ | Custodian captures and capture counters | `custodian` captures on placement and slides (with `anvils`), `pool` and `absent` win rules, `forbid_region`; Pente and Brandubh (an asymmetric hunt game) in JSON |
| ✅ | Clone-or-jump leaps with infection | `leap` action; Ataxx in JSON |
| ✅ | Courtfall: a full tactical skirmish | 37-hex arena, terrain draft, five Courts, secret dice splits, Momentum, Clear Turns; ~2,000 lines of Go + a JSON legend ([courtfall.md](courtfall.md)) |
| ✅ | Cthulhu Wars: four cults, an area map, dice battles | core set with its four factions on a simplified 17-area world; ~4,000 lines of Go + JSON map + Markdown guide ([cthulhu-wars.md](cthulhu-wars.md)) |
| ✅ | Cthulhu Wars: Windwalker, Sleeper, High Priests and five seats | six cults at a table of three to five, the High Priests as an option; tables are *derived games* named by their choices (ADR 0028) |
| ✅ | Area maps, many seats and tabbed rules guides | `board.style: "areas"` (polygons, tokens, clickable units), three-or-more-seat tables with bots for open seats, Markdown guides with tabs/tables/icons/search, generated for every other game (ADR 0026) |
| ✅ | Fast integer-indexed win evaluation | `board.Graph` index tables; `boardView`; 40 µs per move on the 169-cell Havannah board |
| ✅ | Hidden information, randomness, per-unit state | `engine: "ruleset"`: a Go `state.Ruleset` with seeded dice, public/private views, determinized search and a no-undo-across-chance guard (ADR 0025); Courtfall |

## Persistence, sessions & API

| | Item | Notes |
|---|---|---|
| ✅ | Full position document | `state` (v2: cells/stacks, reserves, ply); legacy flat `board_state` kept |
| ✅ | Move notation + payload stored | `moves.notation`, `moves.move_json`; additive column migrations |
| ✅ | Undo | takes back your last move (and bot replies); blocked once another human moved |
| ✅ | Take-back with consent | `POST …/takeback`: request / accept / decline once a human has replied |
| ✅ | Custom start positions | `start_tps` / `start_state` on create; remembered for replay, undo and export |
| ✅ | Resign | ends the game with `end_reason = resignation` |
| ✅ | Draw offers | offer / cancel / accept / decline; any move withdraws the offer |
| ✅ | Per-move time limits | `move_time_seconds`; loser on time; lazy server enforcement |
| ✅ | Game clock (chess clock) | `clock_seconds` (one time per seat, handicaps allowed); the mover's own store runs down; forfeit on the flag |
| ✅ | Replay at any point | `GET …/state?at_seq=N` (0 = starting position) |
| ✅ | Simulate / analysis | `POST …/simulate` applies notation to any point without changing the game |
| ✅ | Export | JSON game record + PTN document |
| ✅ | Analysis tab | batches of bot games for any game, win rate per seat and per bot with intervals, and a guide to the bots |
| ✅ | Built-in bots | random, tactical (1-ply), minimax (alpha-beta, node-budgeted, deterministic), MCTS; play first or second |
| ✅ | WebSocket events | `move`, `state` (join / undo / resign) with full position |
| ✅ | SQLite robustness | WAL + busy timeout + immediate write transactions (fixed `SQLITE_BUSY` under concurrent tabs) |
| ➖ | Accounts, auth, rating | out of scope per brief |

## MCP (agents & training)

| | Item | Notes |
|---|---|---|
| ✅ | Rules & notation for LLMs | `get_game_info` |
| ✅ | Decision-ready observation | `get_observation`: text board, TPS, legal moves, reward, done |
| ✅ | Whole-position legal moves | `get_legal_moves` (no starting square needed) |
| ✅ | Notation / structured moves | `make_move` accepts `notation` or action/piece/from/to/count/path/drops |
| ✅ | Undo / resign / simulate / export | `undo_move`, `resign`, `simulate`, `export_game` |
| ✅ | Gym-style environments | `env_reset`, `env_step`, `env_observe`, `env_fork`, `env_close`, `env_list`; in-memory, seedable, self-play, TPS start |
| ✅ | Opponent bots in rooms and envs | `opponent: random \| tactical \| minimax \| mcts` |
| ✅ | Bot-vs-bot matches | `match_bots`: tallies and move records for baselines / datasets |
| ✅ | Model-ready observations | `planes` (viewer-relative tensor) and fixed `action_ids` via `env_reset include`; `env_spec` describes shapes and the action space |
| ✅ | Proven puzzles | `list_puzzles`, `env_reset puzzle_id`; exhaustive "win in N" solver (`internal/puzzles`) |
| ✅ | Environments over plain HTTP | `/api/envs`, `/api/env-specs`, `/api/matches`; stdlib-only Python Gym-style client `scripts/bingle_env.py` |
| ✅ | Vectorised stepping | `env_batch_step`: up to 128 environments per call; `env_step` also takes `action_id` |

## Web UI

| | Item | Notes |
|---|---|---|
| ✅ | Real board rendering | SVG, squares/points/graph styles, coordinates, stacks drawn as piles with height badges |
| ✅ | Click-to-move for any game | driven by the server's legal-move list (`steps`), so no per-game UI code |
| ✅ | Move preview + confirm | `simulate`-backed ghost position; optional auto-submit |
| ✅ | Hand/reserve panels, counters | per-seat pools; piece buttons for placement; capture counters |
| ✅ | Turn tracking | status bar, to-move highlighting, move counter, title marker, browser notifications |
| ✅ | Move list + history navigation | click any move; ⏮ ◀ ▶ ⏭ and arrow keys; read-only past positions |
| ✅ | Undo / resign / flip / rules / export | two-step confirmation; rules dialog with pieces legend and notation help |
| ✅ | Lobby | game picker grouped by family, bot opponents, play first/second, "Your games" with your-turn flags |
| ✅ | Live updates & reconnect | WebSocket with backoff; opponents joining/undoing show immediately |
| ✅ | Light/dark theme, responsive layout | follows OS preference |
| ✅ | Analysis mode | scratch variations from the live or any past position, backed by `simulate` |
| ✅ | Rematch | same bot, other side of the board; "New game" link for human opponents |
| ✅ | Per-move time limits | optional; live countdown |
| ✅ | Display settings | coordinates, legal-square hints, last-move highlight, animations, board themes |
| ✅ | Keyboard and accessibility | roving tab stop with arrow-key navigation, named squares, reduced-motion aware |
| ✅ | Hover ghost piece, SVG image export, TPS copy | placement preview; downloadable board image; Tak position string |
| ✅ | Stack inspector, carried-piece display | hover a square to see a stack bottom-to-top; carried pieces shown while moving |
| ✅ | Replay and deep links | auto-play replay; `#move=N` links to a position |
| ✅ | Import games | PTN / move lists into review rooms (`POST /games/import`, `import_game`, lobby card) |
| ✅ | Watch bots play | `POST /bot-matches`; paced bot-vs-bot rooms with live spectators |
| ✅ | Hints | `POST …/hint`, `suggest_move`: minimax or MCTS suggestion (win rate for MCTS), never against a human |
| ✅ | Play from here | start a new game (bot or friend) from any analysis position |
| ✅ | Puzzles | lobby card; positions with a proven forced win |
| ➖ | Chat, friends, leaderboards, ratings | out of scope |

## Quality

- Engine: move enumeration is cross-checked against brute force (every
  conceivable stack move/placement must be accepted by `Apply` iff it is
  listed — ~100k candidates over random positions); ~4 ms per random 5×5 Tak
  game (≈40 µs/ply) so bots and training get useful throughput.
- Engine: unit tests per rule, perft check against published Tak node counts
  (25 / 600 / 43 320 / 2 999 784), randomized consistency test that cross-checks
  `LegalMoves` against `Apply` and mutated moves, notation/TPS round trips.
- Engine hardening: native fuzz targets (parsers, structured moves against
  every game, the definition compiler), a cross-game invariant battery
  (`tests/allgames`) and a brute-force enumeration check for every compound
  action (Tak stacks, Amazons slides, Ataxx leaps).
- Puzzles are re-proven by the exhaustive solver on every test run.
- API/MCP/browser integration tests under `tests/app` (≈90 s with headless
  Chrome); a Python client test exercises the HTTP environments.
- `make test`, `make lint` clean; `go test -race` on engine, session, store,
  bot and API packages.
