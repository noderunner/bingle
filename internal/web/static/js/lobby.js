// The lobby of a game that has not started: the invite link, who sits where,
// bots for the seats nobody takes, and — for the host — the Start button.

import { h, brandName } from "./util.js";
import { api, gameTitle } from "./api.js";
import { cssColor } from "./render.js";
import { t, tParts, lang } from "./i18n.js";

/** Kinds of bots that can be added, with their labels (from GET /api/bots). */
function botOptions(bots, id) {
	const select = h("select", { class: "lobby-bot-select", id, "aria-label": t("lobby.bot_kind") });
	for (const b of bots) select.append(h("option", { value: b.id, title: b.description || null }, b.label));
	const pref = bots.find((b) => b.id === "tactical") || bots[0];
	if (pref) select.value = pref.id;
	return select;
}

/**
 * ctx: {code, info, game, me: {name, seat}, bots, factions, inviteURL, lastName, busy,
 *       actions: {take, moveTo, setFaction, addBot, remove, leave, start, fillBots, copy}}
 * factions lists the factions an open seat can be given ({id, label, color, text}), when the game lets the table change them.
 */
/** A time, in the viewer's own zone. */
const localTime = (iso) => new Date(iso).toLocaleString(lang, { weekday: "short", month: "short", day: "numeric", hour: "numeric", minute: "2-digit", timeZoneName: "short" });

/** A row of times to offer, for its creator to fill in. */
function newTimesForm(ctx) {
	const list = h("div", { class: "stack" });
	const add = () => list.append(h("input", { type: "datetime-local", class: "schedule-new-slot", "aria-label": t("lobby.new_time") }));
	add();
	return h(
		"div",
		{ class: "schedule-new", id: "schedule-new" },
		list,
		h(
			"div",
			{ class: "row" },
			h("button", { type: "button", class: "ghost small", id: "schedule-new-add", onclick: add }, t("setup.add_time")),
			h(
				"button",
				{
					type: "button",
					class: "primary small",
					id: "schedule-new-send",
					onclick: () => {
						const slots = [...list.querySelectorAll("input")].map((i) => i.value).filter(Boolean).map((v) => new Date(v).toISOString());
						if (slots.length) ctx.actions.offerTimes(slots);
					},
				},
				t("lobby.offer_times")
			)
		)
	);
}

/** The game's time, or the times being answered, in the lobby. */
function scheduleBox(ctx) {
	const { game } = ctx;
	const sc = game.schedule;
	const box = h("div", { class: "lobby-schedule", id: "lobby-schedule", "data-status": sc.status });
	if (sc.status === "scheduled" || sc.status === "started") {
		box.append(h("p", { class: "schedule-when", id: "schedule-when" }, ...tParts("lobby.scheduled_for", { when: h("strong", {}, localTime(sc.start_at)) })));
		if (sc.status === "scheduled") {
			box.append(h("p", { class: "small muted" }, t("lobby.no_auto_start")));
			const calendar = h("a", { class: "button small", id: "schedule-ics", href: `/api/games/${encodeURIComponent(ctx.code)}/schedule.ics`, download: "bingle-game.ics" }, t("lobby.add_calendar"));
			box.append(calendar);
		}
	}
	if (sc.status === "polling" || sc.status === "no_agreement") {
		box.append(h("h3", {}, sc.status === "no_agreement" ? t("lobby.muster_none") : t("lobby.muster_poll")));
		if (sc.status === "no_agreement" && sc.decide_by) {
			box.append(h("p", { class: "small", id: "schedule-no-agreement" }, sc.mine ? t("lobby.offer_or_cancel", { when: localTime(sc.decide_by) }) : t("lobby.closes_on", { when: localTime(sc.decide_by) })));
		}
		const total = sc.people.filter((p) => p.status === "pending" || p.status === "accepted").length + 1;
		const rows = h("ul", { class: "schedule-slots", id: "schedule-slot-list" });
		for (const slot of sc.slots || []) {
			const tick = h("input", { type: "checkbox", "data-slot": slot.id, checked: slot.mine, disabled: !sc.can_vote || slot.past || ctx.busy, "aria-label": localTime(slot.at), onchange: (ev) => ctx.actions.vote(slot.id, ev.target.checked) });
			rows.append(h("li", { class: slot.past ? "past" : null }, h("label", { class: "check" }, tick, h("span", {}, localTime(slot.at))), h("span", { class: "small muted" }, t("lobby.n_of", { n: slot.count, total }))));
		}
		box.append(rows);
		if (sc.mine && sc.status === "no_agreement") box.append(newTimesForm(ctx));
		if (sc.mine && sc.people.some((p) => p.status === "pending" || (p.status === "accepted" && !p.answered))) {
			box.append(h("button", { type: "button", class: "small", id: "schedule-decide", title: t("lobby.decide_title"), onclick: () => ctx.actions.decide() }, t("lobby.decide")));
		}
	}
	if (sc.people.length) {
		const said = { pending: "lobby.person_pending", accepted: "lobby.person_accepted", declined: "lobby.person_declined", dropped: "lobby.person_dropped" };
		box.append(
			h(
				"p",
				{ class: "small muted", id: "schedule-people" },
				sc.people.map((p) => ((sc.status === "polling" || sc.status === "no_agreement") && p.answered ? t("lobby.answered", { handle: "@" + p.handle }) : t("lobby.person", { handle: "@" + p.handle, state: said[p.status] ? t(said[p.status]) : p.status }))).join(" · ")
			)
		);
	}
	if (sc.mine && sc.status !== "closed" && sc.status !== "started") {
		box.append(
			h(
				"button",
				{
					type: "button",
					class: "ghost small",
					id: "schedule-cancel",
					onclick: () => {
						if (window.confirm(t("lobby.cancel_confirm"))) ctx.actions.cancelSchedule();
					},
				},
				t("lobby.cancel")
			)
		);
	}
	return box;
}

