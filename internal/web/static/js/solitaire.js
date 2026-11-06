// The table of a game of patience (Klondike, Spider, FreeCell) and of Bowling
// Solitaire. The cards come as the zones of any card game (view.table), and a move is
// the two clicks the server lists: from-<pile>-<count> and to-<pile>, or the pins
// (pin-<letter>) and then the ball (ball-<pile>). The page lays them out; the rules are
// the server's, and a card can be picked up exactly when a move starts with it.

import { h } from "./util.js";
import { cardBack, cardFace, enableZoom, holdCardZoom } from "./cards.js";
import { currentCardSet, setSuitIcon } from "./card-sets.js";

const pinRows = ["abcd", "efg", "hi", "j"];

/** The zones of the view by id. */
function zonesOf(view) {
	return new Map((view.table || []).map((z) => [z.id, z]));
}

/**
 * A card that says which colour it counts as. A themed set's suits are not red and
 * black, and where colours are the rule a player has to be able to tell at a glance.
 */
function face(ctx, id) {
	const node = cardFace(ctx.info, id);
	if (ctx.view.alternate && currentCardSet(ctx.info)) {
		const t = (ctx.info.cards.types || []).find((x) => x.id === String(id).split("#")[0]);
		const suit = t ? ctx.info.cards.suits[t.suit] : null;
		if (suit) node.append(h("span", { class: "sol-colour", style: `color: ${suit.color}`, "aria-hidden": "true" }, suit.symbol));
	}
	return node;
}

// ---- patience ---------------------------------------------------------------

/** What clicking a card (or its pile) does. */
function clickPile(ctx, pile, count) {
	const options = ctx.options();
	const picked = ctx.picked();
	const from = count ? `from-${pile}-${count}` : "";
	if (picked.length) {
		const again = from && picked[0] === from;
		// A card clicked twice goes home, if it can.
		if (again && options.includes("to-f")) return ctx.onAction("to-f");
		if (again) return ctx.cancelMove();
		const to = `to-${pile.replace(/^[fc]\d+$/, (m) => m[0])}`;
		const target = options.includes(to) ? to : options.includes(`to-${pile}`) ? `to-${pile}` : "";
		if (target) return ctx.onAction(target);
		ctx.cancelMove();
		if (!from || !ctx.options().includes(from)) return;
	}
	if (from && ctx.options().includes(from)) lift(ctx, from);
}

/** Picks up cards; if they can go to only one place, they go there at once. */
function lift(ctx, from) {
	ctx.onAction(from);
	const next = ctx.options();
	if (next.length === 1) ctx.onAction(next[0]);
}

/** A pile's top-level attributes: where a drop lands, and whether it is a target now. */
function pileAttrs(ctx, pile, extra) {
	const options = ctx.options();
	const named = pile.replace(/^[fc]\d+$/, (m) => m[0]);
	const target = ctx.picked().length && (options.includes(`to-${pile}`) || options.includes(`to-${named}`));
	return { ...extra, class: `${extra.class || ""}${target ? " sol-target" : ""}`, "data-pile": pile };
}

/** One card of a pile, as a button when a move can start with it. */
function cardIn(ctx, pile, c, count, attrs = {}) {
	const node = c.id ? face(ctx, c.id) : cardBack(ctx.info);
	if (!c.id) return h("span", { class: "sol-card", ...attrs }, node);
	const from = `from-${pile}-${count}`;
	const can = ctx.options().includes(from);
	const picked = ctx.picked()[0] === from;
	const target = ctx.picked().length && pileTargetted(ctx, pile);
	if (!can && !target) return h("span", { class: "sol-card" + (picked ? " sol-picked" : ""), ...attrs }, node);
	return h(
		"button",
		{
			type: "button",
			class: "sol-card pc-btn" + (can ? " sol-movable" : "") + (picked ? " sol-picked" : ""),
			"data-from": from,
			"data-count": count,
			title: node.title,
			onclick: () => {
				if (dragged) return;
				clickPile(ctx, pile, can ? count : 0);
			},
			onpointerdown: (ev) => can && startDrag(ctx, ev, from),
			...attrs,
		},
		node
	);
}

function pileTargetted(ctx, pile) {
	const options = ctx.options();
	return options.includes(`to-${pile}`) || options.includes(`to-${pile.replace(/^[fc]\d+$/, (m) => m[0])}`);
}

