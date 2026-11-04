// Moving a guest to another device, in the profile window: the key that makes this
// browser's seats its own, to copy, and the way to use one copied from elsewhere
// (the accounts plan, §3a, and ADR 0072).

import { h, clear } from "./util.js";
import { api } from "./api.js";

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
			say(err.message || "That did not work.", "error");
		} finally {
			button.disabled = false;
		}
	};

	const linkText = h("code", { id: "guest-link", class: "wrap" });
	const keyText = h("code", { id: "guest-key", class: "wrap" });
	const shown = h(
		"div",
		{ id: "guest-shown", hidden: true },
		h("p", { class: "small" }, "Open this link on the other device (it is copied), or paste the key there:"),
		h("p", { class: "small" }, linkText),
		h("p", { class: "small" }, keyText)
	);
	const copy = h("button", { type: "button", class: "primary", id: "guest-copy" }, "Copy my guest key");
	copy.addEventListener("click", () =>
		run(copy, async () => {
			const { key, link } = await api.guestKey();
			const url = location.origin + link;
			linkText.textContent = url;
			keyText.textContent = key;
			shown.hidden = false;
			try {
				await navigator.clipboard.writeText(url);
				say("The link is copied.");
			} catch (_) {
				say("Copy the link or the key by hand.");
			}
		})
	);

	const useKey = h("input", { id: "guest-use-key", type: "text", maxlength: "60", autocapitalize: "none", spellcheck: "false", placeholder: "bgg_…" });
	const use = h("button", { type: "button", class: "ghost", id: "guest-use" }, "Submit Key");
	use.addEventListener("click", () =>
		run(use, async () => {
			await api.adoptGuest(useKey.value.trim());
			useKey.value = "";
			say("This browser is that guest now: its games are under Active games, and this browser's own came along.");
			paint("guest");
		})
	);

	const rotate = h("button", { type: "button", class: "ghost", id: "guest-rotate" }, "New key, same me");
	rotate.addEventListener("click", () =>
		run(rotate, async () => {
			await api.rotateGuest();
			shown.hidden = true;
			say("This browser has a new key. The old one, wherever you copied it, no longer works; your games are still yours.");
		})
	);
	const discard = h("button", { type: "button", class: "ghost", id: "guest-discard" }, "Start over as a new guest");
	discard.addEventListener("click", () =>
		run(discard, async () => {
			if (!window.confirm("Start over as a new guest? This browser loses its seats: the games you play now stay with the old guest, which nobody can reach again.")) return;
			await api.discardGuest();
			shown.hidden = true;
			say("This browser is a new guest from its next game on.");
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
				h("p", { class: "small muted" }, "Your guest key is what makes your seats yours. Copy it to your phone or another browser to carry on your games there. Anybody with the key can play as you."),
				h("div", { class: "row" }, copy),
				shown
			);
		} else {
			// A browser is a guest from its first game, and has no key until then.
			body.append(h("p", { class: "small muted", id: "guest-no-key" }, "You get a guest key to copy once you make or join your first game. If you have one from another device, use it here."));
		}
		body.append(h("div", { class: "field guest-use" }, h("label", { for: "guest-use-key" }, "Use a guest key from another device"), useKey), h("div", { class: "row" }, use));
		if (kind === "guest") {
			body.append(h("details", { class: "guest-more" }, h("summary", { class: "small" }, "If your key got out, or to start again"), h("div", { class: "row" }, rotate, discard)));
		}
		body.append(result);
	}
	section.append(h("h3", {}, "Move to Another Device"), body);
	return { section, paint };
}
