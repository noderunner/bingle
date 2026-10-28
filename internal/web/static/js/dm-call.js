// The voice call of a direct message: one person talking to one other, audio
// only, peer to peer (WebRTC). The audio goes straight between the two
// browsers; the server only relays the small messages with which they find each
// other (POST /api/dm/signal in, the /ws-dm socket out), as a game's call does.
//
// A game's call is a mesh of seats and lives in js/voice.js; this is the
// one-to-one case, kept apart because it is addressed by person, not by seat.
// Both of them obey one rule: a person is in one call at a time, which the
// server keeps (joining a call leaves the other) and both modules honour here.
//
// The lower of the two addresses makes the offer, so the two never offer at
// once — the same trick as the higher seat offering in a game.

import { h } from "./util.js";
import { api } from "./api.js";
import { sessionSecret } from "./util.js";
import { leaveVoice, inCall as inGameCall, voiceUnsupported, micStream, iceServers } from "./voice.js";

const C = {
	peer: "", // the address of the person we are talking to
	name: "", // what they go by
	state: "", // "", "calling", "ringing", "live"
	muted: false,
	error: "",
	local: null, // our microphone
	pc: null,
	audio: null,
	pending: [], // ICE candidates that arrived before the description
	me: "", // our own address
};

const listeners = new Set();

/** What the call is doing, for the window to draw. */
export function callState() {
	return { peer: C.peer, name: C.name, state: C.state, muted: C.muted, error: C.error };
}

/** Calls fn whenever that changes; returns a function that stops it. */
export function onCallChange(fn) {
	listeners.add(fn);
	return () => listeners.delete(fn);
}

function changed() {
	for (const fn of listeners) {
		try {
			fn(callState());
		} catch (err) {
			console.error("direct call listener failed:", err);
		}
	}
}

/** True while a direct call is up or being set up. */
export function inDirectCall() {
	return C.state !== "";
}

/** Somebody is calling us: the window offers to answer. */
export function ringing(from, name) {
	if (C.state === "live" || C.state === "calling") {
		// Already busy: the window shows who else wants to talk.
		if (C.peer !== from) return;
	}
	if (C.state === "" || C.peer === from) {
		C.peer = from;
		C.name = name || C.name;
		if (C.state === "") C.state = "ringing";
		else if (C.state === "calling") start().catch(() => {});
		changed();
	}
}

/** Rings somebody (or answers them): the call is on once both have. */
export async function callPerson(address, name) {
	const why = voiceUnsupported();
	if (why) {
		C.error = why;
		changed();
		return;
	}
	const answering = C.state === "ringing" && C.peer === address;
	// One voice at a time: leave a game's call before taking this one up.
	if (inGameCall()) leaveVoice();
	C.peer = address;
	C.name = name || C.name;
	C.error = "";
	C.state = answering ? "live" : "calling";
	changed();
	try {
		await api.directCall(sessionSecret(), address, true);
	} catch (err) {
		C.error = err.message || "The call could not be made.";
		hangUp({ quiet: true });
		return;
	}
	if (answering) await start();
}

/** Ends the call (the other side is told unless the page is going away). */
export function hangUp({ quiet = false } = {}) {
	const had = C.state !== "";
	teardown();
	if (had && !quiet) api.directCall(sessionSecret(), C.peer, false).catch(() => {});
	C.peer = "";
	C.name = "";
	C.state = "";
	changed();
}

/** The other side hung up, or the server ended the call. */
export function ended(from, dropped) {
	if (from && C.peer && from !== C.peer) return;
	teardown();
	C.state = "";
	C.error = dropped ? "The call ended: you joined another one." : "";
	changed();
}

export function toggleMute() {
	C.muted = !C.muted;
	applyMic();
	changed();
}

function applyMic() {
	if (!C.local) return;
	for (const track of C.local.getAudioTracks()) track.enabled = !C.muted;
}

function teardown() {
	if (C.pc) {
		try {
			C.pc.close();
		} catch (_) {
			/* already closed */
		}
	}
	C.pc = null;
	if (C.audio) {
		C.audio.srcObject = null;
		C.audio.remove();
	}
	C.audio = null;
	if (C.local) for (const t of C.local.getTracks()) t.stop();
	C.local = null;
	C.pending = [];
	C.muted = false;
}

function audioHost() {
	let host = document.getElementById("dm-audio");
	if (!host) {
		host = h("div", { id: "dm-audio", "data-persist": "1", hidden: true });
		document.body.append(host);
	}
	return host;
}

async function connection() {
	if (C.pc) return C.pc;
	C.local = C.local || (await micStream());
	applyMic();
	const pc = new RTCPeerConnection({ iceServers: await iceServers() });
	C.pc = pc;
	for (const track of C.local.getTracks()) pc.addTrack(track, C.local);
	pc.onicecandidate = (ev) => {
		if (ev.candidate) signal("ice", ev.candidate.toJSON());
	};
	pc.ontrack = (ev) => {
		const stream = ev.streams[0] || new MediaStream([ev.track]);
		if (!C.audio) {
			C.audio = h("audio", { autoplay: true, playsinline: true, "data-dm-audio": "1" });
			audioHost().append(C.audio);
		}
		C.audio.srcObject = stream;
		C.audio.play().catch(() => {});
	};
	pc.onconnectionstatechange = () => {
		if (C.pc !== pc) return;
		if (pc.connectionState === "connected") C.state = "live";
		if (pc.connectionState === "failed") C.error = "The call could not be connected.";
		changed();
	};
	return pc;
}

/** The lower address offers; the other waits for it. */
function weOffer() {
	return C.me && C.peer && C.me < C.peer;
}

/** Sets the call up once both sides want it. */
async function start() {
	C.state = "live";
	changed();
	try {
		const pc = await connection();
		if (!weOffer()) return; // the other side offers; we answer
		const offer = await pc.createOffer();
		await pc.setLocalDescription(offer);
		await signal("offer", { type: pc.localDescription.type, sdp: pc.localDescription.sdp });
	} catch (err) {
		C.error = err && err.message ? err.message : "The microphone could not be opened.";
		changed();
	}
}

function signal(kind, data) {
	return api.directSignal(sessionSecret(), C.peer, kind, data).catch(() => {});
}

/** A WebRTC message from the other side (the "dm-rtc" event of /ws-dm). */
export async function directSignal(sig) {
	if (!sig || !C.peer || sig.from !== C.peer) return;
	try {
		switch (sig.kind) {
			case "offer": {
				const pc = await connection();
				await pc.setRemoteDescription(sig.data);
				await drain();
				const answer = await pc.createAnswer();
				await pc.setLocalDescription(answer);
				await signal("answer", { type: pc.localDescription.type, sdp: pc.localDescription.sdp });
				C.state = "live";
				changed();
				break;
			}
			case "answer":
				if (C.pc) {
					await C.pc.setRemoteDescription(sig.data);
					await drain();
				}
				break;
			case "ice": {
				const c = new RTCIceCandidate(sig.data);
				if (C.pc && C.pc.remoteDescription) await C.pc.addIceCandidate(c);
				else C.pending.push(c);
				break;
			}
			case "retry":
				if (weOffer()) await start();
				break;
		}
	} catch (err) {
		console.warn("direct call:", err.message);
	}
}

async function drain() {
	const queued = C.pending;
	C.pending = [];
	for (const c of queued) {
		try {
			await C.pc.addIceCandidate(c);
		} catch (_) {
			/* a stale candidate */
		}
	}
}

/** The address of this browser, learnt from the socket. */
export function setMyAddress(address) {
	C.me = address || C.me;
}
