package guest

import (
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"time"
)

// Heuristic is what a game's Rules may also implement: a value of a position
// for seat, in [-1, 1], for the search to chase where it cannot see the end
// of the game. Without it, minimax counts the pieces on top of each cell and
// MCTS scores a playout it had to cut short as a draw.
type Heuristic interface {
	Heuristic(pos *Position, seat int) float64
}

// The levels a search plays at: the host's bots of the same names.
const (
	LevelTactical = "tactical"
	LevelMinimax  = "minimax"
	LevelMCTS     = "mcts"
)

// BotRequest is what the bot call asks for, beside the position and the
// seat. A zero field takes its default.
type BotRequest struct {
	// Level is the bot that searches: tactical, minimax or mcts.
	Level string `json:"level"`
	// BudgetMS is the time the search may take, in milliseconds; 0 is no
	// limit but the others. A module reads the time only while it answers a
	// bot call.
	BudgetMS int64 `json:"budget_ms,omitempty"`
	// Iterations caps the playouts of mcts.
	Iterations int `json:"iterations,omitempty"`
	// Nodes caps the positions minimax looks at (default 30000).
	Nodes int `json:"nodes,omitempty"`
	// Depth caps how deep minimax searches, in selections (default 6).
	Depth int `json:"depth,omitempty"`
	// Plies caps a playout of mcts (default 120).
	Plies int `json:"plies,omitempty"`
	// Exploration is the UCT constant of mcts (default 1.4).
	Exploration float64 `json:"exploration,omitempty"`
	// Seed is the search's chance. The same seed and the same caps give the
	// same answer, unless the time ran out first.
	Seed uint64 `json:"seed"`
}

// BotReply is the bot call's answer: the selection, and how good the search
// thinks it is.
type BotReply struct {
	Selection Selection `json:"selection"`
	// WinRate is the selection's chance of winning for the seat, in [0, 1],
	// draws counting one half: a squashed score where Heuristic is set.
	WinRate   float64 `json:"win_rate"`
	Heuristic bool    `json:"heuristic,omitempty"`
	// Visits is the positions (minimax) or playouts (mcts) behind it.
	Visits int `json:"visits"`
	// Alternatives are the best few selections, best first, the chosen one
	// among them.
	Alternatives []Candidate `json:"alternatives,omitempty"`
}

// Candidate is one selection the search weighed.
type Candidate struct {
	Selection Selection `json:"selection"`
	WinRate   float64   `json:"win_rate"`
	Visits    int       `json:"visits,omitempty"`
}

// Search chooses a selection for seat with the host's bots' own methods, run
// over rules rather than across the boundary: the bot call of ABI v0. A
// module answers it for every game it plays, and a game that implements
// Heuristic is searched with it. colors are the seats' colours, which minimax
// counts pieces by, and cells the board's size, which bounds tactical's
// look at replies as the host's bot does.
//
// Rules that are also Searchable are searched over their own State instead:
// the position is turned into one once, and the search plays it from there.
func Search(rules Rules, colors map[int]string, cells int, pos *Position, seat int, req BotRequest) (BotReply, error) {
	if sr, ok := rules.(Searchable); ok {
		st, err := sr.State(pos)
		if err != nil {
			return BotReply{}, fmt.Errorf("guest: bot: %w", err)
		}
		return searchWith[State](stateOps{}, cells, st, seat, req)
	}
	o := positionOps{rules: rules, colors: colors}
	o.heur, _ = rules.(Heuristic)
	return searchWith[*Position](o, cells, pos, seat, req)
}

// Searchable is what a game's Rules may also implement, for the search
// inside a module: the game's own State, which is cheaper to copy and to read
// than a Position. The search turns a Position into one once, at its root,
// and then plays only States.
type Searchable interface {
	State(pos *Position) (State, error)
}

