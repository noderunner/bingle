// What went wrong in this browser, reported once to POST /api/client-error.
//
// The UI is where most of bingle's recent work went and where there was no
// visibility at all: a script that throws on one person's machine left no
// trace anywhere. This is that trace, and it is built so that it cannot
// become a nuisance — to the server or to the person whose browser it is.
//
// What is sent is the message, the stack, the **route pattern** of the page
// (never its URL, which carries the room code), the release, and a short
// browser string. Nothing else: no name, no room code, no secret. Anything
// shaped like one of bingle's tokens is taken out before sending, and the
// server scrubs it again.

const ENDPOINT = "/api/client-error";

// At most this many reports from one page, ever. A script that throws inside
// a redraw throws on every frame; the point is to learn that it happened, not
// how many times.
const MAX_PER_PAGE = 3;

// How long to sit on a report, so a burst of the same fault is one send.
const SETTLE_MS = 1000;

// Caps that match the server's, so a long stack is cut here rather than
// refused there.
const MAX_MESSAGE = 500;
const MAX_STACK = 4000;

/**
 * The route pattern of this page, from the set the server knows. A URL would
 * carry the room code, which is the key to the game (ADR 0063), so the path
 * is turned into its pattern here and the server checks it against the same
 * closed set.
 */
function routePattern(path) {
	if (path === "/" || path === "") return "/";
	if (/^\/game\/[^/]+\/?$/.test(path)) return "/game/{code}";
	if (path === "/profile" || path === "/profile/") return "/profile";
	if (path === "/batches" || path === "/batches/") return "/batches";
	if (/^\/batches\/[^/]+\/?$/.test(path)) return "/batches/{id}";
	if (path === "/notify/confirm") return "/notify/confirm";
	if (path === "/notify/stop") return "/notify/stop";
	return "other";
}

/** The release this page was served by. */
function version() {
	return document.querySelector('meta[name="bingle-version"]')?.content || "";
}

/**
 * A short browser string. The full user agent is long, and the parts that
 * make it long are the parts that identify a particular machine; the engine
 * and its major version are what a fault actually depends on.
 */
function browser() {
	const ua = navigator.userAgent || "";
	const m = ua.match(/(Firefox|Chrome|Safari|Edg)\/(\d+)/);
	const name = m ? `${m[1]} ${m[2]}` : "unknown";
	const platform = /Android|iPhone|iPad|Mobile/.test(ua) ? "mobile" : "desktop";
	return `${name} ${platform}`;
}

/** Anything shaped like one of bingle's secrets, gone. */
function scrub(text) {
	return String(text == null ? "" : text).replace(/[A-Za-z0-9_-]{24,}/g, "[redacted]");
}

/**
 * The room code appears in this page's own URL, so it appears in stack frames
 * and in message text that quotes the URL. Take it out by name as well as by
 * shape: a six-character room code is too short for the token rule to catch.
 */
function withoutRoom(text) {
	const m = location.pathname.match(/^\/game\/([^/]+)/);
	if (!m) return text;
	return text.split(m[1]).join("{code}");
}

function clean(text, max) {
	const out = withoutRoom(scrub(text)).trim();
	return out.length > max ? `${out.slice(0, max)}…` : out;
}

let sent = 0;
let pending = null;
let timer = null;

/** Send what is pending, at most once per fault. */
function flush() {
	timer = null;
	const report = pending;
	pending = null;
	if (!report || sent >= MAX_PER_PAGE) return;
	sent++;

	// keepalive so a fault during a navigation still gets out; a failure to
	// report a fault is not worth reporting.
	try {
		fetch(ENDPOINT, {
			method: "POST",
			headers: { "Content-Type": "application/json", "X-Bingle-Session": "1" },
			body: JSON.stringify(report),
			keepalive: true,
		}).catch(() => {});
	} catch {
		/* nothing sensible to do about it */
	}
}

/** Note a fault, to be sent once the burst has settled. */
function report(message, stack) {
	if (sent >= MAX_PER_PAGE) return;
	const next = {
		message: clean(message || "an error with no message", MAX_MESSAGE),
		stack: clean(stack || "", MAX_STACK),
		page: routePattern(location.pathname),
		version: version(),
		browser: browser(),
	};
	if (!next.message) return;
	// The first fault of a burst is the interesting one: a later throw is
	// usually the same thing again, or a consequence of it.
	if (!pending) pending = next;
	if (timer === null) timer = setTimeout(flush, SETTLE_MS);
}

window.addEventListener("error", (ev) => {
	if (ev.error instanceof Error) {
		report(ev.error.message, ev.error.stack);
	} else {
		report(ev.message, `${ev.filename || ""}:${ev.lineno || 0}:${ev.colno || 0}`);
	}
});

window.addEventListener("unhandledrejection", (ev) => {
	const r = ev.reason;
	if (r instanceof Error) {
		report(r.message, r.stack);
	} else {
		report(`unhandled rejection: ${r}`, "");
	}
});

// A page being closed should not sit on a report it has already decided to
// send.
window.addEventListener("pagehide", flush);
