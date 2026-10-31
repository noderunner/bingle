package auth

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/noderunner/bingle/internal/mail"
	"github.com/noderunner/bingle/internal/store"
)

// Mailer sends the mail an account needs (session.Manager is one).
type Mailer interface {
	MailEnabled() bool
	SendAccountMail(ctx context.Context, kind, addr, ip string, build func(base string) mail.Message) error
}

// Signup modes (BINGLE_SIGNUP).
const (
	SignupOpen   = "open"
	SignupInvite = "invite" // with an administrator's invitation
	SignupClosed = "closed"
)

// Session lifetimes' defaults (BINGLE_SESSION_IDLE, BINGLE_SESSION_MAX).
const (
	DefaultSessionIdle = 14 * 24 * time.Hour
	DefaultSessionMax  = 90 * 24 * time.Hour
)

// confirmFor is how long a new account's confirmation link works, and how
// long an unconfirmed account is kept.
const confirmFor = 48 * time.Hour

// Accounts signs people up and in (ADR 0068).
type Accounts struct {
	st     *store.Store
	mail   Mailer
	log    *slog.Logger
	signup string
	idle   time.Duration
	max    time.Duration
	params Argon2Params
	now    func() time.Time

	loginsByAddr   windowLimiter
	loginsByHandle windowLimiter
	signups        windowLimiter
	forgots        windowLimiter
	failures       backoff
	pending        pendingLogins
}

// AccountOptions configures Accounts.
type AccountOptions struct {
	Mailer Mailer
	Log    *slog.Logger
	// Signup is SignupOpen or SignupClosed; "" means open.
	Signup string
	// SessionIdle and SessionMax end a session unused that long, and any
	// session that old; zero means the defaults.
	SessionIdle, SessionMax time.Duration
	// Argon2 is the password hash's cost; zero means DefaultArgon2. Tests
	// make it cheap.
	Argon2 Argon2Params
	// LoginsPerMinute caps the sign-ins one address may try in a minute (the
	// capabilities file's guest login limit); zero means 10.
	LoginsPerMinute int
}

// NewAccounts returns the account service over st.
func NewAccounts(st *store.Store, o AccountOptions) *Accounts {
	if o.Log == nil {
		o.Log = slog.Default()
	}
	if o.Signup == "" {
		o.Signup = SignupOpen
	}
	if o.SessionIdle <= 0 {
		o.SessionIdle = DefaultSessionIdle
	}
	if o.SessionMax <= 0 {
		o.SessionMax = DefaultSessionMax
	}
	if o.Argon2 == (Argon2Params{}) {
		o.Argon2 = DefaultArgon2
	}
	return &Accounts{
		st: st, mail: o.Mailer, log: o.Log, signup: o.Signup, idle: o.SessionIdle, max: o.SessionMax, params: o.Argon2, now: time.Now,
		loginsByAddr:   windowLimiter{per: cmp.Or(o.LoginsPerMinute, 10), window: time.Minute},
		loginsByHandle: windowLimiter{per: 10, window: 15 * time.Minute},
		signups:        windowLimiter{per: 5, window: time.Hour},
		forgots:        windowLimiter{per: 5, window: time.Hour},
	}
}

// SetClock replaces the service's clock (tests).
func (a *Accounts) SetClock(now func() time.Time) { a.now = now }

// The answers a person can be given. They are worded for the page.
var (
	ErrSignupClosed  = errors.New("auth: signing up is closed on this server: ask its administrator for an account")
	ErrSignupNoMail  = errors.New("auth: signing up needs email, which this server is not set up to send: ask its administrator for an account")
	ErrHandleTaken   = errors.New("auth: that handle is taken")
	ErrBadLogin      = errors.New("auth: the handle or the password is wrong")
	ErrNotConfirmed  = errors.New("auth: confirm your email address first: the link is in the mail Bingle sent you")
	ErrTooManyTries  = errors.New("auth: too many tries; wait a few minutes")
	ErrMailLater     = errors.New("auth: the confirmation mail could not be sent just now; try again later")
	ErrBadLink       = errors.New("auth: that link is not valid any more: it was used already, or it is too old")
	ErrSamePassword  = errors.New("auth: choose a password different from the old one")
	ErrNotSignedIn   = errors.New("auth: you are not signed in")
	ErrMustChangePwd = errors.New("auth: change your password first")
	ErrNeedsInvite   = errors.New("auth: signing up here needs an invitation: ask the server's administrator for a link")
)

