package mycelium

import "slices"

// strand is the length of the longest strand of cluster g ending at node from
// — a path through the cluster from node to touching node, turning as it
// likes and never visiting a node twice — or limit, if one that long exists.
//
// A hyphae spread converts nodes of a single enemy cluster (each converted node
// touches the last, so they all belonged to the cluster the spread first
// entered), so its size is the useful limit; the two clusters share 61 cells,
// so the limit is never more than 30.
//
// The longest path is a hard problem in general, and the search is exhaustive.
// Two things keep it quick: it tries the most constrained node first, which on
// a hex grid usually finds a path through everything at once, and it drops a
// partial strand that cannot beat the best so far, by the bound below. On
// 80,000 random clusters, from sparse to nearly full boards, the slowest search
// took 10ms and none came near strandBudget, which is there so that a position
// nobody has met yet cannot stall a game: past it, the longest strand found so
// far counts.
func (e *eng) strand(from int, g cluster, limit int) int {
	n, _ := e.strandWithin(from, g, limit, strandBudget)
	return n
}

// strandBudget caps the partial strands one search looks at.
const strandBudget = 20000

// strandWithin is strand looking at no more than budget partial strands; it
// reports whether it finished.
func (e *eng) strandWithin(from int, g cluster, limit, budget int) (int, bool) {
	limit = min(limit, len(g))
	in := make([]bool, len(e.cells))
	for _, i := range g {
		in[i] = true
	}
	on := make([]bool, len(e.cells))
	free := func(j int) bool { return in[j] && !on[j] }
	best := 0
	var walk func(at, length int)
	walk = func(at, length int) {
		best = max(best, length)
		budget--
		if best >= limit || budget < 0 || length+e.bound(at, free) <= best {
			return
		}
		var next []int
		for _, j := range e.g.nbr[at] {
			if free(j) {
				next = append(next, j)
			}
		}
		onward := func(j int) int {
			n := 0
			for _, k := range e.g.nbr[j] {
				if free(k) {
					n++
				}
			}
			return n
		}
		slices.SortStableFunc(next, func(a, b int) int { return onward(a) - onward(b) })
		for _, j := range next {
			on[j] = true
			walk(j, length+1)
			on[j] = false
			if best >= limit || budget < 0 {
				return
			}
		}
	}
	on[from] = true
	walk(from, 1)
	return best, budget >= 0
}

// bound is an upper limit on how many free nodes a path leaving at can still
// take in. The free nodes reachable from at split into blocks at the nodes
// that cut them in two (the blocks of a block-cut tree). A path that leaves a
// block through such a node can never come back, so it takes in at most one
// branch of the tree: the bound is the heaviest branch from at, each block
// counting its nodes but the one it is entered by.
func (e *eng) bound(at int, free func(int) bool) int {
	n := len(e.cells)
	ok := func(j int) bool { return j == at || free(j) }
	disc, low := make([]int, n), make([]int, n)
	timer := 0
	var edges [][2]int
	var blocks [][]int
	of := make([][]int, n) // node → the blocks holding it
	var dfs func(u, parent int)
	dfs = func(u, parent int) {
		timer++
		disc[u], low[u] = timer, timer
		for _, v := range e.g.nbr[u] {
			if !ok(v) || v == parent {
				continue
			}
			switch {
			case disc[v] == 0:
				edges = append(edges, [2]int{u, v})
				dfs(v, u)
				low[u] = min(low[u], low[v])
				if low[v] < disc[u] {
					continue
				}
				// u cuts v's side off: the edges stacked since (u, v) are a block.
				var block []int
				for {
					ed := edges[len(edges)-1]
					edges = edges[:len(edges)-1]
					for _, x := range ed {
						if !slices.Contains(block, x) {
							block = append(block, x)
						}
					}
					if ed == [2]int{u, v} {
						break
					}
				}
				for _, x := range block {
					of[x] = append(of[x], len(blocks))
				}
				blocks = append(blocks, block)
			case disc[v] < disc[u]:
				edges = append(edges, [2]int{u, v})
				low[u] = min(low[u], disc[v])
			}
		}
	}
	dfs(at, -1)
	var gain func(b, entry int) int
	gain = func(b, entry int) int {
		more := 0
		for _, c := range blocks[b] {
			if c == entry {
				continue
			}
			for _, b2 := range of[c] {
				if b2 != b {
					more = max(more, gain(b2, c))
				}
			}
		}
		return len(blocks[b]) - 1 + more
	}
	best := 0
	for _, b := range of[at] {
		best = max(best, gain(b, at))
	}
	return best
}
