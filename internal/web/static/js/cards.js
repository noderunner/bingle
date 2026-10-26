// Card games: how the cards of a game are drawn. The game definition's "cards"
// section (suits, card types, zones) arrives with the game info; the position's
// view lists the zones with the cards the viewer may see. This module turns
// that into a table: your own hand (a fan of faces), piles with a count, rows
// and splayed stacks of public cards, with a flip when a card is turned over
// and a slide when one is dealt or played. It knows nothing about any game's
// rules: a card is clickable when the game says it can be chosen.
//
// The other players' hands are not drawn: the game page lists every player with
// the number of cards held. Your own hand shows a card twice as large while the
// pointer is over it (or it has the focus), for any game whose hand is private
// to its owner (a zone with the visibility "owner").
//
// It also draws a board of choices (a "grid" of lines of boxes) for games such
// as Nyet! that settle the terms of a round by crossing boxes off. Each box is
// a picture (see choice-icons.js); a box that has been crossed off shows the
// player who crossed it.

import { h } from "./util.js";
import { cardFaceSvg, isClassicSuit } from "./card-art.js";
import { choiceIcon } from "./choice-icons.js";

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

/**
 * The face of a card. The four classic suits are drawn as pictures (the rank in
 * two corners, the pips of its value); any other deck gets the rank and pip in
 * two corners, the pip large in the middle and its text below.
 */