// State is a game's own position, for the search. A State is never changed:
// Play returns another.
type State interface {
	// Turn is the seat to move, 0 once the game is over, and Winner the seat
	// that won a finished game (0 for a draw).
	Turn() int
	Winner() int
	// Legal lists what the seat to move may do, as Rules.Legal does. The
	// search does not change the list, so a State may keep it.
	Legal() []Selection
	// Play applies a selection that Legal listed, as Rules.Apply does.
	Play(sel Selection) (State, error)
	// Value is what the State is worth to seat, from -1 to 1, as Heuristic.
	Value(seat int) float64
}

// after is what a selection led to, as the search needs it.
type after[P any] struct {
	pos    P
	next   int // the seat to move, 0 once the game is over
	ended  bool
	winner int
}

// ops are what the search does with a position of type P.
type ops[P any] interface {
	legal(p P, seat int) []Selection
	play(p P, seat int, sel Selection) (after[P], error)
	// value is the position's worth to seat, if the game says.
	value(p P, seat int) (float64, bool)
	// material is the pieces on top of a cell that are seat's, less the rest.
	material(p P, seat int) int
}

// positionOps search a Position through Rules.
type positionOps struct {
	rules  Rules
	heur   Heuristic
	colors map[int]string
}

func (o positionOps) legal(p *Position, seat int) []Selection { return o.rules.Legal(p, seat) }

func (o positionOps) play(p *Position, seat int, sel Selection) (after[*Position], error) {
	res, err := o.rules.Apply(p, seat, sel)
	if err != nil {
		return after[*Position]{}, err
	}
	return after[*Position]{pos: res.Position, next: res.NextSeat, ended: res.Ended, winner: res.Winner}, nil
}

func (o positionOps) value(p *Position, seat int) (float64, bool) {
	if o.heur == nil {
		return 0, false
	}
	return o.heur.Heuristic(p, seat), true
}

func (o positionOps) material(p *Position, seat int) int {
	mine, score := o.colors[seat], 0
	for _, stack := range p.Cells {
		if len(stack) == 0 {
			continue
		}
		if stack[len(stack)-1].Color == mine {
			score++
		} else {
			score--
		}
	}
	return score
}

// stateOps search a game's own State.
type stateOps struct{}

func (stateOps) legal(p State, seat int) []Selection {
	if p.Turn() != seat {
		return nil
	}
	return p.Legal()
}

func (stateOps) play(p State, _ int, sel Selection) (after[State], error) {
	next, err := p.Play(sel)
	if err != nil {
		return after[State]{}, err
	}
	turn := next.Turn()
	return after[State]{pos: next, next: turn, ended: turn == 0, winner: next.Winner()}, nil
}

func (stateOps) value(p State, seat int) (float64, bool) { return p.Value(seat), true }
func (stateOps) material(State, int) int                 { return 0 }

func searchWith[P any](o ops[P], cells int, pos P, seat int, req BotRequest) (BotReply, error) {
	s := &search[P]{ops: o, cells: max(cells, 1), rng: rand.New(rand.NewPCG(req.Seed, req.Seed^0x9e3779b97f4a7c15))}
	if req.BudgetMS > 0 {
		s.stop = time.Now().Add(time.Duration(req.BudgetMS) * time.Millisecond)
	}
	legal := o.legal(pos, seat)
	if len(legal) == 0 {
		return BotReply{}, errors.New("guest: bot: the seat has no legal selection")
	}
	if len(legal) == 1 {
		return even(legal[0]), nil
	}
	switch req.Level {
	case LevelTactical:
		return s.tactical(pos, seat, legal), nil
	case LevelMinimax:
		return s.minimax(pos, seat, legal, orDefault(req.Nodes, 30000), orDefault(req.Depth, 6)), nil
	case LevelMCTS:
		return s.mcts(pos, seat, legal, req.Iterations, orDefault(req.Plies, 120), orDefault(req.Exploration, 1.4)), nil
	}
	return BotReply{}, fmt.Errorf("guest: bot: no level %q", req.Level)
}

func orDefault[T int | float64](v, def T) T {
	if v > 0 {
		return v
	}
	return def
}

// even is a selection played without a search.
func even(sel Selection) BotReply {
	return BotReply{Selection: sel, WinRate: 0.5, Alternatives: []Candidate{{Selection: sel, WinRate: 0.5}}}
}

