// Themed sets of cards. A set replaces how a standard deck looks and nothing about
// how it plays: its own back, eight suits and four jokers, its own palette and its
// own lettering for the ranks, all drawn here as vector art. Any game whose deck is
// plain suits and ranks can wear any set. The set is chosen when the game is set up
// and belongs to the table (ADR 0085): everyone sees the same suits, so "the frogs
// are trump" means the same thing to all of them. The server keeps the choice and
// knows the ids below (catalog.CardSetIDs).
//
//   * A game with fewer than eight suits uses the first suits of the set, in the
//     game's own order of suits: a poker deck wears suits one to four.
//   * The ranks are the game's, whatever it calls them: card-lettering.js letters
//     digits and capitals. Numbers from 2 to 10 lay out that many pips, 1 or A one
//     large one, and anything else is a framed court card.
//   * Jokers (a card with the tag "joker") belong to no suit. Each of the four has
//     the colours of two suits: suits one and two, three and four, five and six,
//     seven and eight. A game's first joker is the first of the set, its second the
//     second, and so on round.
//
// Icons are drawn in a 24×24 box from a list of layers: [kind, path data, width].
//   f  filled in the suit's colour        l  a line in the suit's colour
//   g  filled in the second colour        m  a line in the second colour
//   k  filled in the colour of the paper  x  a line in the colour of the paper
// A capital kind is drawn twice, the second time mirrored about the middle.

import { s } from "./util.js";
import { canLetter, lettering } from "./card-lettering.js";
import { LAYOUTS, rankNumber } from "./card-art.js";

// ---------------------------------------------------------------------------
// Path helpers
// ---------------------------------------------------------------------------

const n1 = (v) => (Math.round(v * 100) / 100).toString();

/** A circle as path data. */
const o = (cx, cy, r) => `M${n1(cx - r)} ${n1(cy)}a${n1(r)} ${n1(r)} 0 1 0 ${n1(2 * r)} 0a${n1(r)} ${n1(r)} 0 1 0 ${n1(-2 * r)} 0z`;

/** An ellipse, turned `rot` degrees about its centre, as path data. */
const el = (cx, cy, rx, ry, rot = 0) => {
	const a = (rot * Math.PI) / 180;
	const dx = rx * Math.cos(a);
	const dy = rx * Math.sin(a);
	return `M${n1(cx - dx)} ${n1(cy - dy)}A${n1(rx)} ${n1(ry)} ${rot} 1 0 ${n1(cx + dx)} ${n1(cy + dy)}A${n1(rx)} ${n1(ry)} ${rot} 1 0 ${n1(cx - dx)} ${n1(cy - dy)}z`;
};

/** A star of n points between the radii ro and ri, pointing up. */
function star(cx, cy, n, ro, ri, rot = 0) {
	const pts = [];
	for (let k = 0; k < 2 * n; k++) {
		const r = k % 2 ? ri : ro;
		const a = -Math.PI / 2 + (rot * Math.PI) / 180 + (Math.PI * k) / n;
		pts.push(`${n1(cx + r * Math.cos(a))} ${n1(cy + r * Math.sin(a))}`);
	}
	return `M${pts.join("L")}z`;
}

/** n spokes round a point, from radius r0 to r1. */
function spokes(cx, cy, n, r0, r1, rot = 0) {
	let d = "";
	for (let k = 0; k < n; k++) {
		const a = ((rot + (360 * k) / n) * Math.PI) / 180;
		d += `M${n1(cx + r0 * Math.cos(a))} ${n1(cy + r0 * Math.sin(a))}L${n1(cx + r1 * Math.cos(a))} ${n1(cy + r1 * Math.sin(a))}`;
	}
	return d;
}

/** A spiral from radius r0 out to r1, over this many turns. */
function spiral(cx, cy, r0, r1, turns, rot = 0) {
	let d = "";
	const steps = Math.round(turns * 24);
	for (let k = 0; k <= steps; k++) {
		const t = k / steps;
		const r = r0 + (r1 - r0) * t;
		const a = rot + t * turns * 2 * Math.PI;
		d += `${k ? "L" : "M"}${n1(cx + r * Math.cos(a))} ${n1(cy + r * Math.sin(a))}`;
	}
	return d;
}

const dots = (...p) => p.map(([x, y, r]) => o(x, y, r)).join("");

// ---------------------------------------------------------------------------
// Ancient Civilizations
// ---------------------------------------------------------------------------

const ANCIENT = {
	id: "ancient",
	label: "Ancient Civilizations",
	paper: "#f3e6c4",
	ink: "#3a2a12",
	edge: "#a98a4e",
	trim: { rx: 2, dash: "1 2.5" },
	pen: { width: 1.45, serif: 1.3 }, // cut in stone: straight strokes with serifs
	suits: [
		{ label: "Ankhs", of: "Egypt", color: "#1b4a8f", art: [["l", "M12 11C7.6 7.8 8.4 2.2 12 2.2S16.4 7.8 12 11", 2.4], ["l", "M5.4 12.6H18.6", 2.8], ["l", "M12 11V22", 3]] },
		{ label: "Amphorae", of: "Greece", color: "#b33a1a", art: [["f", "M9 2.5h6v1.8c0 1 .4 1.7 1 2.3 1.8 1.8 2.8 4 2.8 6.4 0 3.4-2.2 5.8-4.6 7.4L13 22h-2l-1.2-1.6C7.4 18.8 5.2 16.4 5.2 13c0-2.4 1-4.6 2.8-6.4.6-.6 1-1.3 1-2.300z"], ["L", "M9 4.800C4.4 4.8 3.2 8.2 6.2 10.6", 1.5], ["x", "M5.6 11.600H18.400M5.6 15H18.4", 1], ["x", "M7.5 13.300h1.800M11.1 13.300h1.800M14.7 13.300h1.8", 1.4]] },
		{ label: "Ziggurats", of: "Mesopotamia", color: "#8a5e00", art: [["f", "M9.5 3h5v3.600H17v3.800h2.600V15H22v6H2v-6h2.400v-4.600H7V6.600h2.500z"], ["x", "M12 21V7", 1.6], ["x", "M10.7 18.500h2.600M10.7 15.500h2.600M10.7 12.500h2.600M10.7 9.500h2.6", .9]] },
		{ label: "Arches", of: "Rome", color: "#0f6b52", art: [["f", "M2 2.500h20v2.700h-1V20h1.500v2H1.500v-2H3V5.200H2z"], ["k", "M8 20v-7.500a4 4 0 0 1 8 0V20z"], ["x", "M5.8 7.400h12.4", 1.1]] },
		{ label: "Coins", of: "China", color: "#a31621", art: [["f", o(12, 12, 10.2)], ["k", "M8.8 8.800h6.400v6.400H8.800z"], ["x", o(12, 12, 8.3), .8], ["x", "M12 4.900v1.700M12 17.400v1.700M4.9 12h1.700M17.4 12h1.7", 1.2]] },
		{ label: "Suns", of: "the Maya", color: "#0b6e7c", art: [["f", star(12, 12, 10, 11.5, 6.8)], ["f", o(12, 12, 6.9)], ["k", dots([9.5, 10.8, 1.150], [14.5, 10.8, 1.150])], ["x", "M9 14.400Q12 16.8 15 14.4", 1.3]] },
		{ label: "Wheels", of: "India", color: "#6a2c7d", art: [["l", o(12, 12, 8.9), 2.4], ["f", o(12, 12, 2.7)], ["l", spokes(12, 12, 8, 2.5, 8.5), 1.3], ["f", dots([12, 1.7, 1.3], [12, 22.3, 1.3], [1.7, 12, 1.3], [22.3, 12, 1.3])]] },
		{ label: "Tumi Knives", of: "the Inca", color: "#2b2420", art: [["f", "M3 15h18c0 4.2-3.8 7-9 7s-9-2.8-9-7z"], ["f", "M10.3 15.500V7h3.400v8.500z"], ["f", "M7.5 7.800V3.500l2.3 1.700L12 1.800l2.2 3.4 2.3-1.700v4.300z"], ["k", dots([10.3, 6.1, .7], [13.7, 6.1, .7])]] },
	],
	jokers: [
		{ label: "Scarab", art: [["G", "M12 9.500C7.5 8.2 3.2 10.2 2 15c2.4-.200 4 1 5.2 3.200l4.8 2.800z"], ["f", o(12, 4.8, 2.6)], ["f", el(12, 13, 3, 8)], ["L", "M9.5 3.500L6.5 1.5", 1.3], ["x", "M12 8V21", 1]] },
		{ label: "Minotaur", art: [["G", "M7.5 9.200C3.8 8.8 1.4 5.8 2.2 1.800c1 2.8 3.4 4.4 6.2 4.800z"], ["f", "M6.8 9.400C6.8 7.2 9.2 5.5 12 5.500s5.2 1.7 5.2 3.900c0 3-1 4.4-1.6 7.4-.4 2.4-1.9 4.7-3.6 4.700s-3.2-2.3-3.6-4.700C7.8 13.8 6.8 12.4 6.8 9.400z"], ["K", o(9.3, 11, 1.2)], ["x", "M10.7 16.800h.1M13.3 16.800h.1", 1.3], ["m", "M10.2 18.700a1.9 1.9 0 0 0 3.6 0", 1.4]] },
		{ label: "Sphinx", art: [["g", "M12 1.800C7 1.8 5 4.8 4.5 9L2.8 20.500H8.500L9.5 16h5l1 4.500h5.700L19.5 9C19 4.8 17 1.8 12 1.800z"], ["k", "M4.8 12h1.600M4.4 15h1.600M4 18h1.600M19.2 12h-1.600M19.6 15h-1.600M20 18h-1.6"], ["x", "M5.2 10.500h1.800M4.8 13.300h1.800M4.3 16.100h1.800M18.8 10.500h-1.800M19.2 13.300h-1.800M19.7 16.100h-1.8", .9], ["f", "M8.3 6.800h7.400v5.200c0 2.8-1.7 5.4-3.7 5.400s-3.7-2.6-3.7-5.400z"], ["f", "M10.8 17h2.400v4.800h-2.400z"], ["K", el(10.2, 10.3, 1, .55)], ["x", "M11 14.200h2", 1], ["f", "M11 1l1 1.100L13 1v3.500h-2z"]] },
		{ label: "Feathered Serpent", art: [["l", "M6.5 6C12.5 2.5 19 4 19 8.500s-13 2.8-13 7.5 6.5 6 13 3.5", 3.2], ["f", o(6, 6, 3.5)], ["k", o(5.3, 5.3, .9)], ["g", "M19 2.500l3.5-1-1.3 3.600zM22 8.200l2-.2-1.6 3.200zM13 7.500l1.8-3 1.3 3.200zM9.5 13l-1.8-3-1.5 3.400zM5 18l-3 .500 2.4 2.500zM11 21l.5 2.8 2.6-2.300zM16.5 11l1.5 2.8 1.8-3z"], ["m", "M3 7.500L.5 10M4 9.500L3 12", 1]] },
	],
	back: ancientBack,
};

