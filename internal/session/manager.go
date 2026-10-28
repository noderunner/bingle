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
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/noderunner/bingle/internal/bots"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/store"
)

// Manager orchestrates game lifecycle operations against a Store and a
// Catalog of compiled game definitions.
type Manager struct {
	store     *store.Store
	catalog   *catalog.Catalog
	hub       *hub
	bots      *bots.Registry
	log       *slog.Logger           // the server's logger; tests may replace it via SetLogger
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

// lockRoom returns an unlock function for roomCode's mutex, creating that
// mutex on first use. It never removes entries (room codes are cheap and
// bounded by how many games actually get created in a process's lifetime;
// not worth the complexity of eviction for v1).
func (m *Manager) lockRoom(roomCode string) (unlock func()) {
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
