// The profile page: the player's settings (name, email, notifications) and the
// games they have finished, kept out of the way of the home page.

import { clear, lastName, setLastName, getProfile, saveProfile } from "./util.js";
import { installTopbar } from "./topbar.js";
import { isFinished, sortRows, gameRow } from "./mygames.js";
import { installNav } from "./nav.js";
import { notifyConfig, localContact, submitAddress, contactStatus, resendConfirmation, forgetAddress, statusText, whitelistNote, insecureNote } from "./notify-ui.js";

installNav();

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
