// The icons of the bar at the top of every page: the active games (with a
// count of those waiting for you), joining a game by its room code, the
// player's profile, help (the version, what changed lately, the project's page,
// where to report problems) and — while a voice call is up — a button for the call.

import { h, clear, toast, lastName, setLastName, rememberGame, setTurnBadge, store } from "./util.js";
import { api, whoAmI } from "./api.js";
import { gameDefs, loadRows, isFinished, yourTurn, sortRows, gameRow, openGameRow } from "./mygames.js";
import { navigate, onLeave } from "./nav.js";
import { voiceStatus, onVoiceChange, toggleMute, toggleDeafen, leaveVoice } from "./voice.js";
import { gamesIcon, joinIcon, micIcon, helpIcon, profileIcon, messageIcon } from "./ui-icons.js";
import { whoIsHere, onHereChange, startHere, watchHere, beat as beatHere, myName } from "./here.js";
import { mountDirectMessages, startDirectMessages, unreadCount } from "./dm.js";

const REPO_URL = "https://github.com/noderunner/bingle";

// The version of the changelog this browser has shown its player (the Recent Changes of the help window).
const CHANGES_SEEN_KEY = "bingle.changelog.seen";

// Which tab of the active-games window was last open ("mine" or "all").
const TAB_KEY = "bingle.gamesTab";

// The direct-message window of this page, so that the profile window can hand
// it somebody to write to.
let messages = null;

/** Opens the direct messages on a correspondence with one person. */
function writeTo(address, name) {
	if (!messages) return;
	document.getElementById("open-messages")?.click();
	messages.open(address, name);
}

/** The version of the bingle engine, which the server puts in every page. */
function engineVersion() {
	const meta = document.querySelector('meta[name="bingle-version"]');
	return (meta && meta.content) || "?";
}

