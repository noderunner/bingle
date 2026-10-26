// Voice chat between the players of a game: WebRTC, peer to peer, audio only.
//
// The browsers send the audio to each other directly. The server keeps the list
// of who is in the call and relays the small messages with which two browsers
// find each other (REST in, WebSocket out; see docs/voice.md). Every pair of
// players has one connection (a mesh), which is fine for the few seats of a
// game. For each pair the higher seat makes the offer, so two players never
// offer at once.
//
// The page redraws on every move, so the call lives here, in module state; the
// card is rebuilt from that state whenever the page asks for it, and updated in
// place when the state changes.
//
// The call belongs to one game and outlives the pages: it has a WebSocket of its
// own (only=voice) and, while it is up, the site moves between pages without
// reloading the document (nav.js). The game that is on screen may be another one
// than the game of the call; the top bar then carries a button for the call
// (topbar.js), and the card of the other game says where the call is.

import { h, store } from "./util.js";
import { api, ApiError } from "./api.js";
import { setKeepAlive, untracked, onNavigate } from "./nav.js";

const HEARTBEAT_MS = 15000;
const SPEAK_LEVEL = 0.035; // RMS of the signal above which somebody counts as speaking

const prefs = {
	ptt: false, // push to talk: the microphone is open only while the key is held
	pttKey: "KeyV",
	input: "", // microphone device id ("" = the browser's default)
	output: "", // speaker device id
	volumes: {}, // by player name: 0..1
	...(store.get("bingle.voice", {}) || {}),
};
const savePrefs = () => store.set("bingle.voice", prefs);

const V = {
	code: null,
	me: { name: null, seat: 0 },
	joined: false,
	joining: false,
	muted: false,
	deafened: false,
	pttHeld: false,
	error: "",
	local: null, // MediaStream of the microphone
	ice: [],
	roster: new Map(), // seat -> name, as the server lists them
	rosterCode: null, // the game the roster is of
	peers: new Map(), // seat -> {seat, name, pc, audio, state, speaking, pending, stream}
	devices: { inputs: [], outputs: [] },
	beat: null,
	meter: null,
	audioCtx: null,
	analysers: new Map(), // key ("me" or seat) -> {node, data}
};

let cardEl = null;
let audioHost = null;
let sock = null; // the call's WebSocket
let sockTries = 0;
let sockTimer = null;
const listeners = new Set(); // who wants to know when the call changes (the top bar)
let viewing = { code: null }; // the game on screen

/** True while a call is up or being joined. */
export function inCall() {
	return V.joined || V.joining;
}
setKeepAlive(inCall);
// A new page is on screen: it is not the game that was.
onNavigate(() => {
	viewing = { code: null };
	cardEl = null;
});

/** What the top bar shows of the call. */
export function voiceStatus() {
	return {
		active: inCall(),
		joined: V.joined,
		code: V.code,
		viewing: viewing.code,
		muted: V.muted,
		deafened: V.deafened,
		open: micOpen(),
		peers: [...V.roster.keys()].filter((seat) => seat !== V.me.seat).length,
	};
}

/** fn(status) is called whenever the call changes; the result stops it. */
export function onVoiceChange(fn) {
	listeners.add(fn);
	return () => listeners.delete(fn);
}

function changed() {
	for (const fn of [...listeners]) {
		try {
			fn(voiceStatus());
		} catch (_) {
			/* a bar that is gone */
		}
	}
}

export function toggleMute() {
	V.muted = !V.muted;
	applyMic();
	paint();
}

export function toggleDeafen() {
	V.deafened = !V.deafened;
	applyMic();
	for (const p of V.peers.values()) applyOutput(p);
	paint();
}

// ---- what the browser can do ----------------------------------------------------------

/** Why voice cannot work here, or "" when it can. */
export function voiceUnsupported() {
	if (!window.isSecureContext) {
		return "Voice chat needs a secure connection: open this page over https:// (or on localhost). Your browser does not allow the microphone on plain http.";
	}
	if (!navigator.mediaDevices || !navigator.mediaDevices.getUserMedia || typeof RTCPeerConnection === "undefined") {
		return "This browser cannot do voice chat (no microphone or WebRTC support).";
	}
	return "";
}

