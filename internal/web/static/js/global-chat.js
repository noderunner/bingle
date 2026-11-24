// The global chat room of the home page: one room everybody who opens the page
// can read and write in, like the table talk of a game but without a game.
// New messages arrive over a WebSocket (/ws-chat); the page keeps the draft and
// counts the messages the person has not read — those that came in while
// another tab was showing, and those said while this browser was away, as the
// last message it has shown is remembered (localStorage bingle.chat.seen).

import { handleBadge } from "./friend-badge.js";
import { h, store, lastName } from "./util.js";
import { api, whoAmI } from "./api.js";
import { emojiPicker } from "./chat.js";
import { chatText } from "./chat-text.js";
import { t, lang } from "./i18n.js";

const MAX_LINES = 300;

// The id of the newest message this browser has shown its person.
const SEEN_KEY = "bingle.chat.seen";

function lastSeenID() {
	const v = store.get(SEEN_KEY, 0);
	return typeof v === "number" ? v : 0;
}

function clock(iso) {
	// SQLite stores "YYYY-MM-DD HH:MM:SS" in UTC.
	const at = Date.parse(String(iso || "").replace(" ", "T") + (/[zZ]|[+-]\d\d:?\d\d$/.test(iso || "") ? "" : "Z"));
	return Number.isNaN(at) ? "" : new Date(at).toLocaleTimeString(lang, { hour: "2-digit", minute: "2-digit" });
}

/**
 * Builds the chat into host. `onUnread(n)` is told how many messages came in
 * while `visible()` was false (the chat tab was not showing).
 */
