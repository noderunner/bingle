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
	"github.com/noderunner/bingle/internal/logging"
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

	a, err := app.New(cfg, app.WithLogger(lg))
	if err != nil {
		lg.Error("the server could not be built", "err", err)
		os.Exit(1)
	}
	defer func() { _ = a.Close() }()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

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
