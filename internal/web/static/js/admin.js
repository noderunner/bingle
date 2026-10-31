// The administration page (ADR 0071): who may do what, and invitations.

import { api, ApiError, whoAmI } from "./api.js";

const el = (id) => document.getElementById(id);
const say = (text, kind = "") => {
	el("caps-result").textContent = text;
	el("caps-result").className = "small " + kind;
};

function paint(m) {
	el("caps-mode").textContent =
		m.mode === "closed" ? "This server is closed: only signed-in members can play (BINGLE_AUTH_MODE)." : "Guests can play without an account (BINGLE_AUTH_MODE=hybrid).";
	el("caps-drift").hidden = !m.drift;
	const rows = el("caps-rows");
	rows.textContent = "";
	for (const c of m.caps) {
		const tr = document.createElement("tr");
		const name = document.createElement("th");
		name.scope = "row";
		const code = document.createElement("code");
		code.textContent = c.capability;
		name.append(code);
		tr.append(name);
		for (const cls of ["user", "guest"]) {
			const td = document.createElement("td");
			td.className = "col-check";
			const box = document.createElement("input");
			box.type = "checkbox";
			box.id = `cap-${cls}-${c.capability}`;
			box.checked = !!c[cls];
			box.setAttribute("aria-label", `${cls === "user" ? "Users" : "Guests"} may ${c.capability}`);
			box.addEventListener("change", async () => {
				box.disabled = true;
				try {
					paint(await api.setCap(cls, c.capability, box.checked));
					say(`Saved: ${cls === "user" ? "users" : "guests"} ${box.checked ? "may" : "may not"} ${c.capability}.`);
				} catch (err) {
					box.checked = !box.checked;
					say(err instanceof ApiError ? err.message : "That did not work.", "error");
				} finally {
					box.disabled = false;
				}
			});
			td.append(box);
			tr.append(td);
		}
		rows.append(tr);
	}
	// One row a kind, a column a class: "—" where a class has no limit of it.
	const limits = el("caps-limits");
	limits.textContent = "";
	const kinds = new Map();
	for (const l of m.limits) {
		if (!kinds.has(l.kind)) kinds.set(l.kind, {});
		kinds.get(l.kind)[l.class] = l.per_minute;
	}
	for (const [kind, per] of [...kinds].sort(([a], [b]) => a.localeCompare(b))) {
		const tr = document.createElement("tr");
		const name = document.createElement("th");
		name.scope = "row";
		name.textContent = kind.replaceAll("_", " ");
		tr.append(name);
		for (const cls of ["user", "guest"]) {
			const td = document.createElement("td");
			td.className = "num";
			td.textContent = per[cls] === undefined ? "—" : per[cls].toLocaleString();
			tr.append(td);
		}
		limits.append(tr);
	}
	el("invite-state").textContent =
		m.signup === "invite" ? "Signing up needs an invitation (BINGLE_SIGNUP=invite). Each link works once, for a week." : `Signing up is ${m.signup} (BINGLE_SIGNUP); invitations are for BINGLE_SIGNUP=invite.`;
	el("invite-go").hidden = m.signup !== "invite";
}

api.adminCaps()
	.then((m) => {
		paint(m);
		el("caps-card").hidden = false;
		el("invite-card").hidden = false;
		return api.adminEvents().then(({ events }) => {
			const rows = el("events-rows");
			for (const e of events) {
				const tr = document.createElement("tr");
				const when = new Date(e.at);
				const cells = [
					["when", isNaN(when) ? e.at : when.toLocaleString()],
					["who", e.account ? "@" + e.account : "—"],
					["what", e.kind],
					["detail", e.detail || ""],
				];
				for (const [cls, v] of cells) {
					const td = document.createElement("td");
					td.className = cls;
					if (cls === "what") {
						const tag = document.createElement("span");
						tag.className = "event-kind" + (/fail|in-use/.test(v) ? " bad" : "");
						tag.textContent = v;
						td.append(tag);
					} else {
						td.textContent = v;
					}
					if (cls === "when") td.title = e.at;
					tr.append(td);
				}
				rows.append(tr);
			}
			el("events-card").hidden = false;
		});
	})
	.catch((err) => {
		el("admin-refused").textContent = err instanceof ApiError ? err.message : "This page could not be read.";
		el("admin-refused").hidden = false;
		// An administrator who only lacks the second factor gets the way to one.
		whoAmI()
			.then((me) => (el("admin-second-factor").hidden = !me.needs_second_factor))
			.catch(() => {});
	});

el("invite-go").addEventListener("click", async () => {
	try {
		const inv = await api.invite();
		el("invite-link").textContent = `${location.origin}${inv.path} (until ${new Date(inv.expires).toLocaleString()})`;
	} catch (err) {
		el("invite-link").textContent = err instanceof ApiError ? err.message : "That did not work.";
	}
});
