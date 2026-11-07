// Cthulhu Wars' sections of a practice batch's report (ADR 0124: the batches
// page asks page.js's batchSections for them): win rates per cult, the rounds,
// Doom by source, what goes with winning, the map, spellbooks and battles.

import { h } from "/static/js/util.js";
import { rateBars, lineChart } from "/static/js/charts.js";
import { factionName, factionColor, pretty } from "./cw-names.js";

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

function legend(factions) {
	return h("div", { class: "legend" }, factions.map((f) => h("span", {}, h("i", { style: `background:${factionColor(f.key)}` }), factionName(f.key))));
}

export const endReason = (k) => ({ instant_death: "Instant Death", humanity: "Humanity won", draw: "Draw", ritual: "Ritual win", doom: "30 Doom" }[k] || k);

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

	const tabs = h("div", { class: "batch-tabs", role: "tablist" }, rep.factions.map((x) => h("button", { type: "button", class: x.key === focus ? "on" : "", "data-focus": x.key, onclick: () => { focus = x.key; redraw(); } }, factionName(x.key))));

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

// Draws the report again (a cult's tab chosen): the page's, handed in.
let redraw = () => {};

/** The cults' sections, with the game length (every game's) beside the win rates. */
export function batchSections(rep, other, length, again) {
	if (again) redraw = again;
	return [
		h("div", { class: "report-grid" }, winRateCard(rep, other), length),
		h("div", { class: "report-grid" }, roundsCard(rep, "doom", "Doom over the rounds", "Doom"), roundsCard(rep, "gates", "Gates held", "Gates")),
		h("div", { class: "report-grid" }, roundsCard(rep, "units", "Units on the map", "Units"), roundsCard(rep, "power", "Power in hand", "Power")),
		doomSourcesCard(rep),
		insightCard(rep),
		heatCard(rep),
		reqsCard(rep),
		booksCard(rep),
		battlesCard(rep),
	];
}
