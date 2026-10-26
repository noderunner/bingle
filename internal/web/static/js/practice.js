// The practice-batch wizard on the lobby page: set up a table of bots, run
// hundreds of simulated Cthulhu Wars games in the background and study the
// result in the batch reports (/batches).

import { h, clear, store } from "./util.js";
import { api } from "./api.js";
import { FACTION_KEYS, factionName, factionColor, BOTS } from "./cw-names.js";

const card = document.getElementById("practice-card");
const STORE_KEY = "bingle.practice";

let defs = [];
let loaded = false;
let step = 0;
let workers = 12;
let busy = false;
let model = null;

// Single-core minutes one bot adds to a game (summed over the seats, divided by
// four), measured on the 4-player table: tactical and minimax play a game in
// about 8 s, MCTS at 20 iterations per move in about 90 s.
function seatCost(p) {
	switch (p.bot) {
		case "mcts":
			return 0.14 + (1.4 * (p.iters || 50)) / 20;
		case "minimax":
			return 0.14 + (0.02 * (p.nodes || 30000)) / 30000;
		case "random":
			return 0.12;
		default:
			return 0.14;
	}
}

export function estimateMinutes(seats, games, nWorkers) {
	const perGame = seats.reduce((n, p) => n + seatCost(p), 0) / 4;
	return (games * perGame) / Math.max(1, nWorkers * 0.8);
}

export function fmtDuration(min) {
	if (min < 1) return "under a minute";
	if (min < 90) return `about ${Math.round(min)} min`;
	const hours = min / 60;
	return hours < 48 ? `about ${hours.toFixed(1)} h` : `about ${(hours / 24).toFixed(1)} days`;
}

function defaults(def) {
	const n = def ? def.seats_required : 4;
	const seats = [];
	for (let i = 0; i < n; i++) seats.push({ faction: FACTION_KEYS[i % FACTION_KEYS.length], bot: "tactical", iters: 50, nodes: 30000 });
	return { layout: def ? def.id : "cthulhu-wars", hp: false, seats, games: 100, seed: 1, rotate: true, name: "", maxPlies: 3000 };
}

function layouts() {
	return defs.filter((d) => (d.family || d.id) === "cthulhu-wars");
}

function chooseLayout(id) {
	const def = defs.find((d) => d.id === id);
	if (!def) return;
	const old = model.seats;
	model.layout = id;
	const fresh = defaults(def).seats;
	model.seats = fresh.map((s, i) => (old[i] ? { ...old[i] } : s));
	// Cults must be unique: refill any clash.
	const used = new Set();
	for (const s of model.seats) {
		if (used.has(s.faction)) s.faction = FACTION_KEYS.find((k) => !used.has(k) && !model.seats.some((o) => o !== s && o.faction === k)) || s.faction;
		used.add(s.faction);
	}
}

// ---- steps -------------------------------------------------------------------

function stepTable() {
	const def = defs.find((d) => d.id === model.layout);
	const layoutSel = h(
		"select",
		{ id: "practice-layout", onchange: (e) => { chooseLayout(e.target.value); render(); } },
		layouts().map((d) => h("option", { value: d.id, selected: d.id === model.layout }, d.name))
	);
	const seats = model.seats.map((s, i) => {
		const sel = h(
			"select",
			{
				"data-practice-cult": String(i),
				onchange: (e) => {
					const want = e.target.value;
					const other = model.seats.findIndex((o, j) => j !== i && o.faction === want);
					if (other >= 0) model.seats[other].faction = s.faction; // swap on a clash
					s.faction = want;
					render();
				},
			},
			FACTION_KEYS.map((k) => h("option", { value: k, selected: k === s.faction }, factionName(k)))
		);
		return h("div", { class: "practice-seat" }, h("span", { class: "practice-swatch", style: `background:${factionColor(s.faction)}` }), h("span", { class: "muted small" }, `Seat ${i + 1}`), sel);
	});
	return [
		h("p", { class: "muted small" }, "Pick the map and which cult sits in each seat. Seats are shifted round the table game by game, so the order of play does not favour anyone."),
		h("div", { class: "field" }, h("label", { for: "practice-layout" }, "Map"), layoutSel),
		h("div", { class: "practice-seats" }, seats),
		h(
			"label",
			{ class: "check" },
			h("input", { type: "checkbox", id: "practice-hp", checked: model.hp, onchange: (e) => (model.hp = e.target.checked) }),
			" ",
			h("span", {}, "High Priests expansion")
		),
		def ? h("p", { class: "muted small" }, `${def.seats_required} players.`) : null,
	];
}

