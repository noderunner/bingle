// The home page: a card with three tabs — the games (pick one, then set it up
// and create it, or import a recorded game), the puzzles and the global chat —
// under a fun fact. Joining a game and the games already going live in the
// icons of the top bar (topbar.js).

import { h, clear, store, rememberGame, lastName, toast } from "./util.js";
import { api } from "./api.js";
import { renderBoard } from "./render.js";
import { showRules as openRules } from "./rules.js";
import { syncPractice } from "./practice.js";
import { installTopbar } from "./topbar.js";
import { randomFact } from "./facts.js";
import { mountGlobalChat } from "./global-chat.js";
import { installNav, navigate } from "./nav.js";
import { gameIcon } from "./game-icons.js";

installNav();

const createForm = document.getElementById("create-game-form");

let defs = [];
let populated = false;

// ---- create ---------------------------------------------------------------

function groupedDefs() {
	const groups = new Map();
	for (const d of defs) {
		const fam = d.family || d.id;
		if (!groups.has(fam)) groups.set(fam, []);
		groups.get(fam).push(d);
	}
	// Variants of one family read smallest board first.
	for (const list of groups.values()) {
		list.sort((a, b) => (a.board_width || 0) - (b.board_width || 0) || a.id.localeCompare(b.id));
	}
	// Tak (the flagship) first, then the rest alphabetically by title.
	const ordered = [...groups.entries()].sort(([fa, la], [fb, lb]) => {
		if (fa === "tak-5x5") return -1;
		if (fb === "tak-5x5") return 1;
		return familyName(la).localeCompare(familyName(lb));
	});
	return new Map(ordered);
}

function familyName(list) {
	// "Tak (5×5)" -> "Tak"; a lone game keeps its name without the size of its board ("Gomoku (15×15)" -> "Gomoku").
	const first = list[0].name;
	return list.length > 1 ? first.replace(/\s*\([^)]*\)\s*$/, "") : first.replace(/\s*\(\d+\s*[×x]\s*\d+\)\s*$/, "");
}

function populateSelect() {
	const select = document.getElementById("game_def_id");
	if (!select || !defs.length) return;
	const previous = populated ? select.value : "";
	populated = true;
	clear(select);
	for (const [fam, list] of groupedDefs()) {
		if (list.length === 1) {
			select.append(h("option", { value: list[0].id }, list[0].name));
		} else {
			const g = h("optgroup", { label: familyName(list) });
			for (const d of list) g.append(h("option", { value: d.id }, d.name));
			select.append(g);
		}
	}
	// Prefer Tak 5x5 as the default if nothing was chosen before.
	const requested = new URLSearchParams(window.location.search).get("game");
	const want =
		requested && defs.some((d) => d.id === requested)
			? requested
			: previous && defs.some((d) => d.id === previous)
			? previous
			: defs.some((d) => d.id === "tak-5x5")
			? "tak-5x5"
			: defs[0].id;
	select.value = want;
	// A link to a game (?game=ID) opens its setup at once.
	if (requested && requested === want) openSetup();
	updateBlurb();
}

const infoCache = new Map();

async function gameInfo(id) {
	if (!infoCache.has(id)) infoCache.set(id, await api.gameDef(id));
	return infoCache.get(id);
}

async function updateBlurb() {
	const select = document.getElementById("game_def_id");
	const blurb = document.getElementById("game-blurb");
	if (!select || !blurb) return;
	const id = select.value;
	const d = defs.find((x) => x.id === id);
	blurb.textContent = d ? d.description || "" : "";
	syncPractice(setupOpen ? id : "");

	markGallerySelection();
	renderSetupHead(d);
	updateImportMode(d);
	updateBotMatch(d);
	try {
		const info = await gameInfo(id);
		renderSeatChoices(info);
		renderSetupPreview(id, info);
	} catch (_) {
		/* the default First/Second choice stays */
	}
}

// ---- the setup of the chosen game -------------------------------------------------

let setupOpen = false;

function openSetup() {
	setupOpen = true;
	document.getElementById("gallery-view").hidden = true;
	document.getElementById("game-setup").hidden = false;
	const select = document.getElementById("game_def_id");
	syncPractice(select.value);
	const name = document.getElementById("create_player_name");
	if (name && !name.value) name.value = lastName();
	document.getElementById("browser-card").scrollIntoView({ block: "nearest" });
}

function closeSetup() {
	setupOpen = false;
	document.getElementById("game-setup").hidden = true;
	document.getElementById("gallery-view").hidden = false;
	syncPractice("");
	const url = new URL(window.location.href);
	if (url.searchParams.has("game")) {
		url.searchParams.delete("game");
		window.history.replaceState(null, "", url);
	}
}

