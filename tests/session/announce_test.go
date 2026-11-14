// Awards are announced (ADR 0130): to the holder's pages by a "badge" event,
// and to the room by a line from the server in the game's chat; a secret
// award reaches the room only when its game ends.

package session_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/noderunner/bingle/internal/session"
)

// badgeEvents follows the events sent to an account's pages.
func (w *ratedWorld) badgeEvents(p player) <-chan session.Event {
	w.t.Helper()
	events, stop := w.m.SubscribeDirect(session.AddressOf("p-" + p.handle))
	w.t.Cleanup(stop)
	return events
}

// nextBadge waits up to wait for the next "badge" event, and says if none
// came. The workers announce before they return, so a short wait is enough
// to say there was none.
func nextBadge(events <-chan session.Event, wait time.Duration) (session.BadgeEventData, bool) {
	timeout := time.After(wait)
	for {
		select {
		case ev := <-events:
			if d, ok := ev.Data.(session.BadgeEventData); ok && ev.Type == "badge" {
				return d, true
			}
		case <-timeout:
			return session.BadgeEventData{}, false
		}
	}
}

// serverLines is the text of the lines the server wrote in a game's chat.
func (w *ratedWorld) serverLines(room string) []string {
	w.t.Helper()
	lines, err := w.m.ChatLog(context.Background(), room, 0, 100)
	if err != nil {
		w.t.Fatal(err)
	}
	var out []string
	for _, l := range lines {
		if l.Server {
			if l.Seat != 0 || l.Handle != "" {
				w.t.Errorf("a server line names seat %d, handle %q", l.Seat, l.Handle)
			}
			out = append(out, l.Text)
		}
	}
	return out
}

func TestAnAwardIsAnnouncedToItsHolderAndTheRoom(t *testing.T) {
	w := newRatedWorld(t)
	ana, bea := w.account("ana"), w.account("bea")
	anas, beas := w.badgeEvents(ana), w.badgeEvents(bea)

	room := w.unrated("yavalath", ana, bea)
	w.moveEach(room, ana, bea)
	w.resign(room, bea)

	d, ok := nextBadge(anas, 2*time.Second)
	if !ok || d.Badge != "first-win" || d.Name != "First Win" || d.Room != room || d.Hidden {
		t.Fatalf("ana's badge event = %+v (%v), want First Win in %s, not hidden", d, ok, room)
	}
	if d, ok := nextBadge(beas, 200*time.Millisecond); ok {
		t.Errorf("bea was told of %+v, want nothing", d)
	}
	lines := w.serverLines(room)
	if len(lines) != 1 || !strings.Contains(lines[0], "@ana earned the badge First Win") {
		t.Errorf("server lines = %q, want one announcing ana's First Win", lines)
	}

	// The badge is earned once, and announced once.
	room = w.unrated("yavalath", ana, bea)
	w.moveEach(room, ana, bea)
	w.resign(room, bea)
	if d, ok := nextBadge(anas, 200*time.Millisecond); ok {
		t.Errorf("a second win announced %+v, want nothing", d)
	}
	if lines := w.serverLines(room); len(lines) != 0 {
		t.Errorf("server lines of the second game = %q, want none", lines)
	}
}

func TestASecretAwardReachesTheRoomAtTheEnd(t *testing.T) {
	w := newRatedWorld(t)
	w.withBadgeFixture()
	ana, bea := w.account("ana"), w.account("bea")
	anas := w.badgeEvents(ana)
	ctx := context.Background()

	room := w.unrated("fixture-badges", ana, bea)
	w.play(room, ana, "hide", "", "")
	w.play(room, bea, "place", "white", "a4")
	w.m.CheckBadges(ctx)
	d, ok := nextBadge(anas, 2*time.Second)
	if !ok || d.Badge != "fixture-badges.hider" || !d.Hidden {
		t.Fatalf("ana's badge event = %+v (%v), want Hider, hidden", d, ok)
	}
	if lines := w.serverLines(room); len(lines) != 0 {
		t.Fatalf("server lines before the end = %q, want none", lines)
	}

	w.finish(room, 0)
	lines := w.serverLines(room)
	if len(lines) != 1 || !strings.Contains(lines[0], "@ana earned the badge Hider") {
		t.Errorf("server lines after the end = %q, want one announcing ana's Hider", lines)
	}
	if d, ok := nextBadge(anas, 200*time.Millisecond); ok {
		t.Errorf("ana was told again: %+v", d)
	}
}
