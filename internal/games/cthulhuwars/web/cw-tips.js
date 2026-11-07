// What a hover card says about a unit or spellbook of Cthulhu Wars, shared by the
// game page and the setup page's picker.

import { h } from "/static/js/util.js";
import { pieceIcon } from "/static/js/render.js";
import { statIcon } from "./cw-icons.js";

/**
 * The spellbook pictogram: in colour for a spellbook that is earned (or a
 * requirement that is met), greyed out for one that is not.
 */
export function spellMark(on, label, px = 15) {
	const icon = statIcon("books", label || (on ? "Spellbook" : "Spellbook not yet earned"), px);
	icon.classList.add("spell-mark", on ? "on" : "off");
	return icon;
}

/** "How to awaken": the numbered steps of a Great Old One, or the one line of where another unit comes into play. */
export function enterBlock(unit) {
	if (unit.awaken && unit.awaken.length) {
		const ol = h("ol", { class: "awaken-steps small" });
		for (const step of unit.awaken) ol.append(h("li", {}, step));
		return h("div", { class: "unit-enter unit-awaken" }, h("p", { class: "small" }, h("strong", {}, "How to awaken")), ol);
	}
	if (unit.enter) return h("p", { class: "unit-enter small" }, h("strong", {}, "Where: "), unit.enter);
	return null;
}

/** What earns the spellbook of an Independent Great Old One, until it is earned. */
export function bookRequirements(unit) {
	return unit.books.filter((b) => b.req && !b.unlocked).map((b) => h("p", { class: "unit-enter small" }, h("strong", {}, "Requirement: "), b.req));
}

/** The named abilities of a unit's card (a Great Old One's Desecrate, Vengeance, Devour…). */
export function abilitiesBlock(unit) {
	if (!unit.abilities || !unit.abilities.length) return null;
	const wrap = h("div", { class: "unit-abilities" });
	for (const a of unit.abilities) {
		wrap.append(
			h("p", { class: "unit-ability small", "data-ability": a.name }, h("strong", {}, a.name), h("span", { class: "muted" }, ` (${a.timing})`), `: ${a.text}`)
		);
	}
	return wrap;
}

/** A unit's own text without the "Awaken for …" sentences the steps above it say again. */
export function unitText(unit) {
	let text = unit.text || "";
	if (unit.awaken && unit.awaken.length) {
		text = text
			.split(/\.\s+/)
			.filter((x) => x && !/^Awaken /.test(x))
			.map((x) => (x.endsWith(".") ? x : x + "."))
			.join(" ");
	}
	return text;
}

/** The hover card of a neutral item: its rules, the price and, for a unit, its numbers. */
export function neutralTip(item, pieces) {
	const box = h("div", { class: "unit-tip neutral-tip", "data-item": item.id });
	const def = pieces.find((p) => p.type === item.id);
	const head = h("div", { class: "unit-tip-head" });
	if (item.kind === "book") head.append(spellMark(true, item.name, 28));
	else head.append(pieceIcon(def ? def.shape : "disc", "black", 34));
	const label = { book: "Neutral spellbook", monster: "Neutral Monster", terror: "Terror", goo: "Independent Great Old One" }[item.kind];
	head.append(h("div", { class: "unit-tip-title" }, h("strong", {}, item.name), h("span", { class: "muted small" }, label)));
	box.append(head);
	const stats = h("dl", { class: "unit-stats" });
	const stat = (name, value) => stats.append(h("div", {}, h("dt", {}, name), h("dd", {}, value)));
	if (item.kind === "book") {
		stat("When", item.timing);
	} else {
		stat(item.kind === "goo" ? "To awaken" : "To take", item.kind === "goo" ? `${item.cost} Power` : item.obtain);
		if (item.kind !== "goo") stat("To summon", `${item.cost} Power`);
		const combat = /^-?\d+$/.test(item.combat) ? (item.combat === "0" ? "none" : `${item.combat} ${item.combat === "1" ? "die" : "dice"}`) : item.combat;
		stat("Combat", combat);
		stat("In the box", String(item.total));
	}
	box.append(stats);
	if (item.kind === "book") {
		box.append(h("p", { class: "unit-text small" }, item.text), h("p", { class: "unit-enter small" }, h("strong", {}, "How: "), item.how));
		return box;
	}
	const abilities = abilitiesBlock(item);
	if (abilities) box.append(abilities);
	const enter = enterBlock(item);
	if (enter) box.append(enter);
	if (item.kind !== "goo") box.append(h("p", { class: "unit-enter small" }, h("strong", {}, "The card: "), item.how));
	if (item.text) box.append(h("p", { class: "unit-text small" }, unitText(item)));
	if (item.book) {
		box.append(
			h("p", { class: "small muted unit-books-title" }, "Spellbook"),
			h(
				"ul",
				{ class: "card-rows unit-books" },
				h(
					"li",
					{ class: "todo" },
					h("span", { class: "row-mark" }, spellMark(false, item.book.name)),
					h("span", { class: "row-text" }, h("strong", {}, item.book.name), h("span", { class: "muted small" }, ` (${item.book.timing}): ${item.book.text}`))
				)
			)
		);
		if (item.req) box.append(h("p", { class: "unit-enter small" }, h("strong", {}, "Requirement: "), item.req));
	}
	return box;
}
