// Badges as the page shows them (ADR 0130): the server says what a badge is,
// and the page draws it. One shelf serves your own page, where the badges not
// yet earned are greyed out, another player's, which shows only what they
// hold, and a game's rules window, which shows that game's.

import { h, clear, store } from "./util.js";
import { api } from "./api.js";
import { botIcon } from "./ui-icons.js";

const FILTER_KEY = "bingle.badges.filter";
const SHOWN_KEY = "bingle.badges.shown";

let catalog = null;

/** Every badge there is to earn, read once a page. */
export function badgeCatalog() {
	if (!catalog) {
		catalog = api.badges().then((r) => r.badges || []).catch(() => {
			catalog = null;
			return [];
		});
	}
	return catalog;
}

/** The day a badge was earned, in the reader's own way of writing dates. */
function earned(iso) {
	const t = Date.parse(iso || "");
	return Number.isNaN(t) ? "" : new Date(t).toLocaleDateString([], { year: "numeric", month: "short", day: "numeric" });
}

/**
 * The awards of each badge held, by how they were earned: a badge earned
 * against people and against bots is held twice. The first is the one shown
 * first.
 */
function heldWays(held) {
	const out = new Map();
	for (const a of held) {
		const w = out.get(a.badge) || { first: a, people: null, bots: null };
		w[a.against === "bots" ? "bots" : "people"] = a;
		out.set(a.badge, w);
	}
	return out;
}

/**
 * The badges to show, each what the catalog says it is with its awards, if
 * held: award, the first, and ways, the award of each way it was earned.
 * With all, every badge there is, earned or not; otherwise only those held.
 * An award of a badge no longer in the catalog keeps its own name.
 */
function joined(list, held, all) {
	const byId = new Map(list.map((b) => [b.badge, b]));
	const ways = heldWays(held);
	const withWays = (b, w) => ({ ...b, award: w ? w.first : null, ways: w ? { people: w.people, bots: w.bots } : { people: null, bots: null } });
	const out = [];
	if (all) for (const b of list) out.push(withWays(b, ways.get(b.badge)));
	for (const [id, w] of ways) {
		if (all && byId.has(id)) continue;
		const a = w.first;
		const b = byId.get(id);
		const game = id.includes(".");
		out.push(
			withWays(
				// A retired badge: a game's id is family.id, the platform's has no dot.
				b || { badge: id, name: a.name, description: a.description, platform: !game, family: game ? a.family : "", game: game ? a.game : "", kind: "" },
				w
			)
		);
	}
	// Earned first, then the rest, each in the catalog's order.
	return out.sort((x, y) => Number(!x.award) - Number(!y.award));
}

/** Where a badge belongs among the filters: the platform, or its game. */
const groupOf = (b) => (b.platform ? "platform" : b.family || "platform");

/** What kind of game a badge counts, in words. */
function kindText(b) {
	switch (b.kind) {
		case "rated":
			return "Counts rated games only.";
		case "bot":
			return `Earned in games with computer opponents: beat ${b.bot || "the named bot"} or a stronger one.`;
		case "player":
			return b.bots
				? "Earned two ways: in a game of people only, which gives it the gold star, and in a game with a computer opponent at the table, which gives it the bot."
				: "Earned in games of people only.";
		default:
			return "";
	}
}

/** One badge's details, in a dialog of its own. */
export function showBadge(b, { mine = false } = {}) {
	document.getElementById("badge-dialog")?.remove();
	const a = b.award;
	const lines = [h("p", {}, b.description)];
	const facts = [];
	facts.push(b.platform ? (b.game ? `A platform badge, earned in ${b.game}.` : "A platform badge, earned in any game.") : `A badge of ${b.game}.`);
	if (kindText(b)) facts.push(kindText(b));
	if (b.when) facts.push(b.when === "during" ? "Awarded the moment it happens." : "Checked when the game ends.");
	lines.push(h("p", { class: "small muted" }, facts.join(" ")));
	if (a) {
		const both = b.ways.people && b.ways.bots;
		const earnedBox = h("div", { id: "badge-dialog-earned" });
		for (const [way, label] of [["people", "With people only"], ["bots", "With computer opponents"]]) {
			const w = b.ways[way];
			if (!w) continue;
			const where = [w.game, earned(w.awarded_at)].filter(Boolean).join(", ");
			const line = h("p", { class: "small", "data-way": way }, `${label}: earned ${where ? "in " + where : ""}.`);
			if (w.room) line.append(" ", h("a", { href: `/game/${encodeURIComponent(w.room)}`, class: "badge-dialog-room" }, "Open the game"));
			earnedBox.append(line);
			if (w.hidden) earnedBox.append(h("p", { class: "small muted" }, "Others see this when that game ends."));
		}
		lines.push(earnedBox);
		if (mine && b.bots && !both) {
			lines.push(h("p", { class: "small muted" }, b.ways.people ? "Not yet earned in a game with computer opponents." : "Not yet earned in a game of people only."));
		}
	} else if (mine) {
		lines.push(h("p", { class: "small muted", id: "badge-dialog-earned" }, "Not earned yet."));
	}
	const close = h("button", { type: "button", class: "ghost", id: "badge-dialog-close" }, "Close");
	const dlg = h(
		"dialog",
		{ id: "badge-dialog", class: "badge-dialog", "aria-labelledby": "badge-dialog-title", "data-badge": b.badge },
		h("div", { class: "badge-dialog-head" }, medal(b, !a && mine, 22), h("h2", { id: "badge-dialog-title" }, b.name)),
		...lines,
		h("div", { class: "row" }, close)
	);
	close.addEventListener("click", () => dlg.close());
	dlg.addEventListener("click", (ev) => {
		if (ev.target === dlg) dlg.close();
	});
	dlg.addEventListener("close", () => dlg.remove());
	document.body.append(dlg);
	if (typeof dlg.showModal === "function") dlg.showModal();
	else dlg.setAttribute("open", "");
	close.focus();
	return dlg;
}

