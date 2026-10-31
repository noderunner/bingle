package auth

import (
	"context"
	"errors"

	"github.com/noderunner/bingle/internal/mail"
	"github.com/noderunner/bingle/internal/store"
)

// OIDCResult is what a provider's sign-in comes to: exactly one of a
// session, a sign-in waiting for its second factor, an identity waiting for
// a handle, or a provider linked to the account that asked.
type OIDCResult struct {
	Session  *Session
	Pending  string
	Fresh    string
	Linked   bool
	ReturnTo string
}

// OIDCSignIn decides what a provider's identity signs in to. The linking
// rule, because this is the classic takeover vector: an identity links to an
// existing account by its address only when the provider says the address is
// verified and the account's address is confirmed here; otherwise the person
// signs in with their password and links the provider from the account page.
func (a *Accounts) OIDCSignIn(ctx context.Context, o *OIDC, id *Identity, f *oidcFlow, ip, agent string) (OIDCResult, error) {
	key := store.OIDCKey(id.Provider, id.Subject)
	res := OIDCResult{ReturnTo: id.returnTo}
	// Linking from the account page: the person is signed in already.
	if f.link != 0 {
		acc, err := a.st.AccountByID(ctx, f.link)
		if err != nil {
			return res, err
		}
		if owner, err := a.st.AccountByOIDC(ctx, key); err == nil {
			if owner.ID != acc.ID {
				return res, ErrLinkedElsewhere
			}
			res.Linked = true
			return res, nil
		}
		if err := a.st.LinkOIDC(ctx, acc.ID, key); err != nil {
			if errors.Is(err, store.ErrTaken) {
				return res, ErrLinkedElsewhere
			}
			return res, err
		}
		a.audit(ctx, acc.ID, "provider-linked", id.Provider, ip)
		res.Linked = true
		return res, nil
	}
	acc, err := a.st.AccountByOIDC(ctx, key)
	switch {
	case err == nil:
		return a.oidcSession(ctx, res, acc, id, ip, agent)
	case !errors.Is(err, store.ErrNotFound):
		return res, err
	}
	if id.Email != "" {
		byAddr, err := a.st.AccountByEmail(ctx, id.Email)
		switch {
		case err == nil && id.EmailVerified && byAddr.EmailVerified:
			if err := a.st.LinkOIDC(ctx, byAddr.ID, key); err != nil {
				return res, err
			}
			a.audit(ctx, byAddr.ID, "provider-linked", id.Provider+", by its verified address", ip)
			return a.oidcSession(ctx, res, byAddr, id, ip, agent)
		case err == nil:
			return res, ErrOIDCAddressUsed
		case !errors.Is(err, store.ErrNotFound):
			return res, err
		}
	}
	// Nobody here yet: a new account, once its person chooses a handle.
	switch {
	case a.signup == SignupClosed:
		return res, ErrSignupClosed
	case !id.EmailVerified:
		return res, ErrOIDCUnverified
	}
	if err := a.checkInvite(ctx, id.invite); err != nil {
		return res, err
	}
	if _, err := mail.ValidAddress(id.Email); err != nil {
		return res, ErrOIDCUnverified
	}
	res.Fresh = o.waitForHandle(id)
	return res, nil
}

// oidcSession signs acc in, or leaves the sign-in waiting for its second
// factor: a provider stands in for the password, not for the code.
func (a *Accounts) oidcSession(ctx context.Context, res OIDCResult, acc store.Account, id *Identity, ip, agent string) (OIDCResult, error) {
	if acc.HasTOTP {
		res.Pending = a.pending.add(acc.ID, id.guest, a.now())
		return res, nil
	}
	s, err := a.finishLogin(ctx, acc, id.guest, ip, agent, id.Provider)
	if err != nil {
		return res, err
	}
	res.Session = &s
	return res, nil
}

// FinishOIDCSignup makes the account of an identity waiting for a handle,
// with the provider's (verified) address, and signs it in. The browser's
// guest becomes the account, as with a password sign-up.
func (a *Accounts) FinishOIDCSignup(ctx context.Context, o *OIDC, token, handle, ip, agent string) (Session, string, error) {
	h, err := CanonHandle(handle, false)
	if err != nil {
		return Session{}, "", err
	}
	if a.handleTaken(ctx, h, 0) {
		return Session{}, "", ErrHandleTaken
	}
	id, ok := o.takeFresh(token)
	if !ok {
		return Session{}, "", ErrSignInAgain
	}
	principal, fresh := id.guest, false
	if principal == "" || a.principalHasAccount(ctx, principal) {
		principal, fresh = NewPrincipalID(), true
	}
	accID, err := a.st.CreateAccount(ctx, store.NewAccount{Handle: h, DisplayName: h, Email: id.Email, Verified: true, PrincipalID: principal, NewPrincipal: fresh,
		OIDC: store.OIDCKey(id.Provider, id.Subject)})
	if err != nil {
		if errors.Is(err, store.ErrTaken) {
			return Session{}, "", ErrOIDCAddressUsed
		}
		return Session{}, "", err
	}
	if err := a.spendInvite(ctx, id.invite, accID); err != nil {
		return Session{}, "", err
	}
	a.audit(ctx, accID, "signup", "by "+id.Provider, ip)
	acc, err := a.st.AccountByID(ctx, accID)
	if err != nil {
		return Session{}, "", err
	}
	s, err := a.finishLogin(ctx, acc, "", ip, agent, id.Provider)
	return s, id.returnTo, err
}

// UnlinkOIDC stops acc signing in with a provider, given the password: an
// account that would have no way in left is refused.
func (a *Accounts) UnlinkOIDC(ctx context.Context, acc store.Account, provider, password, ip string) error {
	if _, err := a.st.PasswordHash(ctx, acc.ID); errors.Is(err, store.ErrNotFound) {
		return ErrLastWayIn
	}
	if err := a.checkPassword(ctx, acc, password, ip); err != nil {
		return err
	}
	if err := a.st.UnlinkOIDC(ctx, acc.ID, provider); err != nil {
		return err
	}
	a.audit(ctx, acc.ID, "provider-unlinked", provider, ip)
	return nil
}

// Linked are the providers acc signs in with.
func (a *Accounts) Linked(ctx context.Context, acc store.Account) []string {
	l, _ := a.st.OIDCLinks(ctx, acc.ID)
	return l
}

// HasPassword reports whether acc has a password (an account a provider made
// has none until it sets one).
func (a *Accounts) HasPassword(ctx context.Context, acc store.Account) bool {
	_, err := a.st.PasswordHash(ctx, acc.ID)
	return err == nil
}
