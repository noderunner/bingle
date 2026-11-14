// Badges as the page shows them (ADR 0130): the server says what a badge is,
// and the page draws it.

import { h } from "./util.js";

/** The day a badge was earned, in the reader's own way of writing dates. */
function earned(iso) {
	const t = Date.parse(iso || "");
	return Number.isNaN(t) ? "" : new Date(t).toLocaleDateString([], { year: "numeric", month: "short", day: "numeric" });
}

/** One badge held: its name, what it is for, and where and when it was earned. */
export function badgeItem(b) {
	const where = [b.game, earned(b.awarded_at)].filter(Boolean).join(", ");
	return h(
		"li",
		{ class: "badge-item", "data-badge": b.badge },
		h("span", { class: "badge-medal", "aria-hidden": "true" }, "★"),
		h(
			"div",
			{ class: "badge-body" },
			h("strong", { class: "badge-name" }, b.name),
			h("span", { class: "small" }, b.description),
			h("span", { class: "small muted" }, where + (b.hidden ? " · others see it when the game ends" : ""))
		)
	);
}
