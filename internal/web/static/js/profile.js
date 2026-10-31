// The profile page: the player's settings (name, email, notifications) and the
// games they have finished, kept out of the way of the home page.

import { clear, lastName, setLastName, getProfile, saveProfile } from "./util.js";
import { installTopbar } from "./topbar.js";
import { isFinished, sortRows, gameRow } from "./mygames.js";
import { installNav } from "./nav.js";
import { api } from "./api.js";
import { notifyConfig, localContact, submitAddress, contactStatus, resendConfirmation, forgetAddress, statusText, whitelistNote, insecureNote } from "./notify-ui.js";

installNav();

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
		} else {
			document.getElementById("guest-box").hidden = me.kind !== "guest";
			// A guest whose browser confirmed an address: that is the
			// quickest way to an account (ADR 0073).
			if (localContact() && !(me.caps || []).includes("notify")) document.getElementById("upgrade-box").hidden = false;
			line.append("You play as a guest in this browser. ", link("/login", "Sign in", "login-link"));
			if (me.signup) line.append(" or ", link("/signup", "sign up", "signup-link"), " to keep your games on every device.");
		}
		box.hidden = false;
	})
	.catch(() => {});

// ---- the guest key ------------------------------------------------------------------

{
	const el = (id) => document.getElementById(id);
	const sayGuest = (text, kind = "") => {
		el("guest-result").textContent = text;
		el("guest-result").className = "small " + kind;
	};
	const run = async (button, f) => {
		button.disabled = true;
		sayGuest("");
		try {
			await f();
		} catch (err) {
			sayGuest(err.message || "That did not work.", "error");
		} finally {
			button.disabled = false;
		}
	};
	el("guest-copy").addEventListener("click", () =>
		run(el("guest-copy"), async () => {
			const { key, link } = await api.guestKey();
			const url = location.origin + link;
			el("guest-link").textContent = url;
			el("guest-key").textContent = key;
			el("guest-shown").hidden = false;
			try {
				await navigator.clipboard.writeText(url);
				sayGuest("The link is copied.");
			} catch (_) {
				sayGuest("Copy the link or the key by hand.");
			}
		})
	);
	el("guest-rotate").addEventListener("click", () =>
		run(el("guest-rotate"), async () => {
			await api.rotateGuest();
			el("guest-shown").hidden = true;
			sayGuest("This browser has a new key. The old one, wherever you copied it, no longer works; your games are still yours.");
		})
	);
	el("guest-discard").addEventListener("click", () =>
		run(el("guest-discard"), async () => {
			if (!window.confirm("Start over as a new guest? This browser loses its seats: the games you play now stay with the old guest, which nobody can be again unless you copied its key.")) return;
			await api.discardGuest();
			sayGuest("This browser is a new guest from its next game on.");
		})
	);
	el("guest-use").addEventListener("click", () =>
		run(el("guest-use"), async () => {
			const key = el("guest-use-key").value.trim();
			await api.adoptGuest(key);
			el("guest-use-key").value = "";
			sayGuest("This browser is that guest now: its games are under Active games, and this browser's own came along.");
		})
	);
}

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
	server = await notifyConfig();
	if (!server.enabled) {
		document.getElementById("notify-off").hidden = false;
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
