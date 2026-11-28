// Package app composes bingle's dependencies into a servable HTTP router.
package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/noderunner/bingle/internal/api"
	"github.com/noderunner/bingle/internal/auth"
	"github.com/noderunner/bingle/internal/batch"
	"github.com/noderunner/bingle/internal/botladder"
	"github.com/noderunner/bingle/internal/bots"
	"github.com/noderunner/bingle/internal/clientbuilds"
	"github.com/noderunner/bingle/internal/config"
	"github.com/noderunner/bingle/internal/engine/catalog"
	"github.com/noderunner/bingle/internal/engine/wasmrules"
	"github.com/noderunner/bingle/internal/envs"
	_ "github.com/noderunner/bingle/internal/games/all" // every game this repository ships; build tags leave one out
	"github.com/noderunner/bingle/internal/games/registry"
	"github.com/noderunner/bingle/internal/journal"
	"github.com/noderunner/bingle/internal/logging"
	"github.com/noderunner/bingle/internal/mail"
	"github.com/noderunner/bingle/internal/mcpserver"
	"github.com/noderunner/bingle/internal/metrics"
	"github.com/noderunner/bingle/internal/pack"
	"github.com/noderunner/bingle/internal/repro"
	"github.com/noderunner/bingle/internal/session"
	"github.com/noderunner/bingle/internal/store"
	"github.com/noderunner/bingle/internal/translate"
	"github.com/noderunner/bingle/internal/transport"
	"github.com/noderunner/bingle/internal/version"
	"github.com/noderunner/bingle/internal/web"
)

// App is the composition root: it wires the store and catalog into a router,
// and never binds a real network listener itself (see internal/httpserve),
// so it can be exercised entirely in-process by tests.
type App struct {
	store    *store.Store
	guests   *auth.Resolver
	accounts *auth.Accounts
	caps     *auth.Caps
	// seeded is the administrator made for an empty database (nil if there
	// were accounts), whose password main prints once.
	seeded *auth.Seeded
	// argon2 is the cost of a password hash; zero means auth's default.
	argon2     auth.Argon2Params
	catalog    *catalog.Catalog
	manager    *session.Manager
	envs       *envs.Manager
	batches    *batch.Manager
	router     chi.Router
	publicURL  string
	rtc        api.RTCConfig
	changelog  string
	changelogs map[string]string
	apiDocs    string // docs/api.md, for GET /api/docs
	apiSchema  []byte // docs/api.schema.json, for GET /api/schema
	brand      config.Brand
	log        *slog.Logger
	metrics    *metrics.Metrics
	journal    *journal.DB
	slower     time.Duration
	history    journal.PageOptions
	// clientErrors mounts POST /api/client-error.
	clientErrors bool
	// transport says how to read a request's scheme and peer address.
	transport transport.Policy
	// translator is the chat's translation service, nil when none is set up
	// (ADR 0143).
	translator *translate.Service
	// wasm runs the rules of the packs loaded from BINGLE_PACKS (ADR 0147),
	// and is nil without them.
	wasm  *wasmrules.Runtime
	packs []*pack.Pack
	// clientBuilds are the builds of the bingle-3d client the server offers
	// (BINGLE_CLIENT_BUILDS, ADR 0151), nil without a directory of them;
	// certificate answers with the fingerprint of the certificate the
	// server made for itself, which a client's setup file carries.
	clientBuilds *clientbuilds.Set
	certificate  func() string
}

