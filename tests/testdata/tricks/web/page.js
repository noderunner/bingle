// Tricks' page script (tests/testdata/tricks): the pictures of its board of
// choices and of its players, so the tests see a game's own drawing asked for
// them (ADR 0124). Every picture is a 32×32 SVG in the colour of the text.

import { s } from "/static/js/util.js";
import { currentCardSet, setSuitIcon } from "/static/js/card-sets.js";

const svg = (cls, title, ...kids) => {
	const el = s("svg", { viewBox: "0 0 32 32", class: `tricks-icon ${cls}`, role: "img", "aria-label": title }, ...kids);
	return el;
};
const text = (str, size = 16) => s("text", { x: 16, y: 16, "font-size": size, "text-anchor": "middle", "dominant-baseline": "central" }, str);

/** A player's picture: the initial of the seat's label on a disc in the seat's colour. */
export function seatIcon(label, { color = "", size = 22, title = "" } = {}) {
	if (!label) return null;
	const el = s("svg", { viewBox: "0 0 32 32", width: size, height: size, class: `tricks-seat color-${color}`, role: "img", "aria-label": title || label });
	el.append(s("circle", { cx: 16, cy: 16, r: 14 }), text(label[0].toUpperCase()));
	return el;
}

/** A picture of the board of choices: "suit:red", "points:2", "line:trump" or "seat:north". */
export function choiceIcon(desc, ctx = {}, size = 34) {
	const [kind, arg] = String(desc).split(":");
	let el = null;
	switch (kind) {
		case "suit": {
			// At a table that wears a set of cards, the suit is the set's.
			const set = currentCardSet(ctx.info);
			if (set) return setSuitIcon(set, ctx.info, arg, size);
			const suit = ((ctx.info && ctx.info.cards && ctx.info.cards.suits) || {})[arg] || {};
			el = svg(`ti-suit ti-suit-${arg}`, suit.label || arg, text(suit.symbol || arg, 22));
			break;
		}
		case "points":
			el = svg("ti-points", `${arg} points`, s("circle", { cx: 16, cy: 16, r: 12 }), text(arg));
			break;
		case "line":
			el = svg(`ti-line ti-line-${arg}`, arg, text(arg === "trump" ? "♛" : "★", 20));
			break;
		case "seat": {
			const seat = (ctx.seats || []).find((x) => (x.label || "").toLowerCase() === arg);
			return seatIcon(seat ? seat.label : arg, { color: seat ? seat.color : "", size });
		}
		default:
			return null;
	}
	el.setAttribute("width", size);
	el.setAttribute("height", size);
	return el;
}