// inviteFor is how long an invitation works.
const inviteFor = 7 * 24 * time.Hour

// ---- signing up ------------------------------------------------------------------

// Signup makes a pending account and mails its confirmation link. guest is
// the guest the browser is (or ""): the account takes its principal, so its
// seats and games are the account's once the address is confirmed. An
// address that is already an account's gets the same answer as a new one,
// and no mail, so the answer does not say whose it is.
func (a *Accounts) Signup(ctx context.Context, handle, email, password, guest, invite, ip string) error {
	switch {
	case a.signup == SignupClosed:
		return ErrSignupClosed
	case a.mail == nil || !a.mail.MailEnabled():
		return ErrSignupNoMail
	}
	if err := a.checkInvite(ctx, invite); err != nil {
		return err
	}
	h, err := CanonHandle(handle, false)
	if err != nil {
		return err
	}
	addr, err := mail.ValidAddress(email)
	if err != nil {
		return err
	}
	if err := CheckPassword(password); err != nil {
		return err
	}
	if !a.signups.allow(ip, a.now()) {
		return ErrTooManyTries
	}
	now := a.now()
	if err := a.st.DeletePendingAccounts(ctx, now.Add(-confirmFor)); err != nil {
		return err
	}
	if _, err := a.st.AccountByHandle(ctx, h); err == nil {
		return ErrHandleTaken
	}
	hash := HashPassword(password, a.params)
	principal, fresh := guest, false
	if principal == "" || a.principalHasAccount(ctx, principal) {
		principal, fresh = NewPrincipalID(), true
	}
	id, err := a.st.CreateAccount(ctx, store.NewAccount{Handle: h, DisplayName: h, Email: addr, PrincipalID: principal, NewPrincipal: fresh, PasswordHash: hash})
	switch {
	case errors.Is(err, store.ErrTaken):
		// The handle was checked a moment ago, so it is the address: answer as
		// if it went through.
		return nil
	case err != nil:
		return err
	}
	if err := a.spendInvite(ctx, invite, id); err != nil {
		_ = a.st.DeletePendingAccount(ctx, id)
		return err
	}
	token := newSecret()
	if err := a.st.CreateAccountToken(ctx, hashSecret(token), id, "confirm", now.Add(confirmFor)); err != nil {
		return err
	}
	if err := a.mail.SendAccountMail(ctx, "confirm", addr, ip, func(base string) mail.Message { return mail.AccountConfirmation(addr, h, base, token) }); err != nil {
		// Without the mail the account can never be used: let the handle go.
		_ = a.st.DeletePendingAccount(ctx, id)
		a.log.Warn("the confirmation of a new account was not sent", "err", err)
		return ErrMailLater
	}
	a.audit(ctx, id, "signup", "", ip)
	return nil
}

func (a *Accounts) principalHasAccount(ctx context.Context, principal string) bool {
	kind, err := a.st.PrincipalKind(ctx, principal)
	if err != nil || kind != store.KindGuest {
		return true
	}
	has, err := a.st.PrincipalHasAccount(ctx, principal)
	return err != nil || has
}

// Confirm spends a confirmation link: the account is usable from now on, and
// the guest it was made from, if any, stops being a guest — its keys stop
// working, and signing in takes over.
func (a *Accounts) Confirm(ctx context.Context, token, ip string) (store.Account, error) {
	id, err := a.st.UseAccountToken(ctx, hashSecret(token), "confirm", a.now())
	if errors.Is(err, store.ErrNotFound) {
		return store.Account{}, ErrBadLink
	}
	if err != nil {
		return store.Account{}, err
	}
	if err := a.st.ActivateAccount(ctx, id, a.now()); err != nil {
		return store.Account{}, err
	}
	acc, err := a.st.AccountByID(ctx, id)
	if err != nil {
		return store.Account{}, err
	}
	if err := a.st.RevokeGuestKeys(ctx, acc.PrincipalID); err != nil {
		return store.Account{}, err
	}
	a.audit(ctx, id, "confirmed", "", ip)
	return acc, nil
}