function ancientBack() {
	const gold = "#e0b85a";
	const g = [s("rect", { width: 100, height: 140, fill: "#123a6b" })];
	// A Greek key round the edge.
	const key = (x, y, flip) => s("path", { d: "M0 7V0H7V5H3V3", transform: `translate(${x} ${y}) scale(${flip ? -1 : 1} 1)`, fill: "none", stroke: gold, "stroke-width": 1.3, "stroke-linecap": "square" });
	for (let x = 6; x < 90; x += 9.8) g.push(key(x, 5, false), key(x, 128, false));
	for (let y = 15; y < 120; y += 9.8) g.push(key(5, y, false), key(88, y, false));
	g.push(s("rect", { x: 14.5, y: 14.5, width: 71, height: 111, rx: 2, fill: "#0e2d54", stroke: gold, "stroke-width": 1 }));
	// A winged sun, an eye of the sky over a step pyramid.
	const wing = "M50 54C40 47 28 46 19 49c3 2 5 3 9 4-3 0-5 0-7 1 4 2 8 3 12 3-2 1-3 1-5 2 8 2 15 1 22-2z";
	g.push(s("path", { d: wing, fill: gold }), s("path", { d: wing, fill: gold, transform: "matrix(-1 0 0 1 100 0)" }));
	g.push(s("circle", { cx: 50, cy: 52, r: 8.5, fill: "#d8562c", stroke: gold, "stroke-width": 1.5 }));
	g.push(s("path", { d: "M20 112h60v-6h-8v-7h-8v-7h-8v-7h-12v7h-8v7h-8v7h-8z", fill: gold }));
	g.push(s("path", { d: "M50 112V85", stroke: "#0e2d54", "stroke-width": 3, fill: "none" }));
	g.push(s("path", { d: "M46.5 106h7M46.5 99h7M46.5 92h7", stroke: "#0e2d54", "stroke-width": 1.2, fill: "none" }));
	for (const [x, y] of [[26, 26], [74, 26], [50, 24], [34, 34], [66, 34]]) g.push(s("path", { d: star(x, y, 4, 3.2, 1), fill: gold }));
	return g;
}

// ---------------------------------------------------------------------------
// Amphibians
// ---------------------------------------------------------------------------