type search[P any] struct {
	ops   ops[P]
	cells int
	rng   *rand.Rand
	stop  time.Time // zero: no time limit
	nodes int
}

// late reports whether the search's time is up.
func (s *search[P]) late() bool { return !s.stop.IsZero() && time.Now().After(s.stop) }

func (s *search[P]) pick(list []Selection) Selection { return list[s.rng.IntN(len(list))] }

func won[P any](res after[P], seat int) bool { return res.ended && res.winner == seat }
func lost[P any](res after[P], seat int) bool {
	return res.ended && res.winner != 0 && res.winner != seat
}

// replyBudget bounds the replies classify looks at, in cells visited, as the
// host's bots do.
const replyBudget = 2_000_000

// classify splits legal into the selections that win at once, the safe ones
// (no loss, and no reply that wins for the other side) and the rest. Once
// its budget of replies or its time is spent, a selection that neither wins
// nor loses at once counts as safe.
func (s *search[P]) classify(pos P, seat int, legal []Selection) (wins, safe, unsafe []Selection) {
	budget := replyBudget
	for _, sel := range legal {
		res, err := s.ops.play(pos, seat, sel)
		s.nodes++
		switch {
		case err != nil || lost(res, seat):
			unsafe = append(unsafe, sel)
			continue
		case won(res, seat):
			wins = append(wins, sel)
			continue
		case res.ended, budget <= 0, s.late():
			safe = append(safe, sel)
			continue
		}
		losing := false
		for _, reply := range s.ops.legal(res.pos, res.next) {
			budget -= s.cells
			r2, err := s.ops.play(res.pos, res.next, reply)
			s.nodes++
			if err == nil && lost(r2, seat) {
				losing = true
				break
			}
		}
		if losing {
			unsafe = append(unsafe, sel)
		} else {
			safe = append(safe, sel)
		}
	}
	return wins, safe, unsafe
}

// tactical takes a win when it sees one, keeps clear of a loss on the next
// selection, and otherwise plays at random.
func (s *search[P]) tactical(pos P, seat int, legal []Selection) BotReply {
	wins, safe, _ := s.classify(pos, seat, legal)
	switch {
	case len(wins) > 0:
		sel := s.pick(wins)
		return BotReply{Selection: sel, WinRate: 1, Alternatives: []Candidate{{Selection: sel, WinRate: 1}}}
	case len(safe) > 0:
		return even(s.pick(safe))
	}
	return even(s.pick(legal))
}

// --- minimax ----------------------------------------------------------------

const (
	minimaxWin  = 1_000_000
	minimaxInf  = 1 << 30
	minimaxHalf = minimaxWin / 2
)

type minimax[P any] struct {
	*search[P]
	root   int
	budget int
	out    bool
}

// minimax is alpha-beta search deepened a selection at a time while its
// budget lasts, the host's minimax bot over rules.
func (s *search[P]) minimax(pos P, seat int, legal []Selection, budget, depth int) BotReply {
	m := &minimax[P]{search: s, root: seat, budget: budget}
	order := slices.Clone(legal)
	s.rng.Shuffle(len(order), func(i, j int) { order[i], order[j] = order[j], order[i] })
	type scored struct {
		sel   Selection
		score int
	}
	var best []scored
	for d := 1; d <= depth; d++ {
		var cur []scored
		alpha := -minimaxInf
		complete := true
		for _, sel := range order {
			res, err := s.ops.play(pos, seat, sel)
			s.nodes++
			if err != nil {
				continue
			}
			var score int
			switch {
			case res.ended:
				score = m.terminal(res, 0)
			case d == 1:
				score = m.leaf(res.pos)
			default:
				v := m.value(res.pos, res.next, d-1, -minimaxInf, minimaxInf, 1)
				if res.next == seat {
					score = v
				} else {
					score = -v
				}
			}
			if m.out {
				complete = false
				break
			}
			cur = append(cur, scored{sel, score})
			alpha = max(alpha, score)
		}
		if !complete {
			break
		}
		best = cur
		if len(best) > 0 && alpha >= minimaxHalf {
			break
		}
	}
	if len(best) == 0 {
		return even(order[0])
	}
	top := best[0]
	for _, c := range best[1:] {
		if c.score > top.score {
			top = c
		}
	}
	out := BotReply{Selection: top.sel, WinRate: squash(top.score), Heuristic: true, Visits: s.nodes,
		Alternatives: []Candidate{{Selection: top.sel, WinRate: squash(top.score)}}}
	for _, c := range best {
		if len(out.Alternatives) >= 4 {
			break
		}
		if !sameSelection(c.sel, top.sel) {
			out.Alternatives = append(out.Alternatives, Candidate{Selection: c.sel, WinRate: squash(c.score)})
		}
	}
	return out
}

