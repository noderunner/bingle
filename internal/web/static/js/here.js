// Who is here: this browser tells the server, every minute, that somebody is
// looking at bingle (under the name they go by), and keeps the list of everyone
// else who has said so lately. The profile window lists them.
//
// A browser says who it is with a secret of its own (util.js sessionSecret),
// sent to the server and to nobody else; what the list carries is the address
// that secret hashes to, which is where a direct message to that person goes.
// Names are self-chosen — there are no accounts — so the window says so.

import { api } from "./api.js";
import { lastName, sessionSecret } from "./util.js";

// How often this browser says it is still here (the server forgets a browser
// after two and a half minutes) — and how often while a window that shows the
// list is open, where somebody arriving should appear at once.
const BEAT_MS = 60000;
const WATCH_MS = 8000;

let people = [];
let myAddress = "";
let timer = null;
let watching = 0; // how many open windows are showing the list
const listeners = new Set();

/** Everybody who is here now, the longest-standing first. */
export function whoIsHere() {
	return people;
}

/** The address of this browser, once the server has been told we are here. */
export function myHereAddress() {
	return myAddress;
}

/** Calls fn whenever the list changes; returns a function that stops it. */
export function onHereChange(fn) {
	listeners.add(fn);
	return () => listeners.delete(fn);
}

function announce() {
	for (const fn of listeners) {
		try {
			fn(people);
		} catch (err) {
			console.error("who-is-here listener failed:", err);
		}
	}
}

/** Says we are here (and reads the list), now and every minute. */
export async function beat() {
	const name = lastName();
	try {
		if (!name) {
			// Nobody has given a name in this browser yet: read the list, but
			// do not put an unnamed entry in it.
			const res = await api.whoIsHere();
			people = (res && res.people) || [];
		} else {
			const res = await api.here(sessionSecret(), name);
			people = (res && res.people) || [];
			myAddress = (res && res.address) || myAddress;
		}
		announce();
	} catch (_) {
		/* the page works without the list */
	}
}

function arm() {
	if (timer) clearInterval(timer);
	timer = setInterval(() => {
		if (!document.hidden) beat();
	}, watching > 0 ? WATCH_MS : BEAT_MS);
}

/**
 * Starts saying that somebody is here, and keeps it up while the tab lives. It
 * is called once per page by the top bar.
 */
export function startHere() {
	beat();
	if (!timer) {
		arm();
		document.addEventListener("visibilitychange", () => {
			if (!document.hidden) beat();
		});
	}
}

/**
 * While a window that shows who is here is open, the list is read more often,
 * so that somebody arriving or leaving shows up while it is being looked at.
 * Every opener must call it again with false when it closes.
 */
export function watchHere(on) {
	watching = Math.max(0, watching + (on ? 1 : -1));
	if (on) beat();
	arm();
}

/** The name this browser goes by, as the list shows it. */
export function myName() {
	return lastName();
}
