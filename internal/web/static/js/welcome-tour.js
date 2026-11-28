// The welcome tour (ADR 0139): the main page's floating card (js/tour-card.js)
// with a slide for each feature worth knowing about, and a picture drawn by the
// page's own code from sample data, so that it looks like the page in either
// theme and changes when the page does. It is offered once to each guest and
// account (home.js asks offerTour), and the help window takes it again (openTour).

import { h, store, brandName } from "./util.js";
import { api, whoAmI } from "./api.js";
import { renderBoard } from "./render.js";
import { rateBars } from "./charts.js";
import { gameRow } from "./mygames.js";
import { badgeItem } from "./badges.js";
import { guestKeySection } from "./guest-key.js";
import { gamesIcon, profileIcon, rulesIcon } from "./ui-icons.js";
import { t, tParts, lang } from "./i18n.js";
import { openCard } from "./tour-card.js";

// This browser has skipped or finished the tour: kept beside the server's mark
// so that a browser signed out, which is nobody again until its next game, is
// not offered the tour once more, nor is anybody else who uses it.
export const TOUR_SEEN_KEY = "bingle.tour.seen";

/**
 * Whether the main page offers the tour to me, the answer of /auth/me: only
 * when neither the guest or account nor this browser has seen it. A newcomer
 * is offered it unless the server is for its members only; the page's first
 * request makes its guest, and Skip or Done is such a request.
 */
export function shouldOfferTour(me) {
	if (me.tour_seen || store.get(TOUR_SEEN_KEY, false)) return false;
	if (me.kind === "account" || me.kind === "guest") return true;
	return me.mode !== "closed";
}

/** Offers the tour if this browser's player has not seen it. */
export async function offerTour() {
	const me = await whoAmI();
	if (shouldOfferTour(me)) openTour(me);
}

// ---- the slides ---------------------------------------------------------------

// What an account adds, by capability, for the guest's slide.
const ACCOUNT_ADDS = [
	["dm", "tour.adds_dm"],
	["voice_direct", "tour.adds_voice_direct"],
	["chat_global", "tour.adds_chat_global"],
	["notify", "tour.adds_notify"],
	["run_batch", "tour.adds_run_batch"],
	["enter_tournament", "tour.adds_enter_tournament"],
];

/** The slides for me, in order: those whose feature this server offers. */
export function slidesFor(me) {
	const brand = brandName();
	const caps = me.caps || [];
	const accountCaps = me.account_caps || [];
	const isAccount = me.kind === "account";
	const accounts = isAccount || !!me.accounts;
	// A feature this player has, or would have with an account (then the slide says so).
	const offer = (cap) => (caps.includes(cap) ? "yes" : accountCaps.includes(cap) ? "account" : "");
	const slides = [
		{
			id: "welcome",
			title: t("tour.welcome_title", { brand }),
			text: t("tour.welcome_text"),
			alt: t("tour.welcome_alt"),
			draw: drawGallery,
		},
		{
			id: "opponents",
			title: t("tour.opponents_title"),
			text: t("tour.opponents_text"),
			alt: t("tour.opponents_alt"),
			draw: drawOpponents,
		},
		{
			id: "learn",
			title: t("tour.learn_title"),
			text: t("tour.learn_text"),
			alt: t("tour.learn_alt"),
			draw: drawRules,
		},
		{
			id: "table",
			title: t("tour.table_title"),
			text: t("tour.table_text"),
			alt: t("tour.table_alt"),
			draw: drawTable,
		},
		{
			id: "puzzles",
			title: t("tour.puzzles_title"),
			text: t("tour.puzzles_text"),
			alt: t("tour.puzzles_alt"),
			draw: drawPuzzle,
		},
		{
			id: "analysis",
			title: t("tour.analysis_title"),
			when: offer("run_batch"),
			text: t("tour.analysis_text"),
			alt: t("tour.analysis_alt"),
			draw: drawAnalysis,
		},
		{
			id: "games",
			title: t("tour.games_title"),
			text: t("tour.games_text"),
			alt: t("tour.games_alt"),
			draw: drawMyGames,
		},
		{
			id: "compete",
			title: t("tour.compete_title"),
			when: accounts || caps.includes("enter_tournament") ? (isAccount ? "yes" : "account") : "",
			text: t("tour.compete_text"),
			alt: t("tour.compete_alt"),
			draw: drawCompete,
		},
		{
			id: "friends",
			title: t("tour.friends_title"),
			when: offer("dm"),
			text: t("tour.friends_text"),
			alt: t("tour.friends_alt"),
			draw: drawFriends,
		},
		{
			id: "muster",
			title: t("tour.muster_title"),
			when: accounts ? (isAccount ? "yes" : "account") : "",
			text: t("tour.muster_text"),
			alt: t("tour.muster_alt"),
			draw: drawMuster,
		},
	];
	if (!isAccount) {
		const adds = ACCOUNT_ADDS.filter(([cap]) => accountCaps.includes(cap) && !caps.includes(cap)).map(([, what]) => t(what));
		if (accounts) adds.push(t("tour.adds_musters"), t("tour.adds_rated"), t("tour.adds_badges"), t("tour.adds_devices"));
		slides.push({
			id: "guest",
			title: t("tour.guest_title"),
			text: t("tour.guest_text"),
			alt: t("tour.guest_alt"),
			adds,
			signup: !!me.signup,
			draw: drawGuestKey,
		});
	}
	return slides.filter((sl) => sl.when === undefined || sl.when !== "").map((sl) => ({ ...sl, accountOnly: sl.when === "account" }));
}

