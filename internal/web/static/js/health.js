// The Health page, served from the private metrics listener only.
//
// It is a reader of /metrics/history, drawn with the same charts the batch
// reports use. Nothing here is on the public site, and nothing it shows is
// about a person: it is the server's own account of itself.

import { h, clear } from "./util.js";
import { lineChart, columnChart } from "./charts.js";

const root = document.getElementById("health-root");
const windowHours = Number(root?.dataset.windowHours || 24);

const COLORS = ["var(--accent)", "var(--warn, #c86)", "var(--ok, #6a8)", "var(--muted)"];

const num = (v, d = 1) => (Math.abs(v - Math.round(v)) < 1e-9 ? String(Math.round(v)) : v.toFixed(d));

/** Bytes as something a person reads. */
function bytes(v) {
	const units = ["B", "kB", "MB", "GB"];
	let i = 0;
	while (v >= 1024 && i < units.length - 1) {
		v /= 1024;
		i++;
	}
	return `${num(v, i === 0 ? 0 : 1)} ${units[i]}`;
}

async function getJSON(path) {
	const resp = await fetch(path, { headers: { Accept: "application/json" } });
	if (!resp.ok) throw new Error(`${path}: ${resp.status}`);
	return resp.json();
}

/** One series' history, as [{labels, label, points}]. */
async function history(name, { hours = windowHours, labels } = {}) {
	const q = new URLSearchParams({ name, hours: String(hours) });
	if (labels) q.set("labels", labels);
	const body = await getJSON(`/metrics/history?${q}`);
	return body.series || [];
}

/**
 * A counter is stored as its monotonic total, so a rate is the difference
 * between neighbouring points over the time between them — and a restart,
 * which sends the total back to zero, is dropped rather than drawn as a
 * cliff. This is what Prometheus does on read, for the same reason.
 */
function perMinute(points) {
	const out = [];
	for (let i = 1; i < points.length; i++) {
		const dt = (new Date(points[i].at) - new Date(points[i - 1].at)) / 1000;
		const dv = points[i].value - points[i - 1].value;
		if (dt <= 0 || dv < 0) continue; // a restart, or two samples at once
		out.push([new Date(points[i].at).getTime() / 1000, (dv / dt) * 60]);
	}
	return out;
}

/** The last value of a series, or undefined. */
function latest(series) {
	const points = series?.[0]?.points || [];
	return points.length ? points[points.length - 1].value : undefined;
}

/** Sum the last value of every label set of a series. */
function totalLatest(series) {
	let sum = 0;
	for (const s of series) {
		const p = s.points;
		if (p.length) sum += p[p.length - 1].value;
	}
	return sum;
}

/** Hours of x-axis labels read as clock times rather than unix seconds. */
function clockLines(series) {
	return series.map((s, i) => ({
		label: s.label,
		color: COLORS[i % COLORS.length],
		points: s.points.map(([x, y]) => [Math.round((x - series[0].points[0]?.[0] || 0) / 60), y]),
	}));
}

function card(title, ...kids) {
	return h("section", { class: "card" }, h("h2", {}, title), ...kids);
}

function statRow(stats) {
	return h(
		"div",
		{ class: "stats" },
		...stats.map(([label, value]) => h("div", { class: "stat" }, h("span", { class: "stat-value" }, value), h("span", { class: "stat-label" }, label)))
	);
}

/** p95 from a histogram's buckets: the first bucket holding 95% of the count. */
function p95(bucketSeries, count) {
	if (!count) return undefined;
	const want = count * 0.95;
	const edges = bucketSeries
		.map((s) => ({ le: Number(s.label?.le), value: latest([s]) || 0 }))
		.filter((b) => Number.isFinite(b.le))
		.sort((a, b) => a.le - b.le);
	for (const b of edges) {
		if (b.value >= want) return b.le;
	}
	return edges.length ? edges[edges.length - 1].le : undefined;
}

