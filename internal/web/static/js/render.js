// SVG board renderer. It is deliberately game-agnostic: it draws whatever
// board graph the server describes (grid or explicit), with pieces styled by
// the shape/color hints in the game definition, stacks drawn as piles, and
// overlays for the last move, the selection and legal targets.

import { h, s, clear } from "./util.js";
import { frameDecor, stoneMarks, installThemes, currentTheme } from "./themes.js";
import { onNavigate } from "./nav.js";

export const CELL = 80;

/** Prefix of the piece value of the unit that stands on a Gate (see the server's notation). */
export const ONGATE = "gate:";
const MARGIN = 30;
// A themed board has an illustrated frame, and the coordinates would be drawn on
// that picture: they go in a plain strip round the whole frame instead.
const COORD_STRIP = 26;
const stripFor = (showCoords) => (showCoords && currentTheme() ? COORD_STRIP : 0);
const NODE_MARGIN = 46;

// Hive's tiles: a hexagon bearing the insect.
const BUG_GLYPHS = { queen: "🐝", beetle: "🪲", spider: "🕷", grasshopper: "🦗", ant: "🐜", mosquito: "🦟", ladybug: "🐞", pillbug: "🐛" };
const UNIT_GLYPHS = { pawn: "♟", knight: "♞", bishop: "♝", rook: "♜", queen: "♛", king: "♚" };

// Shapes a game's page script adds (ADR 0124): name -> {body, detail?, dots?},
// paths in a 100 × 100 box centred on the origin (Cthulhu Wars' units).
const extraShapes = new Map();

/** Adds a game's own piece shapes, by name, for pieceShape to draw. */
export function addPieceShapes(shapes) {
	for (const [name, icon] of Object.entries(shapes)) extraShapes.set(name, icon);
}

/** The parts of a shape a game added, `size` units across. */
function unitIconParts(shape, size) {
	const icon = extraShapes.get(shape);
	const g = s("g", { class: "unit-icon", transform: `scale(${(size / 100).toFixed(4)})` });
	// The outline keeps its width however small the icon is drawn.
	for (const d of icon.body) g.append(s("path", { class: "piece-body", d, "vector-effect": "non-scaling-stroke", "stroke-linejoin": "round" }));
	for (const d of icon.detail || []) g.append(s("path", { class: "piece-detail", d, "vector-effect": "non-scaling-stroke" }));
	for (const [x, y, r] of icon.dots || []) g.append(s("circle", { class: "piece-eye", cx: x, cy: y, r }));
	return [g];
}

/** Visual body of one piece, centred on the origin, `size` units across. */
export function pieceShape(shape, size) {
	const S = size;
	if (extraShapes.has(shape)) return unitIconParts(shape, size);
	if (shape.startsWith("bug-")) {
		const pts = [];
		for (let k = 0; k < 6; k++) {
			const a = (Math.PI / 180) * (60 * k - 30);
			pts.push(`${(0.5 * S * Math.cos(a)).toFixed(2)},${(0.5 * S * Math.sin(a)).toFixed(2)}`);
		}
		return [
			s("polygon", { class: `piece-body bug-${shape.slice(4)}`, points: pts.join(" "), "stroke-linejoin": "round" }),
			s("text", { class: "bug-glyph", x: 0, y: 0.03 * S, "text-anchor": "middle", "dominant-baseline": "central", "font-size": 0.56 * S }, BUG_GLYPHS[shape.slice(4)] || "?"),
		];
	}
	switch (shape) {
		case "wall":
			return [
				s("rect", { class: "piece-body", x: -0.17 * S, y: -0.46 * S, width: 0.34 * S, height: 0.92 * S, rx: 0.08 * S }),
				s("path", { class: "piece-detail", d: `M ${-0.17 * S} ${-0.3 * S} H ${0.17 * S}` }),
			];
		case "cap":
			return [
				s("circle", { class: "piece-body", r: 0.44 * S }),
				s("circle", { class: "piece-detail", r: 0.2 * S }),
			];
		case "ring":
			return [
				s("circle", { class: "piece-body", r: 0.44 * S }),
				s("circle", { class: "piece-detail", r: 0.24 * S }),
			];
		case "diamond":
			return [
				s("polygon", {
					class: "piece-body",
					points: `0,${-0.5 * S} ${0.5 * S},0 0,${0.5 * S} ${-0.5 * S},0`,
				}),
			];
		case "triangle":
			return [
				s("polygon", {
					class: "piece-body",
					points: `0,${-0.46 * S} ${0.46 * S},${0.4 * S} ${-0.46 * S},${0.4 * S}`,
				}),
			];
		case "cross": {
			const a = 0.12 * S;
			const b = 0.45 * S;
			return [
				s("path", {
					class: "piece-body",
					d: `M ${-a} ${-b} H ${a} V ${-a} H ${b} V ${a} H ${a} V ${b} H ${-a} V ${a} H ${-b} V ${-a} H ${-a} Z`,
				}),
			];
		}
		case "pawn":
		case "knight":
		case "bishop":
		case "rook":
		case "queen":
		case "king":
			// A disc with the chess glyph on it (Courtfall's units).
			return [
				s("circle", { class: "piece-body", r: 0.46 * S }),
				s(
					"text",
					{ class: "piece-glyph", x: 0, y: 0.03 * S, "text-anchor": "middle", "dominant-baseline": "central", "font-size": 0.64 * S },
					UNIT_GLYPHS[shape]
				),
			];
		case "cultist": {
			// A hooded figure: a robe and a head.
			return [
				s("path", { class: "piece-body", d: `M 0 ${-0.12 * S} L ${0.34 * S} ${0.44 * S} L ${-0.34 * S} ${0.44 * S} Z`, "stroke-linejoin": "round" }),
				s("circle", { class: "piece-body", cx: 0, cy: -0.29 * S, r: 0.18 * S }),
			];
		}
		case "monster": {
			// A shield: a hexagon with a claw mark.
			const pts = [];
			for (let k = 0; k < 6; k++) {
				const a = (Math.PI / 180) * (60 * k - 90);
				pts.push(`${(0.47 * S * Math.cos(a)).toFixed(2)},${(0.47 * S * Math.sin(a)).toFixed(2)}`);
			}
			return [
				s("polygon", { class: "piece-body", points: pts.join(" "), "stroke-linejoin": "round" }),
				s("path", { class: "piece-detail", d: `M ${-0.2 * S} ${0.08 * S} L 0 ${-0.16 * S} L ${0.2 * S} ${0.08 * S}` }),
			];
		}
		case "goo": {
			// A many-pointed star: a Great Old One.
			const pts = [];
			for (let k = 0; k < 16; k++) {
				const r = k % 2 === 0 ? 0.54 * S : 0.31 * S;
				const a = (Math.PI / 8) * k - Math.PI / 2;
				pts.push(`${(r * Math.cos(a)).toFixed(2)},${(r * Math.sin(a)).toFixed(2)}`);
			}
			return [
				s("polygon", { class: "piece-body", points: pts.join(" "), "stroke-linejoin": "round" }),
				s("circle", { class: "piece-detail", r: 0.13 * S }),
			];
		}
		case "donut":
			// A ring with a hole (Yinsh's rings).
			return [s("circle", { class: "piece-body", r: 0.46 * S }), s("circle", { class: "ring-hole", r: 0.27 * S })];
		case "chip":
			// A small flat marker (Yinsh's markers).
			return [s("circle", { class: "piece-body", r: 0.3 * S })];
		case "square":
		case "flat":
			return [
				s("rect", { class: "piece-body", x: -0.41 * S, y: -0.41 * S, width: 0.82 * S, height: 0.82 * S, rx: 0.12 * S }),
			];
		default:
			return [s("circle", { class: "piece-body", r: 0.44 * S })];
	}
}

