package auth

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/noderunner/bingle/internal/store"
	"github.com/noderunner/bingle/internal/transport"
)

// CookieName is the cookie that carries a browser's guest key.
const CookieName = "bingle_guest"

// SessionCookie is the cookie of a signed-in browser.
const SessionCookie = "bingle_session"

// PageHeader marks a request from bingle's own pages. A state-changing
// request proved by a cookie must carry it (a cross-site form or script can
// send the cookie, but not a custom header), and a page's first such request
// is the one that makes its guest.
const PageHeader = "X-Bingle-Session"

// cookieMaxAge is how long a browser keeps the cookie: the most browsers
// allow. When a key stops working is the server's decision (Idle), not the
// cookie's.
const cookieMaxAge = 400 * 24 * time.Hour

// touchEvery is how often a key in use has its last use written down.
const touchEvery = time.Hour

// Guests is what the resolver needs of the store.
type Guests interface {
	CreateGuest(ctx context.Context, id, keyHash string) error
	GuestByKey(ctx context.Context, keyHash string, idleSince time.Time, touchEvery time.Duration, now time.Time) (string, error)
}

// Resolver resolves the principal of each request.
type Resolver struct {
	guests    Guests
	accounts  *Accounts
	oidc      *OIDC
	caps      *Caps
	transport transport.Policy
	idle      time.Duration
	log       *slog.Logger
	now       func() time.Time
	made      windowLimiter
}

// Options configures a Resolver.
type Options struct {
	// Transport says whether a request is secure (the cookie's Secure flag)
	// and where it comes from (the limit on new guests per address).
	Transport transport.Policy
	// Idle is how long a guest key may go unused before it stops working
	// (BINGLE_GUEST_IDLE); zero means DefaultIdle.
	Idle time.Duration
	// Log records failures to read the store; nil means slog's default.
	Log *slog.Logger
	// NewGuestsPerHour caps the guests one address may make; zero means
	// DefaultNewGuestsPerHour.
	NewGuestsPerHour int
	// Accounts, when set, signs people in: the session cookie is read before
	// any guest key.
	Accounts *Accounts
	// OIDC are the sign-in providers (nil for none).
	OIDC *OIDC
	// Caps, when set, says who may do what, and the mode.
	Caps *Caps
}

// DefaultIdle is BINGLE_GUEST_IDLE's default.
const DefaultIdle = 90 * 24 * time.Hour

// DefaultNewGuestsPerHour is how many guests one address may make an hour.
// A household behind one address makes a handful; a script making thousands
// is filling the database.
const DefaultNewGuestsPerHour = 60

// NewResolver returns a Resolver over g.
func NewResolver(g Guests, o Options) *Resolver {
	if o.Idle <= 0 {
		o.Idle = DefaultIdle
	}
	if o.NewGuestsPerHour <= 0 {
		o.NewGuestsPerHour = DefaultNewGuestsPerHour
	}
	if o.Log == nil {
		o.Log = slog.Default()
	}
	return &Resolver{guests: g, accounts: o.Accounts, oidc: o.OIDC, caps: o.Caps, transport: o.Transport, idle: o.Idle, log: o.Log, now: time.Now, made: windowLimiter{per: o.NewGuestsPerHour, window: time.Hour}}
}

// SetClock replaces the resolver's clock (tests).
func (rs *Resolver) SetClock(now func() time.Time) { rs.now = now }

// Middleware resolves the request's principal and carries it in the
// request's context:
//
//   - a bearer guest key, or else the guest cookie, is looked up; a bearer key
//     that does not work is refused, a cookie that does not work is cleared;
//   - a state-changing request proved by the cookie must carry PageHeader;
//   - a state-changing request from a page that has no guest yet makes one,
//     and the response sets its cookie;
//   - anything else is anonymous.
func (rs *Resolver) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok, err := rs.resolve(w, r)
		if err != nil {
			return
		}
		unsafe := r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions
		fromPage := r.Header.Get(PageHeader) != ""
		if ok && p.Cookie && unsafe && !fromPage {
			writeError(w, http.StatusForbidden, "auth: a request from a browser must come from bingle's own page (the "+PageHeader+" header is missing)")
			return
		}
		// An account whose password must be changed may change it, sign out,
		// and look; nothing else. Nor may an administrator who has no second
		// factor yet, off this machine, until one is set up. A token is
		// neither: it never uses the password, and never the admin group's
		// powers.
		if ok && p.Account != nil && p.Scopes == nil && unsafe && !strings.HasPrefix(r.URL.Path, "/auth/") {
			switch {
			case p.Account.MustChangePassword:
				writeError(w, http.StatusForbidden, ErrMustChangePwd.Error())
				return
			case rs.needsSecondFactor(r, *p.Account):
				writeError(w, http.StatusForbidden, ErrAdminNeedsTOTP.Error())
				return
			}
		}
		if !ok && unsafe && fromPage && (rs.caps == nil || rs.caps.Mode() != ModeClosed) {
			if p, ok = rs.newGuest(w, r); ok {
				p.Cookie = true
			}
		}
		if rs.caps != nil && rs.caps.check(w, r, p, ok, rs.transport.PeerIP(r), rs.now()) {
			return
		}
		if ok {
			r = r.WithContext(WithPrincipal(r.Context(), p))
		}
		next.ServeHTTP(w, r)
	})
}

