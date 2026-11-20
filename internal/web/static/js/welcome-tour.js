// The welcome tour (ADR 0139): a floating card of slides on the main page,
// each a feature worth knowing about, with a picture drawn by the page's own
// code from sample data, so that it looks like the page in either theme and
// changes when the page does. It is offered once to each guest and account
// (home.js asks offerTour), and the help window takes it again (openTour).
//
// The pictures are only pictures: inert, out of the tab order and read to a
// screen reader as one description each.

import { h, clear, store, brandName } from "./util.js";
import { api, whoAmI } from "./api.js";
import { renderBoard } from "./render.js";
import { rateBars } from "./charts.js";
import { gameRow } from "./mygames.js";
import { badgeItem } from "./badges.js";
import { guestKeySection } from "./guest-key.js";
import { gamesIcon, profileIcon, rulesIcon } from "./ui-icons.js";

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
	["dm", "friends and messages"],
	["voice_direct", "calls with friends"],
	["chat_global", "writing in the global chat"],
	["notify", "an email when it's your turn"],
	["run_batch", "analysis batches"],
	["enter_tournament", "tournaments"],
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
			title: `Welcome to ${brand}`,
			text: "Abstract strategy games, from classics like Go and Hex to new designs. Pick a game from the list to play it: you don't need to sign up.",
			alt: "The list of games, each with a picture of its board.",
			draw: drawGallery,
		},
		{
			id: "opponents",
			title: "Play anyone",
			text: "Play a friend by sending them the game's link, or play a bot as strong as you like. You can also set two bots against each other and watch.",
			alt: "A game's setup, with the choice of opponent: a friend, yourself, a bot or two bots.",
			draw: drawOpponents,
		},
		{
			id: "learn",
			title: "Learn a game",
			text: "Every game has its rules a click away, with diagrams, a table of contents and search. Many games have tutorials too, which teach you by playing.",
			alt: "The rules window of a game, open at its rules.",
			draw: drawRules,
		},
		{
			id: "table",
			title: "At the table",
			text: "Choose a piece and the places it can go light up. Build your move, go back if you change your mind, and submit it when it's right. Offer a draw, chat with the table, or talk over voice chat.",
			alt: "A game of Tak in play, a piece chosen and the squares it can move to marked.",
			draw: drawTable,
		},
		{
			id: "puzzles",
			title: "Sharpen up",
			text: "Puzzles give you a position with a win to find. In a game, a bot can give you a hint, and afterwards you can replay it move by move and play on from any point.",
			alt: "A puzzle: a position to win from, with a Solve button.",
			draw: drawPuzzle,
		},
		{
			id: "analysis",
			title: "Analysis",
			when: offer("run_batch"),
			text: "Set bots playing hundreds of games of any game in the background, then read the report: how often each side wins, how the games end, and which bot is stronger.",
			alt: "A report of a batch of bot games: the win rate of each side, with a line at an even share.",
			draw: drawAnalysis,
		},
		{
			id: "games",
			title: "Never lose a game",
			text: "Your games wait for you. The games button at the top counts those where it's your turn, so you can play several at once, at your own pace.",
			alt: "The list of your games: one where it's your turn, one waiting for the other player, one in its lobby.",
			draw: drawMyGames,
		},
		{
			id: "compete",
			title: "Compete",
			when: accounts || caps.includes("enter_tournament") ? (isAccount ? "yes" : "account") : "",
			text: "Play rated games for a rating in each game, enter tournaments, and earn badges for the moments worth telling people about.",
			alt: "A table of ratings, and a badge earned.",
			draw: drawCompete,
		},
		{
			id: "friends",
			title: "Friends",
			when: offer("dm"),
			text: "Add friends by their handle, see who's online, write to them, call them and invite them straight into a game. And everybody meets in the global chat.",
			alt: "The friends window: friends with a dot for who is online, and a conversation.",
			draw: drawFriends,
		},
		{
			id: "muster",
			title: "Muster",
			when: accounts ? (isAccount ? "yes" : "account") : "",
			text: "Can't all play now? Call a muster: offer the times you could play, the people you invite tick the ones they can make, and the game is set for the earliest time that suits everyone, with an invitation for their calendars.",
			alt: "A muster in a game's lobby: the times offered, each with how many can make it.",
			draw: drawMuster,
		},
	];
	if (!isAccount) {
		const adds = ACCOUNT_ADDS.filter(([cap]) => accountCaps.includes(cap) && !caps.includes(cap)).map(([, what]) => what);
		if (accounts) adds.push("musters", "rated games", "badges", "your games on every device you sign in on");
		slides.push({
			id: "guest",
			title: "You're playing as a guest",
			text: "Your guest key is what makes your games yours, and this browser keeps it. To play on your phone or another computer too, open your profile, the person at the top, and choose Move to Another Device.",
			alt: "The Move to Another Device part of the profile window, with the button that copies your guest key.",
			adds,
			signup: !!me.signup,
			draw: drawGuestKey,
		});
	}
	return slides.filter((sl) => sl.when === undefined || sl.when !== "").map((sl) => ({ ...sl, accountOnly: sl.when === "account" }));
}

