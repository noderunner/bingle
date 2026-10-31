// Package auth says who a request comes from (ADR 0067, and
// docs/history/auth-and-accounts-plan.md): a principal, resolved once by
// middleware and carried in the request's context, so that the session layer
// asks "which principal?" and never "which name?".
//
// A principal is an account, proved by the bingle_session cookie of a
// signed-in browser (ADR 0068), or a guest, proved by a guest key: the
// bingle_guest cookie of a browser, or "Authorization: Bearer bgg_…" from a
// script. A request with none of them is anonymous, and keeps what an
// anonymous request has always had: seats nobody owns, acted for by name.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"strings"

	"github.com/noderunner/bingle/internal/store"
)

// Principal is whoever a request acts for.
type Principal struct {
	// ID is the server's name for the principal (principals.id). It is not a
	// secret, but it is never shown in the interface either.
	ID string
	// Kind is store.KindGuest (or, later, store.KindAccount).
	Kind string
	// Cookie reports that the principal was proved by a cookie, which a
	// browser sends by itself, and so the cross-site rules apply to it.
	Cookie bool
	// Account is the signed-in account (nil for a guest).
	Account *store.Account
	// session is the session cookie's value, for signing out and for a
	// password change, which replaces it.
	session string
}

type ctxKey struct{}

// WithPrincipal returns ctx carrying p.
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, ctxKey{}, p)
}

// From returns the principal ctx carries, if any.
func From(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(ctxKey{}).(Principal)
	return p, ok && p.ID != ""
}

// IDFrom is the ID of the principal ctx carries, or "" for an anonymous
// request.
func IDFrom(ctx context.Context) string {
	p, _ := From(ctx)
	return p.ID
}

// GuestKeyPrefix starts every guest key, so one is recognisable in a bearer
// header (and in a secret scanner's rules).
const GuestKeyPrefix = "bgg_"

// NewGuestKey returns a fresh guest key: the prefix and 32 random bytes.
func NewGuestKey() string {
	return GuestKeyPrefix + base64.RawURLEncoding.EncodeToString(randomBytes(32))
}

// HashKey is how a guest key is stored. A key is 256 random bits, so a plain
// SHA-256 is as good as a slow hash would be, and much cheaper on every
// request.
func HashKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

// NewPrincipalID returns a fresh principal ID: "p_" and 16 random bytes in
// lower-case base32.
func NewPrincipalID() string {
	return "p_" + strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(randomBytes(16)))
}

func randomBytes(n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic("auth: no randomness: " + err.Error())
	}
	return b
}
