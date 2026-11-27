// The rules window: a floating dialog with one tab per page of the game's
// guide, rich text (headings, tables, callouts, inline images and piece
// icons), a table of contents for long pages and a search across all of
// them. Games without their own guide get one generated from their
// definition by the server, so every game gets the same window.

import { h, clear, store } from "./util.js";
import { api, gameTitle, whoAmI } from "./api.js";
import { renderMarkdown } from "./markdown.js";
import { pieceIcon, legendMarker } from "./render.js";
import { seatKey } from "./paint.js";
import { tutorialList } from "./tutorials-ui.js";
import { badgeCatalog, paintShelf } from "./badges.js";
import { t, forwardKey, backKey } from "./i18n.js";

const KEY = "bingle.rules.";

const escapeMd = (t) => String(t).replace(/([\\`*_{}[\]<>|~])/g, "\\$1");

/** The client-side page: how to play with the mouse and keyboard. */
function controlsPage(info) {
	const parts = [
		t("rules.controls_title"),
		"",
		t("rules.making_move"),
		"",
		t("rules.move_click"),
		t("rules.move_sent"),
		t("rules.move_hover"),
		"",
		t("rules.keyboard"),
		"",
		t("rules.key_head"),
		"|---|---|",
		t("rules.key_tab"),
		t("rules.key_arrows"),
		t("rules.key_enter"),
		t("rules.key_enter_else"),
		t("rules.key_esc"),
		t("rules.key_back"),
		t("rules.key_history"),
		t("rules.key_home"),
		"",
	];
	if ((info.notation_help || []).length) {
		parts.push(t("rules.notation"), "", t("rules.notation_text"), "");
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
	if (!d || (info.engine !== "ruleset" && !Object.keys(d.tints || {}).length)) return "";
	const parts = [];
	// A game whose page script knows its marks writes its own key (Cthulhu Wars' map).
	if (info.page && info.page.mapKey) return info.page.mapKey(info);
	parts.push(t("rules.key_title"), "", t("rules.key_text"), "", t("rules.key_table"), "|---|---|");
	const row = (def) => {
		const label = `${def.glyph ? def.glyph + " " : ""}${def.label}`;
		parts.push(`| ![${escapeMd(def.label)}](marker:swatch:${def.paint}) | **${escapeMd(label)}**${def.text ? " — " + escapeMd(def.text) : ""} |`);
	};
	for (const [k, def] of Object.entries(d.tints || {})) {
		if (!k.startsWith("zone") && def.paint) row(def);
	}
	for (const [k, def] of Object.entries(d.badges || {})) {
		if (k !== "active" && k !== "target" && def.paint) row(def);
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
		seatKey(seat, info),
		h("strong", {}, seat.label)
	);
}

/** The game's badges: those the reader holds, and greyed out, the rest. */
function badgesPage(family) {
	const shelf = h("div", { id: "rules-badges" });
	const intro = h("p", { class: "small muted" }, t("rules.badges_intro"));
	whoAmI()
		.catch(() => null)
		.then(async (me) => {
			const account = me && me.kind === "account" && me.handle;
			if (!account) intro.append(" " + t("rules.badges_sign_in"));
			const held = account ? await api.playerBadges(me.handle).then((r) => r.badges).catch(() => []) : [];
			await paintShelf(shelf, { held, mine: true, family });
		});
	return h("div", {}, h("h1", {}, t("settings.badges")), intro, shelf);
}

/** Opens (or reopens) the rules window for the game described by info. */
export async function showRules(info, { tab } = {}) {
	const old = document.getElementById("rules-dialog");
	if (old) old.remove();

	const title = h("h2", { id: "rules-title" }, t("rules.title", { game: gameTitle(info) }));
	const closeBtn = h("button", { class: "rules-close", "aria-label": t("rules.close"), title: t("common.close_esc") }, "×");
	const search = h("input", {
		type: "search",
		id: "rules-search",
		class: "rules-search",
		placeholder: t("rules.search"),
		"aria-label": t("rules.search_label"),
		autocomplete: "off",
	});
	const tablist = h("div", { class: "rules-tabs", role: "tablist", "aria-label": t("rules.sections") });
	// The window opens on the rules, not the search box: on a phone a focused box brings up the
	// keyboard, and may zoom the page so that the close button is out of reach.
	const body = h("div", { class: "rules-body", tabindex: -1, autofocus: true }, h("p", { class: "muted" }, t("rules.loading")));
	const dlg = h(
		"dialog",
		{ id: "rules-dialog", class: "rules-window", "aria-labelledby": "rules-title" },
		h("div", { class: "rules-head" }, title, search, closeBtn),
		tablist,
		body
	);
	closeBtn.addEventListener("click", () => dlg.close());
	// The Back button or gesture closes the window, not the page behind it.
	let leftByBack = false;
	const onBack = () => {
		leftByBack = true;
		window.removeEventListener("popstate", onBack);
		if (dlg.open) dlg.close();
	};
	const openedOn = location.pathname;
	history.pushState(history.state, "", location.href);
	window.addEventListener("popstate", onBack);
	dlg.addEventListener("close", () => {
		dlg.remove();
		window.removeEventListener("popstate", onBack);
		// Unless a link in the rules has already left for another page.
		if (!leftByBack && location.pathname === openedOn) history.back();
	});
	// A click on the backdrop (the dialog element itself) closes it.
	dlg.addEventListener("click", (ev) => {
		if (ev.target === dlg) dlg.close();
	});
	document.body.append(dlg);
	if (typeof dlg.showModal === "function") dlg.showModal();
	else dlg.setAttribute("open", "");

	let guide;
	let tutorials = null;
	let badges = [];
	try {
		[guide, tutorials, badges] = await Promise.all([api.guide(info.id), api.tutorials(info.id).catch(() => null), badgeCatalog()]);
	} catch (err) {
		clear(body).append(h("p", { class: "error" }, t("rules.not_loaded", { error: err.message })));
		return dlg;
	}
	const pages = [...guide.tabs];
	// The game's tutorials teach by playing (ADR 0077): a tab of their own.
	if (tutorials && tutorials.tutorials.length) {
		pages.push({ id: "tutorials", title: t("setup.tutorials"), node: tutorialList(tutorials.family, tutorials.tutorials) });
	}
	// The game's own badges (ADR 0130), with those the reader holds.
	if (info.family && badges.some((b) => !b.platform && b.family === info.family)) {
		pages.push({ id: "badges", title: t("settings.badges"), node: badgesPage(info.family) });
	}
	const key = keyPage(info);
	if (key) pages.push({ id: "key", title: t("rules.tab_key"), markdown: key });
	pages.push({ id: "controls", title: t("rules.tab_controls"), markdown: controlsPage(info) });
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
		// A page is Markdown, or an element of its own (the tutorials).
		const rendered = page.node ? { node: h("div", { class: "md" }, page.node), headings: [] } : renderMarkdown(page.markdown, {
			assetBase,
			idPrefix: `rules-${page.id}-`,
			piece: (type, color) => {
				const def = (info.pieces || []).find((p) => p.type === type);
				return def ? pieceIcon(def.shape, color || pieceColor, 28, info) : null;
			},
			seat: (n) => seatNode(info, n),
			// The game's page script draws its own marks; a swatch is every game's.
			marker: (kind, arg) => (info.page && info.page.legendMarker ? info.page.legendMarker(kind, arg, info) : legendMarker(kind, arg)),
			// A picture the game draws itself, such as its map (ADR 0124: the server draws none).
			figure: (name) => (info.page && info.page.guideFigure ? info.page.guideFigure(name, info) : null),
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
			const nav = h("nav", { class: "rules-toc", "aria-label": t("rules.on_page") });
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
			nav.append(h("strong", {}, t("rules.on_page")), ul);
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
			if (ev.key === forwardKey) to = (idx + 1) % pages.length;
			else if (ev.key === backKey) to = (idx - 1 + pages.length) % pages.length;
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
		results.append(h("h3", {}, hits.length ? t("rules.results", { n: hits.length, query: search.value.trim() }) : t("rules.no_match", { query: search.value.trim() })));
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