/** A standalone piece icon (for legends and hand panels). */
export function pieceIcon(shape, color, px = 28) {
	const svg = s("svg", { viewBox: "-20 -20 40 40", width: px, height: px, "aria-hidden": "true" });
	svg.append(s("g", { class: `piece color-${cssColor(color)}` }, pieceShape(shape, 34)));
	return svg;
}

export function cssColor(color) {
	return String(color || "").toLowerCase().replace(/[^a-z0-9]/g, "") || "white";
}

/**
 * Layout of the board in SVG user units.
 * Returns {width, height, place(pos) -> {x, y}, kind, cols, rows, labelFor}.
 */
function layout(info, flipped) {
	const b = info.board;
	if (b.kind === "grid") {
		const W = b.width;
		const H = b.height;
		const margin = b.labels === "algebraic" ? MARGIN : 14;
		const place = (p) => {
			const gx = flipped ? W - 1 - p.x : p.x;
			const gy = flipped ? H - 1 - p.y : p.y;
			return { x: margin + gx * CELL + CELL / 2, y: margin + gy * CELL + CELL / 2, gx, gy };
		};
		return { kind: "grid", width: W * CELL + 2 * margin, height: H * CELL + 2 * margin, W, H, margin, place };
	}
	// Explicit graph: scale the declared coordinates.
	const xs = b.positions.map((p) => p.x);
	const ys = b.positions.map((p) => p.y);
	const minX = Math.min(...xs);
	const minY = Math.min(...ys);
	const maxX = Math.max(...xs);
	const maxY = Math.max(...ys);
	const span = Math.max(maxX - minX, maxY - minY, 1);
	const unit = Math.min(CELL * 1.4, (8 * CELL) / span);
	const place = (p) => {
		let x = p.x - minX;
		let y = p.y - minY;
		if (flipped) {
			x = maxX - minX - x;
			y = maxY - minY - y;
		}
		return { x: NODE_MARGIN + x * unit, y: NODE_MARGIN + y * unit };
	};
	return {
		kind: "graph",
		width: (maxX - minX) * unit + 2 * NODE_MARGIN,
		height: (maxY - minY) * unit + 2 * NODE_MARGIN,
		place,
	};
}

function describeStack(info, stack) {
	if (!stack || stack.length === 0) return "empty";
	const label = (pc) => {
		const def = info.pieces.find((p) => p.type === pc.piece);
		return `${pc.color} ${def ? def.label.toLowerCase() : pc.piece}`;
	};
	if (stack.length === 1) return label(stack[0]);
	return `stack of ${stack.length} (bottom to top): ` + stack.map(label).join(", ");
}

/**
 * Draws the board into host.
 *
 * view: {
 *   cells:      {positionId: [{piece, color}, ...]}  (bottom first)
 *   flipped:    boolean
 *   lastMove:   [positionId, ...]    squares to tint as "last move"
 *   hint:       [positionId, ...]    squares to ring (a bot's hint, a tutorial's highlight)
 *   arrows:     [[from, to], ...]    arrows to draw over the board (a tutorial)
 *   illegal:    boolean              the arrows and rings are what must not be done
 *   selected:   positionId | null
 *   targets:    Set<positionId>      squares that may be clicked next
 *   pathCounts: Map<positionId, n>   pieces already dropped on a square
 *   interactive: boolean
 * }
 * handlers: {onCell(positionId)}
 */
export function renderBoard(host, info, view, handlers = {}) {
	// Keyboard focus survives re-rendering: remember which square had it.
	installThemes();
	const active = document.activeElement;
	const focusCell = view.navigable && active && host.contains(active) && active.closest ? active.closest(".cell") : null;
	const focusedId = focusCell ? focusCell.getAttribute("data-position") : null;
	const svg = drawBoard(host, info, view, handlers);
	if (view.navigable && svg) enableKeyboard(svg, info, view, focusedId);
	// Slide, drop or flip the pieces that changed since the last drawing of
	// the game board (the page rebuilds the host element on every render, so
	// the previous position is kept here, only for callers that opt in).
	if (svg && "animate" in view) {
		const sig = signature(view.cells);
		if (view.animate && lastDrawn.game === info.id && lastDrawn.sig) animateChanges(svg, lastDrawn.sig, sig);
		lastDrawn = { game: info.id, sig };
	}
	return svg;
}

let lastDrawn = { game: null, sig: null };
// Another page: what was drawn, and how far the map was zoomed, is of the page before.
onNavigate(() => {
	lastDrawn = { game: null, sig: null };
	mapView = null;
});

function signature(cells) {
	const out = {};
	for (const [pos, stack] of Object.entries(cells || {})) out[pos] = (stack || []).map((p) => `${p.piece}:${p.color}`);
	return out;
}

function reducedMotion() {
	return !!(window.matchMedia && window.matchMedia("(prefers-reduced-motion: reduce)").matches);
}

/**
 * Animates what changed between two signatures: a piece that arrived on a
 * square slides in from the square that lost an identical piece, a piece
 * with no donor (a placement) pops in, and a piece whose colour changed in
 * place (Reversi flips) turns over.
 */
