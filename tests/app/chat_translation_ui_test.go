package app_test

// The page's side of chat in the reader's language (ADR 0143): a translated
// line is marked with the language it came from and shows the original at a
// click, and the profile's switch is there only where the server translates.

import (
	"testing"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/noderunner/bingle/internal/config"
)

// lastLine is the JavaScript for the last line of the chat log with id log.
func lastLine(log string) string {
	return `[...document.querySelectorAll("#` + log + ` .chat-line")].pop()`
}

func TestWebChatTranslation_AGameLineIsShownInTheReadersLanguage(t *testing.T) {
	f := newFakeTranslator(t)
	_, base := translatingApp(t, f.URL, "")
	ana, room := hostRoom(t, base, "Ana")
	ctx, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.Navigate(base+"/game/"+room)); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the chat", `document.getElementById("chat-log") !== null`, 20*time.Second)
	time.Sleep(500 * time.Millisecond) // let the socket connect
	if code := ana.do("POST", "/api/games/"+room+"/chat", map[string]any{"player_name": "Ana", "text": "hola amigos"}, nil); code != 200 {
		t.Fatalf("chat: %d", code)
	}
	line := lastLine("chat-log")
	waitJS(t, ctx, "the translation in place of the line", line+`.querySelector(".chat-text").textContent === "[English] hola amigos" && `+
		line+`.querySelector(".chat-translation").textContent.startsWith("Translated from Spanish")`, 10*time.Second)
	clickSel(t, ctx, "#chat-log .chat-line:last-child .chat-original")
	waitJS(t, ctx, "the original", line+`.querySelector(".chat-text").textContent === "hola amigos" && `+
		line+`.querySelector(".chat-text").lang === "es" && `+line+`.querySelector(".chat-original").textContent === "Show translation"`, 5*time.Second)
	clickSel(t, ctx, "#chat-log .chat-line:last-child .chat-original")
	waitJS(t, ctx, "the translation again", line+`.querySelector(".chat-text").textContent === "[English] hola amigos"`, 5*time.Second)
}

func TestWebChatTranslation_TheGlobalChatAndTheSwitch(t *testing.T) {
	f := newFakeTranslator(t)
	a, base := translatingApp(t, f.URL, "")
	bo := signedIn(t, a, base, "bob")
	ctx, cancel := newChromedpContext(t, 120*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.Navigate(base+"/#chat"), chromedp.WaitVisible("#global-chat-input", chromedp.ByID)); err != nil {
		t.Fatal(err)
	}
	time.Sleep(500 * time.Millisecond) // let the live feed connect
	if code := bo.do("POST", "/api/chat", map[string]any{"player_name": "x", "text": "hola"}, nil); code != 200 {
		t.Fatalf("chat: %d", code)
	}
	line := lastLine("global-chat-log")
	waitJS(t, ctx, "the translation", line+`.querySelector(".chat-text").textContent === "[English] hola" && `+line+`.querySelector(".chat-original") !== null`, 10*time.Second)

	// The switch is on the profile page, on; turning it off loads the page
	// again with it off.
	if err := chromedp.Run(ctx, chromedp.Navigate(base+"/profile")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the switch, on", `(() => { const b = document.getElementById("profile_translate"); return b && !b.closest(".field").hidden && b.checked; })()`, 20*time.Second)
	clickSel(t, ctx, "#profile_translate")
	waitJS(t, ctx, "the switch, off", `(() => { const b = document.getElementById("profile_translate"); return b && !b.closest(".field").hidden && !b.checked && !b.disabled; })()`, 20*time.Second)

	// Chat is then as written: the history, and what comes next.
	if err := chromedp.Run(ctx, chromedp.Navigate(base+"/#chat"), chromedp.WaitVisible("#global-chat-input", chromedp.ByID)); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the history as written", line+` && `+line+`.querySelector(".chat-text").textContent === "hola" && !`+line+`.querySelector(".chat-translation")`, 10*time.Second)
	time.Sleep(500 * time.Millisecond)
	if code := bo.do("POST", "/api/chat", map[string]any{"player_name": "x", "text": "hola otra vez"}, nil); code != 200 {
		t.Fatalf("chat: %d", code)
	}
	waitJS(t, ctx, "the new line", line+`.querySelector(".chat-text").textContent === "hola otra vez"`, 10*time.Second)
	time.Sleep(time.Second)
	if evalBool(t, ctx, line+`.querySelector(".chat-translation") !== null || `+line+`.querySelector(".chat-text").textContent !== "hola otra vez"`) {
		t.Error("a line was translated with the switch off")
	}
}

func TestWebChatTranslation_NoSwitchWithoutAService(t *testing.T) {
	base := newServerWith(t, func(*config.Config) {})
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.Navigate(base+"/profile")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the profile page", `document.getElementById("profile_language") !== null && document.getElementById("profile_translate") !== null`, 20*time.Second)
	time.Sleep(500 * time.Millisecond) // /auth/me has answered
	if !evalBool(t, ctx, `document.getElementById("profile_translate").closest(".field").hidden`) {
		t.Error("the switch is offered on a server that does not translate")
	}
}
