package bots

import (
	"context"
	"fmt"
	"math/rand"
	"sync"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/dsl"
	"github.com/noderunner/bingle/internal/engine/state"
)

// MinimaxOptions tunes the alpha-beta bot.
type MinimaxOptions struct {
	// NodeBudget bounds the positions examined per move (default 30000);
	// the search deepens one ply at a time while the budget lasts, so play
	// is deterministic for a given seed.
	NodeBudget int
	// MaxDepth caps the search depth in plies (default 6).
	MaxDepth int
	// NoConnectivity turns off the connection term of the heuristic (see
	// NewMinimax); mostly for comparing the two.
	NoConnectivity bool
	// Seed makes tie-breaking reproducible when non-zero.
	Seed int64
}

type minimaxBot struct {
	opts MinimaxOptions
	mu   sync.Mutex
	rng  *rand.Rand
}

// NewMinimax returns an alpha-beta search bot with iterative deepening. It
// sees forced wins and losses a few plies deep and otherwise prefers
// positions with more of its pieces on top of the board than the opponent's
// — a material heuristic that suits stacking, flipping and capturing games
// (Tak's flat count, Reversi discs, draughts) — and, in games with a
// connection goal (Tak's roads, Hex), a larger connected group of the pieces
// that count towards it.
func NewMinimax(opts MinimaxOptions) Bot {
	if opts.NodeBudget <= 0 {
		opts.NodeBudget = 30000
	}
	if opts.MaxDepth <= 0 {
		opts.MaxDepth = 6
	}
	return &minimaxBot{opts: opts, rng: newRand(opts.Seed)}
}

func (b *minimaxBot) ID() string    { return "minimax" }
func (b *minimaxBot) Label() string { return "Minimax" }
func (b *minimaxBot) Description() string {
	return fmt.Sprintf("Alpha-beta search a few plies deep (about %d positions per move): sees short forced wins and losses, otherwise counts pieces on top and rewards the largest connected group where the game is about connection. Deterministic for a given seed.", b.opts.NodeBudget)
}

const (
	minimaxWin  = 1_000_000
	minimaxInf  = 1 << 30
	minimaxHalf = minimaxWin / 2
)

type minimaxSearch struct {
	game   *catalog.CompiledGame
	root   int // seat the search plays for
	colors map[int]string
	// connect holds the game's connection rules (tag/piece filters) so
	// the heuristic can reward the largest connected group.
	connect []dsl.WinRuleDef
	opp     string
	// useHeur makes the game's own valuation the leaf heuristic (games
	// whose rules are written in Go, where material and connection mean
	// little).
	useHeur bool
	nodes   int
	// botID is only for counting nodes (see nodes.go).
	botID  string
	budget int
	ctx    context.Context
	out    bool // budget or context exhausted
}

func (b *minimaxBot) Choose(ctx context.Context, game *catalog.CompiledGame, st *state.BoardState, seat int, legal []state.Move) (state.Move, error) {
	ev, err := b.Evaluate(ctx, game, st, seat, legal)
	if err != nil {
		return state.Move{}, err
	}
	return ev.Move, nil
}

// Evaluate runs the search; WinRate is a squashed score in [0, 1].
func (b *minimaxBot) Evaluate(ctx context.Context, game *catalog.CompiledGame, st *state.BoardState, seat int, legal []state.Move) (Evaluation, error) {
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
	// Search the world the seat can know (dice and hidden choices re-drawn).
	st = state.Determinize(game, st, seat, rng)

	s := &minimaxSearch{game: game, root: seat, botID: b.ID(), budget: b.opts.NodeBudget, ctx: ctx, colors: map[int]string{}}
	_, s.useHeur = state.HeuristicValue(game, st, seat)
	for _, sd := range game.Definition.Seats {
		s.colors[sd.Seat] = sd.Color
	}
	if !b.opts.NoConnectivity && len(game.Definition.Seats) == 2 {
		for _, r := range game.Definition.WinCondition.Rules {
			if r.Type == "connect" && r.Result != "lose" {
				s.connect = append(s.connect, r)
			}
		}
		if o, ok := game.Opponent(seat); ok {
			s.opp = s.colors[o]
		}
	}

	// Order the root moves randomly once so ties break variedly but
	// reproducibly.
	order := append([]state.Move(nil), legal...)
	rng.Shuffle(len(order), func(i, j int) { order[i], order[j] = order[j], order[i] })

	type scored struct {
		mv    state.Move
		score int
	}
	var best []scored
	for depth := 1; depth <= b.opts.MaxDepth; depth++ {
		var cur []scored
		alpha := -minimaxInf
		complete := true
		for _, mv := range order {
			res, err := state.Apply(game, st, seat, mv)
			s.nodes++
			if err != nil {
				continue
			}
			var score int
			switch {
			case res.GameEnded:
				score = s.terminal(res, 0)
			case depth == 1:
				score = s.leaf(res.NewState)
			default:
				// Negamax from the opponent's side, scores relative to
				// the next mover; flip back for the root.
				v := s.search(res.NewState, res.NextTurnSeat, depth-1, -minimaxInf, minimaxInf, 1)
				if res.NextTurnSeat == seat {
					score = v
				} else {
					score = -v
				}
			}
			if s.out {
				complete = false
				break
			}
			cur = append(cur, scored{mv, score})
			if score > alpha {
				alpha = score
			}
		}
		if !complete {
			break
		}
		best = cur
		// A forced win needs no deeper search.
		if len(best) > 0 && alpha >= minimaxHalf {
			break
		}
	}
	if len(best) == 0 {
		mv := order[0]
		return Evaluation{Move: mv, WinRate: 0.5, Alternatives: []Candidate{{Move: mv, WinRate: 0.5}}}, nil
	}
	top := best[0]
	for _, c := range best[1:] {
		if c.score > top.score {
			top = c
		}
	}
	// One atomic add per search; see nodes.go.
	addNodes(s.botID, s.nodes)
	ev := Evaluation{Move: top.mv, WinRate: squash(top.score), Visits: s.nodes, Heuristic: true}
	ev.Alternatives = append(ev.Alternatives, Candidate{Move: top.mv, WinRate: squash(top.score)})
	for _, c := range best {
		if len(ev.Alternatives) >= 4 {
			break
		}
		if c.mv.Key() != top.mv.Key() {
			ev.Alternatives = append(ev.Alternatives, Candidate{Move: c.mv, WinRate: squash(c.score)})
		}
	}
	return ev, nil
}

