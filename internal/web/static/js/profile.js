// The profile page: the player's settings (name, email, notifications) and the
// games they have finished, kept out of the way of the home page.

import { clear, lastName, setLastName, getProfile, saveProfile } from "./util.js";
import { installTopbar } from "./topbar.js";
import { isFinished, sortRows, gameRow } from "./mygames.js";
import { installNav } from "./nav.js";
import { api, whoAmI } from "./api.js";
import { ratingText } from "./ratings.js";
import { notifyConfig, localContact, submitAddress, contactStatus, resendConfirmation, forgetAddress, statusText, whitelistNote, insecureNote } from "./notify-ui.js";

installNav();

/** What an account chose about its friends. */
function paintFriendSettings() {
	const box = document.getElementById("friend-mail");
	const status = document.getElementById("friend-settings-status");
	api.friendSettings()
		.then((settings) => {
			box.checked = settings.mail_requests;
			document.getElementById("friends-card").hidden = false;
			box.addEventListener("change", () => {
				status.textContent = "";
				api.setFriendSettings({ mail_requests: box.checked })
					.then(() => (status.textContent = "Saved."))
					.catch((err) => {
						box.checked = !box.checked;
						status.textContent = err.message || "That did not save.";
					});
			});
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
				for (const text of [r.name, ratingText(r.rating, r.provisional), r.won, r.drawn, r.lost, r.listed ? "" : "Not listed yet"]) {
					const cell = document.createElement("td");
					cell.textContent = String(text);
					row.append(cell);
				}
				rows.append(row);
			}
			document.getElementById("ratings-note").textContent = `A question mark means the rating is provisional. You appear on a game's leaderboard after 5 rated games against at least 3 different people.`;
			document.getElementById("ratings-box").hidden = false;
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
			line.append(`Signed in as @${me.handle}. `, link("/account", "Your account", "account-link"));
			paintRatings(me.handle);
			paintFriendSettings();
		} else {
			// A guest whose browser confirmed an address: that is the
			// quickest way to an account (ADR 0073).
			if (localContact() && !(me.caps || []).includes("notify")) document.getElementById("upgrade-box").hidden = false;
			line.append("You play as a guest in this browser. ", link("/login", "Sign in", "login-link"));
			if (me.signup) line.append(" or ", link("/signup", "sign up", "signup-link"), " to keep your games on every device.");
		}
		box.hidden = false;
	})
	.catch(() => {});

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
		line.textContent = "Could not reach the server.";
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
		line.textContent = r && r.confirmation_sent ? `A new confirmation was sent to ${r.masked}.` : (r && r.note) || statusText(r);
	} catch (err) {
		line.textContent = err.message;
	}
});

document.getElementById("notify-forget").addEventListener("click", async () => {
	try {
		await forgetAddress();
		email.value = "";
		saveProfile({ email: "" });
		line.textContent = "Your address was deleted from the server.";
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
		say("That does not look like an email address.", "error");
		email.focus();
		return;
	}
	setLastName(name.value.trim());
	saveProfile({ email: address, notify: notify.checked });
	say("Saved.");
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
		line.textContent = r.confirmation_sent ? `We sent a confirmation link to ${r.masked}. Open it and press Confirm (look in the spam folder too).` : r.note || statusText(r);
		say("Saved.");
		paintContact();
	} catch (err) {
		say(err.message || "The address could not be sent.", "error");
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
		out.textContent = err.message || "That did not work.";
	} finally {
		go.disabled = false;
	}
});
