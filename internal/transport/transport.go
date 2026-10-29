// Package transport answers three questions about a request that the rest of
// bingle must not answer for itself: did it arrive securely, where did it come
// from, and is it local.
//
// They look trivial and are not. Behind a terminating proxy the answers come
// from headers a client can also send, so they are only trustworthy when the
// operator has said there is a proxy; without that they must be ignored
// completely. And the third question is a trap:
//
//	A loopback request is exempt from things a public one is not — admin
//	two-factor, the deterministic development password. With a proxy on the
//	same host, *every* request arrives from 127.0.0.1. So when a proxy is
//	trusted, no loopback exemption applies at all.
//
// That rule is load-bearing (docs/history/auth-and-accounts-plan.md, section
// 2) and it is enforced here, once, rather than remembered at each call site.
package transport

import (
	"net"
	"net/http"
	"strings"
)

// Policy is how to read a request, given what the operator has said about the
// network in front of bingle. Its zero value is the safe one: no proxy
// trusted, no headers believed.
type Policy struct {
	// TrustProxy says a terminating proxy is in front of bingle and its
	// forwarded headers may be believed (BINGLE_TRUST_PROXY). It also turns
	// every loopback exemption off; see the package comment.
	TrustProxy bool
}

// Forwarded headers. Only these two, and only from a trusted proxy: RFC 7239's
// `Forwarded` is not read, because supporting two spellings of the same claim
// means two things to get wrong.
const (
	headerProto = "X-Forwarded-Proto"
	headerFor   = "X-Forwarded-For"
)

// Scheme is how the request reached the client's end of the connection:
// "https" or "http".
//
// Without a trusted proxy this is simply whether bingle itself terminated
// TLS. With one it is what the proxy says, because bingle's own leg is plain
// HTTP by design.
func (p Policy) Scheme(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	if p.TrustProxy {
		if proto := forwardedProto(r); proto != "" {
			return proto
		}
	}
	return "http"
}

// Secure reports whether the connection is one bingle will carry a password or
// a session cookie over.
func (p Policy) Secure(r *http.Request) bool {
	return p.Scheme(r) == "https"
}

// PeerIP is the address the request came from: the socket's, or — behind a
// trusted proxy — the address that proxy saw.
//
// It assumes **one** trusted hop, which is what BINGLE_TRUST_PROXY describes,
// and so takes the last entry of X-Forwarded-For: a proxy appends the peer it
// saw, so anything a client put in the header earlier is to its left and is
// ignored. With two proxies the address is the first one's, which is wrong but
// in the safe direction: it narrows nobody's rate limit but its own.
func (p Policy) PeerIP(r *http.Request) string {
	if p.TrustProxy {
		if ip := lastForwardedFor(r); ip != "" {
			return ip
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// Loopback reports whether the request came from this machine, and so whether
// the exemptions a local operator gets may apply.
//
// It is **always false when a proxy is trusted**, however local the socket
// looks, because a proxy on the same host makes every request local. See the
// package comment: this is the rule the whole exemption scheme rests on.
func (p Policy) Loopback(r *http.Request) bool {
	if p.TrustProxy {
		return false
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// RequireSecure refuses a request that did not arrive securely.
//
// It is middleware rather than a check inside the router on purpose: the
// router stays transport-agnostic, so the in-process test suites keep running
// over loopback HTTP and the browser tests keep their trustworthy origin
// (docs/history/auth-and-accounts-plan.md, section 2).
//
// exempt, when set, says a request may pass anyway. internal/httpserve uses it
// to let a loopback bind serve plain HTTP, which is what makes `./bin/bingle`
// on a laptop and `httptest` in a test work with nothing configured.
func (p Policy) RequireSecure(exempt func(*http.Request) bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if p.Secure(r) || (exempt != nil && exempt(r)) {
			next.ServeHTTP(w, r)
			return
		}
		// 421 says "you reached the right server by the wrong route", which
		// is exactly the case: the operator has said there is a proxy and
		// this request did not come through it.
		http.Error(w, "bingle: this connection is not secure. "+
			"Reach the server over HTTPS, or set X-Forwarded-Proto on the proxy in front of it.",
			http.StatusMisdirectedRequest)
	})
}

// forwardedProto is the scheme a proxy says it served, lower-cased and taking
// the first of a list.
func forwardedProto(r *http.Request) string {
	raw := r.Header.Get(headerProto)
	if raw == "" {
		return ""
	}
	if i := strings.IndexByte(raw, ','); i >= 0 {
		raw = raw[:i]
	}
	switch proto := strings.ToLower(strings.TrimSpace(raw)); proto {
	case "https", "http":
		return proto
	default:
		return ""
	}
}

// lastForwardedFor is the rightmost address of X-Forwarded-For; see PeerIP.
func lastForwardedFor(r *http.Request) string {
	raw := r.Header.Get(headerFor)
	if raw == "" {
		return ""
	}
	parts := strings.Split(raw, ",")
	for i := len(parts) - 1; i >= 0; i-- {
		candidate := strings.TrimSpace(parts[i])
		// A proxy may write "host:port", and IPv6 arrives bracketed.
		if host, _, err := net.SplitHostPort(candidate); err == nil {
			candidate = host
		}
		candidate = strings.Trim(candidate, "[]")
		if net.ParseIP(candidate) != nil {
			return candidate
		}
	}
	return ""
}
