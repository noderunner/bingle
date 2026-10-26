package state

import (
	"fmt"
	"strings"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/dsl"
)

// Outcome is the result of evaluating a game's win condition after a move.
type Outcome struct {
	Ended bool
	// Winner is the winning seat; nil with Ended set means a draw.
	Winner *int
	// Reason names the rule that ended the game (e.g. "road").
	Reason string
	// Detail adds specifics such as a final score.
	Detail string
}

// evaluateWin computes whether the game has ended after the mover's move
// produced ns, per game.Definition.WinCondition.
func evaluateWin(game *catalog.CompiledGame, ns *BoardState, moverSeat, nextSeat int) (Outcome, error) {
	wc := game.Definition.WinCondition
	switch wc.Type {
	case "none":
		return Outcome{}, nil
	case "callback":
		winner, ended := game.Callbacks.CheckWinner(ns)
		return Outcome{Ended: ended, Winner: winner, Reason: "callback"}, nil
	case "rules":
		return evaluateRules(game, ns, wc.Rules, moverSeat, nextSeat), nil
	default:
		return Outcome{}, fmt.Errorf("state: unsupported win_condition.type %q", wc.Type)
	}
}

// Evaluate evaluates the win condition against st as if seat had just
// moved. It is used by clients that need the current outcome of a position.
func Evaluate(game *catalog.CompiledGame, st *BoardState, moverSeat int) (Outcome, error) {
	if rs, ok := RulesetOf(game); ok {
		return rs.Outcome(game, st), nil
	}
	next, err := nextTurnSeat(game, moverSeat)
	if err != nil {
		return Outcome{}, err
	}
	return evaluateWin(game, st, moverSeat, next)
}

func evaluateRules(game *catalog.CompiledGame, ns *BoardState, rules []dsl.WinRuleDef, mover, next int) Outcome {
	var view *boardView // built on first use, shared by every rule
	for _, r := range rules {
		reason := r.Reason
		if reason == "" {
			reason = r.Type
		}
		switch r.Type {
		case "connect", "line", "ring":
			if view == nil {
				view = newBoardView(game, ns)
			}
			var winners []int
			for _, seat := range game.Definition.Seats {
				if r.Seat != 0 && r.Seat != seat.Seat {
					continue
				}
				var hit bool
				switch r.Type {
				case "connect":
					hit = connects(view, seat.Color, r)
				case "ring":
					hit = hasRing(view, seat.Color, r)
				default:
					hit = hasLine(view, seat.Color, r)
				}
				if hit {
					winners = append(winners, seat.Seat)
				}
			}
			if len(winners) == 0 {
				continue
			}
			winner := winners[0]
			for _, w := range winners {
				if w == mover {
					winner = mover // the player who made the move wins ties
				}
			}
			return ruleOutcome(game, r, winner, reason)

		case "reach":
			region, _ := game.Board.Region(r.Region)
			var winners []int
			for _, seat := range game.Definition.Seats {
				if r.Seat != 0 && r.Seat != seat.Seat {
					continue
				}
				for _, id := range region {
					if topMatches(game, ns, id, seat.Color, r.Tag, r.Piece) {
						winners = append(winners, seat.Seat)
						break
					}
				}
			}
			if len(winners) == 0 {
				continue
			}
			winner := winners[0]
			for _, w := range winners {
				if w == mover {
					winner = mover
				}
			}
			return ruleOutcome(game, r, winner, reason)

		case "absent":
			// The named piece type has left the board: seat wins.
			present := false
			for _, stack := range ns.cells {
				for _, o := range stack {
					if o.Piece == r.Piece {
						present = true
					}
				}
			}
			if present || r.Seat == 0 {
				continue
			}
			winner := r.Seat
			return ruleOutcome(game, r, winner, reason)

		case "pool":
			var winners []int
			for _, seat := range game.Definition.Seats {
				if r.Seat != 0 && r.Seat != seat.Seat {
					continue
				}
				if ns.Reserve(seat.Color, r.Pool) >= r.AtLeast {
					winners = append(winners, seat.Seat)
				}
			}
			if len(winners) == 0 {
				continue
			}
			winner := winners[0]
			for _, w := range winners {
				if w == mover {
					winner = mover
				}
			}
			return ruleOutcome(game, r, winner, reason)

		case "end":
			if !endTriggered(game, ns, r, next) {
				continue
			}
			return decide(game, ns, r.Decide, mover, reason)
		}
	}
	return Outcome{}
}

