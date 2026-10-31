package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Account statuses.
const (
	AccountPending = "pending" // the address is not confirmed yet
	AccountActive  = "active"
)

// GroupAdmin is the administrators' group: it holds every capability.
const GroupAdmin = "admin"

// Account is an accounts row.
type Account struct {
	ID                 int64
	Handle             string
	DisplayName        string
	Email              string
	EmailVerified      bool
	Status             string
	MustChangePassword bool
	PrincipalID        string
	CreatedAt          string
	// HasTOTP says the account signs in with a second factor; Admin that it
	// is in the admin group.
	HasTOTP bool
	Admin   bool
}

// ErrTaken is returned when a handle or an address is already an account's.
var ErrTaken = errors.New("store: already taken")

// accountCols are an account's columns, the accounts table being called t.
func accountCols(t string) string {
	return t + `.id, ` + t + `.handle, ` + t + `.display_name, ` + t + `.email, ` + t + `.email_verified_at IS NOT NULL, ` + t + `.status, ` +
		t + `.must_change_password, ` + t + `.principal_id, ` + t + `.created_at, ` +
		`EXISTS (SELECT 1 FROM credentials c WHERE c.account_id = ` + t + `.id AND c.kind = 'totp'), ` +
		`EXISTS (SELECT 1 FROM account_groups g WHERE g.account_id = ` + t + `.id AND g.group_name = '` + GroupAdmin + `')`
}

var accountColumns = accountCols("accounts")

func (a *Account) fields() []any {
	return []any{&a.ID, &a.Handle, &a.DisplayName, &a.Email, &a.EmailVerified, &a.Status, &a.MustChangePassword, &a.PrincipalID, &a.CreatedAt, &a.HasTOTP, &a.Admin}
}

func scanAccount(row interface{ Scan(...any) error }) (Account, error) {
	var a Account
	err := row.Scan(a.fields()...)
	if errors.Is(err, sql.ErrNoRows) {
		return Account{}, ErrNotFound
	}
	return a, err
}

// NewAccount is what CreateAccount needs.
type NewAccount struct {
	Handle, DisplayName, Email string
	Verified                   bool
	MustChangePassword         bool
	// PrincipalID is the principal the account takes: a guest's, which
	// becomes the account's when the account is confirmed, or a new one.
	PrincipalID string
	// NewPrincipal says PrincipalID is to be created (rather than a guest's
	// that exists).
	NewPrincipal bool
	PasswordHash string
	Groups       []string
	// OIDC is a provider's identity ("provider:subject") the account signs
	// in with, when a provider made it.
	OIDC string
}

// CountAccounts is how many accounts there are, deleted ones aside.
func (s *Store) CountAccounts(ctx context.Context) (int, error) {
	var n int
	err := s.queryRow(ctx, "CountAccounts", `SELECT COUNT(*) FROM accounts WHERE deleted_at IS NULL`).Scan(&n)
	return n, err
}

