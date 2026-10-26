// Thin fetch wrapper around bingle's REST API.

import { setSeatToken } from "./util.js";

export class ApiError extends Error {
	constructor(message, status) {
		super(message);
		this.status = status;
	}
}

async function call(path, { method = "GET", body, headers } = {}) {
	const init = { method, headers: { ...(headers || {}) } };
	if (body !== undefined) {
		init.headers["Content-Type"] = "application/json";
		init.body = JSON.stringify(body);
	}
	let resp;
	try {
		resp = await fetch(path, init);
	} catch (err) {
		throw new ApiError("Could not reach the server.", 0);
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

export const api = {
	batches: () => call("/api/batches"),
	startBatch: (body) => call("/api/batches", { method: "POST", body }),
	batchReport: (id) => call(`/api/batches/${enc(id)}/report`),
	batchGames: (id) => call(`/api/batches/${enc(id)}/games`),
	batchEvents: (id, n) => call(`/api/batches/${enc(id)}/games/${n}/events`),
	cancelBatch: (id) => call(`/api/batches/${enc(id)}/cancel`, { method: "POST" }),
	deleteBatch: (id) => call(`/api/batches/${enc(id)}`, { method: "DELETE" }),
	gameDefs: () => call("/api/game-defs"),
	gameDef: (id) => call(`/api/game-defs/${enc(id)}`),
	guide: (id) => call(`/api/game-defs/${enc(id)}/guide`),
	bots: () => call("/api/bots"),
	puzzles: (gameDefID) => call("/api/puzzles" + (gameDefID ? `?game_def_id=${enc(gameDefID)}` : "")),
	puzzle: (id, reveal) => call(`/api/puzzles/${enc(id)}${reveal ? "?reveal=1" : ""}`),
	createGame: (body) =>
		call("/api/games", { method: "POST", body }).then((res) => {
			if (res && res.room_code && res.seat_token) setSeatToken(res.room_code, res.seat_token);
			return res;
		}),
	importGame: (body) => call("/api/games/import", { method: "POST", body }),
	botMatch: (body) => call("/api/bot-matches", { method: "POST", body }),
	game: (code) => call(`/api/games/${enc(code)}`),
	join: (code, name, role = "player", seat) =>
		call(`/api/games/${enc(code)}/join`, { method: "POST", body: { player_name: name, role, seat: seat || undefined } }).then((res) => {
			if (res && res.seat_token) setSeatToken(String(code).toUpperCase(), res.seat_token);
			return res;
		}),
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
	gameNotify: (code, name, token) => call(`/api/games/${enc(code)}/notify?player_name=${enc(name)}`, { headers: { "X-Seat-Token": token } }),
	setGameNotify: (code, body) => call(`/api/games/${enc(code)}/notify`, { method: "PUT", body }),
	voice: (code) => call(`/api/games/${enc(code)}/voice`),
	setVoice: (code, name, active) => call(`/api/games/${enc(code)}/voice`, { method: "POST", body: { player_name: name, active } }),
	voiceSignal: (code, name, to, kind, data) => call(`/api/games/${enc(code)}/voice/signal`, { method: "POST", body: { player_name: name, to, kind, data } }),
	state: (code, atSeq, name) => {
		const q = [];
		if (atSeq !== undefined && atSeq !== null) q.push(`at_seq=${atSeq}`);
		if (name) q.push(`player_name=${enc(name)}`);
		return call(`/api/games/${enc(code)}/state${q.length ? "?" + q.join("&") : ""}`);
	},
	moves: (code) => call(`/api/games/${enc(code)}/moves`),
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
