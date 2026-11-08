// The games this browser has going: reading their state from the server and
// drawing them as rows. The "Active Games" window of the top bar lists the
// unfinished ones, the profile page the finished ones.

import { h, rememberedGames, rememberGame, rememberOlderGame, forgetGame, forgottenGames, chatReadUpTo, ago, toast } from "./util.js";
import { api, gameTitle } from "./api.js";

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
	// The server knows this browser's games too, wherever they were started:
	// on another device whose guest key this one adopted, or under the
	// account it signed in to (stage 7 of the accounts plan).
	try {
		const known = new Set([...rememberedGames().map((g) => g.code), ...forgottenGames()]);
		const { games } = await api.ownGames();
		for (const g of games || []) {
			if (!known.has(g.room_code) && g.seats && g.seats.length) rememberOlderGame(g.room_code, g.game_def_id, g.seats[0]);
		}
	} catch (_) {
		/* the browser's own list is still there */
	}
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

/** Adds to each game how many chat messages from others have come since it was last on screen. */
export async function loadUnread(rows) {
	await Promise.all(
		rows.map(async (r) => {
			try {
				const name = r.g.name.toLowerCase();
				const lines = (await api.chat(r.g.code, chatReadUpTo(r.g.code))) || [];
				r.unread = lines.filter((l) => String(l.name).toLowerCase() !== name).length;
			} catch (_) {
				r.unread = 0;
			}
		})
	);
}

/** True when it is this browser's player's turn in the game. */
export function yourImpulse(r) {
	if (!r.info || r.info.status !== "active") return false;
	const me = r.info.seats.find((s) => s.name && s.name.toLowerCase() === r.g.name.toLowerCase());
	return !!me && r.info.impulse_seat === me.seat;
}

export const isFinished = (r) => !!r.info && r.info.status === "finished";

/** How many seats of a game nobody holds (people or bots). */
export function openSeats(info) {
	if (!info || !info.seats) return 0;
	return info.seats.filter((s) => !s.name).length;
}

/** "2 seats open" / "1 seat open", or "" when the table is full. */
function openSeatsText(n) {
	return n > 0 ? `${n} seat${n === 1 ? "" : "s"} open` : "";
}

/** Games where it is your turn float to the top, then the newest. */
export function sortRows(rows) {
	return [...rows].sort((a, b) => Number(yourImpulse(b)) - Number(yourImpulse(a)) || b.g.ts - a.g.ts);
}

/** A game's badges, in two columns, as many rows as they need. */
const badgeColumns = (badges) => (badges.length ? h("div", { class: "game-badges" }, ...badges) : null);

/** The badges a game's own rules give it in a list ("Core only", "High Priests"). */
const tagBadges = (tags) => (tags || []).map((tag) => h("span", { class: "badge game-tag" }, tag));

/**
 * A game as the server lists it: the name and code, who is at the table, how far it
 * has gone, and a badge for the seats nobody holds, with `action` to go at the end.
 * Without one it is a link to watch it.
 */
export function openGameRow(g, defs, isMine, action = null) {
	const def = defs.find((d) => d.id === String(g.game_def_id).split("~")[0]);
	const title = g.name || (def ? gameTitle(def) : g.game_def_id);
	const names = (g.players || []).map((p) => p.Name + (p.Bot ? " (bot)" : ""));
	const meta = [!names.length ? "nobody seated yet" : names.length > 2 ? `${names.length} players` : names.join(" vs ")];
	if (g.status !== "active") meta.push(g.lobby ? "waiting for its host" : "waiting for players");
	if (g.updated_at) meta.push(ago(g.updated_at));
	const badges = [];
	badges.push(...tagBadges(g.tags));
	if (g.rated) badges.push(h("span", { class: "badge rated" }, "Rated"));
	if (g.lobby) badges.push(h("span", { class: "badge waiting" }, "In the lobby"));
	else if (g.status === "waiting") badges.push(h("span", { class: "badge waiting" }, "Waiting"));
	if (g.open_seats > 0) badges.push(h("span", { class: "badge open-seats", "data-open-seats": g.open_seats }, openSeatsText(g.open_seats)));
	if (isMine) badges.push(h("span", { class: "badge" }, "yours"));
	return h(
		"li",
		{ "data-code": g.room_code },
		h("div", { class: "grow" }, h("div", { class: "title" }, `${title} · ${g.room_code}`), h("div", { class: "meta" }, meta.join(" · "))),
		badgeColumns(badges),
		action || h("a", { class: "button primary", href: "/game/" + g.room_code }, "Watch")
	);
}

