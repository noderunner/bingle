// The friends list: a window of its own in the bar at the top of every page, for
// signed-in accounts (ADR 0105). It holds friend requests and invitations to
// games, opens a correspondence with a friend, can put a voice call up inside it
// (js/dm-call.js), invites them to a game and removes them.
//
// What is said is kept by the server in memory only, for a couple of hours, and
// nothing is stored anywhere: it is a way to say "fancy a game?", not a mailbox.
// So a friend who is offline cannot be written to, though they can be invited.
//
// Messages are sent over REST and arrive over the /ws-dm socket, which also
// carries the ringing of a call and its WebRTC messages.

import { h, clear, store, lastName, rememberGame, toast } from "./util.js";
import { api, whoAmI } from "./api.js";
import { notifyHidden } from "./browser-notify.js";
import { gameDefs, loadRows, isFinished } from "./mygames.js";
import { onLeave, setKeepAlive, untracked } from "./nav.js";
import { callPerson, hangUp, toggleMute, callState, onCallChange, ringing, ended, directSignal, inDirectCall, setMyAddress } from "./dm-call.js";
import { t } from "./i18n.js";
import { chatText } from "./chat-text.js";

// The newest message this browser has shown, by the address it was with.
const SEEN_KEY = "bingle.dm.seen";