function animateChanges(svg, before, after) {
	if (reducedMotion() || typeof svg.animate !== "function") return;
	const centre = (pos) => {
		const hit = svg.querySelector(`.cell[data-position="${CSS.escape(pos)}"] .cell-hit`);
		if (!hit || !hit.getBBox) return null;
		const b = hit.getBBox();
		return { x: b.x + b.width / 2, y: b.y + b.height / 2 };
	};
	// Donors: squares that lost pieces, by piece signature.
	const donors = new Map();
	for (const [pos, stack] of Object.entries(before)) {
		const now = after[pos] || [];
		for (let i = now.length; i < stack.length; i++) {
			const key = stack[i];
			if (!donors.has(key)) donors.set(key, []);
			donors.get(key).push(pos);
		}
		// A pile that got shorter from the top also loses what changed above
		// the common prefix.
	}
	let animated = 0;
	for (const el of svg.querySelectorAll(".piece")) {
		if (animated > 24) break;
		const pos = el.getAttribute("data-position");
		const level = Number(el.getAttribute("data-level"));
		const was = (before[pos] || [])[level];
		const now = (after[pos] || [])[level];
		if (!now || was === now) continue;
		// Pieces are drawn around their own origin and placed with
		// transform="translate(x y)"; animating `transform` itself keeps the
		// scaling centred on the piece.
		const m = /translate\(\s*(-?[\d.]+)[ ,]+(-?[\d.]+)\s*\)/.exec(el.getAttribute("transform") || "");
		if (!m) continue;
		const x = Number(m[1]);
		const y = Number(m[2]);
		const at = (dx, dy, extra) => `translate(${x + dx}px, ${y + dy}px)${extra ? " " + extra : ""}`;
		const sameShape = was && was.split(":")[0] === now.split(":")[0];
		let anim = null;
		if (sameShape) {
			// Same piece, new colour: flip over.
			anim = el.animate([{ transform: at(0, 0, "scale(0.1, 1)") }, { transform: at(0, 0, "scale(1, 1)") }], { duration: 260, easing: "ease-out" });
		} else {
			const list = donors.get(now);
			const from = list && list.length ? list.shift() : null;
			const a = from && centre(from);
			const b = centre(pos);
			if (a && b) {
				anim = el.animate([{ transform: at(a.x - b.x, a.y - b.y) }, { transform: at(0, 0) }], { duration: 240, easing: "ease-out" });
			} else {
				anim = el.animate(
					[
						{ transform: at(0, 0, "scale(0.4)"), opacity: 0 },
						{ transform: at(0, 0, "scale(1)"), opacity: 1 },
					],
					{ duration: 200, easing: "ease-out" }
				);
			}
		}
		if (anim) animated++;
	}
}

const ARROWS = { ArrowRight: [1, 0], ArrowLeft: [-1, 0], ArrowDown: [0, 1], ArrowUp: [0, -1] };

/**
 * Roving tab stop plus arrow-key navigation over the squares: Tab enters the
 * board once, the arrow keys move to the nearest square in that direction
 * (as drawn, so flipped boards work), Enter/Space activates a clickable one.
 */
function enableKeyboard(svg, info, view, focusedId) {
	const cells = [...svg.querySelectorAll(".cell")];
	if (!cells.length) return;
	const layout = new Map(info.board.positions.map((p) => [p.id, p]));
	const byId = new Map(cells.map((c) => [c.getAttribute("data-position"), c]));
	let current = (focusedId && byId.get(focusedId)) || cells.find((c) => c.classList.contains("clickable")) || cells[0];
	const setStop = (cell) => {
		for (const c of cells) c.setAttribute("tabindex", c === cell ? "0" : "-1");
	};
	setStop(current);
	for (const c of cells) {
		if (!c.getAttribute("role")) c.setAttribute("role", "img");
	}
	if (focusedId && current.getAttribute("data-position") === focusedId) current.focus({ preventScroll: true });

	svg.addEventListener("focusin", (ev) => {
		const c = ev.target.closest && ev.target.closest(".cell");
		if (c) {
			current = c;
			setStop(c);
		}
	});
	svg.addEventListener("keydown", (ev) => {
		const dir = ARROWS[ev.key];
		if (!dir || ev.altKey || ev.ctrlKey || ev.metaKey) return;
		const from = layout.get(current.getAttribute("data-position"));
		if (!from) return;
		// Screen direction -> layout direction.
		const dx = view.flipped ? -dir[0] : dir[0];
		const dy = view.flipped ? -dir[1] : dir[1];
		let best = null;
		let bestScore = Infinity;
		for (const p of info.board.positions) {
			if (p.id === from.id) continue;
			const vx = p.x - from.x;
			const vy = p.y - from.y;
			const along = vx * dx + vy * dy;
			if (along <= 1e-6) continue;
			const across = Math.abs(vx * dy - vy * dx);
			// Prefer squares straight ahead; sideways drift costs double.
			const score = along + 2 * across;
			if (score < bestScore) {
				bestScore = score;
				best = p;
			}
		}
		ev.preventDefault();
		ev.stopPropagation();
		const target = best && byId.get(best.id);
		if (target) target.focus();
	});
}

