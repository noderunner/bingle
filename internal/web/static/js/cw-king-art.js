// The King in Yellow, rising through the trap of a rotting theatre: the curtains part, the footlights
// catch, and he stands in tatters and a pale mask with the Yellow Sign burning behind him. He has
// always been in the play. Original art, drawn in code (see js/cw-art-kit.js).

import { H, W, add, between, curve, dot, limb, lerp, num, outCubic, randoms, round, sceneSvg, smooth, vertical, writhe } from "./cw-art-kit.js";
import { s } from "./util.js";

export const KING_SECONDS = 7.2;
export const KING_PEAK = 5.4;

const FLOOR = 372; // where the boards of the stage meet the backdrop

/** A curtain's outline, gathered towards its outer edge as it opens. */
function curtain(side, open, t) {
	const inner = lerp(side ? 500 : 500, side ? 500 + open * 395 : 500 - open * 395, 1);
	const outer = side ? W + 20 : -20;
	let d = `M${outer} -10L${num(inner)} -10`;
	const hem = 438 - open * 34;
	const steps = 7;
	for (let i = 0; i <= steps; i++) {
		const k = i / steps;
		d += `L${num(inner + Math.sin(k * 7 + t * 1.3 + side) * (5 + k * 8))} ${num(lerp(-10, hem - k * k * 36 * open, k))}`;
	}
	// The hem has rotted away in places.
	const width = outer - inner;
	const rags = 12;
	for (let i = 1; i <= rags; i++) {
		const k = i / rags;
		const drop = (i % 2 ? 18 : -12) + Math.sin(i * 2.7) * 12;
		d += `L${num(inner + width * k)} ${num(hem + drop - (1 - k) * 36 * open + Math.sin(t * 1.6 + i) * 3)}`;
	}
	return { outline: `${d}Z`, inner, outer, hem };
}

