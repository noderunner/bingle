// The words of a chat line in the reader's language (ADR 0143), for a game's
// chat, the global chat and direct messages alike.

import { h } from "./util.js";
import { t, lang } from "./i18n.js";

// The lines the reader asked to see as written, by scope ("g" a game's, "w"
// the global chat's, "d" direct messages) and id: kept across redraws.
const showingOriginal = new Set();

let languageNames = null;
try {
	languageNames = new Intl.DisplayNames([lang], { type: "language" });
} catch (_) {
	/* an old browser: "Translated", without the language */
}

/**
 * The words of a chat line (ADR 0143): its translation into the reader's
 * language once the server has one, marked, with the original a click away;
 * else the line as written. Shown as text, never as markup.
 */
export function chatText(line, scope) {
	const text = h("span", { class: "chat-text", dir: "auto" }, line.text);
	const tr = line.translation;
	if (!tr || !tr.text || line.server) return [text];
	const key = scope + line.id;
	let from = "";
	if (tr.from && tr.from !== "und") {
		try {
			from = (languageNames && languageNames.of(tr.from)) || "";
		} catch (_) {
			/* a code the browser does not know */
		}
	}
	const mark = h("span", { class: "small muted" }, from ? t("chat.translated_from", { language: from }) : t("chat.translated"));
	const toggle = h("button", { type: "button", class: "ghost small chat-original" });
	const paint = () => {
		const original = showingOriginal.has(key);
		text.textContent = original ? line.text : tr.text;
		if (original && tr.from && tr.from !== "und") text.lang = tr.from;
		else if (original) text.removeAttribute("lang");
		else text.lang = tr.lang;
		toggle.textContent = original ? t("chat.show_translation") : t("chat.show_original");
	};
	toggle.addEventListener("click", () => {
		if (!showingOriginal.delete(key)) showingOriginal.add(key);
		paint();
	});
	paint();
	return [text, h("span", { class: "chat-translation" }, mark, " ", toggle)];
}
