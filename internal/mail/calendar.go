package mail

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

// A calendar invitation, as an iCalendar (RFC 5545) file. It names one attendee, the person it is
// sent to: nobody's address reaches anybody else.

// Event is one game on a calendar.
type Event struct {
	// UID stays the same through changes of the event, Seq rises with each.
	UID string
	Seq int
	// Start and End are written in UTC: the calendar shows them in its own time zone.
	Start, End time.Time
	Summary    string
	URL        string
	// Method is REQUEST, CANCEL or PUBLISH (a file to download, with no people in it).
	Method string
	// Organizer and Attendee are addresses, for REQUEST and CANCEL.
	Organizer, Attendee string
	// Lang is the language of the words the calendar shows (ADR 0141).
	Lang string
}

// EventSummary is what a calendar calls a game: its name, on this server.
func EventSummary(lang, game string) string { return say(lang, "mail.event_summary", "game", game) }

// icsText escapes a value of an iCalendar property.
func icsText(s string) string {
	return strings.NewReplacer(`\`, `\\`, ";", `\;`, ",", `\,`, "\n", `\n`, "\r", "").Replace(s)
}

// icsLine folds a line at 75 octets, on the boundary of a character.
func icsLine(b *strings.Builder, line string) {
	for first := true; ; first = false {
		limit := 75
		if !first {
			limit = 74
			b.WriteByte(' ')
		}
		if len(line) <= limit {
			b.WriteString(line + "\r\n")
			return
		}
		cut := limit
		for cut > 0 && line[cut]&0xC0 == 0x80 {
			cut--
		}
		b.WriteString(line[:cut] + "\r\n")
		line = line[cut:]
	}
}

// ICS renders the event. now is when it was made.
func (e Event) ICS(now time.Time) string {
	const form = "20060102T150405Z"
	var b strings.Builder
	line := func(s string) { icsLine(&b, s) }
	line("BEGIN:VCALENDAR")
	line("VERSION:2.0")
	line("PRODID:-//Bingle//Game Scheduling//EN")
	line("CALSCALE:GREGORIAN")
	line("METHOD:" + e.Method)
	line("BEGIN:VEVENT")
	line("UID:" + icsText(e.UID))
	line(fmt.Sprintf("SEQUENCE:%d", e.Seq))
	line("DTSTAMP:" + now.UTC().Format(form))
	line("DTSTART:" + e.Start.UTC().Format(form))
	line("DTEND:" + e.End.UTC().Format(form))
	line("SUMMARY:" + icsText(e.Summary))
	line("DESCRIPTION:" + icsText(say(e.Lang, "mail.event_open", "link", e.URL)))
	line("URL:" + icsText(e.URL))
	if e.Method != "PUBLISH" {
		line("ORGANIZER;CN=Bingle:mailto:" + e.Organizer)
		line("ATTENDEE;RSVP=FALSE:mailto:" + e.Attendee)
	}
	if e.Method == "CANCEL" {
		line("STATUS:CANCELLED")
	} else {
		line("STATUS:CONFIRMED")
	}
	line("END:VEVENT")
	line("END:VCALENDAR")
	return b.String()
}

func gameURL(base, room string) string { return base + "/game/" + url.PathEscape(room) }

// whenUTC writes a time for a mail, which has no time zone of its reader's to go by, in its
// language: the names of the day and the month are the catalog's.
func whenUTC(lang string, t time.Time) string {
	t = t.UTC()
	return say(lang, "mail.date", "weekday", say(lang, "mail.weekday_"+strings.ToLower(t.Weekday().String())), "day", t.Day(),
		"month", say(lang, "mail.month_"+strings.ToLower(t.Month().String())), "year", t.Year(), "time", t.Format("15:04"))
}

// dayUTC is a shorter time, for a subject: the day of the month and the hour.
func dayUTC(lang string, t time.Time) string {
	t = t.UTC()
	return say(lang, "mail.date_short", "day", t.Day(), "month", say(lang, "mail.month_"+strings.ToLower(t.Month().String())), "time", t.Format("15:04"))
}

// GameInvitation invites an account to a game set up for later. Only the handle, the game's name and
// the room vary.
func GameInvitation(lang, to, from, game, room, base string, at time.Time, severalTimes bool) Message {
	from, game, room = Sanitize(from, 24), Sanitize(game, 60), Sanitize(room, 12)
	when := say(lang, "mail.invite_several")
	if !severalTimes {
		when = say(lang, "mail.invite_at", "when", whenUTC(lang, at))
	}
	return Message{
		To:      to,
		Subject: say(lang, "mail.invite_subject", "handle", from, "game", game),
		Body: paragraphs(say(lang, "mail.invite", "handle", from, "game", game)+" "+when, say(lang, "mail.invite_answer"), gameURL(base, room),
			say(lang, "mail.invite_why"), base+"/profile"),
	}
}

// GameScheduled sends a game's time, with the invitation for a calendar.
func GameScheduled(lang, to, game, room, base string, event Event, now time.Time) Message {
	game, room = Sanitize(game, 60), Sanitize(room, 12)
	return Message{
		To:       to,
		Subject:  say(lang, "mail.scheduled_subject", "game", game, "when", dayUTC(lang, event.Start)),
		Body:     paragraphs(say(lang, "mail.scheduled", "game", game, "when", whenUTC(lang, event.Start)), say(lang, "mail.scheduled_start"), gameURL(base, room)),
		Calendar: &Calendar{Method: event.Method, Body: event.ICS(now)},
	}
}

// GameCancelled says a game will not be played, for a reason (a key of the catalog). event is
// its calendar entry, if one was sent.
func GameCancelled(lang, to, game, room, reason, base string, event *Event, now time.Time) Message {
	game, room = Sanitize(game, 60), Sanitize(room, 12)
	msg := Message{
		To:      to,
		Subject: say(lang, "mail.closed_subject", "game", game),
		Body:    paragraphs(say(lang, "mail.closed", "game", game, "room", room, "reason", say(lang, reason)), base),
	}
	if event != nil {
		msg.Calendar = &Calendar{Method: event.Method, Body: event.ICS(now)}
	}
	return msg
}

// NoCommonTime tells a game's creator that nobody's times meet, and how long they have.
func NoCommonTime(lang, to, game, room, base string, by time.Time) Message {
	game, room = Sanitize(game, 60), Sanitize(room, 12)
	return Message{
		To:      to,
		Subject: say(lang, "mail.no_time_subject", "game", game),
		Body:    paragraphs(say(lang, "mail.no_time", "game", game, "when", whenUTC(lang, by)), gameURL(base, room)),
	}
}

// NewTimesOffered tells those invited that a game's creator has offered new times.
func NewTimesOffered(lang, to, game, room, base string) Message {
	game, room = Sanitize(game, 60), Sanitize(room, 12)
	return Message{
		To:      to,
		Subject: say(lang, "mail.new_times_subject", "game", game),
		Body:    paragraphs(say(lang, "mail.new_times", "game", game), gameURL(base, room)),
	}
}
