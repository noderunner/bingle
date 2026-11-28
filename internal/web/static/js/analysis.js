// The Analysis tab of the home page: the place to set a batch of bot-versus-bot
// games going for any game, to watch it run and to read what it says — and a
// guide to the bots themselves, so that the numbers mean something.
//
// A batch plays hundreds of games in memory, with no rooms and no database rows
// per move, and reports what happened: how often each seat won, how long the
// games ran, how they ended, and (for Cthulhu Wars, which is watched far more
// closely) a report of its own at /batches/{id}.

import { h, clear, store } from "./util.js";
import { api, gameTitle, whoAmI } from "./api.js";
import { rateBars } from "./charts.js";
import { renderMarkdown } from "./markdown.js";
import { t, tParts } from "./i18n.js";
import { seatKey } from "./paint.js";
import { botLadder } from "./botladder.js";

const SETUP_KEY = "bingle.analysis";
const STATUS_TEXT = { queued: "batch.queued", running: "batch.running", done: "batch.done", cancelled: "batch.cancelled", failed: "batch.failed", interrupted: "batch.interrupted" };
// A batch ended at a limit says so in place of its status.
const LIMIT_TEXT = { time: "batch.limit_time" };
const SEAT_COLORS = ["#c0392b", "#2f6f9f", "#3f8a4f", "#c9a227", "#8a5fbf", "#d1731f"];

let mounted = false;
let defs = [];
let botList = [];
let timer = null;
const expanded = new Set(); // the batches whose card shows its report

/** What the form was last set to, so the tab comes back as it was left. */
function setup() {
	const s = store.get(SETUP_KEY, {}) || {};
	return { game: s.game || "", games: s.games || 100, rotate: s.rotate !== false, bots: s.bots || {} };
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
		const sel = h("select", { class: "analysis-bot", "data-seat": seat.seat, "aria-label": t("analysis.bot_for", { seat: seat.label || seat.color }) });
		for (const b of botList) sel.append(h("option", { value: b.id }, t("setup.bot", { name: b.label })));
		sel.value = chosen[String(seat.seat)] || (botList.some((b) => b.id === "tactical") ? "tactical" : botList[0].id);
		rows.append(
			h(
				"label",
				{ class: "row analysis-seat" },
				seatKey(seat, info),
				h("span", { class: "grow" }, seat.label || seat.color),
				sel
			)
		);
	}
	return rows;
}

function form(host, maxGames) {
	const s = setup();
	const gameSel = h("select", { id: "analysis-game", "aria-label": t("analysis.game_to_study") });
	for (const d of defs) gameSel.append(h("option", { value: d.id }, d.seats_required ? t("analysis.game_players", { game: gameTitle(d), n: d.seats_required }) : gameTitle(d)));
	if (defs.some((d) => d.id === s.game)) gameSel.value = s.game;
	const count = h("input", { id: "analysis-games", type: "number", min: "1", max: String(maxGames), value: String(s.games), "aria-label": t("analysis.how_many") });
	const rotate = h("input", { id: "analysis-rotate", type: "checkbox" });
	rotate.checked = s.rotate;
	const seats = h("div", { id: "analysis-seats-field" });
	const err = h("div", { class: "error-text", id: "analysis-error", role: "alert" });
	const start = h("button", { type: "button", id: "analysis-start", class: "primary" }, t("analysis.run"));

	const paintSeats = async () => {
		clear(seats);
		seats.append(h("p", { class: "small muted" }, t("analysis.reading_game")));
		try {
			const info = await api.gameDef(gameSel.value);
			clear(seats);
			seats.append(h("label", {}, t("analysis.who_plays")), seatRows(info, setup().bots));
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
			games: Math.max(1, Math.min(maxGames, Number(count.value) || 1)),
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
			await api.startBatch(body);
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
			h("h2", {}, t("analysis.run")),
			h("div", { class: "field" }, h("label", { for: "analysis-game" }, t("analysis.game")), gameSel),
			seats,
			h("div", { class: "field" }, h("label", { for: "analysis-games" }, t("analysis.how_many")), count),
			h("div", { class: "field" }, h("label", { class: "check" }, rotate, h("span", {}, t("analysis.rotate")))),
			err,
			h("div", { class: "row" }, start),
		)
	);
}

