package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/noderunner/bingle/internal/mail"
	"github.com/noderunner/bingle/internal/store"
)

// Routes is /auth: signing up, in and out (ADR 0068). Mount it behind
// Middleware, which says who is asking.
//
//	POST /auth/signup   {handle, email, password}  202: check your mail
//	POST /auth/confirm  {token}                    the account is usable
//	POST /auth/login    {handle, password}         sets the session cookie
//	POST /auth/logout                               ends this session
//	POST /auth/password {old, new}                  a new password, a new cookie
//	GET  /auth/me                                   who this browser is
//	POST /auth/second   {code | recovery_code}      the second step of a sign-in
//	POST /auth/totp/setup   {password}               a secret for an app
//	POST /auth/totp/enable  {code}                   on, with ten recovery codes
//	POST /auth/totp/disable {password, code | recovery_code}
//	POST /auth/recovery {password}                  ten new recovery codes
//	POST /auth/forgot   {who}                       mail a reset link
//	POST /auth/reset    {token, password, code?}    a new password from the link
//	GET  /auth/oidc/{p}/start?return=&link=1        off to a provider (ADR 0070)
//	GET  /auth/oidc/{p}/callback                    and back
//	POST /auth/oidc/finish {handle}                 a new account a provider vouched for
//	POST /auth/oidc/unlink {provider, password}     stop signing in with one
//	GET|POST /auth/tokens, DELETE /auth/tokens/{id} API tokens (ADR 0071)
//	GET|PUT  /auth/admin/caps                       the capability matrix
//	POST /auth/admin/invites                        an invitation to sign up
func (rs *Resolver) Routes() http.Handler {
	r := chi.NewRouter()
	r.Post("/signup", rs.handleSignup)
	r.Post("/confirm", rs.handleConfirm)
	r.Post("/login", rs.handleLogin)
	r.Post("/logout", rs.handleLogout)
	r.Post("/password", rs.handlePassword)
	r.Get("/me", rs.handleMe)
	r.Post("/second", rs.handleSecond)
	r.Post("/totp/setup", rs.handleTOTPSetup)
	r.Post("/totp/enable", rs.handleTOTPEnable)
	r.Post("/totp/disable", rs.handleTOTPDisable)
	r.Post("/recovery", rs.handleRecovery)
	r.Post("/forgot", rs.handleForgot)
	r.Post("/reset", rs.handleReset)
	r.Get("/oidc/{provider}/start", rs.handleOIDCStart)
	r.Get("/oidc/{provider}/callback", rs.handleOIDCCallback)
	r.Post("/oidc/finish", rs.handleOIDCFinish)
	r.Post("/oidc/unlink", rs.handleOIDCUnlink)
	r.Get("/tokens", rs.handleTokens)
	r.Post("/tokens", rs.handleNewToken)
	r.Delete("/tokens/{id}", rs.handleRevokeToken)
	r.Get("/admin/caps", rs.handleAdminCaps)
	r.Put("/admin/caps", rs.handleAdminSetCap)
	r.Post("/admin/invites", rs.handleAdminInvite)
	r.Get("/admin/events", rs.handleAdminEvents)
	r.Post("/handle", rs.handleRename)
	r.Get("/sessions", rs.handleSessions)
	r.Post("/sessions/end-others", rs.handleEndOthers)
	return r
}

