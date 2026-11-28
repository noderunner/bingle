package app_test

import (
	"strings"
	"testing"
	"time"
)

// The key (ADR 0154, decision 5): wherever the game's page names a seat, a
// swatch in the viewer's own paint stands beside the name. With an illustrated
// board theme chosen, the swatch has the paint of that theme's stones of the
// seat's colors, which are the paint of its pieces on the board, without a
// reload: in a game of one color a seat (Ataxx), in one whose seats play two
// (Blooms), where the swatch shows both and names both, and in one whose
// history names a seat at each move (Meridians: the list of moves).
func TestWebSeatKey_ASeatsSwatchHasThePaintOfItsPieces(t *testing.T) {
	baseURL := newRESTTestServer(t)
	for _, def := range []string{"ataxx", "blooms", "meridians"} {
		t.Run(def, func(t *testing.T) {
			// A move each, so that both seats have pieces and the history has lines, and a word in the chat.
			room := playedAgainstABot(t, baseURL, def)
			if code := doJSON(t, "POST", baseURL+"/api/games/"+room+"/chat", map[string]any{"player_name": "Alice", "text": "hello"}, nil); code != 200 {
				t.Fatalf("chat: %d", code)
			}
			ctx, cancel := newChromedpContext(t, 90*time.Second)
			defer cancel()
			if err := runSteps(ctx, stepViewport(1400, 1000)); err != nil {
				t.Fatal(err)
			}
			openGameAs(t, ctx, baseURL, room, def, "Alice")
			waitJS(t, ctx, "both seats' pieces", `new Set([...document.querySelectorAll('#board .piece')].map(p => p.dataset.color)).size >= 2`, 20*time.Second)
			waitJS(t, ctx, "the chat line", `document.querySelector('.chat-line[data-seat] .chat-key') !== null`, 15*time.Second)
			clickSel(t, ctx, "#open-options")
			waitJS(t, ctx, "the theme list", `document.getElementById("opt-board") !== null`, 10*time.Second)
			evalString(t, ctx, `(function(){ var s = document.getElementById("opt-board"); s.value = "synthwave"; s.dispatchEvent(new Event("change", {bubbles: true})); return "ok"; })()`)
			waitJS(t, ctx, "synthwave applied", `document.documentElement.dataset.board === "synthwave"`, 10*time.Second)
			script := `(async () => {
				const { THEMES } = await import("/static/js/themes.js");
				const { api } = await import("/static/js/api.js");
				const th = THEMES.find((x) => x.id === "synthwave");
				const info = await api.gameDef("` + def + `");
				const rgb = (paint) => {
					const probe = document.createElement("span");
					probe.style.backgroundColor = paint;
					document.body.append(probe);
					const got = getComputedStyle(probe).backgroundColor;
					probe.remove();
					return got;
				};
				const out = [];
				const seen = new Set();
				const check = (where, sw, seat) => {
					if (!sw) return out.push(where + ": no swatch");
					seen.add(where.split(" ")[0] === "seat" ? "seat" : where);
					const colors = seat.colors && seat.colors.length ? seat.colors : [seat.color];
					const style = getComputedStyle(sw);
					for (const c of colors) {
						// The seat's pieces are drawn in the theme's stone of the color.
						const piece = document.querySelector('#board .piece[data-color="' + c + '"] .piece-body');
						if (piece && !getComputedStyle(piece).fill.includes("th-synthwave-" + c)) out.push(where + ": a " + c + " piece is drawn " + getComputedStyle(piece).fill);
						const want = rgb(th.stones[c].mid);
						const got = colors.length > 1 ? style.backgroundImage : style.backgroundColor;
						if (!got.includes(want)) out.push(where + ": " + c + " should be " + want + ", the swatch is " + got);
					}
					const names = colors.join(", ");
					if (sw.getAttribute("aria-label") !== names) out.push(where + ": a screen reader hears " + sw.getAttribute("aria-label") + ", want " + names);
					if (!(sw.title || "").endsWith(names)) out.push(where + ": the tooltip is " + sw.title + ", want " + names);
				};
				for (const seat of info.seats) {
					check("seat " + seat.seat, document.querySelector("#seat-" + seat.seat + " .seat-head .swatch"), seat);
					const line = document.querySelector('.log-actor[data-seat="' + seat.seat + '"] .swatch');
					if (line) check("the log", line, seat);
				}
				const title = document.getElementById("impulse-title");
				const now = info.seats.find((s) => title && title.textContent.trim() && document.querySelector("#seat-" + s.seat + ".on-impulse"));
				if (now) check("the impulse card", title.querySelector(".swatch"), now);
				const chat = document.querySelector(".chat-line[data-seat] .chat-key");
				check("the chat", chat, info.seats.find((s) => s.seat === Number(chat.closest(".chat-line").dataset.seat)));
				for (const dot of document.querySelectorAll(".move-list .seat-dot")) {
					const seat = info.seats.find((s) => dot.closest("li").classList.contains("by-seat-" + s.seat));
					if (seat) check("the move list", dot, seat);
				}
				return (out.join("\n") || "ok") + "|" + [...seen].sort().join(",");
			})()`
			// The History tab draws the log or the list of moves, one at a time.
			checked := map[string]bool{}
			for _, view := range []string{"history-friendly", "history-notation"} {
				evalString(t, ctx, `(function () { var tab = document.getElementById("`+view+`"); if (tab) tab.click(); return "ok"; })()`)
				time.Sleep(300 * time.Millisecond)
				got, seen, _ := strings.Cut(evalStringAwait(t, ctx, script), "|")
				if got != "ok" {
					t.Errorf("%s, the keys:\n%s", view, strings.TrimSpace(got))
				}
				for _, w := range strings.Split(seen, ",") {
					checked[w] = true
				}
			}
			want := []string{"seat", "the chat", "the impulse card"}
			if def == "meridians" {
				want = append(want, "the move list")
			}
			for _, w := range want {
				if !checked[w] {
					t.Errorf("no key checked in %s: %v", w, checked)
				}
			}
		})
	}
}
