// The administration page (ADR 0071): who may do what, and invitations.

import { api, ApiError } from "./api.js";

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
		const name = document.createElement("td");
		name.textContent = c.capability;
		tr.append(name);
		for (const cls of ["user", "guest"]) {
			const td = document.createElement("td");
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
	const limits = el("caps-limits");
	limits.textContent = "";
	for (const l of m.limits) {
		const li = document.createElement("li");
		li.textContent = `${l.class === "user" ? "Users" : "Guests"}: ${l.per_minute} ${l.kind.replace("_", " ")} a minute`;
		limits.append(li);
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
				for (const v of [e.at, e.account ? "@" + e.account : "", e.kind, e.detail || ""]) {
					const td = document.createElement("td");
					td.textContent = v;
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
	});

el("invite-go").addEventListener("click", async () => {
	try {
		const inv = await api.invite();
		el("invite-link").textContent = `${location.origin}${inv.path} (until ${new Date(inv.expires).toLocaleString()})`;
	} catch (err) {
		el("invite-link").textContent = err instanceof ApiError ? err.message : "That did not work.";
	}
});