const AMPHIBIANS = {
	id: "amphibians",
	label: "Amphibians",
	paper: "#f1f8e2",
	ink: "#21401f",
	edge: "#8db66a",
	trim: { rx: 12, dash: "" },
	pen: { smooth: true, width: 1.75, pads: 1.3 }, // round strokes that end in a toe pad
	suits: [
		{ label: "Tree Frogs", of: "", color: "#2a7a2e", art: [["f", el(12, 13.5, 5, 6.8)], ["F", o(8.5, 5.8, 2.7)], ["f", "M7 7h10v5H7z"], ["K", o(8.5, 5.5, 1.2)], ["L", "M8.5 11.500L4 9.5 2.8 6", 1.8], ["L", "M8 17.800L2.8 15.5 2.8 21", 2], ["F", dots([2.8, 5.5, 1.3], [2.8, 21.3, 1.4])], ["x", "M9.5 10.500Q12 12 14.5 10.5", 1]] },
		{ label: "Salamanders", of: "", color: "#cf3f0c", art: [["l", "M12 4.500C8 7 15.5 10 12 14S7.5 18.5 10.5 22.5", 3.8], ["f", el(12, 4, 3.3, 2.9)], ["l", "M11.3 8.300L6 6.200M12.8 9.300L18 7.500M10.5 15.800L5.5 18M11 16.500L16 19", 1.6], ["k", dots([12.7, 11.2, .85], [10.3, 17.5, .75], [11, 7, .75])]] },
		{ label: "Toads", of: "", color: "#74531a", art: [["f", "M2.2 18.200c0-4.8 2.4-7 4-8.800C6.2 6.2 8 4.5 12 4.500s5.8 1.7 5.8 4.900c1.6 1.8 4 4 4 8.8 0 2-1.4 3.3-3.2 3.300H5.400c-1.8 0-3.2-1.3-3.2-3.300z"], ["F", o(8, 5.4, 2.9)], ["K", o(8, 5.2, 1.3)], ["x", "M6.5 11Q12 14.2 17.5 11", 1.1], ["K", dots([6, 15.5, .8], [8.8, 17.8, .7], [10, 15.2, .6], [4.8, 18.7, .55])], ["x", "M2.5 21.500H21.5", .6]] },
		{ label: "Axolotls", of: "", color: "#bd1f76", art: [["f", "M12 7.500c5 0 8.3 2.7 8.3 6.700S17 21.5 12 21.500s-8.3-3.3-8.3-7.300S7 7.5 12 7.500z"], ["L", "M5.8 9.500L1.5 5M5 11.500L.8 10.500M7.5 8.300L5 3M3.5 7.500L1.2 7.800M6 5.800L3.8 5.5", 1.6], ["K", o(8, 13.2, 1.3)], ["x", "M8.5 17Q12 19.7 15.5 17", 1.2]] },
		{ label: "Dart Frogs", of: "", color: "#1557c0", art: [["f", "M2.5 15.500c0-4 3-7 7.5-7.6.6-2 2.3-3.4 4.3-3.4 2.4 0 4 1.4 4.5 3.400l3.7 1.7-3.4 1.500c-.4 2.8-1.4 5-2.9 6.400l2.9 1.700c1 .6.5 2.1-.6 2.100H8c-3.3 0-5.5-2.3-5.5-5.300z"], ["k", o(15, 7.5, 1.3)], ["k", dots([6.5, 12.5, 1.1], [10.5, 11.5, .8], [13.5, 13, .9], [5.5, 16, .7])], ["x", "M7.5 20.500C12.5 20 13.5 15.5 9.5 14.8", 1.1]] },
		{ label: "Newts", of: "", color: "#00756f", art: [["f", "M1 15.500C3 12.8 6 12 9 12h7c2.6 0 5.3 1 7 3-1.8 1.8-4.3 2.5-7 2.500H9C6 17.5 3 17 1 15.500z"], ["f", "M5 12.800L6.3 9l1.7 3 1.3-4.2 1.8 4.2 1.3-4.2 1.8 4.2 1.3-3.2 1.2 3.200z"], ["l", "M7 17L5.5 21.500M15 17L16.5 21.5", 1.9], ["k", o(19.5, 14.3, .95)]] },
		{ label: "Caecilians", of: "", color: "#6f35b5", art: [["l", spiral(12, 12, 1, 9.2, 2.250, 0), 2.7], ["f", o(12, 2.8, 2.3)], ["k", o(11.6, 2.4, .65)]] },
		{ label: "Tadpoles", of: "", color: "#2a2f2a", art: [["f", el(8.5, 9, 6.8, 6)], ["f", "M11 13.500c3 1.5 3 4.5 6 6s4.5.2 6 2.500c-3.5 1.2-7 .500-9.5-1.500S10.3 16 8.5 14z"], ["k", dots([5.5, 7, 1.3], [10, 7, 1.3])], ["x", "M5.5 11.500Q7.8 13 10 11.5", 1.1]] },
	],
	jokers: [
		{ label: "Frog Prince", art: [["f", "M12 9.500c5 0 9.5 2.3 9.5 6.300S17 22.5 12 22.5 2.5 19.8 2.5 15.8 7 9.5 12 9.500z"], ["F", o(7.2, 10.2, 3)], ["K", o(7.2, 10, 1.4)], ["x", "M5.5 16.500Q12 20.7 18.5 16.5", 1.3], ["g", "M7.5 8.500L5.8 1.8 9.6 5 12 1 14.4 5 18.2 1.8 16.5 8.500z"]] },
		{ label: "Frogspawn", art: [["f", dots([12, 12, 4], [5, 7.5, 3.7], [19, 7.5, 3.7], [5, 16.8, 3.7], [19, 16.8, 3.7], [12, 3.8, 3.2], [12, 20.4, 3.2])], ["k", dots([12, 12, 2.9], [5, 7.5, 2.7], [19, 7.5, 2.7], [5, 16.8, 2.7], [19, 16.8, 2.7], [12, 3.8, 2.2], [12, 20.4, 2.2])], ["g", dots([12, 12, 1.7], [5, 7.5, 1.5], [19, 7.5, 1.5], [5, 16.8, 1.5], [19, 16.8, 1.5], [12, 3.8, 1.2], [12, 20.4, 1.2])]] },
		{ label: "Fire Salamander", art: [["g", "M12 1c1 3.5 5 5.5 6.8 9 2 3.8 1.7 8-.5 10.800C16.5 23 14 23.8 12 23.800s-4.5-.8-6.3-3C3.5 18 3.2 13.8 5.2 10c1-2 2.3-3 2.8-5 .6 1.5 1 2.5 1.8 3.5.8-2.5 1-5 2.2-7.500z"], ["k", "M12 10c1.5 2.2 4.3 3.6 4.3 7S14.3 22 12 22s-4.3-1.6-4.3-5S10.5 12.2 12 10z"], ["l", "M12 11.500C10.5 13.5 13.8 15 12 17.500S10.3 20 11.5 21.5", 2.3], ["f", el(12, 11, 1.9, 1.6)], ["l", "M11.7 14L9.3 13.200M12.7 14.500L15 13.800M11.3 18.500L9 19.700M12 18.800L14.5 20", 1]] },
		{ label: "Moon Toad", art: [["g", "M15 1.200A11 11 0 1 0 22.8 19 9.2 9.2 0 0 1 15 1.200z"], ["f", "M8.5 22c-1 0-1.8-.8-1.8-2 0-3.5 1.5-5 3-6.3.1-2.2 1.5-3.5 4-3.500s3.9 1.3 4 3.500c1.5 1.3 3 2.8 3 6.3 0 1.2-.8 2-1.8 2z"], ["f", dots([11.2, 11, 1.9], [17, 11, 1.9])], ["k", dots([11.2, 10.9, .85], [17, 10.9, .85])], ["x", "M10.5 15.200Q14.1 17.3 17.7 15.2", 1]] },
	],
	back: amphibianBack,
};

function amphibianBack() {
	const g = [s("rect", { width: 100, height: 140, fill: "#0d5c58" })];
	// Rings on the water, and lily pads.
	for (const [x, y, r] of [[22, 28, 16], [80, 38, 12], [18, 108, 13], [78, 112, 18], [50, 70, 38], [50, 70, 46]]) {
		g.push(s("circle", { cx: x, cy: y, r, fill: "none", stroke: "#3fb9a6", "stroke-opacity": 0.55, "stroke-width": 1.3 }));
		g.push(s("circle", { cx: x, cy: y, r: r * 0.68, fill: "none", stroke: "#3fb9a6", "stroke-opacity": 0.32, "stroke-width": 1 }));
	}
	const pad = (x, y, r, rot) => s("path", { d: `M${x} ${y}L${n1(x + r * Math.cos(rot))} ${n1(y + r * Math.sin(rot))}A${r} ${r} 0 1 1 ${n1(x + r * Math.cos(rot + 0.6))} ${n1(y + r * Math.sin(rot + 0.6))}z`, fill: "#7cc04b", stroke: "#2f7a2e", "stroke-width": 1 });
	g.push(pad(22, 28, 11, 0.4), pad(80, 112, 13, 3.3), pad(82, 34, 8, 2), pad(19, 110, 9, 5));
	g.push(s("circle", { cx: 50, cy: 70, r: 29, fill: "#7cc04b", stroke: "#2f7a2e", "stroke-width": 1.2 }), s("path", { d: "M50 70L76 84A29 29 0 0 1 66 94z", fill: "#0d5c58" }));
	g.push(s("circle", { cx: 34, cy: 18, r: 3.5, fill: "#f6b7d2" }), s("circle", { cx: 34, cy: 18, r: 1.5, fill: "#ffe680" }));
	g.push(s("circle", { cx: 66, cy: 122, r: 3.5, fill: "#f6b7d2" }), s("circle", { cx: 66, cy: 122, r: 1.5, fill: "#ffe680" }));
	// A frog sitting on the big pad, its colours the back's own.
	g.push(s("g", { transform: "translate(32 50) scale(1.5)", style: "color:#14432b;--paper:#ffe680" }, drawIcon(AMPHIBIANS.suits[0].art)));
	g.push(s("rect", { x: 3.5, y: 3.5, width: 93, height: 133, rx: 9, fill: "none", stroke: "#bfe58f", "stroke-width": 1.8 }));
	return g;
}

// ---------------------------------------------------------------------------
// Fantasy Creatures
// ---------------------------------------------------------------------------

