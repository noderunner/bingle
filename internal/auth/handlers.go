package auth

import (
	"encoding/json"
	"errors"
	"net/http"
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
func (rs *Resolver) Routes() http.Handler {
	r := chi.NewRouter()
	r.Post("/signup", rs.handleSignup)
	r.Post("/confirm", rs.handleConfirm)
	r.Post("/login", rs.handleLogin)
	r.Post("/logout", rs.handleLogout)
	r.Post("/password", rs.handlePassword)
	r.Get("/me", rs.handleMe)
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
	case errors.Is(err, ErrBadLogin), errors.Is(err, ErrNotSignedIn):
		return http.StatusUnauthorized
	case errors.Is(err, ErrSignupClosed), errors.Is(err, ErrSignupNoMail), errors.Is(err, ErrNotConfirmed), errors.Is(err, ErrMustChangePwd):
		return http.StatusForbidden
	case errors.Is(err, ErrHandleTaken):
		return http.StatusConflict
	case errors.Is(err, ErrTooManyTries), errors.Is(err, ErrMailLater):
		return http.StatusTooManyRequests
	case errors.Is(err, ErrBadHandle), errors.Is(err, ErrReservedHandle), errors.Is(err, ErrWeakPassword), errors.Is(err, ErrBadLink),
		errors.Is(err, ErrSamePassword), mail.IsAddressError(err):
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
	}
	if !decode(w, r, &req) {
		return
	}
	if err := rs.accounts.Signup(r.Context(), req.Handle, req.Email, req.Password, guestOf(r), rs.transport.PeerIP(r)); err != nil {
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
	s, err := rs.accounts.Login(r.Context(), req.Handle, req.Password, guestOf(r), rs.transport.PeerIP(r), r.UserAgent())
	if err != nil {
		rs.fail(w, err)
		return
	}
	rs.startSession(w, r, s)
	writeJSON(w, http.StatusOK, meOf(s.Account))
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
	writeJSON(w, http.StatusOK, meOf(s.Account))
}

// me is GET /auth/me's answer.
type me struct {
	Kind               string `json:"kind"` // "account", "guest" or "anonymous"
	Handle             string `json:"handle,omitempty"`
	DisplayName        string `json:"display_name,omitempty"`
	MustChangePassword bool   `json:"must_change_password,omitempty"`
	// Accounts says whether this server has accounts at all, and Signup
	// whether a person can make one here.
	Accounts bool `json:"accounts"`
	Signup   bool `json:"signup"`
}

func meOf(a store.Account) me {
	return me{Kind: store.KindAccount, Handle: a.Handle, DisplayName: a.DisplayName, MustChangePassword: a.MustChangePassword, Accounts: true}
}

func (rs *Resolver) handleMe(w http.ResponseWriter, r *http.Request) {
	out := me{Kind: "anonymous"}
	if p, ok := From(r.Context()); ok {
		if p.Account != nil {
			out = meOf(*p.Account)
		} else {
			out.Kind = store.KindGuest
		}
	}
	if rs.accounts != nil {
		out.Accounts = true
		out.Signup = rs.accounts.signup == SignupOpen && rs.accounts.mail != nil && rs.accounts.mail.MailEnabled()
	}
	writeJSON(w, http.StatusOK, out)
}