const maxAuthBody = 4 << 10

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxAuthBody)).Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "auth: malformed JSON body")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// statusOf maps the service's answers to HTTP.
func statusOf(err error) int {
	switch {
	case errors.Is(err, ErrBadLogin), errors.Is(err, ErrNotSignedIn), errors.Is(err, ErrBadCode), errors.Is(err, ErrSignInAgain), errors.Is(err, ErrNeedsCode):
		return http.StatusUnauthorized
	case errors.Is(err, ErrSignupClosed), errors.Is(err, ErrSignupNoMail), errors.Is(err, ErrNotConfirmed), errors.Is(err, ErrMustChangePwd),
		errors.Is(err, ErrNeedsInvite), errors.Is(err, ErrNotAdmin), errors.Is(err, ErrNotYourContact):
		return http.StatusForbidden
	case errors.Is(err, ErrHandleTaken), errors.Is(err, ErrLinkedElsewhere), errors.Is(err, ErrOIDCAddressUsed), errors.Is(err, ErrAddressUsed):
		return http.StatusConflict
	case errors.Is(err, ErrUnknownProvider):
		return http.StatusNotFound
	case errors.Is(err, ErrTooManyTries), errors.Is(err, ErrMailLater):
		return http.StatusTooManyRequests
	case errors.Is(err, ErrBadHandle), errors.Is(err, ErrReservedHandle), errors.Is(err, ErrWeakPassword), errors.Is(err, ErrBadLink),
		errors.Is(err, ErrSamePassword), errors.Is(err, ErrNoSetup), errors.Is(err, ErrNoSecondFactor), errors.Is(err, ErrOIDCUnverified),
		errors.Is(err, ErrLastWayIn), errors.Is(err, ErrOIDCFailed), errors.Is(err, ErrBadScopes), errors.Is(err, ErrBadName), mail.IsAddressError(err):
		return http.StatusBadRequest
	}
	return http.StatusInternalServerError
}

func (rs *Resolver) fail(w http.ResponseWriter, err error) {
	status := statusOf(err)
	msg := err.Error()
	if status == http.StatusInternalServerError {
		rs.log.Error("an account request failed", "err", err)
		msg = "auth: something went wrong; try again"
	}
	writeError(w, status, msg)
}

func (rs *Resolver) noAccounts(w http.ResponseWriter) bool {
	if rs.accounts == nil {
		writeError(w, http.StatusNotFound, "auth: this server has no accounts")
		return true
	}
	return false
}

// guestOf is the guest the request comes from, if it is one.
func guestOf(r *http.Request) string {
	if p, ok := From(r.Context()); ok && p.Account == nil {
		return p.ID
	}
	return ""
}

func (rs *Resolver) handleSignup(w http.ResponseWriter, r *http.Request) {
	if rs.noAccounts(w) {
		return
	}
	var req struct {
		Handle   string `json:"handle"`
		Email    string `json:"email"`
		Password string `json:"password"`
		Invite   string `json:"invite"`
		// A confirmed contact of this browser, instead of an address: it
		// becomes the account at once (ADR 0073).
		ContactID     int64  `json:"contact_id"`
		ContactSecret string `json:"contact_secret"`
	}
	if !decode(w, r, &req) {
		return
	}
	if req.ContactID != 0 {
		s, err := rs.accounts.SignupWithContact(r.Context(), req.ContactID, req.ContactSecret, req.Handle, req.Password, guestOf(r), req.Invite, rs.transport.PeerIP(r), r.UserAgent())
		if err != nil {
			rs.fail(w, err)
			return
		}
		rs.startSession(w, r, s)
		writeJSON(w, http.StatusOK, rs.meOf(r, s.Account))
		return
	}
	if err := rs.accounts.Signup(r.Context(), req.Handle, req.Email, req.Password, guestOf(r), req.Invite, rs.transport.PeerIP(r)); err != nil {
		rs.fail(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "Check your mail: Bingle sent a link to confirm the address."})
}

func (rs *Resolver) handleConfirm(w http.ResponseWriter, r *http.Request) {
	if rs.noAccounts(w) {
		return
	}
	var req struct {
		Token string `json:"token"`
	}
	if !decode(w, r, &req) {
		return
	}
	acc, err := rs.accounts.Confirm(r.Context(), req.Token, rs.transport.PeerIP(r))
	if err != nil {
		rs.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"handle": acc.Handle})
}

