// Package envs provides in-memory, gym-style game environments for training
// and evaluating agents (reinforcement learning, search, LLM policies)
// through the MCP server.
//
// Unlike persisted rooms (internal/session), an environment keeps its
// position only in memory: stepping costs a move application and nothing
// else, no database round trip and no room code. An environment is a
// single-agent view of a game: the agent controls one seat, a built-in bot
// (or nothing, for self-play) controls the others, and the opponent's reply
// is played inside Step so each call returns the agent's next decision
// point directly.
package envs

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"sync"
	"time"

	"github.com/noderunner/bingle/internal/bots"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

// Errors returned by the manager.
var (
	ErrNotFound    = errors.New("envs: environment not found")
	ErrTooMany     = errors.New("envs: too many live environments")
	ErrGameOver    = errors.New("envs: the episode is over; call reset or fork")
	ErrUnknownGame = errors.New("envs: unknown game definition")
	ErrUnknownBot  = errors.New("envs: unknown opponent")
)

const (
	defaultMaxEnvs  = 256
	defaultIdleTTL  = 30 * time.Minute
	defaultMaxPlies = 1000
)

// ResetOptions configures a new episode.
type ResetOptions struct {
	GameDefID string
	// Seat is the agent's seat (default 1).
	Seat int
	// Opponent is a bot ID to play every other seat, or "" / "self" for
	// self-play where the agent moves for whoever is to move.
	Opponent string
	// Seed makes bot behaviour reproducible (0 = random).
	Seed int64
	// MaxPlies truncates the episode (default 1000).
	MaxPlies int
	// StartTPS optionally starts from a position given in TPS (games that
	// support it, e.g. Tak) instead of the standard start.
	StartTPS string
	// StartState optionally starts from a position document (any game);
	// StartSeat says who moves first (0 = the game's first seat).
	StartState string
	StartSeat  int
	// BotIterations, when positive and the opponent is "mcts", limits its
	// search to that many iterations per move instead of ~1 s of wall-clock
	// time: faster, and reproducible for a given Seed.
	BotIterations int
	// Planes adds a numeric board encoding (state.Planes) to every
	// observation; ActionIDs adds each legal move's fixed action ID.
	// PlaneDepth is the number of stack levels encoded (0 = automatic).
	Planes     bool
	ActionIDs  bool
	PlaneDepth int
}

// Env is one live environment. Its methods are not safe for concurrent
// use; Manager serialises access.
type Env struct {
	mu       sync.Mutex // serialises Step/Observe/Fork on this environment
	ID       string
	game     *catalog.CompiledGame
	st       *state.BoardState
	toMove   int
	agent    int
	opp      bots.Bot // nil for self-play
	oppID    string
	oppIters int
	seed     int64
	maxPlies int

	planes     bool
	actionIDs  bool
	planeDepth int

	history []string
	done    bool
	winner  *int
	reason  string
	trunc   bool
	last    string
	touched time.Time
}

// Observation is what an agent sees after a reset or step.
type Observation struct {
	EnvID      string   `json:"env_id"`
	GameDefID  string   `json:"game_def_id"`
	AgentSeat  int      `json:"agent_seat"`
	AgentColor string   `json:"agent_color"`
	ToMove     int      `json:"to_move"`
	YourTurn   bool     `json:"your_turn"`
	Ply        int      `json:"ply"`
	BoardText  string   `json:"board_text"`
	TPS        string   `json:"tps,omitempty"`
	State      any      `json:"state"`
	LegalMoves []string `json:"legal_moves"`
	History    []string `json:"history"`
	// LastMoves are the moves played since the agent's previous
	// observation: its own move followed by any bot replies.
	LastMoves []string `json:"last_moves,omitempty"`
	Done      bool     `json:"done"`
	// Truncated is true when the episode ended by hitting max_plies.
	Truncated  bool    `json:"truncated"`
	WinnerSeat *int    `json:"winner_seat"`
	EndReason  string  `json:"end_reason,omitempty"`
	Reward     float64 `json:"reward"`
	Result     string  `json:"result,omitempty"`

	// Planes is the numeric encoding of the position from the agent's
	// point of view (only with the planes option).
	Planes *state.Planes `json:"planes,omitempty"`
	// LegalActionIDs are the fixed action IDs of LegalMoves, in the same
	// order (only with the action_ids option and a fixed action space).
	LegalActionIDs []int `json:"legal_action_ids,omitempty"`
	// NumActions is the size of the fixed action space (0 = dynamic).
	NumActions int `json:"num_actions,omitempty"`
}

