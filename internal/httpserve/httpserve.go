// Package httpserve binds and runs bingle's HTTP listener: plain HTTP, HTTPS
// from a certificate and key on disk, or HTTPS with certificates that bingle
// gets and renews itself from Let's Encrypt (ACME).
package httpserve

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
)

// Config holds the settings needed to bind and run bingle's HTTP listener.
type Config struct {
	Addr string

	// CertFile and KeyFile serve HTTPS from files. Both must be set; with
	// only one (or neither) the server falls back to plain HTTP.
	CertFile string
	KeyFile  string

	// Domains turns on automatic certificates: the server gets one for each
	// of these names from Let's Encrypt, keeps it in CacheDir and renews it.
	// Only these names are served; a handshake for any other name is refused.
	// It takes the place of CertFile/KeyFile.
	Domains  []string
	Email    string // contact address for the certificate authority (optional)
	CacheDir string // where certificates and the account key are kept
	// HTTPAddr is a second, plain-HTTP listener that answers the
	// certificate authority's HTTP-01 challenge and redirects everything else
	// to HTTPS ("" means none: only the TLS-ALPN-01 challenge on Addr, which
	// then must be reachable on port 443).
	HTTPAddr string
	// ACMEURL is the certificate authority's directory (default: Let's
	// Encrypt's production one).
	ACMEURL string
	// RedirectBase is the https URL the redirect sends browsers to, such as
	// "https://games.example.com:8443"; empty means the name the request
	// came to, on the standard port.
	RedirectBase string

	// HSTSSeconds, when above zero, adds Strict-Transport-Security to the
	// responses of an HTTPS server.
	HSTSSeconds int

	// Logger is the server's logger; nil means slog's default.
	Logger *slog.Logger
}

// logger is cfg's logger, or slog's default for a Config built by a test.
func (cfg Config) logger() *slog.Logger {
	if cfg.Logger == nil {
		return slog.Default()
	}
	return cfg.Logger
}

// Serve starts an HTTP server on cfg.Addr with the given handler, and shuts
// it down gracefully when ctx is cancelled. Only main.go should call this —
// application code must never call net.Listen directly.
//
// With cfg.Domains set it serves HTTPS with automatic certificates (and, when
// cfg.HTTPAddr is set, the challenge/redirect listener beside it). Otherwise,
// if both cfg.CertFile and cfg.KeyFile are set, it serves HTTPS from those
// files. Otherwise — including when only one of the two is set — it falls back
// to plain HTTP.
func Serve(ctx context.Context, cfg Config, handler http.Handler) error {
	useACME := len(cfg.Domains) > 0
	useFiles := !useACME && cfg.CertFile != "" && cfg.KeyFile != ""

	if useACME || useFiles {
		handler = withHSTS(handler, cfg.HSTSSeconds)
	}
	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           handler,
		ReadHeaderTimeout: 20 * time.Second,
	}

	var redirectSrv *http.Server
	if useACME {
		mgr := newManager(cfg)
		srv.TLSConfig = mgr.TLSConfig()
		srv.TLSConfig.MinVersion = tls.VersionTLS12
		if cfg.HTTPAddr != "" {
			redirectSrv = &http.Server{
				Addr:              cfg.HTTPAddr,
				Handler:           mgr.HTTPHandler(redirectHandler(cfg.RedirectBase, handler)),
				ReadHeaderTimeout: 20 * time.Second,
			}
		}
	}

	errCh := make(chan error, 1)
	go func() {
		var err error
		switch {
		case useACME:
			// The certificates come from the TLSConfig.
			err = srv.ListenAndServeTLS("", "")
		case useFiles:
			err = srv.ListenAndServeTLS(cfg.CertFile, cfg.KeyFile)
		default:
			err = srv.ListenAndServe()
		}
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("listen and serve: %w", err)
			return
		}
		errCh <- nil
	}()
	if useACME {
		go warmCertificates(ctx, cfg)
	}
	if redirectSrv != nil {
		// Losing this listener (port 80 taken or not allowed) leaves the
		// TLS-ALPN-01 challenge on the HTTPS port working, so it is not fatal.
		go func() {
			if err := redirectSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				cfg.logger().Error("httpserve: the HTTP listener (certificate challenges and the redirect to HTTPS) is not running",
					"addr", cfg.HTTPAddr, "err", err)
			}
		}()
	}

	select {
	case <-ctx.Done():
		if redirectSrv != nil {
			_ = redirectSrv.Shutdown(context.Background())
		}
		if err := srv.Shutdown(context.Background()); err != nil {
			return fmt.Errorf("shutdown: %w", err)
		}
		return <-errCh
	case err := <-errCh:
		if redirectSrv != nil {
			_ = redirectSrv.Close()
		}
		return err
	}
}

