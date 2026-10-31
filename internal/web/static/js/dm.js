// Direct messages: a window of its own in the bar at the top of every page.
// It lists the people who are here (js/here.js), opens a correspondence with
// one of them, and can put a voice call up inside it (js/dm-call.js).
//
// What is said is kept by the server in memory only, for a couple of hours, and
// nothing is stored anywhere: it is a way to say "fancy a game?", not a mailbox.
// Direct messages are between signed-in accounts (ADR 0073), so a message is
// from the handle it says.
//
// Messages are sent over REST and arrive over the /ws-dm socket, which also
// carries the ringing of a call and its WebRTC messages.

import { h, clear, store, lastName, toast } from "./util.js";
import { api, whoAmI } from "./api.js";
import { whoIsHere, onHereChange } from "./here.js";
import { onLeave, setKeepAlive, untracked } from "./nav.js";
import { callPerson, hangUp, toggleMute, callState, onCallChange, ringing, ended, directSignal, inDirectCall, setMyAddress } from "./dm-call.js";

// The newest message this browser has shown, by the address it was with.
const SEEN_KEY = "bingle.dm.seen";

const D = {
	address: "", // our own address, from the socket
	threads: [], // [{with, name, last_id, last_at, text, count}]
	open: "", // the address whose correspondence is on screen
	openName: "",
	messages: [], // the correspondence on screen
	unread: {}, // address -> how many messages have not been read
};

let sock = null;
let sockTimer = null;
let sockTries = 0;
let paintWindow = () => {};
let onUnread = () => {};

const seen = () => {
	const m = store.get(SEEN_KEY, {});
	return m && typeof m === "object" ? m : {};
};
const markSeen = (addr, id) => {
	if (!addr || !id) return;
	const all = seen();
	if ((all[addr] || 0) >= id) return;
	all[addr] = id;
	store.set(SEEN_KEY, all);
};

/** How many messages in all have not been read. */
export function unreadCount() {
	return Object.values(D.unread).reduce((a, b) => a + b, 0);
}

function countUnread() {
	const all = seen();
	D.unread = {};
	for (const t of D.threads) {
		if (t.with === D.open) continue; // on screen: read as it arrives
		if (t.last_id > (all[t.with] || 0)) D.unread[t.with] = t.count;
	}
	onUnread(unreadCount());
}

async function loadThreads() {
	try {
		const res = await api.directThreads();
		D.threads = (res && res.threads) || [];
		D.address = (res && res.address) || D.address;
		setMyAddress(D.address);
		countUnread();
		paintWindow();
	} catch (_) {
		/* the window works without the list */
	}
}

async function openThread(address, name) {
	D.open = address;
	D.openName = name || nameOf(address);
	D.messages = [];
	paintWindow();
	try {
		const res = await api.directHistory(address, 0);
		D.messages = (res && res.messages) || [];
		if (D.messages.length) markSeen(address, D.messages[D.messages.length - 1].id);
		delete D.unread[address];
		onUnread(unreadCount());
	} catch (_) {
		/* nothing said yet */
	}
	paintWindow();
}

function nameOf(address) {
	const here = whoIsHere().find((p) => p.address === address);
	if (here) return here.name;
	const thread = D.threads.find((t) => t.with === address);
	return (thread && thread.name) || "somebody who has gone";
}

/** Sends what is in the box. */
async function send(text) {
	if (!D.open || !text.trim()) return;
	try {
		const msg = await api.sendDirect(D.open, text);
		receive(msg);
	} catch (err) {
		toast(err.message || "The message was not sent.", "error");
	}
}

/** One message, from the socket or from sending it. */
function receive(msg) {
	if (!msg || !msg.id) return;
	const other = msg.from === D.address ? msg.to : msg.from;
	const t = D.threads.find((x) => x.with === other);
	if (t) {
		t.last_id = msg.id;
		t.last_at = msg.at;
		t.text = msg.text;
		t.count += 1;
		if (msg.from === other && msg.name) t.name = msg.name;
	} else {
		D.threads.unshift({ with: other, name: msg.from === other ? msg.name : "", last_id: msg.id, last_at: msg.at, text: msg.text, count: 1 });
	}
	D.threads.sort((a, b) => b.last_id - a.last_id);
	if (other === D.open) {
		if (!D.messages.some((m) => m.id === msg.id)) D.messages.push(msg);
		markSeen(other, msg.id);
		delete D.unread[other];
		onUnread(unreadCount());
	} else if (msg.from !== D.address) {
		D.unread[other] = (D.unread[other] || 0) + 1;
		onUnread(unreadCount());
	}
	paintWindow();
}

