// Council's page script (tests/testdata/council): one part, so the tests see
// a game's own script loaded and asked (ADR 0124).

import { s } from "/static/js/util.js";

/** The games list's picture: four houses round a hall. */
export function listIcon() {
	const svg = s("svg", { viewBox: "0 0 100 100", role: "img", "aria-label": "Council: four houses round a hall", class: "council-icon" });
	svg.append(s("rect", { x: 30, y: 30, width: 40, height: 40, fill: "#d8c9a3" }));
	for (const [x, y, c] of [[50, 12, "#c62f2f"], [88, 50, "#2f5fc6"], [50, 88, "#2f8f4f"], [12, 50, "#d0a020"]]) {
		svg.append(s("circle", { cx: x, cy: y, r: 9, fill: c }));
	}
	return svg;
}