export function lobbyCard(ctx) {
	const { info, game, me, bots } = ctx;
	const seats = info.seats;
	const bySeat = new Map((game.seats || []).map((s) => [s.seat, s]));
	const hostSeat = game.host_seat || 0;
	const iAmHost = !!me.seat && me.seat === hostSeat;
	const open = seats.filter((sd) => !(bySeat.get(sd.seat) || {}).name);
	const full = open.length === 0;
	const startsByItself = !game.lobby;

	const card = h("div", { class: "card lobby", id: "lobby" });
	card.append(
		h(
			"div",
			{ class: "lobby-head" },
			// A table's name lists every choice made for it: those are shown below.
			h("h2", {}, game.list_title || gameTitle(info)),
			h("span", { class: "badge" }, t("lobby.room", { code: ctx.code })),
			h("span", { class: "badge waiting" }, full ? (startsByItself ? t("lobby.starting") : t("lobby.ready")) : t("lobby.waiting_more", { n: open.length }))
		)
	);

	// What a table was set up with is the game's to draw (its page script's
	// lobbyPool: Cthulhu Wars' map, cults and expansions).
	if (info.setup_catalog && info.page && info.page.lobbyPool) card.append(info.page.lobbyPool(info));
	if (game.schedule) card.append(scheduleBox(ctx));

	// The invitation.
	const link = ctx.inviteURL;
	const field = h("input", { type: "text", readonly: true, value: link, id: "invite-link", "aria-label": t("lobby.invite_link"), onclick: (ev) => ev.target.select() });
	const invite = h("div", { class: "lobby-invite" });
	invite.append(
		h("label", { for: "invite-link", class: "small muted" }, t("lobby.send_link")),
		h(
			"div",
			{ class: "share-row" },
			field,
			h("button", { id: "copy-invite", class: "primary", onclick: () => ctx.actions.copy(field) }, t("lobby.copy_link")),
			typeof navigator !== "undefined" && navigator.share
				? h(
						"button",
						{ id: "share-invite", onclick: () => navigator.share({ title: t("lobby.share_title", { game: info.name, brand: brandName() }), text: t("lobby.share_text", { game: info.name }), url: link }).catch(() => {}) },
						t("lobby.share")
				  )
				: null
		)
	);
	card.append(invite);

	// The table.
	const pooled = ((info.params && info.params.faction_pool) || []).length;
	if (pooled) card.append(h("p", { class: "small muted", id: "lobby-pool" }, t("lobby.pool", { factions: pooled, seats: seats.length })));
	const table = h("ul", { class: "lobby-seats", id: "lobby-seats" });
	for (const sd of seats) {
		const si = bySeat.get(sd.seat) || {};
		const taken = !!si.name;
		const mine = me.seat === sd.seat;
		const kept = (game.reservations || []).find((r) => r.seat === sd.seat);
		const li = h("li", { class: "lobby-seat" + (taken ? " taken" : " open") + (mine ? " mine" : ""), "data-seat": sd.seat });
		li.append(ctx.seatGlyph(sd.seat) || h("span", { class: `swatch color-${cssColor(sd.color)}` }));
		// The faction of an open seat can be changed by anyone at the table, and at a table
		// set up with a pool of factions a player can change their own.
		const factions = ctx.factions || [];
		const pool = (info.params && info.params.faction_pool) || [];
		if ((!taken || (mine && pool.length)) && me.seat && factions.length && factions.some((c) => c.label === sd.label)) {
			const select = h("select", { class: "lobby-faction-select", id: `faction-${sd.seat}`, "data-seat": sd.seat, "aria-label": t("lobby.faction_of", { n: sd.seat }), disabled: ctx.busy });
			const current = factions.find((c) => c.label === sd.label);
			for (const c of factions) {
				// A faction a player or bot already plays cannot be taken from them.
				const holder = seats.find((o) => o.seat !== sd.seat && o.label === c.label && (bySeat.get(o.seat) || {}).name);
				const opt = h("option", { value: c.id, title: c.text || null, disabled: !!holder }, holder ? t("lobby.taken", { label: c.label }) : c.label);
				select.append(opt);
			}
			select.value = current.id;
			select.addEventListener("change", () => ctx.actions.setFaction(sd.seat, select.value));
			li.append(select);
		} else {
			li.append(h("span", { class: "lobby-faction" }, sd.label));
		}
		const who = h("span", { class: "lobby-who" });
		if (taken) {
			// A bot is named after its faction: say only its kind.
			const shown = si.bot && si.name.startsWith(sd.label) ? t("setup.bot", { name: si.name.slice(sd.label.length).replace(/^[\s(]+|[)\s]+$/g, "") }) : si.name;
			who.append(h("strong", { class: "lobby-name", translate: "no" }, shown));
			if (si.bot) who.append(h("span", { class: "badge bot" }, t("lobby.bot")));
			if (mine) who.append(h("span", { class: "badge" }, t("lobby.you")));
			if (sd.seat === hostSeat && !si.bot) who.append(h("span", { class: "badge host", title: t("lobby.host_title") }, t("lobby.host")));
		} else if (kept) {
			who.append(h("span", { class: "badge reserved", "data-reserved": kept.handle }, kept.yours ? t("lobby.reserved_you") : t("lobby.reserved_for", { handle: kept.handle })));
		} else {
			who.append(h("span", { class: "muted" }, t("lobby.open_seat")));
		}
		li.append(who);
		const acts = h("span", { class: "lobby-actions" });
		if (!taken) {
			if (kept && iAmHost) {
				acts.append(h("button", { class: "small ghost", "data-act": "release", disabled: ctx.busy, onclick: () => ctx.actions.release(sd.seat), title: t("lobby.release_title") }, t("lobby.release")));
			}
			if (me.seat && !ctx.busy && (!kept || kept.yours)) {
				acts.append(h("button", { class: "small", "data-act": "move", onclick: () => ctx.actions.moveTo(sd.seat), title: t("lobby.sit_title") }, t("lobby.sit")));
			}
			if (me.seat && (iAmHost || startsByItself) && bots.length) {
				const sel = botOptions(bots, `bot-kind-${sd.seat}`);
				acts.append(
					sel,
					h("button", { class: "small", "data-act": "add-bot", disabled: ctx.busy, onclick: () => ctx.actions.addBot(sel.value, sd.seat) }, t("lobby.add_bot"))
				);
			}
		} else if (iAmHost && !mine) {
			acts.append(h("button", { class: "small ghost", "data-act": "remove", disabled: ctx.busy, onclick: () => ctx.actions.remove(sd.seat) }, si.bot ? t("lobby.remove_bot") : t("lobby.remove")));
		}
		li.append(acts);
		table.append(li);
	}
	card.append(table);

	// A visitor who has no seat yet.
	const free = open.filter((sd) => !(game.reservations || []).some((r) => r.seat === sd.seat && !r.yours));
	if (!me.seat && open.length && !free.length) card.append(h("p", { class: "small muted", id: "lobby-all-reserved" }, t("lobby.all_reserved")));
	if (!me.seat && free.length) {
		const row = h("div", { class: "lobby-join", id: "lobby-join" });
		const name = h("input", { type: "text", id: "lobby-name", placeholder: t("chat.your_name"), value: ctx.lastName || "", maxlength: "32", autocomplete: "nickname", "aria-label": t("chat.your_name") });
		const seatSel = h("select", { id: "lobby-seat", "aria-label": t("lobby.faction_to_play") });
		// A seat kept for this person comes first.
		const yours = (sd) => (game.reservations || []).some((r) => r.seat === sd.seat && r.yours);
		for (const sd of [...free].sort((a, b) => Number(yours(b)) - Number(yours(a)))) seatSel.append(h("option", { value: String(sd.seat) }, yours(sd) ? t("lobby.reserved_label", { label: sd.label }) : sd.label));
		const join = () => {
			const n = name.value.trim();
			if (!n) {
				name.focus();
				return;
			}
			ctx.actions.take(Number(seatSel.value), n);
		};
		name.addEventListener("keydown", (ev) => {
			if (ev.key === "Enter") join();
		});
		row.append(name, seatSel, h("button", { class: "primary", id: "lobby-take", onclick: join }, t("lobby.take")));
		card.append(row);
	}

	// What the host can do.
	const foot = h("div", { class: "lobby-foot" });
	// From its time on, any player at the table can start a scheduled game.
	const timeHasCome = !!game.schedule && game.schedule.status === "scheduled" && Date.parse(game.schedule.start_at) <= Date.now();
	if (me.seat && !iAmHost && timeHasCome) {
		foot.append(h("span", { class: "grow" }), h("button", { id: "start-game", class: "primary", disabled: !full || ctx.busy, title: full ? t("lobby.begin") : t("lobby.need_player"), onclick: () => ctx.actions.start() }, t("lobby.start")));
	} else if (iAmHost && !startsByItself) {
		if (!full && bots.length) {
			const sel = botOptions(bots, "fill-kind");
			foot.append(
				h("label", { class: "small muted", for: "fill-kind" }, t("lobby.fill_with")),
				sel,
				h("button", { id: "fill-bots", disabled: ctx.busy, onclick: () => ctx.actions.fillBots(sel.value) }, t("lobby.bots"))
			);
		}
		foot.append(
			h("span", { class: "grow" }),
			h(
				"button",
				{ id: "start-game", class: "primary", disabled: !full || ctx.busy, title: full ? t("lobby.begin") : t("lobby.need_player_bot"), onclick: () => ctx.actions.start() },
				t("lobby.start")
			)
		);
	} else if (me.seat && !startsByItself) {
		foot.append(h("span", { class: "muted" }, full ? t("lobby.wait_host") : t("lobby.wait_fill")));
	} else if (startsByItself && !full) {
		foot.append(h("span", { class: "muted" }, t("lobby.auto_start")));
	}
	if (me.seat) {
		foot.append(h("span", { class: "grow" }), h("button", { id: "leave-seat", class: "ghost", disabled: ctx.busy, onclick: () => ctx.actions.leave() }, t("lobby.leave")));
	}
	if (foot.childNodes.length) card.append(foot);

	const settings = [];
	if (game.move_time_seconds) settings.push(t("lobby.min_per_move", { n: Math.round(game.move_time_seconds / 60) || 1 }));
	settings.push(t("games.players", { n: seats.length }));
	if (game.invite_only) settings.push(t("lobby.invite_only"));
	card.append(h("p", { class: "small muted lobby-settings" }, settings.join(" · ")));
	return card;
}