/** "a, b and c". */
function listWords(items) {
	return items.length < 2 ? items.join("") : `${items.slice(0, -1).join(", ")} and ${items[items.length - 1]}`;
}

// ---- the card -----------------------------------------------------------------

let current = null;

/** Opens the tour. Closing it in any way (Skip, Done, Esc) marks it seen. */
export function openTour(me = null) {
	if (current) return current;
	current = (async () => {
		me = me || (await whoAmI());
		const slides = slidesFor(me);
		const dlg = buildCard(slides);
		document.body.append(dlg);
		if (typeof dlg.showModal === "function") dlg.showModal();
		else dlg.setAttribute("open", "");
		dlg.querySelector("#tour-next").focus();
		return dlg;
	})().catch(() => {
		current = null;
	});
	return current;
}

function buildCard(slides) {
	const count = h("p", { class: "tour-count small muted", id: "tour-count", "aria-live": "polite" });
	const picture = h("div", { class: "tour-picture", id: "tour-picture", role: "img" });
	const title = h("h2", { id: "tour-title" });
	const text = h("p", { class: "tour-text", id: "tour-text" });
	const extra = h("div", { class: "tour-extra", id: "tour-extra" });
	const dots = h("div", { class: "tour-dots", role: "group", "aria-label": "Slides" });
	const back = h("button", { type: "button", class: "ghost", id: "tour-back" }, "Back");
	const next = h("button", { type: "button", class: "primary", id: "tour-next" }, "Next");
	const skip = h("button", { type: "button", class: "ghost", id: "tour-skip" }, "Skip");
	const dlg = h(
		"dialog",
		{ id: "welcome-tour", class: "tour-card", "aria-labelledby": "tour-title", "aria-describedby": "tour-text" },
		h("div", { class: "tour-body" }, count, picture, title, text, extra),
		dots,
		h("div", { class: "tour-foot" }, skip, h("span", { class: "spacer" }), back, next)
	);

	let at = 0;
	let drawing = 0;
	const dotButtons = slides.map((sl, i) => {
		const b = h("button", { type: "button", class: "tour-dot", "data-slide": sl.id, "aria-label": `${i + 1}: ${sl.title}`, onclick: () => show(i) });
		dots.append(b);
		return b;
	});

	function show(i) {
		at = Math.max(0, Math.min(slides.length - 1, i));
		const sl = slides[at];
		dlg.dataset.slide = sl.id;
		count.textContent = `${at + 1} of ${slides.length}`;
		title.textContent = sl.title;
		text.textContent = sl.text;
		clear(extra);
		if (sl.accountOnly) extra.append(h("p", { class: "tour-note small", id: "tour-account-note" }, "With an account."));
		if (sl.adds && sl.adds.length) extra.append(h("p", { class: "tour-adds small", id: "tour-adds" }, h("strong", {}, "An account adds "), listWords(sl.adds) + "."));
		if (sl.signup) extra.append(h("p", {}, h("a", { class: "button primary", id: "tour-signup", href: "/signup" }, "Sign up")));
		picture.setAttribute("aria-label", sl.alt);
		dotButtons.forEach((b, j) => {
			b.classList.toggle("on", j === at);
			b.setAttribute("aria-current", j === at ? "step" : "false");
		});
		back.disabled = at === 0;
		next.textContent = at === slides.length - 1 ? "Done" : "Next";
		drawPicture(picture, sl, ++drawing, () => drawing);
	}

	next.addEventListener("click", () => (at === slides.length - 1 ? dlg.close() : show(at + 1)));
	back.addEventListener("click", () => show(at - 1));
	skip.addEventListener("click", () => dlg.close());
	dlg.addEventListener("keydown", (ev) => {
		if (ev.target.closest && ev.target.closest("input, textarea, select")) return;
		if (ev.key === "ArrowRight") show(at + 1);
		else if (ev.key === "ArrowLeft") show(at - 1);
		else return;
		ev.preventDefault();
	});
	dlg.addEventListener("close", () => {
		store.set(TOUR_SEEN_KEY, true);
		api.tourSeen().catch(() => {});
		dlg.remove();
		current = null;
	});
	show(0);
	return dlg;
}

