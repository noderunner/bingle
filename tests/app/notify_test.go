package app_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/noderunner/bingle/internal/app"
	"github.com/noderunner/bingle/internal/config"
	"github.com/noderunner/bingle/internal/mail/fakesmtp"
)

const notifyFrom = "bingle_games@proton.me"

// notifyEnv is a bingle with email notifications set up against a fake SMTP server.
type notifyEnv struct {
	URL  string
	SMTP *fakesmtp.Server
	// keys are the guest keys of the seats this test took, by "ROOM/name".
	keys map[string]string
}

type notifyMods func(*config.Config)

func newNotifyEnv(t *testing.T, smtpOpts fakesmtp.Options, mods ...notifyMods) *notifyEnv {
	t.Helper()
	smtp := fakesmtp.Start(t, smtpOpts)
	cfg := newTestConfig(t)
	cfg.PublicURL = "https://bingle.example.test"
	cfg.TLSEmail = notifyFrom
	mode := "starttls"
	if smtpOpts.Implicit {
		mode = "tls"
	}
	cfg.Mail = config.Mail{Host: smtp.Host, Port: smtp.Port, User: notifyFrom, Password: config.NewSecret("pw-123"), From: notifyFrom,
		Mode: mode, CAFile: smtp.CAFile, HourlyLimit: 50, DailyLimit: 500, AwayUnit: 20 * time.Millisecond}
	for _, m := range mods {
		m(&cfg)
	}
	a, err := app.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go a.Manager().RunNotifier(ctx)
	t.Cleanup(func() { cancel(); _ = a.Close() })
	srv := httptest.NewServer(a.Router())
	t.Cleanup(srv.Close)
	return &notifyEnv{URL: srv.URL, SMTP: smtp, keys: map[string]string{}}
}

type contactResp struct {
	ID               int64  `json:"contact_id"`
	Secret           string `json:"contact_secret"`
	Status           string `json:"status"`
	Masked           string `json:"masked"`
	ConfirmationSent bool   `json:"confirmation_sent"`
	Note             string `json:"note"`
}

// seatGame is a game and the guest key of the seat that made it: a seat's
// email setting is its owner's alone (ADR 0067).
type seatGame struct {
	Room  string
	Token string
}

// createGame makes a game as a guest of its own.
func (e *notifyEnv) createGame(t *testing.T, def, name string) seatGame {
	t.Helper()
	key := newGuestKey(t, e.URL)
	var out struct {
		RoomCode string `json:"room_code"`
	}
	if code := doJSONAs(t, key, "POST", e.URL+"/api/games", map[string]any{"game_def_id": def, "player_name": name}, &out); code != 200 {
		t.Fatalf("create: %d", code)
	}
	e.keys[out.RoomCode+"/"+strings.ToLower(name)] = key
	return seatGame{out.RoomCode, key}
}

// join takes a seat as a guest of its own, and returns the guest's key.
func (e *notifyEnv) join(t *testing.T, room, name string) string {
	t.Helper()
	key := newGuestKey(t, e.URL)
	if code := doJSONAs(t, key, "POST", e.URL+"/api/games/"+room+"/join", map[string]any{"player_name": name}, nil); code != 200 {
		t.Fatalf("join: %d", code)
	}
	e.keys[room+"/"+strings.ToLower(name)] = key
	return key
}

// move plays a move for name, as the guest that holds the seat.
func (e *notifyEnv) move(t *testing.T, room, name, notation string) {
	t.Helper()
	if code := doJSONAs(t, e.keys[room+"/"+strings.ToLower(name)], "POST", e.URL+"/api/games/"+room+"/moves", map[string]any{"player_name": name, "notation": notation}, nil); code != 200 {
		t.Fatalf("%s plays %s: %d", name, notation, code)
	}
}

// getNotify reads a seat's setting as the guest whose key is key.
func (e *notifyEnv) getNotify(t *testing.T, room, name, key string) (int, []byte) {
	t.Helper()
	req, _ := http.NewRequest("GET", e.URL+"/api/games/"+room+"/notify?player_name="+name, nil)
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b
}

