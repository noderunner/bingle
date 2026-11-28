package mail

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"mime"
	"mime/quotedprintable"
	"net/url"
	"strings"
	"time"

	"github.com/noderunner/bingle/internal/i18n"
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
	// Calendar, when set, goes with the message: an invitation for a calendar.
	Calendar *Calendar
}

// branded is the message with the name of this instance in place of the name in the templates.
func (m Message) branded(brand string) Message {
	if brand == "" || brand == "Bingle" {
		return m
	}
	swap := func(s string) string { return strings.ReplaceAll(s, "Bingle", brand) }
	m.Subject, m.Body = swap(m.Subject), swap(m.Body)
	if m.Calendar != nil {
		cal := *m.Calendar
		cal.Body = swap(cal.Body)
		m.Calendar = &cal
	}
	return m
}

// Calendar is an iCalendar file sent with a message.
type Calendar struct {
	Method string // REQUEST or CANCEL
	Body   string
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
	boundary := ""
	if m.Calendar != nil {
		boundary = "bingle-" + hex.EncodeToString(id)
		h("Content-Type", `multipart/mixed; boundary="`+boundary+`"`)
	} else {
		h("Content-Type", "text/plain; charset=utf-8")
		if plain {
			h("Content-Transfer-Encoding", "7bit")
		} else {
			h("Content-Transfer-Encoding", "quoted-printable")
		}
	}
	h("Auto-Submitted", "auto-generated")
	if m.UnsubscribeURL != "" {
		h("List-Unsubscribe", "<"+m.UnsubscribeURL+">")
		h("List-Unsubscribe-Post", "List-Unsubscribe=One-Click")
	}
	b.WriteString("\r\n")
	if boundary != "" {
		b.WriteString("--" + boundary + "\r\n")
		h("Content-Type", "text/plain; charset=utf-8")
		if plain {
			h("Content-Transfer-Encoding", "7bit")
		} else {
			h("Content-Transfer-Encoding", "quoted-printable")
		}
		b.WriteString("\r\n")
	}
	if plain {
		b.WriteString(body)
	} else {
		w := quotedprintable.NewWriter(&b)
		if _, err := w.Write([]byte(body)); err != nil {
			return nil, err
		}
		if err := w.Close(); err != nil {
			return nil, err
		}
	}
	if boundary == "" {
		return b.Bytes(), nil
	}
	if strings.ContainsAny(m.Calendar.Method, "\r\n\"") {
		return nil, fmt.Errorf("bad calendar method")
	}
	b.WriteString("\r\n--" + boundary + "\r\n")
	h("Content-Type", fmt.Sprintf(`text/calendar; charset=utf-8; method=%s; name="bingle.ics"`, m.Calendar.Method))
	h("Content-Disposition", `attachment; filename="bingle.ics"`)
	h("Content-Transfer-Encoding", "base64")
	b.WriteString("\r\n")
	enc := base64.StdEncoding.EncodeToString([]byte(m.Calendar.Body))
	for len(enc) > 76 {
		b.WriteString(enc[:76] + "\r\n")
		enc = enc[76:]
	}
	b.WriteString(enc + "\r\n--" + boundary + "--\r\n")
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

// say is a mail's words in a language (ADR 0141): the catalog's key, with
// {brand} the name the mail is branded with when it is sent (Message.branded).
func say(lang, key string, params ...any) string {
	return i18n.T(lang, key, append([]any{"brand", "Bingle"}, params...)...)
}

// paragraphs joins a mail's paragraphs and links, a blank line between each.
func paragraphs(parts ...string) string { return strings.Join(parts, "\n\n") + "\n" }

// Confirmation is the one mail sent to check that an address belongs to the
// person who typed it. Doing nothing leaves it harmless.
func Confirmation(lang, to, base, confirmToken, stopToken string) Message {
	return Message{
		To:      to,
		Subject: say(lang, "mail.confirm_subject"),
		Body: paragraphs(say(lang, "mail.confirm_asked"), say(lang, "mail.confirm_open"), ConfirmURL(base, confirmToken),
			say(lang, "mail.confirm_not_you"), StopURL(base, stopToken)),
		UnsubscribeURL: StopURL(base, stopToken),
	}
}

// Turn tells a player that it is their turn. Only the game's name and the room
// code (both sanitised) vary.
func Turn(lang, to, game, room, base, stopToken string) Message {
	game = Sanitize(game, 60)
	room = Sanitize(room, 12)
	return Message{
		To:      to,
		Subject: say(lang, "mail.turn_subject", "game", game, "room", room),
		Body: paragraphs(say(lang, "mail.turn", "game", game, "room", room), say(lang, "mail.play_here", "link", base+"/game/"+url.PathEscape(room)),
			say(lang, "mail.turn_why"), StopURL(base, stopToken)),
		UnsubscribeURL: StopURL(base, stopToken),
	}
}

// AddressChanged goes to the old address of a profile when it is changed.
func AddressChanged(lang, to, base, stopToken string) Message {
	return Message{
		To:             to,
		Subject:        say(lang, "mail.changed_subject"),
		Body:           paragraphs(say(lang, "mail.changed"), say(lang, "mail.changed_not_you"), StopURL(base, stopToken)),
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
func AccountConfirmation(lang, to, handle, base, token string) Message {
	handle = Sanitize(handle, 24)
	return Message{
		To:      to,
		Subject: say(lang, "mail.account_subject"),
		Body: paragraphs(say(lang, "mail.account_made", "handle", handle), say(lang, "mail.account_open"), AccountConfirmURL(base, token),
			say(lang, "mail.account_not_you")),
	}
}

// AccountExists is the mail to an account's address when someone signs up
// with it again: the sign-up page answers as it would for a new address, so
// this is how the person learns they have an account already. It carries no
// token; its links are the public sign-in and forgotten-password pages.
func AccountExists(lang, to, handle, base string) Message {
	handle = Sanitize(handle, 24)
	return Message{
		To:      to,
		Subject: say(lang, "mail.exists_subject"),
		Body: paragraphs(say(lang, "mail.exists_tried"), say(lang, "mail.exists_belongs", "handle", handle), base+"/login",
			say(lang, "mail.exists_forgot"), base+"/forgot", say(lang, "mail.exists_not_you")),
	}
}

// FriendRequest tells an account that somebody has asked to be its friend. Only the handle varies.
func FriendRequest(lang, to, handle, base string) Message {
	handle = Sanitize(handle, 24)
	return Message{
		To:      to,
		Subject: say(lang, "mail.friend_subject", "handle", handle),
		Body:    paragraphs(say(lang, "mail.friend", "handle", handle), base+"/", say(lang, "mail.friend_why"), base+"/profile"),
	}
}

// RatingRemoved tells a player that a moderator took a rated game of theirs out
// of the ratings (ADR 0157), and where their rating is now shown.
func RatingRemoved(lang, to, moderator, game, opponent, room, handle, base string) Message {
	moderator, opponent, handle = Sanitize(moderator, 24), Sanitize(opponent, 24), Sanitize(handle, 24)
	game, room = Sanitize(game, 60), Sanitize(room, 12)
	return Message{
		To:      to,
		Subject: say(lang, "mail.rating_removed_subject"),
		Body: paragraphs(say(lang, "mail.rating_removed", "moderator", moderator, "game", game, "opponent", opponent, "room", room),
			base+"/players/"+url.PathEscape(handle)),
	}
}

// PasswordResetURL is the link that resets an account's password.
func PasswordResetURL(base, token string) string {
	return base + "/account/reset?t=" + url.QueryEscape(token)
}

// PasswordReset is the mail with a link to choose a new password.
func PasswordReset(lang, to, handle, base, token string) Message {
	handle = Sanitize(handle, 24)
	return Message{
		To:      to,
		Subject: say(lang, "mail.reset_subject"),
		Body: paragraphs(say(lang, "mail.reset_asked", "handle", handle), say(lang, "mail.reset_open"), PasswordResetURL(base, token),
			say(lang, "mail.reset_not_you")),
	}
}
