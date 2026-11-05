// Shub-Niggurath, the Black Goat of the Woods, heaving up out of a dead swamp by moonlight: bubbles first, then the
// water parts round a mass with no fixed shape, horns on it, hooves under it and mouths opening all over it. Original
// art, drawn in code (see js/cw-art-kit.js).

import { H, W, add, between, curve, limb, lerp, num, outCubic, randoms, round, sceneSvg, smooth, vertical, writhe } from "./cw-art-kit.js";
import { s } from "./util.js";

export const SHUB_SECONDS = 7.2;
export const SHUB_PEAK = 5.4;

/** A wobbling blob: a closed curve round a point. */
function blob(cx, cy, rx, ry, t, rate, phase, lumps = 7) {
	const pts = [];
	for (let i = 0; i < 18; i++) {
		const a = (i / 18) * 6.283;
		const wob = 1 + Math.sin(a * lumps + phase + t * rate) * 0.05 + Math.sin(a * 3 - t * rate * 0.7 + phase) * 0.07;
		pts.push([cx + Math.cos(a) * rx * wob, cy + Math.sin(a) * ry * wob]);
	}
	let d = "";
	for (let i = 0; i < pts.length; i++) {
		const p = pts[i];
		const q = pts[(i + 1) % pts.length];
		const m = [(p[0] + q[0]) / 2, (p[1] + q[1]) / 2];
		d += i ? `Q${num(p[0])} ${num(p[1])} ${num(m[0])} ${num(m[1])}` : `M${num(m[0])} ${num(m[1])}`;
	}
	return `${d}Z`;
}

/** A dead tree, one path of tapering limbs. */
function tree(rand, x, y, height, lean) {
	let d = "";
	const grow = (x0, y0, angle, len, width, depth) => {
		const bend = (rand() - 0.5) * 40;
		const x1 = x0 + Math.sin(angle) * len;
		const y1 = y0 - Math.cos(angle) * len;
		d += limb(curve([x0, y0], [(x0 + x1) / 2 + bend, (y0 + y1) / 2], [x1, y1], 5), (k) => lerp(width, width * 0.55, k));
		if (depth <= 0) return;
		const n = depth > 2 ? 2 : 2 + Math.floor(rand() * 2);
		for (let i = 0; i < n; i++) grow(x1, y1, angle + (i - (n - 1) / 2) * (0.7 + rand() * 0.5), len * (0.55 + rand() * 0.25), width * 0.55, depth - 1);
	};
	grow(x, y, lean, height * 0.45, height * 0.085, 4);
	return d;
}