// squash maps a score to (0, 1): wins/losses saturate, material is gentle.
func squash(score int) float64 {
	switch {
	case score >= minimaxHalf:
		return 1
	case score <= -minimaxHalf:
		return 0
	}
	return 0.5 + float64(score)/(2*float64(abs(score)+8))
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// terminal scores a finished game for the root seat; sooner wins are better.
func (s *minimaxSearch) terminal(res *state.MoveResult, ply int) int {
	if res.WinnerSeat == nil {
		return 0
	}
	if *res.WinnerSeat == s.root {
		return minimaxWin - ply
	}
	return -(minimaxWin - ply)
}

// leaf is the heuristic for the root seat: its top pieces minus the others',
// plus — in games with connection goals (Tak's roads, Hex, Havannah) — twice
// the difference of the largest connected groups of qualifying pieces.
func (s *minimaxSearch) leaf(st *state.BoardState) int {
	if s.useHeur {
		v, _ := state.HeuristicValue(s.game, st, s.root)
		return int(v * 1000)
	}
	mine := s.colors[s.root]
	score := 0
	for _, id := range st.Occupied() {
		top, ok := st.Get(id)
		if !ok {
			continue
		}
		if top.Color == mine {
			score++
		} else {
			score--
		}
	}
	if len(s.connect) > 0 && s.opp != "" {
		best := 0
		for _, r := range s.connect {
			d := s.largestGroup(st, mine, r) - s.largestGroup(st, s.opp, r)
			if d > best || best == 0 {
				best = d
			}
		}
		score += 2 * best
	}
	return score
}

// largestGroup is the size of the biggest connected group of color's top
// pieces that satisfy the rule's tag/piece filter.
func (s *minimaxSearch) largestGroup(st *state.BoardState, color string, r dsl.WinRuleDef) int {
	g := s.game.Board
	in := make([]bool, g.Len())
	any := false
	for _, id := range st.Occupied() {
		top, ok := st.Get(id)
		if !ok || top.Color != color {
			continue
		}
		if r.Piece != "" && top.Piece != r.Piece {
			continue
		}
		if r.Tag != "" && !s.game.Pieces[top.Piece].HasTag(r.Tag) {
			continue
		}
		if i, ok := g.Index(id); ok {
			in[i] = true
			any = true
		}
	}
	if !any {
		return 0
	}
	seen := make([]bool, len(in))
	var stack []int
	best := 0
	for start := range in {
		if !in[start] || seen[start] {
			continue
		}
		size := 0
		seen[start] = true
		stack = append(stack[:0], start)
		for len(stack) > 0 {
			cur := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			size++
			for _, nb := range g.NeighborIndices(cur) {
				if in[nb] && !seen[nb] {
					seen[nb] = true
					stack = append(stack, nb)
				}
			}
		}
		if size > best {
			best = size
		}
	}
	return best
}

// search returns the value (for the seat to move, negamax style) of st.
func (s *minimaxSearch) search(st *state.BoardState, seat, depth, alpha, beta, ply int) int {
	if s.out {
		return 0
	}
	if s.nodes >= s.budget || s.ctx.Err() != nil {
		s.out = true
		return 0
	}
	if depth == 0 {
		v := s.leaf(st)
		if seat != s.root {
			v = -v
		}
		return v
	}
	// Inner nodes follow the game's strategy guide where it has one, so the
	// node budget goes into plausible lines.
	moves := state.GuidedMoves(s.game, st, seat, state.LegalMoves(s.game, st, seat))
	if len(moves) == 0 {
		return 0
	}
	best := -minimaxInf
	for _, mv := range moves {
		res, err := state.Apply(s.game, st, seat, mv)
		s.nodes++
		if err != nil {
			continue
		}
		var v int
		if res.GameEnded {
			v = s.terminal(res, ply)
			if seat != s.root {
				v = -v
			}
		} else {
			if res.NextTurnSeat == seat {
				// The same seat moves again (the opponent had to pass): no
				// sign change, so the window is kept as it is.
				v = s.search(res.NewState, res.NextTurnSeat, depth-1, alpha, beta, ply+1)
			} else {
				v = -s.search(res.NewState, res.NextTurnSeat, depth-1, -beta, -alpha, ply+1)
			}
		}
		if s.out {
			return 0
		}
		if v > best {
			best = v
		}
		if best > alpha {
			alpha = best
		}
		if alpha >= beta {
			break
		}
	}
	return best
}
