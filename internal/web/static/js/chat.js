// Table talk: a small chat panel every player and spectator of a game shares.
// The server keeps the latest messages of a game and pushes new ones over the
// game's WebSocket as "chat" events; the page redraws on every move, so the
// draft, the scroll position and the focus are kept here between redraws.

import { h, store, lastName } from "./util.js";
import { api } from "./api.js";
import { cssColor } from "./render.js";
import { onNavigate } from "./nav.js";

const chat = {
	code: null,
	lines: [],
	loaded: false,
	loading: false,
	draft: "",
	focused: false,
	unread: 0,
	sending: false,
	picker: false, // the emoji picker is open
};

// The emoji of the picker, by group: [emoji, name]. The names are for the
// screen reader and the tooltip.
const EMOJI_GROUPS = [
	[
		"Faces",
		[
			["😀", "grinning"], ["😄", "smiling"], ["😁", "beaming"], ["😆", "laughing"], ["😅", "nervous laugh"], ["😂", "tears of joy"], ["🤣", "rolling on the floor"],
			["🙂", "slight smile"], ["😉", "wink"], ["😊", "blush"], ["😍", "heart eyes"], ["😘", "blowing a kiss"], ["😎", "cool"], ["🤩", "star-struck"],
			["🥳", "party"], ["😏", "smirk"], ["😬", "grimace"], ["🙄", "eye roll"], ["🤔", "thinking"], ["🤨", "raised eyebrow"], ["😐", "neutral"],
			["😮", "surprised"], ["😲", "astonished"], ["😱", "screaming"], ["😭", "crying"], ["😢", "sad"], ["😤", "huffing"], ["😡", "angry"],
			["🤬", "cursing"], ["😈", "devil"], ["💀", "skull"], ["😴", "sleeping"], ["🤯", "mind blown"], ["🥺", "pleading"], ["😇", "angel"], ["🫠", "melting"],
		],
	],
	[
		"Hands",
		[
			["👍", "thumbs up"], ["👎", "thumbs down"], ["👏", "clap"], ["🙌", "raised hands"], ["🙏", "thanks"], ["🤝", "handshake"], ["💪", "strong"],
			["👀", "eyes"], ["🤞", "fingers crossed"], ["✌️", "victory"], ["🤷", "shrug"], ["🤦", "facepalm"], ["🫡", "salute"], ["👋", "wave"], ["👌", "ok"],
		],
	],
	[
		"Game",
		[
			["🎲", "dice"], ["🏆", "trophy"], ["⚔️", "swords"], ["🛡️", "shield"], ["🗡️", "dagger"], ["🎯", "bullseye"], ["🃏", "joker"], ["🧩", "puzzle"],
			["👑", "crown"], ["💎", "gem"], ["🗺️", "map"], ["📜", "scroll"], ["⏳", "hourglass"], ["⏰", "alarm clock"], ["🔥", "fire"], ["💥", "boom"],
			["⚡", "lightning"], ["✨", "sparkles"], ["🎉", "party popper"], ["🎊", "confetti"], ["✅", "yes"], ["❌", "no"], ["❓", "question"], ["❗", "exclamation"], ["💯", "hundred"],
		],
	],
	[
		"Mythos",
		[
			["🐙", "octopus"], ["🦑", "squid"], ["🌊", "wave"], ["👁️", "eye"], ["🌑", "new moon"], ["🌙", "crescent moon"], ["🕯️", "candle"], ["📖", "book"],
			["🗝️", "key"], ["🔮", "crystal ball"], ["🧿", "amulet"], ["🦇", "bat"], ["🐍", "snake"], ["🕷️", "spider"], ["🐐", "goat"], ["🌱", "seedling"],
			["👻", "ghost"], ["👽", "alien"], ["🧟", "zombie"], ["☠️", "skull and crossbones"], ["🩸", "blood"], ["🌀", "cyclone"], ["🌪️", "tornado"], ["❄️", "snowflake"], ["🟡", "yellow sign"],
		],
	],
	[
		"Hearts",
		[
			["❤️", "red heart"], ["🧡", "orange heart"], ["💛", "yellow heart"], ["💚", "green heart"], ["💙", "blue heart"], ["💜", "purple heart"],
			["🖤", "black heart"], ["🤍", "white heart"], ["💔", "broken heart"],
		],
	],
];

let listEl = null;
let badgeEl = null;

// Another page, perhaps another game: nothing of this chat belongs to it.
onNavigate(() => {
	Object.assign(chat, { code: null, lines: [], loaded: false, loading: false, draft: "", focused: false, unread: 0, sending: false, picker: false });
	listEl = badgeEl = null;
});

const RECENT_KEY = "bingle.recentEmoji";

/** The emoji used last, most recent first. */
function recentEmoji() {
	const l = store.get(RECENT_KEY, []);
	return Array.isArray(l) ? l.filter((x) => typeof x === "string").slice(0, 12) : [];
}