/** The title and the variants of the family of the chosen game. */
function renderSetupHead(d) {
	const title = document.getElementById("setup-title");
	const host = document.getElementById("setup-variants");
	if (!title || !host || !d) return;
	const list = defs.filter((x) => (x.family || x.id) === (d.family || d.id));
	title.textContent = list.length > 1 ? familyName(sortedFamily(list)) : d.name;
	clear(host);
	if (list.length < 2) return;
	for (const v of sortedFamily(list)) {
		host.append(
			h(
				"button",
				{
					type: "button",
					class: "chip" + (v.id === d.id ? " on" : ""),
					"data-id": v.id,
					title: v.name,
					"aria-pressed": v.id === d.id ? "true" : "false",
					onclick: () => chooseGame(v.id),
				},
				variantLabel(v)
			)
		);
	}
}

const sortedFamily = (list) => [...list].sort((a, b) => (a.board_width || 0) - (b.board_width || 0) || a.id.localeCompare(b.id));

/** "Havannah (base 8)" -> "base 8"; falls back to the board size. */
function variantLabel(d) {
	const paren = /\(([^)]+)\)\s*$/.exec(d.name);
	return paren ? paren[1] : d.board_width ? `${d.board_width}×${d.board_height}` : d.name;
}

let previewFor = "";
function renderSetupPreview(id, info) {
	const slot = document.getElementById("setup-preview");
	if (!slot || previewFor === id) return;
	previewFor = id;
	try {
		renderBoard(slot, info, { cells: info.preview || info.start.cells, flipped: false, interactive: false, showCoords: false, preview: true }, {});
	} catch (_) {
		clear(slot);
	}
}

// ---- starting from a recorded game ---------------------------------------------------

/** Games a move log can be replayed for: two seats, no chance. */
function importable(d) {
	return !!d && d.seats_required === 2 && !d.id.startsWith("~");
}

function startMode() {
	const checked = createForm.querySelector('input[name="start_mode"]:checked');
	return checked && !document.getElementById("start-mode-field").hidden ? checked.value : "new";
}

function updateImportMode(d) {
	const field = document.getElementById("start-mode-field");
	if (!field) return;
	field.hidden = !importable(d);
	if (field.hidden) createForm.querySelector('input[name="start_mode"][value="new"]').checked = true;
	applyStartMode();
}

function applyStartMode() {
	const importing = startMode() === "import";
	const bots = botMatchChosen();
	createForm.dataset.mode = importing ? "import" : "new";
	createForm.dataset.opponent = bots ? "bots" : "";
	document.getElementById("create-submit").textContent = importing ? "Import game" : bots ? "Watch the bots" : "Create game";
	document.getElementById("create_player_name").required = !importing && !bots;
}

// ---- two bots playing each other ---------------------------------------------------------

/** Games the bots can play against each other: two seats (the others pick bots per seat). */
const botMatchable = (d) => !!d && d.seats_required === 2;

/** True when "Two bots" is the chosen opponent of a game that offers it. */
function botMatchChosen() {
	return !document.getElementById("slots-field") && selectedOpponent() === BOTS_ONLY;
}

function updateBotMatch(d) {
	const choice = document.getElementById("opponent-bots");
	if (!choice) return;
	choice.hidden = !botMatchable(d);
	if (choice.hidden && selectedOpponent() === BOTS_ONLY) createForm.querySelector('input[name="opponent"][value=""]').checked = true;
	applyStartMode();
}

/** Starts two bots against each other, to be watched from a spectator's seat. */
async function startBotMatch(err) {
	const submit = document.getElementById("create-submit");
	submit.disabled = true;
	try {
		const res = await api.botMatch({
			game_def_id: document.getElementById("game_def_id").value,
			bot_a: document.getElementById("botmatch-a").value,
			bot_b: document.getElementById("botmatch-b").value,
			delay_ms: Number(document.getElementById("botmatch-speed").value),
		});
		store.set("bingle.watch." + res.room_code, true);
		navigate("/game/" + res.room_code);
	} catch (e) {
		showError(err, e.message);
	} finally {
		submit.disabled = false;
	}
}

/** Replays the pasted move log in a room of its own, opened for reviewing. */
async function importFromLog(err) {
	const text = document.getElementById("import-text").value;
	if (!text.trim()) {
		showError(err, "Paste a game record or choose a file first.");
		return;
	}
	const id = document.getElementById("game_def_id").value;
	// A Tak record says its board size itself; any other game is the chosen one.
	const sized = /^\s*\[Size\s+"\d+"\]/im.test(text) && id.startsWith("tak-");
	const submit = document.getElementById("create-submit");
	submit.disabled = true;
	try {
		const res = await api.importGame({ text, game_def_id: sized ? undefined : id });
		// Imported rooms are for reviewing: open them as a spectator.
		store.set("bingle.watch." + res.room_code, true);
		navigate("/game/" + res.room_code);
	} catch (e) {
		showError(err, e.message);
	} finally {
		submit.disabled = false;
	}
}

