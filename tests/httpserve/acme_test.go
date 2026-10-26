package httpserve_test

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/noderunner/bingle/internal/httpserve"
)

// acmeConfig is a server with automatic certificates for games.example.test.
// The certificate authority is an address nothing listens on, so no
// certificate is ever issued: these tests cover everything around it.
func acmeConfig(t *testing.T) httpserve.Config {
	return httpserve.Config{
		Addr:     freeAddr(t),
		HTTPAddr: freeAddr(t),
		Domains:  []string{"games.example.test"},
		CacheDir: t.TempDir(),
		ACMEURL:  "https://127.0.0.1:1/directory",
	}
}

// noRedirect makes a client that reports a redirect instead of following it.
func noRedirect() *http.Client {
	return &http.Client{
		Timeout:       2 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

func TestServe_ACME_PlainListenerRedirectsToHTTPS(t *testing.T) {
	cfg := acmeConfig(t)
	cfg.RedirectBase = ""
	runServeAndCancel(t, cfg, okHandler, func() {
		client := noRedirect()
		waitForHTTP(t, client, "http://"+cfg.HTTPAddr+"/")

		// A browser gets the same path on https, on the name it asked for
		// (without the plain port).
		req, _ := http.NewRequest(http.MethodGet, "http://"+cfg.HTTPAddr+"/game/ABC123?x=1", nil)
		req.Host = "games.example.test:8080"
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusMovedPermanently || resp.Header.Get("Location") != "https://games.example.test/game/ABC123?x=1" {
			t.Errorf("GET: %d, Location %q", resp.StatusCode, resp.Header.Get("Location"))
		}

		// A POST keeps its method through the redirect.
		req, _ = http.NewRequest(http.MethodPost, "http://"+cfg.HTTPAddr+"/api/games", strings.NewReader("{}"))
		req.Host = "games.example.test"
		resp, err = client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusPermanentRedirect {
			t.Errorf("POST: %d, want 308", resp.StatusCode)
		}

		// The health check does not redirect, so a container check works.
		resp, err = client.Get("http://" + cfg.HTTPAddr + "/healthz")
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK || string(body) != "ok" {
			t.Errorf("/healthz: %d %q, want the app's answer", resp.StatusCode, body)
		}
	})
}

func TestServe_ACME_RedirectUsesThePublicURLsPort(t *testing.T) {
	cfg := acmeConfig(t)
	cfg.RedirectBase = "https://games.example.test:8443"
	runServeAndCancel(t, cfg, okHandler, func() {
		client := noRedirect()
		resp := waitForHTTP(t, client, "http://"+cfg.HTTPAddr+"/a")
		_ = resp.Body.Close()
		if got := resp.Header.Get("Location"); got != "https://games.example.test:8443/a" {
			t.Errorf("Location = %q", got)
		}
	})
}

func TestServe_ACME_ChallengePathIsNotRedirected(t *testing.T) {
	cfg := acmeConfig(t)
	runServeAndCancel(t, cfg, okHandler, func() {
		client := noRedirect()
		waitForHTTP(t, client, "http://"+cfg.HTTPAddr+"/")
		req, _ := http.NewRequest(http.MethodGet, "http://"+cfg.HTTPAddr+"/.well-known/acme-challenge/unknown-token", nil)
		req.Host = "games.example.test"
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		// The challenge handler owns this path: an unknown token is "not found", never a redirect.
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("challenge path: %d, want 404", resp.StatusCode)
		}
	})
}

// Only the configured names get a certificate: a client asking for another name is refused.
func TestServe_ACME_RefusesOtherNames(t *testing.T) {
	cfg := acmeConfig(t)
	runServeAndCancel(t, cfg, okHandler, func() {
		waitForHTTP(t, noRedirect(), "http://"+cfg.HTTPAddr+"/")
		for _, name := range []string{"evil.example.test", "127.0.0.1"} {
			var conn *tls.Conn
			var err error
			deadline := time.Now().Add(3 * time.Second)
			for time.Now().Before(deadline) {
				d := &net.Dialer{Timeout: time.Second}
				conn, err = tls.DialWithDialer(d, "tcp", cfg.Addr, &tls.Config{ServerName: name, InsecureSkipVerify: true}) //nolint:gosec // only the refusal is looked at
				if err == nil || strings.Contains(err.Error(), "handshake") || strings.Contains(err.Error(), "tls:") {
					break
				}
				time.Sleep(30 * time.Millisecond)
			}
			if err == nil {
				_ = conn.Close()
				t.Errorf("a certificate was served for %q", name)
			}
		}
	})
}

// An unavailable plain port is not fatal: the HTTPS side keeps running.
func TestServe_ACME_PlainPortInUseIsNotFatal(t *testing.T) {
	cfg := acmeConfig(t)
	taken, err := net.Listen("tcp", cfg.HTTPAddr)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = taken.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- httpserve.Serve(ctx, cfg, okHandler) }()
	select {
	case err := <-errCh:
		t.Fatalf("Serve ended early: %v", err)
	case <-time.After(500 * time.Millisecond):
	}
	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Serve returned %v after shutdown", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not stop")
	}
}

// Both listeners are closed on shutdown.
func TestServe_ACME_ShutdownFreesBothPorts(t *testing.T) {
	cfg := acmeConfig(t)
	runServeAndCancel(t, cfg, okHandler, func() {
		waitForHTTP(t, noRedirect(), "http://"+cfg.HTTPAddr+"/")
	})
	for _, addr := range []string{cfg.Addr, cfg.HTTPAddr} {
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			t.Errorf("%s still in use after shutdown: %v", addr, err)
			continue
		}
		_ = ln.Close()
	}
}

func TestServe_HSTSOnlyOverHTTPSAndOnlyWhenAsked(t *testing.T) {
	certPath, keyPath := generateSelfSignedCert(t)
	insecure := &http.Client{
		Timeout:   2 * time.Second,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}, //nolint:gosec // self-signed test cert
	}
	for _, c := range []struct {
		seconds int
		want    string
	}{{0, ""}, {600, "max-age=600"}} {
		cfg := httpserve.Config{Addr: freeAddr(t), CertFile: certPath, KeyFile: keyPath, HSTSSeconds: c.seconds}
		runServeAndCancel(t, cfg, okHandler, func() {
			resp := waitForHTTP(t, insecure, "https://"+cfg.Addr+"/")
			_ = resp.Body.Close()
			if got := resp.Header.Get("Strict-Transport-Security"); got != c.want {
				t.Errorf("HSTSSeconds=%d: header %q, want %q", c.seconds, got, c.want)
			}
		})
	}
	// Plain HTTP never carries it, whatever the setting.
	cfg := httpserve.Config{Addr: freeAddr(t), HSTSSeconds: 600}
	runServeAndCancel(t, cfg, okHandler, func() {
		resp := waitForHTTP(t, &http.Client{Timeout: 2 * time.Second}, "http://"+cfg.Addr+"/")
		_ = resp.Body.Close()
		if got := resp.Header.Get("Strict-Transport-Security"); got != "" {
			t.Errorf("plain HTTP response has Strict-Transport-Security %q", got)
		}
	})
}
