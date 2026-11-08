// Hive's page script (ADR 0124): the insect tiles the board draws, and the
// games list's picture. The page asks for nothing else of Hive.

import { s } from "/static/js/util.js";
import { addPieceShapes, pieceShape } from "/static/js/render.js";

const GLYPHS = { queen: "🐝", beetle: "🪲", spider: "🕷", grasshopper: "🦗", ant: "🐜", mosquito: "🦟", ladybug: "🐞", pillbug: "🐛" };

/** An insect's tile: a pointy-topped hexagon edged in the insect's colour (page.css), with its glyph. */
function tile(insect) {
	return (size) => {
		const pts = [];
		for (let k = 0; k < 6; k++) {
			const a = (Math.PI / 180) * (60 * k - 30);
			pts.push(`${(0.5 * size * Math.cos(a)).toFixed(2)},${(0.5 * size * Math.sin(a)).toFixed(2)}`);
		}
		return [
			s("polygon", { class: `piece-body bug-${insect}`, points: pts.join(" "), "stroke-linejoin": "round" }),
			s("text", { class: "bug-glyph", x: 0, y: 0.03 * size, "text-anchor": "middle", "dominant-baseline": "central", "font-size": 0.56 * size }, GLYPHS[insect]),
		];
	};
}

addPieceShapes(Object.fromEntries(Object.keys(GLYPHS).map((insect) => [`bug-${insect}`, tile(insect)])));

/** The games list's picture: a small hive of tiles, the Queen in the middle and some of each insect round her. */
export function listIcon() {
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