function stepBots() {
	const preset = (label, bot, extra = {}) =>
		h("button", { type: "button", class: "ghost", onclick: () => { model.seats.forEach((s) => Object.assign(s, { bot }, extra)); render(); } }, label);
	const rows = model.seats.map((s, i) => {
		const kind = h(
			"select",
			{ "data-practice-bot": String(i), onchange: (e) => { s.bot = e.target.value; render(); } },
			BOTS.map((b) => h("option", { value: b.id, selected: b.id === s.bot, title: b.help }, b.label))
		);
		const knobs = [];
		if (s.bot === "mcts") {
			knobs.push(
				h("label", { class: "inline small" }, "Iterations ", h("input", { type: "number", min: 5, max: 5000, value: s.iters, "data-practice-iters": String(i), onchange: (e) => (s.iters = Math.max(5, Number(e.target.value) || 50)) }))
			);
		}
		if (s.bot === "minimax") {
			knobs.push(
				h("label", { class: "inline small" }, "Nodes ", h("input", { type: "number", min: 1000, max: 500000, step: 1000, value: s.nodes, "data-practice-nodes": String(i), onchange: (e) => (s.nodes = Math.max(1000, Number(e.target.value) || 30000)) }))
			);
		}
		return h("div", { class: "practice-seat" }, h("span", { class: "practice-swatch", style: `background:${factionColor(s.faction)}` }), h("strong", {}, factionName(s.faction)), kind, knobs);
	});
	return [
		h("p", { class: "muted small" }, "Choose how each cult is played. MCTS searches more with more iterations — stronger, but slower to simulate."),
		h("div", { class: "row" }, h("span", { class: "muted small" }, "Everyone:"), preset("Tactical", "tactical"), preset("Minimax", "minimax"), preset("MCTS (50)", "mcts", { iters: 50 })),
		h("div", { class: "practice-seats" }, rows),
		h("p", { class: "muted small" }, BOTS.map((b) => `${b.label}: ${b.help}`).join(" ")),
	];
}

function stepRun() {
	const est = estimateMinutes(model.seats, model.games, workers);
	const num = (id, key, min, max, label, help) =>
		h("div", { class: "field" }, h("label", { for: id }, label), h("input", { type: "number", id, min, max, value: model[key], onchange: (e) => { model[key] = Math.min(max, Math.max(min, Number(e.target.value) || min)); render(); } }), help ? h("span", { class: "muted small" }, help) : null);
	return [
		h("div", { class: "practice-run" },
			num("practice-games", "games", 1, 5000, "Games", "More games make the numbers firmer: a win rate needs a few hundred."),
			num("practice-seed", "seed", 0, 2147483647, "Seed", "The same seed replays the same games."),
		),
		h("div", { class: "field" }, h("label", { for: "practice-name" }, "Name (optional)"), h("input", { type: "text", id: "practice-name", maxlength: 60, value: model.name, onchange: (e) => (model.name = e.target.value) })),
		h("label", { class: "check" }, h("input", { type: "checkbox", id: "practice-rotate", checked: model.rotate, onchange: (e) => (model.rotate = e.target.checked) }), " ", h("span", {}, "Rotate the seats every game (recommended)")),
		h("div", { class: "practice-summary" },
			h("div", {}, h("strong", {}, `${model.games} games`), ` on ${layouts().find((d) => d.id === model.layout)?.name || model.layout}${model.hp ? " with High Priests" : ""}`),
			h("div", { class: "muted small" }, model.seats.map((s) => `${factionName(s.faction)}: ${s.bot}${s.bot === "mcts" ? ` ${s.iters}` : ""}`).join(" · ")),
			h("div", {}, "Estimated time: ", h("strong", { id: "practice-estimate" }, fmtDuration(est)), h("span", { class: "muted small" }, ` (${workers} games at once; a guess from measured speeds)`)),
		),
	];
}