const FANTASY = {
	id: "fantasy",
	label: "Fantasy Creatures",
	paper: "#f8f0fb",
	ink: "#31164d",
	edge: "#a57fc8",
	trim: { rx: 7, dash: "5 2 1 2" },
	pen: { smooth: true, width: 1.050, nib: 0.42, skew: -9 }, // a broad nib, slanted
	suits: [
		{ label: "Dragons", of: "", color: "#c0143c", art: [["f", "M4 22.500l2.6-7.200C5.3 12 6 8.3 9 5.800L7.8 1.200l4.4 3c3.2-.9 6-.2 8 1.800l3.3 4.2-5.3.8.6 3.2-4-1.100c-1 3.2-2.6 5-5 6l-.800 3z"], ["k", "M13.5 7.200l3 .8-2.3 1.400z"], ["x", "M18.3 11.200l-.5 1.500M15.8 11l-.3 1.600M8 10.500Q6.8 13.4 8.5 16.5", 1]] },
		{ label: "Unicorns", of: "", color: "#6a1fb0", art: [["f", "M7 22.800c0-4.5-1.2-7.3 1-11.3 1.4-2.7 3.8-4.7 6.5-5.300L19.7.8l-.8 6.300c1.6 1.5 2.8 3.7 3.4 6 .2 1-.4 1.7-1.4 1.500l-3.3-1.100c-.6 1.4-1.6 2.5-1.6 3.7.1 2.2 1 3.8 1.7 5.600z"], ["k", o(16, 9.5, 1)], ["x", "M12.5 7.500C9 9 7.6 12 7.3 15M11.8 11C10.5 12.6 10 15 10 18", 1.2]] },
		{ label: "Phoenixes", of: "", color: "#c44d05", art: [["F", "M12 9.500C9.3 5.5 5.5 3 1 2.800c.5 3.5 1.7 5.5 3.3 7-1 .1-1.7 0-2.3-.3 1.6 3 5.3 5.5 10 5.500z"], ["f", o(12, 6.5, 2.5)], ["f", "M9.5 7h5v8h-5z"], ["f", "M11 4.500l1-4 1 4z"], ["F", "M11 14c-.8 3.2-2.8 5.2-4.3 8.5 2-.5 3.5-1.5 4.3-2.8.2 1 .5 2.3 1 3.800z"], ["k", o(12.8, 6.2, .6)]] },
		{ label: "Hydras", of: "", color: "#0a7a44", art: [["f", "M4.8 23c0-3.5 3.2-6 7.2-6s7.2 2.5 7.2 6z"], ["L", "M10.5 19C6 17 4 12 4.5 7.5", 2.5], ["l", "M12 19V8", 2.8], ["F", el(5, 5.6, 3.3, 2.2, -18)], ["f", el(12, 5, 2.9, 2.8)], ["K", o(4.8, 4.9, .7)], ["k", dots([10.8, 4.5, .65], [13.2, 4.5, .65])], ["x", "M10.5 6.500h3", .8]] },
		{ label: "Griffins", of: "", color: "#a26500", art: [["f", "M5 8.800C5 5 8 2.3 11.5 2.300c2.3 0 4.3 1 5.4 3.200l4.8 2.8-4.2 2c-.2 1.3-.6 2.4-1.5 3.500l2.5 9.200H6.500l1.3-7.500C6 14.2 5 11.5 5 8.800z"], ["k", o(13, 6.7, 1.1)], ["x", "M16 9.800c-1.5.2-2.7-.200-3.3-.8", 1], ["x", "M7.5 8c1.2 2.3 2.3 3.3 4 4M9 3.800C9.6 6.5 11 8 13 9M8.5 15l3 2M8 18l3.5 1.800M12.5 13l3.5 3", 1]] },
		{ label: "Fairies", of: "", color: "#b4148f", art: [["F", "M11.3 11C8 4.5 3.8 2.2 1.4 3.6.2 6.8 4.6 11.5 11.3 12.500z"], ["F", "M11.3 13C6.8 12.6 3.4 14.8 3.6 18c1.8 2.3 5.8.500 7.7-3z"], ["K", el(5.8, 6.8, 1.2, 2, -48)], ["f", o(12, 5.5, 2.3)], ["f", "M10.8 8h2.400l.5 6.5-1 8.500h-1.400l-1-8.500z"]] },
		{ label: "Treants", of: "", color: "#087f8c", art: [["f", dots([12, 7.5, 6.7], [5.8, 10, 4.7], [18.2, 10, 4.7])], ["f", "M8.3 10h7.400v8l3.8 5h-5l-2.5-2.800L9.5 23h-5l3.8-5z"], ["k", dots([9.8, 11.7, 1.3], [14.2, 11.7, 1.3])], ["x", "M9.8 15.800Q12 14 14.2 15.8", 1.2], ["x", "M7 5.500L9 7.500M17 5.500L15 7.5", 1]] },
		{ label: "Merfolk", of: "", color: "#0e55ba", art: [["f", "M8.3 1.500h7.400c.8 5.3-.5 9.5-2.3 13.800h-2.800C8.8 11 7.5 6.8 8.3 1.500z"], ["F", "M12 13.500C11.3 18 8 20.4 2.5 19.800c1.8 3.2 6.2 4.2 9.5 1z"], ["x", "M9 5Q10.5 6.8 12 5Q13.5 6.8 15 5M9.3 8.300Q10.7 10 12 8.300Q13.3 10 14.7 8.300M10 11.500Q11 12.9 12 11.500Q13 12.9 14 11.5", 1]] },
	],
	jokers: [
		{ label: "Mimic", art: [["g", "M3 11V8c0-3.3 3-5.5 9-5.500S21 4.7 21 8v3z"], ["f", "M3 13.500h18V21a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1z"], ["k", "M4.5 11l1.8 4 1.8-4 1.9 4 1.9-4 1.9 4 1.9-4 1.8 4 1.8-4z"], ["K", el(8, 6.5, 1.7, 1.4, 18)], ["f", dots([8, 6.5, .75], [16, 6.5, .75])], ["x", "M10 19.500h4", 1.2]] },
		{ label: "Goblin", art: [["G", "M6.5 9.500C4 7.5 1.8 7 .3 7.500c.6 3.5 2.7 6 6.5 6.500z"], ["f", "M12 3.500c4.3 0 6.7 2.7 6.7 6.7 0 3.5-.8 5.5-2.7 8.3-1.1 1.7-2.5 3-4 3s-2.9-1.3-4-3c-1.9-2.8-2.7-4.8-2.7-8.3 0-4 2.4-6.7 6.7-6.700z"], ["K", "M6.8 9l3.7 1.8-.9 1.700c-1.5 0-2.7-1.5-2.8-3.500z"], ["F", o(9.1, 10.8, .6)], ["k", "M8.5 15.800l1.4 1 .9-1 1.2 1 1.2-1 .9 1 1.4-1-1 2.900h-4.800z"], ["x", "M11.3 12.700l.7.7.7-.7", .8]] },
		{ label: "Will-o'-the-Wisp", art: [["g", "M12 .8c.8 3.8 3 4.9 5.3 7.7 2.8 3.5 2.9 8.3.3 11.500C16.2 21.9 14.2 23 12 23s-4.2-1.1-5.6-3c-2.6-3.2-2.5-8 .3-11.500C7.8 7 8.3 6 8.7 4.500c.6 1.2 1 2 1.7 2.8.2-2.3.5-4.3 1.6-6.500z"], ["k", "M12 9c1 2.3 3.8 3.5 3.8 7 0 3-1.7 5-3.8 5s-3.8-2-3.8-5c0-3.5 2.8-4.7 3.8-7z"], ["f", "M12 12c.7 1.5 2.2 2.3 2.2 4.3 0 1.8-1 3.2-2.2 3.200s-2.2-1.4-2.2-3.200c0-2 1.5-2.8 2.2-4.300z"]] },
		{ label: "Jackalope", art: [["M", "M9.8 9L8.2 5.8 4.5 3.800M8.2 5.800L8 2.200M6.3 4.800L3.8 6M15 9l1.6-3.2 3.7-2M16.6 5.800l.2-3.600M18.5 4.800L21 6", 1.6], ["F", "M9.8 12C8 13 5.8 12.3 4.8 10c-.5 3.2.7 5.7 3.5 6.500z"], ["f", "M12 9c3.5 0 5.8 2.8 5.8 6.2 0 3.2-2.8 7.3-5.8 7.300s-5.8-4.1-5.8-7.300C6.2 11.8 8.5 9 12 9z"], ["K", o(9.5, 14, 1)], ["k", "M10.8 17.800h2.400L12 19.300z"], ["x", "M12 19.200v1.1", .8]] },
	],
	back: fantasyBack,
};

