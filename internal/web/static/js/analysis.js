// The Analysis tab of the home page: the place to set a batch of bot-versus-bot
// games going for any game, to watch it run and to read what it says — and a
// guide to the bots themselves, so that the numbers mean something.
//
// A batch plays hundreds of games in memory, with no rooms and no database rows
// per move, and reports what happened: how often each seat won, how long the
// games ran, how they ended, and (for Cthulhu Wars, which is watched far more
// closely) a report of its own at /batches/{id}.

import { h, clear, store, ago } from "./util.js";
import { api } from "./api.js";
import { rateBars } from "./charts.js";
import { renderMarkdown } from "./markdown.js";

const SETUP_KEY = "bingle.analysis";
const STATUS_TEXT = { queued: "Queued", running: "Running", done: "Finished", cancelled: "Cancelled", failed: "Failed", interrupted: "Interrupted" };
const SEAT_COLORS = ["#c0392b", "#2f6f9f", "#3f8a4f", "#c9a227", "#8a5fbf", "#d1731f"];

let mounted = false;
let defs = [];
let botList = [];
let timer = null;
let openReport = 0; // the batch whose report is on screen

/** What the form was last set to, so the tab comes back as it was left. */
function setup() {
	const s = store.get(SETUP_KEY, {}) || {};
	return { game: s.game || "tic-tac-toe", games: s.games || 100, rotate: s.rotate !== false, bots: s.bots || {} };
}

const pct = (v) => `${(v * 100).toFixed(1)}%`;

/** Wilson's interval for a win rate, which is honest about few games. */
function interval(wins, n) {
	if (!n) return { rate: 0, lo: 0, hi: 0 };
	const z = 1.96;
	const p = wins / n;
	const d = 1 + (z * z) / n;
	const mid = (p + (z * z) / (2 * n)) / d;
	const half = (z * Math.sqrt((p * (1 - p)) / n + (z * z) / (4 * n * n))) / d;
	return { rate: p, lo: Math.max(0, mid - half), hi: Math.min(1, mid + half) };
}

// ---- the form ---------------------------------------------------------------

function seatRows(info, chosen) {
	const rows = h("div", { class: "stack", id: "analysis-seats" });
	for (const seat of info.seats) {
		const sel = h("select", { class: "analysis-bot", "data-seat": seat.seat, "aria-label": `Bot for ${seat.label || seat.color}` });
		for (const b of botList) sel.append(h("option", { value: b.id }, `${b.label} bot`));
		sel.value = chosen[String(seat.seat)] || (botList.some((b) => b.id === "tactical") ? "tactical" : botList[0].id);
		rows.append(
			h(
				"label",
				{ class: "row analysis-seat" },
				h("span", { class: "swatch color-" + seat.color, "aria-hidden": "true" }),
				h("span", { class: "grow" }, seat.label || seat.color),
				sel
			)
		);
	}
	return rows;
}

function form(host) {
	const s = setup();
	const gameSel = h("select", { id: "analysis-game", "aria-label": "Game to study" });
	for (const d of defs) gameSel.append(h("option", { value: d.id }, `${d.name}${d.seats_required ? ` · ${d.seats_required} players` : ""}`));
	if (defs.some((d) => d.id === s.game)) gameSel.value = s.game;
	const count = h("input", { id: "analysis-games", type: "number", min: "1", max: "5000", value: String(s.games), "aria-label": "How many games" });
	const rotate = h("input", { id: "analysis-rotate", type: "checkbox" });
	rotate.checked = s.rotate;
	const seats = h("div", { id: "analysis-seats-field" });
	const err = h("div", { class: "error-text", id: "analysis-error", role: "alert" });
	const start = h("button", { type: "button", id: "analysis-start", class: "primary" }, "Run the batch");
	const note = h("p", { class: "small muted" }, "The games are played in memory, as fast as the machine allows, and nothing is saved in your games. One batch runs at a time.");

	const paintSeats = async () => {
		clear(seats);
		seats.append(h("p", { class: "small muted" }, "Reading the game…"));
		try {
			const info = await api.gameDef(gameSel.value);
			clear(seats);
			seats.append(h("label", {}, "Who plays"), seatRows(info, setup().bots));
		} catch (e) {
			clear(seats);
			seats.append(h("p", { class: "error-text" }, e.message));
		}
	};
	gameSel.addEventListener("change", paintSeats);
	paintSeats();

	start.addEventListener("click", async () => {
		err.textContent = "";
		const players = [...seats.querySelectorAll(".analysis-bot")].map((sel) => ({ bot: sel.value }));
		const body = {
			name: `${gameSel.selectedOptions[0].textContent.split(" · ")[0]} · ${players.map((p) => p.bot).join(" v ")}`,
			base: gameSel.value,
			players,
			games: Math.max(1, Math.min(5000, Number(count.value) || 1)),
			rotate: rotate.checked,
		};
		store.set(SETUP_KEY, {
			game: gameSel.value,
			games: body.games,
			rotate: body.rotate,
			bots: Object.fromEntries([...seats.querySelectorAll(".analysis-bot")].map((sel) => [sel.dataset.seat, sel.value])),
		});
		start.disabled = true;
		try {
			const info = await api.startBatch(body);
			openReport = info.id;
			await refresh();
		} catch (e) {
			err.textContent = e.message;
		} finally {
			start.disabled = false;
		}
	});

	host.append(
		h(
			"div",
			{ class: "card analysis-form" },
			h("h2", {}, "Run a batch"),
			h("div", { class: "field" }, h("label", { for: "analysis-game" }, "Game"), gameSel),
			seats,
			h("div", { class: "field" }, h("label", { for: "analysis-games" }, "How many games"), count),
			h("div", { class: "field" }, h("label", { class: "check" }, rotate, h("span", {}, "Move the bots round the seats (so the order of play favours nobody)"))),
			err,
			h("div", { class: "row" }, start),
			note
		)
	);
}