const STEPS = [
	{ title: "Table", body: stepTable },
	{ title: "Bots", body: stepBots },
	{ title: "Run", body: stepRun },
];

async function start(errBox) {
	busy = true;
	render();
	try {
		const body = {
			name: model.name.trim(),
			base: model.layout,
			high_priests: model.hp,
			games: model.games,
			seed: model.seed,
			rotate: model.rotate,
			max_plies: model.maxPlies,
			event_games: 20,
			players: model.seats.map((s) => ({
				faction: s.faction,
				bot: s.bot,
				...(s.bot === "mcts" ? { mcts_iterations: s.iters } : {}),
				...(s.bot === "minimax" ? { minimax_nodes: s.nodes } : {}),
			})),
		};
		store.set(STORE_KEY, model);
		const info = await api.startBatch(body);
		window.location.href = `/batches/${info.id}`;
	} catch (err) {
		busy = false;
		render();
		const box = card.querySelector("#practice-error");
		if (box) box.textContent = err.message;
	}
}

// ---- the card ----------------------------------------------------------------

function render() {
	if (!card || !model) return;
	const host = card.querySelector("#practice-body");
	clear(host);
	const s = STEPS[step];
	host.append(
		h("ol", { class: "practice-steps" }, STEPS.map((x, i) => h("li", { class: i === step ? "on" : i < step ? "done" : "" }, h("button", { type: "button", class: "ghost", onclick: () => { step = i; render(); } }, `${i + 1}. ${x.title}`)))),
		h("div", { class: "practice-step stack" }, s.body()),
		h("div", { class: "error-text", id: "practice-error", role: "alert" }),
		h(
			"div",
			{ class: "row" },
			step > 0 ? h("button", { type: "button", id: "practice-back", onclick: () => { step--; render(); } }, "Back") : null,
			step < STEPS.length - 1
				? h("button", { type: "button", class: "primary", id: "practice-next", onclick: () => { step++; render(); } }, "Next")
				: h("button", { type: "button", class: "primary", id: "practice-start", disabled: busy, onclick: () => start() }, busy ? "Starting…" : "Start the batch")
		)
	);
}

async function renderRecent() {
	const host = card.querySelector("#practice-recent");
	if (!host) return;
	try {
		const data = await api.batches();
		workers = data.default_workers || workers;
		const list = (data.batches || []).slice(0, 5);
		clear(host);
		if (!list.length) return;
		host.append(
			h("h3", {}, "Recent batches"),
			h("ul", { class: "practice-recent" },
				list.map((b) => h("li", {}, h("a", { href: `/batches/${b.id}` }, b.name || `Batch ${b.id}`), h("span", { class: `batch-status ${b.status}` }, b.status === "running" ? `running ${b.done}/${b.games}` : b.status === "done" ? `${b.games} games` : b.status))),
			),
			h("a", { href: "/batches" }, "All batches and reports →")
		);
	} catch (_) {
		/* the list is a convenience */
	}
}

/** Called by the lobby whenever the selected game changes. */
export async function syncPractice(gameID) {
	if (!card) return;
	const isCW = typeof gameID === "string" && gameID.startsWith("cthulhu-wars");
	card.classList.toggle("hidden", !isCW);
	if (!isCW) return;
	if (!loaded) {
		loaded = true;
		try {
			const data = await api.gameDefs();
			defs = Array.isArray(data) ? data : data.game_defs || [];
		} catch (_) {
			defs = [];
		}
		const base = defs.find((d) => d.id === gameID) || layouts()[0];
		model = defaults(base);
		const saved = store.get(STORE_KEY, null);
		if (saved && saved.seats && layouts().some((d) => d.id === saved.layout) && saved.seats.length === (layouts().find((d) => d.id === saved.layout)?.seats_required || 0)) {
			model = { ...model, ...saved };
		}
		await renderRecent();
		render();
	}
}

if (card) {
	card.querySelector("#practice-toggle")?.addEventListener("click", () => {
		const body = card.querySelector("#practice-wizard");
		body.classList.toggle("hidden");
		if (!body.classList.contains("hidden")) renderRecent();
	});
}
