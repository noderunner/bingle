// Cthulhu Wars' action buttons on the game page (ADR 0124): each action with
// the Power it costs just now and the spellbook it comes from, a unit to
// summon or awaken as its own button, and their hover cards. game.js asks
// page.js's actionList once, and draws a plain button for a game without one.

import { isMobileLayout } from "/static/js/mobile.js";
import { pieceIcon } from "/static/js/render.js";
import { hoverCard } from "/static/js/tooltip.js";
import { h, s } from "/static/js/util.js";

/** The action buttons of one game page. */
export function actionList(ctx) {
	const { S, actionCost, chooseAction, colorOf, spellMark, statIcon, unitCardFor, unitTip } = ctx;

	/** The button that summons or awakens one kind of unit: icon, name and the cost it has now. */
	function unitButton(kind, type, seat, actionId) {
		const unit = unitCardFor(seat, type);
		if (!unit) return null;
		const color = colorOf(seat);
		const piece = S.info.pieces.find((p) => p.type === type);
		const id = actionId || `${kind}-${type}`;
		const verb = kind === "awaken" ? "Awaken" : kind === "recruit" ? "Recruit" : "Summon";
		const cost = actionCost(id) ?? unit.cost;
		const button = h(
			"button",
			{
				id: `action-${id}`,
				class: "summon-button unit-button",
				"data-action": id,
				"data-piece": type,
				"data-cost": cost,
				onclick: () => chooseAction(id),
				"aria-label": `${verb} ${unit.name} for ${cost} Power`,
			},
			pieceIcon(piece ? piece.shape : "disc", color, 22),
			// The hover card names the unit. A phone has no hover, and keeps the words.
			kind !== "awaken" && !isMobileLayout() ? null : h("span", { class: "summon-name" }, `${verb} ${unit.name}`),
			h("span", { class: "summon-cost", title: "Power" }, `${cost}`, statIcon("power", "Power", 13))
		);
		hoverCard(button, () => unitTip(unit, color, cost));
		return button;
	}

	/** The hover card of an action: its name, what it costs just now, and the rule in a few lines. */
	function actionTip(def, id, label) {
		const box = h("div", { class: "unit-tip action-tip" });
		const cost = actionCost(id);
		// A spellbook's rule reads "Timing: what it does".
		let timing = "";
		let text = def.hint || "";
		if (id.startsWith("book-")) {
			const i = text.indexOf("\n");
			if (i > 0) {
				timing = text.slice(0, i);
				text = text.slice(i + 1);
			}
		}
		box.append(
			h(
				"div",
				{ class: "unit-tip-head" },
				h(
					"div",
					{ class: "unit-tip-title" },
					h("strong", {}, label || def.label),
					timing ? h("span", { class: "muted small" }, timing) : cost !== null ? h("span", { class: "muted small" }, `${cost} Power`) : null
				)
			)
		);
		if (cost !== null && timing) box.append(h("p", { class: "action-cost small" }, h("strong", {}, "Cost: "), `${cost} Power`));
		if (text) box.append(h("p", { class: "unit-text small" }, text));
		if (def.source === "book") {
			box.append(h("p", { class: "action-source small muted" }, spellMark(true, "Unlocked spellbook", 14), " An ability of a spellbook you have unlocked."));
		} else if (def.source === "requirement") {
			box.append(h("p", { class: "action-source small muted" }, spellMark(false, "Spellbook requirement", 14), " Meets a spellbook requirement: it earns a spellbook of your choice."));
		}
		return box;
	}

	/** A button of the action list: its name with the Power it costs just now, and a card with the rule on hover. */
	function actionButton(id, def, label, rv) {
		const cost = actionCost(id);
		// "Pay 4 Power" and "Give 3 Power to …" say what they cost already.
		const named = /^(pay|bestow)-/.test(id);
		const priced = cost !== null && !named;
		// An ability a spellbook gives wears a spellbook; an action that meets a
		// spellbook requirement wears one greyed out (the book is still to come).
		const source = def && def.source ? def.source : "";
		const mark =
			source === "book"
				? spellMark(true, "Unlocked spellbook")
				: source === "requirement"
				? spellMark(false, "Spellbook requirement")
				: null;
		const button = h(
			"button",
			{
				id: `action-${id}`,
				class: source ? `has-spell spell-${source}` : null,
				"data-action": id,
				"data-cost": cost === null ? null : cost,
				"data-source": source || null,
				"aria-label": priced ? `${label}, ${cost} Power` : label,
				onclick: () => chooseAction(id),
			},
			mark,
			// One span, as a button spaces out its children: the icon sits among the words.
			h("span", {}, ...powerWords(label)),
			priced ? h("span", { class: "summon-cost", title: "Power" }, `${cost}`, statIcon("power", "Power", 13)) : null
		);
		if (def && def.hint) hoverCard(button, () => actionTip(def, id, label));
		return button;
	}

	/** A label with the word Power drawn as its icon. */
	function powerWords(label) {
		return String(label)
			.split(/\bPower\b/)
			.flatMap((part, i) => (i ? [statIcon("power", "Power", 13), part] : [part]));
	}

	return { actionTip, actionButton, unitButton, powerWords };
}