// renderSeatChoices rebuilds the "Play as" control from the game's seats:
// First/Second for two-player games, the faction or color names otherwise.
function renderSeatChoices(info) {
	const host = document.getElementById("seat-choices");
	if (!host || !info || !(info.seats || []).length) return;
	currentSeats = info.seats.map((s) => s.seat);
	const prev = host.querySelector('input[name="seat"]:checked');
	let want = prev ? Number(prev.value) : 1;
	if (!info.seats.some((s) => s.seat === want)) want = info.seats[0].seat;
	clear(host);
	info.seats.forEach((seat, i) => {
		const text = info.seats.length === 2 ? ["First", "Second"][i] : seat.label;
		host.append(
			h(
				"label",
				{},
				h("input", { type: "radio", name: "seat", value: String(seat.seat), checked: seat.seat === want }),
				h("span", {}, text)
			)
		);
	});
	host.classList.toggle("many", info.seats.length > 2);
	// A table of three or more gathers in a lobby by default.
	const lobbyBox = document.getElementById("lobby");
	if (lobbyBox && lobbyBox.dataset.touched !== "1") lobbyBox.checked = info.seats.length > 2;
	renderSlots(info);
	applyStartMode();
}

let botList = [];
// The seat numbers of the game being set up.
let currentSeats = [];

/** The id of the cult the layout itself seats in a seat (matched by name). */
function byLabelDefault(choice, seat) {
	const c = choice.choices.find((x) => x.label === seat.label);
	return c ? c.id : "";
}

// What was chosen for each seat, kept across redraws: choosing another layout
// of the game (or another game and back) rebuilds the control, and must not
// turn every bot back into the default kind.
const slotMemory = new Map();
// Which faction sits in each seat (games with a faction choice).
const factionMemory = new Map();
// Whether an expansion option was ticked, by option id.
const flagMemory = new Map();
// The neutral spellbook chosen to replace a cult's book, by cult id.
const swapMemory = new Map();

