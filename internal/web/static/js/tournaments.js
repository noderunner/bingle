// Tournaments: the tab of the home page that lists them, the form an organiser
// fills in, and the page of one tournament. No rule of a tournament lives here:
// the server says what stands and what is allowed, and this only shows it.

import { h, clear } from "./util.js";
import { api, whoAmI } from "./api.js";
import { FAIR_PLAY } from "./ratings.js";

const FORMAT_NAMES = { round_robin: "Round robin", swiss: "Swiss" };
const STATUS_NAMES = { draft: "Draft", registration: "Open for entries", running: "Being played", finished: "Finished", abandoned: "Abandoned" };
const REASON_NAMES = { timeout: "on time", resignation: "by resignation", forfeit: "by forfeit", abandoned: "abandoned", bye: "bye", agreement: "agreed", lost: "game lost" };
const MINUTES_DEFAULT = 10;
const REFRESH_MS = 30000;

/** A clock in words: "10 minutes each", "1 day each, plus 1 hour a move". */
export function clockWords(seconds, increment) {
	const words = (n) => {
		if (n % 86400 === 0) return n === 86400 ? "1 day" : `${n / 86400} days`;
		if (n % 3600 === 0) return n === 3600 ? "1 hour" : `${n / 3600} hours`;
		if (n % 60 === 0) return n === 60 ? "1 minute" : `${n / 60} minutes`;
		return `${n} seconds`;
	};
	return words(seconds) + " each" + (increment ? `, plus ${words(increment)} a move` : "");
}

function settingsLine(t) {
	const parts = [t.game_name, FORMAT_NAMES[t.format] || t.format, clockWords(t.clock_seconds, t.clock_increment_seconds)];
	parts.push(t.games_per_pairing === 2 ? "two games a pairing, sides swapped" : "one game a pairing");
	parts.push(t.rated ? "rated" : "not rated");
	return parts.join(" · ");
}

// ---- the tab ---------------------------------------------------------------------

let mounted = false;

/** Builds the tab the first time it is asked for. */
export async function mountTournaments(host) {
	if (mounted) return;
	mounted = true;
	clear(host);
	const body = h("div", { id: "tournament-list" }, h("p", { class: "muted small" }, "Loading…"));
	host.append(h("p", { class: "muted small" }, "Round robin and Swiss tournaments for signed-in players, played by correspondence: every game has a clock, and you move when you have time."));
	const me = await whoAmI();
	if ((me.caps || []).includes("create_tournament")) {
		const button = h("button", { type: "button", class: "primary", id: "tournament-new" }, "New tournament");
		const formHost = h("div", {});
		button.addEventListener("click", () => {
			button.hidden = true;
			tournamentForm(formHost, null, (t) => {
				window.location.href = `/tournament/${t.id}`;
			});
		});
		host.append(button, formHost);
	}
	host.append(body);
	try {
		const { tournaments } = await api.tournaments();
		clear(body);
		if (!tournaments.length) {
			body.append(h("p", { class: "muted", id: "tournament-empty" }, "There are no tournaments yet."));
			return;
		}
		body.append(
			h(
				"ul",
				{ class: "tournament-rows" },
				tournaments.map((t) =>
					h(
						"li",
						{ "data-tournament": String(t.id) },
						h("a", { href: `/tournament/${t.id}` }, t.name),
						" ",
						h("span", { class: "badge" }, STATUS_NAMES[t.status] || t.status),
						h("div", { class: "muted small" }, settingsLine(t), ` · ${t.players} ${t.players === 1 ? "player" : "players"}`)
					)
				)
			)
		);
	} catch (err) {
		clear(body);
		body.append(h("p", { class: "error" }, err.message));
	}
}

// ---- the form --------------------------------------------------------------------

function labelled(text, input, hint) {
	return h("div", { class: "field" }, h("label", { for: input.id }, text), input, hint ? h("span", { class: "small muted" }, hint) : null);
}

function options(list, chosen) {
	return list.map(([value, text]) => h("option", { value: String(value), selected: String(value) === String(chosen) }, text));
}

