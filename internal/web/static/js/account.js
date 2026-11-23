// The account pages: sign in, sign up, confirm an account's address, and the
// account itself (who you are, a new password, signing out). ADR 0068.

import { api, ApiError } from "./api.js";
import { t as tr, tParts, lang } from "./i18n.js";

// Data inside a sentence (an address, a browser's name): left as it is, and
// kept whole in a sentence written right to left.
const data = (text) => {
	const b = document.createElement("bdi");
	b.translate = false;
	b.textContent = text;
	return b;
};
// A moment, in the page's language.
const when = (iso) => {
	const d = new Date(iso);
	return isNaN(d) ? iso : d.toLocaleString(lang, { dateStyle: "medium", timeStyle: "short" });
};
// An API scope is the API's word, shown as code.
const code = (text) => {
	const c = document.createElement("code");
	c.textContent = text;
	return c;
};

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
		a.textContent = tr("account.with", { verb, provider: p.label });
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
			say(err instanceof ApiError ? err.message : tr("common.not_done"), "error");
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
			providerButtons(document.getElementById("oidc-buttons"), me.providers, tr("common.sign_in"));
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
			out.textContent = err instanceof ApiError ? err.message : tr("common.not_done");
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
			if (!provider) providerButtons(document.getElementById("signup-oidc-buttons"), me.providers, tr("common.sign_up"));
			const p = (me.providers || []).find((x) => x.id === provider);
			if (p) document.getElementById("oidc-signup-who").textContent = tr("account.vouched", { provider: p.label });
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
				out.textContent = err instanceof ApiError ? err.message : tr("common.not_done");
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
	if (!token) say(tr("account.incomplete"), "error");
	submit(form, document.getElementById("reset-go"), async () => {
		const res = await api.resetPassword(token, document.getElementById("reset-password").value, document.getElementById("reset-code").value);
		form.querySelectorAll("input").forEach((i) => (i.disabled = true));
		document.getElementById("reset-go").hidden = true;
		result.className = "small";
		result.textContent = res.status + " ";
		const a = document.createElement("a");
		a.href = "/login";
		a.textContent = tr("common.sign_in");
		result.append(a);
	});
} else if (page === "confirm") {
	const go = document.getElementById("confirm-go");
	const token = new URLSearchParams(location.search).get("t") || "";
	if (!token) {
		go.disabled = true;
		say(tr("account.incomplete"), "error");
	}
	go.addEventListener("click", async () => {
		go.disabled = true;
		try {
			const res = await api.confirmAccount(token);
			go.hidden = true;
			result.className = "small";
			result.textContent = "";
			result.append(...tParts("account.ready", { handle: data("@" + res.handle) }), " ");
			const a = document.createElement("a");
			a.href = "/login";
			a.textContent = tr("common.sign_in");
			result.append(a);
		} catch (err) {
			say(err instanceof ApiError ? err.message : tr("common.not_done"), "error");
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
		who.replaceChildren(...tParts("account.signed_in_as", { handle: data("@" + me.handle) }));
		if (me.email) el("account-email").replaceChildren(...tParts("account.your_address", { email: data(me.email) }));
		el("account-must-change").hidden = !me.must_change_password;
		el("tf-needed").hidden = !me.needs_second_factor;
		el("tf-state").textContent = me.second_factor
			? tr("account.tf_on", { n: me.recovery_codes_left })
			: tr("account.tf_off");
		el("tf-off-actions").hidden = !!me.second_factor;
		el("tf-on-actions").hidden = !me.second_factor;
		// An account a provider made has no password yet: it sets one.
		el("password-old-field").hidden = !me.password;
		el("password-title").textContent = me.password ? tr("account.change_password") : tr("account.set_password");
		showLinks(me);
		el("admin-link").hidden = !me.admin;
	};
	const paintTokens = async () => {
		const { tokens, scopes } = await api.tokens();
		const list = el("tokens-list");
		list.textContent = "";
		for (const t of tokens) {
			const li = document.createElement("li");
			li.append(data(t.name), " (", ...t.scopes.flatMap((sc, i) => (i ? [", ", code(sc)] : [code(sc)])), ") ");
			const b = document.createElement("button");
			b.type = "button";
			b.className = "ghost";
			b.id = `token-revoke-${t.id}`;
			b.textContent = tr("account.revoke");
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
				label.append(box, " ", code(s));
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
			li.replaceChildren(...tParts(s.current ? "account.session_this" : "account.session", { agent: s.agent ? data(s.agent) : tr("account.a_browser"), since: when(s.created_at), seen: when(s.last_seen_at) }));
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
			out.textContent = tr("account.done");
			el("handle-new").value = "";
		} catch (err) {
			out.className = "small error";
			out.textContent = err instanceof ApiError ? err.message : tr("common.not_done");
		}
	});
	el("token-go").addEventListener("click", async () => {
		const scopes = [...el("token-scopes").querySelectorAll("input:checked")].map((b) => b.value);
		try {
			const res = await api.newToken(el("token-name").value, scopes);
			el("token-new").textContent = tr("account.new_token", { token: res.token });
			el("token-name").value = "";
			paintTokens();
		} catch (err) {
			el("token-new").textContent = err instanceof ApiError ? err.message : tr("common.not_done");
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
			li.append(tr(linked ? "account.linked_yes" : "account.linked_no", { provider: p.label }), " ");
			if (linked) {
				const b = document.createElement("button");
				b.type = "button";
				b.className = "ghost";
				b.id = `oidc-unlink-${p.id}`;
				b.textContent = tr("account.unlink");
				b.addEventListener("click", () =>
					tf(b, async () => {
						show(await api.oidcUnlink(p.id, el("tf-password").value));
						tfSay(tr("account.unlinked", { provider: p.label }));
					})
				);
				li.append(b);
			} else {
				const a = document.createElement("a");
				a.id = `oidc-link-${p.id}`;
				a.href = `/auth/oidc/${encodeURIComponent(p.id)}/start?link=1&return=/account`;
				a.textContent = tr("account.link_provider", { provider: p.label });
				li.append(a);
			}
			list.append(li);
		}
		box.hidden = false;
	};
	showError("oidc-error");
	if (params.get("linked")) tfSay(tr("account.linked_now", { provider: params.get("linked") }));
	// A request that may fail with a message for the two-step box.
	const tf = async (button, run) => {
		button.disabled = true;
		tfSay("");
		try {
			await run();
		} catch (err) {
			tfSay(err instanceof ApiError ? err.message : tr("common.not_done"), "error");
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
			tfSay(tr("account.tf_now_off"));
		})
	);
	api.me().then(show).catch(() => say(tr("account.not_read"), "error"));
	const form = document.getElementById("password-form");
	submit(form, document.getElementById("password-go"), async () => {
		const me = await api.changePassword(document.getElementById("password-old").value, document.getElementById("password-new").value);
		form.reset();
		show(me);
		say(tr("account.password_changed"));
	});
	document.getElementById("logout-go").addEventListener("click", async () => {
		try {
			await api.logout();
		} finally {
			location.assign("/");
		}
	});
}
