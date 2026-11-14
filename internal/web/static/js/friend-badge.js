// A player's @handle in a chat: a badge that offers to send them a friend request. For a friend, or
// for yourself, or to anybody without an account, clicking it does nothing for now.

import { h } from "./util.js";
import { api, whoAmI } from "./api.js";
import { askFriend, friendStateOf, loadFriends } from "./dm.js";

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
	pop = h("div", { class: "friend-pop", id: "friend-pop", role: "dialog", "aria-label": "Friend request" }, ...content);
	document.body.append(pop);
	const at = anchor.getBoundingClientRect();
	const box = pop.getBoundingClientRect();
	pop.style.left = `${Math.max(8, Math.min(at.left, window.innerWidth - box.width - 8))}px`;
	pop.style.top = `${at.bottom + box.height + 8 > window.innerHeight ? Math.max(8, at.top - box.height - 4) : at.bottom + 4}px`;
	document.addEventListener("pointerdown", awayFromPop, true);
	document.addEventListener("keydown", escapeFromPop, true);
}

async function offerFriend(anchor, handle) {
	const me = await whoAmI();
	if (me.kind !== "account") return;
	if (me.handle && me.handle.toLowerCase() === handle.toLowerCase()) return;
	const parts = [];
	if ((me.caps || []).includes("dm")) {
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
	if (held.length) {
		parts.push(
			h(
				"div",
				{ class: "small pop-badges", id: "pop-badges" },
				h("span", { class: "muted" }, "Badges: "),
				...held.map((b, i) => h("span", { title: b.description, "data-badge": b.badge }, (i ? ", " : "") + b.name))
			)
		);
	}
	if (parts.length === 1) openPop(anchor, parts[0]);
	else if (parts.length) openPop(anchor, h("div", { class: "pop-stack" }, ...parts));
}

/** The badge of a handle, with or without its @. */
export function handleBadge(name) {
	const handle = String(name).replace(/^@/, "");
	return h(
		"button",
		{
			type: "button",
			class: "handle-badge",
			"data-handle": handle,
			onclick: (ev) => {
				ev.stopPropagation();
				offerFriend(ev.currentTarget, handle);
			},
		},
		"@" + handle
	);
}