function fantasyBack() {
	const gold = "#e8c25c";
	const g = [s("rect", { width: 100, height: 140, fill: "#34155c" })];
	// A field of lozenges, a shield on it and a dragon on the shield.
	for (let y = 0; y < 150; y += 14) {
		for (let x = (y / 14) % 2 ? 0 : 10; x < 110; x += 20) g.push(s("path", { d: `M${x} ${y - 7}L${x + 10} ${y}L${x} ${y + 7}L${x - 10} ${y}z`, fill: "#4a1f7e" }));
	}
	g.push(s("rect", { x: 4, y: 4, width: 92, height: 132, rx: 7, fill: "none", stroke: gold, "stroke-width": 1.8 }));
	g.push(s("rect", { x: 7.5, y: 7.5, width: 85, height: 125, rx: 5, fill: "none", stroke: gold, "stroke-width": 0.7 }));
	g.push(s("path", { d: "M22 38h56v36c0 18-14 28-28 36-14-8-28-18-28-36z", fill: "#a6163d", stroke: gold, "stroke-width": 2.4 }));
	g.push(s("g", { transform: "translate(29 42) scale(1.750)", style: `color:${gold};--paper:#a6163d` }, drawIcon(FANTASY.suits[0].art)));
	for (const [x, y] of [[18, 18], [82, 18], [18, 122], [82, 122], [50, 24]]) g.push(s("path", { d: star(x, y, 8, 5, 1.8), fill: gold }));
	return g;
}

// ---------------------------------------------------------------------------
// Lovecraftian
// ---------------------------------------------------------------------------

const LOVECRAFTIAN = {
	id: "lovecraftian",
	label: "Lovecraftian",
	paper: "#10191b",
	ink: "#cfe8d6",
	edge: "#3f6d62",
	trim: { rx: 3, dash: "0.5 4" },
	pen: { smooth: true, width: 1.35, wobble: 0.3, pads: 0.36, skew: 3 }, // a thin, unsteady hand
	suits: [
		{ label: "Tentacles", of: "", color: "#7fe06a", art: [["f", "M7 23C3.8 17.3 5 12 9.5 8.800c3-2.1 6.2-2.7 7.3-5.2.5-1.2 0-2-.8-2.3 2.7-.7 5.2 1 5.2 3.9 0 3.8-3.8 5.5-6.4 7.3-2.5 1.8-3.4 5.3-1.8 10.500z"], ["k", dots([9.4, 13.3, 1.150], [13, 10.3, 1], [16.7, 7.8, .8], [19.2, 4.7, .55], [9, 18.2, 1.150])]] },
		{ label: "Elder Signs", of: "", color: "#e8d44d", art: [["l", "M12 1.800L15 21.5 2.3 8.900H21.700L9 21.500z", 1.5], ["l", o(12, 12, 10.4), 1], ["f", "M12 8c1.8 1.6 3 3 3 5s-1.3 3-3 3-3-1-3-3 1.2-3.4 3-5z"], ["k", o(12, 13.2, 1)]] },
		{ label: "Deep Ones", of: "", color: "#4fd8c4", art: [["F", "M5 9L.8 5.3 1.5 13l3.2 2z"], ["f", "M12 2.500c5 0 8 4 8 9.5 0 6-3.7 10.5-8 10.500S4 18 4 12c0-5.5 3-9.5 8-9.500z"], ["f", "M12 .5l-2 3h4z"], ["K", o(8, 10, 2.7)], ["F", o(8, 10.3, 1.2)], ["k", "M6 16.500Q12 13.5 18 16.500L16 19.500Q12 18 8 19.500z"], ["F", "M8 16.300l.8 1.7.8-2.1.9 2 .9-2.3.6 2.3.8-2.3.9 2 .8-2 .9 2.1.8-1.600L12 15z"]] },
		{ label: "Shoggoths", of: "", color: "#b58cff", art: [["f", "M4 21.500c-2.8-.5-3.3-4-1.8-5.8-1.5-2.7-.5-6 2-7 .3-3.3 3.3-6 6.3-5 2.2-2 6-1.7 7.5 1 3 .5 4.5 3.5 3.7 6 1.8 2 2 5.8-.2 7.5.3 2.8-2.3 4-4.5 3.3-1.5 1.2-3.5 1-5 0-2.5 1-5.8 1.2-8 0z"], ["k", dots([8, 9, 2.5], [15.5, 7.5, 1.7], [17.5, 14, 2.2], [10.5, 15.5, 1.7], [5, 15.5, 1.2], [13, 11.2, 1], [14, 19.5, 1], [20, 9.8, .7])], ["f", dots([8.5, 9.2, 1.2], [15.8, 7.7, .8], [17, 14.3, 1], [10.2, 15.7, .8], [5.2, 15.5, .55], [13, 11.2, .45], [14, 19.5, .45])]] },
		{ label: "Mi-Go", of: "", color: "#ff9d4d", art: [["F", "M10.5 9C7.5 3.5 3.5 2 .8 3.500c.4 3.5.2 5.8 2.7 9.3 3-.1 5.3-1.3 7-3z"], ["K", "M3 6Q6 7 8.5 9.500M4 10Q6 10.5 8 9.5"], ["x", "M2.7 5.800Q6 6.8 8.8 9.300M3.8 10.300Q6.3 10.6 8.5 9.600M5 3.800Q6.8 5.8 9 8.5", .8], ["f", el(12, 13.5, 3.2, 6.3)], ["f", el(12, 4.8, 2.7, 3.4)], ["x", "M9.8 3.800h4.400M9.5 5.300h5M10 6.800h4M9.2 11.500h5.600M9 14h6M9.8 16.500h4.4", .85], ["L", "M9.3 11.500L4.5 15.500M9.3 15L5 20M10.3 18.500L8 23", 1.3]] },
		{ label: "Elder Things", of: "", color: "#ff7b7b", art: [["f", "M8.5 8.500h7l1.8 8.5-2 4.800H8.700l-2-4.800z"], ["x", "M9 11h6M8 14h8M8 17h8M9.5 8.800V21.500M14.5 8.800V21.5", .85], ["f", star(12, 4.8, 5, 5, 1.9)], ["k", o(12, 4.8, .8)], ["L", "M7.5 11.500C4.5 10.5 3 12.8 1.2 10.500M7 15.500C4.5 15.5 3.2 18.5 1 17", 1.4]] },
		{ label: "Silver Keys", of: "", color: "#e6dcc3", art: [["l", o(12, 6, 4), 2.2], ["k", el(12, 6, 1.5, 1)], ["f", o(12, 6, .75)], ["l", "M12 10V23", 2.2], ["f", "M12 14.800h4.800v2.200H14.500v1.200H12zM12 19.800h5.800v2.700H12z"], ["k", "M15.5 20.800h1.200v.8h-1.200z"]] },
		{ label: "Dread Tomes", of: "", color: "#8ec7ff", art: [["f", "M3 2h15.500A2.5 2.5 0 0 1 21 4.500V20a2.5 2.5 0 0 1-2.5 2.500H3z"], ["x", "M6 2.500V22", 1], ["k", "M8.3 11.500Q13.5 5 18.7 11.500Q13.5 18 8.3 11.500z"], ["f", o(13.5, 11.5, 2.3)], ["k", "M13.2 9.500h.6v4h-.6z"], ["x", "M8.5 5l2 1.500M18 5l-2 1.500M8.5 18l2-1.500M18 18l-2-1.5", 1]] },
	],
	jokers: [
		{ label: "Cthulhu", art: [["G", "M7 11C4 7 1.6 6 .3 6.6.8 10 1.2 14 3 17l4-1.500z"], ["f", "M12 2C7.5 2 5.5 5.3 5.5 8.800c0 2.5 1 4.5 2.5 6h8c1.5-1.5 2.5-3.5 2.5-6C18.5 5.3 16.5 2 12 2z"], ["K", el(8.8, 9, 1.8, 1.1, 22)], ["F", o(8.8, 9, .55)], ["G", "M8 14.500c0 3.5-1.8 5-2.5 8.5 1.8-1 3.2-2.5 4.1-5.500z"], ["g", "M10.5 14.500h3l-.3 4.5-.7 4.500h-.9l-.8-4.500z"], ["M", "M10 14.500C9.8 17 9 19 8.7 21", 1]] },
		{ label: "Nyarlathotep", art: [["f", "M12 .8c2 1.8 3.5 4 4 7l-1 3H9l-1-3c.5-3 2-5.2 4-7z"], ["x", "M9 8h6", 1.1], ["K", "M9.7 11h4.600v.1z"], ["f", "M4.5 23c.5-4.5 3-8.3 7.5-8.300s7 3.8 7.5 8.300z"], ["f", "M9 10h6v6H9z"], ["G", "M9 12.500C6 11 5.8 8 3.5 6.500c.3 2 .1 3.3-.8 4.5.3 2.8 2.5 5.3 5.8 5.500zM7 16.500C5 18 3 18 1 17.500c1 2 2.8 3.5 5 3.500z"], ["m", "M12 11v6", 0.8]] },
		{ label: "Azathoth", art: [["m", spiral(12, 12, 4, 11.5, 1.1, 0) + spiral(12, 12, 4, 11.5, 1.1, Math.PI), 1.7], ["f", o(12, 12, 6.3)], ["k", el(12, 12, 4.7, 2.8)], ["f", o(12, 12, 2.1)], ["k", o(12.7, 11.3, .6)]] },
		{ label: "Yog-Sothoth", art: [["f", dots([12, 12, 5], [5, 7, 3.3], [19.5, 8, 2.7], [6.7, 18.7, 2.8])], ["g", dots([15, 3.8, 2.8], [18, 17.5, 3.8], [11.5, 21, 2.3], [2.8, 13, 2], [21.7, 13, 1.5])], ["k", dots([12, 12, 2], [5, 7, 1.3], [19.5, 8, 1], [6.7, 18.7, 1.1])], ["f", dots([12, 12, .9])], ["m", "M9 9L7 8M15 9L16 6M15 15L17 16M9 15.500L8 17", 1.3]] },
	],
	back: lovecraftianBack,
};