/** "a, b and c". */
function listWords(items) {
	return new Intl.ListFormat(lang, { type: "conjunction" }).format(items);
}

// ---- the card -----------------------------------------------------------------

let current = null;

/** Opens the tour. Closing it in any way (Skip, Done, Esc) marks it seen. */
export function openTour(me = null) {
	if (current) return current;
	current = (async () => {
		me = me || (await whoAmI());
		const dlg = openCard({
			id: "welcome-tour",
			slides: slidesFor(me),
			skip: t("tour.skip"),
			extra: extraOf,
			onClose: () => {
				store.set(TOUR_SEEN_KEY, true);
				api.tourSeen().catch(() => {});
				current = null;
			},
		});
		if (!dlg) current = null;
		return dlg;
	})().catch(() => {
		current = null;
	});
	return current;
}

// What a slide says beside its words: that it needs an account, and what an
// account adds, with the way to one.
function extraOf(sl) {
	const out = [];
	if (sl.accountOnly) out.push(h("p", { class: "tour-note small", id: "tour-account-note" }, t("tour.with_account")));
	if (sl.adds && sl.adds.length) out.push(h("p", { class: "tour-adds small", id: "tour-adds" }, ...tParts("tour.account_adds", { adds: h("strong", {}, t("tour.adds_label")), list: listWords(sl.adds) })));
	if (sl.signup) out.push(h("p", {}, h("a", { class: "button primary", id: "tour-signup", href: "/signup" }, t("common.sign_up"))));
	return out;
}

// ---- the pictures -------------------------------------------------------------

let defsOnce = null;
const defs = () => (defsOnce = defsOnce || api.gameDefs().catch(() => []));
const infos = new Map();
const info = (id) => {
	if (!infos.has(id)) infos.set(id, api.gameDef(id));
	return infos.get(id);
};

// The games the welcome draws first, when the server has them. A family is
// named after its first game ("tak-5x5"), so it is known by its first word.
const FAVOURITES = ["tak", "hex", "go", "amazons", "havannah", "othello", "yavalath", "breakthrough"];
const familyWord = (d) => (d.family || d.id).split("-")[0];

async function drawGallery(host) {
	const all = await defs();
	const families = new Map();
	for (const d of all) {
		const fam = familyWord(d);
		// The family's own game, which names it, before its variants.
		if (!families.has(fam) || d.id === d.family) families.set(fam, d);
	}
	const order = [...FAVOURITES.filter((f) => families.has(f)), ...[...families.keys()].filter((f) => !FAVOURITES.includes(f))].slice(0, 8);
	const gallery = h("div", { class: "gallery tour-gallery" });
	host.append(gallery);
	await Promise.all(
		order.map(async (fam) => {
			const lead = families.get(fam);
			const slot = h("div", { class: "game-card-preview" });
			gallery.append(h("div", { class: "game-card" }, slot, h("div", { class: "game-card-title" }, lead.name.replace(/\s*\(.*\)$/, ""))));
			try {
				const gi = await info(lead.id);
				const icon = gi.page && gi.page.listIcon ? gi.page.listIcon() : null;
				if (icon) slot.append(icon);
				else renderBoard(slot, gi, { cells: gi.preview || gi.start.cells, flipped: false, interactive: false, showCoords: false, preview: true }, {});
			} catch (_) {
				/* a card without its picture */
			}
		})
	);
}

