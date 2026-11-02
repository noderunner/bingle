// The Ratings tab of the home page: a leaderboard for each game that has one. It
// shows the handle, the rating and the number of games: never when or against
// whom. Who is on a board, and everything about it, is the server's to say.

import { h, clear } from "./util.js";
import { api } from "./api.js";

let mounted = false;

/** A rating as a number, marked when it is not settled yet. */
export function ratingText(rating, provisional) {
	return String(rating) + (provisional ? "?" : "");
}

/** The fair-play rule, wherever rated play is offered or shown. */
export const FAIR_PLAY = "Rated games are for people playing their own moves: using a bot or another program to choose them is against the rules.";

function board(host, game) {
	const table = h("div", { id: "leaderboard" }, h("p", { class: "muted small" }, "Loading…"));
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
					h("td", { title: p.provisional ? "Provisional: this player has few recent games, so the rating could move a long way" : "" }, ratingText(p.rating, p.provisional)),
					h("td", {}, String(p.games))
				)
			);
			table.append(
				h(
					"div",
					{ class: "table-scroll" },
					h("table", { class: "data" }, h("thead", {}, h("tr", {}, ["", "Player", "Rating", "Rated games"].map((t) => h("th", {}, t)))), h("tbody", {}, rows))
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
		h("p", { class: "muted small" }, "Each game has its own rating, across all its sizes and variants. Choose ", h("em", {}, "Rated game"), " when you set up a game of two with a game clock. You appear here after 5 rated games against at least 3 different people."),
		h("p", { class: "muted small" }, FAIR_PLAY)
	);
	const pick = h("select", { id: "leaderboard-game", "aria-label": "Game" });
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
		body.append(h("p", { class: "muted", id: "leaderboard-empty" }, "Nobody has played enough rated games to be listed yet."));
		return;
	}
	for (const g of games) pick.append(h("option", { value: g.family }, g.name));
	document.getElementById("leaderboard-pick").hidden = false;
	pick.addEventListener("change", () => board(body, games.find((g) => g.family === pick.value)));
	board(body, games[0]);
}
