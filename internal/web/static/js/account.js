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

const params = new URLSearchParams(location.search);

// providerButtons fills host with a "Sign in with …" link per provider.
function providerButtons(host, providers, verb) {
	if (!host || !providers || !providers.length) return;
	for (const p of providers) {
		const a = document.createElement("a");
		a.className = "button ghost";
		a.id = `oidc-${p.id}`;
		const invite = params.get("invite");
		a.href = `/auth/oidc/${encodeURIComponent(p.id)}/start?return=${encodeURIComponent(returnTo())}${invite ? `&invite=${encodeURIComponent(invite)}` : ""}`;
		a.textContent = `${verb} with ${p.label}`;
		host.append(a);
	}
	host.hidden = false;
}

function showError(id) {
	const el = document.getElementById(id);
	const msg = params.get("error");
	if (el && msg) {
		el.textContent = msg.replace(/^auth: /, "");
		el.hidden = false;
	}
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
	showError("login-error");
	api.me()
		.then((me) => {
			document.getElementById("login-signup").hidden = !me.signup;
			providerButtons(document.getElementById("oidc-buttons"), me.providers, "Sign in");
		})
		.catch(() => {});
	const second = document.getElementById("second-form");
	// A provider signed the person in; the code from the app is next.
	if (params.get("second") === "1") {
		form.hidden = true;
		second.hidden = false;
	}
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
	const provider = params.get("provider");
	api.me()
		.then((me) => {
			if (!provider) providerButtons(document.getElementById("signup-oidc-buttons"), me.providers, "Sign up");
			const p = (me.providers || []).find((x) => x.id === provider);
			if (p) document.getElementById("oidc-signup-who").textContent = `${p.label} vouched for your email address. Choose the handle others will see.`;
		})
		.catch(() => {});
	if (provider) {
		// A provider vouched for the person: only the handle is left.
		form.hidden = true;
		const of = document.getElementById("oidc-signup-form");
		of.hidden = false;
		of.addEventListener("submit", async (ev) => {
			ev.preventDefault();
			const go = document.getElementById("oidc-go");
			const out = document.getElementById("oidc-result");
			go.disabled = true;
			try {
				const res = await api.oidcFinish(document.getElementById("oidc-handle").value);
				location.assign(res.return || "/");
			} catch (err) {
				out.className = "small error";
				out.textContent = err instanceof ApiError ? err.message : "That did not work.";
			} finally {
				go.disabled = false;
			}
		});
	}
	submit(form, document.getElementById("signup-go"), async () => {
		const res = await api.signup(document.getElementById("signup-handle").value, document.getElementById("signup-email").value, document.getElementById("signup-password").value, params.get("invite"));
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
		if (me.email) el("account-email").textContent = `Your address: ${me.email}`;
		el("account-must-change").hidden = !me.must_change_password;
		el("tf-needed").hidden = !me.needs_second_factor;
		el("tf-state").textContent = me.second_factor
			? `On: you sign in with a code from your app as well. ${me.recovery_codes_left} recovery code${me.recovery_codes_left === 1 ? "" : "s"} left.`
			: "Off: you sign in with your password alone.";
		el("tf-off-actions").hidden = !!me.second_factor;
		el("tf-on-actions").hidden = !me.second_factor;
		// An account a provider made has no password yet: it sets one.
		el("password-old-field").hidden = !me.password;
		el("password-title").textContent = me.password ? "Change your password" : "Set a password";
		showLinks(me);
		el("admin-link").hidden = !me.admin;
	};
	const paintTokens = async () => {
		const { tokens, scopes } = await api.tokens();
		const list = el("tokens-list");
		list.textContent = "";
		for (const t of tokens) {
			const li = document.createElement("li");
			li.append(`${t.name} (${t.scopes.join(", ")}) `);
			const b = document.createElement("button");
			b.type = "button";
			b.className = "ghost";
			b.id = `token-revoke-${t.id}`;
			b.textContent = "Revoke";
			b.addEventListener("click", async () => {
				await api.revokeToken(t.id);
				paintTokens();
			});
			li.append(b);
			list.append(li);
		}
		const host = el("token-scopes");
		if (!host.childElementCount) {
			for (const s of scopes) {
				const label = document.createElement("label");
				label.className = "check";
				const box = document.createElement("input");
				box.type = "checkbox";
				box.id = `token-scope-${s}`;
				box.value = s;
				box.checked = s === "play";
				label.append(box, ` ${s.replace("_", " ")}`);
				host.append(label);
			}
		}
	};
	paintTokens().catch(() => {});
	const paintSessions = async () => {
		const { sessions } = await api.sessions();
		const list = el("sessions-list");
		list.textContent = "";
		for (const s of sessions) {
			const li = document.createElement("li");
			li.textContent = `${s.agent || "a browser"}, since ${s.created_at}, last seen ${s.last_seen_at}${s.current ? " (this one)" : ""}`;
			list.append(li);
		}
	};
	paintSessions().catch(() => {});
	el("sessions-end").addEventListener("click", async () => {
		await api.endOtherSessions();
		paintSessions();
	});
	el("handle-form").addEventListener("submit", async (ev) => {
		ev.preventDefault();
		const out = el("handle-result");
		try {
			show(await api.rename(el("handle-new").value));
			out.className = "small";
			out.textContent = "Done.";
			el("handle-new").value = "";
		} catch (err) {
			out.className = "small error";
			out.textContent = err instanceof ApiError ? err.message : "That did not work.";
		}
	});
	el("token-go").addEventListener("click", async () => {
		const scopes = [...el("token-scopes").querySelectorAll("input:checked")].map((b) => b.value);
		try {
			const res = await api.newToken(el("token-name").value, scopes);
			el("token-new").textContent = `Your new token, shown only now: ${res.token}`;
			el("token-name").value = "";
			paintTokens();
		} catch (err) {
			el("token-new").textContent = err instanceof ApiError ? err.message : "That did not work.";
		}
	});
	const showLinks = (me) => {
		const box = el("oidc-box");
		const list = el("oidc-links");
		if (!me.providers || !me.providers.length) return;
		list.textContent = "";
		for (const p of me.providers) {
			const li = document.createElement("li");
			const linked = (me.linked || []).includes(p.id);
			li.append(`${p.label}: ${linked ? "you can sign in with it. " : "not linked. "}`);
			if (linked) {
				const b = document.createElement("button");
				b.type = "button";
				b.className = "ghost";
				b.id = `oidc-unlink-${p.id}`;
				b.textContent = "Unlink";
				b.addEventListener("click", () =>
					tf(b, async () => {
						show(await api.oidcUnlink(p.id, el("tf-password").value));
						tfSay(`${p.label} is unlinked.`);
					})
				);
				li.append(b);
			} else {
				const a = document.createElement("a");
				a.id = `oidc-link-${p.id}`;
				a.href = `/auth/oidc/${encodeURIComponent(p.id)}/start?link=1&return=/account`;
				a.textContent = `Link ${p.label}`;
				li.append(a);
			}
			list.append(li);
		}
		box.hidden = false;
	};
	showError("oidc-error");
	if (params.get("linked")) tfSay(`Linked: you can sign in with ${params.get("linked")} now.`);
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
			// An image, not markup: an SVG in an <img> cannot run anything.
			el("tf-qr").hidden = !res.qr_svg;
			if (res.qr_svg) el("tf-qr").src = "data:image/svg+xml;charset=utf-8," + encodeURIComponent(res.qr_svg);
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
