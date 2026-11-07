// Pictures for the games list for games whose board makes a poor icon: Hive
// (no board at all) and the card games. Each is a small SVG built from the
// pieces the game itself draws. A game with a page script draws its own
// (info.page.listIcon: Cthulhu Wars' world map makes a poor icon too).

import { s } from "./util.js";
import { pieceShape } from "./render.js";

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

/** Solitaire: seven columns on the green cloth, and the ace on its way home. */
function solitaireIcon() {
	const svg = s("svg", { viewBox: "0 0 200 200", role: "img", "aria-label": "Solitaire: columns of playing cards" });
	svg.append(s("rect", { x: 4, y: 4, width: 192, height: 192, rx: 18, fill: "#2f6f5e", stroke: "#8a6a43", "stroke-width": 4 }));
	const card = (x, y, color, rank) => {
		const g = s("g", { transform: `translate(${x} ${y})` });
		g.append(s("rect", { width: 42, height: 60, rx: 5, fill: "#f8f4ea", stroke: "#3a3631", "stroke-width": 2 }));
		g.append(s("text", { x: 21, y: 21, "text-anchor": "middle", "dominant-baseline": "central", "font-size": 24, "font-weight": 800, fill: color, "font-family": "system-ui, sans-serif" }, rank));
		svg.append(g);
	};
	const back = (x, y) => svg.append(s("rect", { x, y, width: 42, height: 60, rx: 5, fill: "#5b4a8a", stroke: "#f8f4ea", "stroke-width": 2 }));
	card(106, 18, "#c62f2f", "A");
	card(152, 18, "#1f2630", "A");
	back(14, 92);
	card(14, 110, "#c62f2f", "K");
	back(60, 92);
	back(60, 106);
	card(60, 120, "#1f2630", "9");
	back(106, 92);
	card(106, 110, "#c62f2f", "6");
	card(152, 92, "#1f2630", "Q");
	card(152, 116, "#c62f2f", "J");
	return svg;
}

const ICONS = { hive: hiveIcon, nyet: nyetIcon, solitaire: solitaireIcon };

/** The icon of a game of the list, or null when its board is the picture. */
export function gameIcon(id) {
	const make = ICONS[id];
	return make ? make() : null;
}
