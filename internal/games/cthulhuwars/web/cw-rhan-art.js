// Rhan-Tegoth, breaking out of the ice: a glacier's face, with something dark and many-limbed held deep in it.
// Cracks run out through the blue, it stirs, and the ice comes apart: a round body, six limbs with crab's claws,
// a long proboscis and three fishy eyes. Original art, drawn in code (see js/cw-art-kit.js).

import { H, W, add, awakeningBurst, between, blob, dot, limb, lerp, num, outCubic, randoms, round, sceneSvg, smooth, vertical, writhe } from "./cw-art-kit.js";
import { s } from "/static/js/util.js";

export const RHAN_SECONDS = 7.2;
export const RHAN_PEAK = 5.6;

const CX = 500;
const CY = 232;

/** A facet's colour: a glacier is palest towards the top, and each face catches the light its own way. */
function facet(x, y, luck) {
	const k = Math.max(0, Math.min(1, y / H + (luck - 0.5) * 0.55));
	const mix = (a, b) => Math.round(lerp(a, b, k));
	return `rgb(${mix(190, 16)} ${mix(232, 74)} ${mix(242, 102)})`;
}

export function rhanScene() {
	const rand = randoms(1932); // the year of "The Horror in the Museum"
	const svg = sceneSvg();
	svg.append(
		s(
			"defs",
			{},
			vertical("rt-dark", [
				[0, "#02080d"],
				[1, "#071721"],
			]),
			vertical("rt-ice", [
				[0, "#b7e4ef"],
				[0.45, "#4fa3bd"],
				[1, "#123f58"],
			]),
			round("rt-deep", [
				[0, "#d9fbff", 0.55],
				[0.5, "#57c4d8", 0.2],
				[1, "#0f4158", 0],
			]),
			vertical("rt-shard", [
				[0, "#e8fbff"],
				[1, "#3f8ea8"],
			]),
			round("rt-body", [
				[0, "#40515a"],
				[0.55, "#1d2a31"],
				[1, "#080e12"],
			], "0.42", "0.32", "0.75"),
			vertical("rt-limb", [
				[0, "#2e3e47"],
				[1, "#0c1317"],
			]),
			vertical("rt-claw", [
				[0, "#7d99a3"],
				[1, "#1b2930"],
			]),
			round("rt-eye", [
				[0, "#fbfff2"],
				[0.5, "#a7ef9a"],
				[1, "#237a55"],
			]),
			round("rt-eye-glow", [
				[0, "#b4ff9e", 0.85],
				[0.4, "#39b98a", 0.34],
				[1, "#0f4158", 0],
			]),
			round("rt-mist", [
				[0, "#e5f8ff", 0.5],
				[1, "#e5f8ff", 0],
			])
		)
	);
	const cave = add(svg, "g", { opacity: 0 });
	add(cave, "rect", { x: 0, y: 0, width: W, height: H, fill: "url(#rt-dark)" });
	const deep = add(cave, "ellipse", { cx: CX, cy: CY, rx: 480, ry: 300, fill: "url(#rt-deep)", opacity: 0 });

	// He, all there from the start, behind the ice.
	const figure = add(svg, "g", {});
	const limbs = [];
	const roots = [
		[-96, -46, -1.95, 270],
		[96, -46, 1.95, 270],
		[-120, 20, -1.45, 310],
		[120, 20, 1.45, 310],
		[-76, 84, -0.7, 240],
		[76, 84, 0.7, 240],
	];
	for (const [dx, dy, lean, length] of roots) {
		limbs.push({
			el: add(figure, "path", { fill: "url(#rt-limb)", stroke: "#05090c", "stroke-width": 1.2 }),
			rings: add(figure, "path", { fill: "none", stroke: "#6b8791", "stroke-width": 1.4, opacity: 0.6 }),
			upper: add(figure, "path", { fill: "url(#rt-claw)", stroke: "#05090c", "stroke-width": 1 }),
			lower: add(figure, "path", { fill: "url(#rt-claw)", stroke: "#05090c", "stroke-width": 1 }),
			root: [CX + dx, CY + dy],
			lean,
			length,
			phase: rand() * 6.28,
			rate: 0.9 + rand() * 0.7,
		});
	}
	const torso = add(figure, "path", { fill: "url(#rt-body)", stroke: "#05090c", "stroke-width": 1.8 });
	let plates = "";
	for (let i = 0; i < 90; i++) {
		const a = rand() * 6.283;
		const k = Math.sqrt(rand()) * 0.93;
		plates += dot(CX + Math.cos(a) * k * 138, CY + Math.sin(a) * k * 122, 1 + rand() * 2.6);
	}
	add(figure, "path", { d: plates, fill: "#0a1115", stroke: "#54717c", "stroke-width": 0.6, opacity: 0.8 });
	add(figure, "ellipse", { cx: CX, cy: CY - 6, rx: 34, ry: 28, fill: "#26343c", stroke: "#05090c", "stroke-width": 1.3 });
	const proboscis = add(figure, "path", { fill: "url(#rt-limb)", stroke: "#05090c", "stroke-width": 1.3 });
	add(figure, "path", { d: `M${CX - 100} ${CY - 86}C${CX - 60} ${CY - 118} ${CX + 60} ${CY - 118} ${CX + 100} ${CY - 86}`, fill: "none", stroke: "#9fb7c0", "stroke-width": 7, "stroke-linecap": "round", opacity: 0.24 });
	const sucker = add(figure, "circle", { r: 13, fill: "#16090c", stroke: "#8e5b5f", "stroke-width": 2.4 });
	const eyes = [
		[CX - 58, CY - 44, 21],
		[CX + 58, CY - 44, 21],
		[CX, CY - 78, 17],
	].map(([cx, cy, r]) => ({
		glow: add(figure, "circle", { cx, cy, r: r * 3.6, fill: "url(#rt-eye-glow)", opacity: 0 }),
		ball: add(figure, "ellipse", { cx, cy, rx: r, ry: 0.4, fill: "url(#rt-eye)", stroke: "#061014", "stroke-width": 2 }),
		pupil: add(figure, "ellipse", { cx, cy, rx: r * 0.48, ry: 0.3, fill: "#04100a" }),
		r,
	}));

	// The ice is a sheet of facets, each of which goes its own way when it breaks.
	const ice = add(svg, "g", {});
	const shards = [];
	const COLS = 12;
	const ROWS = 7;
	const grid = [];
	for (let y = 0; y <= ROWS; y++) {
		const row = [];
		for (let x = 0; x <= COLS; x++) {
			const edge = x === 0 || y === 0 || x === COLS || y === ROWS;
			row.push([(x / COLS) * (W + 60) - 30 + (edge ? 0 : (rand() - 0.5) * 56), (y / ROWS) * (H + 60) - 30 + (edge ? 0 : (rand() - 0.5) * 50)]);
		}
		grid.push(row);
	}
	for (let y = 0; y < ROWS; y++) {
		for (let x = 0; x < COLS; x++) {
			const [a, b, c, d] = [grid[y][x], grid[y][x + 1], grid[y + 1][x + 1], grid[y + 1][x]];
			for (const tri of [
				[a, b, c],
				[a, c, d],
			]) {
				const cx = (tri[0][0] + tri[1][0] + tri[2][0]) / 3;
				const cy = (tri[0][1] + tri[1][1] + tri[2][1]) / 3;
				const dist = Math.hypot(cx - CX, (cy - CY) * 1.5);
				shards.push({
					el: add(ice, "path", { d: `M${num(tri[0][0])} ${num(tri[0][1])}L${num(tri[1][0])} ${num(tri[1][1])}L${num(tri[2][0])} ${num(tri[2][1])}Z`, fill: facet(cx, cy, rand()), stroke: "#dff7fc", "stroke-width": 0.7, "fill-opacity": 0.42 + (Math.hypot(cx - CX, (cy - CY) * 1.4) / 620) * 0.5, "stroke-opacity": 0.12 + rand() * 0.32 }),
					cx,
					cy,
					delay: dist / 900 + rand() * 0.14,
					spin: (rand() - 0.5) * 420,
					drop: 120 + rand() * 340,
					far: 180 + rand() * 340,
				});
			}
		}
	}
	// The cracks, each drawn along its length, from the middle out.
	const cracks = [];
	for (let i = 0; i < 16; i++) {
		const a = (i / 16) * 6.283 + rand() * 0.3;
		let x = CX + Math.cos(a) * 50;
		let y = CY + Math.sin(a) * 40;
		let d = `M${num(x)} ${num(y)}`;
		let heading = a;
		const legs = 7 + Math.floor(rand() * 5);
		for (let j = 0; j < legs; j++) {
			heading += (rand() - 0.5) * 0.9;
			x += Math.cos(heading) * (32 + rand() * 42);
			y += Math.sin(heading) * (24 + rand() * 34);
			d += `L${num(x)} ${num(y)}`;
		}
		cracks.push({ el: add(svg, "path", { d, fill: "none", stroke: "#f2feff", "stroke-width": 1.2 + rand() * 2.2, "stroke-linecap": "round", pathLength: 1, "stroke-dasharray": "1 1", "stroke-dashoffset": 1 }), at: 1.5 + rand() * 2.1 });
	}
	const mists = [];
	for (let i = 0; i < 9; i++) mists.push({ el: add(svg, "ellipse", { fill: "url(#rt-mist)", opacity: 0 }), x: rand() * W, y: 330 + rand() * 170, rx: 120 + rand() * 180, drift: (rand() - 0.5) * 80 });
	const burst = awakeningBurst(svg, CX, CY, 4.3, ["#f1feff", "#57c4d8", "#e8fbff"], 0.72);

	function seek(t) {
		const fade = 1 - between(t, RHAN_SECONDS - 0.8, RHAN_SECONDS);
		const fall = smooth(between(t, 0.1, 1.1));
		const stir = smooth(between(t, 2.6, 4.2));
		const free = between(t, 4.3, 5.1);
		const spread = smooth(between(t, 4.3, 5));
		const awake = smooth(between(t, 4.55, 4.95));

		svg.style.opacity = String(fade);
		cave.setAttribute("opacity", String(fall));
		deep.setAttribute("opacity", String(smooth(between(t, 1.5, 4.2)) * 0.8 + free * 0.2));

		// The ice shudders before it goes.
		const shake = stir * (t < 4.3 ? 1 : 0) * Math.sin(t * 46) * (1 + stir * 2);
		ice.setAttribute("opacity", String(fall));
		ice.setAttribute("transform", `translate(${num(shake)} ${num(-shake * 0.5)})`);
		for (const sh of shards) {
			const k = between(t, 4.3 + sh.delay, 5.5 + sh.delay);
			if (k <= 0) {
				sh.el.setAttribute("transform", "");
				sh.el.setAttribute("opacity", "1");
				continue;
			}
			const a = Math.atan2(sh.cy - CY, sh.cx - CX);
			const out = outCubic(k);
			const x = Math.cos(a) * sh.far * out;
			const y = Math.sin(a) * sh.far * 0.7 * out + k * k * sh.drop;
			sh.el.setAttribute("transform", `translate(${num(x)} ${num(y)}) rotate(${num(sh.spin * k)} ${num(sh.cx)} ${num(sh.cy)})`);
			sh.el.setAttribute("opacity", String(1 - between(k, 0.55, 1)));
		}
		for (const c of cracks) {
			const k = smooth(between(t, c.at, c.at + 1.1));
			c.el.setAttribute("stroke-dashoffset", num(1 - k));
			c.el.setAttribute("opacity", String(fall * (k > 0 ? 0.9 : 0) * (1 - between(t, 4.3, 4.5))));
		}

		// Inside the ice he is held small and still, and only twitches.
		const size = lerp(0.8, 1.14, spread);
		const heave = Math.sin(t * 1.15) * 5 * spread;
		figure.setAttribute("transform", `translate(${CX} ${num(CY + heave)}) scale(${size.toFixed(3)}) translate(${-CX} ${-CY})`);
		figure.setAttribute("opacity", String(smooth(between(t, 0.5, 1.8))));
		torso.setAttribute("d", blob(CX, CY, 146, 130, (a) => 1 + Math.sin(a * 3 + t * 0.8) * 0.012 * (1 + spread) + Math.sin(a * 2 - 1.57) * 0.03));
		const live = 0.08 + stir * 0.2 + spread * 0.72;
		for (const l of limbs) {
			const reach = lerp(0.46, 1, spread);
			const points = writhe(l.root, l.lean, l.length * reach, 13, t * (0.4 + live), l.rate, l.phase, 0.14 + live * 0.5);
			l.el.setAttribute("d", limb(points, (k) => lerp(36, 12, k)));
			let rings = "";
			for (let i = 2; i < points.length - 1; i += 2) {
				const [ax, ay] = points[i - 1];
				const [bx, by] = points[i + 1];
				const dx = bx - ax;
				const dy = by - ay;
				const len = Math.hypot(dx, dy) || 1;
				const w = lerp(16, 6, i / points.length);
				rings += `M${num(points[i][0] - (dy / len) * w)} ${num(points[i][1] + (dx / len) * w)}L${num(points[i][0] + (dy / len) * w)} ${num(points[i][1] - (dx / len) * w)}`;
			}
			l.rings.setAttribute("d", rings);
			// The claw at its end, which opens and shuts.
			const tip = points[points.length - 1];
			const prev = points[points.length - 2];
			const a = (Math.atan2(tip[1] - prev[1], tip[0] - prev[0]) * 180) / Math.PI;
			const gape = 10 + (0.5 + Math.sin(t * 2.3 + l.phase) * 0.5) * 30 * spread;
			const claw = "M0 -7C20 -24 54 -24 72 -2C50 -10 26 -6 6 6Z";
			l.upper.setAttribute("d", claw);
			l.upper.setAttribute("transform", `translate(${num(tip[0])} ${num(tip[1])}) rotate(${num(a - gape)})`);
			l.lower.setAttribute("d", claw);
			l.lower.setAttribute("transform", `translate(${num(tip[0])} ${num(tip[1])}) rotate(${num(a + gape)}) scale(1 -1)`);
		}
		const noseRoot = [CX, CY - 6];
		const nose = writhe(noseRoot, Math.sin(t * 0.8) * 0.3, lerp(70, 196, spread), 10, t, 1.7, 0.4, 0.2 + spread * 0.5);
		proboscis.setAttribute("d", limb(nose, (k) => lerp(42, 17, k ** 0.6)));
		const end = nose[nose.length - 1];
		sucker.setAttribute("cx", num(end[0]));
		sucker.setAttribute("cy", num(end[1]));
		for (const e of eyes) {
			e.ball.setAttribute("ry", num(0.4 + awake * e.r * 0.9));
			e.pupil.setAttribute("ry", num(0.3 + awake * e.r * 0.68));
			e.glow.setAttribute("opacity", String(awake * (0.85 + Math.sin(t * 4 + e.r) * 0.15)));
		}
		for (const m of mists) {
			const k = between(t, 4.2, 6.6);
			m.el.setAttribute("cx", num(m.x + m.drift * k));
			m.el.setAttribute("cy", num(m.y - k * 34));
			m.el.setAttribute("rx", num(m.rx * (0.7 + k * 0.6)));
			m.el.setAttribute("ry", num(44 + k * 40));
			m.el.setAttribute("opacity", String(smooth(between(t, 4.25, 4.9)) * (1 - k * 0.5) * 0.5));
		}
		burst(t);
	}
	return { el: svg, seek };
}
