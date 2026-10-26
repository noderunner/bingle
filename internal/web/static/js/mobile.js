// The phone layout of the game page: one full-screen view at a time, swiped
// between like the pages of an app, with a thumb-reach bar at the bottom.
//
// This module only knows about layout: which layout is wanted, and how to lay
// out a list of ready-made pages in a swipeable track with a navigation bar.
// What the pages contain is up to the game page (game.js).

import { h, store } from "./util.js";
import { onLeave } from "./nav.js";

const KEY = "bingle.layout";

/** The layouts a player can ask for in the options. */
export const LAYOUTS = [
	["auto", "Automatic (by screen)"],
	["phone", "Phone / tablet"],
	["desktop", "Desktop"],
];

// A touch screen of phone or portrait-tablet width. A narrow desktop window
// (a mouse) keeps the desktop page: it can ask for the phone layout itself.
const smallTouch = window.matchMedia("(pointer: coarse) and (max-width: 900px)");

export function layoutChoice() {
	const v = store.get(KEY, "auto");
	return v === "phone" || v === "desktop" ? v : "auto";
}

export function setLayoutChoice(v) {
	store.set(KEY, v);
}

/** Should the page be laid out for a small touch screen right now? */
export function isMobileLayout() {
	const c = layoutChoice();
	return c === "phone" || (c === "auto" && smallTouch.matches);
}

/** Calls fn when the screen changes in a way that could change the answer. */
export function onScreenChange(fn) {
	smallTouch.addEventListener("change", fn);
	window.addEventListener("orientationchange", fn);
	// smallTouch belongs to the module, not to a page: the page's listener goes with the page.
	onLeave(() => smallTouch.removeEventListener("change", fn));
}

/**
 * Lays pages out in a horizontally swipeable track under a bottom bar.
 *
 *   pages  [{id, label, node, group}]  in swipe order
 *   groups [{id, label, icon, badge, pulse}]  the buttons of the bottom bar; a
 *          group with several pages (one per cult) shows a strip to pick one
 *   chips  {group: [{page, label, color}]}  the strip of a group
 *   current  the id of the page to show
 *   onChange(id)  called when the player settles on another page
 *
 * Returns {node, show(id), restore()}. restore() must be called once the node
 * is in the document: it puts the track back on the current page without
 * animation (the page is rebuilt on every move, the view must not jump).
 */
export function mobileShell({ pages, groups, chips = {}, current, onChange }) {
	const ids = pages.map((p) => p.id);
	let now = ids.includes(current) ? current : ids[0];

	const track = h("div", { class: "m-track", id: "m-track" });
	for (const p of pages) {
		track.append(h("section", { class: `m-page m-page-${p.id}`, id: `m-page-${p.id}`, "data-page": p.id, "aria-label": p.label }, p.node));
	}

	const nav = h("nav", { class: "m-nav", id: "m-nav", "aria-label": "Views" });
	const strip = h("div", { class: "m-chips", id: "m-chips" });
	const groupOf = (id) => (pages.find((p) => p.id === id) || {}).group;

	const paint = () => {
		const g = groupOf(now);
		for (const b of nav.querySelectorAll("button")) {
			const on = b.dataset.group === g;
			b.classList.toggle("on", on);
			b.setAttribute("aria-current", on ? "page" : "false");
		}
		strip.textContent = "";
		const list = chips[g] || [];
		strip.hidden = list.length < 2;
		for (const c of list) {
			strip.append(
				h(
					"button",
					{ type: "button", class: "m-chip" + (c.page === now ? " on" : ""), "data-page": c.page, onclick: () => show(c.page, true) },
					h("span", { class: `swatch color-${c.color}` }),
					c.label
				)
			);
		}
	};

	const show = (id, smooth) => {
		const i = ids.indexOf(id);
		if (i < 0) return;
		now = id;
		track.scrollTo({ left: i * track.clientWidth, behavior: smooth ? "smooth" : "instant" });
		paint();
		onChange(id);
	};

	for (const g of groups) {
		const first = pages.find((p) => p.group === g.id);
		if (!first) continue;
		nav.append(
			h(
				"button",
				{ type: "button", class: "m-nav-item", "data-group": g.id, onclick: () => show(groupOf(now) === g.id ? now : first.id, true) },
				h("span", { class: "m-nav-icon", "aria-hidden": "true" }, g.icon),
				h("span", { class: "m-nav-label" }, g.label),
				g.badge ? h("span", { class: "m-nav-badge" + (g.pulse ? " pulse" : "") }, String(g.badge)) : g.pulse ? h("span", { class: "m-nav-dot pulse" }) : null
			)
		);
	}

	// The player settles on a page by swiping: follow it.
	let timer = null;
	track.addEventListener(
		"scroll",
		() => {
			clearTimeout(timer);
			timer = setTimeout(() => {
				const w = track.clientWidth || 1;
				const id = ids[Math.max(0, Math.min(ids.length - 1, Math.round(track.scrollLeft / w)))];
				if (id !== now) {
					now = id;
					paint();
					onChange(id);
				}
			}, 90);
		},
		{ passive: true }
	);

	const node = h("div", { class: "m-shell", id: "m-shell" }, track, strip, nav);
	paint();
	return {
		node,
		show,
		restore() {
			track.scrollTo({ left: ids.indexOf(now) * track.clientWidth, behavior: "instant" });
		},
	};
}