// ---- signing in and out -------------------------------------------------------------

// Session is a new signed-in browser: the cookie's value, and until when.
type Session struct {
	Token   string
	Expires time.Time
	Account store.Account
}

// Login checks a handle and a password. guest is the guest the browser was
// (or ""): what it holds becomes the account's. Every way of being wrong —
// no such handle, a wrong password — is the same answer in the same time,
// and five of them in a quarter of an hour lock the handle for a while.
//
// An account with a second factor is not signed in yet: Login returns the
// token of a sign-in waiting for its code (SecondFactor finishes it).
func (a *Accounts) Login(ctx context.Context, handle, password, guest, ip, agent string) (Session, string, error) {
	h := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(handle), "@"))
	now := a.now()
	if !a.loginsByAddr.allow(ip, now) || !a.loginsByHandle.allow(h, now) || a.failures.locked("handle:"+h, now) {
		return Session{}, "", ErrTooManyTries
	}
	acc, err := a.st.AccountByHandle(ctx, h)
	if errors.Is(err, store.ErrNotFound) {
		VerifyPassword(password, dummy(), a.params)
		a.failures.fail("handle:"+h, now)
		a.audit(ctx, 0, "login-failed", "", ip)
		return Session{}, "", ErrBadLogin
	}
	if err != nil {
		return Session{}, "", err
	}
	stored, err := a.st.PasswordHash(ctx, acc.ID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return Session{}, "", err
	}
	ok, rehash := VerifyPassword(password, stored, a.params)
	if !ok {
		a.failures.fail("handle:"+h, now)
		a.audit(ctx, acc.ID, "login-failed", "", ip)
		return Session{}, "", ErrBadLogin
	}
	if acc.Status != store.AccountActive {
		return Session{}, "", ErrNotConfirmed
	}
	a.failures.clear("handle:" + h)
	if rehash {
		if err := a.st.SetPassword(ctx, acc.ID, HashPassword(password, a.params), acc.MustChangePassword); err != nil {
			return Session{}, "", err
		}
	}
	if acc.HasTOTP {
		return Session{}, a.pending.add(acc.ID, guest, now), nil
	}
	s, err := a.finishLogin(ctx, acc, guest, ip, agent, "password")
	return s, "", err
}

// finishLogin signs a browser in to acc, once every factor is in.
func (a *Accounts) finishLogin(ctx context.Context, acc store.Account, guest, ip, agent, how string) (Session, error) {
	// What the browser did as a guest is the account's from now on.
	if guest != "" && guest != acc.PrincipalID {
		if err := a.st.MergePrincipal(ctx, guest, acc.PrincipalID); err != nil {
			return Session{}, err
		}
	}
	if err := a.st.RevokeGuestKeys(ctx, acc.PrincipalID); err != nil {
		return Session{}, err
	}
	s, err := a.newSession(ctx, acc, ip, agent)
	if err != nil {
		return Session{}, err
	}
	a.audit(ctx, acc.ID, "login", how, ip)
	return s, nil
}

func (a *Accounts) newSession(ctx context.Context, acc store.Account, ip, agent string) (Session, error) {
	token := newSecret()
	expires := a.now().Add(a.max)
	if len(agent) > 200 {
		agent = agent[:200]
	}
	if err := a.st.CreateSession(ctx, hashSecret(token), acc.ID, expires, hashIP(ip), agent); err != nil {
		return Session{}, err
	}
	return Session{Token: token, Expires: expires, Account: acc}, nil
}

// Logout ends the session whose cookie is token.
func (a *Accounts) Logout(ctx context.Context, token string, accountID int64, ip string) error {
	if err := a.st.DeleteSession(ctx, hashSecret(token)); err != nil {
		return err
	}
	a.audit(ctx, accountID, "logout", "", ip)
	return nil
}

// account is the active account of a session cookie's value.
func (a *Accounts) account(ctx context.Context, token string) (store.Account, error) {
	now := a.now()
	return a.st.SessionAccount(ctx, hashSecret(token), now.Add(-a.idle), now, touchEvery)
}

