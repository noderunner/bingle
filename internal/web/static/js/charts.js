// Small inline-SVG charts for the batch reports. No dependencies; colours come
// from the stylesheet's custom properties so both themes work.

import { s } from "./util.js";

const FONT = "font-size: 11px; fill: var(--muted)";

function svg(w, h, ...kids) {
	return s("svg", { viewBox: `0 0 ${w} ${h}`, class: "chart", role: "img", preserveAspectRatio: "xMidYMid meet" }, ...kids);
}

function text(x, y, str, attrs = {}) {
	return s("text", { x, y, style: FONT, ...attrs }, str);
}

/** Round "nice" tick values from 0 to max. */
function ticks(max, count = 5) {
	if (max <= 0) return [0, 1];
	const raw = max / count;
	const mag = Math.pow(10, Math.floor(Math.log10(raw)));
	const step = [1, 2, 2.5, 5, 10].map((m) => m * mag).find((v) => v >= raw) || raw;
	const out = [];
	for (let v = 0; v < max + step * 0.999; v += step) out.push(Math.round(v * 1000) / 1000);
	return out;
}

/**
 * Horizontal bars with confidence whiskers.
 * rows: [{label, color, value, lo, hi, note}]; values are fractions (0..1).
 * marker: a reference value drawn as a dashed line (a fair share).
 */
export function rateBars(rows, { marker, width = 640, rowH = 34, domain = 1 } = {}) {
	const left = 130,
		right = 60,
		top = 8,
		bottom = 26;
	const h = top + rows.length * rowH + bottom;
	const x = (v) => left + (Math.min(v, domain) / domain) * (width - left - right);
	const kids = [];
	for (const t of ticks(domain, 5)) {
		if (t > domain) break;
		kids.push(s("line", { x1: x(t), x2: x(t), y1: top, y2: h - bottom, class: "grid" }), text(x(t), h - 8, `${Math.round(t * 100)}%`, { "text-anchor": "middle" }));
	}
	rows.forEach((r, i) => {
		const y = top + i * rowH + 5;
		const mid = y + (rowH - 12) / 2;
		kids.push(
			text(left - 8, y + 13, r.label, { "text-anchor": "end", style: "font-size: 12px; fill: var(--text)" }),
			s("rect", { x: left, y, width: Math.max(1, x(r.value) - left), height: rowH - 12, rx: 3, style: `fill:${r.color}` }, s("title", {}, r.note || "")),
			r.hi !== undefined
				? s("g", { class: "whisker" }, s("line", { x1: x(r.lo), x2: x(r.hi), y1: mid, y2: mid }), s("line", { x1: x(r.lo), x2: x(r.lo), y1: y + 3, y2: y + rowH - 15 }), s("line", { x1: x(r.hi), x2: x(r.hi), y1: y + 3, y2: y + rowH - 15 }))
				: null,
			text(x(Math.max(r.value, r.hi || 0)) + 6, y + 14, `${(r.value * 100).toFixed(1)}%`, { style: "font-size: 12px; fill: var(--text); font-weight: 700" })
		);
	});
	if (marker !== undefined) {
		kids.push(s("line", { x1: x(marker), x2: x(marker), y1: top, y2: h - bottom, class: "marker" }));
	}
	return svg(width, h, ...kids);
}

/**
 * Lines over a shared x axis. series: [{label, color, points: [[x, y], ...]}].
 */
export function lineChart(series, { width = 640, height = 280, xLabel = "", yLabel = "", yMax } = {}) {
	const left = 44,
		right = 14,
		top = 12,
		bottom = 36;
	const xs = series.flatMap((l) => l.points.map((p) => p[0]));
	const ys = series.flatMap((l) => l.points.map((p) => p[1]));
	if (!xs.length) return svg(width, 60, text(10, 30, "No data yet."));
	const x0 = Math.min(...xs),
		x1 = Math.max(...xs, x0 + 1);
	const topY = yMax !== undefined ? yMax : Math.max(...ys, 1);
	const yt = ticks(topY, 5);
	const ymax = yt[yt.length - 1];
	const X = (v) => left + ((v - x0) / (x1 - x0)) * (width - left - right);
	const Y = (v) => height - bottom - (v / ymax) * (height - top - bottom);
	const kids = [];
	for (const t of yt) kids.push(s("line", { x1: left, x2: width - right, y1: Y(t), y2: Y(t), class: "grid" }), text(left - 6, Y(t) + 4, String(t), { "text-anchor": "end" }));
	const step = Math.max(1, Math.ceil((x1 - x0 + 1) / 12));
	for (let v = x0; v <= x1; v += step) kids.push(text(X(v), height - bottom + 15, String(v), { "text-anchor": "middle" }));
	if (xLabel) kids.push(text((left + width - right) / 2, height - 4, xLabel, { "text-anchor": "middle" }));
	if (yLabel) kids.push(text(12, top + 10, yLabel));
	for (const l of series) {
		if (!l.points.length) continue;
		const d = l.points.map((p, i) => `${i ? "L" : "M"}${X(p[0]).toFixed(1)},${Y(p[1]).toFixed(1)}`).join(" ");
		kids.push(s("path", { d, class: "line", style: `stroke:${l.color}` }, s("title", {}, l.label)));
		for (const p of l.points) kids.push(s("circle", { cx: X(p[0]), cy: Y(p[1]), r: 2.6, style: `fill:${l.color}` }, s("title", {}, `${l.label}: ${p[1].toFixed(2)} (round ${p[0]})`)));
	}
	return svg(width, height, ...kids);
}

/** Vertical bars. bars: [{label, value, color}]. */
export function columnChart(bars, { width = 640, height = 220, yLabel = "" } = {}) {
	const left = 40,
		right = 10,
		top = 12,
		bottom = 30;
	const yt = ticks(Math.max(1, ...bars.map((b) => b.value)), 4);
	const ymax = yt[yt.length - 1];
	const bw = (width - left - right) / Math.max(1, bars.length);
	const Y = (v) => height - bottom - (v / ymax) * (height - top - bottom);
	const kids = [];
	for (const t of yt) kids.push(s("line", { x1: left, x2: width - right, y1: Y(t), y2: Y(t), class: "grid" }), text(left - 6, Y(t) + 4, String(t), { "text-anchor": "end" }));
	bars.forEach((b, i) => {
		kids.push(
			s("rect", { x: left + i * bw + 2, y: Y(b.value), width: Math.max(1, bw - 4), height: height - bottom - Y(b.value), rx: 2, style: `fill:${b.color || "var(--accent)"}` }, s("title", {}, `${b.label}: ${b.value}`)),
			bars.length <= 14 || i % 2 === 0 ? text(left + i * bw + bw / 2, height - bottom + 14, b.label, { "text-anchor": "middle" }) : null
		);
	});
	if (yLabel) kids.push(text(12, top + 8, yLabel));
	return svg(width, height, ...kids);
}