function dialog(id, title, ...content) {
	const closeBtn = h("button", { type: "button", class: "ghost icon dialog-close", "aria-label": "Close", title: "Close (Esc)" }, "✕");
	const dlg = h("dialog", { id, class: "bar-dialog", "aria-label": title }, h("div", { class: "dialog-head" }, h("h2", {}, title), closeBtn), ...content);
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
export function installTopbar({ onRows = () => {}, manageTitle = true } = {}) {
	const host = document.getElementById("topbar-actions");
	if (!host) return { refresh() {} };
	const baseTitle = document.title.replace(/^\(\d+\)\s*/, "");

	// ---- active games ------------------------------------------------------
	const badge = h("span", { class: "icon-badge", id: "active-badge", hidden: true, "aria-hidden": "true" }, "0");
	const activeBtn = h(
		"button",
		{ type: "button", class: "topbar-icon", id: "open-active-games", "aria-haspopup": "dialog", title: "Active games", "aria-label": "Active games" },
		gamesIcon(),
		badge
	);
	const list = h("ul", { class: "my-games", id: "my-games" });
	const empty = h("p", { class: "muted", id: "my-games-empty", hidden: true }, "No games going. Pick one in the Games tab to start.");
	const allList = h("ul", { class: "my-games", id: "all-games" });
	const allEmpty = h("p", { class: "muted", id: "all-games-empty", hidden: true }, "No games are going anywhere on this server just now.");
	const allNote = h("p", { class: "small muted" }, "Every game on this server that has not finished. Open one to watch it, or take a seat where one is open.");
	// Two tabs: the games of this browser, and every game on the server.
	const mineTab = h("button", { type: "button", class: "tab on", id: "tab-my-games", role: "tab", "aria-selected": "true", "aria-controls": "panel-my-games" }, "My games");
	const allTab = h("button", { type: "button", class: "tab", id: "tab-all-games", role: "tab", "aria-selected": "false", "aria-controls": "panel-all-games" }, "All games");
	const minePanel = h("div", { id: "panel-my-games", role: "tabpanel", "aria-labelledby": "tab-my-games" }, list, empty);
	const allPanel = h("div", { id: "panel-all-games", role: "tabpanel", "aria-labelledby": "tab-all-games", hidden: true }, allNote, allList, allEmpty);
	const tabs = h("div", { class: "tabs", role: "tablist", "aria-label": "Which games to list" }, mineTab, allTab);
	const showTab = (which) => {
		const mine = which === "mine";
		mineTab.classList.toggle("on", mine);
		allTab.classList.toggle("on", !mine);
		mineTab.setAttribute("aria-selected", mine ? "true" : "false");
		allTab.setAttribute("aria-selected", mine ? "false" : "true");
		minePanel.hidden = !mine;
		allPanel.hidden = mine;
		store.set(TAB_KEY, which);
		if (!mine) refreshAll();
	};
	mineTab.addEventListener("click", () => showTab("mine"));
	allTab.addEventListener("click", () => showTab("all"));
	for (const [tab, which] of [[mineTab, "mine"], [allTab, "all"]]) {
		tab.addEventListener("keydown", (ev) => {
			if (ev.key !== "ArrowLeft" && ev.key !== "ArrowRight") return;
			ev.preventDefault();
			const other = which === "mine" ? allTab : mineTab;
			showTab(which === "mine" ? "all" : "mine");
			other.focus();
		});
	}
	const activeDlg = dialog("active-games-dialog", "Active games", tabs, minePanel, allPanel);
	let rows = [];
	let defs = [];
	let allRows = [];

	const paintAll = () => {
		clear(allList);
		const mine = new Set(rows.map((r) => r.g.code));
		for (const g of allRows) allList.append(openGameRow(g, defs, mine.has(g.room_code)));
		allEmpty.hidden = allRows.length > 0;
	};
	async function refreshAll() {
		try {
			const res = await api.openGames();
			allRows = (res && res.games) || [];
			if (!defs.length) defs = await gameDefs();
			paintAll();
		} catch (_) {
			allEmpty.hidden = allRows.length > 0;
		}
	}

	const paint = () => {
		const active = sortRows(rows.filter((r) => !isFinished(r)));
		const waiting = active.filter(yourTurn).length;
		badge.textContent = String(waiting);
		badge.hidden = waiting === 0;
		const label = waiting ? `Active games, ${waiting} waiting for you` : "Active games";
		activeBtn.setAttribute("aria-label", label);
		activeBtn.title = label;
		if (manageTitle) {
			setTurnBadge(waiting > 0);
			document.title = (waiting > 0 ? `(${waiting}) ` : "") + baseTitle;
		}
		clear(list);
		for (const r of active) list.append(gameRow(r, defs, () => refresh(false)));
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
		showTab(store.get(TAB_KEY, "mine") === "all" ? "all" : "mine");
		refresh();
	});

	// ---- join ----------------------------------------------------------------
	const joinBtn = h("button", { type: "button", class: "topbar-icon", id: "open-join", "aria-haspopup": "dialog", title: "Join a game", "aria-label": "Join a game" }, joinIcon());
	const codeInput = h("input", { id: "room_code", name: "room_code", type: "text", required: true, maxlength: "12", autocomplete: "off", autocapitalize: "characters", spellcheck: "false" });
	const nameInput = h("input", { id: "join_player_name", name: "player_name", type: "text", required: true, maxlength: "32", autocomplete: "nickname" });
	const joinErr = h("div", { class: "error-text", id: "join-error", role: "alert" });
	const joinForm = h(
		"form",
		{ id: "join-game-form" },
		h("p", { class: "muted small" }, "Enter the room code from the link a friend sent you."),
		h("div", { class: "field" }, h("label", { for: "room_code" }, "Room code"), codeInput),
		h("div", { class: "field" }, h("label", { for: "join_player_name" }, "Your name"), nameInput),
		joinErr,
		h("div", { class: "row" }, h("button", { type: "submit", class: "primary" }, "Join game"), h("button", { type: "button", id: "watch-game" }, "Watch"))
	);
	const joinDlg = dialog("join-dialog", "Join a game", joinForm);
	joinBtn.addEventListener("click", () => {
		if (!nameInput.value) nameInput.value = lastName();
		joinErr.textContent = "";
		open(joinDlg);
		(codeInput.value ? nameInput : codeInput).focus();
	});
	async function join(role) {
		joinErr.textContent = "";
		const code = codeInput.value.trim().toUpperCase();
		const name = nameInput.value.trim();
		if (!code) {
			codeInput.focus();
			return;
		}
		if (!name && role === "player") {
			nameInput.focus();
			return;
		}
		try {
			await api.join(code, name || "spectator", role);
			if (role === "player") {
				const game = await api.game(code);
				rememberGame(code, game.game_def_id, name);
			}
			navigate("/game/" + code);
		} catch (e) {
			joinErr.textContent = e.message;
			console.error("join game failed:", e);
		}
	}
	joinForm.addEventListener("submit", (ev) => {
		ev.preventDefault();
		join("player");
	});
	joinForm.querySelector("#watch-game").addEventListener("click", () => join("spectator"));

	// ---- direct messages -------------------------------------------------------
	const messagesBtn = messagesWindow();

	// ---- profile ---------------------------------------------------------------
	const profile = profileWindow();

	// ---- help ---------------------------------------------------------------------
	// A dot on the icon says the changelog has an entry the player has not seen.
	const helpDot = h("span", { class: "icon-badge dot", id: "help-badge", hidden: true, "aria-hidden": "true" });
	const helpBtn = h("button", { type: "button", class: "topbar-icon", id: "open-help", "aria-haspopup": "dialog", title: "Help", "aria-label": "Help" }, helpIcon(), helpDot);
	const changesBody = h("div", { class: "help-changes-body md" });
	const changesHead = h("p", { class: "muted small help-changes-head", id: "help-changes-head" });
	const changes = h("section", { class: "help-changes", id: "help-changes", hidden: true }, h("h3", {}, "Recent changes"), changesHead, changesBody);
	const helpDlg = dialog(
		"help-dialog",
		"Help",
		h("p", { class: "help-version", id: "help-version" }, h("strong", {}, "Bingle"), ` v${engineVersion()}`),
		h("p", { class: "muted small" }, "Abstract strategy games to play with friends, against bots, or for training models. Open source under the MIT License."),
		changes,
		h(
			"div",
			{ class: "row help-links" },
			h("a", { class: "button", id: "help-github", href: REPO_URL, target: "_blank", rel: "noopener noreferrer" }, "Bingle on GitHub"),
			h("a", { class: "button primary", id: "help-issues", href: `${REPO_URL}/issues`, target: "_blank", rel: "noopener noreferrer" }, "Report Issues")
		)
	);

	// The changelog's newest entry is read once the page is up; the dot shows until the player has seen it.
	let latest = null;
	let latestShown = false;
	const paintHelpDot = () => {
		const unseen = !!latest && store.get(CHANGES_SEEN_KEY, "") !== latest.version;
		helpDot.hidden = !unseen;
		const label = unseen ? "Help, with changes you have not seen" : "Help";
		helpBtn.setAttribute("aria-label", label);
		helpBtn.title = label;
	};
	const showChanges = async () => {
		if (!latest) return;
		changesHead.textContent = `Bingle ${latest.version}${latest.date ? ` · ${latest.date}` : ""}`;
		changes.hidden = false;
		if (!latestShown) {
			latestShown = true;
			try {
				const { renderMarkdown } = await import("./markdown.js");
				changesBody.append(renderMarkdown(latest.markdown, { idPrefix: "changes" }).node);
			} catch (_) {
				changesBody.textContent = latest.markdown;
			}
		}
		// The section is on screen: the player has seen this entry.
		store.set(CHANGES_SEEN_KEY, latest.version);
		paintHelpDot();
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

	refresh().catch(() => toast("Could not read your games.", "error"));
	// Keep "your turn" fresh while the page sits open in a tab.
	const again = () => {
		if (!document.hidden) refresh().catch(() => {});
	};
	setInterval(again, 20000);
	document.addEventListener("visibilitychange", again);
	return { refresh };
}

/**
 * *Direct messages*: the people who are here, what has been said with one of
 * them, and the voice call that can be put up inside it (js/dm.js). The badge
 * on the icon counts what has not been read.
 */
function messagesWindow() {
	const badge = h("span", { class: "icon-badge", id: "dm-badge", hidden: true, "aria-hidden": "true" }, "0");
	const btn = h(
		"button",
		{ type: "button", class: "topbar-icon", id: "open-messages", "aria-haspopup": "dialog", title: "Direct messages", "aria-label": "Direct messages" },
		messageIcon(),
		badge
	);
	const body = h("div", { class: "dm-window" });
	const dlg = dialog("messages-dialog", "Direct messages", body);
	const paintBadge = (n) => {
		badge.textContent = String(n);
		badge.hidden = n === 0;
		const label = n ? `Direct messages, ${n} unread` : "Direct messages";
		btn.setAttribute("aria-label", label);
		btn.title = label;
	};
	messages = mountDirectMessages(body, { onCount: paintBadge });
	paintBadge(unreadCount());
	// Direct messages are accounts' (the dm capability): the icon stays away
	// until the server says this browser may use them.
	btn.hidden = true;
	whoAmI().then((me) => {
		btn.hidden = !(me.caps || []).includes("dm");
	});
	btn.addEventListener("click", () => {
		open(dlg);
		watchHere(true);
		messages.refresh();
	});
	dlg.addEventListener("close", () => watchHere(false));
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
	const btn = h("button", { type: "button", class: "topbar-icon", id: "open-profile", "aria-haspopup": "dialog", title: "Your profile", "aria-label": "Your profile" }, profileIcon());
	const nameInput = h("input", { id: "profile-window-name", type: "text", maxlength: "32", autocomplete: "nickname", value: lastName() });
	const saved = h("span", { class: "small muted", id: "profile-window-saved", role: "status" });
	const list = h("ul", { class: "here-list", id: "here-list" });
	const count = h("span", { class: "small muted", id: "here-count" });
	// Who this browser is: signed in, or a guest who could sign in (the
	// accounts plan, §6).
	const account = h("section", { class: "account-section", id: "profile-account" });
	const dlg = dialog(
		"profile-dialog",
		"Your profile",
		account,
		h(
			"div",
			{ class: "field" },
			h("label", { for: "profile-window-name" }, "The name you play under"),
			nameInput,
			saved,
			h("span", { class: "small muted" }, "It is offered when you make or join a game, and it is what the others see while you are here.")
		),
		h("section", { class: "here-section" }, h("h3", {}, "Here now ", count), list, h("p", { class: "small muted" }, "The signed-in players who have been here in the last few minutes, by their usernames. Guests are not listed: sign in to be listed and to write to anybody here.")),
		h("div", { class: "row" }, h("a", { class: "button", id: "profile-page-link", href: "/profile" }, "Email notifications and finished games"))
	);
	const paintAccount = (me) => {
		clear(account);
		const back = encodeURIComponent(location.pathname + location.search);
		if (me.kind === "account") {
			const signOut = h("button", { type: "button", class: "ghost", id: "profile-signout" }, "Sign out");
			signOut.addEventListener("click", async () => {
				signOut.disabled = true;
				try {
					await api.logout();
				} finally {
					location.reload();
				}
			});
			account.append(
				h("p", { id: "profile-account-who" }, "Signed in as ", h("strong", {}, "@" + me.handle)),
				h(
					"div",
					{ class: "row" },
					h("a", { class: "button", id: "profile-account-link", href: "/account" }, "Your account"),
					me.admin ? h("a", { class: "button", id: "profile-admin-link", href: "/admin" }, "Administration") : null,
					signOut
				)
			);
			return;
		}
		if (!me.accounts) return;
		account.append(
			h("p", { id: "profile-account-who" }, "You are playing as a guest."),
			h(
				"div",
				{ class: "row" },
				h("a", { class: "button primary", id: "profile-signin", href: `/login?return=${back}` }, "Sign in"),
				me.signup ? h("a", { class: "button", id: "profile-signup", href: `/signup?return=${back}` }, "Sign up") : null
			)
		);
	};
	// The icon says it too, without opening the window.
	whoAmI().then((me) => {
		const label = me.kind === "account" ? `Your profile (signed in as @${me.handle})` : "Your profile (playing as a guest)";
		btn.title = label;
		btn.setAttribute("aria-label", label);
		btn.classList.toggle("signed-in", me.kind === "account");
		paintAccount(me);
	});
	const paint = (people) => {
		clear(list);
		// The list names accounts by handle: "you" is this account's.
		const mine = (myName() || "").toLowerCase();
		count.textContent = people.length ? `(${people.length})` : "";
		for (const p of people) {
			const isMe = p.name && p.name.toLowerCase() === mine;
			list.append(
				h(
					"li",
					{ "data-address": p.address, class: isMe ? "me" : null },
					h("span", { class: "grow" }, p.name),
					isMe ? h("span", { class: "badge" }, "you") : null,
					isMe
						? null
						: h(
								"button",
								{
									type: "button",
									class: "ghost small",
									"data-write-to": p.address,
									title: `Write to ${p.name}`,
									onclick: () => {
										dlg.close();
										writeTo(p.address, p.name);
									},
								},
								"Message"
							)
				)
			);
		}
		if (!people.length) list.append(h("li", { class: "muted" }, "Nobody else just now."));
	};
	nameInput.addEventListener("change", () => {
		const name = nameInput.value.trim();
		if (!name) return;
		setLastName(name);
		saved.textContent = "Saved.";
		beatHere();
	});
	btn.addEventListener("click", () => {
		nameInput.value = lastName();
		saved.textContent = "";
		paint(whoIsHere());
		open(dlg);
		watchHere(true);
	});
	dlg.addEventListener("close", () => watchHere(false));
	const stop = onHereChange(paint);
	onLeave(stop);
	return btn;
}

/**
 * The button for a voice call: there while a call is up, wherever the player is
 * (another game, the main page, the profile), with the room code when the call
 * is in a game other than the one on screen. It opens a small panel with the
 * controls of the call and a way back to its game.
 */
function voiceChip() {
	const dot = h("span", { class: "voice-live-dot", "aria-hidden": "true" });
	const code = h("span", { class: "voice-chip-code", id: "voice-chip-code" });
	const chip = h("button", { type: "button", class: "topbar-icon voice-chip", id: "voice-chip", hidden: true, "aria-haspopup": "true", "aria-expanded": "false" }, micIcon(), dot, code);
	const pop = h("div", { class: "voice-pop", id: "voice-pop", hidden: true, role: "group", "aria-label": "Voice chat" });
	document.body.append(pop);

	const paintPop = (st) => {
		clear(pop);
		const here = st.viewing && st.viewing === st.code;
		pop.append(
			h("p", { class: "voice-pop-title" }, h("strong", {}, "Voice chat"), " · ", here ? "this game" : `game ${st.code}`),
			h("p", { class: "small muted" }, st.joined ? (st.peers ? `${st.peers} other${st.peers === 1 ? "" : "s"} in the call.` : "Nobody else is in the call.") : "Joining…")
		);
		const row = h("div", { class: "row voice-pop-buttons" });
		if (!here) row.append(h("a", { class: "button primary", id: "voice-pop-go", href: `/game/${st.code}` }, "Go to the game"));
		row.append(
			h("button", { type: "button", id: "voice-pop-mute", "aria-pressed": st.muted ? "true" : "false", class: st.muted ? "danger" : "", onclick: toggleMute }, st.muted ? "Unmute" : "Mute"),
			h("button", { type: "button", id: "voice-pop-deafen", "aria-pressed": st.deafened ? "true" : "false", class: st.deafened ? "danger" : "", onclick: toggleDeafen }, st.deafened ? "Undeafen" : "Deafen"),
			h("button", { type: "button", id: "voice-pop-leave", class: "ghost", onclick: () => leaveVoice() }, "Leave")
		);
		pop.append(row);
	};
	const close = () => {
		pop.hidden = true;
		chip.setAttribute("aria-expanded", "false");
	};
	const update = (st) => {
		chip.hidden = !st.active;
		if (!st.active) {
			close();
			return;
		}
		const elsewhere = st.viewing !== st.code;
		chip.classList.toggle("elsewhere", elsewhere);
		chip.classList.toggle("muted", st.muted || st.deafened);
		code.textContent = elsewhere ? st.code : "";
		const label = `In the voice chat of game ${st.code}` + (st.deafened ? " (deafened)" : st.muted ? " (muted)" : "");
		chip.title = label;
		chip.setAttribute("aria-label", label);
		if (!pop.hidden) paintPop(st);
	};
	chip.addEventListener("click", (ev) => {
		ev.stopPropagation();
		if (!pop.hidden) return close();
		paintPop(voiceStatus());
		const r = chip.getBoundingClientRect();
		pop.style.top = `${Math.round(r.bottom + 6)}px`;
		pop.style.right = `${Math.max(8, Math.round(window.innerWidth - r.right))}px`;
		pop.hidden = false;
		chip.setAttribute("aria-expanded", "true");
	});
	document.addEventListener("click", (ev) => {
		if (!pop.hidden && !pop.contains(ev.target)) close();
	});
	document.addEventListener("keydown", (ev) => {
		if (ev.key === "Escape" && !pop.hidden) close();
	});
	const stop = onVoiceChange(update);
	onLeave(() => {
		stop();
		pop.remove();
	});
	update(voiceStatus());
	return chip;
}