export function mountGlobalChat(host, { visible = () => true, onUnread = () => {} } = {}) {
	const lines = [];
	let unread = 0;
	let sending = false;
	let seenID = lastSeenID();

	// Everything on screen has been read, up to the newest line.
	const markSeen = () => {
		if (!lines.length) return;
		const newest = lines[lines.length - 1].id;
		if (newest > seenID) {
			seenID = newest;
			store.set(SEEN_KEY, newest);
		}
	};

	const log = h("ol", { class: "chat-log", id: "global-chat-log", "aria-live": "polite", "aria-label": t("chat.messages") });
	const nameInput = h("input", {
		type: "text",
		id: "global-chat-name",
		class: "chat-name",
		placeholder: t("chat.your_name"),
		maxlength: "32",
		"aria-label": t("chat.name_label"),
		value: store.get("bingle.chatname", "") || lastName() || "",
		autocomplete: "nickname",
	});
	nameInput.addEventListener("input", () => store.set("bingle.chatname", nameInput.value.trim()));
	const input = h("input", {
		type: "text",
		id: "global-chat-input",
		class: "chat-input",
		placeholder: t("chat.say_everyone"),
		maxlength: "500",
		autocomplete: "off",
		"aria-label": t("chat.message"),
	});
	const notice = h("p", { class: "small error chat-notice", id: "global-chat-notice", role: "status" });
	const sendBtn = h("button", { type: "button", id: "global-chat-send", class: "primary" }, t("dm.send"));
	const picker = emojiPicker(input);
	const row = h("div", { class: "chat-entry" }, nameInput, picker.button, input, sendBtn);
	// Everybody reads the room; an account writes in it, under its handle
	// (ADR 0073).
	whoAmI().then((me) => {
		if (!(me.caps || []).includes("chat_global")) {
			input.disabled = sendBtn.disabled = nameInput.disabled = true;
			input.placeholder = t("chat.sign_in");
			return;
		}
		if (me.kind === "account") {
			nameInput.value = "@" + me.handle;
			nameInput.readOnly = true;
		}
	});

	const lineNode = (line) => {
		const mine = nameInput.value.trim() && line.name.toLowerCase() === nameInput.value.trim().toLowerCase();
		return h(
			"li",
			{ class: "chat-line" + (mine ? " mine" : ""), "data-id": line.id },
			String(line.name).startsWith("@") ? handleBadge(line.name) : h("span", { class: "chat-who" }, line.name),
			...chatText(line, "w"),
			h("time", { class: "chat-time muted small" }, clock(line.at))
		);
	};
	const draw = () => {
		log.textContent = "";
		if (!lines.length) log.append(h("li", { class: "chat-empty muted small" }, t("chat.nobody")));
		for (const l of lines) log.append(lineNode(l));
		log.scrollTop = log.scrollHeight;
	};
	const receive = (line, mine = false, quiet = false) => {
		if (!line || lines.some((l) => l.id === line.id)) return;
		lines.push(line);
		if (lines.length > MAX_LINES) lines.splice(0, lines.length - MAX_LINES);
		const atEnd = log.scrollHeight - log.scrollTop - log.clientHeight < 40;
		log.querySelector(".chat-empty")?.remove();
		log.append(lineNode(line));
		if (atEnd || mine) log.scrollTop = log.scrollHeight;
		const read = visible() && !quiet;
		// A line the person wrote themselves is read, even one read back out of
		// the history in a later visit.
		const own = nameInput.value.trim();
		if (own && line.name && line.name.toLowerCase() === own.toLowerCase()) mine = true;
		if (!mine && !read) {
			// Unread: either it came while another tab was showing, or it was
			// said before this browser last looked (history, quiet) and is
			// newer than the last message shown.
			if (!quiet || line.id > seenID) onUnread(++unread);
		} else if (read) {
			markSeen();
		}
	};
	// A line's translation, which follows the line (ADR 0143): drawn in its place.
	const translated = (d) => {
		const line = d && lines.find((l) => l.id === d.id);
		if (!line) return;
		line.translation = { lang: d.lang, from: d.from, text: d.text };
		log.querySelector(`li[data-id="${Number(d.id)}"]`)?.replaceWith(lineNode(line));
	};
	const say = async () => {
		const text = input.value.trim();
		const name = nameInput.value.trim();
		if (!text || sending) return;
		if (!name) {
			nameInput.focus();
			return;
		}
		sending = true;
		sendBtn.disabled = true;
		try {
			const line = await api.sayGlobal(name, text);
			input.value = "";
			markSeen();
			// The server's push may arrive first; receive ignores repeats.
			receive(line, true);
		} catch (err) {
			notice.textContent = err.message || t("dm.not_sent");
			setTimeout(() => (notice.textContent = ""), 4000);
		} finally {
			sending = false;
			sendBtn.disabled = false;
			input.focus();
		}
	};
	sendBtn.addEventListener("click", say);
	input.addEventListener("keydown", (ev) => {
		if (ev.key === "Enter" && !ev.shiftKey) {
			ev.preventDefault();
			say();
		}
	});
	host.append(
		h("p", { class: "muted small" }, t("chat.about")),
		log,
		picker.panel,
		row,
		notice
	);
	draw();

	// History first, then the live feed (reconnecting when it drops).
	const load = async (quiet = false) => {
		try {
			const fresh = await api.globalChat(lines.length ? lines[lines.length - 1].id : 0);
			for (const l of fresh || []) receive(l, false, quiet);
			if (!lines.length) draw();
		} catch (_) {
			/* chat is a nicety */
		}
	};
	let retry = 1000;
	const connect = () => {
		let ws;
		try {
			ws = new WebSocket((location.protocol === "https:" ? "wss://" : "ws://") + location.host + "/ws-chat");
		} catch (_) {
			return;
		}
		ws.onopen = () => {
			retry = 1000;
			if (lines.length) load(); // anything said while the feed was down
		};
		ws.onmessage = (ev) => {
			try {
				const env = JSON.parse(ev.data);
				if (env.type === "chat") receive(env.data);
				else if (env.type === "chat_translation") translated(env.data);
			} catch (_) {
				/* not for us */
			}
		};
		ws.onclose = () => {
			setTimeout(connect, retry);
			retry = Math.min(retry * 2, 30000);
		};
	};
	load(true); // the history is not news
	connect();

	return {
		/** The chat came on screen: everything is read. */
		seen() {
			unread = 0;
			onUnread(0);
			markSeen();
			log.scrollTop = log.scrollHeight;
		},
		focus: () => (nameInput.value.trim() ? input : nameInput).focus(),
	};
}