func (rs *Resolver) handleLogin(w http.ResponseWriter, r *http.Request) {
	if rs.noAccounts(w) {
		return
	}
	var req struct {
		Handle   string `json:"handle"`
		Password string `json:"password"`
	}
	if !decode(w, r, &req) {
		return
	}
	s, pending, err := rs.accounts.Login(r.Context(), req.Handle, req.Password, guestOf(r), rs.transport.PeerIP(r), r.UserAgent())
	if err != nil {
		rs.fail(w, err)
		return
	}
	if pending != "" {
		// The password was right; the code comes next, from this browser.
		rs.setCookie(w, r, loginCookie, pending, int(pendingFor/time.Second))
		writeJSON(w, http.StatusOK, map[string]bool{"second_factor": true})
		return
	}
	rs.startSession(w, r, s)
	writeJSON(w, http.StatusOK, rs.meOf(r, s.Account))
}

// startSession sets the session cookie, and drops the guest cookie: the
// guest is the account's now.
func (rs *Resolver) startSession(w http.ResponseWriter, r *http.Request, s Session) {
	rs.setCookie(w, r, SessionCookie, s.Token, int(time.Until(s.Expires)/time.Second))
	if c, err := r.Cookie(CookieName); err == nil && c.Value != "" {
		rs.clearCookie(w, r)
	}
}

func (rs *Resolver) handleLogout(w http.ResponseWriter, r *http.Request) {
	if rs.noAccounts(w) {
		return
	}
	p, ok := From(r.Context())
	if !ok || p.Account == nil {
		rs.setCookie(w, r, SessionCookie, "", -1)
		writeJSON(w, http.StatusOK, map[string]string{"kind": "guest"})
		return
	}
	if err := rs.accounts.Logout(r.Context(), p.session, p.Account.ID, rs.transport.PeerIP(r)); err != nil {
		rs.fail(w, err)
		return
	}
	rs.setCookie(w, r, SessionCookie, "", -1)
	writeJSON(w, http.StatusOK, map[string]string{"kind": "anonymous"})
}

func (rs *Resolver) handlePassword(w http.ResponseWriter, r *http.Request) {
	if rs.noAccounts(w) {
		return
	}
	p, ok := From(r.Context())
	if !ok || p.Account == nil {
		rs.fail(w, ErrNotSignedIn)
		return
	}
	var req struct {
		Old string `json:"old"`
		New string `json:"new"`
	}
	if !decode(w, r, &req) {
		return
	}
	s, err := rs.accounts.ChangePassword(r.Context(), *p.Account, req.Old, req.New, rs.transport.PeerIP(r), r.UserAgent())
	if err != nil {
		rs.fail(w, err)
		return
	}
	rs.startSession(w, r, s)
	writeJSON(w, http.StatusOK, rs.meOf(r, s.Account))
}

// me is GET /auth/me's answer.
type me struct {
	Kind               string `json:"kind"` // "account", "guest" or "anonymous"
	Handle             string `json:"handle,omitempty"`
	DisplayName        string `json:"display_name,omitempty"`
	MustChangePassword bool   `json:"must_change_password,omitempty"`
	// SecondFactor says the account signs in with a code too, and
	// RecoveryCodesLeft how many recovery codes it has unused.
	SecondFactor      bool `json:"second_factor,omitempty"`
	RecoveryCodesLeft int  `json:"recovery_codes_left,omitempty"`
	// NeedsSecondFactor: an administrator without one, off this machine,
	// who may do nothing else until it is set up.
	NeedsSecondFactor bool `json:"needs_second_factor,omitempty"`
	// Accounts says whether this server has accounts at all, and Signup
	// whether a person can make one here.
	Accounts bool `json:"accounts"`
	Signup   bool `json:"signup"`
	// Providers are the sign-in providers; Linked those the account signs
	// in with, and Password whether it has one (a provider's account may
	// not).
	Providers []ProviderInfo `json:"providers,omitempty"`
	Linked    []string       `json:"linked,omitempty"`
	Password  bool           `json:"password,omitempty"`
	// Admin: the account is in the admin group (and may use its powers from
	// here). Mode is "hybrid" or "closed"; SignupMode "open", "invite" or
	// "closed"; Caps what this browser may do.
	Admin bool `json:"admin,omitempty"`
	// Email is the account's own address (never another's).
	Email      string   `json:"email,omitempty"`
	Mode       string   `json:"mode,omitempty"`
	SignupMode string   `json:"signup_mode,omitempty"`
	Caps       []string `json:"caps,omitempty"`
}

