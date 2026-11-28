// The profile page: the player's settings (name, email, notifications) and the
// games they have finished, kept out of the way of the home page.

import { clear, lastName, setLastName, getProfile, saveProfile } from "./util.js";
import { installTopbar } from "./topbar.js";
import { isFinished, sortRows, gameRow } from "./mygames.js";
import { installNav } from "./nav.js";
import { api, whoAmI } from "./api.js";
import { ratingText } from "./ratings.js";
import { paintShelf } from "./badges.js";
import { notifyConfig, localContact, submitAddress, contactStatus, resendConfirmation, forgetAddress, statusText, whitelistNote, insecureNote } from "./notify-ui.js";
import { t, tParts, forwardKey, backKey } from "./i18n.js";
import { languageField, translateField } from "./language.js";

installNav();

// ---- tabs: Settings, Badges (an account's) and Finished Games ----------------

const TABS = ["settings", "badges", "finished"];

/** Shows one tab, named in the page's address (#badges) so a link can open it. */
function showTab(name, { focus = false } = {}) {
	const open = (t) => !document.getElementById("tab-" + t).hidden;
	if (!TABS.includes(name) || !open(name)) name = "settings";
	for (const t of TABS) {
		const tab = document.getElementById("tab-" + t);
		const on = t === name;
		tab.setAttribute("aria-selected", on ? "true" : "false");
		tab.tabIndex = on ? 0 : -1;
		document.getElementById("panel-" + t).hidden = !on;
		if (on && focus) tab.focus();
	}
	const url = new URL(window.location.href);
	url.hash = name === "settings" ? "" : name;
	window.history.replaceState(null, "", url);
}

for (const tab of TABS.map((t) => document.getElementById("tab-" + t))) {
	tab.addEventListener("click", () => showTab(tab.dataset.tab));
	tab.addEventListener("keydown", (ev) => {
		const shown = TABS.filter((t) => !document.getElementById("tab-" + t).hidden);
		const i = shown.indexOf(tab.dataset.tab);
		const n = shown.length;
		const to = { [forwardKey]: (i + 1) % n, [backKey]: (i - 1 + n) % n, Home: 0, End: n - 1 }[ev.key];
		if (to === undefined) return;
		ev.preventDefault();
		showTab(shown[to], { focus: true });
	});
}
// The tab the address asked for: Badges waits for the account to be known.
const askedTab = window.location.hash.slice(1);
showTab(askedTab);

/** What an account chose about its friends. */
function paintFriendSettings() {
	const mail = document.getElementById("friend-mail");
	const presence = document.getElementById("friend-presence");
	const invites = document.getElementById("friend-invites");
	const status = document.getElementById("friend-settings-status");
	api.friendSettings()
		.then((settings) => {
			mail.checked = settings.mail_requests;
			presence.checked = settings.show_presence;
			invites.checked = settings.accept_invites;
			document.getElementById("friends-card").hidden = false;
			const wire = (box, key) =>
				box.addEventListener("change", () => {
					status.textContent = "";
					api.setFriendSettings({ [key]: box.checked })
						.then(() => (status.textContent = t("common.saved")))
						.catch((err) => {
							box.checked = !box.checked;
							status.textContent = err.message || t("common.not_saved");
						});
				});
			wire(mail, "mail_requests");
			wire(presence, "show_presence");
			wire(invites, "accept_invites");
		})
		.catch(() => {});
}

/** Those the account has blocked (ADR 0156), each with a way to lift it. Nobody else sees them. */
function paintBlocks() {
	const box = document.getElementById("blocked-box");
	const list = document.getElementById("blocked-list");
	const status = document.getElementById("friend-settings-status");
	api.friendBlocks()
		.then(({ blocked }) => {
			clear(list);
			box.hidden = !blocked.length;
			for (const b of blocked) {
				const name = document.createElement("span");
				name.className = "grow";
				name.textContent = b.name;
				const undo = document.createElement("button");
				undo.type = "button";
				undo.className = "ghost small";
				undo.dataset.unblock = b.handle;
				undo.textContent = t("settings.unblock");
				undo.setAttribute("aria-label", t("settings.unblock_who", { name: b.name }));
				undo.addEventListener("click", () => {
					status.textContent = "";
					api.friendUnblock(b.handle)
						.then(paintBlocks)
						.catch((err) => (status.textContent = err.message || t("common.not_saved")));
				});
				const row = document.createElement("li");
				row.append(name, undo);
				list.append(row);
			}
		})
		.catch(() => {});
}

