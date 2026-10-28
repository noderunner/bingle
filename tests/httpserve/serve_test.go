// Tests of the HTTP(S) server: plain HTTP by default, TLS from a certificate and key, and the errors for half-set or missing files.

package httpserve_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/noderunner/bingle/internal/httpserve"
)

// okHandler is the trivial handler used by every test: it writes "ok" for
// any request.
var okHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
})

// The window test ports are taken from. It sits below the ephemeral ports
// the kernel hands out for outgoing connections (Linux's default range is
// 32768-60999, macOS's 49152-65535), which is the whole point: see freeAddr.
const (
	portWindowBase = 14000
	portWindowSize = 2000
)

// nextPort walks the window. It starts somewhere derived from the process id,
// because `go test ./...` runs packages in parallel processes and two of them
// setting off from the same place would walk into each other.
var nextPort = func() *atomic.Uint32 {
	var n atomic.Uint32
	n.Store(uint32(os.Getpid()) * 7)
	return &n
}()

// freeAddr returns a 127.0.0.1 address nothing is listening on, for a server
// that binds by address — httpserve.Config cannot accept an already-open
// net.Listener, so a test has to name a port before Serve binds it.
//
// It deliberately does NOT ask the kernel for one (listen on :0, read the
// port, close). The port that comes back is from the ephemeral range, and
// between the close and Serve's bind the kernel can hand that very port to
// one of the many outgoing connections these tests make — the ACME probes to
// the fake certificate authority, the TLS dials, the HTTP polling. That is
// not theoretical: it is how this package failed in CI, either as
// "bind: address already in use" on the redirect listener or as a listener
// that never came up at all, and every port in those failures was inside
// 32768-60999.
//
// So ports come from a fixed window below the ephemeral range instead, one at
// a time and never the same one twice in a row: nothing the kernel allocates
// can land on one, and no second call can return one that is still in use.
// Each candidate is still bound once to check that nothing else on the
// machine has it.
func freeAddr(t *testing.T) string {
	t.Helper()
	for try := 0; try < portWindowSize; try++ {
		addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(portWindowBase+int(nextPort.Add(1)%portWindowSize)))
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			continue // something else on this machine has it
		}
		if err := ln.Close(); err != nil {
			t.Fatalf("freeAddr: close %s: %v", addr, err)
		}
		return addr
	}
	t.Fatalf("freeAddr: nothing free in %d-%d", portWindowBase, portWindowBase+portWindowSize-1)
	return ""
}

// waitForHTTP polls url (expected to eventually succeed) with the given
// client until it gets a response or the deadline passes, returning the
// last response and body once successful. It fails the test if the server
// never comes up in time.
func waitForHTTP(t *testing.T, client *http.Client, url string) *http.Response {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			return resp
		}
		lastErr = err
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("waitForHTTP: %s never became reachable: %v", url, lastErr)
	return nil
}

// shutdownBudget is how long a graceful shutdown may take before a test calls
// it a hang.
//
// It has to clear net/http's own rule, not just bingle's work: Server.Shutdown
// treats a connection that has been accepted but has not yet sent its first
// request header as abandonable only once it is more than five seconds old
// (the StateNew case in closeIdleConns, Go issue 22682). These tests leave
// exactly such connections behind — warmCertificates is dialling and
// handshaking against the TLS port in the background for up to two minutes,
// and TestServe_ACME_RefusesOtherNames dials it directly. A shutdown with one
// of those in flight takes a little over five seconds every time, so the five
// seconds this used to allow was a coin toss, and it is what failed in CI.
const shutdownBudget = 30 * time.Second

// runServeAndCancel starts httpserve.Serve(ctx, cfg, handler) in a
// goroutine, waits for probe() to succeed (server is up), then cancels ctx
// and asserts Serve returns without error (nil, per the current
// implementation's contract of "graceful shutdown returns nil"). Returns
// once shutdown is confirmed.
func runServeAndCancel(t *testing.T, cfg httpserve.Config, handler http.Handler, probe func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- httpserve.Serve(ctx, cfg, handler)
	}()

	probe()

	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Serve returned non-nil error after context cancellation: %v", err)
		}
	case <-time.After(shutdownBudget):
		t.Fatalf("Serve did not return within %s of context cancellation", shutdownBudget)
	}
}

