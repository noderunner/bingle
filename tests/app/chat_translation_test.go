package app_test

// Chat in each reader's language (ADR 0143), against a fake translation
// service: a line reaches each reader in theirs, the original stays as it was,
// notation and handles never reach the service, a down service changes
// nothing else, history keeps what was made, and the writer's limit holds.

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/noderunner/bingle/internal/app"
	"github.com/noderunner/bingle/internal/config"
)

// fakeTranslator answers like a model: "[Spanish] " and the line, markers and
// all. A line beginning "hola" is Spanish, any other English; a line already
// in the reader's language comes back as it is.
type fakeTranslator struct {
	*httptest.Server
	mu    sync.Mutex
	asked []string // "Spanish: the line as sent"
}

func newFakeTranslator(t *testing.T) *fakeTranslator {
	t.Helper()
	f := &fakeTranslator{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []struct{ Role, Content string } `json:"messages"`
		}
		if r.URL.Path != "/v1/chat/completions" || json.NewDecoder(r.Body).Decode(&req) != nil || len(req.Messages) != 2 {
			http.Error(w, "no", http.StatusBadRequest)
			return
		}
		_, rest, _ := strings.Cut(req.Messages[0].Content, "into ")
		target, _, _ := strings.Cut(rest, ".")
		line := req.Messages[1].Content
		f.mu.Lock()
		f.asked = append(f.asked, target+": "+line)
		f.mu.Unlock()
		source, text := "en", "["+target+"] "+line
		if strings.HasPrefix(line, "hola") {
			source = "es"
		}
		if (source == "en" && target == "English") || (source == "es" && target == "Spanish") {
			text = line
		}
		answer, _ := json.Marshal(map[string]string{"source": source, "text": text})
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": string(answer)}}}})
	}))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeTranslator) requests() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.asked...)
}

// translatingServer is a server whose chat is translated by the service at
// url; capsFile, when not empty, replaces the tests' capabilities.
func translatingServer(t *testing.T, url, capsFile string) string {
	t.Helper()
	_, base := translatingApp(t, url, capsFile)
	return base
}

func translatingApp(t *testing.T, url, capsFile string) (*app.App, string) {
	t.Helper()
	a, base, _ := translatingAppDB(t, url, capsFile)
	return a, base
}

// translatingAppDB is translatingApp, and the path of its database.
func translatingAppDB(t *testing.T, url, capsFile string) (*app.App, string, string) {
	t.Helper()
	cfg := newTestConfig(t)
	cfg.Translate = config.Translate{URL: url + "/v1", Model: "fake", API: "openai", Workers: 1, Timeout: 5 * time.Second}
	if capsFile != "" {
		cfg.CapsFile = capsFile
	}
	a, err := app.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Close() })
	registerFixtures(t, a)
	srv := httptest.NewServer(a.Router())
	t.Cleanup(srv.Close)
	return a, srv.URL, cfg.DBPath
}

// signedIn is a browser signed in to a new account called handle.
func signedIn(t *testing.T, a *app.App, base, handle string) *browser {
	t.Helper()
	given, err := a.Accounts().Create(context.Background(), handle, handle+"@example.com", false)
	if err != nil {
		t.Fatal(err)
	}
	b := newBrowser(t, base)
	if code := b.login(handle, given); code != 200 {
		t.Fatalf("login: %d", code)
	}
	if code := b.do("POST", "/auth/password", map[string]any{"old": given, "new": anaPassword}, nil); code != 200 {
		t.Fatalf("password: %d", code)
	}
	return b
}

// reader is a browser reading a room's chat in a language.
type reader struct {
	*browser
	conn *websocket.Conn
}