export function shubScene() {
	const rand = randoms(1928); // the year she is first called the Goat with a Thousand Young, more or less
	const svg = sceneSvg();
	svg.append(
		s(
			"defs",
			{},
			vertical("sh-sky", [
				[0, "#050805", 0.97],
				[0.6, "#141a0d", 0.95],
				[1, "#262a10", 0.92],
			]),
			round("sh-moon", [
				[0, "#d9dcb0"],
				[0.75, "#a6ac78"],
				[0.8, "#7c8150", 0.35],
				[1, "#7c8150", 0],
			]),
			round("sh-moon-glow", [
				[0, "#a8b070", 0.4],
				[1, "#a8b070", 0],
			]),
			vertical("sh-water", [
				[0, "#1c2410"],
				[0.35, "#0c1007"],
				[1, "#020301"],
			]),
			round("sh-mist", [
				[0, "#8d9a6c", 0.34],
				[0.6, "#6c7a50", 0.15],
				[1, "#6c7a50", 0],
			]),
			round("sh-flesh", [
				[0, "#3a1216"],
				[0.55, "#160709"],
				[1, "#040202"],
			], "0.4", "0.28", "0.8"),
			round("sh-flesh2", [
				[0, "#27090d"],
				[0.65, "#0c0405"],
				[1, "#020101"],
			], "0.45", "0.3", "0.75"),
			vertical("sh-horn", [
				[0, "#d8ceae"],
				[0.5, "#7a6e50"],
				[1, "#221c10"],
			]),
			vertical("sh-leg", [
				[0, "#14090a"],
				[1, "#020101"],
			]),
			round("sh-glow", [
				[0, "#d8201a", 0.7],
				[0.4, "#841010", 0.34],
				[1, "#3a0606", 0],
			]),
			round("sh-eye", [
				[0, "#fff0b0"],
				[0.5, "#ff7a2a"],
				[1, "#b8120e"],
			]),
			round("sh-eye-glow", [
				[0, "#ff4a2a", 0.85],
				[1, "#a01010", 0],
			]),
			round("sh-mouth", [
				[0, "#000"],
				[0.7, "#2c0204"],
				[1, "#7c0a10"],
			])
		)
	);
	const swamp = add(svg, "g", { opacity: 0 });
	add(swamp, "rect", { x: 0, y: 0, width: W, height: H, fill: "url(#sh-sky)" });
	add(swamp, "circle", { cx: 760, cy: 92, r: 160, fill: "url(#sh-moon-glow)" });
	add(swamp, "circle", { cx: 760, cy: 92, r: 44, fill: "url(#sh-moon)" });
	// Far trees, then near ones, dead a long time.
	add(swamp, "path", { d: tree(rand, 90, 372, 240, -0.1) + tree(rand, 220, 372, 170, 0.12) + tree(rand, 800, 372, 210, 0.08) + tree(rand, 910, 372, 260, -0.14) + tree(rand, 350, 368, 120, 0.1) + tree(rand, 670, 368, 130, -0.1), fill: "#0c120a" });
	const glowBack = add(swamp, "ellipse", { cx: 500, cy: 240, rx: 460, ry: 300, fill: "url(#sh-glow)", opacity: 0 });
	const mistBack = [0, 1, 2].map((i) => add(swamp, "ellipse", { cy: 340 + i * 8, rx: 300 + i * 40, ry: 42, fill: "url(#sh-mist)" }));

	// The back of the water, and the thing that breaks it.
	const waterBack = add(swamp, "path", { fill: "url(#sh-water)" });

	const mass = add(swamp, "g", {});
	// Young behind her: smaller, tree-legged, there when she is.
	const young = [
		[186, 340, 0.72],
		[812, 345, 0.82],
	].map(([x, y, sc]) => {
		const g = add(mass, "g", { transform: `translate(${x} ${y}) scale(${sc})`, opacity: 0 });
		const blobEl = add(g, "path", { fill: "url(#sh-flesh2)", stroke: "#54181c", "stroke-width": 1 });
		const legs = add(g, "path", { fill: "url(#sh-leg)", stroke: "#3c1216", "stroke-width": 0.8 });
		const arms = add(g, "path", { fill: "url(#sh-flesh2)", stroke: "#54181c", "stroke-width": 0.8 });
		return { g, blobEl, legs, arms, seed: x };
	});
	const hooves = [
		[300, 300],
		[700, 300],
		[390, 330],
		[610, 330],
	].map(() => add(mass, "path", { fill: "url(#sh-leg)", stroke: "#3c1216", "stroke-width": 1 }));
	const feelers = [];
	for (let i = 0; i < 16; i++) {
		const side = i % 2 ? 1 : -1;
		feelers.push({ el: add(mass, "path", { fill: "url(#sh-flesh)", stroke: "#6a1c20", "stroke-width": 0.9 }), root: [500 + side * (110 + rand() * 120), 170 + rand() * 170], lean: side * (1 + rand() * 1) + Math.PI * 0.12 * side, length: 150 + rand() * 170, width: 22 + rand() * 14, phase: rand() * 6.28, rate: 1 + rand() * 1.1 });
	}
	// The horns, curling, on what serves for a brow.
	const horns = [0, 1].map(() => add(mass, "path", { fill: "url(#sh-horn)", stroke: "#1a140a", "stroke-width": 1.2 }));
	const ridges = [0, 1].map(() => add(mass, "path", { fill: "none", stroke: "#1a140a", "stroke-width": 1.4, opacity: 0.8 }));
	const lumps = [];
	for (const [cx, cy, rx, ry] of [
		[500, 280, 240, 150],
		[345, 270, 118, 104],
		[655, 276, 124, 106],
		[420, 180, 104, 82],
		[584, 184, 106, 84],
		[500, 130, 96, 74],
		[305, 345, 110, 74],
		[695, 345, 112, 74],
		[500, 372, 200, 74],
	]) {
		lumps.push({ cx, cy, rx, ry, phase: rand() * 6.28, rate: 0.9 + rand() * 0.7, el: add(mass, "path", { fill: lumps.length % 2 ? "url(#sh-flesh2)" : "url(#sh-flesh)", stroke: "#6a1c20", "stroke-width": 1.2 }) });
	}
	const rim = add(mass, "path", { fill: "none", stroke: "#e0606a", "stroke-width": 1.6, opacity: 0.4 });
	// Her long face: a goat's, if a goat's were a hole.
	const face = add(mass, "g", {});
	add(face, "path", { d: "M452 124C462 100 538 100 548 124C556 172 538 222 518 262C510 276 490 276 482 262C462 222 444 172 452 124Z", fill: "#0a0305", stroke: "#6a1c20", "stroke-width": 1.4 });
	add(face, "path", { d: "M500 112V230M472 150C478 178 484 204 490 226M528 150C522 178 516 204 510 226", fill: "none", stroke: "#3a1014", "stroke-width": 1.8, opacity: 0.8 });
	const eyes = [
		[466, 148, 14],
		[534, 148, -14],
	].map(([x, y, tilt]) => ({
		glow: add(face, "circle", { cx: x, cy: y, r: 54, fill: "url(#sh-eye-glow)", opacity: 0 }),
		ball: add(face, "ellipse", { cx: x, cy: y, rx: 13, ry: 0.3, fill: "url(#sh-eye)", transform: `rotate(${tilt} ${x} ${y})` }),
		// A goat's pupil lies across the eye.
		slit: add(face, "rect", { x: x - 9, y: y - 1.5, width: 18, height: 3, fill: "#0a0200", transform: `rotate(${tilt} ${x} ${y})`, opacity: 0 }),
	}));
	const muzzle = add(face, "path", { fill: "#000", stroke: "#7c0a10", "stroke-width": 1.4 });
	const muzzleTeeth = add(face, "path", { fill: "#d8ceae", stroke: "#2c1c10", "stroke-width": 0.5 });

	// Mouths, all over her, opening one after another; and her small eyes.
	const mouths = [];
	for (const [cx, cy, rx, tilt] of [
		[338, 240, 42, -14],
		[662, 248, 44, 12],
		[272, 322, 34, -22],
		[728, 330, 36, 20],
		[408, 330, 38, -5],
		[592, 336, 40, 6],
		[372, 150, 26, -24],
		[626, 156, 26, 22],
	]) {
		const g = add(mass, "g", { transform: `translate(${cx} ${cy}) rotate(${tilt})` });
		mouths.push({ cx, rx, born: 3 + rand() * 1.5, phase: rand() * 6.28, rate: 2 + rand() * 2, lip: add(g, "path", { fill: "url(#sh-mouth)", stroke: "#a01820", "stroke-width": 1.4 }), teeth: add(g, "path", { fill: "#d8ceae", stroke: "#2c1c10", "stroke-width": 0.4 }) });
	}
	const smallEyes = [];
	for (let i = 0; i < 16; i++) {
		const x = 280 + rand() * 440;
		const y = 170 + rand() * 180;
		if (Math.abs(x - 500) < 76 && y < 280) continue;
		smallEyes.push({ el: add(mass, "circle", { cx: x, cy: y, r: 0, fill: "url(#sh-eye)" }), size: 2.8 + rand() * 3.4, born: 3.8 + rand() * 1.1 });
	}
	const drips = add(mass, "g", { stroke: "#1a1209", fill: "none", "stroke-linecap": "round" });
	const strands = [];
	for (let i = 0; i < 26; i++) {
		const x = 260 + rand() * 480;
		strands.push({ el: add(drips, "path", { d: `M${num(x)} ${num(220 + rand() * 130)}q${num((rand() - 0.5) * 14)} ${num(50 + rand() * 40)} ${num((rand() - 0.5) * 14)} ${num(90 + rand() * 70)}`, "stroke-width": 1.2 + rand() * 2.4, "stroke-dasharray": `${num(30 + rand() * 40)} 70` }), speed: 80 + rand() * 90 });
	}

	// In front: the water, with her reflection burning in it, then rings, bubbles and mist.
	const glint = add(swamp, "ellipse", { cx: 500, cy: 428, rx: 300, ry: 46, fill: "url(#sh-glow)", opacity: 0 });
	const waterFront = add(swamp, "path", { fill: "url(#sh-water)", opacity: 0.94 });
	const lily = add(swamp, "path", { fill: "none", stroke: "#58643a", "stroke-width": 1.1, opacity: 0.5 });
	const rippleEls = [];
	for (let i = 0; i < 10; i++) rippleEls.push({ el: add(swamp, "ellipse", { fill: "none", stroke: "#9aa66c", "stroke-width": 1.1 }), x: 150 + rand() * 700, y: 400 + rand() * 80, phase: rand(), rate: 0.35 + rand() * 0.3 });
	const bubbleEls = [];
	for (let i = 0; i < 44; i++) bubbleEls.push({ el: add(swamp, "circle", { fill: "#1d2410", stroke: "#a8b478", "stroke-width": 0.9, opacity: 0 }), x: 150 + rand() * 700, y: 390 + rand() * 96, r: 2 + rand() * 7, phase: rand(), rate: 0.7 + rand() * 0.8, born: 0.4 + rand() * 2 });
	const mistFront = [0, 1, 2, 3].map((i) => add(swamp, "ellipse", { cy: 396 + i * 22, rx: 340 + i * 30, ry: 44, fill: "url(#sh-mist)" }));
	const flies = [];
	const flyGroup = add(swamp, "g", { fill: "#d7e88a" });
	for (let i = 0; i < 18; i++) flies.push({ el: add(flyGroup, "circle", { r: 1.3, opacity: 0 }), x: rand() * W, y: 230 + rand() * 170, phase: rand() * 6.28 });
	const flash = add(svg, "rect", { x: 0, y: 0, width: W, height: H, fill: "#c81e1e", opacity: 0 });

	const waterLine = (level, amp, phase, t) => {
		let d = `M-20 ${num(level)}`;
		for (let x = 10; x <= W + 30; x += 30) d += `L${x} ${num(level + Math.sin(x / 90 + t * 0.7 + phase) * amp + Math.sin(x / 31 - t * 0.5) * amp * 0.35)}`;
		return d;
	};

	function seek(t) {
		const fade = 1 - between(t, SHUB_SECONDS - 0.8, SHUB_SECONDS);
		const dark = smooth(between(t, 0.1, 1.2));
		const emerge = outCubic(between(t, 1.8, 4.85));
		const awake = smooth(between(t, 4.35, 4.7));
		const level = 368;
		const rumble = (between(t, 1.6, 2.4) * (1 - between(t, 4.9, 5.5)) + between(t, 4.35, 4.45) * (1 - between(t, 4.45, 5))) * 3;
		const shakeX = Math.sin(t * 53) * rumble;
		const shakeY = Math.cos(t * 41) * rumble * 0.6;
		const breathe = Math.sin(t * 1.5);

		svg.style.opacity = String(fade);
		swamp.setAttribute("opacity", String(dark));
		glowBack.setAttribute("opacity", String(emerge * 0.42 + awake * (0.5 + breathe * 0.05)));
		mistBack.forEach((m, i) => m.setAttribute("cx", num(250 + i * 250 + Math.sin(t * 0.3 + i) * 70)));
		mistFront.forEach((m, i) => m.setAttribute("cx", num(170 + i * 230 + Math.sin(t * 0.28 + i * 2) * 90)));
		waterBack.setAttribute("d", `${waterLine(level - 8, 3, 0.4, t)}L${W + 30} ${H + 20}L-20 ${H + 20}Z`);
		const front = waterLine(level + 16, 5, 2.1, t);
		waterFront.setAttribute("d", `${front}L${W + 30} ${H + 20}L-20 ${H + 20}Z`);
		lily.setAttribute("d", waterLine(level + 46, 3, 3.3, t) + waterLine(level + 84, 4, 5.1, t));

		// She heaves up out of the water.
		const y = (1 - emerge) * 430 + breathe * 5 * emerge;
		mass.setAttribute("transform", `translate(${num(shakeX)} ${num(y + shakeY)})`);
		for (const l of lumps) l.el.setAttribute("d", blob(l.cx, l.cy, l.rx * (1 + Math.sin(t * l.rate + l.phase) * 0.025 * (1 + awake)), l.ry * (1 + Math.cos(t * l.rate + l.phase) * 0.035 * (1 + awake)), t, l.rate, l.phase));
		rim.setAttribute("d", `M${num(300)} 220C340 170 370 130 420 106C470 62 530 62 580 106C630 130 660 170 700 220`);
		rim.setAttribute("opacity", String(0.25 + awake * 0.45));

		// Horns: great curls, swept back, one each way.
		horns.forEach((horn, side) => {
			const mx = (x) => (side ? W - x : x);
			const sway = Math.sin(t * 1.1) * 3;
			// A ram's curl: up from the brow, back and round, tightening as it goes.
			const spine = [];
			for (let i = 0; i <= 26; i++) {
				const k = i / 26;
				const a = -0.5 + k * 6.1;
				const r = 96 * (1 - k * 0.82);
				spine.push([mx(330 - Math.cos(a) * r * -1 - 60 * (1 - k)) + sway * k, 62 + Math.sin(a) * r * 0.82 + 62 * (1 - k)]);
			}
			horn.setAttribute("d", limb(spine, (k) => lerp(48, 2.5, k ** 0.8)));
			let rings = "";
			for (let i = 2; i < 24; i += 2) {
				const p = spine[i];
				const q = spine[i + 1];
				const dx = q[1] - p[1];
				const dy = p[0] - q[0];
				const len = Math.hypot(dx, dy) || 1;
				const w = lerp(48, 2.5, (i / 26) ** 0.8) / 2;
				rings += `M${num(p[0] + (dx / len) * w)} ${num(p[1] + (dy / len) * w)}L${num(p[0] - (dx / len) * w)} ${num(p[1] - (dy / len) * w)}`;
			}
			ridges[side].setAttribute("d", rings);
		});

		// Legs like the boles of trees, with cloven feet, somewhere in the water.
		hooves.forEach((leg, i) => {
			const base = [[286, 300], [714, 300], [392, 342], [608, 342]][i];
			const foot = [base[0] + (i % 2 ? 1 : -1) * 54, 520];
			leg.setAttribute("d", limb(curve(base, [base[0] + (i % 2 ? 1 : -1) * (60 + Math.sin(t + i) * 8), 410], foot, 10), (k) => lerp(92, 40, k)));
		});

		for (const f of feelers) f.el.setAttribute("d", limb(writhe(f.root, f.lean, f.length * (0.4 + emerge * 0.6), 13, t, f.rate, f.phase, 0.55 + awake * 0.25), (k) => lerp(f.width, 1.5, k ** 0.8)));

		// The Young stand up late, at the edge.
		young.forEach((y2, i) => {
			const up = smooth(between(t, 3.2, 4.9));
			const baseY = 340 + (i ? 5 : 0) + (1 - up) * 260;
			y2.g.setAttribute("transform", `translate(${i ? 812 : 186} ${num(baseY - (1 - emerge) * 430 * 0 - y)}) scale(${i ? 0.82 : 0.72})`);
			y2.g.setAttribute("opacity", String(between(t, 3.2, 3.8)));
			y2.blobEl.setAttribute("d", blob(0, -86, 64, 74, t, 1.3, i * 2));
			let legs = "";
			for (const dir of [-1, 1]) {
				legs += limb(curve([dir * 26, -36], [dir * 64, 30], [dir * 70, 100], 8), (k) => lerp(26, 12, k));
				legs += limb(curve([dir * 12, -30], [dir * 24, 40], [dir * 26, 100], 8), (k) => lerp(20, 10, k));
			}
			y2.legs.setAttribute("d", legs);
			let arms = "";
			for (let a = 0; a < 7; a++) arms += limb(writhe([(a - 3) * 16, -130], (a - 3) * 0.45 + Math.PI, 90 + (a % 3) * 26, 9, t, 1.5, a * 1.3 + i, 0.55), (k) => lerp(13, 1.2, k));
			y2.arms.setAttribute("d", arms);
		});

		// The mouths open, and they do not close.
		for (const m of mouths) {
			const open = smooth(between(t, m.born, m.born + 0.55)) * (0.75 + Math.sin(t * m.rate + m.phase) * 0.25);
			const h2 = m.rx * 0.62 * open;
			m.lip.setAttribute("d", `M${num(-m.rx)} 0Q0 ${num(-h2 * 1.3)} ${num(m.rx)} 0Q0 ${num(h2 * 1.5)} ${num(-m.rx)} 0Z`);
			let teeth = "";
			const count = 7;
			for (let i = 0; i < count; i++) {
				const x = -m.rx * 0.8 + ((i + 0.5) / count) * m.rx * 1.6;
				const edge = 1 - (x / m.rx) ** 2;
				const top = -h2 * 0.65 * edge;
				const under = h2 * 0.75 * edge;
				const width = m.rx * 0.085;
				if (open > 0.05) {
					teeth += `M${num(x - width)} ${num(top)}L${num(x)} ${num(top + m.rx * 0.2 * open)}L${num(x + width)} ${num(top)}Z`;
					teeth += `M${num(x - width)} ${num(under)}L${num(x)} ${num(under - m.rx * 0.18 * open)}L${num(x + width)} ${num(under)}Z`;
				}
			}
			m.teeth.setAttribute("d", teeth);
		}
		const jaw = awake * (9 + Math.sin(t * 4) * 3);
		muzzle.setAttribute("d", `M478 ${num(244)}Q500 ${num(236 - jaw * 0.5)} 522 244Q500 ${num(254 + jaw * 1.9)} 478 244Z`);
		let fangs = "";
		if (jaw > 1) for (let i = 0; i < 5; i++) fangs += `M${478 + 8 + i * 7} ${num(242)}l3 ${num(jaw * 0.9)}l3 ${num(-jaw * 0.9)}Z`;
		muzzleTeeth.setAttribute("d", fangs);
		for (const e of eyes) {
			e.ball.setAttribute("ry", num(0.3 + awake * 7.5));
			e.slit.setAttribute("opacity", String(awake));
			e.glow.setAttribute("opacity", String(awake * (0.82 + Math.sin(t * 6) * 0.18)));
		}
		for (const e of smallEyes) e.el.setAttribute("r", num(e.size * smooth(between(t, e.born, e.born + 0.3))));

		for (const st of strands) st.el.setAttribute("stroke-dashoffset", num(-t * st.speed));
		drips.setAttribute("opacity", String(0.85 * between(t, 2, 2.6) * (1 - between(t, 5.3, 7) * 0.6)));
		glint.setAttribute("opacity", String(awake * (0.55 + breathe * 0.1)));

		for (const r of rippleEls) {
			const k = ((t * r.rate + r.phase) % 1) * between(t, 0.7, 1.5);
			r.el.setAttribute("cx", num(r.x));
			r.el.setAttribute("cy", num(r.y));
			r.el.setAttribute("rx", num(4 + k * 70));
			r.el.setAttribute("ry", num(1 + k * 12));
			r.el.setAttribute("opacity", String((1 - k) * 0.55 * between(t, 0.7, 1.5)));
		}
		for (const b of bubbleEls) {
			const k = ((t * b.rate + b.phase) % 1);
			b.el.setAttribute("cx", num(b.x + Math.sin(k * 8 + b.phase) * 2));
			b.el.setAttribute("cy", num(b.y - k * 10));
			b.el.setAttribute("r", num(b.r * (0.4 + k * 0.8)));
			b.el.setAttribute("opacity", String(between(t, b.born, b.born + 0.6) * (k > 0.92 ? (1 - k) * 12 : 0.75)));
		}
		for (const f of flies) {
			f.el.setAttribute("cx", num(f.x + Math.sin(t * 0.6 + f.phase) * 34));
			f.el.setAttribute("cy", num(f.y + Math.cos(t * 0.9 + f.phase) * 18));
			f.el.setAttribute("opacity", String(Math.max(0, Math.sin(t * 2.4 + f.phase)) * 0.85 * (1 - awake * 0.6)));
		}
		flash.setAttribute("opacity", String(between(t, 4.35, 4.42) * (1 - between(t, 4.42, 4.95)) * 0.34));
	}

	return { el: svg, seek };
}
