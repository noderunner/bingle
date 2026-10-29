package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// moveBuckets count moves, not seconds: how many legal moves a position
// offers, from a forced single move to the thousand-odd of a wide-open
// Cthulhu Wars turn.
var moveBuckets = []float64{1, 2, 3, 5, 8, 13, 21, 34, 55, 89, 150, 250, 500, 1000}

// MoveResult is how a move turned out. The set is closed, because it is a
// label.
type MoveResult string

// The outcomes of a move.
const (
	// MoveApplied: the move was legal and the game moved on.
	MoveApplied MoveResult = "applied"
	// MoveIllegal: the ruleset refused it. A spike means a rules change or a
	// stale client, and is one of the two most valuable things in this plan.
	MoveIllegal MoveResult = "illegal"
	// MoveFailed: something other than the rules went wrong — the position
	// would not unmarshal, the database refused the write.
	MoveFailed MoveResult = "failed"
)

// Slot names the pools whose queues are worth watching: the CPU semaphores
// that are where load first hurts. Closed set, because it is a label.
type Slot string

// The pools.
const (
	SlotHint     Slot = "hint"
	SlotBot      Slot = "bot"
	SlotMatch    Slot = "match"
	SlotEnvBatch Slot = "env-batch"
)

// maxEndReason caps the length of an end-reason label. A ruleset's reasons are
// a short closed set ("ply-limit", "no-legal-moves", "resign"), but a label is
// published and a reason is a string from game code, so anything unexpected
// becomes "other" rather than a new series.
const maxEndReason = 40

// Move records a move the engine applied or refused: how it turned out, how
// long Apply took, and — when the game ended — why.
func (m *Metrics) Move(game string, result MoveResult, apply time.Duration) {
	if m == nil {
		return
	}
	m.gameMoves.WithLabelValues(game, string(result)).Inc()
	if result == MoveApplied {
		m.gameApply.WithLabelValues(game).Observe(apply.Seconds())
	}
	if result == MoveIllegal {
		m.gameIllegal.WithLabelValues(game).Inc()
	}
}

// Finished records a game that ended, and why.
//
// A spike in "ply-limit" or "no-legal-moves" means a rule stopped working,
// which is why this is one of the two series the plan calls most valuable.
func (m *Metrics) Finished(game, endReason string) {
	if m == nil {
		return
	}
	m.gameFinished.WithLabelValues(game, safeReason(endReason)).Inc()
}

// safeReason keeps the end-reason label a closed set.
func safeReason(reason string) string {
	if reason == "" {
		return "none"
	}
	if len(reason) > maxEndReason {
		return "other"
	}
	for i := 0; i < len(reason); i++ {
		c := reason[i]
		ok := c == '-' || c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
		if !ok {
			return "other"
		}
	}
	return reason
}

// Legal records how long working out the legal moves took, and how many there
// were.
func (m *Metrics) Legal(game string, d time.Duration, moves int) {
	if m == nil {
		return
	}
	m.gameLegal.WithLabelValues(game).Observe(d.Seconds())
	m.gameLegalMoves.WithLabelValues(game).Observe(float64(moves))
}

// BotSearch records one bot's turn of thinking.
func (m *Metrics) BotSearch(bot, game string, d time.Duration) {
	if m == nil {
		return
	}
	m.botSearch.WithLabelValues(bot, game).Observe(d.Seconds())
}

// SlotWait records how long a caller queued for one of the CPU pools. Queue
// *wait* is the number that matters as load grows: the pools stay full and the
// work takes the same time, so the only sign of trouble is that getting in
// takes longer.
func (m *Metrics) SlotWait(pool Slot, d time.Duration) {
	if m == nil {
		return
	}
	m.slotWait.WithLabelValues(string(pool)).Observe(d.Seconds())
}

// SlotEnter and SlotLeave track how much of a pool is in use.
func (m *Metrics) SlotEnter(pool Slot) {
	if m == nil {
		return
	}
	m.slotInUse.WithLabelValues(string(pool)).Inc()
}

// SlotLeave is SlotEnter's other half; always deferred.
func (m *Metrics) SlotLeave(pool Slot) {
	if m == nil {
		return
	}
	m.slotInUse.WithLabelValues(string(pool)).Dec()
}

// SetSlotSizes records how large each pool is, so "in use" can be read as a
// fraction of it.
func (m *Metrics) SetSlotSizes(sizes map[Slot]int) {
	if m == nil {
		return
	}
	for pool, n := range sizes {
		m.slotSize.WithLabelValues(string(pool)).Set(float64(n))
	}
}

// SetBatchWorkers records how many games a batch plays at once.
func (m *Metrics) SetBatchWorkers(n int) {
	if m == nil {
		return
	}
	m.batchWorkers.Set(float64(n))
}

// ClientError records one report from a browser, by the page it happened on.
// "refused" is a report the endpoint would not take — too many from one
// address, or nothing usable in it — which is worth seeing on its own: it says
// whether the endpoint is being abused rather than used.
func (m *Metrics) ClientError(page string) {
	if m == nil {
		return
	}
	m.clientErrors.WithLabelValues(page).Inc()
}

// botNodes is a Collector over however many positions each bot has looked at,
// read from internal/bots at scrape time: the bots count their own nodes and
// nothing has to be threaded through the search.
type botNodes struct {
	source func() map[string]int64
	desc   *prometheus.Desc
}

// Describe implements prometheus.Collector.
func (c *botNodes) Describe(ch chan<- *prometheus.Desc) { ch <- c.desc }

// Collect implements prometheus.Collector.
func (c *botNodes) Collect(ch chan<- prometheus.Metric) {
	if c.source == nil {
		return
	}
	for bot, n := range c.source() {
		ch <- prometheus.MustNewConstMetric(c.desc, prometheus.CounterValue, float64(n), bot)
	}
}

// SetBotNodeSource gives the registry something to ask about how hard the bots
// have been thinking.
func (m *Metrics) SetBotNodeSource(f func() map[string]int64) {
	if m == nil {
		return
	}
	m.botNodes.source = f
}
