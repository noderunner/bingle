package mail_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/noderunner/bingle/internal/config"
	"github.com/noderunner/bingle/internal/mail"
	"github.com/noderunner/bingle/internal/mail/fakesmtp"
)

func TestValidAddress(t *testing.T) {
	good := map[string]string{
		"a@example.com":                      "a@example.com",
		" Bob.Smith+games@Mail.Example.ORG ": "Bob.Smith+games@mail.example.org",
		"bingle_games@proton.me":             "bingle_games@proton.me",
	}
	for in, want := range good {
		if got, err := mail.ValidAddress(in); err != nil || got != want {
			t.Errorf("ValidAddress(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	bad := []string{
		"", "plain", "@example.com", "a@", "a@example", "a@.com", "a@example..com", "a@-example.com",
		"a b@example.com", "a@example.com\r\nBcc: x@y.com", "a@example.com\nBcc: x@y.com",
		"a@example.com,b@example.com", "a@example.com;b@example.com", "Bob <a@example.com>", "<a@example.com>",
		`"a b"@example.com`, "a(comment)@example.com", "a@exa mple.com", "a@example.com>", "ä@example.com", "a@exämple.com",
		strings.Repeat("a", 65) + "@example.com", "a@" + strings.Repeat("b", 250) + ".com",
	}
	for _, in := range bad {
		if got, err := mail.ValidAddress(in); err == nil {
			t.Errorf("ValidAddress(%q) accepted as %q", in, got)
		}
	}
}

func TestMaskHidesTheAddress(t *testing.T) {
	if got := mail.Mask("alex@example.com"); got != "a••••@example.com" {
		t.Errorf("Mask = %q", got)
	}
	if strings.Contains(mail.Mask("alex.morgan@example.com"), "morgan") {
		t.Error("the mask leaks the name")
	}
}

func TestSanitizeKeepsValuesHarmless(t *testing.T) {
	for in, want := range map[string]string{
		"Tic-Tac-Toe":               "Tic-Tac-Toe",
		"Tak (5×5)":                 "Tak (5×5)",
		"evil\r\nBcc: a@b.c":        "evil Bcc: ab.c",
		"<script>alert(1)</script>": "scriptalert(1)script",
		"  lots   of\tspace ":       "lots of space",
	} {
		if got := mail.Sanitize(in, 60); got != want {
			t.Errorf("Sanitize(%q) = %q, want %q", in, got, want)
		}
	}
	if got := mail.Sanitize(strings.Repeat("x", 100), 10); len(got) != 10 {
		t.Errorf("not capped: %q", got)
	}
}

func TestBuildRejectsInjectionAndFixesTheHeaders(t *testing.T) {
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	m := mail.Turn("p@example.com", "Tic-Tac-Toe", "ABC123", "https://bingle.example.com", "stoptoken")
	raw, err := m.Build("bingle_games@proton.me", "Bingle", now)
	if err != nil {
		t.Fatal(err)
	}
	head, body, _ := strings.Cut(string(raw), "\r\n\r\n")
	for _, h := range []string{"From: Bingle <bingle_games@proton.me>", "To: p@example.com", "Subject: It is your turn in Tic-Tac-Toe (ABC123)", "MIME-Version: 1.0",
		"Content-Type: text/plain; charset=utf-8", "List-Unsubscribe: <https://bingle.example.com/notify/stop?t=stoptoken>", "List-Unsubscribe-Post: List-Unsubscribe=One-Click"} {
		if !strings.Contains(head, h) {
			t.Errorf("header %q missing from:\n%s", h, head)
		}
	}
	for _, h := range strings.Split(head, "\r\n") {
		k := strings.SplitN(h, ":", 2)[0]
		switch k {
		case "From", "To", "Subject", "Date", "Message-ID", "MIME-Version", "Content-Type", "Content-Transfer-Encoding", "Auto-Submitted", "List-Unsubscribe", "List-Unsubscribe-Post":
		default:
			t.Errorf("unexpected header %q", h)
		}
	}
	if !strings.Contains(body, "https://bingle.example.com/game/ABC123") || !strings.Contains(body, "/notify/stop?t=stoptoken") {
		t.Errorf("body = %q", body)
	}
	if strings.Contains(head, "Bcc") || strings.Count(head, "To:") != 1 {
		t.Error("more than one recipient header")
	}

	// A game name or room code with line breaks cannot add headers or recipients.
	evil := mail.Turn("p@example.com", "Game\r\nBcc: victim@example.com", "AB\r\nCD", "https://bingle.example.com", "t")
	raw, err = evil.Build("bingle_games@proton.me", "Bingle", now)
	if err != nil {
		t.Fatal(err)
	}
	head, _, _ = strings.Cut(string(raw), "\r\n\r\n")
	for _, line := range strings.Split(head, "\r\n") {
		if strings.HasPrefix(line, "Bcc") || strings.Contains(line, "victim@example.com") {
			t.Errorf("header injection got through:\n%s", head)
		}
	}
	if strings.Count(head, "\r\nSubject: ") != 1 || strings.Count(head, "\r\nTo: ") != 1 {
		t.Errorf("headers: %s", head)
	}

	// Bad recipients, senders, subjects and links are refused outright.
	for name, bad := range map[string]mail.Message{
		"recipient with a line break": {To: "a@example.com\r\nBcc: b@example.com", Subject: "x", Body: "y"},
		"two recipients":              {To: "a@example.com,b@example.com", Subject: "x", Body: "y"},
		"subject with a line break":   {To: "a@example.com", Subject: "x\r\nBcc: b@example.com", Body: "y"},
		"no subject":                  {To: "a@example.com", Body: "y"},
		"javascript unsubscribe":      {To: "a@example.com", Subject: "x", Body: "y", UnsubscribeURL: "javascript:alert(1)"},
		"unsubscribe with a break":    {To: "a@example.com", Subject: "x", Body: "y", UnsubscribeURL: "https://e.com/\r\nBcc: b@example.com"},
	} {
		if _, err := bad.Build("bingle_games@proton.me", "Bingle", now); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if _, err := (mail.Message{To: "a@example.com", Subject: "x", Body: "y"}).Build("not an address", "Bingle", now); err == nil {
		t.Error("a bad sender was accepted")
	}
}

func mailConfig(srv *fakesmtp.Server, mode string) config.Mail {
	return config.Mail{
		Host: srv.Host, Port: srv.Port, User: "bingle_games@proton.me", Password: config.NewSecret("s3cret-pw"),
		From: "bingle_games@proton.me", Mode: mode, CAFile: srv.CAFile,
	}
}

func send(t *testing.T, cfg config.Mail, to string) error {
	t.Helper()
	s, err := mail.NewSender(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return s.Send(ctx, mail.Confirmation(to, "https://bingle.example.com", "ctoken", "stoken"))
}

func TestSendOverSTARTTLS(t *testing.T) {
	srv := fakesmtp.Start(t, fakesmtp.Options{})
	if err := send(t, mailConfig(srv, "starttls"), "player@example.com"); err != nil {
		t.Fatal(err)
	}
	got := srv.Mails()
	if len(got) != 1 {
		t.Fatalf("%d mails", len(got))
	}
	m := got[0]
	if m.From != "bingle_games@proton.me" || len(m.To) != 1 || m.To[0] != "player@example.com" {
		t.Errorf("envelope: from %q to %v", m.From, m.To)
	}
	if m.User != "bingle_games@proton.me" || m.Pass != "s3cret-pw" || m.AuthedInClear {
		t.Errorf("auth: %+v (the password must be sent only inside TLS)", m)
	}
	if !strings.Contains(m.Data, "Subject: Confirm your email address for Bingle") || !strings.Contains(m.Data, "/notify/confirm?t=ctoken") {
		t.Errorf("data:\n%s", m.Data)
	}
}

func TestSendOverImplicitTLSAndLoginAuth(t *testing.T) {
	srv := fakesmtp.Start(t, fakesmtp.Options{Implicit: true, LoginOnly: true})
	if err := send(t, mailConfig(srv, "tls"), "player@example.com"); err != nil {
		t.Fatal(err)
	}
	if got := srv.Mails(); len(got) != 1 || got[0].Pass != "s3cret-pw" || got[0].AuthedInClear {
		t.Errorf("mails = %+v", got)
	}
}

func TestSendRefusesWithoutSTARTTLS(t *testing.T) {
	srv := fakesmtp.Start(t, fakesmtp.Options{NoSTARTTLS: true})
	err := send(t, mailConfig(srv, "starttls"), "player@example.com")
	if !errors.Is(err, mail.ErrNoSTARTTLS) {
		t.Fatalf("err = %v, want ErrNoSTARTTLS", err)
	}
	if srv.AuthAttempts() != 0 || len(srv.Mails()) != 0 {
		t.Errorf("the password or a message went out in the clear: %d auths, %d mails", srv.AuthAttempts(), len(srv.Mails()))
	}
}

func TestSendRefusesAnUntrustedCertificate(t *testing.T) {
	srv := fakesmtp.Start(t, fakesmtp.Options{})
	cfg := mailConfig(srv, "starttls")
	cfg.CAFile = "" // the system's roots do not know the test certificate
	err := send(t, cfg, "player@example.com")
	if err == nil {
		t.Fatal("sent through a server with an untrusted certificate")
	}
	if srv.AuthAttempts() != 0 {
		t.Error("the password was offered to an untrusted server")
	}
}

func TestSendErrorsNeverContainThePassword(t *testing.T) {
	srv := fakesmtp.Start(t, fakesmtp.Options{AuthFail: true})
	err := send(t, mailConfig(srv, "starttls"), "player@example.com")
	if err == nil {
		t.Fatal("login accepted")
	}
	if strings.Contains(err.Error(), "s3cret-pw") {
		t.Errorf("error leaks the password: %v", err)
	}
	if mail.Classify(err) != mail.BadCredentials {
		t.Errorf("Classify(%v) = %v, want BadCredentials", err, mail.Classify(err))
	}
}

func TestClassify(t *testing.T) {
	srv := fakesmtp.Start(t, fakesmtp.Options{RcptReply: "550 5.1.1 no such user"})
	err := send(t, mailConfig(srv, "starttls"), "player@example.com")
	if err == nil || mail.Classify(err) != mail.Transient {
		t.Errorf("refused recipient: %v (class %v), want a transient error", err, mail.Classify(err))
	}
	srv = fakesmtp.Start(t, fakesmtp.Options{RcptReply: "450 4.2.1 mailbox busy"})
	if err := send(t, mailConfig(srv, "starttls"), "player@example.com"); mail.Classify(err) != mail.RateLimited {
		t.Errorf("450: class %v, want RateLimited", mail.Classify(err))
	}
	if mail.Classify(errors.New("dial tcp: refused")) != mail.Transient {
		t.Error("a network error is not transient")
	}
}

func TestSendRefusesBadRecipientsBeforeConnecting(t *testing.T) {
	srv := fakesmtp.Start(t, fakesmtp.Options{})
	for _, to := range []string{"a@example.com,b@example.com", "a@example.com\r\nBcc: b@example.com", "nobody"} {
		if err := send(t, mailConfig(srv, "starttls"), to); err == nil {
			t.Errorf("sent to %q", to)
		}
	}
	if srv.Connections() != 0 {
		t.Errorf("connected %d times for invalid recipients", srv.Connections())
	}
}

func TestConfigSecretIsNeverPrinted(t *testing.T) {
	dir := t.TempDir()
	pw := filepath.Join(dir, "pw")
	if err := os.WriteFile(pw, []byte("hunter2-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for k, v := range map[string]string{"BINGLE_SMTP_HOST": "smtp.example.com", "BINGLE_SMTP_PASSWORD_FILE": pw, "BINGLE_TLS_EMAIL": "bingle_games@proton.me"} {
		t.Setenv(k, v)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Mail.Enabled() || cfg.Mail.Password.Reveal() != "hunter2-secret" {
		t.Fatalf("mail config = %+v", cfg.Mail)
	}
	for _, s := range []string{
		cfg.Mail.Password.String(), cfg.Mail.Password.GoString(),
		sprint("%v", cfg), sprint("%+v", cfg), sprint("%#v", cfg), sprint("%s", cfg.Mail), sprint("%v", cfg.Mail.Password),
		sprintJSON(t, cfg),
	} {
		if strings.Contains(s, "hunter2") {
			t.Errorf("the password shows up in %q", s)
		}
	}
}

// The mail to an address someone tried to sign up with again names the
// account, links only to the public sign-in pages and carries no token.
func TestAccountExistsMail(t *testing.T) {
	m := mail.AccountExists("p@example.com", "ana\r\nBcc: x", "https://bingle.example.com")
	raw, err := m.Build("bingle_games@proton.me", "Bingle", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	for _, want := range []string{"You already have a Bingle account", "@ana", "https://bingle.example.com/login", "https://bingle.example.com/forgot"} {
		if !strings.Contains(s, want) {
			t.Errorf("no %q in:\n%s", want, s)
		}
	}
	if strings.Contains(s, "?t=") || strings.Contains(s, "\r\nBcc") {
		t.Errorf("a token or a header in:\n%s", s)
	}
}
