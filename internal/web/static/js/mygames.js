// The games this browser has going: reading their state from the server and
// drawing them as rows. The "Active games" window of the top bar lists the
// unfinished ones, the profile page the finished ones.

import { h, rememberedGames, forgetGame, ago } from "./util.js";
import { api } from "./api.js";

let defsPromise = null;

/** The game definitions, read once per page. */
export function gameDefs() {
	if (!defsPromise) {
		defsPromise = api.gameDefs().catch(() => {
			defsPromise = null;
			return [];
		});
	}
	return defsPromise;
}

/** Reads every remembered game: [{g, info, missing}], info null when it could not be read. */
export async function loadRows() {
	return Promise.all(
		rememberedGames().map(async (g) => {
			try {
				return { g, info: await api.game(g.code), missing: false };
			} catch (e) {
				return { g, info: null, missing: e.status === 404 };
			}
		})
	);
}

/** True when it is this browser's player's turn in the game. */
export function yourTurn(r) {
	if (!r.info || r.info.status !== "active") return false;
	const me = r.info.seats.find((s) => s.name && s.name.toLowerCase() === r.g.name.toLowerCase());
	return !!me && r.info.turn_seat === me.seat;
}

export const isFinished = (r) => !!r.info && r.info.status === "finished";

/** Games where it is your turn float to the top, then the newest. */
export function sortRows(rows) {
	return [...rows].sort((a, b) => Number(yourTurn(b)) - Number(yourTurn(a)) || b.g.ts - a.g.ts);
}

/** One game of the list: title, who is playing, a status badge, Open/Review and a remove button. */
export function gameRow(r, defs, onForget) {
	// A table with its own cults is a derived game ("base~ww-sl-gc-hp"): the
	// layout's name, then who sits at it.
	const defId = String((r.info && r.info.game_def_id) || r.g.def);
	const def = defs.find((d) => d.id === defId.split("~")[0]);
	let title = def ? def.name : defId;
	if (defId.includes("~") && r.info && r.info.seats) {
		title += ": " + r.info.seats.map((s) => s.label).filter(Boolean).join(", ");
		if (/-hp$/.test(defId)) title += " + High Priests";
	}
	const meta = [];
	const badges = [];
	if (r.missing) {
		meta.push("This game no longer exists.");
	} else if (!r.info) {
		meta.push("Could not load.");
	} else {
		const others = r.info.seats.filter((s) => s.name && s.name.toLowerCase() !== r.g.name.toLowerCase()).map((s) => s.name);
		meta.push(others.length ? `vs ${others.join(", ")}` : "waiting for an opponent");
		meta.push(`move ${r.info.move_seq}`);
		if (yourTurn(r)) badges.push(h("span", { class: "badge turn" }, "Your turn"));
		else if (r.info.status === "finished") badges.push(h("span", { class: "badge done" }, "Finished"));
		else if (r.info.status === "waiting") badges.push(h("span", { class: "badge waiting" }, "Waiting"));
		else badges.push(h("span", { class: "badge" }, "Their turn"));
	}
	return h(
		"li",
		{ "data-code": r.g.code },
		h("div", { class: "grow" }, h("div", { class: "title" }, `${title} · ${r.g.code}`), h("div", { class: "meta" }, meta.join(" · ") + (r.g.ts ? ` · ${ago(r.g.ts)}` : ""))),
		...badges,
		r.missing || !r.info ? null : h("a", { class: "button primary", href: "/game/" + r.g.code }, r.info.status === "finished" ? "Review" : "Open"),
		h(
			"button",
			{
				class: "ghost icon",
				title: "Remove from this list",
				"aria-label": "Remove from this list",
				onclick: () => {
					forgetGame(r.g.code);
					onForget();
				},
			},
			"✕"
		)
	);
}