function drawBoard(host, info, view, handlers = {}) {
	// An area map is drawn by its game's page script (Cthulhu Wars'); without
	// one its areas are drawn as the graph of their anchors.
	if (info.board.style === "areas" && (info.board.areas || []).length && info.page && info.page.renderAreaMap) return info.page.renderAreaMap(host, info, view, handlers);
	if (info.board.kind === "hex") return renderHexBoard(host, info, view, handlers);
	const b = info.board;
	const L = layout(info, !!view.flipped);
	const pieceDefs = new Map(info.pieces.map((p) => [p.type, p]));

	const labelled = b.labels === "algebraic" && view.showCoords !== false;
	const strip = stripFor(labelled);
	const svg = s("svg", {
		id: "board-svg",
		viewBox: `${-strip} ${-strip} ${L.width + 2 * strip} ${L.height + 2 * strip}`,
		role: "group",
		"aria-label": `${info.name} board`,
		preserveAspectRatio: "xMidYMid meet",
	});

	// Frame / background.
	svg.append(s("rect", { class: "board-frame", x: 0, y: 0, width: L.width, height: L.height, rx: 14 }));
	{
		const decor = frameDecor(L.width, L.height, L.kind === "grid" ? L.margin : NODE_MARGIN);
		if (decor) svg.append(decor);
	}

	if (L.kind === "grid") {
		const m = L.margin;
		const style = b.style || "squares";
		if (style === "points") {
			svg.append(
				s("rect", { class: "cell-bg light", x: m, y: m, width: L.W * CELL, height: L.H * CELL, rx: 4 })
			);
			for (let i = 0; i < L.W; i++) {
				const x = m + i * CELL + CELL / 2;
				svg.append(s("line", { class: "grid-line", x1: x, y1: m + CELL / 2, x2: x, y2: m + L.H * CELL - CELL / 2 }));
			}
			for (let j = 0; j < L.H; j++) {
				const y = m + j * CELL + CELL / 2;
				svg.append(s("line", { class: "grid-line", x1: m + CELL / 2, y1: y, x2: m + L.W * CELL - CELL / 2, y2: y }));
			}
		}
		// Star points (hoshi) on the standard Go sizes.
		if (style === "points") {
			for (const [sx, sy] of starPoints(L.W, L.H)) {
				const p = L.place({ x: sx, y: sy });
				svg.append(s("circle", { class: "hoshi", cx: p.x, cy: p.y, r: 5 }));
			}
		}
		// Coordinate labels.
		if (labelled) {
			// On the frame's margin, or (a themed board) in the strip outside it.
			const cls = strip ? "coord coord-out" : "coord";
			const near = strip ? -strip / 2 : m / 2;
			for (let gx = 0; gx < L.W; gx++) {
				const x = view.flipped ? L.W - 1 - gx : gx;
				const letter = String.fromCharCode(97 + x);
				const cx = m + gx * CELL + CELL / 2;
				svg.append(s("text", { class: cls, x: cx, y: L.height - near }, letter));
				svg.append(s("text", { class: cls, x: cx, y: near }, letter));
			}
			for (let gy = 0; gy < L.H; gy++) {
				const y = view.flipped ? L.H - 1 - gy : gy;
				const rank = L.H - y;
				const cy = m + gy * CELL + CELL / 2;
				svg.append(s("text", { class: cls, x: near, y: cy }, rank));
				svg.append(s("text", { class: cls, x: L.width - near, y: cy }, rank));
			}
		}
	} else {
		// Explicit graph: edges, then nodes.
		const place = new Map(b.positions.map((p) => [p.id, L.place(p)]));
		for (const [a, c] of b.edges || []) {
			const pa = place.get(a);
			const pc = place.get(c);
			if (pa && pc) svg.append(s("line", { class: "grid-line", x1: pa.x, y1: pa.y, x2: pc.x, y2: pc.y }));
		}
		for (const p of b.positions) {
			const pt = place.get(p.id);
			svg.append(s("circle", { class: "cell-bg light", cx: pt.x, cy: pt.y, r: 17, stroke: "var(--board-line)", "stroke-width": 2 }));
		}
	}

	const lastMove = new Set(view.lastMove || []);
	const hinted = new Set(view.hint || []);
	const targets = view.targets || new Set();
	const pathCounts = view.pathCounts || new Map();
	const selectedSet = selectionSet(view.selected);
	const showSquares = L.kind === "grid" && (b.style || "squares") === "squares";
	const pieceSize = L.kind === "grid" ? CELL * 0.78 : 40;
	// Regions tinted by the game ("borders": special squares such as
	// Hnefatafl's corners and throne).
	const tinted = new Map();
	for (const [region, color] of Object.entries(b.borders || {})) {
		for (const id of (b.regions || {})[region] || []) tinted.set(id, color);
	}

	for (const p of b.positions) {
		const pt = L.place(p);
		const stack = (view.cells && view.cells[p.id]) || [];
		const quick = view.interactive && view.quick && view.quick.has(p.id) && !targets.has(p.id);
		const isTarget = targets.has(p.id) || !!quick;
		const clickable = view.interactive && isTarget;
		const g = s("g", {
			class: "cell" + (clickable ? " clickable" : "") + (quick ? " quick" : ""),
			"data-position": p.id,
			"data-height": stack.length,
			role: clickable ? "button" : null,
			tabindex: clickable ? 0 : null,
			"aria-label": `${p.id}: ${describeStack(info, stack)}`,
		});
		g.append(s("title", {}, `${p.id}: ${describeStack(info, stack)}`));

		const half = CELL / 2;
		if (showSquares) {
			const light = (p.x + p.y) % 2 === 0;
			const tint = tinted.get(p.id);
			// A Go ruleset may tint squares too (Fist's Keep and last move).
			const gameTint = (view.tints || {})[p.id];
			const gameTintDef = gameTint && view.legend && view.legend.tints ? view.legend.tints[gameTint] : null;
			g.append(
				s("rect", {
					class: "cell-bg " + (light ? "light" : "dark") + (tint ? ` edge-${cssColor(tint)}` : "") + (gameTint ? ` tint tint-${cssColor(gameTint)}` : ""),
					x: pt.x - half,
					y: pt.y - half,
					width: CELL,
					height: CELL,
					style: gameTintDef && gameTintDef.color ? `fill: color-mix(in srgb, ${gameTintDef.color} 78%, var(--board-light))` : null,
					"data-tint": gameTint || null,
				})
			);
		}

		if (lastMove.has(p.id)) {
			g.append(
				showSquares
					? s("rect", { class: "last-move", x: pt.x - half, y: pt.y - half, width: CELL, height: CELL })
					: s("circle", { class: "last-move", cx: pt.x, cy: pt.y, r: half * 0.9 })
			);
		}
		if (hinted.has(p.id)) {
			g.append(
				showSquares
					? s("rect", { class: "hint-ring", x: pt.x - half + 3, y: pt.y - half + 3, width: CELL - 6, height: CELL - 6, rx: 6 })
					: s("circle", { class: "hint-ring", cx: pt.x, cy: pt.y, r: half * 0.8 })
			);
		}
		if (selectedSet.has(p.id)) {
			g.append(
				showSquares
					? s("rect", { class: "selected-ring", x: pt.x - half, y: pt.y - half, width: CELL, height: CELL })
					: s("circle", { class: "selected-ring", cx: pt.x, cy: pt.y, r: half * 0.9 })
			);
		}

		// Legal-target marker sits behind the pieces so they stay opaque.
		const n = stack.length;
		if (quick && !view.quietTargets) {
			g.append(s("circle", { class: "quick-dot", cx: pt.x, cy: pt.y, r: 6 }));
		} else if (isTarget && view.interactive && !view.quietTargets) {
			g.append(
				showSquares
					? s("rect", {
							class: "target-ring",
							x: pt.x - half + 5,
							y: pt.y - half + 5,
							width: CELL - 10,
							height: CELL - 10,
							rx: 8,
					  })
					: s("circle", { class: "target-ring", cx: pt.x, cy: pt.y, r: half * 0.62 })
			);
			if (n === 0) g.append(s("circle", { class: "target-dot", cx: pt.x, cy: pt.y, r: 9 }));
		}

		// A preview of what a click on this empty square would place.
		if (n === 0 && view.ghost && isTarget && view.interactive) {
			const gp = s("g", {
				class: `ghost-piece color-${cssColor(view.ghost.color)} shape-${view.ghost.shape}`,
				transform: `translate(${pt.x} ${pt.y})`,
			});
			for (const part of pieceShape(view.ghost.shape, pieceSize)) gp.append(part);
			for (const mark of stoneMarks(view.ghost.shape, cssColor(view.ghost.color), pieceSize)) gp.append(mark);
			g.append(gp);
		}

		// The pile.
		if (n > 0) {
			const lift = Math.min(7, 30 / n);
			stack.forEach((pc, i) => {
				const def = pieceDefs.get(pc.piece);
				const y = pt.y + ((n - 1) / 2 - i) * lift;
				const piece = s("g", {
					class: `piece color-${cssColor(pc.color)} shape-${def ? def.shape : "disc"}`,
					transform: `translate(${pt.x} ${y})`,
					"data-piece": pc.piece,
					"data-color": pc.color,
					"data-position": p.id,
					"data-level": i,
					"data-top": i === n - 1 ? "true" : "false",
				});
				for (const part of pieceShape(def ? def.shape : "disc", pieceSize)) piece.append(part);
				for (const mark of stoneMarks(def ? def.shape : "disc", cssColor(pc.color), pieceSize)) piece.append(mark);
				g.append(piece);
			});
			if (n > 1) {
				const bx = pt.x + half * 0.62;
				const by = pt.y - half * 0.62;
				g.append(s("circle", { class: "height-badge", cx: bx, cy: by, r: 11 }));
				g.append(s("text", { class: "height-badge-text", x: bx, y: by }, n));
			}
		}

		// Glyph badges a Go ruleset puts on a square (Fist: Empowered, pawn control).
		((view.badges || {})[p.id] || []).forEach((kind, i) => {
			const bd = view.legend && view.legend.badges ? view.legend.badges[kind] : null;
			if (!bd || !bd.glyph) return;
			g.append(
				s(
					"text",
					{
						class: "square-badge",
						x: pt.x + half * (0.62 - 0.3 * i),
						y: pt.y - half * 0.5,
						"text-anchor": "middle",
						"font-size": CELL * 0.26,
						fill: bd.color || "currentColor",
						"data-badge": kind,
					},
					s("title", {}, bd.label || kind),
					bd.glyph
				)
			);
		});

		if (pathCounts.has(p.id)) {
			const bx = pt.x - half * 0.55;
			const by = pt.y + half * 0.6;
			g.append(s("circle", { class: "path-badge", cx: bx, cy: by, r: 12 }));
			g.append(s("text", { class: "path-badge-text", x: bx, y: by }, "+" + pathCounts.get(p.id)));
		}

		// Hit area on top so clicks land on the cell, not on a piece part.
		g.append(
			s("rect", {
				class: "cell-hit",
				x: pt.x - half,
				y: pt.y - half,
				width: CELL,
				height: CELL,
				fill: "transparent",
			})
		);

		if (clickable && handlers.onCell) {
			g.addEventListener("click", (ev) => handlers.onCell(p.id, ev));
			g.addEventListener("keydown", (ev) => {
				if (ev.key === "Enter" || ev.key === " ") {
					ev.preventDefault();
					ev.stopPropagation();
					handlers.onCell(p.id);
				}
			});
		}
		attachHover(g, p.id, handlers);

		svg.append(g);
	}

	// Fences a Go ruleset draws on the edges between squares (Fendo).
	if (showSquares && view.walls && view.walls.length) {
		const at = new Map(b.positions.map((p) => [p.id, L.place(p)]));
		const fences = s("g", { class: "fences", "pointer-events": "none" });
		for (const [a, c] of view.walls) {
			const pa = at.get(a);
			const pc = at.get(c);
			if (!pa || !pc) continue;
			const mx = (pa.x + pc.x) / 2;
			const my = (pa.y + pc.y) / 2;
			const half = CELL / 2;
			const vertical = Math.abs(pa.x - pc.x) > Math.abs(pa.y - pc.y);
			fences.append(
				s("line", {
					class: "fence",
					x1: vertical ? mx : mx - half,
					y1: vertical ? my - half : my,
					x2: vertical ? mx : mx + half,
					y2: vertical ? my + half : my,
					"data-fence": `${a}|${c}`,
				})
			);
		}
		svg.append(fences);
	}

	const byId = new Map(b.positions.map((p) => [p.id, p]));
	drawArrows(svg, view, (id) => (byId.has(id) ? L.place(byId.get(id)) : null));
	clear(host).append(svg);
	return svg;
}

