// The rank of a card, lettered with a pen: digits and capital letters as strokes
// on a grid 6 wide and 10 high, which a pen turns into a typeface. A font in the
// stylesheet would look the same on no two machines (most have none of the
// interesting ones) and a font file would have to ship with the binary; these
// strokes look the same everywhere, cost a few kilobytes and scale with the card.
//
// A pen says how the strokes are drawn:
//   smooth   curves through the points (otherwise straight lines between them)
//   width    the thickness of the stroke, in grid units
//   skew     slant in degrees
//   serif    a tick across every end of a stroke that is free
//   pads     a round pad on every free end
//   nib      a broad nib: the stroke is drawn again this far along the diagonal
//   wobble   every point is moved by up to this much, the same way every time
//   inline   a line of the paper's colour down the middle of the stroke, this wide

import { s } from "./util.js";

// A glyph is its strokes, separated by ";"; a stroke is its points as "x,y".
// A corner is where two strokes meet: a smooth pen rounds a stroke, never a corner.
const O = "1.2,0 3,0 4.8,0 6,2 6,8 4.8,10 3,10 1.2,10 0,8 0,2 1.2,0";
const GLYPHS = {
	0: O,
	1: "1,2.2 3,0;3,0 3,10;1,10 5,10",
	2: "0,2.6 1,.6 3,0 5,.6 6,2.6 5.4,4.6 0,10;0,10 6,10",
	3: "0,1 3,0 5.4,1 6,2.6 5,4.2 2.4,5;2.4,5 5,5.8 6,7.6 5.4,9 3,10 0,9",
	4: "4.5,10 4.5,0;4.5,0 0,7;0,7 6,7",
	5: "5.6,0 1,0;1,0 .4,4.6;.4,4.6 3,3.8 5.4,5 6,7.2 5,9.2 3,10 0,9",
	6: "5.2,.6 3,0 1.2,1.4 0,5 0,7.6 1,9.4 3,10 5,9.4 6,7.6 5,5.6 3,5 1,5.6 0,7.6",
	7: "0,0 6,0;6,0 2,10",
	8: "3,5 1,4.1 .5,2.3 1.2,.6 3,0 4.8,.6 5.5,2.3 5,4.1 3,5 1,5.9 0,7.7 1,9.4 3,10 5,9.4 6,7.7 5,5.9 3,5",
	9: ".8,9.4 3,10 4.8,8.6 6,5 6,2.4 5,.6 3,0 1,.6 0,2.4 1,4.4 3,5 5,4.4 6,2.4",
	A: "0,10 3,0;3,0 6,10;1.1,6.4 4.9,6.4",
	B: "0,0 0,10;0,0 3.6,0 5.2,1 5.5,2.6 4.6,4.2 3,5 0,5;3,5 5,5.7 6,7.5 5,9.4 3.4,10 0,10",
	C: "6,1.6 4,0 2,.4 .4,2.4 0,5 .4,7.6 2,9.6 4,10 6,8.4",
	D: "0,0 0,10;0,0 2.5,0 5,1.4 6,5 5,8.6 2.5,10 0,10",
	E: "0,0 0,10;0,0 5.6,0;0,4.8 4.4,4.8;0,10 5.8,10",
	F: "0,0 0,10;0,0 5.8,0;0,4.8 4.4,4.8",
	G: "6,1.6 4,0 2,.4 .4,2.4 0,5 .4,7.6 2,9.6 4,10 6,8.8;6,8.8 6,5.6;6,5.6 3.4,5.6",
	H: "0,0 0,10;6,0 6,10;0,4.8 6,4.8",
	I: "3,0 3,10;1,0 5,0;1,10 5,10",
	J: "5,0 5,7.4 4,9.4 2.6,10 1,9.4 0,7.4;2.6,0 6.4,0",
	K: "0,0 0,10;5.8,0 0,6;1.9,4 6,10",
	L: "0,0 0,10;0,10 5.8,10",
	M: "0,10 0,0;0,0 3,6.4;3,6.4 6,0;6,0 6,10",
	N: "0,10 0,0;0,0 6,10;6,10 6,0",
	O,
	P: "0,0 0,10;0,0 3.6,0 5.4,1 6,2.8 5.4,4.6 3.6,5.6 0,5.6",
	Q: `${O};3.6,7 6,10.4`,
	R: "0,0 0,10;0,0 3.6,0 5.4,1 6,2.8 5.4,4.6 3.6,5.6 0,5.6;2.8,5.6 6,10",
	S: "5.8,1.4 4,0 2,0 .4,1.4 .6,3.4 3,5 5.4,6.6 5.8,8.6 4,10 2,10 .2,8.6",
	T: "0,0 6,0;3,0 3,10",
	U: "0,0 0,7.5 1,9.4 3,10 5,9.4 6,7.5 6,0",
	V: "0,0 3,10;3,10 6,0",
	W: "0,0 1.5,10;1.5,10 3,3.6;3,3.6 4.5,10;4.5,10 6,0",
	X: "0,0 6,10;6,0 0,10",
	Y: "0,0 3,5;6,0 3,5;3,5 3,10",
	Z: "0,0 6,0;6,0 0,10;0,10 6,10",
	" ": "",
};