async function render() {
	clear(root);
	root.append(h("p", { class: "muted" }, "Reading the history…"));

	let meta;
	try {
		meta = await getJSON("/metrics/series");
	} catch (err) {
		clear(root);
		root.append(h("p", { class: "error" }, `The history could not be read: ${err.message}`));
		return;
	}

	const have = new Set(meta.names || []);
	const pick = async (name, opts) => (have.has(name) ? history(name, opts) : []);

	const [requests, errors, duration, durCount, dbFile, dbWal, tableRows, moves, finished, slowCount] = await Promise.all([
		pick("bingle_http_requests_total"),
		pick("bingle_http_errors_total"),
		pick("bingle_http_request_duration_seconds_bucket"),
		pick("bingle_http_request_duration_seconds_count"),
		pick("bingle_db_file_bytes"),
		pick("bingle_db_wal_bytes"),
		pick("bingle_db_table_rows"),
		pick("bingle_game_moves_total"),
		pick("bingle_games_finished_total"),
		pick("bingle_db_queries_total"),
	]);

	clear(root);

	// What the figures say right now.
	const rows = Object.fromEntries(tableRows.map((s) => [s.label?.table, latest([s])]));
	root.append(
		card(
			"Now",
			statRow([
				["games", num(rows.games ?? 0, 0)],
				["moves", num(rows.moves ?? 0, 0)],
				["database", bytes(latest(dbFile) ?? 0)],
				["write-ahead log", bytes(latest(dbWal) ?? 0)],
				["requests answered", num(totalLatest(requests), 0)],
				["refused or failed", num(totalLatest(errors), 0)],
			]),
			h("p", { class: "muted" }, `History every ${meta.every}, full resolution for ${meta.retain_days} days. ` + `${num(meta.rows?.samples ?? 0, 0)} samples, ${num((meta.rows?.rollups_hour ?? 0) + (meta.rows?.rollups_day ?? 0), 0)} rollups, ${num(meta.rows?.slow_requests ?? 0, 0)} slow requests.`)
		)
	);

	// Requests a minute, by route, busiest first.
	const busiest = requests
		.map((s) => ({ s, rate: perMinute(s.points) }))
		.filter((r) => r.rate.length)
		.sort((a, b) => (b.rate.at(-1)?.[1] || 0) - (a.rate.at(-1)?.[1] || 0))
		.slice(0, 4);
	if (busiest.length) {
		const series = busiest.map((r, i) => ({
			label: `${r.s.label?.method || ""} ${r.s.label?.route || ""}`.trim(),
			color: COLORS[i % COLORS.length],
			points: r.rate,
		}));
		root.append(card("Requests a minute, busiest routes", lineChart(clockLines(series), { xLabel: `minutes over the last ${windowHours}h`, yLabel: "per minute" })));
	}

	// p95 by route, from the histogram's buckets.
	const byRoute = new Map();
	for (const s of duration) {
		const route = s.label?.route || "";
		if (!byRoute.has(route)) byRoute.set(route, []);
		byRoute.get(route).push(s);
	}
	const bars = [];
	for (const [route, buckets] of byRoute) {
		const count = latest(durCount.filter((c) => c.label?.route === route));
		const v = p95(buckets, count);
		if (v !== undefined) bars.push({ label: route, value: v * 1000, color: COLORS[0] });
	}
	bars.sort((a, b) => b.value - a.value);
	if (bars.length) {
		root.append(card("Slowest routes (95th percentile, ms)", columnChart(bars.slice(0, 8), { yLabel: "ms" })));
	}

	// The database, which is the thing that grows.
	if (dbFile.length || dbWal.length) {
		const toMB = (points) => points.map((p) => [new Date(p.at).getTime() / 1000, p.value / 1024 / 1024]);
		const series = [
			{ label: "database", color: COLORS[0], points: toMB(dbFile[0]?.points || []) },
			{ label: "write-ahead log", color: COLORS[1], points: toMB(dbWal[0]?.points || []) },
		].filter((s) => s.points.length);
		if (series.length) {
			root.append(card("Database on disk", lineChart(clockLines(series), { xLabel: `minutes over the last ${windowHours}h`, yLabel: "MB" })));
		}
	}

	// The busiest statements, which is where a slow page usually turns out to be.
	const statements = slowCount
		.filter((s) => s.label?.result === "ok")
		.map((s) => ({ label: s.label?.stmt || "", value: latest([s]) || 0, color: COLORS[2] }))
		.sort((a, b) => b.value - a.value)
		.slice(0, 8);
	if (statements.length) {
		root.append(card("Busiest statements", columnChart(statements, { yLabel: "run" })));
	}

	// How games are ending, which is the early warning that a rule has broken.
	const endings = finished
		.map((s) => ({ label: `${s.label?.game || ""} ${s.label?.end_reason || ""}`.trim(), value: latest([s]) || 0, color: COLORS[1] }))
		.filter((b) => b.value > 0)
		.sort((a, b) => b.value - a.value)
		.slice(0, 8);
	if (endings.length) {
		root.append(card("How games ended", columnChart(endings, { yLabel: "games" })));
	}

	// Moves refused by the rules: a rise means a stale page or a rules change.
	const illegal = moves.filter((s) => s.label?.result === "illegal");
	if (illegal.length) {
		const series = illegal.map((s, i) => ({ label: s.label?.game || "", color: COLORS[i % COLORS.length], points: perMinute(s.points) })).filter((s) => s.points.length);
		if (series.length) {
			root.append(card("Moves refused by the rules, a minute", lineChart(clockLines(series), { xLabel: `minutes over the last ${windowHours}h`, yLabel: "per minute" })));
		}
	}

	if (root.childElementCount === 1) {
		root.append(h("p", { class: "muted" }, "There is no history yet. The first sample is written one interval after the server starts."));
	}

	root.append(
		h(
			"p",
			{ class: "muted" },
			h("a", { class: "button ghost", href: `/metrics/history.csv?name=bingle_http_requests_total&hours=${windowHours}` }, "Download requests as CSV"),
			" ",
			h(
				"button",
				{
					class: "button ghost",
					onclick: async (e) => {
						e.target.disabled = true;
						try {
							await fetch("/metrics/sample", { method: "POST" });
							await render();
						} finally {
							e.target.disabled = false;
						}
					},
				},
				"Sample now"
			)
		)
	);
}

if (root) {
	render().catch((err) => {
		clear(root);
		root.append(h("p", { class: "error" }, `The health page failed: ${err.message}`));
	});
}