let arrowCount = 0;

/**
 * Draws a tutorial's arrows over a board, from one square to another, and marks
 * them and its rings with crosses when they show what must not be done. centerOf
 * gives a square's centre in the board's coordinates, or null for a square not
 * drawn.
 */
export function drawArrows(parent, view, centerOf) {
	const arrows = view.arrows || [];
	if (arrows.length) {
		const id = `board-arrowhead-${++arrowCount}`;
		parent.append(
			s("defs", {}, s("marker", { id, viewBox: "0 0 10 10", refX: 7, refY: 5, markerWidth: 4, markerHeight: 4, orient: "auto-start-reverse" }, s("path", { d: "M0,0 L10,5 L0,10 z", class: "board-arrowhead" })))
		);
		const g = s("g", { class: "board-arrows", "aria-hidden": "true" });
		for (const [from, to] of arrows) {
			const a = centerOf(from);
			const c = centerOf(to);
			if (!a || !c) continue;
			const dx = c.x - a.x;
			const dy = c.y - a.y;
			const len = Math.hypot(dx, dy) || 1;
			// Start and stop short of the centres, clear of the pieces' middles.
			const trim = Math.min(14, len / 4);
			const x1 = a.x + (dx / len) * trim;
			const y1 = a.y + (dy / len) * trim;
			const x2 = c.x - (dx / len) * trim;
			const y2 = c.y - (dy / len) * trim;
			g.append(s("line", { class: "board-arrow", x1, y1, x2, y2, "marker-end": `url(#${id})` }));
		}
		parent.append(g);
	}
	if (!view.illegal) return;
	parent.classList.add("illegal-marks");
	const crosses = s("g", { class: "illegal-crosses", "aria-hidden": "true" });
	const cross = (at) => {
		if (!at) return;
		const r = 11;
		crosses.append(
			s("line", { class: "illegal-cross", x1: at.x - r, y1: at.y - r, x2: at.x + r, y2: at.y + r }),
			s("line", { class: "illegal-cross", x1: at.x - r, y1: at.y + r, x2: at.x + r, y2: at.y - r })
		);
	};
	for (const id of view.hint || []) cross(centerOf(id));
	for (const [from, to] of arrows) {
		const a = centerOf(from);
		const c = centerOf(to);
		if (a && c) cross({ x: (a.x + c.x) / 2, y: (a.y + c.y) / 2 });
	}
	parent.append(crosses);
}

// selectionSet accepts a single position, an array of positions, or null.
export function selectionSet(sel) {
	if (!sel) return new Set();
	return new Set(Array.isArray(sel) ? sel : [sel]);
}

