// The rules window: a floating dialog with one tab per page of the game's
// guide, rich text (headings, tables, callouts, inline images and piece
// icons), a table of contents for long pages and a search across all of
// them. Games without their own guide get one generated from their
// definition by the server, so every game gets the same window.

import { h, clear, store } from "./util.js";
import { api } from "./api.js";
import { renderMarkdown } from "./markdown.js";
import { pieceIcon, cssColor, legendMarker } from "./render.js";

const KEY = "bingle.rules.";

const escapeMd = (t) => String(t).replace(/([\\`*_{}[\]<>|~])/g, "\\$1");

/** The client-side page: how to play with the mouse and keyboard. */
function controlsPage(info) {
	const parts = [
		"# Controls",
		"",
		"## Making a move",
		"",
		"- Click a highlighted square, token or button to build your move one choice at a time; [[Esc]] starts it over and [[Backspace]] steps back.",
		"- Moves that involve dice or secrets are sent straight away; others show a preview to confirm (see the *Confirm* setting).",
		"- Hover or focus a square to read what is on it.",
		"",
		"## Keyboard",
		"",
		"| Key | Does |",
		"|---|---|",
		"| [[Tab]] | enters the board once |",
		"| [[←]] [[→]] [[↑]] [[↓]] | move between squares |",
		"| [[Enter]] / [[Space]] | select the focused square |",
		"| [[Enter]] elsewhere | submit a previewed move |",
		"| [[Esc]] | reset the move |",
		"| [[Backspace]] | step back inside a move |",
		"| [[←]] [[→]] off the board | walk through the game's history |",
		"| [[Home]] / [[End]] | jump to the start / the live position |",
		"",
	];
	if ((info.notation_help || []).length) {
		parts.push("## Move notation", "", "Moves have a plain-text notation used by the move list, exports, the API and bots.", "");
		for (const r of info.notation_help) parts.push("- " + escapeMd(r));
		parts.push("");
	}
	return parts.join("\n");
}

/**
 * The key of the game's legend: what the marks, tints and tokens on the board
 * mean. Empty for games with nothing to explain.
 */
function keyPage(info) {
	const d = info.display;
	if (!d || info.engine !== "ruleset") return "";
	const parts = [];
	if (info.board && info.board.style === "areas") {
		const tint = (k, fallback) => ((d.tints || {})[k] || {}).color || fallback;
		const first = (info.seats || [])[0];
		const color = first ? first.color : "green";
		parts.push(
			"# Key",
			"",
			"What the marks on the map mean.",
			"",
			"| Mark | Meaning |",
			"|---|---|",
			`| ![Ocean](marker:swatch:${tint("ocean", "#b9d3e6")}) | **Ocean** — areas of water. Only some units may enter them. |`,
			`| ![Land](marker:swatch:${tint("land", "#e5dcc0")}) | **Land**. |`,
			`| ![Desecrated](marker:swatch:${tint("desecrated", "#d8c76a")}) | **Desecrated area** (Yellow Sign): marked with ✠. |`,
			`| ![Gate](marker:gate:${color}) | **A Gate.** The ring is the colour of the cult that controls it; the figure drawn on it is the unit that occupies it — an Acolyte, a High Priest or a Dark Young. To give a Gate up, move that unit (also to the area it stands in). |`,
			"| ![Abandoned Gate](marker:gate:abandoned) | **An abandoned Gate:** a dashed grey ring. It gives 1 Power to every player in Gather Power and scores nothing; a Cultist that arrives takes it. |",
			"| ![Glyph](marker:glyph:chevron) | **A glyph** (chevron, dragon or thorns) after an area's name: what Yellow Sign must Desecrate for its requirements. |",
			"| ![Great Cthulhu glyph](marker:sigil:gc) | **A faction glyph** (here Great Cthulhu's), left of an area's name: the area where that cult begins. Every cult at the table has its own; they are shown with each faction in the Factions page. |",
			"| ![Windwalker glyph](marker:sigil:ww) | **The Windwalker's glyph** marks the two polar areas where it may begin, and where Rhan-Tegoth and Ithaqua awaken. |",
			"| ![Sleeper glyph](marker:sigil:sl) | **The Sleeper's glyph:** where it begins. |",
			"| ![Ice Age](marker:iceage) | **The Ice Age token** (Windwalker): enemy Actions that end here cost 1 Power more. |",
			"",
			"A red outline round an area is the battle being fought. In the game options the area names, glyphs and sigils (the faction glyphs) can be switched off."
		);
		return parts.join("\n");
	}
	parts.push("# Key", "", "What the colours and marks of the board mean.", "", "| Mark | Meaning |", "|---|---|");
	const row = (def) => {
		const label = `${def.glyph ? def.glyph + " " : ""}${def.label}`;
		parts.push(`| ![${escapeMd(def.label)}](marker:swatch:${def.color}) | **${escapeMd(label)}**${def.text ? " — " + escapeMd(def.text) : ""} |`);
	};
	for (const [k, def] of Object.entries(d.tints || {})) {
		if (!k.startsWith("zone") && def.color) row(def);
	}
	for (const [k, def] of Object.entries(d.badges || {})) {
		if (k !== "active" && k !== "target" && def.color) row(def);
	}
	return parts.length > 8 ? parts.join("\n") : "";
}

function defaultPieceColor(info) {
	const colors = (info.seats || []).map((s) => s.color);
	return colors.includes("white") ? "white" : colors[0] || "white";
}

function seatNode(info, n) {
	const seat = (info.seats || []).find((s) => s.seat === n);
	if (!seat) return null;
	return h(
		"span",
		{ class: "md-seat" },
		h("span", { class: `swatch color-${cssColor(seat.color)}`, "aria-hidden": "true" }),
		h("strong", {}, seat.label)
	);
}

/** Opens (or reopens) the rules window for the game described by info. */
export async function showRules(info, { tab } = {}) {
	const old = document.getElementById("rules-dialog");
	if (old) old.remove();

	const title = h("h2", { id: "rules-title" }, `${info.name} — rules`);
	const closeBtn = h("button", { class: "rules-close", "aria-label": "Close the rules", title: "Close (Esc)" }, "×");
	const search = h("input", {
		type: "search",
		id: "rules-search",
		class: "rules-search",
		placeholder: "Search the rules…",
		"aria-label": "Search the rules",
		autocomplete: "off",
	});
	const tablist = h("div", { class: "rules-tabs", role: "tablist", "aria-label": "Rules sections" });
	const body = h("div", { class: "rules-body" }, h("p", { class: "muted" }, "Loading the rules…"));
	const dlg = h(
		"dialog",
		{ id: "rules-dialog", class: "rules-window", "aria-labelledby": "rules-title" },
		h("div", { class: "rules-head" }, title, search, closeBtn),
		tablist,
		body
	);
	closeBtn.addEventListener("click", () => dlg.close());
	dlg.addEventListener("close", () => dlg.remove());
	// A click on the backdrop (the dialog element itself) closes it.
	dlg.addEventListener("click", (ev) => {
		if (ev.target === dlg) dlg.close();
	});
	document.body.append(dlg);
	if (typeof dlg.showModal === "function") dlg.showModal();
	else dlg.setAttribute("open", "");

	let guide;
	try {
		guide = await api.guide(info.id);
	} catch (err) {
		clear(body).append(h("p", { class: "error" }, "The rules could not be loaded: " + err.message));
		return dlg;
	}
	const pages = [...guide.tabs];
	const key = keyPage(info);
	if (key) pages.push({ id: "key", title: "Key", markdown: key });
	pages.push({ id: "controls", title: "Controls", markdown: controlsPage(info) });
	build(dlg, info, pages, { tablist, body, search, initial: tab });
	return dlg;
}

function build(dlg, info, pages, { tablist, body, search, initial }) {
	const assetBase = `/api/game-defs/${encodeURIComponent(info.id)}/assets/`;
	const panels = new Map();
	const tabs = new Map();
	const headings = new Map();
	const pieceColor = defaultPieceColor(info);

	const activate = (id, anchor = "", focus = false) => {
		if (!panels.has(id)) id = pages[0].id;
		for (const [pid, panel] of panels) {
			const on = pid === id;
			panel.hidden = !on;
			const t = tabs.get(pid);
			t.setAttribute("aria-selected", on ? "true" : "false");
			t.tabIndex = on ? 0 : -1;
			t.classList.toggle("active", on);
		}
		store.set(KEY + info.id, id);
		body.scrollTop = 0;
		if (anchor) {
			const el = document.getElementById(`rules-${id}-md-${anchor}`);
			if (el) el.scrollIntoView({ block: "start" });
		}
		if (focus) tabs.get(id).focus();
	};

	pages.forEach((page, idx) => {
		const tabId = `rules-tab-${page.id}`;
		const panelId = `rules-panel-${page.id}`;
		const rendered = renderMarkdown(page.markdown, {
			assetBase,
			idPrefix: `rules-${page.id}-`,
			piece: (type, color) => {
				const def = (info.pieces || []).find((p) => p.type === type);
				return def ? pieceIcon(def.shape, color || pieceColor, 28) : null;
			},
			seat: (n) => seatNode(info, n),
			marker: (kind, arg) => {
				if (kind === "gate") {
					// A Gate with its occupant: the first unit that can hold one.
					const holder = (info.pieces || []).find((p) => p.type === "cultist");
					return legendMarker("gate", arg, info, holder ? holder.shape : "");
				}
				return legendMarker(kind, arg, info);
			},
			onTab: (id, anchor) => activate(id, anchor, false),
			onAnchor: (id) => {
				const el = document.getElementById(`rules-${page.id}-md-${id}`);
				if (el) el.scrollIntoView({ block: "start" });
			},
		});
		headings.set(page.id, rendered.headings);
		const panel = h("section", { class: "rules-panel", id: panelId, role: "tabpanel", "aria-labelledby": tabId, hidden: true });
		const toc = rendered.headings.filter((x) => x.level >= 2);
		if (toc.length >= 4) {
			const nav = h("nav", { class: "rules-toc", "aria-label": "On this page" });
			const ul = h("ul");
			for (const hd of toc) {
				ul.append(
					h(
						"li",
						{ class: `lvl-${hd.level}` },
						h(
							"a",
							{
								href: "#" + hd.id,
								onclick: (ev) => {
									ev.preventDefault();
									const el = document.getElementById(`rules-${page.id}-md-${hd.id}`);
									if (el) el.scrollIntoView({ block: "start" });
								},
							},
							hd.text
						)
					)
				);
			}
			nav.append(h("strong", {}, "On this page"), ul);
			panel.classList.add("has-toc");
			panel.append(rendered.node, nav);
		} else {
			panel.append(rendered.node);
		}
		panels.set(page.id, panel);

		const tab = h(
			"button",
			{ id: tabId, class: "rules-tab", role: "tab", "aria-controls": panelId, "aria-selected": "false", tabindex: "-1", "data-tab": page.id },
			page.title
		);
		tab.addEventListener("click", () => {
			search.value = "";
			clearResults();
			activate(page.id);
		});
		tab.addEventListener("keydown", (ev) => {
			let to = null;
			if (ev.key === "ArrowRight") to = (idx + 1) % pages.length;
			else if (ev.key === "ArrowLeft") to = (idx - 1 + pages.length) % pages.length;
			else if (ev.key === "Home") to = 0;
			else if (ev.key === "End") to = pages.length - 1;
			if (to === null) return;
			ev.preventDefault();
			activate(pages[to].id, "", true);
		});
		tabs.set(page.id, tab);
		tablist.append(tab);
	});

	clear(body);
	for (const panel of panels.values()) body.append(panel);
	const results = h("section", { class: "rules-results", id: "rules-results", hidden: true, "aria-live": "polite" });
	body.append(results);

	// ---- search -------------------------------------------------------------
	const sections = [];
	for (const page of pages) {
		const panel = panels.get(page.id);
		let current = { page, heading: page.title, id: "", text: [] };
		sections.push(current);
		for (const el of panel.querySelector(".md").children) {
			if (/^H[1-6]$/.test(el.tagName)) {
				current = { page, heading: el.textContent, id: el.id, text: [] };
				sections.push(current);
			} else {
				current.text.push(el.textContent);
			}
		}
	}
	for (const sec of sections) sec.body = sec.text.join(" ").replace(/\s+/g, " ").trim();

	const clearResults = () => {
		results.hidden = true;
		clear(results);
		const active = store.get(KEY + info.id, pages[0].id);
		for (const [pid, panel] of panels) panel.hidden = pid !== (panels.has(active) ? active : pages[0].id);
	};
	const runSearch = () => {
		const q = search.value.trim().toLowerCase();
		if (q.length < 2) {
			clearResults();
			return;
		}
		for (const panel of panels.values()) panel.hidden = true;
		results.hidden = false;
		clear(results);
		const hits = [];
		for (const sec of sections) {
			const hay = (sec.heading + " " + sec.body).toLowerCase();
			const at = hay.indexOf(q);
			if (at < 0) continue;
			hits.push({ sec, score: sec.heading.toLowerCase().includes(q) ? 0 : 1 });
		}
		hits.sort((a, b) => a.score - b.score);
		results.append(h("h3", {}, hits.length ? `${hits.length} result${hits.length === 1 ? "" : "s"} for “${search.value.trim()}”` : `Nothing matches “${search.value.trim()}”.`));
		for (const { sec } of hits.slice(0, 40)) {
			const at = sec.body.toLowerCase().indexOf(q);
			let snippet = [];
			if (at >= 0) {
				const from = Math.max(0, at - 50);
				const to = Math.min(sec.body.length, at + q.length + 90);
				snippet = [
					(from > 0 ? "…" : "") + sec.body.slice(from, at),
					h("mark", {}, sec.body.slice(at, at + q.length)),
					sec.body.slice(at + q.length, to) + (to < sec.body.length ? "…" : ""),
				];
			} else {
				snippet = [sec.body.slice(0, 120)];
			}
			results.append(
				h(
					"a",
					{
						class: "rules-hit",
						href: "#",
						onclick: (ev) => {
							ev.preventDefault();
							search.value = "";
							clearResults();
							activate(sec.page.id);
							if (sec.id) {
								const el = document.getElementById(sec.id);
								if (el) el.scrollIntoView({ block: "start" });
							}
						},
					},
					h("span", { class: "rules-hit-where" }, `${sec.page.title} › ${sec.heading}`),
					h("span", { class: "rules-hit-text" }, snippet)
				)
			);
		}
	};
	search.addEventListener("input", runSearch);
	search.addEventListener("keydown", (ev) => {
		if (ev.key === "Escape" && search.value) {
			ev.stopPropagation();
			search.value = "";
			clearResults();
		}
	});

	const remembered = store.get(KEY + info.id, null);
	activate(initial && panels.has(initial) ? initial : remembered && panels.has(remembered) ? remembered : pages[0].id);
}