// renderSlots lets a table of three or more seats be arranged seat by seat:
// who you are, which seats stay open for friends and which get a bot.
function renderSlots(info) {
	const old = document.getElementById("slots-field");
	if (old) old.remove();
	const opp = document.getElementById("opponent-choices");
	const oppField = opp ? opp.closest(".field") : null;
	const seatField = document.getElementById("seat-choices")?.closest(".field");
	const many = info.seats.length > 2;
	if (oppField) oppField.hidden = many;
	if (seatField) seatField.hidden = many;
	if (!many || !oppField) return;
	const field = h("div", { class: "field", id: "slots-field" }, h("label", {}, "Seats"));
	const rows = h("div", { class: "stack" });
	const remember = () => {
		for (const sel of rows.querySelectorAll(".slot-select")) slotMemory.set(Number(sel.dataset.seat), sel.value);
	};
	const fallback = botList.some((b) => b.id === "tactical") ? "bot:tactical" : "open";
	// A game that lets the creator choose which cult sits where.
	const choice = (info.options || []).find((o) => o.kind === "seat-choice");
	const chosen = [];
	if (choice) {
		const byLabel = new Map(choice.choices.map((c) => [c.label, c.id]));
		const used = new Set();
		info.seats.forEach((seat) => {
			let id = factionMemory.get(seat.seat);
			if (!id || used.has(id) || !choice.choices.some((c) => c.id === id)) id = byLabel.get(seat.label);
			if (!id || used.has(id)) id = (choice.choices.find((c) => !used.has(c.id)) || choice.choices[0]).id;
			used.add(id);
			chosen.push(id);
		});
	}
	info.seats.forEach((seat, i) => {
		const select = h("select", { class: "slot-select", "data-seat": seat.seat, "aria-label": `${seat.label}` });
		select.append(h("option", { value: "me" }, "You"));
		select.append(h("option", { value: "open" }, "A friend (open seat)"));
		for (const b of botList) select.append(h("option", { value: "bot:" + b.id }, `${b.label} bot`));
		// The earlier choice for this seat, when it still means something here.
		select.value = i === 0 ? "me" : fallback;
		const before = slotMemory.get(seat.seat);
		if (before && [...select.options].some((o) => o.value === before)) select.value = before;
		select.addEventListener("change", () => {
			// At least one seat is yours — and you may play several, or all of
			// them, to try a game out alone.
			if (![...rows.querySelectorAll(".slot-select")].some((o) => o.value === "me")) select.value = "me";
			remember();
		});
		let who = h("strong", {}, seat.label);
		let swatch = h("span", { class: `swatch color-${seat.color}` });
		if (choice) {
			// The cult of this seat: a list of the cults, none twice.
			const fsel = h("select", { class: "faction-select", "data-cult-seat": seat.seat, "aria-label": `Faction of seat ${seat.seat}` });
			for (const c of choice.choices) fsel.append(h("option", { value: c.id, title: c.text || null }, c.label));
			fsel.value = chosen[i];
			// What the layout seats here when nothing is chosen.
			fsel.dataset.default = byLabelDefault(choice, seat);
			const paint = () => {
				const c = choice.choices.find((x) => x.id === fsel.value);
				swatch.className = `swatch color-${c ? c.color : seat.color}`;
			};
			paint();
			fsel.dataset.was = fsel.value;
			fsel.addEventListener("change", () => {
				// Choosing a cult that sits elsewhere swaps the two.
				for (const other of rows.querySelectorAll(".faction-select")) {
					if (other !== fsel && other.value === fsel.value) {
						other.value = fsel.dataset.was;
						other.dataset.was = other.value;
						other.dispatchEvent(new Event("painted"));
					}
				}
				fsel.dataset.was = fsel.value;
				paint();
				for (const other of rows.querySelectorAll(".faction-select")) factionMemory.set(Number(other.dataset.cultSeat), other.value);
			});
			fsel.addEventListener("painted", paint);
			who = fsel;
		}
		rows.append(h("div", { class: "slot-row" + (choice ? " with-faction" : "") }, swatch, who, select));
	});
	// At least one seat is yours, whatever the layout had before. (Only what
	// the person chooses is remembered, never these defaults: the list of bots
	// may not have arrived yet when the first draw happens.)
	const mine = [...rows.querySelectorAll(".slot-select")].filter((o) => o.value === "me");
	if (!mine.length) rows.querySelector(".slot-select").value = "me";
	// Setting every bot at once saves opening each list.
	if (botList.length) {
		const all = h("select", { id: "slots-all", "aria-label": "Kind for every bot seat" }, h("option", { value: "" }, "Set every bot to…"));
		for (const b of botList) all.append(h("option", { value: b.id }, `${b.label} bot`));
		all.addEventListener("change", () => {
			if (!all.value) return;
			for (const sel of rows.querySelectorAll(".slot-select")) {
				if (sel.value.startsWith("bot:")) sel.value = "bot:" + all.value;
			}
			all.value = "";
			remember();
		});
		field.append(all);
	}
	field.append(rows, h("span", { class: "small muted" }, "Bots are named after their faction and kind (Black Goat (MCTS)). Open seats are for friends joining with the link — or bots you add in the lobby."));
	// Expansions: a check box each.
	for (const opt of (info.options || []).filter((o) => o.kind === "bool")) {
		const box = h("input", { type: "checkbox", id: `option-${opt.id}`, "data-option": opt.id });
		box.checked = flagMemory.has(opt.id) ? flagMemory.get(opt.id) : !!opt.default;
		box.addEventListener("change", () => flagMemory.set(opt.id, box.checked));
		field.append(
			h(
				"div",
				{ class: "option-flag" },
				h("label", { class: "check", title: opt.text || null }, box, h("span", {}, `${opt.label}`)),
				opt.text ? h("span", { class: "small muted" }, opt.text) : null
			)
		);
	}
	// A cult's spellbook that needs a clock may be swapped for a neutral one.
	const swapOpt = (info.options || []).find((o) => o.kind === "book-swap");
	if (swapOpt && choice) field.append(bookSwapBlock(swapOpt, rows));
	oppField.before(field);
}

/**
 * The "neutral spellbook instead of a timer book" choice: one row per cult that
 * has such a book (Black Goat's Ghroth, Crawling Chaos' Thousand Forms), shown
 * while that cult sits at the table. A neutral spellbook can replace one book
 * only, so what one row chose is taken from the lists of the others.
 */
function bookSwapBlock(opt, rows) {
	const block = h("div", { class: "option-flag book-swap", id: "book-swaps" }, h("span", { class: "small" }, opt.label), opt.text ? h("span", { class: "small muted" }, opt.text) : null);
	const selects = [];
	const present = () => new Set([...rows.querySelectorAll(".faction-select")].map((s) => s.value));
	const sync = () => {
		const here = present();
		const taken = new Set(selects.filter((x) => !x.row.hidden && x.select.value).map((x) => x.select.value));
		for (const x of selects) {
			x.row.hidden = !here.has(x.slot.id);
			for (const o of x.select.options) o.disabled = !!o.value && o.value !== x.select.value && taken.has(o.value);
		}
	};
	for (const slot of opt.slots || []) {
		const select = h("select", { class: "swap-select", "data-swap": slot.id, "aria-label": `Spellbook to replace ${slot.label}` });
		select.append(h("option", { value: "" }, `Keep ${slot.label}`));
		for (const c of opt.choices || []) select.append(h("option", { value: c.id, title: c.text || null }, `Replace with ${c.label}`));
		if (swapMemory.has(slot.id)) select.value = swapMemory.get(slot.id);
		const row = h("label", { class: "swap-row", title: slot.text || null }, h("span", { class: "small" }, `${slot.label}: `), select);
		select.addEventListener("change", () => {
			swapMemory.set(slot.id, select.value);
			sync();
		});
		selects.push({ slot, select, row });
		block.append(row);
	}
	rows.addEventListener("change", sync);
	sync();
	return block;
}

