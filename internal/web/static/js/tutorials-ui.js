// The list of a game's tutorials (ADR 0077), shown in the rules window and in
// the game picker: by kind, then by series (ADR 0123), each with its summary, a
// mark when it has been finished (by the signed-in account, or in this
// browser), and a button that opens it in a room of its own.

import { h, store, lastName } from "./util.js";
import { api } from "./api.js";
import { finishedTutorials } from "./tutorial.js";
import { t } from "./i18n.js";

const KINDS = [
	["rules", "tutorial.rules"],
	["strategy", "tutorial.strategy"],
	["variant", "tutorial.variants"],
];

/**
 * tutorialList(family, tutorials, {compact}) returns the list.
 * nameField is an input that already holds the player's name (the game
 * picker's); without one the list asks for the name itself. compact leaves out
 * the headings and summaries (a line under the game picker).
 */
export function tutorialList(family, tutorials, { compact = false } = {}) {
	const root = h("div", { class: "tutorial-list" + (compact ? " compact" : ""), "data-family": family });
	if (!tutorials.length) {
		root.append(h("p", { class: "muted" }, t("tutorial.none")));
		return root;
	}
	const error = h("p", { class: "error tutorial-error", role: "alert", hidden: true });
	const done = finishedTutorials(store);

	const start = async (tut, button) => {
		const name = lastName();
		error.hidden = true;
		button.disabled = true;
		try {
			const res = await api.startTutorial({ game_def_id: family, tutorial: tut.id, player_name: name });
			location.assign(`/game/${res.room_code}`);
		} catch (err) {
			button.disabled = false;
			error.textContent = err.message || t("tutorial.failed");
			error.hidden = false;
		}
	};

	const isDone = (tut) => !!tut.done || done.has(`${family}/${tut.id}`);
	const completed = (title) => h("span", { class: "badge done", title }, t("tutorial.completed"));
	const item = (tut, n) => {
		const finished = isDone(tut);
		const button = h("button", { type: "button", class: finished ? "secondary" : "", "data-tutorial": tut.id }, finished ? t("tutorial.again") : t("tutorial.start"));
		button.addEventListener("click", () => start(tut, button));
		return h(
			"li",
			{ "data-tutorial": tut.id },
			h(
				"div",
				{ class: "tutorial-item-text" },
				n ? h("span", { class: "tutorial-step" }, `${n}.`) : null,
				h("strong", {}, tut.title),
				finished ? completed(t("tutorial.finished_title")) : null,
				!compact && tut.summary ? h("div", { class: "muted small" }, tut.summary) : null
			),
			button
		);
	};

	for (const [kind, heading] of KINDS) {
		const ofKind = tutorials.filter((tut) => tut.kind === kind);
		if (!ofKind.length) continue;
		if (!compact) root.append(h("h3", {}, t(heading)));
		// The tutorials of a series come together, in its order; a tutorial that names none is a series of its own.
		const series = new Map();
		for (const tut of ofKind) {
			const key = tut.series ? "series:" + tut.series : "own:" + tut.id;
			if (!series.has(key)) series.set(key, []);
			series.get(key).push(tut);
		}
		let loose = null;
		for (const list of series.values()) {
			const name = list[0].series;
			if (!name) {
				if (!loose) root.append((loose = h("ul", { class: "tutorial-items" })));
				loose.append(item(list[0]));
				continue;
			}
			loose = null;
			const ul = h("ul", { class: "tutorial-items" });
			list.forEach((tut, i) => ul.append(item(tut, i + 1)));
			root.append(
				h(
					"section",
					{ class: "tutorial-series", "data-series": name, "aria-label": t("tutorial.series", { name }) },
					h("div", { class: "tutorial-series-name" }, h("span", {}, name), h("span", { class: "muted small" }, t("tutorial.count", { n: list.length })), list.every(isDone) ? completed(t("tutorial.series_done")) : null),
					ul
				)
			);
		}
	}
	root.append(error);
	return root;
}
