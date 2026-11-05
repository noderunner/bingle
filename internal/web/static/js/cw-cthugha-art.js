// Cthugha, the Living Flame: sparks over the dark map, a fire that runs up from the bottom of the world until it
// is all there is, and in the middle of it a white heart that gathers itself, too bright to look at, throwing out
// arcs of itself, with the fire vampires going round it. It has no face, only three long rents in the blaze, and
// what shows through them looks back. Original art, drawn in code (see js/cw-art-kit.js).

import { H, W, add, awakeningBurst, between, blob, limb, lerp, num, randoms, round, sceneSvg, smooth, vertical, writhe } from "./cw-art-kit.js";
import { s } from "./util.js";

export const CTHUGHA_SECONDS = 7.2;
export const CTHUGHA_PEAK = 5.5;

const CX = 500;
const CY = 226;

export function cthughaScene() {
	const rand = randoms(1944); // the year of "The Dweller in Darkness"
	const svg = sceneSvg();
	svg.append(
		s(
			"defs",
			{},
			vertical("cg-dark", [
				[0, "#090100"],
				[0.6, "#260502"],
				[1, "#651104"],
			]),
			round("cg-heat", [
				[0, "#ffb23c", 0.85],
				[0.45, "#d8400c", 0.4],
				[1, "#4a0a02", 0],
			]),
			vertical("cg-tongue", [
				[0, "#fff0a8"],
				[0.3, "#ffb32c"],
				[0.65, "#e8480c"],
				[1, "#7c1404"],
			]),
			vertical("cg-tongue-back", [
				[0, "#ff8a1c"],
				[0.5, "#b82a06"],
				[1, "#3c0a02"],
			]),
			round("cg-tongue-up", [
				[0, "#ffffff"],
				[0.4, "#ffe680"],
				[0.75, "#ff8a1c"],
				[1, "#c8300a"],
			]),
			round("cg-ball", [
				[0, "#ffffff"],
				[0.4, "#fff3b0"],
				[0.72, "#ffb02c"],
				[1, "#e24a0c"],
			]),
			round("cg-void", [
				[0, "#000"],
				[0.65, "#160200"],
				[1, "#7c1404"],
			]),
			round("cg-corona", [
				[0, "#fff2b8", 0.9],
				[0.4, "#ffa030", 0.5],
				[1, "#c8300a", 0],
			]),
			round("cg-spark", [
				[0, "#ffffff"],
				[0.4, "#ffcf4a"],
				[1, "#e24a0c", 0],
			])
		)
	);
	const dark = add(svg, "rect", { x: 0, y: 0, width: W, height: H, fill: "url(#cg-dark)", opacity: 0 });
	const heat = add(svg, "ellipse", { cx: CX, cy: 500, rx: 720, ry: 240, fill: "url(#cg-heat)", opacity: 0 });

	// Flames from the bottom of the frame, two ranks of them.
	const makeFlames = (n, length, width, fill, base) => {
		const out = [];
		for (let i = 0; i < n; i++) {
			const x = ((i + 0.5) / n) * (W + 120) - 60 + (rand() - 0.5) * 30;
			out.push({ el: add(svg, "path", { fill }), x, base, length: length * (0.65 + rand() * 0.7), width: width * (0.7 + rand() * 0.6), phase: rand() * 6.28, rate: 3 + rand() * 2.4, delay: rand() * 0.9, lean: (x - CX) / 2200 });
		}
		return out;
	};
	const back = makeFlames(26, 390, 110, "url(#cg-tongue-back)", H + 30);

	// The ball of him, with its corona and the great tongues that leap off it.
	const corona = add(svg, "circle", { cx: CX, cy: CY, r: 10, fill: "url(#cg-corona)", opacity: 0 });
	const leaps = [];
	for (let i = 0; i < 52; i++) {
		const a = (i / 52) * 6.283 + rand() * 0.2;
		leaps.push({ el: add(svg, "path", { fill: "url(#cg-tongue-up)" }), a, length: 70 + rand() ** 2 * 300, width: 14 + rand() * 30, phase: rand() * 6.28, rate: 2.2 + rand() * 2.8 });
	}
	// Arcs of him that go out and come back, as a star's do.
	const arcs = [];
	for (let i = 0; i < 9; i++) arcs.push({ el: add(svg, "path", { fill: "none", stroke: i % 2 ? "#ffd65c" : "#ff8a1c", "stroke-width": 2.5 + rand() * 4, "stroke-linecap": "round" }), a: (i / 9) * 6.283 + rand(), span: 0.5 + rand() * 0.5, height: 70 + rand() * 90, phase: rand() * 6.28 });
	const ball = add(svg, "path", { fill: "url(#cg-ball)" });
	const cells = [];
	for (let i = 0; i < 34; i++) {
		const a = rand() * 6.283;
		const r = Math.sqrt(rand()) * 0.9;
		cells.push({ el: add(svg, "ellipse", { fill: i % 3 === 0 ? "#fff8d8" : "#e2560f", opacity: 0.3 }), a, r, size: 10 + rand() * 22, phase: rand() * 6.28, turn: rand() * 180 });
	}
	// Three long rents in the fire, leaning, with something dark and steady behind them.
	const rents = [
		[-64, -34, -24, 62, 15],
		[64, -34, 24, 62, 15],
		[0, 72, 0, 74, 11],
	].map(([dx, dy, tilt, long, wide]) => ({
		glow: add(svg, "ellipse", { cx: CX + dx, cy: CY + dy, rx: wide * 2.2, ry: long * 0.72, fill: "#7c1404", opacity: 0, transform: `rotate(${tilt} ${CX + dx} ${CY + dy})` }),
		el: add(svg, "path", { fill: "url(#cg-void)", opacity: 0, transform: `rotate(${tilt} ${CX + dx} ${CY + dy})` }),
		x: CX + dx,
		y: CY + dy,
		long,
		wide,
	}));

	const front = makeFlames(30, 225, 86, "url(#cg-tongue)", H + 30);

	// The fire vampires, sparks that know where they are going, and the embers that do not.
	const vampires = [];
	for (let i = 0; i < 18; i++) vampires.push({ el: add(svg, "ellipse", { rx: 13, ry: 4.5, fill: "url(#cg-spark)", opacity: 0 }), a: rand() * 6.283, r: 170 + rand() * 170, rate: (i % 2 ? 1 : -1) * (0.7 + rand() * 0.9), tilt: 0.45 + rand() * 0.25 });
	const embers = [];
	for (let i = 0; i < 110; i++) embers.push({ el: add(svg, "circle", { r: 0.8 + rand() * 2.6, fill: i % 3 ? "#ffb02c" : "#fff2b8", opacity: 0 }), x: rand() * W, speed: 0.16 + rand() * 0.34, phase: rand(), wob: rand() * 6.28 });
	const whiten = add(svg, "rect", { x: 0, y: 0, width: W, height: H, fill: "#fff0b8", opacity: 0 });
	const burst = awakeningBurst(svg, CX, CY, 4.4, ["#fff5c8", "#ff8a1c", "#ffffff"], 0.75);

	function seek(t) {
		const fade = 1 - between(t, CTHUGHA_SECONDS - 0.8, CTHUGHA_SECONDS);
		const fall = smooth(between(t, 0.1, 1.2));
		const climb = smooth(between(t, 0.7, 3.2)); // the fire comes up
		const gather = smooth(between(t, 2.5, 4.4)); // the heart draws itself together
		const out = smooth(between(t, 4.3, 5)); // and breaks out
		const awake = smooth(between(t, 4.5, 5));

		svg.style.opacity = String(fade);
		dark.setAttribute("opacity", String(fall));
		heat.setAttribute("opacity", String(climb));
		heat.setAttribute("ry", num(lerp(180, 430, climb)));

		const flames = (list, tall) => {
			for (const f of list) {
				const k = smooth(between(t, 0.6 + f.delay, 3 + f.delay * 0.6));
				const size = k * tall * (0.9 + Math.sin(t * f.rate * 0.7 + f.phase) * 0.14);
				if (size < 0.01) {
					f.el.setAttribute("d", "");
					continue;
				}
				const pts = writhe([f.x, f.base], Math.PI + f.lean, f.length * size, 11, t, f.rate, f.phase, 0.42);
				f.el.setAttribute("d", limb(pts, (j) => lerp(f.width * size ** 0.5, 1.5, j ** 0.75)));
			}
		};
		// Once he is out the fire draws down a little, so that he can be seen.
		flames(back, 1 - out * 0.22);
		flames(front, 1 - out * 0.32);

		const size = lerp(4, 96, gather) + out * 58;
		const pulse = 1 + Math.sin(t * 5.2) * 0.02 + Math.sin(t * 8.3) * 0.012;
		const r = size * pulse;
		corona.setAttribute("r", num(r * lerp(2.1, 3.1, out)));
		corona.setAttribute("opacity", String(between(t, 2.4, 3) * (0.9 + Math.sin(t * 6) * 0.1)));
		ball.setAttribute("d", blob(CX, CY, r, r, (a) => 1 + Math.sin(a * 5 + t * 3) * 0.035 + Math.sin(a * 11 - t * 5) * 0.022, 36));
		ball.setAttribute("opacity", String(between(t, 2.5, 2.8)));
		for (const l of leaps) {
			const reach = between(t, 3, 4.6) * (0.74 + Math.sin(t * l.rate + l.phase) * 0.26);
			const root = [CX + Math.cos(l.a) * r * 0.88, CY + Math.sin(l.a) * r * 0.88];
			const lean = Math.atan2(Math.cos(l.a), Math.sin(l.a));
			const pts = writhe(root, lean, l.length * reach * (0.5 + out * 0.6) + 1, 10, t, l.rate, l.phase, 0.6);
			l.el.setAttribute("d", limb(pts, (j) => lerp(l.width * (0.5 + gather * 0.5), 1.5, j ** 0.7)));
			l.el.setAttribute("opacity", String(gather));
		}
		for (const c of cells) {
			const a = c.a + t * 0.3;
			const f = 1 + Math.sin(t * 3 + c.phase) * 0.3;
			c.el.setAttribute("cx", num(CX + Math.cos(a) * c.r * r));
			c.el.setAttribute("cy", num(CY + Math.sin(a) * c.r * r));
			c.el.setAttribute("rx", num((c.size * f * r) / 150));
			c.el.setAttribute("ry", num(((c.size * r) / 150) * 0.5));
			c.el.setAttribute("transform", `rotate(${num(c.turn + t * 12)} ${num(CX + Math.cos(a) * c.r * r)} ${num(CY + Math.sin(a) * c.r * r)})`);
			c.el.setAttribute("opacity", String(gather * 0.42));
		}
		for (const a of arcs) {
			const on = smooth(between(t, 3.4, 4.6));
			const a0 = a.a + t * 0.12;
			const a1 = a0 + a.span;
			const mid = (a0 + a1) / 2;
			const up = r + a.height * (0.7 + Math.sin(t * 1.7 + a.phase) * 0.3) * (0.6 + out * 0.7);
			a.el.setAttribute("d", `M${num(CX + Math.cos(a0) * r * 0.97)} ${num(CY + Math.sin(a0) * r * 0.97)}Q${num(CX + Math.cos(mid) * up * 1.3)} ${num(CY + Math.sin(mid) * up * 1.3)} ${num(CX + Math.cos(a1) * r * 0.97)} ${num(CY + Math.sin(a1) * r * 0.97)}`);
			a.el.setAttribute("opacity", String(on * 0.85));
		}
		for (const v of rents) {
			// They open as a crack does, from the middle, and flicker with the fire across them.
			const long = v.long * awake * (1 + Math.sin(t * 2.4 + v.x) * 0.04);
			const wide = v.wide * awake * (0.86 + Math.sin(t * 7.3 + v.y) * 0.14);
			v.el.setAttribute("d", `M${v.x} ${num(v.y - long)}Q${num(v.x + wide * 1.5)} ${num(v.y - long * 0.2)} ${num(v.x + wide * 0.3)} ${num(v.y + long)}Q${num(v.x - wide * 1.5)} ${num(v.y + long * 0.2)} ${v.x} ${num(v.y - long)}Z`);
			v.el.setAttribute("opacity", String(awake));
			v.glow.setAttribute("opacity", String(awake * 0.3));
		}

		for (const v of vampires) {
			const a = v.a + t * v.rate;
			const on = smooth(between(t, 3.2, 4.2));
			const x = CX + Math.cos(a) * v.r * lerp(0.6, 1.15, out);
			const y = CY + Math.sin(a) * v.r * v.tilt * lerp(0.6, 1.15, out);
			const dir = ((a + (v.rate > 0 ? 1.57 : -1.57)) * 180) / Math.PI;
			v.el.setAttribute("transform", `translate(${num(x)} ${num(y)}) rotate(${num(dir)})`);
			v.el.setAttribute("opacity", String(on));
		}
		for (const e of embers) {
			const k = (t * e.speed + e.phase) % 1;
			e.el.setAttribute("cx", num(e.x + Math.sin(k * 8 + e.wob) * 18));
			e.el.setAttribute("cy", num(H + 10 - k * (H + 30)));
			e.el.setAttribute("opacity", String(fall * (1 - k) * (0.4 + climb * 0.6)));
		}
		whiten.setAttribute("opacity", String(gather * (1 - between(t, 4.4, 4.5)) * 0.22));
		burst(t);
	}
	return { el: svg, seek };
}
