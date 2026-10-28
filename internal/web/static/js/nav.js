// Moving between the pages of the site without reloading the document.
//
// A voice call lives in the page: reloading it would drop the call. So while a
// call is up (see setKeepAlive) a click on a link to another page of the site
// does not load the page: this module fetches it, replaces the content of the
// document, runs the scripts of the new page as fresh modules and leaves the
// call — its module state, its sockets and the <audio> elements marked
// data-persist — where they are. With no call, links work as they always did
// (the whole document is loaded), so nothing else depends on this.
//
// A page is a set of modules that set the page up when they run and leave
// listeners, timers and sockets behind. While the page lives, those the page
// makes with addEventListener on the window or the document, setTimeout,
// setInterval and WebSocket are noted here and undone when it is left;
// whatever must outlive a page (the call) is made inside untracked().

const ROUTES = [/^\/$/, /^\/game\/[A-Za-z0-9]+\/?$/, /^\/profile\/?$/];

// Everything that must outlive a page says so here (a game's call, a direct
// call): a soft navigation is needed while any of them answers true.
const keepAlives = new Set();
const keepAlive = () => {
	for (const fn of keepAlives) {
		try {
			if (fn()) return true;
		} catch (_) {
			/* a module that is going away */
		}
	}
	return false;
};
let installed = false;
let tracking = false;
let generation = 0;
let busy = false;
let lastPath = location.pathname;

let scope = freshScope();
const leaveHooks = []; // run once, when the page they were made for is left
const navigateHooks = []; // run at every soft navigation (module state that belongs to one page)

function freshScope() {
	return { listeners: [], timeouts: [], intervals: [], sockets: [] };
}

/**
 * Says when a soft navigation is needed: voice.js answers "a call is up", and
 * dm-call.js "a direct call is up". Every caller is kept, so one module cannot
 * take another's answer away.
 */
export function setKeepAlive(fn) {
	keepAlives.add(fn);
	return () => keepAlives.delete(fn);
}

/** Runs fn without noting what it creates: for what must outlive the page. */
export function untracked(fn) {
	const was = tracking;
	tracking = false;
	try {
		return fn();
	} finally {
		tracking = was;
	}
}

/** fn runs when the page that is showing now is left (once). */
export function onLeave(fn) {
	leaveHooks.push(fn);
}

/** fn runs every time a page is left: for a module that keeps something of the page it served. */
export function onNavigate(fn) {
	navigateHooks.push(fn);
}

/** True when moving to another page should not reload the document. */
export function softNavigation() {
	return installed && keepAlive();
}

/** The path (with query) of a link inside the site that is one of its pages, else null. */
function pageOf(url) {
	let u;
	try {
		u = new URL(url, location.href);
	} catch (_) {
		return null;
	}
	if (u.origin !== location.origin) return null;
	if (!ROUTES.some((r) => r.test(u.pathname))) return null;
	return u.pathname + u.search + u.hash;
}

/**
 * Goes to a page of the site: softly while a call is up, else by loading it.
 * Anything that is not a page of the site is loaded.
 */
export function navigate(url, { replace = false } = {}) {
	const page = pageOf(url);
	if (!page || !softNavigation()) {
		if (replace) location.replace(url);
		else location.assign(url);
		return Promise.resolve();
	}
	return go(page, { push: !replace, replace });
}

// ---- noting what a page makes --------------------------------------------------------------

function install() {
	const addWin = window.addEventListener.bind(window);
	const addDoc = document.addEventListener.bind(document);
	window.addEventListener = (type, fn, opts) => {
		if (tracking) scope.listeners.push([window, type, fn, opts]);
		return addWin(type, fn, opts);
	};
	document.addEventListener = (type, fn, opts) => {
		if (tracking) scope.listeners.push([document, type, fn, opts]);
		return addDoc(type, fn, opts);
	};
	const rawMatchMedia = window.matchMedia.bind(window);
	window.matchMedia = (query) => {
		const list = rawMatchMedia(query);
		if (tracking) {
			const add = list.addEventListener.bind(list);
			list.addEventListener = (type, fn, opts) => {
				scope.listeners.push([list, type, fn, opts]);
				return add(type, fn, opts);
			};
		}
		return list;
	};
	const setT = window.setTimeout.bind(window);
	const setI = window.setInterval.bind(window);
	window.setTimeout = (fn, ms, ...args) => {
		const id = setT(fn, ms, ...args);
		if (tracking) scope.timeouts.push(id);
		return id;
	};
	window.setInterval = (fn, ms, ...args) => {
		const id = setI(fn, ms, ...args);
		if (tracking) scope.intervals.push(id);
		return id;
	};
	const RawSocket = window.WebSocket;
	window.WebSocket = class extends RawSocket {
		constructor(...args) {
			super(...args);
			if (!tracking) return;
			const mine = [];
			const add = this.addEventListener.bind(this);
			this.addEventListener = (type, fn, opts) => {
				mine.push([type, fn, opts]);
				return add(type, fn, opts);
			};
			scope.sockets.push({ sock: this, listeners: mine });
		}
	};
}

