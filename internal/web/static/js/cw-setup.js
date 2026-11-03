// The setup of a table of Cthulhu Wars: tabs of tiles (maps, factions, neutral
// units, Independents, the rest), a card of what is chosen so far, ready-made and
// saved setups, and a shuffle. What is chosen reaches the server as the ordinary
// options of a create call, so this file knows nothing of the rules.

import { h, clear } from "./util.js";
import { api } from "./api.js";
import { pieceIcon } from "./render.js";
import { statIcon } from "./icons.js";
import { hoverCard, hideHoverCard } from "./tooltip.js";
import { neutralTip } from "./cw-tips.js";

const MAP_TAB = { id: "maps", label: "Maps" };

/** One fewer Independent than players is the rule book's advice for a first game. */
const recommended = (seats) => Math.max(0, seats - 1);

function shuffled(list) {
	const out = [...list];
	for (let i = out.length - 1; i > 0; i--) {
		const j = Math.floor(Math.random() * (i + 1));
		[out[i], out[j]] = [out[j], out[i]];
	}
	return out;
}

/** "Cthulhu Wars (4 players, 5-3 map)" -> "4 players, 5-3 map". */
function mapName(def) {
	const m = /\(([^)]*)\)\s*$/.exec(def.name);
	return m ? m[1] : def.name;
}

/**
 * family: the id of the family of games; catalog: GET /game-defs/{id}/setup-catalog;
 * layouts: the games of the family; loadInfo(id): the full description of one, fetched only to apply a setup;
 * onLayout(id): make it the game being set up; onChange(): the factions changed;
 * account: whether the visitor may save setups; accounts: whether this server has any.
 */
