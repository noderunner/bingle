// Ithaqua, the Wind-Walker, coming out of the blizzard: the map whites over, snow drives across it, and a shape
// too tall and too thin strides on through the white with its mane streaming, until one foot comes down and two
// red stars open high above. Original art, drawn in code (see js/cw-art-kit.js).

import { H, W, add, awakeningBurst, between, curve, limb, lerp, num, outCubic, randoms, round, sceneSvg, smooth, vertical, writhe } from "./cw-art-kit.js";
import { s } from "./util.js";

export const ITHAQUA_SECONDS = 7.2;
export const ITHAQUA_PEAK = 5.5;

const CX = 500;
const GROUND = 452;

const wrap = (x, size) => ((x % size) + size) % size;

export function ithaquaScene() {
	const rand = randoms(1933); // the year of "The Thing That Walked on the Wind"
	const svg = sceneSvg();
	svg.append(
		s(
			"defs",
			{},
			vertical("it-sky", [
				[0, "#26364c"],
				[0.5, "#6d869c"],
				[1, "#c5d4dd"],
			]),
			vertical("it-ground", [
				[0, "#e4edf2"],
				[1, "#90a7b8"],
			]),
			round("it-gust", [
				[0, "#eef4f8", 0.55],
				[1, "#eef4f8", 0],
			]),
			vertical("it-body", [
				[0, "#0a1020"],
				[0.55, "#172339"],
				[1, "#3a5068"],
			]),
			vertical("it-limb", [
				[0, "#0c1324"],
				[1, "#2c3d55"],
			]),
			vertical("it-mane", [
				[0, "#0e1527"],
				[1, "#7a93a8"],
			]),
			round("it-eye", [
				[0, "#fff3e6"],
				[0.4, "#ff4b30"],
				[1, "#7c0d0a"],
			]),
			round("it-eye-glow", [
				[0, "#ff5a38", 0.9],
				[0.35, "#c61f18", 0.4],
				[1, "#5c0808", 0],
			])
		)
	);
	const sky = add(svg, "g", { opacity: 0 });
	add(sky, "rect", { x: 0, y: 0, width: W, height: H, fill: "url(#it-sky)" });
	// Peaks that come and go in the white.
	add(sky, "path", { d: "M0 400L90 300L150 356L250 250L340 372L430 318L520 392L640 286L730 362L836 268L922 358L1000 306L1000 470L0 470Z", fill: "#6f8ba1", opacity: 0.55 });
	add(sky, "path", { d: `M0 ${GROUND}C170 ${GROUND - 22} 340 ${GROUND + 8} 520 ${GROUND - 12}C690 ${GROUND - 26} 850 ${GROUND + 6} ${W} ${GROUND - 14}L${W} ${H}L0 ${H}Z`, fill: "url(#it-ground)" });

	const backSnow = add(svg, "g", {});
	const flakes = (parent, n, speed, size) => {
		const out = [];
		for (let i = 0; i < n; i++) out.push({ el: add(parent, "line", { stroke: "#fff", "stroke-width": size * (0.6 + rand() * 0.9), "stroke-linecap": "round", opacity: 0.4 + rand() * 0.55 }), x: rand() * 1200, y: rand() * 620, speed: speed * (0.7 + rand() * 0.6), wob: rand() * 6.28 });
		return out;
	};
	const far = flakes(backSnow, 120, 500, 1.1);

	// He: long legs in stride, a narrow trunk, arms to his knees and a mane the wind takes. Drawn at his full height.
	const figure = add(svg, "g", {});
	const mane = [];
	for (let i = 0; i < 34; i++) {
		const across = (i - 16.5) / 16.5;
		mane.push({ el: add(figure, "path", { fill: "url(#it-mane)", opacity: 0.94 }), root: [CX + across * 92, 62 + Math.abs(across) * 86 + rand() * 18], lean: -1.5 + rand() * 0.5 - across * 0.2, length: 240 + rand() * 330, width: 40 - Math.abs(across) * 14 + rand() * 10, phase: rand() * 6.28, rate: 2.2 + rand() * 1.4 });
	}
	const legs = [0, 1].map(() => add(figure, "path", { fill: "url(#it-limb)" }));
	const feet = [0, 1].map(() => add(figure, "path", { fill: "#0b1222" }));
	// The trunk: wide high shoulders over a starved waist, all one shadow, with a ragged edge of fur.
	add(figure, "path", {
		d: "M392 142C414 112 462 100 500 100C538 100 586 112 608 142C600 166 580 176 568 190C556 222 548 244 542 276L552 300L532 294L522 312L506 296L490 314L478 294L458 302L466 274C458 244 446 222 432 190C420 176 400 166 392 142Z",
		fill: "url(#it-body)",
	});
	add(figure, "path", { d: "M452 160C474 172 526 172 548 160M458 186C478 197 522 197 542 186M466 212C484 221 516 221 534 212M474 238C488 244 512 244 526 238", fill: "none", stroke: "#3c5570", "stroke-width": 2, opacity: 0.4 });
	const arms = [0, 1].map(() => ({ el: add(figure, "path", { fill: "url(#it-limb)" }), claws: add(figure, "path", { fill: "none", stroke: "#0b1222", "stroke-width": 4.5, "stroke-linecap": "round" }) }));
	// A head with hardly a face, in the mane, low between the shoulders.
	add(figure, "path", { d: "M462 138C448 92 470 44 500 38C530 44 552 92 538 138C526 160 474 160 462 138Z", fill: "#070c18" });
	const eyes = [-1, 1].map((side) => ({
		glow: add(figure, "circle", { cx: CX + side * 17, cy: 92, r: 56, fill: "url(#it-eye-glow)", opacity: 0 }),
		ball: add(figure, "circle", { cx: CX + side * 17, cy: 92, r: 0.3, fill: "url(#it-eye)" }),
	}));

	const kick = [];
	for (let i = 0; i < 46; i++) kick.push({ el: add(svg, "circle", { r: 1.5 + rand() * 4, fill: "#fff", opacity: 0 }), a: rand() * 3.14, power: 90 + rand() * 300 });
	const gusts = [];
	for (let i = 0; i < 5; i++) gusts.push({ el: add(svg, "ellipse", { fill: "url(#it-gust)" }), y: 60 + i * 86, rate: 170 + rand() * 170, off: rand() * 1500, ry: 50 + rand() * 60 });
	const frontSnow = add(svg, "g", {});
	const near = flakes(frontSnow, 110, 880, 2.1);
	const whiteout = add(svg, "rect", { x: 0, y: 0, width: W, height: H, fill: "#dfe9ef", opacity: 0 });
	const burst = awakeningBurst(svg, CX, GROUND - 10, 4.4, ["#f4fbff", "#ff5a38", "#ffffff"], 0.55);

	const flakeTo = (list, t, gale) => {
		for (const f of list) {
			const run = t * f.speed * gale;
			const x = wrap(f.x - run, 1200) - 100;
			const y = wrap(f.y + run * 0.28 + Math.sin(t * 2 + f.wob) * 8, 620) - 60;
			const tail = 10 + f.speed * gale * 0.045;
			f.el.setAttribute("x1", num(x));
			f.el.setAttribute("y1", num(y));
			f.el.setAttribute("x2", num(x + tail));
			f.el.setAttribute("y2", num(y - tail * 0.28));
		}
	};

	function seek(t) {
		const fade = 1 - between(t, ITHAQUA_SECONDS - 0.8, ITHAQUA_SECONDS);
		const fall = smooth(between(t, 0.1, 1.2));
		const come = between(t, 1.5, 4.4); // he walks up out of the white
		const near2 = come ** 2.4;
		const landed = smooth(between(t, 4.32, 4.42));
		const awake = smooth(between(t, 4.45, 4.85));
		const gale = 0.75 + smooth(between(t, 0.8, 2.4)) * 0.45;

		svg.style.opacity = String(fade);
		sky.setAttribute("opacity", String(fall));
		backSnow.setAttribute("opacity", String(fall));
		frontSnow.setAttribute("opacity", String(fall));
		flakeTo(far, t, gale);
		flakeTo(near, t, gale);
		// The storm thickens, then he tears a hole in it.
		whiteout.setAttribute("opacity", String(smooth(between(t, 0.8, 1.9)) * (0.78 - smooth(between(t, 2.4, 4.4)) * 0.76)));
		for (const g of gusts) {
			g.el.setAttribute("cx", num(wrap(g.off - t * g.rate * 2, 1700) - 350));
			g.el.setAttribute("cy", num(g.y));
			g.el.setAttribute("rx", "340");
			g.el.setAttribute("ry", num(g.ry));
			g.el.setAttribute("opacity", String(fall * (0.72 - awake * 0.3)));
		}

		// A stride: the legs swing while he comes on, and stand planted once he has come.
		const swing = Math.sin(come * 11) * (1 - landed);
		const scale = lerp(0.2, 1.06, near2);
		const sway = Math.sin(come * 11) * 8 * (1 - landed) + Math.sin(t * 0.9) * 3;
		figure.setAttribute("transform", `translate(${num(CX + sway)} ${GROUND}) scale(${scale.toFixed(3)}) translate(${-CX} ${-GROUND + 4})`);
		figure.setAttribute("opacity", String(smooth(between(t, 1.7, 2.8)) * lerp(0.45, 1, near2)));

		legs.forEach((leg, i) => {
			const side = i ? 1 : -1;
			const stride = side * (34 + swing * side * 52);
			const hip = [CX + side * 26, 260];
			const lift = Math.max(0, swing * side) * 34;
			const knee = [CX + side * 40 + stride * 0.52, 350 - lift * 0.4];
			const foot = [CX + side * 34 + stride, 446 - lift];
			leg.setAttribute("d", limb(curve(hip, [knee[0] + side * 26, knee[1] - 30], foot, 14), (k) => lerp(50, 10, k ** 0.7)));
			const dir = side;
			feet[i].setAttribute("d", `M${num(foot[0] - dir * 12)} ${num(foot[1] - 8)}C${num(foot[0] + dir * 22)} ${num(foot[1] - 12)} ${num(foot[0] + dir * 58)} ${num(foot[1] - 4)} ${num(foot[0] + dir * 72)} ${num(foot[1] + 8)}L${num(foot[0] + dir * 40)} ${num(foot[1] + 4)}L${num(foot[0] + dir * 34)} ${num(foot[1] + 10)}L${num(foot[0] + dir * 8)} ${num(foot[1] + 8)}Z`);
		});
		arms.forEach((arm, i) => {
			const side = i ? 1 : -1;
			const raise = awake * 1;
			const shoulder = [CX + side * 92, 146];
			const elbow = [CX + side * lerp(136, 216, raise) - swing * side * 16, lerp(270, 170, raise)];
			const hand = [CX + side * lerp(118, 330, raise) - swing * side * 30, lerp(392, 74 + Math.sin(t * 1.5 + i) * 9, raise)];
			arm.el.setAttribute("d", limb(curve(shoulder, elbow, hand, 14), (k) => lerp(38, 9, k ** 0.75)));
			let claws = "";
			for (let j = -2; j <= 2; j++) {
				const a = Math.atan2(hand[1] - elbow[1], hand[0] - elbow[0]) + j * 0.3;
				const b = a + j * 0.16 + side * 0.4;
				const mx = hand[0] + Math.cos(a) * 26;
				const my = hand[1] + Math.sin(a) * 26;
				claws += `M${num(hand[0])} ${num(hand[1])}Q${num(mx)} ${num(my)} ${num(mx + Math.cos(b) * 34)} ${num(my + Math.sin(b) * 34)}`;
			}
			arm.claws.setAttribute("d", claws);
		});
		for (const m of mane) {
			// All of it goes one way: the wind's.
			const points = writhe(m.root, m.lean, m.length, 12, t, m.rate, m.phase, 0.42);
			m.el.setAttribute("d", limb(points, (k) => lerp(m.width, 1.2, k)));
		}
		for (const e of eyes) {
			e.ball.setAttribute("r", num(0.3 + awake * 10));
			e.glow.setAttribute("opacity", String(awake * (0.84 + Math.sin(t * 7) * 0.16)));
		}
		for (const k of kick) {
			const go = between(t, 4.35, 5.5);
			const out = outCubic(go);
			k.el.setAttribute("cx", num(CX + Math.cos(k.a) * k.power * 1.4 * out));
			k.el.setAttribute("cy", num(GROUND - Math.sin(k.a) * k.power * 0.7 * out + go * go * 110));
			k.el.setAttribute("opacity", String(go > 0 && go < 1 ? (1 - go) * 0.95 : 0));
		}
		burst(t);
	}
	return { el: svg, seek };
}
