// Badges as the page shows them (ADR 0130): the server says what a badge is,
// and the page draws it. One shelf serves your own page, where the badges not
// yet earned are greyed out, another player's, which shows only what they
// hold, and a game's rules window, which shows that game's.

import { h, clear, store } from "./util.js";
import { api } from "./api.js";
import { botIcon, trophyIcon } from "./ui-icons.js";
import { t, lang } from "./i18n.js";

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
	const at = Date.parse(iso || "");
	return Number.isNaN(at) ? "" : new Date(at).toLocaleDateString(lang, { year: "numeric", month: "short", day: "numeric" });
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

// ---- icons: a game's own, from its web.yaml (ADR 0124), or the trophy ----------

const looks = new Map();
const icons = new Map();

/**
 * Reads the icons of the badges of every game among items, once a page:
 * a game's web.yaml names a picture among its files for each of its badges
 * it draws (badges.<id>.icon).
 */
export async function loadIcons(items) {
	const families = [...new Set(items.filter((b) => !b.platform && b.family && b.badge.includes(".")).map((b) => b.family))];
	await Promise.all(
		families.map((family) => {
			if (!looks.has(family)) {
				looks.set(
					family,
					api
						.gameLook(family)
						.then((look) => {
							for (const [id, b] of Object.entries((look && look.badges) || {})) {
								if (b && b.icon) icons.set(`${family}.${id}`, `/api/game-defs/${encodeURIComponent(family)}/assets/${encodeURI(b.icon)}`);
							}
						})
						.catch(() => {})
				);
			}
			return looks.get(family);
		})
	);
}

/**
 * A badge's medal: its game's icon for it, or the trophy, with a mark in its
 * lower corners, like a notification badge, for each way it was earned: a bot
 * at the left in a game with computer opponents, a gold star at the right in
 * a game of people only. Greyed out when not earned yet.
 */
export function medal(b, unearned, markSize = 14) {
	const ways = b.ways || {};
	const src = icons.get(b.badge);
	return h(
		"span",
		{ class: "badge-medal" + (unearned ? " unearned" : "") + (src ? " has-icon" : ""), "aria-hidden": "true" },
		src ? h("img", { class: "badge-icon", src, alt: "", decoding: "async" }) : trophyIcon(),
		ways.bots ? h("span", { class: "medal-mark medal-bot", title: t("badge.with_bots") }, botIcon(markSize)) : null,
		ways.people ? h("span", { class: "medal-mark medal-people", title: t("badge.with_people") }, "★") : null
	);
}

/** How a held badge was earned, in words for a screen reader. */
function waysText(b) {
	const w = b.ways || {};
	if (w.people && w.bots) return t("badge.both_ways");
	if (w.bots) return t("badge.bots_way");
	if (w.people) return t("badge.people_way");
	return "";
}

/** One badge on a shelf: its medal, name and what it is for, and where and when it was earned. */
export function badgeItem(b, { mine = false, alsoMine = false } = {}) {
	const a = b.award;
	const unearned = !a && mine;
	const where = a ? [a.game, earned(a.awarded_at)].filter(Boolean).join(", ") : unearned ? t("badge.not_yet_earned") : "";
	return h(
		"li",
		{
			class: "badge-item" + (unearned ? " unearned" : ""),
			"data-badge": b.badge,
			"data-earned": a ? "true" : "false",
			"data-ways": [b.ways.people && "people", b.ways.bots && "bots"].filter(Boolean).join(" "),
		},
		medal(b, unearned),
		h(
			"span",
			{ class: "badge-body" },
			h("strong", { class: "badge-name" }, b.name),
			h("span", { class: "small" }, b.description),
			where ? h("span", { class: "small muted" }, where + (a && a.hidden ? " · " + t("badge.others_later") : "")) : null,
			waysText(b) ? h("span", { class: "sr-only" }, waysText(b)) : null,
			alsoMine ? h("span", { class: "small badge-also" }, t("badge.you_too")) : null
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
	await loadIcons(items);
	const ownIds = new Set((mineHeld || []).map((a) => a.badge));

	const groups = [];
	for (const b of items) {
		const g = groupOf(b);
		if (!groups.some(([v]) => v === g)) groups.push([g, g === "platform" ? t("badge.platform") : b.game || g]);
	}
	groups.sort((x, y) => (x[0] === "platform" ? -1 : y[0] === "platform" ? 1 : x[1].localeCompare(y[1])));

	let filter = family ? "all" : store.get(FILTER_KEY, "all");
	if (filter !== "all" && !groups.some(([v]) => v === filter)) filter = "all";
	let shown = mine && !family ? store.get(SHOWN_KEY, "all") : "all";

	const paint = () => {
		clear(box);
		if (!family && groups.length > 1) {
			box.append(
				chips(t("badge.show_of"), "badge-filter", [["all", t("badge.all")], ...groups], filter, (v) => {
					filter = v;
					store.set(FILTER_KEY, v);
					paint();
				})
			);
		}
		if (mine && !family) {
			box.append(
				chips(t("badge.show"), "badge-shown", [["all", t("badge.all")], ["earned", t("badge.earned_filter")], ["unearned", t("badge.not_yet")]], shown, (v) => {
					shown = v;
					store.set(SHOWN_KEY, v);
					paint();
				})
			);
		}
		const visible = items.filter((b) => (filter === "all" || groupOf(b) === filter) && (shown === "all" || (shown === "earned") === Boolean(b.award)));
		if (!visible.length) {
			box.append(h("p", { class: "muted small badges-none" }, held.length || mine ? t("badge.none_here") : t("friend.no_badges")));
			return;
		}
		box.append(h("ul", { class: "badge-list" }, ...visible.map((b) => badgeItem(b, { mine, alsoMine: !mine && ownIds.has(b.badge) }))));
	};
	paint();
	return items;
}
