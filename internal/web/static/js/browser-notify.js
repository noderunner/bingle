// Browser notifications: "your turn" and a badge earned, raised when the page
// is out of sight. They are on by default for an account, off for a guest,
// and the setting is this browser's. A site cannot grant itself the browser's
// permission: the browser asks, and only after a click, so an account is
// offered it once, by a small card with a button that asks.

import { h, store, brandName } from "./util.js";
import { whoAmI } from "./api.js";

const KEY = "bingle.notify";
const OFFERED_KEY = "bingle.notifyOffered";

let account = false;
whoAmI()
	.then((me) => {
		account = me.kind === "account";
	})
	.catch(() => {});

const supported = () => "Notification" in window;

/** Whether this browser wants notifications: as set, or on for an account. */
export function notifyWanted() {
	const v = store.get(KEY, null);
	return v === null ? account : !!v;
}

/** Whether notifications are wanted and the browser allows them. */
export function notifyOn() {
	return notifyWanted() && supported() && Notification.permission === "granted";
}

/**
 * Turns notifications on or off. Turning them on asks the browser, which must
 * happen in a click; it resolves to whether they are on.
 */
export async function setNotify(on) {
	if (!on || !supported()) {
		store.set(KEY, false);
		return false;
	}
	const perm = Notification.permission === "default" ? await Notification.requestPermission() : Notification.permission;
	store.set(KEY, perm === "granted");
	return perm === "granted";
}

/** Raises a notification if the page is out of sight and they are on. */
export function notifyHidden(title, options) {
	if (!document.hidden || !notifyOn()) return;
	try {
		new Notification(title, options);
	} catch (_) {
		/* some browsers allow them only from a service worker: best-effort */
	}
}

/**
 * Offers an account notifications once, while the browser has not been asked
 * and nobody turned them off here.
 */
export async function offerNotify() {
	const me = await whoAmI().catch(() => null);
	if (!me || me.kind !== "account" || !supported() || Notification.permission !== "default") return;
	if (store.get(KEY, null) === false || store.get(OFFERED_KEY, false)) return;
	if (document.getElementById("notify-offer")) return;
	const done = () => {
		store.set(OFFERED_KEY, true);
		card.remove();
	};
	const card = h(
		"div",
		{ class: "notify-offer card", id: "notify-offer", role: "region", "aria-label": "Notifications" },
		h("p", {}, `${brandName()} can tell you when it's your turn or you earn a badge, while you're in another tab.`),
		h(
			"div",
			{ class: "row" },
			h("button", { type: "button", class: "primary", id: "notify-offer-on", onclick: async () => { done(); await setNotify(true); } }, "Turn on"),
			h("button", { type: "button", id: "notify-offer-off", onclick: () => { done(); store.set(KEY, false); } }, "No thanks")
		)
	);
	document.body.append(card);
}
