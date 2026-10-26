// Practice-batch pages: the list at /batches and a batch's report at
// /batches/{id} — charts and tables drawn from GET /api/batches/{id}/report.

import { h, clear } from "./util.js";
import { api } from "./api.js";
import { factionName, factionColor, pretty } from "./cw-names.js";
import { rateBars, lineChart, columnChart } from "./charts.js";

const root = document.getElementById("batch-root");
const batchID = root ? root.dataset.batchId : "";

const pct = (v, d = 1) => `${(v * 100).toFixed(d)}%`;
const num = (v, d = 1) => (Math.abs(v - Math.round(v)) < 1e-9 ? String(Math.round(v)) : v.toFixed(d));

// ---- labels ----------------------------------------------------------------------

const ACTIONS = {
	recruit: "Recruit a Cultist", summon: "Summon a Monster", awaken: "Awaken a Great Old One", gate: "Build a Gate", go: "Move a unit",
	battle: "Battle", capture: "Capture a Cultist", control: "Take control of a Gate", abandon: "Abandon a Gate", leave: "Leave a Gate",
	ritual: "Ritual of Annihilation", sacrifice: "Sacrifice", pass: "Pass", done: "End turn", reveal: "Reveal Elder Signs", pay: "Pay",
	submerge: "Submerge", bring: "Bring a unit (Submerge)", emerge: "Emerge", dreams: "Dreams", devolve: "Devolve", forms: "Thousand Forms",
	ghroth: "Ghroth", scream: "Scream of the Dead", shriek: "Shriek", zingaya: "Zingaya", desecrate: "Desecrate", hwinn: "He Who is Not to be Named",
	skip: "Skip", blood: "Blood Sacrifice", absorb: "Absorb", abduct: "Abduct", invisible: "Invisibility", seek: "Seek", necro: "Necrophagy",
	assign: "Assign a result", retreat: "Retreat", lose: "Lose a unit", book: "Choose a spellbook", cw: "Direction: clockwise", ccw: "Direction: counter-clockwise",
	avatar: "Avatar", swap: "Swap (Avatar)", place: "Place a Cultist", grant: "Grant Doom", harbinger: "Harbinger", start: "Choose the start",
	hibernate: "Hibernate", iceage: "Ice Age", claim: "Claim a spellbook", howl: "Howl", cannibal: "Cannibalism", eternal: "Eternal",
	lethargy: "Lethargy", boon: "Boon", bestow: "Bestow", bane: "Bane", sorcery: "Ancient Sorcery", restore: "Restore", slumber: "Cursed Slumber",
	wake: "Wake", death: "Death from Below", demand: "Demand Sacrifice", nexus: "Energy Nexus", oath: "Unspeakable Oath",
};
// Decisions that are plumbing of other actions, not choices worth comparing.
const PLUMBING = new Set(["go", "done", "assign", "lose", "pass", "skip", "retreat", "bring", "swap", "book", "cw", "ccw", "start", "pay", "demand", "place"]);
const actionLabel = (a) => ACTIONS[a] || pretty(a);

const STATUS_TEXT = { queued: "Queued", running: "Running", done: "Finished", cancelled: "Cancelled", failed: "Failed", interrupted: "Interrupted" };

// ---- statistics ------------------------------------------------------------------

/** z of the difference between two win rates (pooled two-proportion test). */
function zScore(a, b) {
	if (a.n < 2 || b.n < 2) return 0;
	const p1 = a.wins / a.n,
		p2 = b.wins / b.n,
		p = (a.wins + b.wins) / (a.n + b.n);
	const se = Math.sqrt(p * (1 - p) * (1 / a.n + 1 / b.n));
	return se === 0 ? 0 : (p1 - p2) / se;
}

// ---- tables ----------------------------------------------------------------------

/**
 * Rows: [{label, avg, lift}] → a table of win rate above/below the cult's own
 * average of the thing, sortable by its Δ.
 */
