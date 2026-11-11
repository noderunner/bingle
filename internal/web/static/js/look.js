// A game's look on this page (ADR 0124). The server's game info says what
// things are; a game's web.yaml, served as JSON at /api/game-defs/{id}/web,
// says how this page draws them. applyLook lays the file over the info, so the
// rest of the page reads the fields where it always has: info.board.style,
// info.pieces[].shape, info.seats[].blend, info.display, info.preview, info.cards,
// info.board.areas and canvas, and the page script's own info.pageData.
// A value in the file wins over one the info still carries; a game with no file
// is drawn from its info alone.

/** JSON merge patch (RFC 7386): patch over target, a null removing a field. */
export function mergePatch(target, patch) {
	if (patch === null || typeof patch !== "object" || Array.isArray(patch)) return patch;
	const out = target !== null && typeof target === "object" && !Array.isArray(target) ? { ...target } : {};
	for (const [key, value] of Object.entries(patch)) {
		if (value === null) delete out[key];
		else out[key] = mergePatch(out[key], value);
	}
	return out;
}

/** The file for one game: the family's, with that game's variant patched in. */
export function resolveLook(file, id) {
	if (!file || typeof file !== "object") return {};
	const { variants, ...base } = file;
	// A derived game ("family~options") looks like the game it was derived from.
	const own = variants && (variants[id] || variants[String(id).split("~")[0]]);
	return own ? mergePatch(base, own) : base;
}

/** The centre of the largest of an area's polygons. */
export function polygonCenter(polys) {
	const twiceArea = (poly) => poly.reduce((a, p, i) => { const q = poly[(i + 1) % poly.length]; return a + p[0] * q[1] - q[0] * p[1]; }, 0);
	let best = null;
	let bestArea = -1;
	for (const poly of polys || []) {
		const a = Math.abs(twiceArea(poly));
		if (a > bestArea) [best, bestArea] = [poly, a];
	}
	if (!best || !best.length) return [0, 0];
	let cx = 0;
	let cy = 0;
	let a2 = 0;
	best.forEach((p, i) => {
		const q = best[(i + 1) % best.length];
		const cross = p[0] * q[1] - q[0] * p[1];
		a2 += cross;
		cx += (p[0] + q[0]) * cross;
		cy += (p[1] + q[1]) * cross;
	});
	return a2 === 0 ? best[0] : [cx / (3 * a2), cy / (3 * a2)];
}

/**
 * A map of areas from the look's outlines and the board's positions: each
 * area with its name and tags (the info's) and its outlines and anchor (the
 * look's, the anchor the centre of its largest outline when the look gives
 * none). A position the look does not outline (one a derived game adds, as
 * Cthulhu Wars' Moon) is left for the game's page script. Positions are
 * placed at their anchors, so that code reading a position's x and y finds
 * it on the map.
 */
function areaBoard(board, outlines) {
	const byId = new Map(outlines.map((a) => [a.id, a]));
	const areas = [];
	const anchors = new Map();
	for (const p of board.positions || []) {
		const o = byId.get(p.id);
		if (!o) continue;
		const anchor = o.anchor || polygonCenter(o.polygons);
		anchors.set(p.id, anchor);
		areas.push({ id: p.id, name: p.name || p.id, polygons: o.polygons, anchor, tags: p.tags || [] });
	}
	const positions = (board.positions || []).map((p) => (anchors.has(p.id) ? { ...p, x: anchors.get(p.id)[0], y: anchors.get(p.id)[1] } : p));
	return { areas, positions };
}

/**
 * The areas a game's page script outlines itself: a position of a map of
 * areas that the look leaves out (one a derived game adds, as Cthulhu Wars'
 * Moon), when the script exports outlineArea(position, info) and it returns
 * {polygons, anchor}. info is changed in place, once the script has loaded.
 */
export function outlineMissing(info, page) {
	const b = info.board;
	if (!b || b.style !== "areas" || !b.areas || !page || typeof page.outlineArea !== "function") return info;
	const have = new Set(b.areas.map((a) => a.id));
	for (const p of b.positions || []) {
		if (have.has(p.id)) continue;
		const o = page.outlineArea(p, info);
		if (!o || !o.polygons) continue;
		const anchor = o.anchor || polygonCenter(o.polygons);
		b.areas.push({ id: p.id, name: p.name || p.id, polygons: o.polygons, anchor, tags: p.tags || [] });
		p.x = anchor[0];
		p.y = anchor[1];
	}
	return info;
}