export function cardFace(info, id, extra = {}) {
	const t = typeOf(info, id);
	const suit = t && t.suit ? info.cards.suits[t.suit] || {} : {};
	const label = t ? t.label : "?";
	const pip = suit.symbol || "";
	const cls = "pcard face" + (extra.class ? " " + extra.class : "");
	const style = suit.color ? `--suit: ${suit.color}` : null;
	if (t && isClassicSuit(pip) && !t.text && !(t.symbols && t.symbols.length)) {
		const name = `${label} of ${(suit.label || "").toLowerCase()}`;
		return h("span", { class: cls, style, "data-card": id, "data-type": typeId(id), title: name }, cardFaceSvg(pip, label));
	}
	const corner = (c) => h("span", { class: `pc-corner ${c}` }, h("span", { class: "pc-rank" }, label), pip ? h("span", { class: "pc-pip" }, pip) : null);
	const mid = h("span", { class: "pc-mid" }, ...(t && t.symbols && t.symbols.length ? t.symbols.map((s) => h("i", {}, s)) : [pip || label]));
	return h(
		"span",
		{ class: cls, style, "data-card": id, "data-type": typeId(id), title: t ? `${suit.label ? suit.label + " " : ""}${label}${t.text ? " — " + t.text : ""}` : "" },
		corner("top"),
		mid,
		t && t.text ? h("span", { class: "pc-text" }, t.text) : null,
		corner("bottom")
	);
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

// ---- the card under the pointer, twice as large ------------------------------------

let zoomed = null;

/** Takes the enlarged card away (the game page does it whenever it redraws). */
export function hideCardZoom() {
	if (zoomed) {
		zoomed.remove();
		zoomed = null;
	}
}

function showCardZoom(card) {
	hideCardZoom();
	const r = card.getBoundingClientRect();
	const w = r.width * 2;
	const hh = r.height * 2;
	const z = card.cloneNode(true);
	for (const a of ["data-card", "data-type", "title", "id"]) z.removeAttribute(a);
	z.classList.remove("flip-in", "deal-in");
	z.classList.add("pc-zoom");
	z.setAttribute("aria-hidden", "true");
	// Above the card; below it when the card is near the top; always inside the window.
	const left = Math.min(Math.max(8, r.left + r.width / 2 - w / 2), Math.max(8, window.innerWidth - w - 8));
	let top = r.top - hh - 10;
	if (top < 8) top = Math.min(r.bottom + 10, Math.max(8, window.innerHeight - hh - 8));
	z.style.width = `${w}px`;
	z.style.height = `${hh}px`;
	z.style.left = `${left}px`;
	z.style.top = `${top}px`;
	document.body.append(z);
	zoomed = z;
}

/** Makes the faces of a zone show large while the pointer or the focus is on one. */
function enableZoom(zone) {
	const faceOf = (el) => (el && el.closest ? el.closest(".pcard.face") : null);
	zone.addEventListener("pointerover", (ev) => {
		if (ev.pointerType === "touch") return;
		const c = faceOf(ev.target);
		if (c && zone.contains(c)) showCardZoom(c);
	});
	zone.addEventListener("pointerout", (ev) => {
		const to = faceOf(ev.relatedTarget);
		if (!to || !zone.contains(to)) hideCardZoom();
	});
	zone.addEventListener("pointerdown", hideCardZoom);
	zone.addEventListener("focusin", (ev) => {
		const c = ev.target.querySelector ? ev.target.querySelector(".pcard.face") : null;
		if (c) showCardZoom(c);
	});
	zone.addEventListener("focusout", hideCardZoom);
}

function zoneDef(info, id) {
	return ((info.cards && info.cards.zones) || []).find((z) => z.id === id) || {};
}

/** The caption under a card: the player's picture when the page can draw one (ctx.seatIconByLabel), else the words. */
function captionNode(ctx, text) {
	const star = /\s★$/.test(text);
	const name = text.replace(/\s★$/, "");
	const icon = ctx.seatIconByLabel ? ctx.seatIconByLabel(name, 26) : null;
	if (!icon) return h("span", { class: "pc-caption" }, text);
	return h("span", { class: "pc-caption icon", title: text }, icon, star ? h("span", { class: "pc-star", "aria-label": "won the trick" }, "★") : null);
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
	if (def.visibility === "owner" && zv.owner && zv.owner === ctx.me) {
		el.classList.add("zoomable");
		enableZoom(el);
	}
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
		if (zv.captions && zv.captions[i]) wrap.append(captionNode(ctx, zv.captions[i]));
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
 * The card table: what the players share (the trick, the piles) and the
 * viewer's own zones (their hand).
 *   ctx.info, ctx.view ({table: [...]}), ctx.seats [{seat,label,color}], ctx.me,
 *   ctx.toMove, ctx.playable, ctx.onCard, ctx.prev (Map),
 *   ctx.seatIconByLabel(label, px) (the picture of the player a caption names),
 *   ctx.top (a node drawn beside the shared zones: the board of terms)
 * → {node, state}
 */
export function cardTable(ctx) {
	const { info, view } = ctx;
	const zones = (view && view.table) || [];
	const next = new Map();
	const mine = zones.filter((z) => z.owner && z.owner === ctx.me);
	const shared = zones.filter((z) => !z.owner);
	const order = (list) => {
		const defs = (info.cards && info.cards.zones) || [];
		return [...list].sort((a, b) => defs.findIndex((d) => d.id === a.id) - defs.findIndex((d) => d.id === b.id));
	};
	const mid = h("div", { class: "ct-center" });
	for (const z of order(shared)) {
		const def = zoneDef(info, z.id);
		if (z.id === "last" && !z.count) continue;
		if (z.id === "deck" && !z.count) continue;
		if (z.id !== "deck" && z.id !== "last" && z.count === 0 && z.id !== "trick") continue;
		mid.append(h("div", { class: `ct-zone zone-${z.id}` }, h("div", { class: "ct-zone-label small muted" }, def.label || z.id), drawZone(info, z, ctx, next)));
	}
	const node = h("div", { class: "card-table", id: "card-table", "aria-label": "Cards" });
	const top = h("div", { class: "ct-top" });
	if (ctx.top) top.append(ctx.top);
	top.append(mid);
	node.append(top);

	const me = ctx.seats.find((s) => s.seat === ctx.me);
	if (me) {
		const box = h("div", { class: "ct-seat me" + (ctx.toMove === me.seat ? " to-move" : ""), "data-seat": me.seat });
		const parts = h("div", { class: "ct-zones" });
		for (const z of order(mine)) {
			const def = zoneDef(info, z.id);
			if (z.count === 0 && z.id !== "hand") continue;
			const part = h("div", { class: `ct-zone zone-${z.id}` });
			if (z.id !== "hand" || z.count > 0) part.append(h("div", { class: "ct-zone-label small muted" }, def.label || z.id));
			part.append(drawZone(info, z, ctx, next));
			parts.append(part);
		}
		box.append(parts);
		node.append(box);
	}
	return { node, state: next };
}

/**
 * A board of choices: lines of boxes, each open, crossed off (by a seat),
 * covered, or the one left open (chosen). A box is clickable when the game
 * offers the action `cross-<line>-<box>`. A box and the head of a line are
 * pictures (their words are the tooltip); a box crossed off shows the picture of
 * the player who crossed it in place of its own.
 *   ctx.actions (Set), ctx.onAction(id), ctx.seats, ctx.info
 */
export function choiceGrid(grid, ctx) {
	const widest = Math.max(...grid.lines.map((l) => l.boxes.length));
	const root = h("div", { class: "choice-grid", id: "choice-grid", role: "table", "aria-label": "The board of terms", style: `--cols: ${widest}` });
	for (const line of grid.lines) {
		const boxes = h("div", { class: "cg-boxes", role: "presentation", style: `--n: ${line.boxes.length}` });
		for (const b of line.boxes) {
			const id = `cross-${line.id}-${b.id}`;
			const can = b.state === "open" && ctx.actions.has(id);
			const crosser = b.by ? ctx.seats.find((s) => s.seat === b.by) : null;
			let words = b.label;
			if (b.state === "crossed") words += crosser ? `: crossed off by ${crosser.label}` : ": crossed off";
			else if (b.state === "chosen") words += ": left open, the rule of the round";
			else if (b.state === "covered") words += ": not at the table";
			const pic = b.state === "crossed" && b.by_icon ? choiceIcon(b.by_icon, ctx, 34) : choiceIcon(b.icon, ctx, 34);
			const attrs = { class: `cg-box ${b.state}` + (can ? " clickable" : ""), "data-line": line.id, "data-box": b.id, "data-state": b.state, "data-by": b.by || null, title: words, "aria-label": words, role: "cell" };
			const kids = [pic || h("span", { class: "cg-label" }, b.label)];
			boxes.append(can ? h("button", { type: "button", ...attrs, onclick: () => ctx.onAction(id) }, ...kids) : h("span", attrs, ...kids));
		}
		const head = choiceIcon(line.icon, ctx, 30);
		root.append(
			h(
				"div",
				{ class: "cg-line", "data-line": line.id, role: "row" },
				h("div", { class: "cg-title", role: "rowheader", title: `${line.label}: ${line.hint}`, "aria-label": line.label }, head || line.label),
				boxes
			)
		);
	}
	return root;
}