// ruleOutcome turns "seat satisfied rule r" into the game outcome: the seat
// wins, or loses (so its opponent wins) when the rule says result "lose".
func ruleOutcome(game *catalog.CompiledGame, r dsl.WinRuleDef, seat int, reason string) Outcome {
	if r.Result != "lose" {
		return Outcome{Ended: true, Winner: &seat, Reason: reason}
	}
	if opp, ok := game.Opponent(seat); ok {
		return Outcome{Ended: true, Winner: &opp, Reason: reason}
	}
	return Outcome{Ended: true, Reason: reason}
}

// topMatches reports whether the top piece at id has color and satisfies the
// tag/piece filter.
func topMatches(game *catalog.CompiledGame, ns *BoardState, id board.PositionID, color, tag, piece string) bool {
	top, ok := ns.Get(id)
	if !ok || top.Color != color {
		return false
	}
	if piece != "" && top.Piece != piece {
		return false
	}
	if tag != "" && !game.Pieces[top.Piece].HasTag(tag) {
		return false
	}
	return true
}

// boardView is a snapshot of the top piece on every square, by position
// index, taken once per win evaluation so that rules scanning the board do
// no string-keyed map lookups.
type boardView struct {
	game  *catalog.CompiledGame
	color []string // top piece's colour per square; "" = empty
	piece []string // top piece's type per square
}

func newBoardView(game *catalog.CompiledGame, ns *BoardState) *boardView {
	g := game.Board
	v := &boardView{game: game, color: make([]string, g.Len()), piece: make([]string, g.Len())}
	for id, stack := range ns.cells {
		if len(stack) == 0 {
			continue
		}
		if i, ok := g.Index(id); ok {
			top := stack[len(stack)-1]
			v.color[i], v.piece[i] = top.Color, top.Piece
		}
	}
	return v
}

// owned marks, by position index, the squares whose top piece belongs to
// color and satisfies the tag/piece filter, and reports how many there are.
func (v *boardView) owned(color, tag, piece string) ([]bool, int) {
	own := make([]bool, len(v.color))
	n := 0
	for i, c := range v.color {
		if c != color || (piece != "" && v.piece[i] != piece) {
			continue
		}
		if tag != "" && !v.game.Pieces[v.piece[i]].HasTag(tag) {
			continue
		}
		own[i] = true
		n++
	}
	return own, n
}

// connects reports whether color owns a connected group of matching squares
// that touches both regions of any pair in r.Regions (or, with r.Of, at
// least r.Touch of the listed regions).
func connects(v *boardView, color string, r dsl.WinRuleDef) bool {
	game := v.game
	g := game.Board
	own, count := v.owned(color, r.Tag, r.Piece)
	if count == 0 {
		return false
	}
	if len(r.Of) > 0 {
		return touchesEnough(game, own, r)
	}

	n := g.Len()
	queue := make([]int, 0, n)
	for _, pair := range r.Regions {
		starts, _ := g.RegionIndices(pair[0])
		targets, _ := g.RegionIndices(pair[1])
		isTarget := make([]bool, n)
		for _, t := range targets {
			isTarget[t] = true
		}
		seen := make([]bool, n)
		queue = queue[:0]
		for _, s := range starts {
			if own[s] && !seen[s] {
				seen[s] = true
				queue = append(queue, s)
			}
		}
		for head := 0; head < len(queue); head++ {
			cur := queue[head]
			if isTarget[cur] {
				return true
			}
			for _, nb := range g.NeighborIndices(cur) {
				if !seen[nb] && own[nb] {
					seen[nb] = true
					queue = append(queue, nb)
				}
			}
		}
	}
	return false
}