// ---- the batches and their reports ------------------------------------------

/** A batch's game, by its name alone ("Cthulhu Wars", not its layout or size). */
function gameName(id) {
	const base = String(id || "").split("~")[0];
	const def = defs.find((d) => d.id === base) || defs.find((d) => d.family === base);
	if (def) return def.name.replace(/\s*\(.*\)$/, "");
	return base.replace(/-/g, " ").replace(/\b\w/g, (c) => c.toUpperCase());
}

function batchRow(b) {
	const word = LIMIT_TEXT[b.limit] ? t(LIMIT_TEXT[b.limit]) : STATUS_TEXT[b.status] ? t(STATUS_TEXT[b.status]) : b.status;
	const status = b.status === "running" ? t("batch.running_n", { done: b.done, games: b.games }) : `${word} · ${b.done}/${b.games}`;
	const on = expanded.has(b.id);
	const report = h("div", { class: "batch-report", hidden: !on });
	const open = h("button", { type: "button", class: "ghost", "data-open-batch": b.id, "aria-expanded": on ? "true" : "false" }, t("analysis.report"));
	open.addEventListener("click", () => {
		const show = !expanded.has(b.id);
		if (show) expanded.add(b.id);
		else expanded.delete(b.id);
		open.setAttribute("aria-expanded", show ? "true" : "false");
		report.hidden = !show;
		if (show) fillReport(b.id, report);
	});
	if (on) fillReport(b.id, report);
	return h(
		"li",
		{ "data-batch": b.id },
		h("div", { class: "title grow" }, gameName(b.game_def_id)),
		h("span", { class: `batch-status ${b.status}${b.limit ? " limit" : ""}` }, status),
		open,
		h("a", { class: "button", href: `/batches/${b.id}` }, t("analysis.full_report")),
		report
	);
}

/** The win rate of every seat, with how sure we can be of it. */
function seatCard(rep) {
	const rows = (rep.seats || []).map((s, i) => {
		const ci = interval(s.wins, s.games);
		return {
			label: `${s.label || t("analysis.seat", { n: s.seat })}${s.bots && s.bots.length === 1 ? ` (${s.bots[0]})` : ""}`,
			color: SEAT_COLORS[i % SEAT_COLORS.length],
			value: ci.rate,
			lo: ci.lo,
			hi: ci.hi,
			note: t("analysis.wins_of", { wins: s.wins, games: s.games }),
		};
	});
	if (!rows.length) return null;
	return h("div", { class: "card analysis-seat-card" }, h("h3", {}, t("analysis.win_rate")), rateBars(rows, { marker: 1 / rows.length }));
}