function lovecraftianBack() {
	const glow = "#7fe06a";
	const g = [s("rect", { width: 100, height: 140, fill: "#071211" })];
	// Tentacles reach in from the corners for the eye.
	const arm = (d, w) => [
		s("path", { d, fill: "none", stroke: "#0f4032", "stroke-width": w + 3, "stroke-linecap": "round" }),
		s("path", { d, fill: "none", stroke: "#1f7a58", "stroke-width": w, "stroke-linecap": "round" }),
		s("path", { d, fill: "none", stroke: glow, "stroke-width": 1.1, "stroke-linecap": "round", "stroke-dasharray": "0.1 5.5", "stroke-opacity": 0.85 }),
	];
	const reach = "M-4 -4C10 28 40 14 36 36S14 42 26 54";
	for (const m of ["", "matrix(-1 0 0 1 100 0)", "matrix(1 0 0 -1 0 140)", "matrix(-1 0 0 -1 100 140)"]) {
		g.push(s("g", { transform: m }, arm(reach, 6)));
	}
	const eye = "M18 70Q50 38 82 70Q50 102 18 70z";
	g.push(s("path", { d: eye, fill: "#d7e88c", stroke: glow, "stroke-width": 2 }));
	g.push(s("circle", { cx: 50, cy: 70, r: 17, fill: "#3b6a2e" }));
	g.push(s("ellipse", { cx: 50, cy: 70, rx: 3.8, ry: 15, fill: "#040807" }));
	g.push(s("rect", { x: 3, y: 3, width: 94, height: 134, rx: 5, fill: "none", stroke: "#2f6a55", "stroke-width": 1.2, "stroke-dasharray": "9 2 1 2" }));
	return g;
}

// ---------------------------------------------------------------------------
// The Atomic Age (the set of our own: the future as it looked in 1957)
// ---------------------------------------------------------------------------

const ATOMIC = {
	id: "atomic",
	label: "Atomic Age",
	paper: "#e6f5ee",
	ink: "#1f3a44",
	edge: "#65a8a0",
	trim: { rx: 14, dash: "8 3" },
	pen: { smooth: true, width: 2.050, inline: 0.55 }, // a neon sign: a thick tube with a line of light in it
	suits: [
		{ label: "Rockets", of: "", color: "#d13a24", art: [["f", "M12 .8c3.3 2.7 4.8 6.2 4.8 10 0 2.7-.5 5.2-1.3 7.200H8.500c-.8-2-1.3-4.5-1.3-7.2 0-3.8 1.5-7.3 4.8-10z"], ["k", o(12, 8.6, 2.3)], ["F", "M7.8 11.500L3 18v3.800l5.3-3.300z"], ["f", "M9.5 19h5L12 23.800z"], ["x", "M8 13.500h8", 1]] },
		{ label: "Atoms", of: "", color: "#00717a", art: [["l", el(12, 12, 10.7, 4, 0), 1.5], ["l", el(12, 12, 10.7, 4, 60), 1.5], ["l", el(12, 12, 10.7, 4, 120), 1.5], ["f", o(12, 12, 2.7)], ["f", dots([22.7, 12, 1.5], [6.650, 2.750, 1.5], [6.650, 21.250, 1.5])]] },
		{ label: "Saucers", of: "", color: "#946500", art: [["l", "M6.8 10.500C6.8 6.5 9 3.5 12 3.500s5.2 3 5.2 7", 1.7], ["k", "M7.5 10.500C7.5 7 9.5 4.3 12 4.300s4.5 2.7 4.5 6.200z"], ["f", "M.8 12.500c3.2-1.7 7-2.3 11.2-2.300s8 .6 11.2 2.300c-2.5 3-6.7 4.7-11.2 4.700S3.3 15.5.8 12.500z"], ["k", dots([5.8, 13.3, 1.050], [12, 14.7, 1.050], [18.2, 13.3, 1.050])], ["l", "M8 18.300L5.5 23M16 18.300l2.5 4.700M12 18.500V23.5", 1.3]] },
		{ label: "Ringed Worlds", of: "", color: "#4a7510", art: [["f", o(12, 12, 7)], ["x", "M5.5 9.300Q12 7 18.5 9.300M5.2 13.700Q12 11.5 18.8 13.7", 1], ["x", el(12, 12, 11, 3.3, -22), 3], ["l", el(12, 12, 11, 3.3, -22), 1.6], ["f", "M5 12A7 7 0 0 1 19 12C18 9.9 6 9.9 5 12z", 0], ["x", "M5.5 9.300Q12 7 18.5 9.3", 1]] },
		{ label: "Robots", of: "", color: "#8a2a86", art: [["f", "M6 7.500h12A1.5 1.5 0 0 1 19.5 9v11.500A1.5 1.5 0 0 1 18 22H6a1.5 1.5 0 0 1-1.5-1.500V9A1.5 1.5 0 0 1 6 7.500z"], ["K", o(8.7, 12.5, 2.2)], ["F", o(8.7, 12.5, .9)], ["k", "M7.5 17h9v2.700h-9z"], ["x", "M10.5 17v2.700M13.5 17v2.7", 0], ["f", "M10.3 16.800h.9v3.100h-.9zM12.8 16.800h.9v3.100h-.9z"], ["l", "M12 7.500V3", 1.6], ["f", o(12, 2.5, 1.7)], ["F", "M4.5 11h-2v5.500h2z"]] },
		{ label: "Ray Guns", of: "", color: "#203f8f", art: [["f", "M3.5 7.500h10c3 0 5 1.5 5 3.800s-2 3.8-5 3.800h-10c-1.5 0-2.7-1.7-2.7-3.800s1.2-3.8 2.7-3.800z"], ["k", "M6 9.500v3.800M9 9.500v3.800M12 9.500v3.8"], ["x", "M6 8.800v5M9 8.800v5M12 8.800v5", 1.1], ["f", "M18.5 9.500l2.5-2v7.600l-2.5-2z"], ["f", dots([22.5, 11.3, 1.1])], ["f", "M4.5 14.800h4.500l.8 3.800c.2 1.4-.5 3-2.3 3.400H5.200z"]] },
		{ label: "Sputniks", of: "", color: "#b85200", art: [["f", o(9.5, 9.5, 6)], ["k", el(7.7, 7.5, 1.6, 1, -40)], ["l", "M13.5 13.800L23 19.500M11.3 15L18.8 23.500M14.8 11.300L23.5 13.500M8 15.300L10.5 23.8", 1.2]] },
		{ label: "Starbursts", of: "", color: "#26343b", art: [["f", star(12, 12, 8, 12.2, 1.8)], ["f", star(12, 12, 8, 6.5, 1.7, 22.5)], ["k", o(12, 12, 1.6)]] },
	],
	jokers: [
		{ label: "Little Green Man", art: [["f", "M12 1.500c5.7 0 9.3 4 9.3 9 0 5.5-4.8 12-9.3 12S2.7 16 2.7 10.500c0-5 3.6-9 9.3-9z"], ["G", "M4.5 10.500c1.8-1.2 5-.2 6 3 .4 1.2-.2 2-1.3 1.700C6.5 14.5 4 12.5 4.5 10.500z"], ["x", "M10.3 18.500Q12 19.3 13.7 18.5", 1]] },
		{ label: "Astronaut", art: [["f", "M12 1.500c6 0 10 4.5 10 10s-3.8 8.5-6.5 9v2.500h-7V20.500C5.8 20 2 17 2 11.500s4-10 10-10z"], ["g", "M12 5c4.2 0 7 2.7 7 6.200s-2.8 6-7 6-7-2.5-7-6S7.8 5 12 5z"], ["k", el(8.7, 8.8, 2, 1, -30)], ["x", "M9 21.500h6", 1]] },
		{ label: "Space Chimp", art: [["F", o(3.8, 12, 2.8)], ["f", "M12 3c5 0 8.2 3.4 8.2 8.5 0 5.5-3.7 10-8.2 10s-8.2-4.5-8.2-10C3.8 6.4 7 3 12 3z"], ["g", "M6.7 9.700C6.7 7.5 8.8 6.5 10 7c.8.3 1.3 1 2 1s1.2-.7 2-1c1.2-.5 3.3.5 3.3 2.7 0 1.3-.5 2.3-.9 3.3 1.6 1 2.3 2.4 2.3 3.9 0 2.2-2.8 3.8-6.7 3.800s-6.7-1.6-6.7-3.800c0-1.5.7-2.9 2.3-3.9-.4-1-.9-2-.9-3.300z"], ["F", o(9.5, 10.5, 1.050)], ["f", dots([11.2, 14.5, .5], [12.8, 14.5, .5])], ["l", "M8.5 17Q12 19.5 15.5 17", 1.2]] },
		{ label: "Mutant", art: [["g", "M12 12L7.4 4A9.3 9.3 0 0 1 16.6 4zM12 12L21.2 12A9.3 9.3 0 0 1 16.6 20zM12 12L7.4 20A9.3 9.3 0 0 1 2.8 12z"], ["f", o(12, 12, 3.2)], ["k", o(12, 12, 1.5)], ["f", o(12, 12, .75)], ["l", o(12, 12, 11), 1.3]] },
	],
	back: atomicBack,
};

