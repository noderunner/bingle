package app_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/noderunner/bingle/internal/logging"
	"github.com/noderunner/bingle/internal/mail/fakesmtp"
)

// waitFor polls cond until it holds or a few seconds pass.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// addrRef is how an address appears in the log: its hash, never itself.
func addrRef(address string) string { return "addr=" + logging.Addr(address).Value.String() }

// The log says, for each mail, that it was queued and sent or why it was
// not, by its kind and the hash of its address; and never the address.
func TestMailLog_SaysWhatBecameOfEachMail(t *testing.T) {
	e := newNotifyEnv(t, fakesmtp.Options{})
	const address = "ana.mail@example.com"
	if !strings.Contains(e.Log.String(), "notify: mail is on") {
		t.Errorf("no line at startup says mail is on:\n%s", e.Log.String())
	}

	e.saveContact(t, address)
	e.waitMails(t, 1)
	waitFor(t, "the sent line", func() bool { return strings.Contains(e.Log.String(), "notify: sent a message") })
	// The same address again at once is held by the per-address limit.
	e.saveContact(t, address)
	waitFor(t, "the limited line", func() bool { return strings.Contains(e.Log.String(), "held back by the sending limits") })

	log := e.Log.String()
	for _, want := range []string{"notify: queued a message", "kind=confirm", addrRef(address)} {
		if !strings.Contains(log, want) {
			t.Errorf("the log has no %q:\n%s", want, log)
		}
	}
	if strings.Contains(strings.ToLower(log), address) || strings.Contains(log, notifyFrom) {
		t.Errorf("the log carries an address:\n%s", log)
	}

	metrics := e.Scrape()
	for _, want := range []string{
		`bingle_mail_total{kind="confirm",outcome="queued"} 1`,
		`bingle_mail_total{kind="confirm",outcome="sent"} 1`,
		`bingle_mail_total{kind="confirm",outcome="limited"} 1`,
		`bingle_mail_send_duration_seconds_count 1`,
	} {
		if !strings.Contains(metrics, want) {
			t.Errorf("the metrics have no %q:\n%s", want, firstLines(metrics, "bingle_mail"))
		}
	}
}

// A refused login is an error in the log and a count, and so is every mail
// not tried after it.
func TestMailLog_ARefusedLoginIsLoggedAndCounted(t *testing.T) {
	e := newNotifyEnv(t, fakesmtp.Options{AuthFail: true})
	e.saveContact(t, "a@example.com")
	waitFor(t, "the login failure", func() bool { return strings.Contains(e.Scrape(), `outcome="auth_failed"} 1`) })
	e.saveContact(t, "b@example.com")
	waitFor(t, "the stopped mail", func() bool { return strings.Contains(e.Scrape(), `outcome="stopped"} 1`) })
	log := e.Log.String()
	for _, want := range []string{"level=ERROR", "refused the login", "sending is stopped until bingle is restarted"} {
		if !strings.Contains(log, want) {
			t.Errorf("the log has no %q:\n%s", want, log)
		}
	}
}

// Signing up with an address that already has an account answers as any
// sign-up does and sends no confirmation; the log and the audit log say so,
// or the operator is left wondering where the mail went.
func TestMailLog_ASignUpWithAnAddressInUseSaysWhyNoMailWent(t *testing.T) {
	e := newNotifyEnv(t, fakesmtp.Options{})
	signUp(t, e, newBrowser(t, e.URL), "ana", "ana@example.com")
	before := len(e.SMTP.Mails())

	b := newBrowser(t, e.URL)
	if code := b.do("POST", "/auth/signup", map[string]any{"handle": "bob", "email": "Ana@Example.com", "password": "correct horse battery"}, nil); code != http.StatusAccepted {
		t.Fatalf("an address in use: %d, want 202", code)
	}
	e.waitMails(t, before+1) // the owner's "you already have an account"
	waitFor(t, "the notice", func() bool { return strings.Contains(e.Scrape(), `bingle_mail_total{kind="notice",outcome="sent"} 1`) })

	log := e.Log.String()
	if !strings.Contains(log, "the address already belongs to an account") || !strings.Contains(log, addrRef("ana@example.com")) {
		t.Errorf("the log does not say why no mail went:\n%s", log)
	}
	if strings.Contains(strings.ToLower(log), "ana@example.com") {
		t.Errorf("the log carries the address:\n%s", log)
	}
	events, err := e.App.Accounts().Events(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, ev := range events {
		found = found || (ev.Kind == "signup-address-in-use" && ev.AccountID != 0)
	}
	if !found {
		t.Errorf("no signup-address-in-use event against the owner's account: %+v", events)
	}
}
