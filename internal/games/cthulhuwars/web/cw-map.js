// Cthulhu Wars' map (ADR 0124), drawn for render.js through page.js: the
// areas on their canvas, the units grouped by cult and kind, the Gates, the
// glyphs and sigils of the printed map, Desecration and the Ice Age, and the
// marks of the rules window's key.

import { h, s, clear } from "/static/js/util.js";
import { ONGATE, attachHover, cssColor, drawArrows, pieceShape, selectionSet, setupZoom } from "/static/js/render.js";
import { CLASS_SCALE, classOfPiece, hasUnitIcon } from "./cw-icons.js";
import { CW_GLYPHS } from "./cw-glyphs.js";
import { ICE_GLYPH } from "./cw-ice.js";
import { MAP_GLYPHS } from "./cw-map-glyphs.js";

// ---------------------------------------------------------------------------
// Area maps (Cthulhu Wars): straight-edged areas on a canvas, with tokens
// grouped per faction and unit kind.
// ---------------------------------------------------------------------------

const CHIP_R = 18;
const CHIP_GAP = 3;
const CLASS_ORDER = { goo: 0, terror: 1, monster: 2, cultist: 3 };

/** A Desecrate glyph as the board prints it, centred on the origin. */
function mapGlyph(kind, scale = 1) {
	return s("g", { class: "area-glyph", transform: `scale(${scale})` }, ...(MAP_GLYPHS[kind] || []).map((part) => s("path", { d: part.d, fill: part.fill, "fill-rule": "evenodd" })));
}

// The glyphs (sigils) of the factions, as the printed map shows them: where a
// cult begins, and — for the Windwalker and the Sleeper — the areas marked with
// their sigil. The drawings come from cw-glyphs.js (traced from the wiki's
// icons); a cult's table colour tints the ring round its badge.
const FACTION_COLORS = { gc: "green", cc: "blue", bg: "red", ys: "yellow", ww: "white", sl: "orange", ow: "purple", tt: "brown", an: "teal", bu: "sand" };

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
				s("g", { class: "area-glyph-badge" }, s("circle", { r: 12 }), mapGlyph(arg, 0.72))
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
 * Finds where the tokens of an area go: the largest tokens (scale k, as many
 * columns as fit) whose block stays inside the polygon around `anchor` and
 * clear of what has been placed already (`placed`: rectangles). Returns {k,
 * cols, rows, pitch, cx, blockTop, rect}; the caller adds `rect` to `placed`.
 */
function fitBlock(anchor, poly, n, placed) {
	const [ax, ay] = anchor;
	if (!n) return { k: 1, cols: 0, rows: 0, pitch: 0, cx: ax, blockTop: ay, rect: [] };
	const clear = (r) => !placed.some((p) => r.x0 < p.x1 + 4 && r.x1 > p.x0 - 4 && r.y0 < p.y1 + 2 && r.y1 > p.y0 - 2);
	const tryAt = (cx, top, rows, cols, pitch) => {
		const width = cols * pitch;
		const total = rows * pitch;
		for (let r = 0; r < rows; r++) {
			const sp = spanAt(poly, top + pitch * (r + 0.5), cx);
			if (!sp || sp[1] - sp[0] < width + 2) return null;
		}
		const v = vspanAt(poly, cx, ay);
		if (!v || v[0] > top + 1 || v[1] < top + total - 1) return null;
		const chips = { x0: cx - width / 2, x1: cx + width / 2, y0: top, y1: top + total };
		return clear(chips) ? chips : null;
	};
	const dys = [0, -5, 5, -10, 10, -16, 16, -24, 24, -32, 32];
	const dxs = [0, -16, 16, -32, 32, -48, 48];
	for (let k = 1; k >= 0.45; k -= 0.05) {
		const pitch = (2 * CHIP_R + CHIP_GAP) * k;
		for (let rows = 1; rows <= n; rows++) {
			const cols = Math.ceil(n / rows);
			if (cols > 9) continue;
			for (const dx of dxs) {
				for (const dy of dys) {
					const top = ay - (rows * pitch) / 2 + dy;
					const chips = tryAt(ax + dx, top, rows, cols, pitch);
					if (chips) return { k, cols, rows, pitch, cx: ax + dx, blockTop: top, rect: [chips] };
				}
			}
		}
	}
	// Nothing fits cleanly: small tokens, centred, whatever is in the way.
	const k = 0.45;
	const pitch = (2 * CHIP_R + CHIP_GAP) * k;
	const cols = Math.min(n, 6);
	const rows = Math.ceil(n / cols);
	return { k, cols, rows, pitch, cx: ax, blockTop: ay - (rows * pitch) / 2, rect: [] };
}