// newReader is a browser that reads in lang (translation turned off when
// !translate) with a socket open on the game room, or on the global chat
// when room is "".
func newReader(t *testing.T, base, room, name, lang string, translate bool) *reader {
	t.Helper()
	b := newBrowser(t, base)
	if code := b.do("POST", "/auth/language", map[string]any{"language": lang}, nil); code != 200 {
		t.Fatalf("language %s: %d", lang, code)
	}
	if !translate {
		if code := b.do("POST", "/auth/translate", map[string]any{"on": false}, nil); code != 200 {
			t.Fatalf("translate off: %d", code)
		}
	}
	path := "/ws-chat"
	if room != "" {
		path = wsPath(room, name, true, "spectator", true)
	}
	ctx, cancel := context.WithTimeout(context.Background(), wsDialTimeout)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, wsURL(base, path), &websocket.DialOptions{HTTPClient: b.c})
	if err != nil {
		t.Fatalf("%s's socket: %v", name, err)
	}
	t.Cleanup(func() { _ = conn.CloseNow() })
	if room != "" {
		if ev := readFirstEvent(t, conn); ev.Type != "joined" {
			t.Fatalf("%s's first event: %s", name, ev.Type)
		}
	}
	return &reader{browser: b, conn: conn}
}

type translationEvent struct {
	ID   int64  `json:"id"`
	Lang string `json:"lang"`
	From string `json:"from"`
	Text string `json:"text"`
}

// next is the reader's next chat or chat_translation event, or "" when none
// comes within wait.
func (r *reader) next(wait time.Duration) (string, translationEvent) {
	r.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	for {
		var ev wsEvent
		if err := wsjson.Read(ctx, r.conn, &ev); err != nil {
			return "", translationEvent{}
		}
		switch ev.Type {
		case "chat", "chat_translation", "dm", "dm-translation":
			var d translationEvent
			_ = json.Unmarshal(ev.Data, &d)
			return ev.Type, d
		}
	}
}

// dmReader is a signed-in browser reading its direct messages in lang.
func dmReader(t *testing.T, base string, b *browser, lang string) *reader {
	t.Helper()
	if code := b.do("POST", "/auth/language", map[string]any{"language": lang}, nil); code != 200 {
		t.Fatalf("language %s: %d", lang, code)
	}
	ctx, cancel := context.WithTimeout(context.Background(), wsDialTimeout)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, wsURL(base, "/ws-dm"), &websocket.DialOptions{HTTPClient: b.c})
	if err != nil {
		t.Fatalf("the direct messages' socket: %v", err)
	}
	t.Cleanup(func() { _ = conn.CloseNow() })
	if ev := readFirstEvent(t, conn); ev.Type != "ready" {
		t.Fatalf("first event: %s", ev.Type)
	}
	return &reader{browser: b, conn: conn}
}

// A direct message reaches the one it is written to in their language, the
// writer has it as written, and its translation is never stored (ADR 0105).
func TestChatTranslation_ADirectMessageIsTranslatedAndNeverStored(t *testing.T) {
	f := newFakeTranslator(t)
	a, base, dbPath := translatingAppDB(t, f.URL, "")
	anaB, beaB := member(t, a, base, "ana"), member(t, a, base, "bea")
	ana, bea := anaB.here(), beaB.here()
	befriend(t, anaB, beaB, "bea", "ana")
	beaR := dmReader(t, base, beaB, "es")
	anaR := dmReader(t, base, anaB, "en")

	var sent translatedLine
	if code := anaB.do("POST", "/api/dm", map[string]any{"to": bea, "text": "rematch, @bea?"}, &sent); code != 200 {
		t.Fatalf("Ana writes: %d", code)
	}
	if typ, _ := beaR.next(5 * time.Second); typ != "dm" {
		t.Fatalf("Bea's message: %s", typ)
	}
	if typ, ev := beaR.next(5 * time.Second); typ != "dm-translation" || ev.ID != sent.ID || ev.Lang != "es" || ev.Text != "[Spanish] rematch, @bea?" {
		t.Fatalf("Bea's translation: %s %+v", typ, ev)
	}
	if typ, _ := anaR.next(5 * time.Second); typ != "dm" {
		t.Fatalf("Ana's own message: %s", typ)
	}
	if typ, ev := anaR.next(300 * time.Millisecond); typ != "" {
		t.Errorf("the writer was sent %s %+v", typ, ev)
	}

	// The correspondence has it in Bea's language for her, and as written for
	// Ana, who wrote it.
	var read struct {
		Messages []translatedLine `json:"messages"`
	}
	beaB.do("POST", "/api/dm/history", map[string]any{"with": ana}, &read)
	if len(read.Messages) != 1 || read.Messages[0].Text != "rematch, @bea?" || read.Messages[0].Translation == nil || read.Messages[0].Translation.Text != "[Spanish] rematch, @bea?" {
		t.Fatalf("Bea's correspondence: %+v", read.Messages)
	}
	var own struct {
		Messages []translatedLine `json:"messages"`
	}
	anaB.do("POST", "/api/dm/history", map[string]any{"with": bea}, &own)
	if len(own.Messages) != 1 || own.Messages[0].Translation != nil {
		t.Fatalf("Ana's correspondence: %+v", own.Messages)
	}
	if n := len(f.requests()); n != 1 {
		t.Errorf("asked %d times", n)
	}

	// Nothing of it is in the database.
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var rows int
	if err := db.QueryRow(`SELECT (SELECT COUNT(*) FROM chat_translations) + (SELECT COUNT(*) FROM global_chat_translations)`).Scan(&rows); err != nil || rows != 0 {
		t.Errorf("stored translations: %d, %v", rows, err)
	}
}