async function drawOpponents(host) {
	let bots = [];
	try {
		bots = await api.bots();
	} catch (_) {
		bots = [];
	}
	const choice = (label, desc, checked) =>
		h("label", { class: "opponent-choice" }, h("input", { type: "radio", checked }), h("span", {}, h("strong", {}, label), h("br"), h("span", { class: "small muted" }, desc)));
	const bot = bots[Math.min(1, bots.length - 1)];
	host.append(
		h(
			"div",
			{ class: "tour-panel" },
			h("div", { class: "field" }, h("label", {}, t("setup.opponent"))),
			h(
				"div",
				{ class: "stack" },
				choice(t("setup.a_friend"), t("setup.friend_desc"), false),
				bot ? choice(t("setup.bot", { name: bot.label }), bot.description, true) : choice(t("tour.a_bot"), t("tour.a_bot_desc"), true),
				choice(t("setup.two_bots"), t("setup.two_bots_desc"), false)
			),
			h("div", { class: "row" }, h("button", { type: "button", class: "primary" }, t("setup.create_game")))
		)
	);
}

const RULES_SAMPLE = `## How to win

Connect your two edges of the board with an unbroken chain of your stones.

| Turn | What you do |
|:--|:--|
| Your first | Place a stone on any empty cell |
| Every other | Place one stone; stones never move |

A chain can bend and twist as it likes: only touching counts.`;

async function drawRules(host) {
	const { renderMarkdown } = await import("./markdown.js");
	host.append(
		h(
			"div",
			{ class: "tour-panel tour-rules" },
			h("div", { class: "dialog-head" }, h("h2", {}, rulesIcon(18), " ", t("tour.hex_rules"))),
			h("div", { class: "tabs" }, h("button", { type: "button", class: "tab", "aria-selected": "true" }, t("setup.rules")), h("button", { type: "button", class: "tab", "aria-selected": "false" }, t("tour.controls"))),
			h("div", { class: "md" }, renderMarkdown(RULES_SAMPLE, { idPrefix: "tour-rules" }).node)
		)
	);
}

async function drawTable(host) {
	const all = await defs();
	const tak = all.find((d) => d.id === "tak-5x5") || all.find((d) => d.family === "tak");
	if (!tak) throw new Error("no Tak here");
	const gi = await info(tak.id);
	const slot = h("div", { class: "tour-board" });
	host.append(
		h("div", { class: "status-bar your-impulse tour-status" }, h("strong", {}, t("games.your_turn")), " · " + t("tour.tak_status")),
		slot,
		h("div", { class: "row tour-actions" }, h("button", { type: "button", class: "ghost small" }, t("tour.back")), h("button", { type: "button", class: "primary small" }, t("tour.submit")))
	);
	renderBoard(slot, gi, { cells: gi.preview || gi.start.cells, flipped: false, interactive: true, showCoords: true, selected: "c2", targets: new Set(["c1", "b2", "d2"]), lastMove: ["d4"] }, {});
}

async function drawPuzzle(host) {
	const list = await api.puzzles();
	const p = list.find((x) => x.game_def_id === "tak-5x5") || list[0];
	if (!p) throw new Error("no puzzles here");
	const gi = await info(p.game_def_id);
	const slot = h("div", { class: "puzzle-preview" });
	host.append(
		h(
			"div",
			{ class: "puzzle-grid tour-puzzles" },
			h(
				"div",
				{ class: "puzzle-card" },
				slot,
				h("div", { class: "puzzle-title" }, p.title.split(":")[0]),
				h("div", { class: "small muted" }, p.win_in === 1 ? t("home.win_at_once") : t("home.win_in", { n: p.win_in })),
				h("button", { class: "primary", type: "button" }, t("home.solve"))
			)
		)
	);
	renderBoard(slot, gi, { cells: (p.state && p.state.cells) || {}, flipped: false, interactive: false, showCoords: false }, {});
}

async function drawAnalysis(host) {
	const rows = [
		{ label: t("tour.white_mcts"), color: "var(--accent)", value: 0.58, lo: 0.52, hi: 0.64 },
		{ label: t("tour.black_minimax"), color: "var(--muted)", value: 0.42, lo: 0.36, hi: 0.48 },
	];
	host.append(
		h(
			"div",
			{ class: "tour-panel" },
			h("h3", {}, t("tour.tak_200")),
			h("p", { class: "small muted" }, t("tour.even_share")),
			rateBars(rows, { marker: 0.5, width: 520, rowH: 40 })
		)
	);
}