func (rs *Resolver) meOf(r *http.Request, a store.Account) me {
	out := me{Kind: store.KindAccount, Handle: a.Handle, DisplayName: a.DisplayName, MustChangePassword: a.MustChangePassword, Accounts: true, Email: a.Email,
		SecondFactor: a.HasTOTP, NeedsSecondFactor: rs.needsSecondFactor(r, a)}
	if rs.accounts != nil {
		if a.HasTOTP {
			out.RecoveryCodesLeft = rs.accounts.RecoveryCodesLeft(r.Context(), a)
		}
		out.Linked = rs.accounts.Linked(r.Context(), a)
		out.Password = rs.accounts.HasPassword(r.Context(), a)
	}
	out.Providers = rs.oidc.Providers()
	out.Signup = rs.signupOpen()
	out.Admin = a.Admin && !out.NeedsSecondFactor
	rs.decorate(r, &out)
	return out
}

// needsSecondFactor: the admin group signs in with a second factor, except
// on a genuine loopback connection (never behind a trusted proxy, where
// every request looks local).
func (rs *Resolver) needsSecondFactor(r *http.Request, a store.Account) bool {
	return a.Admin && !a.HasTOTP && !rs.transport.Loopback(r)
}

func (rs *Resolver) handleMe(w http.ResponseWriter, r *http.Request) {
	out := me{Kind: "anonymous"}
	if p, ok := From(r.Context()); ok {
		if p.Account != nil {
			out = rs.meOf(r, *p.Account)
		} else {
			out.Kind = store.KindGuest
		}
	}
	if rs.accounts != nil {
		out.Accounts = true
		out.Signup = rs.signupOpen()
		out.Providers = rs.oidc.Providers()
	}
	if out.Kind != store.KindAccount {
		rs.decorate(r, &out)
	}
	writeJSON(w, http.StatusOK, out)
}

// account is the signed-in account of the request, or an answer saying it
// is not signed in.
func (rs *Resolver) account(w http.ResponseWriter, r *http.Request) (*store.Account, bool) {
	if rs.noAccounts(w) {
		return nil, false
	}
	p, ok := From(r.Context())
	if !ok || p.Account == nil {
		rs.fail(w, ErrNotSignedIn)
		return nil, false
	}
	return p.Account, true
}

type codeRequest struct {
	Password string `json:"password"`
	Code     string `json:"code"`
	Recovery string `json:"recovery_code"`
}

func (rs *Resolver) handleSecond(w http.ResponseWriter, r *http.Request) {
	if rs.noAccounts(w) {
		return
	}
	var req codeRequest
	if !decode(w, r, &req) {
		return
	}
	c, err := r.Cookie(loginCookie)
	if err != nil || c.Value == "" {
		rs.fail(w, ErrSignInAgain)
		return
	}
	s, err := rs.accounts.SecondFactor(r.Context(), c.Value, req.Code, req.Recovery, rs.transport.PeerIP(r), r.UserAgent())
	if err != nil {
		if errors.Is(err, ErrSignInAgain) {
			rs.setCookie(w, r, loginCookie, "", -1)
		}
		rs.fail(w, err)
		return
	}
	rs.setCookie(w, r, loginCookie, "", -1)
	rs.startSession(w, r, s)
	writeJSON(w, http.StatusOK, rs.meOf(r, s.Account))
}

func (rs *Resolver) handleTOTPSetup(w http.ResponseWriter, r *http.Request) {
	acc, ok := rs.account(w, r)
	if !ok {
		return
	}
	var req codeRequest
	if !decode(w, r, &req) {
		return
	}
	secret, uri, err := rs.accounts.SetupTOTP(r.Context(), *acc, req.Password, rs.transport.PeerIP(r))
	if err != nil {
		rs.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"secret": secret, "uri": uri})
}

