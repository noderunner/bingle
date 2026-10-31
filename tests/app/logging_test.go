// Tests of what the server's log says while it is being used: the lines carry
// route patterns, request ids and nothing a reader of the log could join a
// game or read a conversation with.

package app_test

import (
	"bytes"
	"log/slog"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/noderunner/bingle/internal/app"
	"github.com/noderunner/bingle/internal/auth"
	"github.com/noderunner/bingle/internal/logging"
)

// logBuffer is a bytes.Buffer safe for the handler goroutines, the bot
// goroutines and the notifier to write to at once.
type logBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *logBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *logBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// newLoggedTestServer is newRESTTestServer with the server's logger turned up
// to debug and pointed at a buffer, which it returns beside the base URL.
func newLoggedTestServer(t *testing.T) (string, *logBuffer, *app.App) {
	t.Helper()
	buf := &logBuffer{}
	cfg := newTestConfig(t)

	a, err := app.New(cfg, app.WithLogger(logging.New(buf, slog.LevelDebug, logging.FormatText)))
	if err != nil {
		t.Fatalf("app.New: %v", err)
	}
	t.Cleanup(func() { _ = a.Close() })
	registerFixtures(t, a)

	srv := httptest.NewServer(a.Router())
	t.Cleanup(srv.Close)
	return srv.URL, buf, a
}

// A game played and talked about from end to end, with the log as verbose as
// it goes: the lines must still carry none of the things the redaction policy
// (docs/decisions/0063) says they never carry.
func TestLog_NeverCarriesASecretEvenAtDebug(t *testing.T) {
	const (
		chatText = "well played, that was a trap"
		dmText   = "fancy another after this one"
		address  = "ana@example.com"
	)

	baseURL, buf, a := newLoggedTestServer(t)

	// A room, a second player, a move, a conversation in the room and a
	// conversation beside it, a browser saying it is about, and a page.
	room := createRoom(t, baseURL, "Ana")
	if code := doJSON(t, "POST", baseURL+"/api/games/"+room+"/join",
		map[string]any{"player_name": "Bo", "role": "player"}, nil); code != 200 {
		t.Fatalf("Bo joins: %d", code)
	}
	if code := doJSON(t, "POST", baseURL+"/api/games/"+room+"/chat",
		map[string]any{"player_name": "Ana", "text": chatText}, nil); code != 200 {
		t.Fatalf("Ana says something: %d", code)
	}
	// Two accounts, about, and a direct message between them (ADR 0073).
	anaB, boB := member(t, a, baseURL, "anabel"), member(t, a, baseURL, "bobo")
	ana, bo := anaB.here(), boB.here()
	if code := anaB.do("POST", "/api/dm", map[string]any{"to": bo, "text": dmText}, nil); code != 200 {
		t.Fatalf("Ana writes to Bo: %d", code)
	}
	session := anaB.cookie(auth.SessionCookie)
	// Notifications are off in this config, so this is refused — the address
	// still went through the handler, and must not have been logged.
	doJSON(t, "POST", baseURL+"/api/notify/contact", map[string]any{"address": address}, nil)
	doJSON(t, "GET", baseURL+"/api/games/"+room, nil, nil)
	doJSON(t, "GET", baseURL+"/game/"+room, nil, nil)
	doJSON(t, "GET", baseURL+"/api/games/"+room+"/nothing-here", nil, nil)

	conn, err := dialWS(t, baseURL, wsPath(room, "Bo", true, "player", true))
	if err != nil {
		t.Fatalf("Bo's socket: %v", err)
	}
	readFirstEvent(t, conn)
	_ = conn.CloseNow()

	got := buf.String()
	if got == "" {
		t.Fatal("the server logged nothing at all at debug")
	}

	for what, secret := range map[string]string{
		"the room code":             room,
		"a session cookie":          session,
		"what was said in the room": chatText,
		"a direct message":          dmText,
		"an email address":          address,
		"an account's address":      ana,
		"a player's name":           "Ana",
	} {
		if strings.Contains(got, secret) {
			t.Errorf("the log carries %s (%q):\n%s", what, secret, got)
		}
	}

	// And it does say useful things: the routes that were asked for, by
	// pattern, each with an id.
	for _, want := range []string{
		"route=/api/games/{code}/chat",
		"route=/api/games/{code}/join",
		"route=/api/dm",
		"route=/game/{code}",
		`msg="websocket open"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the log never says %s:\n%s", want, got)
		}
	}
}

// The room socket lives as long as the player is on the page, so it is logged
// when it is accepted and again when it closes — not once at the end, as a
// request whose duration measures the whole visit.
func TestLog_WebSocketIsLoggedWhenItOpensAndWhenItCloses(t *testing.T) {
	baseURL, buf, _ := newLoggedTestServer(t)
	room := createRoom(t, baseURL, "Ana")

	conn, err := dialWS(t, baseURL, wsPath(room, "Bo", true, "player", true))
	if err != nil {
		t.Fatalf("Bo's socket: %v", err)
	}
	readFirstEvent(t, conn)

	if n := strings.Count(buf.String(), `msg="websocket open"`); n != 1 {
		t.Fatalf("the open line appears %d times:\n%s", n, buf.String())
	}
	if strings.Contains(buf.String(), `msg="websocket closed"`) {
		t.Fatalf("the socket is still open and already logged as closed:\n%s", buf.String())
	}
	if strings.Contains(buf.String(), "route=/ws/{code} status=") {
		t.Fatalf("the open socket was logged as a finished request:\n%s", buf.String())
	}

	_ = conn.CloseNow()
	// The handler is parked on the room's event channel until the room next
	// has something to say, so it notices the closed socket on the next
	// event rather than at once. Give it one.
	doJSON(t, "POST", baseURL+"/api/games/"+room+"/chat",
		map[string]any{"player_name": "Ana", "text": "anyone there?"}, nil)

	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(buf.String(), `msg="websocket closed"`) {
		if time.Now().After(deadline) {
			t.Fatalf("the closing line never came:\n%s", buf.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
	if n := strings.Count(buf.String(), `msg="websocket closed"`); n != 1 {
		t.Fatalf("the closing line appears %d times:\n%s", n, buf.String())
	}
}
