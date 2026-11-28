// The icons of the bar at the top of every page: the active games (with a
// count of those waiting for you), joining a game by its room code, the
// player's profile, help (the version, what changed lately, the project's page,
// where to report problems) and — while a voice call is up — a button for the call.

import { h, clear, toast, lastName, setLastName, ensureName, nameIsMadeUp, forgottenGames, rememberGame, rememberedGames, setTurnBadge, store, brandName } from "./util.js";
import { api, whoAmI } from "./api.js";
import { offerNotify } from "./browser-notify.js";
import { guestKeySection } from "./guest-key.js";
import { gameDefs, loadRows, loadUnread, isFinished, yourImpulse, sortRows, gameRow, openGameRow } from "./mygames.js";
import { navigate, onLeave } from "./nav.js";
import { voiceStatus, voiceSettings, voiceMembers, onVoiceChange, toggleMute, toggleDeafen, leaveVoice } from "./voice.js";
import { callState, onCallChange, hangUp, toggleMute as toggleDirectMute } from "./dm-call.js";
import { gamesIcon, joinIcon, micIcon, helpIcon, profileIcon, messageIcon } from "./ui-icons.js";
import { t, tParts, lang } from "./i18n.js";
import { languageField, translateField } from "./language.js";
import { whoIsHere, onHereChange, startHere, watchHere, beat as beatHere, myName } from "./here.js";
import { mountDirectMessages, startDirectMessages, unreadCount, friendStateOf, askFriend, loadFriends, onFriendsChange } from "./dm.js";

const REPO_URL = "https://github.com/noderunner/bingle";

// The version of the changelog this browser has shown its player (the Recent Changes of the help window).
const CHANGES_SEEN_KEY = "bingle.changelog.seen";

// Which tab of the active-games window was last open ("mine" or "all").

// The direct-message window of this page, so that the profile window can hand
// it somebody to write to.
let messages = null;
let friendsTimer = 0;

/** Opens the friends list on a correspondence with one friend. */
function writeTo(address, name) {
	if (!messages) return;
	document.getElementById("open-friends")?.click();
	messages.open(address, name);
}

/**
 * Whether version a ("0.10.0") comes after version b ("0.9.3"). Nothing read yet ("") comes
 * before every version, and a version that is not three numbers after none.
 */
export function newerVersion(a, b) {
	const parts = (v) => (/^\d+\.\d+\.\d+$/.test(v || "") ? v.split(".").map(Number) : null);
	const pa = parts(a);
	const pb = parts(b);
	if (!pa) return false;
	if (!pb) return true;
	for (let i = 0; i < 3; i++) if (pa[i] !== pb[i]) return pa[i] > pb[i];
	return false;
}

/**
 * A release's Markdown without its "### Bingle Core 0.4.0" heading, which only repeats the
 * release's own heading (under any brand). A game's section keeps its heading.
 */
export function withoutCoreHeading(markdown, version) {
	const lines = String(markdown || "").split("\n");
	const i = lines.findIndex((l) => l.startsWith("### ") && l.trimEnd().endsWith(` Core ${version}`));
	if (i >= 0) lines.splice(i, 1);
	return lines.join("\n").trim();
}

/** The version of the bingle engine, which the server puts in every page. */
function engineVersion() {
	const meta = document.querySelector('meta[name="bingle-version"]');
	return (meta && meta.content) || "?";
}

/**
 * A window of the top bar. title is its name, shown as its heading, or {label, head}: the name
 * a screen reader gives it, and what stands beside the close button instead of a heading.
 */
function dialog(id, title, ...content) {
	const closeBtn = h("button", { type: "button", class: "ghost icon dialog-close", "aria-label": t("common.close"), title: t("common.close_esc") }, "✕");
	const label = typeof title === "string" ? title : title.label;
	const head = typeof title === "string" ? h("h2", {}, title) : title.head;
	const dlg = h("dialog", { id, class: "bar-dialog", "aria-label": label }, h("div", { class: "dialog-head" }, head, closeBtn), ...content);
	closeBtn.addEventListener("click", () => dlg.close());
	// A click on the backdrop (the dialog element itself) closes it.
	dlg.addEventListener("click", (ev) => {
		if (ev.target === dlg) dlg.close();
	});
	document.body.append(dlg);
	return dlg;
}