function atomicBack() {
	const g = [s("rect", { width: 100, height: 140, fill: "#0d8078" })];
	// Boomerangs on the wallpaper, a big starburst and an atom in it.
	for (const [x, y, c, r] of [[14, 16, "#f4c542", 20], [86, 24, "#e6f5ee", 100], [12, 118, "#e6f5ee", -60], [84, 124, "#f4c542", 200], [14, 72, "#ee6f4f", 90], [87, 74, "#ee6f4f", 280]]) {
		g.push(s("path", { d: "M-10 -3Q0 -9 10 -3Q0 -2 -10 -3Q-2 2 -5 9Q-7 2 -10 -3z", transform: `translate(${x} ${y}) rotate(${r})`, fill: c }));
	}
	g.push(s("path", { d: star(50, 70, 12, 42, 13), fill: "#f4c542" }));
	g.push(s("path", { d: star(50, 70, 12, 24, 15, 15), fill: "#ee6f4f" }));
	g.push(s("circle", { cx: 50, cy: 70, r: 15.5, fill: "#e6f5ee" }));
	g.push(s("g", { transform: "translate(35.6 55.6) scale(1.2)", style: "color:#0b4f55;--paper:#e6f5ee" }, drawIcon(ATOMIC.suits[1].art)));
	g.push(s("rect", { x: 3.5, y: 3.5, width: 93, height: 133, rx: 12, fill: "none", stroke: "#e6f5ee", "stroke-width": 1.8 }));
	return g;
}

/** Every set, in the order they are offered. */
export const CARD_SETS = [ANCIENT, AMPHIBIANS, FANTASY, LOVECRAFTIAN, ATOMIC];

// ---------------------------------------------------------------------------
// Drawing
// ---------------------------------------------------------------------------

const LAYER = { f: "ts-f", g: "ts-g", k: "ts-k", l: "ts-l", m: "ts-m", x: "ts-x" };

/** The elements of an icon's layers. */
export function drawIcon(art) {
	const out = [];
	for (const [kind, d, width] of art) {
		const low = kind.toLowerCase();
		const attrs = { d, class: LAYER[low] };
		if (low === "l" || low === "m" || low === "x") attrs["stroke-width"] = width;
		out.push(s("path", attrs));
		if (low !== kind) out.push(s("path", { ...attrs, transform: "matrix(-1 0 0 1 24 0)" }));
	}
	return out;
}

/** An icon as a group: its centre at (x, y), size units wide, upside down when flip. */
function iconAt(art, x, y, size, flip = false) {
	const k = size / 24;
	return s("g", { transform: `translate(${n1(x)} ${n1(y)}) rotate(${flip ? 180 : 0}) scale(${n1(k)}) translate(-12 -12)` }, drawIcon(art));
}

/** What a card or a chip of this set has to set for its art to take its colours. */
function setVars(set, a, b) {
	return `--suit: ${a}; --suit2: ${b || a}; --paper: ${set.paper}; --edge: ${set.edge}; --ink: ${set.ink}`;
}

/** The colours and the art of the joker at this place in the game's list. */
function jokerAt(set, index) {
	const i = index % set.jokers.length;
	return { ...set.jokers[i], color: set.suits[2 * i].color, color2: set.suits[2 * i + 1].color };
}

function svgFace() {
	return s("svg", { viewBox: "0 0 100 140", class: "ts-svg", preserveAspectRatio: "xMidYMid meet", "aria-hidden": "true", focusable: "false" });
}

function trim(set) {
	return s("rect", { x: 3.5, y: 3.5, width: 93, height: 133, rx: set.trim.rx, class: "ts-trim", "stroke-dasharray": set.trim.dash || null });
}

/** The rank in two opposite corners, with the icon below it. */
function corners(set, label, art) {
	const out = [];
	for (const flip of [false, true]) {
		const g = s("g", { transform: flip ? "rotate(180 50 70)" : "" });
		const text = String(label);
		if (canLetter(text)) {
			// Two digits fit; more than that shrink to the width of the corner.
			g.append(lettering(text, 14, 7, text.length > 2 ? 40 / text.length : text.length > 1 ? 14 : 17, set.pen));
		} else {
			g.append(s("text", { x: 14, y: 21, class: "ts-rank-text", "text-anchor": "middle", "font-size": text.length > 1 ? 13 : 18 }, text));
		}
		g.append(iconAt(art, 14, 34, 13));
		out.push(g);
	}
	return out;
}

const faces = new Map();