/** A pile drawn as one place: the waste, a foundation, a free cell. */
function slot(ctx, pile, zone, placeholder, label) {
	const cards = (zone && zone.cards) || [];
	const top = cards.length ? cards[cards.length - 1] : null;
	const el = h("div", pileAttrs(ctx, pile, { class: "sol-slot", "aria-label": label }));
	if (top) el.append(cardIn(ctx, pile, top, 1));
	else {
		const empty = h("span", { class: "sol-empty" }, placeholder || "");
		if (ctx.picked().length && pileTargetted(ctx, pile)) el.append(h("button", { type: "button", class: "sol-card pc-btn", "aria-label": label, onclick: () => clickPile(ctx, pile, 0) }, empty));
		else el.append(empty);
	}
	return el;
}

/** The stock: clicking it draws, recycles or deals, whichever the game offers. */
function stock(ctx, zone) {
	const n = zone ? zone.count : 0;
	const options = ctx.options();
	const act = ["draw", "recycle", "deal"].find((a) => options.includes(a));
	const body = n ? cardBack(ctx.info) : h("span", { class: "sol-empty" }, options.includes("recycle") ? "↻" : "");
	const count = h("span", { class: "pile-count" }, String(n));
	const label = act === "recycle" ? "Turn the waste over" : act === "deal" ? "Deal a row" : "Stock";
	const attrs = { id: "sol-stock", class: "sol-slot sol-stock", "data-pile": "stock", title: label };
	if (!act) return h("div", attrs, body, count);
	return h("button", { ...attrs, type: "button", "data-does": act, "aria-label": label, onclick: () => ctx.onAction(act) }, body, count);
}

/** The waste: the top card, and in a game of three the two under it, fanned. */
function waste(ctx, zone) {
	const cards = (zone && zone.cards) || [];
	const shown = Math.min(cards.length, ctx.info.params && ctx.info.params.draw === 3 ? 3 : 1);
	const el = h("div", pileAttrs(ctx, "w", { class: "sol-slot sol-waste", style: `--shown: ${Math.max(shown, 1)}`, "aria-label": "Waste" }));
	if (!cards.length) el.append(h("span", { class: "sol-empty" }));
	cards.slice(cards.length - shown).forEach((c, i) => {
		const top = i === shown - 1;
		el.append(top ? cardIn(ctx, "w", c, 1, { style: `--i: ${i}` }) : h("span", { class: "sol-card", style: `--i: ${i}` }, face(ctx, c.id)));
	});
	return el;
}

/** A column: cards overlapped from the first to the top one. */
function column(ctx, i, zone) {
	const pile = `t${i}`;
	const cards = (zone && zone.cards) || [];
	const el = h("div", pileAttrs(ctx, pile, { class: "sol-column", "aria-label": `Column ${i}` }));
	if (!cards.length) {
		const empty = h("span", { class: "sol-empty" });
		el.append(ctx.picked().length && pileTargetted(ctx, pile) ? h("button", { type: "button", class: "sol-card pc-btn", "aria-label": `Column ${i}`, onclick: () => clickPile(ctx, pile, 0) }, empty) : empty);
		return el;
	}
	cards.forEach((c, k) => {
		el.append(cardIn(ctx, pile, c, cards.length - k));
	});
	return el;
}

function foundationMark(ctx, suitId) {
	const set = currentCardSet(ctx.info);
	if (set) return setSuitIcon(set, ctx.info, suitId, 30);
	const suit = ctx.info.cards.suits[suitId] || {};
	return h("span", { style: `color: ${suit.color}` }, suit.symbol || "");
}

function patience(ctx) {
	const { view } = ctx;
	const zones = zonesOf(view);
	const top = h("div", { class: "sol-top" });
	const left = h("div", { class: "sol-row" });
	const right = h("div", { class: "sol-row" });
	if (view.solitaire === "klondike") left.append(stock(ctx, zones.get("stock")), waste(ctx, zones.get("waste")));
	if (view.solitaire === "spider") {
		left.append(stock(ctx, zones.get("stock")));
		const done = zones.get("done");
		const count = done ? done.count : 0;
		right.append(h("div", { class: "sol-done small muted", id: "sol-done" }, `${count / 13} of ${view.total / 13} suits finished`));
	}
	for (let i = 1; i <= (view.cells || 0); i++) left.append(slot(ctx, `c${i}`, zones.get(`c${i}`), "", `Free cell ${i}`));
	(view.suits || []).forEach((suit, i) => right.append(slot(ctx, `f${i + 1}`, zones.get(`f${i + 1}`), foundationMark(ctx, suit), `Foundation ${i + 1}`)));
	top.append(left, right);
	const cols = h("div", { class: "sol-columns", style: `--n: ${view.columns}` });
	for (let i = 1; i <= view.columns; i++) cols.append(column(ctx, i, zones.get(`t${i}`)));
	return h("div", { class: "card-table sol-table", id: "card-table", "data-mode": view.solitaire, style: `--n: ${view.columns}` }, top, cols);
}

// ---- dragging -----------------------------------------------------------------

