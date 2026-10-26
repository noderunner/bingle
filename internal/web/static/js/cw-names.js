// Names and colours of Cthulhu Wars' cults and units, shared by the practice
// wizard and the batch reports.

export const FACTIONS = {
	gc: { name: "Great Cthulhu", color: "green" },
	cc: { name: "Crawling Chaos", color: "blue" },
	bg: { name: "Black Goat", color: "red" },
	ys: { name: "Yellow Sign", color: "yellow" },
	ww: { name: "Windwalker", color: "white" },
	sl: { name: "Sleeper", color: "orange" },
	ow: { name: "Opener of the Way", color: "purple" },
	tt: { name: "Tcho-Tcho", color: "brown" },
};

export const FACTION_KEYS = Object.keys(FACTIONS);

export const factionName = (k) => (FACTIONS[k] ? FACTIONS[k].name : k);

/** The CSS colour of a cult (a custom property of the stylesheet). */
export function factionColor(k) {
	const c = FACTIONS[k] ? FACTIONS[k].color : "white";
	return `var(--piece-${c}-fill)`;
}

/** "flying-polyp" -> "Flying Polyp". */
export function pretty(id) {
	return String(id)
		.split(/[-_ ]/)
		.map((w) => (w ? w.charAt(0).toUpperCase() + w.slice(1) : w))
		.join(" ");
}

export const BOTS = [
	{ id: "tactical", label: "Tactical", help: "Fast. Plays the strategy guide's filtered moves, avoiding obvious blunders." },
	{ id: "minimax", label: "Minimax", help: "Fast. Searches a bounded number of nodes ahead." },
	{ id: "mcts", label: "MCTS", help: "Slow but strongest: Monte-Carlo tree search. More iterations play better and slower." },
	{ id: "random", label: "Random", help: "Baseline. Any legal move." },
];
