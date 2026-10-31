// Who is here: a signed-in account tells the server, every minute, that it is
// looking at bingle, and keeps the list of every other account that has said
// so lately. The profile window lists them. Guests are not listed and see no
// list: who is here is who an account can write to (ADR 0073).
//
// The list carries each account's handle and its address, which is where a
// direct message to it goes.

import { api, whoAmI } from "./api.js";

// How often this browser says it is still here (the server forgets a browser
// after two and a half minutes) — and how often while a window that shows the
// list is open, where somebody arriving should appear at once.
const BEAT_MS = 60000;
const WATCH_MS = 8000;

let people = [];
let myAddress = "";
let myHandle = "";
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
	try {
		const me = await whoAmI();
		if (me.kind !== "account" || !(me.caps || []).includes("dm")) {
			people = [];
		} else {
			myHandle = me.handle;
			const res = await api.here();
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

/** The name this browser goes by, as the list shows it: its account's handle. */
export function myName() {
	return myHandle ? "@" + myHandle : "";
}
