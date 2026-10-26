// Package bots provides built-in computer opponents that work for any game
// the engine can express: they only use the engine's generic LegalMoves /
// Apply API, never game-specific knowledge.
//
//   - "random":   uniformly random legal move.
//   - "tactical": plays a move that wins immediately if there is one, avoids
//     moves that hand the opponent an immediate win, otherwise random.
//   - "mcts":     Monte-Carlo tree search (UCT) with random playouts, over
//     the tactically safe moves, under a time budget.
//
// They exist so a human (or a model under training) can play a game alone,
// and as baseline opponents for evaluating learned policies.
package bots

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"sync"
	"time"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/state"
)

// Bot chooses a move for a seat.
type Bot interface {
	// ID is the stable identifier clients use to request this bot.
	ID() string
	// Label is a short human-readable name.
	Label() string
	// Description says how it plays.
	Description() string
	// Choose picks one of legal (which must be non-empty and is the full
	// set of legal moves for seat in st).
	Choose(ctx context.Context, game *catalog.CompiledGame, st *state.BoardState, seat int, legal []state.Move) (state.Move, error)
}

// Evaluation is a bot's assessment of a position: the move it would play and
// how good it thinks it is.
type Evaluation struct {
	Move state.Move
	// Heuristic is true when WinRate is a squashed heuristic score rather
	// than a probability estimate (the minimax bot).
	Heuristic bool
	// WinRate is the chosen move's estimated chance of winning for the
	// side to move, in [0, 1] (draws count one half).
	WinRate float64
	// Visits is the number of simulations behind the estimate (0 when the
	// move was decided without search, e.g. an immediate win).
	Visits int
	// Alternatives are the best few moves with their estimates, best first
	// (including the chosen move).
	Alternatives []Candidate
}

// Candidate is one move with its estimate.
type Candidate struct {
	Move    state.Move
	WinRate float64
	Visits  int
}

// Evaluator is implemented by bots that can say how good a position is, not
// only what to play.
type Evaluator interface {
	Evaluate(ctx context.Context, game *catalog.CompiledGame, st *state.BoardState, seat int, legal []state.Move) (Evaluation, error)
}

// ErrNoMoves is returned by Choose when legal is empty.
var ErrNoMoves = errors.New("bots: no legal moves")

// Info describes a registered bot for listings.
type Info struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

// Registry holds the available bots by ID.
type Registry struct {
	mu    sync.RWMutex
	bots  map[string]Bot
	order []string // registration order, roughly increasing strength
}

// NewRegistry returns a registry with the built-in bots registered.
func NewRegistry() *Registry {
	r := &Registry{bots: map[string]Bot{}}
	r.Register(NewRandom(0))
	r.Register(NewTactical(0))
	r.Register(NewMinimax(MinimaxOptions{}))
	r.Register(NewMCTS(MCTSOptions{}))
	return r
}

// Register adds (or replaces) a bot.
func (r *Registry) Register(b Bot) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.bots[b.ID()]; !exists {
		r.order = append(r.order, b.ID())
	}
	r.bots[b.ID()] = b
}

// Make returns a fresh instance of the built-in bot id seeded with seed
// (0 = clock), for callers that need reproducible, independent bots such as
// training environments.
func Make(id string, seed int64) (Bot, bool) {
	return MakeConfigured(id, seed, Config{})
}

// Config tunes bots built by MakeConfigured.
type Config struct {
	// MCTSIterations, when positive, bounds the MCTS bot by search
	// iterations instead of wall-clock time. An iteration-limited bot with a
	// fixed seed plays reproducibly, which timed search cannot.
	MCTSIterations int
	// MinimaxNodes bounds the minimax bot's search per move (default 30000).
	MinimaxNodes int
}

// MakeConfigured is Make with tuning options.
func MakeConfigured(id string, seed int64, cfg Config) (Bot, bool) {
	switch id {
	case "random":
		return NewRandom(seed), true
	case "tactical":
		return NewTactical(seed), true
	case "minimax":
		return NewMinimax(MinimaxOptions{Seed: seed, NodeBudget: cfg.MinimaxNodes}), true
	case "mcts":
		return NewMCTS(MCTSOptions{Seed: seed, MaxIterations: cfg.MCTSIterations}), true
	}
	return nil, false
}

// Get returns the bot registered under id.
func (r *Registry) Get(id string) (Bot, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	b, ok := r.bots[id]
	return b, ok
}

// List describes every registered bot in registration order (the built-ins
// are registered weakest first).
func (r *Registry) List() []Info {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Info, 0, len(r.order))
	for _, id := range r.order {
		b := r.bots[id]
		out = append(out, Info{ID: b.ID(), Label: b.Label(), Description: b.Description()})
	}
	return out
}

// --- shared helpers ---------------------------------------------------------

