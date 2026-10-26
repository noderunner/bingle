// Card games: how the cards of a game are drawn. The game definition's "cards"
// section (suits, card types, zones) arrives with the game info; the position's
// view lists the zones with the cards the viewer may see. This module turns
// that into a table: each seat's hand (a fan of faces for your own, of backs
// for the others), piles with a count, rows and splayed stacks of public cards,
// with a flip when a card is turned over and a slide when one is dealt or
// played. It knows nothing about any game's rules: a card is clickable when the
// game says it can be chosen.
//
// It also draws a board of choices (a "grid" of lines of boxes) for games such
// as Nyet! that settle the terms of a round by crossing boxes off.

import { h } from "./util.js";

/** The type id of a card instance ("blue-1#2" → "blue-1"). */
export function typeId(id) {
	const i = String(id).indexOf("#");
	return i < 0 ? id : id.slice(0, i);
}

function typeOf(info, id) {
	const c = info.cards;
	if (!c) return null;
	if (!c._byId) c._byId = new Map(c.types.map((t) => [t.id, t]));
	return c._byId.get(typeId(id)) || null;
}

/** The face of a card: rank and pip in two corners, the pip large in the middle, its text below. */
export function cardFace(info, id, extra = {}) {
	const t = typeOf(info, id);
	const suit = t && t.suit ? info.cards.suits[t.suit] || {} : {};
	const label = t ? t.label : "?";
	const pip = suit.symbol || "";
	const corner = (cls) => h("span", { class: `pc-corner ${cls}` }, h("span", { class: "pc-rank" }, label), pip ? h("span", { class: "pc-pip" }, pip) : null);
	const mid = h("span", { class: "pc-mid" }, ...(t && t.symbols && t.symbols.length ? t.symbols.map((s) => h("i", {}, s)) : [pip || label]));
	const card = h(
		"span",
		{
			class: "pcard face" + (extra.class ? " " + extra.class : ""),
			style: suit.color ? `--suit: ${suit.color}` : null,
			"data-card": id,
			"data-type": typeId(id),
			title: t ? `${suit.label ? suit.label + " " : ""}${label}${t.text ? " — " + t.text : ""}` : "",
		},
		corner("top"),
		mid,
		t && t.text ? h("span", { class: "pc-text" }, t.text) : null,
		corner("bottom")
	);
	return card;
}

/** The back of a card. */
export function cardBack(info, extra = {}) {
	const back = (info.cards && info.cards.back) || {};
	return h(
		"span",
		{ class: "pcard back" + (extra.class ? " " + extra.class : ""), style: back.color ? `--back: ${back.color}` : null, "aria-label": "A face-down card" },
		back.text ? h("span", { class: "pc-back-text" }, back.text) : null
	);
}

function zoneDef(info, id) {
	return ((info.cards && info.cards.zones) || []).find((z) => z.id === id) || {};
}

/**
 * Draws one zone. cards are {id?, up?, caption?}: an empty id is a card the
 * viewer cannot see (drawn as a back). ctx.playable(typeId) says whether a card
 * can be chosen, ctx.onCard(typeId) chooses it.
 */
function drawZone(info, zv, ctx, next) {
	const def = zoneDef(info, zv.id);
	const layout = def.layout || "row";
	const cards = zv.cards || [];
	const key = zv.owner ? `${zv.id}:${zv.owner}` : zv.id;
	const el = h("div", { class: `zone layout-${layout}`, "data-zone": zv.id, "data-owner": zv.owner || null, "data-count": zv.count });
	const card = (c, i) => {
		const stateKey = `${key}/${i}`;
		const was = ctx.prev ? ctx.prev.get(stateKey) : undefined;
		const face = !!c.id;
		next.set(stateKey, face ? "face" : "back");
		const classes = [];
		if (face && was === "back") classes.push("flip-in");
		else if (was === undefined && ctx.prev && ctx.prev.size) classes.push("deal-in");
		let node = face ? cardFace(info, c.id, { class: classes.join(" ") }) : cardBack(info, { class: classes.join(" ") });
		if (face && ctx.playable && ctx.playable(typeId(c.id))) {
			const btn = h("button", { type: "button", class: "pc-btn playable", "data-card": c.id, "data-type": typeId(c.id), onclick: () => ctx.onCard(typeId(c.id)), title: node.title }, node);
			node = btn;
		}
		const wrap = h("span", { class: "pc-slot", style: `--i: ${i}` }, node);
		if (zv.captions && zv.captions[i]) wrap.append(h("span", { class: "pc-caption" }, zv.captions[i]));
		return wrap;
	};
	if (layout === "stack") {
		// A pile: its top card, or an empty slot, and how many cards it holds.
		const top = cards.length ? cards[cards.length - 1] : null;
		const inner = h("span", { class: "pile" + (zv.count ? "" : " empty") });
		if (zv.count) {
			inner.append(top && top.id ? cardFace(info, top.id) : cardBack(info));
			next.set(`${key}/top`, top && top.id ? "face" : "back");
		}
		inner.append(h("span", { class: "pile-count" }, String(zv.count)));
		el.append(inner);
	} else {
		cards.forEach((c, i) => el.append(card(c, i)));
		if (!cards.length) {
			// An empty zone shows its count if it is a hand that has been played out.
			el.append(h("span", { class: "zone-empty muted small" }, zv.count ? `${zv.count} card${zv.count === 1 ? "" : "s"}` : "—"));
		}
		if (!zv.cards && zv.count) {
			// Only a count was sent: draw that many backs, overlapped.
			el.classList.add("counted");
			for (let i = 0; i < Math.min(zv.count, 12); i++) el.append(h("span", { class: "pc-slot", style: `--i: ${i}` }, cardBack(info)));
		}
	}
	return el;
}