const D = {
	address: "", // our own address, from the socket
	threads: [], // [{with, name, last_id, last_at, text, count}]
	open: "", // the address whose correspondence is on screen
	openName: "",
	messages: [], // the correspondence on screen
	unread: {}, // address -> how many messages have not been read
	friends: { friends: [], incoming: [], outgoing: [], invites: [] },
	inviting: false, // the choice of a game is on screen
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

/** What waits for this account in the window: unread messages, requests and invitations. */
export function unreadCount() {
	return Object.values(D.unread).reduce((a, b) => a + b, 0) + D.friends.incoming.length + D.friends.invites.length;
}

const friendAt = (address) => D.friends.friends.find((f) => f.address === address) || null;

/** How this account stands with a handle: "friend", "outgoing", "incoming" or "". */
export function friendStateOf(handle) {
	const key = String(handle).replace(/^@/, "").toLowerCase();
	const has = (list) => list.some((f) => f.handle.toLowerCase() === key);
	return has(D.friends.friends) ? "friend" : has(D.friends.outgoing) ? "outgoing" : has(D.friends.incoming) ? "incoming" : "";
}

/** A friend's name in the conversation, a badge like those in every other chat. Clicking it does nothing, for a friend. */
const friendChatBadge = (name) => h("span", { class: "handle-badge", "data-handle": String(name).replace(/^@/, "") }, name);

/** Reads the list again. Anything that shows it is told. */
export async function loadFriends() {
	try {
		if ((await whoAmI()).kind !== "account") return;
		D.friends = await api.friends();
	} catch (_) {
		return; // a guest has none, and the window keeps what it has
	}
	onUnread(unreadCount());
	paintWindow();
	for (const fn of friendListeners) fn();
}

const friendListeners = new Set();
/** Calls fn whenever the list changes; returns what stops it. */
export function onFriendsChange(fn) {
	friendListeners.add(fn);
	return () => friendListeners.delete(fn);
}

/** Asks somebody to be a friend, and says how that went. */
export async function askFriend(handle) {
	try {
		await api.friendRequest(handle);
		toast(t("friend.sent", { handle: String(handle).replace(/^@/, "") }));
		return true;
	} catch (err) {
		toast(err.message || t("friend.not_sent"), "error");
		return false;
	} finally {
		await loadFriends();
	}
}

/** One of the window's actions: what it says on failure, and the list read again after it. */
async function friendAction(act) {
	try {
		await act();
	} catch (err) {
		toast(err.message || t("common.not_done"), "error");
	}
	await loadFriends();
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
	D.inviting = false;
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
	const friend = friendAt(address);
	if (friend) return friend.name;
	const thread = D.threads.find((x) => x.with === address);
	return (thread && thread.name) || t("dm.gone");
}

/** Sends what is in the box. */
async function send(text) {
	if (!D.open || !text.trim()) return;
	try {
		const msg = await api.sendDirect(D.open, text);
		receive(msg);
	} catch (err) {
		toast(err.message || t("dm.not_sent"), "error");
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
			case "dm-translation": {
				// A message's translation, which follows it (ADR 0143).
				const m = D.messages.find((x) => x.id === d.id);
				if (m) {
					m.translation = { lang: d.lang, from: d.from, text: d.text };
					paintWindow();
				}
				break;
			}
			case "friends":
				loadFriends();
				break;
			case "dm-call":
				if (d.kind === "ring") {
					ringing(d.from, d.name);
					if (!D.open) openThread(d.from, d.name);
					toast(t("dm.is_calling", { name: d.name || t("dm.somebody") }));
				} else if (d.kind === "end") {
					ended(d.from, d.dropped, d.declined);
				}
				paintWindow();
				break;
			case "dm-rtc":
				directSignal(d);
				break;
			case "badge":
				announceBadge(d);
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

// A badge the account just earned (ADR 0130): a toast on the page, and a
// notification from the browser when the page is out of sight and they are on
// (browser-notify.js). Never an email.
function announceBadge(d) {
	const later = d.hidden ? " " + t("badge.later") : "";
	toast(`${t("badge.earned", { name: d.name })} ${d.description}${later}`, "long");
	notifyHidden(t("badge.earned", { name: d.name }), { body: d.description, tag: "bingle-badge-" + d.badge });
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
	const addInput = h("input", { type: "text", id: "friend-handle", maxlength: "26", placeholder: t("dm.add_placeholder"), "aria-label": t("dm.add_label"), autocapitalize: "none", autocomplete: "off", spellcheck: "false" });
	const addForm = h("form", { class: "row dm-send-row", id: "friend-add" }, addInput, h("button", { type: "submit", id: "friend-add-go", class: "primary" }, t("dm.add")));
	const requests = h("div", { id: "friend-requests" });
	const invites = h("div", { id: "friend-invites" });
	const people = h("ul", { class: "here-list", id: "friends-list" });
	const waiting = h("div", { id: "friend-outgoing" });
	const list = h("div", { class: "dm-people-pane" }, addForm, requests, invites, h("h3", {}, t("bar.friends")), people, waiting);
	const log = h("ol", { class: "chat-log", id: "dm-log", "aria-live": "polite", "aria-label": t("dm.conversation") });
	const title = h("h3", { class: "dm-title", id: "dm-title" }, t("dm.your_friends"));
	const input = h("input", { type: "text", id: "dm-input", maxlength: "500", placeholder: t("dm.write"), "aria-label": t("dm.your_message"), autocomplete: "off" });
	const sendBtn = h("button", { type: "button", id: "dm-send", class: "primary" }, t("dm.send"));
	const callBtn = h("button", { type: "button", id: "dm-call" }, t("dm.call"));
	const muteBtn = h("button", { type: "button", id: "dm-mute", hidden: true }, t("voice.mute"));
	const hangBtn = h("button", { type: "button", id: "dm-hangup", class: "danger", hidden: true }, t("dm.hang_up"));
	const callState_ = h("span", { class: "small muted", id: "dm-call-state" });
	const inviteBtn = h("button", { type: "button", id: "friend-invite" }, t("dm.invite"));
	const removeBtn = h("button", { type: "button", id: "friend-remove", class: "ghost" }, t("dm.remove"));
	const blockBtn = h("button", { type: "button", id: "friend-block", class: "ghost" }, t("dm.block"));
	const inviteGame = h("select", { id: "friend-invite-game", "aria-label": t("dm.invite_game") });
	const inviteSend = h("button", { type: "button", id: "friend-invite-send", class: "primary" }, t("dm.send_invite"));
	const inviteRow = h("div", { class: "row dm-send-row", id: "friend-invite-row", hidden: true }, inviteGame, inviteSend);
	const offline = h("p", { class: "small muted", id: "dm-offline", hidden: true });
	const thread = h(
		"div",
		{ class: "dm-thread-pane", id: "dm-thread" },
		title,
		h("div", { class: "row dm-call-row" }, callBtn, muteBtn, hangBtn, inviteBtn, removeBtn, blockBtn, callState_),
		inviteRow,
		log,
		offline,
		h("div", { class: "row dm-send-row" }, input, sendBtn),
		h("p", { class: "small muted" }, t("dm.kept"))
	);
	host.append(h("div", { class: "dm-panes" }, list, thread));

	// Blocking (ADR 0156) is silent and lifted in the profile; it asks first, as removing does.
	const block = (handle, name) => {
		if (!window.confirm(t("dm.block_confirm", { name }))) return;
		friendAction(() => api.friendBlock(handle));
	};
	const blockButton = (handle, name) => h("button", { type: "button", class: "ghost small", "data-block": handle, onclick: () => block(handle, name) }, t("dm.block"));

	const section = (host_, heading, rows) => {
		clear(host_);
		if (!rows.length) return;
		host_.append(h("h3", {}, heading), h("ul", { class: "here-list" }, ...rows));
	};

	const drawPeople = () => {
		const F = D.friends;
		section(
			requests,
			t("dm.requests"),
			F.incoming.map((f) =>
				h(
					"li",
					{ "data-handle": f.handle },
					h("span", { class: "grow" }, f.name),
					h("button", { type: "button", class: "primary small", "data-accept": f.handle, onclick: () => friendAction(() => api.friendAnswer(f.handle, true)) }, t("dm.accept")),
					h("button", { type: "button", class: "ghost small", "data-decline": f.handle, onclick: () => friendAction(() => api.friendAnswer(f.handle, false)) }, t("dm.decline")),
					blockButton(f.handle, f.name)
				)
			)
		);
		section(
			invites,
			t("dm.invitations"),
			F.invites.map((g) =>
				h(
					"li",
					{ "data-room": g.room },
					h("span", { class: "grow" }, g.notice ? `${g.game}: ${g.notice}` : `${g.game} · ${g.from}`),
					g.notice
						? null
						: h(
								"a",
								{
									class: "button primary small",
									href: "/game/" + encodeURIComponent(g.room),
									"data-join-invite": g.room,
									onclick: () => {
										rememberGame(g.room, g.def, lastName());
										api.answerInvite(g.room, true).catch(() => {});
									},
								},
								t("dm.accept")
						  ),
					h("button", { type: "button", class: "ghost small", "data-dismiss": g.room, onclick: () => friendAction(() => (g.notice ? api.friendDismiss(g.room) : api.answerInvite(g.room, false))) }, g.notice ? t("dm.dismiss") : t("dm.decline")),
					g.notice ? null : blockButton(g.from.slice(1), g.from)
				)
			)
		);
		section(
			waiting,
			t("dm.waiting_answer"),
			F.outgoing.map((f) =>
				h(
					"li",
					{ "data-handle": f.handle },
					h("span", { class: "grow" }, f.name),
					h("button", { type: "button", class: "ghost small", "data-cancel": f.handle, onclick: () => friendAction(() => api.friendRemove(f.handle)) }, t("dm.cancel"))
				)
			)
		);
		clear(people);
		// Those online first, then by name, which is how the list arrives.
		for (const f of [...F.friends].sort((a, b) => Number(b.online) - Number(a.online))) {
			const n = D.unread[f.address] || 0;
			people.append(
				h(
					"li",
					{ "data-address": f.address, class: f.address === D.open ? "on" : null },
					h(
						"button",
						{ type: "button", class: "dm-person grow", "data-address": f.address, onclick: () => openThread(f.address, f.name) },
						h("span", { class: "friend-dot" + (f.online ? " online" : ""), title: f.online ? t("dm.online") : t("dm.offline") }),
						h("span", { class: "grow" }, f.name),
						n ? h("span", { class: "badge turn", "data-unread": n }, String(n)) : null
					)
				)
			);
		}
		if (!F.friends.length) people.append(h("li", { class: "muted small" }, t("dm.add_somebody")));
	};

	const drawThread = () => {
		const call = callState();
		const friend = friendAt(D.open);
		title.textContent = D.open ? t("dm.with", { name: D.openName }) : t("dm.your_friends");
		thread.classList.toggle("empty", !D.open);
		const online = !!friend && friend.can_write;
		input.disabled = !online;
		sendBtn.disabled = !online;
		offline.hidden = !D.open || online;
		offline.textContent = friend ? t("dm.is_offline", { name: friend.name }) : t("dm.not_friends");
		inviteBtn.hidden = !friend;
		removeBtn.hidden = !friend;
		blockBtn.hidden = !friend;
		inviteRow.hidden = !friend || !D.inviting;
		const busy = call.state !== "" && call.peer === D.open;
		// The button rings somebody, or answers them while they are ringing;
		// a call that is up or being set up leaves nothing to press.
		callBtn.hidden = !online || (busy && call.state !== "ringing");
		callBtn.textContent = busy && call.state === "ringing" ? t("dm.answer") : t("dm.call");
		muteBtn.hidden = !busy || call.state !== "live";
		muteBtn.textContent = call.muted ? t("voice.unmute") : t("voice.mute");
		muteBtn.setAttribute("aria-pressed", call.muted ? "true" : "false");
		hangBtn.hidden = !busy;
		callState_.textContent = call.error
			? call.error
			: !busy
				? ""
				: call.state === "calling"
					? t("dm.ringing")
					: call.state === "ringing"
						? t("dm.is_calling_now", { name: call.name || t("dm.somebody") })
						: t("dm.in_call");
		clear(log);
		if (!D.open) {
			log.append(h("li", { class: "chat-empty muted small" }, t("dm.pick")));
			return;
		}
		if (!D.messages.length) log.append(h("li", { class: "chat-empty muted small" }, t("dm.nothing_yet")));
		for (const m of D.messages) {
			const mine = m.from === D.address;
			log.append(
				h(
					"li",
					{ class: "chat-line" + (mine ? " mine" : ""), "data-id": m.id },
					mine ? h("span", { class: "chat-who" }, t("dm.you")) : friendChatBadge(m.name),
					...chatText(m, "d")
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
	addForm.addEventListener("submit", async (ev) => {
		ev.preventDefault();
		const handle = addInput.value.trim();
		if (handle && (await askFriend(handle))) addInput.value = "";
	});
	// The games to invite to are this player's own that are not over, the one on screen first.
	inviteBtn.addEventListener("click", async () => {
		D.inviting = !D.inviting;
		if (D.inviting) {
			clear(inviteGame);
			const here = document.body.dataset.roomCode || "";
			const [rows, defs] = await Promise.all([loadRows().catch(() => []), gameDefs().catch(() => [])]);
			const open = rows.filter((r) => r.info && !isFinished(r));
			open.sort((a, b) => Number(b.g.code === here) - Number(a.g.code === here));
			for (const r of open) {
				const def = defs.find((x) => x.id === String(r.info.game_def_id).split("~")[0]);
				inviteGame.append(h("option", { value: r.g.code }, `${r.info.list_title || (def ? def.name : r.info.game_def_id)} · ${r.g.code}`));
			}
			if (!open.length) inviteGame.append(h("option", { value: "" }, t("dm.no_game")));
			inviteSend.disabled = !open.length;
		}
		paintWindow();
	});
	inviteSend.addEventListener("click", async () => {
		const friend = friendAt(D.open);
		if (!friend || !inviteGame.value) return;
		try {
			await api.friendInvite(friend.handle, inviteGame.value);
			toast(t("dm.invited", { name: friend.name }));
			D.inviting = false;
			paintWindow();
		} catch (err) {
			toast(err.message || t("dm.invite_failed"), "error");
		}
	});
	removeBtn.addEventListener("click", () => {
		const friend = friendAt(D.open);
		if (!friend || !window.confirm(t("dm.remove_confirm", { name: friend.name }))) return;
		D.open = "";
		D.messages = [];
		D.inviting = false;
		friendAction(() => api.friendRemove(friend.handle));
	});
	blockBtn.addEventListener("click", () => {
		const friend = friendAt(D.open);
		if (!friend || !window.confirm(t("dm.block_confirm", { name: friend.name }))) return;
		D.open = "";
		D.messages = [];
		D.inviting = false;
		friendAction(() => api.friendBlock(friend.handle));
	});
	callBtn.addEventListener("click", () => callPerson(D.open, D.openName));
	muteBtn.addEventListener("click", toggleMute);
	hangBtn.addEventListener("click", () => hangUp());
	const stopCall = onCallChange(paintWindow);
	onLeave(() => {
		stopCall();
		paintWindow = () => {};
	});
	paintWindow();
	return {
		/** The window was opened: read the list again. */
		refresh: () => Promise.all([loadThreads(), loadFriends()]),
		/** Opens the correspondence with one person. */
		open: openThread,
	};
}

/** Starts listening for messages (once per page, from the top bar). */
export function startDirectMessages() {
	openSocket();
	loadThreads();
	loadFriends();
}