/**
 * The organiser's form, for a new draft or (with t) a draft being changed. Its
 * limits are the server's, which says what is wrong with anything else.
 */
export async function tournamentForm(host, t, done) {
	clear(host);
	const defs = (await api.gameDefs()).filter((d) => d.seats_required === 2).sort((a, b) => a.name.localeCompare(b.name));
	const name = h("input", { id: "t-name", type: "text", maxlength: 80, required: true, value: t ? t.name : "" });
	const game = h("select", { id: "t-game" }, options(defs.map((d) => [d.id, d.name]), t ? t.game_def_id : ""));
	const format = h("select", { id: "t-format" }, options([["round_robin", "Round robin: everybody plays everybody"], ["swiss", "Swiss: rounds against players on a similar score"]], t ? t.format : "swiss"));
	const rounds = h("input", { id: "t-rounds", type: "number", min: 0, max: 64, step: 1, value: t && t.rounds ? t.rounds : "", placeholder: "From the number of players" });
	const longTimes = [[3600, "1 hour each"], [7200, "2 hours each"], [86400, "1 day each"], [259200, "3 days each"], [604800, "7 days each"], [1209600, "14 days each"]];
	const seconds = t ? t.clock_seconds : 259200;
	const isLong = longTimes.some(([v]) => v === seconds);
	const clock = h("select", { id: "t-clock" }, options([["minutes", "Minutes each"], ...longTimes], isLong ? seconds : "minutes"));
	const minutes = h("input", { id: "t-minutes", type: "number", min: 1, max: 60, step: 1, value: isLong ? MINUTES_DEFAULT : Math.round(seconds / 60), "aria-label": "Minutes each, from 1 to 60" });
	const increment = h("select", { id: "t-increment" }, options([[0, "Nothing"], [60, "1 minute"], [3600, "1 hour"], [86400, "1 day"]], t ? t.clock_increment_seconds || 0 : 0));
	const pairing = h("select", { id: "t-pairing" }, options([[2, "Two games, with sides swapped"], [1, "One game"]], t ? t.games_per_pairing : 2));
	const rated = h("input", { id: "t-rated", type: "checkbox", checked: t ? t.rated : true });
	const pie = h("input", { id: "t-pie", type: "checkbox", checked: t ? t.pie_rule : true });
	const lo = h("input", { id: "t-min", type: "number", min: 2, max: 128, step: 1, value: t ? t.min_players : 2 });
	const hi = h("input", { id: "t-max", type: "number", min: 2, max: 128, step: 1, value: t ? t.max_players : 128 });
	const text = h("textarea", { id: "t-description", rows: 3, maxlength: 2000 }, t ? t.description || "" : "");
	const status = h("p", { class: "error", id: "t-error", role: "status" });

	const roundsField = labelled("Rounds", rounds, "Leave empty for enough to separate the players.");
	const syncShape = () => {
		roundsField.hidden = format.value !== "swiss";
		minutes.hidden = clock.value !== "minutes";
	};
	format.addEventListener("change", syncShape);
	clock.addEventListener("change", syncShape);
	// The game's own time, until the organiser types one.
	let typed = !!t && !isLong;
	minutes.addEventListener("input", () => {
		typed = true;
	});
	const followGame = async () => {
		if (typed) return;
		try {
			const info = await api.gameDef(game.value);
			if (!typed) minutes.value = info.default_clock_minutes || MINUTES_DEFAULT;
		} catch (_) {
			/* the time already shown stands */
		}
	};
	game.addEventListener("change", followGame);
	followGame();
	syncShape();

	const form = h(
		"form",
		{ id: "tournament-form", class: "tournament-form" },
		labelled("Name", name),
		labelled("Game", game),
		labelled("Format", format),
		roundsField,
		h("div", { class: "field" }, h("label", { for: "t-clock" }, "Game clock"), h("div", { class: "clock-time" }, clock, minutes)),
		labelled("Added per move", increment),
		labelled("Each pairing plays", pairing),
		h("div", { class: "field" }, h("label", { class: "check" }, rated, h("span", {}, "Rated")), h("span", { class: "small muted" }, "The results change the players' ratings for this game.")),
		h("div", { class: "field" }, h("label", { class: "check" }, pie, h("span", {}, "Pie rule, where the game has it")), h("span", { class: "small muted" }, "The second player may swap sides after the first move, which evens out the advantage of moving first.")),
		labelled("Fewest players", lo),
		labelled("Most players", hi),
		labelled("Description", text),
		status,
		h("button", { type: "submit", class: "primary", id: "t-save" }, t ? "Save" : "Create draft")
	);
	form.addEventListener("submit", async (event) => {
		event.preventDefault();
		status.textContent = "";
		const body = {
			name: name.value,
			description: text.value,
			game_def_id: game.value,
			format: format.value,
			clock_seconds: clock.value === "minutes" ? Number(minutes.value) * 60 : Number(clock.value),
			clock_increment_seconds: Number(increment.value),
			games_per_pairing: Number(pairing.value),
			rated: rated.checked,
			pie_rule: pie.checked,
			min_players: Number(lo.value),
			max_players: Number(hi.value),
			rounds: format.value === "swiss" ? Number(rounds.value) : 0,
		};
		try {
			done(await (t ? api.updateTournament(t.id, body) : api.createTournament(body)));
		} catch (err) {
			status.textContent = err.message;
		}
	});
	host.append(form);
}