function rememberEmoji(e) {
	store.set(RECENT_KEY, [e, ...recentEmoji().filter((x) => x !== e)].slice(0, 12));
}

/** Puts text into the input where the cursor is, and keeps the draft. */
function insertAtCursor(input, text) {
	const from = input.selectionStart ?? input.value.length;
	const to = input.selectionEnd ?? input.value.length;
	input.value = input.value.slice(0, from) + text + input.value.slice(to);
	const at = from + text.length;
	try {
		input.setSelectionRange(at, at);
	} catch (_) {
		/* not all input types support it */
	}
	chat.draft = input.value;
	input.dispatchEvent(new Event("input", { bubbles: true }));
}

/**
 * The emoji button and its panel. The panel stays open while the player picks
 * (one emoji after the other); Escape, a click elsewhere or the button closes
 * it. The page redraws on every move, so whether it is open is kept in `chat`.
 */
export function emojiPicker(input) {
	const panel = h("div", { class: "emoji-panel", id: "emoji-panel", role: "group", "aria-label": "Emoji", hidden: !chat.picker });
	const button = h(
		"button",
		{ type: "button", class: "emoji-button", id: "emoji-button", "aria-haspopup": "true", "aria-expanded": chat.picker ? "true" : "false", "aria-controls": "emoji-panel", title: "Add an emoji" },
		"🙂"
	);
	const pick = (emoji) => {
		insertAtCursor(input, emoji);
		rememberEmoji(emoji);
		fill();
		input.focus();
	};
	const cell = (emoji, name) =>
		h("button", { type: "button", class: "emoji-cell", "data-emoji": emoji, title: name, "aria-label": name, onclick: () => pick(emoji) }, emoji);
	const fill = () => {
		panel.textContent = "";
		const recent = recentEmoji();
		const groups = recent.length ? [["Recent", recent.map((e) => [e, "recent: " + e])], ...EMOJI_GROUPS] : EMOJI_GROUPS;
		for (const [title, list] of groups) {
			panel.append(h("div", { class: "emoji-group-title small muted" }, title));
			const grid = h("div", { class: "emoji-grid" });
			for (const [emoji, name] of list) grid.append(cell(emoji, name));
			panel.append(grid);
		}
	};
	fill();
	const set = (open) => {
		chat.picker = open;
		panel.hidden = !open;
		button.setAttribute("aria-expanded", open ? "true" : "false");
	};
	button.addEventListener("click", () => {
		set(!chat.picker);
		if (chat.picker) input.focus();
	});
	panel.addEventListener("keydown", (ev) => {
		if (ev.key === "Escape") {
			ev.stopPropagation();
			set(false);
			button.focus();
		}
	});
	// A click anywhere else closes the panel.
	const away = (ev) => {
		if (!chat.picker) return;
		if (!document.body.contains(panel)) {
			document.removeEventListener("pointerdown", away, true);
			return;
		}
		if (panel.contains(ev.target) || button.contains(ev.target)) return;
		set(false);
	};
	document.addEventListener("pointerdown", away, true);
	return { button, panel };
}

/** Reads the latest messages of the game once. */
export async function loadChat(code) {
	if (chat.loaded || chat.loading) return;
	chat.code = code;
	chat.loading = true;
	try {
		chat.lines = (await api.chat(code)) || [];
		chat.loaded = true;
	} catch (_) {
		/* chat is a nicety: the game goes on without it */
	} finally {
		chat.loading = false;
	}
}

/** True once the history has been read. */
export function chatLoaded() {
	return chat.loaded;
}

/** A message pushed by the server: kept, and drawn in place when on screen. */
export function receiveChat(line, { mine }) {
	if (!line || chat.lines.some((l) => l.id === line.id)) return;
	chat.lines.push(line);
	if (chat.lines.length > 300) chat.lines.splice(0, chat.lines.length - 300);
	if (listEl && document.body.contains(listEl)) {
		const atEnd = listEl.scrollHeight - listEl.scrollTop - listEl.clientHeight < 40;
		listEl.append(lineNode(line, mine));
		if (atEnd || mine) listEl.scrollTop = listEl.scrollHeight;
		else if (document.hidden) bump();
	} else {
		bump();
	}
}

function bump() {
	chat.unread++;
	if (badgeEl) {
		badgeEl.textContent = String(chat.unread);
		badgeEl.hidden = false;
	}
}

function clock(iso) {
	// SQLite stores "YYYY-MM-DD HH:MM:SS" in UTC.
	const t = Date.parse(String(iso || "").replace(" ", "T") + (/[zZ]|[+-]\d\d:?\d\d$/.test(iso || "") ? "" : "Z"));
	if (Number.isNaN(t)) return "";
	return new Date(t).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
}

