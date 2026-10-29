// Package app composes bingle's dependencies into a servable HTTP router.
package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/noderunner/bingle/internal/api"
	"github.com/noderunner/bingle/internal/batch"
	"github.com/noderunner/bingle/internal/bots"
	"github.com/noderunner/bingle/internal/config"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/envs"
	"github.com/noderunner/bingle/internal/games/blooms"
	"github.com/noderunner/bingle/internal/games/classics"
	"github.com/noderunner/bingle/internal/games/courtfall"
	"github.com/noderunner/bingle/internal/games/cthulhuwars"
	"github.com/noderunner/bingle/internal/games/fendo"
	"github.com/noderunner/bingle/internal/games/fist"
	"github.com/noderunner/bingle/internal/games/hive"
	"github.com/noderunner/bingle/internal/games/meridians"
	"github.com/noderunner/bingle/internal/games/nyet"
	"github.com/noderunner/bingle/internal/games/onitama"
	"github.com/noderunner/bingle/internal/games/tak"
	"github.com/noderunner/bingle/internal/games/yinsh"
	"github.com/noderunner/bingle/internal/journal"
	"github.com/noderunner/bingle/internal/logging"
	"github.com/noderunner/bingle/internal/mail"
	"github.com/noderunner/bingle/internal/mcpserver"
	"github.com/noderunner/bingle/internal/metrics"
	"github.com/noderunner/bingle/internal/repro"
	"github.com/noderunner/bingle/internal/session"
	"github.com/noderunner/bingle/internal/store"
	"github.com/noderunner/bingle/internal/transport"
	"github.com/noderunner/bingle/internal/version"
	"github.com/noderunner/bingle/internal/web"
)

// App is the composition root: it wires the store and catalog into a router,
// and never binds a real network listener itself (see internal/httpserve),
// so it can be exercised entirely in-process by tests.
type App struct {
	store     *store.Store
	catalog   *catalog.Catalog
	manager   *session.Manager
	envs      *envs.Manager
	batches   *batch.Manager
	router    chi.Router
	publicURL string
	rtc       api.RTCConfig
	changelog string
	log       *slog.Logger
	metrics   *metrics.Metrics
	journal   *journal.DB
	slower    time.Duration
	history   journal.PageOptions
	// clientErrors mounts POST /api/client-error.
	clientErrors bool
	// transport says how to read a request's scheme and peer address.
	transport transport.Policy
}

// WithTransport tells the App how to read a request's peer address: behind a
// trusted proxy it comes from a header, and otherwise it does not.
func WithTransport(p transport.Policy) Option {
	return func(a *App) { a.transport = p }
}

// WithClientErrors mounts POST /api/client-error, where a browser reports what
// went wrong in it. Off unless asked for, because it is the one observability
// route on the public listener.
func WithClientErrors() Option {
	return func(a *App) { a.clientErrors = true }
}

// Option changes how New builds the App.
type Option func(*App)

// WithLogger hands the App the server's logger. Without it the App logs
// through slog's default logger, which is what the tests want.
func WithLogger(lg *slog.Logger) Option {
	return func(a *App) {
		if lg != nil {
			a.log = lg
		}
	}
}

// WithMetrics hands the App the registry to record into. Without it nothing
// is recorded and the middleware costs nothing, which is what a server with
// BINGLE_METRICS_ADDR empty does and what most tests want.
func WithMetrics(m *metrics.Metrics) Option {
	return func(a *App) { a.metrics = m }
}

// WithHistory mounts the metric history's endpoints and the Health page on
// the private listener. Without it they are simply not there, which is what a
// server with no metrics database serves and what most tests want.
func WithHistory(opts journal.PageOptions) Option {
	return func(a *App) { a.history = opts }
}

// WithJournal turns the slow-request journal on: requests slower than slower,
// and requests the server fails, are written to db (which may be nil to log
// them and store nothing). A zero duration leaves the journal off, collector
// and all, which is what most tests want.
func WithJournal(db *journal.DB, slower time.Duration) Option {
	return func(a *App) {
		a.journal = db
		a.slower = slower
	}
}

