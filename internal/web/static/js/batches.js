// Practice-batch pages: the list at /batches and a batch's report at
// /batches/{id} — charts and tables drawn from GET /api/batches/{id}/report.
// What every game has is drawn here; a game whose page script reports more
// (Cthulhu Wars' cults, ADR 0124) adds its own sections through batchSections,
// and names its picks and endings with pickName, pickColor and endReason.

import { h, clear } from "./util.js";
import { api } from "./api.js";
import { columnChart } from "./charts.js";

const root = document.getElementById("batch-root");
const batchID = root ? root.dataset.batchId : "";

const pct = (v, d = 1) => `${(v * 100).toFixed(d)}%`;
const num = (v, d = 1) => (Math.abs(v - Math.round(v)) < 1e-9 ? String(Math.round(v)) : v.toFixed(d));

const STATUS_TEXT = { queued: "Queued", running: "Running", done: "Finished", cancelled: "Cancelled", failed: "Failed", interrupted: "Interrupted" };

// ---- sections --------------------------------------------------------------------

function summaryStats(rep, info) {
	const mins = info.games_per_minute;
	return h(
		"div",
		{ class: "batch-stat-row" },
		stat("Games", `${rep.games}`),
		stat("Decided", rep.games ? `${rep.decided} (${pct(rep.decided / rep.games, 0)})` : "–"),
		stat("Plies / game", rep.games ? num(rep.plies.mean, 0) : "–"),
		stat("Rounds / game", rep.games ? num(rep.rounds.mean, 1) : "–"),
		mins ? stat("Speed", `${num(mins, 1)} games/min`) : null,
		...Object.entries(rep.end_reasons || {}).map(([k, v]) => stat(k === "" ? "unfinished" : endReason(k), `${v}`))
	);
}

// The batch's game's page script, once its info has come.
let page = {};
const pickName = (k) => (k && page.pickName ? page.pickName(k) : k || "");
const pickColor = (k) => (k && page.pickColor ? page.pickColor(k) : "transparent");
const endReason = (k) => (page.endReason ? page.endReason(k) : k);

function stat(label, value) {
	return h("div", { class: "batch-stat" }, h("b", {}, value), h("span", { class: "muted small" }, label));
}

function lengthCard(rep) {
	const b = rep.ply_buckets;
	const bars = b.counts.map((c, i) => ({ label: String(b.lo + i * b.width), value: c }));
	return h("section", { class: "card" }, h("h2", {}, "Game length"), h("p", { class: "muted small" }, `Plies (single decisions) per game: median ${num(rep.plies.p50, 0)}, 90% under ${num(rep.plies.p90, 0)}, longest ${num(rep.plies.max, 0)}.`), columnChart(bars, { yLabel: "games" }), h("div", { class: "muted small" }, `Bars start at the labelled ply count and are ${b.width} plies wide.`));
}

// ---- games ------------------------------------------------------------------------------

let games = null;
let openGame = -1;
let openEvents = null;

function gamesCard(info) {
	const rows = (games || []).map((g) => {
		const w = g.winner ? g.seating[g.winner - 1] : "";
		const open = openGame === g.index;
		return h(
			"tr",
			{},
			h("td", {}, `#${g.index + 1}`),
			h("td", {}, h("span", { class: "dots" }, g.seating.map((k) => h("i", { style: `background:${pickColor(k)}` }))), " ", g.seating.map((k) => k.toUpperCase()).join(" ")),
			h("td", {}, w ? pickName(w) : g.winner ? `Seat ${g.winner}` : "–"),
			h("td", {}, endReason(g.end_reason)),
			h("td", {}, String(g.plies)),
			h("td", {}, String(g.rounds)),
			h("td", {}, g.index < 20 ? h("button", { type: "button", class: "ghost", "data-game": String(g.index), onclick: () => toggleGame(g.index) }, open ? "Hide moves" : "Moves") : "")
		);
	});
	return h(
		"section",
		{ class: "card", id: "games-card" },
		h("h2", {}, "Games"),
		h("p", { class: "muted small" }, "Seats are listed in order of play for each game. The decisions of the first twenty games are kept."),
		h("div", { class: "table-scroll" }, h("table", { class: "data" }, h("thead", {}, h("tr", {}, ["Game", "Seating", "Winner", "End", "Plies", "Rounds", ""].map((t) => h("th", {}, t)))), h("tbody", {}, rows))),
		openGame >= 0 && openEvents ? h("div", { class: "batch-events", id: "game-events" }, openEvents.map((e) => h("div", {}, `${e.ply}. s${e.seat} ${e.move}`))) : null
	);
}

async function toggleGame(i) {
	if (openGame === i) {
		openGame = -1;
		openEvents = null;
		return renderReport();
	}
	openGame = i;
	openEvents = null;
	try {
		openEvents = (await api.batchEvents(batchID, i)).events || [];
	} catch (_) {
		openEvents = [];
	}
	renderReport();
}

// ---- the page -------------------------------------------------------------------------------

let data = null;
let compare = null; // {info, report} of the batch compared with
let allBatches = [];
let timer = null;

