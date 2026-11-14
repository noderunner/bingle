// A player's @handle, in a chat or at a seat: a badge whose menu shows their badges and the way to
// their page, and offers a friend request to somebody who is not a friend yet. A guest's menu has
// only the way to the page, since guests see no badges.

import { h } from "./util.js";
import { api, whoAmI } from "./api.js";
import { askFriend, friendStateOf, loadFriends } from "./dm.js";
import { medal } from "./badges.js";

let pop = null;

function closePop() {
	pop?.remove();
	pop = null;
	document.removeEventListener("pointerdown", awayFromPop, true);
	document.removeEventListener("keydown", escapeFromPop, true);
}

function awayFromPop(ev) {
	if (pop && !pop.contains(ev.target)) closePop();
}

function escapeFromPop(ev) {
	if (ev.key === "Escape") closePop();
}

/** A small menu beside the badge. */
function openPop(anchor, ...content) {
	closePop();
	pop = h("div", { class: "friend-pop", id: "friend-pop", role: "dialog", "aria-label": "Player" }, ...content);
	document.body.append(pop);
	const at = anchor.getBoundingClientRect();
	const box = pop.getBoundingClientRect();
	pop.style.left = `${Math.max(8, Math.min(at.left, window.innerWidth - box.width - 8))}px`;
	pop.style.top = `${at.bottom + box.height + 8 > window.innerHeight ? Math.max(8, at.top - box.height - 4) : at.bottom + 4}px`;
	document.addEventListener("pointerdown", awayFromPop, true);
	document.addEventListener("keydown", escapeFromPop, true);
}

async function offerFriend(anchor, handle) {
	const me = await whoAmI().catch(() => ({}));
	const page = `/players/${encodeURIComponent(handle)}`;
	// A guest sees no badges, and yourself no friend request: the way to the
	// player's page is all there is.
	if (me.kind !== "account") {
		openPop(anchor, h("div", { class: "small pop-badges", id: "pop-badges" }, h("a", { href: page, id: "pop-player" }, `@${handle}'s page`)));
		return;
	}
	const self = me.handle && me.handle.toLowerCase() === handle.toLowerCase();
	const parts = [];
	if (!self && (me.caps || []).includes("dm")) {
		await loadFriends();
		const state = friendStateOf(handle);
		if (state === "outgoing") {
			parts.push(h("span", { class: "small muted" }, `Friend request sent to @${handle}.`));
		} else if (state === "incoming") {
			const accept = async () => {
				closePop();
				await api.friendAnswer(handle, true).catch(() => {});
				await loadFriends();
			};
			parts.push(h("button", { type: "button", class: "primary small", id: "friend-pop-accept", onclick: accept }, `Accept @${handle}'s friend request`));
		} else if (state !== "friend") {
			const ask = () => {
				closePop();
				askFriend(handle);
			};
			parts.push(h("button", { type: "button", class: "primary small", id: "friend-pop-send", onclick: ask }, `Send @${handle} a friend request`));
		}
	}
	// Their badges, which every account may see (ADR 0130).
	const held = await api
		.playerBadges(handle)
		.then((r) => r.badges)
		.catch(() => []);
	// A row of their medals, each naming its badge, and the way to their page.
	parts.push(
		h(
			"div",
			{ class: "small pop-badges", id: "pop-badges" },
			held.length
				? h("span", { class: "pop-medals" }, ...medalsOf(held))
				: h("span", { class: "muted" }, "No badges yet. "),
			h("a", { href: page, id: "pop-player" }, self ? "Your page" : held.length ? `See @${handle}'s badges` : `@${handle}'s page`)
		)
	);
	if (parts.length === 1) openPop(anchor, parts[0]);
	else if (parts.length) openPop(anchor, h("div", { class: "pop-stack" }, ...parts));
}

/** One small medal per badge held, marked with the ways it was earned. */
function medalsOf(held) {
	const byBadge = new Map();
	for (const a of held) {
		const b = byBadge.get(a.badge) || { ...a, ways: { people: null, bots: null } };
		b.ways[a.against === "bots" ? "bots" : "people"] = a;
		byBadge.set(a.badge, b);
	}
	return [...byBadge.values()].map((b) => {
		const m = medal(b, false, 9);
		m.classList.add("small-medal");
		m.removeAttribute("aria-hidden");
		m.setAttribute("role", "img");
		m.setAttribute("aria-label", b.name);
		m.title = `${b.name}: ${b.description}`;
		m.dataset.badge = b.badge;
		return m;
	});
}

/**
 * The badge of a handle, with or without its @: a button that opens a menu of
 * the player's badges, a link to their page and any friend request. cls adds
 * classes in place of handle-badge, for a badge drawn in a place's own style.
 */
export function handleBadge(name, cls = "") {
	const handle = String(name).replace(/^@/, "");
	return h(
		"button",
		{
			type: "button",
			class: cls || "handle-badge",
			title: `@${handle}: badges and page`,
			"data-handle": handle,
			onclick: (ev) => {
				ev.stopPropagation();
				offerFriend(ev.currentTarget, handle);
			},
		},
		"@" + handle
	);
}