// WithCertificate tells the App how to learn the SHA-256 fingerprint of the
// certificate the server made for itself (BINGLE_TLS_SELFSIGNED), so a
// client's setup file can carry it (ADR 0151). Without it no file does.
func WithCertificate(fingerprint func() string) Option {
	return func(a *App) { a.certificate = fingerprint }
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
	if err := a.loadPacks(cfg.Packs, cat); err != nil {
		_ = st.Close()
		return nil, err
	}
	if cfg.ClientBuilds != "" {
		if a.clientBuilds, err = clientbuilds.Load(cfg.ClientBuilds, version.API, a.log); err != nil {
			a.closeWasm()
			_ = st.Close()
			return nil, fmt.Errorf("app: BINGLE_CLIENT_BUILDS: %w", err)
		}
		for _, b := range a.clientBuilds.Builds() {
			a.log.Info("client build on offer", "platform", b.Platform, "version", b.Version, "file", b.File)
		}
	}
	// The packs' rules go with the store on any error from here on.
	started := false
	defer func() {
		if !started {
			a.closeWasm()
		}
	}()

	// What was stored under a game's former id moves to its id now, before
	// anything reads it (catalog.Module.Formerly).
	if err := renameGames(st, cat, a.log); err != nil {
		_ = st.Close()
		return nil, err
	}

	if err := recordGameVersions(st, cat); err != nil {
		_ = st.Close()
		return nil, err
	}

	// A game whose rules changed form converts its stored positions once.
	if ups := cat.Upgrades(); len(ups) > 0 {
		ids := make([]string, 0, len(ups))
		for id := range ups {
			ids = append(ids, id)
		}
		slices.Sort(ids)
		n, err := st.UpgradeStates(context.Background(), ids, func(def string, doc []byte) ([]byte, bool) {
			base, _, _ := strings.Cut(def, catalog.DerivedSep)
			if up, ok := ups[base]; ok {
				return up(doc)
			}
			return doc, false
		})
		if err != nil {
			_ = st.Close()
			return nil, err
		}
		if n > 0 {
			a.log.Info("converted stored positions to their games' current rules", "positions", n)
		}
	}

	// Retired games stay readable; any still being played end now, with no
	// result (docs/history/game-plugins-plan.md, ADR 0076).
	for _, r := range RetiredGames {
		cat.Retire(r.ID, r.Name)
	}
	if n, err := st.EndRetiredGames(context.Background(), cat.RetiredIDs()); err != nil {
		_ = st.Close()
		return nil, err
	} else if n > 0 {
		a.log.Info("ended the unfinished games of retired games", "games", n, "retired", cat.RetiredIDs())
	}
	// A pack that is gone leaves its games as records, after the retired
	// games have been ended: its unfinished games wait for it rather than
	// end, since a pack may be back at the next start (ADR 0147).
	if err := a.retireGonePacks(st, cat); err != nil {
		_ = st.Close()
		return nil, err
	}

	manager := session.NewManager(st, cat)
	manager.SetBrand(cfg.Brand.DisplayName())
	manager.SetLogger(a.log)
	manager.SetMetrics(a.metrics)
	manager.SetConnectionTimes(session.ConnectionTimes(cfg.Connection))
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
	// Moves stored before impulses were counted get their plies (ADR 0114).
	if err := manager.RegroupMoves(context.Background()); err != nil {
		_ = st.Close()
		return nil, err
	}
	a.metrics.SetBotNodeSource(bots.NodeCounts)
	batch.SetMetrics(a.metrics)

	a.store = st
	a.catalog = cat
	a.manager = manager
	a.envs = envs.NewManager(cat)
	a.batches = batch.NewManager(st, cat, batch.WithLimits(batch.Limits{
		Ceilings: batch.Ceilings{
			Workers: cfg.Batch.Workers, MCTSIterations: cfg.Batch.MCTSIterations, MinimaxNodes: cfg.Batch.MinimaxNodes,
			MaxPlies: cfg.Batch.MaxPlies, Games: cfg.Batch.Games, EventGames: cfg.Batch.EventGames, Minutes: int(cfg.Batch.Time / time.Minute),
		},
		Queue: cfg.Batch.Queue, QueuedPerPlayer: cfg.Batch.QueuedPerPlayer, StoredPerPlayer: cfg.Batch.StoredPerPlayer, GamesPerDay: cfg.Batch.GamesPerDay,
	}))
	a.publicURL = cfg.PublicURL
	a.brand = cfg.Brand
	a.changelog = cfg.Changelog
	a.apiDocs = cfg.APIDocs
	a.apiSchema = cfg.APISchema
	a.changelogs = map[string]string{}
	for code, text := range cfg.Changelogs {
		a.changelogs[code] = text
	}
	if name := cfg.Brand.DisplayName(); name != config.DefaultBrandName {
		a.changelog = strings.ReplaceAll(a.changelog, config.DefaultBrandName, name)
		for code, text := range a.changelogs {
			a.changelogs[code] = strings.ReplaceAll(text, config.DefaultBrandName, name)
		}
	}
	a.rtc = api.RTCConfig{Stun: cfg.RTCStun, TurnURLs: cfg.RTCTurnURLs, TurnSecret: cfg.RTCTurnSecret}
	caps, err := auth.NewCaps(context.Background(), st, cfg.AuthMode, cfg.CapsFile)
	if err != nil {
		_ = st.Close()
		return nil, err
	}
	a.caps = caps
	manager.SetMayEnterTournament(caps.MayEnterTournament)
	manager.SetMayModerate(caps.MayModerate)
	a.accounts = auth.NewAccounts(st, auth.AccountOptions{Brand: cfg.Brand.DisplayName(), Mailer: manager, Log: a.log, Signup: cfg.Signup, SessionIdle: cfg.SessionIdle, SessionMax: cfg.SessionMax, Argon2: a.argon2,
		LoginsPerMinute: caps.Limit(auth.ClassGuest, auth.LimitLogin)})
	if a.translator, err = enableTranslate(cfg.Translate, a.metrics, a.log); err != nil {
		_ = st.Close()
		return nil, err
	}
	if a.translator != nil {
		manager.EnableTranslate(a.translator, caps.MayTranslate, caps.AllowTranslation)
	}
	a.guests = auth.NewResolver(st, auth.Options{Transport: a.transport, Idle: cfg.GuestIdle, Log: a.log, Accounts: a.accounts,
		OIDC: auth.NewOIDC(cfg.PublicURL, cfg.OIDC), Caps: caps, Translation: a.translator != nil, Certificate: a.certificate})

	if err := enableNotify(manager, cfg, a.log); err != nil {
		_ = st.Close()
		return nil, err
	}
	// An empty server gets its first administrator (ADR 0068).
	seeded, err := a.accounts.SeedAdmin(context.Background(), auth.SeedOptions{
		Handle: cfg.AdminHandle, Email: cfg.SeedAdminEmail(), Password: cfg.AdminPassword.Reveal(), Deterministic: cfg.SeedPasswordDeterministic()})
	if err != nil {
		_ = st.Close()
		return nil, err
	}
	a.seeded = seeded
	// BINGLE_DEV (loopback only) gives the first administrator a known API
	// token, so CI can drive a closed server.
	if cfg.Dev {
		handle := cfg.AdminHandle
		if handle == "" {
			handle = "admin"
		}
		if err := a.accounts.SeedDevToken(context.Background(), handle); err != nil && !errors.Is(err, store.ErrNotFound) {
			_ = st.Close()
			return nil, err
		}
	}

	a.router = a.newRouter()
	started = true
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
		lg.Info("notify: mail is off: set BINGLE_SMTP_HOST to send confirmations, password resets and turn notifications")
		return nil
	}
	if cfg.PublicURL == "" {
		lg.Warn("notify: email notifications are NOT enabled: set BINGLE_PUBLIC_URL (or BINGLE_TLS_DOMAIN) so the links in the mail have an address")
		return nil
	}
	sender, err := mail.NewSender(cfg.Mail, cfg.Brand.DisplayName())
	if err != nil {
		return fmt.Errorf("app: email notifications: %w", err)
	}
	m.EnableNotify(session.NotifyOptions{
		Mailer: sender, From: cfg.Mail.From, BaseURL: cfg.PublicURL,
		HourlyLimit: cfg.Mail.HourlyLimit, DailyLimit: cfg.Mail.DailyLimit,
		AwayUnit: cfg.Mail.AwayUnit,
	})
	// The sending address is an address like any other, so it is logged hashed.
	lg.Info("notify: mail is on", "host", cfg.Mail.Host, "port", cfg.Mail.Port, "tls", cfg.Mail.Mode,
		logging.Addr(cfg.Mail.From), "hourly_limit", cfg.Mail.HourlyLimit, "daily_limit", cfg.Mail.DailyLimit)
	return nil
}