// New builds an App from cfg: it opens the store, builds a *catalog.Catalog
// and registers every built-in game module into it, and mounts all routes.
// If any module fails to register, New returns that error instead of a
// usable *App — the server refuses to start rather than serve a broken game.
func New(cfg config.Config, opts ...Option) (*App, error) {
	// The options come first, so the store is recording and logging from its
	// very first statement: the schema migration and the game versions
	// written below are part of what starting up costs.
	a := &App{log: slog.Default()}
	for _, opt := range opts {
		opt(a)
	}

	st, err := store.Open(cfg.DBPath)
	if err != nil {
		return nil, err
	}
	// Every statement in internal/store is recorded, and the registry asks
	// the store about the file, its pages and its tables when it is gathered.
	st.SetMetrics(a.metrics)
	a.metrics.SetDBSource(func() metrics.DBSnapshot { return st.Snapshot(context.Background()) })

	cat := catalog.NewCatalog()
	for _, mod := range BuiltinModules() {
		if err := cat.RegisterModule(mod); err != nil {
			_ = st.Close()
			return nil, fmt.Errorf("app: registering %q: %w", mod.ID, err)
		}
	}

	if err := recordGameVersions(st, cat); err != nil {
		_ = st.Close()
		return nil, err
	}

	manager := session.NewManager(st, cat)
	manager.SetLogger(a.log)
	manager.SetMetrics(a.metrics)
	if w := repro.NewWriter(cfg.Debug.Dir, version.Engine, a.log); w != nil {
		// A bundle carries the full position, so it goes to a directory of
		// its own and never to the log stream (ADR 0063). The move funnel
		// writes them, and so does a batch, which plays hundreds of games
		// with nobody watching.
		manager.SetRepro(w)
		batch.SetRepro(w)
		a.log.Info("repro bundles are on", "dir", w.Dir())
	}
	manager.SetDebug(session.DebugOptions{
		Games:    cfg.Debug.Games,
		Room:     cfg.Debug.Room,
		Secrets:  cfg.Debug.Secrets,
		Paranoid: cfg.Debug.Paranoid,
	})
	a.metrics.SetBotNodeSource(bots.NodeCounts)
	batch.SetMetrics(a.metrics)

	a.store = st
	a.catalog = cat
	a.manager = manager
	a.envs = envs.NewManager(cat)
	a.batches = batch.NewManager(st, cat)
	a.publicURL = cfg.PublicURL
	a.changelog = cfg.Changelog
	a.rtc = api.RTCConfig{Stun: cfg.RTCStun, TurnURLs: cfg.RTCTurnURLs, TurnSecret: cfg.RTCTurnSecret}

	if err := enableNotify(manager, cfg, a.log); err != nil {
		_ = st.Close()
		return nil, err
	}

	a.router = a.newRouter()
	return a, nil
}

// recordGameVersions notes in the database the version of every game this server
// runs, so the history of a game's versions is kept with the games played.
func recordGameVersions(st *store.Store, cat *catalog.Catalog) error {
	seen := map[string]bool{}
	for _, cg := range cat.List() {
		def := cg.Definition
		family := def.Family
		if family == "" {
			family = def.ID
		}
		if def.Version == "" || seen[family+" "+def.Version] {
			continue
		}
		seen[family+" "+def.Version] = true
		if err := st.RecordGameVersion(context.Background(), family, def.Version); err != nil {
			return fmt.Errorf("app: %w", err)
		}
	}
	return nil
}

// enableNotify turns on email notifications when the mail is set up. Without a
// public URL there is nowhere for the links in the mail to point, so the
// feature stays off (with a log line saying why).
func enableNotify(m *session.Manager, cfg config.Config, lg *slog.Logger) error {
	if !cfg.Mail.Enabled() {
		return nil
	}
	if cfg.PublicURL == "" {
		lg.Warn("notify: email notifications are NOT enabled: set BINGLE_PUBLIC_URL (or BINGLE_TLS_DOMAIN) so the links in the mail have an address")
		return nil
	}
	sender, err := mail.NewSender(cfg.Mail)
	if err != nil {
		return fmt.Errorf("app: email notifications: %w", err)
	}
	m.EnableNotify(session.NotifyOptions{
		Mailer: sender, From: cfg.Mail.From, BaseURL: cfg.PublicURL,
		HourlyLimit: cfg.Mail.HourlyLimit, DailyLimit: cfg.Mail.DailyLimit,
		AwayUnit: cfg.Mail.AwayUnit,
	})
	return nil
}

// BuiltinModules returns every game module bingle ships with.
func BuiltinModules() []catalog.Module {
	return append([]catalog.Module{tak.Module(), courtfall.Module(), fendo.Module(), fist.Module(), meridians.Module(), onitama.Module(), blooms.Module(), hive.Module(), nyet.Module(), yinsh.Module(), cthulhuwars.Module()}, classics.Modules()...)
}

// Router returns the application's HTTP handler.
func (a *App) Router() http.Handler {
	return a.router
}

// Catalog returns the application's game catalog.
func (a *App) Catalog() *catalog.Catalog {
	return a.catalog
}