function liftTable(rows, { avgLabel = "Avg per game", minN = 8 } = {}) {
	const items = rows
		.filter((r) => r.lift && r.lift.with.n + r.lift.without.n > 0)
		.map((r) => {
			const { with: w, without: o } = r.lift;
			const ok = w.n >= minN && o.n >= minN;
			const delta = ok ? w.wins / w.n - o.wins / o.n : 0;
			return { ...r, ok, delta, z: ok ? zScore(w, o) : 0 };
		})
		.sort((a, b) => Number(b.ok) - Number(a.ok) || Math.abs(b.z) - Math.abs(a.z));
	if (!items.length) return h("p", { class: "muted small" }, "Nothing to compare yet.");
	const body = items.map((r) => {
		const w = r.lift.with,
			o = r.lift.without;
		const sig = Math.abs(r.z) >= 1.96;
		return h(
			"tr",
			{ class: r.ok ? "" : "faint", title: r.ok ? "" : `Needs at least ${minN} games on each side of the average.` },
			h("td", {}, r.label),
			h("td", {}, num(r.avg, 2)),
			h("td", {}, w.n ? pct(w.wins / w.n, 0) : "–"),
			h("td", { class: "muted" }, `n=${w.n}`),
			h("td", {}, o.n ? pct(o.wins / o.n, 0) : "–"),
			h("td", { class: "muted" }, `n=${o.n}`),
			h("td", { class: r.ok ? `delta ${r.delta > 0 ? "up" : r.delta < 0 ? "down" : ""}` : "" }, r.ok ? `${r.delta > 0 ? "+" : ""}${(r.delta * 100).toFixed(0)} pp${sig ? " ●" : ""}` : "")
		);
	});
	return h(
		"div",
		{ class: "table-scroll" },
		h("table", { class: "data" }, h("thead", {}, h("tr", {}, h("th", {}, ""), h("th", {}, avgLabel), h("th", { title: "Win rate in the games where the cult did more of this than its average" }, "Win % above avg"), h("th", {}, ""), h("th", {}, "Win % otherwise"), h("th", {}, ""), h("th", { title: "● = unlikely to be chance (|z| ≥ 1.96) — but many rows are compared, so some will be flukes" }, "Δ"))), h("tbody", {}, body))
	);
}

// ---- sections --------------------------------------------------------------------

function legend(factions) {
	return h("div", { class: "legend" }, factions.map((f) => h("span", {}, h("i", { style: `background:${factionColor(f.key)}` }), factionName(f.key))));
}

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

const endReason = (k) => ({ instant_death: "Instant Death", humanity: "Humanity won", draw: "Draw", ritual: "Ritual win", doom: "30 Doom" }[k] || k);

function stat(label, value) {
	return h("div", { class: "batch-stat" }, h("b", {}, value), h("span", { class: "muted small" }, label));
}

function winRateCard(rep, other) {
	const n = rep.factions.length || 1;
	const rows = [];
	for (const f of rep.factions) {
		rows.push({ label: factionName(f.key), color: factionColor(f.key), value: f.rate, lo: f.lo, hi: f.hi, note: `${f.wins} of ${f.games} games` });
		if (other) {
			const o = other.factions.find((x) => x.key === f.key);
			if (o) rows.push({ label: "  compared", color: `color-mix(in srgb, ${factionColor(f.key)} 45%, transparent)`, value: o.rate, lo: o.lo, hi: o.hi, note: `${o.wins} of ${o.games} games` });
		}
	}
	return h(
		"section",
		{ class: "card" },
		h("h2", {}, "Win rate by cult"),
		h("p", { class: "muted small" }, `Whiskers are 95% intervals. The dashed line is an even share (${pct(1 / n, 0)}). With a few hundred games the whiskers are still wide — differences smaller than them may be luck.`),
		rateBars(rows, { marker: 1 / n, domain: Math.min(1, Math.max(0.5, ...rep.factions.map((f) => f.hi + 0.05))) }),
		h("div", { class: "muted small" }, "Win rate by the seat a cult sat in: ", rep.factions.map((f) => `${factionName(f.key)} ${f.by_seat.map((t) => (t.n ? `${pct(t.wins / t.n, 0)}` : "–")).join("/")}`).join(" · "))
	);
}

