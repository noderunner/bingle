//go:build ignore

// Command cardsheet draws every card of a card game as one picture, a row for
// each suit with the cards splayed in the order of their values, for the rules
// guide. It uses the page's own card drawing (internal/web/static/js/card-art.js)
// in headless Chrome, so the guide shows the cards as the game does:
//
//	BINGLE_ADDR=127.0.0.1:18099 BINGLE_DB_PATH=/tmp/sheet.db ./bin/bingle &
//	go run scripts/cardsheet/main.go http://127.0.0.1:18099 nyet internal/games/nyet/guide/cards.svg
//
// Re-run it whenever the card drawing or the suits of the game change.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

// sheet is the script run in the page: it reads the game's cards from the
// server, draws the sheet and returns it as the text of an SVG document.
const sheet = `(async (gameID) => {
	const { cardFaceSvg, isClassicSuit } = await import("/static/js/card-art.js");
	const info = await (await fetch("/api/game-defs/" + gameID)).json();
	const cards = info.cards;
	if (!cards) throw new Error(gameID + " has no cards");
	const NS = "http://www.w3.org/2000/svg";
	const el = (name, attrs = {}, ...kids) => {
		const e = document.createElementNS(NS, name);
		for (const [k, v] of Object.entries(attrs)) e.setAttribute(k, v);
		for (const k of kids) e.append(k);
		return e;
	};
	const W = 100, H = 140, STEP = 46, GAP = 20, PAD = 14;
	// A suit's cards by value: 1 first, as many times as the deck holds it (the three 1s).
	const rows = [];
	for (const suitID of cards.suit_order) {
		const suit = cards.suits[suitID];
		const types = cards.types.filter((t) => t.suit === suitID).sort((a, b) => Number(a.label) - Number(b.label));
		const row = [];
		for (const t of types) {
			const copies = Number(t.label) === 1 ? 3 : 1;
			for (let i = 0; i < copies; i++) row.push(t);
		}
		rows.push({ suit, row });
	}
	const longest = Math.max(...rows.map((r) => r.row.length));
	const width = PAD * 2 + STEP * (longest - 1) + W;
	const height = PAD * 2 + rows.length * H + (rows.length - 1) * GAP;
	const svg = el("svg", { xmlns: NS, viewBox: "0 0 " + width + " " + height, width, height, role: "img" });
	svg.append(el("title", {}, document.createTextNode("The cards of " + info.name + ", a row for each suit, in the order of their values")));
	const style = el("style");
	style.textContent =
		".pc-svg path,.pc-svg circle{fill:currentColor}" +
		".pc-svg-rank{fill:currentColor;font-family:Georgia,'Times New Roman',serif;font-weight:700}" +
		".pc-svg .pc-svg-ring,.pc-svg .pc-svg-frame{fill:none;stroke:currentColor;stroke-width:1.6;opacity:.4}";
	svg.append(style);
	rows.forEach(({ suit, row }, r) => {
		if (!isClassicSuit(suit.symbol)) throw new Error("the suit " + suit.label + " is not drawn");
		row.forEach((t, i) => {
			const g = el("g", { transform: "translate(" + (PAD + i * STEP) + " " + (PAD + r * (H + GAP)) + ")", style: "color:" + suit.color });
			g.append(el("rect", { width: W, height: H, rx: 9, fill: "#fffdf6", stroke: suit.color, "stroke-opacity": ".55", "stroke-width": "1.5" }));
			const face = cardFaceSvg(suit.symbol, t.label);
			face.setAttribute("width", W);
			face.setAttribute("height", H);
			g.append(face);
			svg.append(g);
		});
	});
	return '<?xml version="1.0" encoding="UTF-8"?>\n' + new XMLSerializer().serializeToString(svg) + "\n";
})`

func main() {
	if len(os.Args) != 4 {
		log.Fatal("usage: cardsheet <base-url> <game-id> <out.svg>")
	}
	base, game, out := os.Args[1], os.Args[2], os.Args[3]
	profile, err := os.MkdirTemp("", "chromedp-runner-sheet-")
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(profile) }()
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), append(chromedp.DefaultExecAllocatorOptions[:], chromedp.UserDataDir(profile))...)
	defer cancelAlloc()
	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()
	ctx, cancel = context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	var svg string
	if err := chromedp.Run(ctx,
		chromedp.Navigate(base+"/"),
		chromedp.WaitReady("body", chromedp.ByQuery),
		chromedp.Evaluate(fmt.Sprintf("%s(%q)", sheet, game), &svg, func(p *runtime.EvaluateParams) *runtime.EvaluateParams { return p.WithAwaitPromise(true) }),
	); err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(out, []byte(svg), 0o644); err != nil {
		log.Fatal(err)
	}
	fmt.Println("wrote", out, len(svg), "bytes")
}