/**
 * The seat arrangement chosen in the form: {seat, seatBots, options}. options
 * is only there when the table differs from the game's default one: the cults
 * in seat order and the expansions ticked.
 */
function slotChoice() {
	const out = { seat: 0, also: [], seatBots: {}, options: null };
	for (const sel of document.querySelectorAll("#slots-field .slot-select")) {
		const seat = Number(sel.dataset.seat);
		if (sel.value === "me") {
			// The first seat of yours is the one you sit in; the others are yours too.
			if (out.seat === 0) out.seat = seat;
			else out.also.push(seat);
		} else if (sel.value.startsWith("bot:")) out.seatBots[seat] = sel.value.slice(4);
	}
	const options = {};
	const fsels = [...document.querySelectorAll("#slots-field .faction-select")];
	if (fsels.length) {
		const chosen = fsels.map((s) => s.value);
		const defaults = fsels.map((s) => s.dataset.default);
		if (chosen.join(",") !== defaults.join(",")) options.factions = chosen;
	}
	for (const box of document.querySelectorAll("#slots-field input[data-option]")) {
		if (box.checked) options[box.dataset.option] = true;
	}
	// Neutral spellbooks in place of the timer books of cults at the table.
	const swaps = {};
	for (const sel of document.querySelectorAll("#slots-field select.swap-select")) {
		if (sel.value && !sel.closest(".swap-row").hidden) swaps[sel.dataset.swap] = sel.value;
	}
	if (Object.keys(swaps).length) options["book-swaps"] = swaps;
	if (Object.keys(options).length) {
		// Expansions alone keep the layout's own cults.
		if (!options.factions && fsels.length) options.factions = fsels.map((s) => s.value);
		out.options = options;
	}
	return out;
}

// ---- gallery -----------------------------------------------------------------

function markGallerySelection() {
	const select = document.getElementById("game_def_id");
	if (!select) return;
	const sel = defs.find((d) => d.id === select.value);
	for (const card of document.querySelectorAll(".game-card")) {
		const selected = !!sel && card.dataset.family === (sel.family || sel.id);
		card.classList.toggle("selected", selected);
		card.setAttribute("aria-pressed", selected ? "true" : "false");
		for (const chip of card.querySelectorAll(".chip")) {
			chip.classList.toggle("on", selected && chip.dataset.id === sel.id);
		}
	}
}

function chooseGame(id) {
	const select = document.getElementById("game_def_id");
	if (!select) return;
	select.value = id;
	select.dispatchEvent(new Event("change"));
	if (!setupOpen) openSetup();
}

let lastPicked = "";

/** A version below 1.0.0 is a beta: the game is not yet considered finished. */
export function isBeta(version) {
	const major = parseInt(String(version || "0").split(".")[0], 10);
	return !(major >= 1);
}

/** "2 players", "3–5 players": how many can sit at the game's tables (its variants included). */
function playerRange(list) {
	const seats = list.map((d) => d.seats_required).filter((n) => n > 0);
	if (!seats.length) return "";
	const lo = Math.min(...seats);
	const hi = Math.max(...seats);
	return lo === hi ? `${lo} players` : `${lo}\u2013${hi} players`;
}

/** The line under a game's name: the Beta mark (while its version is below 1.0) and the players it takes. */
function cardMeta(list, lead) {
	const out = [];
	if (isBeta(lead.version)) {
		out.push(h("span", { class: "badge beta", title: `Version ${lead.version || "0.0.0"}: still being tested` }, "Beta"));
	}
	const players = playerRange(list);
	if (players) out.push(h("span", { class: "game-card-players muted" }, players));
	return out;
}

// ---- the card shown while the pointer is over a game ----------------------------------

let gameTip = null;
let tipFor = null;

function hideGameTip() {
	tipFor = null;
	if (gameTip) gameTip.hidden = true;
}

