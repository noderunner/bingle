// Badges as the page shows them (ADR 0130): the server says what a badge is,
// and the page draws it. One shelf serves your own page, where the badges not
// yet earned are greyed out, another player's, which shows only what they
// hold, and a game's rules window, which shows that game's.

import { h, clear, store } from "./util.js";
import { api } from "./api.js";

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
 * The badges to show, each what the catalog says it is with the award, if
 * held. With all, every badge there is, earned or not; otherwise only those
 * held. An award of a badge no longer in the catalog keeps its own name.
 */
function joined(list, held, all) {
	const byId = new Map(list.map((b) => [b.badge, b]));
	const awards = new Map(held.map((a) => [a.badge, a]));
	const out = [];
	if (all) for (const b of list) out.push({ ...b, award: awards.get(b.badge) || null });
	for (const a of held) {
		if (all && byId.has(a.badge)) continue;
		const b = byId.get(a.badge);
		out.push(
			b
				? { ...b, award: a }
				: // A retired badge: a game's id is family.id, the platform's has no dot.
					{ badge: a.badge, name: a.name, description: a.description, platform: !a.badge.includes("."), family: a.badge.includes(".") ? a.family : "", game: a.badge.includes(".") ? a.game : "", kind: "", award: a }
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
			return `Earned against computer opponents: beat ${b.bot || "the named bot"} or a stronger one.`;
		case "player":
			return "Earned in games against other players.";
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
		const where = [a.game && `in ${a.game}`, earned(a.awarded_at) && `on ${earned(a.awarded_at)}`].filter(Boolean).join(" ");
		lines.push(h("p", { class: "small", id: "badge-dialog-earned" }, `Earned ${where}.`));
		if (a.hidden) lines.push(h("p", { class: "small muted" }, "Others see this badge when that game ends."));
		if (a.room) lines.push(h("p", { class: "small" }, h("a", { href: `/game/${encodeURIComponent(a.room)}`, id: "badge-dialog-room" }, "Open the game it was earned in")));
	} else if (mine) {
		lines.push(h("p", { class: "small muted", id: "badge-dialog-earned" }, "Not earned yet."));
	}
	const close = h("button", { type: "button", class: "ghost", id: "badge-dialog-close" }, "Close");
	const dlg = h(
		"dialog",
		{ id: "badge-dialog", class: "badge-dialog", "aria-labelledby": "badge-dialog-title", "data-badge": b.badge },
		h("div", { class: "badge-dialog-head" }, medal(b, !a && mine), h("h2", { id: "badge-dialog-title" }, b.name)),
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

function medal(b, unearned) {
	return h("span", { class: "badge-medal" + (unearned ? " unearned" : ""), "aria-hidden": "true" }, "★");
}

/** One badge on a shelf: a button that opens its details. */
export function badgeItem(b, { mine = false, alsoMine = false } = {}) {
	const a = b.award;
	const unearned = !a && mine;
	const where = a ? [a.game, earned(a.awarded_at)].filter(Boolean).join(", ") : unearned ? "Not earned yet" : "";
	return h(
		"li",
		{ class: "badge-item" + (unearned ? " unearned" : ""), "data-badge": b.badge, "data-earned": a ? "true" : "false" },
		h(
			"button",
			{ type: "button", class: "badge-open", onclick: () => showBadge(b, { mine }), "aria-label": `${b.name}${unearned ? ", not earned yet" : ""}: details` },
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