// ---- the socket -------------------------------------------------------------

async function openSocket() {
	closeSocket();
	// Direct messages are an account's (ADR 0073): a guest has no socket.
	const me = await whoAmI();
	if (me.kind !== "account" || !(me.caps || []).includes("dm")) return;
	const proto = location.protocol === "https:" ? "wss://" : "ws://";
	let ws;
	try {
		ws = new WebSocket(`${proto}${location.host}/ws-dm`);
	} catch (_) {
		return;
	}
	sock = ws;
	ws.addEventListener("open", () => {
		if (sock !== ws) return;
		sockTries = 0;
		loadThreads();
	});
	ws.addEventListener("message", (ev) => {
		if (sock !== ws) return;
		let env;
		try {
			env = JSON.parse(ev.data);
		} catch (_) {
			return;
		}
		const d = env.data || {};
		switch (env.type) {
			case "ready":
				D.address = d.address || D.address;
				setMyAddress(D.address);
				break;
			case "dm":
				receive(d);
				break;
			case "dm-call":
				if (d.kind === "ring") {
					ringing(d.from, d.name);
					if (!D.open) openThread(d.from, d.name);
					toast(`${d.name || "Somebody"} is calling.`);
				} else if (d.kind === "end") {
					ended(d.from, d.dropped);
				}
				paintWindow();
				break;
			case "dm-rtc":
				directSignal(d);
				break;
		}
	});
	ws.addEventListener("close", () => {
		if (sock !== ws) return;
		sock = null;
		sockTries = Math.min(sockTries + 1, 6);
		sockTimer = window.setTimeout(() => untracked(openSocket), 700 * 2 ** sockTries);
	});
}

function closeSocket() {
	if (sockTimer) window.clearTimeout(sockTimer);
	sockTimer = null;
	const old = sock;
	sock = null;
	if (old) {
		try {
			old.close();
		} catch (_) {
			/* already closed */
		}
	}
}

// A direct call outlives the page, as a game's call does (nav.js).
setKeepAlive(inDirectCall);

/**
 * Builds the window's contents into host. onCount(n) is told how many messages
 * have not been read, for the badge on the icon.
 */