func sameSelection(a, b Selection) bool {
	return a.Action == b.Action && a.Piece == b.Piece && a.From == b.From && a.To == b.To &&
		a.Count == b.Count && slices.Equal(a.Path, b.Path) && slices.Equal(a.Drops, b.Drops) &&
		a.Aim == b.Aim && a.Card == b.Card
}

// squash maps a score into (0, 1), as the host's minimax does.
func squash(score int) float64 {
	switch {
	case score >= minimaxHalf:
		return 1
	case score <= -minimaxHalf:
		return 0
	}
	a := score
	if a < 0 {
		a = -a
	}
	return 0.5 + float64(score)/(2*float64(a+8))
}

func (m *minimax[P]) terminal(res after[P], ply int) int {
	switch res.winner {
	case 0:
		return 0
	case m.root:
		return minimaxWin - ply
	}
	return -(minimaxWin - ply)
}

// leaf values a position for the root seat: the game's heuristic where it
// has one, or else its pieces on top of a cell less everybody else's.
func (m *minimax[P]) leaf(pos P) int {
	if v, ok := m.ops.value(pos, m.root); ok {
		return int(v * 1000)
	}
	return m.ops.material(pos, m.root)
}

// value is a position's value for the seat to move, negamax style.
func (m *minimax[P]) value(pos P, seat, depth, alpha, beta, ply int) int {
	if m.out {
		return 0
	}
	if m.nodes >= m.budget || (m.nodes%16 == 0 && m.late()) {
		m.out = true
		return 0
	}
	if depth == 0 {
		v := m.leaf(pos)
		if seat != m.root {
			v = -v
		}
		return v
	}
	legal := m.ops.legal(pos, seat)
	if len(legal) == 0 {
		return 0
	}
	best := -minimaxInf
	for _, sel := range legal {
		res, err := m.ops.play(pos, seat, sel)
		m.nodes++
		if err != nil {
			continue
		}
		var v int
		switch {
		case res.ended:
			v = m.terminal(res, ply)
			if seat != m.root {
				v = -v
			}
		case res.next == seat:
			v = m.value(res.pos, res.next, depth-1, alpha, beta, ply+1)
		default:
			v = -m.value(res.pos, res.next, depth-1, -beta, -alpha, ply+1)
		}
		if m.out {
			return 0
		}
		best = max(best, v)
		alpha = max(alpha, best)
		if alpha >= beta {
			break
		}
	}
	return best
}

// --- mcts -------------------------------------------------------------------

type node[P any] struct {
	pos      P
	seat     int // the seat to move here
	sel      Selection
	parent   *node[P]
	children []*node[P]
	untried  []Selection
	visits   int
	score    float64 // for the seat that moved into this node
	mover    int     // that seat, 0 at the root
	terminal bool
	winner   int
}

