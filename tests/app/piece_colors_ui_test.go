package app_test

import (
	"testing"
	"time"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

// Every color word the page knows (paint.js's COLOR_WORDS) draws a piece in a
// paint of its own, in the page's own look and in every board theme, and none
// is drawn black but black, as grey once was (#313).
func TestWebEveryPaletteColorDrawsItsPieces(t *testing.T) {
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
		const { COLOR_WORDS: colors } = await import("/static/js/paint.js");
		if (colors.length !== 12) return "paint.js knows " + colors.length + " color words";
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
		if (new Set(Object.values(plain)).size !== colors.length) out.push("two colors share a fill: " + JSON.stringify(plain));
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

// A color the page has no paint for is painted by seat (ADR 0154): a board of
// a game whose seats play two colors the page does not know, besides red, and
// a third unknown among its extra colors, draws all four in paints of their
// own, none black, in the page's own look and in every board theme. Which
// paint a color gets does not depend on what else is on the board, and a
// screen reader still hears the color's own name.
func TestWebUnknownColorsArePaintedBySeat(t *testing.T) {
	baseURL := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.Navigate(baseURL+"/")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the page", `document.readyState === "complete"`, 15*time.Second)
	script := `(async () => {
		const { renderBoard } = await import("/static/js/render.js");
		const { THEMES } = await import("/static/js/themes.js");
		const info = {
			id: "made-up-colors",
			seats: [{ seat: 1, color: "cultist", label: "Cult" }, { seat: 2, color: "deep-one", label: "Deep" }, { seat: 3, color: "red", label: "Red" }],
			extra_colors: ["moss"],
			pieces: [{ type: "stone", label: "Stone", shape: "disc", pool: "stone" }],
			board: { kind: "grid", style: "squares", width: 4, height: 1, positions: [{ id: "a1", x: 0, y: 0 }, { id: "b1", x: 1, y: 0 }, { id: "c1", x: 2, y: 0 }, { id: "d1", x: 3, y: 0 }] },
		};
		const colors = { a1: "cultist", b1: "deep-one", c1: "red", d1: "moss" };
		const host = document.createElement("div");
		document.body.append(host);
		const draw = (cells) => {
			renderBoard(host, info, { cells, flipped: false, interactive: false, showCoords: false }, {});
			const out = {};
			for (const p of host.querySelectorAll(".piece")) out[p.dataset.color] = getComputedStyle(p.querySelector(".piece-body")).fill;
			return out;
		};
		const all = {};
		for (const [pos, color] of Object.entries(colors)) all[pos] = [{ piece: "stone", color }];
		const out = [];
		const check = (where) => {
			const fills = draw(all);
			for (const c of Object.values(colors)) {
				if (!fills[c]) out.push(where + ": " + c + " is not drawn");
				else if (fills[c] === "rgb(0, 0, 0)") out.push(where + ": " + c + " is drawn black");
			}
			if (new Set(Object.values(fills)).size !== 4) out.push(where + ": two colors share a paint: " + JSON.stringify(fills));
			// Alone on the board, the second seat's color keeps its paint.
			const alone = draw({ b1: [{ piece: "stone", color: "deep-one" }] });
			if (alone["deep-one"] !== fills["deep-one"]) out.push(where + ": deep-one is " + fills["deep-one"] + " beside the others and " + alone["deep-one"] + " alone");
		};
		check("plain");
		for (const th of THEMES) {
			document.documentElement.dataset.board = th.id;
			check(th.id);
		}
		delete document.documentElement.dataset.board;
		draw(all);
		const label = host.querySelector('.cell[data-position="a1"]').getAttribute("aria-label");
		if (!label.includes("cultist")) out.push("a screen reader hears " + label);
		host.remove();
		return out.join("\n") || "ok";
	})()`
	var got string
	if err := chromedp.Run(ctx, chromedp.Evaluate(script, &got, func(p *runtime.EvaluateParams) *runtime.EvaluateParams { return p.WithAwaitPromise(true) })); err != nil {
		t.Fatal(err)
	}
	if got != "ok" {
		t.Errorf("unknown colors drawn wrong:\n%s", got)
	}
}
