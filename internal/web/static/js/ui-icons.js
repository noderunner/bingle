// The small line icons of the interface: the buttons of the bar at the top of
// every page, and the rule book that stands for the word "Rules". They are
// drawn as SVG paths in the current text colour, so they follow the theme and
// need no files. Each takes a size in pixels (20 by default).
//
// Pictures of the games themselves are elsewhere: js/game-icons.js (the cards
// of the games list), js/icons.js (the units and counters of Cthulhu Wars),
// js/choice-icons.js (a board of choices) and js/animals.js (the players of a
// card game).

import { s } from "./util.js";

/**
 * The frame every icon is drawn in. The name travels as data-icon, so a page
 * (or a test) can tell which picture a button carries.
 */
export function icon(name, size, ...paths) {
	return s(
		"svg",
		{
			viewBox: "0 0 24 24",
			width: size || 20,
			height: size || 20,
			fill: "none",
			stroke: "currentColor",
			"stroke-width": 1.8,
			"stroke-linecap": "round",
			"stroke-linejoin": "round",
			"aria-hidden": "true",
			class: "ui-icon",
			"data-icon": name,
		},
		...paths
	);
}

/** A game controller: the games this browser has going. */
export const gamesIcon = (size) =>
	icon(
		"games",
		size,
		s("path", { d: "M6.5 8h11a4 4 0 0 1 3.9 4.9l-.9 4a2.6 2.6 0 0 1-4.4 1.2L14 16.5h-4L7.9 18.1a2.6 2.6 0 0 1-4.4-1.2l-.9-4A4 4 0 0 1 6.5 8z" }),
		s("path", { d: "M8 10.8v3.4M6.3 12.5h3.4" }),
		s("circle", { cx: 15.6, cy: 11.6, r: 0.5 }),
		s("circle", { cx: 17.6, cy: 13.4, r: 0.5 })
	);

/**
 * Two plugs coming together: joining a game. (It used to be a door with an
 * arrow through it, which reads as *leaving*.) A plug on the left with two
 * pins, a socket on the right with two sleeves, and the gap closing between
 * them.
 */
export const joinIcon = (size) =>
	icon(
		"plugs",
		size,
		// The plug: its body, its lead, and the two pins reaching right.
		s("path", { d: "M3 9.5h3.2v5H3z" }),
		s("path", { d: "M6.2 10.6h3.1M6.2 13.4h3.1" }),
		// The socket: its body, its lead, and the two sleeves reaching left.
		s("path", { d: "M21 9.5h-3.2v5H21z" }),
		s("path", { d: "M17.8 10.6h-3.1M17.8 13.4h-3.1" }),
		// The join itself.
		s("path", { d: "M11.4 12h1.2", "stroke-width": 2.6 })
	);

/** A microphone: a voice call. */
export const micIcon = (size) => icon("mic", size, s("rect", { x: 9, y: 3, width: 6, height: 11, rx: 3 }), s("path", { d: "M5.5 11a6.5 6.5 0 0 0 13 0M12 17.5V21" }));

/** A question mark in a circle: help. */
export const helpIcon = (size) => icon("help", size, s("circle", { cx: 12, cy: 12, r: 9 }), s("path", { d: "M9.4 9.4a2.7 2.7 0 0 1 5.2 1c0 1.8-2.6 2.2-2.6 4M12 17.4v.1" }));

/** A head and shoulders: your profile. */
export const profileIcon = (size) => icon("profile", size, s("circle", { cx: 12, cy: 8, r: 3.6 }), s("path", { d: "M4.5 20c.8-4 3.7-6 7.5-6s6.7 2 7.5 6" }));

/** Two heads: the people who are here. */
export const peopleIcon = (size) =>
	icon(
		"people",
		size,
		s("circle", { cx: 9, cy: 8.5, r: 3.1 }),
		s("path", { d: "M3 19c.7-3.4 3-5.2 6-5.2s5.3 1.8 6 5.2" }),
		s("path", { d: "M15.5 5.8a3.1 3.1 0 0 1 0 5.9M17 13.9c2.2.5 3.6 2.2 4 5.1" })
	);

/**
 * A rule book: a closed book with a ribbon, which stands for the word "Rules"
 * on every button that opens a game's rules.
 */
export const rulesIcon = (size) =>
	icon(
		"rules",
		size,
		s("path", { d: "M5 4.8A1.8 1.8 0 0 1 6.8 3h10.4A1.8 1.8 0 0 1 19 4.8v14.4a1.8 1.8 0 0 1-1.8 1.8H6.8A1.8 1.8 0 0 1 5 19.2z" }),
		s("path", { d: "M8.2 3v7.2l1.9-1.4 1.9 1.4V3" }),
		s("path", { d: "M8.4 14h7.2M8.4 17h4.8" })
	);
