# Bingle for agents: MCP tools and training environments

Bingle's MCP server (`/mcp`, Streamable HTTP) is generic: the same tools play
every game. Connect with any MCP client, read the game's rules with
`get_game_info`, then either play in a persistent room or train in an
in-memory environment.

## Resources and prompts

Besides tools, the server offers MCP **resources** — `bingle://games` (the
catalog as JSON), `bingle://puzzles` (the proven puzzles, without solutions) and `bingle://games/{id}/rules` (rules, pieces, win
conditions and move notation as text) — and three **prompts**: `play_game`
(a step-by-step procedure for playing a full game against a bot),
`train_agent` (an evaluation loop over the `env_*` tools) and `solve_puzzles`
(a puzzle benchmark with a solve-rate report).

## Playing in a room

A tool that acts for a seat names it with `player_name`. Connect with an API token of an account (`Authorization: Bearer bgl_…`, made on the account page,
with the `play` and `create_game` scopes; required on a `closed` server) or a guest key
(`Authorization: Bearer bgg_…`, from `POST /api/guest`), and the seats the agent takes are its
own: nobody else can play them by naming them. Without a key the agent is anonymous and plays, as
before, only seats nobody owns ([ADR 0067](decisions/0067-principals-and-guest-keys.md)).

| Tool | Purpose |
|---|---|
| `list_game_defs` | games available (`id`, `name`, `description`, board size, `family`, `version`) |
| `get_game_info` | rules text, pieces, actions, win rules and **move notation help** |
| `get_game_guide` | the game's full rules guide as Markdown pages (`tab` selects one): overview, the round, actions, battle, factions… — generated from the definition for games without one |
| `list_bots` | built-in opponents: `random`, `tactical`, `minimax`, `mcts` |
| `add_bot` | seat a built-in bot in an open seat (`seat`, else the lowest) of a waiting game, named after the seat's faction and the bot's kind; a game without a lobby starts when the table is full |
| `start_game` | the host begins a game waiting in its lobby (`create_game` with `lobby=true`); every seat must be filled |
| `send_chat` / `get_chat` | table talk of a game: say something as a player or a spectator; read the latest messages or those after an id |
| `set_seat_cult` | before the game starts, change the cult of an open seat of a lobby game (Cthulhu Wars: `gc`, `cc`, `bg`, `ys`, `ww`, `sl`); a cult played from another open seat changes places with it, seats a player or bot holds keep theirs |
| `get_reactions` / `set_reactions` | your faction's reactions (Devolve, Necrophagy, Madness, Cannibalism, Energy Nexus, Unspeakable Oath…, with `held` once earned), the reaction waiting for you (`pending`, with `targeted` when an action was aimed at you), and when the game asks you about them. A person's reactions are declined for them unless they ask to be asked (`targeted`, `ask_all`, `ask_seats`, for every reaction or — with `ability`, an id from `get_reactions` — for one: Madness and the Unspeakable Oath have choices of their own, and Madness starts by asking every time); an AI client that creates or joins a game over MCP starts with `ask_all`, as a built-in bot does: every reaction window is put to it, and it answers with `make_move` (`done` declines) |
| `create_game` | new room; `opponent` seats a bot, `seat` picks your side, `options` the setup choices of games that have them (Cthulhu Wars: `{"factions": ["ww","sl","gc","cc","ys"], "high-priests": true, "azathoth": true, "dreamlands": true, "beyond": true, "igoo-yig": true, "book-swaps": {"bg": "shrivel"}}`; the packs and each Independent, `igoo-abhoth` … `igoo-yig`, are bools) |
| `join_game` | join an existing room by code (player or spectator) |
| `get_observation` | **the decision context**: text board, TPS, whose turn, legal moves, reward, done |
| `get_legal_moves` | every legal move for the player to move (notation + structured fields) |
| `make_move` | play `notation` (preferred), or structured action/piece/from/to/count/path/drops/aim |
| `suggest_move` | a built-in bot's choice for the position (or a variation); only in games against bots or after the game |
| `undo_move` / `resign` | take back your last move / concede |
| `takeback` | ask a human opponent to let you take back a move once they have replied (`action`: request / cancel / accept / decline) |
| `draw` | offer / cancel / accept / decline a draw (human opponents only) |
| `simulate` | what-if: apply a line of moves to the current or a past position |
| `import_game` | load a PTN document or move list into a new room (validated, full history) for replay and `simulate` / `suggest_move` analysis |
| `export_game` | full record: moves in notation, initial/final position, PTN (Tak) |
| `get_state`, `get_moves`, `list_legal_moves` | the v1 tools, unchanged |

