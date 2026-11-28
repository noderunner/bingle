// The bot ladder (ADR 0160): the built-in bots' ratings in each game, which the
// server plays for on a schedule and keeps apart from people's. It is shown in
// the Ratings tab and in the guide to the bots, the same table in both.

import { h, clear } from "./util.js";
import { api } from "./api.js";
import { t } from "./i18n.js";

function table(game) {
	const rows = game.bots.map((b) =>
		h(
			"tr",
			{ "data-bot": b.bot },
			h("td", {}, String(b.rank)),
			h("td", {}, b.label),
			h("td", { title: b.provisional ? t("rating.provisional") : "" }, String(b.rating) + (b.provisional ? "?" : "")),
			h("td", {}, String(b.games))
		)
	);
	return h(
		"div",
		{ class: "table-scroll" },
		h("table", { class: "data" }, h("thead", {}, h("tr", {}, ["", t("rating.bot"), t("rating.rating"), t("rating.bot_games")].map((head) => h("th", {}, head)))), h("tbody", {}, rows))
	);
}

/**
 * The bot ladder: a game to choose and its bots, the highest rating first.
 * id names the block; heading says whether it has a heading of its own (the
 * guide's section already has one).
 */
export function botLadder({ id, heading = true }) {
	const title = id + "-title";
	const box = h("section", { id, class: "bot-ladder", "aria-labelledby": heading ? title : null, "aria-label": heading ? null : t("rating.bots") });
	if (heading) box.append(h("h3", { id: title }, t("rating.bots")));
	const body = h("div", {}, h("p", { class: "muted small" }, t("common.loading")));
	box.append(body);
	api.botRatings()
		.then(({ games }) => {
			clear(body);
			if (!games.length) {
				body.append(h("p", { class: "muted bot-ladder-empty" }, t("rating.bots_empty")));
				return;
			}
			const pick = h("select", { class: "bot-ladder-game", "aria-label": t("rating.game") });
			for (const g of games) pick.append(h("option", { value: g.family }, g.name));
			const shown = h("div", {});
			const show = () => {
				clear(shown);
				shown.append(table(games.find((g) => g.family === pick.value) || games[0]));
			};
			pick.addEventListener("change", show);
			body.append(h("div", { class: "field" }, pick), shown);
			show();
		})
		.catch((err) => {
			clear(body);
			body.append(h("p", { class: "error" }, err.message));
		});
	return box;
}
