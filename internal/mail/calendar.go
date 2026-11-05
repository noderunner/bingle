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
}

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
	line("DESCRIPTION:" + icsText("Open the game: "+e.URL))
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

// whenUTC writes a time for a mail, which has no time zone of its reader's to go by.
func whenUTC(t time.Time) string { return t.UTC().Format("Monday 2 January 2006, 15:04 UTC") }

// GameInvitation invites an account to a game set up for later. Only the handle, the game's name and
// the room vary.
func GameInvitation(to, from, game, room, base string, at time.Time, severalTimes bool) Message {
	from, game, room = Sanitize(from, 24), Sanitize(game, 60), Sanitize(room, 12)
	when := "They have offered several times: open the game to say which you can make."
	if !severalTimes {
		when = "It is set for " + whenUTC(at) + "."
	}
	return Message{
		To:      to,
		Subject: fmt.Sprintf("@%s invites you to a game of %s", from, game),
		Body: fmt.Sprintf("@%s has invited you to a game of %s. %s\n\nAnswer in your friends list, or open the game:\n\n%s\n\n", from, game, when, gameURL(base, room)) +
			"To stop being invited to games, turn it off in your profile:\n\n" + base + "/profile\n",
	}
}

// GameScheduled sends a game's time, with the invitation for a calendar.
func GameScheduled(to, game, room, base string, event Event, now time.Time) Message {
	game, room = Sanitize(game, 60), Sanitize(room, 12)
	return Message{
		To:      to,
		Subject: fmt.Sprintf("%s is set for %s", game, event.Start.UTC().Format("2 Jan, 15:04 UTC")),
		Body: fmt.Sprintf("Your game of %s is set for %s. The attached invitation shows it in your own time zone.\n\n", game, whenUTC(event.Start)) +
			"The game does not start by itself: when the time comes, press Start in its lobby.\n\n" + gameURL(base, room) + "\n",
		Calendar: &Calendar{Method: event.Method, Body: event.ICS(now)},
	}
}

// GameCancelled says a game will not be played. event is its calendar entry, if one was sent.
func GameCancelled(to, game, room, reason, base string, event *Event, now time.Time) Message {
	game, room = Sanitize(game, 60), Sanitize(room, 12)
	msg := Message{
		To:      to,
		Subject: fmt.Sprintf("Your game of %s was closed", game),
		Body:    fmt.Sprintf("Your game of %s (room %s) was closed: %s\n\n%s\n", game, room, reason, base),
	}
	if event != nil {
		msg.Calendar = &Calendar{Method: event.Method, Body: event.ICS(now)}
	}
	return msg
}

// NoCommonTime tells a game's creator that nobody's times meet, and how long they have.
func NoCommonTime(to, game, room, base string, by time.Time) Message {
	game, room = Sanitize(game, 60), Sanitize(room, 12)
	return Message{
		To:      to,
		Subject: fmt.Sprintf("No time suits everyone for %s", game),
		Body:    fmt.Sprintf("No time you offered for your game of %s suits everyone. In its lobby you can offer new times or cancel it. If you do neither, it closes on %s and everyone is told.\n\n%s\n", game, whenUTC(by), gameURL(base, room)),
	}
}

// NewTimesOffered tells those invited that a game's creator has offered new times.
func NewTimesOffered(to, game, room, base string) Message {
	game, room = Sanitize(game, 60), Sanitize(room, 12)
	return Message{
		To:      to,
		Subject: fmt.Sprintf("New times for %s", game),
		Body:    fmt.Sprintf("New times have been offered for your game of %s. Open it to say which you can make:\n\n%s\n", game, gameURL(base, room)),
	}
}