A game with setup options lists them in `get_game_info` (`options`); the game
that `create_game` makes for a choice has an id of its own
(`cthulhu-wars-5p~ww-sl-gc-cc-ys-hp`, [ADR 0028](decisions/0028-derived-games-and-setup-options.md)),
which `list_game_defs` does not show but every tool accepts as `game_def_id` —
`env_reset`, `env_spec` and `match_bots` included.

`create_game` also takes `also_seats` (a list of seats, besides yours, that you
play too: a game against yourself; each is a player named
`<you> · <faction or colour>`), `move_time_seconds` (per-move limit; the player who
runs out loses) or `clock_seconds` (a game clock for a game of two: the seconds
each seat starts with, in seat order, which may differ; the player whose clock
runs out forfeits) — `get_observation` then reports `seconds_left` — and
`start_tps` / `start_state` + `start_seat` to begin from a given position
(puzzles, endgames, curricula in persisted rooms; `env_reset` has `start_tps`
for in-memory episodes).

A turn loop is: `get_observation` → pick one of `legal_moves` →
`make_move`. When the opponent is a bot, `make_move` returns its reply in
`bot_moves`, so the next `get_observation` is already yours to answer.

### How strong are the bots?

Seeded `match_bots` results (each pair alternates seats; `mcts` limited to 150
iterations per move; wins–losses–draws for the first-named bot; small samples,
so read them as orders of magnitude):

| game | tactical v random | minimax v tactical | mcts v tactical | mcts v minimax |
|---|---|---|---|---|
| Tak 3×3 | 33–1–6 | 29–0–1 | 28–0–2 | 6–21–3 |
| Tak 5×5 | 10–2–0 | 10–0–0 | 7–1–2 | 1–9–0 |
| Connect Four | 18–2–0 | 11–4–1 | 15–1–0 | 10–6–0 |
| Hex 7×7 | 11–1–0 | 11–1–0 | 7–5–0 | 5–7–0 |
| Tic-Tac-Toe | 29–5–6 | 15–0–15 | 12–2–16 | 0–8–22 |

`minimax` (alpha-beta a few plies deep; it counts pieces on top and rewards
the largest connected group where the game is about connection) is the
strongest opponent in Tak, where those two terms are the flat-win and road
criteria. `mcts` (random playouts) does better where such a heuristic says
little (Connect Four) and improves with more iterations or time. They are
baselines, not engines. Connect Four and Tic-Tac-Toe are no longer shipped,
but the tests keep both as fixtures, so they stay in the table. Regenerate it with
`BINGLE_STRENGTH=1 go test ./tests/bots -run StrengthReport -v`.

Courtfall is a long, chancy game that the table's heuristics do not fit, so it
supplies its own: the ruleset's `Heuristic` (victory points, Momentum, healthy
units, the Eye) is the minimax leaf value and scores unfinished MCTS
playouts, and setup (tiles, Courts, deployment) is played at random. With a
2,500-node budget `minimax` beat `random` 4–0 and `tactical` 4–0, finishing
in about 80–120 plies; `tactical` and `random` never finish a game within
1,800 plies because neither learns to take a Clear Turn. `mcts` is slow and
much weaker here (an unfinished game after 1,800 plies against `random`):
prefer `minimax` for Courtfall hints and opponents.

### Observations

`get_observation` returns structured data plus a readable text block. For Tak:

```
tak-5x5 — room K3N7QX — status: active
You are seat 1 (white) — YOUR TURN (move 5).
Last move: Cc4 (seat 2)
     a   b   c   d   e
5    .   .   .   .   .
4    .   .   bC  wS  .
3    .   .   bF  .   .
2    .   wF  .   .   .
1    .   .   .   .   .
Reserves:
  White (w): cap 1, stone 19
  Black (b): cap 0, stone 20
TPS: x5/x2,2C,1S,x/x2,2,x2/x,1,x3/x5 1 3
Legal moves (70): a5 b5 c5 d5 e5 a4 b4 e4 a3 b3 d3 e3 ... Ce1 d4+ d4> d4- b2+ b2> b2- b2<
```

Cells show `<color initial><piece code>` (`wF` = white flat stone, `bS` =
black standing stone, `wC` = white capstone) followed by the stack height when
there is more than one piece; stacks are spelled out bottom-to-top under
*Stacks*. `TPS` is Tak's standard one-line position string.

