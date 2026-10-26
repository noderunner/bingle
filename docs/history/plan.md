# Bingle — Implementation Plan

Inputs: `docs/history/brief.md` (original brief) and the interview outcome doc
(`.claude/plans/read-docs-brief-md-and-interview-serialized-shamir.md`), whose
decisions are treated as fixed constraints below. Anything not pinned down by
those two documents is decided here explicitly and flagged as a **Plan
Decision** so it can be revisited without re-opening the resolved questions.

## 1. Goal

Ship a single Go binary that lets humans (via a browser web UI) and AI models
(via a generic MCP server, or direct REST/WebSocket) create and join
room-code-based games of custom, JSON-defined abstract board games, backed by
SQLite with full move history/replay. Ship one concrete game — "Demo:
Free Movement & Capture" — as the first exercise of the DSL/engine, per the
brief's no-win-condition demo requirement.

## 2. Plan Decisions (filling gaps the interview left open)

These are architectural choices made in this pass, within the interview's
constraints, so the coder agent has an unambiguous target:

- **Go module path:** `bingle` (no VCS host prefix needed for this
  greenfield, unpublished module).
- **One HTTP(S) listener, not three.** The interview says API/UI/MCP run "as
  internal services/listeners from one `main()` via goroutines." This plan
  mounts all four surfaces (web UI, REST, WebSocket, MCP) as path-prefixed
  sub-routers on a *single* `http.Server` (one port, one TLS config). Internal
  concurrency (the room hub, DB writer serialization) still runs via
  goroutines. This avoids duplicating TLS/port config for no functional gain.
  If a future need arises to split MCP onto its own port, the `internal/app`
  seam (below) makes that a small change.
- **No plugin/reflection game loader in v1.** Only one game (`demo`) ships.
  Game modules are wired into the catalog by explicit Go code in
  `internal/app`, not dynamic discovery. Adding a second game later means
  adding a package under `internal/games/` and one line in `internal/app`.
- **Testing seam:** the tester agent may only write under `tests/`, and
  cannot add app code (helpers, fakes) outside it. Therefore `internal/app`
  MUST expose everything integration tests need as exported, in-process
  constructors (no real network listener required to test REST/WS/MCP), and
  MUST NOT open any `net.Listen` itself — only `main.go` binds a real port.
  This is called out again in section 6.7 and must not be violated by the
  coder.
- **Rendering split:** Go serves only JSON state + a static HTML/JS/CSS
  shell; all SVG board drawing happens client-side in `board.js` from the
  JSON board graph + occupants. Go never generates SVG server-side. This
  keeps one rendering path regardless of which game module is active.
- **MCP `list_games` vs. "no lobby" decision:** the interview's "no lobby
  list" is about human matchmaking UX (no browse-to-join screen in the web
  UI). It does not forbid a small API-level utility. `list_games` is kept as
  an MCP/REST convenience for AI agents that were told "a game exists" out of
  band and need its room code / status — it is not a public matchmaking
  lobby and requires no accounts.
- **Dependencies (well-supported, Go-idiomatic, no CGO):**
  `modernc.org/sqlite` (pure-Go SQLite driver), `github.com/go-chi/chi/v5`
  (router), `github.com/coder/websocket` (WS), `github.com/mark3labs/mcp-go`
  (MCP server, Streamable HTTP transport). Coder should confirm these resolve
  via `go get` and are not superseded by a newer preferred library at
  implementation time; the design does not depend on any library-specific
  API beyond "router", "WS conn", "MCP tool server".

## 3. Package Layout

```
bingle/
├── go.mod
├── main.go                          # flags/env → config, wire App, bind listener, graceful shutdown
├── docs/
│   ├── brief.md
│   └── plan.md
├── internal/
│   ├── config/                      # Config struct + Load() from env/flags
│   ├── httpserve/                   # isolated, testable "pick TLS vs plain HTTP" listener logic
│   ├── app/                         # composition root: wires store+catalog+session+api+web+mcp
│   ├── store/                       # SQLite schema + all persistence queries
│   ├── engine/
│   │   ├── dsl/                     # JSON game-definition Go types + Parse/Validate
│   │   ├── board/                   # board-as-graph: positions, adjacency, generators
│   │   ├── primitives/              # built-in movement/occupancy primitive library
│   │   ├── callback/                # Go-callback escape hatch interface + no-op default
│   │   ├── catalog/                 # compiles a DSL def + primitives + callbacks into a playable CompiledGame
│   │   └── state/                   # BoardState + ApplyMove (the actual rules engine)
│   ├── games/
│   │   └── demo/                    # demo.json (embedded) + Module() registration
│   ├── session/                     # room-code sessions: create/join/move/state, turn logic, pub-sub hub
│   ├── api/                         # REST handlers + WebSocket handler (chi router)
│   ├── web/                         # HTML shell, static JS/CSS (SVG board rendering)
│   └── mcpserver/                   # MCP tool definitions wrapping session.Manager
└── tests/                           # owned by the tester agent — coder must not create/populate this
```

## 4. Files To Create

| Package | Files |
|---|---|
| root | `go.mod`, `main.go` |
| `internal/config` | `config.go` |
| `internal/httpserve` | `httpserve.go` |
| `internal/app` | `app.go` |
| `internal/store` | `store.go`, `schema.sql` (embedded via `go:embed`) |
| `internal/engine/dsl` | `dsl.go`, `parse.go` |
| `internal/engine/board` | `board.go`, `grid.go`, `explicit.go` |
| `internal/engine/primitives` | `primitives.go`, `step.go` |
| `internal/engine/callback` | `callback.go` |
| `internal/engine/catalog` | `catalog.go` |
| `internal/engine/state` | `state.go`, `apply.go` |
| `internal/games/demo` | `demo.json`, `demo.go` |
| `internal/session` | `manager.go`, `hub.go`, `roomcode.go` |
| `internal/api` | `router.go`, `games.go`, `ws.go`, `errors.go` |
| `internal/web` | `router.go`, `templates/index.html.tmpl`, `templates/game.html.tmpl`, `static/app.js`, `static/board.js`, `static/style.css` |
| `internal/mcpserver` | `server.go`, `tools.go` |

`go.sum` is generated by `go mod tidy`, not hand-written. `tests/` is
intentionally absent from this table — see section 2 and 6.7.

## 5. Ordered Implementation Steps

1. `go mod init bingle`; add module dependencies (chi, coder/websocket,
   modernc.org/sqlite, mark3labs/mcp-go) to `go.mod` via `go get`.
2. `internal/engine/dsl`: define `GameDefinition` and nested types (`Board`,
   `BoardGenerator`, `Position`, `Edge`, `PieceDef`, `SeatDef`, `TurnOrderDef`,
   `WinConditionDef`, `CallbackRefs`) with JSON tags; `Parse([]byte)
   (*GameDefinition, error)` validating required fields and referencing only
   known primitive names (cross-checked against `primitives` package in step
   4, so wire that dependency direction: `dsl` has no dependency on
   `primitives`, validation of primitive *names* happens in `catalog`, not
   `dsl`, to avoid an import cycle).
3. `internal/engine/board`: `Graph` type (`Positions map[PositionID]Position`,
   adjacency as `map[PositionID][]PositionID`), `Neighbors`, `HasPosition`,
   `HasEdge`. `grid.go`: `Generate(width, height int, mode string) (*Graph,
   error)` for `mode` in `{orthogonal4, orthogonal8}`. `explicit.go`: builds a
   `Graph` directly from a DSL's hand-authored `positions`/`edges` lists (for
   irregular/hex boards, deferred beyond the demo game but must exist so the
   DSL is not grid-only).
4. `internal/engine/primitives`: movement primitive signature (section 6.2)
   and registry `Get(name string) (MovementPrimitive, bool)`; implement
   `step` supporting `occupancy` modes `empty-only` and `capture-or-empty`.
5. `internal/engine/callback`: `GameCallbacks` interface (section 6.3) +
   `NoopCallbacks{}` default implementation satisfying it.
6. `internal/engine/catalog`: `CompiledGame` (parsed def + resolved
   `MovementPrimitive` per piece type + resolved board `Graph` + resolved
   `GameCallbacks`); `Catalog.Register(id string, defJSON []byte, cb
   callback.GameCallbacks) error` — resolves every primitive name via step 4's
   registry and errors (fails fast) if any is unknown; `Catalog.Get(id)
   (*CompiledGame, bool)`.
7. `internal/engine/state`: `BoardState` (`map[PositionID]Occupant`),
   JSON (de)serialization for DB snapshot storage, `Clone()`.
   `apply.go`: `ApplyMove(game *catalog.CompiledGame, st *BoardState, seat int,
   from, to board.PositionID) (*MoveResult, error)` — the rules engine
   described in section 6.4.
8. `internal/games/demo`: author `demo.json` (section 6.5) with a 9x9
   orthogonal4 grid, two seats (black/white), one piece type (`stone`) using
   the `step` primitive with `capture-or-empty` occupancy, `win_condition:
   none`, no callbacks. `demo.go`: `Module() gamemodule.Module` returning
   `{ID: "demo-freeplay", Definition: embeddedJSON, Callbacks: nil}` (see
   section 6.5 for the small `gamemodule.Module` type, defined alongside
   `catalog` or `dsl` — coder's call, but must not create an import cycle
   between `games/demo` and `app`).
9. `internal/store`: `schema.sql` with the three tables from section 6.6
   (`games`, `players`, `moves`), executed as `CREATE TABLE IF NOT EXISTS` on
   startup (no migration framework needed for a single-schema v1). `store.go`:
   `Store` wrapping `*sql.DB` with methods listed in section 6.6.
10. `internal/session`: room code generation lives in its own subpackage,
    `internal/session/roomcode` (`New() string`, 6-char, unambiguous alphabet,
    `crypto/rand`) — resolved this way while sequencing backlog item 15, since
    it's a self-contained utility with no dependency on the rest of `session`
    and a flat `session.New()` would read ambiguously next to `NewManager`.
    Collision-retry against `Store` happens at the call site (`Manager.CreateGame`,
    a later item), not inside `roomcode` itself. `manager.go` — `Manager` (section 6.4) holding one `*sync.Mutex` per
    active room code, lazily hydrating from `Store` on first touch after a
    restart, orchestrating catalog + state.ApplyMove + Store persistence +
    hub broadcast for every mutating call. `hub.go` — simple per-room
    pub-sub (`Subscribe(code) (ch <-chan Event, cancel func())`,
    `Publish(code string, ev Event)`).