// enableTranslate starts the translation service when one is set up, and
// warns when its address is not on this machine or a private network: chat
// lines go there (ADR 0143).
func enableTranslate(cfg config.Translate, mx *metrics.Metrics, lg *slog.Logger) (*translate.Service, error) {
	if !cfg.Enabled() {
		lg.Info("translate: chat translation is off: set BINGLE_TRANSLATE_URL to a translation service on this machine or network")
		return nil, nil
	}
	svc, err := translate.New(cfg, mx, lg)
	if err != nil {
		return nil, fmt.Errorf("app: chat translation: %w", err)
	}
	mx.EnableTranslation()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	switch local, err := translate.Local(ctx, cfg.URL, nil); {
	case err != nil:
		lg.Warn("translate: could not tell whether the translation service is on this machine or network; chat lines are sent to it", "error", err)
	case !local:
		lg.Warn("translate: the translation service is NOT on this machine or a private network; chat lines are sent to it")
	}
	lg.Info("translate: chat translation is on", "api", cfg.API, "model", cfg.Model, "workers", cfg.Workers, "timeout", cfg.Timeout)
	return svc, nil
}

// loadPacks registers the install packs in dir (BINGLE_PACKS, ADR 0147). A
// pack that fails a check, or cannot be registered, is logged and left out,
// and the server starts without it. Only a runtime that cannot be made at all
// stops it.
func (a *App) loadPacks(dir string, cat *catalog.Catalog) error {
	if dir == "" {
		return nil
	}
	ctx := context.Background()
	rt, err := wasmrules.New(ctx, wasmrules.Options{Log: a.log})
	if err != nil {
		return fmt.Errorf("app: packs: %w", err)
	}
	a.wasm = rt
	packs, err := pack.Load(ctx, rt, dir, a.log)
	if err != nil {
		a.log.Error("pack: no packs loaded", "error", err)
	}
	for _, p := range packs {
		if err := cat.RegisterModule(p.Module); err != nil {
			a.log.Error("pack: left out", "dir", p.Dir, "error", err)
			_ = p.Close(ctx)
			continue
		}
		a.packs = append(a.packs, p)
	}
	return nil
}

