// Tests of the secure-connection rule as the listener enforces it: a
// generated certificate that survives a restart, a proxy whose word is taken
// for the scheme, and a loopback bind that still serves plain HTTP so the
// in-process suites keep working.

package httpserve_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/noderunner/bingle/internal/httpserve"
)

// runServe starts a listener and returns a function that stops it, failing
// the test if it does not stop.
func runServe(t *testing.T, cfg httpserve.Config, handler http.Handler) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- httpserve.Serve(ctx, cfg, handler) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-errCh:
			if err != nil {
				t.Errorf("Serve returned %v, want nil", err)
			}
		case <-time.After(shutdownBudget):
			t.Errorf("Serve did not return within %s", shutdownBudget)
		}
	})
}

// insecureClient trusts anything, because a generated certificate is by
// definition not trusted; the test is about serving it, not about trusting it.
func insecureClient() *http.Client {
	return &http.Client{
		Timeout:   5 * time.Second,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}, //nolint:gosec // the point of the test
	}
}

// A certificate bingle generates for itself is cached, so it is the same one
// after a restart — otherwise every restart is a fresh browser warning and
// nobody would leave it on.
func TestSelfSigned_IsReusedAcrossRestarts(t *testing.T) {
	dir := t.TempDir()

	first, err := httpserve.EnsureSelfSigned(dir, "games.example.test")
	if err != nil {
		t.Fatalf("generating: %v", err)
	}
	if !first.Generated {
		t.Error("the first call did not generate a certificate")
	}
	if first.Fingerprint == "" {
		t.Error("no fingerprint, so an operator cannot check what they are trusting")
	}

	second, err := httpserve.EnsureSelfSigned(dir, "games.example.test")
	if err != nil {
		t.Fatalf("reusing: %v", err)
	}
	if second.Generated {
		t.Error("the second call generated a new certificate instead of reusing the cached one")
	}
	if second.Fingerprint != first.Fingerprint {
		t.Errorf("the fingerprint changed across restarts: %s then %s", first.Fingerprint, second.Fingerprint)
	}

	// It is valid for the configured name and for the local ones, because the
	// reason to want this is usually to reach the server from this machine.
	der, err := os.ReadFile(first.CertFile)
	if err != nil {
		t.Fatalf("reading the certificate: %v", err)
	}
	cert := parsePEM(t, der)
	for _, host := range []string{"games.example.test", "localhost", "127.0.0.1", "::1"} {
		if err := cert.VerifyHostname(host); err != nil {
			t.Errorf("the certificate is not valid for %s: %v", host, err)
		}
	}
	// And the key is not readable by anybody else.
	info, err := os.Stat(first.KeyFile)
	if err != nil {
		t.Fatalf("stat the key: %v", err)
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		t.Errorf("the private key is mode %04o: readable by group or others", perm)
	}
}

// A cached certificate that no longer covers the host is replaced rather than
// served, so moving a server to a new name does not leave it unreachable.
func TestSelfSigned_IsReplacedWhenTheHostChanges(t *testing.T) {
	dir := t.TempDir()
	first, err := httpserve.EnsureSelfSigned(dir, "old.example.test")
	if err != nil {
		t.Fatal(err)
	}
	second, err := httpserve.EnsureSelfSigned(dir, "new.example.test")
	if err != nil {
		t.Fatal(err)
	}
	if !second.Generated {
		t.Error("the certificate was reused for a host it does not cover")
	}
	if second.Fingerprint == first.Fingerprint {
		t.Error("the fingerprint did not change")
	}
}

// The generated certificate is actually served, and HTTPS works.
func TestServe_SelfSignedServesHTTPS(t *testing.T) {
	addr := freeAddr(t)
	runServe(t, httpserve.Config{Addr: addr, SelfSigned: true, CacheDir: t.TempDir()}, okHandler)

	resp, body := waitForTLS(t, insecureClient(), "https://"+addr+"/")
	if resp.StatusCode != http.StatusOK || body != "ok" {
		t.Errorf("GET over https = %d %q", resp.StatusCode, body)
	}
}

// Behind a trusted proxy, bingle serves plain HTTP and takes the proxy's word
// for the scheme — refusing anything that did not arrive as https.
func TestServe_BehindATrustedProxy(t *testing.T) {
	addr := freeAddr(t)
	runServe(t, httpserve.Config{Addr: addr, TrustProxy: true}, okHandler)

	client := noRedirect()
	// A request that reached bingle directly, as a client bypassing the proxy
	// would: refused.
	resp := waitForHTTP(t, client, "http://"+addr+"/")
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusMisdirectedRequest {
		t.Errorf("a plain-HTTP request = %d, want %d", resp.StatusCode, http.StatusMisdirectedRequest)
	}

	// The same request as the proxy makes it: served.
	req, err := http.NewRequest(http.MethodGet, "http://"+addr+"/", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Forwarded-Proto", "https")
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("GET with the proxy's header: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || string(body) != "ok" {
		t.Errorf("a forwarded https request = %d %q", resp.StatusCode, body)
	}

	// A proxy that says it served plain HTTP is refused too: the point is the
	// client's leg, not bingle's.
	req.Header.Set("X-Forwarded-Proto", "http")
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusMisdirectedRequest {
		t.Errorf("a forwarded http request = %d, want it refused", resp.StatusCode)
	}
}

// A loopback bind serves plain HTTP with nothing configured. This is what
// keeps `./bin/bingle` working out of the box and what the ~100 in-process
// browser tests run against.
func TestServe_LoopbackStillServesPlainHTTP(t *testing.T) {
	addr := freeAddr(t)
	runServe(t, httpserve.Config{Addr: addr, LoopbackOK: true}, okHandler)

	resp := waitForHTTP(t, noRedirect(), "http://"+addr+"/")
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || string(body) != "ok" {
		t.Errorf("GET over loopback http = %d %q", resp.StatusCode, body)
	}
}

// Without that exemption even a loopback request is refused, which is what a
// public bind behind no proxy would do if config had let it start.
func TestServe_WithoutTheExemptionPlainHTTPIsRefused(t *testing.T) {
	addr := freeAddr(t)
	runServe(t, httpserve.Config{Addr: addr, TrustProxy: true}, okHandler)

	resp := waitForHTTP(t, noRedirect(), "http://"+addr+"/")
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusMisdirectedRequest {
		t.Errorf("= %d %q, want it refused", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "not secure") {
		t.Errorf("the refusal does not say why: %q", body)
	}
}

// parsePEM decodes a PEM certificate for inspection.
func parsePEM(t *testing.T, raw []byte) *x509.Certificate {
	t.Helper()
	block, _ := pem.Decode(raw)
	if block == nil || block.Type != "CERTIFICATE" {
		t.Fatal("the certificate file is not a PEM certificate")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parsing the certificate: %v", err)
	}
	return cert
}

// waitForTLS polls an HTTPS URL until it answers.
func waitForTLS(t *testing.T, client *http.Client, url string) (*http.Response, string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			body, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			return resp, string(body)
		}
		lastErr = err
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("%s never answered: %v", url, lastErr)
	return nil, ""
}
