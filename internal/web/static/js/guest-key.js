// Moving a guest to another device, in the profile window: the key that makes this
// browser's seats its own, to copy, and the way to use one copied from elsewhere
// (the accounts plan, §3a, and ADR 0072).

import { h, clear } from "./util.js";
import { api } from "./api.js";
import { t } from "./i18n.js";

/** The section of the window, and paint(kind) to draw it for "guest", "anonymous" or "account". */
export function guestKeySection() {
	const section = h("section", { class: "guest-section", id: "guest-box", hidden: true });
	const result = h("p", { class: "small", id: "guest-result", role: "status", "aria-live": "polite" });
	const say = (text, kind = "") => {
		result.textContent = text;
		result.className = "small " + kind;
	};
	const run = async (button, f) => {
		button.disabled = true;
		say("");
		try {
			await f();
		} catch (err) {
			say(err.message || t("common.not_done"), "error");
		} finally {
			button.disabled = false;
		}
	};

	const linkText = h("code", { id: "guest-link", class: "wrap" });
	const keyText = h("code", { id: "guest-key", class: "wrap" });
	const shown = h(
		"div",
		{ id: "guest-shown", hidden: true },
		h("p", { class: "small" }, t("guest.open_link")),
		h("p", { class: "small" }, linkText),
		h("p", { class: "small" }, keyText)
	);
	const copy = h("button", { type: "button", class: "primary", id: "guest-copy" }, t("guest.copy"));
	copy.addEventListener("click", () =>
		run(copy, async () => {
			const { key, link } = await api.guestKey();
			const url = location.origin + link;
			linkText.textContent = url;
			keyText.textContent = key;
			shown.hidden = false;
			try {
				await navigator.clipboard.writeText(url);
				say(t("guest.copied"));
			} catch (_) {
				say(t("guest.copy_by_hand"));
			}
		})
	);

	const useKey = h("input", { id: "guest-use-key", type: "text", maxlength: "60", autocapitalize: "none", spellcheck: "false", placeholder: "bgg_…" });
	const use = h("button", { type: "button", class: "ghost", id: "guest-use" }, t("guest.submit"));
	use.addEventListener("click", () =>
		run(use, async () => {
			await api.adoptGuest(useKey.value.trim());
			useKey.value = "";
			say(t("guest.adopted"));
			paint("guest");
		})
	);

	const rotate = h("button", { type: "button", class: "ghost", id: "guest-rotate" }, t("guest.rotate"));
	rotate.addEventListener("click", () =>
		run(rotate, async () => {
			await api.rotateGuest();
			shown.hidden = true;
			say(t("guest.rotated"));
		})
	);
	const discard = h("button", { type: "button", class: "ghost", id: "guest-discard" }, t("guest.discard"));
	discard.addEventListener("click", () =>
		run(discard, async () => {
			if (!window.confirm(t("guest.discard_confirm"))) return;
			await api.discardGuest();
			shown.hidden = true;
			say(t("guest.discarded"));
			paint("anonymous");
		})
	);

	const body = h("div", { id: "guest-body" });
	function paint(kind) {
		section.hidden = kind === "account";
		if (section.hidden) return;
		clear(body);
		if (kind === "guest") {
			body.append(
				h("p", { class: "small muted" }, t("guest.about")),
				h("div", { class: "row" }, copy),
				shown
			);
		} else {
			// A browser is a guest from its first game, and has no key until then.
			body.append(h("p", { class: "small muted", id: "guest-no-key" }, t("guest.no_key")));
		}
		body.append(h("div", { class: "field guest-use" }, h("label", { for: "guest-use-key" }, t("guest.use")), useKey), h("div", { class: "row" }, use));
		if (kind === "guest") {
			body.append(h("details", { class: "guest-more" }, h("summary", { class: "small" }, t("guest.more")), h("div", { class: "row" }, rotate, discard)));
		}
		body.append(result);
	}
	section.append(h("h3", {}, t("guest.title")), body);
	return { section, paint };
}