// starPoints lists the traditional hoshi of square 9x9, 13x13 and 19x19
// boards as 0-based [x, y] pairs.
function starPoints(w, h) {
	if (w !== h) return [];
	let pts;
	switch (w) {
		case 9:
			pts = [2, 4, 6];
			break;
		case 13:
			pts = [3, 6, 9];
			break;
		case 19:
			pts = [3, 9, 15];
			break;
		default:
			return [];
	}
	const out = [];
	for (const x of pts) for (const y of pts) out.push([x, y]);
	if (w === 9) return out.filter(([x, y]) => (x !== 4 && y !== 4) || (x === 4 && y === 4));
	return out;
}

export function attachHover(g, id, handlers) {
	if (!handlers.onHover) return;
	g.addEventListener("mouseenter", () => handlers.onHover(id));
	g.addEventListener("mouseleave", () => handlers.onHover(null));
	g.addEventListener("focusin", () => handlers.onHover(id));
	g.addEventListener("focusout", () => handlers.onHover(null));
}

// ---------------------------------------------------------------------------
// Hexagonal boards (pointy-top cells; rows shifted half a cell)
// ---------------------------------------------------------------------------

const HEX_R = CELL * 0.5773503; // centre-to-corner so a cell is CELL wide
const HEX_ROW = 0.8660254; // rows apart, in cells (the server lays out positions the same way)

function hexPoints(cx, cy, r) {
	const pts = [];
	for (let k = 0; k < 6; k++) {
		const a = (Math.PI / 180) * (60 * k - 30);
		pts.push(`${(cx + r * Math.cos(a)).toFixed(2)},${(cy + r * Math.sin(a)).toFixed(2)}`);
	}
	return pts.join(" ");
}

/** Whether a board can be drawn either way: a hex board that is a board at all (Hive's table is not). */
export function hasHexStyles(info) {
	return info.board.kind === "hex" && info.board.style !== "free";
}

/** How a hex board is drawn until the player says otherwise: the definition's `style: points` asks for the grid. */
export function defaultHexStyle(info) {
	return info.board.style === "points" ? "grid" : "cells";
}

// The lines of the "hex grid" style, as one path: every pair of neighbouring
// points once. Positions come in units of a cell, so a neighbour is half a cell
// or a whole cell across and zero or one row down.
function hexLattice(positions, place) {
	const key = (col, row) => `${col},${row}`;
	const colOf = (p) => Math.round(p.x * 2);
	const rowOf = (p) => Math.round(p.y / HEX_ROW);
	const at = new Map(positions.map((p) => [key(colOf(p), rowOf(p)), p]));
	const d = [];
	for (const p of positions) {
		const a = place(p);
		for (const [dc, dr] of [[2, 0], [1, 1], [-1, 1]]) {
			const q = at.get(key(colOf(p) + dc, rowOf(p) + dr));
			if (!q) continue;
			const c = place(q);
			d.push(`M${a.x.toFixed(2)} ${a.y.toFixed(2)}L${c.x.toFixed(2)} ${c.y.toFixed(2)}`);
		}
	}
	return d.join("");
}

