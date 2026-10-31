// Email notifications in the browser: the player's address (a "contact" kept
// on the server; the browser holds only its id and secret), and the per-game
// choice to be mailed when it is one's turn. See docs/history/email-notifications-plan.md.
//
// The server never sends an address back, so the one typed in the profile is
// kept here, in this browser, to show in the form.

import { h, store, getProfile } from "./util.js";
import { api, ApiError } from "./api.js";

const CONTACT_KEY = "bingle.contact";

let configPromise = null;

/** {enabled, from}: whether the server can send mail, and from which address. */
export function notifyConfig() {
	if (!configPromise) configPromise = api.notifyConfig().catch(() => ({ enabled: false }));
	return configPromise;
}

/** The contact this browser made: {id, secret, address}, or null. */
export function localContact() {
	const c = store.get(CONTACT_KEY, null);
	return c && c.id && c.secret ? c : null;
}

function saveLocalContact(c) {
	if (c) store.set(CONTACT_KEY, c);
	else store.set(CONTACT_KEY, null);
}

/**
 * Sends the address to the server (a confirmation link is mailed to it).
 * Returns the server's answer: {status, masked, confirmation_sent, note}.
 */
export async function submitAddress(address) {
	const have = localContact();
	const res = await api.notifySaveContact({ address, contact_id: have ? have.id : 0, contact_secret: have ? have.secret : "" }).catch(async (err) => {
		// A contact the server no longer knows (deleted, wrong secret): start again.
		if (have && err instanceof ApiError && err.status === 403) {
			saveLocalContact(null);
			return api.notifySaveContact({ address });
		}
		throw err;
	});
	saveLocalContact({ id: res.contact_id, secret: res.contact_secret || (have && have.secret) || "", address });
	return res;
}

/** The server's view of this browser's contact, or null when there is none. */
export async function contactStatus() {
	const c = localContact();
	if (!c) return null;
	try {
		return await api.notifyContactStatus(c.id, c.secret);
	} catch (err) {
		// A wrong secret: the contact is not this browser's any more. (A guest
		// refused notifications altogether keeps it: it is how the address
		// becomes an account, ADR 0073.)
		if (err instanceof ApiError && err.status === 403 && !/needs "notify"/.test(err.message)) {
			saveLocalContact(null);
			return null;
		}
		if (err instanceof ApiError && err.status === 403) return null;
		throw err;
	}
}

export async function resendConfirmation() {
	const c = localContact();
	if (!c) return null;
	return api.notifyResend(c.id, c.secret);
}

/** Deletes the address from the server (and every game setting that used it). */
export async function forgetAddress() {
	const c = localContact();
	if (!c) return;
	try {
		await api.notifyForget(c.id, c.secret);
	} catch (err) {
		if (!(err instanceof ApiError && err.status === 403)) throw err;
	}
	saveLocalContact(null);
}

/** One line about the state of the address. */
export function statusText(info) {
	if (!info) return "No address set.";
	switch (info.status) {
		case "verified":
			return `Confirmed: ${info.masked}.`;
		case "stopped":
			return `${info.masked} used the unsubscribe link: Bingle will not email it.`;
		default:
			return `Waiting for you to confirm ${info.masked}: open the link in the email we sent (look in the spam folder too).`;
	}
}

/**
 * Tells the player where the mail will come from, so they can allow it. `from`
 * is the sender address the server is configured with.
 */
export function whitelistNote(from) {
	return h(
		"p",
		{ class: "small notify-note", id: "notify-whitelist" },
		"Email notifications are sent from ",
		h("strong", { class: "notify-from" }, from),
		". Add that address to your contacts or safe-sender list (and mark it “not spam” if it lands there) so that the messages reach your inbox."
	);
}

/** A warning when the page is not on a secure connection. */
export function insecureNote() {
	if (location.protocol === "https:" || location.hostname === "localhost" || location.hostname === "127.0.0.1") return null;
	return h("p", { class: "small error", id: "notify-insecure" }, "This connection is not encrypted: your email address could be seen by others on the network. Use an https:// address.");
}

// ---- one game ---------------------------------------------------------------------

/**
 * Builds the "email me when it is my turn" setting of one game into host. The
 * choice belongs to the seat: it needs the seat's secret, which only the browser
 * that took the seat has. `onReady(false)` says there is nothing to show.
 */