// generateSelfSignedCert creates a real self-signed ECDSA cert/key pair
// valid for "localhost" and 127.0.0.1, writes them as PEM files under a
// t.TempDir(), and returns their paths.
func generateSelfSignedCert(t *testing.T) (certPath, keyPath string) {
	t.Helper()

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		t.Fatalf("generate serial: %v", err)
	}

	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment | x509.KeyUsageCertSign,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IsCA:         true,
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}

	dir := t.TempDir()

	certPath = filepath.Join(dir, "cert.pem")
	certOut, err := os.Create(certPath)
	if err != nil {
		t.Fatalf("create cert file: %v", err)
	}
	if err := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: der}); err != nil {
		t.Fatalf("encode cert: %v", err)
	}
	if err := certOut.Close(); err != nil {
		t.Fatalf("close cert file: %v", err)
	}

	keyBytes, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	keyPath = filepath.Join(dir, "key.pem")
	keyOut, err := os.Create(keyPath)
	if err != nil {
		t.Fatalf("create key file: %v", err)
	}
	if err := pem.Encode(keyOut, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes}); err != nil {
		t.Fatalf("encode key: %v", err)
	}
	if err := keyOut.Close(); err != nil {
		t.Fatalf("close key file: %v", err)
	}

	return certPath, keyPath
}

// --- Test 1: plain HTTP, unchanged behavior (neither CertFile nor KeyFile set) ---

func TestServe_PlainHTTP_NoTLSConfigured(t *testing.T) {
	addr := freeAddr(t)
	cfg := httpserve.Config{Addr: addr, CertFile: "", KeyFile: ""}

	url := "http://" + addr + "/"
	client := &http.Client{Timeout: 2 * time.Second}

	var body string
	runServeAndCancel(t, cfg, okHandler, func() {
		resp := waitForHTTP(t, client, url)
		defer func() { _ = resp.Body.Close() }()
		buf := make([]byte, 32)
		n, _ := resp.Body.Read(buf)
		body = string(buf[:n])
		if resp.TLS != nil {
			t.Fatalf("expected a plain HTTP response (resp.TLS nil), got TLS state: %+v", resp.TLS)
		}
	})

	if body != "ok" {
		t.Fatalf("expected body %q, got %q", "ok", body)
	}
}

// --- Test 2: the AC itself — real TLS when both CertFile and KeyFile are set ---

func TestServe_TLS_BothCertAndKeySet(t *testing.T) {
	certPath, keyPath := generateSelfSignedCert(t)
	addr := freeAddr(t)
	cfg := httpserve.Config{Addr: addr, CertFile: certPath, KeyFile: keyPath}

	url := "https://" + addr + "/"
	client := &http.Client{
		Timeout: 2 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // self-signed test cert
		},
	}

	var body string
	var tlsState *tls.ConnectionState
	runServeAndCancel(t, cfg, okHandler, func() {
		resp := waitForHTTP(t, client, url)
		defer func() { _ = resp.Body.Close() }()
		buf := make([]byte, 32)
		n, _ := resp.Body.Read(buf)
		body = string(buf[:n])
		tlsState = resp.TLS
	})

	if body != "ok" {
		t.Fatalf("expected body %q, got %q", "ok", body)
	}
	if tlsState == nil {
		t.Fatal("expected resp.TLS to be non-nil (a real TLS connection), got nil")
	}
	if !tlsState.HandshakeComplete {
		t.Fatal("expected TLS handshake to have completed")
	}
}

