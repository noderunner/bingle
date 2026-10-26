# Bingle — Architecture

This describes the system as designed from `docs/history/brief.md` plus the decisions made during the requirements interview. It is a reference document, not a build plan — see `docs/history/plan.md`  (the original plan, kept for history).

> **Update (v2):** the engine, API, MCP surface and web UI have grown well past the v1 design below — stacks, reserves, placement, Tak, undo, bots, training environments and a real UI. The v1 text is kept because its shape (one binary, one engine behind REST/WS/MCP, SQLite) still holds; see [v2 additions](#v2-additions) at the end, `docs/roadmap.md` for status, `docs/dsl.md`, `docs/mcp.md` and `docs/ui.md`.

## Overview

Bingle is a turn-based abstract-game platform. Human players (via a web UI) and AI/remote clients (via a REST/WebSocket API or MCP) create and join games defined by a JSON DSL, make moves, and watch state update live. Everything runs as one Go binary in one container, backed by an embedded SQLite database.

## Process model

A single Go binary starts, in one `main()`, via goroutines:

- an HTTP server exposing REST endpoints, a WebSocket endpoint, and the static web UI, all on one listener/mux
- a generic MCP server (stdio or its own listener) wrapping the same engine calls
- a SQLite connection pool shared by all of the above

No external process supervisor is needed; no separate services to coordinate. TLS is optional and config-driven (cert/key paths) — off by default for local/dev, enabled for remote deployments by supplying a cert.

```
                 ┌─────────────────────────────────────────┐
                 │              bingle (one binary)          │
                 │                                           │
Web browser ───► │  HTTP mux                                 │
Remote client──► │   ├─ REST  (create/join/move/state)  ─┐   │
                 │   ├─ WebSocket (live state push)      │   │
                 │   └─ static UI assets                 ▼   │
AI / MCP ──────► │  MCP server ──────────────────► Game Engine │
   client        │                                     │       │
                 │                                     ▼       │
                 │                              SQLite (games, │
                 │                              players, moves)│
                 └─────────────────────────────────────────┘
```

## Game engine

The engine is transport-agnostic: REST handlers, WebSocket broadcast, and MCP tools all call the same in-process engine functions (create game, join, make move, get state). This is what lets the MCP server be generic instead of per-game.

### Game definition DSL

Games are defined declaratively in **JSON**:

- **Board**: modeled as a graph — a set of positions plus adjacency edges (optionally typed, e.g. "orthogonal", "diagonal") — rather than a fixed square grid. This lets a Go-style square grid, a hex board, or an irregular board all be expressed the same way.
- **Pieces**: types with a color/owner and a movement rule drawn from a small built-in primitive library (e.g. step-to-adjacent, slide-along-edge-type, jump-over-adjacent).
- **Turn order**: simple rotation among seated players.
- **Escape hatch**: a game may register a Go callback package (implementing a small interface) for logic the JSON primitives can't express — custom legality checks, win conditions, special captures. Most of a game's behavior should be expressible in JSON; the callback exists for the rest.

Each game module = one JSON definition + an optional Go callback package, registered by name into a game-type registry at startup.

### Demo game

Ships as the first game module: a Go-style board, two piece colors, free movement to any empty adjacent-or-any position, and simple capture (landing on an opposing piece removes it). No win condition. Exists to exercise the move/state/turn/capture pipeline, not to be a real game.

## Sessions & matchmaking

- No accounts, no auth. Creating a game returns a **room code** (and a shareable link).
- Joining a room requires the room code and a display name — no password. The first two distinct names to join take seat 1 and seat 2.
- Additional joiners with the same room code become **spectators**: read-only WebSocket subscribers to that room's state.
- There is no session token. "Reconnecting" means re-entering the same room code and the same name you joined with — the server matches the name to your existing seat. This is intentionally lightweight per the brief's low-security, casual-app framing.

## Persistence (SQLite)

Embedded, single-file SQLite (e.g. `modernc.org/sqlite`) — no separate DB process, fits inside the one container.

Conceptual tables:
- `games` — id/room code, game type, status, created_at
- `players` — game_id, seat, name
- `moves` — game_id, sequence number, seat, move payload (JSON), resulting state, timestamp

Full move history is kept (not just current state), so games can be replayed and resume correctly after a server restart.

## Transport

- **REST**: list game types, create a game, join a game (as player or spectator), submit a move, fetch current or historical state.
- **WebSocket**: one channel per room; broadcasts turn changes, applied moves, and join/leave events to every connected client (players and spectators) for that room. Used for turn notifications instead of polling.
- **TLS**: optional, off by default, enabled via config when cert/key are supplied.

## MCP server

One generic MCP server, in-process, not per-game. It exposes tools such as:

- `list_games` — available game types and/or open rooms
- `create_game` / `join_game` — same semantics as the REST equivalents
- `get_state` — current board/turn state for a room
- `make_move` — submit a move on behalf of the AI's seat

Because these tools call the same engine functions the REST API uses, any AI model can join and play (or spectate) any game without game-specific MCP work.

## Web UI

Served by the same binary as static assets plus lightweight JS (no SPA build pipeline). Boards are rendered as **SVG**, generated generically from the board-as-graph model (positions, adjacency, piece colors) — so the UI doesn't need per-game rendering code, only a generic graph-to-SVG layout.

## Explicitly out of scope (per brief)

- Accounts, auth, RBAC
- Full rule sets for real games beyond those shipped
- Audio
- Enforced turn clocks (async-only for v1)

## v2 additions

```
                     ┌──────────────────────── bingle (one binary) ────────────────────────┐
 browser (ES modules) │ REST /api   WS /ws/{code}   MCP /mcp                                 │
 custom clients ─────►│      │            │             │            ┌──────────────┐        │
 MCP agents ─────────►│      ▼            ▼             ▼            │ envs (memory)│        │
                      │   session.Manager ◄─────── mcpserver ──────► │ reset/step   │        │
                      │   │  rooms, undo, resign, simulate, observe  └──────┬───────┘        │
                      │   │  bots (random / tactical / minimax / mcts)      │                │
                      │   ▼                                                 ▼                │
                      │ store (SQLite, WAL)                  engine: catalog → state         │
                      └─────────────────────────────────────────────────────────────────────┘
```

* **Engine** (`internal/engine`): `dsl` (JSON types, variants), `board`
  (grid/explicit graphs, regions, coordinates), `catalog` (validates and
  compiles a definition: pieces, pools, actions, opening, win rules),
  `state` (position document, `LegalMoves`, `Apply`, win evaluation, notation,
  TPS, text rendering, interactive move steps). See ADR 0015. A game may
  instead bring a Go `state.Ruleset` (`engine: "ruleset"`) that implements
  the same functions itself — with chance, hidden information and
  multi-decision turns — behind the same entry points (ADR 0025).
* **Games** (`internal/games`): `tak` (3×3–8×8 from one JSON
  document), `classics` (Tic-Tac-Toe, Connect Four, Gomoku, Hex, Draughts,
  Breakthrough, Reversi, Go, Yavalath, Havannah, Amazons, Ataxx, Pente,
  Brandubh), `courtfall` (a Go ruleset with a JSON legend,
  [courtfall.md](courtfall.md)), `fist` and `onitama` (Go rulesets, one move per turn), `blooms` (a Go ruleset, a turn of one or two placements), `hive` (a Go ruleset with stacks on a large hex field), `fendo` (a Go ruleset whose fences travel in the view), `yinsh` (a Go ruleset on an explicit 85-point graph), `nyet` (a Go ruleset on the card framework in `internal/engine/cards`),
  `cthulhuwars` (up to five of six cults on an area map
  with an embedded Markdown rules guide and setup options that derive a game
  per table, [cthulhu-wars.md](cthulhu-wars.md)).
  Adding a game is adding a JSON file
  (`docs/dsl.md`), or a ruleset when the rules need Go.
* **Session** (`internal/session`): room lifecycle plus undo, take-backs,
  draws, resign, simulation, hints, game import, bot-versus-bot rooms,
  observation, export and bot turns; broadcasts `move` and `state` events on
  the hub.
* **Bots** (`internal/bots`) work for any game through the generic engine API.
* **Envs** (`internal/envs`): in-memory single-agent episodes for training
  (ADR 0019), with model-ready planes and fixed action IDs (ADR 0023).
* **Puzzles** (`internal/puzzles`): an exhaustive "win in N" solver and the
  embedded set it proved (`scripts/genpuzzles` regenerates it).
* **Web** (`internal/web`): a lobby and a game page built from ES modules; the
  board is drawn generically from `GET /api/game-defs/{id}` and moves are
  entered by walking the server's legal-move list (ADR 0017).
* **API/MCP** share the manager (and the environments manager); the v1 tools
  and endpoints are unchanged and new fields are additive (ADR 0016). The
  environments are served both as MCP tools and as `/api/envs` (ADR 0024).
* **Reactions, lobbies, chat** (ADR 0027, 0033): rulesets that implement
  `state.Reactor` mark decisions that interrupt another player's turn (and
  whether the action that opened the window was aimed at the reacting seat);
  the session plays the decline for a person unless they asked to be asked
  (when targeted, after anyone's action, after chosen players') — bots and AI
  clients are always asked. Games may wait in a lobby for their host, where
  anyone may change the cult of an open seat (`state.SeatCults`); every game has
  a small chat with emoji. `BINGLE_PUBLIC_URL` is the base of invite links.
* **Auto-Play Timeout** (ADR 0033): `Manager.RunAutoPlay` watches the games
  whose player to move has the option on and lets the `tactical` bot play the
  turn once the wait since the game's last change has run out; the moves are
  recorded as the person's with `autoplay` set.
* **Operations**: `BINGLE_RETENTION_DAYS` removes games nobody touched for
  that long; hint, match and bot searches share bounded CPU slots.
* **Persistence**: `games` (with `lobby`), `players` (with `bot`), `moves`
  (with `notation`, `move_json`, `auto`, `locked`, `autoplay`), `reaction_prefs`,
  `autoplay_prefs`, `chat_messages`;
  additive column migrations on open; WAL mode with a busy
  timeout so concurrent tabs and agents never see `SQLITE_BUSY`.
