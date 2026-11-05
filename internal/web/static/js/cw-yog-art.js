// Yog-Sothoth, the Key and the Gate, coming from out of space and time: the dials of every hour turn in the dark,
// a thread of light opens between them, and through it press the globes, iridescent, one upon another, each with
// a slow eye in it. Time runs backwards round him. Original art, drawn in code (see js/cw-art-kit.js).

import { H, W, add, awakeningBurst, between, dot, lerp, num, outCubic, randoms, round, sceneSvg, smooth } from "./cw-art-kit.js";
import { s } from "./util.js";

export const YOG_SECONDS = 7.4;
export const YOG_PEAK = 5.6;

const CX = 500;
const CY = 236;

// Round the wheel of hues: each globe is its own colour, and all of them shimmer.
const HUES = [
	["#f2d7ff", "#8a3ad0", "#12021f"],
	["#d5fbff", "#1f93b0", "#02161f"],
	["#f7f9c2", "#8c9a20", "#0d1102"],
	["#ffd9e4", "#c22d68", "#1c0210"],
	["#d6ffe6", "#1e9a70", "#021812"],
	["#ffe6c4", "#c9651c", "#1c0902"],
	["#dcdcff", "#3f40c8", "#05031f"],
];

export function yogScene() {
	const rand = randoms(1927); // the year of "The Case of Charles Dexter Ward"
	const svg = sceneSvg();
	svg.append(
		s(
			"defs",
			{},
			round("yo-void", [
				[0, "#1a0b3a"],
				[0.6, "#08031c"],
				[1, "#010008"],
			]),
			round("yo-rift", [
				[0, "#ffffff"],
				[0.25, "#e7d4ff", 0.9],
				[0.6, "#8c56e0", 0.4],
				[1, "#3a1668", 0],
			]),
			round("yo-eye", [
				[0, "#14041e"],
				[0.5, "#2a0a3e"],
				[1, "#000"],
			]),
			...HUES.map(([a, b, c], i) =>
				round(`yo-globe-${i}`, [
					[0, a],
					[0.3, b],
					[0.78, c],
					[1, b],
				], "0.38", "0.32", "0.72")
			)
		)
	);
	const space = add(svg, "g", { opacity: 0 });
	add(space, "rect", { x: 0, y: 0, width: W, height: H, fill: "url(#yo-void)" });
	let stars = "";
	for (let i = 0; i < 170; i++) stars += dot(rand() * W, rand() * H, 0.4 + rand() ** 3 * 1.5);
	add(space, "path", { d: stars, fill: "#d9ccff", opacity: 0.7 });

	// The dials: rings of hours in numerals nobody uses, with hands, each turning at its own rate.
	const NUMERALS = ["I", "IV", "VII", "X", "XIII", "∞", "0", "XI", "Ω", "III", "Λ", "IX"];
	const dials = [];
	const dialRoot = add(space, "g", {});
	[
		[CX, CY, 118],
		[CX, CY, 186],
		[CX, CY, 262],
		[CX, CY, 352],
		[CX, CY, 452],
		[190, 96, 86],
		[830, 390, 104],
		[860, 84, 60],
		[120, 400, 66],
	].forEach(([cx, cy, r], i) => {
		const g = add(dialRoot, "g", {});
		add(g, "circle", { cx, cy, r, fill: "none", stroke: "#b89cff", "stroke-width": 1.4, "stroke-dasharray": i % 2 ? "3 6" : `${num(r * 0.9)} ${num(r * 0.12)}` });
		add(g, "circle", { cx, cy, r: r - 18, fill: "none", stroke: "#6a4ab8", "stroke-width": 0.8 });
		let ticks = "";
		for (let j = 0; j < 60; j++) {
			const a = (j / 60) * 6.283;
			const long = j % 5 === 0 ? 11 : 5;
			ticks += `M${num(cx + Math.cos(a) * r)} ${num(cy + Math.sin(a) * r)}L${num(cx + Math.cos(a) * (r - long))} ${num(cy + Math.sin(a) * (r - long))}`;
		}
		add(g, "path", { d: ticks, fill: "none", stroke: "#cdb8ff", "stroke-width": 1 });
		NUMERALS.forEach((text, j) => {
			const a = (j / 12) * 6.283;
			const label = s("text", { x: num(cx + Math.cos(a) * (r - 28)), y: num(cy + Math.sin(a) * (r - 28) + 5), "text-anchor": "middle", fill: "#cdb8ff", "font-family": "serif", "font-size": Math.max(10, Math.min(18, r * 0.09)) }, text);
			g.append(label);
		});
		const hand = add(g, "line", { x1: cx, y1: cy, x2: cx, y2: cy - r + 22, stroke: "#f0d890", "stroke-width": 2.2, "stroke-linecap": "round" });
		const hand2 = add(g, "line", { x1: cx, y1: cy, x2: cx, y2: cy - r * 0.55, stroke: "#f0d890", "stroke-width": 3.2, "stroke-linecap": "round" });
		dials.push({ g, hand, hand2, cx, cy, rate: (i % 2 ? -1 : 1) * (10 + rand() * 18), handRate: 90 + rand() * 170, r });
	});

	// The gate: a thread of light that widens to an eye of light.
	const rift = add(svg, "ellipse", { cx: CX, cy: CY, rx: 1, ry: 1, fill: "url(#yo-rift)", opacity: 0 });
	const riftEdge = add(svg, "path", { fill: "none", stroke: "#fff", "stroke-width": 2.4, opacity: 0 });

	// The globes, drawn back to front, that come from the middle and stand round it, turning.
	const globeRoot = add(svg, "g", {});
	const globes = [];
	const COUNT = 46;
	for (let i = 0; i < COUNT; i++) {
		const k = (i + 0.5) / COUNT;
		// Filling a round cluster, the largest towards the middle and front.
		const ang = i * 2.39996;
		const dist = Math.sqrt(1 - k);
		const size = lerp(26, 86, k ** 1.3) * (0.82 + rand() * 0.36);
		const g = add(globeRoot, "g", {});
		const ball = add(g, "circle", { r: size, fill: `url(#yo-globe-${i % HUES.length})`, stroke: "#10031c", "stroke-width": 1 });
		const sheen = add(g, "ellipse", { cx: -size * 0.32, cy: -size * 0.36, rx: size * 0.3, ry: size * 0.19, fill: "#fff", opacity: 0.55, transform: `rotate(-32 ${num(-size * 0.32)} ${num(-size * 0.36)})` });
		// Oil on water: bands of other colours that slide round the surface.
		const films = [0, 1].map((n) => add(g, "circle", { r: size * (0.9 - n * 0.14), fill: "none", stroke: HUES[(i + 2 + n * 3) % HUES.length][1], "stroke-width": size * 0.09, "stroke-dasharray": `${num(size * 1.3)} ${num(size * 1.9)}`, opacity: 0.55 }));
		// Only some of them look back, through a slit, and these are not the same.
		const eyeWhite = add(g, "ellipse", { rx: size * 0.27, ry: 0.2, fill: "#ece4b2" });
		const eye = add(g, "ellipse", { rx: size * 0.055, ry: 0.2, fill: "url(#yo-eye)" });
		globes.push({ g, ball, sheen, films, eye, eyeWhite, size, ang, dist, at: 2 + (1 - k) * 1.9 + rand() * 0.3, spin: (i % 2 ? -1 : 1) * (0.06 + rand() * 0.1), bob: rand() * 6.28, blink: rand() * 6.28, opens: 4.35 + rand() * 0.75, seeing: i % 2 === 1 });
	}
	const burst = awakeningBurst(svg, CX, CY, 4.4, ["#ffffff", "#c58aff", "#f2e8ff"], 0.52);

	function seek(t) {
		const fade = 1 - between(t, YOG_SECONDS - 0.8, YOG_SECONDS);
		const fall = smooth(between(t, 0.1, 1.3));
		const open = smooth(between(t, 1.4, 3.1));
		const reverse = smooth(between(t, 3.9, 4.7)); // when he is through, time turns back
		const clock = t < 3.9 ? t : 3.9 + (t - 3.9) * lerp(1, -3.4, reverse);

		svg.style.opacity = String(fade);
		space.setAttribute("opacity", String(fall));
		for (const d of dials) {
			d.g.setAttribute("transform", `rotate(${num((clock * d.rate) % 360)} ${d.cx} ${d.cy})`);
			d.hand.setAttribute("transform", `rotate(${num((clock * d.handRate) % 360)} ${d.cx} ${d.cy})`);
			d.hand2.setAttribute("transform", `rotate(${num((clock * d.handRate * 0.083) % 360)} ${d.cx} ${d.cy})`);
			d.g.setAttribute("opacity", String(smooth(between(t, 0.3, 1.5)) * (0.78 - open * 0.3)));
		}
		const glowOut = 1 - smooth(between(t, 4.8, 6.2)) * 0.45;
		rift.setAttribute("rx", num(lerp(2, 340, open)));
		rift.setAttribute("ry", num(lerp(170, 300, open)));
		rift.setAttribute("opacity", String(between(t, 1.1, 1.6) * glowOut));
		const w = lerp(1, 150, open);
		const up = lerp(120, 230, open);
		riftEdge.setAttribute("d", `M${CX} ${num(CY - up)}Q${num(CX + w)} ${CY} ${CX} ${num(CY + up)}Q${num(CX - w)} ${CY} ${CX} ${num(CY - up)}Z`);
		riftEdge.setAttribute("opacity", String(between(t, 1.1, 1.5) * (1 - between(t, 3.4, 4.3))));

		for (const b of globes) {
			const come = between(t, b.at, b.at + 1.6);
			const out = outCubic(come);
			const a = b.ang + t * b.spin;
			const r = b.dist * lerp(10, 232, out);
			const x = CX + Math.cos(a) * r * 1.5 + Math.sin(t * 0.9 + b.bob) * 4;
			const y = CY + Math.sin(a) * r * 0.82 + Math.cos(t * 0.75 + b.bob) * 4;
			const scale = Math.max(0.001, out) * (1 + Math.sin(t * 1.6 + b.bob) * 0.03);
			b.g.setAttribute("transform", `translate(${num(x)} ${num(y)}) scale(${scale.toFixed(3)})`);
			b.g.setAttribute("opacity", String(come > 0 ? 0.96 : 0));
			b.films.forEach((film, n) => film.setAttribute("transform", `rotate(${num((clock * (n ? -46 : 62) + b.bob * 57) % 360)})`));
			const lid = b.seeing ? smooth(between(t, b.opens, b.opens + 0.3)) * (0.82 + Math.sin(t * 1.3 + b.blink) * 0.18) : 0;
			b.eyeWhite.setAttribute("ry", num(0.2 + lid * b.size * 0.15));
			b.eyeWhite.setAttribute("opacity", lid > 0 ? "1" : "0");
			b.eye.setAttribute("ry", num(0.2 + lid * b.size * 0.15));
			const look = Math.sin(t * 0.7 + b.blink) * b.size * 0.12;
			b.eye.setAttribute("cx", num(look));
		}
		burst(t);
	}
	return { el: svg, seek };
}
