package auth

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/noderunner/bingle/internal/store"
)

// A setup file for the bingle-3d client (ADR 0151): the server it came
// from, and the player who asked for it, so a client run for the first time
// connects and starts as that player with nothing to type.
//
//	POST /api/client/setup   the file, for whoever asks
//	POST /auth/login {code}  the client spends the file's sign-in code
//
// The file lands in a downloads folder and stays there, so it never carries
// a session. For an account it carries a sign-in code: good once, and for a
// day. For a guest it carries the guest's key, which is all a guest is.

// SetupFileName is what the setup file is saved as, and the name the client
// looks for.
const SetupFileName = "bingle-3d-setup.toml"

// SetupFormat is the number a setup file says its form is. A client refuses
// one it does not know.
const SetupFormat = 1

// SignInCodePrefix starts every sign-in code, so one is recognisable.
const SignInCodePrefix = "bgc_"

// signInCodeFor is how long a sign-in code waits to be spent: long enough to
// download a client and start it, short enough that a file found later is
// worth nothing.
const signInCodeFor = 24 * time.Hour

// setupsPerHour is how many setup files one account is given an hour: each
// is a row until it is spent or out of date.
const setupsPerHour = 10

// ErrBadSignInCode is the one answer to a sign-in code that does not work,
// whatever the reason.
var ErrBadSignInCode = errors.New("auth: that sign-in code does not work: it was used already, or it is too old. Download the setup file again, or sign in with your password")

// ErrSetupNeedsSession is the answer to an API token asking for a setup
// file: a token acts within its scopes, and the file would be the account.
var ErrSetupNeedsSession = errors.New("auth: an API token cannot ask for a setup file: sign in")

// NewSignInCode makes a code that signs a client in to acc once, within a
// day. It is kept by its hash, as a session is.
func (a *Accounts) NewSignInCode(ctx context.Context, acc store.Account, ip string) (string, error) {
	now := a.now()
	if err := a.st.PruneAccountTokens(ctx, acc.ID, store.SignInCodeKind, now); err != nil {
		return "", err
	}
	code := newSecret()
	if err := a.st.CreateAccountToken(ctx, hashSecret(code), acc.ID, store.SignInCodeKind, now.Add(signInCodeFor)); err != nil {
		return "", err
	}
	a.audit(ctx, acc.ID, "sign-in-code", "", ip)
	return SignInCodePrefix + code, nil
}

// LoginWithCode spends a sign-in code: the client is signed in to the
// account the code was made for, with a session of its own. No second
// factor is asked for: the session that asked for the code had given it.
func (a *Accounts) LoginWithCode(ctx context.Context, code, guest, ip, agent string) (Session, error) {
	now := a.now()
	if !a.loginsByAddr.allow(ip, now) {
		return Session{}, ErrTooManyTries
	}
	id, err := a.st.UseAccountToken(ctx, hashSecret(strings.TrimPrefix(code, SignInCodePrefix)), store.SignInCodeKind, now)
	if errors.Is(err, store.ErrNotFound) {
		a.audit(ctx, 0, "login-failed", "code", ip)
		return Session{}, ErrBadSignInCode
	}
	if err != nil {
		return Session{}, err
	}
	acc, err := a.st.AccountByID(ctx, id)
	if err != nil {
		return Session{}, err
	}
	if acc.Status != store.AccountActive {
		return Session{}, ErrNotConfirmed
	}
	return a.finishLogin(ctx, acc, guest, ip, agent, "code")
}

// SetupFile is what a setup file says.
type SetupFile struct {
	// Server is the server's address, as the browser that asked reached it.
	Server string
	// Name is the account's name, for a signed-in player.
	Name string
	// SignInCode signs the client in to the account, once.
	SignInCode string
	// GuestKey is the guest's key, for a player who is not signed in.
	GuestKey string
	// CertificateSHA256 is the fingerprint of the certificate the server
	// made for itself, which the client trusts for this server.
	CertificateSHA256 string
}

// TOML is the file: a flat table, so a client reads it with any TOML reader.
func (f SetupFile) TOML() []byte {
	var b strings.Builder
	b.WriteString("# Setup for the bingle-3d client. Put this file beside the program, or start it with\n")
	b.WriteString("# --setup <this file>. The client reads it once and then deletes it.\n")
	fmt.Fprintf(&b, "format = %d\n", SetupFormat)
	for _, kv := range [][2]string{{"server", f.Server}, {"name", f.Name}, {"sign_in_code", f.SignInCode}, {"guest_key", f.GuestKey}, {"certificate_sha256", f.CertificateSHA256}} {
		if kv[1] != "" {
			b.WriteString(kv[0] + " = " + tomlString(kv[1]) + "\n")
		}
	}
	return []byte(b.String())
}

// tomlString writes s as a TOML basic string.
func tomlString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"' || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case r < 0x20 || r == 0x7f:
			b.WriteString(`\u` + fmt.Sprintf("%04X", r))
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// serverAddress is the address the request reached the server at, as a
// client is to use it: the scheme, the host and the port if there was one.
func (rs *Resolver) serverAddress(r *http.Request) (string, bool) {
	scheme := rs.transport.Scheme(r)
	u, err := url.Parse(scheme + "://" + r.Host)
	if err != nil || u.Host != r.Host || u.Hostname() == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return "", false
	}
	if port := u.Port(); port != "" {
		if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
			return "", false
		}
	}
	return scheme + "://" + u.Host, true
}

// HandleClientSetup is POST /api/client/setup: a setup file for whoever
// asks. A signed-in account gets its name and a sign-in code, a guest its
// key, and anybody else the server's address alone.
//
// It is a POST, never cached, and refused to an API token. Being a
// state-changing request outside /auth, the middleware has already refused
// an account whose password must be changed and an administrator who has no
// second factor yet, as it refuses a sign-in's other doings.
func (rs *Resolver) HandleClientSetup(w http.ResponseWriter, r *http.Request) {
	server, ok := rs.serverAddress(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "auth: the request does not say which address it came to")
		return
	}
	file := SetupFile{Server: server}
	// The certificate is the one this connection was made with: bingle's own.
	if rs.certificate != nil && r.TLS != nil {
		file.CertificateSHA256 = rs.certificate()
	}
	p, known := From(r.Context())
	switch {
	case known && p.Scopes != nil:
		writeError(w, http.StatusForbidden, ErrSetupNeedsSession.Error())
		return
	case known && p.Account != nil && rs.accounts != nil:
		if !rs.setups.allow(strconv.FormatInt(p.Account.ID, 10), rs.now()) {
			writeError(w, http.StatusTooManyRequests, "auth: too many setup files; try again later")
			return
		}
		code, err := rs.accounts.NewSignInCode(r.Context(), *p.Account, rs.transport.PeerIP(r))
		if err != nil {
			rs.fail(w, err)
			return
		}
		file.Name, file.SignInCode = cmp.Or(p.Account.DisplayName, p.Account.Handle), code
	case known:
		file.GuestKey = cmp.Or(bearerWith(r, GuestKeyPrefix), p.issued)
		if _, key, ok := cookieGuest(r); ok {
			file.GuestKey = key
		}
	}
	h := w.Header()
	h.Set("Content-Type", "application/toml")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": SetupFileName}))
	_, _ = w.Write(file.TOML())
}
