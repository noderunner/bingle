// Bastet, coming from the golden pyramid on the moon: a grey shore under a black sky with the Earth low in it,
// and a pyramid of gold. A line of light opens down its face, the two halves draw apart, and against the blaze
// she rises, tall, seated, still, in her collar of gold and lapis, with her cats about her. Original art, drawn
// in code (see js/cw-art-kit.js).

import { H, W, add, awakeningBurst, between, dot, lerp, num, outCubic, randoms, round, sceneSvg, smooth, vertical } from "./cw-art-kit.js";
import { s } from "./util.js";

export const BASTET_SECONDS = 7.4;
export const BASTET_PEAK = 5.7;

const CX = 500;
const FLOOR = 408;
const APEX = 86;
const HALF = 330; // half the width of the pyramid's foot

export function bastetScene() {
	const rand = randoms(1926); // the year of "The Cats of Ulthar" in a magazine, more or less
	const svg = sceneSvg();
	svg.append(
		s(
			"defs",
			{},
			vertical("ba-sky", [
				[0, "#010104"],
				[1, "#0a0a16"],
			]),
			vertical("ba-ground", [
				[0, "#8d8d92"],
				[0.35, "#4c4c52"],
				[1, "#141418"],
			]),
			round("ba-earth", [
				[0, "#a7dcff"],
				[0.55, "#2f6fb5"],
				[1, "#0c2652"],
			], "0.35", "0.35", "0.72"),
			vertical("ba-gold-l", [
				[0, "#fff0a8"],
				[0.35, "#e5b63d"],
				[1, "#7d5412"],
			]),
			vertical("ba-gold-r", [
				[0, "#d6a32c"],
				[0.5, "#8a5e14"],
				[1, "#3f2708"],
			]),
			round("ba-blaze", [
				[0, "#ffffff"],
				[0.25, "#fff3b8"],
				[0.6, "#ffc94a", 0.65],
				[1, "#c98a14", 0],
			]),
			vertical("ba-beam", [
				[0, "#fff6c9", 0],
				[1, "#fff6c9", 0.75],
			]),
			vertical("ba-body", [
				[0, "#17131f"],
				[0.6, "#0a0810"],
				[1, "#040306"],
			]),
			vertical("ba-collar", [
				[0, "#ffe27a"],
				[1, "#a57416"],
			]),
			round("ba-eye", [
				[0, "#fffbe2"],
				[0.5, "#b8f25c"],
				[1, "#347a1c"],
			]),
			round("ba-eye-glow", [
				[0, "#c9ff7a", 0.85],
				[0.4, "#74c030", 0.32],
				[1, "#2a5a10", 0],
			]),
			round("ba-disc", [
				[0, "#fff9d6"],
				[0.6, "#ffcf4a"],
				[1, "#c98a14"],
			])
		)
	);
	const scene = add(svg, "g", { opacity: 0 });
	add(scene, "rect", { x: 0, y: 0, width: W, height: H, fill: "url(#ba-sky)" });
	let stars = "";
	for (let i = 0; i < 230; i++) stars += dot(rand() * W, rand() * 320, 0.35 + rand() ** 4 * 1.7);
	add(scene, "path", { d: stars, fill: "#fff", opacity: 0.85 });
	// The Earth, a little of it lit.
	add(scene, "circle", { cx: 860, cy: 104, r: 52, fill: "url(#ba-earth)" });
	add(scene, "path", { d: "M860 52A52 52 0 0 1 860 156A68 68 0 0 0 860 52Z", fill: "#010104", opacity: 0, transform: "rotate(30 860 104)" });
	add(scene, "path", { d: "M826 92C842 78 862 84 872 96C862 106 840 112 826 92ZM866 124C878 116 892 120 898 128C888 138 874 138 866 124Z", fill: "#e8f3ff", opacity: 0.55 });
	add(scene, "path", { d: "M808 104A52 52 0 0 0 912 104A64 40 0 0 1 808 104Z", fill: "#010104", opacity: 0.62 });

	// Hills, then the shore itself, with its craters.
	add(scene, "path", { d: `M0 ${FLOOR - 26}L90 ${FLOOR - 62}L170 ${FLOOR - 34}L260 ${FLOOR - 72}L330 ${FLOOR - 30}L1000 ${FLOOR - 30}L1000 ${FLOOR + 10}L0 ${FLOOR + 10}Z`, fill: "#3a3a41" });
	add(scene, "path", { d: `M1000 ${FLOOR - 26}L900 ${FLOOR - 58}L820 ${FLOOR - 30}L730 ${FLOOR - 66}L650 ${FLOOR - 30}L0 ${FLOOR - 30}L0 ${FLOOR + 10}L1000 ${FLOOR + 10}Z`, fill: "#33333a" });
	add(scene, "path", { d: `M0 ${FLOOR - 4}C180 ${FLOOR - 16} 360 ${FLOOR + 6} 520 ${FLOOR - 4}C700 ${FLOOR - 16} 860 ${FLOOR + 4} ${W} ${FLOOR - 8}L${W} ${H}L0 ${H}Z`, fill: "url(#ba-ground)" });
	for (let i = 0; i < 15; i++) {
		const x = rand() * W;
		if (Math.abs(x - CX) < 180) continue;
		const y = FLOOR + 14 + rand() * 76;
		const rx = 12 + rand() * 44;
		add(scene, "ellipse", { cx: x, cy: y, rx, ry: rx * 0.24, fill: "#0e0e12", opacity: 0.55 });
		add(scene, "path", { d: `M${num(x - rx)} ${num(y)}A${num(rx)} ${num(rx * 0.24)} 0 0 0 ${num(x + rx)} ${num(y)}`, fill: "none", stroke: "#a4a4ab", "stroke-width": 1.2, opacity: 0.6 });
	}
	const hush = add(scene, "ellipse", { cx: CX, cy: FLOOR + 14, rx: 320, ry: 30, fill: "#ffcf4a", opacity: 0 });

	// The blaze inside it, and a beam straight up from its point before it parts.
	const blaze = add(svg, "ellipse", { cx: CX, cy: 262, rx: 2, ry: 2, fill: "url(#ba-blaze)", opacity: 0 });
	const beam = add(svg, "path", { d: `M${CX - 3} ${APEX}L${CX + 3} ${APEX}L${CX + 16} -10L${CX - 16} -10Z`, fill: "url(#ba-beam)", opacity: 0, transform: `rotate(180 ${CX} ${(APEX - 10) / 2})` });

	// She, behind the halves of the pyramid: drawn seated and side-on in the old way of it, facing us.
	const figure = add(svg, "g", {});
	const disc = add(figure, "circle", { cx: CX, cy: 98, r: 86, fill: "url(#ba-disc)", stroke: "#fff5c0", "stroke-width": 2 });
	add(figure, "path", {
		// As the old bronzes have it: pointed ears, a small head on a long neck, a narrow chest going straight down
		// into the forelegs, and the haunches round and low on each side.
		d: `M${CX - 48} 56L${CX - 54} 116C${CX - 54} 138 ${CX - 40} 154 ${CX - 24} 164C${CX - 26} 190 ${CX - 40} 212 ${CX - 54} 240C${CX - 62} 262 ${CX - 66} 282 ${CX - 76} 300C${CX - 108} 318 ${CX - 116} 372 ${CX - 92} ${FLOOR}L${CX + 92} ${FLOOR}C${CX + 116} 372 ${CX + 108} 318 ${CX + 76} 300C${CX + 66} 282 ${CX + 62} 262 ${CX + 54} 240C${CX + 40} 212 ${CX + 26} 190 ${CX + 24} 164C${CX + 40} 154 ${CX + 54} 138 ${CX + 54} 116L${CX + 48} 56L${CX + 16} 90Q${CX} 84 ${CX - 16} 90Z`,
		fill: "url(#ba-body)",
		stroke: "#ffcf4a",
		"stroke-width": 1.8,
	});
	// The inner lines of her: the ears, the forelegs and paws, the haunches.
	add(figure, "path", { d: `M${CX - 44} 72L${CX - 48} 104L${CX - 26} 92ZM${CX + 44} 72L${CX + 48} 104L${CX + 26} 92Z`, fill: "#3c2a12" });
	add(figure, "path", { d: `M${CX - 46} 276C${CX - 44} 320 ${CX - 42} 368 ${CX - 46} ${FLOOR - 16}Q${CX - 48} ${FLOOR} ${CX - 24} ${FLOOR - 2}Q${CX - 6} ${FLOOR} ${CX} ${FLOOR - 16}Q${CX + 6} ${FLOOR} ${CX + 24} ${FLOOR - 2}Q${CX + 48} ${FLOOR} ${CX + 46} ${FLOOR - 16}C${CX + 42} 368 ${CX + 44} 320 ${CX + 46} 276M${CX} 292L${CX} ${FLOOR - 16}`, fill: "none", stroke: "#ffcf4a", "stroke-width": 1.5, opacity: 0.72 });
	add(figure, "path", { d: `M${CX - 76} 300C${CX - 70} 330 ${CX - 60} 362 ${CX - 50} 382M${CX + 76} 300C${CX + 70} 330 ${CX + 60} 362 ${CX + 50} 382`, fill: "none", stroke: "#ffcf4a", "stroke-width": 1.2, opacity: 0.4 });
	// Her collar, in bands of gold and lapis.
	for (const [ry, stroke, w] of [
		[10, "url(#ba-collar)", 6],
		[18, "#1f4fb0", 5],
		[26, "url(#ba-collar)", 5],
		[34, "#1f4fb0", 4.5],
		[42, "url(#ba-collar)", 4.5],
	]) {
		add(figure, "path", { d: `M${CX - 25 - ry * 0.42} ${170 + ry * 0.2}Q${CX} ${172 + ry * 1.7} ${CX + 25 + ry * 0.42} ${170 + ry * 0.2}`, fill: "none", stroke, "stroke-width": w, "stroke-linecap": "round" });
	}
	add(figure, "path", { d: `M${CX - 12} 234L${CX + 12} 234L${CX} 262Z`, fill: "url(#ba-collar)", stroke: "#5c3d0c", "stroke-width": 1 });
	add(figure, "path", { d: `M${CX - 7} 136L${CX + 7} 136L${CX} 145Z`, fill: "#6a4a10" });
	add(figure, "path", { d: `M${CX} 145L${CX} 151M${CX - 30} 140L${CX - 66} 134M${CX - 30} 146L${CX - 68} 148M${CX + 30} 140L${CX + 66} 134M${CX + 30} 146L${CX + 68} 148`, fill: "none", stroke: "#ffcf4a", "stroke-width": 1.1, opacity: 0.75 });
	// An earring, which is the only thing about her that moves.
	const earring = add(figure, "circle", { cx: CX + 47, cy: 124, r: 6, fill: "none", stroke: "url(#ba-collar)", "stroke-width": 2.6 });
	const eyes = [-1, 1].map((side) => {
		const cx = CX + side * 24;
		const cy = 117;
		return {
			glow: add(figure, "circle", { cx, cy, r: 26, fill: "url(#ba-eye-glow)", opacity: 0 }),
			ball: add(figure, "path", { fill: "url(#ba-eye)", stroke: "#ffcf4a", "stroke-width": 1.1 }),
			slit: add(figure, "ellipse", { cx, cy, rx: 1.8, ry: 0.3, fill: "#050805" }),
			cx,
			cy,
			side,
		};
	});

	// The cats of her court, who were there all along, at the edge of the light.
	const cats = [];
	for (const [x, k, flip] of [
		[150, 0.86, 1],
		[246, 0.62, 1],
		[754, 0.62, -1],
		[850, 0.86, -1],
	]) {
		const g = add(svg, "g", { transform: `translate(${x} ${FLOOR + 22}) scale(${flip * k} ${k})`, opacity: 0 });
		add(g, "path", { d: "M-16 0C-22 -22 -20 -50 -10 -66L-14 -88L-3 -78Q4 -80 10 -78L20 -88L17 -66C24 -54 24 -28 22 0Z", fill: "#07060b", stroke: "#ffcf4a", "stroke-width": 1 });
		add(g, "path", { d: "M22 -2C40 0 44 -18 34 -30", fill: "none", stroke: "#07060b", "stroke-width": 6, "stroke-linecap": "round" });
		add(g, "circle", { cx: 0, cy: -68, r: 1.8, fill: "#c9ff7a" });
		add(g, "circle", { cx: 10, cy: -68, r: 1.8, fill: "#c9ff7a" });
		cats.push({ g, x });
	}

	// The pyramid's two halves, with their courses of stone.
	const coursed = (d, fill, lines) => {
		const g = add(svg, "g", {});
		add(g, "path", { d, fill, stroke: "#5c3d0c", "stroke-width": 1.4 });
		add(g, "path", { d: lines, fill: "none", stroke: "#5c3d0c", "stroke-width": 1.2, opacity: 0.55 });
		return g;
	};
	let courseL = "";
	let courseR = "";
	for (let i = 1; i < 9; i++) {
		const y = APEX + ((FLOOR - APEX) * i) / 9;
		const half = (HALF * (y - APEX)) / (FLOOR - APEX);
		courseL += `M${num(CX - half)} ${num(y)}L${CX} ${num(y)}`;
		courseR += `M${CX} ${num(y)}L${num(CX + half)} ${num(y)}`;
	}
	const left = coursed(`M${CX} ${APEX}L${CX - HALF} ${FLOOR}L${CX} ${FLOOR}Z`, "url(#ba-gold-l)", courseL);
	const right = coursed(`M${CX} ${APEX}L${CX} ${FLOOR}L${CX + HALF} ${FLOOR}Z`, "url(#ba-gold-r)", courseR);
	const seam = add(svg, "line", { x1: CX, y1: APEX, x2: CX, y2: APEX, stroke: "#fffbe0", "stroke-width": 3, "stroke-linecap": "round", opacity: 0 });
	const motes = [];
	for (let i = 0; i < 44; i++) motes.push({ el: add(svg, "circle", { r: 0.8 + rand() * 2.2, fill: "#ffe27a", opacity: 0 }), x: CX + (rand() - 0.5) * 460, y: 180 + rand() * 230, rate: 12 + rand() * 30, phase: rand() });
	const burst = awakeningBurst(svg, CX, 200, 4.4, ["#fff3b8", "#ffc94a", "#fff6d0"], 0.5);

	function seek(t) {
		const fade = 1 - between(t, BASTET_SECONDS - 0.8, BASTET_SECONDS);
		const fall = smooth(between(t, 0.1, 1.3));
		const shine = smooth(between(t, 1.3, 2.3)); // the line runs down its face
		const part = smooth(between(t, 2.7, 4.3));
		const rise = smooth(between(t, 3, 4.5));
		const awake = smooth(between(t, 4.45, 4.8));
		const cat = smooth(between(t, 4.7, 5.4));

		svg.style.opacity = String(fade);
		scene.setAttribute("opacity", String(fall));
		left.setAttribute("opacity", String(fall));
		right.setAttribute("opacity", String(fall));
		seam.setAttribute("y2", num(lerp(APEX, FLOOR, shine)));
		seam.setAttribute("opacity", String(between(t, 1.3, 1.4) * (1 - between(t, 2.7, 3.1))));
		beam.setAttribute("opacity", String(smooth(between(t, 1.8, 2.5)) * (1 - between(t, 3, 3.5))));

		// Away from each other, sliding, with the shore shaking under them.
		const shift = part * 372;
		const shake = part > 0 && part < 1 ? Math.sin(t * 40) * 1.6 : 0;
		left.setAttribute("transform", `translate(${num(-shift)} ${num(shake)})`);
		right.setAttribute("transform", `translate(${num(shift)} ${num(-shake)})`);
		blaze.setAttribute("rx", num(lerp(12, 430, part)));
		blaze.setAttribute("ry", num(lerp(70, 330, part)));
		blaze.setAttribute("opacity", String(between(t, 1.6, 2.7) * lerp(0.75, 1, part) * (1 - smooth(between(t, 4.7, 5.6)) * 0.45)));
		hush.setAttribute("opacity", String(part * 0.18));

		// She comes up from under the shore, and stops: she does not sway.
		figure.setAttribute("transform", `translate(0 ${num(lerp(300, 0, outCubic(rise)))})`);
		figure.setAttribute("opacity", String(between(t, 3, 3.3)));
		disc.setAttribute("r", num(lerp(20, 86 + Math.sin(t * 1.6) * 2, smooth(between(t, 3.9, 4.8)))));
		earring.setAttribute("cy", num(124 + Math.sin(t * 2.4) * 1.2));
		for (const e of eyes) {
			const w = 13;
			const h2 = 0.3 + awake * 7.5;
			const tilt = -e.side * 3.5 * awake;
			e.ball.setAttribute("d", `M${e.cx - w} ${num(e.cy + tilt)}Q${e.cx} ${num(e.cy - h2 * 1.3)} ${e.cx + w} ${num(e.cy - tilt)}Q${e.cx} ${num(e.cy + h2 * 1.3)} ${e.cx - w} ${num(e.cy + tilt)}Z`);
			e.slit.setAttribute("ry", num(0.3 + awake * 8));
			e.glow.setAttribute("opacity", String(awake * (0.84 + Math.sin(t * 2.6) * 0.16)));
		}
		for (const c of cats) c.g.setAttribute("opacity", String(cat));
		for (const m of motes) {
			const k = (t * 0.2 + m.phase) % 1;
			m.el.setAttribute("cx", num(m.x + Math.sin(k * 9 + m.phase * 6) * 12));
			m.el.setAttribute("cy", num(m.y - k * m.rate * 5));
			m.el.setAttribute("opacity", String(part * Math.sin(k * Math.PI) * 0.85));
		}
		burst(t);
	}
	return { el: svg, seek };
}