export async function mountGameNotify(host, { code, name, onReady = () => {} }) {
	const cfg = await notifyConfig();
	if (!cfg.enabled) {
		onReady(false);
		return;
	}
	onReady(true);
	host.textContent = "";
	// The seat's setting is its owner's: a browser that did not take the seat
	// is told so (403) rather than shown a switch it cannot use.
	let state = null;
	let readErr = null;
	try {
		state = await api.gameNotify(code, name);
	} catch (err) {
		if (err && err.status === 403) {
			host.append(
				h("p", { class: "small muted", id: "notify-no-token" }, "This browser did not take this seat, so it cannot change the email setting of the game. Open the game from the browser where you joined it."),
				whitelistNote(cfg.from)
			);
			return;
		}
		readErr = err;
	}
	const on = h("input", { type: "checkbox", id: "notify-on", role: "switch" });
	const everyTime = h("input", { type: "radio", name: "notify-mode", id: "notify-mode-always", value: "always", checked: true });
	const whenAway = h("input", { type: "radio", name: "notify-mode", id: "notify-mode-away", value: "away" });
	const minutes = h("input", { type: "number", id: "notify-away", min: "1", max: "1440", step: "1", value: "30", "aria-label": "minutes" });
	const hint = h("p", { class: "small", id: "notify-hint", "aria-live": "polite" });
	const controls = h(
		"div",
		{ class: "notify-modes" },
		h("label", { class: "check" }, everyTime, h("span", {}, "Every time it is my turn (not while this game is open in a window)")),
		h("label", { class: "check" }, whenAway, h("span", {}, "Only if I have not moved for "), minutes, h("span", {}, " minutes"))
	);
	host.append(h("label", { class: "check" }, on, h("span", {}, "Email me when it is my turn in this game")), controls, hint, whitelistNote(cfg.from));
	const warn = insecureNote();
	if (warn) host.append(warn);

	// Nothing can be changed until the state of the address and of the game is known.
	for (const el of [on, everyTime, whenAway, minutes]) el.disabled = true;
	hint.textContent = "Loading…";
	let verified = false;
	const paint = (state) => {
		const s = state || {};
		on.checked = !!s.enabled;
		const away = s.away_minutes > 0;
		whenAway.checked = away;
		everyTime.checked = !away;
		if (away) minutes.value = String(s.away_minutes);
		for (const el of [on, everyTime, whenAway, minutes]) el.disabled = !verified;
		minutes.disabled = !verified || !whenAway.checked;
	};
	const info = await contactStatus().catch(() => null);
	verified = !!info && info.status === "verified";
	hint.textContent = "";
	if (!info) {
		hint.append("Set your email address in your ", h("a", { href: "/profile" }, "profile"), " first.");
	} else if (!verified) {
		hint.textContent = statusText(info) + " Then come back here, or turn it on from your profile.";
	} else {
		hint.textContent = `Mail goes to ${info.masked}.`;
	}
	if (readErr) hint.textContent = readErr.message || "The setting could not be read.";
	paint(state);

	const save = async () => {
		const away = whenAway.checked ? Math.max(1, Math.min(1440, Math.floor(Number(minutes.value) || 30))) : 0;
		const c = localContact();
		try {
			state = await api.setGameNotify(code, {
				player_name: name,
				contact_id: c ? c.id : 0,
				contact_secret: c ? c.secret : "",
				enabled: on.checked,
				away_minutes: away,
			});
		} catch (err) {
			hint.textContent = (err && err.message) || "The setting could not be saved.";
			hint.className = "small error";
			on.checked = !!(state && state.enabled);
			return;
		}
		hint.className = "small";
		hint.textContent = state.enabled ? `Saved. Mail goes to ${state.contact ? state.contact.masked : "your address"}.` : "Saved. No email for this game.";
		paint(state);
	};
	for (const el of [on, everyTime, whenAway]) el.addEventListener("change", save);
	minutes.addEventListener("change", save);
}

const applied = new Set();

/**
 * The profile's "email me when it is my turn" switch is the default for the
 * games the player has not chosen for: the first time a game is opened by the
 * browser that took its seat, it is switched on there (once; a choice made in
 * the game's options is never overridden).
 */
export async function applyProfileDefault({ code, name }) {
	const key = String(code).toUpperCase();
	if (applied.has(key)) return;
	applied.add(key);
	if (!getProfile().notify) return;
	const c = localContact();
	if (!c) return;
	const cfg = await notifyConfig();
	if (!cfg.enabled) return;
	try {
		const state = await api.gameNotify(code, name);
		if (state.configured) return;
		const info = await contactStatus();
		if (!info || info.status !== "verified") return;
		await api.setGameNotify(code, { player_name: name, contact_id: c.id, contact_secret: c.secret, enabled: true, away_minutes: 0 });
	} catch (_) {
		/* a nicety: the game's options window has the setting */
	}
}
