// The home page: a card with three tabs — the games (pick one, then set it up
// and create it, or import a recorded game), the puzzles and the global chat —
// under a fun fact. Joining a game and the games already going live in the
// icons of the top bar (topbar.js).

import { CARD_SETS, cardSetById, setPreview, useCardSet } from "./card-sets.js";
import { h, clear, store, rememberGame, lastName, toast } from "./util.js";
import { api, gameTitle, whoAmI } from "./api.js";
import { rulesIcon } from "./ui-icons.js";
import { mountAnalysis } from "./analysis.js";
import { mountRatings } from "./ratings.js";
import { mountTournaments } from "./tournaments.js";
import { renderBoard } from "./render.js";
import { showRules as openRules } from "./rules.js";
import { syncPractice } from "./practice.js";
import { installTopbar } from "./topbar.js";
import { randomFact } from "./facts.js";
import { mountGlobalChat } from "./global-chat.js";
import { installNav, navigate } from "./nav.js";
import { gameIcon } from "./game-icons.js";
import { tutorialList } from "./tutorials-ui.js";
import { solitairePreview } from "./solitaire.js";
import { createCwSetup } from "./cw-setup.js";

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

/** Whether d is the base document of its family; each version of a prototype has one. */
const isBaseGame = (d) => d.id === (d.prototype ? `${d.family}-${d.version}` : d.family || d.id);

/** Two major.minor.patch versions in order, by number: 1.10.0 follows 1.9.0. */
function compareVersions(a, b) {
	const [x, y] = [a, b].map((v) => String(v).split(".").map(Number));
	return x[0] - y[0] || x[1] - y[1] || x[2] - y[2];
}

/** The versions of a prototype's rules in a list of its games, newest first. */
const rulesVersions = (list) => [...new Set(list.map((d) => d.version))].sort((a, b) => compareVersions(b, a));

/** The family's headline game: the base document (a prototype's newest), or the first variant. */
function leadOf(list) {
	const games = list[0].prototype ? list.filter((d) => d.version === rulesVersions(list)[0]) : list;
	return games.find(isBaseGame) || games[0];
}

