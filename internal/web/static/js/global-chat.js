// The global chat room of the home page: one room everybody who opens the page
// can read and write in, like the table talk of a game but without a game.
// New messages arrive over a WebSocket (/ws-chat); the page keeps the draft and
// counts the messages that came in while another tab was showing.

import { h, store, lastName } from "./util.js";
import { api } from "./api.js";
import { emojiPicker } from "./chat.js";

const MAX_LINES = 300;

function clock(iso) {
	// SQLite stores "YYYY-MM-DD HH:MM:SS" in UTC.
	const t = Date.parse(String(iso || "").replace(" ", "T") + (/[zZ]|[+-]\d\d:?\d\d$/.test(iso || "") ? "" : "Z"));
	return Number.isNaN(t) ? "" : new Date(t).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
}

/**
 * Builds the chat into host. `onUnread(n)` is told how many messages came in
 * while `visible()` was false (the chat tab was not showing).
 */
export function mountGlobalChat(host, { visible = () => true, onUnread = () => {} } = {}) {
	const lines = [];
	let unread = 0;
	let sending = false;

	const log = h("ol", { class: "chat-log", id: "global-chat-log", "aria-live": "polite", "aria-label": "Global chat messages" });
	const nameInput = h("input", {
		type: "text",
		id: "global-chat-name",
		class: "chat-name",
		placeholder: "Your name",
		maxlength: "32",
		"aria-label": "Your name in the chat",
		value: store.get("bingle.chatname", "") || lastName() || "",
		autocomplete: "nickname",
	});
	nameInput.addEventListener("input", () => store.set("bingle.chatname", nameInput.value.trim()));
	const input = h("input", {
		type: "text",
		id: "global-chat-input",
		class: "chat-input",
		placeholder: "Say something to everyone…",
		maxlength: "500",
		autocomplete: "off",
		"aria-label": "Chat message",
	});
	const notice = h("p", { class: "small error chat-notice", id: "global-chat-notice", role: "status" });
	const sendBtn = h("button", { type: "button", id: "global-chat-send", class: "primary" }, "Send");
	const picker = emojiPicker(input);
	const row = h("div", { class: "chat-entry" }, nameInput, picker.button, input, sendBtn);

	const lineNode = (line) => {
		const mine = nameInput.value.trim() && line.name.toLowerCase() === nameInput.value.trim().toLowerCase();
		return h(
			"li",
			{ class: "chat-line" + (mine ? " mine" : ""), "data-id": line.id },
			h("span", { class: "chat-who" }, line.name),
			h("span", { class: "chat-text" }, line.text),
			h("time", { class: "chat-time muted small" }, clock(line.at))
		);
	};
	const draw = () => {
		log.textContent = "";
		if (!lines.length) log.append(h("li", { class: "chat-empty muted small" }, "Nobody has said anything yet."));
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
		if (!mine && !quiet && !visible()) onUnread(++unread);
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
			// The server's push may arrive first; receive ignores repeats.
			receive(line, true);
		} catch (err) {
			notice.textContent = err.message || "The message was not sent.";
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
		h("p", { class: "muted small" }, "Everyone who has this page open can read and write here. Be kind; the last few hundred messages are kept."),
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
			log.scrollTop = log.scrollHeight;
		},
		focus: () => (nameInput.value.trim() ? input : nameInput).focus(),
	};
}
