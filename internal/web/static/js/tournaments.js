// Tournaments: the tab of the home page that lists them, the form an organiser
// fills in, and the page of one tournament. No rule of a tournament lives here:
// the server says what stands and what is allowed, and this only shows it.

import { h, clear, brandName } from "./util.js";
import { api, gameTitle, whoAmI } from "./api.js";
// t is a tournament throughout this module: the words are tr().
import { t as tr, tParts, lang } from "./i18n.js";

const FORMAT_NAMES = { round_robin: "tourney.format_round_robin", swiss: "tourney.format_swiss" };
const STATUS_NAMES = { draft: "tourney.status_draft", registration: "tourney.status_registration", check_in: "tourney.status_check_in", running: "tourney.status_running", finished: "tourney.status_finished", abandoned: "tourney.status_abandoned" };
const REASON_NAMES = { timeout: "tourney.reason_timeout", resignation: "tourney.reason_resignation", forfeit: "tourney.reason_forfeit", abandoned: "tourney.reason_abandoned", bye: "tourney.reason_bye", agreement: "tourney.reason_agreement", lost: "tourney.reason_lost", no_show: "tourney.reason_no_show", no_shows: "tourney.reason_no_shows" };
const MINUTES_DEFAULT = 10;
const REFRESH_MS = 30000;
const REFRESH_LIVE_MS = 5000;

/** A clock in words: "10 minutes each", "1 day each, plus 1 hour a move". */
export function clockWords(seconds, increment) {
	const words = (n) => {
		if (n % 86400 === 0) return tr("time.days", { n: n / 86400 });
		if (n % 3600 === 0) return tr("time.hours", { n: n / 3600 });
		if (n % 60 === 0) return tr("time.minutes", { n: n / 60 });
		return tr("time.seconds", { n });
	};
	return increment ? tr("tourney.clock_plus", { time: words(seconds), increment: words(increment) }) : tr("tourney.clock_each", { time: words(seconds) });
}

function settingsLine(t) {
	const parts = [t.game_name, FORMAT_NAMES[t.format] ? tr(FORMAT_NAMES[t.format]) : t.format, clockWords(t.clock_seconds, t.clock_increment_seconds)];
	parts.push(t.games_per_pairing === 2 ? tr("tourney.two_games") : tr("tourney.one_game"));
	parts.push(t.rated ? tr("tourney.rated") : tr("tourney.not_rated"));
	if (t.real_time) parts.push(tr("tourney.real_time_starting", { when: when(t.starts_at) }));
	return parts.join(" · ");
}

/** A moment, in the viewer's own time. */
function when(iso) {
	return new Date(iso).toLocaleString(lang, { dateStyle: "medium", timeStyle: "short" });
}

