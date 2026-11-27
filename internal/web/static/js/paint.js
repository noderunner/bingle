// The page's standard look of a color (ADR 0154): the one place that turns a
// color, the name the rules give a set of pieces, into what this page draws it
// in. Callers pass names; no other module of the page reasons about hue.
//
// A color whose name is one of the color words below is painted as that word
// says: the class color-<word>, which the stylesheet (style.css's
// --piece-<word>-fill and -stroke) and every board theme (themes.js's stones)
// paint. Any other color is painted by seat: the game in view gives it the
// paint of a word it does not play, in the order of the seats that play its
// colors and then of its extra_colors, so two unknown colors never look the
// same and each follows the board theme as the words do. What a screen reader
// hears still names the color as the definition gives it.
//
// Wherever the page names a seat, its key stands beside the name (decision 5):
// seatKey's swatch in the viewer's paint of each color the seat plays, which
// a board theme repaints without a reload, with the colors' names for a
// tooltip and a screen reader.

import { h } from "./util.js";
import { t, lang } from "./i18n.js";

/** The color words the page has a paint for, the stylesheet's and every board theme's. */
export const COLOR_WORDS = Object.freeze(["white", "black", "red", "yellow", "orange", "green", "blue", "purple", "teal", "sand", "brown", "grey"]);

const KNOWN = new Set(COLOR_WORDS);

/** The words lent to colors the page does not know, in the order seats get them: the most unlike first, white and black last. */
const BY_SEAT = ["red", "blue", "green", "yellow", "purple", "orange", "teal", "brown", "grey", "sand", "white", "black"];

/** A name (a color's, a tint's, a badge's) as a word of a class or an id: lowercase letters and digits. */
export function classWord(name) {
	return String(name || "").toLowerCase().replace(/[^a-z0-9]/g, "") || "white";
}

/** The color word a color's name is, or null when the page has no paint for it. */
export function knownColor(color) {
	const c = classWord(color);
	return KNOWN.has(c) ? c : null;
}

/**
 * The word each color of a game that the page does not know is painted as, by
 * the seat that plays it (seats in their order, each seat's colors in its
 * order), then by its place among extra_colors: a word the game does not play
 * itself, so its paint differs from the game's other colors'.
 */
function bySeat(info) {
	const names = [];
	for (const s of [...((info && info.seats) || [])].sort((a, b) => a.seat - b.seat)) {
		for (const c of s.colors && s.colors.length ? s.colors : [s.color]) names.push(classWord(c));
	}
	for (const c of (info && info.extra_colors) || []) names.push(classWord(c));
	const lent = new Map();
	const played = new Set(names.filter((c) => KNOWN.has(c)));
	const free = BY_SEAT.filter((w) => !played.has(w));
	const pool = free.length ? free : BY_SEAT;
	for (const c of names) {
		if (!KNOWN.has(c) && !lent.has(c)) lent.set(c, pool[lent.size % pool.length]);
	}
	return lent;
}

let inView = null;
const lent = new WeakMap();

function lentWords(info) {
	if (!info || typeof info !== "object") return null;
	if (!lent.has(info)) lent.set(info, bySeat(info));
	return lent.get(info);
}

/** Tells the resolver which game the page shows, for the callers that do not pass one. */
export function useColors(info) {
	inView = info || null;
}

/**
 * The color word a color is painted as: its own when the page knows it,
 * otherwise the one the game (by default the game in view) lends it by seat.
 * A color the game does not list is given one by its name, the same each time.
 */
export function paintWord(color, info = inView) {
	const c = classWord(color);
	if (KNOWN.has(c)) return c;
	const word = (lentWords(info) || new Map()).get(c);
	if (word) return word;
	let h = 0;
	for (const ch of c) h = (Math.imul(h, 31) + ch.charCodeAt(0)) >>> 0;
	return BY_SEAT[h % BY_SEAT.length];
}

/** The class that paints an element in a color: color-<word>. */
export function colorClass(color, info = inView) {
	return `color-${paintWord(color, info)}`;
}

/** A color's paint as a CSS value, for a style that mixes it (a seat's ink, a gradient). */
export function colorPaint(color, info = inView) {
	return `var(--piece-${paintWord(color, info)}-fill)`;
}

/** A color's name in words: a color word in the page's language, any other as the definition gives it. */
export function colorName(color) {
	const c = knownColor(color);
	return c ? t(`color.${c}`) : String(color);
}

/** The seat of the game in view, by its number, or null. */
export function seatInView(seat) {
	return ((inView && inView.seats) || []).find((s) => s.seat === seat) || null;
}

/** The colors a seat plays: its colors, or its one color. */
export function seatColors(sd) {
	if (!sd) return [];
	return sd.colors && sd.colors.length ? sd.colors : sd.color ? [sd.color] : [];
}

/**
 * The key beside a seat's name: a swatch (or `cls`) in the viewer's paint of
 * the seat's color, and for a seat of several colors a gradient of them (the
 * web file's blend where it gives one), named by every color it plays. `title`
 * goes before the colors' names in the tooltip.
 */
export function seatKey(sd, info = inView, { cls = "swatch", title = "" } = {}) {
	const colors = seatColors(sd);
	const blend = sd && sd.blend && sd.blend.length > 1 ? sd.blend : colors.length > 1 ? colors : null;
	const names = new Intl.ListFormat(lang, { type: "unit" }).format(colors.map(colorName));
	return h("span", {
		class: `seat-key ${cls} ${colorClass(colors[0] || "white", info)}`,
		style: blend ? `background: linear-gradient(90deg, ${blend.map((c) => colorPaint(c, info)).join(", ")})` : null,
		role: "img",
		"aria-label": names || null,
		title: (title ? `${title} · ${names}` : names) || null,
		"data-colors": colors.join(" ") || null,
	});
}
