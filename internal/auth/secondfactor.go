package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/noderunner/bingle/internal/mail"
	"github.com/noderunner/bingle/internal/store"
)

// The second factor (ADR 0069): TOTP, ten recovery codes, the backoff that
// locks a handle or an account after repeated failures, and resetting a
// forgotten password by mail.

var (
	ErrBadCode        = errors.New("auth: that code is not right")
	ErrSignInAgain    = errors.New("auth: that sign-in has run out; sign in again")
	ErrNoSecondFactor = errors.New("auth: this account has no second factor")
	ErrNoSetup        = errors.New("auth: start setting up the second factor again")
	ErrNeedsCode      = errors.New("auth: this account signs in with a second factor: give its code or a recovery code")
	// ErrAdminNeedsTOTP is the answer to an administrator without a second
	// factor, off this machine, doing anything but setting one up.
	ErrAdminNeedsTOTP = errors.New("auth: an administrator signs in with a second factor: set one up on your account page first")
)

// ---- backoff -------------------------------------------------------------------------

// backoff locks a key (a handle, an account) after five failures in a
// quarter of an hour: a minute after the fifth, doubling with each further
// failure, an hour at most. A success clears it.
type backoff struct {
	mu sync.Mutex
	at map[string][]time.Time
}

const (
	backoffFree   = 5
	backoffWindow = 15 * time.Minute
	backoffMax    = time.Hour
)

func (b *backoff) recent(key string, now time.Time) []time.Time {
	var kept []time.Time
	for _, t := range b.at[key] {
		if now.Sub(t) < backoffMax+backoffWindow {
			kept = append(kept, t)
		}
	}
	return kept
}

func (b *backoff) fail(key string, now time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.at == nil {
		b.at = map[string][]time.Time{}
	}
	b.at[key] = append(b.recent(key, now), now)
}

// locked reports whether key may not try now: five failures within a
// quarter of an hour of the last one lock it, for a minute after that last
// one, doubled for every failure beyond the fifth, an hour at most.
func (b *backoff) locked(key string, now time.Time) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	ts := b.at[key]
	if len(ts) == 0 {
		return false
	}
	last := ts[len(ts)-1]
	n := 0
	for _, t := range ts {
		if last.Sub(t) < backoffWindow {
			n++
		}
	}
	if n < backoffFree {
		return false
	}
	wait := min(time.Minute<<min(n-backoffFree, 6), backoffMax)
	return now.Sub(last) < wait
}

func (b *backoff) clear(key string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.at, key)
}

func accountKey(id int64) string { return fmt.Sprintf("account:%d", id) }

// ---- a sign-in waiting for its code ----------------------------------------------------

// loginCookie holds the token of a sign-in whose password was right and
// whose code is still to come; pendingFor is how long it waits.
const (
	loginCookie = "bingle_login"
	pendingFor  = 5 * time.Minute
	// pendingTries is how many wrong codes one waiting sign-in takes.
	pendingTries = 5
)

type pendingLogin struct {
	account int64
	guest   string
	until   time.Time
	tries   int
}

// pendingLogins are kept in memory: a restart only means typing the
// password again.
type pendingLogins struct {
	mu sync.Mutex
	m  map[string]*pendingLogin
}

func (p *pendingLogins) add(account int64, guest string, now time.Time) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.m == nil {
		p.m = map[string]*pendingLogin{}
	}
	for k, v := range p.m {
		if now.After(v.until) {
			delete(p.m, k)
		}
	}
	token := newSecret()
	p.m[hashSecret(token)] = &pendingLogin{account: account, guest: guest, until: now.Add(pendingFor)}
	return token
}

// get is the waiting sign-in of token, if it is still waiting.
func (p *pendingLogins) get(token string, now time.Time) (*pendingLogin, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	l, ok := p.m[hashSecret(token)]
	if !ok || now.After(l.until) || l.tries >= pendingTries {
		return nil, false
	}
	return l, true
}

// miss counts a wrong code against the waiting sign-in.
func (p *pendingLogins) miss(token string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if l, ok := p.m[hashSecret(token)]; ok {
		l.tries++
	}
}

func (p *pendingLogins) done(token string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.m, hashSecret(token))
}

// ---- checking a code -------------------------------------------------------------

