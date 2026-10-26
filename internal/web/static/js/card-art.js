// Playing cards drawn as SVG: the four classic suits (spades, hearts, diamonds,
// clubs) in the usual pip layouts, with the rank in two corners. A deck that
// uses other symbols is drawn by cards.js with text, as before.
//
// A face is a 100×140 picture in the colour of its suit (currentColor): the
// page sets the colour and the card's frame; this module draws what is inside.

import { s } from "./util.js";

// The suits in a 24×24 box, centred on 12,12: [element, ...numbers or path data].
const SUIT_SHAPES = {
	"♥": [["path", "M12 21.6C5.4 16.4 2 12.6 2 8.4 2 5.5 4.2 3.4 7 3.4c2 0 3.8 1.1 5 3 1.2-1.9 3-3 5-3 2.8 0 5 2.1 5 5 0 4.2-3.4 8-10 13.2z"]],
	"♦": [["path", "M12 1.8 21.2 12 12 22.2 2.8 12z"]],
	"♠": [["path", "M12 1.8C11 5.6 3 9.4 3 14.4c0 2.6 2.1 4.6 4.6 4.6 1.5 0 2.9-.7 3.7-1.9-.1 2.2-.8 4-2.3 5.5h6c-1.5-1.5-2.2-3.3-2.3-5.5.8 1.2 2.2 1.9 3.7 1.9 2.5 0 4.6-2 4.6-4.6C21 9.4 13 5.6 12 1.8z"]],
	"♣": [
		["circle", 12, 7.6, 4.9],
		["circle", 6.6, 14.4, 4.9],
		["circle", 17.4, 14.4, 4.9],
		["path", "M12 12.4c.1 3.6-.9 6.6-3.2 9.2h6.4c-2.3-2.6-3.3-5.6-3.2-9.2z"],
	],
};

/** Is this suit symbol one the module can draw? */
export function isClassicSuit(symbol) {
	return Object.prototype.hasOwnProperty.call(SUIT_SHAPES, symbol);
}

/** One suit as a group, its centre at (x, y), size units wide, upside down when flip. */
function pip(symbol, x, y, size, flip = false) {
	const k = size / 24;
	const g = s("g", { transform: `translate(${x} ${y}) rotate(${flip ? 180 : 0}) scale(${k}) translate(-12 -12)` });
	for (const [kind, ...a] of SUIT_SHAPES[symbol]) {
		g.append(kind === "path" ? s("path", { d: a[0] }) : s("circle", { cx: a[0], cy: a[1], r: a[2] }));
	}
	return g;
}

/** A suit symbol on its own, for a button or a label: an SVG of px pixels in currentColor. */
export function suitIcon(symbol, px = 16) {
	const svg = s("svg", { viewBox: "0 0 24 24", width: px, height: px, class: "suit-icon", "aria-hidden": "true" });
	if (!isClassicSuit(symbol)) return svg;
	svg.append(pip(symbol, 12, 12, 24));
	return svg;
}

const L = 30;
const C = 50;
const R = 70;
// Where the pips of each rank go (x, y in the 100×140 face); the lower half is drawn upside down.
const LAYOUTS = {
	2: [[C, 34], [C, 106]],
	3: [[C, 34], [C, 70], [C, 106]],
	4: [[L, 34], [R, 34], [L, 106], [R, 106]],
	5: [[L, 34], [R, 34], [C, 70], [L, 106], [R, 106]],
	6: [[L, 34], [R, 34], [L, 70], [R, 70], [L, 106], [R, 106]],
	7: [[L, 34], [R, 34], [C, 52], [L, 70], [R, 70], [L, 106], [R, 106]],
	8: [[L, 34], [R, 34], [C, 52], [L, 70], [R, 70], [C, 88], [L, 106], [R, 106]],
	9: [[L, 32], [R, 32], [L, 57], [R, 57], [C, 70], [L, 83], [R, 83], [L, 108], [R, 108]],
	10: [[L, 32], [R, 32], [C, 45], [L, 57], [R, 57], [L, 83], [R, 83], [C, 95], [L, 108], [R, 108]],
};
const PIP = 17;

/**
 * The face of a card: symbol is the suit's, label the rank as printed (a number
 * from 1 to 13 here). The picture takes the colour of its container.
 */
export function cardFaceSvg(symbol, label) {
	const n = Number(label);
	const svg = s("svg", { viewBox: "0 0 100 140", class: "pc-svg", preserveAspectRatio: "xMidYMid meet", "aria-hidden": "true", focusable: "false" });
	// The index in two opposite corners.
	const index = (flip) => {
		const g = s("g", { transform: flip ? "rotate(180 50 70)" : "" });
		g.append(s("text", { x: 13, y: 24, class: "pc-svg-rank", "text-anchor": "middle", "font-size": String(label).length > 1 ? 21 : 25 }, String(label)));
		g.append(pip(symbol, 13, 38, 14));
		return g;
	};
	svg.append(index(false), index(true));
	const layout = LAYOUTS[n];
	if (layout) {
		for (const [x, y] of layout) svg.append(pip(symbol, x, y, PIP, y > 70));
	} else if (n === 1) {
		// An ace-like 1: one large suit in a ring.
		svg.append(s("circle", { cx: 50, cy: 70, r: 33, class: "pc-svg-ring" }));
		svg.append(pip(symbol, 50, 70, 40));
	} else {
		// 11, 12, 13 and anything else: a framed court card, the frame doubled for the higher ranks.
		const frames = n >= 13 ? 2 : 1;
		for (let i = 0; i < frames; i++) {
			const d = i * 5;
			svg.append(s("rect", { x: 22 + d, y: 26 + d, width: 56 - 2 * d, height: 88 - 2 * d, rx: 6, class: "pc-svg-frame" }));
		}
		svg.append(pip(symbol, 50, 70, n >= 12 ? 36 : 32));
		const inset = 6 + (frames - 1) * 5;
		svg.append(pip(symbol, 22 + inset + 6, 26 + inset + 6, 10), pip(symbol, 78 - inset - 6, 114 - inset - 6, 10, true));
		if (n === 12) svg.append(pip(symbol, 78 - inset - 6, 26 + inset + 6, 10), pip(symbol, 22 + inset + 6, 114 - inset - 6, 10, true));
	}
	return svg;
}
