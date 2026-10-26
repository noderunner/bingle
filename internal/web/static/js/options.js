// The game options window: one floating panel, opened from the bar at the top
// of the page, that holds everything a player may set or want to reach —
// reactions, map layers, display, tools, the invite link, resigning.
//
// The page is rebuilt on every move, so this window lives outside the rebuilt
// area and is updated in place: a section is only rebuilt when what it shows
// has changed (its signature), so a click never straddles a redraw.

import { h, clear } from "./util.js";

/**
 * Creates the window (hidden). Returns controls:
 *   open(), close(), toggle(), isOpen()
 *   section(id, heading)       the container of a section, created on demand
 *   update(id, sig, build)     rebuilds the section when sig changed
 *   hide(id, on)               hides or shows a whole section
 */
export function createOptionsWindow({ title = "Game options", onToggle = () => {} } = {}) {
	const closeBtn = h("button", { class: "options-close", id: "options-close", "aria-label": "Close the game options", title: "Close (Esc)" }, "×");
	const body = h("div", { class: "options-body", id: "options-body" });
	const win = h(
		"section",
		{ id: "game-options", class: "options-window", role: "dialog", "aria-label": title, "aria-modal": "false", hidden: true },
		h("div", { class: "options-head" }, h("h2", {}, title), closeBtn),
		body
	);
	document.body.append(win);
	const sections = new Map();
	const sigs = new Map();

	const api = {
		el: win,
		isOpen: () => !win.hidden,
		open() {
			win.hidden = false;
			onToggle(true);
			const first = win.querySelector("input, button, select, summary");
			if (first && document.activeElement === document.body) first.focus({ preventScroll: true });
		},
		close() {
			if (win.hidden) return;
			win.hidden = true;
			onToggle(false);
		},
		toggle() {
			if (win.hidden) api.open();
			else api.close();
		},
		section(id, heading) {
			let sec = sections.get(id);
			if (!sec) {
				const content = h("div", { class: "options-section-body", id: `options-${id}` });
				const root = h("section", { class: "options-section", "data-section": id }, heading ? h("h3", {}, heading) : null, content);
				body.append(root);
				sec = { root, content };
				sections.set(id, sec);
			}
			return sec.content;
		},
		update(id, sig, build) {
			const sec = sections.get(id);
			if (!sec) return;
			const key = typeof sig === "string" ? sig : JSON.stringify(sig);
			if (sigs.get(id) === key) return;
			sigs.set(id, key);
			clear(sec.content);
			build(sec.content);
		},
		hide(id, on) {
			const sec = sections.get(id);
			if (sec) sec.root.hidden = !!on;
		},
	};
	closeBtn.addEventListener("click", () => api.close());
	win.addEventListener("keydown", (ev) => {
		if (ev.key === "Escape") {
			ev.stopPropagation();
			api.close();
		}
	});
	return api;
}