/** An account's own ratings, including those not on a leaderboard yet. */
function paintRatings(handle) {
	api.playerRatings(handle)
		.then(({ ratings }) => {
			if (!ratings.length) return;
			const rows = document.getElementById("ratings-rows");
			for (const r of ratings) {
				const row = document.createElement("tr");
				row.dataset.family = r.family;
				for (const text of [r.name, ratingText(r.rating, r.provisional), r.won, r.drawn, r.lost, r.listed ? "" : t("settings.not_listed")]) {
					const cell = document.createElement("td");
					cell.textContent = String(text);
					row.append(cell);
				}
				rows.append(row);
			}
			document.getElementById("ratings-note").textContent = t("settings.ratings_note");
			document.getElementById("ratings-box").hidden = false;
		})
		.catch(() => {});
}

/**
 * An account's own badges (ADR 0130), with those others see only at their
 * game's end, and greyed out, those it has yet to earn.
 */
function paintBadges(handle) {
	api.playerBadges(handle)
		.then(async ({ badges }) => {
			document.getElementById("badges-empty").hidden = badges.length > 0;
			document.getElementById("badges-page").href = `/players/${encodeURIComponent(handle)}`;
			document.getElementById("badges-box").hidden = false;
			// Badges are an account's: their tab shows once there is one.
			document.getElementById("tab-badges").hidden = false;
			if (askedTab === "badges") showTab("badges");
			await paintShelf(document.getElementById("badges-list"), { held: badges, mine: true });
		})
		.catch(() => {});
}

// Who this browser is: an account, or a guest who could sign in (ADR 0068).
api.me()
	.then((me) => {
		if (!me.accounts) return;
		const box = document.getElementById("account-box");
		const line = document.getElementById("account-line");
		const link = (href, text, id) => {
			const a = document.createElement("a");
			a.href = href;
			a.id = id;
			a.textContent = text;
			return a;
		};
		if (me.kind === "account") {
			line.append(...tParts("settings.signed_in_as", { handle: me.handle, account: link("/account", t("common.your_account"), "account-link") }));
			paintBadges(me.handle);
			paintRatings(me.handle);
			paintFriendSettings();
			paintBlocks();
		} else {
			// A guest whose browser confirmed an address: that is the
			// quickest way to an account (ADR 0073).
			if (localContact() && !(me.caps || []).includes("notify")) document.getElementById("upgrade-box").hidden = false;
			const signin = link("/login", t("common.sign_in"), "login-link");
			if (me.signup) line.append(...tParts("settings.guest_signup", { signin, signup: link("/signup", t("settings.sign_up_lower"), "signup-link") }));
			else line.append(...tParts("settings.guest", { signin }));
		}
		box.hidden = false;
	})
	.catch(() => {});

document.getElementById("profile-language").append(languageField("profile_language", { label: false }), translateField("profile_translate"));

const name = document.getElementById("profile_name");
const email = document.getElementById("profile_email");
const notify = document.getElementById("profile_notify");
const form = document.getElementById("profile-form");
const status = document.getElementById("profile-status");

const profile = getProfile();
name.value = lastName();
email.value = profile.email;
notify.checked = profile.notify;

const say = (text, kind = "muted") => {
	status.textContent = text;
	status.className = "small " + kind;
};

// ---- email notifications ------------------------------------------------------------

const box = document.getElementById("notify-box");
const line = document.getElementById("notify-status");
const actions = document.getElementById("notify-actions");
const notes = document.getElementById("notify-notes");
let server = { enabled: false };
let poll = null;

