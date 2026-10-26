// The icons of the bar at the top of every page: the active games (with a
// count of those waiting for you), joining a game by its room code, the
// player's profile, help (the version, what changed lately, the project's page,
// where to report problems) and — while a voice call is up — a button for the call.

import { h, s, clear, toast, lastName, rememberGame, setTurnBadge, store } from "./util.js";
import { api } from "./api.js";
import { gameDefs, loadRows, isFinished, yourTurn, sortRows, gameRow } from "./mygames.js";
import { navigate, onLeave } from "./nav.js";
import { voiceStatus, onVoiceChange, toggleMute, toggleDeafen, leaveVoice } from "./voice.js";

const icon = (...paths) =>
	s("svg", { viewBox: "0 0 24 24", width: 20, height: 20, fill: "none", stroke: "currentColor", "stroke-width": 1.8, "stroke-linecap": "round", "stroke-linejoin": "round", "aria-hidden": "true" }, ...paths);

// A game controller.
const gamesIcon = () =>
	icon(
		s("path", { d: "M6.5 8h11a4 4 0 0 1 3.9 4.9l-.9 4a2.6 2.6 0 0 1-4.4 1.2L14 16.5h-4L7.9 18.1a2.6 2.6 0 0 1-4.4-1.2l-.9-4A4 4 0 0 1 6.5 8z"}),
		s("path", { d: "M8 10.8v3.4M6.3 12.5h3.4" }),
		s("circle", { cx: 15.6, cy: 11.6, r: 0.5 }),
		s("circle", { cx: 17.6, cy: 13.4, r: 0.5 })
	);
// A door with an arrow going in.
const joinIcon = () => icon(s("path", { d: "M14 4h4a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2h-4" }), s("path", { d: "M10 8l4 4-4 4M14 12H3" }));
// A microphone.
const micIcon = () => icon(s("rect", { x: 9, y: 3, width: 6, height: 11, rx: 3 }), s("path", { d: "M5.5 11a6.5 6.5 0 0 0 13 0M12 17.5V21" }));
// A question mark in a circle.
const helpIcon = () => icon(s("circle", { cx: 12, cy: 12, r: 9 }), s("path", { d: "M9.4 9.4a2.7 2.7 0 0 1 5.2 1c0 1.8-2.6 2.2-2.6 4M12 17.4v.1" }));
// A head and shoulders.
const profileIcon = () => icon(s("circle", { cx: 12, cy: 8, r: 3.6 }), s("path", { d: "M4.5 20c.8-4 3.7-6 7.5-6s6.7 2 7.5 6" }));

const REPO_URL = "https://github.com/noderunner/bingle";

// The version of the changelog this browser has shown its player (the Recent Changes of the help window).
const CHANGES_SEEN_KEY = "bingle.changelog.seen";

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
	const activeDlg = dialog("active-games-dialog", "Active games", list, empty);
	let rows = [];
	let defs = [];

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

	// ---- profile ---------------------------------------------------------------
	const profile = h("a", { class: "topbar-icon", id: "open-profile", href: "/profile", title: "Your profile", "aria-label": "Your profile" }, profileIcon());
	if (window.location.pathname === "/profile") profile.setAttribute("aria-current", "page");

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
	host.append(voice, activeBtn, joinBtn, profile, helpBtn);

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
