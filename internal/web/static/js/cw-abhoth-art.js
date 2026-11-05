// Abhoth, the Source of Uncleanness, coming up out of the muck: a low cave and a black sump, with a sick light on
// it. Bubbles come up and burst. It heaves, it swells into a hill of grey jelly, and it will not hold its shape:
// eyes open all over it, and things bud from its sides and drop off into the muck, and crawl. Original art, drawn
// in code (see js/cw-art-kit.js).

import { H, W, add, awakeningBurst, between, blob, dot, limb, lerp, num, outCubic, randoms, round, sceneSvg, smooth, vertical, writhe } from "./cw-art-kit.js";
import { s } from "./util.js";

export const ABHOTH_SECONDS = 7.4;
export const ABHOTH_PEAK = 5.7;

const CX = 500;
const SURFACE = 348; // where the muck meets the back of the cave

export function abhothScene() {
	const rand = randoms(1933); // the year of "The Seven Geases"
	const svg = sceneSvg();
	svg.append(
		s(
			"defs",
			{},
			vertical("ab-cave", [
				[0, "#030403"],
				[0.7, "#111509"],
				[1, "#1d2210"],
			]),
			round("ab-sick", [
				[0, "#a2ae3a", 0.46],
				[0.5, "#4a5416", 0.2],
				[1, "#141806", 0],
			]),
			vertical("ab-muck", [
				[0, "#39401c"],
				[0.25, "#1f2410"],
				[1, "#080a04"],
			]),
			round("ab-mass", [
				[0, "#a3a884"],
				[0.35, "#6c7050"],
				[0.72, "#32352a"],
				[1, "#12130f"],
			], "0.42", "0.28", "0.85"),
			round("ab-lump", [
				[0, "#a3a884"],
				[0.5, "#62664c"],
				[1, "#4a4d3a", 0],
			], "0.42", "0.32", "0.72"),
			vertical("ab-arm", [
				[0, "#3c3f30"],
				[1, "#71755a"],
			]),
			round("ab-bubble", [
				[0, "#d2d89c", 0.1],
				[0.75, "#a2ae3a", 0.3],
				[1, "#e4eab0", 0.9],
			]),
			round("ab-eye", [
				[0, "#fffce0"],
				[0.55, "#d6e04a"],
				[1, "#5e6a10"],
			]),
			round("ab-eye-glow", [
				[0, "#d6e04a", 0.55],
				[1, "#5e6a10", 0],
			])
		)
	);
	const cave = add(svg, "g", { opacity: 0 });
	add(cave, "rect", { x: 0, y: 0, width: W, height: H, fill: "url(#ab-cave)" });
	const sick = add(cave, "ellipse", { cx: CX, cy: SURFACE, rx: 620, ry: 260, fill: "url(#ab-sick)" });
	// The roof comes low, and drips.
	let roof = `M-10 -10L${W + 10} -10L${W + 10} 44`;
	for (let x = W; x >= -40; x -= 52) roof += `Q${num(x - 14)} ${num(60 + rand() * 66)} ${num(x - 26)} ${num(44 + rand() * 14)}L${num(x - 40)} ${num(60 + rand() * 40)}L${num(x - 52)} 44`;
	add(cave, "path", { d: `${roof}L-10 44Z`, fill: "#040503", stroke: "#2b311a", "stroke-width": 1.2 });
	add(cave, "path", { d: `M0 ${SURFACE}L0 228C70 196 120 262 190 ${SURFACE}ZM${W} ${SURFACE}L${W} 210C930 186 860 264 806 ${SURFACE}Z`, fill: "#080a05", stroke: "#2b311a", "stroke-width": 1 });
	add(cave, "rect", { x: 0, y: SURFACE, width: W, height: H - SURFACE, fill: "url(#ab-muck)" });
	// Oily rings on its surface.
	const rings = [];
	for (let i = 0; i < 14; i++) rings.push({ el: add(cave, "ellipse", { cx: rand() * W, cy: SURFACE + 18 + rand() * 130, fill: "none", stroke: "#8a944a", "stroke-width": 1, opacity: 0.25 }), rx: 40 + rand() * 90, phase: rand() * 6.28 });

	// It.
	const mound = add(svg, "g", {});
	const arms = [];
	for (let i = 0; i < 12; i++) {
		const across = (i - 5.5) / 5.5;
		arms.push({ el: add(mound, "path", { fill: "url(#ab-arm)", stroke: "#0d0e0a", "stroke-width": 1 }), root: [CX + across * 270, 190 + Math.abs(across) * 100], lean: Math.PI - across * 1.2, length: 150 + rand() * 170, width: 30 + rand() * 18, phase: rand() * 6.28, rate: 1 + rand() * 0.9 });
	}
	const body = add(mound, "path", { fill: "url(#ab-mass)", stroke: "#0d0e0a", "stroke-width": 1.6 });
	const lumps = [];
	const spots = [
		[-210, 300, 70, 54],
		[220, 306, 76, 52],
		[-120, 196, 62, 56],
		[126, 188, 66, 56],
		[0, 112, 62, 50],
		[-290, 342, 48, 32],
		[296, 344, 52, 34],
		[-50, 288, 46, 40],
		[66, 300, 52, 38],
	];
	for (const [dx, y, rx, ry] of spots) lumps.push({ el: add(mound, "path", { fill: "url(#ab-lump)", opacity: 0.86 }), x: CX + dx, y, rx, ry, phase: rand() * 6.28 });
	let slime = "";
	for (let i = 0; i < 60; i++) {
		const x = CX + (rand() - 0.5) * 560;
		const y = 110 + rand() * 230;
		slime += `M${num(x)} ${num(y)}q${num(rand() * 3)} ${num(8 + rand() * 20)} ${num((rand() - 0.5) * 3)} ${num(20 + rand() * 30)}`;
	}
	const gloss = add(mound, "path", { d: slime, fill: "none", stroke: "#d6da9e", "stroke-width": 1.5, "stroke-linecap": "round", opacity: 0.3 });
	// Eyes, all sizes, none of them matched.
	const eyes = [];
	for (let i = 0; i < 27; i++) {
		const a = rand() * 6.283;
		const r = Math.sqrt(rand());
		const size = i < 3 ? 22 - i * 3 : 4 + rand() * 11;
		const cx = i < 3 ? CX + [-46, 54, 4][i] : CX + Math.cos(a) * r * 262;
		const cy = i < 3 ? [190, 196, 122][i] : 240 + Math.sin(a) * r * 112;
		eyes.push({
			glow: i < 3 ? add(mound, "circle", { cx, cy, r: size * 3, fill: "url(#ab-eye-glow)", opacity: 0 }) : null,
			ball: add(mound, "ellipse", { cx, cy, rx: size, ry: 0.2, fill: "url(#ab-eye)", stroke: "#0d0e0a", "stroke-width": 1 }),
			pupil: add(mound, "ellipse", { cx, cy, rx: size * 0.36, ry: 0.2, fill: "#0a0c02" }),
			size,
			at: i < 3 ? 4.42 : 4.5 + rand() * 1.1,
			wander: rand() * 6.28,
		});
	}
	// What comes off it, and goes crawling away over the muck.
	const brood = [];
	for (let i = 0; i < 7; i++) {
		const dir = i % 2 ? 1 : -1;
		brood.push({ g: add(svg, "g", { opacity: 0 }), dir, at: 4.7 + i * 0.16, x0: CX + dir * (150 + rand() * 120), y0: 300 + rand() * 40, x1: CX + dir * (300 + rand() * 170), y1: 380 + rand() * 94, size: 13 + rand() * 13, phase: rand() * 6.28 });
	}
	for (const b of brood) {
		b.body = add(b.g, "path", { fill: "url(#ab-lump)", stroke: "#0d0e0a", "stroke-width": 1 });
		b.legs = add(b.g, "path", { fill: "none", stroke: "#55583f", "stroke-width": 2.2, "stroke-linecap": "round" });
		b.eye = add(b.g, "circle", { r: b.size * 0.2, fill: "url(#ab-eye)" });
	}
	// The foot of the mound goes down into the muck.
	const lip = add(svg, "path", { fill: "url(#ab-muck)" });
	const bubbles = [];
	for (let i = 0; i < 26; i++) bubbles.push({ el: add(svg, "circle", { fill: "url(#ab-bubble)", stroke: "#c8ce88", "stroke-width": 0.7, opacity: 0 }), pop: add(svg, "circle", { fill: "none", stroke: "#c8ce88", "stroke-width": 1.2, opacity: 0 }), x: 60 + rand() * 880, y: SURFACE + 16 + rand() * 132, size: 6 + rand() * 20, life: 1.3 + rand() * 1.4, phase: rand() * 4 });
	const drops = [];
	for (let i = 0; i < 34; i++) drops.push({ el: add(svg, "circle", { r: 1.6 + rand() * 4, fill: "#6c7050", opacity: 0 }), a: -rand() * 3.14, power: 110 + rand() * 300 });
	const dripping = [];
	for (let i = 0; i < 9; i++) dripping.push({ el: add(svg, "circle", { r: 1.6 + rand() * 1.4, fill: "#8a944a", opacity: 0 }), x: rand() * W, phase: rand(), y0: 76 + rand() * 40 });
	const burst = awakeningBurst(svg, CX, 220, 4.4, ["#d6e04a", "#6c7050", "#e4eab0"], 0.28);

	function seek(t) {
		const fade = 1 - between(t, ABHOTH_SECONDS - 0.8, ABHOTH_SECONDS);
		const fall = smooth(between(t, 0.1, 1.3));
		const heave = smooth(between(t, 2.1, 4.4));
		const after = smooth(between(t, 4.4, 6.2));
		const seethe = 0.25 + heave * 0.75;

		svg.style.opacity = String(fade);
		cave.setAttribute("opacity", String(fall));
		sick.setAttribute("opacity", String(0.55 + Math.sin(t * 1.4) * 0.1 + heave * 0.35));
		for (const r of rings) {
			const k = 0.5 + Math.sin(t * 0.8 + r.phase) * 0.5;
			r.el.setAttribute("rx", num(r.rx * (0.6 + k * 0.6)));
			r.el.setAttribute("ry", num(r.rx * 0.13 * (0.6 + k * 0.6)));
		}
		// It comes up out of the surface, in shoves: it rises, sags a little, and rises further.
		const shove = heave + Math.sin(heave * 13) * 0.035 * (1 - heave);
		const y = lerp(320, 0, shove);
		mound.setAttribute("transform", `translate(0 ${num(y)})`);
		mound.setAttribute("opacity", String(between(t, 1.9, 2.3) * fall));
		const wide = 1 + after * 0.09;
		body.setAttribute("d", blob(CX, 262, 320 * wide, 172, (a) => 1 + Math.sin(a * 3 + t * 1.1) * 0.05 * seethe + Math.sin(a * 5 - t * 1.7) * 0.035 * seethe + Math.sin(a * 9 + t * 2.3) * 0.016, 34));
		for (const l of lumps) {
			const f = 1 + Math.sin(t * 1.5 + l.phase) * 0.08 * seethe;
			l.el.setAttribute("d", blob(l.x, l.y + Math.sin(t * 1.1 + l.phase) * 5, l.rx * f * wide, l.ry * (2 - f), (a) => 1 + Math.sin(a * 4 + t * 2 + l.phase) * 0.06, 18));
		}
		gloss.setAttribute("opacity", String(0.15 + heave * 0.2));
		for (const a of arms) {
			const k = smooth(between(t, 3.3, 5));
			const pts = writhe(a.root, a.lean, a.length * k + 1, 11, t, a.rate, a.phase, 0.62);
			a.el.setAttribute("d", limb(pts, (j) => lerp(a.width, 2.5, j ** 0.85)));
			a.el.setAttribute("opacity", k > 0 ? "1" : "0");
		}
		for (const e of eyes) {
			const open = smooth(between(t, e.at, e.at + 0.32));
			e.ball.setAttribute("ry", num(0.2 + open * e.size * 0.84));
			e.pupil.setAttribute("ry", num(0.2 + open * e.size * 0.36));
			const lookX = Math.sin(t * 0.8 + e.wander) * e.size * 0.34;
			const lookY = Math.cos(t * 0.6 + e.wander) * e.size * 0.2;
			e.pupil.setAttribute("transform", `translate(${num(lookX)} ${num(lookY)})`);
			if (e.glow) e.glow.setAttribute("opacity", String(open * (0.85 + Math.sin(t * 3 + e.wander) * 0.15)));
		}
		// The muck closes round its foot.
		let d = `M-10 ${SURFACE + 2}`;
		for (let x = 0; x <= W; x += 40) d += `L${x} ${num(SURFACE + 2 + Math.sin(x * 0.03 + t * 1.4) * 2 - Math.max(0, 1 - Math.abs(x - CX) / 380) * heave * 14)}`;
		lip.setAttribute("d", `${d}L${W + 10} ${SURFACE + 60}L-10 ${SURFACE + 60}Z`);

		for (const b of brood) {
			const k = between(t, b.at, b.at + 1.5);
			if (k <= 0) {
				b.g.setAttribute("opacity", "0");
				continue;
			}
			const go = outCubic(k);
			const x = lerp(b.x0, b.x1, go);
			const yy = lerp(b.y0, b.y1, go) - Math.sin(Math.min(1, k * 2.2) * Math.PI) * 42;
			b.g.setAttribute("opacity", String(fade));
			b.body.setAttribute("d", blob(x, yy, b.size * 1.25, b.size * 0.8, (a) => 1 + Math.sin(a * 3 + t * 6 + b.phase) * 0.1, 12));
			let legs = "";
			for (let i = 0; i < 6; i++) {
				const lx = x - b.size + (i / 5) * b.size * 2;
				const w = Math.sin(t * 13 + i * 2 + b.phase) * 5;
				legs += `M${num(lx)} ${num(yy + b.size * 0.5)}q${num(w)} ${num(b.size * 0.4)} ${num(w * 1.6 - (i - 2.5) * 2)} ${num(b.size * 0.8)}`;
			}
			b.legs.setAttribute("d", legs);
			b.eye.setAttribute("cx", num(x));
			b.eye.setAttribute("cy", num(yy - b.size * 0.15));
		}
		for (const b of bubbles) {
			const k = ((t + b.phase) % b.life) / b.life;
			const on = fall * (0.7 + heave * 0.3);
			if (k < 0.82) {
				const r = b.size * smooth(k / 0.82);
				b.el.setAttribute("cx", num(b.x));
				b.el.setAttribute("cy", num(b.y - r * 0.6));
				b.el.setAttribute("r", num(Math.max(0.1, r)));
				b.el.setAttribute("opacity", String(on));
				b.pop.setAttribute("opacity", "0");
			} else {
				const p = (k - 0.82) / 0.18;
				b.el.setAttribute("opacity", "0");
				b.pop.setAttribute("cx", num(b.x));
				b.pop.setAttribute("cy", num(b.y - b.size * 0.3));
				b.pop.setAttribute("r", num(b.size * (1 + p * 0.9)));
				b.pop.setAttribute("opacity", String(on * (1 - p) * 0.8));
			}
		}
		for (const p of drops) {
			const go = between(t, 2.6, 4.5);
			const out = outCubic(go);
			p.el.setAttribute("cx", num(CX + Math.cos(p.a) * p.power * 1.2 * out));
			p.el.setAttribute("cy", num(SURFACE + Math.sin(p.a) * p.power * 0.8 * out + go * go * 180));
			p.el.setAttribute("opacity", String(go > 0 && go < 1 ? 0.9 * (1 - go) : 0));
		}
		for (const dp of dripping) {
			const k = (t * 0.45 + dp.phase) % 1;
			dp.el.setAttribute("cx", num(dp.x));
			dp.el.setAttribute("cy", num(lerp(dp.y0, SURFACE + 20, k * k)));
			dp.el.setAttribute("opacity", String(fall * (k < 0.96 ? 0.85 : 0)));
		}
		burst(t);
	}
	return { el: svg, seek };
}
