// Bowling Solitaire's page script (ADR 0124): the games list's picture, since
// its table makes a poor icon.

import { s } from "/static/js/util.js";

/** The games list's picture: ten pins in a triangle on the green cloth, and the three balls below. */
export function listIcon() {
	const svg = s("svg", { viewBox: "0 0 200 200", role: "img", "aria-label": "Bowling Solitaire: ten cards set out as pins, and three piles of balls" });
	svg.append(s("rect", { x: 4, y: 4, width: 192, height: 192, rx: 18, fill: "#2f6f5e", stroke: "#8a6a43", "stroke-width": 4 }));
	const W = 28;
	const H = 38;
	const card = (x, y, color, rank) => {
		const g = s("g", { transform: `translate(${x} ${y})` });
		g.append(s("rect", { width: W, height: H, rx: 4, fill: "#f8f4ea", stroke: "#3a3631", "stroke-width": 1.5 }));
		g.append(s("text", { x: W / 2, y: H / 2, "text-anchor": "middle", "dominant-baseline": "central", "font-size": 16, "font-weight": 800, fill: color, "font-family": "system-ui, sans-serif" }, rank));
		svg.append(g);
	};
	const back = (x, y) => svg.append(s("rect", { x, y, width: W, height: H, rx: 4, fill: "#5b4a8a", stroke: "#f8f4ea", "stroke-width": 1.5 }));
	const red = "#c62f2f";
	const black = "#1f2630";
	// The pins, four at the back down to the head pin.
	const rows = [
		[[red, "7"], [black, "8"], [red, "9"], [black, "10"]],
		[[black, "4"], [red, "5"], [black, "6"]],
		[[red, "2"], [black, "3"]],
		[[red, "A"]],
	];
	rows.forEach((row, i) => {
		const width = row.length * W + (row.length - 1) * 6;
		row.forEach(([color, rank], j) => card(100 - width / 2 + j * (W + 6), 8 + i * 31, color, rank));
	});
	// The balls: three piles of three, two and one, each with its top card up.
	[[3, black, "6"], [2, red, "4"], [1, black, "9"]].forEach(([n, color, rank], k) => {
		const x = 46 + k * 40;
		for (let i = 0; i < n - 1; i++) back(x, 150 - (n - 1 - i) * 4);
		card(x, 150, color, rank);
	});
	return svg;
}