// ---- the page of one tournament ----------------------------------------------------

function score(n) {
	return String(n).replace(".5", "½").replace(/^0½$/, "½");
}

function standingsTable(t) {
	const swiss = t.format === "swiss";
	const started = t.status !== "draft" && t.status !== "registration";
	if (!started) {
		return h("ul", { id: "tournament-entries" }, t.standings.map((s) => h("li", { "data-handle": s.handle }, "@" + s.handle)));
	}
	const heads = ["", "Player", "Score", "Won", "Drawn", "Lost", ...(swiss ? ["Buchholz"] : []), "S-B"];
	const rows = t.standings.map((s) =>
		h(
			"tr",
			{ "data-handle": s.handle, class: s.withdrawn ? "muted" : "" },
			h("td", {}, String(s.place)),
			h("td", {}, "@" + s.handle, s.withdrawn ? " (withdrew)" : ""),
			h("td", {}, score(s.score)),
			h("td", {}, String(s.won)),
			h("td", {}, String(s.drawn)),
			h("td", {}, String(s.lost)),
			swiss ? h("td", {}, score(s.buchholz || 0)) : null,
			h("td", {}, String(s.sonneborn_berger))
		)
	);
	return h(
		"div",
		{ class: "table-scroll" },
		h("table", { class: "data", id: "tournament-standings" }, h("thead", {}, h("tr", {}, heads.map((x) => h("th", { title: x === "S-B" ? "Sonneborn-Berger: what a player scored against each opponent, weighted by that opponent's score" : "" }, x)))), h("tbody", {}, rows))
	);
}

function gameLine(g) {
	if (!g.second) return h("li", { class: "tournament-game" }, "@" + g.first, " has a bye");
	const players = `@${g.first} v @${g.second}`;
	const result = g.done ? ` ${score(g.scores[0])}–${score(g.scores[1])}` : "";
	const why = g.done && REASON_NAMES[g.reason] ? ` (${REASON_NAMES[g.reason]})` : "";
	return h(
		"li",
		{ class: "tournament-game", "data-room": g.room_code || "" },
		g.room_code ? h("a", { href: `/game/${g.room_code}` }, players) : players,
		result + why,
		g.done ? null : h("span", { class: "muted small" }, " being played")
	);
}

function schedule(t) {
	return h(
		"div",
		{ id: "tournament-schedule" },
		[...t.schedule].reverse().map((r) => h("div", { class: "tournament-round", "data-round": String(r.round) }, h("h3", {}, `Round ${r.round}`), h("ul", { class: "tournament-games" }, r.games.map(gameLine))))
	);
}