// ChangePassword replaces an account's password, given the old one. Every
// other session of the account ends, and the one asking gets a new cookie:
// a changed password is the moment a stolen session must stop working.
func (a *Accounts) ChangePassword(ctx context.Context, acc store.Account, oldPw, newPw, ip, agent string) (Session, error) {
	stored, err := a.st.PasswordHash(ctx, acc.ID)
	switch {
	case errors.Is(err, store.ErrNotFound):
		// An account a provider made sets its first password: there is no
		// old one to give.
	case err != nil:
		return Session{}, err
	default:
		if ok, _ := VerifyPassword(oldPw, stored, a.params); !ok {
			a.audit(ctx, acc.ID, "password-change-failed", "", ip)
			return Session{}, ErrBadLogin
		}
	}
	if err := CheckPassword(newPw); err != nil {
		return Session{}, err
	}
	if oldPw == newPw {
		return Session{}, ErrSamePassword
	}
	if err := a.st.SetPassword(ctx, acc.ID, HashPassword(newPw, a.params), false); err != nil {
		return Session{}, err
	}
	if err := a.st.DeleteSessions(ctx, acc.ID, ""); err != nil {
		return Session{}, err
	}
	acc.MustChangePassword = false
	s, err := a.newSession(ctx, acc, ip, agent)
	if err != nil {
		return Session{}, err
	}
	a.audit(ctx, acc.ID, "password-changed", "", ip)
	return s, nil
}

// ---- accounts made without signing up ----------------------------------------------

// Seeded is the administrator made for an empty server, and the password to
// give its operator once.
type Seeded struct {
	Handle   string
	Password string
	// Deterministic says the password is the well-known development one.
	Deterministic bool
}

// SeedOptions says how to make the first administrator.
type SeedOptions struct {
	Handle, Email string
	// Password is the operator's choice (BINGLE_ADMIN_PASSWORD); when empty,
	// the development password if Deterministic, otherwise a random one.
	Password      string
	Deterministic bool
}

// DevPassword is the seeded administrator's password under BINGLE_DEV or on
// a loopback bind. It must be changed at the first sign-in.
const DevPassword = "bingle123"

// SeedAdmin makes the first administrator when there are no accounts at
// all, and reports it (nil when there were accounts). Its password must be
// changed at its first sign-in, whatever it is.
func (a *Accounts) SeedAdmin(ctx context.Context, o SeedOptions) (*Seeded, error) {
	n, err := a.st.CountAccounts(ctx)
	if err != nil || n > 0 {
		return nil, err
	}
	if o.Handle == "" {
		o.Handle = "admin"
	}
	h, err := CanonHandle(o.Handle, true)
	if err != nil {
		return nil, fmt.Errorf("auth: BINGLE_ADMIN_HANDLE: %w", err)
	}
	addr, err := mail.ValidAddress(o.Email)
	if err != nil {
		return nil, fmt.Errorf("auth: BINGLE_ADMIN_EMAIL: %w", err)
	}
	pw, det := o.Password, false
	switch {
	case pw != "":
	case o.Deterministic:
		pw, det = DevPassword, true
	default:
		pw = RandomPassword()
	}
	id, err := a.st.CreateAccount(ctx, store.NewAccount{Handle: h, DisplayName: h, Email: addr, Verified: true, MustChangePassword: true,
		PrincipalID: NewPrincipalID(), NewPrincipal: true, PasswordHash: HashPassword(pw, a.params), Groups: []string{store.GroupAdmin}})
	if err != nil {
		return nil, err
	}
	a.audit(ctx, id, "seeded", "", "")
	return &Seeded{Handle: h, Password: pw, Deterministic: det}, nil
}

// Create makes a confirmed account from the command line, with a random
// password to be changed at its first sign-in, and returns the password.
func (a *Accounts) Create(ctx context.Context, handle, email string, admin bool) (string, error) {
	h, err := CanonHandle(handle, false)
	if err != nil {
		return "", err
	}
	addr, err := mail.ValidAddress(email)
	if err != nil {
		return "", err
	}
	var groups []string
	if admin {
		groups = []string{store.GroupAdmin}
	}
	pw := RandomPassword()
	id, err := a.st.CreateAccount(ctx, store.NewAccount{Handle: h, DisplayName: h, Email: addr, Verified: true, MustChangePassword: true,
		PrincipalID: NewPrincipalID(), NewPrincipal: true, PasswordHash: HashPassword(pw, a.params), Groups: groups})
	if errors.Is(err, store.ErrTaken) {
		return "", fmt.Errorf("auth: the handle or the address is an account's already")
	}
	if err != nil {
		return "", err
	}
	a.audit(ctx, id, "created", "command line", "")
	return pw, nil
}

