// Thin fetch wrapper around bingle's REST API.

// Every request says it comes from bingle's own page: the server refuses a
// state-changing request that is proved by the guest cookie but lacks it, and
// a page's first such request is the one that makes its guest (ADR 0067).
export const PAGE_HEADER = "X-Bingle-Session";

export class ApiError extends Error {
	constructor(message, status) {
		super(message);
		this.status = status;
	}
}

// The first state-changing request of a page goes alone, and the rest wait
// for its answer: a browser without a guest yet gets one from that answer's
// cookie, and two such requests at once would make two guests, the second
// cookie overwriting the first and leaving a seat with the guest nobody has.
let first = null;

async function call(path, { method = "GET", body, headers, keepalive } = {}) {
	const init = { method, headers: { [PAGE_HEADER]: "1", ...(headers || {}) } };
	if (keepalive) init.keepalive = true;
	if (body !== undefined) {
		init.headers["Content-Type"] = "application/json";
		init.body = JSON.stringify(body);
	}
	let resp;
	let done = null;
	if (method !== "GET") {
		if (first) await first;
		else first = new Promise((resolve) => (done = resolve));
	}
	try {
		resp = await fetch(path, init);
	} catch (err) {
		throw new ApiError("Could not reach the server.", 0);
	} finally {
		// After the first answer the cookie is there (or the server chose not
		// to make a guest): nothing more to wait for.
		if (done) {
			done();
			first = Promise.resolve();
		}
	}
	let data = null;
	const text = await resp.text();
	if (text) {
		try {
			data = JSON.parse(text);
		} catch (_) {
			data = null;
		}
	}
	if (!resp.ok) {
		const msg = (data && data.error) || `Request failed (${resp.status})`;
		throw new ApiError(msg.replace(/^(api|state|session|store|catalog): /, ""), resp.status);
	}
	return data;
}

const enc = encodeURIComponent;

// codeBody is a typed second factor as the server wants it: six digits are a
// code from the app, anything else a recovery code.
function codeBody(typed) {
	const t = String(typed || "").replace(/\s+/g, "");
	if (!t) return {};
	return /^\d{6}$/.test(t) ? { code: t } : { recovery_code: t };
}

