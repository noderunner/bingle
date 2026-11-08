// Nyet!'s page script (ADR 0124): the pictures of its board of terms, its
// players' animals, and the games list's picture.

import { s } from "/static/js/util.js";
import { animalIcon, animalOf } from "./animals.js";

export { choiceIcon } from "./choice-icons.js";

/** The picture of a player, by the seat's label (an animal: "Crane"); null for a label that is none. */
export function seatIcon(label, { color = "", size = 22, title = "" } = {}) {
	return animalOf(label) ? animalIcon(label, { color, size, title }) : null;
}

/** The games list's picture: a fan of cards over the board of choices it is played on. */
export function listIcon() {
	const svg = s("svg", { viewBox: "0 0 200 200", role: "img", "aria-label": "Nyet!: a fan of playing cards" });
	svg.append(s("rect", { x: 4, y: 4, width: 192, height: 192, rx: 18, fill: "#2f6f5e", stroke: "#8a6a43", "stroke-width": 4 }));
	const cards = [
		[-32, "#d6453a", "7"],
		[-11, "#3f6fd0", "3"],
		[11, "#3ea86a", "9"],
		[32, "#ecc13a", "5"],
	];
	for (const [angle, color, rank] of cards) {
		const card = s("g", { transform: `translate(100 168) rotate(${angle}) translate(-30 -128)` });
		card.append(s("rect", { width: 60, height: 90, rx: 8, fill: "#f8f4ea", stroke: "#3a3631", "stroke-width": 3 }));
		card.append(s("rect", { x: 7, y: 7, width: 46, height: 76, rx: 5, fill: "none", stroke: color, "stroke-width": 2.5 }));
		card.append(s("text", { x: 30, y: 46, "text-anchor": "middle", "dominant-baseline": "central", "font-size": 30, "font-weight": 800, fill: color, "font-family": "system-ui, sans-serif" }, rank));
		svg.append(card);
	}
	return svg;
}
