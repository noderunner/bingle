// The Ratings tab of the home page: a leaderboard for each game that has one. It
// shows the handle, the rating and the number of games: never when or against
// whom. Who is on a board, and everything about it, is the server's to say.

import { h, clear } from "./util.js";
import { api } from "./api.js";
import { t, tParts } from "./i18n.js";

let mounted = false;

/** A rating as a number, marked when it is not settled yet. */
export function ratingText(rating, provisional) {
	return String(rating) + (provisional ? "?" : "");
}

/** The fair-play rule, wherever rated play is offered or shown. */
export const FAIR_PLAY = t("rating.fair_play");

function board(host, game) {
	const table = h("div", { id: "leaderboard" }, h("p", { class: "muted small" }, t("common.loading")));
	clear(host);
	host.append(table);
	api.leaderboard(game.family)
		.then((res) => {
			clear(table);
			const rows = res.players.map((p) =>
				h(
					"tr",
					{ "data-handle": p.handle },
					h("td", {}, String(p.rank)),
					h("td", {}, "@" + p.handle),
					h("td", { title: p.provisional ? t("rating.provisional") : "" }, ratingText(p.rating, p.provisional)),
					h("td", {}, String(p.games))
				)
			);
			table.append(
				h(
					"div",
					{ class: "table-scroll" },
					h("table", { class: "data" }, h("thead", {}, h("tr", {}, ["", t("rating.player"), t("rating.rating"), t("rating.games")].map((head) => h("th", {}, head)))), h("tbody", {}, rows))
				)
			);
		})
		.catch((err) => {
			clear(table);
			table.append(h("p", { class: "error" }, err.message));
		});
}

/** Builds the tab the first time it is asked for. */
export async function mountRatings(host) {
	if (mounted) return;
	mounted = true;
	clear(host);
	host.append(
		h("p", { class: "muted small" }, ...tParts("rating.about", { rated: h("em", {}, t("setup.rated")) })),
		h("p", { class: "muted small" }, FAIR_PLAY)
	);
	const pick = h("select", { id: "leaderboard-game", "aria-label": t("rating.game") });
	const body = h("div", {});
	host.append(h("div", { class: "field", id: "leaderboard-pick", hidden: true }, pick), body);
	let games = [];
	try {
		games = (await api.ratedGames()).games;
	} catch (err) {
		body.append(h("p", { class: "error" }, err.message));
		return;
	}
	if (!games.length) {
		body.append(h("p", { class: "muted", id: "leaderboard-empty" }, t("rating.empty")));
		return;
	}
	for (const g of games) pick.append(h("option", { value: g.family }, g.name));
	document.getElementById("leaderboard-pick").hidden = false;
	pick.addEventListener("change", () => board(body, games.find((g) => g.family === pick.value)));
	board(body, games[0]);
}