function micError(err) {
	switch (err && err.name) {
		case "NotAllowedError":
		case "SecurityError":
			return "The microphone was blocked. Allow it for this site in the browser (the icon in the address bar), then try again.";
		case "NotFoundError":
		case "OverconstrainedError":
			return "No microphone was found. Plug one in or choose another in the browser's settings.";
		case "NotReadableError":
			return "The microphone is in use by another program.";
		default:
			return (err && err.message) || "The microphone could not be opened.";
	}
}

// ---- who is in the call -----------------------------------------------------------------

function ensureAudioHost() {
	if (!audioHost || !document.body.contains(audioHost)) {
		// data-persist: the page changes around the call and the audio plays on.
		audioHost = h("div", { id: "voice-audio", hidden: true, "data-persist": "1" });
		document.body.append(audioHost);
	}
	return audioHost;
}

function peerFor(seat, name) {
	let p = V.peers.get(seat);
	if (!p) {
		p = { seat, name: name || `Seat ${seat}`, pc: null, audio: null, state: "new", speaking: false, pending: [], stream: null, restarts: 0 };
		V.peers.set(seat, p);
	} else if (name) {
		p.name = name;
	}
	return p;
}

/** Takes the server's list of the call: connects to who is new, drops who left. */
function syncRoster(list) {
	V.roster = new Map((list || []).map((x) => [x.seat, x.name]));
	if (!V.joined) {
		paint();
		return;
	}
	for (const [seat, name] of V.roster) {
		if (seat === V.me.seat) continue;
		const p = peerFor(seat, name);
		// The higher seat of a pair makes the offer.
		if (V.me.seat > seat && !p.pc) startCall(p);
	}
	for (const seat of [...V.peers.keys()]) {
		if (!V.roster.has(seat)) closePeer(seat);
	}
	paint();
}

function closePeer(seat) {
	const p = V.peers.get(seat);
	if (!p) return;
	if (p.pc) {
		p.pc.onicecandidate = p.pc.ontrack = p.pc.onconnectionstatechange = null;
		try {
			p.pc.close();
		} catch (_) {
			/* already closed */
		}
	}
	if (p.audio) {
		p.audio.srcObject = null;
		p.audio.remove();
	}
	V.analysers.delete(seat);
	V.peers.delete(seat);
}

// ---- signalling --------------------------------------------------------------------------

function send(seat, kind, data) {
	return api.voiceSignal(V.code, V.me.name, seat, kind, data).catch((err) => {
		// The other side left, or we did: the roster will say so.
		if (!(err instanceof ApiError) || (err.status !== 409 && err.status !== 403)) console.warn("voice signal failed:", err.message);
	});
}

function makeConnection(p) {
	const pc = new RTCPeerConnection({ iceServers: V.ice });
	p.pc = pc;
	p.state = "connecting";
	for (const track of V.local ? V.local.getTracks() : []) pc.addTrack(track, V.local);
	pc.onicecandidate = (ev) => {
		if (ev.candidate) send(p.seat, "ice", ev.candidate.toJSON());
	};
	pc.ontrack = (ev) => {
		const stream = ev.streams[0] || new MediaStream([ev.track]);
		p.stream = stream;
		if (!p.audio) {
			p.audio = h("audio", { autoplay: true, playsinline: true, "data-voice-audio": p.seat });
			ensureAudioHost().append(p.audio);
		}
		p.audio.srcObject = stream;
		applyOutput(p);
		p.audio.play().catch(() => {});
		watchLevel(p.seat, stream);
	};
	pc.onconnectionstatechange = () => {
		if (V.peers.get(p.seat) !== p || p.pc !== pc) return;
		const st = pc.connectionState;
		p.state = st === "connected" ? "connected" : st === "failed" || st === "closed" ? "failed" : "connecting";
		if (st === "connected") p.restarts = 0;
		if (st === "failed") recover(p);
		paint();
	};
	return pc;
}

/** The higher seat starts the call; the lower one asks for it. */
function recover(p) {
	if (V.me.seat > p.seat) {
		// A new try: the same connection with fresh network paths, a few times.
		if (p.restarts++ < 3) untracked(() => setTimeout(() => V.joined && V.peers.get(p.seat) === p && startCall(p, true), 1500));
	} else {
		send(p.seat, "retry", {});
	}
}

