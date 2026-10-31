package mail

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"mime"
	"mime/quotedprintable"
	"net/url"
	"strings"
	"time"
)

// Message is one email: exactly one recipient and a subject and body made
// by the templates below.
type Message struct {
	To      string
	Subject string
	Body    string
	// UnsubscribeURL, when set (an https URL), becomes the List-Unsubscribe
	// header with one-click support.
	UnsubscribeURL string
}

// Build renders the message as the bytes of an RFC 5322 message with a fixed
// header set. from is the sender address; domain follows the "@" of the
// Message-ID.
func (m Message) Build(from, fromName string, now time.Time) ([]byte, error) {
	to, err := ValidAddress(m.To)
	if err != nil {
		return nil, fmt.Errorf("recipient: %w", err)
	}
	from, err = ValidAddress(from)
	if err != nil {
		return nil, fmt.Errorf("sender: %w", err)
	}
	if strings.ContainsAny(m.Subject, "\r\n") || m.Subject == "" || len(m.Subject) > 200 {
		return nil, fmt.Errorf("bad subject")
	}
	if strings.ContainsAny(fromName, "\r\n\"<>") {
		return nil, fmt.Errorf("bad sender name")
	}
	if m.UnsubscribeURL != "" {
		u, err := url.Parse(m.UnsubscribeURL)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || strings.ContainsAny(m.UnsubscribeURL, "\r\n<> ") {
			return nil, fmt.Errorf("bad unsubscribe URL")
		}
	}
	id := make([]byte, 12)
	if _, err := rand.Read(id); err != nil {
		return nil, err
	}
	domain := from[strings.LastIndexByte(from, '@')+1:]

	var b bytes.Buffer
	h := func(k, v string) { b.WriteString(k + ": " + v + "\r\n") }
	h("From", fmt.Sprintf("%s <%s>", mime.QEncoding.Encode("utf-8", fromName), from))
	h("To", to)
	h("Subject", mime.QEncoding.Encode("utf-8", m.Subject))
	h("Date", now.UTC().Format(time.RFC1123Z))
	h("Message-ID", "<"+hex.EncodeToString(id)+"@"+domain+">")
	h("MIME-Version", "1.0")
	body := strings.ReplaceAll(strings.ReplaceAll(m.Body, "\r\n", "\n"), "\n", "\r\n")
	plain := isPlain(body)
	h("Content-Type", "text/plain; charset=utf-8")
	if plain {
		h("Content-Transfer-Encoding", "7bit")
	} else {
		h("Content-Transfer-Encoding", "quoted-printable")
	}
	h("Auto-Submitted", "auto-generated")
	if m.UnsubscribeURL != "" {
		h("List-Unsubscribe", "<"+m.UnsubscribeURL+">")
		h("List-Unsubscribe-Post", "List-Unsubscribe=One-Click")
	}
	b.WriteString("\r\n")
	if plain {
		b.WriteString(body)
		return b.Bytes(), nil
	}
	w := quotedprintable.NewWriter(&b)
	if _, err := w.Write([]byte(body)); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// isPlain reports whether a body can go as 7-bit text: ASCII, with lines of
// at most 998 characters.
func isPlain(body string) bool {
	for _, line := range strings.Split(body, "\r\n") {
		if len(line) > 998 {
			return false
		}
		for i := 0; i < len(line); i++ {
			if line[i] >= 0x80 || (line[i] < 0x20 && line[i] != '\t') {
				return false
			}
		}
	}
	return true
}

// Link builders: base is the server's public URL without a trailing slash.

func ConfirmURL(base, token string) string {
	return base + "/notify/confirm?t=" + url.QueryEscape(token)
}
func StopURL(base, token string) string { return base + "/notify/stop?t=" + url.QueryEscape(token) }

// Confirmation is the one mail sent to check that an address belongs to the
// person who typed it. Doing nothing leaves it harmless.
func Confirmation(to, base, confirmToken, stopToken string) Message {
	return Message{
		To:      to,
		Subject: "Confirm your email address for Bingle",
		Body: "Someone, probably you, asked Bingle to email this address when it is their turn in a game.\n\n" +
			"To turn this on, open the link below and press Confirm (the link works for 24 hours):\n\n" +
			ConfirmURL(base, confirmToken) + "\n\n" +
			"If this was not you, ignore this message and nothing happens. To make sure Bingle never emails this address, open:\n\n" +
			StopURL(base, stopToken) + "\n",
		UnsubscribeURL: StopURL(base, stopToken),
	}
}

// Turn tells a player that it is their turn. Only the game's name and the room
// code (both sanitised) vary.
func Turn(to, game, room, base, stopToken string) Message {
	game = Sanitize(game, 60)
	room = Sanitize(room, 12)
	return Message{
		To:      to,
		Subject: fmt.Sprintf("It is your turn in %s (%s)", game, room),
		Body: fmt.Sprintf("It is your turn in %s, room %s.\n\nPlay here: %s/game/%s\n\n", game, room, base, url.PathEscape(room)) +
			"You get this mail because you asked Bingle for turn notices. To stop all mail from Bingle, open:\n\n" +
			StopURL(base, stopToken) + "\n",
		UnsubscribeURL: StopURL(base, stopToken),
	}
}

// AddressChanged goes to the old address of a profile when it is changed.
func AddressChanged(to, base, stopToken string) Message {
	return Message{
		To:      to,
		Subject: "Your Bingle notification address was changed",
		Body: "The email address for your turn notices on Bingle was changed to another address, so you will not get them here any more.\n\n" +
			"If this was not you, open the link below to make sure Bingle never emails this address again:\n\n" +
			StopURL(base, stopToken) + "\n",
		UnsubscribeURL: StopURL(base, stopToken),
	}
}

// AccountConfirmURL is the link that confirms a new account's address.
func AccountConfirmURL(base, token string) string {
	return base + "/account/confirm?t=" + url.QueryEscape(token)
}

// AccountConfirmation is the mail that confirms the address of a new account.
// Doing nothing leaves it harmless: the account is never usable, and is
// forgotten after two days.
func AccountConfirmation(to, handle, base, token string) Message {
	handle = Sanitize(handle, 24)
	return Message{
		To:      to,
		Subject: "Confirm your Bingle account",
		Body: fmt.Sprintf("Someone, probably you, made the Bingle account @%s with this email address.\n\n", handle) +
			"To finish, open the link below and press Confirm (the link works for 48 hours):\n\n" +
			AccountConfirmURL(base, token) + "\n\n" +
			"If this was not you, ignore this message: the account is never usable and is removed in two days.\n",
	}
}