// ResetPassword gives an account a new random password, to be changed at its
// next sign-in, takes away its second factor and signs it out everywhere:
// the command line's way back in.
func (a *Accounts) ResetPassword(ctx context.Context, handle string) (string, error) {
	acc, err := a.st.AccountByHandle(ctx, handle)
	if err != nil {
		return "", err
	}
	pw := RandomPassword()
	if err := a.st.SetPassword(ctx, acc.ID, HashPassword(pw, a.params), true); err != nil {
		return "", err
	}
	// Lost the phone and the recovery codes too: that is what an operator
	// is for, and the reset takes the second factor away with the password.
	if err := a.st.RemoveSecondFactor(ctx, acc.ID); err != nil {
		return "", err
	}
	if err := a.st.DeleteSessions(ctx, acc.ID, ""); err != nil {
		return "", err
	}
	a.failures.clear("handle:" + acc.Handle)
	a.failures.clear(accountKey(acc.ID))
	a.audit(ctx, acc.ID, "password-reset", "command line, second factor removed", "")
	return pw, nil
}

// ---- helpers ------------------------------------------------------------------------

func (a *Accounts) audit(ctx context.Context, accountID int64, kind, detail, ip string) {
	if err := a.st.AddAuthEvent(ctx, accountID, kind, detail, hashIP(ip)); err != nil {
		a.log.Error("writing the audit log", "kind", kind, "err", err)
	}
}

// newSecret is a session cookie's value or a one-time link's token.
func newSecret() string { return base64.RawURLEncoding.EncodeToString(randomBytes(32)) }

func hashSecret(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// hashIP is how an address is kept in the audit log and with a session:
// enough to tell two apart, not to read either back.
func hashIP(ip string) string {
	if ip == "" {
		return ""
	}
	sum := sha256.Sum256([]byte("bingle-ip:" + ip))
	return hex.EncodeToString(sum[:6])
}

// dummy is a hash to check a password against when there is no account, so
// that "no such handle" takes as long as "wrong password". Made on first use,
// not at start-up, which every test binary would pay for.
var dummy = sync.OnceValue(func() string { return HashPassword("not anybody's password", DefaultArgon2) })

// ---- invitations ---------------------------------------------------------------------

// checkInvite: in invite mode, signing up needs a live invitation.
func (a *Accounts) checkInvite(ctx context.Context, invite string) error {
	if a.signup != SignupInvite {
		return nil
	}
	if invite == "" {
		return ErrNeedsInvite
	}
	live, err := a.st.InviteLive(ctx, hashSecret(invite), a.now())
	if err != nil {
		return err
	}
	if !live {
		return ErrBadLink
	}
	return nil
}

// spendInvite uses the invitation on the account just made (invite mode).
func (a *Accounts) spendInvite(ctx context.Context, invite string, accountID int64) error {
	if a.signup != SignupInvite {
		return nil
	}
	ok, err := a.st.UseInvite(ctx, hashSecret(invite), accountID, a.now())
	if err != nil {
		return err
	}
	if !ok {
		return ErrBadLink
	}
	return nil
}

// Invite mints an invitation to sign up, for a week, and returns its code.
func (a *Accounts) Invite(ctx context.Context, by store.Account, ip string) (string, time.Time, error) {
	code := newSecret()
	until := a.now().Add(inviteFor)
	if err := a.st.CreateInvite(ctx, hashSecret(code), by.ID, until); err != nil {
		return "", time.Time{}, err
	}
	a.audit(ctx, by.ID, "invite-made", "", ip)
	return code, until, nil
}

// SignupMode is "open", "invite" or "closed".
func (a *Accounts) SignupMode() string { return a.signup }