// mcts is UCT Monte-Carlo tree search with random playouts over the safe
// selections, the host's mcts bot over rules.
func (s *search[P]) mcts(pos P, seat int, legal []Selection, iterations, plies int, exploration float64) BotReply {
	wins, safe, _ := s.classify(pos, seat, legal)
	if len(wins) > 0 {
		sel := s.pick(wins)
		return BotReply{Selection: sel, WinRate: 1, Alternatives: []Candidate{{Selection: sel, WinRate: 1}}}
	}
	candidates := safe
	if len(candidates) == 0 {
		candidates = legal
	}
	if len(candidates) == 1 {
		return even(candidates[0])
	}
	root := &node[P]{pos: pos, seat: seat, untried: slices.Clone(candidates)}
	for it := 0; ; it++ {
		if iterations > 0 && it >= iterations {
			break
		}
		// A playout in a module takes milliseconds, so the time is read before
		// each one: the host stops a search that keeps it waiting.
		if s.late() {
			break
		}
		s.iterate(root, plies, exploration)
	}
	children := slices.Clone(root.children)
	slices.SortStableFunc(children, func(a, b *node[P]) int { return b.visits - a.visits })
	if len(children) == 0 {
		return even(s.pick(candidates))
	}
	out := BotReply{Selection: children[0].sel, Visits: root.visits}
	for _, ch := range children[:min(4, len(children))] {
		out.Alternatives = append(out.Alternatives, Candidate{Selection: ch.sel, WinRate: ch.score / float64(ch.visits), Visits: ch.visits})
	}
	out.WinRate = out.Alternatives[0].WinRate
	return out
}

func (s *search[P]) iterate(root *node[P], plies int, exploration float64) {
	n := root
	for len(n.untried) == 0 && len(n.children) > 0 && !n.terminal {
		n = choose(n, exploration)
	}
	if !n.terminal && len(n.untried) > 0 {
		i := s.rng.IntN(len(n.untried))
		sel := n.untried[i]
		n.untried[i] = n.untried[len(n.untried)-1]
		n.untried = n.untried[:len(n.untried)-1]
		res, err := s.ops.play(n.pos, n.seat, sel)
		if err != nil {
			return
		}
		child := &node[P]{pos: res.pos, seat: res.next, sel: sel, parent: n, mover: n.seat, terminal: res.ended, winner: res.winner}
		if !child.terminal {
			// Cloned: the search takes selections out of it, and a State may
			// keep the list it gives.
			child.untried = slices.Clone(s.ops.legal(child.pos, child.seat))
			if len(child.untried) == 0 {
				child.terminal = true // no selection: scored as a draw
			}
		}
		n.children = append(n.children, child)
		n = child
	}
	winner, decided, hv, hvSeat, haveHV := s.playout(n, plies)
	for ; n != nil; n = n.parent {
		n.visits++
		if n.mover == 0 {
			continue
		}
		switch {
		case !decided && haveHV:
			v := hv
			if n.mover != hvSeat {
				v = -v
			}
			n.score += 0.5 + 0.5*v
		case !decided, winner == 0:
			n.score += 0.5
		case winner == n.mover:
			n.score++
		}
	}
}

func choose[P any](n *node[P], exploration float64) *node[P] {
	var best *node[P]
	bestVal := -1.0
	logN := math.Log(float64(n.visits))
	for _, ch := range n.children {
		val := ch.score/float64(ch.visits) + exploration*math.Sqrt(logN/float64(ch.visits))
		if best == nil || val > bestVal {
			best, bestVal = ch, val
		}
	}
	return best
}

// playout plays at random from n to the end of the game or for plies
// selections. winner is 0 for a draw; decided is false when it was cut short,
// and the game's heuristic, if it has one, then values where it stopped.
func (s *search[P]) playout(n *node[P], plies int) (winner int, decided bool, hv float64, hvSeat int, haveHV bool) {
	if n.terminal {
		return n.winner, true, 0, 0, false
	}
	pos, seat := n.pos, n.seat
	for range plies {
		legal := s.ops.legal(pos, seat)
		if len(legal) == 0 {
			return 0, true, 0, 0, false
		}
		res, err := s.ops.play(pos, seat, s.pick(legal))
		if err != nil {
			return 0, false, 0, 0, false
		}
		if res.ended {
			return res.winner, true, 0, 0, false
		}
		pos, seat = res.pos, res.next
	}
	if v, ok := s.ops.value(pos, seat); ok {
		return 0, false, v, seat, true
	}
	return 0, false, 0, 0, false
}