function renderHexBoard(host, info, view, handlers) {
	const b = info.board;
	const M = 34;
	// The "hex grid" style: the spaces are the points of a triangular lattice, as on a Go board.
	const grid = hasHexStyles(info) && (view.hexStyle || defaultHexStyle(info)) === "grid";
	// A "free" board (Hive) has no board to draw: only the tiles on the table
	// and the cells a tile may go to appear, and the picture is fitted to them.
	const free = b.style === "free";
	const stackAt = (id) => (view.cells && view.cells[id]) || [];
	let shown = b.positions;
	if (free) {
		const keep = new Set([...(view.targets || []), ...(view.hint || []), ...selectionSet(view.selected), ...(view.lastMove || []), ...Object.keys(view.tints || {})]);
		for (const p of b.positions) if (stackAt(p.id).length) keep.add(p.id);
		shown = b.positions.filter((p) => keep.has(p.id));
		if (!shown.length) {
			// An empty table: the middle of the field.
			const mx = (Math.min(...b.positions.map((p) => p.x)) + Math.max(...b.positions.map((p) => p.x))) / 2;
			const my = (Math.min(...b.positions.map((p) => p.y)) + Math.max(...b.positions.map((p) => p.y))) / 2;
			shown = [b.positions.reduce((best, p) => (Math.hypot(p.x - mx, p.y - my) < Math.hypot(best.x - mx, best.y - my) ? p : best))];
		}
	}
	const xs = shown.map((p) => p.x);
	const ys = shown.map((p) => p.y);
	const minX = Math.min(...xs);
	const maxX = Math.max(...xs);
	const minY = Math.min(...ys);
	const maxY = Math.max(...ys);
	const contentW = (maxX - minX) * CELL + CELL;
	const contentH = (maxY - minY) * CELL + 2 * HEX_R;
	const innerW = free ? Math.max(contentW + 2 * CELL, 9 * CELL) : contentW;
	const innerH = free ? Math.max(contentH + 2 * CELL, innerW * 0.68) : contentH;
	const width = innerW + 2 * M;
	const height = innerH + 2 * M;
	const ox = (innerW - contentW) / 2;
	const oy = (innerH - contentH) / 2;
	const flipped = !!view.flipped;
	const place = (p) => {
		let x = p.x - minX;
		let y = p.y - minY;
		if (flipped) {
			x = maxX - minX - x;
			y = maxY - minY - y;
		}
		return { x: M + ox + x * CELL + CELL / 2, y: M + oy + y * CELL + HEX_R };
	};

	// Which border region (if any) colours each cell.
	const edgeOf = new Map();
	for (const [region, color] of Object.entries(b.borders || {})) {
		for (const id of (b.regions || {})[region] || []) edgeOf.set(id, color);
	}

	const strip = free ? 0 : stripFor(view.showCoords !== false);
	const svg = s("svg", {
		id: "board-svg",
		viewBox: `${-strip} ${-strip} ${width + 2 * strip} ${height + 2 * strip}`,
		role: "group",
		"aria-label": `${info.name} board`,
		preserveAspectRatio: "xMidYMid meet",
		class: free ? "free-board" : grid ? "hex-grid-board" : null,
	});
	if (!free) {
		svg.append(s("rect", { class: "board-frame", x: 0, y: 0, width, height, rx: 14 }));
		const decor = frameDecor(width, height, M);
		if (decor) svg.append(decor);
	}
	if (grid) {
		// The same lines twice: a wide round stroke is the board, a thin one the lattice on it.
		const d = hexLattice(b.positions, place);
		svg.append(s("path", { class: "hex-backing", d, fill: "none", "stroke-width": CELL * 0.86, "stroke-linecap": "round" }), s("path", { class: "grid-line hex-lattice", d, fill: "none" }));
	}

	const pieceDefs = new Map(info.pieces.map((p) => [p.type, p]));
	const lastMove = new Set(view.lastMove || []);
	const hinted = new Set(view.hint || []);
	const targets = view.targets || new Set();
	const pieceSize = free ? CELL * 1.04 : CELL * 0.7;
	// Overlays of games whose rules are written in Go: terrain tints and
	// per-unit badges, with the game's legend saying how to draw them.
	const tints = view.tints || {};
	const badges = view.badges || {};
	const legend = view.legend || {};

	for (const p of shown) {
		const pt = place(p);
		const stack = (view.cells && view.cells[p.id]) || [];
		const quick = view.interactive && view.quick && view.quick.has(p.id) && !targets.has(p.id);
		const isTarget = targets.has(p.id) || !!quick;
		const clickable = view.interactive && isTarget;
		const g = s("g", {
			class: "cell" + (clickable ? " clickable" : "") + (quick ? " quick" : ""),
			"data-position": p.id,
			"data-height": stack.length,
			role: clickable ? "button" : null,
			tabindex: clickable ? 0 : null,
			"aria-label": `${p.id}: ${describeStack(info, stack)}`,
		});
		g.append(s("title", {}, free ? describeStack(info, stack) : `${p.id}: ${describeStack(info, stack)}`));

		const shade = ["light", "mid", "dark"][(((p.x - p.y / 2) % 3) + 3) % 3 | 0];
		const edge = edgeOf.get(p.id);
		const tint = tints[p.id];
		const tintDef = tint && legend.tints ? legend.tints[tint] : null;
		// A mark that fills a space: the hexagon of a cell, or the disc round a point.
		const mark = (cls, r) => (grid ? s("circle", { class: cls, cx: pt.x, cy: pt.y, r: r * 0.85 }) : s("polygon", { class: cls, points: hexPoints(pt.x, pt.y, r) }));
		const spaceAttrs = {
			class: `cell-bg ${free ? "free-slot" : grid ? "light" : shade}` + (edge ? ` edge-${cssColor(edge)}` : "") + (tint ? ` tint tint-${cssColor(tint)}` : ""),
			stroke: "var(--board-line)",
			"stroke-width": 1.5,
			style: tintDef && tintDef.color ? `fill: color-mix(in srgb, ${tintDef.color} 78%, var(--board-light))` : null,
			"data-tint": tint || null,
		};
		if (grid) {
			// A plain point is a dot on the lattice; only a tint or a player's edge needs a disc.
			if (edge || tint) g.append(s("circle", { ...spaceAttrs, cx: pt.x, cy: pt.y, r: tint ? HEX_R * 0.72 : HEX_R * 0.4 }));
			g.append(s("circle", { class: "grid-point", cx: pt.x, cy: pt.y, r: CELL * 0.09 }));
		} else if (!free || !stack.length) {
			g.append(s("polygon", { ...spaceAttrs, points: hexPoints(pt.x, pt.y, HEX_R) }));
		}
		if (tintDef && tintDef.glyph) {
			g.append(s("text", { class: "tint-glyph", x: pt.x, y: pt.y + HEX_R * 0.62, "text-anchor": "middle" }, tintDef.glyph));
		}
		if (lastMove.has(p.id)) g.append(mark("last-move", HEX_R));
		if (hinted.has(p.id)) g.append(mark("hint-ring", HEX_R * 0.85));
		if (selectionSet(view.selected).has(p.id)) g.append(mark("selected-ring", HEX_R));
		if (quick && !view.quietTargets) {
			g.append(s("circle", { class: "quick-dot", cx: pt.x, cy: pt.y, r: 6 }));
		} else if (isTarget && view.interactive && !view.quietTargets) {
			g.append(mark("target-ring", HEX_R * 0.78));
			if (stack.length === 0) g.append(s("circle", { class: "target-dot", cx: pt.x, cy: pt.y, r: 7 }));
		}

		const n = stack.length;
		if (n === 0 && view.ghost && isTarget && view.interactive) {
			const gp = s("g", {
				class: `ghost-piece color-${cssColor(view.ghost.color)} shape-${view.ghost.shape}`,
				transform: `translate(${pt.x} ${pt.y})`,
			});
			for (const part of pieceShape(view.ghost.shape, pieceSize)) gp.append(part);
			for (const mark of stoneMarks(view.ghost.shape, cssColor(view.ghost.color), pieceSize)) gp.append(mark);
			g.append(gp);
		}
		stack.forEach((pc, i) => {
			const def = pieceDefs.get(pc.piece);
			const piece = s("g", {
				class: `piece color-${cssColor(pc.color)} shape-${def ? def.shape : "disc"}`,
				// A tile on top of others sits higher, the ones it covers peeking out below.
				transform: free ? `translate(${pt.x} ${pt.y + (n - 1 - i) * 7})` : `translate(${pt.x} ${pt.y - ((n - 1) / 2 - i) * 4})`,
				"data-piece": pc.piece,
				"data-color": pc.color,
				"data-position": p.id,
				"data-level": i,
				"data-top": i === n - 1 ? "true" : "false",
			});
			for (const part of pieceShape(def ? def.shape : "disc", pieceSize)) piece.append(part);
			for (const mark of stoneMarks(def ? def.shape : "disc", cssColor(pc.color), pieceSize)) piece.append(mark);
			g.append(piece);
		});
		if (free && n > 1) {
			g.append(
				s("circle", { class: "height-badge", cx: pt.x + CELL * 0.36, cy: pt.y - CELL * 0.34, r: 10 }),
				s("text", { class: "height-badge-text", x: pt.x + CELL * 0.36, y: pt.y - CELL * 0.34 }, String(n))
			);
		}
		for (const kind of badges[p.id] || []) {
			const bd = legend.badges && legend.badges[kind] ? legend.badges[kind] : {};
			if (kind === "active") {
				g.append(s("circle", { class: "badge-active", cx: pt.x, cy: pt.y, r: pieceSize * 0.62, "data-badge": kind }));
			} else if (kind === "target") {
				g.append(s("circle", { class: "badge-target", cx: pt.x, cy: pt.y, r: pieceSize * 0.62, "data-badge": kind }));
			} else {
				// A stone beside the unit: white = Fatigued, black = Exhausted.
				g.append(
					s("circle", {
						class: `badge-stone stone-${cssColor(kind)}`,
						cx: pt.x + CELL * 0.27,
						cy: pt.y + CELL * 0.2,
						r: CELL * 0.1,
						"data-badge": kind,
					}, s("title", {}, bd.label || kind))
				);
			}
		}

		g.append(s("polygon", { class: "cell-hit", points: hexPoints(pt.x, pt.y, HEX_R), fill: "transparent" }));
		if (clickable && handlers.onCell) {
			g.addEventListener("click", (ev) => handlers.onCell(p.id, ev));
			g.addEventListener("keydown", (ev) => {
				if (ev.key === "Enter" || ev.key === " ") {
					ev.preventDefault();
					ev.stopPropagation();
					handlers.onCell(p.id);
				}
			});
		}
		attachHover(g, p.id, handlers);
		svg.append(g);
	}

	// Column letters sit just beyond the top-most cell of each column, along
	// the column's direction; row numbers sit past the left-most cell of each row (the
	// right-most for hexagon-shaped boards, whose upper-left side already
	// carries the letters). The labels hug the outline of any hex layout.
	if (view.showCoords !== false && !free) {
		const hexagonal = b.width > 0 && b.positions.length < b.width * b.height;
		const topOfColumn = new Map();
		const endOfRow = new Map();
		for (const p of b.positions) {
			const pt = place(p);
			const letter = p.id.replace(/\d+$/, "");
			const row = p.id.slice(letter.length);
			if (!topOfColumn.has(letter) || pt.y < topOfColumn.get(letter).y) topOfColumn.set(letter, pt);
			const better = hexagonal ? pt.x > (endOfRow.get(row) || { x: -Infinity }).x : pt.x < (endOfRow.get(row) || { x: Infinity }).x;
			if (!endOfRow.has(row) || better) endOfRow.set(row, pt);
		}
		for (const [letter, pt] of topOfColumn) {
			if (strip) svg.append(s("text", { class: "coord coord-out", x: pt.x - CELL * 0.36, y: -strip / 2 }, letter));
			else svg.append(s("text", { class: "coord", x: pt.x - CELL * 0.36, y: pt.y - CELL * 0.62 }, letter));
		}
		for (const [row, pt] of endOfRow) {
			// Outside the frame the numbers stand level with their row, at the side where they sit inside.
			if (strip) svg.append(s("text", { class: "coord coord-out", x: hexagonal ? width + strip / 2 : -strip / 2, y: pt.y }, row));
			else svg.append(s("text", { class: "coord", x: pt.x + (hexagonal ? CELL : -CELL) * 0.72, y: pt.y }, row));
		}
	}

	const byId = new Map(b.positions.map((p) => [p.id, p]));
	drawArrows(svg, view, (id) => (byId.has(id) ? place(byId.get(id)) : null));
	clear(host).append(svg);
	return svg;
}