func (rs *Resolver) handleTOTPEnable(w http.ResponseWriter, r *http.Request) {
	acc, ok := rs.account(w, r)
	if !ok {
		return
	}
	var req codeRequest
	if !decode(w, r, &req) {
		return
	}
	codes, s, err := rs.accounts.EnableTOTP(r.Context(), *acc, req.Code, rs.transport.PeerIP(r), r.UserAgent())
	if err != nil {
		rs.fail(w, err)
		return
	}
	rs.startSession(w, r, s)
	writeJSON(w, http.StatusOK, map[string]any{"recovery_codes": codes, "me": rs.meOf(r, s.Account)})
}

func (rs *Resolver) handleTOTPDisable(w http.ResponseWriter, r *http.Request) {
	acc, ok := rs.account(w, r)
	if !ok {
		return
	}
	var req codeRequest
	if !decode(w, r, &req) {
		return
	}
	s, err := rs.accounts.DisableTOTP(r.Context(), *acc, req.Password, req.Code, req.Recovery, rs.transport.PeerIP(r), r.UserAgent())
	if err != nil {
		rs.fail(w, err)
		return
	}
	rs.startSession(w, r, s)
	writeJSON(w, http.StatusOK, rs.meOf(r, s.Account))
}

func (rs *Resolver) handleRecovery(w http.ResponseWriter, r *http.Request) {
	acc, ok := rs.account(w, r)
	if !ok {
		return
	}
	var req codeRequest
	if !decode(w, r, &req) {
		return
	}
	codes, err := rs.accounts.NewRecoveryCodes(r.Context(), *acc, req.Password, rs.transport.PeerIP(r))
	if err != nil {
		rs.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"recovery_codes": codes})
}

func (rs *Resolver) handleForgot(w http.ResponseWriter, r *http.Request) {
	if rs.noAccounts(w) {
		return
	}
	var req struct {
		Who string `json:"who"`
	}
	if !decode(w, r, &req) {
		return
	}
	if err := rs.accounts.ForgotPassword(r.Context(), req.Who, rs.transport.PeerIP(r)); err != nil {
		rs.fail(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "If there is an account by that name or address, a link to reset its password is in its mail."})
}

func (rs *Resolver) handleReset(w http.ResponseWriter, r *http.Request) {
	if rs.noAccounts(w) {
		return
	}
	var req struct {
		Token    string `json:"token"`
		Password string `json:"password"`
		Code     string `json:"code"`
		Recovery string `json:"recovery_code"`
	}
	if !decode(w, r, &req) {
		return
	}
	if err := rs.accounts.ResetWithLink(r.Context(), req.Token, req.Password, req.Code, req.Recovery, rs.transport.PeerIP(r)); err != nil {
		rs.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "Your password is changed. Sign in with it."})
}

// signupOpen: a person can make an account here, with a password (which
// needs mail) or with a provider (which vouches for the address itself).
func (rs *Resolver) signupOpen() bool {
	a := rs.accounts
	return a != nil && a.signup != SignupClosed && (a.mail != nil && a.mail.MailEnabled() || rs.oidc != nil)
}

// decorate adds the mode and the capabilities to an answer of /auth/me.
func (rs *Resolver) decorate(r *http.Request, out *me) {
	if rs.accounts != nil {
		out.SignupMode = rs.accounts.signup
	}
	if rs.caps == nil {
		return
	}
	out.Mode = rs.caps.Mode()
	p, ok := From(r.Context())
	out.Caps = rs.caps.Has(p, ok)
}

// ErrNotAdmin is the answer to an admin request from anybody else (or from
// an API token, which never carries the admin group's powers).
var ErrNotAdmin = errors.New("auth: that is for the server's administrators")

// admin is the request's administrator, or an answer saying it is none.
func (rs *Resolver) admin(w http.ResponseWriter, r *http.Request) (*store.Account, bool) {
	acc, ok := rs.account(w, r)
	if !ok {
		return nil, false
	}
	p, _ := From(r.Context())
	if !acc.Admin || p.Scopes != nil || rs.needsSecondFactor(r, *acc) || rs.caps == nil {
		rs.fail(w, ErrNotAdmin)
		return nil, false
	}
	return acc, true
}

