// Command bingle runs the bingle server.
package main

import (
	"context"
	_ "embed" // CHANGELOG.md
	"log"
	"os/signal"
	"strings"
	"syscall"

	"github.com/noderunner/bingle/internal/app"
	"github.com/noderunner/bingle/internal/config"
	"github.com/noderunner/bingle/internal/httpserve"
)

// changelog is CHANGELOG.md, built into the binary for the help window.
//
//go:embed CHANGELOG.md
var changelog string

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	cfg.Changelog = changelog

	a, err := app.New(cfg)
	if err != nil {
		log.Fatalf("build app: %v", err)
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
	}
	if strings.HasPrefix(cfg.PublicURL, "https://") {
		srvCfg.RedirectBase = cfg.PublicURL
	}
	switch {
	case len(cfg.TLSDomains) > 0:
		log.Printf("bingle listening on %s over HTTPS, certificates for %s from %s (cache %s); %s answers challenges and redirects to HTTPS",
			cfg.Addr, strings.Join(cfg.TLSDomains, ", "), acmeName(cfg.ACMEURL), cfg.TLSCacheDir, cfg.HTTPAddr)
	case cfg.TLSEnabled():
		log.Printf("bingle listening on %s over HTTPS (certificate %s)", cfg.Addr, cfg.TLSCert)
	default:
		if cfg.TLSCert != "" || cfg.TLSKey != "" {
			log.Printf("warning: BINGLE_TLS_CERT and BINGLE_TLS_KEY must both be set for HTTPS; serving plain HTTP")
		}
		log.Printf("bingle listening on %s (plain HTTP)", cfg.Addr)
	}
	if err := httpserve.Serve(ctx, srvCfg, a.Router()); err != nil {
		log.Fatalf("serve: %v", err)
	}
}

// acmeName says which certificate authority an ACME directory URL is.
func acmeName(url string) string {
	if url == "" {
		return "Let's Encrypt"
	}
	return url
}