function showGameTip(card, list, lead) {
	if (!gameTip) {
		gameTip = h("div", { id: "game-tip", class: "game-tip", role: "tooltip", hidden: true });
		document.body.append(gameTip);
	}
	tipFor = card;
	const designers = lead.designers || [];
	// Only what is known is listed: no line for a missing designer or publisher.
	// (Element.append would write the text "null" for a null child, so h() builds it.)
	clear(gameTip).append(
		h(
			"div",
			{ class: "game-tip-card" },
			h("div", { class: "game-tip-name" }, familyName(list)),
			designers.length ? h("div", { class: "game-tip-line" }, h("span", { class: "muted" }, designers.length > 1 ? "Designers: " : "Designer: "), designers.join(", ")) : null,
			lead.publisher ? h("div", { class: "game-tip-line" }, h("span", { class: "muted" }, "Publisher: "), lead.publisher) : null,
			lead.description ? h("p", { class: "game-tip-text" }, lead.description) : null,
			h("div", { class: "game-tip-version muted small" }, `Version ${lead.version || "0.0.0"}${isBeta(lead.version) ? " (beta)" : ""}`)
		)
	);
	card.setAttribute("aria-describedby", "game-tip");
	gameTip.hidden = false;
	// Beside the card when there is room under it, else above.
	const r = card.getBoundingClientRect();
	const tw = gameTip.offsetWidth;
	const th = gameTip.offsetHeight;
	let x = Math.min(Math.max(8, r.left + r.width / 2 - tw / 2), window.innerWidth - tw - 8);
	let y = r.bottom + 8;
	if (y + th > window.innerHeight - 8) y = Math.max(8, r.top - th - 8);
	gameTip.style.left = `${x + window.scrollX}px`;
	gameTip.style.top = `${y + window.scrollY}px`;
}

function attachGameTip(card, list, lead) {
	card.addEventListener("mouseenter", () => showGameTip(card, list, lead));
	card.addEventListener("mouseleave", hideGameTip);
	card.addEventListener("focus", () => showGameTip(card, list, lead));
	card.addEventListener("blur", hideGameTip);
	card.addEventListener("keydown", (ev) => {
		if (ev.key === "Escape" && tipFor === card) hideGameTip();
	});
}

async function renderGallery() {
	const host = document.getElementById("game-gallery");
	if (!host || !defs.length) return;
	clear(host);

	const cards = [];
	for (const [family, list] of groupedDefs()) {
		// The family's headline game: the base document, or the first variant.
		const lead = list.find((d) => d.id === family) || list[0];
		const card = h(
			"div",
			{ class: "game-card", "data-family": family, role: "button", tabindex: 0, "aria-pressed": "false" },
			h("div", { class: "game-card-preview", "data-preview": lead.id }),
			h("div", { class: "game-card-title" }, familyName(list)),
			h("div", { class: "game-card-meta small" }, ...cardMeta(list, lead)),
		);
		attachGameTip(card, list, lead);
		const pick = () => {
			hideGameTip();
			// Keep the variant already chosen within this family.
			const select = document.getElementById("game_def_id");
			const cur = defs.find((d) => d.id === select.value);
			chooseGame(cur && (cur.family || cur.id) === family && lastPicked === family ? cur.id : lead.id);
			lastPicked = family;
		};
		card.addEventListener("click", pick);
		card.addEventListener("keydown", (ev) => {
			if (ev.key === "Enter" || ev.key === " ") {
				ev.preventDefault();
				pick();
			}
		});
		host.append(card);
		cards.push({ card, lead });
	}
	markGallerySelection();

	// Fill in the previews as their descriptions arrive.
	await Promise.all(
		cards.map(async ({ card, lead }) => {
			try {
				const slot = card.querySelector(".game-card-preview");
				const icon = gameIcon(lead.id);
				if (icon) {
					slot.append(icon);
					return;
				}
				const info = await gameInfo(lead.id);
				renderBoard(slot, info, { cells: info.preview || info.start.cells, flipped: false, interactive: false, showCoords: false, preview: true }, {});
			} catch (_) {
				/* a card without a preview is still usable */
			}
		})
	);
}

function showRules() {
	const select = document.getElementById("game_def_id");
	if (!select) return;
	gameInfo(select.value).then((info) => openRules(info));
}

// The value of the opponent choice "Myself": the creator plays every seat.
const SELF = "@self";
// The value of the opponent choice "Two bots": nobody plays, the creator watches.
const BOTS_ONLY = "@bots";

function selectedOpponent() {
	const checked = createForm.querySelector('input[name="opponent"]:checked');
	return checked ? checked.value : "";
}

function selectedSeat() {
	const checked = createForm.querySelector('input[name="seat"]:checked');
	return checked ? Number(checked.value) : 1;
}

function showError(el, message) {
	if (el) el.textContent = message || "";
}

async function setupBots() {
	const host = document.getElementById("opponent-choices");
	if (!host) return;
	let bots = [];
	try {
		bots = await api.bots();
	} catch (_) {
		bots = [];
	}
	const add = (value, label, desc, checked) => {
		host.append(
			h(
				"label",
				{ class: "opponent-choice" },
				h("input", { type: "radio", name: "opponent", value, checked }),
				h("span", {}, h("strong", {}, label), h("br"), h("span", { class: "small muted" }, desc))
			)
		);
	};
	botList = bots;
	clear(host);
	add("", "A friend", "You get a link to share; the game starts when they join.", true);
	add(SELF, "Myself", "You play both sides — to try ideas out or play a mock game alone.", false);
	for (const b of bots) add(b.id, b.label + " bot", b.description, false);
	add(BOTS_ONLY, "Two bots", "Nobody plays: you watch two bots play each other.", false);
	host.lastElementChild.id = "opponent-bots";
	host.lastElementChild.hidden = true;
	for (const sel of ["botmatch-a", "botmatch-b"]) {
		const el = document.getElementById(sel);
		clear(el);
		for (const b of bots) el.append(h("option", { value: b.id }, b.label || b.id));
	}
	const pick = (id, bot) => {
		if (bots.some((b) => b.id === bot)) document.getElementById(id).value = bot;
	};
	pick("botmatch-a", "mcts");
	pick("botmatch-b", "tactical");
	// Multi-seat games pick bots per seat: refresh that control.
	updateBlurb();
}

