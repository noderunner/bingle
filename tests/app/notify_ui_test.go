package app_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/noderunner/bingle/internal/mail/fakesmtp"
)

func (e *notifyEnv) seedBrowser(t *testing.T, run func(actions ...chromedp.Action) error, kv map[string]any) {
	t.Helper()
	if err := run(chromedp.Navigate(e.URL + "/healthz")); err != nil {
		t.Fatal(err)
	}
	for k, v := range kv {
		b, _ := json.Marshal(v)
		if err := run(chromedp.Evaluate(fmt.Sprintf(`localStorage.setItem(%q, %q); true`, k, string(b)), nil)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestNotifyUI_ProfileOffWhenTheServerCannotMail(t *testing.T) {
	base := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 30*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.Navigate(base+"/profile"), chromedp.WaitVisible("#profile-form", chromedp.ByID)); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the notice", `!document.getElementById("notify-off").hidden`, 10*time.Second)
	if evalBool(t, ctx, `!document.getElementById("notify-box").hidden || !!document.getElementById("notify-whitelist")`) {
		t.Error("mail settings show on a server that cannot send mail")
	}
}

// The profile: save an address, get the confirmation mail, follow its link, and see the page notice it.
func TestNotifyUI_ProfileAddressAndWhitelistNote(t *testing.T) {
	e := newNotifyEnv(t, fakesmtp.Options{})
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.Navigate(e.URL+"/profile"), chromedp.WaitVisible("#notify-box", chromedp.ByID)); err != nil {
		t.Fatal(err)
	}
	// The player is told where the mail will come from.
	waitJS(t, ctx, "whitelist note", `document.getElementById("notify-whitelist")`, 10*time.Second)
	if got := evalString(t, ctx, `document.querySelector("#notify-whitelist .notify-from").textContent`); got != notifyFrom {
		t.Errorf("the note names %q, want %q", got, notifyFrom)
	}
	if got := evalString(t, ctx, `document.getElementById("notify-whitelist").textContent`); !strings.Contains(got, "safe-sender") && !strings.Contains(got, "contacts") {
		t.Errorf("note = %q", got)
	}

	evalString(t, ctx, `document.getElementById("profile_email").value = "uitest@example.com"; document.getElementById("profile_notify").checked = true; "ok"`)
	clickSel(t, ctx, "#profile-save")
	waitJS(t, ctx, "confirmation sent", `document.getElementById("notify-status").textContent.indexOf("u••••@example.com") >= 0`, 10*time.Second)
	m := e.waitMails(t, 1)[0]
	if m.To[0] != "uitest@example.com" {
		t.Errorf("mail to %v", m.To)
	}
	// Following the link in the mail: the page asks for a click, and the profile page notices.
	if st, _ := post(t, e.URL+link(t, m, "confirm")); st != 200 {
		t.Fatal("confirm failed")
	}
	waitJS(t, ctx, "confirmed", `document.getElementById("notify-status").textContent.indexOf("Confirmed") === 0`, 15*time.Second)

	// Reload: the address is still known (kept in this browser), the status too.
	if err := chromedp.Run(ctx, chromedp.Navigate(e.URL+"/profile"), chromedp.WaitVisible("#notify-box", chromedp.ByID)); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "still confirmed", `document.getElementById("notify-status").textContent.indexOf("Confirmed") === 0 && document.getElementById("profile_email").value === "uitest@example.com"`, 10*time.Second)

	// Forgetting deletes it on the server.
	clickSel(t, ctx, "#notify-forget")
	waitJS(t, ctx, "forgotten", `document.getElementById("notify-status").textContent.indexOf("deleted") >= 0`, 10*time.Second)
}