// drawPicture draws a slide's picture, unless another slide has been shown
// while it was being drawn. A picture that cannot be drawn leaves the frame
// empty: the words still say what the slide is about.
async function drawPicture(frame, slide, n, latest) {
	clear(frame);
	const stage = h("div", { class: "tour-stage", inert: true, "aria-hidden": "true" });
	try {
		await slide.draw(stage, slide);
	} catch (_) {
		clear(stage);
	}
	if (n !== latest()) return;
	quiet(stage);
	clear(frame).append(stage);
}

// quiet makes a picture only a picture: no ids or names that the page's own
// elements have, nothing to focus, nothing to submit.
function quiet(root) {
	for (const el of root.querySelectorAll("*")) {
		if (el instanceof SVGElement) continue; // an SVG's ids are its own gradients and clips
		el.removeAttribute("id");
		el.removeAttribute("for");
		el.removeAttribute("name");
		if (el.tabIndex >= 0) el.setAttribute("tabindex", "-1");
	}
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
const FAVOURITES = ["tak", "hex", "go", "amazons", "havannah", "reversi", "yavalath", "breakthrough"];
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
			h("div", { class: "field" }, h("label", {}, "Opponent")),
			h(
				"div",
				{ class: "stack" },
				choice("A friend", "You get a link to share; the game starts when they join.", false),
				bot ? choice(bot.label + " bot", bot.description, true) : choice("A bot", "A built-in opponent.", true),
				choice("Two bots", "Nobody plays: you watch two bots play each other.", false)
			),
			h("div", { class: "row" }, h("button", { type: "button", class: "primary" }, "Create Game"))
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
			h("div", { class: "dialog-head" }, h("h2", {}, rulesIcon(18), " Hex: Rules")),
			h("div", { class: "tabs" }, h("button", { type: "button", class: "tab", "aria-selected": "true" }, "Rules"), h("button", { type: "button", class: "tab", "aria-selected": "false" }, "Controls")),
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
		h("div", { class: "status-bar your-impulse tour-status" }, h("strong", {}, "Your turn"), " · move your capstone, or place a stone"),
		slot,
		h("div", { class: "row tour-actions" }, h("button", { type: "button", class: "ghost small" }, "Back"), h("button", { type: "button", class: "primary small" }, "Submit"))
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
				h("div", { class: "small muted" }, p.win_in === 1 ? "Win at once" : `Win in ${p.win_in}`),
				h("button", { class: "primary", type: "button" }, "Solve")
			)
		)
	);
	renderBoard(slot, gi, { cells: (p.state && p.state.cells) || {}, flipped: false, interactive: false, showCoords: false }, {});
}

