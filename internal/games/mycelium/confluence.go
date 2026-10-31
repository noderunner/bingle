package mycelium

import (
	"slices"

	"github.com/noderunner/bingle/internal/engine/board"
	"github.com/noderunner/bingle/internal/engine/state"
)

// A confluence is resolved one decision at a time. Each decision is a spore
// spread (one conversion), the first conversion of a hyphae spread, a further
// conversion of the hyphae spread under way, or the end of that spread. After
// each decision resolve finds the next seat with a spread to choose, moving the
// initiative on — and from the spore phase to the hyphae phase, and from that
// to the end of the confluence — while nobody has one.

// cluster is a connected group of one colour's nodes, as cell indices.
type cluster []int

// clusters returns every cluster of the given colour.
func (e *eng) clusters(color string) []cluster {
	seen := make([]bool, len(e.cells))
	var out []cluster
	for i, c := range e.cells {
		if c != color || seen[i] {
			continue
		}
		group := cluster{i}
		seen[i] = true
		for k := 0; k < len(group); k++ {
			for _, n := range e.g.nbr[group[k]] {
				if e.cells[n] == color && !seen[n] {
					seen[n] = true
					group = append(group, n)
				}
			}
		}
		out = append(out, group)
	}
	return out
}

// clusterAt is the cluster holding cell i.
func (e *eng) clusterAt(i int) cluster {
	for _, g := range e.clusters(e.cells[i]) {
		if slices.Contains(g, i) {
			return g
		}
	}
	return nil
}

// isHyphae reports whether a cluster touches an enemy node; one that does not
// is a spore cluster.
func (e *eng) isHyphae(g cluster) bool {
	color := e.cells[g[0]]
	for _, i := range g {
		for _, n := range e.g.nbr[i] {
			if e.cells[n] != "" && e.cells[n] != color {
				return true
			}
		}
	}
	return false
}

// spent reports whether a cluster has had its spread in this phase.
func (e *eng) spent(g cluster) bool {
	for _, i := range g {
		if slices.Contains(e.d.Confluence.Spent, string(e.g.ids[i])) {
			return true
		}
	}
	return false
}

// eligible lists seat's clusters that may spread now: the size of the
// initiative, of the phase's kind, and not yet spread in this phase.
func (e *eng) eligible(seat int) []cluster {
	cf := e.d.Confluence
	var out []cluster
	for _, g := range e.clusters(seatColors[seat]) {
		if len(g) == cf.Initiative && e.isHyphae(g) == (cf.Phase == phaseHyphae) && !e.spent(g) {
			out = append(out, g)
		}
	}
	return out
}

// spreads lists seat's decisions in the confluence.
func (e *eng) spreads(seat int) []state.Move {
	cf := e.d.Confluence
	var out []state.Move
	if cf.Head != "" {
		// A hyphae spread under way: convert another enemy node touching the
		// last one, or stop.
		for _, n := range e.enemyNbrs(e.g.index[board.PositionID(cf.Head)]) {
			out = append(out, state.Move{Action: actHyphae, To: e.g.ids[n]})
		}
		return append(out, state.Move{Action: actDone})
	}
	for _, g := range e.eligible(seat) {
		for _, src := range g {
			if cf.Phase == phaseSpore {
				// Line of sight across empty cells to the first node; a spore
				// cluster touches no enemy, so a seen enemy is never adjacent.
				for d := range dirVec {
					for _, j := range e.g.ray[src][d] {
						if e.cells[j] != "" {
							if e.cells[j] != e.cells[src] {
								out = append(out, state.Move{Action: actSpore, From: e.g.ids[src], To: e.g.ids[j]})
							}
							break
						}
					}
				}
				continue
			}
			// A hyphae spread starts from the end of a strand, so from any node
			// of the cluster, into an enemy node it touches.
			for _, n := range e.enemyNbrs(src) {
				out = append(out, state.Move{Action: actHyphae, From: e.g.ids[src], To: e.g.ids[n]})
			}
		}
	}
	return out
}

// enemyNbrs are the cells touching i that hold the other colour.
func (e *eng) enemyNbrs(i int) []int {
	var out []int
	for _, n := range e.g.nbr[i] {
		if e.cells[n] != "" && e.cells[n] != e.cells[i] {
			out = append(out, n)
		}
	}
	return out
}