type translatedLine struct {
	ID          int64  `json:"id"`
	Text        string `json:"text"`
	Translation *struct {
		Lang string `json:"lang"`
		From string `json:"from"`
		Text string `json:"text"`
	} `json:"translation"`
}

// hostRoom makes a game for a guest called host and returns its room code.
func hostRoom(t *testing.T, base, host string) (*browser, string) {
	t.Helper()
	b := newBrowser(t, base)
	var created struct {
		RoomCode string `json:"room_code"`
	}
	if code := b.do("POST", "/api/games", map[string]any{"game_def_id": "tic-tac-toe", "player_name": host}, &created); code != 200 {
		t.Fatalf("creating a game: %d", code)
	}
	return b, created.RoomCode
}

func TestChatTranslation_ALineReachesEachReaderInTheirLanguage(t *testing.T) {
	f := newFakeTranslator(t)
	base := translatingServer(t, f.URL, "")
	ana, room := hostRoom(t, base, "Ana")
	bea := newReader(t, base, room, "Bea", "es", true)
	chen := newReader(t, base, room, "Chen", "fr", true)
	dee := newReader(t, base, room, "Dee", "de", false)
	eli := newReader(t, base, room, "Eli", "en", true)

	var said translatedLine
	if code := ana.do("POST", "/api/games/"+room+"/chat", map[string]any{"player_name": "Ana", "text": "take a1-b2 now @bea, Ana"}, &said); code != 200 {
		t.Fatalf("chat: %d", code)
	}
	// Everyone has the line as written at once; a translation follows for
	// each reader in theirs, and for nobody else.
	for _, r := range []*reader{bea, chen, dee, eli} {
		if typ, _ := r.next(5 * time.Second); typ != "chat" {
			t.Fatalf("the line itself: %q", typ)
		}
	}
	for r, want := range map[*reader]string{bea: "es", chen: "fr"} {
		typ, ev := r.next(5 * time.Second)
		if typ != "chat_translation" || ev.ID != said.ID || ev.Lang != want || ev.From != "en" || !strings.HasSuffix(ev.Text, "] take a1-b2 now @bea, Ana") {
			t.Errorf("%s reader: %s %+v", want, typ, ev)
		}
	}
	for name, r := range map[string]*reader{"translation off": dee, "already in English": eli} {
		if typ, ev := r.next(300 * time.Millisecond); typ != "" {
			t.Errorf("%s: got %s %+v", name, typ, ev)
		}
	}
	// One request per language read, none for German, and the notation, the
	// handle and the player's name were never sent.
	asked := f.requests()
	if len(asked) != 3 {
		t.Fatalf("asked %d times: %q", len(asked), asked)
	}
	for _, a := range asked {
		if strings.Contains(a, "a1-b2") || strings.Contains(a, "@bea") || strings.Contains(a, "Ana") || strings.HasPrefix(a, "German") {
			t.Errorf("sent %q", a)
		}
	}

	// History keeps what was made, in each reader's language, and the line as
	// it was written; reading it asks for nothing again.
	for r, want := range map[*reader]string{bea: "[Spanish] ", chen: "[French] ", dee: "", eli: ""} {
		var log []translatedLine
		if code := r.do("GET", "/api/games/"+room+"/chat", nil, &log); code != 200 || len(log) != 1 {
			t.Fatalf("history: %d %+v", code, log)
		}
		if log[0].Text != "take a1-b2 now @bea, Ana" {
			t.Errorf("the original became %q", log[0].Text)
		}
		got := ""
		if log[0].Translation != nil {
			got = strings.TrimSuffix(log[0].Translation.Text, "take a1-b2 now @bea, Ana")
		}
		if got != want {
			t.Errorf("history in %q: %+v", want, log[0].Translation)
		}
	}
	if n := len(f.requests()); n != 3 {
		t.Errorf("reading history asked again: %d requests", n)
	}
}

