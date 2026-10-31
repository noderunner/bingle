// Package fakeoidc is an OpenID Connect provider in a test's own process,
// as internal/mail/fakesmtp is a mail server: discovery, a signing key, an
// authorization endpoint that approves at once whoever the test says is
// signing in, and a token endpoint that checks PKCE and spends each code
// once. It is for tests only.
package fakeoidc

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

// Identity is who signs in at the provider.
type Identity struct {
	Subject       string
	Email         string
	EmailVerified bool
	Name          string
}

// Provider is a running fake provider.
type Provider struct {
	Server       *httptest.Server
	ClientID     string
	ClientSecret string

	key *rsa.PrivateKey
	mu  sync.Mutex
	who Identity
	// codes are the authorization codes handed out and not yet spent.
	codes map[string]grant
	// Exchanged counts the codes spent at the token endpoint.
	Exchanged int
}

type grant struct {
	nonce, challenge, redirect string
	who                        Identity
}

// Start runs a provider until the test ends.
func Start(t testing.TB) *Provider {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	p := &Provider{ClientID: "bingle-test", ClientSecret: "fake-secret", key: key, codes: map[string]grant{},
		who: Identity{Subject: "sub-1", Email: "ana@example.com", EmailVerified: true, Name: "Ana"}}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", p.discovery)
	mux.HandleFunc("/jwks", p.jwks)
	mux.HandleFunc("/authorize", p.authorize)
	mux.HandleFunc("/token", p.token)
	p.Server = httptest.NewServer(mux)
	t.Cleanup(p.Server.Close)
	return p
}

// Issuer is the provider's issuer URL.
func (p *Provider) Issuer() string { return p.Server.URL }

// SignInAs sets who the next sign-in is.
func (p *Provider) SignInAs(who Identity) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.who = who
}

func (p *Provider) discovery(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{
		"issuer":                                p.Issuer(),
		"authorization_endpoint":                p.Issuer() + "/authorize",
		"token_endpoint":                        p.Issuer() + "/token",
		"jwks_uri":                              p.Issuer() + "/jwks",
		"response_types_supported":              []string{"code"},
		"subject_types_supported":               []string{"public"},
		"id_token_signing_alg_values_supported": []string{"RS256"},
		"code_challenge_methods_supported":      []string{"S256"},
	})
}

func (p *Provider) jwks(w http.ResponseWriter, _ *http.Request) {
	pub := p.key.PublicKey
	writeJSON(w, map[string]any{"keys": []map[string]string{{
		"kty": "RSA", "alg": "RS256", "use": "sig", "kid": "k1",
		"n": b64(pub.N.Bytes()), "e": b64(big.NewInt(int64(pub.E)).Bytes()),
	}}})
}

// authorize approves at once: the browser goes straight back with a code.
func (p *Provider) authorize(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if q.Get("client_id") != p.ClientID || q.Get("response_type") != "code" || q.Get("code_challenge_method") != "S256" || q.Get("code_challenge") == "" {
		http.Error(w, "bad authorization request", http.StatusBadRequest)
		return
	}
	code := b64(random(16))
	p.mu.Lock()
	p.codes[code] = grant{nonce: q.Get("nonce"), challenge: q.Get("code_challenge"), redirect: q.Get("redirect_uri"), who: p.who}
	p.mu.Unlock()
	back, err := url.Parse(q.Get("redirect_uri"))
	if err != nil {
		http.Error(w, "bad redirect_uri", http.StatusBadRequest)
		return
	}
	v := back.Query()
	v.Set("code", code)
	v.Set("state", q.Get("state"))
	back.RawQuery = v.Encode()
	http.Redirect(w, r, back.String(), http.StatusFound)
}

// token spends a code, once, if its verifier matches its challenge.
func (p *Provider) token(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	id, secret, ok := r.BasicAuth()
	if !ok {
		id, secret = r.PostForm.Get("client_id"), r.PostForm.Get("client_secret")
	}
	code := r.PostForm.Get("code")
	p.mu.Lock()
	g, found := p.codes[code]
	delete(p.codes, code)
	p.mu.Unlock()
	sum := sha256.Sum256([]byte(r.PostForm.Get("code_verifier")))
	switch {
	case id != p.ClientID || secret != p.ClientSecret:
		w.WriteHeader(http.StatusUnauthorized)
		writeJSON(w, map[string]string{"error": "invalid_client"})
		return
	case !found || r.PostForm.Get("redirect_uri") != g.redirect || b64(sum[:]) != g.challenge:
		w.WriteHeader(http.StatusBadRequest)
		writeJSON(w, map[string]string{"error": "invalid_grant"})
		return
	}
	p.mu.Lock()
	p.Exchanged++
	p.mu.Unlock()
	now := time.Now()
	idToken := p.sign(map[string]any{
		"iss": p.Issuer(), "aud": p.ClientID, "sub": g.who.Subject, "nonce": g.nonce,
		"iat": now.Unix(), "exp": now.Add(5 * time.Minute).Unix(),
		"email": g.who.Email, "email_verified": g.who.EmailVerified, "name": g.who.Name,
	})
	writeJSON(w, map[string]any{"access_token": b64(random(16)), "token_type": "Bearer", "expires_in": 300, "id_token": idToken})
}

func (p *Provider) sign(claims map[string]any) string {
	head, _ := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT", "kid": "k1"})
	body, _ := json.Marshal(claims)
	signing := b64(head) + "." + b64(body)
	sum := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, p.key, crypto.SHA256, sum[:])
	if err != nil {
		panic(err)
	}
	return signing + "." + b64(sig)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func b64(b []byte) string { return strings.TrimRight(base64.URLEncoding.EncodeToString(b), "=") }

func random(n int) []byte {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return b
}