// touchesEnough reports whether some connected group of owned squares
// touches at least r.Touch of the regions listed in r.Of.
func touchesEnough(game *catalog.CompiledGame, own []bool, r dsl.WinRuleDef) bool {
	g := game.Board
	n := g.Len()
	bits := make([]uint32, n) // bit i set: the square lies in region r.Of[i]
	for i, name := range r.Of {
		idx, _ := g.RegionIndices(name)
		for _, p := range idx {
			bits[p] |= 1 << uint(i)
		}
	}
	seen := make([]bool, n)
	queue := make([]int, 0, n)
	for start := 0; start < n; start++ {
		if seen[start] || !own[start] {
			continue
		}
		var touched uint32
		seen[start] = true
		queue = append(queue[:0], start)
		for head := 0; head < len(queue); head++ {
			cur := queue[head]
			touched |= bits[cur]
			if bitsSet(touched) >= r.Touch {
				return true
			}
			for _, nb := range g.NeighborIndices(cur) {
				if !seen[nb] && own[nb] {
					seen[nb] = true
					queue = append(queue, nb)
				}
			}
		}
	}
	return false
}

func bitsSet(v uint32) int {
	n := 0
	for ; v != 0; v &= v - 1 {
		n++
	}
	return n
}

// hasRing reports whether color's matching pieces enclose at least one
// square (Havannah's ring). A square is enclosed when it is not a matching
// piece and cannot be reached from the board's rim without crossing one; on
// a hex board that can only be because a closed loop of matching, mutually
// adjacent pieces surrounds it.
func hasRing(v *boardView, color string, r dsl.WinRuleDef) bool {
	g := v.game.Board
	own, count := v.owned(color, r.Tag, r.Piece)
	// A ring needs at least six pieces.
	if count < 6 {
		return false
	}
	n := g.Len()
	outside := make([]bool, n)
	queue := make([]int, 0, n)
	// The rim of a hex board is the set of squares with fewer than six
	// neighbours.
	for i := 0; i < n; i++ {
		if len(g.NeighborIndices(i)) < 6 && !own[i] {
			outside[i] = true
			queue = append(queue, i)
		}
	}
	for head := 0; head < len(queue); head++ {
		for _, nb := range g.NeighborIndices(queue[head]) {
			if !outside[nb] && !own[nb] {
				outside[nb] = true
				queue = append(queue, nb)
			}
		}
	}
	// Enclosed squares exist iff the free squares reached from outside are
	// fewer than all the non-owned squares.
	return len(queue) < n-count
}

// hasLine reports whether color has r.Length matching squares in a row
// along a grid axis or diagonal (the three axes of a hex board).
func hasLine(v *boardView, color string, r dsl.WinRuleDef) bool {
	g := v.game.Board
	axes := [][2]int{{1, 0}, {0, 1}, {1, 1}, {1, -1}}
	if g.IsHex() {
		// Axial coordinates: (1,1) is not adjacent; the three hex axes are
		// the two coordinate axes and the (1,-1) diagonal.
		axes = [][2]int{{1, 0}, {0, 1}, {1, -1}}
	}
	own, count := v.owned(color, r.Tag, r.Piece)
	if count < r.Length {
		return false
	}
	for i, ok := range own {
		if !ok {
			continue
		}
		c, hasC := g.CoordOfIndex(i)
		if !hasC {
			continue
		}
		for _, ax := range axes {
			run := 1
			for step := 1; step < r.Length; step++ {
				j, ok := g.IndexAt(board.Coord{X: c.X + step*ax[0], Y: c.Y + step*ax[1]})
				if !ok || !own[j] {
					break
				}
				run++
			}
			if run >= r.Length {
				return true
			}
		}
	}
	return false
}

func endTriggered(game *catalog.CompiledGame, ns *BoardState, rule dsl.WinRuleDef, nextSeat int) bool {
	for _, w := range rule.When {
		switch w {
		case "ply-limit":
			if ns.Ply() >= rule.Plies {
				return true
			}
		case "passes":
			if ns.Passes() >= rule.Count {
				return true
			}
		case "board-full":
			occupied := 0
			for _, stack := range ns.cells {
				if len(stack) > 0 {
					occupied++
				}
			}
			if occupied >= game.Board.Len() {
				return true
			}
		case "reserve-empty":
			if len(game.Definition.Reserves) == 0 {
				continue
			}
			for _, seat := range game.Definition.Seats {
				if ns.ReserveTotal(seat.Color) == 0 {
					return true
				}
			}
		case "no-legal-moves":
			if !HasLegalMove(game, ns, nextSeat) {
				return true
			}
		}
	}
	return false
}