async function startCall(p, restart = false) {
	try {
		if (p.pc && !restart) closePeerConnection(p);
		const pc = p.pc || makeConnection(p);
		const offer = await pc.createOffer({ iceRestart: restart });
		await pc.setLocalDescription(offer);
		await send(p.seat, "offer", { type: pc.localDescription.type, sdp: pc.localDescription.sdp });
	} catch (err) {
		console.warn("voice: could not start a call:", err.message);
		p.state = "failed";
		paint();
	}
}

async function drainCandidates(p) {
	const queued = p.pending;
	p.pending = [];
	for (const c of queued) {
		try {
			await p.pc.addIceCandidate(c);
		} catch (_) {
			/* a stale candidate */
		}
	}
}

async function onOffer(p, data) {
	const pc = p.pc || makeConnection(p);
	await pc.setRemoteDescription(data);
	await drainCandidates(p);
	const answer = await pc.createAnswer();
	await pc.setLocalDescription(answer);
	await send(p.seat, "answer", { type: pc.localDescription.type, sdp: pc.localDescription.sdp });
}

/** A message from another seat (the "rtc" event of the room's socket). */
async function onSignal(sig) {
	if (!V.joined || !sig || sig.from === V.me.seat) return;
	const p = peerFor(sig.from, V.roster.get(sig.from));
	try {
		switch (sig.kind) {
			case "offer":
				await onOffer(p, sig.data);
				break;
			case "answer":
				if (p.pc) {
					await p.pc.setRemoteDescription(sig.data);
					await drainCandidates(p);
				}
				break;
			case "ice":
				if (p.pc && p.pc.remoteDescription) await p.pc.addIceCandidate(sig.data).catch(() => {});
				else p.pending.push(sig.data);
				break;
			case "retry":
				// The lower seat asks the higher one to call again.
				if (V.me.seat > p.seat) {
					if (p.pc) closePeerConnection(p);
					startCall(p);
				}
				break;
			default:
		}
	} catch (err) {
		console.warn("voice: could not handle a", sig.kind, "message:", err.message);
		p.state = "failed";
		paint();
	}
}

function closePeerConnection(p) {
	if (!p.pc) return;
	p.pc.onicecandidate = p.pc.ontrack = p.pc.onconnectionstatechange = null;
	try {
		p.pc.close();
	} catch (_) {
		/* already closed */
	}
	p.pc = null;
	p.pending = [];
}

/** The room's socket delivered an event; handles the voice ones. Returns true if it was one. */
export function voiceEvent(env, code) {
	if (!env) return false;
	// A page of another game than the call says nothing about the call.
	if ((env.type === "rtc" || env.type === "voice") && inCall() && code && code !== V.code) return true;
	if (env.type === "rtc") {
		onSignal(env.data);
		return true;
	}
	if (env.type !== "voice") return false;
	const d = env.data || {};
	if (d.kind === "join") {
		V.roster.set(d.seat, d.name);
		if (V.joined && d.seat !== V.me.seat) {
			const p = peerFor(d.seat, d.name);
			// A seat that comes back replaces its old connection.
			if (p.pc && V.me.seat > d.seat) closePeerConnection(p);
			if (V.me.seat > d.seat && !p.pc) startCall(p);
		}
	} else if (d.kind === "leave") {
		V.roster.delete(d.seat);
		if (d.seat !== V.me.seat) closePeer(d.seat);
	}
	paint();
	return true;
}

/** The room's socket (re)connected: look at the call again. */
export async function voiceResync(code) {
	if (!code || (inCall() && code !== V.code)) return;
	try {
		const r = await api.voice(code);
		syncRoster(r.peers);
	} catch (_) {
		/* the call goes on without the list for now */
	}
}

// ---- the call's socket ---------------------------------------------------------------------

/**
 * The call's own WebSocket: who joins and leaves, and the messages for our seat.
 * The page's socket carries neither, so the call does not depend on the page.
 */
