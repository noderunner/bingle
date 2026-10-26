// The pictures of a board of choices (the board of terms of Nyet!): one for each
// box and for the head of each line, so the board needs no words. A game names
// a picture as "kind:argument" (see GridBox.Icon in internal/games/nyet/view.go):
//
//   animal:wolf:red   a player's animal in its seat colour
//   suit:red          a suit of the deck (the deck says how it looks)
//   super:red         the 1s of a suit, which are super trumps; super:none for no super trump
//   discard:none|1|2|except1|passleft
//   points:1|2|3|4|-2
//   line:first|discard|trump|super|points   the head of a line
//
// Everything is drawn in a 32×32 box with the colour of the text, and the
// red of a warning where a picture says "no".

import { s } from "./util.js";
import { animalIcon } from "./animals.js";
import { isClassicSuit, suitIcon } from "./card-art.js";

const svg = (cls, ...kids) => s("svg", { viewBox: "0 0 32 32", class: `cicon ${cls}`, "aria-hidden": "true", focusable: "false" }, ...kids);
const path = (d, cls = "") => s("path", { d, class: cls });
const rect = (x, y, w, h, r, cls = "") => s("rect", { x, y, width: w, height: h, rx: r, class: cls });
const text = (x, y, size, str, cls = "") => s("text", { x, y, "font-size": size, "text-anchor": "middle", class: `ci-text ${cls}` }, str);

/** A playing card, upright, of the given width in the 32×32 box (its height is 1.4 times that). */
const card = (cx, cy, w, cls = "") => rect(cx - w / 2, cy - (w * 0.7), w, w * 1.4, 2.4, `ci-card ${cls}`);

/** The "no" sign: a circle with a slash. */
const noSign = () => [s("circle", { cx: 16, cy: 16, r: 11, class: "ci-no" }), path("M8.2 23.8 23.8 8.2", "ci-no")];

function leftArrow(x, y, len, cls = "ci-arrow") {
	return path(`M${x + len} ${y}H${x}M${x + 4} ${y - 4}L${x} ${y}L${x + 4} ${y + 4}`, cls);
}

const DISCARD = {
	none: () => svg("ci-discard-none", ...noSign()),
	1: () => svg("ci-discard-1", card(16, 14, 14), text(16, 19.5, 12, "−1", "ci-bold"), path("M16 26.5v3M13 27.6l3 3 3-3", "ci-arrow")),
	2: () => svg("ci-discard-2", card(12, 14, 13), card(19, 15.4, 13), text(19, 20.6, 11, "−2", "ci-bold")),
	except1: () => svg("ci-discard-except1", card(16, 15, 15), text(16, 20.4, 14, "1", "ci-bold"), path("M7 25.5 25 5.5", "ci-no")),
	passleft: () => svg("ci-discard-passleft", card(20, 13, 12), card(14, 15, 12), leftArrow(5, 27, 22)),
};

const LINES = {
	first: () => svg("ci-line-first", path("M9 28V5", "ci-pole"), path("M9 6h15l-4 5 4 5H9z", "ci-flag")),
	discard: () => svg("ci-line-discard", card(14, 12, 12), card(18, 14, 12), path("M5 27h22M16 21v4M13 23.4l3 3 3-3", "ci-arrow")),
	trump: () => svg("ci-line-trump", path("M5 24V11l7 6 4-9 4 9 7-6v13z", "ci-crown"), path("M5 27.5h22", "ci-arrow")),
	super: () => svg("ci-line-super", path("M16 3l3.4 7.2 7.8 1-5.7 5.5 1.5 7.8L16 20.6l-7 3.9 1.5-7.8L4.8 11.2l7.8-1z", "ci-star"), text(16, 19.4, 11, "1", "ci-bold ci-ontop")),
	points: () => svg("ci-line-points", s("ellipse", { cx: 14, cy: 23, rx: 9, ry: 3.6, class: "ci-coin" }), s("ellipse", { cx: 14, cy: 18.4, rx: 9, ry: 3.6, class: "ci-coin" }), s("ellipse", { cx: 18, cy: 12, rx: 9, ry: 3.6, class: "ci-coin" }), text(18, 14, 8, "1", "ci-bold")),
};

/** A card with its value in the corner and the suit big in the middle, and a star when it is a super trump. */
function oneOf(suit, star) {
	const symbol = suit && suit.symbol;
	const kids = [card(16, 16, 17, "ci-face"), text(11.2, 12.6, 9, "1", "ci-bold")];
	const g = svg("ci-super", ...kids);
	if (symbol && isClassicSuit(symbol)) {
		const pip = suitIcon(symbol, 12);
		pip.setAttribute("x", "10");
		pip.setAttribute("y", "15.4");
		pip.style.color = suit.color || "";
		g.append(pip);
	}
	if (star) g.append(path("M25.6 3.8l1.4 3 3.2.4-2.4 2.2.6 3.2-2.8-1.6-2.8 1.6.6-3.2-2.4-2.2 3.2-.4z", "ci-star"));
	return g;
}

/**
 * The picture for a descriptor. ctx.info is the game's info (its suits).
 * size is the width in pixels; an unknown descriptor draws nothing.
 */
export function choiceIcon(desc, ctx = {}, size = 34) {
	const [kind, a, b] = String(desc || "").split(":");
	let node = null;
	switch (kind) {
		case "animal":
			return animalIcon(a, { color: b, size });
		case "suit": {
			const suit = ((ctx.info && ctx.info.cards && ctx.info.cards.suits) || {})[a] || {};
			if (isClassicSuit(suit.symbol)) {
				node = suitIcon(suit.symbol, size);
				node.style.color = suit.color || "";
			} else {
				node = svg("ci-suit", text(16, 22, 20, suit.symbol || a, "ci-bold"));
				node.style.color = suit.color || "";
			}
			break;
		}
		case "super": {
			if (a === "none") {
				node = svg("ci-super-none", ...noSign(), text(16, 20, 12, "1", "ci-bold"));
				break;
			}
			const suit = ((ctx.info && ctx.info.cards && ctx.info.cards.suits) || {})[a];
			node = oneOf(suit, true);
			break;
		}
		case "discard":
			node = DISCARD[a] ? DISCARD[a]() : null;
			break;
		case "points": {
			const n = Number(a);
			node = svg(`ci-points${n < 0 ? " ci-negative" : ""}`, s("circle", { cx: 16, cy: 16, r: 12.5, class: "ci-coin" }), s("circle", { cx: 16, cy: 16, r: 9.4, class: "ci-coin-ring" }), text(16, n < 0 ? 21.2 : 21.6, n < 0 ? 14 : 16, n < 0 ? "−" + Math.abs(n) : String(n), "ci-bold"));
			break;
		}
		case "line":
			node = LINES[a] ? LINES[a]() : null;
			break;
	}
	if (!node) return null;
	node.setAttribute("width", String(size));
	node.setAttribute("height", String(size));
	return node;
}