// newRand returns a rand.Rand seeded with seed, or the clock when seed is 0.
func newRand(seed int64) *rand.Rand {
	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	return rand.New(rand.NewSource(seed))
}

// hasWon reports whether res ended the game with seat as the winner.
func hasWon(res *state.MoveResult, seat int) bool {
	return res.GameEnded && res.WinnerSeat != nil && *res.WinnerSeat == seat
}

// hasLost reports whether res ended the game with a different winner.
func hasLost(res *state.MoveResult, seat int) bool {
	return res.GameEnded && res.WinnerSeat != nil && *res.WinnerSeat != seat
}

// classify applies every legal move once and splits them into immediate
// wins, safe moves (no immediate loss and no opponent winning reply) and the
// rest. It also returns the resulting positions for reuse.
type classified struct {
	wins   []state.Move
	safe   []state.Move
	unsafe []state.Move
}

// replyBudget bounds the opponent-reply checks of one classify call, in
// "square visits": each reply application is charged the board's size, since
// win evaluation scans the board. Checking every reply of every move is
// O(moves²); on big boards (Go 19×19, Havannah) that would take seconds per
// move, so once the budget is spent the remaining moves are kept as safe
// without the check. The cut-off is deterministic (move order, not time), so
// seeded bots stay reproducible.
const replyBudget = 2_000_000

func classify(ctx context.Context, game *catalog.CompiledGame, st *state.BoardState, seat int, legal []state.Move) (classified, error) {
	var out classified
	budget := replyBudget
	cost := game.Board.Len()
	if cost < 1 {
		cost = 1
	}
	for _, mv := range legal {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		res, err := state.Apply(game, st, seat, mv)
		if err != nil {
			// Should not happen for a legal move; treat as unsafe.
			out.unsafe = append(out.unsafe, mv)
			continue
		}
		switch {
		case hasWon(res, seat):
			out.wins = append(out.wins, mv)
			continue
		case hasLost(res, seat):
			out.unsafe = append(out.unsafe, mv)
			continue
		case res.GameEnded:
			out.safe = append(out.safe, mv) // a draw: acceptable
			continue
		}
		// Does the opponent have an immediate winning reply?
		if budget <= 0 {
			out.safe = append(out.safe, mv)
			continue
		}
		oppSeat := res.NextTurnSeat
		losing := false
		for _, reply := range state.LegalMoves(game, res.NewState, oppSeat) {
			budget -= cost
			r2, err := state.Apply(game, res.NewState, oppSeat, reply)
			if err == nil && hasLost(r2, seat) {
				losing = true
				break
			}
		}
		if losing {
			out.unsafe = append(out.unsafe, mv)
		} else {
			out.safe = append(out.safe, mv)
		}
	}
	return out, nil
}

// --- random -----------------------------------------------------------------

type randomBot struct {
	mu  sync.Mutex
	rng *rand.Rand
}

// NewRandom returns the uniformly random bot (seed 0 = clock seed).
func NewRandom(seed int64) Bot { return &randomBot{rng: newRand(seed)} }

func (b *randomBot) ID() string    { return "random" }
func (b *randomBot) Label() string { return "Random" }
func (b *randomBot) Description() string {
	return "Plays a uniformly random legal move. Useful as a floor when measuring a policy."
}