if (createForm) {
	const nameInput = document.getElementById("create_player_name");
	if (nameInput && !nameInput.value) nameInput.value = lastName();
	const err = document.getElementById("create-error");

	createForm.addEventListener("submit", async (event) => {
		event.preventDefault();
		showError(err, "");
		if (startMode() === "import") {
			await importFromLog(err);
			return;
		}
		if (botMatchChosen()) {
			await startBotMatch(err);
			return;
		}
		const gameDefID = document.getElementById("game_def_id").value;
		const playerName = nameInput.value.trim();
		if (!playerName) {
			nameInput.focus();
			return;
		}
		const body = { game_def_id: gameDefID, player_name: playerName };
		if (document.getElementById("slots-field")) {
			const slots = slotChoice();
			if (slots.seat && slots.seat !== 1) body.seat = slots.seat;
			if (slots.also.length) body.also_seats = slots.also;
			if (Object.keys(slots.seatBots).length) body.seat_bots = slots.seatBots;
			if (slots.options) body.options = slots.options;
		} else {
			const opp = selectedOpponent();
			const seat = selectedSeat();
			if (opp === SELF) {
				// Both sides: the other seat is yours too.
				body.also_seats = (currentSeats || []).filter((n) => n !== seat);
			} else if (opp) {
				body.opponent = opp;
			}
			if (seat !== 1) body.seat = seat;
		}
		const limit = Number(document.getElementById("move_time_seconds").value);
		if (limit > 0) body.move_time_seconds = limit;
		const lobbyBox = document.getElementById("lobby");
		if (lobbyBox && lobbyBox.checked) body.lobby = true;
		const submit = createForm.querySelector('button[type="submit"]');
		submit.disabled = true;
		try {
			const res = await api.createGame(body);
			rememberGame(res.room_code, gameDefID, playerName);
			navigate("/game/" + res.room_code);
		} catch (e) {
			showError(err, e.message);
			console.error("create game failed:", e);
		} finally {
			submit.disabled = false;
		}
	});

	const select = document.getElementById("game_def_id");
	if (select) select.addEventListener("change", updateBlurb);
	const lobbyBoxEl = document.getElementById("lobby");
	if (lobbyBoxEl) lobbyBoxEl.addEventListener("change", () => (lobbyBoxEl.dataset.touched = "1"));
	const rulesButton = document.getElementById("game-rules-button");
	if (rulesButton) rulesButton.addEventListener("click", showRules);
	document.getElementById("setup-back").addEventListener("click", closeSetup);
	document.getElementById("opponent-choices").addEventListener("change", applyStartMode);
	for (const radio of createForm.querySelectorAll('input[name="start_mode"]')) radio.addEventListener("change", applyStartMode);
	// A file fills the box for the move log.
	const fileInput = document.getElementById("import-file");
	fileInput.addEventListener("change", async () => {
		const file = fileInput.files && fileInput.files[0];
		if (!file) return;
		if (file.size > 256 << 10) {
			showError(err, "That file is too large for a move log (256 KB at most).");
			fileInput.value = "";
			return;
		}
		showError(err, "");
		document.getElementById("import-text").value = await file.text();
	});
}

// ---- puzzles -----------------------------------------------------------------

let puzzles = [];
let puzzleGame = "";
let puzzlesExpanded = false;
const PUZZLES_COLLAPSED = 6;

