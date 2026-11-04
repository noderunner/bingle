// The list of a game's tutorials (ADR 0077), shown in the rules window and in
// the game picker: by kind, each with its summary, a mark when this browser
// has finished it, and a button that opens it in a room of its own.

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

	for (const [kind, heading] of KINDS) {
		const ofKind = tutorials.filter((t) => t.kind === kind);
		if (!ofKind.length) continue;
		if (!compact) root.append(h("h3", {}, heading));
		const ul = h("ul", { class: "tutorial-items" });
		for (const t of ofKind) {
			const finished = done.has(`${family}/${t.id}`);
			const button = h("button", { type: "button", class: finished ? "secondary" : "", "data-tutorial": t.id }, finished ? "Again" : "Start");
			button.addEventListener("click", () => start(t, button));
			ul.append(
				h(
					"li",
					{ "data-tutorial": t.id },
					h(
						"div",
						{ class: "tutorial-item-text" },
						h("strong", {}, t.title),
						finished ? h("span", { class: "badge done", title: "You finished this tutorial in this browser" }, "✓ Done") : null,
						!compact && t.summary ? h("div", { class: "muted small" }, t.summary) : null
					),
					button
				)
			);
		}
		root.append(ul);
	}
	root.append(error);
	return root;
}