function endingsCard(rep) {
	const reasons = Object.entries(rep.end_reasons || {}).sort((a, b) => b[1] - a[1]);
	const draws = (rep.games || 0) - (rep.decided || 0);
	return h(
		"div",
		{ class: "card analysis-endings" },
		h("h3", {}, t("analysis.summary")),
		h(
			"dl",
			{ class: "stat-list" },
			h("dt", {}, t("analysis.games")),
			h("dd", {}, String(rep.games || 0)),
			h("dt", {}, t("analysis.decided")),
			h("dd", {}, `${rep.decided || 0} (${rep.games ? pct(rep.decided / rep.games) : "0%"})`),
			h("dt", {}, t("analysis.drawn")),
			h("dd", {}, String(draws)),
			h("dt", {}, t("analysis.moves")),
			h("dd", {}, rep.plies ? t("analysis.median", { median: Math.round(rep.plies.p50), min: Math.round(rep.plies.min), max: Math.round(rep.plies.max) }) : "—")
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

// The last report drawn for each batch, so that redrawing the list while a
// batch runs does not empty an open card while its report is read again.
const drawn = new Map();

/** Fills a batch's card with its win rate by seat and its result summary. */
async function fillReport(id, host) {
	if (drawn.has(id)) host.replaceChildren(...drawn.get(id).map((n) => n.cloneNode(true)));
	else host.replaceChildren(h("p", { class: "muted" }, t("analysis.reading_report")));
	try {
		const { batch: info, report: rep } = await api.batchReport(id);
		const nodes = rep.games
			? [h("div", { class: "report-grid" }, seatCard(rep), endingsCard(rep))]
			: [h("p", { class: "muted" }, info.status === "running" || info.status === "queued" ? t("analysis.waiting") : t("analysis.none_played"))];
		drawn.set(id, nodes);
		host.replaceChildren(...nodes);
	} catch (e) {
		host.replaceChildren(h("p", { class: "error-text" }, e.message));
	}
}

async function refresh() {
	const list = document.getElementById("analysis-batches");
	if (!list) return;
	clearTimeout(timer);
	try {
		const { batches } = await api.batches();
		clear(list);
		if (!batches.length) list.append(h("li", { class: "muted" }, t("analysis.no_batches")));
		for (const b of batches) list.append(batchRow(b));
		if (batches.some((b) => b.status === "running" || b.status === "queued")) timer = setTimeout(refresh, 3000);
	} catch (e) {
		clear(list);
		list.append(h("li", { class: "error-text" }, e.message));
	}
}

// ---- the guide to the bots ---------------------------------------------------

/** Opens the guide to the bots in a window of its own, like a game's rules. */
export async function openBotGuide() {
	document.getElementById("bot-guide-dialog")?.remove();
	const box = h("div", { class: "rules-body analysis-guide md", id: "analysis-guide", tabindex: -1 }, h("p", { class: "muted" }, t("analysis.reading_guide")));
	const close = h("button", { class: "rules-close", "aria-label": t("analysis.close_guide"), title: t("common.close_esc") }, "×");
	const dlg = h("dialog", { id: "bot-guide-dialog", class: "rules-window", "aria-labelledby": "bot-guide-title" }, h("div", { class: "rules-head" }, h("h2", { id: "bot-guide-title" }, t("home.bot_guide")), close), box);
	close.addEventListener("click", () => dlg.close());
	dlg.addEventListener("click", (ev) => {
		if (ev.target === dlg) dlg.close();
	});
	dlg.addEventListener("close", () => dlg.remove());
	document.body.append(dlg);
	if (typeof dlg.showModal === "function") dlg.showModal();
	else dlg.setAttribute("open", "");
	box.focus();
	try {
		const text = await api.botGuide();
		clear(box);
		// The window's title is the guide's: its own first heading is not repeated.
		const { node } = renderMarkdown(text.replace(/^# .*\n+/, ""), { idPrefix: "bots", assetBase: "/api/bot-guide/assets/" });
		box.append(node);
		// The section on the bot ladder ends with the ladder itself (ADR 0160).
		const section = box.querySelector("#botsmd-the-bot-ladder");
		if (section) {
			let end = section;
			while (end.nextElementSibling && !/^H[12]$/.test(end.nextElementSibling.tagName)) end = end.nextElementSibling;
			end.after(botLadder({ id: "bot-ladder-guide", heading: false }));
		}
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
	host.append(h("div", { class: "analysis-top" }));
	const top = host.querySelector(".analysis-top");
	try {
		[defs, botList] = await Promise.all([api.gameDefs(), api.bots()]);
	} catch (_) {
		defs = defs || [];
		botList = botList || [];
	}
	// Only the bots that think are worth a batch, but random is kept: it is the
	// baseline every other bot is measured against. Running one is an
	// account's (run_batch); reading the reports and the guide is anybody's.
	const me = await whoAmI();
	if ((me.caps || []).includes("run_batch")) {
		const limits = (await api.batches().catch(() => ({}))).limits || {};
		form(top, limits.games || 5000);
	} else {
		top.append(
			h(
				"div",
				{ class: "card", id: "analysis-signin" },
				h("h2", {}, t("analysis.run")),
				h("p", {}, ...tParts("analysis.signin", { signin: h("a", { href: "/login?return=" + encodeURIComponent("/#analysis") }, t("common.sign_in")) }))
			)
		);
	}
	top.append(
		h(
			"div",
			{ class: "card analysis-batches-card" },
			h("h2", {}, t("analysis.batches")),
			h("ul", { class: "my-games batch-list", id: "analysis-batches" }, h("li", { class: "muted" }, t("analysis.reading")))
		)
	);
	await refresh();
}