/** A suit card's picture. Faces are built once and copied: a hand has many pips. */
function suitFaceSvg(set, index, label) {
	const key = `${set.id}|${index}|${label}`;
	if (!faces.has(key)) {
		const art = set.suits[index].art;
		const svg = svgFace();
		svg.append(trim(set), ...corners(set, label, art));
		const n = rankNumber(label);
		const layout = LAYOUTS[n];
		if (layout) {
			// Drawn a little closer together than the classic pips: these are heavier, and the corners need the room.
			const big = n >= 9;
			for (const [x, y] of layout) svg.append(iconAt(art, 50 + (x - 50) * 0.84, y, big ? 16.5 : 19, y > 70));
		} else if (n === 1) {
			svg.append(s("circle", { cx: 50, cy: 70, r: 34, class: "ts-ring" }));
			svg.append(iconAt(art, 50, 70, 48));
		} else {
			// A court card, whatever the game calls it: the frame doubled for the highest ranks.
			const tier = Number.isFinite(n) && n >= 13 ? 2 : 1;
			for (let i = 0; i < tier; i++) {
				const d = i * 5;
				svg.append(s("rect", { x: 24 + d, y: 27 + d, width: 52 - 2 * d, height: 86 - 2 * d, rx: set.trim.rx, class: "ts-ring" }));
			}
			svg.append(iconAt(art, 50, 70, tier > 1 ? 33 : 38));
			const inset = tier === 2 ? 15 : 10;
			svg.append(iconAt(art, 24 + inset, 27 + inset, 9), iconAt(art, 76 - inset, 113 - inset, 9, true));
			if (n === 12) svg.append(iconAt(art, 76 - inset, 27 + inset, 9), iconAt(art, 24 + inset, 113 - inset, 9, true));
		}
		faces.set(key, svg);
	}
	return faces.get(key).cloneNode(true);
}

/** A joker's picture: its icon in its two colours, its name below it. */
function jokerFaceSvg(set, index) {
	const key = `${set.id}|joker|${index}`;
	if (!faces.has(key)) {
		const j = jokerAt(set, index);
		const svg = svgFace();
		const small = (flip) => s("g", { transform: flip ? "rotate(180 50 70)" : "" }, iconAt(j.art, 15, 17, 18));
		svg.append(trim(set), small(false), small(true), s("circle", { cx: 50, cy: 66, r: 32, class: "ts-ring" }), iconAt(j.art, 50, 66, 46));
		// The name, as large as fits between the corners.
		const name = j.label.toUpperCase().replace(/'/g, "").replace(/[^A-Z0-9 ]/g, " ");
		const cap = Math.min(7.5, 7.6 * (76 / (name.length * 7.9)));
		svg.append(lettering(name, 50, 106 + (7.5 - cap) / 2, cap, set.pen));
		faces.set(key, svg);
	}
	return faces.get(key).cloneNode(true);
}

const backs = new Map();

/** The back of a card of this set. */
export function backSvg(set) {
	if (!backs.has(set.id)) {
		const svg = svgFace();
		svg.append(...set.back());
		backs.set(set.id, svg);
	}
	return backs.get(set.id).cloneNode(true);
}

/** A set's icon on a small piece of its own paper, for a button or a label. */
function chip(set, art, a, b, px, label) {
	const svg = s("svg", { viewBox: "0 0 32 32", width: px, height: px, class: "ts-chip", style: setVars(set, a, b), role: label ? "img" : null, "aria-hidden": label ? null : "true", "aria-label": label || null });
	svg.append(s("rect", { x: 0.5, y: 0.5, width: 31, height: 31, rx: 6, class: "ts-chip-paper" }));
	svg.append(iconAt(art, 16, 16, 25));
	if (label) svg.append(s("title", {}, label));
	return svg;
}

// ---------------------------------------------------------------------------
// Which set is in force, and what it makes of a game's deck
// ---------------------------------------------------------------------------

export function isJoker(t) {
	return !!(t && t.tags && t.tags.includes("joker"));
}

// The set of the game on the page, or null for the game's own cards. The page says
// which before it draws, so that everything drawn here agrees with the table.
let inForce = null;

/** The game page: this game was set up with this set ("" for the game's own cards). */
export function useCardSet(id) {
	inForce = cardSetById(id);
}

export function cardSetById(id) {
	return CARD_SETS.find((x) => x.id === id) || null;
}

/** The set this game's cards are drawn with, or null for the game's own cards. */
export function currentCardSet(info) {
	return inForce && info && info.cards && info.cards.standard ? inForce : null;
}

/** Where a suit of the game falls in a set: its place in the game's order of suits. */
function suitIndex(info, suitId) {
	return (info.cards.suit_order || []).indexOf(suitId);
}

/** Where a joker falls among the jokers of the game: its type's place, then its copy. */
function jokerIndex(info, t, id) {
	const jokers = info.cards.types.filter(isJoker).map((x) => x.id).sort();
	const hash = String(id).indexOf("#");
	const copy = hash < 0 ? 0 : Number(String(id).slice(hash + 1)) || 0;
	return jokers.indexOf(t.id) + copy;
}

/** "Tree Frogs (hearts)": a suit's name in this set, with the name the rules use. */
export function setSuitName(set, info, suitId) {
	const i = suitIndex(info, suitId);
	if (i < 0) return "";
	const own = ((info.cards.suits || {})[suitId] || {}).label;
	return own ? `${set.suits[i].label} (${own.toLowerCase()})` : set.suits[i].label;
}

/**
 * What a card of this type looks like in a set: { class, style, title, picture }.
 * cards.js puts it in the frame every card has.
 */
export function setFace(set, info, t, id) {
	if (isJoker(t)) {
		const index = jokerIndex(info, t, id);
		const j = jokerAt(set, index);
		const own = t.label && t.label.toLowerCase() !== j.label.toLowerCase() ? ` (${t.label})` : "";
		return {
			style: setVars(set, j.color, j.color2),
			title: `${j.label}${own}${t.text ? " — " + t.text : ""}`,
			picture: jokerFaceSvg(set, index % set.jokers.length),
		};
	}
	const i = suitIndex(info, t.suit);
	return {
		style: setVars(set, set.suits[i].color),
		title: `${t.label} of ${setSuitName(set, info, t.suit)}`,
		picture: suitFaceSvg(set, i, t.label),
	};
}

/** A suit of the game as a small picture in this set (the board of terms, for one). */
export function setSuitIcon(set, info, suitId, px) {
	const i = suitIndex(info, suitId);
	if (i < 0) return null;
	return chip(set, set.suits[i].art, set.suits[i].color, null, px);
}

/** A game's suit in this set, for a picture that frames it itself. */
export function setSuitArt(set, info, suitId) {
	const i = suitIndex(info, suitId);
	if (i < 0) return null;
	return { group: (x, y, size) => iconAt(set.suits[i].art, x, y, size), style: setVars(set, set.suits[i].color) };
}

/**
 * A set at a glance, for choosing one: its back, its eight suits and its four
 * jokers. The suits this game does not reach are dimmed.
 */
export function setPreview(set, used = 8) {
	const root = s("svg", { viewBox: "0 0 244 72", class: "ts-preview", role: "img", "aria-label": `${set.label}: the back of a card, eight suits and four jokers` });
	const back = backSvg(set);
	back.setAttribute("x", "0");
	back.setAttribute("y", "0");
	back.setAttribute("width", "51");
	back.setAttribute("height", "71.4");
	root.append(back);
	set.suits.forEach((suit, i) => {
		const c = chip(set, suit.art, suit.color, null, 31, suit.of ? `${suit.label}: ${suit.of}` : suit.label);
		c.setAttribute("x", String(58 + (i % 4) * 33));
		c.setAttribute("y", String(i < 4 ? 2 : 38));
		if (i >= used) c.classList.add("ts-unused");
		root.append(c);
	});
	set.jokers.forEach((_, i) => {
		const j = jokerAt(set, i);
		const c = chip(set, j.art, j.color, j.color2, 31, `Joker: ${j.label}`);
		c.setAttribute("x", String(194 + (i % 2) * 25));
		c.setAttribute("y", String(i < 2 ? 2 : 38));
		c.setAttribute("width", "24");
		c.setAttribute("height", "24");
		root.append(c);
	});
	return root;
}