function open(dlg) {
	if (typeof dlg.showModal === "function") dlg.showModal();
	else dlg.setAttribute("open", "");
}

/**
 * Puts the icons into #topbar-actions. onRows(rows) is called with every
 * reading of the player's games (the page shows what it needs of them).
 * Returns {refresh} to read the games again.
 */
/** The room code in what was typed: the code itself, or a link to the game. */
export function roomCodeOf(typed) {
	const text = String(typed || "").trim();
	const link = /\/game\/([^/?#\s]+)/.exec(text);
	return (link ? link[1] : text).toUpperCase();
}

export function installTopbar({ onRows = () => {}, manageTitle = true } = {}) {
	ensureName();
	const host = document.getElementById("topbar-actions");
	if (!host) return { refresh() {} };
	const baseTitle = document.title.replace(/^\(\d+\)\s*/, "");

	// ---- active games ------------------------------------------------------
	const badge = h("span", { class: "icon-badge", id: "active-badge", hidden: true, "aria-hidden": "true" }, "0");
	const activeBtn = h(
		"button",
		{ type: "button", class: "topbar-icon", id: "open-active-games", "aria-haspopup": "dialog", title: t("bar.active_games"), "aria-label": t("bar.active_games") },
		gamesIcon(),
		badge
	);
	const list = h("ul", { class: "my-games", id: "my-games" });
	const empty = h("p", { class: "muted", id: "my-games-empty", hidden: true }, t("bar.no_games"));
	const activeDlg = dialog("active-games-dialog", t("bar.active_games"), list, empty);
	let rows = [];
	let defs = [];

	const paint = () => {
		// A refresh under way when a game was taken off the list still has it.
		const gone = new Set(forgottenGames());
		const active = sortRows(rows.filter((r) => !isFinished(r) && !gone.has(r.g.code)));
		const waiting = active.filter(yourImpulse).length;
		badge.textContent = String(waiting);
		badge.hidden = waiting === 0;
		const label = waiting ? t("bar.active_waiting", { n: waiting }) : t("bar.active_games");
		activeBtn.setAttribute("aria-label", label);
		activeBtn.title = label;
		if (manageTitle) {
			setTurnBadge(waiting > 0);
			document.title = (waiting > 0 ? `(${waiting}) ` : "") + baseTitle;
		}
		clear(list);
		for (const r of active) {
			list.append(
				gameRow(r, defs, () => {
					rows = rows.filter((x) => x !== r);
					paint();
					onRows(rows, defs);
				})
			);
		}
		empty.hidden = active.length > 0;
	};
	async function refresh(fetchAgain = true) {
		if (fetchAgain) {
			[defs, rows] = await Promise.all([gameDefs(), loadRows()]);
		}
		paint();
		onRows(rows, defs);
	}
	activeBtn.addEventListener("click", () => {
		paint();
		open(activeDlg);
		refresh()
			.then(() => loadUnread(rows.filter((r) => r.info && r.info.status !== "finished")))
			.then(paint)
			.catch(() => {});
	});

	// ---- join ----------------------------------------------------------------
	const joinBtn = h("button", { type: "button", class: "topbar-icon", id: "open-join", "aria-haspopup": "dialog", title: t("bar.join_game"), "aria-label": t("bar.join_game") }, joinIcon());
	const codeInput = h("input", { id: "room_code", name: "room_code", type: "text", required: true, placeholder: t("bar.room_code"), "aria-label": t("bar.room_code_label"), autocomplete: "off", autocapitalize: "characters", spellcheck: "false" });
	const joinErr = h("div", { class: "error-text", id: "join-error", role: "alert" });
	const joinForm = h(
		"form",
		{ id: "join-game-form" },
		h("div", { class: "field" }, codeInput),
		joinErr,
		h("div", { class: "row" }, h("button", { type: "submit", class: "primary", id: "join-submit" }, t("bar.join_game")))
	);
	// The tables with a seat for anybody, for those without a link to one.
	const openList = h("ul", { class: "my-games", id: "join-open-games" });
	const openEmpty = h("p", { class: "muted small", id: "join-open-empty", hidden: true }, t("bar.no_lobbies"));
	const openSection = h("section", { id: "join-open" }, h("hr", { class: "join-divider" }), h("h3", {}, t("bar.public_lobbies")), openList, openEmpty);
	const joinDlg = dialog("join-dialog", t("bar.join_game"), joinForm, openSection);
	async function refreshOpen() {
		let games = [];
		try {
			const res = await api.openGames({ openSeats: true });
			if (!defs.length) defs = await gameDefs();
			const mine = new Set(rememberedGames().map((g) => g.code));
			games = ((res && res.games) || []).filter((g) => !mine.has(g.room_code));
		} catch (_) {
			/* the code above still works */
		}
		clear(openList);
		for (const g of games) {
			openList.append(
				// A friend's game with no seat for this person can still be watched.
				openGameRow(g, defs, false, g.open_seats > 0 ? h("button", { type: "button", class: "primary", "data-join": g.room_code, onclick: () => {
					codeInput.value = g.room_code;
					join();
				} }, t("bar.join")) : null)
			);
		}
		openEmpty.hidden = games.length > 0;
	}
	joinBtn.addEventListener("click", () => {
		joinErr.textContent = "";
		open(joinDlg);
		codeInput.focus();
		refreshOpen();
	});
	/** A seat where one is open or the name already holds one, else a place to watch from. */
	async function join() {
		joinErr.textContent = "";
		const code = roomCodeOf(codeInput.value);
		if (!code) {
			codeInput.focus();
			return;
		}
		try {
			const name = lastName();
			const game = await api.game(code);
			const seats = game.seats || [];
			const mine = seats.some((s) => s.name && s.name.toLowerCase() === name.toLowerCase());
			const free = seats.some((s) => !s.name && !(game.reservations || []).some((r) => r.seat === s.seat && !r.yours));
			if (!mine && !free) {
				await api.join(code, name, "spectator");
			} else {
				try {
					await api.join(code, name, "player");
					rememberGame(code, game.game_def_id, name);
				} catch (e) {
					// The last seat went while we looked.
					if (!/seats? full/i.test(e.message)) throw e;
					await api.join(code, name, "spectator");
				}
			}
			navigate("/game/" + code);
		} catch (e) {
			joinErr.textContent = e.message;
			console.error("join game failed:", e);
		}
	}
	joinForm.addEventListener("submit", (ev) => {
		ev.preventDefault();
		join();
	});

	// ---- direct messages -------------------------------------------------------
	const messagesBtn = messagesWindow();

	// ---- profile ---------------------------------------------------------------
	const profile = profileWindow();

	// ---- help ---------------------------------------------------------------------
	// A dot on the icon says the changelog has an entry the player has not seen.
	const helpDot = h("span", { class: "icon-badge dot", id: "help-badge", hidden: true, "aria-hidden": "true" });
	const helpBtn = h("button", { type: "button", class: "topbar-icon", id: "open-help", "aria-haspopup": "dialog", title: t("bar.help"), "aria-label": t("bar.help") }, helpIcon(), helpDot);
	// Recent changes: every release since the one this browser last marked read, newest first, in a
	// box of its own that scrolls; Mark all as read clears the dot.
	const changesBody = h("div", { class: "help-changes-body", id: "help-changes-body", tabindex: "0", "aria-label": t("bar.the_changes") });
	const markRead = h("button", { type: "button", class: "small", id: "help-mark-read", hidden: true, onclick: () => markAllRead() }, t("bar.mark_read"));
	const changes = h(
		"section",
		{ class: "help-changes", id: "help-changes", hidden: true },
		h("div", { class: "help-changes-top" }, h("h3", {}, t("bar.recent_changes")), markRead),
		changesBody
	);
	// The version stands where a heading would, and the buttons share one line at the bottom.
	const helpDlg = dialog(
		"help-dialog",
		{ label: t("bar.help"), head: h("p", { class: "help-version", id: "help-version" }, h("strong", {}, brandName()), ` v${engineVersion()}`) },
		changes,
		h(
			"div",
			{ class: "help-links" },
			h("button", { type: "button", class: "button", id: "help-tour", onclick: () => takeTour() }, t("bar.tour")),
			// For anybody writing a client of their own: the API's documentation, from this server.
			h("a", { class: "button", id: "help-api", href: "/api/docs", target: "_blank", rel: "noopener" }, "API"),
			h("a", { class: "button", id: "help-github", href: REPO_URL, target: "_blank", rel: "noopener noreferrer" }, brandName() === "Bingle" ? "GitHub" : t("bar.source")),
			// The issues of another instance are not this project's to answer.
			brandName() === "Bingle" ? h("a", { class: "button primary", id: "help-issues", href: `${REPO_URL}/issues`, target: "_blank", rel: "noopener noreferrer" }, t("bar.report")) : null
		)
	);

	// The welcome tour (ADR 0139), loaded only when it is asked for.
	const takeTour = () => {
		helpDlg.close();
		import("./welcome-tour.js").then((m) => m.openTour()).catch(() => toast(t("bar.tour_failed"), "error"));
	};

	// The changelog's newest entry is read once the page is up, to know whether there is news: the
	// dot shows until the player marks the changes read. The releases since the one they last
	// marked are read when the window opens. A browser that never has gets the newest alone.
	let latest = null;
	const seenVersion = () => store.get(CHANGES_SEEN_KEY, "") || "";
	const unseen = () => !!latest && newerVersion(latest.version, seenVersion());
	const paintHelpDot = () => {
		const news = unseen();
		helpDot.hidden = !news;
		const label = news ? t("bar.help_news") : t("bar.help");
		helpBtn.setAttribute("aria-label", label);
		helpBtn.title = label;
	};
	const showChanges = async () => {
		if (!latest) return;
		const news = unseen();
		const seen = seenVersion();
		let entries = [latest];
		if (news && seen) {
			try {
				const got = await api.changelogSince(seen);
				if (got && got.entries && got.entries.length) entries = got.entries;
			} catch (_) {
				/* the newest alone, as the page already has it */
			}
		}
		let renderMarkdown = null;
		try {
			({ renderMarkdown } = await import("./markdown.js"));
		} catch (_) {
			renderMarkdown = null;
		}
		clear(changesBody);
		const articles = new Map();
		for (const e of entries) {
			const text = h("div", { class: "md" });
			const markdown = withoutCoreHeading(e.markdown, e.version);
			if (renderMarkdown) text.append(renderMarkdown(markdown, { idPrefix: `changes-${e.version}` }).node);
			else text.textContent = markdown;
			// A release not yet translated comes in English, and is marked so (left to right too).
			const other = e.lang && e.lang !== lang;
			const article = h("article", { class: "help-change", "data-version": e.version, lang: other ? e.lang : null, dir: other ? "ltr" : null }, h("h4", { class: "help-change-head" }, `${e.version}${e.date ? ` · ${e.date}` : ""}`), text);
			articles.set(e, article);
			changesBody.append(article);
		}
		changesBody.scrollTop = 0;
		markRead.hidden = !news;
		changes.hidden = false;
		linkAnnouncements(articles);
	};
	// The announcements (ADR 0155), each linked by its title under the release it came after (the
	// newest release has those after it too), newest first; a link shows it alone in the tour's card,
	// which marks nothing seen. Those from before the releases shown are under older ones, not here.
	const linkAnnouncements = async (articles) => {
		let list = [];
		try {
			({ announcements: list } = await api.announcements());
		} catch (_) {
			return; // the changes without them
		}
		const dated = [...articles.keys()].filter((e) => e.date);
		const under = new Map();
		for (const a of [...list].reverse()) {
			const e = dated.find((x) => x.date <= a.published_at.slice(0, 10));
			if (!e) continue;
			if (!under.has(e)) under.set(e, []);
			under.get(e).push(a);
		}
		const openOne = (ev, a) => {
			ev.preventDefault();
			import("./announcements.js").then((m) => m.openAnnouncement(a)).catch(() => toast(t("common.not_done"), "error"));
		};
		for (const [e, items] of under) {
			const links = items.map((a) => h("li", {}, h("a", { href: "#", class: "help-announcement", "data-announcement": a.id, onclick: (ev) => openOne(ev, a) }, a.title)));
			articles.get(e).append(h("ul", { class: "help-announcements" }, links));
		}
	};
	const markAllRead = () => {
		if (!latest) return;
		store.set(CHANGES_SEEN_KEY, latest.version);
		markRead.hidden = true;
		paintHelpDot();
		// The button has gone: the focus stays in the window, on the changes.
		changesBody.focus();
	};
	api.changelog().then(
		(entry) => {
			if (entry && entry.version && entry.markdown) {
				latest = entry;
				paintHelpDot();
			}
		},
		() => {} // no changelog on this server: no section, no dot
	);
	helpBtn.addEventListener("click", () => {
		open(helpDlg);
		showChanges();
	});

	const voice = voiceChip();
	host.append(voice, activeBtn, joinBtn, messagesBtn, profile, helpBtn);
	// Say that somebody is here, so the others' lists can show them, and listen
	// for anything written to us.
	startHere();
	startDirectMessages();
	offerNotify();

	refresh().catch(() => toast(t("bar.games_failed"), "error"));
	// Keep "your turn" fresh while the page sits open in a tab.
	const again = () => {
		if (!document.hidden) refresh().catch(() => {});
	};
	setInterval(again, 20000);
	document.addEventListener("visibilitychange", again);
	return { refresh };
}

/**
 * *Friends*: requests and invitations, what has been said with a friend, and the
 * voice call that can be put up inside it (js/dm.js). The badge on the icon counts
 * what waits to be read or answered.
 */
function messagesWindow() {
	const badge = h("span", { class: "icon-badge", id: "dm-badge", hidden: true, "aria-hidden": "true" }, "0");
	const btn = h(
		"button",
		{ type: "button", class: "topbar-icon", id: "open-friends", "aria-haspopup": "dialog", title: t("bar.friends"), "aria-label": t("bar.friends") },
		messageIcon(),
		badge
	);
	const body = h("div", { class: "dm-window" });
	const dlg = dialog("friends-dialog", t("bar.friends"), body);
	const paintBadge = (n) => {
		badge.textContent = String(n);
		badge.hidden = n === 0;
		const label = n ? t("bar.friends_waiting", { n }) : t("bar.friends");
		btn.setAttribute("aria-label", label);
		btn.title = label;
	};
	messages = mountDirectMessages(body, { onCount: paintBadge });
	paintBadge(unreadCount());
	// Friends are accounts' (ADR 0105): the icon stays away until the server says this is one.
	btn.hidden = true;
	whoAmI().then((me) => {
		btn.hidden = me.kind !== "account" || !(me.caps || []).includes("dm");
	});
	btn.addEventListener("click", () => {
		open(dlg);
		watchHere(true);
		messages.refresh();
		// Who is online shows while the window is open.
		clearInterval(friendsTimer);
		friendsTimer = setInterval(loadFriends, 8000);
	});
	dlg.addEventListener("close", () => {
		watchHere(false);
		clearInterval(friendsTimer);
	});
	onLeave(() => clearInterval(friendsTimer));
	onLeave(() => (messages = null));
	return btn;
}

/**
 * *Your profile*, as a floating window rather than a page of its own: the name
 * this browser plays under, the people who are here now, and a way on to the
 * full profile page (the email notifications and the games you have finished).
 * The list of people comes from js/here.js: signed-in accounts, by their
 * handles (ADR 0073).
 */
function profileWindow() {
	const btn = h("button", { type: "button", class: "topbar-icon", id: "open-profile", "aria-haspopup": "dialog", title: t("profile.button"), "aria-label": t("profile.button") }, profileIcon());
	const nameInput = h("input", { id: "profile-window-name", type: "text", maxlength: "32", autocomplete: "nickname", value: lastName() });
	const saved = h("span", { class: "small muted", id: "profile-window-saved", role: "status" });
	const list = h("ul", { class: "here-list", id: "here-list" });
	const count = h("span", { class: "small muted", id: "here-count" });
	// Who this browser is: signed in, or a guest who could sign in (the
	// accounts plan, §6).
	const account = h("section", { class: "account-section", id: "profile-account" });
	const guestKey = guestKeySection();
	const here = h(
		"section",
		{ class: "here-section", id: "profile-here", hidden: true },
		h("h3", {}, t("profile.here_now"), " ", count),
		list,
		h("p", { class: "small muted" }, t("profile.here_note"))
	);
	const dlg = dialog(
		"profile-dialog",
		t("profile.title"),
		account,
		h(
			"section",
			{ class: "name-section" },
			h("div", { class: "field" }, h("label", { for: "profile-window-name" }, t("profile.my_name")), nameInput, saved),
			languageField("profile-window-language"),
			translateField("profile-window-translate")
		),
		guestKey.section,
		here,
		h("div", { class: "row window-foot" }, h("a", { class: "button", id: "profile-page-link", href: "/profile" }, t("profile.all_settings")))
	);
	const paintAccount = (me) => {
		clear(account);
		const back = encodeURIComponent(location.pathname + location.search);
		if (me.kind === "account") {
			const signOut = h("button", { type: "button", class: "ghost", id: "profile-signout" }, t("profile.sign_out"));
			signOut.addEventListener("click", async () => {
				signOut.disabled = true;
				try {
					await api.logout();
				} finally {
					location.reload();
				}
			});
			account.append(
				h("p", { id: "profile-account-who" }, ...tParts("profile.signed_in_as", { handle: h("strong", { dir: "ltr" }, "@" + me.handle) })),
				h(
					"div",
					{ class: "row" },
					h("a", { class: "button", id: "profile-account-link", href: "/account" }, t("common.your_account")),
					me.admin ? h("a", { class: "button", id: "profile-admin-link", href: "/admin" }, t("profile.administration")) : null,
					signOut
				)
			);
			return;
		}
		if (!me.accounts) return;
		account.append(
			h("p", { id: "profile-account-who" }, t("profile.guest")),
			h(
				"div",
				{ class: "row" },
				h("a", { class: "button primary", id: "profile-signin", href: `/login?return=${back}` }, t("common.sign_in")),
				me.signup ? h("a", { class: "button", id: "profile-signup", href: `/signup?return=${back}` }, t("common.sign_up")) : null
			)
		);
	};
	// The icon says it too, without opening the window.
	whoAmI().then((me) => {
		const label = me.kind === "account" ? t("profile.button_account", { handle: me.handle }) : t("profile.button_guest");
		btn.title = label;
		btn.setAttribute("aria-label", label);
		btn.classList.toggle("signed-in", me.kind === "account");
		// Who is here is accounts' (ADR 0073), and an account has a name of its own.
		here.hidden = me.kind !== "account";
		if (me.kind === "account" && nameIsMadeUp()) setLastName(me.display_name || me.handle);
		paintAccount(me);
	});
	const paint = (people) => {
		clear(list);
		// The list names accounts by handle: "you" is this account's.
		const mine = (myName() || "").toLowerCase();
		count.textContent = people.length ? `(${people.length})` : "";
		for (const p of people) {
			const isMe = p.name && p.name.toLowerCase() === mine;
			const state = friendStateOf(p.name);
			list.append(
				h(
					"li",
					{ "data-address": p.address, class: isMe ? "me" : null },
					h("span", { class: "grow" }, p.name),
					isMe ? h("span", { class: "badge" }, t("profile.you")) : null,
					isMe
						? null
						: h(
								"button",
								{
									type: "button",
									class: "ghost small",
									disabled: state === "outgoing",
									"data-friend-action": state || "add",
									"data-address": p.address,
									title: t(state === "friend" ? "profile.write_to" : state === "outgoing" ? "profile.waiting_for" : state === "incoming" ? "profile.asked_you" : "profile.ask", { name: p.name }),
									onclick: () => {
										if (state === "friend") {
											dlg.close();
											writeTo(p.address, p.name);
										} else if (state === "incoming") {
											dlg.close();
											document.getElementById("open-friends")?.click();
										} else {
											askFriend(p.name);
										}
									},
								},
								t(state === "friend" ? "profile.message" : state === "outgoing" ? "profile.requested" : state === "incoming" ? "profile.answer" : "profile.add_friend")
							)
				)
			);
		}
		if (!people.length) list.append(h("li", { class: "muted" }, t("profile.here_nobody")));
	};
	nameInput.addEventListener("change", () => {
		const name = nameInput.value.trim();
		if (!name) return;
		setLastName(name);
		saved.textContent = t("common.saved");
		beatHere();
	});
	btn.addEventListener("click", () => {
		nameInput.value = lastName();
		saved.textContent = "";
		paint(whoIsHere());
		loadFriends();
		open(dlg);
		watchHere(true);
		// Asked afresh: this browser may have become a guest since the page loaded.
		api.me().then((me) => guestKey.paint(me.kind)).catch(() => {});
	});
	dlg.addEventListener("close", () => watchHere(false));
	const stop = onHereChange(paint);
	const stopFriends = onFriendsChange(() => paint(whoIsHere()));
	onLeave(() => {
		stop();
		stopFriends();
	});
	return btn;
}

/**
 * The button for a voice call: there while a call is up, wherever the player is
 * (another game, the main page, the profile), with the room code when the call
 * is in a game other than the one on screen. A game's call (js/voice.js) and a
 * direct call with a friend (js/dm-call.js) both show it; a person is in one at a
 * time. It opens a small panel with the channel's name, its controls (mute,
 * deafen, leave), who is in it, the settings of a game's call (push to talk, the
 * microphone and speakers) and a way back to its game.
 */
function voiceChip() {
	const dot = h("span", { class: "voice-live-dot", "aria-hidden": "true" });
	const code = h("span", { class: "voice-chip-code", id: "voice-chip-code" });
	const chip = h("button", { type: "button", class: "topbar-icon voice-chip", id: "voice-chip", hidden: true, "aria-haspopup": "true", "aria-expanded": "false" }, micIcon(), dot, code);
	const pop = h("div", { class: "voice-pop", id: "voice-pop", hidden: true, role: "group", "aria-label": t("voice.label") });
	document.body.append(pop);

	const handleOf = (name) => "@" + String(name || "").replace(/^@/, "");
	// The call that is up: a direct one, or a game's.
	const status = () => {
		const d = callState();
		if (d.state) return { direct: true, active: true, muted: d.muted, deafened: false, call: d };
		return voiceStatus();
	};

	const paintPop = (st) => {
		clear(pop);
		if (st.direct) {
			const c = st.call;
			pop.append(h("p", { class: "voice-pop-title", id: "voice-pop-title" }, h("strong", {}, t("voice.channel", { name: handleOf(c.name) }))));
			const live = c.state === "live";
			pop.append(
				h(
					"div",
					{ class: "row voice-pop-buttons" },
					live ? h("button", { type: "button", id: "voice-pop-mute", "aria-pressed": c.muted ? "true" : "false", class: c.muted ? "danger" : "", onclick: toggleDirectMute }, c.muted ? t("voice.unmute") : t("voice.mute")) : null,
					h("button", { type: "button", id: "voice-pop-leave", class: "ghost", onclick: () => hangUp() }, t("voice.leave"))
				),
				h(
					"ul",
					{ class: "voice-peers", id: "voice-peers" },
					h("li", { class: "voice-peer", "data-state": "connected", "data-me": "1" }, h("span", { class: "voice-dot", "aria-hidden": "true" }), h("span", { class: "voice-name" }, myName() ? t("voice.you", { name: myName() }) : t("voice.you_alone")), h("span", { class: "voice-state small muted" }, c.muted ? t("voice.mic_off") : t("voice.mic_on"))),
					h(
						"li",
						{ class: "voice-peer", "data-state": live ? "connected" : "connecting" },
						h("span", { class: "voice-dot", "aria-hidden": "true" }),
						h("span", { class: "voice-name" }, handleOf(c.name)),
						h("span", { class: "voice-state small muted" }, live ? t("voice.connected") : c.state === "ringing" ? t("voice.calling_you") : t("voice.ringing"))
					)
				)
			);
			return;
		}
		const here = st.viewing && st.viewing === st.code;
		pop.append(h("p", { class: "voice-pop-title", id: "voice-pop-title" }, h("strong", {}, t("voice.channel", { name: st.code }))));
		const row = h("div", { class: "row voice-pop-buttons" });
		if (!here) row.append(h("a", { class: "button primary", id: "voice-pop-go", href: `/game/${st.code}` }, t("voice.go")));
		row.append(
			h("button", { type: "button", id: "voice-pop-mute", "aria-pressed": st.muted ? "true" : "false", class: st.muted ? "danger" : "", onclick: toggleMute }, st.muted ? t("voice.unmute") : t("voice.mute")),
			h("button", { type: "button", id: "voice-pop-deafen", "aria-pressed": st.deafened ? "true" : "false", class: st.deafened ? "danger" : "", title: t("voice.deafen_title"), onclick: toggleDeafen }, st.deafened ? t("voice.undeafen") : t("voice.deafen")),
			h("button", { type: "button", id: "voice-pop-leave", class: "ghost", onclick: () => leaveVoice() }, t("voice.leave"))
		);
		pop.append(row, voiceMembers() || "", ...voiceSettings());
	};
	// Under the button, its right edge on the button's, but always inside the window: on a
	// phone the button can be nearer the left edge than the panel is wide.
	const place = () => {
		const r = chip.getBoundingClientRect();
		const vw = document.documentElement.clientWidth || window.innerWidth;
		const top = Math.round(r.bottom + 6);
		pop.style.top = `${top}px`;
		pop.style.right = "auto";
		pop.style.maxHeight = `${Math.max(120, window.innerHeight - top - 8)}px`;
		const w = pop.offsetWidth;
		pop.style.left = `${Math.round(Math.max(8, Math.min(r.right - w, vw - w - 8)))}px`;
	};
	const close = () => {
		pop.hidden = true;
		chip.setAttribute("aria-expanded", "false");
	};
	const update = () => {
		const st = status();
		chip.hidden = !st.active;
		if (!st.active) {
			close();
			clear(pop);
			return;
		}
		const elsewhere = !st.direct && st.viewing !== st.code;
		chip.classList.toggle("elsewhere", elsewhere);
		chip.classList.toggle("muted", st.muted || st.deafened);
		code.textContent = elsewhere ? st.code : "";
		const where = st.direct ? t("voice.in_call_with", { name: handleOf(st.call.name) }) : t("voice.in_game", { code: st.code });
		const label = st.deafened ? t("voice.deafened", { where }) : st.muted ? t("voice.muted", { where }) : where;
		chip.title = label;
		chip.setAttribute("aria-label", label);
		// Painted while folded up too: who is in the call is there the moment it opens.
		paintPop(st);
	};
	chip.addEventListener("click", (ev) => {
		ev.stopPropagation();
		if (!pop.hidden) return close();
		paintPop(status());
		pop.hidden = false;
		place();
		chip.setAttribute("aria-expanded", "true");
	});
	document.addEventListener("click", (ev) => {
		// The path, not the target: a button of the panel repaints it, so the target is gone by now.
		if (!pop.hidden && !ev.composedPath().includes(pop)) close();
	});
	document.addEventListener("keydown", (ev) => {
		if (ev.key === "Escape" && !pop.hidden) close();
	});
	window.addEventListener("resize", () => {
		if (!pop.hidden) place();
	});
	const stop = onVoiceChange(update);
	const stopDirect = onCallChange(update);
	onLeave(() => {
		stop();
		stopDirect();
		pop.remove();
	});
	update();
	return chip;
}