// tokenOwner is the signed-in account managing its tokens; a token cannot
// make or end tokens.
func (rs *Resolver) tokenOwner(w http.ResponseWriter, r *http.Request) (*store.Account, bool) {
	acc, ok := rs.account(w, r)
	if !ok {
		return nil, false
	}
	if p, _ := From(r.Context()); p.Scopes != nil {
		rs.fail(w, ErrNotSignedIn)
		return nil, false
	}
	return acc, true
}

func (rs *Resolver) handleTokens(w http.ResponseWriter, r *http.Request) {
	acc, ok := rs.tokenOwner(w, r)
	if !ok {
		return
	}
	list, err := rs.accounts.Tokens(r.Context(), *acc)
	if err != nil {
		rs.fail(w, err)
		return
	}
	type row struct {
		ID         int64    `json:"id"`
		Name       string   `json:"name"`
		Scopes     []string `json:"scopes"`
		CreatedAt  string   `json:"created_at"`
		LastUsedAt string   `json:"last_used_at,omitempty"`
	}
	out := []row{}
	for _, t := range list {
		out = append(out, row{t.ID, t.Name, t.Scopes, t.CreatedAt, t.LastUsedAt})
	}
	writeJSON(w, http.StatusOK, map[string]any{"tokens": out, "scopes": AllScopes})
}

func (rs *Resolver) handleNewToken(w http.ResponseWriter, r *http.Request) {
	acc, ok := rs.tokenOwner(w, r)
	if !ok {
		return
	}
	var req struct {
		Name   string   `json:"name"`
		Scopes []string `json:"scopes"`
	}
	if !decode(w, r, &req) {
		return
	}
	tok, err := rs.accounts.CreateToken(r.Context(), *acc, req.Name, req.Scopes, rs.transport.PeerIP(r))
	if err != nil {
		rs.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"token": tok})
}

func (rs *Resolver) handleRevokeToken(w http.ResponseWriter, r *http.Request) {
	acc, ok := rs.tokenOwner(w, r)
	if !ok {
		return
	}
	var id int64
	if _, err := fmt.Sscan(chi.URLParam(r, "id"), &id); err != nil {
		writeError(w, http.StatusBadRequest, "auth: no such token")
		return
	}
	if err := rs.accounts.RevokeToken(r.Context(), *acc, id, rs.transport.PeerIP(r)); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "auth: no such token")
			return
		}
		rs.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (rs *Resolver) handleAdminCaps(w http.ResponseWriter, r *http.Request) {
	if _, ok := rs.admin(w, r); !ok {
		return
	}
	rows, limits, drift, err := rs.caps.Matrix(r.Context())
	if err != nil {
		rs.fail(w, err)
		return
	}
	type limit struct {
		Class     string `json:"class"`
		Kind      string `json:"kind"`
		PerMinute int    `json:"per_minute"`
	}
	ls := []limit{}
	for _, l := range limits {
		ls = append(ls, limit{l.Class, l.Kind, l.PerMinute})
	}
	writeJSON(w, http.StatusOK, map[string]any{"mode": rs.caps.Mode(), "signup": rs.accounts.signup, "caps": rows, "limits": ls, "drift": drift})
}

func (rs *Resolver) handleAdminSetCap(w http.ResponseWriter, r *http.Request) {
	acc, ok := rs.admin(w, r)
	if !ok {
		return
	}
	var req struct {
		Class      string `json:"class"`
		Capability string `json:"capability"`
		Allowed    bool   `json:"allowed"`
	}
	if !decode(w, r, &req) {
		return
	}
	if err := rs.caps.Set(r.Context(), req.Class, req.Capability, req.Allowed); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	rs.accounts.audit(r.Context(), acc.ID, "capability-changed", fmt.Sprintf("%s %s %v", req.Class, req.Capability, req.Allowed), rs.transport.PeerIP(r))
	rs.handleAdminCaps(w, r)
}

