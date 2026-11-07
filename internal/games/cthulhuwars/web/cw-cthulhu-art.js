// Great Cthulhu rising from the sea: original art, drawn entirely in code. A scene is an SVG and a
// seek(t) that draws it as it stands t seconds in, so one clock drives everything, nothing is random
// from one showing to the next, and a still picture is only seek() at one moment.
//
// He is lit from behind: a dark mass, a rim of cold light, wet highlights, and two eyes. His long
// shapes (tentacles, arms, fingers, wing bones) are spines with a varying width, recomputed each
// frame, which is what lets them writhe.

import { s } from "/static/js/util.js";
import { H, W, between, curve, limb, lerp, num, outCubic, randoms, scales, smooth, stop } from "./cw-art-kit.js";

/** How long the scene lasts, and the moment it looks its best. */
export const CTHULHU_SECONDS = 7.2;
export const CTHULHU_PEAK = 5.4;

/** A band of storm sea: sharp crests, with chop on them, closed along the foot of the picture. */
function seaBand(level, amp, length, speed, t, phase) {
	const y = (x) => {
		const swell = 1 - Math.abs(Math.sin(x / length + t * speed + phase));
		const under = Math.sin((x / length) * 0.37 - t * speed * 0.4 + phase * 1.7);
		const chop = Math.sin((x / length) * 3.1 + t * speed * 1.6 + phase);
		return level - amp * (swell ** 1.6 + 0.55 * under + 0.12 * chop);
	};
	let d = `M-20 ${num(y(-20))}`;
	for (let x = -8; x <= W + 24; x += 12) d += `L${x} ${num(y(x))}`;
	return { line: d, shape: `${d}L${W + 24} ${H + 20}L-20 ${H + 20}Z` };
}

const SKULL = "M450 176C426 146 412 104 428 74C436 52 456 46 466 42C478 20 492 26 500 36C508 26 522 20 534 42C544 46 564 52 572 74C588 104 574 146 550 176C546 196 534 212 522 218L478 218C466 212 454 196 450 176Z";
const TORSO = "M176 540C170 440 196 346 250 306C282 282 330 278 374 266C398 256 416 238 438 222C470 208 530 208 562 222C584 238 602 256 626 266C670 278 718 282 750 306C804 346 830 440 824 540Z";

const BANDS = [
	{ off: -44, amp: 15, len: 86, speed: 0.9, phase: 0.3, fill: "em-sea-a" },
	{ off: -22, amp: 18, len: 70, speed: -1.15, phase: 2.2, fill: "em-sea-b" },
	{ off: 4, amp: 19, len: 60, speed: 1.3, phase: 4.1, fill: "em-sea-c" },
	{ off: 34, amp: 22, len: 74, speed: -1.0, phase: 1.1, fill: "em-sea-d" },
	{ off: 70, amp: 26, len: 88, speed: 0.85, phase: 5.2, fill: "em-sea-e" },
];