function roundsCard(rep, key, title, yLabel) {
	const series = rep.factions.map((f) => ({
		label: factionName(f.key),
		color: factionColor(f.key),
		// A late round reached by a handful of games is noise.
		points: f.rounds.filter((r) => r.n >= Math.max(3, f.games * 0.15)).map((r) => [r.round, r[key]]),
	}));
	return h("section", { class: "card" }, h("h2", {}, title), legend(rep.factions), lineChart(series, { xLabel: "End of round", yLabel }));
}

function lengthCard(rep) {
	const b = rep.ply_buckets;
	const bars = b.counts.map((c, i) => ({ label: String(b.lo + i * b.width), value: c }));
	return h("section", { class: "card" }, h("h2", {}, "Game length"), h("p", { class: "muted small" }, `Plies (single decisions) per game: median ${num(rep.plies.p50, 0)}, 90% under ${num(rep.plies.p90, 0)}, longest ${num(rep.plies.max, 0)}.`), columnChart(bars, { yLabel: "games" }), h("div", { class: "muted small" }, `Bars start at the labelled ply count and are ${b.width} plies wide.`));
}

function doomSourcesCard(rep) {
	const rows = rep.factions.map((f) => {
		const d = f.doom_from;
		const total = d.gates + d.rituals + d.gifts + d.signs || 1;
		return h("tr", {}, h("td", {}, factionName(f.key)), h("td", {}, num(f.avg_doom)), h("td", {}, num(d.gates)), h("td", {}, num(d.rituals)), h("td", {}, num(d.signs)), h("td", {}, num(f.avg_gates)), h("td", {}, num(f.avg_books)), h("td", {}, num(f.avg_signs)), h("td", {}, num(f.avg_power)), h("td", {}, h("span", { class: "bar-cell", style: `width:${(d.rituals / total) * 80}px` })));
	});
	return h("section", { class: "card" }, h("h2", {}, "Final standing"), h("p", { class: "muted small" }, "Averages at the end of a game. Doom from Gates, Rituals of Annihilation and Elder Signs."), h("div", { class: "table-scroll" }, h("table", { class: "data" }, h("thead", {}, h("tr", {}, ["Cult", "Doom", "from Gates", "from Rituals", "from Signs", "Gates", "Books", "Elder Signs", "Power", "Ritual share"].map((t) => h("th", {}, t)))), h("tbody", {}, rows))));
}

function reqsCard(rep) {
	const blocks = rep.factions
		.filter((f) => f.reqs && f.reqs.length)
		.map((f) =>
			h("div", {}, h("strong", {}, factionName(f.key)), h("div", { class: "table-scroll" }, h("table", { class: "data" }, h("thead", {}, h("tr", {}, h("th", {}, "Requirement"), h("th", {}, "Met in"), h("th", {}, "Avg round"))), h("tbody", {}, f.reqs.map((r) => h("tr", { class: r.share < 0.5 ? "weak" : "" }, h("td", {}, r.text), h("td", {}, pct(r.share, 0)), h("td", {}, r.share ? num(r.round, 1) : "–")))))))
		);
	if (!blocks.length) return null;
	return h("section", { class: "card" }, h("h2", {}, "Spellbook requirements"), h("p", { class: "muted small" }, "The share of games in which each requirement was met. A low number is a bottleneck: the cult cannot earn its sixth book without it, and with fewer than six nobody wins."), h("div", { class: "report-grid" }, blocks));
}