// ---- the batches and their reports ------------------------------------------

function batchRow(b) {
	const status = b.status === "running" ? `running ${b.done}/${b.games}` : `${STATUS_TEXT[b.status] || b.status} · ${b.done}/${b.games}`;
	const open = h("button", { type: "button", class: "ghost", "data-open-batch": b.id, onclick: () => showReport(b.id) }, "Report");
	return h(
		"li",
		{ "data-batch": b.id, class: b.id === openReport ? "on" : null },
		h(
			"div",
			{ class: "grow" },
			h("div", { class: "title" }, b.name || `Batch ${b.id}`),
			h("div", { class: "meta muted small" }, `${b.game_def_id} · ${b.spec.players.map((p) => p.bot).join(" v ")} · ${ago(b.created_at)}`)
		),
		h("span", { class: `batch-status ${b.status}` }, status),
		open,
		h("a", { class: "button", href: `/batches/${b.id}` }, "Full report")
	);
}

/** The win rate of every seat, with how sure we can be of it. */
function seatCard(rep) {
	const rows = (rep.seats || []).map((s, i) => {
		const ci = interval(s.wins, s.games);
		return {
			label: `${s.label || "Seat " + s.seat}${s.bots && s.bots.length === 1 ? ` (${s.bots[0]})` : ""}`,
			color: SEAT_COLORS[i % SEAT_COLORS.length],
			value: ci.rate,
			lo: ci.lo,
			hi: ci.hi,
			note: `${s.wins} of ${s.games} games`,
		};
	});
	if (!rows.length) return null;
	const fair = rows.length ? 1 / rows.length : 0;
	return h(
		"div",
		{ class: "card", id: "analysis-seat-card" },
		h("h3", {}, "Win rate by seat"),
		rateBars(rows, { marker: fair }),
		h("p", { class: "small muted" }, `The dashed line is an even share (${pct(fair)}). The whiskers are a 95% interval: where they overlap the line, the batch has not shown a difference.`)
	);
}

/** The same by bot, which is what says whether one plays better than another. */
function botCard(rep) {
	const rows = (rep.bots || []).map((b, i) => {
		const ci = interval(b.wins, b.games);
		return { label: b.bot, color: SEAT_COLORS[(i + 2) % SEAT_COLORS.length], value: ci.rate, lo: ci.lo, hi: ci.hi, note: `${b.wins} of ${b.games} seats` };
	});
	if (rows.length < 2) return null;
	return h("div", { class: "card", id: "analysis-bot-card" }, h("h3", {}, "Win rate by bot"), rateBars(rows, {}), h("p", { class: "small muted" }, "Each seat of each game counts once, wherever the bot sat."));
}

function endingsCard(rep) {
	const reasons = Object.entries(rep.end_reasons || {}).sort((a, b) => b[1] - a[1]);
	const draws = (rep.games || 0) - (rep.decided || 0);
	return h(
		"div",
		{ class: "card", id: "analysis-endings" },
		h("h3", {}, "How the games went"),
		h(
			"dl",
			{ class: "stat-list" },
			h("dt", {}, "Games"),
			h("dd", {}, String(rep.games || 0)),
			h("dt", {}, "Decided"),
			h("dd", {}, `${rep.decided || 0} (${rep.games ? pct(rep.decided / rep.games) : "0%"})`),
			h("dt", {}, "Drawn or unfinished"),
			h("dd", {}, String(draws)),
			h("dt", {}, "Moves per game"),
			h("dd", {}, rep.plies ? `${rep.plies.p50.toFixed(0)} median, ${rep.plies.min.toFixed(0)}–${rep.plies.max.toFixed(0)}` : "—")
		),
		reasons.length
			? h(
					"ul",
					{ class: "small muted analysis-reasons" },
					reasons.map(([why, n]) => h("li", { "data-reason": why }, `${why}: ${n}`))
				)
			: null
	);
}

