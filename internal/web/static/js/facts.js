import { brandName } from "./util.js";
import { t } from "./i18n.js";
// Fun facts for the top of the home page: one is drawn at random each time the
// page loads. They are about game theory in game design, the algorithms that
// play games (and the bots of this site), and the games themselves. Their words
// are the catalogs' fact.* keys, in every language (ADR 0141).
//
// Keep them true and checkable. The Tak figure is computed (see
// docs/ui.md, "Fun facts"), not a published statistic, and says so.

export const FACTS = [
	// ---- game theory and game design ----
	"fact.zermelo",
	"fact.hex_draw",
	"fact.hex_invented",
	"fact.pie_rule",
	"fact.nim",
	"fact.rps",
	"fact.dominant",
	"fact.nash",
	"fact.yavalath",
	"fact.cepheus",
	"fact.elo",
	"fact.branching",
	"fact.intransitive",
	"fact.hidden_info",

	// ---- algorithms ----
	"fact.monte_carlo",
	"fact.mcts",
	"fact.alpha_beta",
	"fact.minimax",
	"fact.shannon",
	"fact.deep_blue",
	"fact.alphago",
	"fact.bots",
	"fact.transposition",
	"fact.deepening",
	"fact.checkers_solved",
	"fact.playouts",

	// ---- how big are games ----
	"fact.chess_positions",
	"fact.tak_6x6",
	"fact.tak_5x5",
	"fact.connect_four",
	"fact.tic_tac_toe",
	"fact.checkers_size",
	"fact.rubik",

	// ---- the games ----
	"fact.tak_origin",
	"fact.tak_stacks",
	"fact.ur",
	"fact.go_age",
	"fact.havannah",
];

/** One fact, chosen at random. */
export function randomFact(rand = Math.random) {
	return t(FACTS[Math.floor(rand() * FACTS.length)], { brand: brandName() });
}
