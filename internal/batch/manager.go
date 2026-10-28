package batch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/store"
)

// ErrNotFound is returned for a batch that does not exist.
var ErrNotFound = store.ErrBatchNotFound

// MaxGames bounds the games of one batch.
const MaxGames = 5000

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
}

// NewManager starts the manager. Batches a stopped server left unfinished are
// marked interrupted.
func NewManager(st *store.Store, cat *catalog.Catalog) *Manager {
	m := &Manager{st: st, cat: cat, cancels: map[int64]context.CancelFunc{}, queue: make(chan int64, 256), reports: map[int64]Report{}}
	_ = st.InterruptBatches(context.Background())
	go m.loop()
	return m
}

// Start validates spec and queues a batch.
func (m *Manager) Start(ctx context.Context, name string, spec Spec) (int64, error) {
	if spec.Games > MaxGames {
		return 0, fmt.Errorf("batch: at most %d games per batch", MaxGames)
	}
	if err := spec.Validate(m.cat); err != nil {
		return 0, err
	}
	raw, _ := json.Marshal(spec)
	if name == "" {
		name = fmt.Sprintf("%s · %d games", spec.Base, spec.Games)
	}
	id, err := m.st.CreateBatch(ctx, name, spec.Base, string(raw), spec.Games)
	if err != nil {
		return 0, err
	}
	select {
	case m.queue <- id:
	default:
		_ = m.st.SetBatchStatus(ctx, id, store.BatchFailed, "too many batches are queued")
		return id, errors.New("batch: the queue is full")
	}
	return id, nil
}

func (m *Manager) loop() {
	for id := range m.queue {
		m.run(id)
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
	err = Run(ctx, m.cat, spec, func(r Record) {
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
	}, nil)
	switch {
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
			_, seated := spec.layout(g.Index)
			for _, p := range seated {
				r.Bots = append(r.Bots, p.Bot)
			}
		}
		if g.Metrics != "" {
			_ = json.Unmarshal([]byte(g.Metrics), &r.Metrics)
		}
		recs = append(recs, r)
	}
	names := map[string]string{}
	if game, ok := m.cat.Get(b.GameDefID); ok {
		for _, p := range game.Definition.Board.Positions {
			names[p.ID] = p.Name
		}
	}
	rep := BuildReport(recs, names)
	rep.Seats = seatReports(m.cat, b.GameDefID, recs)
	rep.Bots = botReports(recs)
	if final {
		m.mu.Lock()
		m.reports[id] = rep
		m.mu.Unlock()
	}
	return rep, nil
}