Structured fields: `state` (`cells` → stacks bottom-to-top, `reserves`,
`ply`), `legal_moves` (notation strings), `your_turn`, `done`,
`winner_seat`, `end_reason`, `result` (`win`/`loss`/`draw` for your seat) and
`reward` (+1 / −1 / 0).

### Games with chance and hidden information (Courtfall)

A game with `engine: "ruleset"` (see [courtfall.md](courtfall.md)) differs in a
few ways that matter to an agent:

* **A turn is many decisions.** `legal_moves` lists the next *decision* (a unit
  to activate, a destination, a dice split…); after your move the same seat may
  be asked again, and the opponent may be asked mid-turn (the target of a
  strike answers with `respond/N`). `to_move` / `turn_seat` always says who.
* **`state.ext.view`** is the derived picture of the position — `prompt` (what
  is being waited for), `tints`, `badges`, `seats[].counters` (Momentum, VP),
  `contest` (both sides' dice once rolled) — and `state.ext.events` narrates
  the last decision. `board_text` shows the same as text. A multi-seat game
  (Cthulhu Wars, see [cthulhu-wars.md](cthulhu-wars.md)) also carries
  `state.ext.log` (what the latest decisions did, per seat and ply — read it to
  see what the other players did since your last move), `view.phases`,
  `view.order`, `view.bag` and, per seat, `units` (cost, Combat, pool and the
  spellbooks that bear on each unit) and your own Elder Sign values
  (`signs`; other seats show only `hidden_signs`).
* **Secrets.** The dice generator is never exposed; a strike's committed split
  is `a_atk: -1` (`Qxd4/?` in move lists) to everyone but the committing
  player's own `get_observation`. `simulate`, `suggest_move` and forked
  environments run on a *determinization*: dice and unknowable choices are
  re-drawn, so they estimate rather than reveal.
* **No undo across dice.** `undo_move` and take-backs are refused once a die
  has been rolled since the move.
* **Seeding.** `env_reset seed=N` fixes the game's dice as well as the bot's
  play; `match_bots seed=N` does the same per game, so seeded runs replay
  exactly. Planes (`env_spec`) are viewer-relative: units by type, conditions,
  terrain, Momentum, VP, phase and stage; the action space is fixed (1,823
  IDs; mask with `legal_action_ids`).

## Training environments (`env_*`)

Rooms are persistent and multi-party; for training you want something fast and
disposable. Environments live only in memory (no database round trip), are
seedable, and follow the Gym `reset`/`step` shape.

| Tool | Purpose |
|---|---|
| `env_reset` | start an episode: `game_def_id`, `seat`, `opponent` (`random`/`tactical`/`minimax`/`mcts`/`self`), `seed`, `max_plies`, `start_tps` / `start_state` + `start_seat` / `puzzle_id`, `bot_iterations`, `include` (`planes`, `action_ids`), `plane_depth` |
| `env_step` | play `move` (notation) or `action_id`; the bot replies inside the call; returns the next observation |
| `env_observe` | read the current observation again |
| `list_puzzles` | curated "win in N" positions (`game_def_id`, `max_win_in`, `reveal` for solutions); start one with `env_reset puzzle_id` |
| `env_batch_step` | step up to 128 environments in one call (`steps`: list of `{env_id, move \| action_id}`); a bad step reports its own error |
| `env_spec` | describe a game for model building: board geometry, plane channels, fixed action-space size (`include_actions` lists every action) |
| `env_fork` | clone an environment (position, history, opponent) — for tree search and exploration |
| `env_close`, `env_list` | housekeeping (idle environments expire after 30 minutes) |
| `match_bots` | play N games between two built-in bots (`bot_a`, `bot_b`, `games`, `seed`, `include_moves`) and get the tally — baselines and move records without an agent in the loop. `record` (≤ 50 games) returns every position as a training example: `state`, `move`, fixed `action_id`, and the game's `result` for the mover (+1/0/−1); `record_planes` adds mover-relative planes |

Observation fields: `env_id`, `agent_seat`, `to_move`, `your_turn`, `ply`,
`board_text`, `tps`, `state`, `legal_moves`, `history`, `last_moves` (your
move followed by the replies), `done`, `truncated`, `winner_seat`,
`end_reason`, `reward`, `result`.

* **Single-agent**: the agent plays `seat`; a bot plays the rest.
  `reward` is +1/−1/0 at the end and 0 before; a truncated episode
  (`max_plies`) has `truncated: true` and reward 0.
* **Self-play**: `opponent: "self"` — you move for whoever is to move
  (`to_move`); `winner_seat` tells you who won so you can credit either side.
* **Curricula and puzzles**: `start_tps` starts from any Tak position, e.g. a
  position one move from a road; `start_state` (+ `start_seat`) does the same
  for any game. `puzzle_id` (see `list_puzzles`) starts a curated position —
  the side to move has a forced win proven by exhaustive search — with
  `max_plies` set to the proven line plus slack, so reward `+1` means "solved"
  and a truncated episode means "failed": a ready-made reasoning benchmark.
* **Illegal moves** are tool errors that leave the environment unchanged;
  restrict yourself to `legal_moves` for a fixed action space per step.
* **Determinism**: the engine is deterministic and `legal_moves` is in a
  stable order; with a `seed`, `random` and `tactical` are reproducible. The
  `mcts` opponent searches for about a second per move by default — pass
  `bot_iterations` (say 100) to bound it by iterations instead, which is both
  fast and reproducible for a given seed. The same option exists on
  `match_bots`.

### Model input: planes and fixed action IDs

Text boards are for language models; for a network you want tensors and a
fixed action space. Ask for them with `include: ["planes", "action_ids"]` on
`env_reset` (they stay on for every later observation of that environment and
its forks), and size your network with `env_spec`.

* **`planes`** — `{shape: [C, H, W], channels: [...], data: [...]}`, row-major
  `[channel][y][x]`, float. It is encoded *relative to the agent* (`me` /
  `opp`), so one network plays either seat. Channels are, in order: piece
  one-hots for each stack depth (`top/me/flat`, `top/opp/wall`, …,
  `depth2/me/flat`, … — one level for single-piece games, four for stacking
  games unless `plane_depth` says otherwise), `height` (raw stack size),
  `reserve/<who>/<pool>` (remaining ÷ starting count), `to-move` (1 if the
  agent is on move) and `ply` (÷ 100). `env_spec.board.cells` maps each plane
  cell back to a position (`a5`, …). Grid and hex boards only.
* **`legal_action_ids`** — the fixed integer ID of every entry in
  `legal_moves` (same order) and `num_actions`, the size of the space, so a
  policy head with one logit per ID can mask illegal actions. `env_step`
  accepts `action_id` instead of notation. Tak 5×5 has 1,575 actions (3×3: 135,
  7×7: 12,495, 8×8: 32,704); Amazons 10×10 has 86,980 (from × to × arrow).
  Games with chained jumps (Draughts) have unbounded move paths and report a
  *dynamic* space: pick from `legal_moves`.

### Sketch of a training/eval loop

```python
obs = call("env_reset", game_def_id="tak-5x5", opponent="tactical", seed=0)
env = obs["env_id"]
while not obs["done"]:
    move = policy(obs["board_text"], obs["legal_moves"])     # your model
    obs = call("env_step", env_id=env, move=move)
reward = obs["reward"]                                        # +1 / 0 / -1
```

Use `env_fork` to branch a position for rollouts/MCTS, and `export_game`
(rooms) or `obs["history"]` (environments) to collect move records.

## Training over plain HTTP (and Python)

The same environments are served over REST for clients that do not speak MCP:

| Endpoint | Equivalent |
|---|---|
| `POST /api/envs` (body as `env_reset`, plus `include`) | `env_reset` |
| `GET /api/envs`, `GET /api/envs/{id}`, `DELETE /api/envs/{id}` | `env_list`, `env_observe`, `env_close` |
| `POST /api/envs/{id}/step` (`{move}` or `{action_id}`) | `env_step` |
| `POST /api/envs/{id}/fork` | `env_fork` |
| `POST /api/envs/batch-step` (`{steps: [...]}`) | `env_batch_step` |
| `GET /api/env-specs/{game}?include_actions=1&plane_depth=N` | `env_spec` |
| `POST /api/matches` (body as `match_bots`) | `match_bots` |

`scripts/bingle_env.py` is a standard-library-only Gym-style client over these
endpoints (`reset` / `step` / `fork` / `close`, planes and fixed action IDs):

```python
from bingle_env import BingleEnv
env = BingleEnv("http://localhost:8080", "tak-5x5", opponent="tactical")
obs, info = env.reset(seed=0)
obs, reward, terminated, truncated, info = env.step(info["legal_action_ids"][0])
```

## Using the HTTP API instead

The room tools are also available over REST, e.g.
`GET /api/games/{code}/observe?player_name=…`,
`GET /api/games/{code}/moves/legal`, `POST /api/games/{code}/simulate`,
`POST /api/games/{code}/undo`, `GET /api/games/{code}/export?format=ptn`,
`GET /api/game-defs/{id}`, `GET /api/bots`.
