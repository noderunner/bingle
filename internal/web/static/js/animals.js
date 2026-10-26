// The animals of Nyet!'s players — Wolf, Crane, Tiger, Bear and Eagle — as small
// icons in the colour of their seat. They stand for the players wherever a
// player is named in a picture: the seat strips, under the cards of a trick,
// on the boxes of the board that a player crossed off.
//
// Each icon is a head drawn in a 32×32 box: the body takes the seat's colour
// (the same theme-aware colours as the squares and pieces of other games), the
// details a darker ink.

import { s } from "./util.js";

/** The animal of a player: the seat's label, as the game names it ("Crane"). */
const ANIMALS = {
	wolf: [
		// Head with pointed ears and a narrow muzzle.
		["body", "M4.5 3.5 11 9c3.2-1 6.8-1 10 0l6.5-5.5V15c0 5.2-3 9.2-7.2 11.2L16 30.2l-4.3-4C7.5 24.2 4.5 20.2 4.5 15z"],
		["inner", "M7.7 8.4 10 11.2 7.7 14.5zM24.3 8.4 22 11.2l2.3 3.3z"],
		["muzzle", "M16 19.5c2.4 0 4.6 1.8 5 4.2L16 28.4l-5-4.7c.4-2.4 2.6-4.2 5-4.2z"],
		["ink", "M8.8 15.6l4.7 1.6M23.2 15.6l-4.7 1.6", "line"],
		["ink", "M13.8 22.6h4.4L16 25.4z"],
	],
	crane: [
		// Head in profile on a long S-curved neck, a long beak, a red crown and a dark cheek.
		["body", "M14.2 4.6c2.8-1.2 5.6.4 5.8 3.6.2 2.4-.8 3.6-1.4 5.2-.8 2.2.4 4 2.2 6.2 1.6 2 3 4.2 3 7.4H11.6c.2-3 1.4-5 2.8-7 1.2-1.8 1.6-3.2.6-4.6-1-1.4-2.2-2.2-2.6-4L2.4 11.6l8.8-3.4c.4-1.6 1.4-2.8 3-3.6z"],
		["crown", "M13.6 5.2c1.8-1 4-.6 5 .8l-.6 2.2c-1.4-.6-3.2-.6-4.8.2z"],
		["ink", "M16.6 10.2c1.2 1.6 1.8 3.4 1.6 5.4-.6-1.8-1.6-3.2-2.8-4z"],
		["ink", "M2.4 11.6l8.6-3.2 0 2.6z"],
		["eye", "M14 8.4a1.3 1.3 0 1 0 .01 0z"],
	],
	tiger: [
		// Broad round face, round ears and stripes.
		["body", "M16 5.6c6.8 0 11.6 4.6 11.6 10.8 0 6.4-4.8 11.4-11.6 11.4S4.4 22.8 4.4 16.4C4.4 10.2 9.2 5.6 16 5.6zM3.6 7.6a4 4 0 0 1 6.6-2.8L7 9.8zM28.4 7.6a4 4 0 0 0-6.6-2.8L25 9.8z"],
		["muzzle", "M16 17.6c3 0 5.6 1.8 5.6 4.4 0 2.8-2.4 4.4-5.6 4.4s-5.6-1.6-5.6-4.4c0-2.6 2.6-4.4 5.6-4.4z"],
		["ink", "M16 5.6v4.4M12.6 6.4l.8 3.6M19.4 6.4l-.8 3.6M4.8 14l4 1.2M4.8 18.4l4-.2M27.2 14l-4 1.2M27.2 18.4l-4-.2", "line"],
		["ink", "M13.4 18.4h5.2L16 21.4z"],
		["ink", "M16 21.4v2.4M13 24.2c1.2.8 2.2.8 3 0 .8.8 1.8.8 3 0", "line"],
		["eye", "M10.6 14a1.3 1.3 0 1 0 .01 0zM21.4 14a1.3 1.3 0 1 0 .01 0z"],
	],
	bear: [
		// Round head, round ears, a pale muzzle.
		["body", "M16 6.8c6.4 0 11 4.4 11 10.6 0 6.4-4.8 10.8-11 10.8S5 23.8 5 17.4C5 11.2 9.6 6.8 16 6.8zM2.8 9.2a4.4 4.4 0 1 1 8.4-2.2L6.4 11.6zM29.2 9.2a4.4 4.4 0 1 0-8.4-2.2l4.8 4.6z"],
		["inner", "M5.4 8.4a1.7 1.7 0 1 1 3.2-.6l-2.2 2.2zM26.6 8.4a1.7 1.7 0 1 0-3.2-.6l2.2 2.2z"],
		["muzzle", "M16 17.6c3.4 0 6 2 6 4.8 0 2.6-2.6 4.4-6 4.4s-6-1.8-6-4.4c0-2.8 2.6-4.8 6-4.8z"],
		["ink", "M13.4 18.8h5.2l-2.6 2.6z"],
		["ink", "M16 21.4v2.2M13.2 24c1.2.8 2.2.8 2.8 0 .6.8 1.6.8 2.8 0", "line"],
		["eye", "M10.8 14.6a1.4 1.4 0 1 0 .01 0zM21.2 14.6a1.4 1.4 0 1 0 .01 0z"],
	],
	eagle: [
		// Head in profile: a heavy brow over the eye, a big hooked beak, ruffled neck feathers.
		["body", "M17 3.6c5.8-.2 10 3.8 10.2 9.6.1 3-.8 5.4-2.4 7.6l3 3.4-4.2.4 1.8 3.6-5.2-1.4-1.4 3-3.6-3.2-3.6.6 1.2-3.6c-2.6-1.4-4-3.8-4-6.8 0-5.8 3.8-10.8 9.2-11.2z"],
		["beak", "M10.8 9.6C6.6 9.4 3 11.8 2.2 16c-.4 2.4.6 4.4 2.6 5.2.2-1.6 1.2-2.8 2.8-3.2.4 1.8 1.8 3 3.6 3 .6-3.2.4-7-.4-11.4z"],
		["ink", "M9.4 12.4c1.6 0 3.4 1.2 3.8 2.8", "line"],
		["ink", "M10.4 7.8c2.8-1.6 6.6-2 9.6-.4l-.6 2.6c-2.4-1-5.2-.8-8 .6z"],
		["eye", "M14.6 12.6a1.8 1.8 0 1 0 .01 0z"],
		["inner", "M22 12c2 1.2 3.4 3.4 3.6 6-2.2-1-3.6-3.2-3.6-6zM20.4 20c2.2.2 3.8 1.6 4.4 3.6-2.2-.2-3.8-1.4-4.4-3.6z"],
	],
};

/** The animal's key in this module for a seat label ("Crane" → "crane"), or null. */
export function animalOf(label) {
	const key = String(label || "").trim().toLowerCase();
	return ANIMALS[key] ? key : null;
}

/**
 * The icon of an animal in the colour of a seat (a colour name of the game's
 * seats: red, blue, green, yellow, orange…). The icon carries its own title.
 */
export function animalIcon(name, { color = "", size = 22, title = "" } = {}) {
	const parts = ANIMALS[animalOf(name)];
	const svg = s("svg", {
		viewBox: "0 0 32 32",
		width: size,
		height: size,
		class: `animal animal-${animalOf(name)} color-${String(color).toLowerCase()}`,
		role: "img",
		"aria-label": title || name,
	});
	if (title) svg.append(s("title", {}, title));
	for (const [part, d, kind] of parts) {
		const attrs = { d, class: `a-${part}${kind ? " a-" + kind : ""}` };
		svg.append(s("path", attrs));
	}
	return svg;
}