/**
 * Whether a game can go from the list, having asked first where that costs anything:
 * a game in play is resigned. A game that has not started has nothing to resign, and
 * one that is over has nothing to ask about.
 */
async function leaveGame(r, title) {
	const status = r.info && r.info.status;
	if (status === "waiting") return window.confirm(`Remove ${title} from your list? You keep your seat, and its link still brings you back.`);
	if (status !== "active") return true;
	// Resigning ends the game for the whole table, however many sit at it.
	const others = r.info.seats.filter((s) => s.name && s.name.toLowerCase() !== r.g.name.toLowerCase()).length;
	const cost = others > 1 ? " This ends the game for everyone at the table." : r.info.rated ? " This is a rated game, and counts as a loss." : "";
	if (!window.confirm(`Resign from ${title} and remove it from your list?${cost}`)) return false;
	try {
		await api.resign(r.g.code, r.g.name);
		return true;
	} catch (err) {
		toast(err.message || "Could not resign from that game.", "error");
		return false;
	}
}

/** One game of the list: title, who is playing, a status badge, Open/Review and a remove button. */
export function gameRow(r, defs, onForget) {
	// A table with its own cults is a derived game ("base~ww-sl-gc-hp"): the
	// layout's name, then who sits at it.
	const defId = String((r.info && r.info.game_def_id) || r.g.def);
	const def = defs.find((d) => d.id === defId.split("~")[0]);
	let title = def ? gameTitle(def) : defId;
	if (r.info && r.info.list_title) {
		title = r.info.list_title;
	} else if (defId.includes("~") && r.info && r.info.seats) {
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
		meta.push(!others.length ? "waiting for an opponent" : others.length === 1 ? `vs ${others[0]}` : `${others.length + 1} players`);
		badges.push(...tagBadges(r.info.tags));
		if (r.info.rated) badges.push(h("span", { class: "badge rated" }, "Rated"));
		if (yourImpulse(r)) badges.push(h("span", { class: "badge turn" }, "Your turn"));
		else if (r.info.status === "finished") badges.push(h("span", { class: "badge done" }, "Finished"));
		else if (r.info.status === "waiting") badges.push(h("span", { class: "badge waiting" }, r.info.lobby ? "In the lobby" : "Waiting"));
		else badges.push(h("span", { class: "badge" }, "Their turn"));
		// A game still filling up says how many seats are free.
		const free = r.info.status === "waiting" ? openSeats(r.info) : 0;
		if (free > 0) badges.push(h("span", { class: "badge open-seats", "data-open-seats": free }, openSeatsText(free)));
	}
	return h(
		"li",
		{ "data-code": r.g.code },
		h("div", { class: "grow" }, h("div", { class: "title" }, `${title} · ${r.g.code}`), h("div", { class: "meta" }, meta.join(" · ") + (r.g.ts ? ` · ${ago(r.g.ts)}` : ""))),
		badgeColumns(badges),
		r.missing || !r.info
			? null
			: h(
					"a",
					{ class: "button primary", href: "/game/" + r.g.code },
					r.info.status === "finished" ? "Review" : "Open",
					r.unread ? h("span", { class: "unread-count", title: `${r.unread} unread chat message${r.unread === 1 ? "" : "s"}` }, String(r.unread)) : null
			  ),
		h(
			"button",
			{
				class: "ghost icon",
				title: r.info && r.info.status === "active" ? "Resign and remove from this list" : "Remove from this list",
				"aria-label": r.info && r.info.status === "active" ? "Resign and remove from this list" : "Remove from this list",
				onclick: async () => {
					if (!(await leaveGame(r, title))) return;
					forgetGame(r.g.code);
					onForget();
				},
			},
			"✕"
		)
	);
}
