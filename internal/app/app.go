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
	"github.com/noderunner/bingle/internal/logging"
	"github.com/noderunner/bingle/internal/mail"
	"github.com/noderunner/bingle/internal/mcpserver"
	"github.com/noderunner/bingle/internal/session"
	"github.com/noderunner/bingle/internal/store"
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

// New builds an App from cfg: it opens the store, builds a *catalog.Catalog
// and registers every built-in game module into it, and mounts all routes.
// If any module fails to register, New returns that error instead of a
// usable *App — the server refuses to start rather than serve a broken game.
func New(cfg config.Config, opts ...Option) (*App, error) {
	st, err := store.Open(cfg.DBPath)
	if err != nil {
		return nil, err
	}

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

	a := &App{store: st, catalog: cat, manager: manager, envs: envs.NewManager(cat), batches: batch.NewManager(st, cat), publicURL: cfg.PublicURL, changelog: cfg.Changelog,
		rtc: api.RTCConfig{Stun: cfg.RTCStun, TurnURLs: cfg.RTCTurnURLs, TurnSecret: cfg.RTCTurnSecret}, log: slog.Default()}
	for _, opt := range opts {
		opt(a)
	}
	manager.SetLogger(a.log)

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
	r.Use(logging.Recover(a.log))
	r.Get("/healthz", a.handleHealthz)
	r.Mount("/api", api.NewRouter(a.manager, api.WithEnvs(a.envs), api.WithBatches(a.batches), api.WithPublicURL(a.publicURL), api.WithRTC(a.rtc), api.WithChangelog(a.changelog)))
	r.Get("/ws/{code}", api.NewWSHandler(a.manager))
	r.Get("/ws-chat", api.NewGlobalChatWSHandler(a.manager))
	r.Get("/ws-dm", api.NewDirectWSHandler(a.manager))
	r.Mount("/mcp", mcpserver.NewServer(a.manager, a.envs, a.batches))
	// Mounted last: internal/web acts as "/"'s catch-all/root, so every more
	// specific path above must be registered first to take precedence.
	r.Mount("/", web.NewRouter(a.manager))
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
