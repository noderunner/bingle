package app_test

import (
	"io"
	"mime"
	"mime/quotedprintable"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/noderunner/bingle/internal/i18n"
	"github.com/noderunner/bingle/internal/mail/fakesmtp"
)

// inLanguage gives a test browser the language cookie, as the Language setting would.
func (b *browser) inLanguage(code string) {
	u, _ := url.Parse(b.base)
	b.c.Jar.SetCookies(u, []*http.Cookie{{Name: i18n.Cookie, Value: code, Path: "/"}})
}

// readMail is a mail's subject and its text, decoded.
func readMail(t *testing.T, raw string) (subject, text string) {
	t.Helper()
	head, body, _ := strings.Cut(raw, "\r\n\r\n")
	for _, line := range strings.Split(head, "\r\n") {
		if v, ok := strings.CutPrefix(line, "Subject: "); ok {
			s, err := new(mime.WordDecoder).DecodeHeader(v)
			if err != nil {
				t.Fatalf("subject %q: %v", v, err)
			}
			subject = s
		}
	}
	if strings.Contains(head, "quoted-printable") {
		b, err := io.ReadAll(quotedprintable.NewReader(strings.NewReader(body)))
		if err != nil {
			t.Fatal(err)
		}
		body = string(b)
	}
	return subject, body
}

// Mail is in the reader's language (ADR 0141): the browser's that asked for it,
// and later the language the account chose, else its browser's as it last saw the site.
func TestMail_IsInTheReadersLanguage(t *testing.T) {
	e := newNotifyEnv(t, fakesmtp.Options{})

	// An address set up for turn notices from a French browser.
	guest := newBrowser(t, e.URL)
	guest.inLanguage("fr")
	before := len(e.SMTP.Mails())
	if code := guest.do("POST", "/api/notify/contact", map[string]any{"address": "lea@example.com"}, nil); code != 200 {
		t.Fatalf("the contact: %d", code)
	}
	subject, text := readMail(t, e.waitMails(t, before+1)[before].Data)
	if subject != "Confirmez votre adresse e-mail pour Bingle" || !strings.Contains(text, "Pour l’activer, ouvrez le lien") || !strings.Contains(text, "/notify/confirm?t=") {
		t.Errorf("the confirmation in French: %q\n%s", subject, text)
	}

	// A new account's confirmation, from a Japanese browser.
	ken := newBrowser(t, e.URL)
	ken.inLanguage("ja")
	before = len(e.SMTP.Mails())
	if code := ken.do("POST", "/auth/signup", map[string]any{"handle": "ken", "email": "ken@example.com", "password": "correct horse battery"}, nil); code != http.StatusAccepted {
		t.Fatalf("Ken signs up: %d", code)
	}
	if subject, text := readMail(t, e.waitMails(t, before+1)[before].Data); subject != "Bingle アカウントの確認" || !strings.Contains(text, "@ken") {
		t.Errorf("the account's confirmation in Japanese: %q\n%s", subject, text)
	}

	// An account that chose no language is written to in its browser's, as the site last saw it.
	bob := newBrowser(t, e.URL)
	bobPw := signUp(t, e, bob, "bob", "bob@example.com")
	if code := bob.login("bob", bobPw); code != 200 {
		t.Fatalf("Bob signs in: %d", code)
	}
	bob.inLanguage("de")
	if code := bob.do("GET", "/auth/me", nil, nil); code != 200 {
		t.Fatalf("Bob's page asks who he is: %d", code)
	}
	if code := bob.do("PUT", "/api/friends/settings", map[string]any{"mail_requests": true}, nil); code != 200 {
		t.Fatalf("Bob asks for mail: %d", code)
	}
	ana := newBrowser(t, e.URL)
	anaPw := signUp(t, e, ana, "ana", "ana@example.com")
	if code := ana.login("ana", anaPw); code != 200 {
		t.Fatalf("Ana signs in: %d", code)
	}
	before = len(e.SMTP.Mails())
	if code := friendPost(ana, "request", map[string]any{"handle": "bob"}); code != 200 {
		t.Fatalf("Ana asks: %d", code)
	}
	if subject, text := readMail(t, e.waitMails(t, before+1)[before].Data); subject != "@ana möchte auf Bingle dein Freund sein" || !strings.Contains(text, "Freundschaftsanfrage") {
		t.Errorf("the friend request in German: %q\n%s", subject, text)
	}

	// A choice of language wins over the browser's.
	if code := bob.do("POST", "/auth/language", map[string]any{"language": "es"}, nil); code != 200 {
		t.Fatalf("Bob chooses Spanish: %d", code)
	}
	friendPost(ana, "remove", map[string]any{"handle": "bob"})
	before = len(e.SMTP.Mails())
	if code := friendPost(ana, "request", map[string]any{"handle": "bob"}); code != 200 {
		t.Fatalf("Ana asks again: %d", code)
	}
	if subject, _ := readMail(t, e.waitMails(t, before+1)[before].Data); subject != "@ana quiere ser tu amigo en Bingle" {
		t.Errorf("the friend request in Spanish: %q", subject)
	}
}
