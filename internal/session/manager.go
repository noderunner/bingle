// Package session is the layer between the transports (REST, WebSocket, MCP)
// and the engine: it creates, joins, plays, undoes and ends games, backed by
// internal/store for persistence and internal/engine/catalog for compiled game
// definitions. The Manager in this file holds the shared state; its methods are
// grouped by what they do:
//
//	create.go, gamedefs.go   starting a game; the list of game definitions
//	join.go, presence.go     taking a seat or watching; who is looking now
//	state.go, history.go     reading a game: snapshots, moves, legal moves, simulation
//	moves.go, botturns.go    playing a move and everything that follows it (bots, clocks)
//	undo.go, takeback.go     taking moves back
//	resign.go                resigning and draws
//	lobby.go, reactions.go,
//	autoplay.go, chat.go,
//	voice.go, notify.go      the features around a game
//	hub.go                   the live events sent to the room's sockets
//
// See docs/history/plan.md section 6.4b for the original contract of CreateGame and GetState.
package session

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/noderunner/bingle/internal/bots"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/metrics"
	"github.com/noderunner/bingle/internal/store"
	"github.com/noderunner/bingle/internal/trace"
)

// Manager orchestrates game lifecycle operations against a Store and a
// Catalog of compiled game definitions.
type Manager struct {
	store     *store.Store
	catalog   *catalog.Catalog
	hub       *hub
	bots      *bots.Registry
	log       *slog.Logger           // the server's logger; tests may replace it via SetLogger
	mx        *metrics.Metrics       // the registry; nil records nothing
	debug     DebugOptions           // which games and rooms get the firehose
	now       func() time.Time       // clock; tests may replace it via SetClock
	mu        sync.Mutex             // guards roomLocks itself, not room state
	roomLocks map[string]*sync.Mutex // lazily created, one per room code ever touched
	shuffles  shuffleTracker         // recent piece arrangements of bots (see avoidShuffles)
	chat      chatLimiter            // how often each name has spoken (see SendChat)
	presence  presenceTracker        // who says they are watching right now (see SetPresent)
	here      hereTracker            // the browsers that are about (see here.go)
	dms       dmBox                  // the direct messages, in memory (see dm.go)
	calls     callRegistry           // the one voice call each browser is in (see dm.go)
	voice     voiceTracker           // who is in the voice chat of each game (see SetVoice)
	notify    *notifier              // email notifications (nil when not set up; see EnableNotify)
	botsBusy  map[string]bool        // rooms with a background bot run, guarded by mu
	// autoPlayFrom is when the Auto-Play watcher started (zero when it is not
	// running, as in tests): a wait that ran while the server was down does
	// not count, see autoPlaySince. Guarded by mu.
	autoPlayFrom time.Time
}

// NewManager returns a *Manager backed by st and cat.
func NewManager(st *store.Store, cat *catalog.Catalog) *Manager {
	return &Manager{
		store:     st,
		catalog:   cat,
		hub:       newHub(),
		bots:      bots.NewRegistry(),
		now:       time.Now,
		roomLocks: make(map[string]*sync.Mutex),
	}
}

// SetClock replaces the manager's time source (used by tests to exercise
// move time limits without sleeping).
func (m *Manager) SetClock(now func() time.Time) { m.now = now }

// SetMetrics hands the manager the registry to record into. Without it
// nothing is recorded, which is what a server with BINGLE_METRICS_ADDR empty
// does and what most tests want.
func (m *Manager) SetMetrics(mx *metrics.Metrics) {
	m.mx = mx
	mx.SetSlotSizes(map[metrics.Slot]int{
		metrics.SlotHint: cap(hintSlots),
		metrics.SlotBot:  cap(botSlots),
	})
}