// True between the end of a drag and the click that follows it, which is not a click.
let dragged = false;

/**
 * Lets a card be dragged to where it should go. The cards on top of it come along; a
 * drop on a pile that takes them plays the move, and any other drop changes nothing.
 */
function startDrag(ctx, ev, from) {
	if (ev.button) return;
	const source = ev.currentTarget;
	const startX = ev.clientX;
	const startY = ev.clientY;
	let ghost = null;
	const finish = (drop) => {
		window.removeEventListener("pointermove", move);
		window.removeEventListener("pointerup", up);
		window.removeEventListener("pointercancel", cancel);
		if (!ghost) return;
		holdCardZoom(false);
		ghost.remove();
		ghost = null;
		dragged = true;
		setTimeout(() => (dragged = false), 0);
		if (drop) drop();
	};
	function move(e) {
		if (!ghost) {
			if (Math.hypot(e.clientX - startX, e.clientY - startY) < 6) return;
			const r = source.getBoundingClientRect();
			const rest = [source];
			for (let el = source.nextElementSibling; el; el = el.nextElementSibling) rest.push(el);
			ghost = h("div", { class: "sol-ghost", style: `left:${r.left}px;top:${r.top}px;--cw:${r.width}px;--ch:${(r.width * 92) / 66}px`, "aria-hidden": "true" });
			for (const el of rest) ghost.append(el.cloneNode(true));
			document.body.append(ghost);
			holdCardZoom(true);
		}
		ghost.style.transform = `translate(${e.clientX - startX}px, ${e.clientY - startY}px)`;
	}
	function up(e) {
		if (!ghost) return finish();
		ghost.hidden = true;
		const under = document.elementFromPoint(e.clientX, e.clientY);
		ghost.hidden = false;
		const place = under && under.closest ? under.closest("[data-pile]") : null;
		finish(() => {
			if (!place) return;
			const pile = place.getAttribute("data-pile");
			const named = pile.replace(/^[fc]\d+$/, (m) => m[0]);
			if (ctx.picked().length) ctx.cancelMove();
			ctx.onAction(from);
			const options = ctx.options();
			const to = options.includes(`to-${pile}`) ? `to-${pile}` : options.includes(`to-${named}`) ? `to-${named}` : "";
			if (to) ctx.onAction(to);
			else ctx.cancelMove();
		});
	}
	function cancel() {
		finish();
	}
	window.addEventListener("pointermove", move);
	window.addEventListener("pointerup", up);
	window.addEventListener("pointercancel", cancel);
}

// ---- bowling ------------------------------------------------------------------

// The pins chosen so far, and the position they were chosen in: a new position forgets them.
const chosen = { pins: new Set(), key: "" };

function positionKey(ctx) {
	const b = ctx.view.bowling;
	return `${ctx.room}/${b.frame}/${b.ball}/${(ctx.view.table || []).map((z) => z.count).join(",")}`;
}

/** The pins the tutorial's step wants, or null outside a tutorial. */
function tutorialPins(ctx) {
	const steps = ctx.tutorialGestures();
	return steps ? steps.filter((st) => String(st.value).startsWith("pin-")).map((st) => String(st.value).slice(4)) : null;
}

/** Throws a ball at the chosen pins: the steps go in the order the server lists them. */
function throwBall(ctx, ball) {
	const pins = [...chosen.pins].sort();
	chosen.pins = new Set();
	for (const p of pins) ctx.onAction(`pin-${p}`);
	ctx.onAction(`ball-${ball}`);
}

/**
 * The balls that can knock down exactly the chosen pins. The server lists every play,
 * first pin first, so each step of the way has to be on offer.
 */
function ballsFor(ctx) {
	const moves = ctx.playsList();
	const want = [...chosen.pins].sort().join("+");
	const balls = new Set(moves.filter((m) => m.split("@")[0] === want).map((m) => m.split("@")[1]));
	// In a tutorial only the step's own ball.
	const steps = ctx.tutorialGestures();
	if (!steps) return balls;
	const last = steps.length ? String(steps[steps.length - 1].value) : "";
	return new Set([...balls].filter((b) => last === `ball-${b}`));
}