/** A moment as a datetime-local input holds it. */
function localInput(iso) {
	const d = iso ? new Date(iso) : new Date(Date.now() + 86400000);
	const pad = (n) => String(n).padStart(2, "0");
	return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

// ---- the tab ---------------------------------------------------------------------

let mounted = false;

/** Builds the tab the first time it is asked for. */
export async function mountTournaments(host) {
	if (mounted) return;
	mounted = true;
	clear(host);
	const body = h("div", { id: "tournament-list" }, h("p", { class: "muted small" }, tr("common.loading")));
	host.append(h("p", { class: "muted small" }, tr("tourney.about")));
	const me = await whoAmI();
	if ((me.caps || []).includes("create_tournament")) {
		const button = h("button", { type: "button", class: "primary", id: "tournament-new" }, tr("tourney.new"));
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
			body.append(h("p", { class: "muted", id: "tournament-empty" }, tr("tourney.none")));
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
						h("a", { href: `/tournament/${t.id}`, translate: "no" }, t.name),
						" ",
						h("span", { class: "badge" }, STATUS_NAMES[t.status] ? tr(STATUS_NAMES[t.status]) : t.status),
						h("div", { class: "muted small" }, settingsLine(t), " · " + tr("tourney.players", { n: t.players }))
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
	const game = h("select", { id: "t-game" }, options(defs.map((d) => [d.id, gameTitle(d)]), t ? t.game_def_id : ""));
	const format = h("select", { id: "t-format" }, options([["round_robin", tr("tourney.format_rr_long")], ["swiss", tr("tourney.format_swiss_long")]], t ? t.format : "swiss"));
	const rounds = h("input", { id: "t-rounds", type: "number", min: 0, max: 64, step: 1, value: t && t.rounds ? t.rounds : "", placeholder: tr("tourney.rounds_auto") });
	const longTimes = [[3600, tr("tourney.t_1h_each")], [7200, tr("setup.2h_each")], [86400, tr("setup.1d_each")], [259200, tr("tourney.t_3d_each")], [604800, tr("tourney.t_7d")], [1209600, tr("tourney.t_14d")]];
	const seconds = t ? t.clock_seconds : 259200;
	const isLong = longTimes.some(([v]) => v === seconds);
	const clock = h("select", { id: "t-clock" }, options([["minutes", tr("setup.minutes_each")], ...longTimes], isLong ? seconds : "minutes"));
	const minutes = h("input", { id: "t-minutes", type: "number", min: 1, max: 60, step: 1, value: isLong ? MINUTES_DEFAULT : Math.round(seconds / 60), "aria-label": tr("setup.minutes_each_label") });
	const increment = h("select", { id: "t-increment" }, options([[0, tr("setup.nothing")], [60, tr("setup.t_1m")], [3600, tr("setup.t_1h")], [86400, tr("setup.t_1d")]], t ? t.clock_increment_seconds || 0 : 0));
	const pairing = h("select", { id: "t-pairing" }, options([[2, tr("tourney.pairing_two")], [1, tr("tourney.pairing_one")]], t ? t.games_per_pairing : 2));
	const rated = h("input", { id: "t-rated", type: "checkbox", checked: t ? t.rated : true });
	const pie = h("input", { id: "t-pie", type: "checkbox", checked: t ? t.pie_rule : true });
	const lo = h("input", { id: "t-min", type: "number", min: 2, max: 128, step: 1, value: t ? t.min_players : 2 });
	const hi = h("input", { id: "t-max", type: "number", min: 2, max: 128, step: 1, value: t ? t.max_players : 128 });
	const text = h("textarea", { id: "t-description", rows: 3, maxlength: 2000 }, t ? t.description || "" : "");
	const status = h("p", { class: "error", id: "t-error", role: "status" });
	const realTime = h("input", { id: "t-real-time", type: "checkbox", checked: t ? !!t.real_time : false });
	const starts = h("input", { id: "t-starts", type: "datetime-local", value: localInput(t && t.starts_at) });
	const checkIn = h("input", { id: "t-check-in", type: "number", min: 1, max: 1440, step: 1, value: t && t.check_in_seconds ? Math.round(t.check_in_seconds / 60) : 15 });
	const roundBreak = h("input", { id: "t-break", type: "number", min: 0, max: 60, step: 1, value: t && t.real_time ? Math.round(t.round_break_seconds / 60) : 1 });
	const firstMove = h("input", { id: "t-first-move", type: "number", min: 10, step: 1, value: t && t.first_move_seconds ? t.first_move_seconds : "", placeholder: tr("tourney.first_move_hint") });
	const realTimeFields = h(
		"div",
		{ id: "t-real-time-fields" },
		labelled(tr("tourney.starts_at"), starts, tr("tourney.starts_note")),
		labelled(tr("tourney.check_in_opens"), checkIn, tr("tourney.dropped_note")),
		labelled(tr("tourney.break"), roundBreak),
		labelled(tr("tourney.first_move"), firstMove, tr("tourney.first_move_note"))
	);

	const roundsField = labelled(tr("tourney.rounds"), rounds, tr("tourney.rounds_note"));
	const syncShape = () => {
		roundsField.hidden = format.value !== "swiss";
		minutes.hidden = clock.value !== "minutes";
		realTimeFields.hidden = !realTime.checked;
	};
	realTime.addEventListener("change", syncShape);
	format.addEventListener("change", syncShape);
	clock.addEventListener("change", syncShape);
	// The game's own time, until the organiser types one.
	let typed = !!t && !isLong;
	minutes.addEventListener("input", () => {
		typed = true;
	});
	const followGame = async () => {
		try {
			const info = await api.gameDef(game.value);
			// A prototype is never rated.
			rated.disabled = !!info.prototype;
			if (info.prototype) rated.checked = false;
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
		labelled(tr("tourney.name"), name),
		labelled(tr("tourney.game"), game),
		labelled(tr("tourney.format"), format),
		roundsField,
		h("div", { class: "field" }, h("label", { for: "t-clock" }, tr("setup.game_clock")), h("div", { class: "clock-time" }, clock, minutes)),
		labelled(tr("setup.added_per_move"), increment),
		h("div", { class: "field" }, h("label", { class: "check" }, realTime, h("span", {}, tr("tourney.real_time"))), h("span", { class: "small muted" }, tr("tourney.real_time_note"))),
		realTimeFields,
		labelled(tr("tourney.each_pairing"), pairing),
		h("div", { class: "field" }, h("label", { class: "check" }, rated, h("span", {}, tr("tourney.rated_label"))), h("span", { class: "small muted" }, tr("tourney.rated_note"))),
		h("div", { class: "field" }, h("label", { class: "check" }, pie, h("span", {}, tr("tourney.pie"))), h("span", { class: "small muted" }, tr("tourney.pie_note"))),
		labelled(tr("tourney.fewest"), lo),
		labelled(tr("tourney.most"), hi),
		labelled(tr("tourney.description"), text),
		status,
		h("button", { type: "submit", class: "primary", id: "t-save" }, t ? tr("common.save") : tr("tourney.create_draft"))
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
		if (realTime.checked) {
			const at = new Date(starts.value);
			body.real_time = true;
			body.starts_at = Number.isNaN(at.getTime()) ? "" : at.toISOString().replace(/\.\d+Z$/, "Z");
			body.check_in_seconds = Number(checkIn.value) * 60;
			body.round_break_seconds = Number(roundBreak.value) * 60;
			body.first_move_seconds = Number(firstMove.value) || 0;
		}
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
	const started = t.status !== "draft" && t.status !== "registration" && t.status !== "check_in";
	if (!started) {
		return h("ul", { id: "tournament-entries" }, t.standings.map((s) => h("li", { "data-handle": s.handle }, "@" + s.handle, t.status === "check_in" && s.checked_in ? " " + tr("tourney.checked_in") : "")));
	}
	const heads = ["", tr("tourney.place_player"), tr("tourney.score"), tr("tourney.won"), tr("tourney.drawn"), tr("tourney.lost"), ...(swiss ? ["Buchholz"] : []), "S-B"];
	const rows = t.standings.map((s) =>
		h(
			"tr",
			{ "data-handle": s.handle, class: s.withdrawn ? "muted" : "" },
			h("td", {}, String(s.place)),
			h(
				"td",
				{},
				"@" + s.handle,
				s.withdrawn ? " " + tr("tourney.withdrew") : "",
				// A game a moderator took out of the ratings (ADR 0157): its result stands here.
				...(s.rating_removals || []).map((r) =>
					h("div", { class: "muted small rating-removed", "data-room": r.room_code }, tr("tourney.rating_removed", { change: r.change > 0 ? "+" + r.change : String(r.change), handle: r.by }))
				)
			),
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
		h("table", { class: "data", id: "tournament-standings" }, h("thead", {}, h("tr", {}, heads.map((x) => h("th", { title: x === "S-B" ? tr("tourney.sb_title") : "" }, x)))), h("tbody", {}, rows))
	);
}

function gameLine(g) {
	if (!g.second) return h("li", { class: "tournament-game" }, tr("tourney.bye", { handle: "@" + g.first }));
	const players = `@${g.first} v @${g.second}`;
	const result = g.done ? ` ${score(g.scores[0])}–${score(g.scores[1])}` : "";
	const why = g.done && REASON_NAMES[g.reason] ? ` (${tr(REASON_NAMES[g.reason])})` : "";
	return h(
		"li",
		{ class: "tournament-game", "data-room": g.room_code || "" },
		g.room_code ? h("a", { href: `/game/${g.room_code}` }, players) : players,
		result + why,
		g.done ? null : h("span", { class: "muted small" }, " " + (g.room_code ? tr("tourney.being_played") : tr("tourney.to_come")))
	);
}

function schedule(t) {
	return h(
		"div",
		{ id: "tournament-schedule" },
		[...t.schedule].reverse().map((r) => h("div", { class: "tournament-round", "data-round": String(r.round) }, h("h3", {}, tr("tourney.round", { n: r.round })), h("ul", { class: "tournament-games" }, r.games.map(gameLine))))
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
			const edit = h("button", { type: "button", id: "t-edit" }, tr("tourney.change"));
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
			out.push(button(tr("tourney.open"), "open", null, true), edit);
		}
		if (t.status === "check_in" && y.entered) {
			if (!y.checked_in) out.push(button(tr("tourney.check_in"), "check-in", null, true));
			out.push(button(tr("tourney.leave"), "leave"));
		}
		if (t.status === "registration") {
			if (!y.entered && me.kind === "account" && caps.includes("enter_tournament")) out.push(button(tr("tourney.enter"), "enter", null, true));
			if (y.entered) out.push(button(tr("tourney.leave"), "leave"));
			if (y.organiser && !t.real_time) out.push(button(tr("tourney.start"), "start", tr("tourney.start_confirm", { n: t.players }), true));
		}
		if (t.status === "running" && y.entered && !y.withdrawn) {
			out.push(button(tr("tourney.withdraw"), "leave", tr("tourney.withdraw_confirm")));
		}
		if (y.organiser && ["draft", "registration", "check_in", "running"].includes(t.status)) {
			out.push(button(tr("tourney.abandon"), "abandon", tr("tourney.abandon_confirm")));
		}
		return out;
	}

	function paint(t) {
		document.title = `${brandName()} — ${t.name}`;
		clear(host);
		const parts = [
			h("h1", { id: "tournament-name", translate: "no" }, t.name),
			h("p", {}, h("span", { class: "badge", id: "tournament-status" }, STATUS_NAMES[t.status] ? tr(STATUS_NAMES[t.status]) : t.status), " " + tr("tourney.organised_by", { handle: "@" + t.organiser })),
			h("p", { class: "muted small", id: "tournament-settings" }, settingsLine(t)),
			t.description ? h("p", { id: "tournament-description", class: "tournament-description", translate: "no" }, t.description) : null,
			t.end_detail ? h("p", { class: "muted" }, t.end_detail) : null,
			t.status === "running" ? h("p", { id: "tournament-round" }, tr("tourney.round_of", { n: t.current_round, total: t.total_rounds })) : null,
			t.status === "running" && t.next_round_at ? h("p", { id: "tournament-break" }, tr("tourney.next_round", { when: when(t.next_round_at) })) : null,
			t.you.game && t.status === "running" ? h("p", { class: "notice attention", id: "tournament-your-game" }, ...tParts("tourney.your_game", { link: h("a", { href: `/game/${t.you.game}` }, tr("tourney.go_to_it")) })) : null,
			t.real_time && t.status === "registration" ? h("p", { class: "muted small", id: "tournament-check-in-note" }, tr("tourney.check_in_note", { n: Math.round(t.check_in_seconds / 60) })) : null,
			t.status === "check_in" && t.you.entered ? h("p", { class: t.you.checked_in ? "muted small" : "notice attention", id: "tournament-check-in" }, t.you.checked_in ? tr("tourney.checked_in_at", { when: when(t.starts_at) }) : tr("tourney.check_in_before", { when: when(t.starts_at) })) : null,
			t.status === "registration" && !t.you.entered ? h("p", { class: "muted small" }, me.kind === "account" ? tr("tourney.mail_note") : tr("tourney.sign_in")) : null,
			h("p", { class: "muted small" }, tr("tourney.fair_play")),
			h("div", { class: "row", id: "tournament-actions" }, actions(t)),
			status,
			h("section", {}, h("h2", {}, t.status === "finished" ? tr("tourney.final") : t.status === "draft" || t.status === "registration" || t.status === "check_in" ? tr("tourney.players_n", { n: t.players }) : tr("tourney.standings")), standingsTable(t)),
			t.schedule.length ? h("section", {}, h("h2", {}, tr("tourney.games")), schedule(t)) : null,
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
		if (t && ["running", "registration", "check_in"].includes(t.status)) setInterval(() => load().catch(() => {}), t.real_time ? REFRESH_LIVE_MS : REFRESH_MS);
	} catch (err) {
		clear(host);
		host.append(h("p", { class: "error", id: "tournament-error" }, err.message));
	}
}
