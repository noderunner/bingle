// Table talk: a small chat panel every player and spectator of a game shares.
// The server keeps the latest messages of a game and pushes new ones over the
// game's WebSocket as "chat" events; the page redraws on every move, so the
// draft, the scroll position and the focus are kept here between redraws.

import { handleBadge } from "./friend-badge.js";
import { h, store, lastName, markChatRead } from "./util.js";
import { api } from "./api.js";
import { cssColor } from "./render.js";
import { onNavigate } from "./nav.js";
import { t, lang } from "./i18n.js";
import { chatText } from "./chat-text.js";

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

// The emoji of the picker, by group: [emoji, the key of its name]. The names are
// for the screen reader and the tooltip, in the reader's language (ADR 0141).
const EMOJI_GROUPS = [
	[
		"emoji.group_faces",
		[
			["😀", "emoji.grinning"], ["😄", "emoji.smiling"], ["😁", "emoji.beaming"], ["😆", "emoji.laughing"], ["😅", "emoji.nervous_laugh"], ["😂", "emoji.tears_of_joy"], ["🤣", "emoji.rolling_on_the_floor"],
			["🙂", "emoji.slight_smile"], ["😉", "emoji.wink"], ["😊", "emoji.blush"], ["😍", "emoji.heart_eyes"], ["😘", "emoji.blowing_a_kiss"], ["😎", "emoji.cool"], ["🤩", "emoji.star_struck"],
			["🥳", "emoji.party"], ["😏", "emoji.smirk"], ["😬", "emoji.grimace"], ["🙄", "emoji.eye_roll"], ["🤔", "emoji.thinking"], ["🤨", "emoji.raised_eyebrow"], ["😐", "emoji.neutral"],
			["😮", "emoji.surprised"], ["😲", "emoji.astonished"], ["😱", "emoji.screaming"], ["😭", "emoji.crying"], ["😢", "emoji.sad"], ["😤", "emoji.huffing"], ["😡", "emoji.angry"],
			["🤬", "emoji.cursing"], ["😈", "emoji.devil"], ["💀", "emoji.skull"], ["😴", "emoji.sleeping"], ["🤯", "emoji.mind_blown"], ["🥺", "emoji.pleading"], ["😇", "emoji.angel"], ["🫠", "emoji.melting"],
		],
	],
	[
		"emoji.group_hands",
		[
			["👍", "emoji.thumbs_up"], ["👎", "emoji.thumbs_down"], ["👏", "emoji.clap"], ["🙌", "emoji.raised_hands"], ["🙏", "emoji.thanks"], ["🤝", "emoji.handshake"], ["💪", "emoji.strong"],
			["👀", "emoji.eyes"], ["🤞", "emoji.fingers_crossed"], ["✌️", "emoji.victory"], ["🤷", "emoji.shrug"], ["🤦", "emoji.facepalm"], ["🫡", "emoji.salute"], ["👋", "emoji.wave"], ["👌", "emoji.ok"],
		],
	],
	[
		"emoji.group_game",
		[
			["🎲", "emoji.dice"], ["🏆", "emoji.trophy"], ["⚔️", "emoji.swords"], ["🛡️", "emoji.shield"], ["🗡️", "emoji.dagger"], ["🎯", "emoji.bullseye"], ["🃏", "emoji.joker"], ["🧩", "emoji.puzzle"],
			["👑", "emoji.crown"], ["💎", "emoji.gem"], ["🗺️", "emoji.map"], ["📜", "emoji.scroll"], ["⏳", "emoji.hourglass"], ["⏰", "emoji.alarm_clock"], ["🔥", "emoji.fire"], ["💥", "emoji.boom"],
			["⚡", "emoji.lightning"], ["✨", "emoji.sparkles"], ["🎉", "emoji.party_popper"], ["🎊", "emoji.confetti"], ["✅", "emoji.yes"], ["❌", "emoji.no"], ["❓", "emoji.question"], ["❗", "emoji.exclamation"], ["💯", "emoji.hundred"],
		],
	],
	[
		"emoji.group_mythos",
		[
			["🐙", "emoji.octopus"], ["🦑", "emoji.squid"], ["🌊", "emoji.water_wave"], ["👁️", "emoji.eye"], ["🌑", "emoji.new_moon"], ["🌙", "emoji.crescent_moon"], ["🕯️", "emoji.candle"], ["📖", "emoji.book"],
			["🗝️", "emoji.key"], ["🔮", "emoji.crystal_ball"], ["🧿", "emoji.amulet"], ["🦇", "emoji.bat"], ["🐍", "emoji.snake"], ["🕷️", "emoji.spider"], ["🐐", "emoji.goat"], ["🌱", "emoji.seedling"],
			["👻", "emoji.ghost"], ["👽", "emoji.alien"], ["🧟", "emoji.zombie"], ["☠️", "emoji.skull_and_crossbones"], ["🩸", "emoji.blood"], ["🌀", "emoji.cyclone"], ["🌪️", "emoji.tornado"], ["❄️", "emoji.snowflake"], ["🟡", "emoji.yellow_sign"],
		],
	],
	[
		"emoji.group_hearts",
		[
			["❤️", "emoji.red_heart"], ["🧡", "emoji.orange_heart"], ["💛", "emoji.yellow_heart"], ["💚", "emoji.green_heart"], ["💙", "emoji.blue_heart"], ["💜", "emoji.purple_heart"],
			["🖤", "emoji.black_heart"], ["🤍", "emoji.white_heart"], ["💔", "emoji.broken_heart"],
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
	const panel = h("div", { class: "emoji-panel", id: "emoji-panel", role: "group", "aria-label": t("chat.emoji"), hidden: !chat.picker });
	const button = h(
		"button",
		{ type: "button", class: "emoji-button", id: "emoji-button", "aria-haspopup": "true", "aria-expanded": chat.picker ? "true" : "false", "aria-controls": "emoji-panel", title: t("chat.add_emoji") },
		"🙂"
	);
	const pick = (emoji) => {
		insertAtCursor(input, emoji);
		rememberEmoji(emoji);
		fill();
		input.focus();
	};
	const cell = (emoji, name) =>
		h("button", { type: "button", class: "emoji-cell", "data-emoji": emoji, title: t(name), "aria-label": t(name), onclick: () => pick(emoji) }, emoji);
	const fill = () => {
		panel.textContent = "";
		const recent = recentEmoji();
		const groups = recent.length ? [[t("chat.recent"), recent.map((e) => [e, e])], ...EMOJI_GROUPS] : EMOJI_GROUPS;
		for (const [title, list] of groups) {
			panel.append(h("div", { class: "emoji-group-title small muted" }, t(title)));
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

/** A line's translation, pushed by the server after the line: drawn in its place. */
export function receiveTranslation(d) {
	const line = d && chat.lines.find((l) => l.id === d.id);
	if (!line) return;
	line.translation = { lang: d.lang, from: d.from, text: d.text };
	const node = listEl && listEl.querySelector(`li[data-id="${Number(d.id)}"]`);
	if (node) node.replaceWith(lineNode(line, node.classList.contains("mine")));
}

/** A message pushed by the server: kept, and drawn in place when on screen. */
export function receiveChat(line, { mine, seen = true }) {
	if (!line || chat.lines.some((l) => l.id === line.id)) return;
	chat.lines.push(line);
	if (chat.lines.length > 300) chat.lines.splice(0, chat.lines.length - 300);
	if (listEl && document.body.contains(listEl)) {
		const atEnd = listEl.scrollHeight - listEl.scrollTop - listEl.clientHeight < 40;
		listEl.querySelector(".chat-empty")?.remove();
		listEl.append(lineNode(line, mine));
		if (atEnd || mine) listEl.scrollTop = listEl.scrollHeight;
		if (!mine && !seen) bump();
		if (seen && chat.code) markChatRead(chat.code, line.id);
	} else if (!mine) {
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
	const at = Date.parse(String(iso || "").replace(" ", "T") + (/[zZ]|[+-]\d\d:?\d\d$/.test(iso || "") ? "" : "Z"));
	if (Number.isNaN(at)) return "";
	return new Date(at).toLocaleTimeString(lang, { hour: "2-digit", minute: "2-digit" });
}

function lineNode(line, mine) {
	// A line from the server, such as a badge earned (ADR 0130), names no seat.
	if (line.server) {
		return h(
			"li",
			{ class: "chat-line server", "data-id": line.id },
			h("span", { class: "chat-who" }, line.name),
			...chatText(line, "g"),
			h("time", { class: "chat-time muted small" }, clock(line.at))
		);
	}
	return h(
		"li",
		{ class: "chat-line" + (mine ? " mine" : "") + (line.seat ? "" : " spectator"), "data-id": line.id, "data-seat": line.seat || null },
		h("span", { class: `chat-who ${line.color ? "color-" + cssColor(line.color) : ""}`, title: line.seat ? "" : t("chat.spectator") }, line.seat ? line.name : t("chat.watching", { name: line.name })),
		// Who said it: an account, by its handle, or a guest.
		line.handle ? handleBadge(line.handle) : line.guest ? h("span", { class: "chat-guest small muted" }, t("chat.guest")) : null,
		...chatText(line, "g"),
		h("time", { class: "chat-time muted small" }, clock(line.at))
	);
}

/** The next chat card takes the keyboard: the page is redrawing around a message being typed. */
export function keepChatFocus() {
	chat.focused = true;
}

/**
 * The chat card. `me` is {name, seat} of the viewer (name null when they only
 * watch); `canSay` false hides the entry row.
 */
export function chatCard({ code, me, voice = null }) {
	chat.code = code;
	const card = h("div", { class: "card chat", id: "chat-card" });
	badgeEl = h("span", { class: "badge chat-unread", id: "chat-unread", hidden: chat.unread === 0 }, String(chat.unread));
	// The voice chat's microphone sits in the head, and its line under it (js/voice.js).
	card.append(h("h3", { class: "chat-head" }, h("span", { class: "chat-title" }, t("chat.title")), badgeEl, voice && voice.button ? voice.button : null));
	if (voice && voice.note) card.append(voice.note);

	listEl = h("ol", { class: "chat-log", id: "chat-log", "aria-live": "polite", "aria-label": t("chat.messages_table") });
	if (!chat.lines.length) listEl.append(h("li", { class: "chat-empty muted small" }, chat.loaded ? t("chat.nobody") : t("common.loading")));
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
			placeholder: t("chat.your_name"),
			maxlength: "32",
			"aria-label": t("chat.name_label"),
			value: store.get("bingle.chatname", "") || lastName() || "",
			autocomplete: "nickname",
		});
		nameInput.addEventListener("input", () => store.set("bingle.chatname", nameInput.value.trim()));
	}
	const input = h("input", {
		type: "text",
		id: "chat-input",
		class: "chat-input",
		placeholder: seated ? t("chat.say_table") : t("chat.say_spectator"),
		maxlength: "500",
		autocomplete: "off",
		"aria-label": t("chat.message"),
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
			notice.textContent = err.message || t("dm.not_sent");
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
	const sendBtn = h("button", { type: "button", id: "chat-send", class: "primary", onclick: send }, t("dm.send"));
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
	return card;
}

/** Marks everything as read (the card is on screen and its tab is visible). */
export function chatSeen() {
	chat.unread = 0;
	const last = chat.lines[chat.lines.length - 1];
	if (chat.code && last) markChatRead(chat.code, last.id);
	if (badgeEl) badgeEl.hidden = true;
}

/** How many messages have come in since the chat was last on screen. */
export function chatUnread() {
	return chat.unread;
}