function booksCard(rep) {
	const blocks = rep.factions.map((f) =>
		h("div", {}, h("strong", {}, factionName(f.key)), h("div", { class: "table-scroll" }, h("table", { class: "data" }, h("thead", {}, h("tr", {}, h("th", {}, "Spellbook"), h("th", {}, "Earned in"), h("th", {}, "On average in round"))), h("tbody", {}, f.books.map((b) => h("tr", {}, h("td", {}, pretty(b.book)), h("td", {}, pct(b.share, 0)), h("td", {}, b.share ? num(b.round, 1) : "–")))))))
	);
	return h("section", { class: "card" }, h("h2", {}, "Spellbook timing"), h("div", { class: "report-grid" }, blocks));
}

function battlesCard(rep) {
	const rows = rep.factions.map((f) => {
		const b = f.battles;
		return h("tr", {}, h("td", {}, factionName(f.key)), h("td", {}, num(b.attacked)), h("td", {}, num(b.defended)), h("td", {}, num(b.won)), h("td", {}, num(b.lost)), h("td", {}, num(b.kills_dealt)), h("td", {}, num(b.kills_taken)));
	});
	return h("section", { class: "card" }, h("h2", {}, "Battles per game"), h("div", { class: "table-scroll" }, h("table", { class: "data" }, h("thead", {}, h("tr", {}, ["Cult", "Attacked", "Defended", "Won", "Lost", "Kills dealt", "Kills taken"].map((t) => h("th", {}, t)))), h("tbody", {}, rows))));
}

// ---- what correlates with winning ---------------------------------------------------

let focus = ""; // the cult whose tables are shown
let areaSort = "lift";

function insightCard(rep) {
	if (!rep.factions.length) return h("section", { class: "card" });
	if (!rep.factions.some((f) => f.key === focus)) focus = rep.factions[0].key;
	const f = rep.factions.find((x) => x.key === focus);
	const names = rep.area_names || {};

	const tabs = h("div", { class: "batch-tabs", role: "tablist" }, rep.factions.map((x) => h("button", { type: "button", class: x.key === focus ? "on" : "", "data-focus": x.key, onclick: () => { focus = x.key; renderReport(); } }, factionName(x.key))));

	const unitRows = f.units.filter((u) => u.built > 0.02).map((u) => ({ label: pretty(u.type), avg: u.built, lift: u.lift }));
	const actionRows = f.actions.filter((a) => !PLUMBING.has(a.action) && a.uses > 0.02).map((a) => ({ label: actionLabel(a.action), avg: a.uses, lift: a.lift }));
	const areaRows = f.areas.filter((a) => a.control > 0.005).map((a) => ({ label: names[a.area] || pretty(a.area), avg: a.control, lift: a.lift, a }));
	const bookRows = f.books.map((b) => ({ label: pretty(b.book), avg: b.share, lift: b.lift }));

	return h(
		"section",
		{ class: "card", id: "insights" },
		h("h2", {}, "What goes with winning?"),
		h("p", { class: "muted small" }, "For one cult: the games where it did more of something than its own average (built more of a unit, used an ability more, held an area for more of the game) against the rest, and the win rate in each. Correlation, not proof — a cult that is winning also builds and holds more."),
		tabs,
		h("div", { class: "report-grid" },
			h("div", {}, h("h3", {}, "Units built"), liftTable(unitRows, { avgLabel: "Built / game" })),
			h("div", {}, h("h3", {}, "Spellbooks"), liftTable(bookRows, { avgLabel: "Earned" }))
		),
		h("h3", {}, "Abilities and actions"),
		liftTable(actionRows, { avgLabel: "Uses / game" }),
		h("h3", {}, "Areas held"),
		h("p", { class: "muted small" }, "Control = the cult's Gate is there, or it is the only one with units there, sampled at the end of every round. “Avg” is the share of those moments."),
		liftTable(areaRows, { avgLabel: "Held (share)" })
	);
}

