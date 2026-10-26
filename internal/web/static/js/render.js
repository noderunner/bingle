// SVG board renderer. It is deliberately game-agnostic: it draws whatever
// board graph the server describes (grid or explicit), with pieces styled by
// the shape/color hints in the game definition, stacks drawn as piles, and
// overlays for the last move, the selection and legal targets.

import { h, s, clear } from "./util.js";
import { UNIT_ICONS, CLASS_SCALE, classOfPiece, hasUnitIcon } from "./icons.js";
import { CW_GLYPHS } from "./cw-glyphs.js";
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

/** The parts of a unit icon (see icons.js), `size` units across. */
function unitIconParts(shape, size) {
	const icon = UNIT_ICONS[shape];
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
	if (hasUnitIcon(shape)) return unitIconParts(shape, size);
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
	if (info.board.style === "areas" && (info.board.areas || []).length) return renderAreaMap(host, info, view, handlers);
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

	clear(host).append(svg);
	return svg;
}

// selectionSet accepts a single position, an array of positions, or null.
function selectionSet(sel) {
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

function attachHover(g, id, handlers) {
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

function hexPoints(cx, cy, r) {
	const pts = [];
	for (let k = 0; k < 6; k++) {
		const a = (Math.PI / 180) * (60 * k - 30);
		pts.push(`${(cx + r * Math.cos(a)).toFixed(2)},${(cy + r * Math.sin(a)).toFixed(2)}`);
	}
	return pts.join(" ");
}

function renderHexBoard(host, info, view, handlers) {
	const b = info.board;
	const M = 34;
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
		class: free ? "free-board" : null,
	});
	if (!free) {
		svg.append(s("rect", { class: "board-frame", x: 0, y: 0, width, height, rx: 14 }));
		const decor = frameDecor(width, height, M);
		if (decor) svg.append(decor);
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
		if (!free || !stack.length) g.append(
			s("polygon", {
				class: `cell-bg ${free ? "free-slot" : shade}` + (edge ? ` edge-${cssColor(edge)}` : "") + (tint ? ` tint tint-${cssColor(tint)}` : ""),
				points: hexPoints(pt.x, pt.y, HEX_R),
				stroke: "var(--board-line)",
				"stroke-width": 1.5,
				style: tintDef && tintDef.color ? `fill: color-mix(in srgb, ${tintDef.color} 78%, var(--board-light))` : null,
				"data-tint": tint || null,
			})
		);
		if (tintDef && tintDef.glyph) {
			g.append(s("text", { class: "tint-glyph", x: pt.x, y: pt.y + HEX_R * 0.62, "text-anchor": "middle" }, tintDef.glyph));
		}
		if (lastMove.has(p.id)) g.append(s("polygon", { class: "last-move", points: hexPoints(pt.x, pt.y, HEX_R) }));
		if (hinted.has(p.id)) g.append(s("polygon", { class: "hint-ring", points: hexPoints(pt.x, pt.y, HEX_R * 0.85) }));
		if (selectionSet(view.selected).has(p.id)) g.append(s("polygon", { class: "selected-ring", points: hexPoints(pt.x, pt.y, HEX_R) }));
		if (quick && !view.quietTargets) {
			g.append(s("circle", { class: "quick-dot", cx: pt.x, cy: pt.y, r: 6 }));
		} else if (isTarget && view.interactive && !view.quietTargets) {
			g.append(s("polygon", { class: "target-ring", points: hexPoints(pt.x, pt.y, HEX_R * 0.78) }));
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

	clear(host).append(svg);
	return svg;
}


// ---------------------------------------------------------------------------
// Area maps (Cthulhu Wars): straight-edged areas on a canvas, with tokens
// grouped per faction and unit kind.
// ---------------------------------------------------------------------------

const CHIP_R = 18;
const CHIP_GAP = 3;
const CLASS_ORDER = { goo: 0, terror: 1, monster: 2, cultist: 3 };

const GLYPHS = {
	// Small symbols of the three glyph requirements, in a 24x24 box.
	dragon: "M -9 4 Q -6 -9 -1 -1 T 8 -3 M 8 -3 l 3 -2 M 6 -6 l 0 3",
	chevron: "M -9 5 L 0 -6 L 9 5 M -9 -1 L 0 -12 L 9 -1",
	thorns: "M -10 8 L -7 -6 L -4 8 M -4 8 L -1 -9 L 2 8 M 2 8 L 5 -5 L 8 8",
};

// The glyphs (sigils) of the factions, as the printed map shows them: where a
// cult begins, and — for the Windwalker and the Sleeper — the areas marked with
// their sigil. The drawings come from cw-glyphs.js (traced from the wiki's
// icons); a cult's table colour tints the ring round its badge.
const FACTION_COLORS = { gc: "green", cc: "blue", bg: "red", ys: "yellow", ww: "white", sl: "orange", ow: "purple", tt: "brown" };

/** A faction's glyph drawn at the origin, `size` units across. */
export function glyphShape(key, size, cls = "area-sigil") {
	const g = CW_GLYPHS[key];
	if (!g) return null;
	const k = size / 100;
	return s("path", {
		class: cls,
		d: g.d,
		fill: g.fill,
		stroke: g.edge,
		"stroke-width": 0.9 / k > 2.2 ? 2.2 : 0.9 / k,
		"fill-rule": "evenodd",
		"stroke-linejoin": "round",
		transform: `translate(${-size / 2} ${-size / 2}) scale(${k})`,
	});
}

/** The areas that carry a glyph on the map: [{ key, area }] for the cults at the table. */
function glyphAreas(info) {
	const colors = new Set((info.seats || []).map((st) => st.color));
	const out = [];
	const starts = (info.params && info.params.starts) || {};
	const sigils = (info.params && info.params.sigils) || {};
	for (const [key, color] of Object.entries(FACTION_COLORS)) {
		if (!colors.has(color)) continue;
		const areas = [];
		if (typeof starts[key] === "string") areas.push(starts[key]);
		for (const a of sigils[key] || []) if (!areas.includes(a)) areas.push(a);
		for (const area of areas) out.push({ key, area });
	}
	return out;
}

/**
 * A small drawing of a mark of the map, for the key of the rules window: kind
 * is "gate" (arg: a cult colour, or "abandoned"), "glyph" (arg: dragon,
 * chevron, thorns), "sigil" (arg: ww, sl), "desecrated", "iceage" or "swatch"
 * (arg: a CSS colour). `shape` draws the unit occupying a Gate.
 */
export function legendMarker(kind, arg, info, shape = "") {
	const px = 30;
	const svg = s("svg", { viewBox: "-16 -16 32 32", width: px, height: px, class: "legend-marker", "aria-hidden": "true" });
	switch (kind) {
		case "gate": {
			const abandoned = arg === "abandoned";
			const g = s("g", { class: `gate-chip color-${abandoned ? "grey" : cssColor(arg)}` + (abandoned ? " abandoned" : "") });
			g.append(s("circle", { class: "gate-ring", r: 13.5 }), s("circle", { class: "gate-core", r: 8.5 }));
			if (shape && !abandoned) {
				const p = s("g", { class: `piece gate-holder color-${cssColor(arg)}` });
				for (const part of pieceShape(shape, 17)) p.append(part);
				g.append(p);
			} else {
				g.append(s("path", { class: "gate-star", d: "M0 -6 L1.8 -1.8 L6 0 L1.8 1.8 L0 6 L-1.8 1.8 L-6 0 L-1.8 -1.8 Z" }));
			}
			svg.append(g);
			break;
		}
		case "glyph":
			svg.append(
				s("g", { class: "area-glyph-badge" }, s("circle", { r: 12 }), s("path", { class: "area-glyph", d: GLYPHS[arg] || "", transform: "scale(0.85)" }))
			);
			break;
		case "sigil": {
			const shape = glyphShape(arg, 23);
			if (!shape) break;
			svg.append(s("g", { class: `area-sigil-badge color-${FACTION_COLORS[arg] || "grey"}` }, s("circle", { class: "sigil-disc", r: 12.5 }), shape));
			break;
		}
		case "desecrated":
		case "iceage":
			svg.append(
				s(
					"g",
					{ class: kind === "iceage" ? "iceage-chip" : "desecration-chip" },
					s("rect", { x: -9.5, y: -9.5, width: 19, height: 19, rx: 4, transform: "rotate(45)" }),
					s("text", { x: 0, y: 1, "text-anchor": "middle", "dominant-baseline": "central", "font-size": 14 }, kind === "iceage" ? "❄" : "✠")
				)
			);
			break;
		case "swatch":
			svg.append(s("rect", { x: -11, y: -11, width: 22, height: 22, rx: 5, fill: arg, stroke: "#6b6454", "stroke-width": 1.5 }));
			break;
		default:
			break;
	}
	return svg;
}

function polygonBox(poly) {
	let minX = Infinity;
	let minY = Infinity;
	let maxX = -Infinity;
	let maxY = -Infinity;
	for (const [x, y] of poly) {
		minX = Math.min(minX, x);
		minY = Math.min(minY, y);
		maxX = Math.max(maxX, x);
		maxY = Math.max(maxY, y);
	}
	return { minX, minY, maxX, maxY, w: maxX - minX, h: maxY - minY };
}

function inPolygon(pt, poly) {
	let inside = false;
	for (let i = 0, j = poly.length - 1; i < poly.length; j = i++) {
		const [xi, yi] = poly[i];
		const [xj, yj] = poly[j];
		if (yi > pt[1] !== yj > pt[1] && pt[0] < ((xj - xi) * (pt[1] - yi)) / (yj - yi) + xi) inside = !inside;
	}
	return inside;
}

/** The polygon of an area that holds its anchor (else its largest). */
function homePolygon(area) {
	const at = area.anchor;
	let best = area.polygons[0];
	let bestSize = -1;
	for (const poly of area.polygons) {
		if (inPolygon(at, poly)) return poly;
		const b = polygonBox(poly);
		if (b.w * b.h > bestSize) {
			best = poly;
			bestSize = b.w * b.h;
		}
	}
	return best;
}

function describeArea(info, area, groups) {
	if (!groups.length) return `${area.name}: empty`;
	const parts = groups.map((g) => `${g.n} ${g.color} ${g.def ? g.def.label.toLowerCase() : g.piece}`);
	return `${area.name}: ${parts.join(", ")}`;
}

/** Groups an area's stack into [{color, piece, n, def}] in seat then kind order. */
function groupTokens(info, stack, seatOrder, pieceDefs) {
	const map = new Map();
	for (const pc of stack) {
		const key = pc.color + "\u0000" + pc.piece;
		const cur = map.get(key);
		if (cur) cur.n++;
		else map.set(key, { color: pc.color, piece: pc.piece, n: 1, def: pieceDefs.get(pc.piece) });
	}
	const rank = (g) => (seatOrder.has(g.color) ? seatOrder.get(g.color) : 99);
	return [...map.values()].sort((a, b) => {
		const ka = a.def ? CLASS_ORDER[classOfPiece(a.def)] ?? 4 : 4;
		const kb = b.def ? CLASS_ORDER[classOfPiece(b.def)] ?? 4 : 4;
		return rank(a) - rank(b) || ka - kb || a.piece.localeCompare(b.piece);
	});
}

/** The horizontal slice [x0, x1] of poly at height y that holds x (else null). */
function spanAt(poly, y, x) {
	const xs = [];
	for (let i = 0, j = poly.length - 1; i < poly.length; j = i++) {
		const [xi, yi] = poly[i];
		const [xj, yj] = poly[j];
		if (yi > y !== yj > y) xs.push(xi + ((y - yi) * (xj - xi)) / (yj - yi));
	}
	xs.sort((p, q) => p - q);
	for (let i = 0; i + 1 < xs.length; i += 2) {
		if (x >= xs[i] && x <= xs[i + 1]) return [xs[i], xs[i + 1]];
	}
	return null;
}

/** The vertical slice [y0, y1] of poly at x that holds y (else null). */
function vspanAt(poly, x, y) {
	const ys = [];
	for (let i = 0, j = poly.length - 1; i < poly.length; j = i++) {
		const [xi, yi] = poly[i];
		const [xj, yj] = poly[j];
		if (xi > x !== xj > x) ys.push(yi + ((x - xi) * (yj - yi)) / (xj - xi));
	}
	ys.sort((p, q) => p - q);
	for (let i = 0; i + 1 < ys.length; i += 2) {
		if (y >= ys[i] && y <= ys[i + 1]) return [ys[i], ys[i + 1]];
	}
	return null;
}

/**
 * Finds where the name and the tokens of an area go: the largest tokens
 * (scale k, as many columns as fit) whose block, with the name above it,
 * stays inside the polygon around its anchor and clear of what has been
 * placed already (`placed`: rectangles). Returns {k, cols, rows, pitch, cx,
 * labelY, blockTop, rect}; the caller adds `rect` to `placed`.
 */
function fitBlock(area, poly, n, nameW, lines, placed, badge) {
	const labelH = 5 + 13 * lines;
	const [ax, ay] = area.anchor;
	const clear = (r) => !placed.some((p) => r.x0 < p.x1 + 4 && r.x1 > p.x0 - 4 && r.y0 < p.y1 + 2 && r.y1 > p.y0 - 2);
	const tryAt = (cx, top, rows, cols, pitch) => {
		const width = cols * pitch;
		const total = labelH + rows * pitch;
		const l = spanAt(poly, top + labelH / 2, cx);
		if (!l || l[1] - l[0] < Math.min(nameW, Math.max(width, 40)) * 0.8) return null;
		for (let r = 0; r < rows; r++) {
			const sp = spanAt(poly, top + labelH + pitch * (r + 0.5), cx);
			if (!sp || sp[1] - sp[0] < width + 2) return null;
		}
		const v = vspanAt(poly, cx, ay);
		if (!v || v[0] > top + 1 || v[1] < top + total - 1) return null;
		const label = { x0: cx - nameW / 2, x1: cx + nameW / 2 + badge, y0: top, y1: top + labelH };
		const chips = rows ? { x0: cx - width / 2, x1: cx + width / 2, y0: top + labelH, y1: top + total } : null;
		if (!clear(label) || (chips && !clear(chips))) return null;
		return { label, chips };
	};
	const dys = [0, -5, 5, -10, 10, -16, 16, -24, 24, -32, 32];
	const dxs = [0, -16, 16, -32, 32, -48, 48];
	if (!n) {
		const top0 = ay - labelH / 2;
		for (const dx of dxs) {
			for (const dy of dys) {
				const hit = tryAt(ax + dx, top0 + dy, 0, 0, 0);
				if (hit) return { k: 1, cols: 0, rows: 0, pitch: 0, cx: ax + dx, labelY: top0 + dy + 12, blockTop: ay, rect: [hit.label] };
			}
		}
		return { k: 1, cols: 0, rows: 0, pitch: 0, cx: ax, labelY: ay + 4 - (13 * (lines - 1)) / 2, blockTop: ay, rect: [] };
	}
	for (let k = 1; k >= 0.45; k -= 0.05) {
		const pitch = (2 * CHIP_R + CHIP_GAP) * k;
		for (let rows = 1; rows <= n; rows++) {
			const cols = Math.ceil(n / rows);
			if (cols > 9) continue;
			const total = labelH + rows * pitch;
			for (const dx of dxs) {
				for (const dy of dys) {
					const top = ay - total / 2 + dy;
					const hit = tryAt(ax + dx, top, rows, cols, pitch);
					if (hit) {
						return { k, cols, rows, pitch, cx: ax + dx, labelY: top + 12, blockTop: top + labelH, rect: [hit.label, hit.chips] };
					}
				}
			}
		}
	}
	// Nothing fits cleanly: small tokens, centred, whatever is in the way.
	const k = 0.45;
	const pitch = (2 * CHIP_R + CHIP_GAP) * k;
	const cols = Math.min(n, 6);
	const rows = Math.ceil(n / cols);
	const top = ay - (labelH + rows * pitch) / 2;
	return { k, cols, rows, pitch, cx: ax, labelY: top + 12, blockTop: top + labelH, rect: [] };
}

/** A five-pointed star of outer radius r, centred on the origin. */
function starPath(r) {
	const pts = [];
	for (let k = 0; k < 10; k++) {
		const rad = k % 2 === 0 ? r : r * 0.45;
		const a = (Math.PI / 5) * k - Math.PI / 2;
		pts.push(`${(rad * Math.cos(a)).toFixed(2)} ${(rad * Math.sin(a)).toFixed(2)}`);
	}
	return `M ${pts.join(" L ")} Z`;
}

// One hidden SVG holds the gradients and the wave pattern every map uses
// (CSS colours them, so dark mode and themes just work).
function ensureMapDefs() {
	if (document.getElementById("map-defs")) return;
	const mk = (tag, attrs, ...kids) => s(tag, attrs, ...kids);
	const stops = (prefix, n) => Array.from({ length: n }, (_, i) => mk("stop", { offset: n === 1 ? 0 : i / (n - 1), class: `${prefix}-${i}` }));
	const defs = mk("defs");
	for (const id of ["pacific", "atlantic", "indian", "arctic"]) {
		defs.append(mk("linearGradient", { id: `g-${id}`, x1: 0, y1: 0, x2: 0.45, y2: 1 }, ...stops(`gs-${id}`, 2)));
	}
	for (const id of ["c-na", "c-sa", "c-eu", "c-me", "c-af", "c-as", "c-au", "c-ice", "desecrated"]) {
		defs.append(mk("radialGradient", { id: `g-${id}`, cx: 0.5, cy: 0.42, r: 0.82 }, ...stops(`gs-${id}`, 2)));
	}
	for (const c of ["green", "blue", "red", "yellow", "orange", "purple", "brown", "black", "white"]) {
		defs.append(mk("radialGradient", { id: `tg-${c}`, cx: 0.36, cy: 0.3, r: 0.85 }, ...stops(`tgs-${c}`, 3)));
	}
	const holder = s("svg", { id: "map-defs", "data-persist": "1", width: 0, height: 0, "aria-hidden": "true", style: "position:absolute;width:0;height:0;overflow:hidden" }, defs);
	document.body.append(holder);
}

function renderAreaMap(host, info, view, handlers) {
	ensureMapDefs();
	const b = info.board;
	const canvas = b.canvas || { width: 1000, height: 490 };
	const M = 8;
	const width = canvas.width + 2 * M;
	const height = canvas.height + 2 * M;
	const svg = s("svg", {
		id: "board-svg",
		class: "area-map",
		viewBox: `${-M} ${-M} ${width} ${height}`,
		role: "group",
		"aria-label": `${info.name} map`,
		preserveAspectRatio: "xMidYMid meet",
	});
	const clipId = `map-clip-${cssColor(info.id)}`;
	svg.append(
		s("clipPath", { id: clipId }, s("rect", { x: 0, y: 0, width: canvas.width, height: canvas.height })),
		s("rect", { class: "board-frame", x: -M, y: -M, width, height, rx: 12 })
	);

	const pieceDefs = new Map(info.pieces.map((p) => [p.type, p]));
	const seatOrder = new Map((info.seats || []).map((st, i) => [st.color, i]));
	const seatColor = new Map((info.seats || []).map((st) => [st.seat, st.color]));
	const tints = view.tints || {};
	const badges = view.badges || {};
	const legend = view.legend || {};
	const targets = view.targets || new Set();
	const selected = selectionSet(view.selected);
	const lastMove = new Set(view.lastMove || []);
	const hinted = new Set(view.hint || []);
	const tokenOk = view.tokenOk || null;
	// Units a spellbook of their cult enhances ("color|piece"), and the unit
	// whose card is open ({pos, piece, color}).
	const enhancedUnits = view.enhanced || new Set();
	const inspect = view.inspect || null;
	const holders = view.holders || null;
	// Which layers of the map are drawn: the names, the glyph badges, the sigils.
	const layers = { names: true, glyphs: true, sigils: true, ...(view.layers || {}) };
	const glyphMarks = glyphAreas(info);
	const pts = (poly) => poly.map((p) => p.join(",")).join(" ");

	// Where each area's name and tokens go. Small areas are planned first,
	// so they claim the room they need and the big ones work around them.
	const placed = [];
	const planOf = (area) => {
		const stack = (view.cells && view.cells[area.id]) || [];
		const groups = groupTokens(info, stack, seatOrder, pieceDefs);
		// Everything that stands in the area, for its description.
		const everyone = groups.map((g) => ({ ...g }));
		const ocean = area.tags.includes("ocean");
		const rowBadges = badges[area.id] || [];
		// What stands in the area: its Gate and Desecration, then the units.
		const chips = [];
		const gateKind = rowBadges.find((k) => k.startsWith("gate-"));
		if (gateKind) {
			const owner = Number(gateKind.slice(5));
			const ownerColor = seatColor.get(owner) || "";
			// The figure that stands on the Gate, when it has one: it is one of
			// the units of its kind there, drawn on the Gate and not in the
			// stack beside it.
			const holder = owner !== 0 && holders ? pieceDefs.get(holders[area.id]) || null : null;
			if (holder) {
				const gi = groups.findIndex((g) => g.piece === holder.type && g.color === ownerColor);
				if (gi >= 0) {
					groups[gi].n -= 1;
					if (groups[gi].n <= 0) groups.splice(gi, 1);
				}
			}
			chips.push({ special: "gate", color: ownerColor, abandoned: owner === 0, holder, def: (legend.badges || {})[gateKind] || {} });
		}
		if (rowBadges.includes("desecrated")) chips.push({ special: "desecrated", def: (legend.badges || {}).desecrated || {} });
		if (rowBadges.includes("iceage")) chips.push({ special: "iceage", def: (legend.badges || {}).iceage || {} });
		for (const grp of groups) chips.push(grp);

		// The name, on two lines when the area is too narrow for one.
		const perChar = ocean ? 9.0 : 8.3;
		const home = homePolygon(area);
		const mid = spanAt(home, area.anchor[1], area.anchor[0]);
		const avail = mid ? mid[1] - mid[0] : polygonBox(home).w;
		let nameLines = [area.name];
		if (area.name.length * perChar > avail * 0.92 && area.name.includes(" ")) {
			const words = area.name.split(" ");
			let best = null;
			for (let i = 1; i < words.length; i++) {
				const l1 = words.slice(0, i).join(" ");
				const l2 = words.slice(i).join(" ");
				const w = Math.max(l1.length, l2.length);
				if (!best || w < best.w) best = { w, lines: [l1, l2] };
			}
			nameLines = best.lines;
		}
		const lineW = nameLines.map((l) => l.length * perChar);
		const nameW = Math.max(...lineW);
		const glyphTag = (area.tags.find((t) => t.startsWith("glyph-")) || "").slice(6);
		// The glyph badge sits just past the end of the name's longest line,
		// at the height of the first line, so it never covers any text.
		const badgeLine = 0;
		const badgeX = nameW / 2 + 13;
		const badge = glyphTag && GLYPHS[glyphTag] ? 24 : 0;
		return { stack, groups, everyone, ocean, rowBadges, chips, home, nameLines, nameW, badgeX, badgeLine, badge, fit: null };
	};
	const plans = new Map(b.areas.map((area) => [area.id, planOf(area)]));
	const sizeOf = (area) => {
		let sum = 0;
		for (const poly of area.polygons) {
			const bx = polygonBox(poly);
			sum += bx.w * bx.h;
		}
		return sum;
	};
	for (const area of [...b.areas].sort((x, y) => sizeOf(x) - sizeOf(y))) {
		const pl = plans.get(area.id);
		pl.fit = fitBlock(area, pl.home, pl.chips.length, pl.nameW, pl.nameLines.length, placed, pl.badge);
		for (const r of pl.fit.rect) if (r) placed.push(r);
	}

	const drawArea = (area) => {
		const { stack, everyone, ocean, rowBadges, chips, nameLines, nameW, badgeX, badgeLine, fit } = plans.get(area.id);
		const isTarget = view.interactive && targets.has(area.id);
		const tint = tints[area.id] || (ocean ? "ocean" : "land");
		const g = s("g", {
			class: "cell area" + (isTarget ? " clickable" : "") + (ocean ? " ocean" : " land") + area.tags.map((t) => " tag-" + t).join(""),
			"data-position": area.id,
			"data-height": stack.length,
			role: isTarget ? "button" : null,
			tabindex: isTarget ? 0 : null,
			"aria-label": describeArea(info, area, everyone),
		});
		g.append(s("title", {}, describeArea(info, area, everyone)));

		for (const poly of area.polygons) {
			g.append(s("polygon", { class: `area-bg tint-${cssColor(tint)}`, points: pts(poly) }));
		}
		const overlay = (cls) => {
			for (const poly of area.polygons) g.append(s("polygon", { class: cls, points: pts(poly) }));
		};
		if (lastMove.has(area.id)) overlay("area-last");
		if (hinted.has(area.id)) overlay("area-hint");
		if (selected.has(area.id)) overlay("area-selected");
		if (isTarget && !view.quietTargets) overlay("area-target");
		if (rowBadges.includes("target")) overlay("area-battle");
		// The hit area sits under the tokens, so units stay clickable.
		for (const poly of area.polygons) g.append(s("polygon", { class: "cell-hit", points: pts(poly), fill: "transparent" }));

		const cx = fit.cx;
		const label = s("text", { class: "area-label" + (ocean ? " ocean-label" : ""), "data-area": area.id, "text-anchor": "middle" });
		nameLines.forEach((line, i) => {
			label.append(s("tspan", { x: cx, y: (fit.labelY + i * 13).toFixed(1) }, line));
		});
		if (layers.names) labelNodes.push(label);
		// The glyphs of the cults at the table, left of the name: where a cult
		// begins, and the areas marked with the Windwalker's and the Sleeper's
		// sigils — drawn as the sigil itself, a ring that is no Gate.
		for (const sg of glyphMarks) {
			if (!layers.sigils) break;
			if (sg.area !== area.id) continue;
			const gl = CW_GLYPHS[sg.key];
			const shape = glyphShape(sg.key, 23);
			if (!shape) continue;
			labelNodes.push(
				s(
					"g",
					{
						class: `area-sigil-badge color-${FACTION_COLORS[sg.key]}`,
						"data-area": area.id,
						"data-sigil": sg.key,
						transform: `translate(${(cx - (layers.names ? nameW / 2 : 0) - 15).toFixed(1)} ${(fit.labelY - 4).toFixed(1)})`,
					},
					s("title", {}, `${gl.name} glyph: where that cult begins${sg.key === "ww" ? " and awakens Rhan-Tegoth and Ithaqua" : ""}`),
					s("circle", { class: "sigil-disc", r: 12.5 }),
					shape
				)
			);
		}
		const glyph = (area.tags.find((t) => t.startsWith("glyph-")) || "").slice(6);
		if (layers.glyphs && glyph && GLYPHS[glyph]) {
			const gy = fit.labelY - 4 + badgeLine * 13;
			labelNodes.push(
				s(
					"g",
					{ class: "area-glyph-badge", "data-area": area.id, transform: `translate(${(cx + badgeX).toFixed(1)} ${gy.toFixed(1)})` },
					s("title", {}, `${glyph[0].toUpperCase() + glyph.slice(1)} glyph (Yellow Sign's Desecrate requirement)`),
					s("circle", { r: 11 }),
					s("path", { class: "area-glyph", d: GLYPHS[glyph], transform: "scale(0.8)" })
				)
			);
		}

		if (chips.length) {
			const S = CHIP_R * 2 * fit.k;
			let n = 0;
			for (let r = 0; r < fit.rows; r++) {
				const inRow = Math.min(fit.cols, chips.length - r * fit.cols);
				const x0 = cx - ((inRow - 1) * fit.pitch) / 2;
				for (let c = 0; c < inRow; c++) {
					const chip = chips[n++];
					const x = x0 + c * fit.pitch;
					const y = fit.blockTop + fit.pitch / 2 + r * fit.pitch;
					const at = `translate(${x.toFixed(1)} ${y.toFixed(1)})`;
					if (chip.special === "gate") {
						const c2 = cssColor(chip.color);
						// The unit on the Gate is picked up by clicking the figure (its
						// move abandons the Gate); once a unit of the area is chosen, the
						// Gate itself is where it goes: the area it stands in.
						const holderType = chip.holder ? chip.holder.type : "";
						const holderPiece = holderType ? ONGATE + holderType : "";
						const pickable = !!(view.interactive && tokenOk && holderPiece && tokenOk(area.id, holderPiece, chip.color));
						// The Gate of the area the chosen unit stands in is the place to put
						// it on (or to step off to): marked, unlike a Gate it merely may walk to.
						const aimed = isTarget && selected.has(area.id);
						const gate = s(
							"g",
							{
								class:
									`gate-chip color-${c2}` +
									(chip.abandoned ? " abandoned" : "") +
									(pickable ? " token-target gate-pickable" : "") +
									(aimed ? " gate-target" : ""),
								transform: at,
								"data-badge": chip.abandoned ? "gate-0" : "gate",
								"data-position": area.id,
								"data-piece": holderPiece || null,
								"data-color": chip.color || null,
								role: pickable || isTarget ? "button" : null,
								tabindex: pickable || isTarget ? "0" : null,
							},
							s(
								"title",
								{},
								(chip.def.label || "Gate") +
									(chip.holder ? ` — held by ${chip.holder.label}` : "") +
									(aimed ? " — click to put the chosen unit on it" : pickable ? " — click to move it; the Gate is abandoned" : "")
							),
							s("circle", { class: "gate-ring", r: S * 0.47 }),
							s("circle", { class: "gate-core", r: S * 0.3 }),
							// The card of the unit on the Gate is open: ring it, like a token.
							inspect && inspect.onGate && inspect.pos === area.id && inspect.color === chip.color
								? s("circle", { class: "token-inspect-ring", r: S * 0.62 })
								: null,
							// The unit that occupies the Gate stands on it; an abandoned
							// Gate (or one with no recorded unit) shows the plain star.
							chip.holder
								? s("g", { class: `piece gate-holder color-${c2}`, "data-holder": chip.holder.type }, ...pieceShape(chip.holder.shape, S * 0.62))
								: s("path", { class: "gate-star", d: `M0 ${-S * 0.2} L${S * 0.06} ${-S * 0.06} L${S * 0.2} 0 L${S * 0.06} ${S * 0.06} L0 ${S * 0.2} L${-S * 0.06} ${S * 0.06} L${-S * 0.2} 0 L${-S * 0.06} ${-S * 0.06} Z` })
						);
						gate.setAttribute("data-holder", holderType);
						const onGateClick = (ev) => {
							ev.stopPropagation();
							if (isTarget && handlers.onCell) handlers.onCell(area.id, ev);
							else if (holderPiece && handlers.onToken) handlers.onToken(area.id, holderPiece, chip.color, ev);
						};
						if (handlers.onToken || handlers.onCell) {
							gate.addEventListener("click", onGateClick);
							gate.addEventListener("keydown", (ev) => {
								if (ev.key === "Enter" || ev.key === " ") {
									ev.preventDefault();
									onGateClick(ev);
								}
							});
						}
						attachHover(gate, area.id, handlers);
						tokenNodes.push(gate);
						continue;
					}
					if (chip.special === "desecrated") {
						const mark = s(
							"g",
							{ class: "desecration-chip", transform: at, "data-badge": "desecrated", "data-position": area.id },
							s("title", {}, chip.def.label || "Desecrated"),
							s("rect", { x: -S * 0.4, y: -S * 0.4, width: S * 0.8, height: S * 0.8, rx: S * 0.18, transform: "rotate(45)" }),
							s("text", { x: 0, y: 1, "text-anchor": "middle", "dominant-baseline": "central", "font-size": S * 0.62 }, chip.def.glyph || "✠")
						);
						attachHover(mark, area.id, handlers);
						tokenNodes.push(mark);
						continue;
					}
					if (chip.special === "iceage") {
						const mark = s(
							"g",
							{ class: "iceage-chip", transform: at, "data-badge": "iceage", "data-position": area.id },
							s("title", {}, chip.def.text || chip.def.label || "Ice Age"),
							s("rect", { x: -S * 0.4, y: -S * 0.4, width: S * 0.8, height: S * 0.8, rx: S * 0.18, transform: "rotate(45)" }),
							s("text", { x: 0, y: 1, "text-anchor": "middle", "dominant-baseline": "central", "font-size": S * 0.62 }, chip.def.glyph || "❄")
						);
						attachHover(mark, area.id, handlers);
						tokenNodes.push(mark);
						continue;
					}
					const grp = chip;
					// Units are drawn in proportion to their class: cultists the
					// smallest, Great Old Ones the largest, in every area alike.
					const U = S * 1.02 * (CLASS_SCALE[classOfPiece(grp.def)] || 1);
					const canClick = !!(view.interactive && tokenOk && tokenOk(area.id, grp.piece, grp.color));
					const enhanced = enhancedUnits.has(`${grp.color}|${grp.piece}`);
					// (the card of the unit on the Gate belongs to the figure on it, not to the stack)
					const inspected = !!inspect && !inspect.onGate && inspect.pos === area.id && inspect.piece === grp.piece && inspect.color === grp.color;
					// Every unit can be clicked: a legal choice is picked up, any
					// other opens its information card.
					const tok = s("g", {
						class:
							`piece token color-${cssColor(grp.color)} shape-${grp.def ? grp.def.shape : "disc"}` +
							(canClick ? " token-target" : "") +
							(enhanced ? " token-enhanced" : "") +
							(inspected ? " token-inspected" : ""),
						transform: at,
						"data-piece": grp.piece,
						"data-color": grp.color,
						"data-position": area.id,
						"data-count": grp.n,
						role: "button",
						tabindex: "0",
					});
					tok.append(
						s(
							"title",
							{},
							`${grp.n} × ${grp.def ? grp.def.label : grp.piece} (${grp.color})` + (enhanced ? " — a spellbook enhances this unit" : "") + " — click for details"
						)
					);
					if (inspected) tok.append(s("circle", { class: "token-inspect-ring", r: Math.max(U, S * 0.8) * 0.72 }));
					const shape = grp.def ? grp.def.shape : "disc";
					for (const part of pieceShape(shape, U)) tok.append(part);
					// Units with a spellbook earned wear a small golden star in
					// the corner: nothing else about the icon changes.
					if (enhanced) {
						const sr = Math.max(S * 0.2, U * 0.26);
						tok.append(s("path", { class: "token-star", d: starPath(sr), transform: `translate(${(U * 0.4).toFixed(1)} ${(-U * 0.4).toFixed(1)})` }));
					}
					// Placeholder shapes (no icon of their own) carry a code.
					if (grp.def && grp.def.code && !hasUnitIcon(shape) && shape !== "cultist") {
						tok.append(s("text", { class: "token-code", x: 0, y: 1, "text-anchor": "middle", "dominant-baseline": "central", "font-size": S * 0.36 }, grp.def.code));
					}
					if (grp.n > 1) {
						const bx = Math.max(U, S * 0.7) * 0.4;
						tok.append(s("circle", { class: "token-count-bg", cx: bx, cy: bx, r: S * 0.24 }));
						tok.append(s("text", { class: "token-count", x: bx, y: bx + 0.5, "text-anchor": "middle", "dominant-baseline": "central", "font-size": S * 0.32 }, grp.n));
					}
					if (handlers.onToken) {
						tok.addEventListener("click", (ev) => {
							ev.stopPropagation();
							handlers.onToken(area.id, grp.piece, grp.color, ev);
						});
						tok.addEventListener("keydown", (ev) => {
							if (ev.key === "Enter" || ev.key === " ") {
								ev.preventDefault();
								ev.stopPropagation();
								handlers.onToken(area.id, grp.piece, grp.color, ev);
							}
						});
					}
					attachHover(tok, area.id, handlers);
					tokenNodes.push(tok);
				}
			}
		}

		if (isTarget && handlers.onCell) {
			g.addEventListener("click", (ev) => handlers.onCell(area.id, ev));
			g.addEventListener("keydown", (ev) => {
				if (ev.key === "Enter" || ev.key === " ") {
					ev.preventDefault();
					ev.stopPropagation();
					handlers.onCell(area.id);
				}
			});
		}
		attachHover(g, area.id, handlers);
		return g;
	};
	// Names and tokens are drawn above every area, so a neighbour's outline
	// never cuts them off.
	const labelNodes = [];
	const tokenNodes = [];

	// Seas first, then a pale halo of shallows round every coast, then the
	// lands on top; within each, big areas first so small ones sit above.
	const bySize = (area) => {
		let sum = 0;
		for (const poly of area.polygons) {
			const bx = polygonBox(poly);
			sum += bx.w * bx.h;
		}
		return sum;
	};
	const ordered = [...b.areas].sort((a, c) => bySize(c) - bySize(a));
	const seas = ordered.filter((a) => a.tags.includes("ocean"));
	const lands = ordered.filter((a) => !a.tags.includes("ocean"));
	for (const area of seas) svg.append(drawArea(area));
	const shore = s("g", { class: "shore", "aria-hidden": "true", "clip-path": `url(#${clipId})` });
	for (const area of lands) for (const poly of area.polygons) shore.append(s("polygon", { points: pts(poly) }));
	svg.append(shore);
	for (const area of lands) svg.append(drawArea(area));
	svg.append(s("g", { class: "map-labels", "aria-hidden": "true" }, ...labelNodes));
	svg.append(s("g", { class: "map-tokens" }, ...tokenNodes));

	clear(host).append(svg);
	let full = { x: -M, y: -M, w: width, h: height };
	let key = info.id;
	// On a phone the map has a tall box to fill: give the view that shape, so
	// zooming in uses all of it (the whole map still fits the width).
	if (document.body.classList.contains("m-mode")) {
		const r = host.getBoundingClientRect();
		if (r.width > 0 && r.height > 0 && r.width / r.height < width / height) {
			const tall = width / (r.width / r.height);
			full = { x: -M, y: -M - (tall - height) / 2, w: width, h: tall };
			key += ":fill";
		}
	}
	svg.setAttribute("viewBox", `${full.x} ${full.y} ${full.w} ${full.h}`);
	// A preview (the home page's cards) is a picture: no buttons, no panning.
	if (!view.preview) setupZoom(host, svg, full, key);
	return svg;
}

// The zoom of the map survives re-rendering (every move redraws the board).
let mapView = null;

/** Mouse-wheel / pinch zoom, drag to pan, and +/−/reset buttons. */
function setupZoom(host, svg, full, key) {
	const v = mapView && mapView.key === key ? { ...mapView.v } : { ...full };
	const clamp = (x, lo, hi) => Math.min(Math.max(x, lo), Math.max(lo, hi));
	const apply = () => {
		v.w = clamp(v.w, full.w / 8, full.w);
		v.h = (v.w * full.h) / full.w;
		v.x = clamp(v.x, full.x, full.x + full.w - v.w);
		v.y = clamp(v.y, full.y, full.y + full.h - v.h);
		svg.setAttribute("viewBox", `${v.x} ${v.y} ${v.w} ${v.h}`);
		mapView = { key, v: { ...v } };
		reset.disabled = v.w >= full.w - 0.5;
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
	const button = (label, title, fn) => h("button", { type: "button", class: "ghost", title, "aria-label": title, onclick: fn }, label);
	const centre = () => {
		const r = svg.getBoundingClientRect();
		return [r.left + r.width / 2, r.top + r.height / 2];
	};
	const reset = button("⟲", "Show the whole map", () => {
		Object.assign(v, full);
		apply();
	});
	host.append(
		h(
			"div",
			{ class: "map-zoom" },
			button("+", "Zoom in", () => zoomAt(...centre(), 0.7)),
			button("−", "Zoom out", () => zoomAt(...centre(), 1 / 0.7)),
			reset
		)
	);
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
