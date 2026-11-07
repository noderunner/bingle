// Nyarlathotep, the Crawling Chaos, condensing out of the shadows: the dark comes in over the map from its
// edges, pale eyes open in it, and it draws itself up into a tall black pharaoh with no face, and a three-lobed
// eye that burns. Original art, drawn in code (see js/cw-art-kit.js).

import { H, W, add, between, curve, limb, lerp, num, outCubic, randoms, round, sceneSvg, smooth, vertical, writhe } from "./cw-art-kit.js";
import { s } from "/static/js/util.js";

export const NYARLATHOTEP_SECONDS = 7.2;
export const NYARLATHOTEP_PEAK = 5.4;

const HEAD = "M464 138C462 96 476 66 500 64C524 66 538 96 536 138C534 164 520 184 500 188C480 184 466 164 464 138Z";
// A pharaoh's headcloth, its wings falling to the shoulders.
const CLOTH = "M500 24C454 28 432 62 428 104C424 138 416 178 396 226C420 234 444 226 462 206C474 196 526 196 538 206C556 226 580 234 604 226C584 178 576 138 572 104C568 62 546 28 500 24Z";
const BODY = "M396 228C424 214 462 208 500 208C538 208 576 214 604 228C602 270 572 296 562 340C556 372 590 440 640 540L360 540C410 440 444 372 438 340C428 296 398 270 396 228Z";
// The tall crown, far taller than any crown should stand.
const CROWN = "M500 -46C478 -40 462 -10 458 34C468 50 532 50 542 34C538 -10 522 -40 500 -46Z";

