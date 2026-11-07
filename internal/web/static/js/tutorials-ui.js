// The list of a game's tutorials (ADR 0077), shown in the rules window and in
// the game picker: by kind, then by series (ADR 0123), each with its summary, a
// mark when it has been finished (by the signed-in account, or in this
// browser), and a button that opens it in a room of its own.

import { h, store, lastName } from "./util.js";
import { api } from "./api.js";
import { finishedTutorials } from "./tutorial.js";

const KINDS = [
	["rules", "Learn the rules"],
	["strategy", "Strategy"],
	["variant", "Variants"],
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
		root.append(h("p", { class: "muted" }, "This game has no tutorials yet."));
		return root;
	}
	const error = h("p", { class: "error tutorial-error", role: "alert", hidden: true });
	const done = finishedTutorials(store);

	const start = async (t, button) => {
		const name = lastName();
		error.hidden = true;
		button.disabled = true;
		try {
			const res = await api.startTutorial({ game_def_id: family, tutorial: t.id, player_name: name });
			location.assign(`/game/${res.room_code}`);
		} catch (err) {
			button.disabled = false;
			error.textContent = err.message || "The tutorial could not be started.";
			error.hidden = false;
		}
	};

	const isDone = (t) => !!t.done || done.has(`${family}/${t.id}`);
	const completed = (title) => h("span", { class: "badge done", title }, "✓ Completed");
	const item = (t, n) => {
		const finished = isDone(t);
		const button = h("button", { type: "button", class: finished ? "secondary" : "", "data-tutorial": t.id }, finished ? "Again" : "Start");
		button.addEventListener("click", () => start(t, button));
		return h(
			"li",
			{ "data-tutorial": t.id },
			h(
				"div",
				{ class: "tutorial-item-text" },
				n ? h("span", { class: "tutorial-step" }, `${n}.`) : null,
				h("strong", {}, t.title),
				finished ? completed("You have finished this tutorial") : null,
				!compact && t.summary ? h("div", { class: "muted small" }, t.summary) : null
			),
			button
		);
	};

	for (const [kind, heading] of KINDS) {
		const ofKind = tutorials.filter((t) => t.kind === kind);
		if (!ofKind.length) continue;
		if (!compact) root.append(h("h3", {}, heading));
		// The tutorials of a series come together, in its order; a tutorial that names none is a series of its own.
		const series = new Map();
		for (const t of ofKind) {
			const key = t.series ? "series:" + t.series : "own:" + t.id;
			if (!series.has(key)) series.set(key, []);
			series.get(key).push(t);
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
			list.forEach((t, i) => ul.append(item(t, i + 1)));
			root.append(
				h(
					"section",
					{ class: "tutorial-series", "data-series": name, "aria-label": `${name}, a series of tutorials` },
					h("div", { class: "tutorial-series-name" }, h("span", {}, name), h("span", { class: "muted small" }, list.length === 1 ? "1 tutorial" : `${list.length} tutorials`), list.every(isDone) ? completed("You have finished every tutorial of this series") : null),
					ul
				)
			);
		}
	}
	root.append(error);
	return root;
}