// PurgeInactive deletes games with no activity for at least days days and
// returns how many were removed.
func (a *App) PurgeInactive(ctx context.Context, days int) (int64, error) {
	if days <= 0 {
		return 0, nil
	}
	return a.store.DeleteInactiveGames(ctx, time.Now().Add(-time.Duration(days)*24*time.Hour))
}

// RunRetention purges inactive games now and then hourly until ctx ends.
func (a *App) RunRetention(ctx context.Context, days int) {
	if days <= 0 {
		return
	}
	purge := func() {
		if n, err := a.PurgeInactive(ctx, days); err != nil {
			a.log.Error("retention", "err", err)
		} else if n > 0 {
			a.log.Info("retention: removed games nobody had touched", "games", n, "days", days)
		}
	}
	purge()
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			purge()
		}
	}
}

// Manager returns the application's session manager.
func (a *App) Manager() *session.Manager {
	return a.manager
}

// Close releases resources held by the App.
func (a *App) Close() error {
	return a.store.Close()
}

func (a *App) newRouter() chi.Router {
	r := chi.NewRouter()
	// In this order: the request id is in the context before anything logs,
	// and the recover sits INSIDE the access log rather than outside it (the
	// order docs/history/observability-plan.md lists), because a panic
	// unwinds past every middleware outside the one that recovers — so with
	// the recover outermost a panicking request would be the one request
	// never logged as a request. Neither middleware ever sees the path.
	r.Use(logging.RequestID)
	r.Use(logging.Access(a.log))
	// Inside the access log, which wraps the writer all of them read the
	// status off, and outside the recover, so a request that panicked is
	// counted as the 500 it became and written down as the one nobody can
	// otherwise explain. The journal is outermost of the three so its
	// "handler" span covers what the other two measure.
	r.Use(journal.Middleware(journal.Options{DB: a.journal, Logger: a.log, Slower: a.slower}))
	r.Use(a.metrics.Middleware)
	r.Use(logging.Recover(a.log))
	r.Get("/healthz", a.handleHealthz)
	apiOpts := []api.Option{api.WithEnvs(a.envs), api.WithBatches(a.batches), api.WithPublicURL(a.publicURL), api.WithRTC(a.rtc), api.WithChangelog(a.changelog), api.WithTransport(a.transport)}
	if a.clientErrors {
		// The one observability route on the public listener: a browser has
		// to be able to reach it. It is write-only, validated against closed
		// sets and rate limited per address — see internal/api/clienterror.go.
		apiOpts = append(apiOpts, api.WithClientErrors(a.log, a.metrics, a.journal))
	}
	r.Mount("/api", api.NewRouter(a.manager, apiOpts...))
	r.Get("/ws/{code}", api.NewWSHandler(a.manager))
	r.Get("/ws-chat", api.NewGlobalChatWSHandler(a.manager))
	r.Get("/ws-dm", api.NewDirectWSHandler(a.manager))
	r.Mount("/mcp", mcpserver.NewServer(a.manager, a.envs, a.batches))
	// Mounted last: internal/web acts as "/"'s catch-all/root, so every more
	// specific path above must be registered first to take precedence.
	r.Mount("/", web.NewRouter(a.manager))
	return r
}

// MetricsHandler is what the private listener serves: the exposition, and
// nothing else yet. /healthz is on it as well so a scrape target and a health
// check can be the same port, while the public /healthz stays where the
// container's health check already looks for it.
//
// Note what is NOT here: this handler is never mounted on the public router.
// The public site has no authentication of any kind, so what bingle is doing
// and how busy it is must not be readable from it (ADR 0064), and a test
// asserts /metrics is a 404 there.
func (a *App) MetricsHandler() http.Handler {
	r := chi.NewRouter()
	r.Use(logging.RequestID)
	r.Use(logging.Recover(a.log))
	// No access log and no metrics middleware: a scraper calls this every few
	// seconds for ever, and neither a line per scrape nor a series counting
	// the scrapes of the series is worth having. promhttp counts its own
	// requests, which is the part that matters.
	r.Handle("/metrics", a.metrics.Handler())
	r.Get("/healthz", a.handleHealthz)
	if a.history.DB != nil {
		journal.Routes(r, a.history)
		// The same stylesheet and the same charts the rest of the site uses,
		// so the Health page needs no assets of its own.
		r.Handle("/static/*", web.StaticHandler())
	}
	return r
}

type healthzResponse struct {
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
	// Version is the bingle engine's version (internal/version).
	Version string `json:"version,omitempty"`
}

func (a *App) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if err := a.store.Ping(r.Context()); err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(healthzResponse{Status: "error", Error: err.Error()})
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(healthzResponse{Status: "ok", Version: version.Engine})
}
