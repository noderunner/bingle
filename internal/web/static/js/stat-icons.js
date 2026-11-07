// Pictograms of the counters every game's seats may show (ADR 0124): the cards
// in a hand. A game's page script draws its own beside them (statIcon:
// Cthulhu Wars' Power, Doom, Gates and the rest).

/** Path data of the pictograms, in a 24 × 24 box: fill, line and cut paths. */
const STAT_ICONS = {
	cards: {
		fill: ["M8.5 5.5h9a1.5 1.5 0 0 1 1.5 1.5v12a1.5 1.5 0 0 1-1.5 1.5h-9A1.5 1.5 0 0 1 7 19V7a1.5 1.5 0 0 1 1.5-1.5z"],
		line: ["M5.2 16.6A1.5 1.5 0 0 1 4 15.1V5A1.5 1.5 0 0 1 5.5 3.5h8.1A1.5 1.5 0 0 1 15 5"],
	}
};

/** A counter's pictogram, or null for one this file has none of. */
export function statIcon(kind, label, px = 16) {
	const def = STAT_ICONS[kind];
	if (!def) return null;
	const NS = "http://www.w3.org/2000/svg";
	const svg = document.createElementNS(NS, "svg");
	svg.setAttribute("viewBox", "0 0 24 24");
	svg.setAttribute("width", String(px));
	svg.setAttribute("height", String(px));
	svg.setAttribute("class", `stat-icon stat-${kind}`);
	svg.setAttribute("role", "img");
	svg.setAttribute("aria-label", label || kind);
	const add = (d, cls) => {
		const p = document.createElementNS(NS, "path");
		p.setAttribute("d", d);
		p.setAttribute("class", cls);
		svg.append(p);
	};
	for (const d of def.fill || []) add(d, "stat-fill");
	for (const d of def.line || []) add(d, "stat-line");
	for (const d of def.cut || []) add(d, "stat-cut");
	return svg;
}