function familyName(list) {
	// "Tak (5×5)" -> "Tak"; a lone game keeps its name without the size of its board ("Gomoku (15×15)" -> "Gomoku").
	const first = leadOf(list).name;
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
			select.append(h("option", { value: list[0].id }, gameTitle(list[0])));
		} else {
			const g = h("optgroup", { label: familyName(list) });
			for (const d of list) g.append(h("option", { value: d.id }, gameTitle(d)));
			select.append(g);
		}
	}
	// Prefer Tak 5x5 as the default if nothing was chosen before.
	// A link may name a family, which is a prototype's only lasting name.
	const asked = new URLSearchParams(window.location.search).get("game");
	const askedFamily = asked && groupedDefs().get(asked);
	const requested = askedFamily ? leadOf(askedFamily).id : asked;
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
	renderSetupTutorials(d);
	updateImportMode(d);
	updateBotMatch(d);
	try {
		const info = await gameInfo(id);
		await preparePicker(info);
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

/** The game of another version of d's rules: the same variant if it has one, else its base game. */
function inVersion(d, version, family) {
	const games = family.filter((x) => x.version === version);
	const same = d.id.replace(`${d.family}-${d.version}`, `${d.family}-${version}`);
	return (games.find((x) => x.id === same) || games.find(isBaseGame) || games[0]).id;
}

/** The choice of a prototype's rules: which version, and a word on what that means. */
function rulesVersionPicker(d, family) {
	const picker = h("select", { id: "setup-rules-version", onchange: (ev) => chooseGame(inVersion(d, ev.target.value, family)) });
	rulesVersions(family).forEach((v, i) => picker.append(h("option", { value: v, selected: v === d.version }, i === 0 ? `${v} (newest)` : v)));
	picker.value = d.version;
	return [
		h("label", { class: "setup-variant-label", for: "setup-rules-version" }, "Rules version"),
		picker,
		h("p", { class: "muted small setup-prototype-note" }, "This game is a prototype: its rules are still changing. A game keeps the version it starts with."),
	];
}

/** The title, the rules version and the variants of the family of the chosen game. */
function renderSetupHead(d) {
	const title = document.getElementById("setup-title");
	const host = document.getElementById("setup-variants");
	if (!title || !host || !d) return;
	const family = defs.filter((x) => (x.family || x.id) === (d.family || d.id));
	// The variants offered are those of the version chosen.
	const list = d.prototype ? family.filter((x) => x.version === d.version) : family;
	title.textContent = family.length > 1 ? familyName(sortedFamily(family)) : d.name;
	clear(host);
	if (d.prototype) host.append(...rulesVersionPicker(d, family));
	if (list.length < 2) return;
	const picker = h("select", { id: "setup-variant", onchange: (ev) => chooseGame(ev.target.value) });
	for (const v of sortedFamily(list)) picker.append(h("option", { value: v.id, selected: v.id === d.id }, v.name));
	picker.value = d.id;
	host.append(h("label", { class: "setup-variant-label", for: "setup-variant" }, "Variant"), picker);
}

// The tutorials offered with each game, fetched once.
const tutorialsOf = new Map();

/** "Learn to play": the chosen game's tutorials, played under the name in the form. */
async function renderSetupTutorials(d) {
	const host = document.getElementById("setup-tutorials");
	if (!host) return;
	if (!d || !d.tutorials) {
		host.hidden = true;
		clear(host);
		return;
	}
	try {
		if (!tutorialsOf.has(d.id)) tutorialsOf.set(d.id, await api.tutorials(d.id));
	} catch (_) {
		host.hidden = true;
		return;
	}
	// Another game may have been chosen while the list was on its way.
	if (document.getElementById("game_def_id").value !== d.id) return;
	clear(host).append(
		h("h3", { class: "setup-tutorials-title" }, "Learn to play"),
		tutorialList(tutorialsOf.get(d.id).family, tutorialsOf.get(d.id).tutorials || [], { nameField: document.getElementById("create_player_name"), compact: true })
	);
	host.hidden = false;
}

/** A family in the order its picker lists it: smallest board first, and among equals the base game, then by name. */
const sortedFamily = (list) => {
	const baseFirst = (d) => (isBaseGame(d) ? 0 : 1);
	return [...list].sort((a, b) => (a.board_width || 0) - (b.board_width || 0) || baseFirst(a) - baseFirst(b) || a.name.localeCompare(b.name));
};

let previewFor = "";
function renderSetupPreview(id, info) {
	const slot = document.getElementById("setup-preview");
	if (!slot || previewFor === id) return;
	previewFor = id;
	try {
		clear(slot);
		// A table of cards has no board to draw: it shows a game in progress.
		if (info.sample_view && info.sample_view.solitaire) {
			slot.append(solitairePreview(info, info.sample_view));
			return;
		}
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
	renderSetupOptions(info);
	renderClockField(info);
	renderRealTimeField(info);
	renderRatedField(info);
	renderCardSetField(info);
	applyStartMode();
}

let botList = [];
// The seat numbers of the game being set up.
let currentSeats = [];

// What was chosen for each seat, kept across redraws: choosing another layout
// of the game (or another game and back) rebuilds the control, and must not
// turn every bot back into the default kind.
const slotMemory = new Map();
// Whether an expansion option was ticked, by option id.
const flagMemory = new Map();
// The pickers of games that have one, by family: another layout of the game keeps
// what was chosen. picker is the one of the game being set up.
const pickers = new Map();
let picker = null;

/** The picker of a family. */
async function buildPicker(family, info) {
	const [catalog, me] = await Promise.all([api.setupCatalog(info.id), whoAmI().catch(() => null)]);
	return createCwSetup({
		family,
		catalog,
		layouts: sortedFamily(defs.filter((d) => (d.family || d.id) === family)),
		loadInfo: gameInfo,
		onLayout: (id) => {
			if (document.getElementById("game_def_id").value !== id) chooseGame(id);
		},
		onChange: paintSeatCults,
		account: !!me && me.kind === "account",
		accounts: !!me && !!me.accounts,
	});
}

/**
 * Makes the picker of a game whose setup has one ready; the seats are drawn after
 * it. The page may ask twice while the first answer is on its way, so what is kept
 * is the promise: a second picker would replace the first, and what was chosen in it.
 */
async function preparePicker(info) {
	if (!info.setup_catalog) {
		picker = null;
		return;
	}
	const family = info.family || info.id;
	if (!pickers.has(family)) pickers.set(family, buildPicker(family, info));
	try {
		picker = await pickers.get(family);
	} catch (_) {
		// The check boxes are still there to fall back on; the next look tries again.
		pickers.delete(family);
		picker = null;
	}
}

/** Puts each seat's faction on its row. */
function paintSeatCults() {
	if (!picker) return;
	const cults = picker.factions();
	for (const cell of document.querySelectorAll("#slots-field .slot-cult")) {
		const id = cults[Number(cell.dataset.cultSeat) - 1];
		clear(cell);
		cell.dataset.value = id || "";
		if (id) cell.append(picker.glyph(id, 22), h("strong", {}, picker.factionName(id)));
		else cell.append(h("span", { class: "muted" }, "Choose a faction"));
	}
	syncPoolLobby();
}

/** More factions than seats are chosen from in the lobby, so a lobby there must be. */
function syncPoolLobby() {
	const box = document.getElementById("lobby");
	if (!box) return;
	const pool = !!picker && picker.hasPool();
	if (pool && !box.disabled) box.dataset.before = box.checked ? "1" : "0";
	if (!pool && box.disabled) box.checked = box.dataset.before === "1";
	box.disabled = pool;
	if (pool) box.checked = true;
}

/**
 * The game's `bool` setup options (an expansion, the pie rule) as check boxes.
 * What was ticked is kept per family of games while the page lives — another
 * layout of the same game keeps the choice, another game does not, as the
 * default is the game's own: the pie rule starts on in Hex and off in Yavalath.
 */
function boolOptions(info) {
	return (info.options || [])
		.filter((o) => o.kind === "bool")
		.map((opt) => {
			const key = `${info.family || info.id}:${opt.id}`;
			const box = h("input", { type: "checkbox", id: `option-${opt.id}`, "data-option": opt.id, "data-default": opt.default ? "1" : "0" });
			box.checked = flagMemory.has(key) ? flagMemory.get(key) : !!opt.default;
			box.addEventListener("change", () => flagMemory.set(key, box.checked));
			return h(
				"div",
				{ class: "option-flag" },
				h("label", { class: "check", title: opt.text || null }, box, h("span", {}, `${opt.label}`)),
				opt.text ? h("span", { class: "small muted" }, opt.text) : null
			);
		});
}

// The minutes a clock suggests for a game whose definition names none.
const CLOCK_MINUTES_DEFAULT = 10;
// Each time a clock can be set to: the choice of kind, and the minutes it may hold.
const CLOCK_TIMES = [
	["clock_seconds", "clock_minutes"],
	["clock_seconds_1", "clock_minutes_1"],
	["clock_seconds_2", "clock_minutes_2"],
];

/** Whole minutes from 1 to 60, or fallback for a box left empty. */
function clockMinutes(value, fallback) {
	const n = Math.round(Number(value));
	if (value === "" || !Number.isFinite(n)) return fallback;
	return Math.min(60, Math.max(1, n));
}

/** The seconds one of CLOCK_TIMES is set to (0: no clock). */
function clockTimeSeconds([choice, minutes]) {
	const value = document.getElementById(choice).value;
	if (value !== "minutes") return Number(value);
	const box = document.getElementById(minutes);
	return clockMinutes(box.value, Number(box.dataset.suggested) || CLOCK_MINUTES_DEFAULT) * 60;
}

/**
 * The game clock (a chess clock) is offered to a game of two seats or more, with
 * different times for a game of two (a game that seats a range is a layout of its
 * own for each number). A clock and a time per move are the same question asked
 * twice, so choosing one clears the other.
 */
function renderClockField(info) {
	const field = document.getElementById("clock-field");
	if (!field) return;
	const timed = info.seats.length >= 2;
	const two = info.seats.length === 2;
	field.hidden = !timed;
	field.dataset.seats = String(info.seats.length);
	// The game's own time, unless the player has typed one of theirs.
	const suggested = String(info.default_clock_minutes || CLOCK_MINUTES_DEFAULT);
	for (const [, id] of CLOCK_TIMES) {
		const box = document.getElementById(id);
		if (!box) continue;
		box.dataset.suggested = suggested;
		if (box.dataset.touched !== "1") box.value = suggested;
	}
	const clock = document.getElementById("clock_seconds");
	const perMove = document.getElementById("move_time_seconds");
	const hc = document.getElementById("clock-handicap");
	if (hc) {
		hc.parentElement.hidden = !two;
		if (!two && hc.checked) {
			hc.checked = false;
			hc.dispatchEvent(new Event("change", { bubbles: true }));
		}
	}
	if (!timed) {
		if (clock) clock.value = "0";
		const row = document.getElementById("clock-handicap-row");
		if (row) row.hidden = true;
		const inc = document.getElementById("clock_increment");
		if (inc) inc.value = "0";
		return;
	}
	if (field.dataset.wired === "1") return;
	field.dataset.wired = "1";
	const handicap = document.getElementById("clock-handicap");
	const row = document.getElementById("clock-handicap-row");
	const first = document.getElementById("clock_seconds_1");
	const second = document.getElementById("clock_seconds_2");
	const sync = () => {
		row.hidden = !handicap.checked;
		clock.parentElement.hidden = handicap.checked;
		for (const [choice, minutes] of CLOCK_TIMES) {
			document.getElementById(minutes).hidden = document.getElementById(choice).value !== "minutes";
		}
		if (perMove && clockChoice().length) perMove.value = "0";
		// An increment is part of a clock: without one there is nothing to add to.
		const timed = clockChoice().length > 0;
		document.getElementById("clock-increment-row").hidden = !timed;
		if (!timed) document.getElementById("clock_increment").value = "0";
	};
	handicap.addEventListener("change", () => {
		if (handicap.checked && clock.value !== "0") {
			// Start the two sides at what both were set to.
			const both = document.getElementById("clock_minutes");
			for (const [sel, id] of [[first, "clock_minutes_1"], [second, "clock_minutes_2"]]) {
				sel.value = clock.value;
				const box = document.getElementById(id);
				box.value = both.value;
				box.dataset.touched = both.dataset.touched || "";
			}
		}
		sync();
	});
	for (const el of [clock, first, second]) el.addEventListener("change", sync);
	for (const [, id] of CLOCK_TIMES) {
		const box = document.getElementById(id);
		box.addEventListener("input", () => {
			box.dataset.touched = "1";
		});
		box.addEventListener("change", () => {
			box.value = clockMinutes(box.value, Number(box.dataset.suggested));
			sync();
		});
	}
	if (perMove) {
		perMove.addEventListener("change", () => {
			// A time per move takes the clock's place, as the clock takes its.
			if (Number(perMove.value) > 0) {
				clock.value = "0";
				handicap.checked = false;
				sync();
			}
		});
	}
	sync();
}

/**
 * "Cards" is offered by a game with a standard deck: a themed set for the whole
 * table, chosen here and nowhere else (ADR 0085). The picture shows the set, with
 * the suits this game does not reach dimmed.
 */
function renderCardSetField(info) {
	const field = document.getElementById("card-set-field");
	const select = document.getElementById("card-set");
	const preview = document.getElementById("card-set-preview");
	if (!field || !select || !preview) return;
	field.hidden = !(info.cards && info.cards.standard);
	if (select.options.length === 1) {
		for (const set of CARD_SETS) select.append(h("option", { value: set.id }, set.label));
	}
	const suits = info.cards ? (info.cards.suit_order || []).length : 0;
	const show = () => {
		const set = cardSetById(select.value);
		preview.replaceChildren(...(set ? [setPreview(set, suits)] : []));
		useCardSet(select.value);
	};
	select.onchange = () => {
		show();
		// A game in progress in the preview wears the set too.
		if (info.sample_view) {
			previewFor = "";
			renderSetupPreview(info.id, info);
		}
	};
	show();
}

/**
 * "Rated game" is offered to a signed-in player at a table of two. The server
 * says what a rated game takes; the page only saves asking for one it would
 * refuse: choosing it means a friend for an opponent and one clock for both, and
 * giving up either of those gives up the choice.
 */
function renderRatedField(info) {
	const field = document.getElementById("rated-field");
	const rated = document.getElementById("rated");
	if (!field || !rated) return;
	const clock = document.getElementById("clock_seconds");
	const handicap = document.getElementById("clock-handicap");
	const perMove = document.getElementById("move_time_seconds");
	const sync = () => {
		const on = rated.checked && !field.hidden;
		for (const radio of document.querySelectorAll('#opponent-choices input[name="opponent"]')) {
			radio.disabled = on && radio.value !== "";
			if (on && radio.value === "") radio.checked = true;
		}
		if (!on) return;
		if (handicap.checked) handicap.click();
		if (perMove) perMove.value = "0";
		// A sensible clock rather than a refusal.
		if (!clockChoice().length) {
			clock.value = "minutes";
			clock.dispatchEvent(new Event("change", { bubbles: true }));
		}
	};
	const giveUp = () => {
		if (rated.checked && (handicap.checked || !clockChoice().length || Number(perMove.value) > 0)) {
			rated.checked = false;
			sync();
		}
	};
	field.hidden = true;
	whoAmI().then((me) => {
		field.hidden = info.seats.length !== 2 || me.kind !== "account";
		if (field.hidden) rated.checked = false;
		sync();
	});
	if (field.dataset.wired === "1") return;
	field.dataset.wired = "1";
	rated.addEventListener("change", sync);
	for (const el of [clock, handicap, perMove]) el.addEventListener("change", giveUp);
}

/** Real time needs someone else to play with, and its warnings are for a game with no timer. */
function renderRealTimeField(info) {
	const field = document.getElementById("real-time-field");
	const box = document.getElementById("real-time");
	if (!field || !box) return;
	field.hidden = info.seats.length < 2;
	if (field.hidden) box.checked = false;
	const sync = () => {
		document.getElementById("real-time-warnings").hidden = !box.checked || !realTimeWarns();
	};
	if (!field.dataset.wired) {
		field.dataset.wired = "1";
		for (const id of ["real-time", "clock_seconds", "clock_seconds_1", "clock_seconds_2", "clock-handicap", "move_time_seconds"]) {
			const el = document.getElementById(id);
			if (el) el.addEventListener("change", sync);
		}
	}
	sync();
}

/** Whether the game being set up has no timer, so that the warnings apply. */
function realTimeWarns() {
	return clockChoice().length === 0 && !(Number(document.getElementById("move_time_seconds").value) > 0);
}

/** What the setup asks for in real time, or null for a game that is not. */
function realTimeChoice() {
	const box = document.getElementById("real-time");
	if (!box || !box.checked || document.getElementById("real-time-field").hidden) return null;
	if (!realTimeWarns()) return {};
	const minutes = (id, fallback) => {
		const n = Math.round(Number(document.getElementById(id).value));
		return (n >= 1 && n <= 60 ? n : fallback) * 60;
	};
	return { first_warning_seconds: minutes("real-time-first", 2), second_warning_seconds: minutes("real-time-second", 5) };
}

/**
 * The clock chosen in the setup: the seconds for each seat in seat order, or
 * nothing for a game played without one.
 */
function clockChoice() {
	const field = document.getElementById("clock-field");
	if (!field || field.hidden) return [];
	const handicap = document.getElementById("clock-handicap");
	if (handicap && handicap.checked) {
		return [clockTimeSeconds(CLOCK_TIMES[1]), clockTimeSeconds(CLOCK_TIMES[2])];
	}
	const both = clockTimeSeconds(CLOCK_TIMES[0]);
	return both > 0 ? Array(Number(field.dataset.seats) || 2).fill(both) : [];
}

/**
 * The setup options of a game whose seats are not arranged one by one (a game
 * of two): the check boxes stand in a field of their own.
 */
function renderSetupOptions(info) {
	const field = document.getElementById("setup-options");
	if (!field) return;
	for (const old of [...field.children].slice(1)) old.remove();
	const boxes = info.seats.length > 2 ? [] : boolOptions(info);
	for (const node of boxes) field.append(node);
	field.hidden = boxes.length === 0;
}

// renderSlots lets a table of three or more seats be arranged seat by seat:
// who you are, which seats stay open for friends and which get a bot.
function renderSlots(info) {
	const old = document.getElementById("slots-field");
	if (old) old.remove();
	const opp = document.getElementById("opponent-choices");
	const oppField = opp ? opp.closest(".field") : null;
	const seatField = document.getElementById("seat-choices")?.closest(".field");
	const many = info.seats.length > 2;
	syncPoolLobby();
	if (!many) {
		const box = document.querySelector(".setup-layout");
		if (box) box.classList.remove("with-picker");
		const variants = document.getElementById("setup-variants");
		if (variants) variants.hidden = false;
	}
	// A game for one player has no opponent to choose and only the one seat.
	const alone = info.seats.length === 1;
	if (oppField) oppField.hidden = many || alone;
	if (seatField) seatField.hidden = many || alone;
	if (!many || !oppField) return;
	const field = h("div", { class: "field", id: "slots-field" });
	const layoutBox = document.querySelector(".setup-layout");
	if (layoutBox) layoutBox.classList.toggle("with-picker", !!picker);
	const variants = document.getElementById("setup-variants");
	if (variants) variants.hidden = !!picker;
	if (picker) field.append(picker.el);
	field.append(h("label", {}, "Seats"));
	const rows = h("div", { class: "stack" });
	const remember = () => {
		for (const sel of rows.querySelectorAll(".slot-select")) slotMemory.set(Number(sel.dataset.seat), sel.value);
	};
	const fallback = botList.some((b) => b.id === "tactical") ? "bot:tactical" : "open";
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
		if (picker) {
			who = h("span", { class: "slot-cult", "data-cult-seat": seat.seat });
			swatch = null;
		}
		rows.append(h("div", { class: "slot-row" + (picker ? " with-faction" : "") }, swatch, who, select));
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
	// Expansions: a check box each, where no picker offers them.
	if (!picker) for (const node of boolOptions(info)) field.append(node);
	oppField.before(field);
	if (picker) {
		picker.setLayout(info);
		paintSeatCults();
	}
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
	for (const box of document.querySelectorAll("#create-game-form input[data-option]")) {
		if (box.checked !== (box.dataset.default === "1")) options[box.dataset.option] = box.checked;
	}
	if (Object.keys(options).length) out.options = options;
	if (picker) out.options = picker.options();
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
	if (lo === 1 && hi === 1) return "1 player";
	return lo === hi ? `${lo} players` : `${lo}\u2013${hi} players`;
}

/** The line under a game's name: the Prototype mark, or the Beta mark while its version is below 1.0, and the players it takes. */
function cardMeta(list, lead) {
	const out = [];
	if (lead.prototype) {
		out.push(h("span", { class: "badge prototype", title: `Rules version ${lead.version}: the rules are still changing` }, "Prototype"));
	} else if (isBeta(lead.version)) {
		out.push(h("span", { class: "badge beta", title: `Version ${lead.version || "0.0.0"}: still being tested` }, "Beta"));
	}
	const players = playerRange(list);
	if (players) out.push(h("span", { class: "game-card-players muted" }, players));
	if (list.some((d) => d.tutorials)) out.push(h("span", { class: "badge tutorial", title: "This game has a tutorial that teaches it by playing" }, "Tutorial"));
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
			h("div", { class: "game-tip-version muted small" }, lead.prototype ? `Prototype, rules version ${lead.version}` : `Version ${lead.version || "0.0.0"}${isBeta(lead.version) ? " (beta)" : ""}`)
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
		const lead = leadOf(list);
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
		if (picker && !picker.complete()) {
			showError(err, "Choose a faction for every seat.");
			document.getElementById("cw-seat-notice").scrollIntoView({ block: "nearest" });
			return;
		}
		if (document.getElementById("slots-field")) {
			const slots = slotChoice();
			if (slots.seat && slots.seat !== 1) body.seat = slots.seat;
			if (slots.also.length) body.also_seats = slots.also;
			if (Object.keys(slots.seatBots).length) body.seat_bots = slots.seatBots;
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
		// The game's own options (the pie rule) are chosen whether or not the
		// seats are arranged one by one.
		const chosen = slotChoice();
		if (chosen.options) body.options = chosen.options;
		const clock = clockChoice();
		if (clock.length) {
			body.clock_seconds = clock;
			const inc = Number(document.getElementById("clock_increment").value);
			if (inc > 0) body.clock_increment_seconds = inc;
		}
		const limit = Number(document.getElementById("move_time_seconds").value);
		if (limit > 0 && !clock.length) body.move_time_seconds = limit;
		const realTime = realTimeChoice();
		if (realTime) body.real_time = realTime;
		const lobbyBox = document.getElementById("lobby");
		if (lobbyBox && lobbyBox.checked) body.lobby = true;
		const ratedBox = document.getElementById("rated");
		if (ratedBox && ratedBox.checked && !document.getElementById("rated-field").hidden) body.rated = true;
		const cardSet = document.getElementById("card-set");
		if (cardSet && cardSet.value && !document.getElementById("card-set-field").hidden) body.card_set = cardSet.value;
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
	if (rulesButton) {
		// The rule book stands for the word (ui-icons.js).
		rulesButton.replaceChildren(rulesIcon());
		rulesButton.classList.add("icon-only");
		rulesButton.setAttribute("aria-label", "Rules");
		rulesButton.title = "Rules";
		rulesButton.addEventListener("click", showRules);
	}
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

const TABS = ["games", "puzzles", "analysis", "ratings", "tournaments", "chat"];
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
	// The Analysis tab is built the first time it is asked for: it reads the
	// batches and the bots' guide, which the other tabs do not need.
	if (name === "analysis") mountAnalysis(document.getElementById("panel-analysis"));
	if (name === "ratings") mountRatings(document.getElementById("panel-ratings"));
	if (name === "tournaments") mountTournaments(document.getElementById("panel-tournaments"));
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