// retireGonePacks notes the name of every game the packs brought, and
// retires those whose pack is no longer loaded, so their games stay
// readable (ADR 0076). Their unfinished games are not ended.
func (a *App) retireGonePacks(st *store.Store, cat *catalog.Catalog) error {
	ctx := context.Background()
	for _, p := range a.packs {
		for _, id := range p.Games {
			cg, _ := cat.Get(id)
			if err := st.RecordPackGame(ctx, id, cg.Definition.Title()); err != nil {
				return fmt.Errorf("app: %w", err)
			}
		}
	}
	known, err := st.PackGames(ctx)
	if err != nil {
		return fmt.Errorf("app: %w", err)
	}
	var gone []string
	for id, name := range known {
		if _, loaded := cat.Get(id); !loaded {
			cat.Retire(id, name)
			gone = append(gone, id)
		}
	}
	if len(gone) > 0 {
		slices.Sort(gone)
		a.log.Info("pack: the games of packs that are gone are kept as records", "games", gone)
	}
	return nil
}

// closeWasm closes the packs' rules.
func (a *App) closeWasm() {
	if a.wasm != nil {
		_ = a.wasm.Close(context.Background())
		a.wasm = nil
	}
}

// renameGames moves what the store holds under each former id of a game to
// its id now.
func renameGames(st *store.Store, cat *catalog.Catalog, log *slog.Logger) error {
	var renames []store.GameRename
	for from, to := range cat.Renamed() {
		cg, ok := cat.Get(to)
		if !ok {
			continue
		}
		renames = append(renames, store.GameRename{From: from, To: to, Name: store.FamilyNameOf(cg.Definition.Name)})
	}
	slices.SortFunc(renames, func(a, b store.GameRename) int { return strings.Compare(a.From, b.From) })
	n, err := st.RenameGames(context.Background(), renames)
	if err != nil {
		return fmt.Errorf("app: renaming games: %w", err)
	}
	if n > 0 {
		log.Info("moved what was stored under games' former ids", "rows", n)
	}
	return nil
}

// RetiredGames are the games bingle used to ship and no longer does. Games of
// them already played stay readable, by these names, as a record.
var RetiredGames = []struct{ ID, Name string }{
	{"tic-tac-toe", "Tic-Tac-Toe"},
	{"connect-four", "Connect Four"},
}

