// The lobby of a game that has not started: the invite link, who sits where,
// bots for the seats nobody takes, and — for the host — the Start button.

import { h } from "./util.js";
import { api, gameTitle } from "./api.js";
import { setupIcon } from "./cw-setup.js";
import { cssColor } from "./render.js";

/** Kinds of bots that can be added, with their labels (from GET /api/bots). */
function botOptions(bots, id) {
	const select = h("select", { class: "lobby-bot-select", id, "aria-label": "Kind of bot" });
	for (const b of bots) select.append(h("option", { value: b.id, title: b.description || null }, b.label));
	const pref = bots.find((b) => b.id === "tactical") || bots[0];
	if (pref) select.value = pref.id;
	return select;
}

/**
 * ctx: {code, info, game, me: {name, seat}, bots, cults, inviteURL, lastName, busy,
 *       actions: {take, moveTo, setCult, addBot, remove, leave, start, fillBots, copy}}
 * cults lists the cults an open seat can be given ({id, label, color, text}), when the game lets the table change them.
 */
/** A time, in the viewer's own zone. */
const localTime = (iso) => new Date(iso).toLocaleString([], { weekday: "short", month: "short", day: "numeric", hour: "numeric", minute: "2-digit", timeZoneName: "short" });

/** A row of times to offer, for its creator to fill in. */
function newTimesForm(ctx) {
	const list = h("div", { class: "stack" });
	const add = () => list.append(h("input", { type: "datetime-local", class: "schedule-new-slot", "aria-label": "A new time to offer" }));
	add();
	return h(
		"div",
		{ class: "schedule-new", id: "schedule-new" },
		list,
		h(
			"div",
			{ class: "row" },
			h("button", { type: "button", class: "ghost small", id: "schedule-new-add", onclick: add }, "Add a time"),
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
				"Offer these times"
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
		box.append(h("p", { class: "schedule-when", id: "schedule-when" }, h("strong", {}, "Scheduled for "), localTime(sc.start_at)));
		if (sc.status === "scheduled") {
			box.append(h("p", { class: "small muted" }, "The game does not start by itself: when the time comes, any player at the table can press Start game."));
			const calendar = h("a", { class: "button small", id: "schedule-ics", href: `/api/games/${encodeURIComponent(ctx.code)}/schedule.ics`, download: "bingle-game.ics" }, "Add to calendar");
			box.append(calendar);
		}
	}
	if (sc.status === "polling" || sc.status === "no_agreement") {
		box.append(h("h3", {}, sc.status === "no_agreement" ? "No time suits everyone" : "Which times can you make?"));
		if (sc.status === "no_agreement" && sc.decide_by) {
			box.append(h("p", { class: "small", id: "schedule-no-agreement" }, sc.mine ? `Offer new times or cancel. Otherwise the game closes on ${localTime(sc.decide_by)}.` : `The game closes on ${localTime(sc.decide_by)} unless new times are offered.`));
		}
		const total = sc.people.filter((p) => p.status === "pending" || p.status === "accepted").length + 1;
		const rows = h("ul", { class: "schedule-slots", id: "schedule-slot-list" });
		for (const slot of sc.slots || []) {
			const tick = h("input", { type: "checkbox", "data-slot": slot.id, checked: slot.mine, disabled: !sc.can_vote || slot.past || ctx.busy, "aria-label": localTime(slot.at), onchange: (ev) => ctx.actions.vote(slot.id, ev.target.checked) });
			rows.append(h("li", { class: slot.past ? "past" : null }, h("label", { class: "check" }, tick, h("span", {}, localTime(slot.at))), h("span", { class: "small muted" }, `${slot.count} of ${total}`)));
		}
		box.append(rows);
		if (sc.mine && sc.status === "no_agreement") box.append(newTimesForm(ctx));
		if (sc.mine && sc.people.some((p) => p.status === "pending" || (p.status === "accepted" && !p.answered))) {
			box.append(h("button", { type: "button", class: "small", id: "schedule-decide", title: "Leave out anybody who has not answered", onclick: () => ctx.actions.decide() }, "Schedule with those who've answered"));
		}
	}
	if (sc.people.length) {
		const said = { pending: "invited", accepted: "accepted", declined: "declined", dropped: "left out" };
		box.append(
			h(
				"p",
				{ class: "small muted", id: "schedule-people" },
				sc.people.map((p) => `@${p.handle}: ${(sc.status === "polling" || sc.status === "no_agreement") && p.answered ? "answered" : said[p.status] || p.status}`).join(" · ")
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
						if (window.confirm("Cancel this game? Everybody invited is told.")) ctx.actions.cancelSchedule();
					},
				},
				"Cancel the game"
			)
		);
	}
	return box;
}