// A reader who comes later has the lines said before them translated when
// their socket opens, whatever case the room's code was typed in.
func TestChatTranslation_ALateReaderHasTheRecentLinesTranslated(t *testing.T) {
	f := newFakeTranslator(t)
	base := translatingServer(t, f.URL, "")
	ana, room := hostRoom(t, base, "Ana")
	ana.do("POST", "/api/games/"+room+"/chat", map[string]any{"player_name": "Ana", "text": "good luck"}, nil)
	if n := len(f.requests()); n != 0 {
		t.Fatalf("translated for nobody: %d", n)
	}
	bea := newReader(t, base, strings.ToLower(room), "Bea", "es", true)
	if typ, ev := bea.next(5 * time.Second); typ != "chat_translation" || ev.Text != "[Spanish] good luck" {
		t.Fatalf("the earlier line: %s %+v", typ, ev)
	}
	var log []translatedLine
	bea.do("GET", "/api/games/"+room+"/chat", nil, &log)
	if len(log) != 1 || log[0].Translation == nil || log[0].Translation.Text != "[Spanish] good luck" {
		t.Errorf("history: %+v", log)
	}
}

func TestChatTranslation_TheGlobalChatIsTranslatedToo(t *testing.T) {
	f := newFakeTranslator(t)
	a, base := translatingApp(t, f.URL, "")
	// A guest may not speak in the global chat, but reads it in their language.
	bea := newReader(t, base, "", "Bea", "pt-BR", true)
	writer := signedIn(t, a, base, "ana")
	if code := writer.do("POST", "/api/chat", map[string]any{"player_name": "x", "text": "hola a todos"}, nil); code != 200 {
		t.Fatalf("chat: %d", code)
	}
	if typ, _ := bea.next(5 * time.Second); typ != "chat" {
		t.Fatalf("the line: %s", typ)
	}
	if typ, ev := bea.next(5 * time.Second); typ != "chat_translation" || ev.Lang != "pt-BR" || ev.From != "es" || ev.Text != "[Brazilian Portuguese] hola a todos" {
		t.Fatalf("the translation: %s %+v", typ, ev)
	}
}

func TestChatTranslation_ADownServiceChangesNothingElse(t *testing.T) {
	down := httptest.NewServer(http.NotFoundHandler())
	url := down.URL
	down.Close()
	base := translatingServer(t, url, "")
	ana, room := hostRoom(t, base, "Ana")
	bea := newReader(t, base, room, "Bea", "es", true)
	for _, text := range []string{"one", "two", "three", "four"} {
		if code := ana.do("POST", "/api/games/"+room+"/chat", map[string]any{"player_name": "Ana", "text": text}, nil); code != 200 {
			t.Fatalf("chat with the service down: %d", code)
		}
		if typ, _ := bea.next(5 * time.Second); typ != "chat" {
			t.Fatalf("the line %q: %s", text, typ)
		}
	}
	if typ, ev := bea.next(500 * time.Millisecond); typ != "" {
		t.Errorf("got %s %+v", typ, ev)
	}
	var log []translatedLine
	if code := bea.do("GET", "/api/games/"+room+"/chat", nil, &log); code != 200 || len(log) != 4 || log[3].Translation != nil {
		t.Errorf("history: %d %+v", code, log)
	}
}