func (b *randomBot) Choose(_ context.Context, _ *catalog.CompiledGame, _ *state.BoardState, _ int, legal []state.Move) (state.Move, error) {
	if len(legal) == 0 {
		return state.Move{}, ErrNoMoves
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return legal[b.rng.Intn(len(legal))], nil
}

// --- tactical ---------------------------------------------------------------

type tacticalBot struct {
	mu  sync.Mutex
	rng *rand.Rand
}

// NewTactical returns the one-move-lookahead bot (seed 0 = clock seed).
func NewTactical(seed int64) Bot { return &tacticalBot{rng: newRand(seed)} }

func (b *tacticalBot) ID() string    { return "tactical" }
func (b *tacticalBot) Label() string { return "Tactical" }
func (b *tacticalBot) Description() string {
	return "Takes an immediate win when it sees one, avoids moves that let the opponent win next turn, and otherwise plays randomly."
}

func (b *tacticalBot) Choose(ctx context.Context, game *catalog.CompiledGame, st *state.BoardState, seat int, legal []state.Move) (state.Move, error) {
	if len(legal) == 0 {
		return state.Move{}, ErrNoMoves
	}
	b.mu.Lock()
	st = state.Determinize(game, st, seat, b.rng)
	b.mu.Unlock()
	c, err := classify(ctx, game, st, seat, legal)
	if err != nil {
		return state.Move{}, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	switch {
	case len(c.wins) > 0:
		return c.wins[b.rng.Intn(len(c.wins))], nil
	case len(c.safe) > 0:
		return c.safe[b.rng.Intn(len(c.safe))], nil
	default:
		return legal[b.rng.Intn(len(legal))], nil
	}
}

// --- mcts -------------------------------------------------------------------

// MCTSOptions tunes the Monte-Carlo tree search bot.
type MCTSOptions struct {
	// Budget is the wall-clock search time per move (default 1s).
	Budget time.Duration
	// MaxIterations caps search iterations when > 0.
	MaxIterations int
	// MaxPlayoutPlies bounds one random playout (default 120); an unfinished
	// playout is scored as a draw.
	MaxPlayoutPlies int
	// Exploration is the UCT constant (default 1.4).
	Exploration float64
	// Seed makes the search reproducible when non-zero.
	Seed int64
}

type mctsBot struct {
	opts MCTSOptions
	mu   sync.Mutex
	rng  *rand.Rand
}

// NewMCTS returns a UCT Monte-Carlo tree search bot.
func NewMCTS(opts MCTSOptions) Bot {
	if opts.Budget <= 0 {
		opts.Budget = time.Second
		if opts.MaxIterations > 0 {
			// Iteration-limited: the clock is only a safety net.
			opts.Budget = 10 * time.Minute
		}
	}
	if opts.MaxPlayoutPlies <= 0 {
		opts.MaxPlayoutPlies = 120
	}
	if opts.Exploration <= 0 {
		opts.Exploration = 1.4
	}
	return &mctsBot{opts: opts, rng: newRand(opts.Seed)}
}

func (b *mctsBot) ID() string    { return "mcts" }
func (b *mctsBot) Label() string { return "MCTS" }
func (b *mctsBot) Description() string {
	if b.opts.MaxIterations > 0 {
		return fmt.Sprintf("Monte-Carlo tree search with random playouts (%d iterations per move), restricted to tactically safe moves.", b.opts.MaxIterations)
	}
	return fmt.Sprintf("Monte-Carlo tree search with random playouts (about %s per move), restricted to tactically safe moves. Plays stronger than 'tactical' but is not a strong engine.", b.opts.Budget)
}

type mctsNode struct {
	st       *state.BoardState
	seat     int // seat to move at this node
	move     state.Move
	parent   *mctsNode
	children []*mctsNode
	untried  []state.Move
	visits   int
	score    float64 // from the perspective of the seat that moved INTO this node
	mover    int     // the seat that made `move`
	terminal bool
	winner   *int
}

func (b *mctsBot) Choose(ctx context.Context, game *catalog.CompiledGame, st *state.BoardState, seat int, legal []state.Move) (state.Move, error) {
	ev, err := b.Evaluate(ctx, game, st, seat, legal)
	if err != nil {
		return state.Move{}, err
	}
	return ev.Move, nil
}

// Evaluate searches and reports the chosen move with its estimated win rate
// and the best alternatives.
func (b *mctsBot) Evaluate(ctx context.Context, game *catalog.CompiledGame, st *state.BoardState, seat int, legal []state.Move) (Evaluation, error) {
	if len(legal) == 0 {
		return Evaluation{}, ErrNoMoves
	}
	if len(legal) == 1 {
		return Evaluation{Move: legal[0], WinRate: 0.5, Alternatives: []Candidate{{Move: legal[0], WinRate: 0.5}}}, nil
	}

	b.mu.Lock()
	rng := rand.New(rand.NewSource(b.rng.Int63()))
	b.mu.Unlock()
	if !state.WorthSearching(game, st) {
		mv := legal[rng.Intn(len(legal))]
		return Evaluation{Move: mv, WinRate: 0.5, Alternatives: []Candidate{{Move: mv, WinRate: 0.5}}}, nil
	}
	// Search a sample of the world the seat can actually know: dice and the
	// opponent's committed choices are re-drawn.
	st = state.Determinize(game, st, seat, rng)

	c, err := classify(ctx, game, st, seat, legal)
	if err != nil {
		return Evaluation{}, err
	}
	if len(c.wins) > 0 {
		mv := c.wins[rng.Intn(len(c.wins))]
		return Evaluation{Move: mv, WinRate: 1, Alternatives: []Candidate{{Move: mv, WinRate: 1}}}, nil
	}
	candidates := c.safe
	if len(candidates) == 0 {
		candidates = legal
	}
	if len(candidates) == 1 {
		return Evaluation{Move: candidates[0], WinRate: 0.5, Alternatives: []Candidate{{Move: candidates[0], WinRate: 0.5}}}, nil
	}

	root := &mctsNode{st: st, seat: seat, untried: append([]state.Move(nil), candidates...), mover: -1}
	deadline := time.Now().Add(b.opts.Budget)

	for it := 0; ; it++ {
		if b.opts.MaxIterations > 0 && it >= b.opts.MaxIterations {
			break
		}
		if it%4 == 0 && (time.Now().After(deadline) || ctx.Err() != nil) {
			break
		}
		b.iterate(game, root, rng)
	}

	// Most-visited child is the move; the next few by visits are the
	// alternatives.
	children := append([]*mctsNode(nil), root.children...)
	sort.SliceStable(children, func(i, j int) bool { return children[i].visits > children[j].visits })
	if len(children) == 0 {
		mv := candidates[rng.Intn(len(candidates))]
		return Evaluation{Move: mv, WinRate: 0.5, Alternatives: []Candidate{{Move: mv, WinRate: 0.5}}}, nil
	}
	ev := Evaluation{Move: children[0].move, Visits: root.visits}
	for i, ch := range children {
		if i >= 4 {
			break
		}
		wr := ch.score / float64(ch.visits)
		ev.Alternatives = append(ev.Alternatives, Candidate{Move: ch.move, WinRate: wr, Visits: ch.visits})
	}
	ev.WinRate = ev.Alternatives[0].WinRate
	return ev, nil
}

func (b *mctsBot) iterate(game *catalog.CompiledGame, root *mctsNode, rng *rand.Rand) {
	node := root

	// Selection.
	for len(node.untried) == 0 && len(node.children) > 0 && !node.terminal {
		node = b.selectChild(node)
	}

	// Expansion.
	if !node.terminal && len(node.untried) > 0 {
		i := rng.Intn(len(node.untried))
		mv := node.untried[i]
		node.untried[i] = node.untried[len(node.untried)-1]
		node.untried = node.untried[:len(node.untried)-1]

		res, err := state.Apply(game, node.st, node.seat, mv)
		if err != nil {
			return
		}
		child := &mctsNode{
			st:       res.NewState,
			seat:     res.NextTurnSeat,
			move:     mv,
			parent:   node,
			mover:    node.seat,
			terminal: res.GameEnded,
			winner:   res.WinnerSeat,
		}
		if !child.terminal {
			child.untried = state.GuidedMoves(game, child.st, child.seat, state.LegalMoves(game, child.st, child.seat))
			if len(child.untried) == 0 {
				child.terminal = true // stalemate: scored as a draw
			}
		}
		node.children = append(node.children, child)
		node = child
	}

	// Simulation.
	winner, decided, hv, hvSeat, haveHV := b.playout(game, node, rng)

	// Backpropagation: score from the perspective of each node's mover.
	for n := node; n != nil; n = n.parent {
		n.visits++
		if n.mover < 0 {
			continue
		}
		switch {
		case !decided && haveHV:
			// An unfinished playout of a long game: the game's own
			// valuation of the final position stands in for the result.
			v := hv
			if n.mover != hvSeat {
				v = -v
			}
			n.score += 0.5 + 0.5*v
		case !decided:
			n.score += 0.5
		case winner == n.mover:
			n.score += 1
		case winner < 0:
			n.score += 0.5
		}
	}
}

func (b *mctsBot) selectChild(n *mctsNode) *mctsNode {
	var best *mctsNode
	bestVal := -1.0
	logN := logf(float64(n.visits))
	for _, ch := range n.children {
		val := ch.score/float64(ch.visits) + b.opts.Exploration*sqrtf(logN/float64(ch.visits))
		if best == nil || val > bestVal {
			best, bestVal = ch, val
		}
	}
	return best
}

// playout plays random moves from node to the end of the game (or the ply
// cap). winner is the winning seat, or -1 for a draw; decided is false when
// the cap was hit.
func (b *mctsBot) playout(game *catalog.CompiledGame, node *mctsNode, rng *rand.Rand) (winner int, decided bool, hv float64, hvSeat int, haveHV bool) {
	if node.terminal {
		if node.winner != nil {
			return *node.winner, true, 0, 0, false
		}
		return -1, true, 0, 0, false
	}
	st, seat := node.st, node.seat
	for ply := 0; ply < b.opts.MaxPlayoutPlies; ply++ {
		// Playouts follow the game's strategy guide where it has one: a
		// playout of random moves tells nothing about a game where most
		// moves are plain silly.
		moves := state.GuidedMoves(game, st, seat, state.LegalMoves(game, st, seat))
		if len(moves) == 0 {
			return -1, true, 0, 0, false
		}
		res, err := state.Apply(game, st, seat, moves[rng.Intn(len(moves))])
		if err != nil {
			return -1, false, 0, 0, false
		}
		if res.GameEnded {
			if res.WinnerSeat != nil {
				return *res.WinnerSeat, true, 0, 0, false
			}
			return -1, true, 0, 0, false
		}
		st, seat = res.NewState, res.NextTurnSeat
	}
	if v, ok := state.HeuristicValue(game, st, seat); ok {
		return -1, false, v, seat, true
	}
	return -1, false, 0, 0, false
}