// resolve reads the request's session cookie, or else its guest key, if it
// has one. It reports an error when it has already answered the request.
func (rs *Resolver) resolve(w http.ResponseWriter, r *http.Request) (Principal, bool, error) {
	if c, err := r.Cookie(SessionCookie); err == nil && c.Value != "" && rs.accounts != nil {
		acc, err := rs.accounts.account(r.Context(), c.Value)
		switch {
		case err == nil:
			return Principal{ID: acc.PrincipalID, Kind: store.KindAccount, Cookie: true, Account: &acc, session: c.Value}, true, nil
		case errors.Is(err, store.ErrNotFound):
			// Signed out elsewhere, or too old: the browser is not signed in.
			rs.setCookie(w, r, SessionCookie, "", -1)
		default:
			rs.log.Error("reading a session", "err", err)
			writeError(w, http.StatusServiceUnavailable, "auth: could not check who you are; try again")
			return Principal{}, false, err
		}
	}
	if tok := bearerToken(r); tok != "" {
		if rs.accounts == nil {
			writeError(w, http.StatusUnauthorized, ErrNoToken.Error())
			return Principal{}, false, ErrNoToken
		}
		acc, scopes, err := rs.accounts.tokenAccount(r.Context(), tok)
		switch {
		case err == nil:
			if scopes == nil {
				scopes = []string{}
			}
			return Principal{ID: acc.PrincipalID, Kind: store.KindAccount, Account: &acc, Scopes: scopes}, true, nil
		case errors.Is(err, store.ErrNotFound):
			writeError(w, http.StatusUnauthorized, ErrNoToken.Error())
			return Principal{}, false, err
		default:
			rs.log.Error("reading an API token", "err", err)
			writeError(w, http.StatusServiceUnavailable, "auth: could not check who you are; try again")
			return Principal{}, false, err
		}
	}
	key, fromCookie := bearerKey(r), false
	if key == "" {
		if c, err := r.Cookie(CookieName); err == nil && c.Value != "" {
			key, fromCookie = c.Value, true
		}
	}
	if key == "" {
		return Principal{}, false, nil
	}
	now := rs.now()
	id, err := rs.guests.GuestByKey(r.Context(), HashKey(key), now.Add(-rs.idle), touchEvery, now)
	switch {
	case err == nil:
		return Principal{ID: id, Kind: store.KindGuest, Cookie: fromCookie}, true, nil
	case errors.Is(err, store.ErrNotFound) && fromCookie:
		// Revoked, idle too long, or from another database: the browser is
		// a newcomer again.
		rs.clearCookie(w, r)
		return Principal{}, false, nil
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusUnauthorized, "auth: that guest key is not one this server knows (it may have been replaced, or gone unused too long)")
		return Principal{}, false, err
	default:
		rs.log.Error("reading a guest key", "err", err)
		writeError(w, http.StatusServiceUnavailable, "auth: could not check who you are; try again")
		return Principal{}, false, err
	}
}

// newGuest makes a guest for this browser and sets its cookie. Past the limit
// for the address the request goes on anonymous, as every request did before
// there were guests.
func (rs *Resolver) newGuest(w http.ResponseWriter, r *http.Request) (Principal, bool) {
	key, id, err := rs.issue(r)
	if err != nil {
		return Principal{}, false
	}
	rs.setCookie(w, r, CookieName, key, int(cookieMaxAge/time.Second))
	return Principal{ID: id, Kind: store.KindGuest}, true
}

// errTooMany is issue's answer past the limit for an address.
var errTooMany = errors.New("auth: too many new guests from this address; try again later")