export function cthulhuScene() {
	const rand = randoms(1928); // the year of The Call of Cthulhu
	const svg = s("svg", { class: "em-scene", viewBox: `0 0 ${W} ${H}`, preserveAspectRatio: "xMidYMax slice", "aria-hidden": "true" });
	const band = (id, a, b) => s("linearGradient", { id, x1: "0", y1: "0", x2: "0", y2: "1" }, stop(0, a), stop(0.35, b), stop(1, "#010a0e"));
	svg.append(
		s(
			"defs",
			{},
			s("linearGradient", { id: "em-storm", x1: "0", y1: "0", x2: "0", y2: "1" }, stop(0, "#020807", 0.94), stop(0.7, "#06140f", 0.7), stop(1, "#06140f", 0.4)),
			s("radialGradient", { id: "em-aura", cx: "0.5", cy: "0.5", r: "0.5" }, stop(0, "#8ff5c4", 0.8), stop(0.45, "#2fa883", 0.42), stop(1, "#0a3a2c", 0)),
			s("radialGradient", { id: "em-cloud", cx: "0.5", cy: "0.5", r: "0.5" }, stop(0, "#0a1a17", 0.92), stop(0.7, "#071311", 0.55), stop(1, "#050d0c", 0)),
			s("radialGradient", { id: "em-skin", cx: "0.5", cy: "0.2", r: "0.85" }, stop(0, "#1c463a"), stop(0.5, "#0d2820"), stop(1, "#030c09")),
			s("linearGradient", { id: "em-torso", x1: "0", y1: "0", x2: "0", y2: "1" }, stop(0, "#123429"), stop(0.5, "#07190f"), stop(1, "#010604")),
			s("linearGradient", { id: "em-arm", x1: "0", y1: "0", x2: "0", y2: "1" }, stop(0, "#1a4638"), stop(1, "#04100b")),
			s("linearGradient", { id: "em-tent", x1: "0", y1: "0", x2: "0", y2: "1" }, stop(0, "#0a221a"), stop(0.6, "#1a4436"), stop(1, "#0a2019")),
			s("linearGradient", { id: "em-wing", x1: "0", y1: "0", x2: "0", y2: "1" }, stop(0, "#04100c"), stop(1, "#020806")),
			s("radialGradient", { id: "em-wet", cx: "0.5", cy: "0.5", r: "0.5" }, stop(0, "#bff5de", 0.32), stop(1, "#bff5de", 0)),
			band("em-sea-a", "#1a4f55", "#0b2c34"),
			band("em-sea-b", "#1f5f64", "#0a2d36"),
			band("em-sea-c", "#247079", "#0a2f39"),
			band("em-sea-d", "#2a7f86", "#08303a"),
			band("em-sea-e", "#2f8c92", "#062a33"),
			s("radialGradient", { id: "em-eye", cx: "0.5", cy: "0.5", r: "0.5" }, stop(0, "#fffde0"), stop(0.5, "#f2f76a"), stop(1, "#9bd62c")),
			s("radialGradient", { id: "em-eye-glow", cx: "0.5", cy: "0.5", r: "0.5" }, stop(0, "#eaff7a", 0.95), stop(0.25, "#b8ec4a", 0.42), stop(1, "#a7e04a", 0)),
			s("linearGradient", { id: "em-reflect", x1: "0", y1: "0", x2: "0", y2: "1" }, stop(0, "#d6f75a", 0.55), stop(1, "#d6f75a", 0)),
			s("clipPath", { id: "em-skull-clip" }, s("path", { d: SKULL })),
			s("clipPath", { id: "em-torso-clip" }, s("path", { d: TORSO }))
		)
	);
	const add = (parent, tag, attrs) => {
		const el = s(tag, attrs);
		parent.append(el);
		return el;
	};

	// The storm: the sky closes over the dimmed map, and clouds gather.
	const stormy = add(svg, "g", { opacity: 0 });
	add(stormy, "rect", { x: 0, y: 0, width: W, height: H, fill: "url(#em-storm)" });
	for (let i = 0; i < 12; i++) {
		add(stormy, "ellipse", { cx: rand() * W, cy: rand() * 110 - 10, rx: 120 + rand() * 170, ry: 36 + rand() * 46, fill: "url(#em-cloud)" });
	}
	const aura = add(svg, "ellipse", { cx: 500, cy: 180, rx: 560, ry: 310, fill: "url(#em-aura)", opacity: 0 });
	const bolt = add(svg, "path", { fill: "none", stroke: "#eafff7", "stroke-width": 2.6, "stroke-linejoin": "bevel", opacity: 0 });

	// The drowned city behind him: angles that are wrong.
	const city = add(svg, "g", {});
	for (const d of ["M38 520L66 190L138 150L176 520Z", "M150 520L190 262L262 300L268 520Z", "M250 520L262 330L330 302L346 520Z", "M652 520L668 310L736 336L748 520Z", "M730 520L750 176L842 226L870 520Z", "M840 520L888 262L962 232L978 520Z"]) {
		add(city, "path", { d, fill: "#05140f", stroke: "#2a6a58", "stroke-width": 1.1 });
	}
	add(city, "path", {
		d: "M70 230L138 196M68 276L146 236M64 330L156 282M196 300L258 332M198 350L262 378M268 360L334 336M672 346L734 372M754 214L840 262M752 268L850 320M750 330L860 384M892 300L958 272M888 350L964 320",
		fill: "none",
		stroke: "#2f8f75",
		"stroke-width": 1.2,
		opacity: 0.5,
	});

	const seaBehind = BANDS.slice(0, 1).map((b) => ({ b, fill: add(svg, "path", { fill: `url(#${b.fill})` }), foam: add(svg, "path", { fill: "none", stroke: "#b9e9e2", "stroke-width": 0.9, "stroke-dasharray": "14 90 6 130 3 60", opacity: 0.3 }) }));

	// Behind the body: the wings, then the arms.
	const wingLayer = add(svg, "g", {});
	const wings = [0, 1].map(() => {
		const skin = add(wingLayer, "path", { fill: "url(#em-wing)", stroke: "#27705a", "stroke-width": 0.9 });
		const veins = add(wingLayer, "path", { fill: "none", stroke: "#1b4a3b", "stroke-width": 1.1, opacity: 0.7 });
		const bones = add(wingLayer, "path", { fill: "#0d2b21", stroke: "#2f7f66", "stroke-width": 0.7 });
		return { skin, veins, bones };
	});
	const figure = add(svg, "g", { class: "em-figure" });
	const arms = [0, 1].map(() => ({
		limb: add(figure, "path", { fill: "url(#em-arm)", stroke: "#2f7f66", "stroke-width": 1 }),
		fingers: add(figure, "path", { fill: "url(#em-arm)", stroke: "#2f7f66", "stroke-width": 0.8 }),
		claws: add(figure, "path", { fill: "#d5e6c9", stroke: "#0a1a12", "stroke-width": 0.6 }),
	}));
	add(figure, "path", { d: TORSO, fill: "url(#em-torso)" });
	add(figure, "path", { d: scales(rand, 230, 770, 230, 500, 520), fill: "none", stroke: "#2c6a55", "stroke-width": 0.8, opacity: 0.32, "clip-path": "url(#em-torso-clip)" });
	add(figure, "path", {
		d: "M500 250V520M382 306C420 362 470 376 500 376C530 376 580 362 618 306M362 410C420 456 468 466 500 466C532 466 580 456 638 410M300 322C288 370 296 430 318 510M700 322C712 370 704 430 682 510",
		fill: "none",
		stroke: "#010604",
		"stroke-width": 5,
		opacity: 0.8,
	});
	add(figure, "ellipse", { cx: 318, cy: 306, rx: 92, ry: 30, fill: "url(#em-wet)", transform: "rotate(-14 318 306)" });
	add(figure, "ellipse", { cx: 682, cy: 306, rx: 92, ry: 30, fill: "url(#em-wet)", transform: "rotate(14 682 306)" });

	add(figure, "path", { d: SKULL, fill: "url(#em-skin)", stroke: "#062017", "stroke-width": 2 });
	add(figure, "path", { d: scales(rand, 420, 580, 24, 216, 240), fill: "none", stroke: "#3a7d67", "stroke-width": 0.7, opacity: 0.32, "clip-path": "url(#em-skull-clip)" });
	add(figure, "path", {
		d: "M500 36C495 62 498 88 494 128M466 44C458 66 466 92 458 126M534 44C544 70 534 94 544 124M432 90C438 106 432 124 448 144M568 90C562 110 570 126 552 144M484 62C478 78 484 92 480 108M516 60C524 80 516 96 522 112",
		fill: "none",
		stroke: "#010806",
		"stroke-width": 2,
		"stroke-linecap": "round",
		opacity: 0.6,
	});
	for (const [cx, cy, rx, ry] of [[478, 72, 20, 26], [526, 66, 18, 22], [446, 104, 14, 24], [556, 108, 12, 22]]) add(figure, "ellipse", { cx, cy, rx, ry, fill: "url(#em-wet)" });
	const rim = add(figure, "path", {
		d: "M447 172C426 146 412 104 428 74C436 52 456 46 466 42C478 20 492 26 500 36C508 26 522 20 534 42C544 46 564 52 572 74C588 104 574 146 553 172M374 266C330 278 282 282 250 306M626 266C670 278 718 282 750 306",
		fill: "none",
		stroke: "#a8f5d0",
		"stroke-width": 1.8,
		opacity: 0.4,
	});
	// Sockets under a heavy brow, and eyes set deep and narrow.
	add(figure, "path", { d: "M452 142L494 158C498 170 492 176 480 174C466 172 454 160 452 142ZM548 142L506 158C502 170 508 176 520 174C534 172 546 160 548 142Z", fill: "#000" });
	add(figure, "path", { d: "M444 134C462 136 484 146 498 160L490 164C476 154 460 148 446 146ZM556 134C538 136 516 146 502 160L510 164C524 154 540 148 554 146Z", fill: "#04120d", stroke: "#2c7a62", "stroke-width": 0.8 });
	const eyes = [
		[474, 162, 18],
		[526, 162, -18],
	].map(([x, y, tilt]) => {
		const at = `rotate(${tilt} ${x} ${y})`;
		return {
			glow: add(figure, "circle", { cx: x, cy: y, r: 46, fill: "url(#em-eye-glow)", opacity: 0 }),
			ball: add(figure, "ellipse", { cx: x, cy: y, rx: 12, ry: 0.3, fill: "url(#em-eye)", transform: at }),
			slit: add(figure, "ellipse", { cx: x, cy: y, rx: 1.6, ry: 0.3, fill: "#0a1405", transform: at }),
		};
	});
	const tentacles = [];
	const beard = add(figure, "g", {});
	for (let i = 0; i < 13; i++) {
		const across = (i - 6) / 6;
		tentacles.push({
			root: [500 + across * 48, 196 + (1 - Math.abs(across)) * 20],
			length: 150 + (1 - Math.abs(across)) * 120 + rand() * 40,
			width: 19 - Math.abs(across) * 5,
			lean: across * 0.62,
			phase: rand() * 6.28,
			rate: 1.1 + rand() * 0.9,
			front: 1 - Math.abs(across),
		});
	}
	// The longest hang in front.
	tentacles.sort((a, b) => a.length - b.length);
	for (const ten of tentacles) {
		ten.skin = add(beard, "path", { fill: "url(#em-tent)", stroke: "#2f7f66", "stroke-width": 0.8 });
		ten.suckers = add(beard, "path", { fill: "#020c08", stroke: "#57a88d", "stroke-width": 0.5, opacity: 0.7 });
	}
	const shed = add(figure, "g", { fill: "none", stroke: "#cdeee8", "stroke-linecap": "round" });
	const streams = [];
	for (let i = 0; i < 26; i++) {
		const x = 240 + rand() * 520;
		const y0 = 250 + rand() * 90 + Math.abs(x - 500) * 0.12;
		const el = add(shed, "path", { d: `M${num(x)} ${num(y0)}q${num((rand() - 0.5) * 16)} ${num(50 + rand() * 50)} ${num((rand() - 0.5) * 18)} ${num(110 + rand() * 90)}`, "stroke-width": 0.6 + rand() * 1.2, "stroke-dasharray": `${num(12 + rand() * 18)} 46` });
		streams.push({ el, speed: 130 + rand() * 100 });
	}

	// In front of him: the eye-light on the water, the sea, its foam, and a collar of white where he breaks it.
	const reflect = add(svg, "g", { opacity: 0 });
	const glints = [470, 530].map((x) => add(reflect, "rect", { x: x - 16, width: 32, height: 74, fill: "url(#em-reflect)" }));
	const seaFront = BANDS.slice(1).map((b, i) => ({
		b,
		fill: add(svg, "path", { fill: `url(#${b.fill})` }),
		foam: add(svg, "path", { fill: "none", stroke: "#dff9f4", "stroke-width": 0.8 + i * 0.3, "stroke-dasharray": `${8 + i * 5} ${48 + i * 5} 3 ${26} ${5 + i * 3} ${96 - i * 8}`, "stroke-linecap": "round", opacity: 0.34 + i * 0.07 }),
	}));
	// Where he breaks the water, it boils: a mass of small bubbles along the line.
	const boil = add(svg, "g", { fill: "#eefffb", opacity: 0 });
	const bubbles = [];
	for (let i = 0; i < 190; i++) {
		bubbles.push({ el: add(boil, "ellipse", { rx: 0.7 + rand() * 2.8, ry: 0.4 + rand() * 1.1, opacity: 0.15 + rand() * 0.5 }), at: rand() * 2 - 1, depth: rand() ** 2, phase: rand() * 6.28, rate: 1.4 + rand() * 2.4 });
	}

	const spray = add(svg, "g", { fill: "#e4fcf8" });
	const drops = [];
	for (let i = 0; i < 90; i++) {
		const el = add(spray, "circle", { r: 0.8 + rand() * 2.6, opacity: 0 });
		drops.push({ el, x: 500 + (rand() - 0.5) * 660, born: 2.1 + rand() * 2.4, up: 90 + rand() * 250, drift: (rand() - 0.5) * 120, life: 0.7 + rand() * 0.7 });
	}
	const rain = add(svg, "g", { stroke: "#a8cfcb", "stroke-width": 0.9, opacity: 0 });
	const drops2 = [];
	for (let i = 0; i < 70; i++) drops2.push({ el: add(rain, "line", {}), x: rand() * (W + 200), y: rand() * H, len: 14 + rand() * 24, speed: 620 + rand() * 340 });
	const flash = add(svg, "rect", { x: 0, y: 0, width: W, height: H, fill: "#e6fff5", opacity: 0 });

	const BOLTS = [
		[1.55, "M660 0L622 70L654 92L596 170L632 190L578 280M622 70L586 108L594 142"],
		[1.82, "M240 0L268 72L228 110L292 190L252 232L300 320M228 110L188 150"],
		[4.42, "M500 0L460 22L512 44L452 64"],
		[5.3, "M842 0L808 72L850 100L788 186L822 208M808 72L772 110"],
	];

	function seek(t) {
		const rise = outCubic(between(t, 0.4, 2.2));
		const level = 530 - rise * 130; // where the sea stands
		const emerge = outCubic(between(t, 1.9, 4.9));
		const open = smooth(between(t, 3.3, 4.8));
		const lift = smooth(between(t, 2.9, 4.8));
		const awake = smooth(between(t, 4.35, 4.7));
		const fade = 1 - between(t, CTHULHU_SECONDS - 0.8, CTHULHU_SECONDS);
		const rumble = (between(t, 1.5, 2.4) * (1 - between(t, 4.9, 5.6)) + between(t, 4.35, 4.5) * (1 - between(t, 4.5, 5.1))) * 3.2;
		const shakeX = Math.sin(t * 57) * rumble;
		const shakeY = Math.cos(t * 43) * rumble * 0.7;
		const heave = Math.sin(t * 1.15) * 5 * emerge;
		const y = (1 - emerge) * 490 + heave;

		svg.style.opacity = String(fade);
		stormy.setAttribute("opacity", String(smooth(between(t, 0.2, 1.8))));
		aura.setAttribute("opacity", String(emerge * 0.42 + awake * (0.48 + Math.sin(t * 5) * 0.05)));
		city.setAttribute("transform", `translate(${num(shakeX * 0.4)} ${num((1 - outCubic(between(t, 1.2, 4.8))) * 340 + shakeY * 0.4)})`);
		figure.setAttribute("transform", `translate(${num(shakeX)} ${num(y + shakeY)})`);
		wingLayer.setAttribute("transform", `translate(${num(shakeX)} ${num(y * 0.98 + shakeY)})`);

		const draw = ({ b, fill, foam }) => {
			const wave = seaBand(level + b.off, b.amp * (0.6 + rise * 0.4), b.len, b.speed, t, b.phase);
			fill.setAttribute("d", wave.shape);
			foam.setAttribute("d", wave.line);
			foam.setAttribute("stroke-dashoffset", num(t * b.speed * -60));
		};
		seaBehind.forEach(draw);
		seaFront.forEach(draw);

		// Where he breaks the water.
		const span = 170 + emerge * 180;
		for (const b of bubbles) {
			const x = 500 + b.at * span + Math.sin(t * b.rate + b.phase) * 9;
			const edge = 1 - b.at * b.at;
			b.el.setAttribute("cx", num(x));
			b.el.setAttribute("cy", num(level - 10 + b.depth * 26 + Math.sin(t * b.rate * 1.4 + b.phase) * 3 - edge * 5));
		}
		boil.setAttribute("opacity", String(between(t, 1.9, 2.5)));

		// The wings open out from behind his shoulders: curved bones, a torn skin between them.
		wings.forEach((wing, side) => {
			const mx = (x) => (side ? W - x : x);
			const beat = Math.sin(t * 1.05) * 10 * open;
			const at = (ox, oy) => [mx(lerp(372, ox, open)), lerp(258, oy + beat, open)];
			const root = [mx(372), 258];
			const knuckle = at(196, 30);
			const tips = [at(14, 58), at(-6, 214), at(64, 332), at(176, 388)];
			const bones = [limb(curve(root, [mx(lerp(372, 300, open)), lerp(258, 120, open)], knuckle, 8), (k) => lerp(19, 9, k))];
			const bends = [
				[-20, -34],
				[-34, -6],
				[-30, 8],
				[-20, 12],
			];
			tips.forEach((tip, i) => {
				const cx = lerp(knuckle[0], tip[0], 0.5) + (side ? -1 : 1) * bends[i][0] * open;
				const cy = lerp(knuckle[1], tip[1], 0.5) + bends[i][1] * open;
				bones.push(limb(curve(knuckle, [cx, cy], tip, 8), (k) => lerp(8, 1.6, k)));
			});
			wing.bones.setAttribute("d", bones.join(""));
			// The edge sags between the tips, and has been torn.
			let skin = `M${num(root[0])} ${num(root[1])}L${num(knuckle[0])} ${num(knuckle[1])}L${num(tips[0][0])} ${num(tips[0][1])}`;
			const dir = side ? -1 : 1;
			for (let i = 1; i < tips.length; i++) {
				const a = tips[i - 1];
				const b = tips[i];
				const sag = 46 * open;
				const mid = [lerp(a[0], b[0], 0.5) + dir * sag, lerp(a[1], b[1], 0.5)];
				const tear = [lerp(a[0], mid[0], 0.62), lerp(a[1], mid[1], 0.62)];
				skin += `Q${num(lerp(a[0], mid[0], 0.5) + dir * 8 * open)} ${num(lerp(a[1], mid[1], 0.5))} ${num(tear[0])} ${num(tear[1])}`;
				skin += `L${num(tear[0] + dir * (i === 2 ? 15 : 7) * open)} ${num(tear[1] - 3)}L${num(mid[0])} ${num(mid[1])}`;
				skin += `Q${num(lerp(mid[0], b[0], 0.5) + dir * 6 * open)} ${num(lerp(mid[1], b[1], 0.5))} ${num(b[0])} ${num(b[1])}`;
			}
			skin += `Q${num(lerp(tips[3][0], root[0], 0.5))} ${num(root[1] + 150 * open)} ${num(root[0])} ${num(root[1])}Z`;
			wing.skin.setAttribute("d", skin);
			let veins = "";
			for (let i = 0; i < tips.length - 1; i++) {
				const f = 0.5;
				const p = [lerp(tips[i][0], tips[i + 1][0], f), lerp(tips[i][1], tips[i + 1][1], f)];
				veins += `M${num(knuckle[0])} ${num(knuckle[1])}Q${num(lerp(knuckle[0], p[0], 0.5) + 12)} ${num(lerp(knuckle[1], p[1], 0.5))} ${num(lerp(knuckle[0], p[0], 0.82))} ${num(lerp(knuckle[1], p[1], 0.82))}`;
			}
			wing.veins.setAttribute("d", veins);
		});

		// The arms come up out of the sea, and the hands open.
		arms.forEach((arm, side) => {
			const mx = (x) => (side ? W - x : x);
			const sway = Math.sin(t * 1.2 + side * 1.9) * 6;
			const shoulder = [mx(262), 340];
			const elbow = [mx(lerp(214, 118, lift)), lerp(480, 366, lift)];
			const wrist = [mx(lerp(196, 96, lift)) + sway, lerp(530, 214, lift) + sway * 0.6];
			const upper = curve(shoulder, [lerp(shoulder[0], elbow[0], 0.5), lerp(shoulder[1], elbow[1], 0.5) + 16], elbow, 7);
			const fore = curve(elbow, [lerp(elbow[0], wrist[0], 0.5) + (side ? 14 : -14), lerp(elbow[1], wrist[1], 0.5)], wrist, 7).slice(1);
			const spine = upper.concat(fore);
			arm.limb.setAttribute(
				"d",
				limb(spine, (k) => 40 + 56 * Math.abs(Math.sin(k * Math.PI * 2 + 0.5)) * (1 - k * 0.55) - k * 16)
			);
			let fingers = "";
			let claws = "";
			const spread = 0.34 + open * 0.3;
			for (let f = 0; f < 4; f++) {
				const dir = (side ? 1 : -1) * (0.05 + (f - 1.3) * spread);
				const len = f === 0 ? 56 : 76 - Math.abs(f - 1.7) * 7;
				const knuckleBend = (side ? -1 : 1) * 16 * (1 - open * 0.5);
				const tip = [wrist[0] + Math.sin(dir) * len, wrist[1] - Math.cos(dir) * len];
				const mid = [lerp(wrist[0], tip[0], 0.55) - knuckleBend, lerp(wrist[1], tip[1], 0.55)];
				fingers += limb(curve([wrist[0], wrist[1] + 6], mid, tip, 6), (k) => lerp(17, 7, k));
				const hook = [tip[0] + Math.sin(dir + (side ? -1 : 1) * 0.9) * 22, tip[1] - Math.cos(dir + (side ? -1 : 1) * 0.9) * 22];
				claws += limb(curve(tip, [lerp(tip[0], hook[0], 0.5), lerp(tip[1], hook[1], 0.5) - 3], hook, 4), (k) => lerp(6.5, 0.4, k));
			}
			arm.fingers.setAttribute("d", fingers);
			arm.claws.setAttribute("d", claws);
		});

		// The tentacles of the face, writhing the more as he wakes.
		for (const ten of tentacles) {
			const points = [];
			let [x, py] = ten.root;
			const stepLen = ten.length / 17;
			const curl = 0.12 + awake * 0.14;
			for (let i = 0; i <= 17; i++) {
				points.push([x, py]);
				const k = i / 17;
				const angle = ten.lean * (1 - k * 0.45) + Math.sin(k * 5.5 - t * ten.rate + ten.phase) * (curl + k * k * 1.15);
				x += Math.sin(angle) * stepLen;
				py += Math.cos(angle) * stepLen;
			}
			ten.skin.setAttribute(
				"d",
				limb(points, (k) => lerp(ten.width, 1.2, k ** 0.8))
			);
			let dots = "";
			for (let i = 3; i < points.length - 3; i += 1) {
				const r = (ten.width * 0.13 + 0.3) * (1 - i / points.length);
				const px = points[i][0] + (i % 2 ? 1 : -1) * ten.width * 0.1 * (1 - i / 17);
				dots += `M${num(px - r)} ${num(points[i][1])}a${num(r)} ${num(r)} 0 1 0 ${num(r * 2)} 0a${num(r)} ${num(r)} 0 1 0 ${num(-r * 2)} 0`;
			}
			ten.suckers.setAttribute("d", dots);
		}

		// His eyes open last, and what they light falls on the water.
		const blink = t > 6.15 && t < 6.28 ? 0.12 : 1;
		for (const e of eyes) {
			e.ball.setAttribute("ry", num(0.3 + awake * 6.2 * blink));
			e.slit.setAttribute("ry", num(0.3 + awake * 5.6 * blink));
			e.glow.setAttribute("opacity", String(awake * (0.85 + Math.sin(t * 6) * 0.15)));
		}
		rim.setAttribute("opacity", String(0.24 + emerge * 0.2 + awake * 0.3));
		reflect.setAttribute("opacity", String(awake * (0.7 + Math.sin(t * 7) * 0.12)));
		for (const g of glints) g.setAttribute("y", num(level + 2));

		// Water runs off him, and stops.
		shed.setAttribute("opacity", String(0.65 * between(t, 2.2, 2.8) * (1 - between(t, 5.2, 6.8))));
		for (const st of streams) st.el.setAttribute("stroke-dashoffset", num(-t * st.speed));
		for (const d of drops) {
			const k = (t - d.born) / d.life;
			if (k < 0 || k > 1) {
				d.el.setAttribute("opacity", "0");
				continue;
			}
			d.el.setAttribute("cx", num(d.x + d.drift * k));
			d.el.setAttribute("cy", num(level - d.up * (4 * k * (1 - k))));
			d.el.setAttribute("opacity", String(0.85 * (1 - k)));
		}
		rain.setAttribute("opacity", String(0.3 * smooth(between(t, 0.8, 2))));
		for (const r of drops2) {
			const yy = (r.y + t * r.speed) % (H + 40);
			const xx = ((r.x - t * r.speed * 0.26) % (W + 200) + W + 200) % (W + 200);
			r.el.setAttribute("x1", num(xx));
			r.el.setAttribute("y1", num(yy));
			r.el.setAttribute("x2", num(xx - r.len * 0.26));
			r.el.setAttribute("y2", num(yy + r.len));
		}

		// Lightning.
		let lit = 0;
		for (const [at, path] of BOLTS) {
			const k = (t - at) / 0.26;
			if (k >= 0 && k <= 1) {
				lit = Math.max(lit, (1 - k) * (k < 0.3 || k > 0.5 ? 1 : 0.3));
				bolt.setAttribute("d", path);
			}
		}
		bolt.setAttribute("opacity", String(lit));
		flash.setAttribute("opacity", String(lit * 0.38));
	}

	return { el: svg, seek };
}