// The game's options: the setting of the seat, with the note about the sender.
func TestNotifyUI_GameOptionsSection(t *testing.T) {
	e := newNotifyEnv(t, fakesmtp.Options{})
	g := e.createGame(t, "tic-tac-toe", "Alice")
	e.join(t, g.Room, "Bob")
	c := e.verified(t, "alice@example.com")

	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	e.seedBrowser(t, func(a ...chromedp.Action) error { return chromedp.Run(ctx, a...) }, map[string]any{
		"bingle.games":   []map[string]any{{"code": g.Room, "def": "tic-tac-toe", "name": "Alice", "ts": time.Now().UnixMilli()}},
		"bingle.name":    "Alice",
		"bingle.tokens":  map[string]string{g.Room: g.Token},
		"bingle.contact": map[string]any{"id": c.ID, "secret": c.Secret, "address": "alice@example.com"},
	})
	if err := chromedp.Run(ctx, chromedp.Navigate(e.URL+"/game/"+g.Room), chromedp.WaitReady("#board-svg", chromedp.ByQuery)); err != nil {
		t.Fatal(err)
	}
	clickSel(t, ctx, "#open-options")
	waitJS(t, ctx, "the section", `document.getElementById("notify-on") && !document.getElementById("notify-on").disabled`, 15*time.Second)
	if got := evalString(t, ctx, `document.querySelector("#options-notify .notify-from").textContent`); got != notifyFrom {
		t.Errorf("the options name %q as the sender, want %q", got, notifyFrom)
	}
	if got := evalString(t, ctx, `document.getElementById("notify-hint").textContent`); !strings.Contains(got, "a••••@example.com") {
		t.Errorf("hint = %q", got)
	}
	// Switch it on, choose the "away" mode: the server has it.
	clickSel(t, ctx, "#notify-on")
	waitJS(t, ctx, "saved", `document.getElementById("notify-hint").textContent.indexOf("Saved") === 0`, 10*time.Second)
	clickSel(t, ctx, "#notify-mode-away")
	waitJS(t, ctx, "saved away", `document.getElementById("notify-hint").textContent.indexOf("Saved") === 0 && !document.getElementById("notify-away").disabled`, 10*time.Second)
	req, _ := http.NewRequest("GET", e.URL+"/api/games/"+g.Room+"/notify?player_name=Alice", nil)
	req.Header.Set("X-Seat-Token", g.Token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	var got struct {
		Enabled     bool `json:"enabled"`
		AwayMinutes int  `json:"away_minutes"`
	}
	_ = json.Unmarshal(b, &got)
	if !got.Enabled || got.AwayMinutes != 30 {
		t.Errorf("server state = %s", b)
	}
}

// Without the seat's secret (another browser) the setting cannot be changed from the page.
func TestNotifyUI_NoSeatSecretNoControls(t *testing.T) {
	e := newNotifyEnv(t, fakesmtp.Options{})
	g := e.createGame(t, "tic-tac-toe", "Alice")
	e.join(t, g.Room, "Bob")
	ctx, cancel := newChromedpContext(t, 40*time.Second)
	defer cancel()
	openGameAs(t, ctx, e.URL, g.Room, "tic-tac-toe", "Alice") // no bingle.tokens
	clickSel(t, ctx, "#open-options")
	waitJS(t, ctx, "the explanation", `document.getElementById("notify-no-token")`, 15*time.Second)
	if evalBool(t, ctx, `!!document.getElementById("notify-on")`) {
		t.Error("a switch is offered without the seat's secret")
	}
	// The note about the sender is shown anyway.
	if !evalBool(t, ctx, `!!document.getElementById("notify-whitelist")`) {
		t.Error("no note about the sender")
	}
}

// The profile's switch is the default for a game the player has not chosen for.
func TestNotifyUI_ProfileSwitchAppliesToANewGame(t *testing.T) {
	e := newNotifyEnv(t, fakesmtp.Options{})
	g := e.createGame(t, "tic-tac-toe", "Alice")
	e.join(t, g.Room, "Bob")
	c := e.verified(t, "alice@example.com")
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	e.seedBrowser(t, func(a ...chromedp.Action) error { return chromedp.Run(ctx, a...) }, map[string]any{
		"bingle.games":   []map[string]any{{"code": g.Room, "def": "tic-tac-toe", "name": "Alice", "ts": time.Now().UnixMilli()}},
		"bingle.name":    "Alice",
		"bingle.tokens":  map[string]string{g.Room: g.Token},
		"bingle.contact": map[string]any{"id": c.ID, "secret": c.Secret, "address": "alice@example.com"},
		"bingle.profile": map[string]any{"email": "alice@example.com", "notify": true},
	})
	if err := chromedp.Run(ctx, chromedp.Navigate(e.URL+"/game/"+g.Room), chromedp.WaitReady("#board-svg", chromedp.ByQuery)); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		req, _ := http.NewRequest("GET", e.URL+"/api/games/"+g.Room+"/notify?player_name=Alice", nil)
		req.Header.Set("X-Seat-Token", g.Token)
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			b, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if strings.Contains(string(b), `"enabled":true`) {
				return
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Error("the profile's switch was not applied to the game")
}