function openSocket() {
	closeSocket();
	if (!V.joined) return Promise.resolve();
	const proto = location.protocol === "https:" ? "wss://" : "ws://";
	const url = `${proto}${location.host}/ws/${encodeURIComponent(V.code)}?name=${encodeURIComponent(V.me.name)}&role=player&only=voice`;
	const mine = new WebSocket(url);
	sock = mine;
	// Resolves once the server has said "joined", which it does after it listens for our seat.
	return new Promise((ready) => {
		const giveUp = window.setTimeout(ready, 4000);
		const done = () => {
			window.clearTimeout(giveUp);
			ready();
		};
		mine.addEventListener("open", () => {
			if (sock !== mine) return;
			sockTries = 0;
			// Whatever was said while the socket was down is in the list.
			voiceResync(V.code);
		});
		mine.addEventListener("message", (ev) => {
			if (sock !== mine) return;
			try {
				const env = JSON.parse(ev.data);
				if (env.type === "joined") done();
				else voiceEvent(env, V.code);
			} catch (_) {
				/* not for us */
			}
		});
		mine.addEventListener("close", () => {
			done();
			if (sock !== mine) return;
			sock = null;
			if (!V.joined) return;
			sockTries = Math.min(sockTries + 1, 6);
			sockTimer = window.setTimeout(() => untracked(openSocket), 700 * 2 ** sockTries);
		});
	});
}

function closeSocket() {
	if (sockTimer) window.clearTimeout(sockTimer);
	sockTimer = null;
	const old = sock;
	sock = null;
	if (old) {
		try {
			old.close();
		} catch (_) {
			/* already closed */
		}
	}
}

// ---- joining and leaving ----------------------------------------------------------------

async function openMicrophone() {
	const audio = { echoCancellation: true, noiseSuppression: true, autoGainControl: true };
	if (prefs.input) audio.deviceId = { ideal: prefs.input };
	return navigator.mediaDevices.getUserMedia({ audio });
}

async function listDevices() {
	try {
		const all = await navigator.mediaDevices.enumerateDevices();
		V.devices = {
			inputs: all.filter((d) => d.kind === "audioinput"),
			outputs: all.filter((d) => d.kind === "audiooutput"),
		};
	} catch (_) {
		V.devices = { inputs: [], outputs: [] };
	}
}

async function join() {
	const why = voiceUnsupported();
	if (why) {
		V.error = why;
		paint();
		return;
	}
	if (V.joined || V.joining || !V.me.name || !V.me.seat) return;
	V.joining = true;
	V.error = "";
	paint();
	try {
		V.local = await openMicrophone();
		await listDevices();
		try {
			V.ice = (await api.rtcConfig()).ice_servers || [];
		} catch (_) {
			V.ice = [];
		}
		V.audioCtx = V.audioCtx || new (window.AudioContext || window.webkitAudioContext)();
		if (V.audioCtx.state === "suspended") V.audioCtx.resume().catch(() => {});
		V.joined = true; // before the server knows us: offers may arrive at once
		applyMic();
		watchLevel("me", V.local);
		// The socket first: the others offer the moment they hear we are in, and a message
		// for a seat nobody listens for is lost.
		await untracked(openSocket);
		const r = await api.setVoice(V.code, V.me.name, true);
		untracked(() => {
			V.beat = window.setInterval(() => {
				api.setVoice(V.code, V.me.name, true)
					.then((x) => syncRoster(x.peers))
					.catch(() => {});
			}, HEARTBEAT_MS);
			V.meter = window.setInterval(measure, 120);
		});
		syncRoster(r.peers);
	} catch (err) {
		teardown();
		V.error = err instanceof ApiError ? err.message : micError(err);
	} finally {
		V.joining = false;
		paint();
	}
}

function teardown() {
	V.joined = false;
	closeSocket();
	if (V.beat) window.clearInterval(V.beat);
	if (V.meter) window.clearInterval(V.meter);
	V.beat = V.meter = null;
	for (const seat of [...V.peers.keys()]) closePeer(seat);
	if (V.local) for (const t of V.local.getTracks()) t.stop();
	V.local = null;
	V.analysers.clear();
	V.pttHeld = false;
}

/** Leaves the call (the server is told unless it is the page going away). */
export function leaveVoice({ quiet = false } = {}) {
	if (!V.joined && !V.joining) return;
	const code = V.code;
	const name = V.me.name;
	teardown();
	if (!quiet) api.setVoice(code, name, false).catch(() => {});
	adoptViewing();
	paint();
	if (viewing.code) voiceResync(viewing.code);
}