// newManager builds the certificate manager of cfg: it gets certificates for
// the configured names only, and accepts the certificate authority's terms
// (running the server with the setting is the acceptance).
func newManager(cfg Config) *autocert.Manager {
	m := &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist(cfg.Domains...),
		Email:      cfg.Email,
	}
	if cfg.CacheDir != "" {
		m.Cache = autocert.DirCache(cfg.CacheDir)
	}
	if cfg.ACMEURL != "" {
		m.Client = &acme.Client{DirectoryURL: cfg.ACMEURL}
	}
	return m
}

// withHSTS adds Strict-Transport-Security to every response when seconds > 0.
func withHSTS(next http.Handler, seconds int) http.Handler {
	if seconds <= 0 {
		return next
	}
	value := "max-age=" + strconv.Itoa(seconds)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Strict-Transport-Security", value)
		next.ServeHTTP(w, r)
	})
}

// redirectHandler answers what the challenge handler leaves over on the plain
// listener: /healthz (so a container health check works whatever the HTTPS
// setup) and a redirect of everything else to the same path over HTTPS.
func redirectHandler(base string, app http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			app.ServeHTTP(w, r)
			return
		}
		target := httpsURL(base, r)
		code := http.StatusPermanentRedirect // keeps the method of a POST
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			code = http.StatusMovedPermanently
		}
		http.Redirect(w, r, target, code)
	})
}

// httpsURL is the https address of the request: on base when there is one,
// else on the name the request came to.
func httpsURL(base string, r *http.Request) string {
	path := r.URL.RequestURI()
	if base != "" {
		if u, err := url.Parse(base); err == nil && u.Host != "" {
			return "https://" + u.Host + path
		}
	}
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return "https://" + strings.TrimSuffix(host, ".") + path
}

// warmCertificates asks the server for each certificate once it is up, so the
// certificates are fetched when bingle starts instead of making the first
// visitor wait, and so a problem (a name that does not lead here, a port the
// certificate authority cannot reach) shows up in the log right away. It is
// only a convenience: certificates are also fetched on demand, and renewed in
// the background.
func warmCertificates(ctx context.Context, cfg Config) {
	host, port, err := net.SplitHostPort(cfg.Addr)
	if err != nil {
		return
	}
	if ip := net.ParseIP(host); host == "" || (ip != nil && ip.IsUnspecified()) {
		host = "127.0.0.1"
	}
	target := net.JoinHostPort(host, port)
	for _, name := range cfg.Domains {
		if ctx.Err() != nil {
			return
		}
		if err := handshake(ctx, target, name); err != nil {
			cfg.logger().Warn("httpserve: no certificate yet. Check that the name leads to this server and that ports 80 and 443 reach it from the internet; the line above has the certificate authority's answer. Another try is made when a browser connects.",
				"domain", name, "err", err)
			continue
		}
		cfg.logger().Info("httpserve: certificate is ready", "domain", name)
	}
}

// handshake connects to target asking for name and reports whether the
// server answered with a certificate. It trusts nothing: only the handshake
// matters, which is what makes the certificate manager fetch the certificate.
func handshake(ctx context.Context, target, name string) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	// The listener may not be accepting yet.
	var raw net.Conn
	var err error
	for i := 0; i < 20; i++ {
		d := net.Dialer{Timeout: 2 * time.Second}
		if raw, err = d.DialContext(ctx, "tcp", target); err == nil {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
	if err != nil {
		return err
	}
	defer func() { _ = raw.Close() }()
	conn := tls.Client(raw, &tls.Config{ServerName: name, InsecureSkipVerify: true, MinVersion: tls.VersionTLS12}) //nolint:gosec // only fetching the certificate
	return conn.HandshakeContext(ctx)
}