/** info with the look laid over it. info itself is not changed. */
export function applyLook(info, file) {
	const look = resolveLook(file, info.id);
	if (!Object.keys(look).length) return info;
	const out = { ...info };
	if (look.seats && info.seats) {
		out.seats = info.seats.map((s) => (look.seats[s.seat] ? { ...s, ...look.seats[s.seat] } : s));
	}
	if (look.pieces && info.pieces) {
		out.pieces = info.pieces.map((p) => (look.pieces[p.type] ? { ...p, ...look.pieces[p.type] } : p));
	}
	if (look.board) out.board = { ...info.board, ...look.board };
	// What the game's page script alone reads (the look's data).
	if (look.data) out.pageData = look.data;
	if (look.board && look.board.areas && info.board) out.board = { ...out.board, ...areaBoard(info.board, look.board.areas) };
	if (look.display) {
		const display = { ...(info.display || {}) };
		for (const kind of ["tints", "badges"]) {
			if (!look.display[kind]) continue;
			const marks = { ...(display[kind] || {}) };
			for (const [key, mark] of Object.entries(look.display[kind])) marks[key] = { ...(marks[key] || {}), ...mark };
			display[kind] = marks;
		}
		if (look.display.no_hands !== undefined) display.no_hands = look.display.no_hands;
		out.display = display;
	}
	if (look.cards && info.cards) {
		const cards = { ...info.cards };
		if (look.cards.back) cards.back = { ...(cards.back || {}), ...look.cards.back };
		if (look.cards.zones && cards.zones) cards.zones = cards.zones.map((z) => (look.cards.zones[z.id] ? { ...z, ...look.cards.zones[z.id] } : z));
		if (look.cards.suits && cards.suits) {
			const suits = { ...cards.suits };
			for (const [id, suit] of Object.entries(look.cards.suits)) if (suits[id]) suits[id] = { ...suits[id], ...suit };
			cards.suits = suits;
		}
		out.cards = cards;
	}
	if (look.preview) {
		// In the info's shape: stacks by square, bottom first, leaving out
		// the squares a smaller variant's board lacks.
		const squares = new Set(((info.board && info.board.positions) || []).map((p) => p.id));
		const preview = {};
		for (const e of look.preview) {
			if (squares.size && !squares.has(e.position)) continue;
			(preview[e.position] = preview[e.position] || []).push({ piece: e.piece, color: e.color });
		}
		out.preview = preview;
	}
	return out;
}

const pages = new Map();
const styles = new Map();

/**
 * Links a game's own stylesheet (the look's `style`) once; it stays for the life
 * of the page. The promise settles when the sheet has loaded (or failed to), so
 * the game's parts are not drawn before their rules.
 */
function linkStyle(href) {
	if (typeof document === "undefined") return Promise.resolve();
	if (!styles.has(href)) {
		styles.set(
			href,
			new Promise((done) => {
				const link = document.createElement("link");
				link.rel = "stylesheet";
				link.href = href;
				link.dataset.gameStyle = href;
				link.onload = () => done();
				link.onerror = () => {
					console.error(`the stylesheet ${href} did not load`);
					done();
				};
				document.head.append(link);
			})
		);
	}
	return styles.get(href);
}

export function pageOf(family, file) {
	const parts = file && typeof file === "object" ? file : {};
	const styled = parts.style ? linkStyle(`/static/games/${encodeURIComponent(family)}/${parts.style}`) : Promise.resolve();
	const script = parts.script || "";
	if (!script) return styled.then(() => ({}));
	const url = `/static/games/${encodeURIComponent(family)}/${script}`;
	if (!pages.has(url)) {
		pages.set(
			url,
			import(url).catch((err) => {
				console.error(`the page script of ${family} did not load`, err);
				return {};
			})
		);
	}
	return Promise.all([pages.get(url), styled]).then(([page]) => page);
}
