package batch

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/noderunner/bingle/internal/auth"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/store"
)

// ErrNotFound is returned for a batch that does not exist.
var ErrNotFound = store.ErrBatchNotFound

// Info describes a batch for listings.
type Info struct {
	ID         int64      `json:"id"`
	Name       string     `json:"name"`
	GameDefID  string     `json:"game_def_id"`
	Status     string     `json:"status"`
	Games      int        `json:"games"`
	Done       int        `json:"done"`
	Error      string     `json:"error,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	Spec       Spec       `json:"spec"`
	// Limit names the limit that ended the batch before its games were all
	// played ("time"); its status is then cancelled. Empty when none did.
	Limit string `json:"limit,omitempty"`
	// GamesPerMinute is the measured speed of a running or finished batch.
	GamesPerMinute float64 `json:"games_per_minute,omitempty"`
}

// GameInfo is a finished game in a listing (no metrics).
type GameInfo struct {
	Index     int      `json:"index"`
	Seed      int64    `json:"seed"`
	GameID    string   `json:"game_id"`
	Seating   []string `json:"seating"`
	Winner    int      `json:"winner"`
	EndReason string   `json:"end_reason"`
	Plies     int      `json:"plies"`
	Rounds    int      `json:"rounds"`
	HasEvents bool     `json:"has_events"`
}

// Manager runs batches one after another in the background and keeps their
// records in the store. A batch uses most of the CPUs, so two never run at once.
type Manager struct {
	st  *store.Store
	cat *catalog.Catalog

	mu      sync.Mutex
	cancels map[int64]context.CancelFunc
	queue   chan int64
	reports map[int64]Report // finished batches' reports
	// turn is held by whatever plays bot games on the batches' share of the
	// machine: a batch, or a run of the bot ladder (Hold). One at a time.
	turn chan struct{}
	// limits are the ceilings and allowances (ADR 0161); startMu makes
	// counting a player's batches and adding one a single step.
	limits  Limits
	startMu sync.Mutex
	// wall is how long a batch with these ceilings may play (a test shortens
	// it from the minute the setting allows).
	wall func(Ceilings) time.Duration
}

// maxCachedReports bounds the finished batches' reports kept in memory (ADR
// 0161): a report is rebuilt from the stored games when it is not there.
const maxCachedReports = 32

// Option changes how a Manager is made.
type Option func(*Manager)

// WithLimits sets the manager's limits (default DefaultLimits). A field left
// at zero keeps its default.
func WithLimits(l Limits) Option {
	return func(m *Manager) {
		d := DefaultLimits()
		for _, f := range []struct {
			to   *int
			from int
		}{
			{&d.Workers, l.Workers}, {&d.MCTSIterations, l.MCTSIterations}, {&d.MinimaxNodes, l.MinimaxNodes},
			{&d.MaxPlies, l.MaxPlies}, {&d.Games, l.Games}, {&d.EventGames, l.EventGames}, {&d.Minutes, l.Minutes},
			{&d.Queue, l.Queue}, {&d.QueuedPerPlayer, l.QueuedPerPlayer}, {&d.StoredPerPlayer, l.StoredPerPlayer}, {&d.GamesPerDay, l.GamesPerDay},
		} {
			if f.from > 0 {
				*f.to = f.from
			}
		}
		m.limits = d
	}
}

// NewManager starts the manager. Batches a stopped server left unfinished are
// marked interrupted.
func NewManager(st *store.Store, cat *catalog.Catalog, opts ...Option) *Manager {
	m := &Manager{st: st, cat: cat, cancels: map[int64]context.CancelFunc{}, reports: map[int64]Report{}, turn: make(chan struct{}, 1), limits: DefaultLimits(), wall: Ceilings.Wall}
	for _, o := range opts {
		o(m)
	}
	m.queue = make(chan int64, m.limits.Queue)
	_ = st.InterruptBatches(context.Background())
	go m.loop()
	return m
}

// Limits are the limits the manager keeps to.
func (m *Manager) Limits() Limits { return m.limits }

// DefaultWorkers is how many games a batch plays at once when its spec does
// not say: the default, within the workers ceiling.
func (m *Manager) DefaultWorkers() int { return min(DefaultWorkers(), m.limits.Workers) }

// Start validates spec and queues a batch. A spec over a ceiling is refused
// with a *LimitError, one from a player with no room left (or into a full
// queue) with an *AllowanceError, and one with limits of its own from anybody
// but an administrator (an account's own session, never a token's) with ErrNotAdmin.
func (m *Manager) Start(ctx context.Context, name string, spec Spec) (int64, error) {
	who, _ := auth.From(ctx)
	admin := who.IsAdmin()
	if spec.Limits != nil && !admin {
		return 0, ErrNotAdmin
	}
	ceil := m.limits.over(spec.Limits)
	if err := checkCeilings(name, spec, ceil); err != nil {
		return 0, err
	}
	asked := spec
	if err := spec.Validate(m.cat); err != nil {
		return 0, err
	}
	// What a spec leaves to the defaults is kept within the ceilings too.
	if asked.Workers <= 0 {
		spec.Workers = min(spec.Workers, ceil.Workers)
	}
	if asked.MaxPlies <= 0 {
		spec.MaxPlies = min(spec.MaxPlies, ceil.MaxPlies)
	}
	raw, _ := json.Marshal(spec)
	if name == "" {
		name = fmt.Sprintf("%s · %d games", spec.Base, spec.Games)
	}
	owner := who.ID
	if owner == "" {
		owner = store.BatchAnonymous
	}
	m.startMu.Lock()
	defer m.startMu.Unlock()
	if len(m.queue) >= cap(m.queue) {
		return 0, &AllowanceError{Limit: "queue", Max: cap(m.queue)}
	}
	if !admin {
		use, err := m.st.BatchUsageOf(ctx, owner)
		if err != nil {
			return 0, err
		}
		if err := m.limits.checkAllowances(use, spec.Games); err != nil {
			return 0, err
		}
	}
	id, err := m.st.CreateBatch(ctx, name, spec.Base, string(raw), spec.Games, owner)
	if err != nil {
		return 0, err
	}
	m.queue <- id // there is room: only Start adds, and it holds startMu
	return id, nil
}

func (m *Manager) loop() {
	for id := range m.queue {
		m.turn <- struct{}{}
		m.run(id)
		<-m.turn
	}
}

// Hold waits until no batch is playing and keeps the batches waiting until
// release is called, so that other bot games on the batches' share of the
// machine (the bot ladder, ADR 0160) never run beside a batch. It gives up
// when ctx ends.
func (m *Manager) Hold(ctx context.Context) (release func(), err error) {
	select {
	case m.turn <- struct{}{}:
		var once sync.Once
		return func() { once.Do(func() { <-m.turn }) }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (m *Manager) run(id int64) {
	bg := context.Background()
	b, err := m.st.GetBatch(bg, id)
	if err != nil || b.Status != store.BatchQueued {
		return // deleted or cancelled while it waited
	}
	var spec Spec
	if err := json.Unmarshal([]byte(b.Spec), &spec); err != nil {
		_ = m.st.SetBatchStatus(bg, id, store.BatchFailed, err.Error())
		return
	}
	ctx, cancel := context.WithCancel(bg)
	// A batch is given a wall-clock time from the moment it starts playing; at
	// its end the batch stops and is marked with the limit (ADR 0161).
	timeCtx, stopTimer := context.WithTimeoutCause(ctx, m.wall(m.limits.over(spec.Limits)), errWallClock)
	defer stopTimer()
	m.mu.Lock()
	m.cancels[id] = cancel
	m.mu.Unlock()
	defer func() {
		cancel()
		m.mu.Lock()
		delete(m.cancels, id)
		m.mu.Unlock()
	}()
	_ = m.st.SetBatchStatus(bg, id, store.BatchRunning, "")
	var finished atomic.Int64
	err = Run(timeCtx, m.cat, spec, func(r Record) {
		seating, _ := json.Marshal(r.Seating)
		metrics, _ := json.Marshal(r.Metrics)
		events := ""
		if len(r.Events) > 0 {
			raw, _ := json.Marshal(r.Events)
			events = string(raw)
		}
		_ = m.st.AddBatchGame(bg, store.BatchGame{
			BatchID: id, Index: r.Index, Seed: r.Seed, GameID: r.GameID, Seating: string(seating),
			Winner: r.Winner, EndReason: r.EndReason, Plies: r.Plies, Rounds: r.Rounds,
			Metrics: string(metrics), Events: events,
		})
	}, func(done int) { finished.Store(int64(done)) })
	switch {
	case int(finished.Load()) >= spec.Games:
		_ = m.st.SetBatchStatus(bg, id, store.BatchDone, "")
	case context.Cause(timeCtx) == errWallClock:
		_ = m.st.EndBatchAtLimit(bg, id, LimitTime)
	case ctx.Err() != nil:
		_ = m.st.SetBatchStatus(bg, id, store.BatchCancelled, "")
	case err != nil:
		_ = m.st.SetBatchStatus(bg, id, store.BatchFailed, err.Error())
	default:
		_ = m.st.SetBatchStatus(bg, id, store.BatchDone, "")
	}
}

// Cancel stops a running batch or drops a queued one.
func (m *Manager) Cancel(ctx context.Context, id int64) error {
	b, err := m.st.GetBatch(ctx, id)
	if err != nil {
		return err
	}
	m.mu.Lock()
	cancel := m.cancels[id]
	m.mu.Unlock()
	if cancel != nil {
		cancel()
		return nil
	}
	if b.Status == store.BatchQueued {
		return m.st.SetBatchStatus(ctx, id, store.BatchCancelled, "")
	}
	return nil
}

// Delete removes a batch (cancelling it first if it runs).
func (m *Manager) Delete(ctx context.Context, id int64) error {
	if err := m.Cancel(ctx, id); err != nil {
		return err
	}
	m.mu.Lock()
	delete(m.reports, id)
	m.mu.Unlock()
	return m.st.DeleteBatch(ctx, id)
}

func info(b store.Batch) Info {
	in := Info{ID: b.ID, Name: b.Name, GameDefID: b.GameDefID, Status: b.Status, Games: b.NGames, Done: b.Done,
		Error: b.Error, CreatedAt: b.CreatedAt, StartedAt: b.StartedAt, FinishedAt: b.FinishedAt}
	_ = json.Unmarshal([]byte(b.Spec), &in.Spec)
	in.Limit = b.LimitHit
	if b.StartedAt != nil && b.Done > 0 {
		end := time.Now()
		if b.FinishedAt != nil {
			end = *b.FinishedAt
		}
		if mins := end.Sub(*b.StartedAt).Minutes(); mins > 0 {
			in.GamesPerMinute = float64(b.Done) / mins
		}
	}
	return in
}

// Get describes one batch.
func (m *Manager) Get(ctx context.Context, id int64) (Info, error) {
	b, err := m.st.GetBatch(ctx, id)
	if err != nil {
		return Info{}, err
	}
	return info(b), nil
}

// List describes the batches, newest first.
func (m *Manager) List(ctx context.Context) ([]Info, error) {
	bs, err := m.st.ListBatches(ctx, 200)
	if err != nil {
		return nil, err
	}
	out := make([]Info, 0, len(bs))
	for _, b := range bs {
		out = append(out, info(b))
	}
	return out, nil
}

// Games lists the finished games of a batch.
func (m *Manager) Games(ctx context.Context, id int64) ([]GameInfo, error) {
	if _, err := m.st.GetBatch(ctx, id); err != nil {
		return nil, err
	}
	gs, err := m.st.BatchGames(ctx, id, false)
	if err != nil {
		return nil, err
	}
	out := make([]GameInfo, 0, len(gs))
	for _, g := range gs {
		gi := GameInfo{Index: g.Index, Seed: g.Seed, GameID: g.GameID, Winner: g.Winner, EndReason: g.EndReason, Plies: g.Plies, Rounds: g.Rounds}
		_ = json.Unmarshal([]byte(g.Seating), &gi.Seating)
		out = append(out, gi)
	}
	return out, nil
}

// Events returns the decisions of one game of a batch (only the first games
// of a batch keep them).
func (m *Manager) Events(ctx context.Context, id int64, index int) ([]Event, error) {
	g, err := m.st.BatchGame(ctx, id, index)
	if err != nil {
		return nil, err
	}
	var ev []Event
	if g.Events != "" {
		_ = json.Unmarshal([]byte(g.Events), &ev)
	}
	return ev, nil
}

// Report aggregates the games finished so far. A finished batch's report is
// cached.
func (m *Manager) Report(ctx context.Context, id int64) (Report, error) {
	b, err := m.st.GetBatch(ctx, id)
	if err != nil {
		return Report{}, err
	}
	final := b.Status != store.BatchQueued && b.Status != store.BatchRunning
	m.mu.Lock()
	if r, ok := m.reports[id]; ok && final {
		m.mu.Unlock()
		return r, nil
	}
	m.mu.Unlock()
	gs, err := m.st.BatchGames(ctx, id, false)
	if err != nil {
		return Report{}, err
	}
	var spec Spec
	_ = json.Unmarshal([]byte(b.Spec), &spec)
	recs := make([]Record, 0, len(gs))
	for _, g := range gs {
		r := Record{
			Index: g.Index, Seed: g.Seed, GameID: g.GameID,
			Winner: g.Winner, EndReason: g.EndReason, Plies: g.Plies, Rounds: g.Rounds,
		}
		_ = json.Unmarshal([]byte(g.Seating), &r.Seating)
		// Which bot sat where is the spec's own doing (a batch may move them
		// round the seats), so it is worked out again rather than stored.
		if len(spec.Players) > 0 {
			if _, seated, err := spec.layout(m.cat, g.Index); err == nil {
				for _, p := range seated {
					r.Bots = append(r.Bots, p.Bot)
				}
			}
		}
		if g.Metrics != "" && g.Metrics != "null" {
			r.Metrics = json.RawMessage(g.Metrics)
		}
		recs = append(recs, r)
	}
	game, _ := m.cat.Get(b.GameDefID)
	rep, err := BuildReport(game, recs)
	if err != nil {
		return Report{}, err
	}
	rep.Seats = seatReports(m.cat, b.GameDefID, recs)
	rep.Bots = botReports(recs)
	if final {
		m.mu.Lock()
		if len(m.reports) >= maxCachedReports {
			for k := range m.reports { // any one: the cache only saves a rebuild
				delete(m.reports, k)
				break
			}
		}
		m.reports[id] = rep
		m.mu.Unlock()
	}
	return rep, nil
}