// DebugOptions narrow the debug firehose. `debug` across two dozen games is
// unreadable, so these say which of it to keep.
type DebugOptions struct {
	// Games limits the per-move lines to these game families; empty means
	// every game.
	Games []string
	// Room logs everything about one room whatever the level, for chasing a
	// single report. It is a room code, matched case-insensitively.
	Room string
	// Secrets allows a full position in a debug line instead of the public
	// view. Development only: a position carries committed dice, hidden
	// hands and RNG state (ADR 0063, and the plan's section 6g).
	Secrets bool
	// Paranoid makes the move funnel check the engine against itself after
	// every move. Development and CI only: it doubles Legal on every move.
	Paranoid bool
}

// SetDebug narrows the debug firehose; see DebugOptions.
func (m *Manager) SetDebug(o DebugOptions) { m.debug = o }

// debugGame reports whether per-move debug lines are wanted for this family.
func (m *Manager) debugGame(family string) bool {
	if len(m.debug.Games) == 0 {
		return true
	}
	for _, g := range m.debug.Games {
		if strings.EqualFold(g, family) {
			return true
		}
	}
	return false
}

// watchedRoom reports whether this is the one room BINGLE_DEBUG_ROOM names, in
// which case its lines are logged whatever the level.
func (m *Manager) watchedRoom(roomCode string) bool {
	return m.debug.Room != "" && strings.EqualFold(m.debug.Room, roomCode)
}

// SetLogger replaces the manager's logger. Without one it logs through slog's
// default logger, which is what the tests want.
func (m *Manager) SetLogger(lg *slog.Logger) {
	if lg != nil {
		m.log = lg
	}
}

// logger is the logger to use: the one the server handed over, or slog's
// default for a manager built straight by a test.
func (m *Manager) logger() *slog.Logger {
	if m.log == nil {
		return slog.Default()
	}
	return m.log
}

// Bots returns the registry of built-in bots.
func (m *Manager) Bots() *bots.Registry { return m.bots }

// Catalog returns the manager's game catalog.
func (m *Manager) Catalog() *catalog.Catalog { return m.catalog }

// Subscribe registers the caller as a live listener for roomCode's events
// (e.g. moves made by other players). It returns a receive-only channel and
// an unsubscribe function the caller must call (typically via defer) once
// done listening, to release the channel.
func (m *Manager) Subscribe(roomCode string) (<-chan Event, func()) {
	roomCode = canonRoom(roomCode)
	return m.hub.subscribe(roomCode)
}

// gameFamily is the family of a compiled game ("courtfall" for
// "courtfall~arena"), which is what a metric label or a span attribute may
// carry: the family is a closed set of about twenty-five, while a definition
// id with all its variants is not (ADR 0064).
func gameFamily(compiled *catalog.CompiledGame) string {
	if compiled == nil {
		return ""
	}
	return familyOf(compiled.Definition.Family, compiled.Definition.ID)
}

// familyOfGame is gameFamily by game definition id, for the few places that
// have the id but not the compiled game.
func (m *Manager) familyOfGame(defID string) string {
	compiled, ok := m.catalog.Get(defID)
	if !ok {
		return ""
	}
	return gameFamily(compiled)
}

// lockRoom returns an unlock function for roomCode's mutex, creating that
// mutex on first use. It never removes entries (room codes are cheap and
// bounded by how many games actually get created in a process's lifetime;
// not worth the complexity of eviction for v1).
//
// ctx is only for the span: waiting here is one of the few things in bingle
// that can take a long time while nothing appears to be happening, which is
// why docs/history/observability-plan.md names it as a boundary to time. The
// span covers the wait, not the time the lock is held.
func (m *Manager) lockRoom(ctx context.Context, roomCode string) (unlock func()) {
	_, span := trace.Start(ctx, "room.lock")
	defer span.End()

	m.mu.Lock()
	roomMu, ok := m.roomLocks[roomCode]
	if !ok {
		roomMu = &sync.Mutex{}
		m.roomLocks[roomCode] = roomMu
	}
	m.mu.Unlock()

	roomMu.Lock()
	return roomMu.Unlock
}

// canonRoom normalizes a user-typed room code: codes are generated in upper
// case, so lookups ignore case and surrounding whitespace.
func canonRoom(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}
