package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/noderunner/bingle/internal/config"
)

// Signing in with an OpenID Connect provider (ADR 0070): any provider that
// speaks it — Google, Authentik, Keycloak, Entra — from one implementation,
// authorisation code with PKCE, a state bound to the browser and a nonce
// bound to the ID token.

// oidcFlowFor is how long a sign-in may take at the provider.
const oidcFlowFor = 10 * time.Minute

var (
	ErrUnknownProvider = errors.New("auth: there is no such sign-in provider here")
	ErrOIDCFailed      = errors.New("auth: the sign-in at the provider did not work; try again")
	ErrOIDCUnverified  = errors.New("auth: the provider does not vouch for that email address, so it cannot make an account here: sign up with a password, then link the provider from your account page")
	ErrOIDCAddressUsed = errors.New("auth: an account here has that email address: sign in with its password, then link the provider from your account page")
	ErrLinkedElsewhere = errors.New("auth: that sign-in is another account's already")
	ErrLastWayIn       = errors.New("auth: that is the only way you sign in: set a password first")
)

// OIDC is the configured providers and the sign-ins under way.
type OIDC struct {
	base  string // the public URL, which the providers send people back to
	order []string
	byID  map[string]*oidcProvider

	mu    sync.Mutex
	flows map[string]*oidcFlow // by the state's hash
	fresh map[string]*Identity // identities waiting for a handle, by token hash
	now   func() time.Time
}

type oidcProvider struct {
	cfg config.OIDCProvider
	mu  sync.Mutex
	// Discovered on first use, not at start-up: a provider that is down
	// does not stop bingle from starting.
	verifier *oidc.IDTokenVerifier
	oauth    *oauth2.Config
}

// oidcFlow is a sign-in that went to a provider and has not come back.
type oidcFlow struct {
	provider, verifier, nonce string
	returnTo                  string
	// link is the account to link the provider to (0: sign in instead).
	link  int64
	guest string
	until time.Time
}

// Identity is who a provider says signed in.
type Identity struct {
	Provider, Subject, Email, Name string
	EmailVerified                  bool
	guest                          string
	returnTo                       string
	until                          time.Time
}

// NewOIDC returns the providers of cfg (nil when there are none).
func NewOIDC(publicURL string, providers []config.OIDCProvider) *OIDC {
	if len(providers) == 0 {
		return nil
	}
	o := &OIDC{base: strings.TrimRight(publicURL, "/"), byID: map[string]*oidcProvider{}, flows: map[string]*oidcFlow{}, fresh: map[string]*Identity{}, now: time.Now}
	for _, p := range providers {
		o.order = append(o.order, p.ID)
		o.byID[p.ID] = &oidcProvider{cfg: p}
	}
	return o
}

// ProviderInfo is a provider as the sign-in page shows it.
type ProviderInfo struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// Providers lists the providers, in their configured order.
func (o *OIDC) Providers() []ProviderInfo {
	if o == nil {
		return nil
	}
	out := make([]ProviderInfo, 0, len(o.order))
	for _, id := range o.order {
		out = append(out, ProviderInfo{ID: id, Label: o.byID[id].cfg.Label})
	}
	return out
}

func (o *OIDC) redirectURL(id string) string { return o.base + "/auth/oidc/" + id + "/callback" }

// load discovers a provider, once it works.
func (o *OIDC) load(ctx context.Context, id string) (*oidcProvider, error) {
	p, ok := o.byID[id]
	if !ok {
		return nil, ErrUnknownProvider
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.verifier != nil {
		return p, nil
	}
	prov, err := oidc.NewProvider(ctx, p.cfg.Issuer)
	if err != nil {
		return nil, fmt.Errorf("auth: discovering %s: %w", id, err)
	}
	p.verifier = prov.Verifier(&oidc.Config{ClientID: p.cfg.ClientID})
	p.oauth = &oauth2.Config{ClientID: p.cfg.ClientID, ClientSecret: p.cfg.ClientSecret.Reveal(), Endpoint: prov.Endpoint(),
		RedirectURL: o.redirectURL(id), Scopes: p.cfg.Scopes}
	return p, nil
}

// Start begins a sign-in at a provider: the URL to send the browser to, and
// the state to keep in its cookie, which the callback must bring back.
func (o *OIDC) Start(ctx context.Context, id, returnTo string, link int64, guest string) (string, string, error) {
	p, err := o.load(ctx, id)
	if err != nil {
		return "", "", err
	}
	state, nonce, verifier := newSecret(), newSecret(), oauth2.GenerateVerifier()
	now := o.now()
	o.mu.Lock()
	for k, f := range o.flows {
		if now.After(f.until) {
			delete(o.flows, k)
		}
	}
	o.flows[hashSecret(state)] = &oidcFlow{provider: id, verifier: verifier, nonce: nonce, returnTo: returnTo, link: link, guest: guest, until: now.Add(oidcFlowFor)}
	o.mu.Unlock()
	url := p.oauth.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier), oidc.Nonce(nonce))
	return url, state, nil
}

// Finish takes the provider's answer: the state must be the one this
// browser was given (its cookie) and still waiting, which also spends it,
// and the code is exchanged for an ID token that names this flow's nonce.
func (o *OIDC) Finish(ctx context.Context, id, state, cookieState, code string) (*Identity, *oidcFlow, error) {
	if state == "" || state != cookieState {
		return nil, nil, ErrOIDCFailed
	}
	o.mu.Lock()
	f, ok := o.flows[hashSecret(state)]
	delete(o.flows, hashSecret(state))
	o.mu.Unlock()
	if !ok || o.now().After(f.until) || f.provider != id {
		return nil, nil, ErrOIDCFailed
	}
	p, err := o.load(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	tok, err := p.oauth.Exchange(ctx, code, oauth2.VerifierOption(f.verifier))
	if err != nil {
		return nil, nil, fmt.Errorf("%w (%v)", ErrOIDCFailed, err)
	}
	raw, _ := tok.Extra("id_token").(string)
	if raw == "" {
		return nil, nil, ErrOIDCFailed
	}
	idt, err := p.verifier.Verify(ctx, raw)
	if err != nil || idt.Nonce != f.nonce {
		return nil, nil, ErrOIDCFailed
	}
	var claims struct {
		Email         string `json:"email"`
		EmailVerified any    `json:"email_verified"`
		Name          string `json:"name"`
	}
	if err := idt.Claims(&claims); err != nil {
		return nil, nil, ErrOIDCFailed
	}
	// Some providers send the flag as a string.
	verified := claims.EmailVerified == true || claims.EmailVerified == "true"
	return &Identity{Provider: id, Subject: idt.Subject, Email: claims.Email, Name: claims.Name, EmailVerified: verified, guest: f.guest, returnTo: f.returnTo}, f, nil
}

// waitForHandle keeps an identity that will become a new account once its
// person chooses a handle, and returns the token for that.
func (o *OIDC) waitForHandle(id *Identity) string {
	now := o.now()
	o.mu.Lock()
	defer o.mu.Unlock()
	for k, v := range o.fresh {
		if now.After(v.until) {
			delete(o.fresh, k)
		}
	}
	token := newSecret()
	id.until = now.Add(oidcFlowFor)
	o.fresh[hashSecret(token)] = id
	return token
}

// takeFresh is the identity waiting under token, which it spends.
func (o *OIDC) takeFresh(token string) (*Identity, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	id, ok := o.fresh[hashSecret(token)]
	delete(o.fresh, hashSecret(token))
	if !ok || o.now().After(id.until) {
		return nil, false
	}
	return id, true
}
