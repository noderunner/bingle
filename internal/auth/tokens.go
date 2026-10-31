package auth

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/noderunner/bingle/internal/store"
)

// API tokens (ADR 0071): an account's, for scripts and agents — "Authorization:
// Bearer bgl_…" — scoped, revocable, and never carrying the admin group's
// powers.

// APITokenPrefix starts every API token.
const APITokenPrefix = "bgl_"

// The scopes a token may have.
const (
	ScopePlay        = "play"         // join, play, chat and talk in games
	ScopeCreateGame  = "create_game"  // make games
	ScopeReadMetrics = "read_metrics" // the metrics a server shows its members
)

// AllScopes lists them.
var AllScopes = []string{ScopePlay, ScopeCreateGame, ScopeReadMetrics}

// DevAPIToken is the token BINGLE_DEV seeds for the first administrator, so
// CI can drive a closed server.
const DevAPIToken = APITokenPrefix + "dev-bingle123-not-for-a-server-anybody-else-reaches"

var (
	ErrBadScopes = errors.New("auth: a token's scopes are play, create_game and read_metrics, at least one")
	ErrBadName   = errors.New("auth: give the token a name of 1 to 40 characters")
	ErrNoToken   = errors.New("auth: that API token is not one this server knows, or it was revoked")
)

// CreateToken makes an API token for acc and returns it: shown once.
func (a *Accounts) CreateToken(ctx context.Context, acc store.Account, name string, scopes []string, ip string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 40 {
		return "", ErrBadName
	}
	if len(scopes) == 0 {
		return "", ErrBadScopes
	}
	for _, s := range scopes {
		if !slices.Contains(AllScopes, s) {
			return "", ErrBadScopes
		}
	}
	token := APITokenPrefix + newSecret()
	if _, err := a.st.CreateAPIToken(ctx, acc.ID, name, hashSecret(token), scopes); err != nil {
		return "", err
	}
	a.audit(ctx, acc.ID, "token-created", name+" ("+strings.Join(scopes, ",")+")", ip)
	return token, nil
}

// Tokens lists acc's live tokens.
func (a *Accounts) Tokens(ctx context.Context, acc store.Account) ([]store.APIToken, error) {
	return a.st.APITokens(ctx, acc.ID)
}

// RevokeToken ends one of acc's tokens.
func (a *Accounts) RevokeToken(ctx context.Context, acc store.Account, id int64, ip string) error {
	if err := a.st.RevokeAPIToken(ctx, acc.ID, id); err != nil {
		return err
	}
	a.audit(ctx, acc.ID, "token-revoked", "", ip)
	return nil
}

// tokenAccount is the account and scopes of a live API token.
func (a *Accounts) tokenAccount(ctx context.Context, token string) (store.Account, []string, error) {
	return a.st.APITokenAccount(ctx, hashSecret(token), a.now(), touchEvery)
}

// SeedDevToken gives the first administrator the well-known development
// token, once. Only under BINGLE_DEV, which is only allowed on loopback.
func (a *Accounts) SeedDevToken(ctx context.Context, handle string) error {
	if exists, err := a.st.APITokenExists(ctx, hashSecret(DevAPIToken)); err != nil || exists {
		return err
	}
	acc, err := a.st.AccountByHandle(ctx, handle)
	if err != nil {
		return err
	}
	if _, err := a.st.CreateAPIToken(ctx, acc.ID, "BINGLE_DEV", hashSecret(DevAPIToken), []string{ScopePlay, ScopeCreateGame}); err != nil {
		return err
	}
	a.audit(ctx, acc.ID, "token-created", "BINGLE_DEV", "")
	return nil
}