// checkCode checks a TOTP code, or else a recovery code, for acc, and spends
// it. Failures count towards the account's backoff.
func (a *Accounts) checkCode(ctx context.Context, acc store.Account, code, recovery, ip string) (string, error) {
	now := a.now()
	key := accountKey(acc.ID)
	if a.failures.locked(key, now) {
		return "", ErrTooManyTries
	}
	if recovery != "" {
		ok, err := a.st.UseRecoveryCode(ctx, acc.ID, hashSecret(canonRecovery(recovery)))
		if err != nil {
			return "", err
		}
		if ok {
			a.failures.clear(key)
			return "recovery code", nil
		}
	} else {
		secret, last, err := a.st.TOTP(ctx, acc.ID)
		if errors.Is(err, store.ErrNotFound) {
			return "", ErrNoSecondFactor
		}
		if err != nil {
			return "", err
		}
		if step, ok := CheckTOTP(secret, code, now, last); ok {
			fresh, err := a.st.UseTOTPStep(ctx, acc.ID, step)
			if err != nil {
				return "", err
			}
			if fresh {
				a.failures.clear(key)
				return "totp", nil
			}
		}
	}
	a.failures.fail(key, now)
	a.audit(ctx, acc.ID, "second-factor-failed", "", ip)
	return "", ErrBadCode
}

// SecondFactor finishes a sign-in waiting for its code: token is the
// waiting sign-in's, code a TOTP code or recovery a recovery code.
func (a *Accounts) SecondFactor(ctx context.Context, token, code, recovery, ip, agent string) (Session, error) {
	l, ok := a.pending.get(token, a.now())
	if !ok {
		return Session{}, ErrSignInAgain
	}
	acc, err := a.st.AccountByID(ctx, l.account)
	if err != nil {
		return Session{}, err
	}
	how, err := a.checkCode(ctx, acc, code, recovery, ip)
	if err != nil {
		if errors.Is(err, ErrBadCode) {
			a.pending.miss(token)
		}
		return Session{}, err
	}
	a.pending.done(token)
	return a.finishLogin(ctx, acc, l.guest, ip, agent, "password and "+how)
}

// ---- setting it up -----------------------------------------------------------------

// checkPassword is the re-authentication a change to the second factor asks
// for.
func (a *Accounts) checkPassword(ctx context.Context, acc store.Account, password, ip string) error {
	now := a.now()
	if a.failures.locked(accountKey(acc.ID), now) {
		return ErrTooManyTries
	}
	stored, err := a.st.PasswordHash(ctx, acc.ID)
	if err != nil {
		return err
	}
	if ok, _ := VerifyPassword(password, stored, a.params); !ok {
		a.failures.fail(accountKey(acc.ID), now)
		a.audit(ctx, acc.ID, "reauth-failed", "", ip)
		return ErrBadLogin
	}
	return nil
}

// SetupTOTP starts setting up a second factor: it returns the secret for an
// app, and the otpauth:// link. Nothing changes until EnableTOTP.
func (a *Accounts) SetupTOTP(ctx context.Context, acc store.Account, password, ip string) (string, string, error) {
	if err := a.checkPassword(ctx, acc, password, ip); err != nil {
		return "", "", err
	}
	secret := NewTOTPSecret()
	if err := a.st.SetPendingTOTP(ctx, acc.ID, secret); err != nil {
		return "", "", err
	}
	return secret, TOTPURI(acc.Handle, secret), nil
}

// EnableTOTP turns the second factor on with a code from the app, and
// returns the ten recovery codes, shown once. Every other session ends and
// this browser gets a new cookie: the account's protection changed.
func (a *Accounts) EnableTOTP(ctx context.Context, acc store.Account, code, ip, agent string) ([]string, Session, error) {
	secret, err := a.st.PendingTOTP(ctx, acc.ID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, Session{}, ErrNoSetup
	}
	if err != nil {
		return nil, Session{}, err
	}
	step, ok := CheckTOTP(secret, code, a.now(), -1)
	if !ok {
		return nil, Session{}, ErrBadCode
	}
	codes := NewRecoveryCodes()
	hashes := make([]string, len(codes))
	for i, c := range codes {
		hashes[i] = hashSecret(canonRecovery(c))
	}
	if err := a.st.EnableTOTP(ctx, acc.ID, step, hashes); err != nil {
		return nil, Session{}, err
	}
	s, err := a.rotate(ctx, acc, ip, agent)
	if err != nil {
		return nil, Session{}, err
	}
	s.Account.HasTOTP = true
	a.audit(ctx, acc.ID, "second-factor-on", "", ip)
	return codes, s, nil
}

// DisableTOTP turns the second factor off, given the password and a code.
func (a *Accounts) DisableTOTP(ctx context.Context, acc store.Account, password, code, recovery, ip, agent string) (Session, error) {
	if err := a.checkPassword(ctx, acc, password, ip); err != nil {
		return Session{}, err
	}
	if _, err := a.checkCode(ctx, acc, code, recovery, ip); err != nil {
		return Session{}, err
	}
	if err := a.st.RemoveSecondFactor(ctx, acc.ID); err != nil {
		return Session{}, err
	}
	s, err := a.rotate(ctx, acc, ip, agent)
	if err != nil {
		return Session{}, err
	}
	s.Account.HasTOTP = false
	a.audit(ctx, acc.ID, "second-factor-off", "", ip)
	return s, nil
}