async function paintContact() {
	if (!server.enabled) return;
	let info = null;
	try {
		info = await contactStatus();
	} catch (_) {
		line.textContent = t("common.no_server");
		return;
	}
	line.textContent = statusText(info);
	actions.hidden = !info;
	document.getElementById("notify-resend").hidden = !info || info.status !== "pending";
	// While the address waits for its confirmation, watch for it.
	clearInterval(poll);
	poll = null;
	if (info && info.status === "pending") {
		poll = setInterval(() => {
			if (!document.hidden) paintContact();
		}, 5000);
	}
}

(async () => {
	const me = await whoAmI();
	// A guest cannot have email unless this server lets guests: then nothing is said of it.
	document.getElementById("profile-email-section").hidden = me.kind !== "account" && !(me.caps || []).includes("notify");
	server = await notifyConfig();
	if (!server.enabled) {
		document.getElementById("notify-off").hidden = false;
		return;
	}
	// The address and the switch are an account's to set (notify).
	if (!(me.caps || []).includes("notify")) {
		document.getElementById("profile-email-field").hidden = true;
		document.getElementById("profile-notify-field").hidden = true;
		document.getElementById("notify-accounts-only").hidden = false;
		server = { enabled: false };
		return;
	}
	box.hidden = false;
	clear(notes);
	notes.append(whitelistNote(server.from));
	const warn = insecureNote();
	if (warn) notes.append(warn);
	paintContact();
})();

document.getElementById("notify-resend").addEventListener("click", async () => {
	try {
		const r = await resendConfirmation();
		line.textContent = r && r.confirmation_sent ? t("settings.resent", { address: r.masked }) : (r && r.note) || statusText(r);
	} catch (err) {
		line.textContent = err.message;
	}
});

document.getElementById("notify-forget").addEventListener("click", async () => {
	try {
		await forgetAddress();
		email.value = "";
		saveProfile({ email: "" });
		line.textContent = t("settings.forgotten");
		actions.hidden = true;
		clearInterval(poll);
	} catch (err) {
		line.textContent = err.message;
	}
});

form.addEventListener("submit", async (ev) => {
	ev.preventDefault();
	const address = email.value.trim();
	if (address && !email.checkValidity()) {
		say(t("settings.not_an_address"), "error");
		email.focus();
		return;
	}
	setLastName(name.value.trim());
	saveProfile({ email: address, notify: notify.checked });
	say(t("common.saved"));
	if (!server.enabled) return;
	const have = localContact();
	if (!address) {
		if (have) await forgetAddress().catch(() => {});
		paintContact();
		return;
	}
	if (have && have.address && have.address.toLowerCase() === address.toLowerCase()) {
		paintContact();
		return;
	}
	try {
		const r = await submitAddress(address);
		line.textContent = r.confirmation_sent ? t("settings.sent", { address: r.masked }) : r.note || statusText(r);
		say(t("common.saved"));
		paintContact();
	} catch (err) {
		say(err.message || t("settings.not_sent"), "error");
	}
});

// ---- finished games --------------------------------------------------------------------

const list = document.getElementById("finished-games");
const empty = document.getElementById("finished-empty");

let current = [];

function paint(rows, defs) {
	current = rows;
	const done = sortRows(rows.filter(isFinished));
	clear(list);
	for (const r of done) list.append(gameRow(r, defs, () => paint(current.filter((x) => x.g.code !== r.g.code), defs)));
	empty.hidden = done.length > 0;
}

installTopbar({ onRows: paint });

// ---- a confirmed address becomes an account -----------------------------------------------

document.getElementById("upgrade-form").addEventListener("submit", async (ev) => {
	ev.preventDefault();
	const c = localContact();
	const out = document.getElementById("upgrade-result");
	const go = document.getElementById("upgrade-go");
	if (!c) return;
	go.disabled = true;
	try {
		await api.signupWithContact(c.id, c.secret, document.getElementById("upgrade-handle").value, document.getElementById("upgrade-password").value);
		location.reload();
	} catch (err) {
		out.className = "small error";
		out.textContent = err.message || t("common.not_done");
	} finally {
		go.disabled = false;
	}
});