// BuiltinModules returns every game module this binary was built with: the
// families registered by package all, less any a build tag left out.
func BuiltinModules() []catalog.Module {
	return registry.Modules()
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

// RunBotLadder plays the bot ladder's runs on their schedule until ctx ends
// (ADR 0160), each in the batches' turn, and does nothing when they are off.
func (a *App) RunBotLadder(ctx context.Context, cfg config.BotLadder) {
	// A run is background work like a batch and keeps within the batches'
	// ceilings on workers and on a game's length (ADR 0161).
	lim := a.batches.Limits()
	opts := botladder.Options{Off: cfg.Off, At: cfg.At, Games: cfg.Games, Limit: cfg.Limit, Workers: a.batches.DefaultWorkers(), MaxPlies: lim.MaxPlies}
	botladder.New(a.store, a.catalog, a.batches.Hold, opts, a.log).Run(ctx)
}

// Seeded is the administrator this App made for an empty database, or nil.
func (a *App) Seeded() *auth.Seeded { return a.seeded }

// AccountRow is an account as the command line lists it.
type AccountRow struct {
	Handle, Status, CreatedAt string
	Groups                    []string
}

// ListAccounts lists the accounts with their groups.
func (a *App) ListAccounts(ctx context.Context) ([]AccountRow, error) {
	list, err := a.store.ListAccounts(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]AccountRow, 0, len(list))
	for _, acc := range list {
		groups, err := a.store.Groups(ctx, acc.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, AccountRow{Handle: acc.Handle, Status: acc.Status, CreatedAt: acc.CreatedAt, Groups: groups})
	}
	return out, nil
}

// Caps is the capability matrix (the command line's `caps reset`).
func (a *App) Caps() *auth.Caps { return a.caps }

// Accounts is the account service (the command line's way in).
func (a *App) Accounts() *auth.Accounts { return a.accounts }

// Resolver is what tells each request's guest or account (tests set its clock).
func (a *App) Resolver() *auth.Resolver { return a.guests }

// WithArgon2 sets the cost of a password hash: tests make it cheap, since
// every App they start hashes the seeded administrator's password.
func WithArgon2(p auth.Argon2Params) Option { return func(a *App) { a.argon2 = p } }

// Manager returns the application's session manager.
func (a *App) Manager() *session.Manager {
	return a.manager
}

// Close releases resources held by the App.
func (a *App) Close() error {
	if a.translator != nil {
		a.translator.Close()
	}
	a.closeWasm()
	_ = a.clientBuilds.Close()
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
	apiOpts := []api.Option{api.WithClientBuilds(a.clientBuilds), api.WithEnvs(a.envs), api.WithBatches(a.batches), api.WithPublicURL(a.publicURL), api.WithRTC(a.rtc), api.WithChangelog(a.changelog, a.changelogs), api.WithDocs(a.apiDocs), api.WithSchema(a.apiSchema), api.WithTransport(a.transport)}
	if a.clientErrors {
		// The one observability route on the public listener: a browser has
		// to be able to reach it. It is write-only, validated against closed
		// sets and rate limited per address — see internal/api/clienterror.go.
		apiOpts = append(apiOpts, api.WithClientErrors(a.log, a.metrics, a.journal))
	}
	// Every route that acts for somebody resolves who that is first (ADR
	// 0067). The pages and static files do not need to know.
	r.Group(func(r chi.Router) {
		r.Use(a.guests.Middleware)
		r.Mount("/api", api.NewRouter(a.manager, append(apiOpts, api.WithGuests(a.guests))...))
		r.Get("/ws/{code}", api.NewWSHandler(a.manager))
		r.Get("/ws-chat", api.NewGlobalChatWSHandler(a.manager))
		r.Get("/ws-dm", api.NewDirectWSHandler(a.manager))
		r.Mount("/mcp", mcpserver.NewServer(a.manager, a.envs, a.batches, mcpserver.WithCaps(a.caps)))
		r.Mount("/auth", a.guests.Routes())
	})
	// Mounted last: internal/web acts as "/"'s catch-all/root, so every more
	// specific path above must be registered first to take precedence.
	// A closed server shows its pages to its members, and the sign-in to
	// everybody else.
	r.With(a.guests.ClosedPages).Mount("/", web.NewRouter(a.manager, a.brand, a.guests.AccountLanguage, web.WithClientBuilds(a.clientBuilds)))
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