// NewRecoveryCodes replaces an account's recovery codes, given the password.
func (a *Accounts) NewRecoveryCodes(ctx context.Context, acc store.Account, password, ip string) ([]string, error) {
	if !acc.HasTOTP {
		return nil, ErrNoSecondFactor
	}
	if err := a.checkPassword(ctx, acc, password, ip); err != nil {
		return nil, err
	}
	codes := NewRecoveryCodes()
	hashes := make([]string, len(codes))
	for i, c := range codes {
		hashes[i] = hashSecret(canonRecovery(c))
	}
	if err := a.st.ReplaceRecoveryCodes(ctx, acc.ID, hashes); err != nil {
		return nil, err
	}
	a.audit(ctx, acc.ID, "recovery-codes-renewed", "", ip)
	return codes, nil
}

// RecoveryCodesLeft is how many recovery codes an account has unused.
func (a *Accounts) RecoveryCodesLeft(ctx context.Context, acc store.Account) int {
	n, _ := a.st.CountRecoveryCodes(ctx, acc.ID)
	return n
}

// rotate ends every session of acc and starts a new one for this browser.
func (a *Accounts) rotate(ctx context.Context, acc store.Account, ip, agent string) (Session, error) {
	if err := a.st.DeleteSessions(ctx, acc.ID, ""); err != nil {
		return Session{}, err
	}
	return a.newSession(ctx, acc, ip, agent)
}

// ---- a forgotten password ------------------------------------------------------------

// resetFor is how long a password reset link works.
const resetFor = time.Hour

// ForgotPassword mails a link to reset the password of the account with the
// handle or address who, if there is one. The answer is the same either way,
// so it does not say whether there is.
func (a *Accounts) ForgotPassword(ctx context.Context, who, ip string) error {
	if a.mail == nil || !a.mail.MailEnabled() {
		return ErrSignupNoMail
	}
	now := a.now()
	if !a.forgots.allow(ip, now) {
		return ErrTooManyTries
	}
	who = strings.TrimSpace(who)
	var acc store.Account
	var err error
	if strings.Contains(who, "@") && !strings.HasPrefix(who, "@") {
		acc, err = a.st.AccountByEmail(ctx, who)
	} else {
		acc, err = a.st.AccountByHandle(ctx, strings.TrimPrefix(who, "@"))
	}
	if err != nil || acc.Status != store.AccountActive {
		return nil
	}
	if a.failures.locked("reset:"+acc.Handle, now) {
		return nil
	}
	a.failures.fail("reset:"+acc.Handle, now) // five links a quarter, then slower
	token := newSecret()
	if err := a.st.CreateAccountToken(ctx, hashSecret(token), acc.ID, "reset", now.Add(resetFor)); err != nil {
		return err
	}
	if err := a.mail.SendAccountMail(ctx, "reset", acc.Email, ip, func(base string) mail.Message { return mail.PasswordReset(acc.Email, acc.Handle, base, token) }); err != nil {
		a.log.Warn("a password reset mail was not sent", "err", err)
	}
	a.audit(ctx, acc.ID, "reset-requested", "", ip)
	return nil
}

// ResetWithLink sets a new password from a mailed link. An account with a
// second factor gives its code too: a mailbox alone does not get past it.
// Every session ends; the browser signs in with the new password.
func (a *Accounts) ResetWithLink(ctx context.Context, token, password, code, recovery, ip string) error {
	if err := CheckPassword(password); err != nil {
		return err
	}
	id, err := a.st.PeekAccountToken(ctx, hashSecret(token), "reset", a.now())
	if errors.Is(err, store.ErrNotFound) {
		return ErrBadLink
	}
	if err != nil {
		return err
	}
	acc, err := a.st.AccountByID(ctx, id)
	if err != nil {
		return err
	}
	if acc.HasTOTP {
		if code == "" && recovery == "" {
			return ErrNeedsCode
		}
		if _, err := a.checkCode(ctx, acc, code, recovery, ip); err != nil {
			return err
		}
	}
	if _, err := a.st.UseAccountToken(ctx, hashSecret(token), "reset", a.now()); err != nil {
		return ErrBadLink
	}
	if err := a.st.SetPassword(ctx, acc.ID, HashPassword(password, a.params), false); err != nil {
		return err
	}
	if err := a.st.DeleteSessions(ctx, acc.ID, ""); err != nil {
		return err
	}
	a.failures.clear("handle:" + acc.Handle)
	a.audit(ctx, acc.ID, "password-reset", "mailed link", ip)
	return nil
}
