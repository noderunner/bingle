package bots

import (
	"sync"
	"sync/atomic"
)

// How hard the bots have been thinking.
//
// A search is told to count its own nodes and add them once, when it
// finishes, rather than to report them through the Bot interface: a bot is a
// stateless singleton shared by every room, several searches run at once, and
// threading a counter through minimax and MCTS would put bookkeeping in the
// hot loop. One atomic add per search costs nothing and the total is exact.
//
// internal/metrics reads these at scrape time (SetBotNodeSource).
var nodeCounts sync.Map // bot id -> *atomic.Int64

// addNodes records that a search by bot looked at n positions.
func addNodes(bot string, n int) {
	if n <= 0 {
		return
	}
	c, ok := nodeCounts.Load(bot)
	if !ok {
		c, _ = nodeCounts.LoadOrStore(bot, new(atomic.Int64))
	}
	c.(*atomic.Int64).Add(int64(n))
}

// NodeCounts is how many positions each bot has looked at since the server
// started, by bot id.
func NodeCounts() map[string]int64 {
	out := map[string]int64{}
	nodeCounts.Range(func(k, v any) bool {
		out[k.(string)] = v.(*atomic.Int64).Load()
		return true
	})
	return out
}
