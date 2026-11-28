package app_test

// The page's part of a link that opens the 3D client (ADR 0163). A headless
// browser cannot start a program, so the page's navigation to the link is
// caught and its address read.

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

// catchLinks keeps the page from following a bingle-3d link, and keeps the
// link's address in window.__link instead.
const catchLinks = `(() => {
	window.__link = "";
	document.addEventListener("click", (ev) => {
		const a = ev.target.closest && ev.target.closest("a");
		if (a && a.href.startsWith("bingle-3d:")) { window.__link = a.href; ev.preventDefault(); }
	}, true);
	return true;
})()`

func TestWebClientLink_OpensTheGameInTheClientAndOffersTheDownload(t *testing.T) {
	_, base, _ := clientServer(t)
	room := councilWithHistory(t, base, 1)
	ctx, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	openGameAs(t, ctx, base, room, "test-council", "Alice")

	// A seat held here, so the seated player's controls offer the same seat in the client.
	waitJS(t, ctx, "the link to the client", `document.querySelector('[data-client-link="join"]') !== null`, 15*time.Second)
	if got := evalString(t, ctx, `document.querySelector('[data-client-link="join"]').textContent`); got != "Join with Bingle3D" {
		t.Errorf("the link says %q", got)
	}
	if !evalBool(t, ctx, catchLinks) {
		t.Fatal("could not catch the page's links")
	}
	clickSel(t, ctx, `[data-client-link="join"]`)
	waitJS(t, ctx, "the link", `window.__link !== ""`, 10*time.Second)
	link, err := url.Parse(evalString(t, ctx, `window.__link`))
	if err != nil {
		t.Fatal(err)
	}
	q := link.Query()
	code := q.Get("code")
	if link.Scheme != "bingle-3d" || link.Host != "join" || link.Path != "" || link.Fragment != "" || q.Get("server") != base || q.Get("room") != room || !linkCode.MatchString(code) || len(q) != 3 {
		t.Fatalf("the link: %s", link)
	}
	// Each value is percent-encoded, in the order the contract gives.
	if want := "bingle-3d://join?server=" + url.QueryEscape(base) + "&room=" + room + "&code=" + code; link.String() != want {
		t.Errorf("the link is %s, want %s", link, want)
	}
	// The code is this browser's guest's, and good for the client.
	if status, out := trade(t, base, "", code); status != http.StatusOK || out.GuestKey == "" {
		t.Errorf("trading the link's code: %d %+v", status, out)
	}

	// Still in front a moment later: the client did not open, so the download is offered beside the link.
	waitJS(t, ctx, "the download", `document.querySelector('[data-client-link="join"] + .client-get') !== null`, 5*time.Second)
	if got := evalString(t, ctx, `(() => { const a = document.querySelector(".client-get"); return a.textContent + " " + a.getAttribute("href"); })()`); got != "Download Game Client /client" {
		t.Errorf("the download offered: %q", got)
	}

	// When the page loses the focus to the client, it offers nothing.
	evalBool(t, ctx, `(() => { document.querySelector(".client-get").remove(); window.__link = ""; return true; })()`)
	clickSel(t, ctx, `[data-client-link="join"]`)
	waitJS(t, ctx, "the second link", `window.__link !== ""`, 10*time.Second)
	evalBool(t, ctx, `window.dispatchEvent(new Event("blur")) || true`)
	time.Sleep(2500 * time.Millisecond)
	if evalBool(t, ctx, `document.querySelector(".client-get") !== null`) {
		t.Error("the download was offered after the page lost the focus")
	}
}

func TestWebClientLink_WatchingAndALobbyOfferItAndAPhoneDoesNot(t *testing.T) {
	_, base, _ := clientServer(t)
	room := councilWithHistory(t, base, 1)
	var waiting createGameResponse
	if c := doJSON(t, "POST", base+"/api/games", map[string]any{"game_def_id": "test-council", "player_name": "Bob"}, &waiting); c != 200 {
		t.Fatalf("create: %d", c)
	}
	ctx, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()

	// Somebody with no seat at a full table is offered a place to watch, in the client too.
	openGameAs(t, ctx, base, room, "test-council", "Carol")
	waitJS(t, ctx, "the join card", `document.getElementById("identity-gate") !== null`, 15*time.Second)
	if got := evalString(t, ctx, `[...document.querySelectorAll("#identity-gate [data-client-link]")].map((b) => b.dataset.clientLink + ":" + b.textContent).join("|")`); got != "watch:Watch with Bingle3D" {
		t.Errorf("the join card offers %q", got)
	}
	evalBool(t, ctx, catchLinks)
	clickSel(t, ctx, `#identity-gate [data-client-link="watch"]`)
	waitJS(t, ctx, "the link", `window.__link !== ""`, 10*time.Second)
	if got := evalString(t, ctx, `window.__link`); !strings.HasPrefix(got, "bingle-3d://watch?server="+url.QueryEscape(base)+"&room="+room+"&code=bgj_") {
		t.Errorf("the link to watch: %s", got)
	}

	// A lobby with a seat open offers it, in the client too.
	evalBool(t, ctx, `(location.href = "/game/`+waiting.RoomCode+`", true)`)
	waitJS(t, ctx, "the lobby", `document.querySelector('#lobby-join [data-client-link="join"]') !== null`, 15*time.Second)

	// On a phone there is no client to open.
	phone, cancelPhone := newChromedpContext(t, 60*time.Second)
	defer cancelPhone()
	openOnPhone(t, phone, base, "Alice", map[string]string{room: "test-council"}, room)
	waitJS(t, phone, "the seated player's controls", `document.getElementById("resign") !== null`, 15*time.Second)
	if evalBool(t, phone, `document.querySelector("[data-client-link]") !== null`) {
		t.Error("a phone is offered the client")
	}
}