func decide(game *catalog.CompiledGame, ns *BoardState, d *dsl.DecideDef, mover int, reason string) Outcome {
	switch d.Type {
	case "mover-wins":
		w := mover
		return Outcome{Ended: true, Winner: &w, Reason: reason}
	case "mover-loses":
		if opp, ok := game.Opponent(mover); ok {
			return Outcome{Ended: true, Winner: &opp, Reason: reason}
		}
		return Outcome{Ended: true, Reason: reason}
	case "score":
		scores := areaScores(game, ns, d.Komi)
		best, bestSeats := -1e18, []int(nil)
		var detail []string
		for _, seat := range game.Definition.Seats {
			sc := scores[seat.Seat]
			label := seat.Label
			if label == "" {
				label = seat.Color
			}
			detail = append(detail, fmt.Sprintf("%s %s", label, formatScore(sc)))
			switch {
			case sc > best:
				best, bestSeats = sc, []int{seat.Seat}
			case sc == best:
				bestSeats = append(bestSeats, seat.Seat)
			}
		}
		out := Outcome{Ended: true, Reason: reason, Detail: strings.Join(detail, " – ")}
		if len(bestSeats) == 1 {
			w := bestSeats[0]
			out.Winner = &w
		}
		return out
	case "most":
		best, bestSeats := -1, []int(nil)
		for _, seat := range game.Definition.Seats {
			n := 0
			for _, id := range ns.Occupied() {
				if topMatches(game, ns, id, seat.Color, d.Tag, d.Piece) {
					n++
				}
			}
			switch {
			case n > best:
				best, bestSeats = n, []int{seat.Seat}
			case n == best:
				bestSeats = append(bestSeats, seat.Seat)
			}
		}
		if len(bestSeats) == 1 {
			w := bestSeats[0]
			return Outcome{Ended: true, Winner: &w, Reason: reason}
		}
		return Outcome{Ended: true, Reason: reason}
	default: // "draw"
		return Outcome{Ended: true, Reason: reason}
	}
}

func formatScore(v float64) string {
	if v == float64(int(v)) {
		return fmt.Sprintf("%d", int(v))
	}
	return fmt.Sprintf("%.1f", v)
}

// areaScores returns each seat's area score: its stones on the board plus
// the empty regions bordered only by its color, plus its komi. Stones are
// never removed at the end (Tromp-Taylor style): dead stones must be
// captured during play.
func areaScores(game *catalog.CompiledGame, st *BoardState, komi map[string]float64) map[int]float64 {
	g := game.Board
	byColor := map[string]int{}
	for _, seat := range game.Definition.Seats {
		byColor[seat.Color] = seat.Seat
	}
	out := map[int]float64{}
	for _, seat := range game.Definition.Seats {
		out[seat.Seat] = komi[fmt.Sprintf("%d", seat.Seat)]
	}

	seen := map[board.PositionID]bool{}
	for _, id := range g.Positions() {
		top, occupied := st.Get(id)
		if occupied {
			if seat, ok := byColor[top.Color]; ok {
				out[seat]++
			}
			continue
		}
		if seen[id] {
			continue
		}
		// Flood the empty region, noting which colors border it.
		region := 0
		borders := map[string]bool{}
		queue := []board.PositionID{id}
		seen[id] = true
		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]
			region++
			for _, n := range g.Neighbors(cur) {
				if t, occ := st.Get(n); occ {
					borders[t.Color] = true
				} else if !seen[n] {
					seen[n] = true
					queue = append(queue, n)
				}
			}
		}
		if len(borders) == 1 {
			for color := range borders {
				if seat, ok := byColor[color]; ok {
					out[seat] += float64(region)
				}
			}
		}
	}
	return out
}