export function createCwSetup({ family, catalog, layouts, loadInfo, onLayout, onChange, account, accounts }) {
	const items = new Map();
	for (const tab of catalog.tabs) for (const item of tab.items) items.set(item.id, item);
	const maps = layouts
		.map((def) => ({ kind: "map", id: def.id, name: mapName(def), seats: def.seats_required }))
		.sort((a, b) => a.seats - b.seats || a.name.localeCompare(b.name));
	const mapById = new Map(maps.map((m) => [m.id, m]));
	const swapOpt = catalog.swaps || null;

	let info = null;
	let seats = []; // the faction of each seat, null for one still to choose
	const chosen = new Set(); // ids of the other items
	const swaps = new Map(); // cult -> neutral spellbook replacing its timer book
	const dealt = new Set(); // what the last shuffle added, which the next one replaces
	let currentTab = MAP_TAB.id;
	let profiles = [];
	let message = "";

	const pieceShape = (id) => {
		const def = info && info.pieces.find((p) => p.type === id);
		return def ? def.shape : id;
	};

	// ---- what the tiles look like ----------------------------------------------------

	function glyph(key, px) {
		return h("img", { class: "cw-glyph", src: `/api/game-defs/${encodeURIComponent(family)}/assets/guide/glyphs/${key}.svg`, width: px, height: px, alt: "", loading: "lazy" });
	}

	/** A map as the server draws it: a small file, where the full description of a layout is a large one. */
	function mapPicture(id, cls) {
		return h("img", { class: cls, src: `/api/game-defs/${encodeURIComponent(id)}/assets/board.svg`, alt: "", loading: "lazy" });
	}

	function icon(item, px = 52) {
		switch (item.kind) {
			case "map":
				return mapPicture(item.id, "cw-map-thumb");
			case "faction":
				return glyph(item.id, px);
			case "book":
				return statIcon("books", item.name, px - 6);
			case "rule":
				return pieceIcon(pieceShape(item.shape), "black", px);
			default:
				return pieceIcon(pieceShape(item.id), "black", px);
		}
	}

	function factionTip(item) {
		const box = h(
			"div",
			{ class: "unit-tip cw-tip", "data-item": item.id },
			h("div", { class: "unit-tip-head" }, glyph(item.id, 34), h("div", { class: "unit-tip-title" }, h("strong", {}, item.name), h("span", { class: "muted small" }, "Faction")))
		);
		if (item.text) box.append(h("p", { class: "unit-text small" }, item.text.replace(/^Expansion: /, "")));
		if (item.ability) box.append(h("p", { class: "unit-enter small" }, h("strong", {}, "Ability: "), item.ability));
		const rows = h("ul", { class: "cw-tip-units small" });
		for (const u of item.units || []) {
			rows.append(h("li", {}, pieceIcon(pieceShape(u.id), item.color, 22), h("span", {}, u.count > 1 ? `${u.count} ${u.name}` : u.name), h("span", { class: "muted" }, `${u.cost} Power`)));
		}
		box.append(rows);
		if ((item.books || []).length) box.append(h("p", { class: "unit-enter small" }, h("strong", {}, "Spellbooks: "), item.books.join(", ")));
		return box;
	}

	function mapTip(item) {
		return h("div", { class: "unit-tip cw-tip cw-map-tip", "data-item": item.id }, h("strong", {}, item.name), mapPicture(item.id, "cw-map-large"));
	}

	function tip(item) {
		switch (item.kind) {
			case "map":
				return mapTip(item);
			case "faction":
				return factionTip(item);
			case "rule":
				return h(
					"div",
					{ class: "unit-tip cw-tip", "data-item": item.id },
					h("div", { class: "unit-tip-head" }, icon(item, 34), h("div", { class: "unit-tip-title" }, h("strong", {}, item.name), h("span", { class: "muted small" }, "A rule for the whole table"))),
					h("p", { class: "unit-text small" }, item.text)
				);
			default:
				return neutralTip(item, info ? info.pieces : []);
		}
	}

	function tile(item, onClick, extra = {}) {
		const btn = h(
			"button",
			{ type: "button", class: "cw-tile", "data-item": item.id, "data-kind": item.kind, ...extra },
			h("span", { class: "cw-tile-icon" }, icon(item)),
			h("span", { class: "cw-tile-name" }, item.name)
		);
		btn.addEventListener("click", onClick);
		hoverCard(btn, () => tip(item), { keepOnClick: true });
		return btn;
	}

	// ---- choosing --------------------------------------------------------------------

	const isChosen = (item) => (item.kind === "map" ? info && info.id === item.id : item.kind === "faction" ? seats.includes(item.id) : chosen.has(item.id));

	function setFaction(id, on) {
		if (on) {
			const open = seats.indexOf(null);
			if (open < 0) {
				say(`All ${seats.length} seats have a faction: remove one first.`);
				return;
			}
			seats[open] = id;
		} else {
			seats[seats.indexOf(id)] = null;
		}
		say("");
		changed(true);
	}

	function toggle(item) {
		if (item.kind === "map") {
			if (info && info.id !== item.id) {
				profileSelect.value = "";
				deleteBtn.hidden = true;
				onLayout(item.id);
			}
			return;
		}
		if (item.kind === "faction") {
			setFaction(item.id, !seats.includes(item.id));
			return;
		}
		if (chosen.has(item.id)) chosen.delete(item.id);
		else chosen.add(item.id);
		dealt.delete(item.id);
		say("");
		changed();
	}

	/** The seats of a layout of n, keeping what is chosen; new seats get the layout's own cults, then any. */
	function fitSeats(layout) {
		const own = layout.seats.map((seat) => {
			const f = catalog.tabs[0].items.find((x) => x.name === seat.label);
			return f ? f.id : null;
		});
		const n = own.length;
		if (seats.length === n) return;
		const kept = seats.filter(Boolean).slice(0, n);
		const spare = [...own, ...catalog.tabs[0].items.map((x) => x.id)].filter((id, i, all) => id && all.indexOf(id) === i);
		for (const id of spare) {
			if (kept.length === n) break;
			if (!kept.includes(id)) kept.push(id);
		}
		seats = kept;
	}

	function layoutFactions(layout) {
		return layout.seats.map((seat) => {
			const f = catalog.tabs[0].items.find((x) => x.name === seat.label);
			return f ? f.id : "";
		});
	}

	/** The options of the create call, null for the layout as it comes. */
	function options({ always = false } = {}) {
		const out = {};
		for (const id of chosen) {
			const item = items.get(id);
			if (!item) continue;
			if (item.value) (out[item.option] = out[item.option] || []).push(item.value);
			else out[item.option] = true;
		}
		const replaced = {};
		for (const [cult, book] of swaps) if (seats.includes(cult)) replaced[cult] = book;
		if (Object.keys(replaced).length) out[swapOpt.id] = replaced;
		const asItComes = info && seats.join(",") === layoutFactions(info).join(",");
		if (!always && asItComes && !Object.keys(out).length) return null;
		out.factions = [...seats];
		return out;
	}

	function apply(setup, layout) {
		const o = setup.options || {};
		info = layout;
		seats = Array.isArray(o.factions) && o.factions.length === layout.seats.length ? [...o.factions] : layoutFactions(layout);
		chosen.clear();
		dealt.clear();
		swaps.clear();
		for (const item of items.values()) {
			if (item.kind === "faction") continue;
			const v = o[item.option];
			if (item.value ? Array.isArray(v) && v.includes(item.value) : v === true) chosen.add(item.id);
		}
		for (const [cult, book] of Object.entries((swapOpt && o[swapOpt.id]) || {})) swaps.set(cult, book);
		say("");
		onLayout(layout.id);
		paintPanel();
		changed(true, true);
	}

	function shuffle() {
		for (const id of dealt) chosen.delete(id);
		dealt.clear();
		const dealtFactions = shuffled(catalog.tabs[0].items.filter((f) => !seats.includes(f.id)));
		for (let i = 0; i < seats.length; i++) if (seats[i] === null) seats[i] = dealtFactions.pop().id;
		const room = recommended(seats.length) - budgetCount();
		if (room <= 0) {
			changed(true);
			say(`You already have the recommended ${recommended(seats.length)}: remove one to shuffle.`);
			return;
		}
		const pool = shuffled([...items.values()].filter((x) => x.budget && !chosen.has(x.id)));
		for (const item of pool.slice(0, room)) {
			chosen.add(item.id);
			dealt.add(item.id);
		}
		say("");
		changed(true);
	}

	const budgetCount = () => [...chosen].filter((id) => items.get(id) && items.get(id).budget).length;

	// ---- the page --------------------------------------------------------------------

	const tabsRow = h("div", { class: "cw-tabs", role: "tablist", "aria-label": "What to put in the game" });
	const panel = h("div", { class: "cw-panel", id: "cw-panel", role: "tabpanel" });
	const selection = h("div", { class: "cw-chosen", id: "cw-chosen" });
	const notice = h("p", { class: "cw-notice small", id: "cw-notice", role: "status", hidden: true });
	const seatNotice = h("p", { class: "cw-notice small", id: "cw-seat-notice", role: "status", hidden: true });
	const profileSelect = h("select", { id: "cw-profile", "aria-label": "Start from a setup" });
	const deleteBtn = h("button", { type: "button", id: "cw-profile-delete", hidden: true }, "Delete");
	const saveBtn = h("button", { type: "button", id: "cw-profile-save", hidden: !account }, "Save…");
	const nameInput = h("input", { type: "text", id: "cw-profile-name", maxlength: 40, placeholder: "Name of this setup", "aria-label": "Name of this setup" });
	const shareBox = h("input", { type: "checkbox", id: "cw-profile-share" });
	const profileMsg = h("span", { class: "small muted", id: "cw-profile-msg", role: "status" });
	const saveForm = h(
		"div",
		{ class: "cw-save", id: "cw-save", hidden: true },
		nameInput,
		h("label", { class: "check small", title: "Everyone on this server, guests too, can start from a shared setup" }, shareBox, h("span", {}, "Share with everyone")),
		h("button", { type: "button", class: "primary", id: "cw-profile-confirm" }, "Save"),
		h("button", { type: "button", id: "cw-profile-cancel" }, "Cancel")
	);

	function say(text) {
		message = text;
		paintNotices();
	}

	function paintNotices() {
		const n = seats.length;
		const count = budgetCount();
		const rec = recommended(n);
		notice.hidden = count <= rec;
		notice.textContent =
			count > rec ? `${count} neutral units and Independents chosen: more than the ${rec} recommended for ${n} players.` : "";
		const open = seats.filter((x) => x === null).length;
		const text = message || (open ? `${open === 1 ? "A seat still needs" : `${open} seats still need`} a faction.` : "");
		seatNotice.hidden = !text;
		seatNotice.textContent = text;
	}

	// The tabs are made once and updated in place: the seats are redrawn when the
	// list of bots arrives, and a click must not land on a button just replaced.
	const allTabs = [MAP_TAB, ...catalog.tabs];
	const tabButtons = new Map();
	for (const tab of allTabs) {
		const count = h("span", { class: "cw-tab-count" });
		const btn = h("button", { type: "button", class: "cw-tab", role: "tab", "data-tab": tab.id }, tab.label, count);
		btn.addEventListener("click", () => showTab(tab.id));
		btn.addEventListener("keydown", (ev) => {
			if (ev.key !== "ArrowRight" && ev.key !== "ArrowLeft") return;
			ev.preventDefault();
			const i = allTabs.indexOf(tab);
			const next = allTabs[(i + (ev.key === "ArrowRight" ? 1 : allTabs.length - 1)) % allTabs.length];
			showTab(next.id);
			tabButtons.get(next.id).btn.focus();
		});
		tabButtons.set(tab.id, { btn, count, tab });
		tabsRow.append(btn);
	}

	function paintTabs() {
		for (const { btn, count, tab } of tabButtons.values()) {
			const on = tab.id === currentTab;
			btn.classList.toggle("on", on);
			btn.setAttribute("aria-selected", on ? "true" : "false");
			btn.tabIndex = on ? 0 : -1;
			let n = "";
			if (tab.id === "factions") n = `${seats.filter(Boolean).length}/${seats.length}`;
			else if (tab !== MAP_TAB) n = tab.items.filter((x) => chosen.has(x.id)).length || "";
			count.textContent = String(n);
		}
	}

	function grid(list, onClick) {
		const out = h("div", { class: "cw-grid" });
		for (const item of list) out.append(tile(item, () => onClick(item)));
		return out;
	}

	function swapBlock() {
		const block = h("div", { class: "book-swap", id: "book-swaps" }, h("p", { class: "cw-group small muted", title: swapOpt.text || null }, swapOpt.label));
		const taken = new Set([...swaps].filter(([cult]) => seats.includes(cult)).map(([, book]) => book));
		for (const slot of swapOpt.slots || []) {
			const select = h("select", { class: "swap-select", "data-swap": slot.id, "aria-label": `Spellbook to replace ${slot.label}` }, h("option", { value: "" }, `Keep ${slot.label}`));
			const mine = swaps.get(slot.id) || "";
			for (const c of swapOpt.choices || []) {
				select.append(h("option", { value: c.id, title: c.text || null, disabled: c.id !== mine && taken.has(c.id) }, `Replace with ${c.label}`));
			}
			select.value = mine;
			select.addEventListener("change", () => {
				if (select.value) swaps.set(slot.id, select.value);
				else swaps.delete(slot.id);
				changed(false, true);
				paintPanel();
			});
			block.append(h("label", { class: "swap-row", title: slot.text || null, hidden: !seats.includes(slot.id) }, h("span", { class: "small" }, `${slot.label}: `), select));
		}
		return block;
	}

	function paintPanel() {
		hideHoverCard();
		clear(panel);
		if (currentTab === MAP_TAB.id) {
			panel.append(grid(maps, toggle));
		} else {
			const tab = catalog.tabs.find((t) => t.id === currentTab);
			const groups = new Map();
			for (const item of tab.items) {
				const g = item.group || "";
				if (!groups.has(g)) groups.set(g, []);
				groups.get(g).push(item);
			}
			for (const [name, list] of groups) {
				if (name) panel.append(h("p", { class: "cw-group small muted" }, name));
				panel.append(grid(list, toggle));
			}
			if (currentTab === "other" && swapOpt) panel.append(swapBlock());
		}
		paintTiles();
	}

	function paintTiles() {
		const full = !seats.includes(null);
		for (const btn of panel.querySelectorAll(".cw-tile")) {
			const id = btn.dataset.item;
			const item = items.get(id) || mapById.get(id);
			const on = !!item && isChosen(item);
			btn.classList.toggle("on", on);
			btn.setAttribute("aria-pressed", on ? "true" : "false");
			const blocked = item && item.kind === "faction" && full && !on;
			btn.classList.toggle("blocked", !!blocked);
		}
	}

	function paintSelection() {
		hideHoverCard();
		clear(selection);
		const remove = (item) => () => {
			toggle(item);
			selection.focus({ preventScroll: true });
		};
		const add = (node) => selection.append(node);
		const map = info && mapById.get(info.id);
		if (map) add(tile(map, () => showTab(MAP_TAB.id), { title: "Choose another map in the Maps tab" }));
		for (const id of seats) if (id) add(tile(items.get(id), remove(items.get(id))));
		for (const item of items.values()) if (item.kind !== "faction" && chosen.has(item.id)) add(tile(item, remove(item)));
		for (const [cult, book] of swaps) {
			if (!seats.includes(cult) || !swapOpt) continue;
			const slot = (swapOpt.slots || []).find((x) => x.id === cult);
			const choice = (swapOpt.choices || []).find((x) => x.id === book);
			if (!slot || !choice) continue;
			const item = { kind: "book", id: `swap-${cult}`, name: `${choice.label} for ${slot.label}`, timing: "", text: choice.text, how: slot.text };
			add(
				tile(item, () => {
					swaps.delete(cult);
					changed(false, true);
					if (currentTab === "other") paintPanel();
				})
			);
		}
	}

	function changed(seatsToo = false, keepProfile = false) {
		if (!keepProfile) {
			profileSelect.value = "";
			deleteBtn.hidden = true;
		}
		paintTabs();
		paintTiles();
		paintSelection();
		paintNotices();
		if (seatsToo) {
			onChange();
			if (currentTab === "other") paintPanel();
		}
	}

	function showTab(id) {
		currentTab = id;
		paintTabs();
		paintPanel();
	}

	// ---- setups to start from --------------------------------------------------------

	function paintProfiles(selected = "") {
		clear(profileSelect);
		profileSelect.append(h("option", { value: "" }, "Start from a setup…"));
		const group = (label, list) => {
			if (!list.length) return;
			const g = h("optgroup", { label });
			for (const [value, text] of list) g.append(h("option", { value }, text));
			profileSelect.append(g);
		};
		group("Ready-made", (catalog.presets || []).map((p) => [`preset:${p.id}`, p.name]));
		group("Yours", profiles.filter((p) => p.mine).map((p) => [`saved:${p.id}`, p.shared ? `${p.name} (shared)` : p.name]));
		group("Shared", profiles.filter((p) => !p.mine).map((p) => [`saved:${p.id}`, `${p.name} (@${p.owner})`]));
		profileSelect.value = selected;
		showDelete();
	}

	function selectedSetup() {
		const [kind, id] = profileSelect.value.split(":");
		if (kind === "preset") return (catalog.presets || []).find((p) => p.id === id) || null;
		if (kind === "saved") return profiles.find((p) => String(p.id) === id) || null;
		return null;
	}

	function showDelete() {
		const setup = selectedSetup();
		deleteBtn.hidden = !(setup && setup.mine);
	}

	async function refreshProfiles(selected = "") {
		try {
			const res = await api.setupProfiles(family);
			profiles = res.profiles || [];
		} catch (_) {
			profiles = [];
		}
		paintProfiles(selected);
	}

	profileSelect.addEventListener("change", async () => {
		profileMsg.textContent = "";
		showDelete();
		const setup = selectedSetup();
		if (!setup) return;
		try {
			apply(setup, await loadInfo(setup.game_def_id));
		} catch (_) {
			profileMsg.textContent = "That setup's map is no longer in the game.";
		}
	});

	saveBtn.addEventListener("click", () => {
		const setup = selectedSetup();
		saveForm.hidden = !saveForm.hidden;
		if (saveForm.hidden) return;
		nameInput.value = setup && setup.mine ? setup.name : "";
		shareBox.checked = !!(setup && setup.mine && setup.shared);
		nameInput.focus();
	});

	async function saveProfile() {
		if (seats.includes(null)) {
			profileMsg.textContent = "Give every seat a faction first.";
			return;
		}
		try {
			const saved = await api.saveSetupProfile({ name: nameInput.value, game_def_id: info.id, options: options({ always: true }), shared: shareBox.checked });
			saveForm.hidden = true;
			await refreshProfiles(`saved:${saved.id}`);
			profileMsg.textContent = `Saved as ${saved.name}.`;
		} catch (e) {
			profileMsg.textContent = e.message;
		}
	}

	saveForm.querySelector("#cw-profile-confirm").addEventListener("click", saveProfile);
	saveForm.querySelector("#cw-profile-cancel").addEventListener("click", () => (saveForm.hidden = true));
	// The name box is inside the form that creates the game: Enter saves the setup.
	nameInput.addEventListener("keydown", (ev) => {
		if (ev.key !== "Enter") return;
		ev.preventDefault();
		saveProfile();
	});

	deleteBtn.addEventListener("click", async () => {
		const setup = selectedSetup();
		if (!setup || !setup.mine) return;
		if (!window.confirm(`Delete the setup "${setup.name}"?`)) return;
		try {
			await api.deleteSetupProfile(setup.id);
			await refreshProfiles();
			profileMsg.textContent = `Deleted ${setup.name}.`;
		} catch (e) {
			profileMsg.textContent = e.message;
		}
	});

	const shuffleBtn = h("button", { type: "button", id: "cw-shuffle", title: "Fill open seats and add random neutral units and Independents, up to the recommended number. Again for another deal." }, "Shuffle");
	shuffleBtn.addEventListener("click", shuffle);

	const el = h(
		"div",
		{ class: "cw-setup", id: "cw-setup" },
		h(
			"div",
			{ class: "cw-toolbar" },
			profileSelect,
			deleteBtn,
			saveBtn,
			shuffleBtn,
			!account && accounts ? h("span", { class: "small muted", id: "cw-profile-guest" }, "Sign in to save your own setups.") : null,
			profileMsg
		),
		saveForm,
		h(
			"div",
			{ class: "cw-cards" },
			h("div", { class: "cw-card cw-picks", id: "cw-picks" }, tabsRow, panel),
			h("div", { class: "cw-card cw-current", id: "cw-current" }, h("h3", {}, "Current selections"), notice, seatNotice, selection)
		)
	);
	selection.tabIndex = -1;
	paintProfiles();
	refreshProfiles();

	return {
		el,
		/** The game being set up changed: its seats follow. */
		setLayout(layout) {
			const same = info && info.id === layout.id && seats.length === layout.seats.length;
			info = layout;
			fitSeats(layout);
			// The same table drawn again (the list of bots arrived; a setup was just applied).
			if (same) return;
			showTab(currentTab);
			changed(true, true);
		},
		factions: () => [...seats],
		factionName: (id) => (items.get(id) ? items.get(id).name : ""),
		glyph,
		complete: () => !seats.includes(null),
		options: () => options(),
	};
}
