package app_test

// The page's side of chat in the reader's language (ADR 0143): a translated
// line is marked with the language it came from and shows the original at a
// click, and the profile's switch is there only where the server translates.

import (
	"testing"
	"time"

	"github.com/noderunner/bingle/internal/auth"
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
	if err := runSteps(ctx, stepNavigate(base+"/game/"+room)); err != nil {
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
	if err := runSteps(ctx, stepNavigate(base+"/#chat"), stepWaitVisible("#global-chat-input")); err != nil {
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
	if err := runSteps(ctx, stepNavigate(base+"/profile")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the switch, on", `(() => { const b = document.getElementById("profile_translate"); return b && !b.closest(".field").hidden && b.checked; })()`, 20*time.Second)
	clickSel(t, ctx, "#profile_translate")
	waitJS(t, ctx, "the switch, off", `(() => { const b = document.getElementById("profile_translate"); return b && !b.closest(".field").hidden && !b.checked && !b.disabled; })()`, 20*time.Second)

	// Chat is then as written: the history, and what comes next.
	if err := runSteps(ctx, stepNavigate(base+"/#chat"), stepWaitVisible("#global-chat-input")); err != nil {
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
	if err := runSteps(ctx, stepNavigate(base+"/profile")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the profile page", `document.getElementById("profile_language") !== null && document.getElementById("profile_translate") !== null`, 20*time.Second)
	time.Sleep(500 * time.Millisecond) // /auth/me has answered
	if !evalBool(t, ctx, `document.getElementById("profile_translate").closest(".field").hidden`) {
		t.Error("the switch is offered on a server that does not translate")
	}
}

// A direct message from a friend shows in the reader's language in the friends
// window, with the original a click away.
func TestWebChatTranslation_ADirectMessageIsShownInTheReadersLanguage(t *testing.T) {
	f := newFakeTranslator(t)
	a, base := translatingApp(t, f.URL, "")
	anaB, beaB := member(t, a, base, "ana"), member(t, a, base, "bea")
	anaB.here()
	bea := beaB.here()
	befriend(t, anaB, beaB, "bea", "ana")
	ctx, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	signInChrome(t, ctx, base, beaB.cookie(auth.SessionCookie))
	if err := runSteps(ctx, stepNavigate(base+"/"), stepWaitVisible("#open-friends")); err != nil {
		t.Fatal(err)
	}
	clickSel(t, ctx, "#open-friends")
	waitJS(t, ctx, "Ana in the list", `[...document.querySelectorAll("#friends-list .dm-person")].some(b => b.textContent.includes("@ana"))`, 20*time.Second)
	clickSel(t, ctx, `#friends-list .dm-person[data-address]`)
	waitJS(t, ctx, "the correspondence", `document.getElementById("dm-title").textContent === "With @ana"`, 10*time.Second)
	time.Sleep(500 * time.Millisecond)
	if code := anaB.do("POST", "/api/dm", map[string]any{"to": bea, "text": "hola amigos"}, nil); code != 200 {
		t.Fatalf("Ana writes: %d", code)
	}
	line := lastLine("dm-log")
	waitJS(t, ctx, "the translation", line+`.querySelector(".chat-text").textContent === "[English] hola amigos" && `+
		line+`.querySelector(".chat-translation").textContent.startsWith("Translated from Spanish")`, 10*time.Second)
	clickSel(t, ctx, "#dm-log .chat-line:last-child .chat-original")
	waitJS(t, ctx, "the original", line+`.querySelector(".chat-text").textContent === "hola amigos"`, 5*time.Second)
}
