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
// What game.js draws of the ruleset's view: counters, spellbooks, unit cards and
// neutral items, and its battles and awakenings.
export { statIcon } from "./cw-icons.js";
export { spellMark, enterBlock, bookRequirements, abilitiesBlock, unitText, neutralTip } from "./cw-tips.js";
export { battles } from "./cw-view-battles.js";
// The cults' cards, the round and its Ritual track, units and neutral items.
export { cultCards } from "./cw-view-cards.js";
// Its actions' buttons: costs in Power, spellbook marks, units to summon and awaken.
export { actionList } from "./cw-view-actions.js";
export { lobbyPool } from "./cw-lobby.js";
export { batchSections, endReason } from "./cw-batches.js";
export { factionName as pickName, factionColor as pickColor } from "./cw-names.js";

/** A practice batch's option, by name. */
export function optionName(option) {
	return option === "hp" ? "High Priests" : option;
}
export { syncPractice } from "./cw-practice.js";
export { createCwSetup as createSetup } from "./cw-setup.js";
export { mountAnimationTester as mountTester } from "./cw-animation-tester.js";

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

/** The explanation of "When I'm targeted", in Cthulhu Wars' words: its reactions and what aims at a cult. */
export function reactionHelp() {
	return [
		"Reactions are abilities you can use on another player's turn — Devolve, Necrophagy, Cannibalism, Madness, Energy Nexus and the Unspeakable Oath. Each time another player acts, the game works out whether you could react, and would then wait for your answer. So that nobody holds the table up, your reactions are passed for you unless you ask to be asked.",
		"With “When I'm targeted” on (it is by default) you are asked whenever an opponent's action is aimed at:",
		[
			"you, the player — an action that affects you directly, such as The Thousand Forms, Ghroth, a battle declared against you, a gift of Doom or Power, or a Cultist of yours being captured;",
			"one of your units — a Cultist or Monster of yours that is captured, attacked, swapped or replaced;",
			"an area where you have a presence — any unit of yours, or a Gate you control. An enemy that moves into it, recruits or summons in it, builds a Gate or awakens a Great Old One there, or brings Cthulhu up out of the sea there, is aiming at that area.",
		],
		"Only actions count. What happens as part of a battle — Madness sending a unit of yours to an area its player chooses, retreats, kills and pains, post-battle abilities — is not an action, so it never opens a question for you.",
		"When you are not targeted, your reaction is passed for you and the move list shows it as an automatic pass. Switch the option off to be asked less, or add “After any other player's action” or chosen players below to be asked more.",
		"Each reaction has boxes of its own, so you can be asked about one every time and about another only when it is aimed at you. Madness starts with every box ticked: you are asked about it in every battle, whoever fights, until you change it.",
	];
}

/** Its endings, for the game-over line ("Great Cthulhu won with …"). */
export const endReasons = { doom: "with the most Doom and all six spellbooks", "instant-death": "after Instant Death" };

/** The game-over line of a game nobody won, in its own words, or null. */
export function drawnMessage(reason, detail) {
	return reason === "humanity" ? `Game over — humanity survives: no cult had all six spellbooks${detail}.` : null;
}

/** An action's button label with what it is worth just now: revealing Elder Signs says their Doom. */
export function actionLabel(id, label, rv, acting) {
	if (id !== "reveal") return label;
	// Only your own view knows what the signs are worth.
	const sum = ((rv && rv.seats && rv.seats[String(acting)]) || {}).signs || [];
	const total = sum.reduce((a, c) => a + c, 0);
	return total ? `${label} (+${total} Doom)` : label;
}

/** The game options' name for its battles and awakenings. */
export const battleOption = "Battle and Great Old One animations";

/** A unit that holds a Gate, named for the buttons that choose it. */
export function heldLabel(label) {
	return `${label} on the Gate`;
}

/** What choosing a unit on a Gate does, for its button. */
export function heldNote(label, capturing) {
	return capturing ? `${label}: capturing it leaves the Gate abandoned, and your own Cultist takes it` : `${label}: moving it abandons the Gate`;
}

/** What a Pass gives up. */
export function passWarning(power) {
	return `You will lose your ${power} unspent Power.`;
}

/** The pane of the seats, at a table of more than two. */
export const seatsName = "Cults";

/** A Move's units queued from some areas, waiting for more or for where they go. */
export function queueReady(from) {
	return `Units from ${from} are ready: select more from any area, then the area they all go to.`;
}