// CreateAccount records an account, its password and its groups, in one
// transaction. ErrTaken when the handle or the address is somebody's.
func (s *Store) CreateAccount(ctx context.Context, a NewAccount) (int64, error) {
	tx, err := s.begin(ctx, "CreateAccount")
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	status := AccountPending
	var verified any
	if a.Verified {
		status, verified = AccountActive, time.Now().UTC().Format(sqlTime)
	}
	res, err := tx.exec(ctx, "CreateAccount.account",
		`INSERT INTO accounts (handle, handle_lower, display_name, email, email_lower, email_verified_at, status, must_change_password, principal_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.Handle, strings.ToLower(a.Handle), a.DisplayName, a.Email, strings.ToLower(a.Email), verified, status, a.MustChangePassword, a.PrincipalID)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return 0, ErrTaken
		}
		return 0, err
	}
	id, _ := res.LastInsertId()
	switch {
	case a.NewPrincipal:
		if _, err := tx.exec(ctx, "CreateAccount.principal", `INSERT INTO principals (id, kind, account_id) VALUES (?, ?, ?)`, a.PrincipalID, KindAccount, id); err != nil {
			return 0, err
		}
	case a.Verified:
		if err := becomeAccount(ctx, tx, a.PrincipalID, id); err != nil {
			return 0, err
		}
	}
	// An account made by a provider has no password until it sets one.
	if a.PasswordHash != "" {
		if _, err := tx.exec(ctx, "CreateAccount.password", `INSERT INTO credentials (account_id, kind, secret) VALUES (?, 'password', ?)`, id, a.PasswordHash); err != nil {
			return 0, err
		}
	}
	if a.OIDC != "" {
		if _, err := tx.exec(ctx, "CreateAccount.oidc", `INSERT INTO credentials (account_id, kind, secret) VALUES (?, 'oidc', ?)`, id, a.OIDC); err != nil {
			if strings.Contains(err.Error(), "UNIQUE") {
				return 0, ErrTaken
			}
			return 0, err
		}
	}
	for _, g := range a.Groups {
		if _, err := tx.exec(ctx, "CreateAccount.group", `INSERT INTO account_groups (account_id, group_name) VALUES (?, ?)`, id, g); err != nil {
			return 0, err
		}
	}
	return id, tx.Commit()
}

// becomeAccount turns a guest's principal into the account's: the guest's
// seats, games and lines are the account's from now on, without a rewrite.
func becomeAccount(ctx context.Context, tx *Tx, principalID string, accountID int64) error {
	_, err := tx.exec(ctx, "becomeAccount", `UPDATE principals SET kind = ?, account_id = ? WHERE id = ?`, KindAccount, accountID, principalID)
	return err
}

// AccountByHandle finds an account by its handle, in any case.
func (s *Store) AccountByHandle(ctx context.Context, handle string) (Account, error) {
	return scanAccount(s.queryRow(ctx, "AccountByHandle", `SELECT `+accountColumns+` FROM accounts WHERE handle_lower = ? AND deleted_at IS NULL`, strings.ToLower(handle)))
}

// PrincipalHasAccount reports whether any account, pending or not, has the
// principal.
func (s *Store) PrincipalHasAccount(ctx context.Context, principalID string) (bool, error) {
	var n int
	err := s.queryRow(ctx, "PrincipalHasAccount", `SELECT COUNT(*) FROM accounts WHERE principal_id = ? AND deleted_at IS NULL`, principalID).Scan(&n)
	return n > 0, err
}

// AccountByID finds an account.
func (s *Store) AccountByID(ctx context.Context, id int64) (Account, error) {
	return scanAccount(s.queryRow(ctx, "AccountByID", `SELECT `+accountColumns+` FROM accounts WHERE id = ? AND deleted_at IS NULL`, id))
}

// ListAccounts lists every account, oldest first.
func (s *Store) ListAccounts(ctx context.Context) ([]Account, error) {
	rows, err := s.query(ctx, "ListAccounts", `SELECT `+accountColumns+` FROM accounts WHERE deleted_at IS NULL ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Account
	for rows.Next() {
		a, err := scanAccount(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// Groups lists the groups an account is in.
func (s *Store) Groups(ctx context.Context, accountID int64) ([]string, error) {
	rows, err := s.query(ctx, "Groups", `SELECT group_name FROM account_groups WHERE account_id = ? ORDER BY group_name`, accountID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var g string
		if err := rows.Scan(&g); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// PasswordHash is the encoded hash of an account's password.
func (s *Store) PasswordHash(ctx context.Context, accountID int64) (string, error) {
	var h string
	err := s.queryRow(ctx, "PasswordHash", `SELECT secret FROM credentials WHERE account_id = ? AND kind = 'password' ORDER BY id DESC LIMIT 1`, accountID).Scan(&h)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return h, err
}

// SetPassword replaces an account's password and says whether it must be
// changed at the next sign-in.
func (s *Store) SetPassword(ctx context.Context, accountID int64, hash string, mustChange bool) error {
	tx, err := s.begin(ctx, "SetPassword")
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.exec(ctx, "SetPassword.delete", `DELETE FROM credentials WHERE account_id = ? AND kind = 'password'`, accountID); err != nil {
		return err
	}
	if _, err := tx.exec(ctx, "SetPassword.insert", `INSERT INTO credentials (account_id, kind, secret) VALUES (?, 'password', ?)`, accountID, hash); err != nil {
		return err
	}
	if _, err := tx.exec(ctx, "SetPassword.flag", `UPDATE accounts SET must_change_password = ? WHERE id = ?`, mustChange, accountID); err != nil {
		return err
	}
	return tx.Commit()
}

// ActivateAccount marks an account's address confirmed and makes its
// principal an account's.
func (s *Store) ActivateAccount(ctx context.Context, accountID int64, now time.Time) error {
	tx, err := s.begin(ctx, "ActivateAccount")
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var principal string
	if err := tx.queryRow(ctx, "ActivateAccount.read", `SELECT principal_id FROM accounts WHERE id = ?`, accountID).Scan(&principal); err != nil {
		return err
	}
	if _, err := tx.exec(ctx, "ActivateAccount.account", `UPDATE accounts SET status = ?, email_verified_at = ? WHERE id = ?`, AccountActive, stamp(now), accountID); err != nil {
		return err
	}
	if err := becomeAccount(ctx, tx, principal, accountID); err != nil {
		return err
	}
	return tx.Commit()
}

// DeletePendingAccounts removes accounts never confirmed since before, so an
// address or a handle typed by mistake is free again.
func (s *Store) DeletePendingAccounts(ctx context.Context, before time.Time) error {
	return s.deletePending(ctx, `SELECT id FROM accounts WHERE status = '`+AccountPending+`' AND created_at < ?`, stamp(before))
}

// DeletePendingAccount removes one account that was never confirmed.
func (s *Store) DeletePendingAccount(ctx context.Context, id int64) error {
	return s.deletePending(ctx, `SELECT id FROM accounts WHERE status = '`+AccountPending+`' AND id = ?`, id)
}

func (s *Store) deletePending(ctx context.Context, sel string, arg any) error {
	tx, err := s.begin(ctx, "DeletePendingAccounts")
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, q := range []string{
		`DELETE FROM credentials WHERE account_id IN (` + sel + `)`,
		`DELETE FROM account_tokens WHERE account_id IN (` + sel + `)`,
		`DELETE FROM account_groups WHERE account_id IN (` + sel + `)`,
		`DELETE FROM accounts WHERE id IN (` + sel + `)`,
	} {
		if _, err := tx.exec(ctx, "DeletePendingAccounts", q, arg); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ---- sessions --------------------------------------------------------------

// CreateSession records a signed-in browser.
func (s *Store) CreateSession(ctx context.Context, idHash string, accountID int64, expires time.Time, ipHash, agent string) error {
	_, err := s.exec(ctx, "CreateSession", `INSERT INTO sessions (id_hash, account_id, expires_at, ip_hash, agent) VALUES (?, ?, ?, ?, ?)`,
		idHash, accountID, stamp(expires), ipHash, agent)
	return err
}

// SessionAccount is the active account a live session belongs to: not past
// its end, and seen since idleSince. Its last sighting is written at most
// once every touchEvery.
func (s *Store) SessionAccount(ctx context.Context, idHash string, idleSince, now time.Time, touchEvery time.Duration) (Account, error) {
	var lastSeen string
	var a Account
	err := s.queryRow(ctx, "SessionAccount",
		`SELECT s.last_seen_at, `+accountCols("a")+` FROM sessions s JOIN accounts a ON a.id = s.account_id
		 WHERE s.id_hash = ? AND s.expires_at > ? AND s.last_seen_at >= ? AND a.status = ? AND a.deleted_at IS NULL`,
		idHash, stamp(now), stamp(idleSince), AccountActive).Scan(append([]any{&lastSeen}, a.fields()...)...)
	if errors.Is(err, sql.ErrNoRows) {
		return Account{}, ErrNotFound
	}
	if err != nil {
		return Account{}, err
	}
	if lastSeen < stamp(now.Add(-touchEvery)) {
		if _, err := s.exec(ctx, "SessionAccount.touch", `UPDATE sessions SET last_seen_at = ? WHERE id_hash = ?`, stamp(now), idHash); err != nil {
			return Account{}, err
		}
	}
	return a, nil
}

// DeleteSession signs a browser out.
func (s *Store) DeleteSession(ctx context.Context, idHash string) error {
	_, err := s.exec(ctx, "DeleteSession", `DELETE FROM sessions WHERE id_hash = ?`, idHash)
	return err
}

// DeleteSessions signs an account out everywhere, except the session
// keepHash ("" for none).
func (s *Store) DeleteSessions(ctx context.Context, accountID int64, keepHash string) error {
	_, err := s.exec(ctx, "DeleteSessions", `DELETE FROM sessions WHERE account_id = ? AND id_hash != ?`, accountID, keepHash)
	return err
}

// CountSessions is how many sessions an account has (tests and the account
// page).
func (s *Store) CountSessions(ctx context.Context, accountID int64) (int, error) {
	var n int
	err := s.queryRow(ctx, "CountSessions", `SELECT COUNT(*) FROM sessions WHERE account_id = ?`, accountID).Scan(&n)
	return n, err
}

// ---- one-time tokens ---------------------------------------------------------

// CreateAccountToken records a one-time token of the kind, by hash.
func (s *Store) CreateAccountToken(ctx context.Context, hash string, accountID int64, kind string, expires time.Time) error {
	_, err := s.exec(ctx, "CreateAccountToken", `INSERT INTO account_tokens (hash, account_id, kind, expires_at) VALUES (?, ?, ?, ?)`, hash, accountID, kind, stamp(expires))
	return err
}

// PeekAccountToken is the account a live token of the kind is for, without
// spending it.
func (s *Store) PeekAccountToken(ctx context.Context, hash, kind string, now time.Time) (int64, error) {
	var id int64
	err := s.queryRow(ctx, "PeekAccountToken", `SELECT account_id FROM account_tokens WHERE hash = ? AND kind = ? AND used_at IS NULL AND expires_at > ?`, hash, kind, stamp(now)).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return id, err
}

// UseAccountToken spends a token of the kind: once, and before it expires.
// It returns the account the token was for; ErrNotFound when it is unknown,
// spent or out of date.
func (s *Store) UseAccountToken(ctx context.Context, hash, kind string, now time.Time) (int64, error) {
	res, err := s.exec(ctx, "UseAccountToken", `UPDATE account_tokens SET used_at = ? WHERE hash = ? AND kind = ? AND used_at IS NULL AND expires_at > ?`, stamp(now), hash, kind, stamp(now))
	if err != nil {
		return 0, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return 0, ErrNotFound
	}
	var id int64
	err = s.queryRow(ctx, "UseAccountToken.read", `SELECT account_id FROM account_tokens WHERE hash = ?`, hash).Scan(&id)
	return id, err
}

// ---- principals ---------------------------------------------------------------

// MergePrincipal moves everything that belongs to from — seats, games, chat
// lines — to into, and retires from: its keys stop working and it records
// where it went.
func (s *Store) MergePrincipal(ctx context.Context, from, into string) error {
	if from == "" || from == into {
		return nil
	}
	tx, err := s.begin(ctx, "MergePrincipal")
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, q := range []string{
		`UPDATE players SET principal_id = ? WHERE principal_id = ?`,
		`UPDATE games SET owner_principal_id = ? WHERE owner_principal_id = ?`,
		`UPDATE chat_messages SET principal_id = ? WHERE principal_id = ?`,
		`UPDATE global_chat SET principal_id = ? WHERE principal_id = ?`,
		`UPDATE principals SET merged_into = ? WHERE id = ?`,
	} {
		if _, err := tx.exec(ctx, "MergePrincipal", q, into, from); err != nil {
			return err
		}
	}
	if _, err := tx.exec(ctx, "MergePrincipal.keys", `UPDATE guest_keys SET revoked_at = CURRENT_TIMESTAMP WHERE principal_id = ? AND revoked_at IS NULL`, from); err != nil {
		return err
	}
	return tx.Commit()
}

// RevokeGuestKeys makes a principal's guest keys stop working (the principal
// has become an account, which signs in instead).
func (s *Store) RevokeGuestKeys(ctx context.Context, principalID string) error {
	_, err := s.exec(ctx, "RevokeGuestKeys", `UPDATE guest_keys SET revoked_at = CURRENT_TIMESTAMP WHERE principal_id = ? AND revoked_at IS NULL`, principalID)
	return err
}

// PrincipalKind is a principal's kind (ErrNotFound when there is none).
func (s *Store) PrincipalKind(ctx context.Context, id string) (string, error) {
	var k string
	err := s.queryRow(ctx, "PrincipalKind", `SELECT kind FROM principals WHERE id = ?`, id).Scan(&k)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return k, err
}

// ---- the audit log ---------------------------------------------------------------

// AuthEvent is one auth_events row.
type AuthEvent struct {
	At        string
	AccountID int64
	Kind      string
	Detail    string
}

// AddAuthEvent writes down what happened (accountID 0 for none).
func (s *Store) AddAuthEvent(ctx context.Context, accountID int64, kind, detail, ipHash string) error {
	var acc any
	if accountID != 0 {
		acc = accountID
	}
	_, err := s.exec(ctx, "AddAuthEvent", `INSERT INTO auth_events (account_id, kind, detail, ip_hash) VALUES (?, ?, ?, ?)`, acc, kind, detail, ipHash)
	return err
}

// AuthEvents lists the latest events, newest first.
func (s *Store) AuthEvents(ctx context.Context, limit int) ([]AuthEvent, error) {
	rows, err := s.query(ctx, "AuthEvents", `SELECT at, COALESCE(account_id, 0), kind, detail FROM auth_events ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []AuthEvent
	for rows.Next() {
		var e AuthEvent
		if err := rows.Scan(&e.At, &e.AccountID, &e.Kind, &e.Detail); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ---- the second factor -----------------------------------------------------------

// SetPendingTOTP keeps a TOTP secret an account is setting up, until a code
// from it confirms the app has it.
func (s *Store) SetPendingTOTP(ctx context.Context, accountID int64, secret string) error {
	tx, err := s.begin(ctx, "SetPendingTOTP")
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.exec(ctx, "SetPendingTOTP.delete", `DELETE FROM credentials WHERE account_id = ? AND kind = 'totp-pending'`, accountID); err != nil {
		return err
	}
	if _, err := tx.exec(ctx, "SetPendingTOTP.insert", `INSERT INTO credentials (account_id, kind, secret) VALUES (?, 'totp-pending', ?)`, accountID, secret); err != nil {
		return err
	}
	return tx.Commit()
}

// credential is the secret and params of an account's credential of a kind.
func (s *Store) credential(ctx context.Context, accountID int64, kind string) (string, string, error) {
	var secret, params string
	err := s.queryRow(ctx, "credential", `SELECT secret, params FROM credentials WHERE account_id = ? AND kind = ? ORDER BY id DESC LIMIT 1`, accountID, kind).Scan(&secret, &params)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", ErrNotFound
	}
	return secret, params, err
}

// PendingTOTP is the secret an account is setting up.
func (s *Store) PendingTOTP(ctx context.Context, accountID int64) (string, error) {
	secret, _, err := s.credential(ctx, accountID, "totp-pending")
	return secret, err
}

// TOTP is an account's TOTP secret and the last step a code was used for.
func (s *Store) TOTP(ctx context.Context, accountID int64) (string, int64, error) {
	secret, params, err := s.credential(ctx, accountID, "totp")
	if err != nil {
		return "", 0, err
	}
	var last int64
	_, _ = fmt.Sscan(params, &last)
	return secret, last, nil
}

// UseTOTPStep records that a code of step was used, if no later one has
// been: it reports false when the step is spent, which is a replay.
func (s *Store) UseTOTPStep(ctx context.Context, accountID, step int64) (bool, error) {
	res, err := s.exec(ctx, "UseTOTPStep", `UPDATE credentials SET params = ?, last_used_at = CURRENT_TIMESTAMP
		WHERE account_id = ? AND kind = 'totp' AND CAST(CASE params WHEN '' THEN '0' ELSE params END AS INTEGER) < ?`, fmt.Sprint(step), accountID, step)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// EnableTOTP makes the pending secret the account's second factor, at step
// (the code that confirmed it), with the recovery codes' hashes.
func (s *Store) EnableTOTP(ctx context.Context, accountID int64, step int64, recoveryHashes []string) error {
	tx, err := s.begin(ctx, "EnableTOTP")
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.exec(ctx, "EnableTOTP.old", `DELETE FROM credentials WHERE account_id = ? AND kind IN ('totp', 'recovery')`, accountID); err != nil {
		return err
	}
	res, err := tx.exec(ctx, "EnableTOTP.promote", `UPDATE credentials SET kind = 'totp', params = ? WHERE account_id = ? AND kind = 'totp-pending'`, fmt.Sprint(step), accountID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return ErrNotFound
	}
	if err := insertRecovery(ctx, tx, accountID, recoveryHashes); err != nil {
		return err
	}
	return tx.Commit()
}

func insertRecovery(ctx context.Context, tx *Tx, accountID int64, hashes []string) error {
	for _, h := range hashes {
		if _, err := tx.exec(ctx, "insertRecovery", `INSERT INTO credentials (account_id, kind, secret) VALUES (?, 'recovery', ?)`, accountID, h); err != nil {
			return err
		}
	}
	return nil
}

// ReplaceRecoveryCodes gives an account a fresh set of recovery codes.
func (s *Store) ReplaceRecoveryCodes(ctx context.Context, accountID int64, hashes []string) error {
	tx, err := s.begin(ctx, "ReplaceRecoveryCodes")
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.exec(ctx, "ReplaceRecoveryCodes.delete", `DELETE FROM credentials WHERE account_id = ? AND kind = 'recovery'`, accountID); err != nil {
		return err
	}
	if err := insertRecovery(ctx, tx, accountID, hashes); err != nil {
		return err
	}
	return tx.Commit()
}

// UseRecoveryCode spends a recovery code: it reports false when the account
// has no such code (any more).
func (s *Store) UseRecoveryCode(ctx context.Context, accountID int64, hash string) (bool, error) {
	res, err := s.exec(ctx, "UseRecoveryCode", `DELETE FROM credentials WHERE account_id = ? AND kind = 'recovery' AND secret = ?`, accountID, hash)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// CountRecoveryCodes is how many unused recovery codes an account has.
func (s *Store) CountRecoveryCodes(ctx context.Context, accountID int64) (int, error) {
	var n int
	err := s.queryRow(ctx, "CountRecoveryCodes", `SELECT COUNT(*) FROM credentials WHERE account_id = ? AND kind = 'recovery'`, accountID).Scan(&n)
	return n, err
}

// RemoveSecondFactor takes an account's TOTP and recovery codes away.
func (s *Store) RemoveSecondFactor(ctx context.Context, accountID int64) error {
	_, err := s.exec(ctx, "RemoveSecondFactor", `DELETE FROM credentials WHERE account_id = ? AND kind IN ('totp', 'totp-pending', 'recovery')`, accountID)
	return err
}

// AccountByEmail finds the active account with an address, in any case.
func (s *Store) AccountByEmail(ctx context.Context, email string) (Account, error) {
	return scanAccount(s.queryRow(ctx, "AccountByEmail", `SELECT `+accountColumns+` FROM accounts WHERE email_lower = ? AND status = ? AND deleted_at IS NULL`, strings.ToLower(email), AccountActive))
}

// ---- providers ------------------------------------------------------------------

// OIDCKey is how a provider's identity is stored.
func OIDCKey(provider, subject string) string { return provider + ":" + subject }

// AccountByOIDC is the active account a provider's identity signs in to.
func (s *Store) AccountByOIDC(ctx context.Context, key string) (Account, error) {
	return scanAccount(s.queryRow(ctx, "AccountByOIDC", `SELECT `+accountColumns+` FROM accounts
		WHERE id = (SELECT account_id FROM credentials WHERE kind = 'oidc' AND secret = ?) AND status = ? AND deleted_at IS NULL`, key, AccountActive))
}

// LinkOIDC lets an account sign in with a provider's identity. ErrTaken when
// the identity is another account's.
func (s *Store) LinkOIDC(ctx context.Context, accountID int64, key string) error {
	_, err := s.exec(ctx, "LinkOIDC", `INSERT INTO credentials (account_id, kind, secret) VALUES (?, 'oidc', ?)`, accountID, key)
	if err != nil && strings.Contains(err.Error(), "UNIQUE") {
		return ErrTaken
	}
	return err
}

// UnlinkOIDC stops an account signing in with a provider.
func (s *Store) UnlinkOIDC(ctx context.Context, accountID int64, provider string) error {
	_, err := s.exec(ctx, "UnlinkOIDC", `DELETE FROM credentials WHERE account_id = ? AND kind = 'oidc' AND secret LIKE ?`, accountID, provider+":%")
	return err
}

// OIDCLinks are the providers an account signs in with.
func (s *Store) OIDCLinks(ctx context.Context, accountID int64) ([]string, error) {
	rows, err := s.query(ctx, "OIDCLinks", `SELECT secret FROM credentials WHERE account_id = ? AND kind = 'oidc' ORDER BY id`, accountID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		if p, _, ok := strings.Cut(k, ":"); ok {
			out = append(out, p)
		}
	}
	return out, rows.Err()
}