// Manager owns the live environments.
type Manager struct {
	catalog *catalog.Catalog

	mu     sync.Mutex
	envs   map[string]*Env
	maxEnv int
	ttl    time.Duration
	rng    *rand.Rand
}

// NewManager returns a Manager over cat.
func NewManager(cat *catalog.Catalog) *Manager {
	return &Manager{
		catalog: cat,
		envs:    map[string]*Env{},
		maxEnv:  defaultMaxEnvs,
		ttl:     defaultIdleTTL,
		rng:     rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (m *Manager) newID() string {
	const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 8)
	for i := range b {
		b[i] = alphabet[m.rng.Intn(len(alphabet))]
	}
	return "env-" + string(b)
}

// gc drops idle environments. Callers hold m.mu.
func (m *Manager) gc() {
	cutoff := time.Now().Add(-m.ttl)
	for id, e := range m.envs {
		if e.touched.Before(cutoff) {
			delete(m.envs, id)
		}
	}
}

// Reset starts a new episode and returns its first observation.
func (m *Manager) Reset(ctx context.Context, opts ResetOptions) (Observation, error) {
	game, ok := m.catalog.Get(opts.GameDefID)
	if !ok {
		return Observation{}, fmt.Errorf("%w: %q", ErrUnknownGame, opts.GameDefID)
	}
	seat := opts.Seat
	if seat == 0 {
		seat = 1
	}
	if _, ok := game.Seat(seat); !ok {
		return Observation{}, fmt.Errorf("envs: game %q has no seat %d", opts.GameDefID, seat)
	}
	maxPlies := opts.MaxPlies
	if maxPlies <= 0 {
		maxPlies = defaultMaxPlies
	}

	e := &Env{
		game: game, agent: seat, maxPlies: maxPlies, seed: opts.Seed, touched: time.Now(),
		planes: opts.Planes, actionIDs: opts.ActionIDs, planeDepth: opts.PlaneDepth,
	}
	if opts.Planes {
		if _, _, _, err := state.PlaneChannels(game, state.PlaneOptions{Depth: opts.PlaneDepth}); err != nil {
			return Observation{}, err
		}
	}
	if opts.Opponent != "" && opts.Opponent != "self" {
		bot, ok := bots.MakeConfigured(opts.Opponent, opts.Seed, bots.Config{MCTSIterations: opts.BotIterations})
		if !ok {
			return Observation{}, fmt.Errorf("%w: %q", ErrUnknownBot, opts.Opponent)
		}
		e.opp, e.oppID, e.oppIters = bot, opts.Opponent, opts.BotIterations
	}

	switch {
	case opts.StartTPS != "":
		st, toMove, err := state.ParseTPS(game, opts.StartTPS)
		if err != nil {
			return Observation{}, err
		}
		e.st, e.toMove = st, toMove
	case opts.StartState != "":
		st := state.NewBoardState()
		if err := st.UnmarshalJSON([]byte(opts.StartState)); err != nil {
			return Observation{}, fmt.Errorf("envs: bad start_state: %w", err)
		}
		if err := state.Validate(game, st); err != nil {
			return Observation{}, err
		}
		toMove := opts.StartSeat
		if toMove == 0 {
			toMove = state.FirstSeat(game)
		}
		if _, ok := game.Seat(toMove); !ok {
			return Observation{}, fmt.Errorf("envs: game %q has no seat %d", opts.GameDefID, toMove)
		}
		e.st, e.toMove = st, toMove
	default:
		if state.IsRuleset(game) && opts.Seed != 0 {
			// Games with chance start from the env's seed so that a seeded
			// environment replays exactly.
			st, err := state.InitialStateSeeded(game, uint64(opts.Seed))
			if err != nil {
				return Observation{}, err
			}
			e.st, e.toMove = st, state.FirstSeat(game)
		} else {
			e.st, e.toMove = state.InitialState(game), state.FirstSeat(game)
		}
	}

	m.mu.Lock()
	m.gc()
	if len(m.envs) >= m.maxEnv {
		m.mu.Unlock()
		return Observation{}, ErrTooMany
	}
	e.ID = m.newID()
	m.envs[e.ID] = e
	m.mu.Unlock()

	// A bot that moves first plays before the agent's first decision.
	var played []string
	if err := e.advanceBots(ctx, &played); err != nil {
		return Observation{}, err
	}
	return e.observe(played), nil
}

// get returns the env with id, refreshing its idle timer.
func (m *Manager) get(id string) (*Env, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.envs[id]
	if !ok {
		return nil, ErrNotFound
	}
	e.touched = time.Now()
	return e, nil
}

// Observe returns the environment's current observation.
func (m *Manager) Observe(id string) (Observation, error) {
	e, err := m.get(id)
	if err != nil {
		return Observation{}, err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.observe(nil), nil
}

// Step plays the agent's move (canonical notation) for the seat to move,
// then lets bot opponents reply, and returns the resulting observation. An
// illegal move returns an error and leaves the environment unchanged.
func (m *Manager) Step(ctx context.Context, id, notation string) (Observation, error) {
	e, err := m.get(id)
	if err != nil {
		return Observation{}, err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.done {
		return Observation{}, ErrGameOver
	}
	if e.opp != nil && e.toMove != e.agent {
		return Observation{}, fmt.Errorf("envs: it is not the agent's turn")
	}

	mv, err := state.ParseMove(e.game, notation)
	if err != nil {
		return Observation{}, err
	}
	var played []string
	if err := e.apply(mv, &played); err != nil {
		return Observation{}, err
	}
	if err := e.advanceBots(ctx, &played); err != nil {
		return Observation{}, err
	}
	return e.observe(played), nil
}

// Fork clones an environment (position, history and opponent) into a new
// independent one — the primitive tree search and exploration need.
func (m *Manager) Fork(id string) (Observation, error) {
	src, err := m.get(id)
	if err != nil {
		return Observation{}, err
	}
	src.mu.Lock()
	clone := &Env{
		game:     src.game,
		st:       src.st.Clone(),
		toMove:   src.toMove,
		agent:    src.agent,
		oppID:    src.oppID,
		oppIters: src.oppIters,
		seed:     src.seed,
		maxPlies: src.maxPlies,

		planes: src.planes, actionIDs: src.actionIDs, planeDepth: src.planeDepth,

		history: append([]string(nil), src.history...),
		done:    src.done,
		winner:  src.winner,
		reason:  src.reason,
		trunc:   src.trunc,
		last:    src.last,
		touched: time.Now(),
	}
	histLen := len(src.history)
	if state.IsRuleset(src.game) {
		// A fork is an independent sample of the game's chance: it must not
		// replay the dice the original is about to roll.
		clone.st = state.Determinize(src.game, clone.st, src.agent, rand.New(rand.NewSource(time.Now().UnixNano())))
	}
	src.mu.Unlock()
	if src.oppID != "" {
		if bot, ok := bots.MakeConfigured(src.oppID, src.seed+int64(histLen)+1, bots.Config{MCTSIterations: src.oppIters}); ok {
			clone.opp = bot
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.gc()
	if len(m.envs) >= m.maxEnv {
		return Observation{}, ErrTooMany
	}
	clone.ID = m.newID()
	m.envs[clone.ID] = clone
	return clone.observe(nil), nil
}

// PlayMatch plays a series of bot-versus-bot games over the manager's
// catalog (see the package-level PlayMatch).
func (m *Manager) PlayMatch(ctx context.Context, opts MatchOptions) (MatchResult, error) {
	return PlayMatch(ctx, m.catalog, opts)
}

// Close discards an environment. Closing an unknown ID is not an error.
func (m *Manager) Close(id string) {
	m.mu.Lock()
	delete(m.envs, id)
	m.mu.Unlock()
}

// List returns the IDs of live environments, sorted.
func (m *Manager) List() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.gc()
	ids := make([]string, 0, len(m.envs))
	for id := range m.envs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// apply plays mv for the seat to move.
func (e *Env) apply(mv state.Move, played *[]string) error {
	res, err := state.Apply(e.game, e.st, e.toMove, mv)
	if err != nil {
		return err
	}
	e.st = res.NewState
	e.toMove = res.NextTurnSeat
	e.history = append(e.history, res.Notation)
	e.last = res.Notation
	*played = append(*played, res.Notation)
	if res.GameEnded {
		e.done = true
		e.winner = res.WinnerSeat
		e.reason = res.EndReason
	} else if e.st.Ply() >= e.maxPlies {
		e.done = true
		e.trunc = true
		e.reason = "max-plies"
	}
	return nil
}

// advanceBots lets the opponent bot play while it is its turn.
func (e *Env) advanceBots(ctx context.Context, played *[]string) error {
	if e.opp == nil {
		return nil
	}
	for !e.done && e.toMove != e.agent {
		legal := state.LegalMoves(e.game, e.st, e.toMove)
		if len(legal) == 0 {
			e.done, e.reason = true, "no-legal-moves"
			return nil
		}
		// The opponent plays within the game's strategy guide, as it does at
		// a table.
		mv, err := e.opp.Choose(ctx, e.game, e.st, e.toMove, state.GuidedMoves(e.game, e.st, e.toMove, legal))
		if err != nil {
			mv = legal[0]
		}
		if err := e.apply(mv, played); err != nil {
			return err
		}
	}
	return nil
}

func (e *Env) observe(played []string) Observation {
	color := ""
	if sd, ok := e.game.Seat(e.agent); ok {
		color = sd.Color
	}
	obs := Observation{
		EnvID:      e.ID,
		GameDefID:  e.game.Definition.ID,
		AgentSeat:  e.agent,
		AgentColor: color,
		ToMove:     e.toMove,
		Ply:        e.st.Ply(),
		BoardText:  state.RenderText(e.game, e.st),
		History:    append([]string{}, e.history...),
		LastMoves:  played,
		Done:       e.done,
		Truncated:  e.trunc,
		WinnerSeat: e.winner,
		EndReason:  e.reason,
		LegalMoves: []string{},
	}
	// The observer sees their own secrets and nobody else's; the position
	// an agent playing both sides sees is the side to move's.
	viewer := e.agent
	if e.opp == nil {
		viewer = e.toMove
	}
	if doc, err := state.Public(e.game, e.st, viewer).MarshalJSON(); err == nil {
		obs.State = rawJSON(doc)
	}
	if e.game.Definition.Notation == "ptn" {
		if tps, err := state.FormatTPS(e.game, e.st, e.toMove); err == nil {
			obs.TPS = tps
		}
	}
	if e.opp == nil {
		obs.YourTurn = !e.done
	} else {
		obs.YourTurn = !e.done && e.toMove == e.agent
	}
	var space *state.ActionSpace
	if e.actionIDs {
		space = state.ActionSpaceOf(e.game)
		if space.Fixed {
			obs.NumActions = len(space.Moves)
		}
	}
	if obs.YourTurn {
		for _, mv := range state.LegalMoves(e.game, e.st, e.toMove) {
			text := state.FormatMove(e.game, mv)
			obs.LegalMoves = append(obs.LegalMoves, text)
			if space != nil && space.Fixed {
				if id, ok := space.ID(text); ok {
					obs.LegalActionIDs = append(obs.LegalActionIDs, id)
				}
			}
		}
	}
	if e.planes {
		if p, err := state.EncodePlanes(e.game, e.st, e.agent, e.toMove, state.PlaneOptions{Depth: e.planeDepth}); err == nil {
			obs.Planes = p
		}
	}
	if e.done && !e.trunc {
		switch {
		case e.winner == nil:
			obs.Result = "draw"
		case *e.winner == e.agent:
			obs.Result, obs.Reward = "win", 1
		default:
			obs.Result, obs.Reward = "loss", -1
		}
	}
	return obs
}

// stateActionSpace returns the fixed action space of the env's game.
func stateActionSpace(e *Env) *state.ActionSpace { return state.ActionSpaceOf(e.game) }