async function drawMyGames(host) {
	const all = await defs();
	const pick = (fam, fallback) => (all.find((d) => familyWord(d) === fam && d.id === d.family) || all.find((d) => familyWord(d) === fam) || all[fallback] || all[0] || { id: fam }).id;
	const you = t("setup.you");
	const now = Date.now();
	const row = (def, code, status, impulse, other, ago, extra = {}) =>
		gameRow(
			{
				g: { code, name: you, def, ts: new Date(now - ago).toISOString() },
				info: { game_def_id: def, status, impulse_seat: impulse, seats: [{ seat: 1, name: you }, { seat: 2, name: other }], ...extra },
			},
			all,
			() => {}
		);
	host.append(
		h("div", { class: "dialog-head" }, h("h2", {}, gamesIcon(18), " ", t("bar.active_games"))),
		h(
			"ul",
			{ class: "my-games" },
			row(pick("tak", 0), "K7QM", "active", 1, "Ana", 3 * 60000),
			row(pick("hex", 1), "B2XR", "active", 2, t("tour.tactical_bot"), 40 * 60000),
			row(pick("go", 2), "P9DW", "waiting", 1, "", 2 * 3600000, { lobby: true, seats: [{ seat: 1, name: you }, { seat: 2, name: "" }] })
		)
	);
}

async function drawCompete(host) {
	const rows = [
		["1", "@ana", "1812", "41"],
		["2", "@kofi", "1744", "35"],
		["3", "@mei", "1690", "52"],
	];
	const badge = {
		badge: "tour.sample-badge",
		name: t("tour.sample_badge"),
		description: t("tour.sample_badge_desc"),
		ways: { people: true },
		award: { game: "Hex", awarded_at: new Date(Date.now() - 86400000).toISOString() },
	};
	host.append(
		h(
			"div",
			{ class: "tour-panel" },
			h("table", { class: "data" }, h("thead", {}, h("tr", {}, ["", t("rating.player"), t("rating.rating"), t("rating.games")].map((head) => h("th", {}, head)))), h("tbody", {}, rows.map((r) => h("tr", {}, r.map((c) => h("td", {}, c)))))),
			h("ul", { class: "badge-list tour-badges" }, badgeItem(badge))
		)
	);
}

async function drawFriends(host) {
	const friend = (name, online, on = false) => h("li", { class: on ? "on" : null }, h("span", { class: "dm-person" }, h("span", { class: "friend-dot" + (online ? " online" : "") }), name));
	const line = (who, text, mine = false) => h("li", { class: "chat-line" + (mine ? " mine" : "") }, h("strong", {}, who), " ", text);
	host.append(
		h(
			"div",
			{ class: "tour-panel tour-friends" },
			h("ul", { class: "tour-friend-list" }, friend("@ana", true, true), friend("@kofi", true), friend("@mei", false)),
			h(
				"div",
				{ class: "tour-dm" },
				h("div", { class: "row" }, h("strong", {}, "@ana"), h("span", { class: "spacer" }), h("button", { type: "button", class: "small" }, t("dm.call")), h("button", { type: "button", class: "small primary" }, t("dm.invite"))),
				h("ol", { class: "chat-log tour-dm-lines small" }, line("@ana", t("tour.rematch")), line(t("setup.you"), t("tour.hex_this_time"), true))
			)
		)
	);
}

async function drawMuster(host) {
	const day = (d, hour) => {
		const at = new Date();
		at.setDate(at.getDate() + d);
		at.setHours(hour, 0, 0, 0);
		return at.toLocaleString(lang, { weekday: "long", hour: "numeric", minute: "2-digit" });
	};
	const slot = (when, n, mine, best = false) =>
		h("li", { class: best ? "tour-best" : null }, h("label", { class: "check" }, h("input", { type: "checkbox", checked: mine }), h("span", {}, when)), h("span", { class: "small muted" }, t("lobby.n_of", { n, total: 3 })));
	host.append(
		h(
			"div",
			{ class: "lobby-schedule", "data-status": "polling" },
			h("h3", {}, t("lobby.muster_poll")),
			h("ul", { class: "schedule-slots" }, slot(day(1, 19), 2, true), slot(day(2, 20), 3, true, true), slot(day(4, 18), 1, false)),
			h("p", { class: "small muted" }, [t("lobby.answered", { handle: "@ana" }), t("lobby.answered", { handle: "@kofi" })].join(" · "))
		)
	);
}

async function drawGuestKey(host) {
	const { section, paint } = guestKeySection();
	paint("guest");
	section.hidden = false;
	host.append(h("div", { class: "dialog-head" }, h("h2", {}, profileIcon(18), " ", t("tour.profile"))), h("div", { class: "tour-panel" }, section));
}