export function mountDirectMessages(host, { onCount = () => {} } = {}) {
	onUnread = onCount;
	const people = h("ul", { class: "here-list", id: "dm-people" });
	const list = h("div", { class: "dm-people-pane" }, h("h3", {}, "Here now"), people);
	const log = h("ol", { class: "chat-log", id: "dm-log", "aria-live": "polite", "aria-label": "This conversation" });
	const title = h("h3", { class: "dm-title", id: "dm-title" }, "Direct messages");
	const input = h("input", { type: "text", id: "dm-input", maxlength: "500", placeholder: "Write a message…", "aria-label": "Your message", autocomplete: "off" });
	const sendBtn = h("button", { type: "button", id: "dm-send", class: "primary" }, "Send");
	const callBtn = h("button", { type: "button", id: "dm-call" }, "Call");
	const muteBtn = h("button", { type: "button", id: "dm-mute", hidden: true }, "Mute");
	const hangBtn = h("button", { type: "button", id: "dm-hangup", class: "danger", hidden: true }, "Hang up");
	const callState_ = h("span", { class: "small muted", id: "dm-call-state" });
	const thread = h(
		"div",
		{ class: "dm-thread-pane", id: "dm-thread" },
		title,
		h("div", { class: "row dm-call-row" }, callBtn, muteBtn, hangBtn, callState_),
		log,
		h("div", { class: "row dm-send-row" }, input, sendBtn),
		h("p", { class: "small muted" }, "Kept for a couple of hours on the server and nowhere else. A message is from the signed-in account whose handle it shows.")
	);
	host.append(h("div", { class: "dm-panes" }, list, thread));

	const drawPeople = () => {
		clear(people);
		const mine = D.address;
		const here = whoIsHere().filter((p) => p.address !== mine);
		// Everybody who is here, and anybody written to who has since gone.
		const rows = [...here.map((p) => ({ address: p.address, name: p.name, here: true }))];
		for (const t of D.threads) {
			if (!rows.some((r) => r.address === t.with)) rows.push({ address: t.with, name: t.name || "somebody who has gone", here: false });
		}
		for (const r of rows) {
			const n = D.unread[r.address] || 0;
			people.append(
				h(
					"li",
					{ "data-address": r.address, class: r.address === D.open ? "on" : null },
					h(
						"button",
						{ type: "button", class: "dm-person grow", "data-address": r.address, onclick: () => openThread(r.address, r.name) },
						h("span", { class: "grow" }, r.name),
						n ? h("span", { class: "badge turn", "data-unread": n }, String(n)) : null,
						r.here ? null : h("span", { class: "badge" }, "gone")
					)
				)
			);
		}
		if (!rows.length) people.append(h("li", { class: "muted" }, "Nobody else is here just now."));
	};

	const drawThread = () => {
		const call = callState();
		title.textContent = D.open ? `With ${D.openName}` : "Direct messages";
		thread.classList.toggle("empty", !D.open);
		input.disabled = !D.open;
		sendBtn.disabled = !D.open;
		const busy = call.state !== "" && call.peer === D.open;
		// The button rings somebody, or answers them while they are ringing;
		// a call that is up or being set up leaves nothing to press.
		callBtn.hidden = !D.open || (busy && call.state !== "ringing");
		callBtn.textContent = busy && call.state === "ringing" ? "Answer" : "Call";
		muteBtn.hidden = !busy || call.state !== "live";
		muteBtn.textContent = call.muted ? "Unmute" : "Mute";
		muteBtn.setAttribute("aria-pressed", call.muted ? "true" : "false");
		hangBtn.hidden = !busy;
		callState_.textContent = call.error
			? call.error
			: !busy
				? ""
				: call.state === "calling"
					? "Ringing…"
					: call.state === "ringing"
						? `${call.name || "Somebody"} is calling`
						: "In the call";
		clear(log);
		if (!D.open) {
			log.append(h("li", { class: "chat-empty muted small" }, "Pick somebody on the left to write to them."));
			return;
		}
		if (!D.messages.length) log.append(h("li", { class: "chat-empty muted small" }, "Nothing said yet."));
		for (const m of D.messages) {
			const mine = m.from === D.address;
			log.append(
				h(
					"li",
					{ class: "chat-line" + (mine ? " mine" : ""), "data-id": m.id },
					h("span", { class: "chat-who" }, mine ? "you" : m.name),
					h("span", { class: "chat-text" }, m.text)
				)
			);
		}
		log.scrollTop = log.scrollHeight;
	};

	paintWindow = () => {
		drawPeople();
		drawThread();
	};
	sendBtn.addEventListener("click", () => {
		const text = input.value;
		input.value = "";
		send(text);
	});
	input.addEventListener("keydown", (ev) => {
		if (ev.key === "Enter" && !ev.shiftKey) {
			ev.preventDefault();
			sendBtn.click();
		}
	});
	callBtn.addEventListener("click", () => callPerson(D.open, D.openName));
	muteBtn.addEventListener("click", toggleMute);
	hangBtn.addEventListener("click", () => hangUp());
	const stopHere = onHereChange(paintWindow);
	const stopCall = onCallChange(paintWindow);
	onLeave(() => {
		stopHere();
		stopCall();
		paintWindow = () => {};
	});
	paintWindow();
	return {
		/** The window was opened: read the list again. */
		refresh: loadThreads,
		/** Opens the correspondence with one person. */
		open: openThread,
	};
}

/** Starts listening for messages (once per page, from the top bar). */
export function startDirectMessages() {
	openSocket();
	loadThreads();
}