/** With no call, the voice state is that of the game on screen. */
function adoptViewing() {
	if (inCall()) return;
	V.code = viewing.code;
	const me = viewing.me;
	V.me = me && me.name && me.seat ? { name: me.name, seat: me.seat } : { name: null, seat: 0 };
	if (V.rosterCode !== viewing.code) {
		V.roster = new Map();
		V.rosterCode = viewing.code;
	}
}

/** Moves the call to the game on screen: leave the one it is in, join this one. */
async function switchHere() {
	const here = viewing;
	if (!here.me || !here.me.seat) return;
	leaveVoice();
	V.code = here.code;
	V.me = { name: here.me.name, seat: here.me.seat };
	await join();
}

// The tab is closing: tell the server at once (the place would lapse in 45 seconds anyway).
untracked(() =>
	window.addEventListener("pagehide", () => {
		if (!V.joined) return;
		try {
			navigator.sendBeacon(`/api/games/${encodeURIComponent(V.code)}/voice`, new Blob([JSON.stringify({ player_name: V.me.name, active: false })], { type: "application/json" }));
		} catch (_) {
			/* the place lapses by itself */
		}
		teardown();
	})
);

// ---- microphone state, push to talk, speakers --------------------------------------------

/** Opens or closes the microphone according to mute, deafen and push to talk. */
function applyMic() {
	if (!V.local) return;
	const open = !V.muted && !V.deafened && (!prefs.ptt || V.pttHeld);
	for (const t of V.local.getAudioTracks()) t.enabled = open;
}

function applyOutput(p) {
	if (!p.audio) return;
	p.audio.muted = V.deafened;
	const vol = prefs.volumes[p.name];
	p.audio.volume = typeof vol === "number" ? Math.min(1, Math.max(0, vol)) : 1;
	if (prefs.output && typeof p.audio.setSinkId === "function") p.audio.setSinkId(prefs.output).catch(() => {});
}

function typing(target) {
	return !!target && (/^(INPUT|TEXTAREA|SELECT)$/.test(target.tagName) || target.isContentEditable);
}

untracked(() => {
	document.addEventListener("keydown", (ev) => {
		if (!V.joined || !prefs.ptt || ev.code !== prefs.pttKey || ev.repeat || typing(ev.target)) return;
		V.pttHeld = true;
		applyMic();
		paint();
	});
	document.addEventListener("keyup", (ev) => {
		if (!V.joined || !prefs.ptt || ev.code !== prefs.pttKey) return;
		V.pttHeld = false;
		applyMic();
		paint();
	});
	window.addEventListener("blur", () => {
		if (V.pttHeld) {
			V.pttHeld = false;
			applyMic();
			paint();
		}
	});
});

async function chooseInput(id) {
	prefs.input = id;
	savePrefs();
	if (!V.joined) return;
	try {
		const fresh = await openMicrophone();
		const track = fresh.getAudioTracks()[0];
		for (const p of V.peers.values()) {
			const sender = p.pc && p.pc.getSenders().find((s) => s.track && s.track.kind === "audio");
			if (sender) await sender.replaceTrack(track);
		}
		for (const t of V.local.getTracks()) t.stop();
		V.local = fresh;
		applyMic();
		watchLevel("me", fresh);
	} catch (err) {
		V.error = micError(err);
	}
	paint();
}

function chooseOutput(id) {
	prefs.output = id;
	savePrefs();
	for (const p of V.peers.values()) applyOutput(p);
}

// ---- who is speaking --------------------------------------------------------------------

function watchLevel(key, stream) {
	if (!V.audioCtx || !stream.getAudioTracks().length) return;
	try {
		const node = V.audioCtx.createAnalyser();
		node.fftSize = 512;
		V.audioCtx.createMediaStreamSource(stream).connect(node); // not to the speakers: the <audio> plays it
		V.analysers.set(key, { node, data: new Uint8Array(node.fftSize) });
	} catch (_) {
		/* no indicator, the call still works */
	}
}

function measure() {
	for (const [key, a] of V.analysers) {
		a.node.getByteTimeDomainData(a.data);
		let sum = 0;
		for (const v of a.data) sum += ((v - 128) / 128) ** 2;
		let on = Math.sqrt(sum / a.data.length) > SPEAK_LEVEL;
		const seat = key === "me" ? V.me.seat : key;
		if (key === "me") on = on && V.local && V.local.getAudioTracks().some((t) => t.enabled);
		const p = key === "me" ? null : V.peers.get(key);
		if (p) p.speaking = on;
		for (const el of document.querySelectorAll(`[data-voice-seat="${seat}"]`)) el.classList.toggle("speaking", !!on);
	}
}