11. `internal/api`: chi router mounted under `/api` and `/ws`; handlers for
    the REST endpoints and the WS upgrade handler (section 6.4/6.6), all
    delegating to `session.Manager`; `errors.go` maps `session` error kinds to
    HTTP status codes (section 6.8).
12. `internal/web`: static shell — one page to create/join a room by code and
    name, one page that renders the board. `board.js` fetches
    `/api/games/{code}/state`, subscribes to `/ws/{code}`, and draws SVG from
    the JSON board graph + occupants; no server-side SVG generation.
13. `internal/mcpserver`: register the tool set from section 6.6 using
    `mark3labs/mcp-go`'s Streamable HTTP transport, each tool handler calling
    `session.Manager` directly (in-process, not via HTTP loopback) and
    returning MCP tool results/errors.
14. `internal/httpserve`: `Serve(ctx context.Context, cfg Config, handler
    http.Handler) error` — decides plain vs TLS `ListenAndServe`, returns
    (does not `os.Exit`) a wrapped error for a missing/unreadable cert or key
    file so this logic is unit-testable without a real bind.
15. `internal/app`: `New(cfg Config) (*App, error)` — opens the `Store`,
    builds the `Catalog` (registers `demo.Module()`), builds the
    `session.Manager`, builds one `chi.Router` mounting `internal/web` at
    `/`, `internal/api` at `/api` and `/ws`, `internal/mcpserver` at `/mcp`.
    `App.Router() http.Handler`, `App.DB() *sql.DB` (test introspection),
    `App.Close() error`. **`App` must never call `net.Listen`.**
16. `internal/config`: `Config{Addr, DBPath, TLSCert, TLSKey string}` +
    `Load() (Config, error)` from env vars (`BINGLE_ADDR`, `BINGLE_DB_PATH`,
    `BINGLE_TLS_CERT`, `BINGLE_TLS_KEY`) with defaults `:8080` and
    `./bingle.db`; TLS is off unless both cert and key are set.
17. `main.go`: `config.Load()` → `app.New(cfg)` → `httpserve.Serve(ctx,
    httpserve.Config{...}, app.Router())`; `signal.NotifyContext` for
    graceful shutdown; `log.Fatal` only here, never inside `internal/*`.
18. Do not create `tests/`. Leave it for the tester agent; this plan's
    section 6.7 and section 8 are its contract.

## 6. Interfaces & Data Shapes

### 6.1 Game-definition JSON DSL

```jsonc
{
  "id": "demo-freeplay",
  "name": "Demo: Free Movement & Capture",
  "board": {
    // exactly one of "generator" (built-in) or "positions"+"edges" (explicit, for irregular/hex boards)
    "generator": { "type": "grid", "width": 9, "height": 9, "adjacency": "orthogonal4" }
  },
  "seats": [
    { "seat": 1, "color": "black" },
    { "seat": 2, "color": "white" }
  ],
  "pieces": {
    "stone": { "movement": { "primitive": "step", "occupancy": "capture-or-empty" } }
  },
  "setup": [
    { "position": "0,0", "piece": "stone", "color": "black" },
    { "position": "8,8", "piece": "stone", "color": "white" }
    // demo: coder places a small symmetric row of stones per color on each player's home edge
  ],
  "turn_order": { "type": "round-robin", "seats": [1, 2] },
  "win_condition": { "type": "none" },
  "callbacks": {}
}
```

Built-in v1 primitive library (intentionally small):

- **Board generators:** `grid` (`orthogonal4` / `orthogonal8` neighbor modes)
  and `explicit` (hand-authored positions + edges — the escape hatch for
  hex/irregular boards until a dedicated generator is warranted).
- **Movement primitive:** `step` — move a piece from an occupied position to
  one graph-adjacent position. `occupancy: "empty-only"` requires the
  destination be unoccupied; `occupancy: "capture-or-empty"` additionally
  allows landing on an opponent-colored piece, which is removed (simple
  capture). Landing on a same-color piece is always rejected.