// waitMails waits until the fake server holds n mails (or fails).
func (e *notifyEnv) waitMails(t *testing.T, n int) []fakesmtp.Mail {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if got := e.SMTP.Mails(); len(got) >= n {
			return got
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("waited for %d mails, have %d", n, len(e.SMTP.Mails()))
	return nil
}

// quiet asserts that no more than n mails exist after a short wait.
func (e *notifyEnv) quiet(t *testing.T, n int) {
	t.Helper()
	time.Sleep(500 * time.Millisecond)
	if got := len(e.SMTP.Mails()); got != n {
		t.Fatalf("%d mails, want %d: %+v", got, n, e.SMTP.Mails())
	}
}

var linkRe = regexp.MustCompile(`https://bingle\.example\.test(/notify/(?:confirm|stop)\?t=[A-Za-z0-9_-]+)`)

// link finds the path of the confirm or stop link in a mail.
func link(t *testing.T, m fakesmtp.Mail, kind string) string {
	t.Helper()
	for _, l := range linkRe.FindAllStringSubmatch(m.Data, -1) {
		if strings.HasPrefix(l[1], "/notify/"+kind) {
			return l[1]
		}
	}
	t.Fatalf("no %s link in:\n%s", kind, m.Data)
	return ""
}

func (e *notifyEnv) saveContact(t *testing.T, address string) (int, contactResp) {
	t.Helper()
	var out contactResp
	code := doJSON(t, "POST", e.URL+"/api/notify/contact", map[string]any{"address": address}, &out)
	return code, out
}

func post(t *testing.T, url string) (int, string) {
	t.Helper()
	resp, err := http.Post(url, "application/x-www-form-urlencoded", strings.NewReader("List-Unsubscribe=One-Click"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func (e *notifyEnv) setNotify(t *testing.T, g seatGame, name string, c contactResp, enabled bool, away int) (int, map[string]any) {
	t.Helper()
	var out map[string]any
	code := doJSONAs(t, g.Token, "PUT", e.URL+"/api/games/"+g.Room+"/notify", map[string]any{
		"player_name": name, "contact_id": c.ID, "contact_secret": c.Secret, "enabled": enabled, "away_minutes": away}, &out)
	return code, out
}

// verified makes a confirmed contact for the address.
func (e *notifyEnv) verified(t *testing.T, address string) contactResp {
	t.Helper()
	// Count before saving: the confirmation mail may be delivered before the
	// response returns, and a count taken after would wait for one more.
	n := len(e.SMTP.Mails())
	code, c := e.saveContact(t, address)
	if code != 200 || c.Status != "pending" || c.Secret == "" || !c.ConfirmationSent {
		t.Fatalf("save: %d %+v", code, c)
	}
	m := e.waitMails(t, n+1)
	last := m[len(m)-1]
	if st, body := post(t, e.URL+link(t, last, "confirm")); st != 200 {
		t.Fatalf("confirm: %d %s", st, body)
	}
	return c
}

func TestNotify_OffWithoutMailSetup(t *testing.T) {
	base := newRESTTestServer(t)
	var cfg map[string]any
	if code := doJSON(t, "GET", base+"/api/notify/config", nil, &cfg); code != 200 || cfg["enabled"] != false {
		t.Errorf("config: %d %v", code, cfg)
	}
	var out map[string]any
	if code := doJSON(t, "POST", base+"/api/notify/contact", map[string]any{"address": "a@example.com"}, &out); code != 404 {
		t.Errorf("contact on a server without mail: %d, want 404", code)
	}
	room := newTakRoom(t, base, "tic-tac-toe", "Alice")
	if code := doJSON(t, "GET", base+"/api/games/"+room+"/notify?player_name=Alice", nil, &out); code != 404 {
		t.Errorf("game setting: %d, want 404", code)
	}
}

func TestNotify_ConfigNamesTheSender(t *testing.T) {
	e := newNotifyEnv(t, fakesmtp.Options{})
	var cfg struct {
		Enabled bool   `json:"enabled"`
		From    string `json:"from"`
	}
	if code := doJSON(t, "GET", e.URL+"/api/notify/config", nil, &cfg); code != 200 || !cfg.Enabled || cfg.From != notifyFrom {
		t.Errorf("config: %d %+v", code, cfg)
	}
}

// A seat's setting is for the guest that holds the seat: not for another
// seat's guest, not for a request with no guest at all.
func TestNotify_ASeatsSettingIsItsOwners(t *testing.T) {
	e := newNotifyEnv(t, fakesmtp.Options{})
	g := e.createGame(t, "tic-tac-toe", "Alice")
	bob := e.join(t, g.Room, "Bob")
	if code, _ := e.getNotify(t, g.Room, "Alice", g.Token); code != 200 {
		t.Errorf("Alice reads her own setting: %d, want 200", code)
	}
	if code, _ := e.getNotify(t, g.Room, "Alice", bob); code != 403 {
		t.Errorf("Bob reads Alice's setting: %d, want 403", code)
	}
	if code, _ := e.getNotify(t, g.Room, "Alice", ""); code != 403 {
		t.Errorf("an anonymous request reads Alice's setting: %d, want 403", code)
	}
}

func TestNotify_ConfirmationFlowAndTurnMail(t *testing.T) {
	e := newNotifyEnv(t, fakesmtp.Options{})
	g := e.createGame(t, "tic-tac-toe", "Alice")

	code, c := e.saveContact(t, "Alice.Player@Example.com")
	if code != 200 || c.Status != "pending" || c.ID == 0 || c.Secret == "" || !c.ConfirmationSent || c.Masked != "A••••@example.com" {
		t.Fatalf("save: %d %+v", code, c)
	}
	conf := e.waitMails(t, 1)[0]
	if conf.From != notifyFrom || len(conf.To) != 1 || conf.To[0] != "Alice.Player@example.com" || conf.User != notifyFrom || conf.AuthedInClear {
		t.Errorf("confirmation mail: %+v", conf)
	}
	if !strings.Contains(conf.Data, "Subject: Confirm your email address for Bingle") || !strings.Contains(conf.Data, "List-Unsubscribe:") {
		t.Errorf("confirmation mail:\n%s", conf.Data)
	}

	// Not confirmed yet: it cannot be switched on, and a status check says so.
	if st, _ := e.setNotify(t, g, "Alice", c, true, 0); st != 409 {
		t.Errorf("enabling with an unconfirmed address: %d, want 409", st)
	}
	var status contactResp
	doJSON(t, "POST", e.URL+"/api/notify/contact/status", map[string]any{"contact_id": c.ID, "contact_secret": c.Secret}, &status)
	if status.Status != "pending" {
		t.Errorf("status = %+v", status)
	}

	// Opening the link (GET) shows a page and confirms nothing; the token works once, by POST.
	path := link(t, conf, "confirm")
	resp, err := http.Get(e.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	page, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(page), `id="notify-go"`) {
		t.Errorf("confirm page: %d", resp.StatusCode)
	}
	doJSON(t, "POST", e.URL+"/api/notify/contact/status", map[string]any{"contact_id": c.ID, "contact_secret": c.Secret}, &status)
	if status.Status != "pending" {
		t.Errorf("a GET of the link confirmed the address: %+v", status)
	}
	if st, body := post(t, e.URL+path); st != 200 || !strings.Contains(body, `"ok":true`) {
		t.Fatalf("confirm: %d %s", st, body)
	}
	if st, _ := post(t, e.URL+path); st != 403 {
		t.Errorf("the token worked twice: %d", st)
	}
	doJSON(t, "POST", e.URL+"/api/notify/contact/status", map[string]any{"contact_id": c.ID, "contact_secret": c.Secret}, &status)
	if status.Status != "verified" {
		t.Fatalf("status = %+v", status)
	}

	// Switch it on for the game (every time).
	st, set := e.setNotify(t, g, "Alice", c, true, 0)
	if st != 200 || set["enabled"] != true {
		t.Fatalf("enable: %d %v", st, set)
	}
	// Bob joins: the game starts and it is Alice's turn; she is not connected, so she is mailed.
	e.join(t, g.Room, "Bob")
	turn := e.waitMails(t, 2)[1]
	if len(turn.To) != 1 || turn.To[0] != "Alice.Player@example.com" || turn.From != notifyFrom {
		t.Errorf("turn mail envelope: %+v", turn)
	}
	for _, want := range []string{"Subject: It is your turn in Tic-Tac-Toe (" + g.Room + ")", "https://bingle.example.test/game/" + g.Room, "List-Unsubscribe: <https://bingle.example.test/notify/stop?t=", "List-Unsubscribe-Post: List-Unsubscribe=One-Click"} {
		if !strings.Contains(turn.Data, want) {
			t.Errorf("turn mail lacks %q:\n%s", want, turn.Data)
		}
	}
	if strings.Contains(turn.Data, "Bob") {
		t.Error("the mail names the other player: only fixed text and the game are allowed")
	}

	// Alice moves, Bob moves: it is Alice's turn again, but one mail per game per 15 minutes is the limit.
	e.move(t, g.Room, "Alice", "a1")
	e.move(t, g.Room, "Bob", "b2")
	e.quiet(t, 2)
}

func TestNotify_NoMailWhileThePlayerIsConnected(t *testing.T) {
	e := newNotifyEnv(t, fakesmtp.Options{})
	g := e.createGame(t, "tic-tac-toe", "Alice")
	c := e.verified(t, "alice@example.com")
	if st, _ := e.setNotify(t, g, "Alice", c, true, 0); st != 200 {
		t.Fatal(st)
	}
	n := len(e.SMTP.Mails())
	_ = dialRoomAs(t, e.keys[g.Room+"/alice"], e.URL, g.Room, "Alice", "player", "") // Alice has the game open
	time.Sleep(200 * time.Millisecond)
	e.join(t, g.Room, "Bob")
	e.quiet(t, n)
}

func TestNotify_AwayModeWaitsAndChecksTheTurn(t *testing.T) {
	e := newNotifyEnv(t, fakesmtp.Options{}) // an "away minute" is 20 ms here
	g := e.createGame(t, "tic-tac-toe", "Alice")
	c := e.verified(t, "alice@example.com")
	if st, _ := e.setNotify(t, g, "Alice", c, true, 40); st != 200 { // 40 away-minutes = 800 ms
		t.Fatal(st)
	}
	n := len(e.SMTP.Mails())
	_ = dialRoomAs(t, e.keys[g.Room+"/alice"], e.URL, g.Room, "Alice", "player", "") // connected, but the away mode mails anyway
	e.join(t, g.Room, "Bob")
	e.quiet(t, n) // not yet: the wait (800 ms) has not passed
	e.waitMails(t, n+1)

	// If the turn moves on before the wait is over, nothing is sent.
	g2 := e.createGame(t, "tic-tac-toe", "Alice")
	c2 := c
	if st, _ := e.setNotify(t, g2, "Alice", c2, true, 100); st != 200 { // 2 s
		t.Fatal(st)
	}
	e.join(t, g2.Room, "Bob")
	e.move(t, g2.Room, "Alice", "a1")
	time.Sleep(2500 * time.Millisecond)
	if got := len(e.SMTP.Mails()); got != n+1 {
		t.Errorf("%d mails, want %d: a mail came for a turn that had passed", got, n+1)
	}
}

func TestNotify_BotsAndOthersAreNotMailed(t *testing.T) {
	e := newNotifyEnv(t, fakesmtp.Options{})
	g := e.createGame(t, "tic-tac-toe", "Alice")
	c := e.verified(t, "alice@example.com")
	e.setNotify(t, g, "Alice", c, true, 0)
	n := len(e.SMTP.Mails())
	// Bob has no setting: his turn mails nobody.
	e.join(t, g.Room, "Bob")
	e.waitMails(t, n+1) // Alice's own first turn
	e.move(t, g.Room, "Alice", "a1")
	e.quiet(t, n+1)
}

func TestNotify_WrongSecretsChangeNothing(t *testing.T) {
	e := newNotifyEnv(t, fakesmtp.Options{})
	g := e.createGame(t, "tic-tac-toe", "Alice")
	bobToken := e.join(t, g.Room, "Bob")
	c := e.verified(t, "alice@example.com")

	for name, tc := range map[string]struct {
		key  string
		req  map[string]any
		want int
	}{
		"no guest":               {"", map[string]any{"player_name": "Alice", "contact_id": c.ID, "contact_secret": c.Secret, "enabled": true}, 403},
		"another seat's guest":   {bobToken, map[string]any{"player_name": "Alice", "contact_id": c.ID, "contact_secret": c.Secret, "enabled": true}, 403},
		"a made-up guest key":    {"bgg_nope", map[string]any{"player_name": "Alice", "contact_id": c.ID, "contact_secret": c.Secret, "enabled": true}, 401},
		"no contact secret":      {g.Token, map[string]any{"player_name": "Alice", "contact_id": c.ID, "enabled": true}, 403},
		"a wrong contact secret": {g.Token, map[string]any{"player_name": "Alice", "contact_id": c.ID, "contact_secret": "nope", "enabled": true}, 403},
	} {
		var out map[string]any
		if code := doJSONAs(t, tc.key, "PUT", e.URL+"/api/games/"+g.Room+"/notify", tc.req, &out); code != tc.want {
			t.Errorf("%s: %d, want %d", name, code, tc.want)
		}
	}
	// Reading is the owner's too (it also shows the masked address).
	var out map[string]any
	if code, _ := e.getNotify(t, g.Room, "Alice", ""); code != 403 {
		t.Errorf("GET with no guest: %d, want 403", code)
	}
	if code, _ := e.getNotify(t, g.Room, "Alice", g.Token); code != 200 {
		t.Fatalf("GET as the owner: %d", code)
	}
	// Nobody else's contact can be changed or read.
	for _, path := range []string{"contact", "contact/status", "contact/resend", "contact/forget"} {
		body := map[string]any{"contact_id": c.ID, "contact_secret": "guess", "address": "x@example.com"}
		if code := doJSON(t, "POST", e.URL+"/api/notify/"+path, body, &out); code != 403 {
			t.Errorf("%s with a wrong secret: %d, want 403", path, code)
		}
	}
	// A stranger's name is not a seat.
	if st, _ := e.setNotify(t, seatGame{g.Room, g.Token}, "Mallory", c, true, 0); st != 403 {
		t.Errorf("a stranger: %d, want 403", st)
	}
	e.quiet(t, len(e.SMTP.Mails()))
}

func TestNotify_AddressIsNeverReturned(t *testing.T) {
	e := newNotifyEnv(t, fakesmtp.Options{})
	const addr = "very.secret.person@example.org"
	g := e.createGame(t, "tic-tac-toe", "Alice")
	c := e.verified(t, addr)
	e.setNotify(t, g, "Alice", c, true, 0)
	e.join(t, g.Room, "Bob")
	e.move(t, g.Room, "Alice", "a1")

	paths := []string{"/api/games/" + g.Room, "/api/games/" + g.Room + "/state", "/api/games/" + g.Room + "/moves", "/api/games/" + g.Room + "/history",
		"/api/games/" + g.Room + "/export", "/api/games/" + g.Room + "/chat", "/api/games/" + g.Room + "/presence", "/api/games/" + g.Room + "/voice",
		"/api/games/" + g.Room + "/reactions?player_name=Alice", "/api/config", "/api/notify/config", "/api/chat", "/game/" + g.Room, "/", "/profile"}
	for _, p := range paths {
		resp, err := http.Get(e.URL + p)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if strings.Contains(string(b), addr) || strings.Contains(string(b), "very.secret") {
			t.Errorf("GET %s shows the address", p)
		}
	}
	// What the owner sees is masked.
	var own map[string]any
	_, b := e.getNotify(t, g.Room, "Alice", g.Token)
	_ = json.Unmarshal(b, &own)
	if strings.Contains(string(b), addr) || !strings.Contains(string(b), "v••••@example.org") {
		t.Errorf("owner's view = %s", b)
	}
	// The WebSocket frames of the room carry no address either.
	ws := dialRoomAs(t, e.keys[g.Room+"/bob"], e.URL, g.Room, "Bob", "player", "")
	e.move(t, g.Room, "Bob", "b2")
	if ev, ok := nextEvent(t, ws, "move", 3*time.Second); !ok || strings.Contains(string(ev.Data), "very.secret") {
		t.Errorf("move event: %v %s", ok, ev.Data)
	}
}

func TestNotify_StopLinkSilencesTheAddressForGood(t *testing.T) {
	e := newNotifyEnv(t, fakesmtp.Options{})
	g := e.createGame(t, "tic-tac-toe", "Alice")
	c := e.verified(t, "alice@example.com")
	e.setNotify(t, g, "Alice", c, true, 0)
	e.join(t, g.Room, "Bob")
	turn := e.waitMails(t, 2)[1]
	stop := link(t, turn, "stop")

	// A mail program's one-click POST works, and a plain visit does not stop anything by itself.
	if resp, err := http.Get(e.URL + stop); err != nil || resp.StatusCode != 200 {
		t.Fatalf("stop page: %v %v", resp, err)
	}
	var status contactResp
	doJSON(t, "POST", e.URL+"/api/notify/contact/status", map[string]any{"contact_id": c.ID, "contact_secret": c.Secret}, &status)
	if status.Status != "verified" {
		t.Fatalf("a GET stopped the mail: %+v", status)
	}
	if st, body := post(t, e.URL+stop); st != 200 {
		t.Fatalf("stop: %d %s", st, body)
	}
	doJSON(t, "POST", e.URL+"/api/notify/contact/status", map[string]any{"contact_id": c.ID, "contact_secret": c.Secret}, &status)
	if status.Status != "stopped" {
		t.Errorf("status = %+v", status)
	}
	// No more turn mail ...
	n := len(e.SMTP.Mails())
	e.move(t, g.Room, "Alice", "a1")
	e.move(t, g.Room, "Bob", "b2")
	e.quiet(t, n)
	// ... it cannot be switched on again, and nobody can have it mailed under another contact.
	if st, _ := e.setNotify(t, g, "Alice", c, true, 0); st != 409 {
		t.Errorf("enabling a stopped address: %d, want 409", st)
	}
	if code, _ := e.saveContact(t, "Alice@Example.com"); code != 409 {
		t.Errorf("registering a stopped address: %d, want 409", code)
	}
	if st, _ := post(t, e.URL+"/notify/stop?t=wrong"); st != 403 {
		t.Errorf("a wrong stop token: %d, want 403", st)
	}
}

func TestNotify_BadAddressesAreRefused(t *testing.T) {
	e := newNotifyEnv(t, fakesmtp.Options{})
	for _, a := range []string{"", "nobody", "a@b", "a@example.com\r\nBcc: v@example.com", "a@example.com,b@example.com", "Bob <a@example.com>", strings.Repeat("a", 300) + "@example.com"} {
		if code, _ := e.saveContact(t, a); code != 400 {
			t.Errorf("address %q: %d, want 400", a, code)
		}
	}
	e.quiet(t, 0)
}

func TestNotify_ConfirmationMailIsLimited(t *testing.T) {
	e := newNotifyEnv(t, fakesmtp.Options{})
	code, c := e.saveContact(t, "limited@example.com")
	if code != 200 || !c.ConfirmationSent {
		t.Fatal(code, c)
	}
	e.waitMails(t, 1)
	// The same address again, at once: no second mail.
	var again contactResp
	doJSON(t, "POST", e.URL+"/api/notify/contact", map[string]any{"address": "limited@example.com", "contact_id": c.ID, "contact_secret": c.Secret}, &again)
	if again.ConfirmationSent || again.Note == "" {
		t.Errorf("resent at once: %+v", again)
	}
	var resent contactResp
	doJSON(t, "POST", e.URL+"/api/notify/contact/resend", map[string]any{"contact_id": c.ID, "contact_secret": c.Secret}, &resent)
	if resent.ConfirmationSent {
		t.Errorf("resend at once: %+v", resent)
	}
	e.quiet(t, 1)

	// One client cannot make the server mail many addresses: 5 per hour per client.
	sent := 1
	for i := 0; i < 8; i++ {
		_, got := e.saveContact(t, fmt.Sprintf("victim%d@example.com", i))
		if got.ConfirmationSent {
			sent++
		}
	}
	if sent != 5 {
		t.Errorf("%d confirmation mails from one client in an hour, want 5", sent)
	}
}

func TestNotify_ServerWideLimit(t *testing.T) {
	e := newNotifyEnv(t, fakesmtp.Options{}, func(c *config.Config) { c.Mail.HourlyLimit = 3 })
	for i := 0; i < 5; i++ {
		e.saveContact(t, fmt.Sprintf("p%d@example.com", i))
	}
	e.waitMails(t, 3)
	e.quiet(t, 3)
}

func TestNotify_BadLoginStopsTheMailer(t *testing.T) {
	e := newNotifyEnv(t, fakesmtp.Options{AuthFail: true})
	e.saveContact(t, "a@example.com")
	deadline := time.Now().Add(5 * time.Second)
	for e.SMTP.AuthAttempts() == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if e.SMTP.AuthAttempts() != 1 {
		t.Fatalf("%d login attempts", e.SMTP.AuthAttempts())
	}
	// Nothing more is tried: a wrong password is not retried into a lockout.
	e.saveContact(t, "b@example.com")
	e.saveContact(t, "c@example.com")
	time.Sleep(600 * time.Millisecond)
	if n := e.SMTP.AuthAttempts(); n != 1 {
		t.Errorf("%d login attempts after the failure, want 1", n)
	}
}

func TestNotify_ChangingTheAddressNeedsConfirmationAndWarnsTheOldOne(t *testing.T) {
	e := newNotifyEnv(t, fakesmtp.Options{})
	g := e.createGame(t, "tic-tac-toe", "Alice")
	c := e.verified(t, "old@example.com")
	e.setNotify(t, g, "Alice", c, true, 0)
	n := len(e.SMTP.Mails())

	var changed contactResp
	if code := doJSON(t, "POST", e.URL+"/api/notify/contact", map[string]any{"address": "new@example.com", "contact_id": c.ID, "contact_secret": c.Secret}, &changed); code != 200 || changed.Status != "pending" {
		t.Fatalf("change: %d %+v", code, changed)
	}
	mails := e.waitMails(t, n+2)
	var toOld, toNew bool
	for _, m := range mails[n:] {
		switch m.To[0] {
		case "old@example.com":
			toOld = strings.Contains(m.Data, "was changed")
		case "new@example.com":
			toNew = strings.Contains(m.Data, "/notify/confirm?t=")
		}
	}
	if !toOld || !toNew {
		t.Errorf("notice to the old address: %v, confirmation to the new: %v", toOld, toNew)
	}
	// Until the new one is confirmed, nothing is sent to either.
	e.join(t, g.Room, "Bob")
	e.quiet(t, n+2)
}

func TestNotify_ForgetDeletesTheContactAndItsSettings(t *testing.T) {
	e := newNotifyEnv(t, fakesmtp.Options{})
	g := e.createGame(t, "tic-tac-toe", "Alice")
	c := e.verified(t, "gone@example.com")
	e.setNotify(t, g, "Alice", c, true, 0)
	var out map[string]any
	if code := doJSON(t, "POST", e.URL+"/api/notify/contact/forget", map[string]any{"contact_id": c.ID, "contact_secret": c.Secret}, &out); code != 200 {
		t.Fatalf("forget: %d", code)
	}
	if code := doJSON(t, "POST", e.URL+"/api/notify/contact/status", map[string]any{"contact_id": c.ID, "contact_secret": c.Secret}, &out); code != 403 {
		t.Errorf("the contact still answers: %d", code)
	}
	n := len(e.SMTP.Mails())
	e.join(t, g.Room, "Bob")
	e.quiet(t, n)
}

func TestNotify_ImplicitTLSServer(t *testing.T) {
	e := newNotifyEnv(t, fakesmtp.Options{Implicit: true})
	if code, c := e.saveContact(t, "tls@example.com"); code != 200 || !c.ConfirmationSent {
		t.Fatal(code, c)
	}
	if m := e.waitMails(t, 1)[0]; m.AuthedInClear {
		t.Error("login in the clear")
	}
}

func TestNotify_RefusesAServerWithoutSTARTTLS(t *testing.T) {
	e := newNotifyEnv(t, fakesmtp.Options{NoSTARTTLS: true})
	e.saveContact(t, "a@example.com")
	time.Sleep(700 * time.Millisecond)
	if e.SMTP.AuthAttempts() != 0 || len(e.SMTP.Mails()) != 0 {
		t.Errorf("the password or a mail went to a server without STARTTLS")
	}
}