export function kingScene() {
	const rand = randoms(1895); // the year of The King in Yellow
	const svg = sceneSvg();
	svg.append(
		s(
			"defs",
			{},
			vertical("ky-dark", [
				[0, "#050402", 0.97],
				[1, "#0d0a04", 0.92],
			]),
			vertical("ky-backdrop", [
				[0, "#17100a"],
				[0.55, "#3a2710"],
				[1, "#1a1007"],
			]),
			vertical("ky-boards", [
				[0, "#3b2811"],
				[1, "#0f0a04"],
			]),
			vertical("ky-curtain", [
				[0, "#8a6a10"],
				[0.5, "#6a4e0a"],
				[1, "#2c1f05"],
			]),
			vertical("ky-robe", [
				[0, "#a07d16"],
				[0.4, "#5c440b"],
				[0.8, "#1f1604"],
				[1, "#0c0802"],
			]),
			vertical("ky-rag", [
				[0, "#8c6c12"],
				[0.55, "#5a430b"],
				[1, "#160f03"],
			]),
			vertical("ky-frame", [
				[0, "#4c3a12"],
				[1, "#1a1206"],
			]),
			vertical("ky-spot", [
				[0, "#f6e39a", 0.2],
				[1, "#f6e39a", 0.02],
			]),
			round("ky-sun", [
				[0, "#f9eeb4"],
				[0.55, "#d7a93a", 0.75],
				[1, "#7a4d10", 0],
			]),
			round("ky-sign-glow", [
				[0, "#ffe45e", 0.75],
				[0.4, "#d6a11d", 0.32],
				[1, "#7a4d10", 0],
			]),
			round("ky-foot", [
				[0, "#fff3b0", 0.95],
				[0.35, "#f5c94a", 0.45],
				[1, "#c98a14", 0],
			]),
			round("ky-mask", [
				[0, "#f7f2dc"],
				[0.7, "#d9d2b2"],
				[1, "#938b66"],
			], "0.45", "0.35", "0.7")
		)
	);

	// The dark falls over the map, and the theatre is there.
	const house = add(svg, "g", { opacity: 0 });
	add(house, "rect", { x: 0, y: 0, width: W, height: H, fill: "url(#ky-dark)" });

	// The painted backdrop: Carcosa, its twin suns low, black stars and impossible towers.
	const backdrop = add(house, "g", {});
	add(backdrop, "rect", { x: 60, y: 30, width: 880, height: FLOOR - 30, fill: "url(#ky-backdrop)" });
	const suns = [
		[330, 150, 74],
		[664, 196, 48],
	].map(([cx, cy, r]) => add(backdrop, "circle", { cx, cy, r, fill: "url(#ky-sun)", opacity: 0.8 }));
	let stars = "";
	for (let i = 0; i < 26; i++) {
		const x = 90 + rand() * 820;
		const y = 42 + rand() * 96;
		const r = 1.5 + rand() * 2.2;
		stars += `M${num(x)} ${num(y - r * 2)}L${num(x + r)} ${num(y)}L${num(x)} ${num(y + r * 2)}L${num(x - r)} ${num(y)}Z`;
	}
	add(backdrop, "path", { d: stars, fill: "#070402" });
	add(backdrop, "path", {
		d: "M60 372L60 300L104 300L118 214L146 214L156 288L206 288L222 170L238 118L256 172L268 300L330 300L344 256L392 256L404 322L596 322L606 240L652 240L666 306L726 306L742 150L762 96L778 156L792 292L846 292L858 226L892 226L904 300L940 300L940 372Z",
		fill: "#0b0704",
		stroke: "#5c431a",
		"stroke-width": 1,
	});

	// A cloud of dust where the Sign burns, then the Sign itself: three arms turning from one eye.
	const signGlow = add(house, "circle", { cx: 500, cy: 190, r: 300, fill: "url(#ky-sign-glow)", opacity: 0 });
	const sign = add(house, "g", { opacity: 0 });
	const signArms = [0, 120, 240].map((turn) => {
		const a = (turn * Math.PI) / 180;
		const point = (r, off) => [500 + Math.sin(a + off) * r, 190 - Math.cos(a + off) * r];
		return limb(curve(point(60, -0.25), point(190, -0.75), point(292, 0.42), 16), (k) => lerp(34, 4, k));
	});
	add(sign, "path", { d: signArms.join(""), fill: "#ffd83d", stroke: "#fff3a8", "stroke-width": 1 });
	add(sign, "path", { d: dot(500, 190, 22), fill: "#ffd83d" });

	// The stage: boards in perspective.
	const boards = add(house, "g", {});
	add(boards, "path", { d: `M60 ${FLOOR}L940 ${FLOOR}L${W + 90} ${H + 20}L-90 ${H + 20}Z`, fill: "url(#ky-boards)" });
	let planks = "";
	for (let i = -8; i <= 8; i++) planks += `M${500 + i * 55} ${FLOOR}L${500 + i * 130} ${H + 20}`;
	for (const y of [395, 425, 465]) planks += `M-20 ${y}H${W + 20}`;
	add(boards, "path", { d: planks, fill: "none", stroke: "#0a0602", "stroke-width": 1.4, opacity: 0.8 });
	// The trap he comes through, and what comes up with him.
	const trap = add(boards, "ellipse", { cx: 500, cy: 412, rx: 0, ry: 0, fill: "#020100", stroke: "#6b4e16", "stroke-width": 2 });
	const smoke = [];
	for (let i = 0; i < 16; i++) {
		smoke.push({ el: add(house, "circle", { fill: "#c3a862", opacity: 0 }), x: 500 + (rand() - 0.5) * 130, phase: rand(), r: 22 + rand() * 40, drift: (rand() - 0.5) * 80 });
	}

	// The King.
	const king = add(house, "g", {});
	const backRags = add(king, "g", {});
	const rags = [];
	for (let i = 0; i < 21; i++) {
		const across = (i - 10) / 10;
		rags.push({
			root: [500 + across * 72, 172 + Math.abs(across) * 34],
			lean: across * 0.34,
			length: 186 + rand() * 90,
			width: 14 + rand() * 15,
			phase: rand() * 6.28,
			rate: 1 + rand() * 0.8,
			el: add(backRags, "path", { fill: "url(#ky-rag)", stroke: "#2c1f07", "stroke-width": 0.9 }),
		});
	}
	const sleeves = [0, 1].map(() => ({
		cloth: add(king, "path", { fill: "#6a4f0d", stroke: "#2c1f07", "stroke-width": 1 }),
		tatter: add(king, "path", { fill: "url(#ky-rag)", stroke: "#2c1f07", "stroke-width": 0.8 }),
		hand: add(king, "path", { fill: "#cfc7a4", stroke: "#3b3419", "stroke-width": 0.8 }),
	}));
	add(king, "path", { d: "M452 196C456 168 474 152 500 150C526 152 544 168 548 196C552 240 546 286 558 340L546 392L530 352L516 404L500 360L484 410L470 352L454 396L442 340C454 286 448 240 452 196Z", fill: "url(#ky-robe)", stroke: "#2c1f07", "stroke-width": 1.4 });
	add(king, "path", { d: "M500 156V350M478 170C474 230 470 290 462 346M522 170C526 230 530 290 538 346", fill: "none", stroke: "#3b2a08", "stroke-width": 2.2, opacity: 0.7 });
	// The cowl, drawn up to a point, and the crown of jags it holds.
	add(king, "path", { d: "M452 170C446 112 468 58 500 22C532 58 554 112 548 170C528 158 472 158 452 170Z", fill: "url(#ky-robe)", stroke: "#2c1f07", "stroke-width": 1.4 });
	add(king, "path", { d: "M500 30C492 64 488 94 490 118M500 30C508 64 512 94 510 118", fill: "none", stroke: "#3b2a08", "stroke-width": 2, opacity: 0.65 });
	add(king, "path", { d: "M470 104C468 136 478 166 500 174C522 166 532 136 530 104C520 90 480 90 470 104Z", fill: "#080500" });
	// The pale mask. Nothing at all shows behind it.
	add(king, "path", { d: "M482 106C480 134 488 164 500 174C512 164 520 134 518 106C510 97 490 97 482 106Z", fill: "url(#ky-mask)", stroke: "#4b4426", "stroke-width": 1 });
	const slits = [
		[491, 124, 12],
		[509, 124, -12],
	].map(([x, y, tilt]) => ({
		glow: add(king, "circle", { cx: x, cy: y, r: 13, fill: "url(#ky-sign-glow)", opacity: 0 }),
		eye: add(king, "ellipse", { cx: x, cy: y, rx: 5, ry: 1.2, fill: "#050300", transform: `rotate(${tilt} ${x} ${y})` }),
	}));

	// In front of him: the lights and the frame.
	const spots = [
		[240, "M180 -10L320 -10L640 408L400 408Z"],
		[760, "M680 -10L820 -10L600 408L360 408Z"],
	].map(([, d]) => add(house, "path", { d, fill: "url(#ky-spot)", opacity: 0 }));
	const footGlow = add(house, "g", { opacity: 0 });
	for (let i = 0; i < 12; i++) {
		const x = 80 + (i * 840) / 11;
		add(footGlow, "ellipse", { cx: x, cy: 456, rx: 62, ry: 30, fill: "url(#ky-foot)" });
		add(footGlow, "path", { d: `M${x - 16} 470Q${x} 446 ${x + 16} 470Z`, fill: "#1a1206", stroke: "#6b4e16", "stroke-width": 1 });
	}
	const dust = add(house, "g", { fill: "#f4e4a0" });
	const motes = [];
	for (let i = 0; i < 46; i++) motes.push({ el: add(dust, "circle", { r: 0.5 + rand() * 1.4, opacity: 0 }), x: 200 + rand() * 600, y: rand() * 400, rate: 3 + rand() * 7, phase: rand() * 6.28 });

	const curtains = [0, 1].map((side) => ({
		side,
		cloth: add(house, "path", { fill: "url(#ky-curtain)", stroke: "#2a1d05", "stroke-width": 1.4 }),
		folds: add(house, "path", { fill: "none", stroke: "#241703", "stroke-width": 3.4, opacity: 0.75 }),
		shine: add(house, "path", { fill: "none", stroke: "#c9a227", "stroke-width": 1.4, opacity: 0.35 }),
	}));
	// The frame of the stage, with a valance gnawed along its edge.
	add(house, "path", { d: `M-20 -20H${W + 20}V50C940 66 900 44 860 62C820 80 780 44 740 58C700 72 660 48 620 62C580 76 540 44 500 60C460 76 420 44 380 62C340 80 300 48 260 60C220 72 180 44 140 62C100 80 60 44 -20 56Z`, fill: "url(#ky-frame)", stroke: "#8a6a10", "stroke-width": 1.6 });
	add(house, "path", { d: "M-20 -20H62V520H-20ZM938 -20H1020V520H938Z", fill: "url(#ky-frame)", stroke: "#6b4e16", "stroke-width": 1.4 });
	const flash = add(svg, "rect", { x: 0, y: 0, width: W, height: H, fill: "#fff3b0", opacity: 0 });

	function seek(t) {
		const fade = 1 - between(t, KING_SECONDS - 0.8, KING_SECONDS);
		const dark = smooth(between(t, 0.15, 1.1));
		const open = smooth(between(t, 1, 3));
		const lights = smooth(between(t, 0.8, 1.7)) * (0.86 + Math.sin(t * 13) * 0.05 + Math.sin(t * 29) * 0.04);
		const rise = outCubic(between(t, 2.3, 4.7));
		const raise = smooth(between(t, 3.4, 4.7));
		const awake = smooth(between(t, 4.35, 4.7));
		const trapOpen = smooth(between(t, 1.9, 2.5));

		svg.style.opacity = String(fade);
		house.setAttribute("opacity", String(dark));
		footGlow.setAttribute("opacity", String(lights));
		for (const spot of spots) spot.setAttribute("opacity", String(smooth(between(t, 2, 3)) * (0.72 + Math.sin(t * 9) * 0.1)));
		for (const sun of suns) sun.setAttribute("opacity", String(0.45 + open * 0.45));

		for (const c of curtains) {
			const shape = curtain(c.side, open, t);
			c.cloth.setAttribute("d", shape.outline);
			let folds = "";
			let shine = "";
			for (let i = 1; i < 11; i++) {
				const k = i / 11;
				const x = lerp(shape.inner, shape.outer, k * k);
				folds += `M${num(x)} -10Q${num(x + Math.sin(t + i) * 8)} 200 ${num(x + Math.sin(i) * 7)} ${num(shape.hem - 6)}`;
				shine += `M${num(x + 8)} 20Q${num(x + 10 + Math.sin(t + i) * 8)} 200 ${num(x + 8 + Math.sin(i) * 7)} ${num(shape.hem - 30)}`;
			}
			c.folds.setAttribute("d", folds);
			c.shine.setAttribute("d", shine);
		}

		trap.setAttribute("rx", num(trapOpen * 92));
		trap.setAttribute("ry", num(trapOpen * 15));
		// He comes up from below the boards, and stays cut off by them until he is through.
		const up = (1 - rise) * 360 + Math.sin(t * 1.1) * 2.5 * rise;
		king.setAttribute("transform", `translate(0 ${num(up)})`);
			const figureOpacity = between(t, 2.3, 2.7);
		king.setAttribute("opacity", String(figureOpacity));

		for (const r of rags) r.el.setAttribute("d", limb(writhe(r.root, r.lean, r.length, 12, t, r.rate, r.phase, 0.16 + awake * 0.1), (k) => lerp(r.width, 2.4, k ** 1.3)));

		sleeves.forEach((sleeve, side) => {
			const mx = (x) => (side ? W - x : x);
			const shoulder = [mx(454), 196];
			const elbow = [mx(lerp(428, 352, raise)), lerp(290, 214, raise)];
			const sway = Math.sin(t * 1.2 + side * 2) * 5;
			const wrist = [mx(lerp(420, 268, raise)) + sway, lerp(350, 122, raise) + sway];
			sleeve.cloth.setAttribute("d", limb(curve(shoulder, elbow, wrist, 12), (k) => lerp(34, 13, k ** 0.8)));
			const hang = [];
			for (let i = 0; i < 5; i++) {
				const k = [0.14, 0.32, 0.52, 0.7, 0.88][i];
				const root = [lerp(shoulder[0], wrist[0], k), lerp(shoulder[1], wrist[1], k) + 7];
				hang.push(limb(writhe(root, Math.sin(i * 2.3) * 0.2, [120, 70, 150, 92, 58][i], 9, t, 1.5, i * 1.9, 0.5), (q) => lerp([26, 18, 30, 20, 15][i], 1.5, q)));
			}
			sleeve.tatter.setAttribute("d", hang.join(""));
			let hand = "";
			for (let f = 0; f < 4; f++) {
				const dir = (side ? 1 : -1) * (0.15 + (f - 1.5) * 0.3 * (0.5 + raise * 0.5));
				const tip = [wrist[0] + Math.sin(dir) * 46, wrist[1] - Math.cos(dir) * 46];
				hand += limb(curve(wrist, [lerp(wrist[0], tip[0], 0.5) + (side ? -8 : 8), lerp(wrist[1], tip[1], 0.5)], tip, 5), (k) => lerp(8, 1.4, k));
			}
			sleeve.hand.setAttribute("d", hand);
		});

		// The Sign burns, and his mask looks back.
		const burn = awake * (0.92 + Math.sin(t * 7) * 0.08);
		sign.setAttribute("opacity", String(burn));
		sign.setAttribute("transform", `rotate(${num((t - 4.4) * 7)} 500 190)`);
		signGlow.setAttribute("opacity", String(smooth(between(t, 3.3, 4.6)) * 0.72 + burn * 0.28));
		for (const e of slits) {
			e.glow.setAttribute("opacity", String(awake * (0.85 + Math.sin(t * 6) * 0.15)));
			e.eye.setAttribute("fill", awake > 0.5 ? "#ffe45e" : "#050300");
		}

		for (const puff of smoke) {
			const age = ((t * 0.34 + puff.phase) % 1) * between(t, 1.9, 2.4);
			puff.el.setAttribute("cx", num(puff.x + puff.drift * age));
			puff.el.setAttribute("cy", num(412 - age * 170));
			puff.el.setAttribute("r", num(puff.r * (0.4 + age)));
			puff.el.setAttribute("opacity", String(0.17 * Math.sin(age * Math.PI) * between(t, 1.9, 2.4)));
		}
		for (const m of motes) {
			m.el.setAttribute("cx", num(m.x + Math.sin(t * 0.8 + m.phase) * 16));
			m.el.setAttribute("cy", num(((m.y - t * m.rate) % 400) + (m.y - t * m.rate < 0 ? 400 : 0)));
			m.el.setAttribute("opacity", String(open * (0.32 + Math.sin(t * 2 + m.phase) * 0.28)));
		}
		flash.setAttribute("opacity", String(between(t, 4.35, 4.45) * (1 - between(t, 4.45, 4.95)) * 0.5));
	}

	return { el: svg, seek };
}