async function renderPuzzles() {
	const list = document.getElementById("puzzle-list");
	if (!list) return;
	try {
		puzzles = await api.puzzles();
	} catch (_) {
		puzzles = [];
	}
	if (!puzzles.length) {
		clear(list).append(h("p", { class: "muted small" }, "No puzzles yet."));
		return;
	}
	const games = [...new Set(puzzles.map((p) => p.game_def_id))];
	if (!puzzleGame || !games.includes(puzzleGame)) puzzleGame = games.includes("tak-5x5") ? "tak-5x5" : games[0];

	const tabs = document.getElementById("puzzle-games");
	clear(tabs);
	for (const g of games) {
		const def = defs.find((d) => d.id === g);
		tabs.append(
			h(
				"button",
				{
					type: "button",
					class: "chip" + (g === puzzleGame ? " on" : ""),
					"data-id": g,
					onclick: () => {
						puzzleGame = g;
						puzzlesExpanded = false;
						renderPuzzles();
					},
				},
				def ? def.name : g
			)
		);
	}

	const shown = puzzles.filter((x) => x.game_def_id === puzzleGame);
	tabs.append(
		h(
			"button",
			{
				type: "button",
				class: "ghost",
				id: "random-puzzle",
				title: "Start a random puzzle of this game",
				onclick: () => startPuzzle(shown[Math.floor(Math.random() * shown.length)]),
			},
			"🎲 Random"
		)
	);

	clear(list);
	const visible = puzzlesExpanded ? shown : shown.slice(0, PUZZLES_COLLAPSED);
	for (const p of visible) {
		const slot = h("div", { class: "puzzle-preview" });
		const item = h(
			"div",
			{ class: "puzzle-card", "data-puzzle": p.id },
			slot,
			h("div", { class: "puzzle-title" }, p.title.split(":")[0]),
			h("div", { class: "small muted" }, p.win_in === 1 ? "Win at once" : `Win in ${p.win_in}`),
			h("button", { class: "primary", type: "button", onclick: () => startPuzzle(p) }, "Solve")
		);
		list.append(item);
		gameInfo(p.game_def_id)
			.then((info) => renderBoard(slot, info, { cells: (p.state && p.state.cells) || {}, flipped: false, interactive: false, showCoords: false }, {}))
			.catch(() => {});
	}
	if (shown.length > PUZZLES_COLLAPSED) {
		list.append(
			h(
				"button",
				{
					type: "button",
					class: "ghost",
					id: "puzzles-toggle",
					onclick: () => {
						puzzlesExpanded = !puzzlesExpanded;
						renderPuzzles();
					},
				},
				puzzlesExpanded ? "Show fewer" : `Show all ${shown.length}`
			)
		);
	}
}

async function startPuzzle(p) {
	const nameInput = document.getElementById("puzzle_player_name");
	const name = ((nameInput && nameInput.value) || lastName() || "").trim();
	if (!name) {
		toast("Enter your name first.", "error");
		if (nameInput) nameInput.focus();
		return;
	}
	try {
		const res = await api.createGame({
			game_def_id: p.game_def_id,
			player_name: name,
			opponent: "tactical",
			seat: p.to_move,
			start_state: p.state,
			start_seat: p.to_move,
		});
		rememberGame(res.room_code, p.game_def_id, name);
		navigate("/game/" + res.room_code);
	} catch (e) {
		toast(e.message, "error");
	}
}


// ---- tabs ---------------------------------------------------------------------------

const TABS = ["games", "puzzles", "chat"];
let chat = null;

function showTab(name, { focus = false } = {}) {
	if (!TABS.includes(name)) name = "games";
	for (const t of TABS) {
		const tab = document.getElementById("tab-" + t);
		const on = t === name;
		tab.setAttribute("aria-selected", on ? "true" : "false");
		tab.tabIndex = on ? 0 : -1;
		document.getElementById("panel-" + t).hidden = !on;
		if (on && focus) tab.focus();
	}
	if (name === "chat" && chat) {
		chat.seen();
		chat.focus();
	}
	const url = new URL(window.location.href);
	url.hash = name === "games" ? "" : name;
	window.history.replaceState(null, "", url);
}

function setupTabs() {
	const tabs = TABS.map((t) => document.getElementById("tab-" + t));
	for (const tab of tabs) {
		tab.addEventListener("click", () => showTab(tab.dataset.tab));
		tab.addEventListener("keydown", (ev) => {
			const i = tabs.indexOf(tab);
			const to = ev.key === "ArrowRight" ? i + 1 : ev.key === "ArrowLeft" ? i - 1 : ev.key === "Home" ? 0 : ev.key === "End" ? tabs.length - 1 : -1;
			if (to < 0) return;
			ev.preventDefault();
			showTab(TABS[(to + tabs.length) % tabs.length], { focus: true });
		});
	}
	const badge = document.getElementById("chat-tab-badge");
	chat = mountGlobalChat(document.getElementById("panel-chat"), {
		visible: () => !document.hidden && !document.getElementById("panel-chat").hidden,
		onUnread: (n) => {
			badge.textContent = String(n);
			badge.hidden = n === 0;
		},
	});
	showTab(window.location.hash.slice(1));
}

// ---- boot -----------------------------------------------------------------------

(async () => {
	const fact = document.getElementById("fun-fact");
	if (fact) fact.textContent = randomFact();
	installTopbar();
	setupTabs();
	const name = document.getElementById("puzzle_player_name");
	if (name) name.value = lastName();
	try {
		defs = await api.gameDefs();
	} catch (e) {
		toast("Could not load the game list.", "error");
	}
	populateSelect();
	renderGallery();
	renderPuzzles();
	await setupBots();
})();