/** How far a point is from the nearest edge of a polygon: negative outside it. */
function clearanceIn(pt, poly) {
	let best = Infinity;
	for (let i = 0; i < poly.length; i++) {
		const [ax, ay] = poly[i];
		const [bx, by] = poly[(i + 1) % poly.length];
		const dx = bx - ax;
		const dy = by - ay;
		const t = Math.max(0, Math.min(1, ((pt[0] - ax) * dx + (pt[1] - ay) * dy) / (dx * dx + dy * dy || 1)));
		best = Math.min(best, Math.hypot(pt[0] - ax - t * dx, pt[1] - ay - t * dy));
	}
	return inPolygon(pt, poly) ? best : -best;
}

// How far apart an area's glyphs stand, and the room each needs from the area's border.
const MARK_PITCH = 26;
const MARK_ROOM = 12;

/**
 * Where an area's row of `n` glyphs goes: above or below its name if they fit there, else
 * beside it, and always as far inside the border as the area allows.
 */
function marksPlace(poly, centre, n, lines, named, lands) {
	const [cx, cy] = centre;
	const half = ((n - 1) / 2) * MARK_PITCH;
	const room = (x, y) => {
		let c = Infinity;
		for (const px of [x - half, x + half]) {
			c = Math.min(c, clearanceIn([px, y], poly));
			for (const land of lands) c = Math.min(c, -clearanceIn([px, y], land));
		}
		return c;
	};
	const off = named ? 6.5 * lines + 13 : 0;
	const side = 40 + half;
	const spots = [[cx, cy - off], [cx, cy + off], [cx + side, cy], [cx - side, cy], [cx, cy]];
	const fits = spots.find(([x, y]) => room(x, y) >= MARK_ROOM);
	if (fits) return fits;
	let best = spots[0];
	for (const spot of spots) if (room(...spot) > room(...best)) best = spot;
	return best;
}

const areaCentres = new WeakMap();

/**
 * The middle of an area as the eye sees it: the centre of the largest circle that fits in
 * it, which stays inside a crooked shape as an average of its corners would not. A sea runs
 * under the land drawn over it, so the land counts as its edge.
 */
function areaCentre(area, poly, lands) {
	const known = areaCentres.get(area);
	if (known) return known;
	const room = (pt) => {
		let c = clearanceIn(pt, poly);
		for (const land of lands) if (c > 0) c = Math.min(c, -clearanceIn(pt, land));
		return c;
	};
	const box = polygonBox(poly);
	let best = { pt: [box.minX + box.w / 2, box.minY + box.h / 2], room: -Infinity };
	const scan = (x0, y0, w, h) => {
		for (let i = 0; i <= 20; i++) {
			for (let j = 0; j <= 20; j++) {
				const pt = [x0 + (w * i) / 20, y0 + (h * j) / 20];
				const r = room(pt);
				if (r > best.room) best = { pt, room: r };
			}
		}
	};
	scan(box.minX, box.minY, box.w, box.h);
	// A closer look around the best of the first pass.
	for (const span of [box.w / 10, box.w / 50]) scan(best.pt[0] - span, best.pt[1] - (span * box.h) / box.w, span * 2, (span * 2 * box.h) / box.w);
	areaCentres.set(area, best);
	return best;
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
	for (const c of ["green", "blue", "red", "yellow", "orange", "purple", "brown", "teal", "sand", "black", "white"]) {
		defs.append(mk("radialGradient", { id: `tg-${c}`, cx: 0.36, cy: 0.3, r: 0.85 }, ...stops(`tgs-${c}`, 3)));
	}
	const holder = s("svg", { id: "map-defs", "data-persist": "1", width: 0, height: 0, "aria-hidden": "true", style: "position:absolute;width:0;height:0;overflow:hidden" }, defs);
	document.body.append(holder);
}

// The picture behind a map that has one. The same element is used again on every
// redraw: a new one could show nothing for a frame while it decodes.
const mapArt = new Map();