/**
 * A badge's medal, with a mark in its corners for each way it was earned: a
 * bot at the lower left in a game with computer opponents, a gold star at the
 * lower right in a game of people only.
 */
export function medal(b, unearned, markSize = 14) {
	const ways = b.ways || {};
	return h(
		"span",
		{ class: "badge-medal" + (unearned ? " unearned" : ""), "aria-hidden": "true" },
		"★",
		ways.bots ? h("span", { class: "medal-mark medal-bot", title: "Earned in a game with computer opponents" }, botIcon(markSize)) : null,
		ways.people ? h("span", { class: "medal-mark medal-people", title: "Earned in a game of people only" }, "★") : null
	);
}

/** How a held badge was earned, in words for a screen reader. */
function waysText(b) {
	const w = b.ways || {};
	if (w.people && w.bots) return ", earned with people only and with computer opponents";
	if (w.bots) return ", earned with computer opponents";
	return "";
}

/** One badge on a shelf: a button that opens its details. */
export function badgeItem(b, { mine = false, alsoMine = false } = {}) {
	const a = b.award;
	const unearned = !a && mine;
	const where = a ? [a.game, earned(a.awarded_at)].filter(Boolean).join(", ") : unearned ? "Not earned yet" : "";
	return h(
		"li",
		{
			class: "badge-item" + (unearned ? " unearned" : ""),
			"data-badge": b.badge,
			"data-earned": a ? "true" : "false",
			"data-ways": [b.ways.people && "people", b.ways.bots && "bots"].filter(Boolean).join(" "),
		},
		h(
			"button",
			{ type: "button", class: "badge-open", onclick: () => showBadge(b, { mine }), "aria-label": `${b.name}${unearned ? ", not earned yet" : waysText(b)}: details` },
			medal(b, unearned),
			h(
				"span",
				{ class: "badge-body" },
				h("strong", { class: "badge-name" }, b.name),
				h("span", { class: "small" }, b.description),
				h("span", { class: "small muted" }, where + (a && a.hidden ? " · others see it when the game ends" : "")),
				alsoMine ? h("span", { class: "small badge-also" }, "You have this too") : null
			)
		)
	);
}

/** A row of filter chips, one of which is on. */
function chips(label, id, options, on, pick) {
	return h(
		"div",
		{ class: "chips", role: "group", "aria-label": label, id },
		...options.map(([value, text]) =>
			h("button", { type: "button", class: "chip" + (value === on ? " on" : ""), "aria-pressed": value === on ? "true" : "false", "data-filter": value, onclick: () => pick(value) }, text)
		)
	);
}

/**
 * Fills box with a player's badges. held is what they hold; mine says they
 * are the reader, so the badges not yet earned show too, greyed out; mineHeld,
 * on another player's shelf, is what the reader holds, to mark what both
 * have. With family, only that game's badges, and no filters.
 */
export async function paintShelf(box, { held, mine = false, mineHeld = null, family = "" }) {
	const list = await badgeCatalog();
	let items = joined(list, held, mine);
	if (family) items = items.filter((b) => !b.platform && b.family === family);
	const ownIds = new Set((mineHeld || []).map((a) => a.badge));

	const groups = [];
	for (const b of items) {
		const g = groupOf(b);
		if (!groups.some(([v]) => v === g)) groups.push([g, g === "platform" ? "Platform" : b.game || g]);
	}
	groups.sort((x, y) => (x[0] === "platform" ? -1 : y[0] === "platform" ? 1 : x[1].localeCompare(y[1])));

	let filter = family ? "all" : store.get(FILTER_KEY, "all");
	if (filter !== "all" && !groups.some(([v]) => v === filter)) filter = "all";
	let shown = mine && !family ? store.get(SHOWN_KEY, "all") : "all";

	const paint = () => {
		clear(box);
		if (!family && groups.length > 1) {
			box.append(
				chips("Show the badges of", "badge-filter", [["all", "All"], ...groups], filter, (v) => {
					filter = v;
					store.set(FILTER_KEY, v);
					paint();
				})
			);
		}
		if (mine && !family) {
			box.append(
				chips("Show", "badge-shown", [["all", "All"], ["earned", "Earned"], ["unearned", "Not yet"]], shown, (v) => {
					shown = v;
					store.set(SHOWN_KEY, v);
					paint();
				})
			);
		}
		const visible = items.filter((b) => (filter === "all" || groupOf(b) === filter) && (shown === "all" || (shown === "earned") === Boolean(b.award)));
		if (!visible.length) {
			box.append(h("p", { class: "muted small badges-none" }, held.length || mine ? "No badges here." : "No badges yet."));
			return;
		}
		box.append(h("ul", { class: "badge-list" }, ...visible.map((b) => badgeItem(b, { mine, alsoMine: !mine && ownIds.has(b.badge) }))));
	};
	paint();
	return items;
}