// ---- the card ----------------------------------------------------------------------------

const STATE_TEXT = { new: "waiting", connecting: "connecting…", connected: "connected", failed: "no connection" };

function peerRow(seat, name, me) {
	const p = V.peers.get(seat);
	const state = me ? "connected" : p ? p.state : "new";
	const row = h(
		"li",
		{ class: "voice-peer", "data-voice-seat": seat, "data-state": state, "data-me": me ? "1" : null },
		h("span", { class: "voice-dot", "aria-hidden": "true" }),
		h("span", { class: "voice-name" }, me ? `${name} (you)` : name),
		me
			? h("span", { class: "voice-state small muted" }, V.deafened ? "deafened" : micOpen() ? "mic on" : "mic off")
			: h("span", { class: "voice-state small muted" }, STATE_TEXT[state] || state)
	);
	if (!me && p) {
		const vol = h("input", {
			type: "range",
			min: "0",
			max: "100",
			value: String(Math.round((typeof prefs.volumes[p.name] === "number" ? prefs.volumes[p.name] : 1) * 100)),
			class: "voice-volume",
			"aria-label": `Volume of ${p.name}`,
			title: "Volume",
		});
		vol.addEventListener("input", () => {
			prefs.volumes[p.name] = Number(vol.value) / 100;
			savePrefs();
			applyOutput(p);
		});
		row.append(vol);
		if (p.state === "failed") {
			row.append(
				h(
					"button",
					{
						type: "button",
						class: "ghost voice-retry",
						onclick: () => {
							p.restarts = 0;
							if (V.me.seat > p.seat) {
								closePeerConnection(p);
								startCall(p);
							} else {
								send(p.seat, "retry", {});
							}
							p.state = "connecting";
							paint();
						},
					},
					"Retry"
				)
			);
		}
	}
	return row;
}

function micOpen() {
	return !!V.local && V.local.getAudioTracks().some((t) => t.enabled);
}

function deviceSelect(label, id, devices, value, onchange) {
	const sel = h("select", { id, "aria-label": label });
	sel.append(h("option", { value: "" }, "Default"));
	devices.forEach((d, i) => sel.append(h("option", { value: d.deviceId }, d.label || `${label} ${i + 1}`)));
	sel.value = devices.some((d) => d.deviceId === value) ? value : "";
	sel.addEventListener("change", () => onchange(sel.value));
	return h("label", { class: "voice-device small" }, h("span", {}, label), sel);
}