func (rs *Resolver) handleAdminInvite(w http.ResponseWriter, r *http.Request) {
	acc, ok := rs.admin(w, r)
	if !ok {
		return
	}
	code, until, err := rs.accounts.Invite(r.Context(), *acc, rs.transport.PeerIP(r))
	if err != nil {
		rs.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"code": code, "path": "/signup?invite=" + url.QueryEscape(code), "expires": until.UTC().Format(time.RFC3339)})
}

// The cookies of a provider sign-in: the state the callback must bring
// back, and the identity waiting for a handle.
const (
	oidcCookie    = "bingle_oidc"
	oidcNewCookie = "bingle_oidc_new"
)

// safeReturn is a path on this server to go back to, never an absolute URL.
func safeReturn(to, fallback string) string {
	if strings.HasPrefix(to, "/") && !strings.HasPrefix(to, "//") && !strings.HasPrefix(to, "/\\") {
		return to
	}
	return fallback
}

// toPage sends the browser to a page with a message for it.
func toPage(w http.ResponseWriter, r *http.Request, page, key, msg string) {
	http.Redirect(w, r, page+"?"+url.Values{key: {msg}}.Encode(), http.StatusSeeOther)
}

func (rs *Resolver) handleOIDCStart(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "provider")
	if rs.oidc == nil || rs.accounts == nil {
		toPage(w, r, "/login", "error", ErrUnknownProvider.Error())
		return
	}
	var link int64
	back := "/login"
	if r.URL.Query().Get("link") == "1" {
		p, ok := From(r.Context())
		if !ok || p.Account == nil {
			toPage(w, r, "/login", "error", ErrNotSignedIn.Error())
			return
		}
		link, back = p.Account.ID, "/account"
	}
	to, state, err := rs.oidc.Start(r.Context(), id, safeReturn(r.URL.Query().Get("return"), "/"), link, guestOf(r), r.URL.Query().Get("invite"))
	if err != nil {
		rs.log.Warn("starting a provider sign-in", "provider", id, "err", err)
		msg := ErrOIDCFailed.Error()
		if errors.Is(err, ErrUnknownProvider) {
			msg = err.Error()
		}
		toPage(w, r, back, "error", msg)
		return
	}
	rs.setCookie(w, r, oidcCookie, state, int(oidcFlowFor/time.Second))
	http.Redirect(w, r, to, http.StatusFound)
}

func (rs *Resolver) handleOIDCCallback(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "provider")
	if rs.oidc == nil || rs.accounts == nil {
		toPage(w, r, "/login", "error", ErrUnknownProvider.Error())
		return
	}
	q := r.URL.Query()
	cookie := ""
	if c, err := r.Cookie(oidcCookie); err == nil {
		cookie = c.Value
	}
	rs.setCookie(w, r, oidcCookie, "", -1)
	if q.Get("error") != "" {
		toPage(w, r, "/login", "error", "The provider did not sign you in.")
		return
	}
	ident, flow, err := rs.oidc.Finish(r.Context(), id, q.Get("state"), cookie, q.Get("code"))
	if err != nil {
		rs.log.Warn("a provider sign-in failed", "provider", id, "err", err)
		toPage(w, r, "/login", "error", ErrOIDCFailed.Error())
		return
	}
	res, err := rs.accounts.OIDCSignIn(r.Context(), rs.oidc, ident, flow, rs.transport.PeerIP(r), r.UserAgent())
	back := "/login"
	if flow.link != 0 {
		back = "/account"
	}
	switch {
	case err != nil:
		if statusOf(err) == http.StatusInternalServerError {
			rs.log.Error("a provider sign-in failed", "err", err)
			err = ErrOIDCFailed
		}
		toPage(w, r, back, "error", err.Error())
	case res.Linked:
		toPage(w, r, "/account", "linked", id)
	case res.Pending != "":
		rs.setCookie(w, r, loginCookie, res.Pending, int(pendingFor/time.Second))
		http.Redirect(w, r, "/login?second=1&return="+url.QueryEscape(res.ReturnTo), http.StatusSeeOther)
	case res.Fresh != "":
		rs.setCookie(w, r, oidcNewCookie, res.Fresh, int(oidcFlowFor/time.Second))
		http.Redirect(w, r, "/signup?provider="+url.QueryEscape(id), http.StatusSeeOther)
	default:
		rs.startSession(w, r, *res.Session)
		http.Redirect(w, r, safeReturn(res.ReturnTo, "/"), http.StatusSeeOther)
	}
}

