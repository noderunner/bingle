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
	const second = document.getElementById("second-form");
	const after = (me) => location.assign(me.must_change_password || me.needs_second_factor ? "/account" : returnTo());
	submit(form, document.getElementById("login-go"), async () => {
		const me = await api.login(document.getElementById("login-handle").value, document.getElementById("login-password").value);
		if (me.second_factor && !me.kind) {
			// The password was right; now the code from the app.
			form.hidden = true;
			second.hidden = false;
			document.getElementById("second-code").focus();
			return;
		}
		after(me);
	});
	second.addEventListener("submit", async (ev) => {
		ev.preventDefault();
		const go = document.getElementById("second-go");
		const out = document.getElementById("second-result");
		go.disabled = true;
		out.textContent = "";
		try {
			after(await api.secondFactor(document.getElementById("second-code").value));
		} catch (err) {
			out.className = "small error";
			out.textContent = err instanceof ApiError ? err.message : "That did not work.";
			if (err instanceof ApiError && /sign in again/.test(err.message)) {
				second.hidden = true;
				form.hidden = false;
			}
		} finally {
			go.disabled = false;
		}
	});
} else if (page === "signup") {
	const form = document.getElementById("signup-form");
	submit(form, document.getElementById("signup-go"), async () => {
		const res = await api.signup(document.getElementById("signup-handle").value, document.getElementById("signup-email").value, document.getElementById("signup-password").value);
		say(res.status);
		form.querySelectorAll("input").forEach((i) => (i.disabled = true));
		document.getElementById("signup-go").hidden = true;
	});
} else if (page === "forgot") {
	const form = document.getElementById("forgot-form");
	submit(form, document.getElementById("forgot-go"), async () => {
		const res = await api.forgot(document.getElementById("forgot-who").value);
		say(res.status);
		document.getElementById("forgot-go").hidden = true;
	});
} else if (page === "reset") {
	const form = document.getElementById("reset-form");
	const token = new URLSearchParams(location.search).get("t") || "";
	if (!token) say("This link is incomplete.", "error");
	submit(form, document.getElementById("reset-go"), async () => {
		const res = await api.resetPassword(token, document.getElementById("reset-password").value, document.getElementById("reset-code").value);
		form.querySelectorAll("input").forEach((i) => (i.disabled = true));
		document.getElementById("reset-go").hidden = true;
		result.className = "small";
		result.textContent = res.status + " ";
		const a = document.createElement("a");
		a.href = "/login";
		a.textContent = "Sign in";
		result.append(a);
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
	const el = (id) => document.getElementById(id);
	const tfSay = (text, kind = "") => {
		el("tf-result").textContent = text;
		el("tf-result").className = "small " + kind;
	};
	const show = (me) => {
		if (me.kind !== "account") {
			location.replace("/login?return=/account");
			return;
		}
		who.textContent = `Signed in as @${me.handle}.`;
		el("account-must-change").hidden = !me.must_change_password;
		el("tf-needed").hidden = !me.needs_second_factor;
		el("tf-state").textContent = me.second_factor
			? `On: you sign in with a code from your app as well. ${me.recovery_codes_left} recovery code${me.recovery_codes_left === 1 ? "" : "s"} left.`
			: "Off: you sign in with your password alone.";
		el("tf-off-actions").hidden = !!me.second_factor;
		el("tf-on-actions").hidden = !me.second_factor;
	};
	// A request that may fail with a message for the two-step box.
	const tf = async (button, run) => {
		button.disabled = true;
		tfSay("");
		try {
			await run();
		} catch (err) {
			tfSay(err instanceof ApiError ? err.message : "That did not work.", "error");
		} finally {
			button.disabled = false;
		}
	};
	const showCodes = (codes) => {
		const list = el("tf-codes");
		list.textContent = "";
		for (const c of codes) {
			const li = document.createElement("li");
			li.textContent = c;
			list.append(li);
		}
		el("tf-codes-box").hidden = false;
	};
	el("tf-setup").addEventListener("click", () =>
		tf(el("tf-setup"), async () => {
			const res = await api.totpSetup(el("tf-password").value);
			el("tf-secret").textContent = res.secret;
			el("tf-uri").textContent = res.uri;
			el("tf-setting-up").hidden = false;
			el("tf-code").focus();
		})
	);
	el("tf-enable").addEventListener("click", () =>
		tf(el("tf-enable"), async () => {
			const res = await api.totpEnable(el("tf-code").value);
			el("tf-setting-up").hidden = true;
			el("tf-password").value = "";
			showCodes(res.recovery_codes);
			show(res.me);
		})
	);
	el("tf-renew").addEventListener("click", () =>
		tf(el("tf-renew"), async () => {
			const res = await api.newRecoveryCodes(el("tf-password").value);
			el("tf-password").value = "";
			showCodes(res.recovery_codes);
			show(await api.me());
		})
	);
	el("tf-off").addEventListener("click", () =>
		tf(el("tf-off"), async () => {
			const me = await api.totpDisable(el("tf-password").value, el("tf-off-code").value);
			el("tf-password").value = "";
			el("tf-off-code").value = "";
			el("tf-codes-box").hidden = true;
			show(me);
			tfSay("Two-step sign-in is off.");
		})
	);
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