/**
 * The card table: every seat with its zones, and the zones everyone shares.
 *   ctx.info, ctx.view ({table: [...]}), ctx.seats [{seat,label,color}], ctx.me,
 *   ctx.toMove, ctx.playable, ctx.onCard, ctx.prev (Map) → returns {node, state}
 */
export function cardTable(ctx) {
	const { info, view } = ctx;
	const zones = (view && view.table) || [];
	const next = new Map();
	const byOwner = new Map();
	const shared = [];
	for (const z of zones) {
		if (z.owner) {
			if (!byOwner.has(z.owner)) byOwner.set(z.owner, []);
			byOwner.get(z.owner).push(z);
		} else {
			shared.push(z);
		}
	}
	const order = (list) => {
		const defs = (info.cards && info.cards.zones) || [];
		return [...list].sort((a, b) => defs.findIndex((d) => d.id === a.id) - defs.findIndex((d) => d.id === b.id));
	};
	const seatBox = (s) => {
		const mine = s.seat === ctx.me;
		const box = h("div", { class: "ct-seat" + (mine ? " me" : "") + (ctx.toMove === s.seat ? " to-move" : ""), "data-seat": s.seat });
		box.append(
			h(
				"div",
				{ class: "ct-seat-name" },
				h("span", { class: `swatch color-${String(s.color || "").toLowerCase()}` }),
				h("strong", {}, s.label || `Seat ${s.seat}`),
				mine ? h("span", { class: "muted small" }, " (you)") : null,
				ctx.dealer === s.seat ? h("span", { class: "ct-badge", title: "Dealer" }, "D") : null,
				ctx.bonus === s.seat ? h("span", { class: "ct-badge bonus", title: "Holds the x2 bonus card" }, "×2") : null,
				ctx.team && ctx.team.includes(s.seat) ? h("span", { class: "ct-badge team", title: "On the leader's team" }, "★") : null
			)
		);
		const hand = h("div", { class: "ct-zones" });
		for (const z of order(byOwner.get(s.seat) || [])) {
			const def = zoneDef(info, z.id);
			const label = def.label || z.id;
			const part = h("div", { class: `ct-zone zone-${z.id}` });
			if (z.id !== "hand" || z.count > 0 || !mine) part.append(h("div", { class: "ct-zone-label small muted" }, def.counts || def.layout === "stack" ? `${label}` : label));
			part.append(drawZone(info, z, ctx, next));
			if (z.count === 0 && z.id !== "hand") continue;
			hand.append(part);
		}
		box.append(hand);
		return box;
	};
	const node = h("div", { class: "card-table", id: "card-table", "aria-label": "Cards" });
	const others = ctx.seats.filter((s) => s.seat !== ctx.me);
	const row = h("div", { class: "ct-others" });
	for (const s of others) row.append(seatBox(s));
	node.append(row);
	const mid = h("div", { class: "ct-center" });
	for (const z of order(shared)) {
		const def = zoneDef(info, z.id);
		if (z.id === "last" && !z.count) continue;
		if (z.id === "deck" && !z.count) continue;
		if (z.id !== "deck" && z.id !== "last" && z.count === 0 && z.id !== "trick") continue;
		mid.append(h("div", { class: `ct-zone zone-${z.id}` }, h("div", { class: "ct-zone-label small muted" }, def.label || z.id), drawZone(info, z, ctx, next)));
	}
	node.append(mid);
	const me = ctx.seats.find((s) => s.seat === ctx.me);
	if (me) node.append(seatBox(me));
	return { node, state: next };
}

/**
 * A board of choices: lines of boxes, each open, crossed off (by a seat),
 * covered, or the one left open (chosen). A box is clickable when the game
 * offers the action `cross-<line>-<box>`.
 */
export function choiceGrid(grid, ctx) {
	const root = h("div", { class: "choice-grid", id: "choice-grid", role: "table", "aria-label": "The board of terms" });
	for (const line of grid.lines) {
		const boxes = h("div", { class: "cg-boxes" });
		for (const b of line.boxes) {
			const id = `cross-${line.id}-${b.id}`;
			const can = b.state === "open" && ctx.actions.has(id);
			const crosser = b.by ? ctx.seats.find((s) => s.seat === b.by) : null;
			const kids = [];
			if (b.color) kids.push(h("span", { class: "cg-swatch", style: `background:${b.color}` }));
			kids.push(h("span", { class: "cg-label" }, b.label));
			if (b.state === "crossed") kids.push(h("span", { class: "cg-cross", "aria-hidden": "true" }, "✕"));
			if (crosser) kids.push(h("span", { class: `cg-by swatch color-${String(crosser.color).toLowerCase()}`, title: `Crossed by ${crosser.label}` }));
			const attrs = { class: `cg-box ${b.state}` + (can ? " clickable" : ""), "data-line": line.id, "data-box": b.id, "data-state": b.state, title: line.hint };
			boxes.append(can ? h("button", { type: "button", ...attrs, onclick: () => ctx.onAction(id) }, ...kids) : h("span", attrs, ...kids));
		}
		root.append(h("div", { class: "cg-line", "data-line": line.id }, h("div", { class: "cg-title", title: line.hint }, line.label), boxes));
	}
	return root;
}
