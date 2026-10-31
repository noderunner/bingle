// The account pages: sign in, sign up, confirm an account's address, and the
// account itself (who you are, a new password, signing out). ADR 0068.

import { api, ApiError } from "./api.js";

const page = document.body.dataset.page;
const result = document.getElementById("account-result");

const say = (text, kind = "") => {
	if (!result) return;
	result.textContent = text;
	result.className = "small " + kind;
};

// Only a path on this server is a place to go back to after signing in.
function returnTo() {
	const to = new URLSearchParams(location.search).get("return") || "/";
	return to.startsWith("/") && !to.startsWith("//") ? to : "/";
}

async function submit(form, button, run) {
	form.addEventListener("submit", async (ev) => {
		ev.preventDefault();
		button.disabled = true;
		say("");
		try {
			await run();
		} catch (err) {
			say(err instanceof ApiError ? err.message : "That did not work.", "error");
		} finally {
			button.disabled = false;
		}
	});
}

if (page === "login") {
	const form = document.getElementById("login-form");
	api.me()
		.then((me) => {
			document.getElementById("login-signup").hidden = !me.signup;
		})
		.catch(() => {});
	submit(form, document.getElementById("login-go"), async () => {
		const me = await api.login(document.getElementById("login-handle").value, document.getElementById("login-password").value);
		location.assign(me.must_change_password ? "/account" : returnTo());
	});
} else if (page === "signup") {
	const form = document.getElementById("signup-form");
	submit(form, document.getElementById("signup-go"), async () => {
		const res = await api.signup(document.getElementById("signup-handle").value, document.getElementById("signup-email").value, document.getElementById("signup-password").value);
		say(res.status);
		form.querySelectorAll("input").forEach((i) => (i.disabled = true));
		document.getElementById("signup-go").hidden = true;
	});
} else if (page === "confirm") {
	const go = document.getElementById("confirm-go");
	const token = new URLSearchParams(location.search).get("t") || "";
	if (!token) {
		go.disabled = true;
		say("This link is incomplete.", "error");
	}
	go.addEventListener("click", async () => {
		go.disabled = true;
		try {
			const res = await api.confirmAccount(token);
			go.hidden = true;
			result.className = "small";
			result.textContent = "";
			result.append(`@${res.handle} is ready. `);
			const a = document.createElement("a");
			a.href = "/login";
			a.textContent = "Sign in";
			result.append(a, ".");
		} catch (err) {
			say(err instanceof ApiError ? err.message : "That did not work.", "error");
			go.disabled = false;
		}
	});
} else {
	const who = document.getElementById("account-who");
	const show = (me) => {
		if (me.kind !== "account") {
			location.replace("/login?return=/account");
			return;
		}
		who.textContent = `Signed in as @${me.handle}.`;
		document.getElementById("account-must-change").hidden = !me.must_change_password;
	};
	api.me().then(show).catch(() => say("Your account could not be read.", "error"));
	const form = document.getElementById("password-form");
	submit(form, document.getElementById("password-go"), async () => {
		const me = await api.changePassword(document.getElementById("password-old").value, document.getElementById("password-new").value);
		form.reset();
		show(me);
		say("Done: your password is changed, and every other device is signed out.");
	});
	document.getElementById("logout-go").addEventListener("click", async () => {
		try {
			await api.logout();
		} finally {
			location.assign("/");
		}
	});
}