// spread plays one confluence decision for seat.
func (e *eng) spread(mv state.Move, seat int) {
	cf := e.d.Confluence
	color := seatColors[seat]
	switch {
	case mv.Action == actDone:
		e.say("%s ends the hyphae spread.", seatLabel(seat))
		cf.Head, cf.Left = "", 0
	case mv.Action == actSpore:
		e.spend(e.clusterAt(e.g.index[mv.From]))
		e.convert(e.g.index[mv.To], color)
		e.say("%s's spore at %s converts %s.", seatLabel(seat), mv.From, mv.To)
	case mv.From != "":
		// The first conversion of a hyphae spread fixes its length: the
		// longest strand of the cluster that ends at the chosen node.
		from, to := e.g.index[mv.From], e.g.index[mv.To]
		g := e.clusterAt(from)
		n := e.strand(from, g, len(e.clusterAt(to)))
		e.spend(g)
		e.convert(to, color)
		cf.Head, cf.Left = string(mv.To), n-1
		e.say("%s's hyphae spread from %s converts %s (%d at most).", seatLabel(seat), mv.From, mv.To, n)
	default:
		e.convert(e.g.index[mv.To], color)
		cf.Head, cf.Left = string(mv.To), cf.Left-1
		e.say("%s's hyphae spread converts %s.", seatLabel(seat), mv.To)
	}
	if cf.Head != "" && (cf.Left == 0 || len(e.enemyNbrs(e.g.index[board.PositionID(cf.Head)])) == 0) {
		cf.Head, cf.Left = "", 0
	}
	if cf.Head == "" {
		e.resolve(other(seat))
	}
}

// convert turns the node on cell i to color; a converted node is new to its
// colony, so it no longer counts as having spread.
func (e *eng) convert(i int, color string) {
	e.cells[i] = color
	cf := e.d.Confluence
	cf.Spent = slices.DeleteFunc(cf.Spent, func(c string) bool { return c == string(e.g.ids[i]) })
}

// spend marks a cluster as having spread in this phase.
func (e *eng) spend(g cluster) {
	cf := e.d.Confluence
	for _, i := range g {
		cf.Spent = append(cf.Spent, string(e.g.ids[i]))
	}
	slices.Sort(cf.Spent)
	cf.Spent = slices.Compact(cf.Spent)
}

// resolve hands the confluence to the next seat with a spread to choose,
// preferring prefer, and otherwise moves it on until one has or it ends.
func (e *eng) resolve(prefer int) {
	for {
		cf := e.d.Confluence
		for _, s := range [2]int{prefer, other(prefer)} {
			if len(e.spreads(s)) > 0 {
				e.d.Turn = s
				return
			}
		}
		// Nobody can spread at this size: the next initiative, where the
		// player who did not take the centre chooses first.
		cf.Initiative++
		prefer = other(cf.Initiator)
		if cf.Initiative <= e.largestCluster() {
			continue
		}
		if cf.Phase == phaseHyphae {
			e.endConfluence()
			return
		}
		cf.Phase, cf.Initiative, cf.Spent = phaseHyphae, 1, nil
		e.say("The spore phase is over; the hyphae spread.")
	}
}

func (e *eng) largestCluster() int {
	n := 0
	for s := 1; s <= 2; s++ {
		for _, g := range e.clusters(seatColors[s]) {
			n = max(n, len(g))
		}
	}
	return n
}

// endConfluence clears the centre and decides whether a colony has been wiped
// out; if not, the player who did not take the centre places next.
func (e *eng) endConfluence() {
	initiator := e.d.Confluence.Initiator
	e.d.Confluence = nil
	if c := e.cells[e.g.centre]; c != "" {
		e.cells[e.g.centre] = ""
		e.say("The confluence ends and the %s node on the centre is removed.", c)
	}
	n := e.count()
	switch {
	case n[0] == 0 && n[1] == 0:
		e.finish(other(initiator), "confluence")
		e.say("Both colonies are wiped out, and %s took the centre: %s wins.", seatLabel(initiator), seatLabel(other(initiator)))
	case n[0] == 0 || n[1] == 0:
		w := 1
		if n[0] == 0 {
			w = 2
		}
		e.finish(w, "confluence")
		e.say("%s has no nodes left: %s wins.", seatLabel(other(w)), seatLabel(w))
	default:
		e.d.Turn = other(initiator)
		e.say("%s places next.", seatLabel(e.d.Turn))
	}
}