async function showReport(id) {
	openReport = id;
	const host = document.getElementById("analysis-report");
	if (!host) return;
	clear(host);
	host.append(h("p", { class: "muted" }, "Reading the report…"));
	try {
		const { batch: info, report: rep } = await api.batchReport(id);
		clear(host);
		if (!rep.games) {
			host.append(h("p", { class: "muted" }, info.status === "running" || info.status === "queued" ? "Waiting for the first games to finish…" : "No games were played."));
			return;
		}
		host.append(
			h("h2", {}, info.name || `Batch ${id}`),
			h("p", { class: "small muted" }, `${info.game_def_id} · ${rep.games} games · ${STATUS_TEXT[info.status] || info.status}`),
			h("div", { class: "report-grid" }, seatCard(rep), endingsCard(rep)),
			botCard(rep),
			h("p", { class: "small" }, h("a", { href: `/batches/${id}` }, "The full report"), " has everything the batch measured.")
		);
	} catch (e) {
		clear(host);
		host.append(h("p", { class: "error-text" }, e.message));
	}
}

async function refresh() {
	const list = document.getElementById("analysis-batches");
	if (!list) return;
	clearTimeout(timer);
	try {
		const { batches } = await api.batches();
		clear(list);
		if (!batches.length) list.append(h("li", { class: "muted" }, "No batches yet. Set one going above."));
		for (const b of batches) list.append(batchRow(b));
		if (!openReport && batches.length) openReport = batches[0].id;
		if (openReport) await showReport(openReport);
		if (batches.some((b) => b.status === "running" || b.status === "queued")) timer = setTimeout(refresh, 3000);
	} catch (e) {
		clear(list);
		list.append(h("li", { class: "error-text" }, e.message));
	}
}

// ---- the guide to the bots ---------------------------------------------------

async function guide(host) {
	const box = h("div", { class: "card analysis-guide md", id: "analysis-guide" }, h("p", { class: "muted" }, "Reading the guide…"));
	host.append(box);
	try {
		const text = await api.botGuide();
		clear(box);
		const { node, headings } = renderMarkdown(text, { idPrefix: "bots", assetBase: "/api/bot-guide/assets/" });
		if (headings && headings.length > 2) {
			box.append(
				h(
					"nav",
					{ class: "rules-toc", "aria-label": "On this page" },
					h("p", { class: "small muted" }, "On this page"),
					h("ul", {}, headings.filter((t) => t.level === 2).map((t) => h("li", { class: `lvl-${t.level}` }, h("a", { href: `#botsmd-${t.id}` }, t.text))))
				)
			);
		}
		box.append(node);
	} catch (e) {
		clear(box);
		box.append(h("p", { class: "error-text" }, e.message));
	}
}

// ---- the tab ----------------------------------------------------------------

/** Builds the Analysis tab the first time it is shown. */
export async function mountAnalysis(host) {
	if (!host || mounted) return;
	mounted = true;
	clear(host);
	host.append(
		h("p", { class: "muted" }, "Set bots against each other over hundreds of games to see how a game behaves — who the first move favours, how long games run, how often they are drawn — and how the bots themselves compare."),
		h("div", { class: "analysis-top" })
	);
	const top = host.querySelector(".analysis-top");
	try {
		[defs, botList] = await Promise.all([api.gameDefs(), api.bots()]);
	} catch (_) {
		defs = defs || [];
		botList = botList || [];
	}
	// Only the bots that think are worth a batch, but random is kept: it is the
	// baseline every other bot is measured against.
	form(top);
	top.append(
		h(
			"div",
			{ class: "card analysis-batches-card" },
			h("h2", {}, "Batches"),
			h("ul", { class: "my-games batch-list", id: "analysis-batches" }, h("li", { class: "muted" }, "Reading…"))
		)
	);
	host.append(h("div", { id: "analysis-report", class: "analysis-report" }));
	host.append(h("h2", { id: "analysis-guide-head" }, "The bots, and what to do with them"));
	await refresh();
	await guide(host);
}