async function drawAnalysis(host) {
	const rows = [
		{ label: "White (MCTS)", color: "var(--accent)", value: 0.58, lo: 0.52, hi: 0.64 },
		{ label: "Black (Minimax)", color: "var(--muted)", value: 0.42, lo: 0.36, hi: 0.48 },
	];
	host.append(
		h(
			"div",
			{ class: "tour-panel" },
			h("h3", {}, "Tak (5×5) · 200 games"),
			h("p", { class: "small muted" }, "How often each side won, with a line at an even share."),
			rateBars(rows, { marker: 0.5, width: 520, rowH: 40 })
		)
	);
}

async function drawMyGames(host) {
	const all = await defs();
	const pick = (fam, fallback) => (all.find((d) => familyWord(d) === fam && d.id === d.family) || all.find((d) => familyWord(d) === fam) || all[fallback] || all[0] || { id: fam }).id;
	const you = "You";
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
		h("div", { class: "dialog-head" }, h("h2", {}, gamesIcon(18), " Active Games")),
		h(
			"ul",
			{ class: "my-games" },
			row(pick("tak", 0), "K7QM", "active", 1, "Ana", 3 * 60000),
			row(pick("hex", 1), "B2XR", "active", 2, "Tactical bot", 40 * 60000),
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
		badge: "tour.sample",
		name: "Against the Odds",
		description: "Win a game you were losing by a long way.",
		ways: { people: true },
		award: { game: "Hex", awarded_at: new Date(Date.now() - 86400000).toISOString() },
	};
	host.append(
		h(
			"div",
			{ class: "tour-panel" },
			h("table", { class: "data" }, h("thead", {}, h("tr", {}, ["", "Player", "Rating", "Rated games"].map((t) => h("th", {}, t)))), h("tbody", {}, rows.map((r) => h("tr", {}, r.map((c) => h("td", {}, c)))))),
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
				h("div", { class: "row" }, h("strong", {}, "@ana"), h("span", { class: "spacer" }), h("button", { type: "button", class: "small" }, "Call"), h("button", { type: "button", class: "small primary" }, "Invite to game")),
				h("ol", { class: "chat-log tour-dm-lines small" }, line("@ana", "Rematch tonight?"), line("You", "Yes! Hex this time.", true))
			)
		)
	);
}

async function drawMuster(host) {
	const day = (d, hour) => {
		const t = new Date();
		t.setDate(t.getDate() + d);
		t.setHours(hour, 0, 0, 0);
		return t.toLocaleString([], { weekday: "long", hour: "numeric", minute: "2-digit" });
	};
	const slot = (when, n, mine, best = false) =>
		h("li", { class: best ? "tour-best" : null }, h("label", { class: "check" }, h("input", { type: "checkbox", checked: mine }), h("span", {}, when)), h("span", { class: "small muted" }, `${n} of 3`));
	host.append(
		h(
			"div",
			{ class: "lobby-schedule", "data-status": "polling" },
			h("h3", {}, "Muster: which times can you make?"),
			h("ul", { class: "schedule-slots" }, slot(day(1, 19), 2, true), slot(day(2, 20), 3, true, true), slot(day(4, 18), 1, false)),
			h("p", { class: "small muted" }, "@ana: answered · @kofi: answered")
		)
	);
}

async function drawGuestKey(host) {
	const { section, paint } = guestKeySection();
	paint("guest");
	section.hidden = false;
	host.append(h("div", { class: "dialog-head" }, h("h2", {}, profileIcon(18), " Profile")), h("div", { class: "tour-panel" }, section));
}