function lineNode(line, mine) {
	return h(
		"li",
		{ class: "chat-line" + (mine ? " mine" : "") + (line.seat ? "" : " spectator"), "data-id": line.id, "data-seat": line.seat || null },
		h("span", { class: `chat-who ${line.color ? "color-" + cssColor(line.color) : ""}`, title: line.seat ? "" : "spectator" }, line.name + (line.seat ? "" : " (watching)")),
		h("span", { class: "chat-text" }, line.text),
		h("time", { class: "chat-time muted small" }, clock(line.at))
	);
}

/**
 * The chat card. `me` is {name, seat} of the viewer (name null when they only
 * watch); `canSay` false hides the entry row.
 */
export function chatCard({ code, me }) {
	chat.code = code;
	const card = h("div", { class: "card chat", id: "chat-card" });
	badgeEl = h("span", { class: "badge chat-unread", id: "chat-unread", hidden: chat.unread === 0 }, String(chat.unread));
	card.append(h("h3", {}, "Chat ", badgeEl));

	listEl = h("ol", { class: "chat-log", id: "chat-log", "aria-live": "polite", "aria-label": "Chat messages" });
	if (!chat.lines.length) listEl.append(h("li", { class: "chat-empty muted small" }, chat.loaded ? "Nobody has said anything yet." : "Loading…"));
	for (const line of chat.lines) listEl.append(lineNode(line, me.name && line.name.toLowerCase() === me.name.toLowerCase()));
	card.append(listEl);
	queueMicrotask(() => {
		if (listEl) listEl.scrollTop = listEl.scrollHeight;
	});

	// Spectators speak under a name of their own choosing.
	const seated = !!(me.name && me.seat);
	let nameInput = null;
	if (!seated) {
		nameInput = h("input", {
			type: "text",
			id: "chat-name",
			class: "chat-name",
			placeholder: "Your name",
			maxlength: "32",
			"aria-label": "Your name in the chat",
			value: store.get("bingle.chatname", "") || lastName() || "",
			autocomplete: "nickname",
		});
		nameInput.addEventListener("input", () => store.set("bingle.chatname", nameInput.value.trim()));
	}
	const input = h("input", {
		type: "text",
		id: "chat-input",
		class: "chat-input",
		placeholder: seated ? "Say something to the table…" : "Say something (as a spectator)…",
		maxlength: "500",
		autocomplete: "off",
		"aria-label": "Chat message",
		value: chat.draft,
	});
	input.addEventListener("input", () => {
		chat.draft = input.value;
	});
	input.addEventListener("focus", () => (chat.focused = true));
	input.addEventListener("blur", () => (chat.focused = false));
	const send = async () => {
		const text = input.value.trim();
		const name = seated ? me.name : (nameInput.value || "").trim();
		if (!text || chat.sending) return;
		if (!name) {
			nameInput.focus();
			return;
		}
		chat.sending = true;
		sendBtn.disabled = true;
		try {
			const line = await api.say(code, name, text);
			chat.draft = "";
			input.value = "";
			// The server's push may arrive first; receiveChat ignores repeats.
			receiveChat(line, { mine: true });
		} catch (err) {
			notice.textContent = err.message || "The message was not sent.";
			setTimeout(() => {
				if (notice.textContent) notice.textContent = "";
			}, 4000);
		} finally {
			chat.sending = false;
			sendBtn.disabled = false;
			input.focus();
		}
	};
	input.addEventListener("keydown", (ev) => {
		if (ev.key === "Enter" && !ev.shiftKey) {
			ev.preventDefault();
			send();
		}
	});
	const sendBtn = h("button", { type: "button", id: "chat-send", class: "primary", onclick: send }, "Send");
	const notice = h("p", { class: "small error chat-notice", id: "chat-notice", role: "status" });
	const row = h("div", { class: "chat-entry" });
	const picker = emojiPicker(input);
	if (nameInput) row.append(nameInput);
	row.append(picker.button, input, sendBtn);
	card.append(picker.panel, row, notice);

	// The page redraws on every move: put the cursor back where it was.
	if (chat.focused) {
		setTimeout(() => {
			if (document.body.contains(input)) {
				input.focus();
				const n = input.value.length;
				try {
					input.setSelectionRange(n, n);
				} catch (_) {
					/* not all input types support it */
				}
			}
		}, 0);
	}
	document.addEventListener(
		"visibilitychange",
		() => {
			if (!document.hidden) {
				chat.unread = 0;
				if (badgeEl) badgeEl.hidden = true;
			}
		},
		{ once: true }
	);
	return card;
}

/** Marks everything as read (the card is on screen and its tab is visible). */
export function chatSeen() {
	chat.unread = 0;
	if (badgeEl) badgeEl.hidden = true;
}

/** How many messages have come in since the chat was last on screen. */
export function chatUnread() {
	return chat.unread;
}
