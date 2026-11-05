// The Animation Tester of Cthulhu Wars' setup: a map, with buttons below it that play each of the shows without
// starting a game. Everything it plays is made up here, from a fixed table. It is a tool for looking at them,
// and is likely to go once they settle.

import { h, clear } from "./util.js";
import { cssColor, pieceIcon, renderBoard } from "./render.js";
import { isMobileLayout, isLandscape } from "./mobile.js";
import { createBattleTheatre } from "./cw-battle.js";
import { createEmergence, emergenceTypes } from "./cw-emerge.js";

const SEATS = [
	{ seat: 1, key: "gc", name: "Great Cthulhu", color: "green" },
	{ seat: 2, key: "cc", name: "Crawling Chaos", color: "blue" },
	{ seat: 3, key: "bg", name: "Black Goat", color: "red" },
	{ seat: 4, key: "ys", name: "Yellow Sign", color: "yellow" },
];

// Where each Great Old One comes up, and whose it is.
const EMERGE_AT = {
	cthulhu: { seat: 1, area: "south-pacific", areaName: "South Pacific" },
	nyarlathotep: { seat: 2, area: "asia", areaName: "Asia" },
	"shub-niggurath": { seat: 3, area: "africa", areaName: "Africa" },
	"king-in-yellow": { seat: 4, area: "europe", areaName: "Europe" },
	hastur: { seat: 4, area: "north-atlantic", areaName: "North Atlantic" },
};

const many = (seat, type, n) => Array.from({ length: n }, () => ({ seat, type }));

// The battle: everything that a battle can show, in one. Great Cthulhu attacks Crawling Chaos in Asia.
const BEFORE = [
	...many(1, "cthulhu", 1),
	...many(1, "starspawn", 1),
	...many(1, "shoggoth", 2),
	...many(1, "deep-one", 3),
	...many(1, "cultist", 2),
	...many(2, "nyarlathotep", 1),
	...many(2, "flying-polyp", 2),
	...many(2, "nightgaunt", 2),
	...many(2, "cultist", 2),
];

const without = (units, ...gone) => {
	const out = [...units];
	for (const [seat, type] of gone) {
		const at = out.findIndex((u) => u.seat === seat && u.type === type);
		if (at >= 0) out.splice(at, 1);
	}
	return out;
};

function battleSnaps() {
	const side = (seat, name, combat, dice, kills, pains) => ({ seat, name, combat, dice, kills, pains });
	const shot = (step, sides, abilities, units, pained = []) => ({
		bt: { area: "asia", area_name: "Asia", att: 1, def: 2, step, resolved: false, sides, abilities },
		units,
		pained,
	});
	const pool = (a, b) => [side(1, "Great Cthulhu", a, null, 0, 0), side(2, "Crawling Chaos", b, null, 0, 0)];
	const ability = (name, seat, current) => ({ name, seat, current });
	const pre = (now) => ["Devour|1", "Absorb|1", "Abduct|2", "Invisibility|2", "Seek and Destroy|2"].slice(now).map((a, i) => ability(a.split("|")[0], Number(a.split("|")[1]), i === 0));

	const afterDevour = without(BEFORE, [2, "nightgaunt"]);
	const afterAbsorb = without(afterDevour, [1, "deep-one"]);
	const afterAbduct = without(afterAbsorb, [1, "cultist"]);
	const afterSeek = [...afterAbduct, ...many(2, "hunting-horror", 2)];
	const gcDice = [6, 2, 5, 1, 6, 3, 4, 2, 6, 1, 3, 5, 2, 1];
	const ccDice = [6, 3, 1, 5, 6, 2, 4, 1, 2];
	const rolled = (k1, p1, k2, p2) => [side(1, "Great Cthulhu", 14, gcDice, k1, p1), side(2, "Crawling Chaos", 9, ccDice, k2, p2)];
	const hit1 = without(afterSeek, [1, "deep-one"]);
	const hit2 = without(hit1, [1, "cultist"]);
	const hit3 = without(hit2, [2, "flying-polyp"]);
	const hit4 = without(hit3, [2, "hunting-horror"]);
	const hit5 = without(hit4, [2, "cultist"]);
	const pains = [
		{ seat: 1, type: "shoggoth" },
		{ seat: 2, type: "nightgaunt" },
		{ seat: 2, type: "flying-polyp" },
	];
	const after = rolled(2, 2, 3, 2);
	return [
		shot("pre", pool(11, 6), pre(0), BEFORE),
		shot("pre", pool(11, 5), pre(1), afterDevour),
		shot("pre", pool(14, 5), pre(2), afterAbsorb),
		shot("pre", pool(14, 5), pre(3), afterAbduct),
		shot("pre", pool(14, 9), pre(4), afterSeek),
		shot("assign", after, [], afterSeek),
		shot("assign", after, [], hit1),
		shot("assign", after, [], hit2),
		shot("assign", after, [], hit3),
		shot("assign", after, [], hit4),
		shot("assign", after, [], hit5, pains),
		shot("post", after, [ability("The Harbinger", 2, true), ability("Necrophagy", 3, false)], hit5, pains),
		shot("post", after, [ability("Necrophagy", 3, true)], hit5, pains),
		shot("retreat", after, [], hit5, pains),
	];
}

