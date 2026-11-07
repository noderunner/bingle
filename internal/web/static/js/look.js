// A game's look on this page (ADR 0124). The server's game info says what
// things are; a game's web.yaml, served as JSON at /api/game-defs/{id}/web,
// says how this page draws them. applyLook lays the file over the info, so the
// rest of the page reads the fields where it always has: info.board.style,
// info.pieces[].shape, info.seats[].blend, info.display, info.preview, info.cards.
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

/**
 * The game's own page script (the look's `script`, ADR 0124), imported once
 * per family: an ES module whose exports the page asks for what it draws for
 * this game alone (`info.page.seatGlyph`, ...). A game without one, or one
 * whose script fails to load, gets an empty object, and the page draws it by
 * itself.
 */
export function pageOf(family, file) {
	const script = file && typeof file === "object" ? file.script : "";
	if (!script) return Promise.resolve({});
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
	return pages.get(url);
}
