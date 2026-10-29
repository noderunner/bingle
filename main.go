// Command bingle runs the bingle server.
package main

import (
	"context"
	_ "embed" // CHANGELOG.md
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/noderunner/bingle/internal/app"
	"github.com/noderunner/bingle/internal/config"
	"github.com/noderunner/bingle/internal/httpserve"
	"github.com/noderunner/bingle/internal/journal"
	"github.com/noderunner/bingle/internal/logging"
	"github.com/noderunner/bingle/internal/metrics"
	"github.com/noderunner/bingle/internal/version"
)

// changelog is CHANGELOG.md, built into the binary for the help window.
//
//go:embed CHANGELOG.md
var changelog string

func main() {
	// A logger at the defaults, so a complaint about the configuration has
	// somewhere to go before the configuration has been read.
	lg := logging.New(os.Stderr, slog.LevelInfo, logging.FormatText)
	slog.SetDefault(lg)

	cfg, err := config.Load()
	if err != nil {
		lg.Error("the configuration was refused", "err", err)
		os.Exit(1)
	}
	cfg.Changelog = changelog

	// The logger the server runs with. slog.SetDefault also points the
	// standard log package at this handler, so a line from chi, the WebSocket
	// library or the SQLite driver lands in the same stream at the same level
	// rather than bypassing both.
	lg = logging.New(os.Stderr, cfg.LogLevel, cfg.LogFormat)
	slog.SetDefault(lg)

	// With no metrics address nothing is registered and nothing is recorded:
	// the middleware returns the handler untouched.
	var mx *metrics.Metrics
	if cfg.MetricsAddr != "" {
		mx = metrics.New(version.Engine)
	}

	// The metrics database holds the slow-request journal (and, from Stage 5,
	// the metric history). It is a separate file from the game database on
	// purpose: measuring the server must not write to what is being measured.
	var jdb *journal.DB
	if cfg.MetricsDB != "" && (cfg.SlowRequest > 0 || cfg.MetricsSample > 0) {
		jdb, err = journal.Open(cfg.MetricsDB)
		if err != nil {
			lg.Error("the metrics database could not be opened", "path", cfg.MetricsDB, "err", err)
			os.Exit(1)
		}
		defer func() { _ = jdb.Close() }()
		if cfg.SlowRequest > 0 {
			lg.Info("the slow-request journal is on", "db", cfg.MetricsDB, "slower_than", cfg.SlowRequest)
		}
	}

	if cfg.Debug.Paranoid {
		lg.Warn("BINGLE_PARANOID is on: the engine is checked against itself after every move, which runs Legal twice. This is for development and CI, never for a server people are playing on.")
	}
	if cfg.Debug.Secrets {
		lg.Warn("BINGLE_DEBUG_SECRETS is on: debug lines carry full positions, including committed dice, hidden hands and the random seed. This is for development only.")
	}

	// The sampler writes the history and compacts it; the Health page and the
	// history endpoints read it back. Both are on the private listener only.
	sampler := &journal.Sampler{
		DB: jdb, Every: cfg.MetricsSample, RetainDays: cfg.MetricsRetainDays, Logger: lg,
	}
	if mx != nil {
		sampler.Source = mx.Samples
	}
	history := journal.PageOptions{
		DB: jdb, Version: version.Engine, Every: cfg.MetricsSample,
		RetainDays: cfg.MetricsRetainDays, Logger: lg, Sample: sampler.Tick,
	}

	a, err := app.New(cfg, app.WithLogger(lg), app.WithMetrics(mx),
		app.WithJournal(jdb, cfg.SlowRequest), app.WithHistory(history))
	if err != nil {
		lg.Error("the server could not be built", "err", err)
		os.Exit(1)
	}
	defer func() { _ = a.Close() }()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if cfg.MetricsAddr != "" {
		priv, err := httpserve.ListenPrivate(cfg.MetricsAddr, cfg.MetricsToken.Reveal(), a.MetricsHandler(), lg)
		if err != nil {
			lg.Error("the metrics listener could not start", "err", err)
			os.Exit(1)
		}
		local, _ := config.LoopbackAddr(priv.Addr())
		lg.Info("metrics listening", "addr", priv.Addr(), "reachable_only_from_this_machine", local,
			"token_required", cfg.MetricsToken.Set())
		go func() {
			if err := priv.Serve(ctx); err != nil {
				lg.Error("the metrics listener stopped", "err", err)
			}
		}()
	}

	go sampler.Run(ctx)
	go a.RunRetention(ctx, cfg.RetentionDays)
	go a.Manager().RunAutoPlay(ctx, 0)
	go a.Manager().RunNotifier(ctx)

	srvCfg := httpserve.Config{
		Addr:        cfg.Addr,
		CertFile:    cfg.TLSCert,
		KeyFile:     cfg.TLSKey,
		Domains:     cfg.TLSDomains,
		Email:       cfg.TLSEmail,
		CacheDir:    cfg.TLSCacheDir,
		HTTPAddr:    cfg.HTTPAddr,
		ACMEURL:     cfg.ACMEURL,
		HSTSSeconds: cfg.HSTSSeconds,
		Logger:      lg,
	}
	if strings.HasPrefix(cfg.PublicURL, "https://") {
		srvCfg.RedirectBase = cfg.PublicURL
	}
	switch {
	case len(cfg.TLSDomains) > 0:
		lg.Info("bingle listening over HTTPS with automatic certificates; the HTTP listener answers challenges and redirects to HTTPS",
			"addr", cfg.Addr, "domains", strings.Join(cfg.TLSDomains, ", "), "authority", acmeName(cfg.ACMEURL),
			"cache", cfg.TLSCacheDir, "http_addr", cfg.HTTPAddr)
	case cfg.TLSEnabled():
		lg.Info("bingle listening over HTTPS", "addr", cfg.Addr, "certificate", cfg.TLSCert)
	default:
		if cfg.TLSCert != "" || cfg.TLSKey != "" {
			lg.Warn("BINGLE_TLS_CERT and BINGLE_TLS_KEY must both be set for HTTPS; serving plain HTTP")
		}
		lg.Info("bingle listening over plain HTTP", "addr", cfg.Addr)
	}
	if err := httpserve.Serve(ctx, srvCfg, a.Router()); err != nil {
		lg.Error("the listener stopped", "err", err)
		os.Exit(1)
	}
}

// acmeName says which certificate authority an ACME directory URL is.
func acmeName(url string) string {
	if url == "" {
		return "Let's Encrypt"
	}
	return url
}
