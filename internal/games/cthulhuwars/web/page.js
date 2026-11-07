// Cthulhu Wars' part of the web page (ADR 0124): the page imports this module
// through the game's web.yaml and asks its exports for what it draws for this
// game alone. Nothing in the shared page names the game for these.

import { h } from "/static/js/util.js";
import { FACTION_KEYS, factionName } from "./cw-names.js";

/** The glyph of the faction a seat plays, beside its name, or null. */
export function seatGlyph(label) {
	const key = FACTION_KEYS.find((k) => factionName(k) === label);
	if (!key) return null;
	return h("img", { class: "seat-glyph", src: `/api/game-defs/cthulhu-wars/assets/guide/glyphs/${key}.svg`, alt: label, title: label, width: 22, height: 22, decoding: "sync" });
}
