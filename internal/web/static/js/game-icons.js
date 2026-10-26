// Pictures for the games list for games whose board makes a poor icon: Cthulhu
// Wars (a world map) and Hive (no board at all). Each is a small SVG built from
// the pieces the game itself draws.

import { s } from "./util.js";
import { glyphShape, pieceShape } from "./render.js";

/** Cthulhu Wars: the sigils of the four core cults. */
function cthulhuIcon() {
	const svg = s("svg", { viewBox: "0 0 200 200", role: "img", "aria-label": "Cthulhu Wars: the sigils of Great Cthulhu, the Crawling Chaos, the Black Goat and the Yellow Sign" });
	svg.append(s("rect", { x: 4, y: 4, width: 192, height: 192, rx: 18, fill: "#1d2a33", stroke: "#8a6a43", "stroke-width": 4 }));
	const at = [
		["gc", 54, 54],
		["cc", 146, 54],
		["bg", 54, 146],
		["ys", 146, 146],
	];
	for (const [key, x, y] of at) {
		const g = glyphShape(key, 78, "game-icon-sigil");
		if (!g) continue;
		svg.append(s("circle", { cx: x, cy: y, r: 42, fill: "#0e171d", stroke: "#8a6a43", "stroke-width": 2 }), s("g", { transform: `translate(${x} ${y})` }, g));
	}
	return svg;
}

/** Hive: a small hive of tiles, the Queen in the middle and some of each insect round her. */
function hiveIcon() {
	const svg = s("svg", { viewBox: "-60 -60 120 120", role: "img", "aria-label": "Hive: a cluster of insect tiles" });
	const size = 40;
	const pitch = (Math.sqrt(3) / 2) * size * 1.06;
	// Pointy-topped tiles: the six neighbours of the middle one.
	const tiles = [
		["queen", "white", 0, 0],
		["ant", "black", pitch, 0],
		["beetle", "white", pitch / 2, pitch * 0.866],
		["spider", "black", -pitch / 2, pitch * 0.866],
		["grasshopper", "white", -pitch, 0],
		["ladybug", "black", -pitch / 2, -pitch * 0.866],
		["mosquito", "white", pitch / 2, -pitch * 0.866],
	];
	for (const [type, color, x, y] of tiles) {
		svg.append(s("g", { class: `piece color-${color}`, transform: `translate(${x.toFixed(2)} ${y.toFixed(2)})` }, pieceShape(`bug-${type}`, size)));
	}
	return svg;
}

/** Nyet!: a fan of cards over the board of choices it is played on. */
function nyetIcon() {
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

const ICONS = { "cthulhu-wars": cthulhuIcon, hive: hiveIcon, nyet: nyetIcon };

/** The icon of a game of the list, or null when its board is the picture. */
export function gameIcon(id) {
	const make = ICONS[id];
	return make ? make() : null;
}
