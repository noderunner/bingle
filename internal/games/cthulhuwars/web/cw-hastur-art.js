// Hastur, Him Who Is Not to Be Named, coming from the abyss between the stars: the map drops away, the stars are
// dragged round into a spiral, and from a point at its heart something vast comes on, a hood with the night
// in it, a ragged mantle and a trailing of arms. Original art, drawn in code (see js/cw-art-kit.js).

import { H, W, add, between, dot, limb, lerp, num, outCubic, randoms, round, sceneSvg, smooth, vertical, writhe } from "./cw-art-kit.js";
import { s } from "/static/js/util.js";

export const HASTUR_SECONDS = 7.2;
export const HASTUR_PEAK = 5.5;

const CX = 500;
const CY = 216;

export function hasturScene() {
	const rand = randoms(1893); // the year of Bierce's Hastur
	const svg = sceneSvg();
	svg.append(
		s(
			"defs",
			{},
			round("ha-void", [
				[0, "#0a0620"],
				[0.55, "#050312"],
				[1, "#010006"],
			]),
			round("ha-nebula-a", [
				[0, "#cf9b2a", 0.5],
				[0.5, "#8a3c86", 0.22],
				[1, "#3a1660", 0],
			]),
			round("ha-nebula-b", [
				[0, "#4a54c8", 0.42],
				[1, "#140a4a", 0],
			]),
			round("ha-heart", [
				[0, "#000", 1],
				[0.32, "#000", 1],
				[0.45, "#f6c845", 0.92],
				[0.62, "#b24a2a", 0.45],
				[1, "#3a1660", 0],
			]),
			vertical("ha-mantle", [
				[0, "#8a6a14"],
				[0.3, "#3c2c0a"],
				[0.7, "#120b08"],
				[1, "#05030c"],
			]),
			vertical("ha-robe", [
				[0, "#a07c18"],
				[0.5, "#3c2c0a"],
				[1, "#0a0604"],
			]),
			vertical("ha-arm", [
				[0, "#1a1206"],
				[0.5, "#6a4e10"],
				[1, "#201606"],
			]),
			round("ha-hood", [
				[0, "#080420"],
				[0.75, "#020010"],
				[1, "#000"],
			]),
			round("ha-eye", [
				[0, "#fffbe0"],
				[0.5, "#ffdc50"],
				[1, "#c17c10"],
			]),
			round("ha-eye-glow", [
				[0, "#ffe46a", 0.9],
				[0.35, "#e0a020", 0.38],
				[1, "#7a4d10", 0],
			]),
			s("clipPath", { id: "ha-hood-clip" }, s("path", { d: "M456 138C452 92 474 66 500 64C526 66 548 92 544 138C540 176 522 198 500 202C478 198 460 176 456 138Z" }))
		)
	);
	const space = add(svg, "g", { opacity: 0 });
	add(space, "rect", { x: 0, y: 0, width: W, height: H, fill: "url(#ha-void)" });
	const nebulae = [
		[CX - 180, CY - 40, 420, 240, "ha-nebula-a"],
		[CX + 230, CY + 50, 380, 260, "ha-nebula-b"],
		[CX + 40, CY - 120, 320, 180, "ha-nebula-a"],
		[CX - 260, CY + 130, 300, 200, "ha-nebula-b"],
	].map(([cx, cy, rx, ry, fill]) => add(space, "ellipse", { cx, cy, rx, ry, fill: `url(#${fill})` }));

	// The stars, each going round, and falling in.
	const starField = add(space, "g", { fill: "#fff" });
	const stars = [];
	for (let i = 0; i < 260; i++) {
		const big = i % 19 === 0;
		stars.push({
			el: add(starField, "line", { stroke: i % 7 === 0 ? "#ffe29a" : i % 5 === 0 ? "#a9b6ff" : "#fff", "stroke-width": big ? 2.2 : 0.8 + rand() * 0.9, "stroke-linecap": "round" }),
			radius: 40 + rand() ** 0.7 * 720,
			angle: rand() * 6.283,
			rate: 0.3 + rand() * 0.5,
		});
	}
	// The accretion: rings of gold going round the point it comes from.
	const rings = [];
	for (let i = 0; i < 7; i++) {
		rings.push({ el: add(space, "ellipse", { cx: CX, cy: CY, fill: "none", stroke: i % 2 ? "#f6c845" : "#e07a2a", "stroke-width": 1 + rand() * 1.6, "stroke-dasharray": `${num(30 + rand() * 90)} ${num(20 + rand() * 60)} ${num(8 + rand() * 30)} ${num(60 + rand() * 90)}` }), base: 70 + i * 34, rate: (i % 2 ? 1 : -1) * (40 + rand() * 50) });
	}
	const heart = add(space, "ellipse", { cx: CX, cy: CY, rx: 1, ry: 1, fill: "url(#ha-heart)" });

	// He, all of him drawn at his full size, and scaled up from a point.
	const figure = add(svg, "g", {});
	const defs = svg.querySelector("defs");
	const mantles = [0, 1].map((side) => {
		const clipPath = s("path", {});
		defs.append(s("clipPath", { id: `ha-mantle-clip-${side}` }, clipPath));
		const skin = add(figure, "path", { fill: "url(#ha-mantle)", stroke: "#a07c18", "stroke-width": 1 });
		const inside = add(figure, "g", { "clip-path": `url(#ha-mantle-clip-${side})` });
		let specks = "";
		for (let i = 0; i < 170; i++) {
			const x = side ? 500 + rand() * 520 : rand() * 520 - 20;
			specks += dot(x, rand() * 490, 0.4 + rand() ** 3 * 1.7);
		}
		add(inside, "path", { d: specks, fill: "#ffeec0", opacity: 0.72 });
		const folds = add(figure, "path", { fill: "none", stroke: "#010006", "stroke-width": 2.6, opacity: 0.6 });
		const rim = add(figure, "path", { fill: "none", stroke: "#ffe9a0", "stroke-width": 1.4, opacity: 0.6 });
		return { clipPath, skin, folds, rim };
	});
	add(figure, "path", { d: "M444 212C456 186 478 176 500 176C522 176 544 186 556 212C564 262 562 310 548 352C530 368 470 368 452 352C438 310 436 262 444 212Z", fill: "url(#ha-robe)", stroke: "#2a1c05", "stroke-width": 1.4 });
	add(figure, "path", { d: "M500 190V360M478 196C470 252 468 310 472 356M522 196C530 252 532 310 528 356", fill: "none", stroke: "#3b2a08", "stroke-width": 2.2, opacity: 0.7 });
	const arms = [];
	const armLayer = add(figure, "g", {});
	for (let i = 0; i < 15; i++) {
		const across = (i - 7) / 7;
		const el = add(armLayer, "path", { fill: "url(#ha-arm)", stroke: "#1a1104", "stroke-width": 0.9 });
		const marks = add(armLayer, "path", { fill: "#fff2c0", opacity: 0.8 });
		arms.push({ el, marks, root: [500 + across * 54, 286 + (1 - Math.abs(across)) * 26], lean: across * 1.05, length: 240 + rand() * 170, width: 27 - Math.abs(across) * 7, phase: rand() * 6.28, rate: 0.9 + rand() * 0.8 });
	}
	// The hood, and the night inside it.
	add(figure, "path", { d: "M424 196C414 122 446 54 500 24C554 54 586 122 576 196C554 216 446 216 424 196Z", fill: "url(#ha-robe)", stroke: "#2a1c05", "stroke-width": 1.6 });
	add(figure, "path", { d: "M500 30C486 56 476 80 472 100M500 30C514 56 524 80 528 100M450 76C440 112 438 150 442 184M550 76C560 112 562 150 558 184", fill: "none", stroke: "#3b2a08", "stroke-width": 2.2, opacity: 0.65 });
	add(figure, "path", { d: "M456 138C452 92 474 66 500 64C526 66 548 92 544 138C540 176 522 198 500 202C478 198 460 176 456 138Z", fill: "url(#ha-hood)", stroke: "#2a1c05", "stroke-width": 1.4 });
	const hoodStars = add(figure, "g", { "clip-path": "url(#ha-hood-clip)" });
	let inHood = "";
	for (let i = 0; i < 46; i++) inHood += dot(456 + rand() * 88, 66 + rand() * 136, 0.5 + rand() * 1);
	add(hoodStars, "path", { d: inHood, fill: "#ffe9b0", opacity: 0.85 });
	// Five eyes that do not match, in the dark of it.
	const eyes = [
		[482, 118, 7.5],
		[518, 118, 7.5],
		[500, 98, 5.5],
		[470, 148, 4.5],
		[530, 148, 4.5],
	].map(([cx, cy, rx]) => ({
		glow: add(figure, "circle", { cx, cy, r: rx * 4.8, fill: "url(#ha-eye-glow)", opacity: 0 }),
		ball: add(figure, "ellipse", { cx, cy, rx, ry: 0.3, fill: "url(#ha-eye)" }),
		slit: add(figure, "ellipse", { cx, cy, rx: rx * 0.17, ry: 0.3, fill: "#100800" }),
		size: rx,
	}));

	const wave = add(svg, "circle", { cx: CX, cy: CY, r: 10, fill: "none", stroke: "#ffe9a0", "stroke-width": 7, opacity: 0 });
	const wave2 = add(svg, "circle", { cx: CX, cy: CY, r: 10, fill: "none", stroke: "#e07a2a", "stroke-width": 3, opacity: 0 });
	const flash = add(svg, "rect", { x: 0, y: 0, width: W, height: H, fill: "#fff0b8", opacity: 0 });

	function seek(t) {
		const fade = 1 - between(t, HASTUR_SECONDS - 0.8, HASTUR_SECONDS);
		const fall = smooth(between(t, 0.1, 1.4));
		const pull = smooth(between(t, 0.6, 3.4)); // how hard the stars are dragged round
		const come = between(t, 2, 4.75);
		const near = come ** 3 * (0.0 + 1); // from very far, then all at once
		const open = smooth(between(t, 3.5, 4.8));
		const awake = smooth(between(t, 4.4, 4.75));

		svg.style.opacity = String(fade);
		space.setAttribute("opacity", String(fall));
		nebulae.forEach((n, i) => n.setAttribute("transform", `rotate(${num((t * (i % 2 ? -3 : 4) * (0.4 + pull)) % 360)} ${CX} ${CY})`));

		for (const st of stars) {
			const a = st.angle + t * st.rate * (0.1 + pull * 0.9) * (120 / (st.radius + 80));
			// They wind in towards the point, and come back round from the edge.
			const r = Math.max(14, st.radius * (1 - pull * 0.28) + Math.sin(t + st.angle) * 3);
			const x = CX + Math.cos(a) * r;
			const y = CY + Math.sin(a) * r * 0.62;
			const tail = (0.015 + pull * 0.17) * (120 / (st.radius + 80)) * (1 + st.rate);
			st.el.setAttribute("x1", num(x));
			st.el.setAttribute("y1", num(y));
			st.el.setAttribute("x2", num(CX + Math.cos(a - tail) * r));
			st.el.setAttribute("y2", num(CY + Math.sin(a - tail) * r * 0.62));
		}
		for (const ring of rings) {
			const size = ring.base * (0.5 + pull * 0.8);
			ring.el.setAttribute("rx", num(size * 1.5));
			ring.el.setAttribute("ry", num(size * 0.56));
			ring.el.setAttribute("stroke-dashoffset", num(t * ring.rate));
			ring.el.setAttribute("opacity", String(pull * (1 - awake * 0.55) * 0.62));
			ring.el.setAttribute("transform", `rotate(-14 ${CX} ${CY})`);
		}
		const heartSize = 40 + pull * 120 + awake * 60;
		heart.setAttribute("rx", num(heartSize * 1.5));
		heart.setAttribute("ry", num(heartSize));
		heart.setAttribute("opacity", String(pull));

		// He comes from a point in the middle of it.
		const scale = lerp(0.008, 1, near);
		const heave = Math.sin(t * 0.95) * 4 * awake;
		figure.setAttribute("transform", `translate(${CX} ${num(CY + heave)}) scale(${scale.toFixed(4)}) translate(${-CX} ${-CY + 2})`);
		figure.setAttribute("opacity", String(between(t, 2, 2.4)));

		// The mantle spreads from behind his shoulders, in rags.
		mantles.forEach((m, side) => {
			const mx = (x) => (side ? W - x : x);
			const beat = Math.sin(t * 0.9) * 10 * open;
			const at = (x, y) => [mx(lerp(440, x, open)), lerp(230, y + beat, open)];
			const pts = [at(352, 34), at(232, 74), at(116, 166), at(46, 306), at(70, 438), at(196, 488), at(330, 440)];
			const root = [mx(440), 212];
			let edge = `M${num(root[0])} ${num(root[1])}Q${num(mx(lerp(440, 410, open)))} ${num(lerp(190, 96, open))} ${num(pts[0][0])} ${num(pts[0][1])}Q${num(mx(lerp(440, 300, open)))} ${num(lerp(230, 42, open))} ${num(pts[1][0])} ${num(pts[1][1])}`;
			const dir = side ? -1 : 1;
			for (let i = 2; i < pts.length; i++) {
				const [ax, ay] = pts[i - 1];
				const [bx, by] = pts[i];
				for (let j = 1; j <= 3; j++) {
					const k = j / 3;
					const x = lerp(ax, bx, k);
					const y = lerp(ay, by, k);
					const dig = (j === 3 ? 0 : 1) * (26 + ((i * 7 + j * 5) % 3) * 24) * open;
					edge += `Q${num(x + dir * dig * 0.3)} ${num(y - dig * 0.9)} ${num(x + dir * dig)} ${num(y - dig * 0.25)}Q${num(x + dir * dig * 0.2)} ${num(y + dig * 0.2)} ${num(x)} ${num(y)}`;
				}
			}
			const whole = `${edge}L${num(mx(430))} ${num(350)}Z`;
			m.skin.setAttribute("d", whole);
			m.clipPath.setAttribute("d", whole);
			let folds = "";
			for (let i = 1; i < pts.length; i++) folds += `M${num(root[0])} ${num(root[1])}Q${num(lerp(root[0], pts[i][0], 0.55) + dir * 12)} ${num(lerp(root[1], pts[i][1], 0.55) - 16)} ${num(lerp(root[0], pts[i][0], 0.86))} ${num(lerp(root[1], pts[i][1], 0.86))}`;
			m.folds.setAttribute("d", folds);
			m.rim.setAttribute("d", `M${num(root[0])} ${num(root[1])}Q${num(mx(lerp(440, 410, open)))} ${num(lerp(190, 96, open))} ${num(pts[0][0])} ${num(pts[0][1])}Q${num(mx(lerp(440, 300, open)))} ${num(lerp(230, 42, open))} ${num(pts[1][0])} ${num(pts[1][1])}L${num(pts[2][0])} ${num(pts[2][1])}`);
		});

		for (const a of arms) {
			const points = writhe(a.root, a.lean, a.length * (0.3 + open * 0.7), 15, t, a.rate, a.phase, 0.6 + awake * 0.2);
			a.el.setAttribute("d", limb(points, (k) => lerp(a.width, 1.5, k ** 0.8)));
			let marks = "";
			for (let i = 2; i < points.length - 1; i += 2) marks += dot(points[i][0], points[i][1], 0.9 * (1 - i / points.length) + 0.5);
			a.marks.setAttribute("d", marks);
		}

		for (const e of eyes) {
			e.ball.setAttribute("ry", num(0.3 + awake * e.size * 0.78));
			e.slit.setAttribute("ry", num(0.3 + awake * e.size * 0.72));
			e.glow.setAttribute("opacity", String(awake * (0.82 + Math.sin(t * 6 + e.size) * 0.18)));
		}

		const blast = between(t, 4.4, 5.5);
		wave.setAttribute("r", num(10 + outCubic(blast) * 720));
		wave.setAttribute("opacity", String(blast > 0 && blast < 1 ? (1 - blast) * 0.8 : 0));
		const blast2 = between(t, 4.55, 5.8);
		wave2.setAttribute("r", num(10 + outCubic(blast2) * 640));
		wave2.setAttribute("opacity", String(blast2 > 0 && blast2 < 1 ? (1 - blast2) * 0.7 : 0));
		flash.setAttribute("opacity", String(between(t, 4.38, 4.45) * (1 - between(t, 4.45, 4.95)) * 0.48));
	}

	return { el: svg, seek };
}