function heatCard(rep) {
	const names = rep.area_names || {};
	const areas = new Set();
	for (const f of rep.factions) for (const a of f.areas) if (a.control > 0.01) areas.add(a.area);
	const list = [...areas].sort((a, b) => (names[a] || a).localeCompare(names[b] || b));
	const head = h("tr", {}, h("th", {}, "Area"), rep.factions.map((f) => h("th", {}, factionName(f.key).split(" ")[0])));
	const rows = list.map((id) =>
		h("tr", {}, h("td", {}, names[id] || pretty(id)), rep.factions.map((f) => {
			const a = f.areas.find((x) => x.area === id);
			const v = a ? a.control : 0;
			return h("td", { class: "heat", style: `background: color-mix(in srgb, ${factionColor(f.key)} ${Math.round(Math.min(1, v * 1.6) * 100)}%, transparent)`, title: `${factionName(f.key)} holds ${names[id] || id} ${pct(v, 0)} of the time` }, v >= 0.005 ? pct(v, 0) : "");
		}))
	);
	return h("section", { class: "card" }, h("h2", {}, "Who holds which area"), h("p", { class: "muted small" }, "The share of round ends in which each cult controlled the area, averaged over all its games."), h("div", { class: "table-scroll" }, h("table", { class: "data" }, h("thead", {}, head), h("tbody", {}, rows))));
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
			h("td", {}, h("span", { class: "dots" }, g.seating.map((k) => h("i", { style: `background:${factionColor(k)}` }))), " ", g.seating.map((k) => k.toUpperCase()).join(" ")),
			h("td", {}, w ? factionName(w) : "–"),
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
			" · ", info.spec.players.map((p) => `${factionName(p.faction)}: ${p.bot}${p.mcts_iterations ? " " + p.mcts_iterations : ""}`).join(", "),
			` · seed ${info.spec.seed}${info.spec.rotate ? ", seats rotated" : ""}${info.spec.high_priests ? ", High Priests" : ""}`
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
	root.append(
		summaryStats(rep, info),
		h("div", { class: "report-grid" }, winRateCard(rep, compare && compare.report), lengthCard(rep)),
		h("div", { class: "report-grid" }, roundsCard(rep, "doom", "Doom over the rounds", "Doom"), roundsCard(rep, "gates", "Gates held", "Gates")),
		h("div", { class: "report-grid" }, roundsCard(rep, "units", "Units on the map", "Units"), roundsCard(rep, "power", "Power in hand", "Power")),
		doomSourcesCard(rep),
		insightCard(rep),
		heatCard(rep),
		reqsCard(rep),
		booksCard(rep),
		battlesCard(rep),
		gamesCard(info)
	);
}

async function load() {
	clearTimeout(timer);
	try {
		data = await api.batchReport(batchID);
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
			h("p", { class: "muted" }, "Bot practice batches. Start one from the home page with Cthulhu Wars selected."),
			batches.length
				? h("ul", { class: "batch-list", id: "batch-list" }, batches.map((b) => h("li", {}, h("div", {}, h("a", { href: `/batches/${b.id}` }, b.name || `Batch ${b.id}`), h("div", { class: "muted small" }, `${b.spec.players.map((p) => `${p.faction.toUpperCase()}:${p.bot}`).join(" ")} · ${new Date(b.created_at).toLocaleString()}`)), h("div", { class: "row" }, h("span", { class: `batch-status ${b.status}` }, b.status === "running" ? `running ${b.done}/${b.games}` : (STATUS_TEXT[b.status] || b.status) + ` · ${b.done}/${b.games}`)))))
				: h("p", {}, "No batches yet.")
		);
		if (batches.some((b) => b.status === "running" || b.status === "queued")) timer = setTimeout(loadList, 3000);
	} catch (err) {
		clear(root);
		root.append(h("p", { class: "error-text" }, err.message));
	}
}

if (root) (batchID ? load() : loadList());