/** Runs the tournament page. */
export async function mountTournamentPage(host, id) {
	const me = await whoAmI();
	const caps = me.caps || [];
	let editing = false;
	const status = h("p", { class: "error", id: "tournament-error", role: "status" });

	const act = async (action, body) => {
		status.textContent = "";
		try {
			paint(await api.tournamentAction(id, action, body));
		} catch (err) {
			status.textContent = err.message;
		}
	};
	const button = (text, action, confirmText, primary) =>
		h(
			"button",
			{
				type: "button",
				class: primary ? "primary" : "",
				id: `t-${action}`,
				onclick: () => {
					if (!confirmText || window.confirm(confirmText)) act(action);
				},
			},
			text
		);

	function actions(t) {
		const out = [];
		const y = t.you;
		if (y.organiser && t.status === "draft") {
			const edit = h("button", { type: "button", id: "t-edit" }, "Change settings");
			edit.addEventListener("click", () => {
				editing = true;
				const box = h("div", {});
				clear(host);
				host.append(h("h1", {}, t.name), box);
				tournamentForm(box, t, (saved) => {
					editing = false;
					paint(saved);
				});
			});
			out.push(button("Open for entries", "open", null, true), edit);
		}
		if (t.status === "registration") {
			if (!y.entered && me.kind === "account" && caps.includes("enter_tournament")) out.push(button("Enter", "enter", null, true));
			if (y.entered) out.push(button("Leave", "leave"));
			if (y.organiser) out.push(button("Start", "start", `Start now, with ${t.players} ${t.players === 1 ? "player" : "players"}? Nobody can enter after this.`, true));
		}
		if (t.status === "running" && y.entered && !y.withdrawn) {
			out.push(button("Withdraw", "leave", "Withdraw from this tournament? You forfeit your games in progress and any still to come, and what you have played stands."));
		}
		if (y.organiser && ["draft", "registration", "running"].includes(t.status)) {
			out.push(button("Abandon", "abandon", "Abandon this tournament? Its games end with no winner and no rating change. This cannot be undone."));
		}
		return out;
	}

	function paint(t) {
		document.title = `Bingle — ${t.name}`;
		clear(host);
		const parts = [
			h("h1", { id: "tournament-name" }, t.name),
			h("p", {}, h("span", { class: "badge", id: "tournament-status" }, STATUS_NAMES[t.status] || t.status), " organised by @" + t.organiser),
			h("p", { class: "muted small", id: "tournament-settings" }, settingsLine(t)),
			t.description ? h("p", { id: "tournament-description", class: "tournament-description" }, t.description) : null,
			t.end_detail ? h("p", { class: "muted" }, t.end_detail) : null,
			t.status === "running" ? h("p", { id: "tournament-round" }, `Round ${t.current_round} of ${t.total_rounds}`) : null,
			t.status === "registration" && !t.you.entered ? h("p", { class: "muted small" }, me.kind === "account" ? "If your account's email address is confirmed, you are mailed when it is your turn in a tournament game." : "Sign in to enter.") : null,
			h("p", { class: "muted small" }, FAIR_PLAY.replace("Rated games are", "Tournament games are")),
			h("div", { class: "row", id: "tournament-actions" }, actions(t)),
			status,
			h("section", {}, h("h2", {}, t.status === "finished" ? "Final standings" : t.status === "draft" || t.status === "registration" ? `Players (${t.players})` : "Standings"), standingsTable(t)),
			t.schedule.length ? h("section", {}, h("h2", {}, "Games"), schedule(t)) : null,
		];
		host.append(...parts.filter(Boolean));
	}

	const load = async () => {
		if (editing || document.hidden) return null;
		const t = await api.tournament(id);
		paint(t);
		return t;
	};
	try {
		const t = await load();
		if (t && (t.status === "running" || t.status === "registration")) setInterval(() => load().catch(() => {}), REFRESH_MS);
	} catch (err) {
		clear(host);
		host.append(h("p", { class: "error", id: "tournament-error" }, err.message));
	}
}
