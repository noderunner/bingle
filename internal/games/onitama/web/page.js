// Onitama's page script (ADR 0124): the Move cards beside the actions.

import { h } from "/static/js/util.js";

/**
 * The Move cards: both hands and the card at the side, each drawn as the
 * grid of squares it lets a piece reach. A card of the player to move that can
 * be played now is a button. ctx is the page's parts (game.js viewContext).
 */
export function choicePanel(ctx) {
	const { S, actingSeat, activeBuilder, chooseAction, rulesetView } = ctx;
	const v = rulesetView();
	if (!v || !v.cards) return null;
	const seats = S.game.seats || [];
	const mover = v.impulse_seat || 0;
	const b = actingSeat() ? activeBuilder() : null;
	const picked = b && b.gestures[0] && b.gestures[0].kind === "action" ? b.gestures[0].value : null;
	const playable = b ? new Set(b.nextActions()) : new Set();
	const grid = (card, seat) => {
		const g = h("span", { class: "card-grid", "aria-hidden": "true" });
		const marks = new Set();
		for (const [dx, dy] of card.moves) {
			// Seen from the board: the second seat moves the other way.
			const bx = seat === 2 ? -dx : dx;
			const by = seat === 2 ? -dy : dy;
			marks.add(`${2 + bx},${2 - by}`);
		}
		for (let r = 0; r < 5; r++) {
			for (let c = 0; c < 5; c++) {
				const centre = r === 2 && c === 2;
				g.append(h("i", { class: centre ? "centre" : marks.has(`${c},${r}`) ? "reach" : "" }));
			}
		}
		return g;
	};
	const one = (card, seat, ownHand) => {
		const id = `card-${card.id}`;
		const can = ownHand && playable.has(id);
		const label = h("span", { class: "card-name" }, card.name);
		const attrs = { class: "move-card" + (can ? " playable" : "") + (picked === id ? " chosen" : ""), "data-card": card.id, title: card.name };
		if (can) {
			return h("button", { type: "button", ...attrs, onclick: () => chooseAction(id) }, grid(card, seat), label);
		}
		return h("span", attrs, grid(card, seat), label);
	};
	const panel = h("div", { class: "cards-panel", id: "cards-panel", "aria-label": "Move cards" });
	const group = (title, cards, seat, turn, own) => {
		const g = h("div", { class: "card-group" + (turn ? " on-impulse" : "") }, h("div", { class: "card-group-title small muted" }, title));
		const row = h("div", { class: "card-row" });
		for (const c of cards) row.append(one(c, seat, own));
		return g.append(row), g;
	};
	const name = (seat) => ((seats.find((x) => x.seat === seat) || {}).label || `Seat ${seat}`);
	for (const seat of [2, 1]) {
		panel.append(group(`${name(seat)}'s cards`, v.cards.hands[String(seat)] || [], seat, mover === seat, mover === seat && seat === actingSeat()));
		if (seat === 2) panel.append(group("Next card", [v.cards.side], mover || 1, false, false));
	}
	return panel;
}