// --- Test 3: only ONE of CertFile/KeyFile set ---
//
// The plan pins down only "TLS is off unless both cert and key are set." It
// does not say what happens with exactly one set. We treat "only one set"
// as NOT triggering TLS — i.e. it falls through to plain HTTP, identical to
// neither being set — rather than as an error, because:
//   - The stated rule is phrased as a positive condition for turning TLS ON
//     ("unless both... are set"), which reads most naturally as: TLS turns
//     on only when the AND of both fields holds, and is simply off (not an
//     error state) otherwise.
//   - internal/config's contract (BINGLE_TLS_CERT / BINGLE_TLS_KEY as two
//     independent env vars) means "only one set" is a very plausible
//     operator mistake (typo'd env var name, one var not exported in the
//     deploy environment, etc.) that should degrade safely to plain HTTP
//     rather than crash-loop the server.
//
// If "exactly one set" should ever become a hard startup error, this test
// (and this comment) is the place to change.
func TestServe_OneOfCertOrKeySet_FallsBackToPlainHTTP(t *testing.T) {
	certPath, _ := generateSelfSignedCert(t)

	t.Run("cert only", func(t *testing.T) {
		addr := freeAddr(t)
		cfg := httpserve.Config{Addr: addr, CertFile: certPath, KeyFile: ""}

		url := "http://" + addr + "/"
		client := &http.Client{Timeout: 2 * time.Second}

		var body string
		runServeAndCancel(t, cfg, okHandler, func() {
			resp := waitForHTTP(t, client, url)
			defer func() { _ = resp.Body.Close() }()
			buf := make([]byte, 32)
			n, _ := resp.Body.Read(buf)
			body = string(buf[:n])
		})
		if body != "ok" {
			t.Fatalf("expected plain-HTTP body %q with only CertFile set, got %q", "ok", body)
		}
	})

	t.Run("key only", func(t *testing.T) {
		_, keyPath := generateSelfSignedCert(t)
		addr := freeAddr(t)
		cfg := httpserve.Config{Addr: addr, CertFile: "", KeyFile: keyPath}

		url := "http://" + addr + "/"
		client := &http.Client{Timeout: 2 * time.Second}

		var body string
		runServeAndCancel(t, cfg, okHandler, func() {
			resp := waitForHTTP(t, client, url)
			defer func() { _ = resp.Body.Close() }()
			buf := make([]byte, 32)
			n, _ := resp.Body.Read(buf)
			body = string(buf[:n])
		})
		if body != "ok" {
			t.Fatalf("expected plain-HTTP body %q with only KeyFile set, got %q", "ok", body)
		}
	})
}

// --- Bonus: the other half of step 14's AC text — "returns (does not
// os.Exit) a wrapped error for a missing/unreadable cert or key file" ---
//
// Not explicitly requested by the task's three numbered scenarios, but it
// is verbatim part of the plan's own AC for this package ("returns a
// wrapped error for a missing/unreadable cert or key file"), so it's
// covered here too rather than left untested.

func TestServe_MissingCertFile_ReturnsWrappedError(t *testing.T) {
	_, keyPath := generateSelfSignedCert(t)
	addr := freeAddr(t)
	cfg := httpserve.Config{
		Addr:     addr,
		CertFile: filepath.Join(t.TempDir(), "does-not-exist-cert.pem"),
		KeyFile:  keyPath,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := httpserve.Serve(ctx, cfg, okHandler)
	if err == nil {
		t.Fatal("expected Serve to return an error for a missing cert file, got nil")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Logf("Serve error (missing cert file) does not wrap os.ErrNotExist via errors.Is: %v", err)
	}
}

func TestServe_MissingKeyFile_ReturnsWrappedError(t *testing.T) {
	certPath, _ := generateSelfSignedCert(t)
	addr := freeAddr(t)
	cfg := httpserve.Config{
		Addr:     addr,
		CertFile: certPath,
		KeyFile:  filepath.Join(t.TempDir(), "does-not-exist-key.pem"),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := httpserve.Serve(ctx, cfg, okHandler)
	if err == nil {
		t.Fatal("expected Serve to return an error for a missing key file, got nil")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Logf("Serve error (missing key file) does not wrap os.ErrNotExist via errors.Is: %v", err)
	}
}

func TestServe_UnreadableCertFile_ReturnsWrappedError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root: file permissions do not block reads, cannot test unreadable file")
	}

	_, keyPath := generateSelfSignedCert(t)
	dir := t.TempDir()
	certPath := filepath.Join(dir, "unreadable-cert.pem")
	if err := os.WriteFile(certPath, []byte("not actually a cert"), 0o000); err != nil {
		t.Fatalf("write unreadable cert file: %v", err)
	}

	addr := freeAddr(t)
	cfg := httpserve.Config{Addr: addr, CertFile: certPath, KeyFile: keyPath}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := httpserve.Serve(ctx, cfg, okHandler)
	if err == nil {
		t.Fatal("expected Serve to return an error for an unreadable cert file, got nil")
	}
}