func TestChatTranslation_TheWritersLimitHolds(t *testing.T) {
	caps := filepath.Join(t.TempDir(), "caps.yaml")
	if err := os.WriteFile(caps, []byte(`classes:
  guest:  [play, join, create_game, chat_game, translate]
  user:   [play, join, create_game, chat_game, translate]
limits:
  guest:  { chat: 100000, create_game: 100000, login: 100000, translate: 2 }
`), 0o600); err != nil {
		t.Fatal(err)
	}
	f := newFakeTranslator(t)
	base := translatingServer(t, f.URL, caps)
	ana, room := hostRoom(t, base, "Ana")
	bea := newReader(t, base, room, "Bea", "es", true)
	// Bea's socket catches up on the room's earlier lines (none) in the
	// background; a line it found would be asked for outside the writer's limit.
	time.Sleep(500 * time.Millisecond)
	for _, text := range []string{"one", "two", "three"} {
		if code := ana.do("POST", "/api/games/"+room+"/chat", map[string]any{"player_name": "Ana", "text": text}, nil); code != 200 {
			t.Fatalf("chat: %d", code)
		}
	}
	got := 0
	for {
		typ, _ := bea.next(500 * time.Millisecond)
		if typ == "" {
			break
		}
		if typ == "chat_translation" {
			got++
		}
	}
	if got != 2 || len(f.requests()) != 2 {
		t.Errorf("%d translations from %d requests, want 2 of each", got, len(f.requests()))
	}
}

func TestChatTranslation_TheSwitchIsOfferedOnlyWithAService(t *testing.T) {
	type me struct {
		Translation   bool `json:"translation"`
		TranslateChat bool `json:"translate_chat"`
	}
	plain := newServerWith(t, func(*config.Config) {})
	var m me
	newBrowser(t, plain).do("GET", "/auth/me", nil, &m)
	if m.Translation || !m.TranslateChat {
		t.Errorf("without a service: %+v", m)
	}
	f := newFakeTranslator(t)
	base := translatingServer(t, f.URL, "")
	b := newBrowser(t, base)
	b.do("GET", "/auth/me", nil, &m)
	if !m.Translation || !m.TranslateChat {
		t.Errorf("with a service: %+v", m)
	}
	if code := b.do("POST", "/auth/translate", map[string]any{"on": false}, nil); code != 200 {
		t.Fatalf("off: %d", code)
	}
	b.do("GET", "/auth/me", nil, &m)
	if m.TranslateChat {
		t.Errorf("turned off: %+v", m)
	}
	b.do("POST", "/auth/translate", map[string]any{"on": true}, nil)
	b.do("GET", "/auth/me", nil, &m)
	if !m.TranslateChat {
		t.Errorf("turned on again: %+v", m)
	}
}

// An account keeps its switch, so it follows the account to another browser.
func TestChatTranslation_AnAccountKeepsItsSwitch(t *testing.T) {
	f := newFakeTranslator(t)
	a, base := translatingApp(t, f.URL, "")
	ana := signedIn(t, a, base, "ana")
	if code := ana.do("POST", "/auth/translate", map[string]any{"on": false}, nil); code != 200 {
		t.Fatalf("off: %d", code)
	}
	other := newBrowser(t, base)
	if code := other.login("ana", anaPassword); code != 200 {
		t.Fatalf("login: %d", code)
	}
	var m struct {
		TranslateChat bool `json:"translate_chat"`
	}
	other.do("GET", "/auth/me", nil, &m)
	if m.TranslateChat {
		t.Error("the account's switch did not follow it")
	}
}
