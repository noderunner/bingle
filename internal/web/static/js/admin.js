// The administration page (ADR 0071): who may do what, announcements (ADR
// 0155), what happened, and invitations.

import { api, ApiError, whoAmI } from "./api.js";
import { t, tParts, lang } from "./i18n.js";

// A capability or a kind of limit: the server's name for it, shown as code.
const codeOf = (text) => {
	const c = document.createElement("code");
	c.textContent = text;
	return c;
};

const el = (id) => document.getElementById(id);
const say = (text, kind = "") => {
	el("caps-result").replaceChildren(...(Array.isArray(text) ? text : [text]));
	el("caps-result").className = "small " + kind;
};

function paint(m) {
	el("caps-mode").textContent =
		m.mode === "closed" ? t("admin.closed") : t("admin.hybrid");
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
			box.setAttribute("aria-label", t(cls === "user" ? "admin.users_may" : "admin.guests_may", { cap: c.capability }));
			box.addEventListener("change", async () => {
				box.disabled = true;
				try {
					paint(await api.setCap(cls, c.capability, box.checked));
					say(tParts(`admin.saved_${cls === "user" ? "users" : "guests"}_${box.checked ? "may" : "not"}`, { cap: codeOf(c.capability) }));
				} catch (err) {
					box.checked = !box.checked;
					say(err instanceof ApiError ? err.message : t("common.not_done"), "error");
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
		name.append(codeOf(kind));
		tr.append(name);
		for (const cls of ["user", "guest"]) {
			const td = document.createElement("td");
			td.className = "num";
			td.textContent = per[cls] === undefined ? "—" : per[cls].toLocaleString(lang);
			tr.append(td);
		}
		limits.append(tr);
	}
	el("invite-state").textContent =
		m.signup === "invite" ? t("admin.invite_only") : t("admin.signup_is", { mode: m.signup });
	el("invite-go").hidden = m.signup !== "invite";
}

api.adminCaps()
	.then((m) => {
		paint(m);
		el("caps-card").hidden = false;
		el("invite-card").hidden = false;
		loadAnnouncements();
		return api.adminEvents().then(({ events }) => {
			const rows = el("events-rows");
			for (const e of events) {
				const tr = document.createElement("tr");
				const when = new Date(e.at);
				const cells = [
					["when", isNaN(when) ? e.at : when.toLocaleString(lang)],
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
						// The kind is the server's word, as the log has it.
						tag.translate = false;
						td.append(tag);
					} else {
						td.textContent = v;
					}
					if (cls === "when") td.title = e.at;
					// What the server logged in detail is its own words.
					if (cls === "detail") td.translate = false;
					tr.append(td);
				}
				rows.append(tr);
			}
			el("events-card").hidden = false;
		});
	})
	.catch((err) => {
		el("admin-refused").textContent = err instanceof ApiError ? err.message : t("admin.not_read");
		el("admin-refused").hidden = false;
		// An administrator who only lacks the second factor gets the way to one.
		whoAmI()
			.then((me) => (el("admin-second-factor").hidden = !me.needs_second_factor))
			.catch(() => {});
	});

el("invite-go").addEventListener("click", async () => {
	try {
		const inv = await api.invite();
		el("invite-link").textContent = t("admin.invite_link", { link: location.origin + inv.path, until: new Date(inv.expires).toLocaleString(lang) });
	} catch (err) {
		el("invite-link").textContent = err instanceof ApiError ? err.message : t("common.not_done");
	}
});

// ---- announcements (ADR 0155) ----------------------------------------------------

const announced = (text, kind = "") => {
	el("announce-result").textContent = text;
	el("announce-result").className = "small " + kind;
};

function announcementRow(a) {
	const tr = document.createElement("tr");
	const when = document.createElement("td");
	when.className = "when";
	const at = new Date(a.published_at);
	when.textContent = isNaN(at) ? a.published_at : at.toLocaleString(lang);
	const title = document.createElement("td");
	title.textContent = a.title;
	// The administrator's own words.
	title.translate = false;
	const act = document.createElement("td");
	if (a.withdrawn) {
		act.className = "muted";
		act.textContent = t("admin.withdrawn");
	} else {
		const b = document.createElement("button");
		b.type = "button";
		b.className = "small";
		b.textContent = t("admin.withdraw");
		b.setAttribute("aria-label", t("admin.withdraw_label", { title: a.title }));
		b.addEventListener("click", async () => {
			b.disabled = true;
			try {
				tr.replaceWith(announcementRow(await api.withdrawAnnouncement(a.id)));
			} catch (err) {
				b.disabled = false;
				announced(err instanceof ApiError ? err.message : t("common.not_done"), "error");
			}
		});
		act.append(b);
	}
	tr.append(when, title, act);
	return tr;
}

async function loadAnnouncements() {
	try {
		const { announcements } = await api.adminAnnouncements();
		el("announce-rows").replaceChildren(...announcements.map(announcementRow));
		el("announce-table").hidden = !announcements.length;
		el("announce-card").hidden = false;
	} catch (_) {
		/* the rest of the page without them */
	}
}

el("announce-form").addEventListener("submit", async (ev) => {
	ev.preventDefault();
	const go = el("announce-go");
	go.disabled = true;
	try {
		const a = await api.announce(el("announce-title").value, el("announce-body").value);
		el("announce-rows").prepend(announcementRow(a));
		el("announce-table").hidden = false;
		el("announce-form").reset();
		announced(t("admin.published", { title: a.title }));
	} catch (err) {
		announced(err instanceof ApiError ? err.message : t("common.not_done"), "error");
	} finally {
		go.disabled = false;
	}
});