function fill(card) {
	card.textContent = "";
	const names = [...V.roster.entries()].filter(([seat]) => seat !== V.me.seat).map(([, n]) => n);
	const head = h("h3", {}, "Voice ", h("span", { class: "badge" + (V.joined ? " turn" : ""), id: "voice-badge" }, V.joined ? "in call" : V.joining ? "joining…" : "off"));
	card.append(head);

	const why = voiceUnsupported();
	if (why) {
		card.append(h("p", { class: "small muted", id: "voice-unsupported" }, why));
		return;
	}
	// A call in another game: where it is, and a way to bring it here.
	if (inCall() && viewing.code && V.code !== viewing.code) {
		card.append(
			h("p", { class: "small", id: "voice-elsewhere" }, "You are in the voice chat of ", h("a", { href: `/game/${V.code}` }, `game ${V.code}`), ".")
		);
		const talkHere = viewing.me && viewing.me.seat && (viewing.seats || []).some((s) => s.seat !== viewing.me.seat && !s.bot);
		if (talkHere) card.append(h("button", { type: "button", class: "primary", id: "voice-switch", onclick: switchHere }, "Talk in this game instead"));
	}
	if (V.error) card.append(h("p", { class: "small error", id: "voice-error", role: "alert" }, V.error));

	if (!V.joined) {
		card.append(
			h(
				"p",
				{ class: "small muted", id: "voice-hint" },
				names.length ? `${names.join(", ")} ${names.length === 1 ? "is" : "are"} in the voice chat.` : "Talk to the other players while you play."
			),
			h("button", { type: "button", class: "primary", id: "voice-join", disabled: V.joining, onclick: join }, V.joining ? "Joining…" : "Join voice"),
			h(
				"p",
				{ class: "small muted" },
				"Audio goes straight between the players' browsers, not through the server. The other players can see your network address."
			)
		);
		return;
	}

	const list = h("ul", { class: "voice-peers", id: "voice-peers" });
	list.append(peerRow(V.me.seat, V.me.name, true));
	for (const [seat, name] of V.roster) if (seat !== V.me.seat) list.append(peerRow(seat, name, false));
	card.append(list);

	const buttons = h(
		"div",
		{ class: "row voice-buttons" },
		h(
			"button",
			{
				type: "button",
				id: "voice-mute",
				"aria-pressed": V.muted ? "true" : "false",
				class: V.muted ? "danger" : "",
				onclick: toggleMute,
			},
			V.muted ? "Unmute" : "Mute"
		),
		h(
			"button",
			{
				type: "button",
				id: "voice-deafen",
				"aria-pressed": V.deafened ? "true" : "false",
				class: V.deafened ? "danger" : "",
				title: "Stop hearing the others (this also mutes you)",
				onclick: toggleDeafen,
			},
			V.deafened ? "Undeafen" : "Deafen"
		),
		h("button", { type: "button", id: "voice-leave", class: "ghost", onclick: () => leaveVoice() }, "Leave")
	);
	card.append(buttons);

	const ptt = h("input", { type: "checkbox", id: "voice-ptt" });
	ptt.checked = !!prefs.ptt;
	ptt.addEventListener("change", () => {
		prefs.ptt = ptt.checked;
		savePrefs();
		V.pttHeld = false;
		applyMic();
		paint();
	});
	const keyName = prefs.pttKey.replace(/^Key/, "").replace(/^Digit/, "");
	const keyBtn = h("button", { type: "button", class: "ghost", id: "voice-ptt-key", title: "Choose the push-to-talk key" }, keyName);
	keyBtn.addEventListener("click", () => {
		keyBtn.textContent = "press a key…";
		const grab = (ev) => {
			ev.preventDefault();
			ev.stopPropagation();
			document.removeEventListener("keydown", grab, true);
			if (ev.code !== "Escape") {
				prefs.pttKey = ev.code;
				savePrefs();
			}
			paint();
		};
		document.addEventListener("keydown", grab, true);
	});
	card.append(
		h(
			"div",
			{ class: "voice-ptt small" },
			h("label", { class: "check" }, ptt, h("span", {}, "Push to talk")),
			prefs.ptt ? h("span", {}, "hold ", keyBtn) : null,
			prefs.ptt && V.pttHeld ? h("span", { class: "badge turn" }, "talking") : null
		)
	);
	if (V.devices.inputs.length > 1 || V.devices.outputs.length > 1) {
		card.append(
			h(
				"div",
				{ class: "voice-devices" },
				V.devices.inputs.length > 1 ? deviceSelect("Microphone", "voice-input", V.devices.inputs, prefs.input, chooseInput) : null,
				V.devices.outputs.length > 1 && typeof HTMLMediaElement.prototype.setSinkId === "function"
					? deviceSelect("Speakers", "voice-output", V.devices.outputs, prefs.output, chooseOutput)
					: null
			)
		);
	}
}

function paint() {
	if (cardEl && document.body.contains(cardEl)) fill(cardEl);
	changed();
}

/**
 * The voice card for the game page. Only people with a seat can talk, and only
 * when there is somebody to talk to; anybody else gets nothing. `seats` is the
 * game's seat list ({seat, name, bot}). While a call is up in another game the
 * card says so, whoever you are here.
 */
export function voiceCard({ code, me, seats }) {
	viewing = { code, me, seats };
	const away = inCall() && V.code !== code;
	if (!away) {
		// Another player on this very game (a game against oneself): the call is theirs, so it ends.
		if (V.joined && me && me.name && me.seat && (V.me.name !== me.name || V.me.seat !== me.seat)) leaveVoice({ quiet: true });
		adoptViewing();
		const others = (seats || []).some((s) => s.seat !== V.me.seat && !s.bot);
		if (!V.me.seat || (!others && !V.joined)) {
			cardEl = null;
			return null;
		}
	}
	cardEl = h("div", { class: "card voice", id: "voice-card" });
	fill(cardEl);
	return cardEl;
}