// issue makes a guest and returns its key and ID.
func (rs *Resolver) issue(r *http.Request) (string, string, error) {
	if !rs.made.allow(rs.transport.PeerIP(r), rs.now()) {
		return "", "", errTooMany
	}
	key, id := NewGuestKey(), NewPrincipalID()
	if err := rs.guests.CreateGuest(r.Context(), id, HashKey(key)); err != nil {
		rs.log.Error("making a guest", "err", err)
		return "", "", err
	}
	return key, id, nil
}

// HandleNewGuest is POST /api/guest: a new guest for a script, whose key is
// in the answer, {"key": "bgg_…"}, to send as "Authorization: Bearer". A
// browser never needs it: its first request makes its guest.
func (rs *Resolver) HandleNewGuest(w http.ResponseWriter, r *http.Request) {
	key, _, err := rs.issue(r)
	switch {
	case errors.Is(err, errTooMany):
		writeError(w, http.StatusTooManyRequests, err.Error())
		return
	case err != nil:
		writeError(w, http.StatusServiceUnavailable, "auth: could not make a guest; try again")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]string{"key": key})
}

func (rs *Resolver) clearCookie(w http.ResponseWriter, r *http.Request) {
	rs.setCookie(w, r, CookieName, "", -1)
}

// setCookie sets one of bingle's cookies: HttpOnly, SameSite=Lax, for the
// whole site, and Secure unless the connection is not (which only a loopback
// one may be).
func (rs *Resolver) setCookie(w http.ResponseWriter, r *http.Request, name, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: value, Path: "/", MaxAge: maxAge, HttpOnly: true, Secure: rs.transport.Secure(r), SameSite: http.SameSiteLaxMode})
}

// bearerKey is the guest key in an Authorization header, if that is what it
// holds.
func bearerKey(r *http.Request) string {
	v := r.Header.Get("Authorization")
	if len(v) < len("Bearer ") || !strings.EqualFold(v[:len("Bearer ")], "Bearer ") {
		return ""
	}
	if k := strings.TrimSpace(v[len("Bearer "):]); strings.HasPrefix(k, GuestKeyPrefix) {
		return k
	}
	return ""
}

// bearerToken is the API token in an Authorization header, if that is what
// it holds.
func bearerToken(r *http.Request) string {
	v := r.Header.Get("Authorization")
	if len(v) < len("Bearer ") || !strings.EqualFold(v[:len("Bearer ")], "Bearer ") {
		return ""
	}
	if k := strings.TrimSpace(v[len("Bearer "):]); strings.HasPrefix(k, APITokenPrefix) {
		return k
	}
	return ""
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// windowLimiter allows per events for each key in any window.
type windowLimiter struct {
	per    int
	window time.Duration
	mu     sync.Mutex
	at     map[string][]time.Time
}

func (l *windowLimiter) allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.at == nil {
		l.at = map[string][]time.Time{}
	}
	cut := now.Add(-l.window)
	kept := l.at[key][:0]
	for _, t := range l.at[key] {
		if t.After(cut) {
			kept = append(kept, t)
		}
	}
	if len(kept) >= l.per {
		l.at[key] = kept
		return false
	}
	l.at[key] = append(kept, now)
	// A key that has gone quiet is forgotten, so the map does not grow with
	// every address ever seen.
	if len(l.at) > 10000 {
		for k, ts := range l.at {
			if len(ts) == 0 || !ts[len(ts)-1].After(cut) {
				delete(l.at, k)
			}
		}
	}
	return true
}

// pagesOpen are the pages a closed server shows to everybody: signing in
// and up, and what their links lead to.
var pagesOpen = []string{"/login", "/signup", "/forgot", "/account/confirm", "/account/reset", "/static/", "/favicon"}

// ClosedPages is the page side of closed mode: whoever is not signed in is
// sent to sign in (and back afterwards). On a hybrid server it does nothing.
func (rs *Resolver) ClosedPages(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if rs.caps == nil || rs.caps.Mode() != ModeClosed || rs.accounts == nil {
			next.ServeHTTP(w, r)
			return
		}
		for _, p := range pagesOpen {
			if strings.HasPrefix(r.URL.Path, p) {
				next.ServeHTTP(w, r)
				return
			}
		}
		if c, err := r.Cookie(SessionCookie); err == nil && c.Value != "" {
			if _, err := rs.accounts.account(r.Context(), c.Value); err == nil {
				next.ServeHTTP(w, r)
				return
			}
		}
		http.Redirect(w, r, "/login?return="+url.QueryEscape(r.URL.RequestURI()), http.StatusSeeOther)
	})
}
