// Solitaire's page script (ADR 0124): the games list's picture, since a
// patience's layout makes a poor icon.

import { s } from "/static/js/util.js";

/** The games list's picture: seven columns on the green cloth, and the ace on its way home. */
export function listIcon() {
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
