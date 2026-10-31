// Small DOM and storage helpers shared by the lobby and the game page.

/** h("div", {class: "x", onclick: fn}, child, "text") builds a DOM element. */
export function h(tag, attrs, ...children) {
	const el = document.createElement(tag);
	for (const [k, v] of Object.entries(attrs || {})) {
		if (v === null || v === undefined || v === false) continue;
		if (k.startsWith("on") && typeof v === "function") {
			el.addEventListener(k.slice(2).toLowerCase(), v);
		} else if (k === "class") {
			el.className = v;
		} else if (k === "dataset") {
			Object.assign(el.dataset, v);
		} else if (v === true) {
			el.setAttribute(k, "");
		} else {
			el.setAttribute(k, String(v));
		}
	}
	for (const c of children.flat()) {
		if (c === null || c === undefined || c === false) continue;
		el.append(c instanceof Node ? c : document.createTextNode(String(c)));
	}
	return el;
}

const SVG_NS = "http://www.w3.org/2000/svg";

/** s("rect", {x: 1, ...}, children) builds an SVG element. */
export function s(tag, attrs, ...children) {
	const el = document.createElementNS(SVG_NS, tag);
	for (const [k, v] of Object.entries(attrs || {})) {
		if (v === null || v === undefined || v === false) continue;
		if (k.startsWith("on") && typeof v === "function") {
			el.addEventListener(k.slice(2).toLowerCase(), v);
		} else {
			el.setAttribute(k === "className" ? "class" : k, String(v));
		}
	}
	for (const c of children.flat()) {
		if (c === null || c === undefined || c === false) continue;
		el.append(c instanceof Node ? c : document.createTextNode(String(c)));
	}
	return el;
}

export function clear(el) {
	while (el.firstChild) el.removeChild(el.firstChild);
	return el;
}

// ---- storage (every access is guarded: private windows can throw) --------

export const store = {
	get(key, fallback = null) {
		try {
			const raw = window.localStorage.getItem(key);
			return raw === null ? fallback : JSON.parse(raw);
		} catch (_) {
			return fallback;
		}
	},
	set(key, value) {
		try {
			window.localStorage.setItem(key, JSON.stringify(value));
		} catch (_) {
			/* storage unavailable: the page still works, just forgets */
		}
	},
};

const GAMES_KEY = "bingle.games";

/** Games this browser has joined: [{code, def, name, ts}], newest first. */
export function rememberedGames() {
	const list = store.get(GAMES_KEY, []);
	return Array.isArray(list) ? list : [];
}

export function rememberGame(code, def, name) {
	const list = rememberedGames().filter((g) => g.code !== code);
	list.unshift({ code, def, name, ts: Date.now() });
	store.set(GAMES_KEY, list.slice(0, 30));
	store.set("bingle.name", name);
}

export function forgetGame(code) {
	store.set(GAMES_KEY, rememberedGames().filter((g) => g.code !== code));
}

export function nameFor(code) {
	const g = rememberedGames().find((x) => x.code === code);
	return g ? g.name : null;
}

export function lastName() {
	return store.get("bingle.name", "") || "";
}

const PROFILE_KEY = "bingle.profile";

/**
 * The settings of this browser's player that are not a game's: the email
 * address (kept for the email notifications that are planned, never sent
 * anywhere yet) and whether those notifications are wanted.
 */
export function getProfile() {
	const p = store.get(PROFILE_KEY, {}) || {};
	return { email: typeof p.email === "string" ? p.email : "", notify: !!p.notify };
}

export function saveProfile(patch) {
	store.set(PROFILE_KEY, { ...getProfile(), ...patch });
}

/** Sets the name offered when creating or joining a game. */
export function setLastName(name) {
	store.set("bingle.name", name);
}

// ---- formatting -----------------------------------------------------------

/** "3 min ago" for a SQLite UTC timestamp ("YYYY-MM-DD HH:MM:SS") or ms. */
export function ago(ts) {
	let ms;
	if (typeof ts === "number") {
		ms = ts;
	} else if (typeof ts === "string" && ts) {
		ms = Date.parse(ts.includes("T") ? ts : ts.replace(" ", "T") + "Z");
	}
	if (!ms || Number.isNaN(ms)) return "";
	const sec = Math.max(0, Math.round((Date.now() - ms) / 1000));
	if (sec < 45) return "just now";
	const min = Math.round(sec / 60);
	if (min < 60) return `${min} min ago`;
	const hr = Math.round(min / 60);
	if (hr < 36) return `${hr} h ago`;
	return `${Math.round(hr / 24)} d ago`;
}

export function titleCase(str) {
	return str ? str.charAt(0).toUpperCase() + str.slice(1) : "";
}

let toastTimer = null;
/** Brief non-blocking message at the bottom of the page. */
export function toast(message, kind = "") {
	let el = document.getElementById("toast");
	if (!el) {
		el = h("div", { id: "toast", class: "toast hidden", role: "status", "aria-live": "polite" });
		document.body.append(el);
	}
	el.textContent = message;
	el.className = "toast" + (kind ? " " + kind : "");
	clearTimeout(toastTimer);
	toastTimer = setTimeout(() => el.classList.add("hidden"), kind === "error" ? 6000 : 2800);
}

/**
 * Marks the browser tab icon with an orange dot while it is your turn
 * (somewhere), so a pinned or background tab says so at a glance.
 */
export function setTurnBadge(on) {
	const link = document.getElementById("favicon");
	if (!link) return;
	const dot = on ? '<circle cx="25" cy="7" r="7" fill="#e8590c" stroke="#fff" stroke-width="2"/>' : "";
	const svg =
		'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32"><rect width="32" height="32" rx="7" fill="#2f6f5e"/>' +
		'<circle cx="16" cy="16" r="8" fill="#f5f0e6"/>' +
		dot +
		"</svg>";
	link.href = "data:image/svg+xml," + encodeURIComponent(svg);
}
