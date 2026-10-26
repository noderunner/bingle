// The lobby of a game that has not started: the invite link, who sits where,
// bots for the seats nobody takes, and — for the host — the Start button.

import { h } from "./util.js";
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
			h("h2", {}, info.name),
			h("span", { class: "badge" }, `Room ${ctx.code}`),
			h("span", { class: "badge waiting" }, full ? (startsByItself ? "Starting…" : "Ready to start") : `Waiting for ${open.length} more`)
		)
	);

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
	const table = h("ul", { class: "lobby-seats", id: "lobby-seats" });
	for (const sd of seats) {
		const si = bySeat.get(sd.seat) || {};
		const taken = !!si.name;
		const mine = me.seat === sd.seat;
		const li = h("li", { class: "lobby-seat" + (taken ? " taken" : " open") + (mine ? " mine" : ""), "data-seat": sd.seat });
		li.append(h("span", { class: `swatch color-${cssColor(sd.color)}` }));
		// The cult of an open seat can be changed by anyone at the table.
		const cults = ctx.cults || [];
		if (!taken && me.seat && cults.length && cults.some((c) => c.label === sd.label)) {
			const select = h("select", { class: "lobby-cult-select", id: `cult-${sd.seat}`, "data-seat": sd.seat, "aria-label": `Faction of seat ${sd.seat}`, disabled: ctx.busy });
			const current = cults.find((c) => c.label === sd.label);
			for (const c of cults) {
				// A cult a player or bot already plays cannot be taken from them.
				const holder = seats.find((o) => o.label === c.label && (bySeat.get(o.seat) || {}).name);
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
		} else {
			who.append(h("span", { class: "muted" }, "open — waiting for a player"));
		}
		li.append(who);
		const acts = h("span", { class: "lobby-actions" });
		if (!taken) {
			if (me.seat && !ctx.busy) {
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
	if (!me.seat && open.length) {
		const row = h("div", { class: "lobby-join", id: "lobby-join" });
		const name = h("input", { type: "text", id: "lobby-name", placeholder: "Your name", value: ctx.lastName || "", maxlength: "32", autocomplete: "nickname", "aria-label": "Your name" });
		const seatSel = h("select", { id: "lobby-seat", "aria-label": "Faction to play" });
		for (const sd of open) seatSel.append(h("option", { value: String(sd.seat) }, sd.label));
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
	if (iAmHost && !startsByItself) {
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
	card.append(h("p", { class: "small muted lobby-settings" }, settings.join(" · ")));
	return card;
}