/** Undoes what the page that is being left made. */
function endPage() {
	const old = scope;
	scope = freshScope();
	for (const fn of leaveHooks.splice(0)) {
		try {
			fn();
		} catch (_) {
			/* a hook of a page that is gone */
		}
	}
	for (const [target, type, fn, opts] of old.listeners) target.removeEventListener(type, fn, opts);
	for (const id of old.timeouts) window.clearTimeout(id);
	for (const id of old.intervals) window.clearInterval(id);
	for (const { sock, listeners } of old.sockets) {
		for (const [type, fn, opts] of listeners) sock.removeEventListener(type, fn, opts);
		sock.onopen = sock.onmessage = sock.onclose = sock.onerror = null;
		try {
			sock.close();
		} catch (_) {
			/* already closed */
		}
	}
	for (const fn of navigateHooks) {
		try {
			fn();
		} catch (_) {
			/* keep going */
		}
	}
}

// ---- the navigation --------------------------------------------------------------------------

function fallback(page, replace) {
	if (replace) location.replace(page);
	else location.assign(page);
}

async function go(page, { push, replace }) {
	if (busy) return;
	busy = true;
	try {
		const res = await fetch(page, { headers: { Accept: "text/html" }, credentials: "same-origin" });
		const type = res.headers.get("Content-Type") || "";
		if (!res.ok || !type.includes("text/html")) return fallback(page, replace);
		const doc = new DOMParser().parseFromString(await res.text(), "text/html");
		const scripts = [...doc.querySelectorAll('script[type="module"][src]')].map((el) => new URL(el.getAttribute("src"), location.href).pathname);
		if (!scripts.length) return fallback(page, replace);

		endPage();
		// The stylesheets the new page has and this one lacks.
		const have = new Set([...document.head.querySelectorAll('link[rel="stylesheet"]')].map((l) => l.getAttribute("href")));
		for (const l of doc.head.querySelectorAll('link[rel="stylesheet"]')) {
			if (!have.has(l.getAttribute("href"))) document.head.append(document.importNode(l, true));
		}
		document.title = doc.title;
		// The body: its attributes, and everything in it but what must stay (the call's audio).
		for (const a of [...document.body.attributes]) document.body.removeAttribute(a.name);
		for (const a of doc.body.attributes) document.body.setAttribute(a.name, a.value);
		for (const el of [...document.body.children]) if (!el.hasAttribute("data-persist")) el.remove();
		for (const el of [...doc.body.children]) {
			if (el.tagName === "SCRIPT") continue;
			document.body.append(document.importNode(el, true));
		}
		if (push) history.pushState({ soft: true }, "", page);
		else if (replace) history.replaceState({ soft: true }, "", page);
		window.scrollTo(0, 0);

		const gen = ++generation;
		tracking = true;
		for (const src of scripts) {
			await import(`${src}?nav=${gen}`);
		}
	} catch (err) {
		console.warn("soft navigation failed, loading the page:", err);
		fallback(page, replace);
	} finally {
		busy = false;
		lastPath = location.pathname;
	}
}

/** Called by every page: from now on the page's resources are noted, and links may be followed softly. */
export function installNav() {
	if (installed) return;
	installed = true;
	install();
	tracking = true;
	untracked(() => {
		document.addEventListener(
			"click",
			(ev) => {
				if (ev.defaultPrevented || ev.button !== 0 || ev.metaKey || ev.ctrlKey || ev.shiftKey || ev.altKey) return;
				const a = ev.target instanceof Element ? ev.target.closest("a[href]") : null;
				if (!a || a.target || a.hasAttribute("download") || !keepAlive()) return;
				const url = new URL(a.href, location.href);
				// A jump inside this very page is the browser's.
				if (url.origin === location.origin && url.pathname === location.pathname && url.search === location.search && url.hash) return;
				const page = pageOf(a.href);
				if (!page) return;
				ev.preventDefault();
				go(page, { push: true, replace: false });
			},
			true
		);
		window.addEventListener("popstate", () => {
			// Only a move to another page matters: a hash or a query is the page's own.
			if (location.pathname === lastPath) return;
			const page = keepAlive() ? pageOf(location.href) : null;
			if (page) go(page, { push: false, replace: false });
			else location.reload();
		});
	});
}
