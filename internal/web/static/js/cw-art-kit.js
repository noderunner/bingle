// What the scenes of the Great Old Ones are drawn with (ADR 0108). A scene is an SVG and a seek(t) that
// draws it as it stands t seconds in: one clock drives everything, nothing is random from one showing to
// the next, and a still picture is only seek() at one moment.

import { s } from "./util.js";

export const W = 1000;
export const H = 500;

export const clamp = (x) => Math.max(0, Math.min(1, x));
export const between = (t, from, to) => clamp((t - from) / (to - from));
export const outCubic = (x) => 1 - (1 - x) ** 3;
export const smooth = (x) => x * x * (3 - 2 * x);
export const lerp = (a, b, k) => a + (b - a) * k;
export const num = (x) => x.toFixed(1);

/** The same numbers at every showing. */
export function randoms(seed) {
	let v = seed;
	return () => {
		v = (v * 1664525 + 1013904223) % 4294967296;
		return v / 4294967296;
	};
}

/** A smooth closed outline around a spine of points; width(k) is its width k of the way along. */
export function limb(points, width) {
	const left = [];
	const right = [];
	const last = points.length - 1;
	points.forEach((p, i) => {
		const a = points[Math.max(0, i - 1)];
		const b = points[Math.min(last, i + 1)];
		const dx = b[0] - a[0];
		const dy = b[1] - a[1];
		const len = Math.hypot(dx, dy) || 1;
		const w = width(i / last) / 2;
		left.push([p[0] - (dy / len) * w, p[1] + (dx / len) * w]);
		right.unshift([p[0] + (dy / len) * w, p[1] - (dx / len) * w]);
	});
	const all = left.concat(right);
	let d = `M${num(all[0][0])} ${num(all[0][1])}`;
	for (let i = 1; i < all.length - 1; i++) {
		d += `Q${num(all[i][0])} ${num(all[i][1])} ${num((all[i][0] + all[i + 1][0]) / 2)} ${num((all[i][1] + all[i + 1][1]) / 2)}`;
	}
	return `${d}Z`;
}

/** Points along a curve through a, b and c. */
export function curve(a, b, c, n = 12) {
	const out = [];
	for (let i = 0; i <= n; i++) {
		const k = i / n;
		const m = 2 * (1 - k) * k;
		out.push([(1 - k) ** 2 * a[0] + m * b[0] + k * k * c[0], (1 - k) ** 2 * a[1] + m * b[1] + k * k * c[1]]);
	}
	return out;
}

/** A writhing spine: from a root, leaning, with a wave that grows along it. */
export function writhe(root, lean, length, steps, t, rate, phase, curl) {
	const points = [];
	let [x, y] = root;
	const stepLen = length / steps;
	for (let i = 0; i <= steps; i++) {
		points.push([x, y]);
		const k = i / steps;
		const angle = lean + Math.sin(k * 5.5 - t * rate + phase) * (curl * (0.2 + k * k));
		x += Math.sin(angle) * stepLen;
		y += Math.cos(angle) * stepLen;
	}
	return points;
}

/** Scales: many small arcs, one path. */
export function scales(rand, x0, x1, y0, y1, count) {
	let d = "";
	for (let i = 0; i < count; i++) {
		const x = x0 + rand() * (x1 - x0);
		const y = y0 + rand() * (y1 - y0);
		const r = 2.5 + rand() * 4;
		d += `M${num(x - r)} ${num(y)}q${num(r)} ${num(r * 0.9)} ${num(r * 2)} 0`;
	}
	return d;
}

/** A small round mark, as part of one path. */
export const dot = (x, y, r) => `M${num(x - r)} ${num(y)}a${num(r)} ${num(r)} 0 1 0 ${num(r * 2)} 0a${num(r)} ${num(r)} 0 1 0 ${num(-r * 2)} 0`;

export const stop = (offset, color, opacity = 1) => s("stop", { offset, "stop-color": color, "stop-opacity": opacity });

/** A vertical gradient. stops is [offset, colour, opacity?] for each. */
export const vertical = (id, stops) => s("linearGradient", { id, x1: "0", y1: "0", x2: "0", y2: "1" }, ...stops.map((x) => stop(...x)));

/** A round gradient, from its middle out. */
export const round = (id, stops, cx = "0.5", cy = "0.5", r = "0.5") => s("radialGradient", { id, cx, cy, r }, ...stops.map((x) => stop(...x)));

/** Adds a new element to a parent, and returns it. */
export function add(parent, tag, attrs) {
	const el = s(tag, attrs);
	parent.append(el);
	return el;
}

/** The empty scene. */
export const sceneSvg = () => s("svg", { class: "em-scene", viewBox: `0 0 ${W} ${H}`, preserveAspectRatio: "xMidYMax slice", "aria-hidden": "true" });

/** Lightning: the bolt that is lit at t, and how bright. bolts is [when, path]. */
export function lightning(bolts, t) {
	let lit = 0;
	let path = "";
	for (const [at, d] of bolts) {
		const k = (t - at) / 0.26;
		if (k >= 0 && k <= 1) {
			lit = Math.max(lit, (1 - k) * (k < 0.3 || k > 0.5 ? 1 : 0.3));
			path = d;
		}
	}
	return { lit, path };
}
