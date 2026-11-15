// Hover cards: a small floating card that follows the pointer or the keyboard
// focus to an element and shows more than a native `title` can — an icon, a few
// numbers, a list. One card is open at a time.
//
// A touch screen has no hover, so there a tap stands for it: a tap on an element
// with a card pins its card open, and a touch anywhere outside the card closes it
// (a second tap on the element too). An element that does something when clicked
// (a button or a link) keeps doing it on a tap, and opens no card, unless it asked
// for keepOnClick: then the tap does both. A pinned card takes the pointer, so it
// can be read and scrolled; a hovered one never does.

import { h } from "./util.js";
import { onNavigate, untracked } from "./nav.js";

let card = null;
let owner = null;
let pinned = false;
let touchAt = -Infinity; // when a touch last went down on an element with a card

const TOUCH_MS = 1000; // the mouse events a browser makes up after a touch come within this

function hide() {
	if (card) card.remove();
	card = null;
	owner = null;
	pinned = false;
}

onNavigate(hide);

// A touch outside the pinned card (and outside its element, whose own tap closes it) closes it.
untracked(() => {
	document.addEventListener(
		"pointerdown",
		(ev) => {
			if (!pinned || !card) return;
			const t = ev.target;
			if (card.contains(t) || (owner && owner.contains(t))) return;
			hide();
		},
		true
	);
});

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

/** Whether a click on el does something of its own: a button or a link that can be used. */
function acts(el) {
	if (el.disabled) return false;
	return el.tagName === "BUTTON" || el.tagName === "A" || el.getAttribute("role") === "button";
}

/**
 * Shows the card build() returns while the pointer is over `el` or `el` has the
 * focus, or, on a touch screen, from a tap on `el` until a touch elsewhere. build is
 * called each time, so the card is always up to date. A click closes it, unless
 * keepOnClick: a click on a tile that stays where it is.
 */
export function hoverCard(el, build, { keepOnClick = false } = {}) {
	const show = (pin) => {
		hide();
		if (!el.isConnected) return;
		const content = build();
		if (!content) return;
		card = h("div", { class: "hover-card" + (pin ? " pinned" : ""), role: "tooltip" }, content);
		owner = el;
		pinned = pin;
		document.body.append(card);
		place(el, card);
	};
	const touched = () => performance.now() - touchAt < TOUCH_MS;
	el.addEventListener("pointerdown", (ev) => {
		if (ev.pointerType === "touch" || ev.pointerType === "pen") touchAt = performance.now();
	});
	// After a touch the browser makes up a hover and a focus: the tap itself decides.
	el.addEventListener("mouseenter", () => {
		if (!touched()) show(false);
	});
	el.addEventListener("focus", () => {
		if (!touched()) show(false);
	});
	el.addEventListener("mouseleave", () => {
		if (owner === el && !pinned) hide();
	});
	el.addEventListener("blur", () => {
		if (owner === el && !pinned) hide();
	});
	el.addEventListener("click", () => {
		if (touched()) {
			// A second tap closes the card it opened.
			if (owner === el && pinned) return hide();
			if (acts(el) && !keepOnClick) {
				if (owner === el) hide();
				return;
			}
			show(true);
			return;
		}
		// A redraw may remove the element under the pointer: the card goes too.
		if (!keepOnClick && owner === el) hide();
	});
}

/** Removes any open card (the page was redrawn). */
export function hideHoverCard() {
	hide();
}
