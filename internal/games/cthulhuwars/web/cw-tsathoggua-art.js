// Tsathoggua, stirring from his slumber in black N'kai: a cavern, a vast furred bulk breathing in the dark and
// the mist of its breath, then dust falling from the roof as he lifts his head, his bat's ears unfold, and two
// heavy eyes open on the one who woke him. Original art, drawn in code (see js/cw-art-kit.js).

import { H, W, add, awakeningBurst, between, blob, lerp, num, randoms, round, sceneSvg, smooth, vertical } from "./cw-art-kit.js";
import { s } from "/static/js/util.js";

export const TSATHOGGUA_SECONDS = 7.4;
export const TSATHOGGUA_PEAK = 5.6;

const CX = 500;
const FLOOR = 430;

export function tsathogguaScene() {
	const rand = randoms(1931); // the year of "The Tale of Satampra Zeiros"
	const svg = sceneSvg();
	svg.append(
		s(
			"defs",
			{},
			vertical("ts-cave", [
				[0, "#040302"],
				[0.6, "#120b05"],
				[1, "#23150a"],
			]),
			round("ts-ember", [
				[0, "#d9802a", 0.7],
				[0.5, "#7e3c12", 0.28],
				[1, "#1a0d05", 0],
			]),
			vertical("ts-fur", [
				[0, "#3a2a1e"],
				[0.45, "#1b120c"],
				[1, "#080504"],
			]),
			vertical("ts-belly", [
				[0, "#4d3620"],
				[1, "#150d07"],
			]),
			vertical("ts-rock", [
				[0, "#17100a"],
				[1, "#050302"],
			]),
			vertical("ts-floor", [
				[0, "#2b1b0e"],
				[1, "#080503"],
			]),
			round("ts-eye", [
				[0, "#fff3b4"],
				[0.45, "#f0a51e"],
				[1, "#8a3f08"],
			]),
			round("ts-eye-glow", [
				[0, "#ffb733", 0.85],
				[0.4, "#c9701e", 0.35],
				[1, "#6e3410", 0],
			]),
			vertical("ts-head", [
				[0, "#4a3522"],
				[0.5, "#21160d"],
				[1, "#1b120c"],
			]),
			vertical("ts-rim", [
				[0, "#1b120c", 0],
				[0.6, "#b06a24", 0.05],
				[1, "#d98a35", 0.3],
			]),
			round("ts-mist", [
				[0, "#b7a68d", 0.34],
				[1, "#b7a68d", 0],
			]),
			vertical("ts-mouth", [
				[0, "#2a0806"],
				[1, "#7a1f14"],
			])
		)
	);
	const cave = add(svg, "g", { opacity: 0 });
	add(cave, "rect", { x: 0, y: 0, width: W, height: H, fill: "url(#ts-cave)" });
	const ember = add(cave, "ellipse", { cx: CX, cy: 400, rx: 560, ry: 250, fill: "url(#ts-ember)", opacity: 0.3 });
	// Columns far off in the dark, then the floor.
	let far = "";
	for (const [x, w] of [
		[70, 56],
		[196, 34],
		[806, 42],
		[930, 62],
	]) {
		far += `M${x - w} ${FLOOR}C${x - w * 0.4} 320 ${x - w * 0.4} 150 ${x - w * 0.8} 0L${x + w * 0.8} 0C${x + w * 0.4} 150 ${x + w * 0.4} 320 ${x + w} ${FLOOR}Z`;
	}
	add(cave, "path", { d: far, fill: "#0b0704", stroke: "#2d1c0f", "stroke-width": 1.2 });
	add(cave, "path", { d: `M0 ${FLOOR}C180 ${FLOOR - 16} 320 ${FLOOR + 8} 500 ${FLOOR - 6}C680 ${FLOOR - 16} 840 ${FLOOR + 10} ${W} ${FLOOR - 8}L${W} ${H}L0 ${H}Z`, fill: "url(#ts-floor)" });

	// He is all one mass: head, shoulders and haunches on the floor of the cave, lifted as a whole.
	const body = add(svg, "g", {});
	const haunch = add(body, "path", { fill: "url(#ts-fur)", stroke: "#6a4c2e", "stroke-width": 1.3 });
	const rimLight = add(body, "path", { fill: "url(#ts-rim)" });
	const furLayer = add(body, "g", {});
	let fur = "";
	for (let i = 0; i < 620; i++) {
		const a = rand() * 6.283;
		const k = Math.sqrt(rand());
		const x = CX + Math.cos(a) * k * 330;
		const y = 330 + Math.sin(a) * k * 120;
		fur += `M${num(x)} ${num(y)}l${num((x - CX) * 0.018 + (rand() - 0.5) * 4)} ${num(7 + rand() * 9)}`;
	}
	add(furLayer, "path", { d: fur, fill: "none", stroke: "#6a4f36", "stroke-width": 1.1, opacity: 0.5 });
	const paws = [-1, 1].map((side) => {
		const g = add(body, "g", {});
		add(g, "path", { d: `M${CX + side * 150} 350C${CX + side * 186} 372 ${CX + side * 200} 410 ${CX + side * 192} 438C${CX + side * 176} 450 ${CX + side * 120} 450 ${CX + side * 100} 440C${CX + side * 92} 410 ${CX + side * 104} 370 ${CX + side * 150} 350Z`, fill: "url(#ts-fur)", stroke: "#4a3524", "stroke-width": 1.2 });
		let claws = "";
		for (let i = 0; i < 4; i++) {
			const x = CX + side * (108 + i * 25);
			claws += `M${x - 6} 440Q${x} 462 ${x + side * 6} 462Q${x + 4} 450 ${x + 6} 440Z`;
		}
		add(g, "path", { d: claws, fill: "#c2a877", stroke: "#23150a", "stroke-width": 0.8 });
		return g;
	});

	// The head, which lifts further than the rest.
	const head = add(body, "g", {});
	const ears = [-1, 1].map((side) => add(head, "path", { fill: "#1c120b", stroke: "#4a3524", "stroke-width": 1.3 }));
	const skull = add(head, "path", { fill: "url(#ts-head)" });
	const brow = add(head, "path", { fill: "none", stroke: "#8a6237", "stroke-width": 2.4, opacity: 0.5 });
	let headFur = "";
	for (let i = 0; i < 280; i++) {
		const a = rand() * 6.283;
		const k = Math.sqrt(rand());
		const x = CX + Math.cos(a) * k * 205;
		const y = 240 + Math.sin(a) * k * 58;
		headFur += `M${num(x)} ${num(y)}l${num((x - CX) * 0.03)} ${num(5 + rand() * 7)}`;
	}
	add(head, "path", { d: headFur, fill: "none", stroke: "#74563a", "stroke-width": 1, opacity: 0.55 });
	const mouth = add(head, "path", { fill: "url(#ts-mouth)", stroke: "#0a0504", "stroke-width": 2 });
	const teeth = add(head, "path", { fill: "#d8caa0" });
	const tongue = add(head, "path", { fill: "#a53a2b", stroke: "#5c1a12", "stroke-width": 1 });
	const nostrils = add(head, "path", { d: `M${CX - 30} 240q-9 -7 -17 0q8 6 17 0ZM${CX + 30} 240q9 -7 17 0q-8 6 -17 0Z`, fill: "#040201" });
	const eyes = [-1, 1].map((side) => {
		const cx = CX + side * 92;
		const cy = 208;
		return {
			glow: add(head, "circle", { cx, cy, r: 90, fill: "url(#ts-eye-glow)", opacity: 0 }),
			ball: add(head, "ellipse", { cx, cy, rx: 34, ry: 0.5, fill: "url(#ts-eye)" }),
			slit: add(head, "ellipse", { cx, cy, rx: 3.5, ry: 0.3, fill: "#120602" }),
			// A heavy lid that never quite lifts: he is not awake for long, and does not care to be.
			lid: add(head, "path", { fill: "#1f150d", stroke: "#8a6237", "stroke-width": 2 }),
			cy,
			cx,
		};
	});

	// What he breathes out, and what comes down from the roof.
	const mists = [];
	for (let i = 0; i < 8; i++) mists.push({ el: add(svg, "ellipse", { fill: "url(#ts-mist)", opacity: 0 }), side: i % 2 ? 1 : -1, at: i * 0.72, drift: 40 + rand() * 60 });
	const roof = add(svg, "path", { fill: "url(#ts-rock)", stroke: "#2d1c0f", "stroke-width": 1.2 });
	let roofD = `M-10 -10L${W + 10} -10L${W + 10} 40`;
	for (let x = W; x >= 0; x -= 44) {
		const long = 34 + rand() * 96 * (Math.abs(x - CX) > 170 ? 1 : 0.5);
		roofD += `L${x - 14} 40L${x - 22} ${num(40 + long)}L${x - 30} 40`;
	}
	roof.setAttribute("d", `${roofD}L-10 40Z`);
	const dust = [];
	for (let i = 0; i < 70; i++) dust.push({ el: add(svg, "circle", { r: 0.8 + rand() * 2, fill: "#a58a63", opacity: 0 }), x: rand() * W, at: 3.1 + rand() * 1.9, fall: 150 + rand() * 180 });
	const burst = awakeningBurst(svg, CX, 224, 4.5, ["#ffcf66", "#c9701e", "#ffd9a0"], 0.36);

	function seek(t) {
		const fade = 1 - between(t, TSATHOGGUA_SECONDS - 0.8, TSATHOGGUA_SECONDS);
		const fall = smooth(between(t, 0.1, 1.3));
		const seen = smooth(between(t, 0.9, 2.6)); // the eye gets used to the dark
		const stir = smooth(between(t, 2.9, 4.2)); // he lifts
		const wake = smooth(between(t, 4.15, 4.65));
		const yawn = smooth(between(t, 4.5, 5.2)) * (1 - 0.6 * smooth(between(t, 6, 6.7)));
		const breath = Math.sin(t * 1.25);

		svg.style.opacity = String(fade);
		cave.setAttribute("opacity", String(fall));
		ember.setAttribute("opacity", String(0.15 + seen * 0.2 + wake * 0.5));
		roof.setAttribute("opacity", String(fall));

		// Asleep he is a low shape that swells and sinks. Awake he stands a third higher.
		const swell = 1 + breath * 0.025 * (1 - stir * 0.6);
		body.setAttribute("opacity", String(0.2 + seen * 0.8));
		body.setAttribute("transform", `translate(${CX} ${FLOOR}) scale(${num(1 + stir * 0.05)} ${(swell * lerp(0.74, 1.04, stir)).toFixed(3)}) translate(${-CX} ${-FLOOR})`);
		const bulk = blob(CX, 334, 360, 134, (a) => 1 + Math.sin(a * 3 + 1) * 0.05 + Math.sin(a * 7 + 0.4) * 0.02 + Math.sin(a * 23) * 0.008);
		haunch.setAttribute("d", bulk);
		rimLight.setAttribute("d", bulk);
		rimLight.setAttribute("opacity", String(seen * (0.45 + wake * 0.55)));
		const tremble = wake * (1 - between(t, 4.8, 5.6)) * Math.sin(t * 40) * 2.5;
		for (const paw of paws) paw.setAttribute("transform", `translate(0 ${num(-stir * 4)})`);

		const lift = lerp(62, -8, stir) + Math.sin(t * 0.8) * 2;
		head.setAttribute("transform", `translate(${num(tremble)} ${num(lift)})`);
		// Flat and very wide, a toad's, sunk into the shoulders with no neck.
		skull.setAttribute("d", blob(CX, 258, 226, 92, (a) => 1 + Math.sin(a * 2 + 1.57) * 0.12 + Math.sin(a * 5) * 0.015));
		brow.setAttribute("d", `M${CX - 158} 196Q${CX - 92} ${num(160 - wake * 10)} ${CX - 34} 200M${CX + 158} 196Q${CX + 92} ${num(160 - wake * 10)} ${CX + 34} 200`);
		ears.forEach((ear, i) => {
			const side = i ? 1 : -1;
			const up = smooth(between(t, 3.5, 4.8));
			// A bat's ear: ribs of it, and torn skin between.
			const tipX = CX + side * lerp(300, 318, up);
			const tipY = lerp(268, 96, up);
			const at = (k, y) => `${num(CX + side * lerp(176, 318, k))} ${num(lerp(268, y, up))}`;
			ear.setAttribute("d", `M${CX + side * 118} 196Q${CX + side * 190} ${num(lerp(210, 104, up))} ${num(tipX)} ${num(tipY)}L${at(0.9, 150)}L${at(0.68, 142)}L${at(0.62, 188)}L${at(0.42, 186)}L${at(0.34, 226)}L${CX + side * 166} 258Z`);
		});

		// The mouth, a seam until he yawns.
		const w = 186;
		const y0 = 280;
		const drop = 3 + yawn * 66;
		// The corners turn down: it is no smile.
		mouth.setAttribute("d", `M${CX - w} ${y0 + 20}Q${CX} ${num(y0 - 22 - yawn * 10)} ${CX + w} ${y0 + 20}Q${CX} ${num(y0 + drop * 1.5)} ${CX - w} ${y0 + 20}Z`);
		let fangs = "";
		if (yawn > 0.02) {
			// Needles, a great many, in no good order.
			for (let i = 0; i < 25; i++) {
				const k = i / 24;
				const x = CX - w * 0.84 + k * w * 1.68;
				const y = y0 + 16 - (1 - (2 * k - 1) ** 2) * (38 + yawn * 9) * 0.46;
				const long = yawn * (9 + ((i * 7) % 5) * 3.2);
				fangs += `M${num(x - 3)} ${num(y)}L${num(x)} ${num(y + long)}L${num(x + 3)} ${num(y)}Z`;
			}
		}
		teeth.setAttribute("d", fangs);
		// The tongue lies in the mouth, in the dark, and only stirs.
		tongue.setAttribute("d", yawn > 0.2 ? `M${CX - 96} ${num(y0 + drop * 0.72)}Q${CX} ${num(y0 + drop * (0.34 + Math.sin(t * 2) * 0.04))} ${CX + 96} ${num(y0 + drop * 0.72)}Q${CX} ${num(y0 + drop * 0.95)} ${CX - 96} ${num(y0 + drop * 0.72)}Z` : "");
		nostrils.setAttribute("opacity", String(0.8));

		// His eyes half open, sink, then open.
		const peek = smooth(between(t, 3.5, 3.8)) * (1 - smooth(between(t, 3.8, 4.1))) * 0.2;
		const open = Math.max(peek, wake);
		for (const e of eyes) {
			e.ball.setAttribute("ry", num(0.5 + open * 25));
			e.slit.setAttribute("ry", num(0.3 + open * 22));
			e.glow.setAttribute("opacity", String(open * (0.86 + Math.sin(t * 3.4) * 0.14)));
			// The lid comes down over the top of the eye, straight across, and stays there.
			const edge = e.cy + lerp(4, -9, open);
			e.lid.setAttribute("d", `M${e.cx - 42} ${e.cy + 2}C${e.cx - 34} ${e.cy - 34} ${e.cx + 34} ${e.cy - 34} ${e.cx + 42} ${e.cy + 2}Q${e.cx} ${num(edge + 4)} ${e.cx - 42} ${e.cy + 2}Z`);
		}

		for (const m of mists) {
			const k = ((t + m.at) % 5.2) / 2.4;
			const on = k < 1 ? Math.sin(k * Math.PI) : 0;
			m.el.setAttribute("cx", num(CX + m.side * (34 + k * m.drift)));
			m.el.setAttribute("cy", num(lift + 262 + k * 30));
			m.el.setAttribute("rx", num(40 + k * 80));
			m.el.setAttribute("ry", num(16 + k * 30));
			m.el.setAttribute("opacity", String(on * seen * 0.8));
		}
		for (const d of dust) {
			const k = between(t, d.at, d.at + 1.5);
			d.el.setAttribute("cx", num(d.x + Math.sin(k * 4 + d.x) * 6));
			d.el.setAttribute("cy", num(50 + k * k * d.fall));
			d.el.setAttribute("opacity", String(k > 0 && k < 1 ? (1 - k) * 0.9 : 0));
		}
		burst(t);
	}
	return { el: svg, seek };
}
