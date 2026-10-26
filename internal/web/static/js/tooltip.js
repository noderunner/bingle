// Hover cards: a small floating card that follows the pointer or the keyboard
// focus to an element and shows more than a native `title` can — an icon, a few
// numbers, a list. One card is open at a time; it never takes the pointer.

import { h } from "./util.js";
import { onNavigate } from "./nav.js";

let card = null;
let owner = null;

function hide() {
	if (card) card.remove();
	card = null;
	owner = null;
}

onNavigate(hide);

function place(el, node) {
	node.style.left = "0px";
	node.style.top = "0px";
	const r = el.getBoundingClientRect();
	const w = node.offsetWidth;
	const hgt = node.offsetHeight;
	let left = r.left + r.width / 2 - w / 2;
	left = Math.max(8, Math.min(left, window.innerWidth - w - 8));
	// Below the element, or above it when there is no room.
	let top = r.bottom + 8;
	if (top + hgt > window.innerHeight - 8 && r.top - hgt - 8 > 8) top = r.top - hgt - 8;
	node.style.left = `${Math.round(left)}px`;
	node.style.top = `${Math.round(top)}px`;
}

/**
 * Shows the card build() returns while the pointer is over `el` or `el` has the
 * focus. build is called each time, so the card is always up to date.
 */
export function hoverCard(el, build) {
	const show = () => {
		hide();
		const content = build();
		if (!content) return;
		card = h("div", { class: "hover-card", role: "tooltip" }, content);
		owner = el;
		document.body.append(card);
		place(el, card);
	};
	el.addEventListener("mouseenter", show);
	el.addEventListener("focus", show);
	el.addEventListener("mouseleave", () => {
		if (owner === el) hide();
	});
	el.addEventListener("blur", () => {
		if (owner === el) hide();
	});
	// A redraw may remove the element under the pointer: the card goes too.
	el.addEventListener("click", () => {
		if (owner === el) hide();
	});
}

/** Removes any open card (the page was redrawn). */
export function hideHoverCard() {
	hide();
}