/** Cells for the map: the table the battle is fought at, and the cults at home. */
function tableCells() {
	const cells = {};
	const color = (seat) => SEATS[seat - 1].color;
	for (const u of BEFORE) (cells.asia ||= []).push({ piece: u.type, color: color(u.seat) });
	const home = { "south-pacific": 1, africa: 3, europe: 4 };
	for (const [area, seat] of Object.entries(home)) cells[area] = many(seat, "cultist", 3).map((u) => ({ piece: u.type, color: color(u.seat) }));
	return cells;
}

/** Draws the tester into host. info is the description of the game. */
export function mountAnimationTester(host, info) {
	clear(host);
	const boardBox = h("div", { id: "tester-board" });
	const buttons = h("div", { class: "row tester-buttons", id: "tester-buttons" });
	host.append(h("p", { class: "small muted" }, "Plays each animation with no game going. The battle is always the same one, made up to show everything."), boardBox, buttons);
	renderBoard(boardBox, info, { cells: tableCells(), flipped: false, interactive: false, showCoords: false });
	const svg = () => boardBox.querySelector("svg");
	const seatOf = (seat) => SEATS[seat - 1];
	const look = (type) => info.pieces.find((p) => p.type === type) || { label: type, shape: "disc" };
	const shared = {
		board: svg,
		onPage: svg,
		phone: isMobileLayout,
		upright: () => isMobileLayout() && !isLandscape(),
		ink: (seat) => `color-mix(in srgb, var(--piece-${cssColor(seatOf(seat).color)}-fill) 72%, var(--text))`,
		glyph: (seat) => h("img", { class: "seat-glyph", src: `/api/game-defs/cthulhu-wars/assets/guide/glyphs/${seatOf(seat).key}.svg`, alt: seatOf(seat).name, title: seatOf(seat).name, width: 22, height: 22 }),
	};
	const theatre = createBattleTheatre({
		...shared,
		colorOf: (seat) => seatOf(seat).color,
		label: (type) => look(type).label,
		icon: (type, color, px) => pieceIcon(look(type).shape, color, px),
		unitOk: () => false,
		onUnit: () => {},
	});
	const emergence = createEmergence(shared);
	const stopAll = () => {
		theatre.close();
		emergence.close();
	};
	buttons.append(
		h(
			"button",
			{
				type: "button",
				class: "primary",
				id: "tester-battle",
				onclick: () => {
					stopAll();
					theatre.play(`tester|${Date.now()}`, battleSnaps(), BEFORE);
				},
			},
			"Test Battle"
		)
	);
	for (const type of emergenceTypes()) {
		const at = EMERGE_AT[type];
		if (!at) continue;
		const name = look(type).label;
		buttons.append(
			h(
				"button",
				{
					type: "button",
					"data-tester-emerge": type,
					onclick: () => {
						stopAll();
						emergence.play({ type, name, seat: at.seat, area: at.area, areaName: at.areaName, still: false });
					},
				},
				`${name} Emerges`
			)
		);
	}
	window.addEventListener("keydown", (ev) => {
		if (ev.key === "Escape") stopAll();
	});
	window.addEventListener("resize", () => {
		theatre.refresh();
		emergence.refresh();
	});
	return stopAll;
}