const catalogs = new Map(); // by family: the setup catalog, once read

/** What this table was set up with, as pictures: its map, then the factions and all that was added. */
function tablePool(info) {
	const family = info.family || info.id;
	const box = h("div", { class: "card lobby-pool", id: "lobby-pool", "aria-label": "What this table plays with" });
	const paint = (catalog) => {
		const params = info.params || {};
		const has = (key, value) => (params[key] || []).includes(value);
		const pool = params.faction_pool || [];
		const shown = [];
		const m = /\(([^)]*)\)/.exec(info.name || "");
		shown.push({ kind: "map", id: String(info.id).split("~")[0], name: m ? m[1] : info.name, art: params.art || "" });
		for (const tab of catalog.tabs) {
			for (const item of tab.items) {
				const on =
					tab.id === "factions"
						? pool.length
							? pool.includes(item.value)
							: info.seats.some((sd) => sd.label === item.name)
						: item.option === "high-priests"
							? !!params.high_priests
							: item.option === "neutral-books"
								? has("neutral_books", item.value)
								: tab.id === "neutrals"
									? has("neutral_units", item.value)
									: tab.id === "independents" && has("independents", item.id);
				if (on) shown.push(item);
			}
		}
		for (const item of shown) {
			box.append(h("span", { class: "lobby-pool-item", title: item.name, "data-kind": item.kind, "data-item": item.id, role: "img", "aria-label": item.name }, setupIcon(item, 34, family, info.pieces)));
		}
	};
	if (catalogs.has(family)) {
		paint(catalogs.get(family));
	} else {
		api.setupCatalog(info.id)
			.then((catalog) => {
				catalogs.set(family, catalog);
				paint(catalog);
			})
			.catch(() => {});
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
			h("span", { class: "badge" }, `Room ${ctx.code}`),
			h("span", { class: "badge waiting" }, full ? (startsByItself ? "Starting…" : "Ready to start") : `Waiting for ${open.length} more`)
		)
	);

	if (info.setup_catalog) card.append(tablePool(info));
	if (game.schedule) card.append(scheduleBox(ctx));

	// The invitation.
	const link = ctx.inviteURL;
	const field = h("input", { type: "text", readonly: true, value: link, id: "invite-link", "aria-label": "Invite link", onclick: (ev) => ev.target.select() });
	const invite = h("div", { class: "lobby-invite" });
	invite.append(
		h("label", { for: "invite-link", class: "small muted" }, "Send this link to the players you want at the table:"),
		h(
			"div",
			{ class: "share-row" },
			field,
			h("button", { id: "copy-invite", class: "primary", onclick: () => ctx.actions.copy(field) }, "Copy link"),
			typeof navigator !== "undefined" && navigator.share
				? h(
						"button",
						{ id: "share-invite", onclick: () => navigator.share({ title: `${info.name} on Bingle`, text: `Join my game of ${info.name}`, url: link }).catch(() => {}) },
						"Share…"
				  )
				: null
		)
	);
	card.append(invite);

	// The table.
	const pooled = ((info.params && info.params.faction_pool) || []).length;
	if (pooled) card.append(h("p", { class: "small muted", id: "lobby-pool" }, `This table has ${pooled} factions for ${seats.length} seats: choose yours from the list in your row. If you don't, you play the one your seat has.`));
	const table = h("ul", { class: "lobby-seats", id: "lobby-seats" });
	for (const sd of seats) {
		const si = bySeat.get(sd.seat) || {};
		const taken = !!si.name;
		const mine = me.seat === sd.seat;
		const kept = (game.reservations || []).find((r) => r.seat === sd.seat);
		const li = h("li", { class: "lobby-seat" + (taken ? " taken" : " open") + (mine ? " mine" : ""), "data-seat": sd.seat });
		li.append(ctx.seatGlyph(sd.seat) || h("span", { class: `swatch color-${cssColor(sd.color)}` }));
		// The cult of an open seat can be changed by anyone at the table, and at a table
		// set up with a pool of factions a player can change their own.
		const cults = ctx.cults || [];
		const pool = (info.params && info.params.faction_pool) || [];
		if ((!taken || (mine && pool.length)) && me.seat && cults.length && cults.some((c) => c.label === sd.label)) {
			const select = h("select", { class: "lobby-cult-select", id: `cult-${sd.seat}`, "data-seat": sd.seat, "aria-label": `Faction of seat ${sd.seat}`, disabled: ctx.busy });
			const current = cults.find((c) => c.label === sd.label);
			for (const c of cults) {
				// A cult a player or bot already plays cannot be taken from them.
				const holder = seats.find((o) => o.seat !== sd.seat && o.label === c.label && (bySeat.get(o.seat) || {}).name);
				const opt = h("option", { value: c.id, title: c.text || null, disabled: !!holder }, holder ? `${c.label} (taken)` : c.label);
				select.append(opt);
			}
			select.value = current.id;
			select.addEventListener("change", () => ctx.actions.setCult(sd.seat, select.value));
			li.append(select);
		} else {
			li.append(h("span", { class: "lobby-faction" }, sd.label));
		}
		const who = h("span", { class: "lobby-who" });
		if (taken) {
			// A bot is named after its faction: say only its kind.
			const shown = si.bot && si.name.startsWith(sd.label) ? si.name.slice(sd.label.length).replace(/^[\s(]+|[)\s]+$/g, "") + " bot" : si.name;
			who.append(h("strong", { class: "lobby-name" }, shown));
			if (si.bot) who.append(h("span", { class: "badge bot" }, "bot"));
			if (mine) who.append(h("span", { class: "badge" }, "you"));
			if (sd.seat === hostSeat && !si.bot) who.append(h("span", { class: "badge host", title: "Created the game: starts it" }, "host"));
		} else if (kept) {
			who.append(h("span", { class: "badge reserved", "data-reserved": kept.handle }, kept.yours ? "Reserved for you" : `Reserved for @${kept.handle}`));
		} else {
			who.append(h("span", { class: "muted" }, "open — waiting for a player"));
		}
		li.append(who);
		const acts = h("span", { class: "lobby-actions" });
		if (!taken) {
			if (kept && iAmHost) {
				acts.append(h("button", { class: "small ghost", "data-act": "release", disabled: ctx.busy, onclick: () => ctx.actions.release(sd.seat), title: "Let anybody sit here" }, "Release"));
			}
			if (me.seat && !ctx.busy && (!kept || kept.yours)) {
				acts.append(h("button", { class: "small", "data-act": "move", onclick: () => ctx.actions.moveTo(sd.seat), title: "Play this faction instead" }, "Sit here"));
			}
			if (me.seat && (iAmHost || startsByItself) && bots.length) {
				const sel = botOptions(bots, `bot-kind-${sd.seat}`);
				acts.append(
					sel,
					h("button", { class: "small", "data-act": "add-bot", disabled: ctx.busy, onclick: () => ctx.actions.addBot(sel.value, sd.seat) }, "Add bot")
				);
			}
		} else if (iAmHost && !mine) {
			acts.append(h("button", { class: "small ghost", "data-act": "remove", disabled: ctx.busy, onclick: () => ctx.actions.remove(sd.seat) }, si.bot ? "Remove bot" : "Remove"));
		}
		li.append(acts);
		table.append(li);
	}
	card.append(table);

	// A visitor who has no seat yet.
	const free = open.filter((sd) => !(game.reservations || []).some((r) => r.seat === sd.seat && !r.yours));
	if (!me.seat && open.length && !free.length) card.append(h("p", { class: "small muted", id: "lobby-all-reserved" }, "The seats here are reserved for friends of the host. You can watch this game."));
	if (!me.seat && free.length) {
		const row = h("div", { class: "lobby-join", id: "lobby-join" });
		const name = h("input", { type: "text", id: "lobby-name", placeholder: "Your name", value: ctx.lastName || "", maxlength: "32", autocomplete: "nickname", "aria-label": "Your name" });
		const seatSel = h("select", { id: "lobby-seat", "aria-label": "Faction to play" });
		// A seat kept for this person comes first.
		const yours = (sd) => (game.reservations || []).some((r) => r.seat === sd.seat && r.yours);
		for (const sd of [...free].sort((a, b) => Number(yours(b)) - Number(yours(a)))) seatSel.append(h("option", { value: String(sd.seat) }, yours(sd) ? `${sd.label} (reserved for you)` : sd.label));
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
		row.append(name, seatSel, h("button", { class: "primary", id: "lobby-take", onclick: join }, "Take this seat"));
		card.append(row);
	}

	// What the host can do.
	const foot = h("div", { class: "lobby-foot" });
	// From its time on, any player at the table can start a scheduled game.
	const timeHasCome = !!game.schedule && game.schedule.status === "scheduled" && Date.parse(game.schedule.start_at) <= Date.now();
	if (me.seat && !iAmHost && timeHasCome) {
		foot.append(h("span", { class: "grow" }), h("button", { id: "start-game", class: "primary", disabled: !full || ctx.busy, title: full ? "Begin the game" : "Every seat needs a player", onclick: () => ctx.actions.start() }, "Start game"));
	} else if (iAmHost && !startsByItself) {
		if (!full && bots.length) {
			const sel = botOptions(bots, "fill-kind");
			foot.append(
				h("label", { class: "small muted", for: "fill-kind" }, "Fill the open seats with"),
				sel,
				h("button", { id: "fill-bots", disabled: ctx.busy, onclick: () => ctx.actions.fillBots(sel.value) }, "Bots")
			);
		}
		foot.append(
			h("span", { class: "grow" }),
			h(
				"button",
				{ id: "start-game", class: "primary", disabled: !full || ctx.busy, title: full ? "Begin the game" : "Every seat needs a player or a bot first", onclick: () => ctx.actions.start() },
				"Start game"
			)
		);
	} else if (me.seat && !startsByItself) {
		foot.append(h("span", { class: "muted" }, full ? "Waiting for the host to start the game…" : "Waiting for the table to fill up…"));
	} else if (startsByItself && !full) {
		foot.append(h("span", { class: "muted" }, "The game starts as soon as every seat is taken."));
	}
	if (me.seat) {
		foot.append(h("span", { class: "grow" }), h("button", { id: "leave-seat", class: "ghost", disabled: ctx.busy, onclick: () => ctx.actions.leave() }, "Leave the table"));
	}
	if (foot.childNodes.length) card.append(foot);

	const settings = [];
	if (game.move_time_seconds) settings.push(`${Math.round(game.move_time_seconds / 60) || 1} min per move`);
	settings.push(`${seats.length} players`);
	if (game.invite_only) settings.push("Invite only: not on any list");
	card.append(h("p", { class: "small muted lobby-settings" }, settings.join(" · ")));
	return card;
}
