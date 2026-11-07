// Cthulhu Wars' part of the web page (ADR 0124): the page imports this module
// through the game's web.yaml and asks its exports for what it draws for this
// game alone. Nothing in the shared page names the game for these.

import { h, s } from "/static/js/util.js";
import { addPieceShapes } from "/static/js/render.js";
import { FACTION_KEYS, factionName } from "./cw-names.js";
import { UNIT_ICONS } from "./cw-icons.js";
import { glyphShape, legendMarker as mapMarker } from "./cw-map.js";

// The units are drawn after the miniatures, wherever the page draws a piece.
addPieceShapes(UNIT_ICONS);

export { renderAreaMap } from "./cw-map.js";

/** A mark of the map for the rules window's key; a Gate shows the unit that holds one. */
export function legendMarker(kind, arg, info) {
	if (kind !== "gate") return mapMarker(kind, arg, info);
	const holder = (info.pieces || []).find((p) => p.type === "cultist");
	return mapMarker("gate", arg, info, holder ? holder.shape : "");
}

/** The glyph of the faction a seat plays, beside its name, or null. */
export function seatGlyph(label) {
	const key = FACTION_KEYS.find((k) => factionName(k) === label);
	if (!key) return null;
	return h("img", { class: "seat-glyph", src: `/api/game-defs/cthulhu-wars/assets/guide/glyphs/${key}.svg`, alt: label, title: label, width: 22, height: 22, decoding: "sync" });
}

/** The rules window's key to the marks of the map, as Markdown. */
export function mapKey(info) {
	const d = info.display || {};
	const parts = [];
	const tint = (k, fallback) => ((d.tints || {})[k] || {}).color || fallback;
	const first = (info.seats || [])[0];
	const color = first ? first.color : "green";
	parts.push(
		"# Key",
		"",
		"What the marks on the map mean.",
		"",
		"| Mark | Meaning |",
		"|---|---|",
		`| ![Ocean](marker:swatch:${tint("ocean", "#b9d3e6")}) | **Ocean** — areas of water. Only some units may enter them. |`,
		`| ![Land](marker:swatch:${tint("land", "#e5dcc0")}) | **Land**. |`,
		`| ![Desecrated](marker:swatch:${tint("desecrated", "#d8c76a")}) | **Desecrated area** (Yellow Sign): marked with ✠. |`,
		`| ![Gate](marker:gate:${color}) | **A Gate.** The ring is the colour of the cult that controls it; the figure drawn on it is the unit that occupies it — an Acolyte, a High Priest or a Dark Young. To give a Gate up, move that unit (also to the area it stands in). |`,
		"| ![Abandoned Gate](marker:gate:abandoned) | **An abandoned Gate:** a dashed grey ring. It gives 1 Power to every player in Gather Power and scores nothing; a Cultist that arrives takes it. |",
		"| ![Glyph](marker:glyph:chevron) | **A glyph** (chevron, dragon or thorns) beside an area's name, or where the board prints it: what Yellow Sign must Desecrate for its requirements. |",
		"| ![Great Cthulhu glyph](marker:sigil:gc) | **A faction glyph** (here Great Cthulhu's), left of an area's name: the area where that cult begins. Every cult at the table has its own; they are shown with each faction in the Factions page. |",
		"| ![Windwalker glyph](marker:sigil:ww) | **The Windwalker's glyph** marks the two polar areas where it may begin, and where Rhan-Tegoth and Ithaqua awaken. |",
		"| ![Sleeper glyph](marker:sigil:sl) | **The Sleeper's glyph:** where it begins. |",
		"| ![Ice Age](marker:iceage) | **The Ice Age token** (Windwalker): enemy Actions that end here cost 1 Power more. |",
		"",
		"A red outline round an area is the battle being fought. In the game options the area names, glyphs and sigils (the faction glyphs) can be switched off."
	);
	return parts.join("\n");
}

/** The games list's picture: the sigils of the four core cults. */
export function listIcon() {
	return cthulhuIcon();
}

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
