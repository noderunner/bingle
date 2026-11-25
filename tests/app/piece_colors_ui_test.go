package app_test

import (
	"testing"
	"time"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

// Every colour of the palette draws a piece in a colour of its own, in the
// page's own look and in every board theme. A colour the stylesheet does not
// know is drawn black, as grey once was (#313).
func TestWebEveryPaletteColourDrawsItsPieces(t *testing.T) {
	baseURL := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.Navigate(baseURL+"/")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the page", `document.readyState === "complete"`, 15*time.Second)
	script := `(async () => {
		const { pieceIcon } = await import("/static/js/render.js");
		const { THEMES, installThemes } = await import("/static/js/themes.js");
		installThemes();
		const colors = ["white", "black", "red", "blue", "green", "yellow", "orange", "purple", "teal", "sand", "brown", "grey"];
		const fill = (c) => {
			const icon = pieceIcon("disc", c, 28);
			document.body.append(icon);
			const got = getComputedStyle(icon.querySelector(".piece-body")).fill;
			icon.remove();
			return got;
		};
		const out = [];
		const plain = {};
		for (const c of colors) {
			plain[c] = fill(c);
			if (plain[c] === "rgb(0, 0, 0)" && c !== "black") out.push(c + " is drawn black");
		}
		if (plain.grey !== "rgb(163, 161, 155)") out.push("grey is drawn " + plain.grey);
		if (new Set(Object.values(plain)).size !== colors.length) out.push("two colours share a fill: " + JSON.stringify(plain));
		for (const th of THEMES) {
			document.documentElement.dataset.board = th.id;
			for (const c of colors) {
				const got = fill(c);
				if (!got.includes("th-" + th.id + "-" + c)) out.push(th.id + ": " + c + " is drawn " + got);
			}
		}
		delete document.documentElement.dataset.board;
		return out.join("\n") || "ok";
	})()`
	var got string
	if err := chromedp.Run(ctx, chromedp.Evaluate(script, &got, func(p *runtime.EvaluateParams) *runtime.EvaluateParams { return p.WithAwaitPromise(true) })); err != nil {
		t.Fatal(err)
	}
	if got != "ok" {
		t.Errorf("pieces drawn wrong:\n%s", got)
	}
}