function bowling(ctx) {
	const { view } = ctx;
	const zones = zonesOf(view);
	const key = positionKey(ctx);
	if (key !== chosen.key) {
		chosen.key = key;
		chosen.pins = new Set();
	}
	const balls = ballsFor(ctx);
	const wanted = tutorialPins(ctx);
	const reachable = new Set(ctx.playsList().flatMap((m) => m.split("@")[0].split("+")));
	const rack = h("div", { class: "sol-rack", id: "sol-rack", role: "group", "aria-label": "The pins" });
	for (const row of pinRows) {
		const r = h("div", { class: "sol-pin-row" });
		for (const letter of row) {
			const zone = zones.get(`pin${letter}`);
			const card = zone && zone.cards && zone.cards[0];
			if (!card) {
				r.append(h("span", { class: "sol-pin down", "data-pin": letter }, h("span", { class: "sol-empty" }, letter)));
				continue;
			}
			const can = reachable.has(letter) && (!wanted || wanted.includes(letter));
			const on = chosen.pins.has(letter);
			const attrs = { class: "sol-pin" + (on ? " sol-picked" : "") + (wanted && wanted.includes(letter) ? " sol-wanted" : ""), "data-pin": letter, "data-card": card.id, title: `Pin ${letter}` };
			if (!can) {
				r.append(h("span", attrs, face(ctx, card.id), h("span", { class: "sol-pin-name" }, letter)));
				continue;
			}
			r.append(
				h(
					"button",
					{
						...attrs,
						type: "button",
						"aria-pressed": on ? "true" : "false",
						onclick: () => {
							if (chosen.pins.has(letter)) chosen.pins.delete(letter);
							else chosen.pins.add(letter);
							ctx.redraw();
						},
					},
					face(ctx, card.id),
					h("span", { class: "sol-pin-name" }, letter)
				)
			);
		}
		rack.append(r);
	}
	const ballsRow = h("div", { class: "sol-balls", id: "sol-balls", role: "group", "aria-label": "The balls" });
	for (let i = 1; i <= 3; i++) {
		const zone = zones.get(`ball${i}`);
		const cards = (zone && zone.cards) || [];
		const top = cards.length ? cards[cards.length - 1] : null;
		const box = h("div", { class: "sol-ball", "data-ball": i });
		const can = balls.has(String(i)) && !!top;
		if (top && top.id) {
			box.append(
				can
					? h("button", { type: "button", class: "sol-ball-card pc-btn playable", "data-throw": i, title: `Throw pile ${i}`, onclick: () => throwBall(ctx, i) }, face(ctx, top.id))
					: face(ctx, top.id)
			);
		} else box.append(h("span", { class: "sol-empty" }));
		box.append(h("span", { class: "pile-count" }, String(zone ? zone.count : 0)));
		ballsRow.append(box);
	}
	const gone = zones.get("gone");
	const out = h("div", { class: "sol-gone zone layout-splay", id: "sol-gone" });
	for (const c of (gone && gone.cards) || []) out.append(h("span", { class: "pc-slot" }, face(ctx, c.id)));
	return h(
		"div",
		{ class: "card-table sol-table", id: "card-table", "data-mode": "bowling" },
		scoreSheet(view.bowling),
		h("div", { class: "sol-lanes" }, rack, h("div", { class: "sol-side" }, h("div", { class: "ct-zone-label small muted" }, "Balls"), ballsRow, h("div", { class: "ct-zone-label small muted" }, "Out of play"), out))
	);
}

/** The score sheet: ten frames, the marks of each ball and the running total. */
function scoreSheet(b) {
	const table = h("div", { class: "sol-sheet", id: "sol-sheet", role: "table", "aria-label": "Score sheet" });
	for (let i = 0; i < 10; i++) {
		const f = b.frames[i] || { marks: [] };
		const current = i === b.frame - 1 && i < b.frames.length;
		const slots = i === 9 ? 3 : 2;
		const marks = h("div", { class: "sol-marks" });
		for (let k = 0; k < slots; k++) marks.append(h("span", { class: "sol-mark" }, f.marks[k] || ""));
		table.append(
			h(
				"div",
				{ class: "sol-frame" + (current ? " current" : ""), "data-frame": i + 1, role: "cell" },
				h("div", { class: "sol-frame-no" }, String(i + 1)),
				marks,
				h("div", { class: "sol-frame-total" }, f.total === undefined || f.total === null ? "" : String(f.total))
			)
		);
	}
	return table;
}

function table(ctx) {
	return ctx.view.bowling ? bowling(ctx) : patience(ctx);
}

/** The table of a game of patience, as cards.js's cardTable draws any other card game: {node, state}. */
export function solitaireTable(ctx) {
	const node = table(ctx);
	enableZoom(node);
	return { node, state: new Map() };
}

/** A game in progress, to look at: the sample view of a game (state.Sampler), and nothing to click. */
export function solitairePreview(info, view) {
	const none = () => [];
	const node = table({ info, view, room: "", options: none, picked: none, playsList: none, tutorialGestures: () => null });
	node.removeAttribute("id");
	node.classList.add("sol-preview");
	node.setAttribute("role", "img");
	node.setAttribute("aria-label", `${info.name}: a game in progress`);
	return node;
}
