package app_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

// Every game can be drawn without its look (ADR 0124): the page draws each
// game's start from its info alone, as a client that never reads web.yaml
// would have to, with no error. Card games have no board to draw, and
// Cthulhu Wars still carries its look in its definition.
func TestWebEveryGameDrawsWithoutItsLook(t *testing.T) {
	baseURL := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 90*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.Navigate(baseURL+"/")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the page", `document.readyState === "complete"`, 15*time.Second)
	script := `(async () => {
		const { renderBoard } = await import("/static/js/render.js");
		const games = await fetch("/api/game-defs").then((r) => r.json());
		const out = [];
		let drawn = 0;
		for (const g of games) {
			try {
				const info = await fetch("/api/game-defs/" + encodeURIComponent(g.id)).then((r) => r.json());
				if (!info.board || info.board.style === "none" || info.board.style === "areas") continue;
				const host = document.createElement("div");
				document.body.append(host);
				renderBoard(host, info, { cells: info.start.cells, flipped: false, interactive: false, showCoords: false, preview: true }, {});
				const cells = host.querySelectorAll(".cell, [data-position]").length;
				host.remove();
				if (!cells) out.push(g.id + ": nothing drawn");
				else drawn++;
			} catch (err) {
				out.push(g.id + ": " + err.message);
			}
		}
		return out.join("\n") || "ok " + drawn;
	})()`
	var got string
	if err := chromedp.Run(ctx, chromedp.Evaluate(script, &got, func(p *runtime.EvaluateParams) *runtime.EvaluateParams { return p.WithAwaitPromise(true) })); err != nil {
		t.Fatal(err)
	}
	var drawn int
	if _, err := fmt.Sscanf(got, "ok %d", &drawn); err != nil {
		t.Fatalf("games drawn without their look:\n%s", got)
	}
	// Every board game in the list: the classics, Tak, Fendo, Fist, Blooms,
	// Meridians, the fixtures and the prototypes.
	if drawn < 20 {
		t.Errorf("only %d games were drawn", drawn)
	}
}