export const api = {
	// Accounts (ADR 0068).
	me: () => call("/auth/me"),
	login: (handle, password) => call("/auth/login", { method: "POST", body: { handle, password } }),
	logout: () => call("/auth/logout", { method: "POST" }),
	signup: (handle, email, password) => call("/auth/signup", { method: "POST", body: { handle, email, password } }),
	confirmAccount: (token) => call("/auth/confirm", { method: "POST", body: { token } }),
	changePassword: (old, next) => call("/auth/password", { method: "POST", body: { old, new: next } }),
	// A code, or a recovery code: what was typed decides which it is.
	secondFactor: (typed) => call("/auth/second", { method: "POST", body: codeBody(typed) }),
	totpSetup: (password) => call("/auth/totp/setup", { method: "POST", body: { password } }),
	totpEnable: (code) => call("/auth/totp/enable", { method: "POST", body: { code } }),
	totpDisable: (password, typed) => call("/auth/totp/disable", { method: "POST", body: { password, ...codeBody(typed) } }),
	newRecoveryCodes: (password) => call("/auth/recovery", { method: "POST", body: { password } }),
	oidcFinish: (handle) => call("/auth/oidc/finish", { method: "POST", body: { handle } }),
	oidcUnlink: (provider, password) => call("/auth/oidc/unlink", { method: "POST", body: { provider, password } }),
	forgot: (who) => call("/auth/forgot", { method: "POST", body: { who } }),
	resetPassword: (token, password, typed) => call("/auth/reset", { method: "POST", body: { token, password, ...codeBody(typed) } }),
	batches: () => call("/api/batches"),
	startBatch: (body) => call("/api/batches", { method: "POST", body }),
	batchReport: (id) => call(`/api/batches/${enc(id)}/report`),
	batchGames: (id) => call(`/api/batches/${enc(id)}/games`),
	batchEvents: (id, n) => call(`/api/batches/${enc(id)}/games/${n}/events`),
	cancelBatch: (id) => call(`/api/batches/${enc(id)}/cancel`, { method: "POST" }),
	deleteBatch: (id) => call(`/api/batches/${enc(id)}`, { method: "DELETE" }),
	botGuide: () => fetch("/api/bot-guide").then((r) => (r.ok ? r.text() : Promise.reject(new Error("The guide could not be read.")))),
	gameDefs: () => call("/api/game-defs"),
	gameDef: (id) => call(`/api/game-defs/${enc(id)}`),
	guide: (id) => call(`/api/game-defs/${enc(id)}/guide`),
	bots: () => call("/api/bots"),
	puzzles: (gameDefID) => call("/api/puzzles" + (gameDefID ? `?game_def_id=${enc(gameDefID)}` : "")),
	puzzle: (id, reveal) => call(`/api/puzzles/${enc(id)}${reveal ? "?reveal=1" : ""}`),
	createGame: (body) => call("/api/games", { method: "POST", body }),
	importGame: (body) => call("/api/games/import", { method: "POST", body }),
	botMatch: (body) => call("/api/bot-matches", { method: "POST", body }),
	game: (code) => call(`/api/games/${enc(code)}`),
	join: (code, name, role = "player", seat) =>
		call(`/api/games/${enc(code)}/join`, { method: "POST", body: { player_name: name, role, seat: seat || undefined } }),
	addBot: (code, name, bot, seat) =>
		call(`/api/games/${enc(code)}/bots`, { method: "POST", body: { player_name: name, bot, seat: seat || undefined } }),
	config: () => call("/api/config"),
	// The newest release of the changelog: {version, date?, markdown}; 404 when the server has none.
	changelog: () => call("/api/changelog"),
	start: (code, name) => call(`/api/games/${enc(code)}/start`, { method: "POST", body: { player_name: name } }),
	chooseSeat: (code, name, seat) => call(`/api/games/${enc(code)}/seat`, { method: "POST", body: { player_name: name, seat } }),
	// Before the game starts: the cult (gc, cc, bg, ys, ww, sl) an open seat plays.
	setSeatCult: (code, name, seat, cult) => call(`/api/games/${enc(code)}/seat-cult`, { method: "POST", body: { player_name: name, seat, cult } }),
	leave: (code, name) => call(`/api/games/${enc(code)}/leave`, { method: "POST", body: { player_name: name } }),
	removeSeat: (code, name, seat) => call(`/api/games/${enc(code)}/remove`, { method: "POST", body: { player_name: name, seat } }),
	reactions: (code, name) => call(`/api/games/${enc(code)}/reactions?player_name=${enc(name)}`),
	// patch: {targeted?, ask_all?, ask_seats?}: when the player wants to be asked (the rest is passed for them).
	setReactions: (code, name, patch) => call(`/api/games/${enc(code)}/reactions`, { method: "PUT", body: { player_name: name, ...patch } }),
	autoplay: (code, name) => call(`/api/games/${enc(code)}/autoplay?player_name=${enc(name)}`),
	// patch: {enabled?, timeout_seconds?}
	setAutoplay: (code, name, patch) => call(`/api/games/${enc(code)}/autoplay`, { method: "PUT", body: { player_name: name, ...patch } }),
	chat: (code, after) => call(`/api/games/${enc(code)}/chat?limit=100${after ? `&after=${after}` : ""}`),
	say: (code, name, text) => call(`/api/games/${enc(code)}/chat`, { method: "POST", body: { player_name: name, text } }),
	globalChat: (after) => call(`/api/chat?limit=100${after ? `&after=${after}` : ""}`),
	sayGlobal: (name, text) => call("/api/chat", { method: "POST", body: { player_name: name, text } }),
	rtcConfig: () => call("/api/rtc/config"),
	notifyConfig: () => call("/api/notify/config"),
	notifySaveContact: (body) => call("/api/notify/contact", { method: "POST", body }),
	notifyContactStatus: (id, secret) => call("/api/notify/contact/status", { method: "POST", body: { contact_id: id, contact_secret: secret } }),
	notifyResend: (id, secret) => call("/api/notify/contact/resend", { method: "POST", body: { contact_id: id, contact_secret: secret } }),
	notifyForget: (id, secret) => call("/api/notify/contact/forget", { method: "POST", body: { contact_id: id, contact_secret: secret } }),
	gameNotify: (code, name) => call(`/api/games/${enc(code)}/notify?player_name=${enc(name)}`),
	setGameNotify: (code, body) => call(`/api/games/${enc(code)}/notify`, { method: "PUT", body }),
	voice: (code) => call(`/api/games/${enc(code)}/voice`),
	setVoice: (code, name, active, secret) => call(`/api/games/${enc(code)}/voice`, { method: "POST", body: { player_name: name, active, secret } }),
	// The tab is closing: a request that outlives the page.
	leaveVoice: (code, name) => call(`/api/games/${enc(code)}/voice`, { method: "POST", body: { player_name: name, active: false }, keepalive: true }),
	voiceSignal: (code, name, to, kind, data) => call(`/api/games/${enc(code)}/voice/signal`, { method: "POST", body: { player_name: name, to, kind, data } }),
	state: (code, atSeq, name) => {
		const q = [];
		if (atSeq !== undefined && atSeq !== null) q.push(`at_seq=${atSeq}`);
		if (name) q.push(`player_name=${enc(name)}`);
		return call(`/api/games/${enc(code)}/state${q.length ? "?" + q.join("&") : ""}`);
	},
	moves: (code) => call(`/api/games/${enc(code)}/moves`),
	openGames: (limit) => call("/api/games" + (limit ? `?limit=${limit}` : "")),
	sendDirect: (secret, to, text) => call("/api/dm", { method: "POST", body: { secret, to, text } }),
	directHistory: (secret, with_, after) => call("/api/dm/history", { method: "POST", body: { secret, with: with_, after: after || 0 } }),
	directThreads: (secret) => call("/api/dm/threads", { method: "POST", body: { secret } }),
	directCall: (secret, to, join) => call("/api/dm/call", { method: "POST", body: { secret, to, join } }),
	directSignal: (secret, to, kind, data) => call("/api/dm/signal", { method: "POST", body: { secret, to, kind, data } }),
	here: (secret, name) => call("/api/here", { method: "POST", body: { secret, player_name: name } }),
	whoIsHere: () => call("/api/here"),
	presence: (code) => call(`/api/games/${enc(code)}/presence`),
	setPresence: (code, name, present) => call(`/api/games/${enc(code)}/presence`, { method: "POST", body: { player_name: name, present } }),
	history: (code) => call(`/api/games/${enc(code)}/history`),
	legal: (code) => call(`/api/games/${enc(code)}/moves/legal?steps=1`),
	move: (code, name, notation) =>
		call(`/api/games/${enc(code)}/moves`, { method: "POST", body: { player_name: name, notation } }),
	undo: (code, name) => call(`/api/games/${enc(code)}/undo`, { method: "POST", body: { player_name: name } }),
	resign: (code, name) => call(`/api/games/${enc(code)}/resign`, { method: "POST", body: { player_name: name } }),
	hint: (code, name, atSeq, moves) =>
		call(`/api/games/${enc(code)}/hint`, {
			method: "POST",
			body: { player_name: name, at_seq: atSeq === undefined || atSeq === null ? undefined : atSeq, moves: moves || [] },
		}),
	takeback: (code, name, action) =>
		call(`/api/games/${enc(code)}/takeback`, { method: "POST", body: { player_name: name, action } }),
	draw: (code, name, action) =>
		call(`/api/games/${enc(code)}/draw`, { method: "POST", body: { player_name: name, action } }),
	simulate: (code, moves, atSeq) =>
		call(`/api/games/${enc(code)}/simulate`, {
			method: "POST",
			body: atSeq === undefined || atSeq === null ? { moves } : { moves, at_seq: atSeq },
		}),
};