- **Turn order:** `round-robin` over a fixed seat sequence.
- **Win condition:** `none` (v1's only built-in; the demo game uses it) or
  `callback` (defers to `GameCallbacks.CheckWinner`, for future games).

### 6.2 Movement primitive signature

**Correction (found while sequencing backlog item 04, not in the original planner draft):** the signature below takes an `OccupancyView` interface owned by `primitives` itself, not a `*state.BoardState`. The original draft had primitives depend directly on `state.BoardState`, but `state` (section 6.4) depends on `catalog`, and `catalog` depends on `primitives` to resolve primitive names by string — `primitives → state → catalog → primitives` would be an import cycle. Decoupling via a small interface (consistent with how `board.Generate` already takes plain `width, height int, mode string` instead of a `dsl.BoardGenerator`) fixes this: `primitives` becomes a leaf package with no dependency on `state` or `catalog`. `state.BoardState` (section 6.4) must implement `OccupancyView`.

```go
package primitives

type Occupancy string // "empty-only" | "capture-or-empty"

// OccupancyView answers "is there a piece at this position, and if so what
// color owns it" — the only thing a movement primitive needs to know about
// board state. state.BoardState implements this; primitives itself has no
// dependency on the state or catalog packages.
type OccupancyView interface {
    Occupant(id board.PositionID) (color string, occupied bool)
}

type Result struct {
    Allowed   bool
    Captured  *board.PositionID // non-nil if this move captures an occupant at `to`
    Reason    string            // human-readable rejection reason when !Allowed
}

type MovementPrimitive func(g *board.Graph, occ OccupancyView, occupancy Occupancy, from, to board.PositionID, movingColor string) Result

func Get(name string) (MovementPrimitive, bool)
```

### 6.3 Go-callback escape hatch

**Correction (same class of fix as section 6.2, found while sequencing backlog item 06):** the original draft typed `MoveContext.Board` and `CheckWinner`'s parameter as `*state.BoardState`. But `state` (section 6.4) depends on `catalog` (`ApplyMove` takes a `*catalog.CompiledGame`), and `catalog` depends on `callback` (a compiled game holds a `GameCallbacks`) — so `callback → state → catalog → callback` would be an import cycle, the same shape as 6.2's. Fixed the same way: a small `BoardView` interface owned by `callback` itself (read access via `Occupant`, plus the mutation `OnMoveApplied`'s doc comment calls for, e.g. multi-capture chains), so `callback` has no dependency on `state` or `catalog`. `state.BoardState` (section 6.4) must implement `BoardView`, in addition to `primitives.OccupancyView`.

```go
package callback

// BoardView is the read/write board access a callback needs — decoupled
// from state.BoardState so this package has no dependency on state or
// catalog (avoiding an import cycle: callback -> state -> catalog -> callback).
type BoardView interface {
    Occupant(id board.PositionID) (color string, occupied bool)
    SetOccupant(id board.PositionID, color string)
    RemoveOccupant(id board.PositionID)
}

type MoveContext struct {
    GameID string
    Board  BoardView
    Seat   int
    Color  string
    From   board.PositionID
    To     board.PositionID
}

type GameCallbacks interface {
    // Return non-nil error to reject a move the DSL rules would otherwise allow.
    ValidateMove(ctx MoveContext) error
    // Called after a move is applied to the in-memory state, before persistence;
    // may mutate ctx.Board further (e.g. multi-capture chains a future game needs).
    OnMoveApplied(ctx MoveContext) error
    // Only consulted when the DSL's win_condition.type == "callback".
    CheckWinner(st BoardView) (winnerSeat *int, ended bool)
}

type NoopCallbacks struct{} // satisfies GameCallbacks as a no-op; demo uses this
```

### 6.3a Catalog compilation contract

**Elaboration (filling a gap while sequencing backlog item 07):** step 6 of section 5 describes `CompiledGame` only as holding "resolved `MovementPrimitive` per piece type," but `ApplyMove` (6.4) needs each piece's `Occupancy` too (a primitive function alone can't be called without it) — so the resolved-piece value is a small struct, not a bare function. This is a direct elaboration of the existing plan, not a new decision.

```go
package catalog // internal/engine/catalog, file catalog.go

type ResolvedPiece struct {
    Primitive primitives.MovementPrimitive
    Occupancy primitives.Occupancy
}

type CompiledGame struct {
    Definition *dsl.GameDefinition
    Board      *board.Graph
    Pieces     map[string]ResolvedPiece // keyed by piece type name (dsl.GameDefinition.Pieces' keys)
    Callbacks  callback.GameCallbacks
}

type Catalog struct { /* unexported map[string]*CompiledGame */ }

func NewCatalog() *Catalog
func (c *Catalog) Register(id string, defJSON []byte, cb callback.GameCallbacks) error
func (c *Catalog) Get(id string) (*CompiledGame, bool)
```

`Register` behavior, in order:
1. `dsl.Parse(defJSON)` — any parse error is returned as-is (wrapped with a `catalog:` prefix).
2. Board: if `def.Board.Generator != nil`, its `Type` must be `"grid"` (the only generator implemented so far — error otherwise); call `board.Generate(Generator.Width, Generator.Height, Generator.Adjacency)`. Otherwise, `def.Board.Positions` must be non-empty (error otherwise); convert it and `def.Board.Edges` to `board.PositionID`/`[][2]board.PositionID` and call `board.FromExplicit`. Any error from either path is returned.
3. Pieces: for every entry in `def.Pieces`, resolve `primitives.Get(pieceDef.Movement.Primitive)`; an unknown name returns an error naming both the piece type and the primitive name (this is backlog item 07's AC). `Occupancy` is stored as given (`primitives.Occupancy(pieceDef.Movement.Occupancy)`) without validation here — an invalid occupancy value is caught later when a primitive is actually invoked, not at registration time.
4. Callbacks: `cb` if non-nil, else `callback.NoopCallbacks{}` (per section 6.5's demo module note).
5. **Atomicity:** on any error from steps 1–3, `Register` returns that error and leaves the catalog's existing state for `id` completely untouched — it must not add a new entry, and if `id` was already registered from an earlier successful call, that entry must survive unmodified. Only a fully-successful compilation replaces (or adds) the stored entry for `id`.

`Get(id)` returns `(*CompiledGame, true)` if registered, else `(nil, false)`.

### 6.4 Rules engine / session manager

```go
package state

type MoveResult struct {
    NewState     *BoardState
    Captured     *board.PositionID
    NextTurnSeat int
    GameEnded    bool
    WinnerSeat   *int
}

func ApplyMove(game *catalog.CompiledGame, st *BoardState, seat int, from, to board.PositionID) (*MoveResult, error)
```

### 6.4a State & ApplyMove contract

**Elaboration (filling a gap while sequencing backlog item 08):** the plan named `BoardState`/`ApplyMove`'s top-level shape but not `Occupant`'s fields or `ApplyMove`'s exact algorithm. Both follow directly from decisions already made in 6.1–6.3a; nothing here is a new architectural choice.

```go
package state // internal/engine/state, files state.go + apply.go

// Occupant is what's on one board position: which piece type (a key into
// CompiledGame.Pieces, e.g. "stone") and which color/seat owns it.
type Occupant struct {
    Piece string
    Color string
}

// BoardState is the live, mutable position -> Occupant map for one game in
// progress. Use NewBoardState; the zero value is not usable.
type BoardState struct { /* unexported map[board.PositionID]Occupant */ }

func NewBoardState() *BoardState

// Native, piece-aware API — used by ApplyMove itself.
func (s *BoardState) Get(id board.PositionID) (Occupant, bool)
func (s *BoardState) Set(id board.PositionID, occ Occupant)
func (s *BoardState) Remove(id board.PositionID)
func (s *BoardState) Clone() *BoardState

// Satisfies primitives.OccupancyView (color-only read, for invoking a
// piece's movement primitive).
func (s *BoardState) Occupant(id board.PositionID) (color string, occupied bool)

// Satisfies callback.BoardView (color-only read/write, for GameCallbacks).
// SetOccupant only carries a color because that's callback.BoardView's
// already-shipped shape (backlog item 06) — no v1 game exercises callback
// mutation (the demo uses NoopCallbacks), so this is a deliberately minimal
// shim: it sets Color and preserves any existing Piece at that position
// (or leaves Piece empty if the position was previously unoccupied), rather
// than trying to guess a piece type from a bare color string.
func (s *BoardState) SetOccupant(id board.PositionID, color string)
func (s *BoardState) RemoveOccupant(id board.PositionID)

// JSON (de)serialization for DB snapshot storage (a straightforward
// encoding of the occupant map — needed by persistence work later, and
// grouped into this file per section 5 step 7).
func (s *BoardState) MarshalJSON() ([]byte, error)
func (s *BoardState) UnmarshalJSON(data []byte) error
```

`ApplyMove(game, st, seat, from, to)` algorithm:
1. Look up `seat` in `game.Definition.Seats`; unknown seat → error, no mutation.
2. `st.Get(from)`; not occupied → error ("no piece at from"), no mutation.
3. The occupant's `Color` must equal the looked-up seat's `Color`; mismatch → error ("not your piece" / wrong seat), no mutation. (This is the enforcement point for "a move by a name that doesn't hold the current turn's seat" — the session layer, built later, is what actually checks *whose turn it is* before even calling `ApplyMove`; this step is `ApplyMove`'s own independent guard that the piece at `from` actually belongs to the seat making the call.)
4. Resolve `game.Pieces[occupant.Piece]` (a `catalog.ResolvedPiece`); missing → error (defensive; shouldn't happen for a catalog-compiled game).
5. Call `resolvedPiece.Primitive(game.Board, st, resolvedPiece.Occupancy, from, to, occupant.Color)`. This is also what rejects `to` positions that aren't graph-adjacent to `from` (including `to` not existing on the board at all — `board.Graph.HasEdge` already returns false for an absent position, so `ApplyMove` needs no separate "is `to` on the board" check).
6. `!result.Allowed` → return that rejection as the error (wrapping `result.Reason`), no mutation.
7. Call `game.Callbacks.ValidateMove(ctx)` with `ctx.Board = st` (the board *before* mutation), `ctx.Seat/Color/From/To` set from this call's arguments, `ctx.GameID = game.Definition.ID`. A non-nil error rejects the move (returned as-is), no mutation. (This is the callback's chance to reject a move the DSL/primitive rules would otherwise allow — irrelevant for the demo game's `NoopCallbacks`, but must be wired correctly for later games.)
8. Mutate: `newState := st.Clone()`; remove the occupant at `from`; if `result.Captured != nil`, remove whatever's at `*result.Captured` (this is `to`, since a capture always lands on the captured piece's own square); place `Occupant{Piece: occupant.Piece, Color: occupant.Color}` at `to`.
9. Call `game.Callbacks.OnMoveApplied(ctx)` with `ctx.Board = newState` (post-mutation, so a callback may chain further mutations, e.g. multi-capture). A non-nil error here means the whole move fails: return that error and **do not** return a `MoveResult` — `newState` is discarded, exactly as if the move had been rejected at any earlier step. This keeps "any error ⇒ no state change" true throughout `ApplyMove`, not just up to the primitive check.
10. Compute `NextTurnSeat`: `game.Definition.TurnOrder.Type` must be `"round-robin"` (the only turn-order type implemented in v1, per section 6.1's primitive-library note) — any other value is an error (no mutation persisted; return the error, discard `newState`, same as step 9). Find `seat`'s index in `TurnOrder.Seats` and take the next entry, wrapping around.
11. Compute `GameEnded`/`WinnerSeat` from `game.Definition.WinCondition.Type`: `"none"` → always `(false, nil)`. `"callback"` → call `game.Callbacks.CheckWinner(newState)`. Any other value is an error (same discard-and-return-error handling as steps 9–10).
12. Return `&MoveResult{NewState: newState, Captured: result.Captured, NextTurnSeat: nextSeat, GameEnded: ended, WinnerSeat: winnerSeat}, nil`.

```go
package session

type Role string // "player" | "spectator"

type Manager struct { /* store, catalog, per-room mutexes, hub */ }

func NewManager(st *store.Store, cat *catalog.Catalog) *Manager

func (m *Manager) CreateGame(gameDefID, creatorName string) (RoomSummary, error)
func (m *Manager) JoinGame(roomCode, name string, role Role) (JoinResult, error)
func (m *Manager) MakeMove(roomCode, name string, from, to board.PositionID) (MoveOutcome, error)
func (m *Manager) GetState(roomCode string, atSeq *int) (StateSnapshot, error)
func (m *Manager) ListMoves(roomCode string, sinceSeq int) ([]MoveRecord, error)
func (m *Manager) ListLegalMoves(roomCode string, from board.PositionID) ([]board.PositionID, error)
func (m *Manager) ListGames(status string) ([]RoomSummary, error)
func (m *Manager) ListGameDefs() ([]GameDefSummary, error)
func (m *Manager) Subscribe(roomCode string) (<-chan Event, func())
```

`MakeMove` acquires the room's mutex for the full
validate → apply → persist → broadcast sequence, so two concurrent calls for
the same room are strictly serialized in-process (this is the *only*
process writing the SQLite file, so no cross-process locking is needed).

Join semantics (`JoinGame`):
- `role == spectator`: always succeeds, no seat, read-only.
- `role == player`, name matches an existing seat's stored name
  (case-insensitive) in that room: reattach to that seat (this *is* the
  reconnect mechanism — no token).
- `role == player`, name unmatched, an open seat remains: assign the next
  open seat; if that fills the last seat, game status moves `waiting` →
  `active`.
- `role == player`, name unmatched, no open seat: return a `SeatsFull` error
  (caller/API surfaces this as 409; client may retry with `role=spectator`).

### 6.4b `Manager.CreateGame` / `GetState` contract

**Elaboration (filling a gap while sequencing backlog item 16):** the plan named `Manager`'s method list but not its exact types or algorithms. Two corrections to the original sketch, both minor and within already-decided architecture:

- The original draft had `CreateGame` return a `RoomSummary` — but `RoomSummary` (used by `ListGames`, a listing type with no single viewer) and what `CreateGame` actually needs to hand back (the caller's own assigned seat/color, matching the REST table's `POST /api/games` response shape `{room_code, seat, color, status}`) are different shapes. `CreateGame` gets its own `CreateGameResult` type instead.
- `Manager` methods take `ctx context.Context` as their first parameter (needed to thread through to every `Store` call) — the original sketch omitted it.

Also added: `Store.ListPlayers`, needed by `GetState` to report every seat (filled or open), not just the one row `CreateGame`/`GetGame` already knew about.

```go
package store // addition to internal/store

// ListPlayers returns every players row for gameID, in seat order.
func (s *Store) ListPlayers(ctx context.Context, gameID int64) ([]Player, error)
```

```go
package session // internal/session, manager.go

type CreateGameResult struct {
    RoomCode string
    Seat     int
    Color    string
    Status   string
}

type SeatInfo struct {
    Seat  int
    Color string
    Name  string // "" if this seat hasn't been filled yet
}

type StateSnapshot struct {
    RoomCode   string
    GameDefID  string
    Status     string
    BoardState string // JSON; the game's current state, or the state at atSeq if given
    TurnSeat   *int
    MoveSeq    int
    WinnerSeat *int
    Seats      []SeatInfo // one entry per seat the game definition declares, filled or not
}

var ErrGameDefNotFound = errors.New("session: game definition not found")

type Manager struct { /* store *store.Store, catalog *catalog.Catalog, room-mutex map + its own sync.Mutex */ }

func NewManager(st *store.Store, cat *catalog.Catalog) *Manager
func (m *Manager) CreateGame(ctx context.Context, gameDefID, creatorName string) (CreateGameResult, error)
func (m *Manager) GetState(ctx context.Context, roomCode string, atSeq *int) (StateSnapshot, error)
```

`CreateGame`:
1. `compiled, ok := m.catalog.Get(gameDefID)` — not found → `ErrGameDefNotFound`.
2. Build the initial `*state.BoardState`: `state.NewBoardState()`, then `Set(board.PositionID(entry.Position), state.Occupant{Piece: entry.Piece, Color: entry.Color})` for every entry in `compiled.Definition.Setup`.
3. `boardJSON, err := bs.MarshalJSON()`.
4. Find seat 1's color in `compiled.Definition.Seats` (error if seat 1 isn't declared — defensive; every real game must have a seat 1).
5. Up to 5 attempts: generate `roomcode.New()`, call `m.store.CreateGame(ctx, code, gameDefID, string(boardJSON), creatorName, seat1Color)`. On success, stop. No attempt at distinguishing "was this failure specifically a room-code collision" from other DB errors — that would require driver-specific error introspection for marginal benefit (a 6-character, 32-symbol space makes a real collision astronomically unlikely; a persistent non-collision DB error will just fail all 5 retries too and correctly surface). After 5 failures, return the last error.
6. Return `CreateGameResult{RoomCode: game.RoomCode, Seat: player.Seat, Color: player.Color, Status: game.Status}`.

`GetState`:
1. `game, err := m.store.GetGame(ctx, roomCode)` — propagate any error (including `store.ErrNotFound`, so callers can `errors.Is` against it directly; `session` doesn't wrap it in a new sentinel).
2. `players, err := m.store.ListPlayers(ctx, game.ID)`.
3. `compiled, ok := m.catalog.Get(game.GameDefID)` (defensive error if missing — shouldn't happen for a game that was successfully created).
4. Build `Seats []SeatInfo`: one entry per `compiled.Definition.Seats` entry, `Name` filled in from the matching `players` row by seat number, or `""` if that seat has no player yet.
5. Resolve `BoardState`: `game.BoardState` normally; if `atSeq != nil`, call `m.store.GetStateAtSeq(ctx, game.ID, *atSeq)` instead and use that (propagate any error, e.g. `store.ErrNotFound` for an out-of-range seq).
6. Return the assembled `StateSnapshot`.

### 6.4c `Manager.JoinGame` contract

**Elaboration (filling a gap while sequencing backlog item 17):** the "Join semantics" bullet list just above this section already fully specifies every branch (spectator, reconnect-by-name, new-seat, seats-full) — nothing here is a new decision, just the Go shape and a matching `Store.AddPlayer` addition. Because every branch is already fully specified (unlike, say, `step`'s two occupancy modes, which were deliberately built one at a time across items 04–05), the `coder` implements the WHOLE method now; backlog items 18–19 add test coverage for the reconnect and seats-full/spectator branches this same implementation already has to get right, rather than adding new code.

```go
package store // addition to internal/store

// AddPlayer inserts one players row (gameID, seat, name, color) and sets the
// game's status in one transaction — both happen or neither does. Used when
// a new (non-reconnecting) player joins; newStatus is whatever the caller
// (Manager) has already decided the game's status should become (e.g.
// "active" once the last seat fills, or unchanged otherwise).
func (s *Store) AddPlayer(ctx context.Context, gameID int64, seat int, name, color, newStatus string) (Player, error)
```

```go
package session // addition to manager.go

type Role string

const (
    RolePlayer    Role = "player"
    RoleSpectator Role = "spectator"
)

type JoinResult struct {
    Role   Role
    Seat   int    // 0 for a spectator
    Color  string // "" for a spectator
    Status string // the game's status after this join
}

var ErrSeatsFull = errors.New("session: seats full")

func (m *Manager) JoinGame(ctx context.Context, roomCode, name string, role Role) (JoinResult, error)
```

`JoinGame`:
1. `game, err := m.store.GetGame(ctx, roomCode)` — propagate any error unwrapped (as `GetState` does).
2. `role == RoleSpectator`: return `JoinResult{Role: RoleSpectator, Status: game.Status}` immediately — no DB write, no seat.
3. `role == RolePlayer`: `players, err := m.store.ListPlayers(ctx, game.ID)`, `compiled, ok := m.catalog.Get(game.GameDefID)` (defensive).
4. If `name` case-insensitively matches an existing player's `Name`: reattach — return `JoinResult{Role: RolePlayer, Seat: <that player's seat>, Color: <that player's color>, Status: game.Status}`, no DB write (this is item 18's test coverage).
5. Else, find the first seat in `compiled.Definition.Seats` (in order) with no matching row in `players`. If every seat is taken: return `ErrSeatsFull` (item 19's test coverage), no DB write.
6. Otherwise: this is a genuinely new seat. Compute `newStatus`: `"active"` if this assignment fills the LAST open seat (every declared seat now has a player), else `game.Status` unchanged. Call `m.store.AddPlayer(ctx, game.ID, openSeat, name, openSeatColor, newStatus, newTurnSeat)` — see 6.4d below for `newTurnSeat` (added there; not needed by item 17's own tests, since nothing checked `TurnSeat` yet, but required for `MakeMove`, sequenced right after, to know whose turn it is on the very first move). Return `JoinResult{Role: RolePlayer, Seat: player.Seat, Color: player.Color, Status: newStatus}` (this is item 17's test coverage — the AC itself).

### 6.4d `Manager.MakeMove` contract

**Elaboration + one gap-fix, found while sequencing backlog item 20:** `AddPlayer` (6.4c) only ever set `status`, never `turn_seat` — so a freshly-activated game would have `turn_seat` stuck at `NULL` forever, and `MakeMove` would have no way to know whose turn the first move is. Fixed by extending `AddPlayer` with one more parameter, `newTurnSeat *int`, non-nil only when this join is the one that activates the game (in which case it's the first seat in `Definition.TurnOrder.Seats`). No existing test pinned `AddPlayer`'s old signature or asserted anything about `TurnSeat`, so this doesn't touch item 17/18/19's already-passing tests.

```go
package store // revises AddPlayer's signature from 6.4c

// AddPlayer inserts one players row and updates the game's status (and,
// only when newTurnSeat is non-nil, its turn_seat too) in one transaction.
func (s *Store) AddPlayer(ctx context.Context, gameID int64, seat int, name, color, newStatus string, newTurnSeat *int) (Player, error)
```

```go
package session // addition to manager.go

type MoveOutcome struct {
    Seq          int
    Seat         int
    Color        string
    From         string
    To           string
    Captured     *string
    NextTurnSeat int
    GameEnded    bool
    WinnerSeat   *int
    Status       string // "active" or "finished" after this move
    BoardState   string // the resulting board state JSON — saves callers (REST/WS) a second GetState round-trip
}

var ErrGameNotActive = errors.New("session: game is not active")
var ErrNotYourTurn = errors.New("session: not your turn")

func (m *Manager) MakeMove(ctx context.Context, roomCode, name string, from, to board.PositionID) (MoveOutcome, error)
```

`MakeMove`:
1. `game, err := m.store.GetGame(ctx, roomCode)` — propagate unwrapped.
2. `game.Status != "active"` → `ErrGameNotActive` (covers both "waiting" and "finished" — this is item 21's coverage).
3. `players, err := m.store.ListPlayers(ctx, game.ID)`.
4. Find the player matching `name` (case-insensitive). If none found, OR `game.TurnSeat == nil`, OR that player's `Seat != *game.TurnSeat`: return `ErrNotYourTurn` (item 21's coverage — this single sentinel covers both "you're not even in this game" and "it's the other seat's turn," matching plan section 7's "a name that doesn't hold the seat whose turn it is").
5. `compiled, ok := m.catalog.Get(game.GameDefID)` (defensive).
6. Deserialize the current state: `bs := state.NewBoardState(); bs.UnmarshalJSON([]byte(game.BoardState))`.
7. `result, err := state.ApplyMove(compiled, bs, player.Seat, from, to)` — an illegal move's error is returned AS-IS, with NO call to `RecordMove` (so nothing is persisted — this is item 21's "illegal move" coverage, for free, since `ApplyMove` itself already guarantees no-mutation-on-error per item 08).
8. `newStateJSON, err := result.NewState.MarshalJSON()`.
9. Convert `result.Captured` (`*board.PositionID`) to `*string` (nil-preserving).
10. `move, err := m.store.RecordMove(ctx, game.ID, store.MoveInput{Seat: player.Seat, Color: player.Color, From: string(from), To: string(to), Captured: capturedStr, ResultingBoardState: string(newStateJSON), NextTurnSeat: result.NextTurnSeat, GameEnded: result.GameEnded, WinnerSeat: result.WinnerSeat})`.
11. `finalStatus := "active"`; if `result.GameEnded`, `finalStatus = "finished"`.
12. Return `MoveOutcome{Seq: move.Seq, Seat: player.Seat, Color: player.Color, From: string(from), To: string(to), Captured: capturedStr, NextTurnSeat: result.NextTurnSeat, GameEnded: result.GameEnded, WinnerSeat: result.WinnerSeat, Status: finalStatus, BoardState: string(newStateJSON)}`.

Like `JoinGame`, every branch here is already fully decided (by `ApplyMove`'s own item-08 guarantees and plan section 7's edge cases) — the `coder` implements the WHOLE method for item 20, and item 21 adds test coverage for the rejection paths (steps 2, 4, and 7) against this same implementation.

### 6.4e Per-room mutex (`Manager` concurrency contract)

**Elaboration (filling a gap while sequencing backlog item 22):** section 6.4 already stated the rule — "`MakeMove` acquires the room's mutex for the full validate → apply → persist → broadcast sequence" — this pins down the actual mechanism, which wasn't specified. Without it, `MakeMove` has a real check-then-act race: two concurrent calls can both read the same pre-move `game.TurnSeat` (step 4 of 6.4d) before either commits, both pass the turn check, and both successfully call `RecordMove` — `RecordMove`'s own transaction prevents a raw `seq` collision (each call reads the current `move_seq` inside its own transaction), but it does NOT prevent two full moves from landing back-to-back when only one should have been allowed, since nothing re-checks "is it still this seat's turn" atomically with the whole read-validate-write sequence.

```go
package session // addition to Manager

type Manager struct {
    store     *store.Store
    catalog   *catalog.Catalog
    mu        sync.Mutex            // guards roomLocks itself, not room state
    roomLocks map[string]*sync.Mutex // lazily created, one per room code ever touched
}

// lockRoom returns an unlock function for roomCode's mutex, creating that
// mutex on first use. It never removes entries (room codes are cheap and
// bounded by how many games actually get created in a process's lifetime;
// not worth the complexity of eviction for v1).
func (m *Manager) lockRoom(roomCode string) (unlock func())
```

`MakeMove` calls `unlock := m.lockRoom(roomCode); defer unlock()` as its very first action — before even `GetGame` — so the entire validate → apply → persist sequence for a given room is strictly serialized in-process. This is sufficient because `Store` is the only writer of the SQLite file (per the interview's decision) — no cross-process locking is needed, only in-process serialization of concurrent goroutines hitting the same room.

### 6.5 Demo game module registration

**Resolved (while sequencing backlog item 09):** the original draft left `Module`'s package as "coder's choice" between `catalog` and `dsl`, with no separate `gamemodule` package. Decision: it lives in `catalog` itself (`catalog.Module`) — catalog is the type's only consumer (something calls `cat.Register(mod.ID, mod.Definition, mod.Callbacks)`), and putting it there avoids a whole extra package for one struct. No cycle: `demo` imports `catalog` (catalog never imports `demo`); `app` (item 10) imports both.

```go
package catalog // added to internal/engine/catalog, alongside the existing Register/Get

type Module struct {
    ID         string
    Definition []byte // embedded demo.json
    Callbacks  callback.GameCallbacks // nil is valid; catalog substitutes NoopCallbacks
}
```

```go
package demo // internal/games/demo

//go:embed demo.json
var definitionJSON []byte

func Module() catalog.Module {
    return catalog.Module{ID: "demo-freeplay", Definition: definitionJSON, Callbacks: nil}
}
```

`internal/app` registers `demo.Module()` into the `Catalog` at startup; adding
a second game later means one more package + one more line here.

### 6.6 SQLite schema

```sql
CREATE TABLE IF NOT EXISTS games (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    room_code     TEXT NOT NULL UNIQUE,
    game_def_id   TEXT NOT NULL,
    status        TEXT NOT NULL DEFAULT 'waiting', -- waiting | active | finished
    board_state   TEXT NOT NULL,                   -- JSON snapshot, current
    turn_seat     INTEGER,                          -- NULL while waiting/finished
    move_seq      INTEGER NOT NULL DEFAULT 0,
    winner_seat   INTEGER,
    created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS players (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    game_id       INTEGER NOT NULL REFERENCES games(id) ON DELETE CASCADE,
    seat          INTEGER NOT NULL,
    name          TEXT NOT NULL,
    color         TEXT NOT NULL,
    joined_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_seen_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(game_id, seat),
    UNIQUE(game_id, name COLLATE NOCASE)
);

CREATE TABLE IF NOT EXISTS moves (
    id                   INTEGER PRIMARY KEY AUTOINCREMENT,
    game_id              INTEGER NOT NULL REFERENCES games(id) ON DELETE CASCADE,
    seq                  INTEGER NOT NULL,
    seat                 INTEGER NOT NULL,
    color                TEXT NOT NULL,
    from_position        TEXT NOT NULL,
    to_position          TEXT NOT NULL,
    captured_position    TEXT,
    resulting_state      TEXT NOT NULL, -- JSON snapshot AFTER this move, for O(1) replay-at-seq
    created_at           TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(game_id, seq)
);

CREATE INDEX IF NOT EXISTS idx_moves_game_seq ON moves(game_id, seq);
```

Spectators are intentionally not persisted (no accounts, ephemeral
read-only connections) — only seated players and moves are durable, matching
"games, every move, players" from the interview.

### 6.6a `Store.CreateGame` / `GetGame` contract

**Elaboration (filling a gap while sequencing backlog item 12):** the plan named the schema but not the Go API on top of it. This pins down `CreateGame`/`GetGame` only — later session-layer backlog items (joining, recording moves, etc.) will need more `Store` methods, added when those items are sequenced, not speculatively now.

```go
package store

type Game struct {
    ID         int64
    RoomCode   string
    GameDefID  string
    Status     string
    BoardState string
    TurnSeat   *int   // nil ⇒ SQL NULL (game is waiting/finished)
    MoveSeq    int
    WinnerSeat *int   // nil ⇒ SQL NULL
    CreatedAt  string // raw SQLite TIMESTAMP text; nothing needs a parsed time.Time yet, so
    UpdatedAt  string // this deliberately skips the driver-specific TEXT↔time.Time scanning
}                      // question until something actually consumes it as a time.Time.

type Player struct {
    ID         int64
    GameID     int64
    Seat       int
    Name       string
    Color      string
    JoinedAt   string
    LastSeenAt string
}

var ErrNotFound = errors.New("store: not found")

// CreateGame inserts one games row (status "waiting", the given board_state,
// move_seq 0, turn_seat and winner_seat left NULL) and one players row for
// the creator at seat 1, in a single transaction — both rows exist or
// neither does. Returns the freshly-inserted Game and Player (re-read after
// commit, so DEFAULT-populated columns like timestamps are accurate).
func (s *Store) CreateGame(ctx context.Context, roomCode, gameDefID, initialBoardState, creatorName, creatorColor string) (Game, Player, error)

// GetGame looks up a game by room code. Returns ErrNotFound (checkable via
// errors.Is) if no game has that room code.
func (s *Store) GetGame(ctx context.Context, roomCode string) (Game, error)
```

A duplicate `roomCode` passed to `CreateGame` (violating the schema's `UNIQUE` constraint) returns that error and leaves no partial row behind (the transaction rolls back) — retry-with-a-fresh-code logic belongs to whatever generates room codes (a later session-layer item), not to `Store` itself.

### 6.6b `Store.RecordMove` contract

**Elaboration (filling a gap while sequencing backlog item 13):** persists the result of an already-computed `state.ApplyMove` call (item 08's `MoveResult`). `Store` stays decoupled from the engine packages — positions are plain strings here, not `board.PositionID` — so persistence has no dependency on `internal/engine/*`; converting between the two is whatever later calls this (the session layer) will do.

```go
package store

type MoveInput struct {
    Seat                int
    Color               string
    From                string
    To                  string
    Captured            *string // nil ⇒ no capture on this move
    ResultingBoardState string  // the JSON board_state after this move
    NextTurnSeat        int     // ignored (games.turn_seat set NULL) if GameEnded
    GameEnded           bool
    WinnerSeat          *int    // nil unless GameEnded
}

type Move struct {
    ID             int64
    GameID         int64
    Seq            int
    Seat           int
    Color          string
    From           string
    To             string
    Captured       *string
    ResultingState string
    CreatedAt      string
}

// RecordMove inserts one moves row (seq = the game's current move_seq + 1,
// read and incremented inside the same transaction) and updates the parent
// games row's board_state/move_seq/turn_seat/winner_seat/status to match, as
// a single transaction — both happen or neither does. If GameEnded, turn_seat
// becomes NULL and status becomes "finished"; otherwise turn_seat becomes
// NextTurnSeat and status is left as whatever it already was (RecordMove
// doesn't itself manage the waiting→active transition — that's a session-
// layer concern, e.g. when a second player joins). Returns store.ErrNotFound
// if gameID doesn't exist.
func (s *Store) RecordMove(ctx context.Context, gameID int64, in MoveInput) (Move, error)
```

### 6.6c `Store.GetStateAtSeq` contract

**Elaboration (filling a gap while sequencing backlog item 14):** the read side of replay — given the `resulting_state` a move recorded (6.6b), fetch it back by sequence number.

```go
// GetStateAtSeq returns the resulting_state JSON stored on the moves row for
// (gameID, seq). Returns store.ErrNotFound if no such row exists — whether
// because gameID itself doesn't exist, or because that seq was never
// recorded for it (including seq=0, which is never a real moves row: a
// fresh game's move_seq starts at 0 but that's the games table's own
// board_state column, not something GetStateAtSeq looks at).
func (s *Store) GetStateAtSeq(ctx context.Context, gameID int64, seq int) (string, error)
```

### REST endpoints

| Method & Path | Body | Response | Notes |
|---|---|---|---|
| `POST /api/games` | `{game_def_id, player_name}` | `{room_code, seat, color, status}` | creates game, creator takes seat 1 |
| `GET /api/games` | — | `[{room_code, game_def_id, status, seats_open}]` | `list_games` equivalent |
| `GET /api/game-defs` | — | `[{id, name, seats_required}]` | discover what can be created |
| `GET /api/games/{code}` | — | `{game_def_id, status, seats:[...], turn_seat}` | |
| `POST /api/games/{code}/join` | `{player_name, role}` | `{role, seat?, color?, state}` | see join semantics |
| `GET /api/games/{code}/state` | `?at_seq=N` optional | `{board_state, turn_seat, status, move_seq}` | replay via `at_seq` |
| `POST /api/games/{code}/moves` | `{player_name, from, to}` | `{move, state}` | 400/403/409 on invalid |
| `GET /api/games/{code}/moves` | `?since_seq=N` optional | `[{seq, seat, color, from, to, captured}]` | full history for replay |
| `GET /api/games/{code}/legal-moves` | `?from=<pos>` | `[<positions>]` | AI convenience |
| `GET /ws/{code}` | query `name`, `role` | WS upgrade | server-push only; see 6.6 WS messages |

### WebSocket message envelope (server → client only; clients never send moves over WS)

```go
type Event struct {
    Type  string      `json:"type"` // "joined" | "state" | "move" | "turn" | "error"
    Data  any         `json:"data"`
}
```

### 6.9 WebSocket implementation contract

**Elaboration (filling gaps while sequencing backlog items 30–32):** the transport (`coder/websocket`, per the earlier dependency decision) and `Event` envelope were already chosen; this pins down the pub-sub mechanism and the `/ws/{code}` handler's exact behavior.

**`internal/session/hub.go`** (new file, per the original file listing in section 3):
```go
package session

// hub is an in-process, per-room publish-subscribe broadcaster. It holds no
// state that needs to survive a restart (unlike everything in Store) — it's
// purely a live-notification side channel; a client that misses an event
// can always re-fetch current state via GetState.
type hub struct {
    mu   sync.Mutex
    subs map[string]map[chan Event]struct{} // roomCode -> set of subscriber channels
}

func newHub() *hub

// subscribe registers a new buffered channel for roomCode and returns it
// (receive-only) plus an unsubscribe function that removes and closes it.
// Buffered (e.g. capacity 16) so a slow consumer doesn't block publish;
// publish uses a non-blocking send and silently drops the event for a
// subscriber whose buffer is full, rather than blocking the whole
// broadcast — an acceptable v1 tradeoff since WS is a convenience layer,
// not the source of truth (persisted Store state always wins).
func (h *hub) subscribe(roomCode string) (events <-chan Event, unsubscribe func())

func (h *hub) publish(roomCode string, ev Event)
```

`Manager` gains an unexported `*hub` field (initialized in `NewManager`) and:
```go
func (m *Manager) Subscribe(roomCode string) (<-chan Event, func()) // delegates to m.hub.subscribe
```

**Publishing:** `MakeMove`, right before returning successfully, calls `m.hub.publish(roomCode, Event{Type: "move", Data: outcome})` (the same `MoveOutcome` the REST/MCP caller gets back). This is the only publish call needed for items 30–32's ACs; `JoinGame` publishing a "joined"/"turn" event to OTHER subscribers is a nice-to-have the coder may add if cheap, but isn't required by any test in this phase — don't over-build it.

**`internal/api/ws.go`** (new file):
```go
func (h *handlers) handleWS(w http.ResponseWriter, r *http.Request)
```
Mounted at `GET /ws/{code}` — this is the FULL handler, built in one pass for item 30 (like `JoinGame`/`MakeMove`, every branch here is already decided, so items 31–32 only add test coverage against this same implementation):
1. Read `code` from the URL param, `name`/`role` from query params (same parsing/validation as `handleJoinGame` — blank `name` or an invalid `role` value is a plain `http.Error` 400, BEFORE upgrading to a WebSocket, since an HTTP error response can't be sent after `websocket.Accept` succeeds).
2. Call `mgr.JoinGame(ctx, code, name, role)` — an error here (e.g. unknown room, seats full) is also a plain pre-upgrade `http.Error` mapped via `statusForError`.
3. `websocket.Accept(w, r, nil)` to upgrade. `defer conn.CloseNow()`.
4. Send `Event{Type: "joined", Data: joinResult}` as the FIRST message on this connection (this is backlog item 30's AC — the coder should verify empirically which `coder/websocket` helper writes JSON, e.g. `wsjson.Write`, since exact API names can drift between versions; check the installed module's actual API via `go doc` rather than assuming).
5. `events, unsubscribe := mgr.Subscribe(code)`; `defer unsubscribe()` — this is what item 32 tests: closing the connection must reliably call this.
6. Loop: for each `ev := range events`, write it to the connection; if the write errors (client disconnected), return (which triggers the deferred `unsubscribe()`/`conn.CloseNow()`). Since clients never send anything over this connection (server-push only), use whatever `coder/websocket` idiom exists for detecting a client-initiated close on a send-only connection (e.g. a background read loop via `conn.CloseRead(ctx)` in some versions of this library) — again, verify against the actually-installed version rather than assuming a specific method name.

Mounted in `internal/app`'s router at `/ws/{code}` (a sibling of the `/api` mount, not nested under it — matches the endpoint table's `GET /ws/{code}` path, no `/api` prefix).

**Addendum for backlog item 31 (move broadcast):** `MakeMove`, right before returning successfully, now actually calls `m.hub.publish(roomCode, Event{Type: "move", Data: <wire DTO>})`. That `Data` must NOT be the raw `MoveOutcome` — it has no `json` tags, and its `BoardState` field is a JSON-text string that would be sent double-encoded (an escaped string, not a nested object) the same way REST's handlers had to special-case with `json.RawMessage`. Define a small, properly-tagged wire struct for this purpose (in `internal/session`, since `publish` is called from inside `MakeMove`) mirroring REST's `POST .../moves` response shape: `seq`, `seat`, `color`, `from`, `to`, `captured`, `turn_seat` (nullable — `nil` when the move ended the game), `game_ended`, `winner_seat`, `status`, and `board_state` as `json.RawMessage`.

### MCP tool set

| Tool | Args | Returns | Purpose |
|---|---|---|---|
| `list_game_defs` | — | game def summaries | what an AI can create |
| `create_game` | `game_def_id, player_name` | `room_code, seat, color` | start a game |
| `list_games` | `status?` | room summaries | find a known/open game |
| `join_game` | `room_code, player_name, role` | seat/color or spectator confirmation | join as player or spectator |
| `get_state` | `room_code` | board state, turn, status | poll current position |
| `list_legal_moves` | `room_code, from` | destination positions | avoid illegal `make_move` calls |
| `make_move` | `room_code, player_name, from, to` | move result + new state | play a turn |
| `get_moves` | `room_code, since_seq?` | move history | catch-up / replay |

All eight tools call `session.Manager` in-process (no HTTP loopback), so MCP
and REST share identical validation and produce identical persisted state.

### 6.10 MCP implementation contract

**Elaboration (filling gaps while sequencing backlog items 33–36):** the tool table above gives args/returns at a business level; this pins down the JSON shape per tool (deliberately mirroring the REST DTOs already built in `internal/api`, so a human using the web UI and an AI using MCP see the same shapes) and the package structure. **`github.com/mark3labs/mcp-go` is not yet a dependency** — add it via `go get`, and verify its actual API for registering tools, building results, and serving over Streamable HTTP via `go doc` (its exact function/type names may not match older docs or training data — check the installed version empirically, the same discipline already used for `coder/websocket` in section 6.9 and the SQLite FK pragma in 6.6). This is real integration risk on an already-decided dependency choice (ADR/decision made in the interview), not a new architectural question — don't guess, verify, and expect this phase may need more back-and-forth than most.

**Package `internal/mcpserver`** (files `server.go`, `tools.go` per the original file listing): `func NewServer(mgr *session.Manager) http.Handler` — a Streamable-HTTP MCP server, mounted at `/mcp` in `internal/app` (a sibling of `/api` and `/ws/{code}`, matching the "no `/api` prefix" pattern already used for `/ws`).

**Per-tool JSON shapes** (tool name → args → result `Data`, all built from the SAME `session.Manager` methods and wire-DTO conventions REST already established — board state as nested JSON, nullable ints as `*int`, etc.):

- `list_game_defs` — args: none. Result: `[{"id","name","seats_required"}]` (from `Manager.ListGameDefs`).
- `create_game` — args: `{game_def_id, player_name}`. Result: `{"room_code","seat","color","status"}` (from `Manager.CreateGame`'s `CreateGameResult` — same shape as `POST /api/games`'s response).
- `list_games` — args: `{status?}`. Result: `[{"room_code","game_def_id","status","seats_open"}]` (from `Manager.ListGames`).
- `join_game` — args: `{room_code, player_name, role}` (role defaults to `"player"` if blank/omitted, same validation as REST). Result: `{"role","seat","color","status"}` (`seat`/`color` omitted for a spectator — same as `POST /games/{code}/join`).
- `get_state` — args: `{room_code}`. Result: `{"board_state","turn_seat","status","move_seq"}` (from `Manager.GetState`; no `at_seq` param for MCP — that's a REST-only replay convenience, not in the tool table).
- `list_legal_moves` — args: `{room_code, from}`. Result: `["<pos>", ...]` (from `Manager.ListLegalMoves`).
- `make_move` — args: `{room_code, player_name, from, to}`. Result: `{"move": {"seq","seat","color","from","to","captured"}, "state": {"board_state","turn_seat","status","move_seq"}}` (from `Manager.MakeMove`'s `MoveOutcome` — same shape as `POST /games/{code}/moves`; this call also triggers `hub.publish`, exactly like the REST path, since both go through the same `Manager.MakeMove` — item 35's AC, "a human spectator's WebSocket connection receives the corresponding move event," should fall out for free from calling the same method, not need separate wiring).
- `get_moves` — args: `{room_code, since_seq?}`. Result: `[{"seq","seat","color","from","to","captured"}]` (from `Manager.ListMoves`).

Every tool maps `Manager`/`store` errors to a tool-level error result (however `mark3labs/mcp-go` represents a failed tool call — verify via `go doc`) rather than a transport-level HTTP error, since MCP tool calls are always "successful" RPCs that can report a business-logic failure in their result, unlike REST's status-code convention. Blank/whitespace required string args get the same pre-call 400-equivalent rejection REST uses (reject before calling `Manager`).

### 6.8 REST API implementation contract

**Elaboration (filling gaps while sequencing backlog items 24–29):** the endpoint table above gives shapes; this pins down the Go types, the `Manager`/`Catalog` methods the handlers need (none of which exist yet), and error→status mapping. Every endpoint is built on already-decided `Manager` methods or small, obvious additions to them — nothing here changes prior behavior.

**New `Catalog` method:**
```go
func (c *Catalog) List() []*CompiledGame // all registered games, sorted by ID for determinism
```

**New `Store` methods** (internal/store, same transaction-free read-query style as `GetStateAtSeq`):
```go
func (s *Store) ListGames(ctx context.Context, status string) ([]Game, error) // status == "" means all
func (s *Store) ListMoves(ctx context.Context, gameID int64, sinceSeq int) ([]Move, error) // seq > sinceSeq, ascending
```

**New `Manager` methods** (internal/session/manager.go):
```go
type GameDefSummary struct {
    ID            string
    Name          string
    SeatsRequired int
}
func (m *Manager) ListGameDefs(ctx context.Context) []GameDefSummary // from catalog.List(), no error possible

type RoomSummary struct {
    RoomCode  string
    GameDefID string
    Status    string
    SeatsOpen int
}
func (m *Manager) ListGames(ctx context.Context, status string) ([]RoomSummary, error)
// per game from store.ListGames: SeatsOpen = len(declared seats) - len(ListPlayers(gameID))

type MoveRecord struct {
    Seq      int
    Seat     int
    Color    string
    From     string
    To       string
    Captured *string
}
func (m *Manager) ListMoves(ctx context.Context, roomCode string, sinceSeq int) ([]MoveRecord, error)
// GetGame(roomCode) for the id, then store.ListMoves

func (m *Manager) ListLegalMoves(ctx context.Context, roomCode string, from board.PositionID) ([]board.PositionID, error)
// GetGame + catalog.Get + deserialize BoardState; if `from` has no occupant, return an empty
// slice (not an error — this is an AI convenience query, not a move attempt); otherwise resolve
// the occupant's piece and, for every position in compiled.Board.Neighbors(from), invoke that
// piece's primitive with that position as `to` and keep it if Result.Allowed.
```

**Package `internal/api`** (files `router.go`, `games.go`, `errors.go` — no `ws.go` yet, that's the WebSocket phase):

```go
func NewRouter(mgr *session.Manager) chi.Router // routes relative to its mount point — NO "/api" prefix
                                                  // baked in here; internal/app mounts it at "/api"
```

Routes (chi, `{code}` is the room code path param): `POST /games`, `GET /games` (optional `?status=`), `GET /game-defs`, `GET /games/{code}`, `POST /games/{code}/join`, `GET /games/{code}/state` (optional `?at_seq=N`), `POST /games/{code}/moves`, `GET /games/{code}/moves` (optional `?since_seq=N`), `GET /games/{code}/legal-moves` (`?from=<pos>`, required).

Every JSON response embeds board state as a nested `json.RawMessage`, not a doubly-encoded string — `StateSnapshot.BoardState`/`MoveOutcome.BoardState` are already JSON strings from `BoardState.MarshalJSON`, so handlers do `json.RawMessage(snapshot.BoardState)` when building the response, not a plain string field.

Request/response bodies (all error responses are `{"error": "<message>"}`; blank/whitespace-only `player_name` on create or join is rejected with 400 before even calling `Manager`, per plan section 7):

- `POST /games` — req `{"game_def_id", "player_name"}` → 200 `{"room_code", "seat", "color", "status"}` (from `CreateGameResult`).
- `GET /game-defs` → 200 `[{"id", "name", "seats_required"}]`.
- `GET /games` (`?status=`) → 200 `[{"room_code", "game_def_id", "status", "seats_open"}]`.
- `GET /games/{code}` → 200 `{"game_def_id", "status", "seats": [{"seat","color","name"}], "turn_seat"}` (from `StateSnapshot`, minus `board_state`/`move_seq` — those live at `.../state`).
- `POST /games/{code}/join` — req `{"player_name", "role"}` (`role` defaults to `"player"` if blank) → 200 `{"role", "seat", "color", "status"}` (`seat`/`color` omitted — `omitempty` — for a spectator).
- `GET /games/{code}/state` (`?at_seq=N`) → 200 `{"board_state": <raw JSON>, "turn_seat", "status", "move_seq"}`.
- `POST /games/{code}/moves` — req `{"player_name", "from", "to"}` → 200 `{"move": {"seq","seat","color","from","to","captured"}, "state": {"board_state": <raw JSON>, "turn_seat", "status", "move_seq"}}`.
- `GET /games/{code}/moves` (`?since_seq=N`) → 200 `[{"seq","seat","color","from","to","captured"}]`.
- `GET /games/{code}/legal-moves` (`?from=<pos>`) → 200 `["<pos>", ...]`.

**Error→status mapping** (`internal/api/errors.go`):
```go
func statusForError(err error) int {
    switch {
    case errors.Is(err, store.ErrNotFound), errors.Is(err, session.ErrGameDefNotFound):
        return http.StatusNotFound
    case errors.Is(err, session.ErrSeatsFull), errors.Is(err, session.ErrGameNotActive):
        return http.StatusConflict
    case errors.Is(err, session.ErrNotYourTurn):
        return http.StatusForbidden
    default:
        return http.StatusBadRequest
    }
}
```
**Known, deliberate v1 simplification:** the `default` case covers both `ApplyMove`'s illegal-move rejections (genuinely a 400 — the intended behavior) and any truly unexpected internal error (which "should" be a 500). `ApplyMove` doesn't use a typed/sentinel error for its rejection reasons, so the two aren't distinguishable here without adding one. Given this is a casual, low-security app, this is an accepted simplification, not an oversight — revisit only if a real need for precise 5xx reporting shows up later.

**Wiring into `internal/app`** (needed starting with item 24, since REST tests go through `app.Router()` per the testing seam in 6.7): `App` gains a `*session.Manager` field, built in `New(cfg)` as `session.NewManager(store, catalog)` right after the catalog is built; `App.newRouter()` mounts `api.NewRouter(manager)` at `/api` alongside the existing `/healthz`.

### 6.11 Web UI implementation contract

**Elaboration (filling gaps while sequencing backlog items 37–40).**

**Test tooling decision (made with the user, not unilaterally):** items 38–40's ACs describe rendered-DOM/live-update behavior that a Go-only HTTP test can't verify (it can check served HTML/JS *source*, not what a browser does after executing that JS). The user chose to add `github.com/chromedp/chromedp` (pure-Go headless-Chrome driver) so tests can verify real rendered behavior. **Environment note for whoever runs these tests locally on this particular dev machine (Bazzite, immutable Fedora-based):** there's no `chromium`/`google-chrome` binary on `$PATH` by default; Chromium here is installed as a flatpak (`org.chromium.Chromium`, via Bazzite's "Bazaar" app store) and invoked via `flatpak run org.chromium.Chromium`. A wrapper script at `~/.local/bin/chromium` (`exec flatpak run org.chromium.Chromium "$@"`, already on `$PATH`) makes it discoverable under the name chromedp's default `ExecAllocator` already searches for, so `chromedp.NewExecAllocator(ctx, chromedp.DefaultExecAllocatorOptions[:]...)` should just find it with no special-casing in test code. This wrapper is host-environment setup, not project code — it doesn't belong in the repo, and CI (a normal Linux runner) will have `chromium`/`google-chrome` on `PATH` directly with no such wrapper needed. If a test can't find a browser, that's a local-environment problem to flag, not a reason to weaken the test.

**Known host-environment quirk — orphaned Chrome processes:** on this flatpak-Chromium setup, killing the browser (context cancellation/timeout at test end) does not reliably terminate the real Chrome process tree underneath the `flatpak run`/`bwrap` sandbox wrapper — likely because chromedp only signals the immediate child it spawned (the wrapper script → `flatpak run`), and that signal doesn't cascade through `bwrap`'s own process tree to the actual sandboxed `chrome` processes. Over many test runs this silently accumulated 385 orphaned headless-Chrome processes and filled `/run/user/1000` (tmpfs) to 100%, which is what caused item 38's tests to fail with "no space left on device" — a host problem, not a code bug, and already cleaned up once. If browser tests start failing with a similar disk/resource error again, check `pgrep -c chromedp-runner` and `df -h /run/user/1000` before assuming it's a code regression; clean up via `pkill -9 -f chromedp-runner` and `rm -rf /run/user/1000/.flatpak/org.chromium.Chromium/tmp/*` (safe — this is disposable per-test-run sandbox data, not project or user data) if it recurs. This is purely a local-dev-machine concern; CI runners start clean every time and won't accumulate this.

**Package `internal/web`** (files `router.go`, `templates/index.html.tmpl`, `templates/game.html.tmpl`, `static/app.js`, `static/board.js`, `static/style.css`, per the original file listing): `func NewRouter(mgr *session.Manager) http.Handler` — `GET /` renders `index.html.tmpl`; `GET /game/{code}` renders `game.html.tmpl`; static files served under whatever path the templates reference (e.g. `/static/...`). Mounted in `internal/app` at `/` (i.e. as the router's catch-all/root, a sibling of `/api`, `/ws/{code}`, `/mcp` — mount the more specific paths first so they take precedence, then `internal/web`'s router last/as the fallback).

- **`index.html.tmpl`** (item 37): server-rendered with the current `Manager.ListGameDefs()` result (so the create form can offer a real dropdown, even though only `demo-freeplay` exists today) — a create-game form (`game_def_id` select, `player_name` text input) that a small bit of `app.js` intercepts on submit, POSTs to `/api/games` via `fetch`, and on success navigates to `/game/{room_code}`; and a join-existing-game form (room code + player name) that POSTs to `/api/games/{code}/join` and navigates to `/game/{code}` — the title says "create/join," but item 37's AC is specifically about the create form/POST, so that's the part to get exactly right; the join form is reasonable but secondary scope.
- **`game.html.tmpl`** (item 38): a minimal shell — a container element for the board (e.g. `<div id="board">` or an inline `<svg>` `board.js` populates), loads `board.js`. Item 38's AC is specifically "fetches and displays the raw JSON state" — the simplest thing that satisfies this without yet doing SVG work is `board.js` doing `fetch('/api/games/' + code + '/state')` and rendering *something* observable from that response (even pre-SVG, e.g. into a debug element) — item 39 is what turns it into real SVG.
- **`board.js`** (items 39–40):
  - Item 39: after fetching state, render one SVG shape (e.g. a `<circle>`) per OCCUPIED position from `board_state`, colored by that occupant's `color`. It does NOT need a separate board-topology endpoint — the demo game's position IDs are already `"x,y"` grid coordinates (per `board.Generate`'s `gridPositionID`), so `board.js` can parse each occupied key as `x,y` and place its shape directly at those grid coordinates. This is a deliberate demo-game-specific simplification (a non-grid future game's IDs wouldn't parse this way) — acceptable for v1 since only the demo game exists; revisit if/when a hex/irregular-board game is ever added.
  - Item 40: also opens a WebSocket to `/ws/{code}?name=...&role=spectator` (a page-load spectator connection — this page doesn't have move-submission UI in v1, since that's not named in any of items 37–40; a human plays via whatever REST/MCP client they like, and any open `/game/{code}` tab just displays live state) and re-renders the SVG whenever a `"move"` event arrives, using that event's `board_state` — no page reload, no polling.

### 6.12 Drag-and-drop move submission (post-v1-backlog addition)

**Context:** after the full 42-item backlog shipped and a first real look at the UI, item 40's "no move-submission UI in v1" scope call turned out to make the demo genuinely unplayable by a human through the browser — you could watch a game live but never actually move a piece. Added by direct request, not a new item in `docs/history/backlog.md` (that list was for the original scoped plan, now complete) — documented here the same way the grid/stroke visual fix was, as a follow-up design note.

**No backend/engine changes are needed.** `POST /api/games/{code}/moves` (item 27) already validates and applies every move, including captures — `step`'s `capture-or-empty` occupancy mode (items 05/08) has been exercised by dozens of tests since early in the project. The only gap is client-side: the game page had no way to *submit* a move, and no concept of "which player am I" at all (it only ever opened a random-named, read-only spectator connection).

**Identity threading:** `app.js`'s create/join success handlers now navigate to `/game/{code}?name=<player_name>` instead of a bare `/game/{code}` (URL-encoded). `board.js` reads `?name=` via `URLSearchParams`. If present, it opens its WebSocket as `role=player` with that name instead of a random spectator name — `JoinGame`'s existing reconnect-by-name semantics (item 18) mean this safely reattaches to whatever seat that name already holds, no new server behavior required. If `?name=` is absent (someone opens a bare `/game/{code}` link), behavior is unchanged from item 40: an anonymous read-only spectator, and dragging is a no-op (there's no name to submit a move as).

**Drag-and-drop mechanics**, entirely in `board.js`:
- `mousedown` on a rendered piece `<circle>` starts a drag, recording its `data-position` as `from`.
- `mousemove` while dragging moves that circle's `cx`/`cy` to follow the cursor, mapped via the SVG element's `getBoundingClientRect()` (valid 1:1 pixel↔SVG-user-unit mapping today since the `<svg>`'s `width`/`height` attributes equal its `viewBox` and no CSS scales it — revisit with a proper `getScreenCTM()` transform if the board ever becomes responsively resized).
- `mouseup` computes the nearest grid cell from the drop point as `to`, and — only if a player name is known — `POST`s `{player_name, from, to}` to `/api/games/{code}/moves`.
- No optimistic local state mutation beyond the piece already following the cursor during the drag: on a rejected move (400/403/409 — wrong turn, illegal move, spectator with no name, game not active), the piece is simply re-rendered from the last known confirmed state (snapping back) and a brief `#move-error` message is shown, mirroring the existing `#board-error` pattern from item 38. On a successful move, nothing bespoke happens — the same WebSocket connection (now joined as this player) receives the broadcast `"move"` event exactly like every other open tab, and the existing `renderState` re-render (item 40) is what actually reflects the confirmed position, so a human's own drag and a second spectator tab's live view are always driven by the identical code path.

### 6.7 Testing seam (binding constraint for the coder)

`internal/app.New(cfg)` must be fully usable with `cfg.DBPath` pointing at a
temp file (e.g. `t.TempDir()+"/test.db"`) and must return a `Router()
http.Handler` that can be wrapped in `httptest.NewServer` with **zero**
real listeners opened by `App` itself. `main.go` is the only place that binds
a port (via `internal/httpserve.Serve`). This is what lets the tester agent
(restricted to `tests/`) write full black-box integration tests — REST, WS,
and MCP — against an in-process instance without touching application code.

## 7. Edge Cases

- Room code not found → 404 (REST) / tool error (MCP); no state change.
- Move submitted while `status != active` (still `waiting`, or `finished`) →
  409, no state change.
- Move submitted by a name that is not the seat whose `turn_seat` it is →
  403, no state change.
- Move where `from` has no piece, `to` is not graph-adjacent to `from`, `to`
  is occupied by the mover's own color, or occupancy mode forbids landing on
  an opponent → 400 with a specific reason, no DB write.
- Two near-simultaneous `MakeMove` calls for the same room → exactly one
  succeeds (serialized by the per-room mutex); the other gets a definite
  reject (stale turn / not-your-turn), never a corrupted or merged state.
- Two joins using the identical name in the same room → treated as the same
  person reconnecting to their seat (accepted risk per the interview's
  no-auth decision; not a bug to fix here).
- Room code generation collision → retry with a fresh random code (bounded
  retries, then error — extremely unlikely with a 6-char space).
- Player joins as `role=player` when both seats are already taken by other
  names → `SeatsFull` (409); client may retry with `role=spectator`.
- Spectator attempts `POST /api/games/{code}/moves` → 403 (no seat to move
  from).
- WebSocket client disconnects → hub removes the subscriber; no persisted
  state change; a later reconnect re-runs join logic idempotently.
- Server process restarts mid-game → in-memory session cache is empty; the
  next request for that room code lazily rehydrates full state (`board_state`,
  `move_seq`, `turn_seat`, seats) from SQLite — no data loss, no re-creation
  of the room.
- Game definition references an unknown primitive name → fails at
  `Catalog.Register` (startup / test-construction time), not at first move —
  server refuses to start rather than serving a broken game.
- TLS configured with a cert or key path that doesn't exist/can't be read →
  `httpserve.Serve` returns an error (not `os.Exit`); only `main.go` turns
  that into a fatal log + nonzero exit.
- Blank/whitespace-only `player_name` on create or join → 400.
- Move referencing a board position not present in the compiled board graph
  → 400.

## 8. Acceptance Criteria

**Engine / DSL**
1. Parsing a well-formed game-definition JSON produces a `GameDefinition`
   with no error.
2. Registering a game definition that references a movement primitive name
   not in the primitive registry returns a non-nil error from
   `Catalog.Register` and the game is not added to the catalog.
3. `board.Generate` with `mode=orthogonal4` on a 9x9 grid produces exactly 4
   neighbors for every interior position and fewer for edge/corner positions.
4. The `step` primitive with `occupancy=empty-only` rejects a destination
   occupied by any piece, own or opponent's.
5. The `step` primitive with `occupancy=capture-or-empty` allows moving onto
   an opponent-occupied destination and reports that destination as
   `Captured`; it rejects moving onto a same-color-occupied destination.

**Persistence**
6. Creating a game inserts exactly one row into `games` and one row into
   `players` (the creator's seat).
7. A successfully applied move inserts exactly one row into `moves` with
   `seq` one greater than the previous move for that game, and updates the
   `games` row's `board_state`, `turn_seat`, and `move_seq` accordingly.
8. `GET /api/games/{code}/state?at_seq=N` returns the exact board state
   captured in `moves.resulting_state` for the move with `seq=N`, for every
   `N` from 1 through the current move count.
9. Restarting the process (constructing a fresh `internal/app.App` against
   the same SQLite file) and then requesting a previously-created room's
   state returns the same `board_state`, `turn_seat`, and `move_seq` as
   before the restart.

**Room / session flow**
10. `POST /api/games` returns a 6-character room code, and a subsequent
    `GET /api/games/{code}` reports `status=waiting` and exactly one seat
    filled.
11. A second distinct player calling `POST /api/games/{code}/join` with
    `role=player` and a new name is assigned the second seat, and the game's
    status becomes `active`.
12. A player re-joining with the exact name already on seat 1 is reattached
    to seat 1 (not assigned a new seat, not rejected).
13. A third distinct player attempting to join an already-full game with
    `role=player` receives a 409 `SeatsFull` response and no seat.
14. A client joining with `role=spectator` receives read-only confirmation
    (no seat, no color) and never appears in the `players` table.

**REST API**
15. An invalid move (e.g., non-adjacent `to`, or `to` occupied by the mover's
    own color) returns 400 and a subsequent `GET .../state` shows the board
    state unchanged (byte-for-byte equal to before the attempt).
16. A move attempted by a name that does not currently hold the seat whose
    turn it is returns 403 and does not change `turn_seat` or `board_state`.
17. A move attempted while the game is `waiting` (only one seat filled)
    returns 409 and does not change persisted state.
18. A legal move returns 200 with the applied move and resulting state, and
    `turn_seat` in the response equals the *other* seat immediately after.
19. `GET /api/games/{code}/moves` returns moves in ascending `seq` order
    matching exactly what was submitted, including any `captured` position.

**WebSocket**
20. A client connected to `/ws/{code}` receives a `move` event containing
    the new board state within the same request/response cycle as a REST
    `make_move` call from a different connection to the same room (i.e., no
    polling required to observe it).
21. Closing a WebSocket connection does not affect the room's persisted
    state or other subscribers' connections.

**MCP**
22. `list_game_defs` returns at least the `demo-freeplay` definition.
23. `create_game` via MCP produces a room joinable via the REST API with the
    same room code (both surfaces share persisted state).
24. `make_move` invoked via MCP by an AI holding a seat, on that seat's turn,
    updates the board state such that a human spectator's open WebSocket
    connection to the same room receives the corresponding `move` event.
25. `make_move` invoked via MCP for a name that holds no seat in the given
    room returns a tool-level error and does not change board state.

**Demo game**
26. The demo game's initial state (after `create_game`/`POST /api/games`)
    places at least one stone of each color on the board per `demo.json`'s
    `setup`.
27. Moving a demo stone onto an empty adjacent position succeeds and leaves
    the piece count for both colors unchanged.
28. Moving a demo stone onto an adjacent position occupied by the opposing
    color succeeds, removes the captured piece, and decreases the opponent's
    piece count on the board by exactly one.
29. The demo game never reports `status=finished` due to a win condition
    (there is none) regardless of how many moves are played — the game only
    ends by process/administrative means, not rules.

**Concurrency / robustness**
30. Firing two concurrent `make_move` requests for the same room and the
    same turn (from the correct seat, racing against each other) results in
    exactly one recorded move (`move_seq` advances by exactly 1, not 2) and
    the other request receiving a non-2xx response.
31. `httpserve.Serve` returns a non-nil error (does not panic or exit the
    process) when given a TLS config pointing at a nonexistent certificate
    file.

## 9. Out of Scope for This Plan (confirmed by the interview, restated for the coder)

- No auth/RBAC, no reconnection tokens, no accounts.
- No enforced turn timer/clock.
- No second game beyond `demo-freeplay`.
- No lobby-browsing UI in the web frontend (room codes are shared out of
  band).
- No animations/audio.