const WIDTH = 6;
const GAP = 1.9;

const f2 = (v) => (Math.round(v * 100) / 100).toString();

function strokesOf(ch) {
	const g = GLYPHS[ch];
	return g.split(";").filter(Boolean).map((st) => st.split(" ").map((p) => p.split(",").map(Number)));
}

/** May this label be lettered? Anything else is set in the page's own type. */
export function canLetter(label) {
	return [...String(label).toUpperCase()].every((ch) => GLYPHS[ch] !== undefined);
}

/** A stroke as path data, straight or as a curve through its points. */
function strokePath(pts, smooth) {
	if (!smooth || pts.length < 3) return "M" + pts.map((p) => `${f2(p[0])} ${f2(p[1])}`).join("L");
	const closed = pts[0][0] === pts[pts.length - 1][0] && pts[0][1] === pts[pts.length - 1][1];
	const n = pts.length;
	const at = (i) => {
		if (closed) return pts[((i % (n - 1)) + (n - 1)) % (n - 1)];
		return pts[Math.max(0, Math.min(n - 1, i))];
	};
	let d = `M${f2(pts[0][0])} ${f2(pts[0][1])}`;
	for (let i = 0; i < n - 1; i++) {
		// A Catmull-Rom spline through the points, written as Béziers.
		const [p0, p1, p2, p3] = [at(i - 1), at(i), at(i + 1), at(i + 2)];
		d += `C${f2(p1[0] + (p2[0] - p0[0]) / 6)} ${f2(p1[1] + (p2[1] - p0[1]) / 6)} ${f2(p2[0] - (p3[0] - p1[0]) / 6)} ${f2(p2[1] - (p3[1] - p1[1]) / 6)} ${f2(p2[0])} ${f2(p2[1])}`;
	}
	return d;
}

/** The same small displacement for the same point of the same glyph, every time. */
function wobbled(pts, amount, seed) {
	return pts.map(([x, y], i) => {
		const a = Math.sin(seed * 12.9898 + i * 78.233) * 43758.5453;
		const b = Math.sin(seed * 39.346 + i * 11.135) * 24634.6345;
		return [x + (a - Math.floor(a) - 0.5) * 2 * amount, y + (b - Math.floor(b) - 0.5) * 2 * amount];
	});
}

/** One glyph, its top left corner at the origin of its group. */
function drawGlyph(ch, pen) {
	const strokes = strokesOf(ch);
	const seen = new Map();
	for (const st of strokes) for (const p of st) seen.set(p.join(), (seen.get(p.join()) || 0) + 1);
	const out = [];
	const line = (d, width, cls = "ts-l") => out.push(s("path", { d, class: cls, "stroke-width": f2(width) }));
	strokes.forEach((raw, i) => {
		const pts = pen.wobble ? wobbled(raw, pen.wobble, ch.charCodeAt(0) + i * 7) : raw;
		const d = strokePath(pts, pen.smooth);
		line(d, pen.width);
		if (pen.nib) {
			for (const k of [-1, 1]) {
				const g = s("path", { d, class: "ts-l", "stroke-width": f2(pen.width * 0.7), transform: `translate(${f2(k * pen.nib)} ${f2(-k * pen.nib)})` });
				out.push(g);
			}
		}
		if (pen.inline) line(d, pen.inline, "ts-x");
		// The free ends of the stroke: a stroke that closes on itself has none.
		for (const [end, next] of [[0, 1], [raw.length - 1, raw.length - 2]]) {
			const key = raw[end].join();
			const closed = raw[0].join() === raw[raw.length - 1].join();
			if (closed || seen.get(key) > 1) continue;
			const [x, y] = pts[end];
			if (pen.pads) out.push(s("circle", { cx: f2(x), cy: f2(y), r: f2(pen.pads), class: "ts-f" }));
			if (pen.serif) {
				const dx = pts[next][0] - x;
				const dy = pts[next][1] - y;
				const len = Math.hypot(dx, dy) || 1;
				const nx = (-dy / len) * pen.serif;
				const ny = (dx / len) * pen.serif;
				line(`M${f2(x - nx)} ${f2(y - ny)}L${f2(x + nx)} ${f2(y + ny)}`, pen.width * 0.72);
			}
		}
	});
	return out;
}

/**
 * A rank in the pen's lettering, centred on x with its top at y, `cap` high.
 * Letters are drawn as capitals, as they are printed on cards.
 */
export function lettering(label, x, y, cap, pen) {
	const text = String(label).toUpperCase();
	const k = cap / 10;
	const chars = [...text];
	const wide = chars.length * WIDTH + (chars.length - 1) * GAP;
	// A slant pivots on the middle of the letters, so that they stay centred.
	const lean = pen.skew ? -(cap / 2) * Math.tan((pen.skew * Math.PI) / 180) : 0;
	const g = s("g", { class: "ts-rank", transform: `translate(${f2(x - (wide * k) / 2 + lean)} ${f2(y)}) scale(${f2(k)})${pen.skew ? ` skewX(${pen.skew})` : ""}` });
	chars.forEach((ch, i) => {
		g.append(s("g", { transform: `translate(${f2(i * (WIDTH + GAP))} 0)` }, drawGlyph(ch, pen)));
	});
	return g;
}