function mapArtImage(info, canvas) {
	const params = info.params || {};
	if (!params.art) return null;
	const href = `/api/game-defs/${encodeURIComponent(String(info.id).split("~")[0])}/assets/${params.art}`;
	if (!mapArt.has(href)) mapArt.set(href, s("image", { class: "map-art", href, x: 0, y: 0, width: canvas.width, height: canvas.height, preserveAspectRatio: "none", decoding: "sync", "aria-hidden": "true" }));
	return mapArt.get(href);
}

/** The map's own Glacier glyphs, printed where the board prints them. */
function iceGlyphs(marks) {
	const g = s("g", { class: "ice-glyphs", "aria-hidden": "true" });
	for (const [x, y] of marks) {
		g.append(s("g", { class: "ice-glyph", transform: `translate(${x} ${y})` }, ...ICE_GLYPH.map((part) => s("path", { d: part.d, fill: part.fill, "fill-rule": "evenodd" }))));
	}
	return g;
}

export function renderAreaMap(host, info, view, handlers) {
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
	const art = mapArtImage(info, canvas);
	if (art) {
		svg.classList.add("has-art");
		svg.append(art);
	}

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
	// Which layers of the map are drawn: the names, the glyph badges, the sigils, the units.
	const layers = { names: true, glyphs: true, sigils: true, units: true, ice: true, ...(view.layers || {}) };
	if (art && layers.ice && info.params.ice_glyphs) svg.append(iceGlyphs(info.params.ice_glyphs));
	const glyphMarks = glyphAreas(info);
	const landShapes = b.areas.filter((a) => !a.tags.includes("ocean")).flatMap((a) => a.polygons);
	// Hidden units stay in play: one the player can pick up now is still drawn, or there would be no way to move it.
	const pts = (poly) => poly.map((p) => p.join(",")).join(" ");

	// Where each area's name and tokens go. Small areas are planned first,
	// so they claim the room they need and the big ones work around them.
	const placed = [];
	const planOf = (area) => {
		// Hidden units stay in play: one the player can pick up now is still drawn, or there would be no way to move it.
		const shownUnit = (piece, color) => layers.units || !!(view.interactive && tokenOk && tokenOk(area.id, piece, color));
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
			let holder = owner !== 0 && holders ? pieceDefs.get(holders[area.id]) || null : null;
			if (holder && !shownUnit(ONGATE + holder.type, ownerColor)) holder = null;
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
		if (rowBadges.includes("cathedral")) chips.push({ special: "desecrated", badge: "cathedral", def: { label: "Cathedral (The Ancients)", glyph: "†" } });
		if (rowBadges.includes("glacier")) chips.push({ special: "desecrated", badge: "glacier", def: { label: "Glacier: any Gate here is abandoned for good", glyph: "❄" } });
		if (rowBadges.includes("web")) chips.push({ special: "desecrated", badge: "web", def: { label: "Web Token (Atlach-Nacha)", glyph: "✳" } });
		for (const grp of groups) if (shownUnit(grp.piece, grp.color)) chips.push(grp);

		// The name, on two lines when the area is too narrow for one.
		const perChar = ocean ? 9.0 : 8.3;
		const home = homePolygon(area);
		const centre = areaCentre(area, home, ocean ? landShapes : []).pt;
		const mid = spanAt(home, centre[1], centre[0]);
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
		return { stack, groups, everyone, ocean, rowBadges, chips, home, centre, nameLines, fit: null };
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
		// The pieces stand a little below the middle, where they leave more of the name to read.
		pl.fit = fitBlock([pl.centre[0], pl.centre[1] + 10], pl.home, pl.chips.length, placed);
		for (const r of pl.fit.rect) if (r) placed.push(r);
	}

	const drawArea = (area) => {
		const { stack, everyone, ocean, rowBadges, chips, home, centre, nameLines, fit } = plans.get(area.id);
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
		// The name is at the middle of the area, whatever stands there: it is a faint layer under the pieces.
		const label = s("text", { class: "area-label" + (ocean ? " ocean-label" : ""), "data-area": area.id, "text-anchor": "middle" });
		nameLines.forEach((line, i) => {
			label.append(s("tspan", { x: centre[0].toFixed(1), y: (centre[1] + 4 + (i - (nameLines.length - 1) / 2) * 13).toFixed(1) }, line));
		});
		if (layers.names) labelNodes.push(label);
		// Its marks: the glyphs of the cults at the table (where a cult begins, and the areas
		// marked with the Windwalker's and the Sleeper's sigils, drawn as the sigil itself, a
		// ring that is no Gate), and Yellow Sign's glyph. They stay inside the area's border.
		const sigils = layers.sigils ? glyphMarks.filter((sg) => sg.area === area.id && CW_GLYPHS[sg.key] && glyphShape(sg.key, 23)) : [];
		const glyph = (area.tags.find((t) => t.startsWith("glyph-")) || "").slice(6);
		const showGlyph = !!(layers.glyphs && glyph && MAP_GLYPHS[glyph]);
		// A map with a picture may say where the board prints the glyph: it then stands there, as printed.
		const printedAt = showGlyph && info.params && info.params.glyph_marks ? info.params.glyph_marks[area.id] : null;
		const marks = sigils.length + (showGlyph && !printedAt ? 1 : 0);
		const markAt = marks ? marksPlace(home, centre, marks, nameLines.length, layers.names, ocean ? landShapes : []) : null;
		let mark = 0;
		const nextMark = () => `translate(${(markAt[0] + (mark++ - (marks - 1) / 2) * MARK_PITCH).toFixed(1)} ${markAt[1].toFixed(1)})`;
		for (const sg of sigils) {
			const gl = CW_GLYPHS[sg.key];
			const shape = glyphShape(sg.key, 23);
			labelNodes.push(
				s(
					"g",
					{
						class: `area-sigil-badge color-${FACTION_COLORS[sg.key]}`,
						"data-area": area.id,
						"data-sigil": sg.key,
						transform: nextMark(),
					},
					s("title", {}, `${gl.name} glyph: where that cult begins${sg.key === "ww" ? " and awakens Rhan-Tegoth and Ithaqua" : ""}`),
					s("circle", { class: "sigil-disc", r: 12.5 }),
					shape
				)
			);
		}
		if (showGlyph) {
			const title = s("title", {}, `${glyph[0].toUpperCase() + glyph.slice(1)} glyph (Yellow Sign's Desecrate requirement)`);
			labelNodes.push(
				printedAt
					? s("g", { class: "area-glyph-badge printed", "data-area": area.id, transform: `translate(${printedAt[0]} ${printedAt[1]})` }, title, mapGlyph(glyph))
					: s("g", { class: "area-glyph-badge", "data-area": area.id, transform: nextMark() }, title, s("circle", { r: 11 }), mapGlyph(glyph, 0.72))
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
							{ class: "desecration-chip", transform: at, "data-badge": chip.badge || "desecrated", "data-position": area.id },
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
							`${grp.n} × ${grp.def ? grp.def.label : grp.piece} (${grp.color})` + (enhanced ? " — a spellbook enhances this unit" : "") + (document.body.classList.contains("m-mode") ? " — tap for details" : "")
						)
					);
					if (inspected) tok.append(s("circle", { class: "token-inspect-ring", r: Math.max(U, S * 0.8) * 0.72 }));
					const shape = grp.def ? grp.def.shape : "disc";
					for (const part of pieceShape(shape, U)) tok.append(part);
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
	// A map with a picture has its shores in it.
	if (!art) for (const area of lands) for (const poly of area.polygons) shore.append(s("polygon", { points: pts(poly) }));
	svg.append(shore);
	for (const area of lands) svg.append(drawArea(area));
	svg.append(s("g", { class: "map-labels", "aria-hidden": "true" }, ...labelNodes));
	svg.append(s("g", { class: "map-tokens" }, ...tokenNodes));
	const anchors = new Map(b.areas.map((area) => [area.id, area.anchor ? { x: area.anchor[0], y: area.anchor[1] } : null]));
	drawArrows(svg, view, (id) => anchors.get(id) || null);

	clear(host).append(svg);
	const full = { x: -M, y: -M, w: width, h: height };
	const key = info.id;
	svg.setAttribute("viewBox", `${full.x} ${full.y} ${full.w} ${full.h}`);
	// A preview (the home page's cards) is a picture: no buttons, no panning.
	if (!view.preview) setupZoom(host, svg, full, key);
	return svg;
}

// The zoom of the map survives re-rendering (every move redraws the board).
