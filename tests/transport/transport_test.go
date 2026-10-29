// Tests of how a request's scheme, peer and locality are read — and of the
// rule the whole exemption scheme rests on: behind a trusted proxy, nothing
// is local.

package transport_test

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/noderunner/bingle/internal/transport"
)

// tlsState stands in for a request that arrived over TLS at bingle itself.
var tlsState = tls.ConnectionState{}

// req builds a request with a peer address and headers.
func req(remote string, headers map[string]string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	r.RemoteAddr = remote
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	return r
}

// Without a trusted proxy, a forwarded header is a claim anybody can make and
// is ignored completely.
func TestPolicy_WithoutAProxyNoHeaderIsBelieved(t *testing.T) {
	var p transport.Policy // the zero value is the safe one

	r := req("203.0.113.7:4000", map[string]string{
		"X-Forwarded-Proto": "https",
		"X-Forwarded-For":   "198.51.100.9",
	})
	if p.Secure(r) {
		t.Error("a request claiming https over plain HTTP was called secure")
	}
	if got := p.Scheme(r); got != "http" {
		t.Errorf("Scheme = %q, want http", got)
	}
	if got := p.PeerIP(r); got != "203.0.113.7" {
		t.Errorf("PeerIP = %q, want the socket's address", got)
	}
}

// With a trusted proxy the headers are the only source of truth, because
// bingle's own leg is plain HTTP by design.
func TestPolicy_WithAProxyTheHeadersAreRead(t *testing.T) {
	p := transport.Policy{TrustProxy: true}

	r := req("10.0.0.2:5000", map[string]string{
		"X-Forwarded-Proto": "https",
		"X-Forwarded-For":   "198.51.100.9",
	})
	if !p.Secure(r) {
		t.Error("a request the proxy served over https was not called secure")
	}
	if got := p.PeerIP(r); got != "198.51.100.9" {
		t.Errorf("PeerIP = %q, want the address the proxy saw", got)
	}

	// A proxy that served plain HTTP says so, and that is not secure.
	r = req("10.0.0.2:5000", map[string]string{"X-Forwarded-Proto": "http"})
	if p.Secure(r) {
		t.Error("a request the proxy served over http was called secure")
	}
	// And a proxy that says nothing is not evidence of anything.
	if p.Secure(req("10.0.0.2:5000", nil)) {
		t.Error("a request with no forwarded header was called secure")
	}
}

// TLS at bingle itself needs no header and is believed either way.
func TestPolicy_ItsOwnTLSIsAlwaysEnough(t *testing.T) {
	for _, p := range []transport.Policy{{}, {TrustProxy: true}} {
		r := req("203.0.113.7:4000", nil)
		r.TLS = &tlsState
		if !p.Secure(r) {
			t.Errorf("TrustProxy=%v: a TLS request was not called secure", p.TrustProxy)
		}
		if got := p.Scheme(r); got != "https" {
			t.Errorf("TrustProxy=%v: Scheme = %q", p.TrustProxy, got)
		}
	}
}

// The rule the exemptions rest on. A proxy on the same host makes every
// request arrive from 127.0.0.1, so trusting a proxy must switch locality off
// altogether — otherwise every visitor in the world would be exempt from
// admin two-factor and would be offered the development password.
func TestPolicy_ATrustedProxyMakesNothingLocal(t *testing.T) {
	local := []string{"127.0.0.1:5000", "[::1]:5000", "127.0.0.53:5000"}

	var direct transport.Policy
	for _, addr := range local {
		if !direct.Loopback(req(addr, nil)) {
			t.Errorf("without a proxy, %s was not local", addr)
		}
	}

	proxied := transport.Policy{TrustProxy: true}
	for _, addr := range local {
		if proxied.Loopback(req(addr, nil)) {
			t.Fatalf("behind a trusted proxy, %s was called local: every request would be exempt", addr)
		}
	}
	// Not even when the proxy helpfully forwards a loopback address.
	r := req("127.0.0.1:5000", map[string]string{"X-Forwarded-For": "127.0.0.1"})
	if proxied.Loopback(r) {
		t.Error("behind a trusted proxy, a forwarded loopback address was called local")
	}
}

// A request from elsewhere is not local, proxy or no proxy.
func TestPolicy_APublicAddressIsNeverLocal(t *testing.T) {
	for _, p := range []transport.Policy{{}, {TrustProxy: true}} {
		if p.Loopback(req("203.0.113.7:4000", nil)) {
			t.Errorf("TrustProxy=%v: a public address was called local", p.TrustProxy)
		}
	}
}

// One trusted hop appends the peer it saw, so the rightmost entry is the
// client and anything a client wrote earlier is to its left and ignored.
func TestPolicy_TheRightmostForwardedAddressWins(t *testing.T) {
	p := transport.Policy{TrustProxy: true}
	for _, c := range []struct{ header, want string }{
		{header: "198.51.100.9", want: "198.51.100.9"},
		{header: "10.1.1.1, 198.51.100.9", want: "198.51.100.9"},
		// A client spoofing the header cannot push itself to the right: the
		// proxy appends after whatever it sent.
		{header: "203.0.113.200, 198.51.100.9", want: "198.51.100.9"},
		{header: "198.51.100.9:4000", want: "198.51.100.9"},
		{header: "[2001:db8::1]:4000", want: "2001:db8::1"},
		{header: "2001:db8::1", want: "2001:db8::1"},
		// Rubbish falls back to the socket.
		{header: "not-an-address", want: "10.0.0.2"},
		{header: "", want: "10.0.0.2"},
	} {
		r := req("10.0.0.2:5000", nil)
		if c.header != "" {
			r.Header.Set("X-Forwarded-For", c.header)
		}
		if got := p.PeerIP(r); got != c.want {
			t.Errorf("X-Forwarded-For %q gave %q, want %q", c.header, got, c.want)
		}
	}
}

// RequireSecure refuses an insecure request, and says what to do about it.
func TestRequireSecure_RefusesAnInsecureRequest(t *testing.T) {
	served := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})

	p := transport.Policy{TrustProxy: true}
	h := p.RequireSecure(nil, served)

	w := httptest.NewRecorder()
	h.ServeHTTP(w, req("203.0.113.7:4000", nil))
	if w.Code != http.StatusMisdirectedRequest {
		t.Errorf("an insecure request = %d, want %d", w.Code, http.StatusMisdirectedRequest)
	}
	if body := w.Body.String(); !strings.Contains(body, "X-Forwarded-Proto") {
		t.Errorf("the refusal does not say how to fix it: %q", body)
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, req("203.0.113.7:4000", map[string]string{"X-Forwarded-Proto": "https"}))
	if w.Code != http.StatusOK || w.Body.String() != "ok" {
		t.Errorf("a forwarded https request = %d %q", w.Code, w.Body.String())
	}
}

// The loopback exemption is what keeps `./bin/bingle` with nothing configured
// and every in-process test working.
func TestRequireSecure_ExemptsWhatItIsToldTo(t *testing.T) {
	served := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	var p transport.Policy
	h := p.RequireSecure(p.Loopback, served)

	w := httptest.NewRecorder()
	h.ServeHTTP(w, req("127.0.0.1:5000", nil))
	if w.Code != http.StatusOK {
		t.Errorf("a loopback request = %d, want it served", w.Code)
	}

	w = httptest.NewRecorder()
	h.ServeHTTP(w, req("203.0.113.7:4000", nil))
	if w.Code != http.StatusMisdirectedRequest {
		t.Errorf("a public plain-HTTP request = %d, want it refused", w.Code)
	}
}