function headerCard(info) {
	const running = info.status === "queued" || info.status === "running";
	const frac = info.games ? info.done / info.games : 0;
	const remaining = running && info.games_per_minute ? (info.games - info.done) / info.games_per_minute : 0;
	return h(
		"section",
		{ class: "card", id: "batch-head" },
		h("div", { class: "row" },
			h("h1", { style: "margin:0" }, info.name || `Batch ${info.id}`),
			h("span", { class: `batch-status ${info.status}`, id: "batch-status" }, STATUS_TEXT[info.status] || info.status),
			h("span", { class: "spacer" }),
			running ? h("button", { type: "button", id: "batch-cancel", onclick: async () => { await api.cancelBatch(info.id); load(); } }, "Cancel") : null,
			h("button", { type: "button", class: "danger", id: "batch-delete", onclick: async () => { if (confirm("Delete this batch and its games?")) { await api.deleteBatch(info.id); window.location.href = "/batches"; } } }, "Delete")
		),
		h("div", { class: "progress", style: "margin:0.6rem 0" }, h("div", { style: `width:${(frac * 100).toFixed(1)}%` })),
		h("div", { class: "muted small" },
			`${info.done} of ${info.games} games`,
			running && remaining ? ` · about ${remaining < 1 ? "under a minute" : Math.round(remaining) + " min"} left` : "",
			info.error ? ` · ${info.error}` : "",
			" · ", info.spec.players.map((p, i) => `${pickName(p.faction) || `Seat ${i + 1}`}: ${p.bot}${p.mcts_iterations ? " " + p.mcts_iterations : ""}`).join(", "),
			` · seed ${info.spec.seed}${info.spec.rotate ? ", seats rotated" : ""}`,
			...(info.spec.options || []).map((o) => `, ${page.optionName ? page.optionName(o) : o}`)
		),
		compareControl(info)
	);
}

function compareControl(info) {
	const others = allBatches.filter((b) => b.id !== info.id && b.done > 0);
	if (!others.length) return null;
	const sel = h(
		"select",
		{ id: "compare-with", onchange: async (e) => { compare = e.target.value ? await api.batchReport(e.target.value) : null; renderReport(); } },
		h("option", { value: "" }, "none"),
		others.map((b) => h("option", { value: String(b.id), selected: compare && compare.batch.id === b.id }, `${b.name || "Batch " + b.id} (${b.done} games)`))
	);
	return h("div", { class: "row", style: "margin-top:0.6rem" }, h("label", { for: "compare-with" }, "Compare win rates with"), sel);
}

function renderReport() {
	if (!data) return;
	const { batch: info, report: rep } = data;
	clear(root);
	root.append(headerCard(info));
	if (!rep.games) {
		root.append(h("p", { class: "muted" }, info.status === "running" || info.status === "queued" ? "Waiting for the first games to finish…" : "No games were played."));
		return;
	}
	// The game's own sections, with the length where it wants it; without
	// any, the length alone.
	const own = page.batchSections ? page.batchSections(rep, compare && compare.report, lengthCard(rep), renderReport) : [h("div", { class: "report-grid" }, lengthCard(rep))];
	root.append(summaryStats(rep, info), ...own, gamesCard(info));
}

async function load() {
	clearTimeout(timer);
	try {
		data = await api.batchReport(batchID);
		page = (await api.gameDef(data.batch.spec.base).catch(() => ({}))).page || {};
		if (!games || data.batch.done !== games.length) games = (await api.batchGames(batchID)).games || [];
		if (!allBatches.length) allBatches = (await api.batches()).batches || [];
		renderReport();
		if (data.batch.status === "queued" || data.batch.status === "running") timer = setTimeout(load, 2000);
	} catch (err) {
		clear(root);
		root.append(h("p", { class: "error-text" }, err.message), h("a", { href: "/batches" }, "All batches"));
	}
}

// ---- the list ----------------------------------------------------------------------------------

async function loadList() {
	clearTimeout(timer);
	try {
		const { batches } = await api.batches();
		clear(root);
		root.append(
			h("p", { class: "muted" }, "Bot practice batches. Start one from the home page, from a game that offers practice."),
			batches.length
				? h("ul", { class: "batch-list", id: "batch-list" }, batches.map((b) => h("li", {}, h("div", {}, h("a", { href: `/batches/${b.id}` }, b.name || `Batch ${b.id}`), h("div", { class: "muted small" }, `${b.spec.players.map((p) => `${p.faction ? p.faction.toUpperCase() + ":" : ""}${p.bot}`).join(" ")} · ${new Date(b.created_at).toLocaleString()}`)), h("div", { class: "row" }, h("span", { class: `batch-status ${b.status}` }, b.status === "running" ? `running ${b.done}/${b.games}` : (STATUS_TEXT[b.status] || b.status) + ` · ${b.done}/${b.games}`)))))
				: h("p", {}, "No batches yet.")
		);
		if (batches.some((b) => b.status === "running" || b.status === "queued")) timer = setTimeout(loadList, 3000);
	} catch (err) {
		clear(root);
		root.append(h("p", { class: "error-text" }, err.message));
	}
}

if (root) (batchID ? load() : loadList());