export function nyarlathotepScene() {
	const rand = randoms(1920); // the year of the prose poem
	const svg = sceneSvg();
	svg.append(
		s(
			"defs",
			{},
			round("ny-aura", [
				[0, "#5b6cff", 0.82],
				[0.4, "#3a2a9c", 0.42],
				[1, "#120a40", 0],
			]),
			round("ny-pool", [
				[0, "#000", 1],
				[0.75, "#02010a", 0.95],
				[1, "#02010a", 0],
			]),
			vertical("ny-robe", [
				[0, "#1a1a48"],
				[0.45, "#0a0924"],
				[1, "#02010a"],
			]),
			vertical("ny-tendril", [
				[0, "#02010a"],
				[1, "#0a0820"],
			]),
			vertical("ny-cloth", [
				[0, "#14173e"],
				[1, "#060514"],
			]),
			round("ny-fire", [
				[0, "#fff4c0"],
				[0.35, "#ff8a2a"],
				[0.75, "#c81e10", 0.85],
				[1, "#7a0a06", 0],
			]),
			round("ny-eye-glow", [
				[0, "#ff5a2a", 0.9],
				[0.35, "#c81e10", 0.4],
				[1, "#7a0a06", 0],
			]),
			round("ny-small-eye", [
				[0, "#fff6c8"],
				[0.6, "#e8c95a"],
				[1, "#7a5a10"],
			])
		)
	);
	const veil = add(svg, "rect", { x: 0, y: 0, width: W, height: H, fill: "#02010a", opacity: 0 });

	// The dark comes in from all round: thick arms of it, reaching for the middle.
	const reach = add(svg, "g", {});
	const arms = [];
	for (let i = 0; i < 44; i++) {
		const edge = i % 4;
		const along = rand();
		const root = edge === 0 ? [along * W, -20] : edge === 1 ? [along * W, H + 20] : edge === 2 ? [-20, along * H] : [W + 20, along * H];
		const to = Math.atan2(500 - root[0], 240 - root[1]);
		arms.push({ root, lean: to + (rand() - 0.5) * 0.7, length: 260 + rand() * 340, width: 30 + rand() * 62, phase: rand() * 6.28, rate: 0.9 + rand() * 0.9, start: 0.1 + rand() * 0.8, el: add(reach, "path", { fill: "url(#ny-tendril)", stroke: "#2c2a78", "stroke-width": 0.7 }) });
	}
	const aura = add(svg, "ellipse", { cx: 500, cy: 170, rx: 460, ry: 320, fill: "url(#ny-aura)", opacity: 0 });

	// Eyes, in the dark. A pair at a time.
	const watchers = [];
	const watch = add(svg, "g", {});
	for (let i = 0; i < 34; i++) {
		const side = i % 2 ? 1 : -1;
		const x = 500 + side * (150 + rand() * 340);
		const y = 30 + rand() * 430;
		const size = 3 + rand() * 4.2;
		const g = add(watch, "g", { transform: `translate(${num(x)} ${num(y)}) rotate(${num((rand() - 0.5) * 22)})`, opacity: 0 });
		const lids = [-1, 1].map((d) => add(g, "ellipse", { cx: d * size * 1.5, cy: 0, rx: size, ry: size * 0.55, fill: "url(#ny-small-eye)" }));
		add(g, "ellipse", { cx: -size * 1.5, cy: 0, rx: size * 0.16, ry: size * 0.5, fill: "#000" });
		add(g, "ellipse", { cx: size * 1.5, cy: 0, rx: size * 0.16, ry: size * 0.5, fill: "#000" });
		watchers.push({ g, lids, born: 1.5 + rand() * 2.8, phase: rand() * 6.28, rate: 0.4 + rand() * 0.5 });
	}

	// Behind him: what he has for a shadow, arms of it, standing up round him.
	const halo = add(svg, "g", {});
	const haloArms = [];
	for (let i = 0; i < 18; i++) {
		const a = -1.5 + (3 * i) / 17;
		haloArms.push({ angle: a, length: 240 + rand() * 170, width: 25 + rand() * 18, phase: rand() * 6.28, rate: 1 + rand() * 1, el: add(halo, "path", { fill: "#03020d", stroke: "#4c55d8", "stroke-width": 0.9 }) });
	}

	const figure = add(svg, "g", {});
	const limbs = [0, 1].map(() => ({
		cloth: add(figure, "path", { fill: "url(#ny-robe)", stroke: "#4c55d8", "stroke-width": 0.9 }),
		hand: add(figure, "path", { fill: "#04030f", stroke: "#6d78ea", "stroke-width": 0.7 }),
	}));
	add(figure, "path", { d: BODY, fill: "url(#ny-robe)", stroke: "#3b42b8", "stroke-width": 1 });
	// A broad collar, as a pharaoh has, of dull gold and lapis.
	add(figure, "path", { d: "M430 212C452 246 480 256 500 256C520 256 548 246 570 212C552 204 448 204 430 212Z", fill: "#0a0a22", stroke: "#a4852f", "stroke-width": 1.4 });
	add(figure, "path", { d: "M442 222C460 240 482 246 500 246C518 246 540 240 558 222M452 214C468 230 484 236 500 236C516 236 532 230 548 214", fill: "none", stroke: "#a4852f", "stroke-width": 1.1, opacity: 0.8 });
	add(figure, "path", { d: "M452 296C470 306 530 306 548 296M446 322C468 334 532 334 554 322M436 262C466 276 534 276 564 262", fill: "none", stroke: "#3b42b8", "stroke-width": 1.5, opacity: 0.7 });
	add(figure, "path", { d: CROWN, fill: "url(#ny-cloth)", stroke: "#6d78ea", "stroke-width": 1.2 });
	add(figure, "path", { d: "M500 -40V40M478 -14C474 8 472 22 472 40M522 -14C526 8 528 22 528 40", fill: "none", stroke: "#a4852f", "stroke-width": 1.2, opacity: 0.7 });
	add(figure, "path", { d: CLOTH, fill: "url(#ny-cloth)", stroke: "#3b42b8", "stroke-width": 1.2 });
	add(figure, "path", {
		d: "M500 26C484 34 472 50 466 72M500 26C516 34 528 50 534 72M460 44C446 62 440 88 438 116M540 44C554 62 560 88 562 116M446 66C434 96 432 128 422 170M554 66C566 96 568 128 578 170M436 106C432 140 424 178 410 214M564 106C568 140 576 178 590 214",
		fill: "none",
		stroke: "#a4852f",
		"stroke-width": 2.2,
		opacity: 0.85,
	});
	add(figure, "path", { d: HEAD, fill: "#000", stroke: "#1a1c50", "stroke-width": 1.2 });
	const rim = add(figure, "path", {
		d: "M428 104C432 62 454 28 500 24C546 28 568 62 572 104M396 226C416 178 424 138 428 104M604 226C584 178 576 138 572 104M396 228C398 270 428 296 438 340C444 372 410 440 360 540M604 228C602 270 572 296 562 340C556 372 590 440 640 540M458 34C462 -10 478 -40 500 -46C522 -40 538 -10 542 34",
		fill: "none",
		stroke: "#a8b4ff",
		"stroke-width": 1.8,
		opacity: 0.35,
	});
	// The three lobes, where a face would be.
	const eyeGlow = add(figure, "circle", { cx: 500, cy: 124, r: 88, fill: "url(#ny-eye-glow)", opacity: 0 });
	const lobes = add(figure, "g", {});
	const lobeShapes = [
		[500, 100, 9, 15],
		[484, 130, 10, 14],
		[516, 130, 10, 14],
	].map(([cx, cy, rx, ry], i) => add(lobes, "ellipse", { cx, cy, rx: 0.3, ry, fill: "url(#ny-fire)", transform: `rotate(${[0, -32, 32][i]} ${cx} ${cy})`, "data-rx": rx }));
	add(lobes, "path", { d: "M500 86L500 122M480 128L500 120L520 128", fill: "none", stroke: "#fff4c0", "stroke-width": 0.9, opacity: 0.6 });

	// At his feet the dark pools, and it hides how he comes to be there.
	const skirts = [];
	const skirtGroup = add(svg, "g", {});
	for (let i = 0; i < 24; i++) {
		const across = (i - 11.5) / 11.5;
		skirts.push({ root: [500 + across * 92, 380 + Math.abs(across) * 40], lean: across * 1.22, length: 150 + rand() * 180, width: 22 + rand() * 20, phase: rand() * 6.28, rate: 0.9 + rand() * 0.8, el: add(skirtGroup, "path", { fill: "#03020f" }) });
	}
	const pool = add(svg, "ellipse", { cx: 500, cy: 506, rx: 700, ry: 108, fill: "url(#ny-pool)", opacity: 0 });
	const smoke = add(svg, "g", { fill: "#6d78ea" });
	const motes = [];
	for (let i = 0; i < 60; i++) motes.push({ el: add(smoke, "circle", { r: 0.6 + rand() * 1.6, opacity: 0 }), x: rand() * W, y: rand() * H, phase: rand() });
	const flash = add(svg, "rect", { x: 0, y: 0, width: W, height: H, fill: "#ff5a2a", opacity: 0 });

	function seek(t) {
		const fade = 1 - between(t, NYARLATHOTEP_SECONDS - 0.8, NYARLATHOTEP_SECONDS);
		const dark = smooth(between(t, 0.2, 2.2));
		const form = outCubic(between(t, 1.7, 4.3));
		const spread = smooth(between(t, 3.1, 4.7));
		const awake = smooth(between(t, 4.35, 4.65));

		svg.style.opacity = String(fade);
		veil.setAttribute("opacity", String(dark * 0.97));
		for (const a of arms) {
			const grow = outCubic(between(t, a.start, a.start + 1.9));
			a.el.setAttribute("d", grow > 0 ? limb(writhe(a.root, a.lean, a.length * grow, 12, t, a.rate, a.phase, 0.42), (k) => lerp(a.width, 1.6, k ** 0.8)) : "");
			a.el.setAttribute("opacity", String(1 - between(t, 2.5, 4) * 0.55));
		}
		for (const w of watchers) {
			const on = between(t, w.born, w.born + 0.3);
			const blink = Math.sin(t * w.rate * 3 + w.phase) > 0.96 ? 0.1 : 1;
			w.g.setAttribute("opacity", String(on * (0.82 - awake * 0.3)));
			for (const lid of w.lids) lid.setAttribute("ry", num(+lid.getAttribute("rx") * 0.55 * on * blink));
		}
		aura.setAttribute("opacity", String(form * 0.33 + awake * (0.52 + Math.sin(t * 4) * 0.06)));

		// He draws up out of the pool, taller than he should be.
		const lift = (1 - form) * 300;
		const stretch = lerp(0.3, 1, form);
		const heave = Math.sin(t * 1.05) * 3 * awake;
		// All of him is drawn tall: at his full height the crown just clears the top of the picture.
		const place = `translate(500 ${num(500 + lift * 0.1 + heave)}) scale(${(lerp(0.66, 0.88, form)).toFixed(3)} ${(stretch * 0.88).toFixed(3)}) translate(-500 -500)`;
		figure.setAttribute("transform", place);
		figure.setAttribute("opacity", String(between(t, 1.7, 2.8)));
		halo.setAttribute("transform", place);
		halo.setAttribute("opacity", String(between(t, 2.4, 3.6)));

		for (const a of haloArms) {
			const dir = a.angle;
			const root = [500 + Math.sin(dir) * 66, 200 - Math.cos(dir) * 28];
			a.el.setAttribute("d", limb(writhe(root, Math.PI - dir, a.length * (0.25 + spread * 0.75), 12, t, a.rate, a.phase, 0.5 + awake * 0.2), (k) => lerp(a.width, 1.4, k ** 0.85)));
		}
		for (const sk of skirts) sk.el.setAttribute("d", limb(writhe(sk.root, sk.lean, sk.length * (0.35 + form * 0.65), 11, t, sk.rate, sk.phase, 0.5), (k) => lerp(sk.width, 1.6, k ** 0.8)));
		skirtGroup.setAttribute("opacity", String(between(t, 1.6, 2.6)));
		pool.setAttribute("opacity", String(smooth(between(t, 0.9, 2.2))));

		// Long arms, with long hands, opening.
		limbs.forEach((arm, side) => {
			const mx = (x) => (side ? W - x : x);
			const sway = Math.sin(t * 1.1 + side * 2.2) * 6;
			const shoulder = [mx(412), 236];
			const elbow = [mx(lerp(390, 268, spread)), lerp(330, 254, spread)];
			const wrist = [mx(lerp(382, 168, spread)) + sway, lerp(400, 140, spread) + sway * 0.7];
			arm.cloth.setAttribute("d", limb(curve(shoulder, elbow, wrist, 14), (k) => lerp(34, 11, k ** 0.8)));
			let hand = "";
			for (let f = 0; f < 5; f++) {
				const dir = (side ? 1 : -1) * (0.12 + (f - 2) * 0.27 * (0.4 + spread * 0.6));
				const len = f === 0 || f === 4 ? 56 : 78;
				const tip = [wrist[0] + Math.sin(dir) * len, wrist[1] - Math.cos(dir) * len];
				hand += limb(curve(wrist, [lerp(wrist[0], tip[0], 0.55) + (side ? -13 : 13), lerp(wrist[1], tip[1], 0.55) + 4], tip, 7), (k) => lerp(7.5, 0.6, k));
			}
			arm.hand.setAttribute("d", hand);
		});

		lobeShapes.forEach((lobe, i) => {
			const wide = +lobe.getAttribute("data-rx") * awake * (0.86 + Math.sin(t * 11 + i * 2) * 0.14);
			lobe.setAttribute("rx", num(Math.max(0.3, wide)));
		});
		eyeGlow.setAttribute("opacity", String(awake * (0.82 + Math.sin(t * 8) * 0.18)));
		rim.setAttribute("opacity", String(0.18 + form * 0.2 + awake * 0.35));

		// Motes of it are drawn in to him.
		for (const m of motes) {
			const k = (t * 0.28 + m.phase) % 1;
			m.el.setAttribute("cx", num(lerp(m.x, 500, k)));
			m.el.setAttribute("cy", num(lerp(m.y, 190, k)));
			m.el.setAttribute("opacity", String(between(t, 1, 2) * Math.sin(k * Math.PI) * 0.7));
		}
		flash.setAttribute("opacity", String(between(t, 4.35, 4.42) * (1 - between(t, 4.42, 4.95)) * 0.3));
	}

	return { el: svg, seek };
}