/**
 * A small drawing of a mark, for the key of the rules window: "swatch" (arg: a
 * CSS colour) is every game's; a game's page script draws its own kinds
 * (info.page.legendMarker, Cthulhu Wars' Gates and glyphs).
 */
export function legendMarker(kind, arg) {
	const svg = s("svg", { viewBox: "-16 -16 32 32", width: 30, height: 30, class: "legend-marker", "aria-hidden": "true" });
	if (kind === "swatch") svg.append(s("rect", { x: -11, y: -11, width: 22, height: 22, rx: 5, fill: arg, stroke: "#6b6454", "stroke-width": 1.5 }));
	return svg;
}

let mapView = null;

/** Mouse-wheel / pinch zoom, and drag to pan. */
export function setupZoom(host, svg, full, key) {
	const v = mapView && mapView.key === key ? { ...mapView.v } : { ...full };
	const clamp = (x, lo, hi) => Math.min(Math.max(x, lo), Math.max(lo, hi));
	const apply = () => {
		v.w = clamp(v.w, full.w / 8, full.w);
		v.h = (v.w * full.h) / full.w;
		v.x = clamp(v.x, full.x, full.x + full.w - v.w);
		v.y = clamp(v.y, full.y, full.y + full.h - v.h);
		svg.setAttribute("viewBox", `${v.x} ${v.y} ${v.w} ${v.h}`);
		mapView = { key, v: { ...v } };
	};
	const zoomAt = (cx, cy, factor) => {
		const r = svg.getBoundingClientRect();
		const fx = (cx - r.left) / r.width;
		const fy = (cy - r.top) / r.height;
		const px = v.x + fx * v.w;
		const py = v.y + fy * v.h;
		const nw = clamp(v.w * factor, full.w / 8, full.w);
		const nh = (nw * full.h) / full.w;
		v.x = px - fx * nw;
		v.y = py - fy * nh;
		v.w = nw;
		apply();
	};
	apply();

	svg.addEventListener(
		"wheel",
		(ev) => {
			ev.preventDefault();
			zoomAt(ev.clientX, ev.clientY, ev.deltaY < 0 ? 0.85 : 1 / 0.85);
		},
		{ passive: false }
	);

	const pointers = new Map();
	let pinch = 0;
	let dragged = false;
	let startPt = null;
	svg.addEventListener("pointerdown", (ev) => {
		pointers.set(ev.pointerId, { x: ev.clientX, y: ev.clientY });
		startPt = { x: ev.clientX, y: ev.clientY };
		dragged = false;
		pinch = pointers.size === 2 ? dist() : 0;
	});
	const dist = () => {
		const [a, b] = [...pointers.values()];
		return Math.hypot(a.x - b.x, a.y - b.y);
	};
	svg.addEventListener("pointermove", (ev) => {
		const prev = pointers.get(ev.pointerId);
		if (!prev) return;
		const cur = { x: ev.clientX, y: ev.clientY };
		if (pointers.size === 2) {
			pointers.set(ev.pointerId, cur);
			const d = dist();
			if (pinch > 0 && d > 0) {
				const [a, b] = [...pointers.values()];
				zoomAt((a.x + b.x) / 2, (a.y + b.y) / 2, pinch / d);
			}
			pinch = d;
			dragged = true;
			return;
		}
		if (!dragged && Math.hypot(cur.x - startPt.x, cur.y - startPt.y) < 5) return;
		if (!dragged) {
			dragged = true;
			svg.classList.add("panning");
			try {
				svg.setPointerCapture(ev.pointerId);
			} catch (_) {
				/* not capturable: dragging still works while over the map */
			}
		}
		const r = svg.getBoundingClientRect();
		v.x -= ((cur.x - prev.x) / r.width) * v.w;
		v.y -= ((cur.y - prev.y) / r.height) * v.h;
		pointers.set(ev.pointerId, cur);
		apply();
	});
	const up = (ev) => {
		pointers.delete(ev.pointerId);
		pinch = 0;
		svg.classList.remove("panning");
		if (dragged) setTimeout(() => (dragged = false), 0);
	};
	svg.addEventListener("pointerup", up);
	svg.addEventListener("pointercancel", up);
	// A drag must not also click whatever it ended on.
	svg.addEventListener(
		"click",
		(ev) => {
			if (dragged) {
				ev.stopPropagation();
				ev.preventDefault();
			}
		},
		true
	);
}
