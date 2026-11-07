// Ubbo-Sathla, the Unbegotten, at Hell's Banquet: a long table laid in a hall by candlelight, its covered dishes
// waiting. The lids begin to shiver, the great one in the middle is thrown off, and what is under it comes up
// and over the table, grey, without a shape, budding mouths, feeding on the feast and on itself. Original art,
// drawn in code (see js/cw-art-kit.js).

import { H, W, add, awakeningBurst, between, blob, dot, lerp, limb, num, outCubic, randoms, round, sceneSvg, smooth, vertical, writhe } from "./cw-art-kit.js";
import { s } from "/static/js/util.js";

export const UBBO_SECONDS = 7.4;
export const UBBO_PEAK = 5.7;

const CX = 500;
const TOP = 268; // the far end of the table
const FRONT = 452; // its near edge

// Where a point of the table's top lies: across from -1 to 1, along from 0 (far) to 1 (near).
const onTable = (across, along) => {
	const half = lerp(150, 462, along);
	return [CX + across * half, lerp(TOP, FRONT, along), lerp(0.42, 1.16, along)];
};

export function ubboScene() {
	const rand = randoms(1933); // the year of "Ubbo-Sathla"
	const svg = sceneSvg();
	svg.append(
		s(
			"defs",
			{},
			vertical("ub-hall", [
				[0, "#080402"],
				[0.55, "#1d0d06"],
				[1, "#0b0502"],
			]),
			round("ub-light", [
				[0, "#e8a040", 0.5],
				[0.5, "#8a3c14", 0.2],
				[1, "#2a1006", 0],
			]),
			vertical("ub-table", [
				[0, "#3a1d0e"],
				[1, "#6d3a1b"],
			]),
			vertical("ub-cloth", [
				[0, "#5d1412"],
				[1, "#2a0807"],
			]),
			vertical("ub-chair", [
				[0, "#2c170b"],
				[1, "#0a0502"],
			]),
			vertical("ub-pillar", [
				[0, "#120904"],
				[0.6, "#2d170b"],
				[1, "#0f0703"],
			]),
			vertical("ub-lid", [
				[0, "#e0c78a"],
				[0.5, "#94743a"],
				[1, "#3f2c10"],
			]),
			round("ub-flame", [
				[0, "#fff6c9"],
				[0.45, "#ffb33c"],
				[1, "#c8380c", 0],
			]),
			round("ub-glow", [
				[0, "#ffb95c", 0.5],
				[1, "#ffb95c", 0],
			]),
			round("ub-mass", [
				[0, "#c2a79d"],
				[0.4, "#7d5f5c"],
				[0.75, "#43303a"],
				[1, "#2c2029"],
			], "0.42", "0.26", "0.9"),
			vertical("ub-drip", [
				[0, "#7a6a60"],
				[1, "#2a2224"],
			]),
			vertical("ub-maw", [
				[0, "#14050a"],
				[1, "#6a1420"],
			]),
			round("ub-eye", [
				[0, "#fff7c4"],
				[0.5, "#e2c04a"],
				[1, "#6a4a10"],
			])
		)
	);
	const hall = add(svg, "g", { opacity: 0 });
	add(hall, "rect", { x: 0, y: 0, width: W, height: H, fill: "url(#ub-hall)" });
	const light = add(hall, "ellipse", { cx: CX, cy: 300, rx: 560, ry: 300, fill: "url(#ub-light)" });
	// An arched hall, going back into the dark.
	for (const [x, w] of [
		[52, 66],
		[210, 44],
		[330, 26],
		[670, 26],
		[790, 44],
		[948, 66],
	]) {
		add(hall, "rect", { x: x - w / 2, y: -10, width: w, height: 330, fill: "url(#ub-pillar)" });
	}
	add(hall, "path", { d: "M236 60Q280 -10 322 60M678 60Q720 -10 764 60M78 90Q130 -30 186 90M814 90Q870 -30 922 90", fill: "none", stroke: "#3b1f0f", "stroke-width": 7 });

	// The chairs down both sides, the high backs of them, all empty.
	for (const along of [0.08, 0.28, 0.5, 0.74]) {
		for (const side of [-1, 1]) {
			const [x, y, k] = onTable(side * 1.03, along);
			const w = 44 * k;
			const up = 138 * k;
			add(hall, "path", { d: `M${num(x - w / 2)} ${num(y + 10 * k)}L${num(x - w / 2)} ${num(y - up + 14 * k)}Q${num(x)} ${num(y - up - 14 * k)} ${num(x + w / 2)} ${num(y - up + 14 * k)}L${num(x + w / 2)} ${num(y + 10 * k)}Z`, fill: "url(#ub-chair)", stroke: "#40220f", "stroke-width": 1 });
		}
	}
	// The table, and its red cloth hanging over the near end.
	add(hall, "path", { d: `M${CX - 150} ${TOP}L${CX + 150} ${TOP}L${CX + 462} ${FRONT}L${CX - 462} ${FRONT}Z`, fill: "url(#ub-table)", stroke: "#1a0c05", "stroke-width": 2 });
	add(hall, "path", { d: `M${CX - 96} ${TOP}L${CX + 96} ${TOP}L${CX + 300} ${FRONT}L${CX - 300} ${FRONT}Z`, fill: "url(#ub-cloth)", opacity: 0.92 });
	add(hall, "path", { d: `M${CX - 462} ${FRONT}L${CX + 462} ${FRONT}L${CX + 462} ${H + 10}L${CX - 462} ${H + 10}Z`, fill: "#1a0c05" });
	let drape = `M${CX - 300} ${FRONT}`;
	for (let i = 0; i <= 12; i++) drape += `Q${CX - 300 + (i - 0.5) * 50} ${FRONT + 60} ${CX - 300 + i * 50} ${FRONT + 36 + (i % 2) * 8}`;
	add(hall, "path", { d: `${drape}L${CX + 300} ${FRONT}Z`, fill: "#3f0d0b", stroke: "#1a0504", "stroke-width": 1 });

	// The feast: dishes and goblets to either side of the great dish.
	const things = add(hall, "g", {});
	const shivers = [];
	const settings = [
		[-0.55, 0.28],
		[0.55, 0.28],
		[-0.62, 0.6],
		[0.62, 0.6],
		[-0.5, 0.9],
		[0.5, 0.9],
		[-0.3, 0.16],
		[0.3, 0.16],
	];
	settings.forEach(([across, along], i) => {
		const [x, y, k] = onTable(across, along);
		add(things, "ellipse", { cx: x, cy: y + 6 * k, rx: 40 * k, ry: 13 * k, fill: "#b89b5e", stroke: "#3f2c10", "stroke-width": 1 });
		const g = add(things, "g", {});
		add(g, "path", { d: `M${num(x - 30 * k)} ${num(y + 5 * k)}C${num(x - 30 * k)} ${num(y - 30 * k)} ${num(x + 30 * k)} ${num(y - 30 * k)} ${num(x + 30 * k)} ${num(y + 5 * k)}Z`, fill: "url(#ub-lid)", stroke: "#2c1e0a", "stroke-width": 1 });
		add(g, "circle", { cx: x, cy: y - 25 * k, r: 4 * k, fill: "#e0c78a", stroke: "#2c1e0a", "stroke-width": 0.8 });
		shivers.push({ g, phase: i * 1.7, at: 1.9 + i * 0.13, x, y });
		const [gx, gy, gk] = onTable(across * 0.62, along + 0.1);
		add(things, "path", { d: `M${num(gx - 8 * gk)} ${num(gy - 22 * gk)}L${num(gx + 8 * gk)} ${num(gy - 22 * gk)}Q${num(gx + 7 * gk)} ${num(gy - 8 * gk)} ${num(gx + 2 * gk)} ${num(gy - 6 * gk)}L${num(gx + 2 * gk)} ${num(gy + 2 * gk)}L${num(gx + 7 * gk)} ${num(gy + 4 * gk)}L${num(gx - 7 * gk)} ${num(gy + 4 * gk)}L${num(gx - 2 * gk)} ${num(gy + 2 * gk)}L${num(gx - 2 * gk)} ${num(gy - 6 * gk)}Q${num(gx - 7 * gk)} ${num(gy - 8 * gk)} ${num(gx - 8 * gk)} ${num(gy - 22 * gk)}Z`, fill: "#c9a95c", stroke: "#2c1e0a", "stroke-width": 0.8 });
	});

	// Candles down the table, which gutter when it comes.
	const candles = [];
	for (const [across, along] of [
		[-0.82, 0.12],
		[0.82, 0.12],
		[-0.86, 0.46],
		[0.86, 0.46],
		[-0.86, 0.82],
		[0.86, 0.82],
	]) {
		const [x, y, k] = onTable(across, along);
		add(things, "path", { d: `M${num(x - 18 * k)} ${num(y + 2 * k)}Q${num(x)} ${num(y - 8 * k)} ${num(x + 18 * k)} ${num(y + 2 * k)}L${num(x + 3 * k)} ${num(y - 6 * k)}L${num(x + 3 * k)} ${num(y - 34 * k)}L${num(x - 3 * k)} ${num(y - 34 * k)}L${num(x - 3 * k)} ${num(y - 6 * k)}Z`, fill: "#a8884a" });
		add(things, "rect", { x: x - 5 * k, y: y - 62 * k, width: 10 * k, height: 30 * k, fill: "#e6dcc0" });
		candles.push({ glow: add(things, "circle", { cx: x, cy: y - 74 * k, r: 64 * k, fill: "url(#ub-glow)" }), flame: add(things, "ellipse", { cx: x, cy: y - 74 * k, rx: 7 * k, ry: 14 * k, fill: "url(#ub-flame)" }), phase: rand() * 6.28, k });
	}

	// The great dish in the middle, then the lid that goes flying.
	const [DX, DY, DK] = onTable(0, 0.52);
	add(hall, "ellipse", { cx: DX, cy: DY + 10, rx: 124 * DK, ry: 42 * DK, fill: "#b89b5e", stroke: "#3f2c10", "stroke-width": 2 });

	// It: lobes that swell up out of the dish and spread, drips over the table's edge, and things it grows.
	const mass = add(svg, "g", {});
	const tendrils = [];
	for (let i = 0; i < 14; i++) {
		const across = (i - 6.5) / 6.5;
		tendrils.push({ el: add(mass, "path", { fill: "url(#ub-drip)", stroke: "#120d0e", "stroke-width": 1 }), root: [DX + across * 170, 348 + Math.abs(across) * 18], lean: across * 1.2, length: 90 + rand() * 130, width: 28 + rand() * 18, phase: rand() * 6.28, rate: 1 + rand() * 0.9, up: i % 3 === 0 });
	}
	const lobes = [];
	const placed = [
		[0, 262, 196, 142],
		[-180, 306, 122, 92],
		[180, 310, 128, 88],
		[-96, 196, 96, 80],
		[110, 186, 88, 76],
		[-292, 372, 84, 54],
		[296, 376, 88, 52],
		[0, 330, 214, 92],
		[10, 128, 58, 56],
		[-60, 268, 120, 96],
		[84, 264, 118, 92],
	];
	for (const [dx, y, rx, ry] of placed) lobes.push({ el: add(mass, "path", { fill: "url(#ub-mass)" }), x: DX + dx, y, rx, ry, phase: rand() * 6.28, delay: Math.abs(dx) / 700 + (262 - Math.min(y, 262)) / 400 });
	// The small things that bud from it, mouths and eyes, opening one after another.
	const maws = [];
	for (let i = 0; i < 19; i++) {
		const a = rand() * 6.283;
		const r = Math.sqrt(rand());
		const x = DX + Math.cos(a) * r * 270;
		const y = 278 + Math.sin(a) * r * 118;
		const size = 11 + rand() * 25;
		const g = add(mass, "g", {});
		const maw = add(g, "path", { fill: "url(#ub-maw)", stroke: "#0c0608", "stroke-width": 1.2 });
		const teeth = add(g, "path", { fill: "#d8cdb0" });
		maws.push({ g, maw, teeth, x, y, size, at: 4.2 + rand() * 1.1, phase: rand() * 6.28 });
	}
	const eyes = [];
	for (let i = 0; i < 23; i++) {
		const a = rand() * 6.283;
		const r = Math.sqrt(rand());
		const size = 3.5 + rand() * 7.5;
		eyes.push({ ball: add(mass, "ellipse", { cx: DX + Math.cos(a) * r * 300, cy: 248 + Math.sin(a) * r * 126, rx: size, ry: 0.2, fill: "url(#ub-eye)", stroke: "#100b0c", "stroke-width": 0.8 }), size, at: 4.3 + rand() * 1 });
	}
	// It is wet: long runs of slime down it, and gleams.
	let sheen = "";
	for (let i = 0; i < 26; i++) sheen += dot(DX + (rand() - 0.5) * 500, 170 + rand() * 190, 1.5 + rand() * 3.5);
	let runs = "";
	for (let i = 0; i < 46; i++) {
		const x = DX + (rand() - 0.5) * 560;
		const y = 150 + rand() * 210;
		runs += `M${num(x)} ${num(y)}q${num((rand() - 0.5) * 7)} ${num(10 + rand() * 18)} ${num((rand() - 0.5) * 6)} ${num(22 + rand() * 40)}`;
	}
	const shine = add(mass, "g", {});
	add(shine, "path", { d: sheen, fill: "#f4e4dc" });
	add(shine, "path", { d: runs, fill: "none", stroke: "#e8d0c8", "stroke-width": 1.6, "stroke-linecap": "round" });

	const lid = add(svg, "g", {});
	add(lid, "path", { d: `M${num(DX - 104 * DK)} ${DY + 6}C${num(DX - 104 * DK)} ${num(DY - 124 * DK)} ${num(DX + 104 * DK)} ${num(DY - 124 * DK)} ${num(DX + 104 * DK)} ${DY + 6}Z`, fill: "url(#ub-lid)", stroke: "#2c1e0a", "stroke-width": 2 });
	add(lid, "circle", { cx: DX, cy: DY - 92 * DK, r: 12 * DK, fill: "#e0c78a", stroke: "#2c1e0a", "stroke-width": 1.4 });
	const spill = [];
	for (let i = 0; i < 26; i++) spill.push({ el: add(svg, "circle", { r: 2 + rand() * 5, fill: "#6e6058", opacity: 0 }), a: -rand() * 3.14, power: 90 + rand() * 280 });
	const burst = awakeningBurst(svg, DX, 260, 4.45, ["#e6d6c0", "#b24a2a", "#f2d4a0"], 0.32);

	function seek(t) {
		const fade = 1 - between(t, UBBO_SECONDS - 0.8, UBBO_SECONDS);
		const fall = smooth(between(t, 0.1, 1.3));
		const thrown = between(t, 3.1, 4.3); // the lid goes
		const rise = smooth(between(t, 3.15, 4.75));
		const after = smooth(between(t, 4.5, 6)); // and it goes on growing

		svg.style.opacity = String(fade);
		hall.setAttribute("opacity", String(fall));
		light.setAttribute("opacity", String(0.75 - after * 0.2));
		for (const sh of shivers) {
			const k = between(t, sh.at, 3.2);
			const buck = Math.sin(t * 34 + sh.phase) * k * 1.8 + (Math.sin(t * 7 + sh.phase) > 0.65 ? -k * 5 : 0);
			sh.g.setAttribute("transform", `translate(${num(Math.sin(t * 21 + sh.phase) * k)} ${num(buck)})`);
		}
		for (const c of candles) {
			const gutter = 1 - after * 0.42;
			const wind = rise * Math.sin(t * 9 + c.phase) * 0.3;
			const f = 0.86 + Math.sin(t * 11 + c.phase) * 0.14;
			c.flame.setAttribute("ry", num(14 * c.k * f * gutter));
			c.flame.setAttribute("transform", `rotate(${num(wind * 30)} ${c.flame.getAttribute("cx")} ${c.flame.getAttribute("cy")})`);
			c.glow.setAttribute("opacity", String(f * gutter));
		}
		// The great lid trembles, rears up on what is under it, and goes over the far end in a spin.
		const quake = between(t, 1.6, 3.1);
		const go = thrown ** 1.6;
		const lx = lerp(0, 330, go);
		const ly = Math.sin(Math.min(1, thrown) * Math.PI * 0.88) * -210 - quake * (Math.sin(t * 8) > 0.4 ? 8 : 0);
		lid.setAttribute("transform", `translate(${num(lx + Math.sin(t * 29) * quake * 2.4)} ${num(ly + go * go * 180)}) rotate(${num(go * 280)} ${num(DX)} ${num(DY - 30)})`);
		lid.setAttribute("opacity", String(fall * (1 - between(t, 4.3, 4.6))));

		for (const l of lobes) {
			const k = smooth(between(t, 3.15 + l.delay * 0.8, 4.6 + l.delay * 0.8));
			const grow = k * (1 + after * 0.1);
			const d = blob(l.x, lerp(DY + 14, l.y, k) - after * 10, Math.max(1, l.rx * grow), Math.max(1, l.ry * grow), (a) => 1 + Math.sin(a * 3 + t * 1.5 + l.phase) * 0.06 + Math.sin(a * 7 - t * 2.1 + l.phase) * 0.035, 24);
			l.el.setAttribute("d", d);
			l.el.setAttribute("opacity", k > 0 ? "1" : "0");
		}
		for (const a of tendrils) {
			const k = smooth(between(t, 3.8, 5.2));
			const pts = writhe(a.root, a.up ? Math.PI - a.lean * 0.8 : a.lean * 0.7, a.length * k + 1, 10, t, a.rate, a.phase, 0.55);
			a.el.setAttribute("d", limb(pts, (j) => lerp(a.width, 3, j ** 0.8)));
			a.el.setAttribute("opacity", k > 0 ? "1" : "0");
		}
		shine.setAttribute("opacity", String(rise * 0.3));
		for (const m of maws) {
			const k = smooth(between(t, m.at, m.at + 0.5));
			if (k <= 0) {
				m.g.setAttribute("opacity", "0");
				continue;
			}
			m.g.setAttribute("opacity", "1");
			const chew = 0.45 + (Math.sin(t * 3.2 + m.phase) * 0.5 + 0.5) * 0.55;
			const open = k * chew * m.size * 0.72;
			const w = m.size * k;
			m.maw.setAttribute("d", `M${num(m.x - w)} ${num(m.y)}Q${num(m.x)} ${num(m.y - open)} ${num(m.x + w)} ${num(m.y)}Q${num(m.x)} ${num(m.y + open)} ${num(m.x - w)} ${num(m.y)}Z`);
			let fangs = "";
			for (let i = 1; i < 6; i++) {
				const x = m.x - w + (i / 6) * w * 2;
				const edge = (1 - ((x - m.x) / w) ** 2) * open * 0.5;
				fangs += `M${num(x - 2)} ${num(m.y - edge)}L${num(x)} ${num(m.y - edge + m.size * 0.2)}L${num(x + 2)} ${num(m.y - edge)}ZM${num(x - 2)} ${num(m.y + edge)}L${num(x)} ${num(m.y + edge - m.size * 0.2)}L${num(x + 2)} ${num(m.y + edge)}Z`;
			}
			m.teeth.setAttribute("d", fangs);
		}
		for (const e of eyes) e.ball.setAttribute("ry", num(0.2 + smooth(between(t, e.at, e.at + 0.25)) * e.size * 0.72));
		for (const p of spill) {
			const go2 = between(t, 3.25, 4.5);
			const out = outCubic(go2);
			p.el.setAttribute("cx", num(DX + Math.cos(p.a) * p.power * out));
			p.el.setAttribute("cy", num(DY - 20 + Math.sin(p.a) * p.power * 0.7 * out + go2 * go2 * 240));
			p.el.setAttribute("opacity", String(go2 > 0 && go2 < 1 ? 0.9 * (1 - go2) : 0));
		}
		burst(t);
	}
	return { el: svg, seek };
}