func (rs *Resolver) handleOIDCFinish(w http.ResponseWriter, r *http.Request) {
	if rs.noAccounts(w) || rs.oidc == nil {
		writeError(w, http.StatusNotFound, ErrUnknownProvider.Error())
		return
	}
	var req struct {
		Handle string `json:"handle"`
	}
	if !decode(w, r, &req) {
		return
	}
	c, err := r.Cookie(oidcNewCookie)
	if err != nil || c.Value == "" {
		rs.fail(w, ErrSignInAgain)
		return
	}
	s, back, err := rs.accounts.FinishOIDCSignup(r.Context(), rs.oidc, c.Value, req.Handle, rs.transport.PeerIP(r), r.UserAgent())
	if err != nil {
		rs.fail(w, err)
		return
	}
	rs.setCookie(w, r, oidcNewCookie, "", -1)
	rs.startSession(w, r, s)
	out := rs.meOf(r, s.Account)
	writeJSON(w, http.StatusOK, map[string]any{"me": out, "return": safeReturn(back, "/")})
}

func (rs *Resolver) handleOIDCUnlink(w http.ResponseWriter, r *http.Request) {
	acc, ok := rs.account(w, r)
	if !ok {
		return
	}
	var req struct {
		Provider string `json:"provider"`
		Password string `json:"password"`
	}
	if !decode(w, r, &req) {
		return
	}
	if err := rs.accounts.UnlinkOIDC(r.Context(), *acc, req.Provider, req.Password, rs.transport.PeerIP(r)); err != nil {
		rs.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rs.meOf(r, *acc))
}

func (rs *Resolver) handleRename(w http.ResponseWriter, r *http.Request) {
	acc, ok := rs.tokenOwner(w, r)
	if !ok {
		return
	}
	var req struct {
		Handle string `json:"handle"`
	}
	if !decode(w, r, &req) {
		return
	}
	renamed, err := rs.accounts.Rename(r.Context(), *acc, req.Handle, rs.transport.PeerIP(r))
	if err != nil {
		rs.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rs.meOf(r, renamed))
}

func (rs *Resolver) handleSessions(w http.ResponseWriter, r *http.Request) {
	acc, ok := rs.tokenOwner(w, r)
	if !ok {
		return
	}
	p, _ := From(r.Context())
	list, err := rs.accounts.Sessions(r.Context(), *acc, p.session)
	if err != nil {
		rs.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sessions": list})
}

func (rs *Resolver) handleEndOthers(w http.ResponseWriter, r *http.Request) {
	acc, ok := rs.tokenOwner(w, r)
	if !ok {
		return
	}
	p, _ := From(r.Context())
	if err := rs.accounts.EndOtherSessions(r.Context(), *acc, p.session, rs.transport.PeerIP(r)); err != nil {
		rs.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (rs *Resolver) handleAdminEvents(w http.ResponseWriter, r *http.Request) {
	if _, ok := rs.admin(w, r); !ok {
		return
	}
	events, err := rs.accounts.Events(r.Context(), 200)
	if err != nil {
		rs.fail(w, err)
		return
	}
	type row struct {
		At      string `json:"at"`
		Account string `json:"account,omitempty"`
		Kind    string `json:"kind"`
		Detail  string `json:"detail,omitempty"`
	}
	handles := map[int64]string{}
	out := []row{}
	for _, e := range events {
		h, seen := handles[e.AccountID]
		if !seen && e.AccountID != 0 {
			if acc, err := rs.accounts.st.AccountByID(r.Context(), e.AccountID); err == nil {
				h = acc.Handle
			}
			handles[e.AccountID] = h
		}
		out = append(out, row{At: e.At, Account: h, Kind: e.Kind, Detail: e.Detail})
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": out})
}
