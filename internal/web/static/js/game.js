// Game page controller: loads a room, keeps it live over a WebSocket, lets
// the player build moves click by click (driven by the server's list of
// legal moves), and offers history navigation, undo, resign, export and
// turn notifications. See docs/ui.md for the interaction model.

import { h, s, clear, store, rememberGame, nameFor, lastName, ago, titleCase, toast, setTurnBadge } from "./util.js";
import { api, ApiError } from "./api.js";
import { MoveBuilder } from "./builder.js";
import { renderBoard, pieceIcon, cssColor, ONGATE } from "./render.js";
import { showRules as openRules } from "./rules.js";
import { loadChat, receiveChat, chatCard, chatSeen, chatUnread } from "./chat.js";
import { voiceCard, voiceEvent, voiceResync } from "./voice.js";
import { installNav, navigate } from "./nav.js";
import { installTopbar } from "./topbar.js";
import { mountGameNotify, applyProfileDefault } from "./notify-ui.js";
import { LAYOUTS, isMobileLayout, layoutChoice, setLayoutChoice, mobileShell, onScreenChange } from "./mobile.js";
import { lobbyCard } from "./lobby.js";
import { statIcon } from "./icons.js";
import { hoverCard, hideHoverCard } from "./tooltip.js";
import { createOptionsWindow } from "./options.js";
import { cardTable, choiceGrid, hideCardZoom } from "./cards.js";
import { rulesIcon } from "./ui-icons.js";
import { choiceIcon } from "./choice-icons.js";
import { animalIcon, animalOf } from "./animals.js";
import { themesFor, themeById } from "./themes.js";
import { progress as tutorialProgress, isExpected, markFinished } from "./tutorial.js";
import { renderMarkdown } from "./markdown.js";

installNav();

const root = document.getElementById("game-root");
const code = document.body.dataset.roomCode;

const S = {
	present: new Set(), // seats whose players say they are watching right now
	code,
	info: null,
	game: null,
	state: null, // full position document of the live position
	moves: [],
	viewSeq: null, // null = live; otherwise the move number being viewed
	viewCache: new Map(),
	legal: [],
	builder: new MoveBuilder([]),
	preview: null, // {notation, cells} for a complete, not-yet-submitted move
	previewToken: 0,
	me: { name: null, seat: 0 },
	// The seats this browser plays: one, or — in a game against oneself —
	// several, each named "<name> · <faction>". `me` follows the one to move.
	mine: [],
	homeSeat: 0, // the first of them: the viewpoint of the page
	flipped: false,
	flippedByUser: false,
	busy: false,
	skewMs: 0, // server clock minus local clock, for countdowns
	analysis: null, // {base, moves: [notation], sim, builder} while exploring variations
	hint: null, // {notation, steps, bot, key}: a bot's suggestion for the position shown
	hintBusy: false,
	conn: "connecting",
	confirmUndo: false,
	confirmResign: false,
	settings: {
		confirm: store.get("bingle.confirm", true),
		notify: store.get("bingle.notify", false),
		coords: store.get("bingle.coords", true),
		hints: store.get("bingle.hints", true),
		lastMove: store.get("bingle.lastMove", true),
		animate: store.get("bingle.animate", true),
		board: store.get("bingle.board", "wood"),
		// Size of the big piece buttons in the player panes (a multiplier).
		tileScale: Number(store.get("bingle.tileScale", 1)) || 1,
		// Which layers of an area map are drawn.
		names: store.get("bingle.layer.names", true),
		// null until the player chooses: then the glyphs show when Yellow Sign is at the table.
		glyphs: store.get("bingle.layer.glyphs", null),
		sigils: store.get("bingle.layer.sigils", true),
	},
	lastPiece: null, // piece type most recently placed (quick placement)
	inspect: null, // square under the pointer, for the stack inspector
	unitInfo: null, // {pos, piece, color}: the unit whose card is open (area maps)
	autoplay: null, // interval handle while history auto-play runs
	fatal: null,
	retired: null, // {name, moves}: a game the server no longer offers, shown as a record
	tut: null, // {family, ref, doc, read, hint (the step whose hint shows), pending}: the tutorial this room plays (ADR 0077)
	spectating: !!store.get("bingle.watch." + code, false),
	bots: null, // built-in bots, fetched when a waiting table could use one
	reactions: null, // {abilities, targeted, ask_all, ask_seats, others, pending}: this player's reaction settings
	autoPlayTimeout: null, // {enabled, timeout_seconds, deadline}: this player's Auto-Play Timeout
	cults: null, // the cults an open seat can be given in the lobby ([] when the game has none)
	lobbyBusy: false,
};

let ws = null;
let wsRetry = 0;
let wsStopped = false;
let titleBase = "Bingle";
let confirmTimer = null;

// ---------------------------------------------------------------------------
// Boot
// ---------------------------------------------------------------------------

async function boot() {
	initOptions();
	// The games going, a way to join one and the profile; the title and the icon of the tab are this game's.
	installTopbar({ manageTitle: false });
	try {
		S.game = await api.game(code);
		if (S.game.retired) {
			// The server no longer offers this game: what was played is a record.
			S.retired = { name: S.game.game_name || S.game.game_def_id, moves: (await api.moves(code)) || [] };
			titleBase = S.retired.name;
			render();
			return;
		}
		S.info = await api.gameDef(S.game.game_def_id);
		if (S.game.tutorial) await loadTutorial();
	} catch (err) {
		S.fatal = err instanceof ApiError && err.status === 404 ? `No game with room code “${code}”.` : String(err.message || err);
		render();
		return;
	}
	titleBase = S.info.name;
	noteServerTime(S.game.server_time);
	resolveIdentity();
	await Promise.all([loadPosition(), loadChat(code)]);
	if (S.me.seat && !S.flippedByUser) S.flipped = defaultFlipped();
	onScreenChange(() => render());
	connect();
	render();
	try {
		// The page lays out differently on wide screens: redraw across the line.
		window.matchMedia("(min-width: 1500px)").addEventListener("change", () => render());
	} catch (_) {
		/* old browsers keep the layout they opened with */
	}
	const wanted = /^#move=(\d+)$/.exec(location.hash || "");
	if (wanted && Number(wanted[1]) < S.moves.length) viewMove(Number(wanted[1]));
	window.setInterval(() => {
		const el = document.getElementById("last-move-age");
		if (el) el.textContent = lastMoveAgeText();
	}, 30000);
	clockTimer = window.setInterval(tickClocks, 1000);
}

function resolveIdentity() {
	// The seat the server says is this browser's (its guest's or account's),
	// when this browser has no name of its own for the game: a phone that
	// adopted the guest key of the laptop that took the seat.
	const yours = (S.game.seats || []).find((s) => s.yours && !s.bot);
	const name = nameFor(code) || (yours ? yours.name : null);
	S.me = { name: null, seat: 0 };
	S.mine = [];
	S.homeSeat = 0;
	if (!name) return;
	const lower = name.toLowerCase();
	const seats = S.game.seats || [];
	const seat = seats.find((s) => s.name && s.name.toLowerCase() === lower);
	if (seat) {
		// The seats played besides one's own in a game against oneself are named
		// after their player: "Alice · Black Goat".
		S.mine = seats
			.filter((s) => s.name && (s.seat === seat.seat || s.name.toLowerCase().startsWith(lower + " · ")))
			.map((s) => ({ seat: s.seat, name: s.name }));
		S.homeSeat = seat.seat;
		S.me = { name: seat.name, seat: seat.seat };
		// In a tutorial the learner plays their own seat; the page plays the others.
		if (S.tut) {
			const learner = S.tut.doc.seat || 1;
			const own = S.mine.find((m) => m.seat === learner);
			if (own) {
				S.mine = [own];
				S.me = { name: own.name, seat: own.seat };
				S.homeSeat = own.seat;
			}
		}
		syncMe();
		if (!S.flippedByUser) S.flipped = defaultFlipped();
	}
}

/** Several seats are mine: `me` is the one to move (else the one I last was). */
function syncMe() {
	if (S.mine.length < 2 || !S.game) return;
	const onMove = isActive() ? S.mine.find((m) => m.seat === S.game.turn_seat) : null;
	const cur = onMove || S.mine.find((m) => m.seat === S.me.seat) || S.mine[0];
	S.me = { name: cur.name, seat: cur.seat };
}

/** Whether the seat is played from this browser. */
function isMine(seat) {
	return S.mine.some((m) => m.seat === seat);
}

function defaultFlipped() {
	// Put the viewer's home side at the bottom of the board. "Home" is where
	// their pieces start; games that start from an empty board (Go, Tak,
	// Hex, ...) have no home side, so the board is left as drawn.
	const b = S.info.board;
	if (b.kind !== "grid" || !S.me.seat) return false;
	const color = colorOf(S.me.seat);
	const ys = new Map(b.positions.map((p) => [p.id, p.y]));
	let sum = 0;
	let n = 0;
	for (const [pos, stack] of Object.entries((S.info.start && S.info.start.cells) || {})) {
		if (stack.length && stack[stack.length - 1].color === color && ys.has(pos)) {
			sum += ys.get(pos);
			n++;
		}
	}
	if (n === 0) return false;
	return sum / n < (b.height - 1) / 2;
}

async function loadPosition() {
	const [st, mv] = await Promise.all([api.state(code, null, S.me.name), api.moves(code)]);
	S.state = st.state;
	S.moves = mv;
	S.game.status = st.status;
	S.game.turn_seat = st.turn_seat;
	S.game.move_seq = st.move_seq;
	S.game.winner_seat = st.winner_seat;
	S.game.end_reason = st.end_reason;
	S.game.end_detail = st.end_detail;
	S.viewCache.clear();
	if (S.viewSeq !== null && S.viewSeq >= S.moves.length) S.viewSeq = null;
	await Promise.all([loadLegal(), loadReactions(), loadAutoPlay()]);
}

/** The reactions of this player's faction and when they want to be asked. */
async function loadReactions() {
	// (Playing several seats oneself, there is nobody to pass for: the question
	// is simply asked of each seat in turn.)
	if (!isRuleset() || !S.me.seat || !S.me.name || S.game.status === "waiting" || S.mine.length > 1) {
		S.reactions = null;
		updateReactionDock();
		return;
	}
	const epoch = reactionsEpoch;
	try {
		const res = await api.reactions(code, S.me.name);
		// A choice made while this answer was on its way is newer than it.
		if (epoch === reactionsEpoch) S.reactions = res;
	} catch (err) {
		// A hiccup keeps what the page knows; a refusal means there is nothing to set.
		if (epoch === reactionsEpoch && err instanceof ApiError && err.status >= 400 && err.status < 500) S.reactions = null;
	}
	updateReactionDock();
}

async function refreshGame() {
	S.game = await api.game(code);
	// Another cult was given to an open seat in the lobby: the game is another
	// definition now, with other names and colors.
	if (S.info && S.game.game_def_id !== S.info.id) {
		S.info = await api.gameDef(S.game.game_def_id);
		titleBase = S.info.name;
	}
	noteServerTime(S.game.server_time);
	resolveIdentity();
	await loadPosition();
}

async function loadLegal() {
	S.builder = new MoveBuilder([]);
	S.preview = null;
	S.legal = [];
	if (isMyTurn()) {
		try {
			const res = await api.legal(code);
			S.legal = res.moves || [];
		} catch (_) {
			S.legal = [];
		}
		// A tutorial accepts only the move its step asks for: the board offers
		// nothing else to click.
		if (S.tut) {
			const p = tutorialState();
			S.legal = S.legal.filter((m) => isExpected(p.next, m.notation));
		}
	}
	MoveBuilder.manual = (rulesetView() || {}).panel_actions || [];
	S.builder = new MoveBuilder(S.legal);
	S.builder.autoAdvance();
	// With one move on offer, a move that is only an action ("done") would
	// be taken as forced and wait unsent: the learner presses its button.
	if (S.tut) {
		const b = S.builder;
		while (b.finished() && b.steps.length && b.steps[b.steps.length - 1].kind === "action") b.pop();
	}
}

// ---------------------------------------------------------------------------
// Derived state
// ---------------------------------------------------------------------------

function noteServerTime(iso) {
	const t = Date.parse(iso || "");
	if (!Number.isNaN(t)) S.skewMs = t - Date.now();
}

/** Milliseconds left on the current turn's clock, or null when untimed. */
function msLeft() {
	const g = S.game;
	if (!g || g.status !== "active" || !g.turn_deadline) return null;
	const deadline = Date.parse(g.turn_deadline);
	if (Number.isNaN(deadline)) return null;
	return deadline - (Date.now() + S.skewMs);
}

function formatClock(ms) {
	if (ms <= 0) return "0:00";
	const total = Math.ceil(ms / 1000);
	const d = Math.floor(total / 86400);
	const h = Math.floor((total % 86400) / 3600);
	const m = Math.floor((total % 3600) / 60);
	const sec = total % 60;
	if (d > 0) return `${d}d ${h}h`;
	if (h > 0) return `${h}:${String(m).padStart(2, "0")}:${String(sec).padStart(2, "0")}`;
	return `${m}:${String(sec).padStart(2, "0")}`;
}

let clockTimer = null;
let expiryRefreshed = false;

/** The game clock of this game (a chess clock), or null without one. */
function gameClock() {
	const c = S.game && S.game.clock;
	return c && Array.isArray(c.seats) && c.seats.length ? c : null;
}

/**
 * Milliseconds left on a seat's own clock: the seat whose clock is running
 * counts down to the turn's deadline, the others hold what they had.
 */
function seatMsLeft(seat) {
	const c = gameClock();
	if (!c) return null;
	const own = c.seats.find((s) => s.seat === seat);
	if (!own) return null;
	if (c.running === seat) {
		const live = msLeft();
		if (live !== null) return live;
	}
	return own.seconds_left * 1000;
}

function tickClocks() {
	const left = msLeft();
	for (const el of document.querySelectorAll("[data-clock]")) {
		// data-clock is a seat number for a game clock, "turn" for the
		// allowance of the turn in progress.
		const seat = Number(el.dataset.clock);
		const ms = seat ? seatMsLeft(seat) : left;
		if (ms === null) {
			el.textContent = "";
			continue;
		}
		el.textContent = "⏱ " + formatClock(ms);
		el.classList.toggle("low", ms < 60000);
		el.classList.toggle("running", !seat || (gameClock() || {}).running === seat);
	}
	if (left !== null && left <= 0 && !expiryRefreshed) {
		// The server ends the game on its next look at it; ask it to look.
		expiryRefreshed = true;
		refreshGame()
			.then(() => {
				expiryRefreshed = false;
				render();
			})
			.catch(() => {
				expiryRefreshed = false;
			});
	}
}

function seatInfo(seat) {
	return (S.game.seats || []).find((s) => s.seat === seat) || null;
}

function seatDef(seat) {
	return S.info.seats.find((s) => s.seat === seat) || null;
}

function isActive() {
	return S.game && S.game.status === "active";
}

function isMyTurn() {
	return isActive() && S.me.seat !== 0 && S.game.turn_seat === S.me.seat;
}

function isLive() {
	return S.viewSeq === null;
}

function inAnalysis() {
	return S.analysis !== null;
}

/** The seat the board accepts clicks for right now, or 0. */
function actingSeat() {
	if (inAnalysis()) {
		const sim = S.analysis.sim;
		return sim && !sim.game_ended && sim.turn_seat ? sim.turn_seat : 0;
	}
	return isLive() && isMyTurn() && !S.busy ? S.me.seat : 0;
}

function activeBuilder() {
	return inAnalysis() ? S.analysis.builder : S.builder;
}

function seatLabel(seat) {
	const d = seatDef(seat);
	return d ? d.label : `Seat ${seat}`;
}

function seatName(seat) {
	const si = seatInfo(seat);
	if (si && si.name) return si.name;
	return null;
}

function displayName(seat) {
	return seatName(seat) || "(open)";
}

function colorOf(seat) {
	const d = seatDef(seat);
	return d ? d.color : "white";
}

function openingActive() {
	const op = S.info.opening;
	return !!op && currentState().ply < op.plies && (isLive() || inAnalysis());
}

function placementColor() {
	// During an "opponent" opening the stone placed takes the opposing color.
	const seat = actingSeat() || S.me.seat;
	if (openingActive() && S.info.opening.owner === "opponent") {
		const other = S.info.seats.find((s) => s.seat !== seat);
		if (other) return other.color;
	}
	return seat ? colorOf(seat) : "white";
}

function currentCells() {
	if (inAnalysis() && S.analysis.sim) return S.analysis.sim.state.cells;
	if (!isLive()) {
		const v = S.viewCache.get(S.viewSeq);
		return v ? v.cells : {};
	}
	if (S.preview) return S.preview.cells;
	return S.state ? S.state.cells : {};
}

function currentState() {
	if (inAnalysis() && S.analysis.sim) return S.analysis.sim.state;
	if (!isLive()) return S.viewCache.get(S.viewSeq) || { cells: {}, reserves: {}, ply: 0 };
	return S.state || { cells: {}, reserves: {}, ply: 0 };
}

function lastMoveRecord() {
	const seq = isLive() ? S.moves.length : S.viewSeq;
	return seq > 0 ? S.moves[seq - 1] || null : null;
}

function lastMoveSquares() {
	if (inAnalysis()) return [];
	const m = lastMoveRecord();
	if (!m) return [];
	const out = [];
	if (m.from) out.push(m.from);
	if (m.path && m.path.length) out.push(...m.path);
	else if (m.to) out.push(m.to);
	if (m.aim) out.push(m.aim);
	return out;
}

function lastMoveAgeText() {
	const m = S.moves[S.moves.length - 1];
	return m && m.created_at ? ago(m.created_at) : "";
}

function scores() {
	const st = currentState();
	return (S.info.scores || []).map((sc) => {
		const per = {};
		for (const sd of S.info.seats) per[sd.seat] = 0;
		for (const stack of Object.values(st.cells || {})) {
			if (!stack.length) continue;
			const top = stack[stack.length - 1];
			const def = S.info.pieces.find((p) => p.type === top.piece);
			const matches = (!sc.piece || top.piece === sc.piece) && (!sc.tag || (def && (def.tags || []).includes(sc.tag)));
			if (!matches) continue;
			const seat = S.info.seats.find((s) => s.color === top.color);
			if (seat) per[seat.seat]++;
		}
		return { label: sc.label, per };
	});
}

// ---------------------------------------------------------------------------
// Ruleset games (Courtfall): their rules are written in Go and a turn is a
// sequence of small decisions. The server derives a view of every position —
// terrain tints, unit badges, scoreboards, the pending decision and the dice
// on the table — and sends it as state.ext.view.
// ---------------------------------------------------------------------------

function isRuleset() {
	return !!S.info && S.info.engine === "ruleset";
}

function rulesetView() {
	if (!isRuleset()) return null;
	const st = currentState();
	return (st && st.ext && st.ext.view) || null;
}

function rulesetEvents() {
	if (!isRuleset()) return [];
	const st = currentState();
	return (st && st.ext && st.ext.events) || [];
}

/** Moves of ruleset games are small decisions: they go in without a preview. */
function needsConfirm() {
	return S.settings.confirm && !isRuleset();
}

function actionDef(id) {
	return (S.info.actions || []).find((a) => a.id === id) || null;
}

/** What a count selection means, from the action chosen just before it. */
function countLabel(b) {
	const labels = S.info.count_labels || {};
	const last = [...b.steps].reverse().find((st) => st.kind === "action");
	if (last && labels[last.value]) return labels[last.value];
	return isRuleset() ? labels[""] || "Choose:" : "Pick up:";
}

/** Moves whose outcome depends on dice are not previewed. */
function previewable(notation) {
	return !isRuleset() || !/^(respond|reroll|keep)|[*^]/.test(notation);
}

// ---------------------------------------------------------------------------
// Actions
// ---------------------------------------------------------------------------

function afterBuilderChange() {
	MoveBuilder.manual = (rulesetView() || {}).panel_actions || [];
	const b = activeBuilder();
	b.autoAdvance();
	S.preview = null;
	const done = b.complete();
	if (inAnalysis()) {
		if (done && b.finished()) {
			analysisPlay(done.notation);
			return;
		}
		render();
		return;
	}
	if (done && b.finished() && !needsConfirm()) {
		submitMove();
		return;
	}
	render();
	if (done && needsConfirm() && previewable(done.notation)) requestPreview(done.notation);
}

async function requestPreview(notation) {
	const token = ++S.previewToken;
	try {
		const sim = await api.simulate(code, [notation]);
		if (token !== S.previewToken) return;
		const complete = S.builder.complete();
		if (!complete || complete.notation !== notation) return;
		S.preview = { notation, cells: sim.state.cells };
		render();
	} catch (_) {
		/* the preview is a nicety; submitting still works */
	}
}

function onCell(pos, ev) {
	if (!actingSeat()) return;
	const b = activeBuilder();
	if (!b.nextPositions().has(pos)) {
		// Quick placement: an empty square clicked with nothing selected
		// places a piece straight away. No modifier uses the piece used last
		// (else the game's first), Shift the second kind, Ctrl/Alt the third.
		const kinds = isRuleset() ? null : b.quickPlacements().get(pos);
		if (!kinds || !kinds.length) return;
		let pick;
		if (ev && ev.shiftKey && kinds.length > 1) pick = kinds[1];
		else if (ev && (ev.ctrlKey || ev.altKey || ev.metaKey) && kinds.length > 2) pick = kinds[2];
		else if (S.lastPiece && kinds.includes(S.lastPiece)) pick = S.lastPiece;
		else pick = kinds[0];
		S.lastPiece = pick;
		b.push("piece", pick);
		b.push("pos", pos);
		afterBuilderChange();
		return;
	}
	b.push("pos", pos);
	afterBuilderChange();
}

function choosePiece(piece) {
	if (!actingSeat()) return;
	const b = activeBuilder();
	if (!b.nextPieces().has(piece)) return;
	S.lastPiece = piece;
	b.push("piece", piece);
	afterBuilderChange();
}

function chooseAction(id) {
	if (!actingSeat()) return;
	const b = activeBuilder();
	if (!b.nextActions().includes(id)) return;
	b.push("action", id);
	afterBuilderChange();
}

function chooseCount(n) {
	if (!actingSeat()) return;
	activeBuilder().push("count", n);
	afterBuilderChange();
}

// ---- tokens on area maps ------------------------------------------------------
// Units are drawn as tokens; clicking one stands for the piece (and area)
// steps of the move being built: the origin of a move, the unit that retreats
// or takes a result, the one that is sacrificed.

const TOKEN_ACTIONS = new Set(["absorb", "invisible", "lose"]);
// Actions that name only the area of the unit they work on: clicking that
// kind of unit (when nothing else claims the click) chooses the action there.
const TOKEN_AREA_ACTIONS = { devolve: "cultist" };

function trialBuilder(b, extra) {
	const t = new MoveBuilder(b.moves);
	t.steps = [...b.steps, ...extra];
	return t;
}

/** The steps a click on a token of `piece` in area `pos` stands for, or null. */
function tokenSteps(b, pos, piece) {
	const queue = [[]];
	for (let guard = 0; queue.length && guard < 40; guard++) {
		const path = queue.shift();
		const hasPos = path.some((st) => st.kind === "pos");
		const opts = trialBuilder(b, path).options();
		for (const o of opts) {
			if (o.kind === "piece" && o.value === piece) {
				const p = [...path, { kind: "piece", value: piece }];
				if (!hasPos && trialBuilder(b, p).options().some((x) => x.kind === "pos" && x.value === pos)) {
					p.push({ kind: "pos", value: pos });
				}
				return p;
			}
			if (o.kind === "pos" && o.value === pos && !hasPos) queue.push([...path, { kind: "pos", value: pos }]);
			if (o.kind === "action" && TOKEN_ACTIONS.has(o.value) && !path.some((st) => st.kind === "action")) {
				queue.push([...path, { kind: "action", value: o.value }]);
			}
		}
	}
	for (const o of trialBuilder(b, []).options()) {
		if (o.kind !== "action" || TOKEN_AREA_ACTIONS[o.value] !== piece) continue;
		const path = [{ kind: "action", value: o.value }];
		if (trialBuilder(b, path).options().some((x) => x.kind === "pos" && x.value === pos)) return [...path, { kind: "pos", value: pos }];
	}
	return null;
}

function tokenOk(pos, piece, color) {
	const acting = actingSeat();
	if (!acting || inAnalysis()) return false;
	const mine = colorOf(acting);
	if (color !== mine) {
		// Another faction's unit may only be chosen when the rules say so
		// (Hastur's Vengeance); never when we have the same kind here.
		const stack = currentCells()[pos] || [];
		if (stack.some((pc) => pc.piece === piece && pc.color === mine)) return false;
	}
	return tokenSteps(activeBuilder(), pos, piece) !== null;
}

/** The unit type of a piece value: the unit on a Gate is "gate:<type>". */
function plainPiece(piece) {
	return String(piece).startsWith(ONGATE) ? String(piece).slice(ONGATE.length) : piece;
}

/** A piece value's name and icon shape ("Acolyte on the Gate" for the unit on a Gate). */
function pieceLook(piece) {
	const onGate = String(piece).startsWith(ONGATE);
	const def = S.info.pieces.find((p) => p.type === plainPiece(piece));
	const label = def ? def.label : plainPiece(piece);
	return { label: onGate ? `${label} on the Gate` : label, shape: def ? def.shape : "disc", color: def ? def.color : "", onGate };
}

function onToken(pos, piece, color) {
	// The unit on a Gate is its own token, and shows the card of its kind.
	const kind = plainPiece(piece);
	const onGate = kind !== piece;
	const same = !!S.unitInfo && S.unitInfo.pos === pos && S.unitInfo.piece === kind && S.unitInfo.color === color && !!S.unitInfo.onGate === onGate;
	if (tokenOk(pos, piece, color)) {
		// A unit that is a legal choice is picked up — and its card opens too.
		S.unitInfo = { pos, piece: kind, color, onGate };
		const b = activeBuilder();
		for (const st of tokenSteps(b, pos, piece)) b.push(st.kind, st.value);
		afterBuilderChange();
		return;
	}
	// Any other unit just shows its card (a second click closes it).
	S.unitInfo = same ? null : { pos, piece: kind, color, onGate };
	render();
}

function closeUnitInfo() {
	if (!S.unitInfo) return;
	S.unitInfo = null;
	render();
}

function resetMove() {
	const b = activeBuilder();
	b.reset();
	b.autoAdvance();
	S.preview = null;
	S.previewToken++;
	render();
}

function undoStep() {
	const b = activeBuilder();
	if (!b.manualSteps().length) return;
	b.pop();
	// Step back past any forced choices as well.
	while (b.steps.length > 0 && b.options().length === 1 && b.options()[0].kind !== "pos") {
		b.pop();
	}
	b.autoAdvance();
	S.preview = null;
	S.previewToken++;
	render();
}

// ---------------------------------------------------------------------------
// Analysis: explore variations from any position without touching the game.
// The server's simulate endpoint applies the variation and hands back the
// legal moves of the resulting position, so analysis uses the same
// click-by-click move builder as real play.
// ---------------------------------------------------------------------------

async function startAnalysis() {
	S.analysis = { base: isLive() ? null : S.viewSeq, moves: [], sim: null, builder: new MoveBuilder([]) };
	S.preview = null;
	await runAnalysis();
}

async function runAnalysis() {
	const a = S.analysis;
	if (!a) return;
	try {
		const sim = await api.simulate(code, a.moves, a.base === null ? undefined : a.base);
		if (S.analysis !== a) return;
		a.sim = sim;
		a.builder = new MoveBuilder(sim.legal_moves || []);
		a.builder.autoAdvance();
	} catch (err) {
		toast(a.moves.length ? err.message : "There is nothing to analyze from this position.", "error");
		if (a.moves.length) {
			a.moves.pop();
			return runAnalysis();
		}
		S.analysis = null;
	}
	render();
}

async function analysisPlay(notation) {
	if (!S.analysis) return;
	S.analysis.moves.push(notation);
	await runAnalysis();
}

async function analysisBack() {
	if (!S.analysis || !S.analysis.moves.length) return;
	S.analysis.moves.pop();
	await runAnalysis();
}

function exitAnalysis() {
	S.analysis = null;
	render();
}

// parseMoveText pulls move notation out of pasted text: it drops move
// numbers ("12."), results (1-0, R-0, F-0, 1/2-1/2), {comments}, quoted PTN
// headers ([Size "5"]) and trailing annotations (!, ?, ').
function parseMoveText(text) {
	const cleaned = String(text || "")
		.replace(/\{[^}]*\}/g, " ")
		.replace(/^\s*\[[^\]]*\]\s*$/gm, " ")
		.replace(/\([^)]*\)/g, " ");
	const out = [];
	for (const raw of cleaned.split(/[\s,]+/)) {
		let tok = raw.trim();
		if (!tok) continue;
		tok = tok.replace(/^\d+\.+/, ""); // "12.c3" -> "c3"
		if (!tok || /^\d+\.?$/.test(tok)) continue; // bare move numbers
		if (/^(1-0|0-1|1\/2-1\/2|[RF0]-[RF0]|\*)$/.test(tok)) continue; // results
		tok = tok.replace(/[!?']+$/, "");
		if (tok) out.push(tok);
	}
	return out;
}

// importMoves loads a pasted move list into the analysis board, keeping the
// longest legal prefix when a move does not fit.
async function importMoves(text) {
	const a = S.analysis;
	if (!a) return { applied: 0, total: 0 };
	const tokens = parseMoveText(text);
	if (!tokens.length) return { applied: 0, total: 0 };
	const ok = async (n) => {
		try {
			await api.simulate(code, tokens.slice(0, n), a.base === null ? undefined : a.base);
			return true;
		} catch (_) {
			return false;
		}
	};
	// Prefix validity is monotonic, so binary-search the longest valid one.
	let lo = 0;
	let hi = tokens.length;
	if (await ok(hi)) lo = hi;
	else {
		hi -= 1;
		while (lo < hi) {
			const mid = Math.ceil((lo + hi) / 2);
			if (await ok(mid)) lo = mid;
			else hi = mid - 1;
		}
	}
	a.moves = tokens.slice(0, lo);
	await runAnalysis();
	return { applied: lo, total: tokens.length };
}

function importDialog() {
	const dlg = h("dialog", { id: "import-dialog" });
	const area = h("textarea", { id: "import-text", rows: "6", placeholder: "1. a5 e1  2. c3 c4 …", spellcheck: "false" });
	const msg = h("p", { class: "small muted", id: "import-msg" });
	const load = h(
		"button",
		{
			class: "primary",
			id: "import-load",
			onclick: async () => {
				load.disabled = true;
				try {
					const r = await importMoves(area.value);
					if (r.total === 0) msg.textContent = "No moves found in that text.";
					else if (r.applied < r.total) msg.textContent = `Loaded ${r.applied} of ${r.total} moves — move ${r.applied + 1} is not legal from here.`;
					else {
						dlg.close();
						toast(`Loaded ${r.applied} moves.`);
					}
				} finally {
					load.disabled = false;
				}
			},
		},
		"Load"
	);
	dlg.append(
		h("h2", {}, "Import moves"),
		h("p", { class: "muted" }, "Paste a move list (PTN, or one move per line). Move numbers, results and comments are ignored. The moves are played out from the analysis position."),
		area,
		msg,
		h("div", { class: "row" }, load, h("button", { onclick: () => dlg.close() }, "Cancel"))
	);
	dlg.addEventListener("close", () => dlg.remove());
	return dlg;
}

function showImport() {
	const old = document.getElementById("import-dialog");
	if (old) old.remove();
	const dlg = importDialog();
	document.body.append(dlg);
	if (typeof dlg.showModal === "function") dlg.showModal();
	else dlg.setAttribute("open", "");
	const area = dlg.querySelector("textarea");
	if (area) area.focus();
}

// playFromHere starts a new game from the analysis position: against a bot
// or a friend, from either seat. The new game remembers its start position,
// so undo, history and export all begin there.
async function playFromHere() {
	const a = S.analysis;
	if (!a || !a.sim || a.sim.game_ended) return;
	let bots = [];
	try {
		bots = await api.bots();
	} catch (_) {
		bots = [];
	}
	const dlg = h("dialog", { id: "from-here-dialog" });
	dlg.append(h("h2", {}, "Play from this position"));
	dlg.append(h("p", { class: "muted" }, `${seatLabel(a.sim.turn_seat)} moves first from here.`));

	const opp = h("select", { id: "fh-opponent" }, h("option", { value: "" }, "A friend (share the link)"));
	for (const b of bots) opp.append(h("option", { value: b.id }, b.label || b.id));
	if (bots.length) opp.value = bots.find((b) => b.id === "tactical") ? "tactical" : bots[0].id;

	const seatSel = h("select", { id: "fh-seat" });
	for (const sd of S.info.seats) seatSel.append(h("option", { value: String(sd.seat) }, sd.label));
	seatSel.value = String(a.sim.turn_seat);

	const nameInput = h("input", { id: "fh-name", type: "text", maxlength: "40", value: S.me.name || lastName() || "" });
	const err = h("p", { class: "error small", id: "fh-error" });
	const start = h(
		"button",
		{
			class: "primary",
			id: "fh-start",
			onclick: async () => {
				const name = nameInput.value.trim();
				if (!name) {
					err.textContent = "Enter a name first.";
					return;
				}
				start.disabled = true;
				try {
					const res = await api.createGame({
						game_def_id: S.game.game_def_id,
						player_name: name,
						opponent: opp.value || undefined,
						seat: Number(seatSel.value),
						start_state: a.sim.state,
						start_seat: a.sim.turn_seat,
					});
					rememberGame(res.room_code, S.game.game_def_id, name);
					navigate("/game/" + res.room_code);
				} catch (e) {
					start.disabled = false;
					err.textContent = e.message;
				}
			},
		},
		"Start game"
	);
	dlg.append(
		h("label", {}, "Opponent", opp),
		h("label", {}, "You play", seatSel),
		h("label", {}, "Your name", nameInput),
		err,
		h("div", { class: "row" }, start, h("button", { onclick: () => dlg.close() }, "Cancel"))
	);
	dlg.addEventListener("close", () => dlg.remove());
	document.body.append(dlg);
	if (typeof dlg.showModal === "function") dlg.showModal();
	else dlg.setAttribute("open", "");
}

async function submitMove() {
	const mv = S.builder.complete();
	if (!mv || S.busy) return;
	if (S.tut && !isExpected(tutorialState().next, mv.notation)) {
		S.tut.hint = tutorialState().step;
		S.builder.reset();
		render();
		return;
	}
	S.busy = true;
	render();
	try {
		await api.move(code, S.me.name, mv.notation);
		await settle();
	} catch (err) {
		toast(err.message || "That move was rejected.", "error");
		S.builder.reset();
		S.builder.autoAdvance();
	} finally {
		S.busy = false;
		S.preview = null;
		render();
	}
}

// settle re-reads everything after our own action; WebSocket events for the
// same change are harmless because they are applied idempotently.
async function settle() {
	await refreshGame();
	notifyIfMyTurn(false);
}

async function doUndo() {
	if (!S.confirmUndo) {
		S.confirmUndo = true;
		clearTimeout(confirmTimer);
		confirmTimer = setTimeout(() => {
			S.confirmUndo = false;
			render();
		}, 4000);
		render();
		return;
	}
	S.confirmUndo = false;
	S.busy = true;
	render();
	try {
		await api.undo(code, S.me.name);
		S.viewSeq = null;
		await settle();
		toast("Move taken back.");
	} catch (err) {
		toast(err.message, "error");
	} finally {
		S.busy = false;
		render();
	}
}

async function doResign() {
	if (!S.confirmResign) {
		S.confirmResign = true;
		clearTimeout(confirmTimer);
		confirmTimer = setTimeout(() => {
			S.confirmResign = false;
			render();
		}, 4000);
		render();
		return;
	}
	S.confirmResign = false;
	S.busy = true;
	render();
	try {
		await api.resign(code, S.me.name);
		await settle();
	} catch (err) {
		toast(err.message, "error");
	} finally {
		S.busy = false;
		render();
	}
}

async function doDraw(action) {
	if (S.busy) return;
	S.busy = true;
	render();
	try {
		await api.draw(code, S.me.name, action);
		await settle();
		if (action === "accept") toast("Draw agreed.");
	} catch (err) {
		toast(err.message, "error");
	} finally {
		S.busy = false;
		render();
	}
}

function drawOfferBanner() {
	const g = S.game;
	if (g.status !== "active" || g.draw_offer_seat === null || g.draw_offer_seat === undefined) return null;
	const offerer = g.draw_offer_seat;
	const who = seatName(offerer) || seatLabel(offerer);
	if (!S.me.seat) return h("div", { class: "notice", id: "draw-banner" }, `${who} has offered a draw.`);
	if (offerer === S.me.seat) {
		return h(
			"div",
			{ class: "notice", id: "draw-banner" },
			h("span", {}, "You offered a draw — waiting for your opponent."),
			h("button", { id: "draw-cancel", class: "ghost", onclick: () => doDraw("cancel") }, "Withdraw")
		);
	}
	return h(
		"div",
		{ class: "notice attention", id: "draw-banner" },
		h("span", {}, `${who} offers a draw.`),
		h("span", { class: "move-actions" },
			h("button", { id: "draw-accept", class: "primary", onclick: () => doDraw("accept") }, "Accept"),
			h("button", { id: "draw-decline", onclick: () => doDraw("decline") }, "Decline")
		)
	);
}

// positionKey identifies the position a hint was asked for, so a stale hint
// disappears as soon as the position on screen changes.
function positionKey() {
	const a = S.analysis;
	return [S.moves.length, S.viewSeq, a ? `${a.base}:${a.moves.join(",")}` : ""].join("|");
}

// canHint: hints are only offered where they cannot be used against a person
// (see the server's rule): every other seat is a bot, or the game is over.
function canHint() {
	if (!S.me.seat || !S.me.name) return false;
	if (S.game.status === "waiting") return false;
	if (S.game.status === "finished") return !isLive() || inAnalysis();
	return (S.game.seats || []).every((x) => x.seat === S.me.seat || x.bot);
}

async function askHint() {
	if (S.hintBusy || !canHint()) return;
	S.hintBusy = true;
	render();
	try {
		let atSeq;
		let moves = [];
		if (inAnalysis()) {
			atSeq = S.analysis.base === null ? undefined : S.analysis.base;
			moves = S.analysis.moves;
		} else if (!isLive()) {
			atSeq = S.viewSeq;
		}
		const h = await api.hint(code, S.me.name, atSeq, moves);
		S.hint = {
			notation: h.notation,
			steps: h.steps || [],
			bot: h.bot,
			winRate: typeof h.win_rate === "number" ? h.win_rate : null,
			alternatives: h.alternatives || [],
			seat: h.turn_seat,
			key: positionKey(),
		};
	} catch (err) {
		toast(err.message, "error");
	} finally {
		S.hintBusy = false;
		render();
	}
}

function hintBanner() {
	const hint = S.hint;
	if (!hint || hint.key !== positionKey()) return null;
	const useIt = () => {
		const b = activeBuilder();
		if (!actingSeat()) {
			S.hint = null;
			render();
			return;
		}
		b.reset();
		for (const st of hint.steps) b.push(st.kind, st.value);
		S.hint = null;
		afterBuilderChange();
	};
	const label = { mcts: "MCTS", minimax: "Minimax", tactical: "Tactical", random: "Random" }[hint.bot] || hint.bot;
	return h(
		"div",
		{ class: "notice", id: "hint-banner" },
		h(
			"span",
			{},
			`Suggested move (${label}): `,
			h("strong", { id: "hint-move" }, hint.notation),
			hint.winRate === null ? null : h("span", { class: "muted", id: "hint-eval" }, ` — about ${Math.round(hint.winRate * 100)}% to win for ${seatLabel(hint.seat)}`),
			hint.alternatives.length > 1
				? h(
						"span",
						{ class: "small muted", id: "hint-alternatives" },
						" · also: " + hint.alternatives.slice(1).map((a) => `${a.notation} (${Math.round(a.win_rate * 100)}%)`).join(", ")
				  )
				: null
		),
		h(
			"span",
			{ class: "move-actions" },
			actingSeat() ? h("button", { id: "hint-use", class: "primary", onclick: useIt }, "Use it") : null,
			h("button", { id: "hint-dismiss", class: "ghost", onclick: () => { S.hint = null; render(); } }, "Dismiss")
		)
	);
}

function hintSquares() {
	const h = S.hint;
	if (!h || h.key !== positionKey()) return [];
	return h.steps.filter((st) => st.kind === "pos").map((st) => st.value);
}

async function doTakeback(action) {
	if (S.busy) return;
	S.busy = true;
	render();
	try {
		await api.takeback(code, S.me.name, action);
		if (action === "accept") S.viewSeq = null;
		await settle();
		if (action === "accept") toast("Move taken back.");
	} catch (err) {
		toast(err.message, "error");
	} finally {
		S.busy = false;
		render();
	}
}

// takebackBanner shows an open take-back request: the requester can
// withdraw it, the opponent can accept (rewinding the game) or decline.
function takebackBanner() {
	const g = S.game;
	if (g.status !== "active" || g.takeback_seat === null || g.takeback_seat === undefined) return null;
	const asker = g.takeback_seat;
	const who = seatName(asker) || seatLabel(asker);
	const rec = S.moves[(g.takeback_to_seq || 0)];
	const what = rec ? `move ${(g.takeback_to_seq || 0) + 1} (${rec.notation})` : "their last move";
	if (!S.me.seat) return h("div", { class: "notice", id: "takeback-banner" }, `${who} asked to take back ${what}.`);
	if (asker === S.me.seat) {
		return h(
			"div",
			{ class: "notice", id: "takeback-banner" },
			h("span", {}, "You asked to take back your last move — waiting for your opponent."),
			h("button", { id: "takeback-cancel", class: "ghost", onclick: () => doTakeback("cancel") }, "Withdraw")
		);
	}
	return h(
		"div",
		{ class: "notice attention", id: "takeback-banner" },
		h("span", {}, `${who} asks to take back ${what}.`),
		h(
			"span",
			{ class: "move-actions" },
			h("button", { id: "takeback-accept", class: "primary", onclick: () => doTakeback("accept") }, "Allow"),
			h("button", { id: "takeback-decline", onclick: () => doTakeback("decline") }, "Refuse")
		)
	);
}

// canAskTakeback: you moved, a human replied since (so undo alone is not
// possible), the opponent is human and no request is open.
function canAskTakeback() {
	if (!S.me.seat || !isActive() || S.moves.length === 0) return false;
	if (S.game.takeback_seat !== null && S.game.takeback_seat !== undefined) return false;
	if ((S.game.seats || []).some((x) => x.bot)) return false;
	if (canUndo()) return false;
	return S.moves.some((m) => m.seat === S.me.seat);
}

function canUndo() {
	if (!S.me.seat || S.moves.length === 0) return false;
	if (S.game.status === "finished" && S.game.end_reason === "resignation") return false;
	const botSeats = new Set((S.game.seats || []).filter((s) => s.bot).map((s) => s.seat));
	let idx = -1;
	for (let i = S.moves.length - 1; i >= 0; i--) {
		if (S.moves[i].seat === S.me.seat) {
			idx = i;
			break;
		}
	}
	if (idx < 0) return false;
	// Dice rolled since (by this move or any after it) lock the game in: the
	// server refuses, so the page does not offer it.
	if (S.moves.slice(idx).some((m) => m.locked)) return false;
	return S.moves.slice(idx + 1).every((m) => botSeats.has(m.seat) || m.auto);
}

// ---------------------------------------------------------------------------
// History navigation
// ---------------------------------------------------------------------------

function setHash(seq) {
	try {
		const url = seq === null ? location.pathname + location.search : `${location.pathname}${location.search}#move=${seq}`;
		history.replaceState(null, "", url);
	} catch (_) {
		/* sharing a position is a convenience only */
	}
}

function stopAutoplay() {
	if (S.autoplay) {
		clearInterval(S.autoplay);
		S.autoplay = null;
	}
}

function toggleAutoplay() {
	if (S.autoplay) {
		stopAutoplay();
		render();
		return;
	}
	if (isLive()) viewMove(0);
	S.autoplay = setInterval(() => {
		const cur = isLive() ? S.moves.length : S.viewSeq;
		if (cur >= S.moves.length) {
			stopAutoplay();
			render();
			return;
		}
		viewMove(cur + 1 >= S.moves.length ? null : cur + 1);
	}, 900);
	render();
}

async function viewMove(seq) {
	const total = S.moves.length;
	if (seq === null || seq >= total) {
		S.viewSeq = null;
		setHash(null);
		render();
		return;
	}
	seq = Math.max(0, seq);
	if (!S.viewCache.has(seq)) {
		try {
			const st = await api.state(code, seq, S.me.name);
			S.viewCache.set(seq, st.state);
		} catch (err) {
			toast(err.message, "error");
			return;
		}
	}
	S.viewSeq = seq;
	S.preview = null;
	setHash(seq);
	render();
}

function stepHistory(delta) {
	stopAutoplay();
	const total = S.moves.length;
	const cur = isLive() ? total : S.viewSeq;
	const next = Math.min(total, Math.max(0, cur + delta));
	viewMove(next >= total ? null : next);
}

// ---------------------------------------------------------------------------
// Live updates
// ---------------------------------------------------------------------------

function connect() {
	if (wsStopped) return;
	const proto = location.protocol === "https:" ? "wss://" : "ws://";
	const asPlayer = S.me.name && S.me.seat;
	const name = asPlayer ? S.me.name : "spectator-" + Math.random().toString(36).slice(2, 8);
	const role = asPlayer ? "player" : "spectator";
	const sock = new WebSocket(`${proto}${location.host}/ws/${encodeURIComponent(code)}?name=${encodeURIComponent(name)}&role=${role}`);
	ws = sock;
	sock.addEventListener("open", () => {
		if (ws !== sock) return;
		S.conn = "online";
		wsRetry = 0;
		updateConn();
		loadPresence();
		voiceResync(code);
	});
	sock.addEventListener("message", (ev) => {
		if (ws !== sock) return;
		let env;
		try {
			env = JSON.parse(ev.data);
		} catch (_) {
			return;
		}
		onEvent(env);
	});
	sock.addEventListener("close", () => {
		// A socket replaced by restartSocket must not trigger a reconnect.
		if (ws !== sock) return;
		S.conn = "offline";
		updateConn();
		if (wsStopped) return;
		wsRetry = Math.min(wsRetry + 1, 6);
		setTimeout(async () => {
			try {
				await refreshGame();
				render();
			} catch (_) {
				/* try again on the next reconnect */
			}
			connect();
		}, 700 * 2 ** wsRetry);
	});
}

function updateConn() {
	const el = document.getElementById("conn-badge");
	if (el) {
		el.textContent = S.conn === "online" ? "live" : S.conn === "offline" ? "reconnecting…" : "connecting…";
		el.className = "badge " + (S.conn === "online" ? "" : "waiting");
	}
}

async function onEvent(env) {
	if (!env || !S.info) return;
	// The voice chat's own events (who is in the call, and the messages for our seat).
	if (voiceEvent(env, code)) return;
	if (env.type === "move") {
		const d = env.data || {};
		if (d.seq === S.moves.length + 1 && S.state) {
			S.moves.push({
				seq: d.seq,
				seat: d.seat,
				color: d.color,
				from: d.from,
				to: d.to,
				captured: d.captured,
				action: d.action,
				piece: d.piece,
				count: d.count,
				path: d.path,
				drops: d.drops,
				aim: d.aim,
				notation: d.notation,
				auto: !!d.auto,
				auto_play: !!d.auto_play,
				locked: !!d.locked,
				created_at: new Date().toISOString(),
			});
			S.state = d.state;
			// In a game against oneself the seat that acts changes with the turn.
			S.game.turn_seat = d.turn_seat;
			S.game.status = d.status;
			syncMe();
			// The broadcast position is the public one: a player's own
			// secrets (Elder Sign values) come from their own view.
			if (isRuleset() && S.me.seat && S.me.name) {
				try {
					const own = await api.state(code, null, S.me.name);
					if (own && own.state && own.move_seq === d.seq) S.state = own.state;
				} catch (_) {
					/* the public position is still right; only private details are missing */
				}
			}
			S.game.status = d.status;
			S.game.turn_seat = d.turn_seat;
			S.game.move_seq = d.seq;
			S.game.winner_seat = d.winner_seat;
			S.game.end_reason = d.end_reason;
			S.game.end_detail = d.end_detail;
			S.game.turn_deadline = d.turn_deadline || "";
			S.game.clock = d.clock || null;
			S.game.draw_offer_seat = null; // any move withdraws an offer
			S.game.takeback_seat = null; // ... and a take-back request
			noteServerTime(d.server_time);
			S.viewCache.clear();
			await Promise.all([loadLegal(), loadReactions(), loadAutoPlay()]);
			if (!d.game_ended && d.turn_seat === d.seat && S.info.seats.length > 1 && !isRuleset()) {
				// The next seat had no legal move and was skipped. (A ruleset's
				// turn is a run of decisions by the same seat: nothing to say.)
				const other = S.info.seats.find((x) => x.seat !== d.seat);
				if (other) toast(`${seatName(other.seat) || seatLabel(other.seat)} has no legal move and passes.`);
			}
		} else if (d.seq > S.moves.length) {
			await refreshGame();
		} else {
			return; // already applied (our own move's echo)
		}
		notifyIfMyTurn(true);
			render();
		} else if (env.type === "presence") {
			const d = env.data || {};
			if (d.present) S.present.add(d.seat);
			else S.present.delete(d.seat);
			if (d.seat === S.me.seat && !d.present && presenceBeat) keepPresence(false);
			render();
		} else if (env.type === "chat") {
			const line = env.data || {};
			receiveChat(line, { mine: !!S.me.name && String(line.name || "").toLowerCase() === S.me.name.toLowerCase() });
			if (S.mPage === "chat") chatSeen();
			paintMobileBadges();
		} else if (env.type === "state") {
			if (env.data && env.data.reason === "draw-offer") toast("A draw was offered.");
		if (env.data && env.data.reason === "takeback-request") toast("Your opponent asked to take back a move.");
		if (env.data && env.data.reason === "takeback-declined") toast("Take-back declined.");
		await refreshGame();
		if (env.data && env.data.reason === "undo") S.viewSeq = null;
		notifyIfMyTurn(true);
		render();
	}
}

// ---------------------------------------------------------------------------
// Notifications
// ---------------------------------------------------------------------------

let lastNotifiedSeq = -1;

function notifyIfMyTurn(fromEvent) {
	if (!isMyTurn()) return;
	updateTitle();
	if (!fromEvent || lastNotifiedSeq === S.moves.length) return;
	lastNotifiedSeq = S.moves.length;
	if (document.hidden && S.settings.notify && "Notification" in window && Notification.permission === "granted") {
		try {
			new Notification("Your turn", { body: `${S.info.name} — room ${code}`, tag: "bingle-" + code });
		} catch (_) {
			/* notifications are best-effort */
		}
	}
}

function updateTitle() {
	document.title = (isMyTurn() ? "● Your turn — " : "") + titleBase + " · " + code;
	setTurnBadge(isMyTurn());
}

document.addEventListener("visibilitychange", updateTitle);

// ---------------------------------------------------------------------------
// Stack inspector: hovering (or focusing) a square spells out what is on it,
// which matters for stacks whose lower pieces are hidden.
// ---------------------------------------------------------------------------

function onHover(pos) {
	S.inspect = pos;
	updateInspector();
}

/** The display name of a position (areas are named; squares are not). */
function areaName(id) {
	const a = ((S.info.board && S.info.board.areas) || []).find((x) => x.id === id);
	return a ? a.name : id;
}

function inspectorText() {
	if (!S.inspect) return "";
	const stack = currentCells()[S.inspect] || [];
	const v = rulesetView();
	if (v) {
		const parts = [];
		if (v.notes && v.notes[S.inspect]) parts.push(v.notes[S.inspect]);
		const tint = v.tints && v.tints[S.inspect];
		const legend = tint && S.info.display && S.info.display.tints ? S.info.display.tints[tint] : null;
		if (legend && !tint.startsWith("zone") && S.info.board.style !== "areas") parts.push(`${legend.label}: ${legend.text || ""}`);
		return parts.length ? `${areaName(S.inspect)}: ${parts.join(" — ")}` : "";
	}
	if (!stack.length) return "";
	const label = (pc) => {
		const def = S.info.pieces.find((p) => p.type === pc.piece);
		return `${pc.color} ${def ? def.label.toLowerCase() : pc.piece}`;
	};
	if (stack.length === 1) return `${S.inspect}: ${label(stack[0])}`;
	return `${S.inspect}: ${stack.length} pieces — ` + stack.map(label).join(" › ") + " (top)";
}

function updateInspector() {
	const el = document.getElementById("inspector");
	if (el) el.textContent = inspectorText();
}

/** The board theme in force: the player's choice, unless this game does not offer it. */
function boardTheme() {
	const want = S.settings.board || "wood";
	if (themeById(want) && !themesFor(S.info).length) return "wood";
	return want;
}

function applyTheme() {
	document.documentElement.dataset.board = boardTheme();
	// Ambient motion of the themes (bubbles, neon, rays) follows the animation setting.
	document.documentElement.dataset.ambient = S.settings.animate !== false ? "on" : "off";
	document.documentElement.style.setProperty("--tile-scale", String(S.settings.tileScale || 1));
}

// ---------------------------------------------------------------------------
// Rendering
// ---------------------------------------------------------------------------

function render() {
	// Typing a chat message on a phone: the keyboard stays up. The page is
	// redrawn when the field is left.
	const typing = document.activeElement;
	if (document.body.classList.contains("m-mode") && typing && typing.id === "chat-input") {
		if (!S.mDeferred) typing.addEventListener("blur", () => { S.mDeferred = false; render(); }, { once: true });
		S.mDeferred = true;
		return;
	}
	rememberMobileScroll();
	if (S.tut) queueMicrotask(tutorialTick);
	clear(root);
	document.body.classList.remove("m-mode");
	hideHoverCard();
	hideCardZoom();
	updateTitle();
	applyTheme();
	initOptions();
	refreshOptions();

	if (S.retired) {
		root.append(retiredRecord());
		return;
	}
	if (S.fatal) {
		root.append(h("div", { class: "card", id: "board-error" }, h("h2", {}, "Can’t open this game"), h("p", {}, "Failed to load game state: " + S.fatal), h("a", { class: "button", href: "/" }, "Back to the lobby")));
		return;
	}
	if (!S.info || !S.game) {
		root.append(h("p", { class: "muted" }, "Loading…"));
		return;
	}

	const seats = S.info.seats;
	syncMe();
	const bottomSeat = S.homeSeat || S.me.seat || seats[0].seat;
	const many = seats.length > 2;
	const areas = S.info.board.style === "areas";
	document.body.classList.toggle("wide-game", areas);
	// Three or more cults: their panels live beside the map, so the scores,
	// spellbooks and Power are in view without scrolling.
	let cults = null;
	if (many) {
		cults = h("div", { class: "cult-stack", id: "cult-stack" });
		// Listed in table order from the viewer: clockwise runs down the list
		// (and from the last cult back to the first).
		for (const seat of seatsFrom(bottomSeat)) cults.append(seatStrip(seat));
		cults.append(
			h(
				"p",
				{ class: "small muted seat-order-note", id: "seat-order-note" },
				S.info.cards ? "Play runs clockwise down this list, from the last player back to the first." : "Play runs clockwise down this list, from the last cult back to the first; counter-clockwise runs up it."
			)
		);
	}

	// A game that has not started shows its lobby instead of an empty board.
	const lobby = inLobby();
	if (areas && !lobby && isMobileLayout()) {
		renderMobile(seats, bottomSeat);
		return;
	}
	if (S.info.cards && !lobby && isMobileLayout() && rulesetView()) {
		renderMobileCards(seats, bottomSeat);
		return;
	}
	const layout = h("div", { class: "game-layout" + (areas && !lobby ? " areas" : "") + (lobby ? " lobby-layout" : "") });
	const main = h("div", { class: "board-column" });
	// On a wide screen the move list and the log go under the map, where
	// there is room, so the side column holds just the cults and the table.
	const below = areas && many && window.matchMedia("(min-width: 1500px)").matches ? [] : null;
	layout.append(main, sidePanel(lobby ? null : cults, below));
	root.append(layout);

	if (lobby) {
		main.append(statusBar());
		main.append(lobbyCard(lobbyContext()));
		layout.querySelector(".side-panel").append(debugPanel());
		return;
	}
	main.append(statusBar());
	if (S.tut) main.append(tutorialCard());
	const banner = drawOfferBanner();
	if (banner) main.append(banner);
	const tbBanner = takebackBanner();
	if (tbBanner) main.append(tbBanner);
	const hintNotice = hintBanner();
	if (hintNotice) main.append(hintNotice);

	if (!S.me.seat && !S.spectating) {
		const gate = identityGate();
		if (gate) main.append(gate);
	}

	const wrap = h("div", { class: "board-wrap" + (areas ? " wide" : "") });
	const boardHost = h("div", { id: "board" });
	if (areas) {
		const b = S.info.board.canvas || { width: 1000, height: 490 };
		wrap.style.setProperty("--aspect", String((b.width + 16) / (b.height + 16)));
	}
	if (many) {
		const rb = roundBar();
		if (rb) wrap.append(rb);
		wrap.append(boardHost);
		wrap.append(moveBar());
		for (const panel of cardPanels()) wrap.append(panel);
		wrap.append(h("div", { class: "inspector small muted", id: "inspector", "aria-live": "off" }, inspectorText()));
		const unitCard = unitInfoCard();
		if (unitCard) wrap.append(unitCard);
		const bp = battlePanel();
		if (bp) wrap.append(bp);
	} else {
		const topSeats = seats.filter((s) => s.seat !== bottomSeat);
		for (const ts of topSeats) wrap.append(seatStrip(ts.seat));
		wrap.append(boardHost);
		wrap.append(seatStrip(bottomSeat));
		wrap.append(moveBar());
		const cards = cardsPanel();
		if (cards) wrap.append(cards);
		for (const panel of cardPanels()) wrap.append(panel);
		wrap.append(h("div", { class: "inspector small muted", id: "inspector", "aria-live": "off" }, inspectorText()));
	}
	const exchange = exchangePanel();
	if (exchange) wrap.append(exchange);
	main.append(wrap);
	if (below && below.length) main.append(h("div", { class: "below-map" }, ...below));

	drawBoard(boardHost);
	layout.querySelector(".side-panel").append(debugPanel());
}


// ---- the phone layout --------------------------------------------------------------------
//
// One full-screen view at a time under a slim top bar, swiped between like the
// pages of an app: the map, the actions, the round, each cult's card, the log
// and the chat. The same pieces as the desktop page, laid out differently.

/** Takes the ids off a copy of a piece of page (it appears twice, the ids must stay unique). */
function withoutIds(el) {
	if (!el) return el;
	el.removeAttribute("id");
	for (const n of el.querySelectorAll("[id]")) n.removeAttribute("id");
	return el;
}

/** Remembers how far each view was scrolled: the page is rebuilt on every move. */
function rememberMobileScroll() {
	const pages = document.querySelectorAll("#m-track .m-page");
	if (!pages.length) return;
	S.mScroll = S.mScroll || {};
	for (const p of pages) S.mScroll[p.dataset.page] = p.scrollTop;
}

/** The chat badge and the "your turn" mark of the bottom bar, kept current without a redraw. */
function paintMobileBadges() {
	const nav = document.getElementById("m-nav");
	if (!nav) return;
	const chatItem = nav.querySelector('[data-group="chat"]');
	if (chatItem) {
		const n = S.mPage === "chat" ? 0 : chatUnread();
		let badge = chatItem.querySelector(".m-nav-badge");
		if (n && !badge) {
			badge = h("span", { class: "m-nav-badge" });
			chatItem.append(badge);
		}
		if (badge) {
			badge.textContent = String(n);
			badge.hidden = n === 0;
		}
	}
}

function renderMobile(seats, bottomSeat) {
	document.body.classList.add("m-mode");
	const myTurn = isLive() && S.game.status === "active" && isMyTurn();

	// The map, with what is needed to act on it close below.
	const boardHost = h("div", { id: "board" });
	const mapPage = h("div", { class: "m-map-page" });
	mapPage.append(statusBar());
	if (S.tut) mapPage.append(tutorialCard());
	for (const banner of [drawOfferBanner(), takebackBanner(), hintBanner()]) if (banner) mapPage.append(banner);
	if (!S.me.seat && !S.spectating) {
		const gate = identityGate();
		if (gate) mapPage.append(gate);
	}
	mapPage.append(
		h(
			"div",
			{ class: "m-board-box" },
			boardHost,
			// Swipes that start at the edges turn the page: the map itself takes
			// every other touch (drag, pinch, tap).
			h("div", { class: "m-edge left", "aria-hidden": "true" }),
			h("div", { class: "m-edge right", "aria-hidden": "true" })
		),
		h("div", { class: "m-dock" }, moveBar())
	);

	// Everything about acting, in large controls.
	const actions = h("div", { class: "m-stack" }, h("h2", {}, "Actions"));
	actions.append(withoutIds(moveBar()));
	actions.append(h("div", { class: "inspector small muted", id: "inspector", "aria-live": "off" }, inspectorText()));
	for (const part of [unitInfoCard(), battlePanel(), exchangePanel()]) if (part) actions.append(part);

	const round = h("div", { class: "m-stack" }, h("h2", {}, "Round"));
	const rb = roundBar();
	if (rb) round.append(rb);
	round.append(h("p", { class: "small muted seat-order-note", id: "seat-order-note" }, "Play runs clockwise down the list of cults, from the last cult back to the first; counter-clockwise runs up it."));

	const neutral = neutralCard();
	if (neutral) round.append(neutral);

	const pages = [
		{ id: "map", label: "Map", group: "map", node: mapPage },
		{ id: "actions", label: "Actions", group: "actions", node: actions },
		{ id: "round", label: "Round", group: "round", node: round },
	];
	const chips = { cults: [] };
	for (const seat of seatsFrom(bottomSeat)) {
		const id = `cult-${seat}`;
		pages.push({ id, label: seatLabel(seat) || `Seat ${seat}`, group: "cults", node: h("div", { class: "m-stack" }, seatStrip(seat)) });
		chips.cults.push({ page: id, label: seatLabel(seat) || `Seat ${seat}`, color: cssColor(colorOf(seat)) });
	}
	pages.push({ id: "log", label: "History", group: "log", node: h("div", { class: "m-stack" }, historyCard()) });
	pages.push({ id: "chat", label: "Chat", group: "chat", node: h("div", { class: "m-stack" }, voiceCard({ code, me: S.me, seats: S.game.seats }), chatCard({ code, me: S.me })) });

	const unread = S.mPage === "chat" ? 0 : chatUnread();
	const shell = mobileShell({
		pages,
		chips,
		groups: [
			{ id: "map", label: "Map", icon: "🗺️" },
			{ id: "actions", label: "Actions", icon: "⚡", pulse: myTurn && S.mPage !== "actions" },
			{ id: "round", label: "Round", icon: "⏳" },
			{ id: "cults", label: "Cults", icon: "👥" },
			{ id: "log", label: "History", icon: "📜" },
			{ id: "chat", label: "Chat", icon: "💬", badge: unread || 0 },
		],
		current: S.mPage || "map",
		onChange: (id) => {
			S.mPage = id;
			if (id === "chat") chatSeen();
			paintMobileBadges();
		},
	});
	root.append(shell.node);
	shell.restore();
	if (S.mScroll) {
		for (const p of shell.node.querySelectorAll(".m-page")) {
			if (S.mScroll[p.dataset.page]) p.scrollTop = S.mScroll[p.dataset.page];
		}
	}
	if (S.mPage === "chat") chatSeen();
	drawBoard(boardHost);
	paintMobileBadges();
	if (!store.get("bingle.mobileHint", false)) {
		store.set("bingle.mobileHint", true);
		toast("Pinch the map to zoom and drag to pan. Swipe from the screen edge, or use the bar below, to change view.");
	}
	// A turn that has just come to the player buzzes the phone, once.
	if (myTurn && S.mBuzzed !== S.moves.length) {
		S.mBuzzed = S.moves.length;
		if (navigator.vibrate && document.visibilityState === "visible") navigator.vibrate(60);
	}
}

/**
 * The phone layout of a card game: the play (the trick and your hand), the board
 * of terms, the players, the moves and the chat, one view each.
 */
function renderMobileCards(seats, bottomSeat) {
	document.body.classList.add("m-mode");
	const myTurn = isLive() && S.game.status === "active" && isMyTurn();
	const v = rulesetView();
	const ctx = cardContext(v);

	const play = h("div", { class: "m-map-page m-cards-page" });
	play.append(statusBar());
	if (S.tut) play.append(tutorialCard());
	for (const banner of [drawOfferBanner(), takebackBanner(), hintBanner()]) if (banner) play.append(banner);
	if (!S.me.seat && !S.spectating) {
		const gate = identityGate();
		if (gate) play.append(gate);
	}
	play.append(moveBar());
	const table = cardTable(ctx);
	S.ctPrev = table.state;
	play.append(table.node);

	const pages = [{ id: "play", label: "Play", group: "play", node: play }];
	if (v.grid) {
		// The boxes are pictures: a legend says what each line is for (a phone has no hover text).
		const legend = h("dl", { class: "cg-legend small" });
		for (const line of v.grid.lines) {
			legend.append(h("dt", {}, choiceIcon(line.icon, ctx, 22), h("strong", {}, line.label)), h("dd", { class: "muted" }, line.hint));
		}
		const terms = h("div", { class: "m-stack" }, h("h2", {}, "Terms"), termsBoard(v, ctx), h("p", { class: "small muted" }, "The box left open on each line is the rule of the round."), legend);
		pages.push({ id: "terms", label: "Terms", group: "terms", node: terms });
	}
	const players = h("div", { class: "m-stack" }, h("h2", {}, "Players"));
	for (const seat of seatsFrom(bottomSeat)) players.append(seatStrip(seat));
	pages.push({ id: "players", label: "Players", group: "players", node: players });
	pages.push({ id: "log", label: "History", group: "log", node: h("div", { class: "m-stack" }, historyCard()) });
	pages.push({ id: "chat", label: "Chat", group: "chat", node: h("div", { class: "m-stack" }, voiceCard({ code, me: S.me, seats: S.game.seats }), chatCard({ code, me: S.me })) });

	const unread = S.mPage === "chat" ? 0 : chatUnread();
	const shell = mobileShell({
		pages,
		groups: [
			{ id: "play", label: "Play", icon: "🃏", pulse: myTurn && S.mPage !== "play" },
			...(v.grid ? [{ id: "terms", label: "Terms", icon: "📋" }] : []),
			{ id: "players", label: "Players", icon: "👥" },
			{ id: "log", label: "History", icon: "📜" },
			{ id: "chat", label: "Chat", icon: "💬", badge: unread || 0 },
		],
		current: S.mPage || "play",
		onChange: (id) => {
			S.mPage = id;
			if (id === "chat") chatSeen();
			paintMobileBadges();
		},
	});
	root.append(shell.node);
	shell.restore();
	if (S.mScroll) {
		for (const p of shell.node.querySelectorAll(".m-page")) {
			if (S.mScroll[p.dataset.page]) p.scrollTop = S.mScroll[p.dataset.page];
		}
	}
	if (S.mPage === "chat") chatSeen();
	paintMobileBadges();
	// A turn that has just come to the player buzzes the phone, once.
	if (myTurn && S.mBuzzed !== S.moves.length) {
		S.mBuzzed = S.moves.length;
		if (navigator.vibrate && document.visibilityState === "visible") navigator.vibrate(60);
	}
}

// ghostPiece describes the piece a click on an empty square would place (a
// translucent preview on hover), or null when clicks do not place pieces.
function ghostPiece() {
	if (!actingSeat()) return null;
	const b = activeBuilder();
	let piece = null;
	if (b.steps.length > 0 && b.steps[0].kind === "piece") {
		piece = b.steps[0].value;
	} else {
		const quick = b.quickPlacements();
		if (quick.size) {
			const kinds = quick.values().next().value || [];
			piece = S.lastPiece && kinds.includes(S.lastPiece) ? S.lastPiece : kinds[0];
		}
	}
	const def = piece && S.info.pieces.find((p) => p.type === piece);
	return def ? { shape: def.shape, color: placementColor() } : null;
}

/** The game waits for its players: its lobby is on screen, not the board. */
function inLobby() {
	return !!S.game && S.game.status === "waiting" && isLive() && !inAnalysis() && (S.game.lobby || S.info.seats.length > 2);
}

/**
 * The cults an open seat can be given in the lobby, from the setup options of
 * the game this one was made from (none for a game whose seats have no cult
 * to choose).
 */
async function loadCults() {
	S.cults = [];
	try {
		const base = String(S.info.id).split("~")[0];
		const info = base === S.info.id ? S.info : await api.gameDef(base);
		const choice = ((info && info.options) || []).find((o) => o.kind === "seat-choice");
		S.cults = choice ? choice.choices : [];
	} catch (_) {
		S.cults = [];
	}
	render();
}

/** What the lobby card draws from, and what its buttons do. */
function lobbyContext() {
	const done = async (fn) => {
		if (S.lobbyBusy) return;
		S.lobbyBusy = true;
		render();
		try {
			await fn();
		} catch (err) {
			toast(err.message || "That did not work.", "error");
		} finally {
			S.lobbyBusy = false;
			try {
				await refreshGame();
			} catch (_) {
				/* the next event brings the table up to date */
			}
			render();
		}
	};
	if (!S.bots) {
		api.bots()
			.then((list) => {
				S.bots = list;
				render();
			})
			.catch(() => {
				S.bots = [];
			});
	}
	if (!S.cults) loadCults();
	return {
		code,
		info: S.info,
		game: S.game,
		me: S.me,
		bots: S.bots || [],
		cults: S.cults || [],
		inviteURL: S.game.invite_url || location.origin + "/game/" + code,
		lastName: lastName(),
		busy: S.lobbyBusy,
		actions: {
			copy: async (field) => {
				const link = field.value;
				try {
					await navigator.clipboard.writeText(link);
					toast("Link copied");
				} catch (_) {
					field.select();
					document.execCommand && document.execCommand("copy");
					toast("Link selected — copy it");
				}
			},
			take: (seat, name) =>
				done(async () => {
					await api.join(code, name, "player", seat);
					rememberGame(code, S.game.game_def_id, name);
					resolveIdentity();
					restartSocket();
				}),
			moveTo: (seat) => done(() => api.chooseSeat(code, S.me.name, seat)),
			setCult: (seat, cult) => done(() => api.setSeatCult(code, S.me.name, seat, cult)),
			addBot: (kind, seat) => done(() => api.addBot(code, S.me.name, kind, seat)),
			remove: (seat) => done(() => api.removeSeat(code, S.me.name, seat)),
			leave: () =>
				done(async () => {
					await api.leave(code, S.me.name);
					S.me = { name: null, seat: 0 };
				}),
			start: () => done(() => api.start(code, S.me.name)),
			fillBots: (kind) =>
				done(async () => {
					for (const sd of S.info.seats) {
						const si = seatInfo(sd.seat);
						if (!si || !si.name) await api.addBot(code, S.me.name, kind, sd.seat);
					}
				}),
		},
	};
}

function drawBoard(host) {
	// A card game has no board: its cards are drawn by cardPanels.
	if (S.info.board.style === "none") {
		host.hidden = true;
		return;
	}
	const acting = actingSeat();
	const interactive = acting !== 0;
	const b = activeBuilder();
	// On an area map, units are picked by clicking their tokens: the areas
	// they stand in are not themselves targets until a choice is under way.
	const tokensFirst = S.info.board.style === "areas" && b.steps.length === 0;
	const targets = interactive && !tokensFirst ? b.nextPositions() : new Set();
	renderBoard(
		host,
		S.info,
		{
			cells: currentCells(),
			flipped: S.flipped,
			lastMove: S.preview || !S.settings.lastMove ? [] : lastMoveSquares(),
			hint: S.tut ? tutorialStep().highlight || [] : hintSquares(),
			arrows: S.tut && (tutorialStep().arrow || []).length === 2 ? [tutorialStep().arrow] : null,
			ghost: ghostPiece(),
			// Every square chosen so far: the origin, then each landing/drop.
			selected: interactive ? b.steps.filter((st) => st.kind === "pos").map((st) => st.value) : null,
			targets,
			quick: interactive && !isRuleset() ? new Set(b.quickPlacements().keys()) : null,
			pathCounts: interactive ? b.dropCounts() : new Map(),
			interactive,
			showCoords: S.settings.coords,
			tints: (rulesetView() || {}).tints || null,
			badges: (rulesetView() || {}).badges || null,
			walls: (rulesetView() || {}).walls || null,
			holders: (rulesetView() || {}).holders || null,
			layers: { names: S.settings.names, glyphs: glyphLayerOn(), sigils: S.settings.sigils },
			legend: isRuleset() ? S.info.display || null : null,
			tokenOk: interactive ? tokenOk : null,
			enhanced: enhancedUnits(),
			inspect: S.unitInfo,
			navigable: true,
			animate: S.settings.animate && S.info.board.style !== "areas",
			// When most of the board is clickable (placing anywhere on an
			// open board) marking every square is noise: rely on hover.
			quietTargets: !S.settings.hints || targets.size > S.info.board.positions.length * 0.4,
		},
		{ onCell, onHover, onToken }
	);
}

function statusBar() {
	const cls = ["status-bar"];
	let msg = "";
	const g = S.game;
	const badges = [];

	if (inAnalysis()) {
		cls.push("history");
		const a = S.analysis;
		const sim = a.sim;
		const where = a.base === null ? "the current position" : `the position after move ${a.base}`;
		if (!sim) msg = "Analysis — loading…";
		else if (sim.game_ended) msg = `Analysis: the variation ends the game${sim.winner_seat ? ` — ${seatLabel(sim.winner_seat)} wins` : " in a draw"}.`;
		else msg = `Analysis from ${where}: ${seatLabel(sim.turn_seat)} to move. Nothing here is played.`;
	} else if (!isLive()) {
		cls.push("history");
		const seq = S.viewSeq;
		const rec = seq > 0 ? S.moves[seq - 1] : null;
		msg = seq === 0 ? "Viewing the starting position." : `Viewing the position after move ${seq}${rec ? ` (${rec.notation})` : ""}.`;
	} else if (g.status === "waiting") {
		msg = inLobby()
			? "Waiting in the lobby — send the invite link to the players you want, choose your seats, then start the game."
			: "Waiting for an opponent — send the link from Options to invite someone, or add a bot there.";
	} else if (g.status === "finished") {
		cls.push("over");
		msg = resultMessage();
	} else if (isMyTurn()) {
		cls.push("your-turn");
		msg = turnPrompt();
	} else if (S.me.seat) {
		const n = seatName(g.turn_seat);
		const si = seatInfo(g.turn_seat);
		msg = si && si.bot ? `${n} is thinking…` : `Waiting for ${n || seatLabel(g.turn_seat)} to move…`;
		const rv = rulesetView();
		if (rv && rv.prompt) msg = si && si.bot ? `${rv.prompt} (${n} is thinking…)` : rv.prompt;
	} else {
		msg = `${seatLabel(g.turn_seat)} (${displayName(g.turn_seat)}) to move.`;
	}

	const bar = h("div", { class: cls.join(" ") });
	bar.append(h("div", { class: "message", id: "status-message", "aria-live": "polite" }, msg));
	if (isLive() && !inAnalysis() && g.status === "active") {
		if (isMyTurn()) badges.push(h("span", { class: "badge turn" }, "Your turn"));
		{
			const rv = rulesetView();
			badges.push(
				h(
					"span",
					{ class: "badge" },
					rv && rv.round ? `Round ${rv.round}` : isRuleset() ? `Ply ${S.moves.length}` : `Move ${Math.floor(S.moves.length / S.info.seats.length) + 1}`
				)
			);
		}
	}
	if (g.status === "finished") badges.push(h("span", { class: "badge done" }, "Finished"));
	badges.push(h("span", { class: "badge", id: "conn-badge" }, S.conn === "online" ? "live" : S.conn === "offline" ? "reconnecting…" : "connecting…"));
	for (const b of badges) bar.append(b);
	if (inAnalysis()) {
		const sim = S.analysis.sim;
		if (sim && !sim.game_ended) {
			bar.append(h("button", { id: "play-from-here", onclick: playFromHere, title: "Start a new game from this position" }, "Play from here…"));
		}
		bar.append(h("button", { class: "primary", id: "exit-analysis", onclick: exitAnalysis }, "Exit analysis"));
	} else if (!isLive()) {
		bar.append(h("button", { id: "analyze-here", onclick: startAnalysis }, "Analyze from here"));
		bar.append(h("button", { class: "primary", id: "return-live", onclick: () => viewMove(null) }, "Return to live"));
	} else if (g.status === "finished") {
		bar.append(rematchControl());
	}
	return bar;
}

function rematchControl() {
	const opp = (S.game.seats || []).find((x) => x.bot && x.seat !== S.me.seat);
	if (S.me.seat && opp) {
		// A game from a custom position (a puzzle, "play from here") is
		// replayed from that position on the same side; otherwise it is the
		// same bot with the other side of the board.
		const custom = !!S.game.start_state && !isRuleset();
		return h(
			"button",
			{
				class: "primary",
				id: "rematch",
				onclick: async () => {
					const other = S.info.seats.find((x) => x.seat !== S.me.seat);
					try {
						const body = {
							game_def_id: S.game.game_def_id,
							player_name: S.me.name,
							opponent: opp.bot,
							seat: custom ? S.me.seat : other ? other.seat : 1,
						};
						if (custom) {
							body.start_state = S.game.start_state;
							body.start_seat = S.game.start_turn_seat || undefined;
						}
						const res = await api.createGame(body);
						rememberGame(res.room_code, S.game.game_def_id, S.me.name);
						navigate("/game/" + res.room_code);
					} catch (err) {
						toast(err.message, "error");
					}
				},
			},
			custom ? "Try again" : "Rematch"
		);
	}
	return h("a", { class: "button primary", id: "new-game", href: `/?game=${encodeURIComponent(S.game.game_def_id)}` }, "New game");
}

/** The page of a game this server no longer offers: who played, how it ended, the moves. */
function retiredRecord() {
	const g = S.game;
	const seats = g.seats || [];
	const who = (seat) => {
		const s = seats.find((x) => x.seat === seat);
		return s && s.name ? s.name : `Seat ${seat}`;
	};
	let ending;
	if (g.end_reason === "retired") ending = "It was still being played when the server stopped offering the game, so it ended with no result.";
	else if (g.status !== "finished") ending = "It was never finished.";
	else if (g.winner_seat === null || g.winner_seat === undefined) ending = "It ended in a draw.";
	else ending = `${who(g.winner_seat)} won.`;
	const players = seats.filter((s) => s.name).map((s) => s.name + (s.bot ? " (bot)" : ""));
	return h(
		"div",
		{ class: "card", id: "retired-game" },
		h("h2", {}, `${S.retired.name} is no longer played here`),
		h("p", {}, `This server no longer offers ${S.retired.name}. This game is kept as a record: the board can't be shown any more, but the moves can.`),
		players.length ? h("p", {}, `Played by ${players.join(" and ")}. ${ending}`) : h("p", {}, ending),
		S.retired.moves.length
			? h("ol", { class: "retired-moves" }, S.retired.moves.map((m) => h("li", {}, `${who(m.seat)}: ${m.notation || m.to || "—"}`)))
			: h("p", { class: "muted" }, "No moves were made."),
		h("a", { class: "button", href: "/" }, "Back to the lobby")
	);
}

function resultMessage() {
	const g = S.game;
	const reasons = {
		road: "by road",
		"flat-win": "on flats",
		resignation: "by resignation",
		timeout: "on time",
		agreement: "by agreement",
		line: "with a line",
		five: "with five in a row",
		connection: "by connecting their edges",
		score: "on points",
		discs: "on discs",
		"no-moves": "because the opponent cannot move",
		breakthrough: "by breaking through",
		doom: "with the most Doom and all six spellbooks",
		"instant-death": "after Instant Death",
		draw: "",
		retired: "",
	};
	const how = g.end_reason ? ` ${reasons[g.end_reason] || "(" + g.end_reason + ")"}` : "";
	const detail = g.end_detail ? ` [${g.end_detail}]` : "";
	if (g.winner_seat === null || g.winner_seat === undefined) {
		if (g.end_reason === "humanity") return `Game over — humanity survives: no cult had all six spellbooks${detail}.`;
		return `Game over — a draw${how ? " " + how : ""}${detail}.`;
	}
	const who = `${seatLabel(g.winner_seat)} (${displayName(g.winner_seat)})`;
	if (S.me.seat) return g.winner_seat === S.me.seat ? `You won${how}${detail}! 🎉` : `${who} won${how}${detail}.`;
	return `${who} won${how}${detail}.`;
}

function turnPrompt() {
	const b = activeBuilder();
	const opts = b.options();
	const rv = rulesetView();
	if (rv && rv.stage === "moving") {
		// Moving units one by one: say which click comes next.
		const from = b.steps.find((st) => st.kind === "pos");
		const piece = b.steps.find((st) => st.kind === "piece");
		const label = piece ? (S.info.pieces.find((p) => p.type === piece.value) || {}).label || piece.value : "";
		if (from && piece) return `Move: click a highlighted area to send your ${label} from ${areaName(from.value)} to.`;
		if (from) return `Move: pick which unit leaves ${areaName(from.value)} (a glowing token or a button).`;
		return "Move: click a glowing unit to pick it up (1 Power each), then its destination. Press Done when finished.";
	}
	if (rv && rv.stage === "direction") {
		// The two choices below spell out who plays after whom.
		return "You are the first player — choose the direction of play: clockwise runs down the list of cults, counter-clockwise runs up it.";
	}
	if (rv && rv.prompt) {
		if (opts.some((o) => o.kind === "count")) return `${rv.prompt} — ${countLabel(b)}`;
		return `Your turn — ${rv.prompt}`;
	}
	if (b.manualSteps().length === 0) {
		if (openingActive() && S.info.opening.owner === "opponent") {
			return "Opening: place a flat stone of your opponent’s color on any square.";
		}
		const acts = (S.info.actions || []).map((a) => a.label.toLowerCase());
		return acts.length ? `Your turn — ${acts.join(" or ")}.` : "Your turn.";
	}
	if (opts.some((o) => o.kind === "count")) return "How many pieces do you want to pick up?";
	if (b.complete()) {
		return opts.length ? "Submit the move, or keep dropping pieces." : needsConfirm() ? "Check the preview, then submit." : "Submitting…";
	}
	const picked = b.pickedCount();
	if (picked) {
		const dropped = [...b.dropCounts().values()].reduce((a, c) => a + c, 0);
		return `Drop the remaining ${picked - dropped} piece${picked - dropped === 1 ? "" : "s"}: click the next square (click again to leave more there).`;
	}
	if (opts.some((o) => o.kind === "pos")) return "Choose a highlighted square.";
	return "Continue your move.";
}

// ---- present / away ---------------------------------------------------------------------

const eyeIcon = () =>
	s(
		"svg",
		{ viewBox: "0 0 16 16", width: 11, height: 11, "aria-hidden": "true" },
		s("path", { d: "M1 8s2.6-4.5 7-4.5S15 8 15 8s-2.6 4.5-7 4.5S1 8 1 8z", fill: "none", stroke: "#fff", "stroke-width": 1.5 }),
		s("circle", { cx: 8, cy: 8, r: 2.2, fill: "#fff" })
	);

/**
 * The picture of a player who is an animal (Nyet!: Wolf, Crane, Tiger, Bear,
 * Eagle), in the colour of the seat; null for the games whose players are not.
 */
function seatAnimal(seat, px) {
	if (!S.info || !S.info.cards) return null;
	const label = seatLabel(seat);
	if (!animalOf(label)) return null;
	return animalIcon(label, { color: cssColor(colorOf(seat)), size: px, title: label });
}

/** The same for a player named by label, as the captions under the trick's cards do. */
function seatAnimalByLabel(label, px) {
	const si = (S.info.seats || []).find((x) => x.label === label);
	return si ? seatAnimal(si.seat, px) : null;
}

/**
 * The coloured box beside a player's name (in a game of animals, the animal).
 * When the player says they are
 * watching the game right now it turns into a round eye, for everyone to see;
 * the player's own box is the switch.
 */
function presenceSwatch(seat, color, isBot) {
	const on = S.present.has(seat);
	const animal = seatAnimal(seat, 30);
	const cls = animal ? `seat-icon${on ? " present" : ""}` : `swatch color-${cssColor(color)}${on ? " present" : ""}`;
	const label = on ? "Watching the game right now" : "Not marked as watching";
	const eye = on ? h("span", { class: "eye" }, eyeIcon()) : null;
	const inner = animal ? [animal, eye] : eye;
	if (isBot || !isMine(seat) || !S.me.name) {
		return h("span", { class: cls, "data-presence": on ? "present" : "away", title: isBot ? "" : label }, inner);
	}
	return h(
		"button",
		{
			type: "button",
			class: cls + " presence-toggle",
			"data-presence": on ? "present" : "away",
			"aria-pressed": on ? "true" : "false",
			title: on ? "You are shown as watching. Click to show as away." : "Click to show the others you are watching the game right now.",
			onclick: () => setMyPresence(!S.present.has(seat)),
		},
		inner
	);
}

let presenceBeat = null;

async function setMyPresence(on) {
	if (!S.me.name || !S.me.seat) return;
	const seat = S.me.seat;
	if (on) S.present.add(seat);
	else S.present.delete(seat);
	render();
	keepPresence(on);
	try {
		await api.setPresence(code, S.me.name, on);
	} catch (err) {
		if (on) S.present.delete(seat);
		else S.present.add(seat);
		keepPresence(false);
		render();
		toast(err instanceof ApiError ? err.message : "Could not update your presence.");
	}
}

/** While present, say so again every minute: the mark lapses by itself otherwise. */
function keepPresence(on) {
	if (presenceBeat) window.clearInterval(presenceBeat);
	presenceBeat = null;
	if (!on) return;
	presenceBeat = window.setInterval(() => {
		api.setPresence(code, S.me.name, true).catch(() => {});
	}, 60000);
}

async function loadPresence() {
	try {
		const r = await api.presence(code);
		const next = new Set((r && r.seats) || []);
		const same = next.size === S.present.size && [...next].every((x) => S.present.has(x));
		S.present = next;
		// A reloaded page picks up where the player left off.
		if (S.me.seat && S.present.has(S.me.seat) && !presenceBeat) keepPresence(true);
		// Nothing changed: leave the page alone (a render would close a card the viewer is reading).
		if (!same) render();
	} catch (_) {
		/* the icons simply stay as they are */
	}
}

function seatStrip(seat) {
	const color = colorOf(seat);
	const si = seatInfo(seat);
	const toMove = inAnalysis()
		? !!S.analysis.sim && !S.analysis.sim.game_ended && S.analysis.sim.turn_seat === seat
		: isActive() && S.game.turn_seat === seat && isLive();
	const strip = h("div", { class: "seat-strip" + (toMove ? " to-move" : ""), id: `seat-${seat}`, "data-seat": seat });
	const head = h("div", { class: "seat-head" });
	head.append(presenceSwatch(seat, color, !!(si && si.bot)));
	head.append(h("span", { class: "name" }, seatName(seat) || "(open seat)"));
	// At a table of many cults the faction is named beside its player (a bot's
	// name already is the faction's).
	if (S.info.seats.length > 2 && seatLabel(seat) && !(seatName(seat) || "").startsWith(seatLabel(seat))) {
		head.append(h("span", { class: "seat-faction small muted" }, seatLabel(seat)));
	}
	if (si && si.bot) head.append(h("span", { class: "badge bot" }, "bot"));
	// Who holds the seat: an account, by its handle, or a guest.
	if (si && si.handle) head.append(h("span", { class: "badge account", title: "A signed-in account" }, "@" + si.handle));
	else if (si && si.guest) head.append(h("span", { class: "badge guest", title: "A guest: anybody with no account" }, "guest"));
	if (isMine(seat)) head.append(h("span", { class: "badge" }, "you"));
	// The roles of a card game's round: the dealer, who holds the bonus card, who is on the leader's team.
	const roles = S.info.cards ? rulesetView() : null;
	if (roles) {
		if (roles.dealer === seat) head.append(h("span", { class: "ct-badge", title: "Dealer" }, "D"));
		if (roles.bonus === seat) head.append(h("span", { class: "ct-badge bonus", title: "Holds the x2 bonus card" }, "×2"));
		if (roles.team && roles.team.includes(seat)) head.append(h("span", { class: "ct-badge team", title: "On the leader's team" }, "★"));
	}
	head.append(h("span", { class: "grow" }));
	const onClock = seatMsLeft(seat);
	if (onClock !== null && isLive()) {
		const c = gameClock();
		const running = c.running === seat;
		head.append(
			h(
				"span",
				{
					class: "clock" + (running ? " running" : "") + (onClock < 60000 ? " low" : ""),
					"data-clock": String(seat),
					title: running ? "This clock is running" : "Time left on this player's clock",
				},
				"⏱ " + formatClock(onClock)
			)
		);
	} else if (toMove && msLeft() !== null) {
		head.append(h("span", { class: "clock", "data-clock": "turn", title: "Time left to move" }, "⏱ " + formatClock(msLeft())));
	}
	for (const sc of scores()) {
		head.append(h("span", { class: "score", title: sc.label }, `${sc.label}: `, h("strong", {}, String(sc.per[seat]))));
	}
	for (const ct of S.info.counters || []) {
		const n = ((currentState().reserves || {})[color] || {})[ct.id] || 0;
		head.append(h("span", { class: "score counter", title: ct.label, "data-counter": ct.id }, `${ct.label}: `, h("strong", {}, String(n))));
	}
	const rv = rulesetView();
	const sv = rv && rv.seats ? rv.seats[String(seat)] : null;
	if (sv) {
		if (sv.court) {
			const term = (S.info.display && S.info.display.terms && S.info.display.terms[sv.court]) || "";
			head.append(h("span", { class: "court-tag", title: term, "data-court": sv.court }, sv.court_label || sv.court));
		}
		// The numbers stay together: the group wraps as a whole.
		const stats = h("span", { class: "seat-stats" });
		head.append(stats);
		for (const c of sv.counters || []) {
			// The spellbooks are listed in their own section below.
			if (c.id === "books") continue;
			// A counter may say what its value is made of (the Doom: Gates, Rituals, gifts, Elder Signs).
			const title = `${c.label}: ${c.value}${c.max ? " of " + c.max : ""}${c.note ? " (" + c.note + ")" : ""}`;
			const icon = statIcon(c.id, c.label, 15);
			if (icon) {
				// The words Power, Doom, Elder Signs and Gates are pictograms.
				stats.append(h("span", { class: "score counter stat", title, "data-counter": c.id, "aria-label": title }, icon, h("strong", {}, String(c.value))));
				continue;
			}
			const pips = h("span", { class: "counter-pip", "aria-hidden": "true" });
			for (let i = 0; i < (c.max && c.max <= 10 ? c.max : 0); i++) pips.append(h("i", { class: `${c.id}${i < c.value ? " on" : ""}` }));
			stats.append(h("span", { class: "score counter", title, "data-counter": c.id }, `${c.label}: `, h("strong", {}, String(c.value)), pips));
		}
	}
	strip.append(head);
	if (sv && sv.cards && sv.cards.length) strip.append(factionCards(seat, sv));
	const hand = handPanel(seat);
	if (hand) strip.append(hand);
	return strip;
}

/**
 * A cult's Elder Signs: face-down ones (their values are private: the owner
 * sees them, everybody else sees a count) and the revealed ones, which are
 * public and already in the Doom total. Drawn as tokens.
 */
function elderSignsBody(seat, sv) {
	const hidden = (sv && sv.hidden_signs) || 0;
	const shown = (sv && sv.shown_signs) || [];
	const mine = seat === S.me.seat;
	const row = h("div", { class: "sign-tokens", id: `signs-${seat}`, "data-seat": seat });
	if (!hidden && !shown.length) {
		row.append(h("span", { class: "muted small" }, "None yet."));
		return row;
	}
	if (hidden) {
		const values = mine ? sv.signs || [] : [];
		const group = h("span", {
			class: "sign-group",
			title: mine ? "Your Elder Signs: only you can see these values until you reveal them." : "Face down: only their owner can see them.",
		});
		for (let i = 0; i < hidden; i++) {
			const v = values[i];
			group.append(h("span", { class: "sign-chip facedown" + (v ? " own" : ""), "data-value": v || null }, v ? String(v) : "?"));
		}
		row.append(group, h("span", { class: "small muted" }, mine ? "yours alone" : "face down"));
	}
	if (shown.length) {
		const group = h("span", { class: "sign-group", title: "Revealed: everybody sees them and they count in the Doom total." });
		for (const v of shown) group.append(h("span", { class: "sign-chip shown" }, String(v)));
		row.append(h("span", { class: "small muted" }, "revealed"), group);
	}
	return row;
}

/**
 * Every unit of the cult, one icon each: those in the pool in full colour, those
 * standing on the map or held elsewhere (captured, asleep, submerged…) greyed
 * out. Hover for the card, which says where that unit is.
 */
function poolBody(seat, sv) {
	const color = colorOf(seat);
	const wrap = h("div", { class: "pool-icons", id: `pool-${seat}`, "data-seat": seat });
	let n = 0;
	for (const u of (sv && sv.units) || []) {
		const def = S.info.pieces.find((p) => p.type === u.id);
		const pool = u.pool || 0;
		const total = Math.max(u.total || 0, pool);
		for (let i = 0; i < total; i++) {
			const inPool = i < pool;
			n++;
			// The units away are told apart by the places the server lists.
			const where = inPool ? "in the pool" : (u.places || [])[i - pool] || "not in the pool";
			const icon = h(
				"span",
				{
					class: "pool-unit" + (inPool ? "" : " away"),
					tabindex: "0",
					"data-piece": u.id,
					"data-zone": inPool ? "pool" : "away",
					role: "img",
					"aria-label": `${u.name}: ${where}`,
				},
				pieceIcon(def ? def.shape : "disc", color, 26)
			);
			hoverCard(icon, () => unitTip(u, color, null, where));
			wrap.append(icon);
		}
	}
	if (!n) wrap.append(h("span", { class: "muted small" }, "No units."));
	const zone = capturedZone(seat, sv);
	if (!zone) return wrap;
	const both = h("div", { class: "pool-wrap" });
	both.append(wrap, zone);
	return both;
}

/**
 * The units this cult holds captive: one icon each, in the colour of the cult
 * they belong to. They go home — to their owners' pools — in Gather Power.
 */
function capturedZone(seat, sv) {
	const held = (sv && sv.captured) || [];
	if (!held.length) return null;
	const zone = h("div", { class: "captured-zone", id: `captured-${seat}`, "data-seat": seat });
	zone.append(h("div", { class: "zone-title small muted" }, `Captured (${held.length})`));
	const icons = h("div", { class: "pool-icons captured-icons" });
	for (const c of held) {
		const def = S.info.pieces.find((p) => p.type === c.unit);
		const ownerColor = colorOf(c.owner);
		const unit = unitCardFor(c.owner, c.unit);
		const name = unit ? unit.name : def ? def.label : c.unit;
		const fate =
			c.fate === "sacrificed"
				? `Held by ${seatLabel(seat)}: sacrificed in Gather Power for 1 Power, then back to ${seatLabel(c.owner)}'s pool`
				: `Held by ${seatLabel(seat)}: goes back to ${seatLabel(c.owner)}'s pool in Gather Power`;
		const icon = h(
			"span",
			{
				class: "pool-unit captured",
				tabindex: "0",
				"data-piece": c.unit,
				"data-owner": c.owner,
				role: "img",
				"aria-label": `${seatLabel(c.owner)} ${name}: ${fate}`,
			},
			pieceIcon(def ? def.shape : "disc", ownerColor, 26)
		);
		hoverCard(icon, () => {
			if (unit) return unitTip(unit, ownerColor, null, fate);
			const box = h("div", { class: "unit-tip" });
			box.append(h("div", { class: "unit-tip-head" }, pieceIcon(def ? def.shape : "disc", ownerColor, 34), h("div", { class: "unit-tip-title" }, h("strong", {}, name))));
			box.append(h("p", { class: "unit-where small" }, fate));
			return box;
		});
		icons.append(icon);
	}
	zone.append(icons);
	return zone;
}

// ---------------------------------------------------------------------------
// Reactions: the dock
// ---------------------------------------------------------------------------
//
// Reactions (Devolve, Necrophagy, Madness…) are abilities used on another
// player's turn: each time one is available the game asks its owner, which
// holds the table up. So the answer "no" is given for the player by default,
// and the player chooses the occasions on which they want to be asked instead
// — when they are targeted, after any other player's action, or after chosen
// players' actions — and must be able to change that at any time, above all
// while other players are acting and the game flies by.
//
// The page is rebuilt on every move, which would swallow a click that
// straddles one. So the control lives in the options window, outside the
// rebuilt area, is built once and only updated in place, and what the player
// chose is shown at once, before the server has answered.

let dockSig = ""; // the structure the dock was built for
let reactionsEpoch = 0; // moves on whenever the player changes the choice
let savesInFlight = 0;
let saveQueue = Promise.resolve();

/** "Black Goat" for an open seat, "Black Goat (Bex)" for a named one. */
function otherLabel(o) {
	const faction = o.label || `Seat ${o.seat}`;
	// A bot is named after its faction already.
	return o.name && !o.name.startsWith(faction) ? `${faction} (${o.name})` : o.name || faction;
}

function joinList(items) {
	if (items.length < 2) return items.join("");
	return items.slice(0, -1).join(", ") + " and " + items[items.length - 1];
}

/** The choice in force for one reaction: its own, else the general one. */
function choiceFor(r, id) {
	const c = r.choices && r.choices[id];
	return c || { targeted: !!r.targeted, ask_all: !!r.ask_all, ask_seats: r.ask_seats || [] };
}

/** What the current choice for one reaction means, in a sentence. */
function askSentence(r, ability) {
	const c = choiceFor(r, ability.id);
	const picked = new Set(c.ask_seats || []);
	const who = (r.others || []).filter((o) => picked.has(o.seat)).map(otherLabel);
	const when = [];
	if (c.targeted) when.push("an action is aimed at you");
	if (who.length) when.push(`${joinList(who)} acts`);
	let s;
	if (c.ask_all) s = `You are asked about ${ability.name} after every other player's action.`;
	else if (when.length) s = `You are asked about ${ability.name} when ${joinList(when)}; every other time it is passed for you.`;
	else s = `${ability.name} is always passed for you: you are never asked.`;
	return ability.held ? s : "Not earned yet — nothing is asked until you have it. Then: " + s.charAt(0).toLowerCase() + s.slice(1);
}

/** A round "?" button that opens a modal (open: the function that shows it). */
function helpButton(id, label, open) {
	const btn = h("button", { type: "button", class: "help-button", id, "aria-label": label, title: label, "aria-haspopup": "dialog" }, "?");
	btn.addEventListener("click", (ev) => {
		ev.preventDefault();
		ev.stopPropagation();
		open();
	});
	return btn;
}

/** A modal: a title, then blocks — strings are paragraphs, arrays are lists. */
function helpModal(id, title, blocks) {
	const old = document.getElementById(id);
	if (old) old.remove();
	const dlg = h("dialog", { id, class: "help-dialog", "aria-label": title });
	const close = h("button", { type: "button", class: "primary", id: `${id}-close` }, "Got it");
	close.addEventListener("click", () => dlg.close());
	dlg.append(h("h2", {}, title));
	for (const b of blocks) {
		if (Array.isArray(b)) dlg.append(h("ul", {}, ...b.map((t) => h("li", {}, t))));
		else dlg.append(h("p", {}, b));
	}
	dlg.append(h("div", { class: "row" }, close));
	dlg.addEventListener("close", () => dlg.remove());
	document.body.append(dlg);
	if (typeof dlg.showModal === "function") dlg.showModal();
	else dlg.setAttribute("open", "");
	close.focus();
}

/** The explanation behind the "?" of "When I'm targeted". */
function showTargetedHelp() {
	helpModal("ask-targeted-dialog", "When I'm targeted", [
		"Reactions are abilities you can use on another player's turn — Devolve, Necrophagy, Cannibalism, Madness, Energy Nexus and the Unspeakable Oath. Each time another player acts, the game works out whether you could react, and would then wait for your answer. So that nobody holds the table up, your reactions are passed for you unless you ask to be asked.",
		"With “When I'm targeted” on (it is by default) you are asked whenever an opponent's action is aimed at:",
		[
			"you, the player — an action that affects you directly, such as The Thousand Forms, Ghroth, a battle declared against you, a gift of Doom or Power, or a Cultist of yours being captured;",
			"one of your units — a Cultist or Monster of yours that is captured, attacked, swapped or replaced;",
			"an area where you have a presence — any unit of yours, or a Gate you control. An enemy that moves into it, recruits or summons in it, builds a Gate or awakens a Great Old One there, or brings Cthulhu up out of the sea there, is aiming at that area.",
		],
		"Only actions count. What happens as part of a battle — Madness sending a unit of yours to an area its player chooses, retreats, kills and pains, post-battle abilities — is not an action, so it never opens a question for you.",
		"When you are not targeted, your reaction is passed for you and the move list shows it as an automatic pass. Switch the option off to be asked less, or add “After any other player's action” or chosen players below to be asked more.",
		"Each reaction has boxes of its own, so you can be asked about one every time and about another only when it is aimed at you. Madness starts with every box ticked: you are asked about it in every battle, whoever fights, until you change it.",
	]);
}

/**
 * Shows, hides or refreshes the reactions section of the options window for the
 * current game and settings. It is built once and updated in place: a click on
 * it is never lost to a redraw of the page.
 */
function updateReactionDock() {
	if (!optionsWin) return;
	const host = optionsWin.section("reactions");
	const r = S.reactions;
	const show = !!r && !!r.abilities && r.abilities.length > 0 && !!S.game && S.game.status === "active" && !!S.me.seat && !inAnalysis();
	optionsWin.hide("reactions", !show);
	if (!show) {
		if (host.firstChild) {
			host.textContent = "";
			dockSig = "";
		}
		return;
	}
	// Rebuilt only when what it shows changes shape (a new ability, a new name).
	const sig = JSON.stringify([r.abilities.map((a) => [a.id, a.name, !!a.held]), (r.others || []).map((o) => [o.seat, o.label, o.name, o.color])]);
	if (sig !== dockSig || !host.firstChild) {
		buildReactionDock(host, r);
		dockSig = sig;
	}
	syncReactionDock(host, r);
}

function buildReactionDock(host, r) {
	host.textContent = "";
	host.classList.toggle("unearned", !r.abilities.some((a) => a.held));
	host.append(
		h(
			"p",
			{ class: "small muted dock-intro" },
			"Reactions are passed for you, so the game never waits on you. For each one, choose when you would like to be asked instead:"
		)
	);
	for (const a of r.abilities) host.append(buildAbilityChoice(a, r));
}

/** The boxes for one reaction: when targeted, after any action, after chosen players' actions. */
function buildAbilityChoice(a, r) {
	const id = a.id;
	const box = h("div", { class: "reaction-ability", "data-ability": id });
	box.append(
		h(
			"div",
			{ class: "dock-name", title: a.text },
			h("strong", {}, a.name),
			a.held ? null : h("span", { class: "muted small" }, " (not earned yet)")
		)
	);

	// When an action is aimed at the player.
	const targeted = h("input", { type: "checkbox", id: `ask-${id}-targeted` });
	targeted.addEventListener("change", () => saveReactions({ targeted: targeted.checked }, id));
	box.append(
		h(
			"div",
			{ class: "ask-row" },
			h("label", { class: "check", title: `Ask me about ${a.name} when an action is aimed at me, at one of my units or at an area where I have a unit or a Gate` }, targeted, h("span", {}, "When I'm targeted")),
			helpButton(`ask-${id}-help`, "What does “When I'm targeted” mean?", showTargetedHelp)
		)
	);

	// After any other player's action.
	const everyone = h("input", { type: "checkbox", id: `ask-${id}-all` });
	everyone.addEventListener("change", () => saveReactions({ ask_all: everyone.checked }, id));
	box.append(
		h(
			"div",
			{ class: "ask-row" },
			h("label", { class: "check", title: `Ask me about ${a.name} after every action of every other player` }, everyone, h("span", {}, "After any other player's action"))
		)
	);

	// Or only after some of the players.
	const details = h("details", { class: "reactions", id: `reactions-${id}` });
	details.append(h("summary", {}, "After chosen players' actions"));
	const list = h("div", { class: "auto-pass-list" });
	for (const o of r.others || []) {
		const input = h("input", { type: "checkbox", id: `ask-${id}-seat-${o.seat}` });
		input.addEventListener("change", () => {
			const next = new Set(choiceFor(S.reactions, id).ask_seats || []);
			if (input.checked) next.add(o.seat);
			else next.delete(o.seat);
			saveReactions({ ask_seats: [...next] }, id);
		});
		list.append(
			h(
				"label",
				{ class: "check auto-pass-option" },
				input,
				o.color ? h("span", { class: `swatch color-${cssColor(o.color)}` }) : null,
				h("span", {}, otherLabel(o))
			)
		);
	}
	details.append(list);
	box.append(details, h("span", { class: "dock-state small", id: `ask-${id}-state`, "aria-live": "polite" }));
	return box;
}

function syncReactionDock(host, r) {
	for (const a of r.abilities) {
		const id = a.id;
		const c = choiceFor(r, id);
		const picked = new Set(c.ask_seats || []);
		const targeted = host.querySelector(`#ask-${id}-targeted`);
		if (targeted) targeted.checked = !!c.targeted;
		const everyone = host.querySelector(`#ask-${id}-all`);
		if (everyone) everyone.checked = !!c.ask_all;
		for (const o of r.others || []) {
			const box = host.querySelector(`#ask-${id}-seat-${o.seat}`);
			if (!box) continue;
			box.checked = !!c.ask_all || picked.has(o.seat);
			box.disabled = !!c.ask_all;
		}
		// Amber when the player is never asked: a reaction cannot be used then.
		const section = host.querySelector(`.reaction-ability[data-ability="${id}"]`);
		if (section) section.classList.toggle("never-asked", !c.targeted && !c.ask_all && picked.size === 0);
		const state = host.querySelector(`#ask-${id}-state`);
		if (state) state.textContent = askSentence(r, a);
	}
}

/**
 * The player's choice of when to be asked about one reaction (id). The page
 * shows it at once; the requests go to the server one after the other, so the
 * last click wins, and an answer to an older question never puts an older
 * choice back.
 */
function saveReactions(patch, id) {
	if (!S.reactions) return Promise.resolve();
	const body = { ...patch, ability: id };
	if (body.ask_seats) body.ask_seats = [...new Set(body.ask_seats)].sort((a, b) => a - b);
	reactionsEpoch++;
	const choices = { ...(S.reactions.choices || {}) };
	choices[id] = { ...choiceFor(S.reactions, id), ...patch };
	if (body.ask_seats) choices[id].ask_seats = body.ask_seats;
	S.reactions = { ...S.reactions, choices };
	updateReactionDock();
	savesInFlight++;
	saveQueue = saveQueue.then(async () => {
		try {
			const res = await api.setReactions(code, S.me.name, body);
			reactionsEpoch++;
			// Another click is already on the page: its answer decides.
			if (savesInFlight === 1) S.reactions = res;
		} catch (err) {
			reactionsEpoch++;
			toast((err && err.message) || "The setting could not be saved.", "error");
			if (savesInFlight === 1) {
				// Fall back to what the server holds.
				try {
					S.reactions = await api.reactions(code, S.me.name);
				} catch (_) {
					/* the next move brings it up to date */
				}
			}
		} finally {
			savesInFlight--;
			updateReactionDock();
		}
	});
	return saveQueue;
}

/**
 * Under the question of a reaction: stop being asked — for the reasons that
 * made the game ask. It is the choice of the options window for that reaction,
 * made where the question is; the window now open is passed when nothing else
 * asks for it.
 */
function askShortcuts(rx) {
	const rxs = S.reactions;
	const ability = (rxs.abilities || []).find((a) => a.name.toLowerCase() === String(rx.ability).toLowerCase());
	if (!ability) return null;
	const id = ability.id;
	const c = choiceFor(rxs, id);
	const trigger = seatName(rx.trigger) || seatLabel(rx.trigger);
	const buttons = [];
	const stop = (shortcut, text, title, patch) =>
		buttons.push(
			h(
				"button",
				{
					type: "button",
					class: "small",
					id: shortcut,
					title,
					onclick: async () => {
						await saveReactions(patch(), id);
						try {
							await refreshGame();
						} catch (_) {
							/* the next move event brings the table up to date */
						}
						render();
					},
				},
				text
			)
		);
	if (rx.targeted && c.targeted) {
		stop("ask-stop-targeted", "when I'm targeted", `Stop asking me about ${rx.ability} when an action is aimed at me, as this one was`, () => ({ targeted: false }));
	}
	if (rx.trigger && (c.ask_seats || []).includes(rx.trigger)) {
		stop("ask-stop-trigger", `after ${trigger}'s actions`, `Stop asking me about ${rx.ability} after ${trigger}'s actions`, () => ({
			ask_seats: (choiceFor(S.reactions, id).ask_seats || []).filter((s) => s !== rx.trigger),
		}));
	}
	if (c.ask_all) {
		stop("ask-stop-all", "after any action", `Stop asking me about ${rx.ability} after every other player's action`, () => ({ ask_all: false }));
	}
	if (!buttons.length) return null;
	return h("span", { class: "auto-pass-shortcuts small", id: "ask-shortcuts" }, h("span", { class: "muted" }, `Stop being asked about ${rx.ability}:`), ...buttons);
}

// ---------------------------------------------------------------------------
// The Auto-Play Timeout
// ---------------------------------------------------------------------------
//
// A person's option, in the options window: when it is their turn to act and
// they have not for the time they chose, a tactical bot plays the turn for
// them. The moves are marked "auto-play" in the Moves and History tabs.

let autoplayBuiltFor = "";
let autoplaySaves = Promise.resolve();

const AUTOPLAY_DEFAULT = 24 * 60 * 60;
const AUTOPLAY_MIN = 60;
const AUTOPLAY_MAX = 365 * 24 * 60 * 60;

/** Seconds as {days, hours, minutes}. */
function splitTimeout(secs) {
	const s = Math.max(0, Math.round(secs));
	return { days: Math.floor(s / 86400), hours: Math.floor((s % 86400) / 3600), minutes: Math.floor((s % 3600) / 60) };
}

function timeoutText(secs) {
	const t = splitTimeout(secs);
	const parts = [];
	if (t.days) parts.push(`${t.days} ${t.days === 1 ? "day" : "days"}`);
	if (t.hours) parts.push(`${t.hours} ${t.hours === 1 ? "hour" : "hours"}`);
	if (t.minutes) parts.push(`${t.minutes} ${t.minutes === 1 ? "minute" : "minutes"}`);
	return parts.length ? joinList(parts) : "no time";
}

/** Loads this player's Auto-Play Timeout (people only: bots and spectators have none). */
async function loadAutoPlay() {
	if (!S.me.seat || !S.me.name || S.mine.length > 1 || !S.game || S.game.status === "waiting") {
		S.autoPlayTimeout = null;
		updateAutoPlay();
		return;
	}
	try {
		S.autoPlayTimeout = await api.autoplay(code, S.me.name);
	} catch (err) {
		if (err instanceof ApiError && err.status >= 400 && err.status < 500) S.autoPlayTimeout = null;
	}
	updateAutoPlay();
}

function autoplayState(a) {
	if (!a.enabled) return "Off. Switch it on and a tactical bot plays for you when it is your turn and you have not acted within the time above.";
	const wait = timeoutText(a.timeout_seconds);
	if (a.deadline && isActive() && S.game.turn_seat === S.me.seat) {
		const at = new Date(a.deadline);
		const left = at.getTime() - (Date.now() + S.skewMs);
		return `On. It is your turn: a tactical bot plays for you at ${at.toLocaleString()} (${left > 0 ? "in " + formatClock(left) : "now"}) unless you act first.`;
	}
	return `On. If it is your turn and you do not act within ${wait}, a tactical bot plays for you.`;
}

function showAutoPlayHelp() {
	helpModal("autoplay-help-dialog", "Auto-Play Timeout", [
		"Players do not always have the time to take their turn when it comes. With the Auto-Play Timeout on, the game does not wait for you for ever: when it is your turn to act — a turn of your own, or a reaction waiting for your answer — and you have not acted within the time you set, a tactical bot takes the turn for you.",
		[
			"The wait is counted from the last move in the game: the one that made it your turn, or your own last decision. Every move you make, and every move the bot makes for you, starts the count again.",
			"The bot plays the decisions of that turn, one after the other, until play passes to another player. Then the count starts over for your next turn.",
			"It is the tactical bot — the same one you can add to an empty seat: sensible, not brilliant. It will spend your Power and use your units as it sees fit.",
			"Every move it makes for you is marked “auto-play” in the Moves tab, and the turn is marked in the History tab, so everyone can see who really played it.",
			"You can switch it off, or change the time, whenever you like, even while it is your turn. It is off by default; when you switch it on it starts at 24 hours.",
		],
		"Dice that the bot rolled for you cannot be rolled again: you cannot take those moves back.",
	]);
}

/**
 * The email notifications section of the options window: only for the person
 * in a seat, and only when the server can send mail. Built once per seat.
 */
let notifyBuiltFor = "";
function updateNotify() {
	if (!optionsWin) return;
	const show = !!S.me.seat && !!S.me.name && !!S.game && S.game.status !== "finished";
	if (!show) {
		optionsWin.hide("notify", true);
		return;
	}
	const sig = `${code}|${S.me.name}`;
	if (notifyBuiltFor === sig) return;
	notifyBuiltFor = sig;
	optionsWin.hide("notify", true);
	const host = optionsWin.section("notify");
	host.textContent = "";
	mountGameNotify(host, { code, name: S.me.name, onReady: (ok) => optionsWin.hide("notify", !ok) });
	// The profile's switch is the default for games the player has not chosen for.
	applyProfileDefault({ code, name: S.me.name });
}

/** The Auto-Play Timeout section of the options window, built once and updated in place. */
function updateAutoPlay() {
	if (!optionsWin) return;
	const host = optionsWin.section("autoplay");
	const a = S.autoPlayTimeout;
	const show = !!a && !!S.game && S.game.status === "active" && !!S.me.seat && !inAnalysis();
	optionsWin.hide("autoplay", !show);
	if (!show) {
		if (host.firstChild) {
			host.textContent = "";
			autoplayBuiltFor = "";
		}
		return;
	}
	if (autoplayBuiltFor !== String(a.seat) || !host.firstChild) {
		buildAutoPlay(host);
		autoplayBuiltFor = String(a.seat);
	}
	const on = host.querySelector("#autoplay-on");
	if (on) on.checked = !!a.enabled;
	// The numbers follow the saved value, unless the player is in the middle
	// of changing them (every move redraws this).
	if (!host.dataset.dirty) setAutoPlayFields(host, a.timeout_seconds || AUTOPLAY_DEFAULT);
	const save = host.querySelector("#autoplay-save");
	if (save) save.disabled = !host.dataset.dirty;
	const state = host.querySelector("#autoplay-state");
	if (state) state.textContent = autoplayState(a);
	host.classList.toggle("on", !!a.enabled);
}

function setAutoPlayFields(host, secs) {
	const t = splitTimeout(secs);
	for (const [id, v] of [["days", t.days], ["hours", t.hours], ["minutes", t.minutes]]) {
		const el = host.querySelector(`#autoplay-${id}`);
		if (el) el.value = String(v);
	}
}

function buildAutoPlay(host) {
	host.textContent = "";
	delete host.dataset.dirty;
	const on = h("input", { type: "checkbox", id: "autoplay-on", role: "switch" });
	on.addEventListener("change", () => saveAutoPlay({ enabled: on.checked }));
	const head = h(
		"div",
		{ class: "ask-row" },
		h("label", { class: "check", title: "A tactical bot plays your turn if you take too long" }, on, h("span", {}, "Let a tactical bot play my turn if I take too long")),
		helpButton("autoplay-help", "How does the Auto-Play Timeout work?", showAutoPlayHelp)
	);
	const read = () => {
		const num = (id) => Math.max(0, Math.floor(Number(host.querySelector(`#autoplay-${id}`).value) || 0));
		return num("days") * 86400 + num("hours") * 3600 + num("minutes") * 60;
	};
	const commit = () => {
		const secs = read();
		if (secs < AUTOPLAY_MIN || secs > AUTOPLAY_MAX) {
			toast(secs < AUTOPLAY_MIN ? "Choose at least one minute." : "Choose at most 365 days.", "error");
			return;
		}
		delete host.dataset.dirty;
		saveAutoPlay({ timeout_seconds: secs });
	};
	const field = (id, label, max) => {
		const input = h("input", { type: "number", id: `autoplay-${id}`, min: "0", max: String(max), step: "1", inputmode: "numeric", "aria-label": label });
		input.addEventListener("input", () => {
			host.dataset.dirty = "1";
			const save = host.querySelector("#autoplay-save");
			if (save) save.disabled = false;
		});
		input.addEventListener("keydown", (ev) => {
			if (ev.key === "Enter") {
				ev.preventDefault();
				commit();
			}
		});
		return h("label", { class: "autoplay-field" }, input, h("span", {}, label));
	};
	const save = h("button", { type: "button", class: "small", id: "autoplay-save", disabled: true, title: "Use this time" }, "Set");
	save.addEventListener("click", commit);
	const row = h(
		"div",
		{ class: "autoplay-timeout", role: "group", "aria-label": "Timeout" },
		field("days", "days", 365),
		field("hours", "hours", 23),
		field("minutes", "minutes", 59),
		save
	);
	host.append(head, row, h("p", { class: "dock-state small", id: "autoplay-state", "aria-live": "polite" }));
}

/** Saves the Auto-Play Timeout; the page shows the choice at once. */
function saveAutoPlay(patch) {
	if (!S.autoPlayTimeout) return Promise.resolve();
	S.autoPlayTimeout = { ...S.autoPlayTimeout, ...patch };
	updateAutoPlay();
	autoplaySaves = autoplaySaves.then(async () => {
		try {
			S.autoPlayTimeout = await api.setAutoplay(code, S.me.name, patch);
		} catch (err) {
			toast((err && err.message) || "The setting could not be saved.", "error");
			try {
				S.autoPlayTimeout = await api.autoplay(code, S.me.name);
			} catch (_) {
				/* the next move brings it up to date */
			}
		} finally {
			updateAutoPlay();
		}
	});
	return autoplaySaves;
}

/**
 * A cult's sections: faction ability, spellbooks, requirements, Elder Signs
 * and units, each a collapsible list. What the player opened is remembered.
 */
const cardState = new Map(Object.entries(store.get("bingle.cards2", {})));
// Sections that start open; the rest start closed.
const OPEN_BY_DEFAULT = new Set(["units"]);

function factionCards(seat, sv) {
	const wrap = h("div", { class: "faction-cards", "data-seat": seat });
	const section = (id, title, count, body) => {
		const key = `${S.info.id}:${seat}:${id}`;
		const open = cardState.has(key) ? cardState.get(key) : OPEN_BY_DEFAULT.has(id);
		const d = h("details", { class: "faction-card", "data-card": id });
		if (open) d.setAttribute("open", "");
		// Remember what the player opened: every move redraws the page.
		d.addEventListener("toggle", () => {
			cardState.set(key, d.open);
			store.set("bingle.cards2", Object.fromEntries(cardState));
		});
		d.append(h("summary", {}, title, count ? h("span", { class: "muted small" }, ` ${count}`) : null), body);
		wrap.append(d);
	};
	const rowsBody = (card) => {
		const ul = h("ul", { class: "card-rows" });
		for (const r of card.rows) {
			ul.append(
				h(
					"li",
					{ class: r.state || (r.detail ? "info-row" : null), title: r.detail || null },
					r.state ? h("span", { class: "row-mark" }, spellMark(r.state === "done", r.state === "done" ? "Done" : "Not yet")) : null,
					h("span", { class: "row-text" }, r.state || !r.detail ? r.text : h("strong", {}, r.text)),
					r.detail && !r.state ? h("span", { class: "muted small row-detail" }, " — " + r.detail) : null
				)
			);
		}
		return ul;
	};
	const counted = (card) => {
		const done = card.rows.filter((r) => r.state === "done").length;
		const todo = card.rows.filter((r) => r.state).length;
		return todo ? `${done}/${todo}` : "";
	};
	const bySlot = new Map((sv.cards || []).map((c) => [c.id, c]));
	// The faction ability first, then what the cult may earn, its Elder Signs
	// and the units still to be placed. Games with other cards list them as sent.
	const known = ["ability", "books", "reqs"];
	for (const id of known) {
		const card = bySlot.get(id);
		if (card) section(card.id, card.title, counted(card), rowsBody(card));
	}
	for (const card of sv.cards || []) {
		if (known.includes(card.id) || card.id === "units") continue;
		section(card.id, card.title, counted(card), rowsBody(card));
	}
	if (sv.hidden_signs !== undefined) {
		const total = (sv.hidden_signs || 0) + (sv.shown_signs || []).length;
		if (total > 0) {
			section("signs", "Elder Signs", String(total), elderSignsBody(seat, sv));
		} else {
			// Nothing to show: a heading that does not open.
			wrap.append(h("div", { class: "faction-card faction-static", "data-card": "signs", id: `signs-${seat}` }, "Elder Signs", h("span", { class: "muted small" }, " 0")));
		}
	}
	if (bySlot.has("units")) {
		const inPool = (sv.units || []).reduce((n, u) => n + (u.pool || 0), 0);
		section("units", "Units", `${inPool} in the pool`, poolBody(seat, sv));
	}
	return wrap;
}

/** Seats in table order starting at `first`, wrapping round (clockwise). */
function seatsFrom(first) {
	const all = S.info.seats.map((x) => x.seat);
	const i = Math.max(0, all.indexOf(first));
	return [...all.slice(i), ...all.slice(0, i)];
}

/** A cult's colour swatch and name, for turn orders. */
function seatChip(seat, first) {
	return h(
		"span",
		{ class: "order-seat" + (first ? " first" : ""), "data-seat": seat, title: `${seatLabel(seat)} — ${displayName(seat)}${first ? " (first player)" : ""}` },
		h("span", { class: `swatch color-${cssColor(colorOf(seat))}` }),
		seatLabel(seat)
	);
}

/** The cults in play order, each way round, as arrowed chips. */
function orderLine(seats) {
	const line = h("span", { class: "order-line" });
	seats.forEach((seat, i) => {
		if (i) line.append(h("span", { class: "order-sep", "aria-hidden": "true" }, "→"));
		line.append(seatChip(seat, i === 0));
	});
	return line;
}

const PHASE_HINTS = {
	action: "Action phase: in turn order everyone spends Power on one action at a time until nobody has any left.",
	gather: "Gather Power: Cultists, Gates and captives pay out Power; nobody ends below half the leader.",
	first: "Determine first player: the most Power goes first and chooses the direction of play.",
	doom: "Doom phase: Gates score Doom; Rituals of Annihilation and Elder Signs are played in turn.",
};

/**
 * The Move cards of a game that has them (Onitama): both hands and the card at
 * the side, each drawn as the grid of squares it lets a piece reach. A card of
 * the player to move that can be played now is a button.
 */
/**
 * The panels of a card game (a board of choices, the card table), drawn from
 * the view's table and grid; empty for other games.
 */
function cardPanels() {
	const v = rulesetView();
	if (!v || !S.info.cards || !v.table) return [];
	const ctx = cardContext(v);
	// The board of terms stands beside the trick (under it on a narrow screen).
	if (v.grid) ctx.top = termsBoard(v, ctx);
	const table = cardTable(ctx);
	S.ctPrev = table.state;
	return [table.node];
}

/** What the card table and the board of terms need to draw this position and to act on it. */
function cardContext(v) {
	const b = actingSeat() ? activeBuilder() : null;
	const actions = new Set(b ? b.nextActions() : []);
	return {
		info: S.info,
		view: v,
		seats: S.info.seats,
		// The room is the key the order of a hand is remembered under.
		room: code,
		me: S.me.seat || 0,
		toMove: v.to_move,
		dealer: v.dealer,
		bonus: v.bonus,
		team: v.team,
		actions,
		playable: (tid) => actions.has(`card-${tid}`),
		onCard: (tid) => chooseAction(`card-${tid}`),
		onAction: (id) => chooseAction(id),
		prev: S.ctPrev,
		seatIconByLabel: seatAnimalByLabel,
	};
}

/** The board of terms of a card game with its round number. */
function termsBoard(v, ctx) {
	return h("div", { class: "ct-grid-wrap", id: "terms-board" }, h("div", { class: "ct-round small muted" }, `Round ${v.round} of ${v.rounds}`), choiceGrid(v.grid, ctx));
}

function cardsPanel() {
	const v = rulesetView();
	if (!v || !v.cards) return null;
	const seats = S.game.seats || [];
	const mover = v.to_move || 0;
	const b = actingSeat() ? activeBuilder() : null;
	const picked = b && b.steps[0] && b.steps[0].kind === "action" ? b.steps[0].value : null;
	const playable = b ? new Set(b.nextActions()) : new Set();
	const grid = (card, seat) => {
		const g = h("span", { class: "card-grid", "aria-hidden": "true" });
		const marks = new Set();
		for (const [dx, dy] of card.moves) {
			// Seen from the board: the second seat moves the other way.
			const bx = seat === 2 ? -dx : dx;
			const by = seat === 2 ? -dy : dy;
			marks.add(`${2 + bx},${2 - by}`);
		}
		for (let r = 0; r < 5; r++) {
			for (let c = 0; c < 5; c++) {
				const centre = r === 2 && c === 2;
				g.append(h("i", { class: centre ? "centre" : marks.has(`${c},${r}`) ? "reach" : "" }));
			}
		}
		return g;
	};
	const one = (card, seat, ownHand) => {
		const id = `card-${card.id}`;
		const can = ownHand && playable.has(id);
		const label = h("span", { class: "card-name" }, card.name);
		const attrs = { class: "move-card" + (can ? " playable" : "") + (picked === id ? " chosen" : ""), "data-card": card.id, title: card.name };
		if (can) {
			return h("button", { type: "button", ...attrs, onclick: () => chooseAction(id) }, grid(card, seat), label);
		}
		return h("span", attrs, grid(card, seat), label);
	};
	const panel = h("div", { class: "cards-panel", id: "cards-panel", "aria-label": "Move cards" });
	const group = (title, cards, seat, turn, own) => {
		const g = h("div", { class: "card-group" + (turn ? " to-move" : "") }, h("div", { class: "card-group-title small muted" }, title));
		const row = h("div", { class: "card-row" });
		for (const c of cards) row.append(one(c, seat, own));
		return g.append(row), g;
	};
	const name = (seat) => ((seats.find((x) => x.seat === seat) || {}).label || `Seat ${seat}`);
	for (const seat of [2, 1]) {
		panel.append(group(`${name(seat)}'s cards`, v.cards.hands[String(seat)] || [], seat, mover === seat, mover === seat && seat === actingSeat()));
		if (seat === 2) panel.append(group("Next card", [v.cards.side], mover || 1, false, false));
	}
	return panel;
}

/** Round, the phases in order, who plays when, the Ritual track and the bag. */
function roundBar() {
	const v = rulesetView();
	if (!v || !v.round || v.table) return null;
	const bar = h("div", { class: "round-bar", id: "round-bar" });
	bar.append(h("span", { class: "round-item round-no" }, h("strong", {}, `Round ${v.round}`)));

	// The phases of a round, left to right in the order they run; the current
	// one is lit and the ones already played this round are ticked.
	const steps = h("ol", { class: "phase-steps", id: "phase-steps", "aria-label": "Phases of the round" });
	for (const ph of v.phases || []) {
		steps.append(
			h(
				"li",
				{
					class: `phase-step ${ph.state}`,
					"data-phase": ph.id,
					"data-state": ph.state,
					"aria-current": ph.state === "current" ? "step" : null,
					title: PHASE_HINTS[ph.id] || null,
				},
				ph.state === "done" ? h("span", { class: "phase-mark", "aria-hidden": "true" }, "✓") : null,
				ph.label
			)
		);
	}
	bar.append(steps);
	if (v.phase === "over") bar.append(h("span", { class: "round-item alarm" }, "Game over"));

	// Who plays when, in the direction chosen.
	const order = v.order ? (v.dir < 0 ? v.order.ccw : v.order.cw) : null;
	if (order && order.length && v.phase !== "over") {
		const item = h(
			"span",
			{ class: "round-item turn-order", id: "turn-order", title: "Turn order from the first player, in the direction of play. Clockwise runs down the list of cults; counter-clockwise runs up it." },
			h("span", { class: "order-dir" }, v.dir < 0 ? "↺ Counter-clockwise" : "↻ Clockwise"),
			orderLine(order)
		);
		bar.append(item);
	}
	if (v.ritual && v.ritual.track && v.ritual.track.length) bar.append(ritualTrack(v.ritual));
	if (v.bag) {
		bar.append(
			h(
				"span",
				{
					class: "round-item bag",
					id: "elder-bag",
					"data-left": v.bag.left,
					"data-total": v.bag.total,
					title: `Elder Signs still in the bag: ${v.bag.left} of ${v.bag.total}. Their values are secret; the count is not.`,
					"aria-label": `Elder Signs in the bag: ${v.bag.left} of ${v.bag.total}`,
				},
				statIcon("signs", "Elder Signs in the bag", 16),
				h("strong", {}, `${v.bag.left}/${v.bag.total}`)
			)
		);
	}
	return bar;
}

/**
 * The Ritual of Annihilation track: a space for each Ritual that may still be
 * performed, with its cost in Power, and the marker on the one the next Ritual
 * pays. After the last space comes Instant Death: the game ends when that
 * Doom phase has been scored.
 */
function ritualTrack(r) {
	const wrap = h("span", { class: "round-item ritual" + (r.instant ? " instant" : ""), id: "ritual" });
	wrap.append(h("span", { class: "ritual-label" }, "Ritual of Annihilation"));
	const steps = h("ol", {
		class: "ritual-track",
		id: "ritual-track",
		"aria-label": `Ritual of Annihilation track: ${r.track.length} spaces, the marker is on ${r.instant ? "Instant Death" : "space " + (r.marker + 1)}`,
	});
	r.track.forEach((cost, i) => {
		const state = i < r.marker ? "past" : i === r.marker && !r.instant ? "next" : "ahead";
		steps.append(
			h(
				"li",
				{
					class: `ritual-step ${state}`,
					"data-step": i,
					"data-cost": cost,
					"data-state": state,
					title:
						state === "past"
							? `Space ${i + 1}: a Ritual was performed here (${cost} Power)`
							: state === "next"
							? `Space ${i + 1}: the next Ritual costs ${cost} Power`
							: `Space ${i + 1}: a later Ritual will cost ${cost} Power`,
				},
				String(cost)
			)
		);
	});
	steps.append(
		h(
			"li",
			{
				class: "ritual-end" + (r.instant ? " reached" : ""),
				"data-state": r.instant ? "reached" : "ahead",
				title: "Instant Death: when the marker gets here the game ends once the Doom phase has been scored.",
			},
			"☠"
		)
	);
	wrap.append(steps);
	// The marker on the track already says what the next Ritual costs; only
	// Instant Death needs a word.
	if (r.instant) {
		wrap.append(h("span", { class: "small ritual-note alarm" }, "Instant Death — the game ends when this Doom phase is scored"));
	}
	return wrap;
}

const CLASS_LABEL = { cultist: "Cultist", monster: "Monster", terror: "Terror", goo: "Great Old One" };

/** "color|unit" of every unit one of whose spellbooks its cult has earned. */
function enhancedUnits() {
	const out = new Set();
	const rv = rulesetView();
	if (!rv || !rv.seats) return out;
	for (const sd of S.info.seats) {
		const sv = rv.seats[String(sd.seat)];
		if (!sv) continue;
		for (const u of sv.units || []) if (u.enhanced) out.add(`${sd.color}|${u.id}`);
	}
	return out;
}

/** The information card of the unit last clicked on the map. */
function unitInfoCard() {
	const ui = S.unitInfo;
	if (!ui || !isRuleset()) return null;
	const rv = rulesetView();
	const sd = S.info.seats.find((x) => x.color === ui.color);
	const sv = rv && rv.seats && sd ? rv.seats[String(sd.seat)] : null;
	const unit = sv ? (sv.units || []).find((x) => x.id === ui.piece) : null;
	const here = ((currentCells()[ui.pos] || []).filter((pc) => pc.color === ui.color && pc.piece === ui.piece) || []).length;
	if (!unit || !here) {
		// The unit has gone from there: nothing to look at any more.
		S.unitInfo = null;
		return null;
	}
	const def = S.info.pieces.find((p) => p.type === ui.piece);
	const card = h("div", { class: "card unit-info", id: "unit-info", "data-piece": ui.piece, "data-seat": sd.seat });
	card.append(
		h(
			"div",
			{ class: "unit-info-head" },
			pieceIcon(def ? def.shape : "disc", ui.color, 30),
			h(
				"div",
				{ class: "unit-info-title" },
				h("strong", {}, unit.name),
				h("span", { class: "muted small" }, `${seatLabel(sd.seat)} · ${CLASS_LABEL[unit.class] || unit.class}${seatName(sd.seat) ? " · " + seatName(sd.seat) : ""}`)
			),
			h("button", { class: "ghost icon", id: "unit-info-close", title: "Close (Esc)", "aria-label": "Close the unit details", onclick: closeUnitInfo }, "×")
		)
	);
	const stats = h("dl", { class: "unit-stats" });
	const stat = (label, value, title) => stats.append(h("div", { title: title || null }, h("dt", {}, label), h("dd", {}, value)));
	const combat = /^\d+$/.test(unit.combat) ? (unit.combat === "0" ? "none" : `${unit.combat} ${unit.combat === "1" ? "die" : "dice"}`) : unit.combat;
	stat(unit.class === "cultist" ? "Recruit" : unit.class === "goo" ? "Awaken" : "Summon", `${unit.cost} Power`);
	stat("Combat", combat);
	stat("Here", `${here} in ${areaName(ui.pos)}`, "units of this kind in the area you clicked");
	stat("On the map", `${unit.on_map} of ${unit.total}`);
	stat("Pool", String(unit.pool), "units still to be placed");
	if (unit.away) stat("Submerged", String(unit.away));
	if (unit.captured) stat("Captured", String(unit.captured), "Cultists held on other cults' cards");
	card.append(stats);
	const abilities = abilitiesBlock(unit);
	if (abilities) card.append(abilities);
	const enter = enterBlock(unit);
	if (enter) card.append(enter);
	const text = unitText(unit);
	if (text) card.append(h("p", { class: "unit-text small" }, text));
	if (unit.books && unit.books.length) {
		card.append(h("p", { class: "small muted unit-books-title" }, "Spellbooks that enhance this unit"));
		const ul = h("ul", { class: "card-rows unit-books" });
		for (const b of unit.books) {
			ul.append(
				h(
					"li",
					{ class: b.unlocked ? "done" : "todo", "data-book": b.id, title: b.text },
					h("span", { class: "row-mark" }, spellMark(b.unlocked, b.unlocked ? `${b.name}: unlocked` : `${b.name}: not unlocked`)),
					h("span", { class: "row-text" }, h("strong", {}, b.name), h("span", { class: "muted small" }, ` — ${b.timing}: ${b.text}`))
				)
			);
		}
		card.append(ul);
	} else {
		card.append(h("p", { class: "small muted" }, "No spellbook enhances this unit."));
	}
	return card;
}

/** The battle being fought, or the last one fought. */
function battlePanel() {
	const v = rulesetView();
	if (!v || !v.battle) return null;
	const bt = v.battle;
	const card = h("div", { class: "exchange battle", id: "battle-panel", "data-resolved": bt.resolved ? "1" : "0" });
	card.append(
		h(
			"h3",
			{},
			`${bt.resolved ? "Last battle" : "Battle"} in ${bt.area_name}`,
			bt.step && !bt.resolved ? h("span", { class: "muted small" }, ` (${bt.step})`) : null
		)
	);
	for (const side of bt.sides) {
		const row = h("div", { class: "side", "data-seat": side.seat });
		row.append(h("div", { class: "who" }, side.name));
		const pool = h("div", { class: "pool" });
		pool.append(h("span", { class: "pool-part" }, h("span", { class: "pool-label" }, "Combat"), h("strong", {}, String(side.combat))));
		if (side.dice && side.dice.length) {
			const dice = h("span", { class: "pool-part" });
			for (const d of side.dice) dice.append(h("span", { class: "die " + (d === 6 ? "ok" : d >= 4 ? "mid" : "fail"), title: d === 6 ? "Kill" : d >= 4 ? "Pain" : "Miss" }, String(d)));
			pool.append(dice);
		}
		pool.append(
			h("span", { class: "pool-part muted small" }, `suffers ${side.kills} Kill${side.kills === 1 ? "" : "s"}, ${side.pains} Pain${side.pains === 1 ? "" : "s"}`)
		);
		row.append(pool);
		card.append(row);
	}
	return card;
}

function handPanel(seat) {
	const pools = S.info.pools || [];
	if (!pools.length) return null;
	// Games with their own unit panels (Cthulhu Wars) have no hands.
	if (S.info.display && S.info.display.no_hands) return null;
	const color = colorOf(seat);
	const reserves = (currentState().reserves || {})[color] || {};
	// Units are only in hand while they are being deployed.
	if (isRuleset() && pools.every((p) => !(reserves[p.id] > 0))) return null;
	const mine = actingSeat() === seat;
	const b = activeBuilder();
	const nextPieces = mine ? b.nextPieces() : new Set();
	const chosen = b.steps.find((s) => s.kind === "piece");
	const wrap = h("div", { class: "hand", id: `hand-${cssColor(color)}` });
	const opening = mine && openingActive() && S.info.opening.owner === "opponent";

	for (const pool of pools) {
		const count = reserves[pool.id] ?? 0;
		const types = pool.pieces.map((t) => S.info.pieces.find((p) => p.type === t)).filter(Boolean);
		// Pieces with a rules text get a big icon and a count badge; the name
		// and the rules are on the hover card.
		const tiles = types.length > 0 && types.every((t) => t.rules);
		const group = h("span", { class: "pool" + (count === 0 ? " empty" : "") + (tiles ? " tiles" : ""), "data-pool": pool.id, title: tiles ? null : `${pool.label}: ${count} in hand` });
		if (!tiles) {
			group.append(h("span", { class: "pool-count" }, String(count)));
			if (types.length === 1) group.append(h("span", { class: "pool-name" }, types[0].label.toLowerCase()));
		}

		for (const pc of types) {
			const drawColor = opening ? placementColor() : color;
			const choosable = nextPieces.has(pc.type);
			const isChosen = !!chosen && chosen.value === pc.type && mine;
			const classes = ["hand-piece"];
			if (choosable) classes.push("choosable");
			if (isChosen) classes.push("chosen");
			let kids;
			if (tiles) {
				classes.push("tile");
				kids = [pieceIcon(pc.shape, drawColor, 44), h("span", { class: "tile-count" }, String(count))];
			} else {
				kids = [pieceIcon(pc.shape, drawColor, 22)];
				if (types.length > 1) kids.push(h("span", {}, pc.label));
			}
			const tip = tiles ? null : choosable ? `Place: ${pc.label}` : pc.label;
			let node;
			if (mine) {
				node = h("button", { class: classes.join(" "), "data-piece": pc.type, "aria-label": `${pc.label}: ${count} left`, disabled: !choosable && !isChosen, title: tip, onclick: () => choosePiece(pc.type) }, kids);
			} else {
				node = h("span", { class: classes.join(" "), "data-piece": pc.type, "aria-label": `${pc.label}: ${count} left`, title: tip, tabindex: tiles ? "0" : null }, kids);
			}
			if (tiles) {
				hoverCard(node, () =>
					h(
						"div",
						{ class: "piece-tip" },
						pieceIcon(pc.shape, drawColor, 56),
						h("div", {}, h("strong", {}, pc.label), h("span", { class: "muted" }, ` · ${count} left`), h("p", {}, pc.rules))
					)
				);
			}
			group.append(node);
		}
		wrap.append(group);
	}
	return wrap;
}

function moveBar() {
	const bar = h("div", { class: "move-bar", id: "move-bar" });
	const acting = actingSeat();

	if (inAnalysis()) {
		const a = S.analysis;
		if (a.moves.length) {
			bar.append(h("span", { class: "pending", id: "variation" }, "Line: " + a.moves.join(" ")));
		} else {
			bar.append(h("span", { class: "hint" }, "Play moves for either side to explore; nothing is sent to your opponent."));
		}
	}

	if (!acting && !inAnalysis()) {
		bar.append(
			h(
				"span",
				{ class: "hint" },
				isLive() ? (S.me.seat ? "" : S.game.status === "waiting" ? "" : "You are watching this game.") : "Browsing history — the board is read-only."
			)
		);
		const undo = undoButton();
		if (undo) bar.append(h("span", { class: "move-actions" }, undo));
		return bar;
	}
	const b = activeBuilder();

	if (acting && b.manualSteps().length) {
		bar.append(h("span", { class: "pending", id: "pending-move" }, describeSteps(b.steps)));
	} else if (acting && !inAnalysis() && isRuleset()) {
		const opts = b.options();
		const hasPos = opts.some((o) => o.kind === "pos");
		const hasAct = opts.some((o) => o.kind === "action");
		const hasPiece = opts.some((o) => o.kind === "piece");
		const cell = S.info.board.style === "areas" ? "area" : "hex";
		const rvh = rulesetView();
		// A normal turn: units to move on the map, or an action to choose.
		const turnStage = !!rvh && rvh.stage === "turn" && b.steps.length === 0;
		// Choosing the spellbook that a requirement earned.
		const choosingBook = hasAct && opts.filter((o) => o.kind === "action").every((o) => String(o.value).startsWith("book-"));
		bar.append(
			h(
				"span",
				{ class: "hint", id: "move-hint" },
				choosingBook
					? "Choose a spellbook to unlock:"
					: hasPiece && !hasPos && rvh && rvh.stage === "prompt"
					? "Choose the unit to place:"
					: hasPiece
					? `Choose a unit, then a highlighted ${cell}.`
					: turnStage && hasAct && (hasPos || hasPiece)
					? "Click a unit on the map to move, or choose an action:"
					: hasPos && hasAct
					? `Click a highlighted ${cell}, or choose an action.`
					: hasPos
					? `Click a highlighted ${cell}.`
					: hasAct
					? (rulesetView() || {}).panel_actions
						? "Click a card or a box on the table, or choose from the buttons."
						: "Choose an action."
					: opts.some((o) => o.kind === "count")
					? "Choose a number."
					: ""
			)
		);
	} else if (acting && !inAnalysis()) {
		const opts = b.options();
		const hasPiece = opts.some((o) => o.kind === "piece");
		const hasPos = opts.some((o) => o.kind === "pos");
		// A forced piece choice was taken automatically: the squares on offer
		// are placement targets, not pieces to pick up.
		const placing = hasPos && !hasPiece && b.steps.length > 0 && b.steps[0].kind === "piece";
		const points = S.info.board.style === "points";
		bar.append(
			h(
				"span",
				{ class: "hint" },
				hasPiece && hasPos
					? "Click an empty square to place a stone (Shift: wall, Ctrl: capstone), or click a highlighted stack to move it."
					: hasPiece && !hasPos && b.quickPlacements().size > 0
					? `${points ? "Click an intersection to place a stone" : "Click an empty square to place a piece"}${
							opts.some((o) => o.kind === "action") ? ", or choose an action" : ""
					  }.`
					: hasPiece
					? "Choose a piece from your hand."
					: placing
					? points
						? "Click an intersection to place a stone."
						: "Click an empty square to place a piece."
					: hasPos
					? "Click a highlighted piece to move it."
					: opts.some((o) => o.kind === "action")
					? (rulesetView() || {}).panel_actions
						? "Click a card or a box on the table, or choose from the buttons."
						: "Choose an action."
					: "No moves available."
			)
		);
	}

	const actionIds = acting && (isRuleset() || b.manualSteps().length === 0) ? b.nextActions() : [];
	if (actionIds.length) {
		const group = h("span", { class: "action-buttons" });
		const rv = rulesetView();
		for (const id of actionIds) {
			// The cards and the boxes of the board are chosen where they are drawn.
			if (rv && rv.panel_actions && rv.panel_actions.some((p) => id.startsWith(p))) continue;
			const def = actionDef(id);
			// The direction of play is shown as the order it makes.
			if ((id === "cw" || id === "ccw") && rv && rv.stage === "direction" && rv.order) {
				group.append(directionButton(id, rv));
				continue;
			}
			let label = def ? def.label : (rv && rv.labels && rv.labels[id]) || id;
			if (id === "done" && rv && rv.reaction && rv.reaction.seat === acting) {
				label = `Pass (do not use ${rv.reaction.ability})`;
			}
			if (id === "reveal") {
				// Only your own view knows what the signs are worth.
				const sum = ((rv && rv.seats && rv.seats[String(acting)]) || {}).signs || [];
				const total = sum.reduce((a, c) => a + c, 0);
				if (total) label += ` (+${total} Doom)`;
			}
			// A unit to summon or awaken has a button of its own: its icon, and
			// what it costs just now; the details are in the card that opens
			// on hover.
			const unitKind = id.startsWith("summon-") ? "summon" : id.startsWith("awaken-") ? "awaken" : "";
			if (unitKind) {
				const button = unitButton(unitKind, id.slice(unitKind.length + 1), acting);
				if (button) {
					group.append(button);
					continue;
				}
			}
			group.append(actionButton(id, def, label, rv));
		}
		const undoInGroup = undoButton();
		if (undoInGroup) group.append(undoInGroup);
		bar.append(group);
		// Under the question of a reaction: a way to stop being asked.
		if (rv && rv.reaction && rv.reaction.seat === acting && acting === S.me.seat && S.reactions) {
			const stops = askShortcuts(rv.reaction);
			if (stops) bar.append(stops);
		}
	}

	// Games without hands offer unit types as buttons.
	if (acting && isRuleset() && S.info.display && S.info.display.no_hands) {
		const kinds = [...b.nextPieces()];
		if (kinds.length) {
			const group = h("span", { class: "piece-buttons", id: "piece-buttons" });
			const color = colorOf(acting);
			for (const type of kinds) {
				const look = pieceLook(type);
				group.append(
					h(
						"button",
						{
							class: "piece-choice" + (look.onGate ? " on-gate" : ""),
							"data-piece": type,
							title: look.onGate
								? b.steps.some((st) => st.kind === "action" && st.value === "capture")
									? `${look.label}: capturing it leaves the Gate abandoned, and your own Cultist takes it`
									: `${look.label}: moving it abandons the Gate`
								: look.label,
							onclick: () => choosePiece(type),
						},
						pieceIcon(look.shape, look.color || color, 22),
						h("span", {}, look.label)
					)
				);
			}
			bar.append(group);
		}
	}

	const counts = acting ? b.nextCounts() : [];
	if (counts.length) {
		const picker = h("span", { class: "count-picker", id: "count-picker" }, h("span", { class: "small muted" }, countLabel(b)));
		for (const n of counts) picker.append(h("button", { "data-count": n, onclick: () => chooseCount(n) }, String(n)));
		bar.append(picker);
	}

	const carried = acting ? b.pickedCount() : 0;
	if (carried) {
		const dropped = [...b.dropCounts().values()].reduce((a, c) => a + c, 0);
		bar.append(carryDisplay(b, carried, dropped));
	}

	const actions = h("span", { class: "move-actions" });
	if (inAnalysis()) {
		actions.append(
			h("button", { id: "analysis-back", class: "ghost", disabled: !S.analysis.moves.length, onclick: analysisBack }, "↶ Take back"),
			h("button", { id: "analysis-import", class: "ghost", onclick: showImport, title: "Paste a move list to play out here" }, "Import moves…")
		);
	}
	if (acting && b.manualSteps().length) {
		actions.append(h("button", { id: "undo-step", class: "ghost", onclick: undoStep, title: "Undo last selection (Backspace)" }, "↶ Back"));
		actions.append(h("button", { id: "reset-move", class: "ghost", onclick: resetMove, title: "Clear the move (Esc)" }, "Reset"));
	}
	// Taking your last move back is one of the things to choose from — in the
	// list of actions when there is one, else beside the move controls.
	if (!actionIds.length && !inAnalysis()) {
		const undo = undoButton();
		if (undo) actions.append(undo);
	}
	if (acting && !inAnalysis() && b.complete() && (needsConfirm() || b.options().length)) {
		actions.append(
			h(
				"button",
				{ id: "submit-move", class: "primary", onclick: submitMove, disabled: S.busy, title: "Play this move (Enter)" },
				`Submit ${b.complete().notation}`
			)
		);
	}
	if (actions.childNodes.length) bar.append(actions);
	return bar;
}

/** "Undo my move": offered while there is a move of yours to take back. */
function undoButton() {
	if (inAnalysis() || !isLive() || !S.me.seat || !S.me.name || !canUndo() || S.busy) return null;
	// Stepping off a Gate in place is free and needs no confirming: putting the
	// unit back is the way back, and nothing offers to take it back.
	const last = S.moves[S.moves.length - 1];
	const off = last && /^leave:[^@]+@([^>]+)>(.+)$/.exec(last.notation || "");
	if (off && off[1] === off[2] && last.seat === S.me.seat) return null;
	return h(
		"button",
		{ id: "undo-move", class: "undo-action", onclick: doUndo, title: "Take back your last move" },
		S.confirmUndo ? "Really undo?" : "↶ Undo my move"
	);
}

/**
 * The spellbook pictogram: in colour for a spellbook that is earned (or a
 * requirement that is met), greyed out for one that is not.
 */
function spellMark(on, label, px = 15) {
	const icon = statIcon("books", label || (on ? "Spellbook" : "Spellbook not yet earned"), px);
	icon.classList.add("spell-mark", on ? "on" : "off");
	return icon;
}

/** "How to awaken": the numbered steps of a Great Old One, or the one line of where another unit comes into play. */
function enterBlock(unit) {
	if (unit.awaken && unit.awaken.length) {
		const ol = h("ol", { class: "awaken-steps small" });
		for (const step of unit.awaken) ol.append(h("li", {}, step));
		return h("div", { class: "unit-enter unit-awaken" }, h("p", { class: "small" }, h("strong", {}, "How to awaken")), ol);
	}
	if (unit.enter) return h("p", { class: "unit-enter small" }, h("strong", {}, "Where: "), unit.enter);
	return null;
}

/** The named abilities of a unit's card (a Great Old One's Desecrate, Vengeance, Devour…). */
function abilitiesBlock(unit) {
	if (!unit.abilities || !unit.abilities.length) return null;
	const wrap = h("div", { class: "unit-abilities" });
	for (const a of unit.abilities) {
		wrap.append(
			h("p", { class: "unit-ability small", "data-ability": a.name }, h("strong", {}, a.name), h("span", { class: "muted" }, ` (${a.timing})`), `: ${a.text}`)
		);
	}
	return wrap;
}

/** A unit's own text without the "Awaken for …" sentences the steps above it say again. */
function unitText(unit) {
	let text = unit.text || "";
	if (unit.awaken && unit.awaken.length) {
		text = text
			.split(/\.\s+/)
			.filter((x) => x && !/^Awaken /.test(x))
			.map((x) => (x.endsWith(".") ? x : x + "."))
			.join(" ");
	}
	return text;
}

/** The unit card the page knows for a seat's unit type, or null. */
function unitCardFor(seat, type) {
	const rv = rulesetView();
	const sv = rv && rv.seats ? rv.seats[String(seat)] : null;
	return sv ? (sv.units || []).find((u) => u.id === type) || null : null;
}

const NEUTRAL_GROUPS = [
	["book", "Spellbooks"],
	["monster", "Monsters"],
	["terror", "Terrors"],
	["goo", "Great Old Ones"],
];

/**
 * What is still there to take of the neutral spellbooks and Loyalty Cards: a
 * unit is just its icon, a spellbook the book pictogram and its name; hover for
 * the rules and the price. Null at a table that plays without neutral content.
 */
function neutralCard() {
	const rv = rulesetView();
	const nv = rv && rv.neutral;
	if (!nv) return null;
	const card = h("div", { class: "card neutral-card", id: "neutral-window" }, h("h3", {}, "Neutral items"));
	const items = nv.items || [];
	if (!items.length) {
		card.append(h("p", { class: "muted small", id: "neutral-empty" }, "Everything has been taken."));
		return card;
	}
	for (const [kind, title] of NEUTRAL_GROUPS) {
		const group = items.filter((it) => it.kind === kind);
		if (!group.length) continue;
		const row = h("div", { class: "neutral-items neutral-" + kind, "data-kind": kind });
		for (const item of group) {
			const label = `${item.name}: available for ${item.obtain}`;
			let node;
			if (kind === "book") {
				node = h("span", { class: "neutral-item neutral-book", tabindex: "0", "data-item": item.id, "aria-label": label }, spellMark(true, item.name, 16), h("span", {}, item.name));
			} else {
				const def = S.info.pieces.find((p) => p.type === item.id);
				node = h("span", { class: "neutral-item neutral-unit", tabindex: "0", role: "img", "data-item": item.id, "aria-label": label }, pieceIcon(def ? def.shape : "disc", "black", 32));
			}
			hoverCard(node, () => neutralTip(item));
			row.append(node);
		}
		card.append(h("p", { class: "neutral-title small muted" }, title), row);
	}
	return card;
}

/** The hover card of a neutral item: its rules, the price and, for a unit, its numbers. */
function neutralTip(item) {
	const box = h("div", { class: "unit-tip neutral-tip", "data-item": item.id });
	const def = S.info.pieces.find((p) => p.type === item.id);
	const head = h("div", { class: "unit-tip-head" });
	if (item.kind === "book") head.append(spellMark(true, item.name, 28));
	else head.append(pieceIcon(def ? def.shape : "disc", "black", 34));
	const label = { book: "Neutral spellbook", monster: "Neutral Monster", terror: "Terror", goo: "Independent Great Old One" }[item.kind];
	head.append(h("div", { class: "unit-tip-title" }, h("strong", {}, item.name), h("span", { class: "muted small" }, label)));
	box.append(head);
	const stats = h("dl", { class: "unit-stats" });
	const stat = (name, value) => stats.append(h("div", {}, h("dt", {}, name), h("dd", {}, value)));
	if (item.kind === "book") {
		stat("When", item.timing);
	} else {
		stat(item.kind === "goo" ? "To awaken" : "To take", item.kind === "goo" ? `${item.cost} Power` : item.obtain);
		if (item.kind !== "goo") stat("To summon", `${item.cost} Power`);
		const combat = /^-?\d+$/.test(item.combat) ? (item.combat === "0" ? "none" : `${item.combat} ${item.combat === "1" ? "die" : "dice"}`) : item.combat;
		stat("Combat", combat);
		stat("In the box", String(item.total));
	}
	box.append(stats);
	if (item.kind === "book") {
		box.append(h("p", { class: "unit-text small" }, item.text), h("p", { class: "unit-enter small" }, h("strong", {}, "How: "), item.how));
		return box;
	}
	const abilities = abilitiesBlock(item);
	if (abilities) box.append(abilities);
	const enter = enterBlock(item);
	if (enter) box.append(enter);
	if (item.kind !== "goo") box.append(h("p", { class: "unit-enter small" }, h("strong", {}, "The card: "), item.how));
	if (item.text) box.append(h("p", { class: "unit-text small" }, unitText(item)));
	if (item.book) {
		box.append(
			h("p", { class: "small muted unit-books-title" }, "Spellbook"),
			h(
				"ul",
				{ class: "card-rows unit-books" },
				h(
					"li",
					{ class: "todo" },
					h("span", { class: "row-mark" }, spellMark(false, item.book.name)),
					h("span", { class: "row-text" }, h("strong", {}, item.book.name), h("span", { class: "muted small" }, ` (${item.book.timing}): ${item.book.text}`))
				)
			)
		);
		if (item.req) box.append(h("p", { class: "unit-enter small" }, h("strong", {}, "Requirement: "), item.req));
	}
	return box;
}

/** The hover card of a unit: its icon, numbers, what it can do and the spellbooks that bear on it. */
function unitTip(unit, color, cost, where) {
	const def = S.info.pieces.find((p) => p.type === unit.id);
	const box = h("div", { class: "unit-tip" });
	box.append(
		h(
			"div",
			{ class: "unit-tip-head" },
			pieceIcon(def ? def.shape : "disc", color, 34),
			h("div", { class: "unit-tip-title" }, h("strong", {}, unit.name), h("span", { class: "muted small" }, CLASS_LABEL[unit.class] || unit.class))
		)
	);
	// Which of the cult's units this is: in the pool, or where it stands.
	if (where) box.append(h("p", { class: "unit-where small" }, h("strong", {}, "This one: "), where));
	const combat = /^\d+$/.test(unit.combat) ? (unit.combat === "0" ? "none" : `${unit.combat} ${unit.combat === "1" ? "die" : "dice"}`) : unit.combat;
	const stats = h("dl", { class: "unit-stats" });
	const stat = (label, value) => stats.append(h("div", {}, h("dt", {}, label), h("dd", {}, value)));
	stat(unit.class === "cultist" ? "Recruit" : unit.class === "goo" ? "Awaken" : "Summon", `${cost === undefined || cost === null ? unit.cost : cost} Power`);
	stat("Combat", combat);
	stat("In the pool", `${unit.pool} of ${unit.total}`);
	if (unit.on_map) stat("On the map", String(unit.on_map));
	box.append(stats);
	const abilities = abilitiesBlock(unit);
	if (abilities) box.append(abilities);
	const enter = enterBlock(unit);
	if (enter) box.append(enter);
	// A Great Old One's text restates where it awakens: the steps above say it.
	const text = unitText(unit);
	if (text) box.append(h("p", { class: "unit-text small" }, text));
	if (unit.books && unit.books.length) {
		const ul = h("ul", { class: "card-rows unit-books" });
		for (const b of unit.books) {
			ul.append(
				h(
					"li",
					{ class: b.unlocked ? "done" : "todo" },
					h("span", { class: "row-mark" }, spellMark(b.unlocked, b.unlocked ? `${b.name}: unlocked` : `${b.name}: not unlocked`)),
					h("span", { class: "row-text" }, h("strong", {}, b.name), h("span", { class: "muted small" }, ` (${b.timing}): ${b.text}`))
				)
			);
		}
		box.append(h("p", { class: "small muted unit-books-title" }, "Spellbooks"), ul);
	}
	return box;
}

/** What an action costs just now, as the server worked it out: null when it is free (or unknown). */
function actionCost(id) {
	const rv = rulesetView();
	const c = rv && rv.costs ? rv.costs[id] : null;
	return typeof c === "number" && c > 0 ? c : null;
}

/** The button that summons or awakens one kind of unit: icon, name and the cost it has now. */
function unitButton(kind, type, seat) {
	const unit = unitCardFor(seat, type);
	if (!unit) return null;
	const color = colorOf(seat);
	const piece = S.info.pieces.find((p) => p.type === type);
	const id = `${kind}-${type}`;
	const verb = kind === "awaken" ? "Awaken" : "Summon";
	const cost = actionCost(id) ?? unit.cost;
	const button = h(
		"button",
		{
			id: `action-${id}`,
			class: "summon-button unit-button",
			"data-action": id,
			"data-piece": type,
			"data-cost": cost,
			onclick: () => chooseAction(id),
			"aria-label": `${verb} ${unit.name} for ${cost} Power`,
		},
		pieceIcon(piece ? piece.shape : "disc", color, 22),
		h("span", { class: "summon-name" }, `${verb} ${unit.name}`),
		h("span", { class: "summon-cost", title: "Power" }, `${cost}`, statIcon("power", "Power", 13))
	);
	hoverCard(button, () => unitTip(unit, color, cost));
	return button;
}

/** The hover card of an action: its name, what it costs just now, and the rule in a few lines. */
function actionTip(def, id, label) {
	const box = h("div", { class: "unit-tip action-tip" });
	const cost = actionCost(id);
	// A spellbook's rule reads "Timing: what it does".
	let timing = "";
	let text = def.hint || "";
	if (id.startsWith("book-")) {
		const i = text.indexOf("\n");
		if (i > 0) {
			timing = text.slice(0, i);
			text = text.slice(i + 1);
		}
	}
	box.append(
		h(
			"div",
			{ class: "unit-tip-head" },
			h(
				"div",
				{ class: "unit-tip-title" },
				h("strong", {}, label || def.label),
				timing ? h("span", { class: "muted small" }, timing) : cost !== null ? h("span", { class: "muted small" }, `${cost} Power`) : null
			)
		)
	);
	if (cost !== null && timing) box.append(h("p", { class: "action-cost small" }, h("strong", {}, "Cost: "), `${cost} Power`));
	if (text) box.append(h("p", { class: "unit-text small" }, text));
	if (def.source === "book") {
		box.append(h("p", { class: "action-source small muted" }, spellMark(true, "Unlocked spellbook", 14), " An ability of a spellbook you have unlocked."));
	} else if (def.source === "requirement") {
		box.append(h("p", { class: "action-source small muted" }, spellMark(false, "Spellbook requirement", 14), " Meets a spellbook requirement: it earns a spellbook of your choice."));
	}
	return box;
}

/** A button of the action list: its name with the Power it costs just now, and a card with the rule on hover. */
function actionButton(id, def, label, rv) {
	const cost = actionCost(id);
	// "Pay 4 Power" and "Give 3 Power to …" say what they cost already.
	const named = /^(pay|bestow)-/.test(id);
	const text = cost !== null && !named ? `${label} (${cost})` : label;
	// An ability a spellbook gives wears a spellbook; an action that meets a
	// spellbook requirement wears one greyed out (the book is still to come).
	const source = def && def.source ? def.source : "";
	const mark =
		source === "book"
			? spellMark(true, "Unlocked spellbook")
			: source === "requirement"
			? spellMark(false, "Spellbook requirement")
			: null;
	const button = h(
		"button",
		{
			id: `action-${id}`,
			class: source ? `has-spell spell-${source}` : null,
			"data-action": id,
			"data-cost": cost === null ? null : cost,
			"data-source": source || null,
			onclick: () => chooseAction(id),
		},
		mark,
		text
	);
	if (def && def.hint) hoverCard(button, () => actionTip(def, id, label));
	return button;
}

/** One of the two directions of play, spelled out as who plays after whom. */
function directionButton(id, rv) {
	const cw = id === "cw";
	return h(
		"button",
		{
			id: `action-${id}`,
			class: "direction-button",
			"data-action": id,
			title: cw ? "Play passes down the list of cults, wrapping from the last to the first." : "Play passes up the list of cults, wrapping from the first to the last.",
			onclick: () => chooseAction(id),
		},
		h("span", { class: "dir-head" }, h("span", { class: "dir-glyph", "aria-hidden": "true" }, cw ? "↻" : "↺"), h("strong", {}, cw ? "Clockwise" : "Counter-clockwise"), h("span", { class: "muted small" }, cw ? " — down the list" : " — up the list")),
		orderLine(cw ? rv.order.cw : rv.order.ccw)
	);
}

// carryDisplay draws the picked-up pieces in drop order (bottom of the
// carried group first), dimming those already dropped.
function carryDisplay(b, carried, dropped) {
	const origin = b.steps[0] && b.steps[0].kind === "pos" ? b.steps[0].value : null;
	const wrap = h("span", { class: "carry", id: "carry", title: `carrying ${carried}, ${carried - dropped} left to drop` });
	const stack = (origin && currentCells()[origin]) || [];
	const group = stack.slice(Math.max(0, stack.length - carried));
	group.forEach((pc, i) => {
		const def = S.info.pieces.find((p) => p.type === pc.piece);
		const icon = pieceIcon(def ? def.shape : "disc", pc.color, 22);
		const holder = h("span", { class: "carry-piece" + (i < dropped ? " dropped" : "") }, icon);
		wrap.append(holder);
	});
	wrap.append(h("span", { class: "small muted" }, `${carried - dropped} to drop`));
	return wrap;
}

function describeSteps(steps) {
	const parts = [];
	let dropRun = [];
	const flush = () => {
		if (dropRun.length) {
			parts.push("→ " + dropRun.join(", "));
			dropRun = [];
		}
	};
	let seenCount = false;
	for (const st of steps) {
		if (st.kind === "piece") {
			parts.push(pieceLook(st.value).label);
		} else if (st.kind === "action") {
			const def = actionDef(st.value);
			parts.push(def ? def.label : st.value);
		} else if (st.kind === "count") {
			if (isRuleset()) {
				parts.push(st.value);
				continue;
			}
			seenCount = true;
			parts.push(`×${st.value}`);
		} else if (seenCount) {
			dropRun.push(st.value);
		} else {
			parts.push(st.value);
		}
	}
	flush();
	// A placement reads "Flat stone → c3".
	if (steps.length === 2 && steps[0].kind === "piece" && steps[1].kind === "pos") return `${parts[0]} → ${parts[1]}`;
	return parts.join(" ");
}

function identityGate() {
	// Seats that have a name we could reconnect as, and any open seat.
	const seats = S.game.seats || [];
	const open = seats.find((s) => !s.name);
	const card = h("div", { class: "card", id: "identity-gate" });
	card.append(h("h2", {}, "Join this game"));

	if (open && S.game.status === "waiting") {
		const input = h("input", { type: "text", id: "gate-name", placeholder: "Your name", value: lastName(), maxlength: 32, autocomplete: "nickname" });
		const join = async () => {
			const name = input.value.trim();
			if (!name) {
				input.focus();
				return;
			}
			try {
				await api.join(code, name, "player");
				rememberGame(code, S.game.game_def_id, name);
				await refreshGame();
				restartSocket();
				render();
			} catch (err) {
				toast(err.message, "error");
			}
		};
		input.addEventListener("keydown", (ev) => {
			if (ev.key === "Enter") join();
		});
		card.append(
			h("div", { class: "row" }, input, h("button", { class: "primary", id: "gate-join", onclick: join }, `Play as ${seatLabel(open.seat)}`)),
			h("p", { class: "small muted" }, "Or just watch below.")
		);
	}

	const named = seats.filter((s) => s.name && !s.bot);
	if (named.length) {
		card.append(h("p", { class: "small muted" }, "Already playing? Continue as:"));
		const row = h("div", { class: "row" });
		for (const s of named) {
			row.append(
				h(
					"button",
					{
						"data-seat": s.seat,
						onclick: async () => {
							rememberGame(code, S.game.game_def_id, s.name);
							resolveIdentity();
							await loadPosition();
							restartSocket();
							render();
						},
					},
					`${s.name} (${seatLabel(s.seat)})`
				)
			);
		}
		card.append(row);
	}
	card.append(
		h(
			"div",
			{ class: "row", style: "margin-top:0.6rem" },
			h(
				"button",
				{
					class: "ghost",
					id: "gate-watch",
					onclick: () => {
						S.spectating = true;
						store.set("bingle.watch." + code, true);
						render();
					},
				},
				"Just watch"
			)
		)
	);
	return card;
}

function restartSocket() {
	const old = ws;
	ws = null; // makes the old socket's handlers inert
	if (old) {
		try {
			old.close();
		} catch (_) {
			/* already closed */
		}
	}
	wsRetry = 0;
	connect();
}

/** Lets a seated player fill an open seat of a waiting table with a bot. */
function addBotControl() {
	if (S.game.status !== "waiting" || !S.me.seat || !S.me.name) return null;
	if (!(S.game.seats || []).some((x) => !x.name)) return null;
	if (S.info.seats.length < 2) return null;
	if (!S.bots) {
		api.bots()
			.then((list) => {
				S.bots = list;
				render();
			})
			.catch(() => {
				S.bots = [];
			});
		return null;
	}
	if (!S.bots.length) return null;
	const select = h("select", { id: "add-bot-select", "aria-label": "Bot to add" });
	for (const b of S.bots) select.append(h("option", { value: b.id, title: b.description || null }, b.label));
	const row = h(
		"div",
		{ class: "row add-bot", style: "margin-top:0.6rem" },
		select,
		h(
			"button",
			{
				id: "add-bot",
				onclick: async () => {
					try {
						await api.addBot(code, S.me.name, select.value);
						await refreshGame();
						render();
					} catch (err) {
						toast(err.message, "error");
					}
				},
			},
			"Add bot to an open seat"
		)
	);
	return row;
}

function sidePanel(top, below) {
	const side = h("aside", { class: "side-panel" });
	if (top) side.append(top);
	if (top) side.append(neutralCard() || "");
	// `below`, when given, takes the move list and the log instead.
	const place = (card) => (below ? below.push(card) : side.append(card));

	// A game that has not started: the table talk, nothing else (the rules and
	// the options are in the bar at the top of the page).
	if (inLobby()) {
		side.append(voiceCard({ code, me: S.me, seats: S.game.seats }) || "");
		side.append(chatCard({ code, me: S.me }));
		chatSeen();
		return side;
	}

	// Voice and table talk.
	side.append(voiceCard({ code, me: S.me, seats: S.game.seats }) || "");
	side.append(chatCard({ code, me: S.me }));
	chatSeen();

	// The moves played and, for games that tell it, what happened.
	place(historyCard());
	return side;
}

/** The moves played, with the history controls (and the log, in its own tab). */
function movesPane() {
	const pane = h("div", { class: "history-pane", id: "moves-pane" });
	pane.append(moveList());
	const total = S.moves.length;
	const cur = isLive() ? total : S.viewSeq;
	pane.append(
		h(
			"div",
			{ class: "history-controls" },
			h("button", { class: "icon", id: "hist-first", title: "First position (Home)", disabled: cur === 0, onclick: () => viewMove(0) }, "⏮"),
			h("button", { class: "icon", id: "hist-prev", title: "Previous move (←)", disabled: cur === 0, onclick: () => stepHistory(-1) }, "◀"),
			h("button", { class: "icon", id: "hist-next", title: "Next move (→)", disabled: cur >= total, onclick: () => stepHistory(1) }, "▶"),
			h("button", { class: "icon", id: "hist-last", title: "Latest position (End)", disabled: cur >= total, onclick: () => viewMove(null) }, "⏭")
		),
		h(
			"button",
			{ class: "ghost", id: "hist-play", style: "margin-top:0.4rem", disabled: total === 0, onclick: toggleAutoplay, title: "Step through the whole game" },
			S.autoplay ? "⏸ Pause replay" : "↻ Replay game"
		)
	);
	return pane;
}

/**
 * One card for the moves and for what happened: two tabs, "Moves" and
 * "History" (the second only for games that narrate their turns).
 */
function historyCard() {
	const log = eventsPane();
	const card = h("div", { class: "card side-moves", id: "history-card" });
	if (!log) {
		card.append(h("h3", {}, "Moves"), movesPane());
		return card;
	}
	// What happened is what a player wants to see first; the move list is a tab away.
	let tab = (S.historyTab || store.get("bingle.historyTab", "history")) === "moves" ? "moves" : "history";
	const tabs = h("div", { class: "card-tabs", role: "tablist", "aria-label": "Moves and history" });
	for (const [id, label] of [["moves", "Moves"], ["history", "History"]]) {
		tabs.append(
			h(
				"button",
				{
					class: "card-tab" + (tab === id ? " active" : ""),
					id: `tab-${id}`,
					role: "tab",
					"aria-selected": tab === id ? "true" : "false",
					"aria-controls": `${id}-pane`,
					onclick: () => {
						S.historyTab = id;
						store.set("bingle.historyTab", id);
						render();
					},
				},
				label
			)
		);
	}
	card.append(tabs);
	card.append(tab === "history" ? log.node : movesPane());
	return card;
}

// ---- the game options window ---------------------------------------------------

let optionsWin = null;

/**
 * The buttons of the bar at the top of the page — Rules and Options — and the
 * window the second one opens. Built once: both live outside the area that is
 * redrawn on every move.
 */
function initOptions() {
	const host = document.getElementById("topbar-actions");
	if (!host || optionsWin) return;
	const rules = h(
		"button",
		{ id: "show-rules", class: "topbar-icon", onclick: () => S.info && openRules(S.info), title: "Rules — read the rules of this game", "aria-label": "Rules" },
		rulesIcon()
	);
	const opts = h(
		"button",
		{ id: "open-options", class: "topbar-button", "aria-haspopup": "dialog", "aria-expanded": "false", title: "Settings, tools and the link to this game" },
		h("span", { "aria-hidden": "true" }, "⚙ "),
		"Options"
	);
	host.append(rules, opts);
	optionsWin = createOptionsWindow({
		title: "Game options",
		onToggle: (open) => opts.setAttribute("aria-expanded", open ? "true" : "false"),
	});
	opts.addEventListener("click", () => optionsWin.toggle());
	for (const [id, heading] of [
		["reactions", "Reactions"],
		["autoplay", "Auto-Play Timeout"],
		["notify", "Email notifications"],
		["layers", "Map layers"],
		["display", "Display"],
		["tools", "Tools"],
		["share", "Share this game"],
		["table", "Table"],
		["game", "Game"],
	]) {
		optionsWin.section(id, heading);
	}
}

/** Yellow Sign is one of the cults at this table (the glyph badges are its Desecrate targets). */
function yellowSignAtTable() {
	return ((S.info && S.info.seats) || []).some((st) => st.color === "yellow");
}

/** Whether the glyph badges are drawn: the player's choice, else only when Yellow Sign plays. */
function glyphLayerOn() {
	return S.settings.glyphs === null ? yellowSignAtTable() : S.settings.glyphs;
}

/** A labelled check box that keeps a setting and redraws the page. */
function settingToggle(key, id, label, storeKey, redraw = true) {
	const box = h("input", { type: "checkbox", id, checked: key === "glyphs" ? glyphLayerOn() : S.settings[key] });
	box.addEventListener("change", () => {
		S.settings[key] = box.checked;
		store.set(storeKey, box.checked);
		if (redraw) render();
	});
	return h("label", { class: "check" }, box, h("span", {}, label));
}

/** Brings every section of the options window up to date (cheaply: a section is rebuilt only when its signature changes). */
function refreshOptions() {
	if (!optionsWin || !S.info || !S.game) return;
	const w = optionsWin;
	const areas = S.info.board.style === "areas";
	const lobby = inLobby();

	// Reactions: the question's "stop being asked" lives here, in place.
	updateReactionDock();
	updateAutoPlay();
	updateNotify();

	// The layers of an area map.
	w.hide("layers", !areas);
	if (areas) {
		w.update("layers", ["v2", glyphLayerOn()], (el) => {
			el.append(
				settingToggle("names", "opt-layer-names", "Area names", "bingle.layer.names"),
				settingToggle("glyphs", "opt-layer-glyphs", "Glyphs", "bingle.layer.glyphs"),
				settingToggle("sigils", "opt-layer-sigils", "Sigils", "bingle.layer.sigils")
			);
		});
	}

	// Display.
	w.update("display", [isRuleset(), S.settings.confirm, boardTheme(), S.settings.tileScale, areas, layoutChoice()], (el) => {
		if (areas) {
			// The phone layout is picked by screen by default; a player may force either.
			const layoutSel = h("select", { id: "opt-layout", "aria-label": "Page layout" });
			for (const [value, label] of LAYOUTS) layoutSel.append(h("option", { value, selected: layoutChoice() === value }, label));
			layoutSel.addEventListener("change", () => {
				setLayoutChoice(layoutSel.value);
				render();
			});
			el.append(h("label", { class: "field" }, h("span", { class: "small" }, "Page layout"), layoutSel));
		}
		if (!isRuleset()) {
			el.append(settingToggle("confirm", "opt-confirm", "Preview and confirm moves before playing them", "bingle.confirm"));
		}
		if (!areas) {
			el.append(settingToggle("coords", "opt-coords", "Show coordinates", "bingle.coords"));
		}
		el.append(
			settingToggle("hints", "opt-hints", "Highlight legal squares", "bingle.hints"),
			settingToggle("lastMove", "opt-lastmove", "Highlight the last move", "bingle.lastMove"),
			settingToggle("animate", "opt-animate", "Animate moves", "bingle.animate")
		);
		const themeSel = h("select", { id: "opt-board", "aria-label": "Board theme" });
		const chosen = boardTheme();
		for (const [value, label] of [["wood", "Wood"], ["slate", "Slate"], ["green", "Green felt"], ["paper", "Paper"]]) {
			themeSel.append(h("option", { value, selected: chosen === value }, label));
		}
		// Games played with stones also offer the illustrated themes.
		const fancy = themesFor(S.info);
		if (fancy.length) {
			const group = h("optgroup", { label: "Illustrated themes" });
			for (const t of fancy) group.append(h("option", { value: t.id, selected: chosen === t.id }, t.label));
			themeSel.append(group);
		}
		themeSel.addEventListener("change", () => {
			S.settings.board = themeSel.value;
			store.set("bingle.board", themeSel.value);
			applyTheme();
			// The illustrated themes draw into the board: redraw it.
			render();
		});
		el.append(h("label", { class: "field" }, h("span", { class: "small" }, "Board theme"), themeSel));
		if ((S.info.pieces || []).some((p) => p.rules)) {
			const sizeSel = h("select", { id: "opt-tile-scale", "aria-label": "Piece button size" });
			for (const [value, label] of [[0.5, "Small"], [0.75, "Medium"], [1, "Large"], [1.25, "Larger"], [1.5, "Largest"]]) {
				sizeSel.append(h("option", { value, selected: S.settings.tileScale === value }, label));
			}
			sizeSel.addEventListener("change", () => {
				S.settings.tileScale = Number(sizeSel.value);
				store.set("bingle.tileScale", S.settings.tileScale);
				applyTheme();
			});
			el.append(h("label", { class: "field" }, h("span", { class: "small" }, "Piece button size"), sizeSel));
		}
		if ("Notification" in window) {
			const notify = h("input", { type: "checkbox", id: "opt-notify", checked: S.settings.notify && Notification.permission === "granted" });
			notify.addEventListener("change", async () => {
				if (notify.checked) {
					const perm = await Notification.requestPermission();
					S.settings.notify = perm === "granted";
					if (perm !== "granted") {
						notify.checked = false;
						toast("Notifications are blocked in this browser.", "error");
					}
				} else {
					S.settings.notify = false;
				}
				store.set("bingle.notify", S.settings.notify);
			});
			el.append(h("label", { class: "check" }, notify, h("span", {}, "Notify me when it’s my turn")));
		}
	});

	// Tools.
	w.hide("tools", lobby);
	w.update("tools", [inAnalysis(), isLive(), S.game.status, canHint(), S.hintBusy, S.info.notation, areas], (el) => {
		const row = h("div", { class: "row" });
		row.append(
			h(
				"button",
				{
					id: "flip-board",
					onclick: () => {
						S.flipped = !S.flipped;
						S.flippedByUser = true;
						render();
					},
				},
				"⇅ Flip board"
			),
			h(
				"button",
				{
					id: "analyze",
					disabled: inAnalysis() || (isLive() && S.game.status !== "active"),
					onclick: startAnalysis,
					title: "Try out moves for both sides without playing them",
				},
				"Analyze"
			),
			...(canHint()
				? [h("button", { id: "hint", disabled: S.hintBusy, onclick: askHint, title: "Ask a bot what it would play" }, S.hintBusy ? "Thinking…" : "Hint")]
				: []),
			h("button", { id: "save-image", onclick: saveBoardImage, title: "Download this position as an SVG image" }, "Image"),
			...(S.info.notation === "ptn"
				? [h("button", { id: "copy-tps", onclick: copyTPS, title: "Copy this position as TPS (for other Tak tools)" }, "TPS")]
				: []),
			h("a", { class: "button", id: "export-json", href: `/api/games/${encodeURIComponent(code)}/export`, target: "_blank", rel: "noopener" }, "Export"),
			...(S.info.notation === "ptn"
				? [h("a", { class: "button", id: "export-ptn", href: `/api/games/${encodeURIComponent(code)}/export?format=ptn`, download: `${code}.ptn` }, "PTN")]
				: [])
		);
		el.append(row);
	});

	// The invite link.
	w.update("share", S.game.invite_url || "", (el) => {
		const link = S.game.invite_url || location.origin + "/game/" + code;
		const linkInput = h("input", { type: "text", readonly: true, value: link, id: "share-link", "aria-label": "Game link" });
		el.append(
			h("p", { class: "small muted" }, `Room ${code}`),
			h(
				"div",
				{ class: "share-row" },
				linkInput,
				h(
					"button",
					{
						id: "copy-link",
						onclick: async () => {
							try {
								await navigator.clipboard.writeText(link);
								toast("Link copied");
							} catch (_) {
								linkInput.select();
								document.execCommand && document.execCommand("copy");
								toast("Link selected — copy it");
							}
						},
					},
					"Copy"
				)
			)
		);
	});

	// A waiting table: fill an open seat with a bot; a spectator may take one.
	const open = (S.game.seats || []).some((x) => !x.name);
	const botRow = addBotControlReady();
	const wantsSeat = S.spectating && S.game.status === "waiting" && open;
	w.hide("table", !(botRow || wantsSeat));
	w.update("table", [S.game.status, S.me.seat, !!S.me.name, open, (S.bots || []).length, wantsSeat], (el) => {
		const row = addBotControl();
		if (row) el.append(row);
		if (wantsSeat) {
			el.append(
				h(
					"button",
					{
						id: "spectator-join",
						class: "primary",
						onclick: () => {
							S.spectating = false;
							store.set("bingle.watch." + code, false);
							render();
						},
					},
					"Join this game"
				)
			);
		}
	});

	// Taking a move back, a draw, resigning.
	const twoParty = S.info.seats.length === 2;
	const seated = !!(S.me.seat && S.me.name);
	w.hide("game", !seated);
	w.update(
		"game",
		[seated, twoParty, isActive(), S.busy, S.confirmResign, canAskTakeback(), S.game.draw_offer_seat, (S.game.seats || []).map((x) => !!x.bot)],
		(el) => {
			if (!seated) return;
			const row = h("div", { class: "row" });
			if (twoParty && canAskTakeback()) {
				row.append(
					h(
						"button",
						{
							id: "request-takeback",
							disabled: S.busy,
							title: "Ask your opponent to let you take back your last move",
							onclick: () => doTakeback("request"),
						},
						"↶ Ask to take back"
					)
				);
			}
			if (isActive()) {
				const opponentIsBot = (S.game.seats || []).some((x) => x.bot && x.seat !== S.me.seat);
				const offered = S.game.draw_offer_seat !== null && S.game.draw_offer_seat !== undefined;
				if (twoParty) {
					row.append(
						h(
							"button",
							{
								id: "offer-draw",
								disabled: S.busy || offered || opponentIsBot,
								title: opponentIsBot ? "Bots do not answer draw offers" : "Propose a draw to your opponent",
								onclick: () => doDraw("offer"),
							},
							"½ Offer draw"
						)
					);
				}
				const leave = twoParty ? "Resign" : "End game";
				row.append(
					h(
						"button",
						{
							id: "resign",
							class: "danger",
							disabled: S.busy,
							title: twoParty ? null : "Ends the game for every player, with no winner",
							onclick: doResign,
						},
						S.confirmResign ? `Really ${leave.toLowerCase()}?` : leave
					)
				);
			}
			if (row.childNodes.length) el.append(row);
			const age = lastMoveAgeText();
			if (S.moves.length) el.append(h("p", { class: "small muted", style: "margin-top:0.5rem" }, "Last move ", h("span", { id: "last-move-age" }, age)));
		}
	);
}

/** Whether a bot could be added to an open seat now (the bots are fetched on the way). */
function addBotControlReady() {
	if (S.game.status !== "waiting" || !S.me.seat || !S.me.name) return false;
	if (!(S.game.seats || []).some((x) => !x.name)) return false;
	if (S.info.seats.length < 2) return false;
	if (!S.bots) {
		api.bots()
			.then((list) => {
				S.bots = list;
				render();
			})
			.catch(() => {
				S.bots = [];
			});
		return false;
	}
	return S.bots.length > 0;
}

// exchangePanel shows the contest being resolved (or the last one): both
// sides' dice, what each needs to succeed, and — when it is your turn to
// reroll — the failed dice you may pay Momentum to roll again.
function exchangePanel() {
	const v = rulesetView();
	if (!v || !v.contest) return null;
	const c = v.contest;
	const mine = actingSeat();
	const rerolling = !c.resolved && v.stage === "reroll" && mine && v.to_move === mine;
	const kind = { strike: "Strike", shoot: "Shot", shove: "Shove" }[c.kind] || c.kind;
	const [a, t] = c.sides;
	const card = h("div", { class: "exchange", id: "exchange", "data-kind": c.kind, "data-resolved": c.resolved ? "1" : "0" });
	card.append(h("h3", {}, `${kind}: ${a.unit} → ${t.unit}`, c.resolved ? h("span", { class: "muted small" }, " (resolved)") : null));

	const dice = (list, need, offset, side) =>
		(list || []).map((d, i) => {
			const k = offset + i + 1;
			const title = `needs ${need}+${d.ok ? "" : " — failed"}`;
			if (rerolling && side.seat === mine && !d.ok && !side.rerolled) {
				return h("button", { class: "die fail rerollable", "data-die": k, title: "Spend 1 Momentum to reroll this die", onclick: () => rerollDie(k) }, String(d.v));
			}
			return h("span", { class: "die " + (d.ok ? "ok" : "fail"), title }, String(d.v));
		});
	const unknown = (n) => (n === null || n === undefined || n < 0 ? [h("span", { class: "die unknown", title: "committed in secret" }, "?")] : []);

	for (const side of c.sides) {
		const row = h("div", { class: "side", "data-seat": side.seat });
		row.append(h("div", { class: "who" }, side.unit));
		const pool = h("div", { class: "pool" });
		if (c.rolled && (side.atk_dice || side.def_dice)) {
			if (side.atk_dice && side.atk_dice.length) {
				pool.append(
					h(
						"span",
						{ class: "pool-part", "data-pool": "attack" },
						h("span", { class: "pool-label" }, `attack ${side.atk_need}+`),
						...dice(side.atk_dice, side.atk_need, 0, side),
						h("strong", {}, `= ${side.atk_successes}`)
					)
				);
			}
			if (side.def_dice && side.def_dice.length) {
				pool.append(
					h(
						"span",
						{ class: "pool-part", "data-pool": "defense" },
						h("span", { class: "pool-label" }, `defense ${side.def_need}+`),
						...dice(side.def_dice, side.def_need, (side.atk_dice || []).length, side),
						h("strong", {}, `= ${side.def_successes}`)
					)
				);
			}
			if (side.rerolled) pool.append(h("span", { class: "small muted" }, "rerolled"));
		} else if (side.attack >= 0) {
			pool.append(h("span", {}, `${side.attack} on attack${side.defense >= 0 ? `, ${side.defense} on defense` : ""}`));
		} else if (side.committed) {
			pool.append(h("span", { class: "muted" }, "committed in secret"), ...unknown(-1));
		} else {
			pool.append(h("span", { class: "muted" }, "has not answered yet"));
		}
		row.append(pool);
		card.append(row);
	}
	return card;
}

function rerollDie(k) {
	chooseAction("reroll");
	if (activeBuilder().nextCounts().includes(k)) chooseCount(k);
}

// ---- the log: what happened since your last move --------------------------------

/** What the latest decisions did, newest last: [{ply, seat, lines}]. */
function rulesetLog() {
	if (!isRuleset()) return [];
	const st = currentState();
	const latest = (st && st.ext && st.ext.log) || [];
	// A position keeps only its latest events: the whole story is fetched from
	// the server (what every move narrated), once per move made. Until the
	// fetch for the newest move arrives, the story so far gets the position's
	// own latest events added.
	if (!S.fullLog || S.fullLog.count !== S.moves.length) loadFullLog();
	if (S.fullLog) {
		const upTo = isLive() ? Infinity : S.viewSeq;
		const seen = S.fullLog.entries.length ? S.fullLog.entries[S.fullLog.entries.length - 1].ply : 0;
		const all = S.fullLog.entries.concat(latest.filter((e) => e.ply > seen)).filter((e) => e.ply <= upTo);
		if (all.length) return all;
	}
	return latest;
}

async function loadFullLog() {
	const count = S.moves.length;
	if (S.fullLogLoading === count) return;
	S.fullLogLoading = count;
	let entries = [];
	try {
		entries = await api.history(code);
	} catch (_) {
		// The latest events are shown instead.
	}
	S.fullLogLoading = undefined;
	S.fullLog = { count, entries: entries || [] };
	if (S.moves.length === count) render();
}

const LOG_VERBS = [
	"takes control",
	"takes a Pain",
	"takes",
	"has no legal action",
	"is raised",
	"is the first player",
	"is killed",
	"chooses",
	"recruits",
	"summons",
	"awakens",
	"creates",
	"builds",
	"steps",
	"puts",
	"moves",
	"attacks",
	"captures",
	"abandons",
	"offers",
	"passes",
	"pays",
	"performs",
	"places",
	"receives",
	"reveals",
	"rolls",
	"gathers",
	"gains",
	"advances",
	"earns",
	"eliminates",
	"fulfils",
	"submerges",
	"retreats",
	"wins",
];
const LOG_VERB_RE = new RegExp("\\b(" + LOG_VERBS.join("|") + ")\\b");

function escapeRegExp(text) {
	return text.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

/**
 * One line of the log: cult names in their colours and the verb in bold (or
 * the "Spellbook:" label that opens an effect).
 */
function logLine(text) {
	const banner = /^—\s*(.+?)\s*—$/.exec(text);
	if (banner) return h("li", { class: "log-banner" }, banner[1]);
	const marks = [];
	const names = S.info.seats.map((sd) => escapeRegExp(sd.label)).join("|");
	if (names) {
		const re = new RegExp(names, "g");
		for (let m = re.exec(text); m; m = re.exec(text)) {
			const sd = S.info.seats.find((x) => x.label === m[0]);
			marks.push({ start: m.index, end: m.index + m[0].length, node: () => h("span", { class: `log-fac color-${cssColor(sd.color)}` }, m[0]) });
		}
	}
	const label = /^([A-Z][A-Za-z'’ -]{1,30}):(?=\s)/.exec(text);
	const verb = label || LOG_VERB_RE.exec(text);
	if (verb) {
		const start = label ? 0 : verb.index;
		const end = start + verb[0].length;
		if (!marks.some((x) => x.start < end && x.end > start)) marks.push({ start, end, node: () => h("strong", { class: "log-verb" }, text.slice(start, end)) });
	}
	marks.sort((a, b) => a.start - b.start);
	const li = h("li", {});
	let at = 0;
	for (const m of marks) {
		if (m.start < at) continue;
		if (m.start > at) li.append(text.slice(at, m.start));
		li.append(m.node());
		at = m.end;
	}
	if (at < text.length) li.append(text.slice(at));
	return li;
}

// eventsPane narrates the whole game so far, with a rule marking where the
// viewer's last decision was (what the other cults did meanwhile comes after
// it). A spectator sees the whole log without the mark. It returns
// {node, since} for the History tab, or null for a game that does not narrate
// its turns. A game that does always has the tab: when there is nothing to
// tell (the start of the game, a stretch of silent decisions such as Done), the
// tab says so instead of vanishing and leaving only Moves.
function eventsPane() {
	if (!isRuleset()) return null;
	const log = rulesetLog();
	let entries = [];
	let since = "";
	let mark = null; // the first entry after the viewer's last move
	let markSeen = false; // the viewer has moved before, so there is something to mark
	let recent = []; // the entries from the mark on
	if (log.length) {
		entries = log.filter((e) => e.lines && e.lines.length);
		if (S.me.seat) {
			// From the player's last decision that someone else's followed
			// (so a turn in progress keeps the others' summary on view and
			// adds the player's own decisions to it).
			let from = 0;
			let found = false;
			let seenOther = false;
			for (let i = log.length - 1; i >= 0; i--) {
				if (log[i].seat !== S.me.seat) seenOther = true;
				else if (seenOther) {
					from = i;
					found = true;
					break;
				}
			}
			// The whole of that turn, not just its last decision (a Done
			// says nothing: what the turn did is in the decisions before it).
			while (found && from > 0 && log[from - 1].seat === S.me.seat) from--;
			since = found ? "since your last move" : "so far";
			recent = log.slice(from).filter((e) => e.lines && e.lines.length);
			if (found) {
				markSeen = true;
				mark = recent[0] || null;
			}
		} else {
			since = "full history";
		}
	} else {
		const events = rulesetEvents();
		if (events.length) entries = [{ seat: 0, lines: events }];
	}
	const card = h("div", { class: "history-pane", id: "history-pane" });
	const box = h("div", { class: "events", id: "events" });
	if (!entries.length) {
		box.append(h("p", { class: "muted small empty-history", id: "history-empty" }, "Nothing to tell yet. What the other players do between your moves is narrated here."));
	}
	// Only the viewer's own turn is told after the mark: nobody else has said
	// anything since.
	const quiet = markSeen && recent.every((e) => e.seat === S.me.seat);
	const sinceRule = () => h("div", { class: "log-since", id: "history-since", role: "separator" }, h("span", {}, "Since your last move"));
	// Consecutive decisions of one seat read as one turn, except that the mark
	// always starts a new one. Decisions a tactical bot took for a person whose
	// Auto-Play Timeout ran out are marked.
	const groups = [];
	for (const e of entries) {
		const last = groups[groups.length - 1];
		if (last && last.seat === e.seat && e !== mark) last.entries.push(e);
		else groups.push({ seat: e.seat, entries: [e], mark: e === mark });
	}
	const botPlayed = (e) => {
		const mv = e.ply ? S.moves.find((m) => m.seq === e.ply) : null;
		return !!(mv && mv.auto_play);
	};
	const badge = (all) =>
		h("span", { class: "badge auto-play", title: "A tactical bot played this for the player: their Auto-Play Timeout ran out" }, all ? "auto-play" : "partly auto-play");
	for (const g of groups) {
		if (g.mark) {
			box.append(sinceRule());
			if (quiet) box.append(h("p", { class: "muted small empty-history", id: "history-quiet" }, "Nothing new from the others since your last move."));
		}
		const all = g.entries.every(botPlayed);
		const some = g.entries.some(botPlayed);
		const turnClass = "log-turn" + (some ? " has-auto-play" : "");
		let group = h("ul", { class: turnClass });
		if (g.seat) {
			const sd = seatDef(g.seat);
			const who = seatName(g.seat);
			box.append(
				h(
					"div",
					{ class: `log-actor color-${cssColor(colorOf(g.seat))}`, "data-seat": g.seat, "data-auto-play": some ? "1" : null },
					h("span", { class: `swatch color-${cssColor(colorOf(g.seat))}` }),
					h("strong", {}, sd ? sd.label : `Seat ${g.seat}`),
					who ? h("span", { class: "muted small" }, g.seat === S.me.seat ? ` · ${who} (you)` : ` · ${who}`) : null,
					some ? badge(all) : null
				)
			);
		}
		box.append(group);
		for (const e of g.entries) {
			const auto = botPlayed(e);
			e.lines.forEach((line, i) => {
				// A new round starts a new section of the log.
				const round = /^—\s*(Round\s+\d+)\s*—$/.exec(line);
				if (round) {
					box.append(h("div", { class: "log-round", role: "separator" }, h("span", {}, round[1])));
					group = h("ul", { class: turnClass });
					box.append(group);
					return;
				}
				const li = logLine(line);
				if (auto) {
					li.classList.add("auto-play");
					// The turn's header says it when the whole turn was the bot's.
					if (i === 0 && !all) li.append(" ", h("span", { class: "badge auto-play small" }, "auto-play"));
				}
				group.append(li);
			});
		}
	}
	if (markSeen && !mark) {
		box.append(sinceRule(), h("p", { class: "muted small empty-history", id: "history-quiet" }, "Nothing new since your last move."));
	}
	card.append(box);
	// Open on what is new: the rule marking the viewer's last move at the top,
	// or the end of the log when there is no rule.
	queueMicrotask(() => {
		const el = document.getElementById("events");
		if (!el) return;
		const rule = document.getElementById("history-since");
		el.scrollTop = rule ? Math.max(0, rule.getBoundingClientRect().top - el.getBoundingClientRect().top + el.scrollTop - 6) : el.scrollHeight;
	});
	return { node: card, since };
}

function moveList() {
	const n = S.info.seats.length;
	const list = h("ol", { class: "move-list" + (isRuleset() ? " flat" : ""), id: "move-list" });
	if (!S.moves.length) {
		list.append(h("li", { class: "empty-row" }, h("span", { class: "empty" }, "No moves yet.")));
		return list;
	}
	const cur = isLive() ? S.moves.length : S.viewSeq;
	if (isRuleset()) {
		// A turn is several decisions, often by the same seat, so the list is
		// flat: one entry per decision, tinted by who made it.
		for (const m of S.moves) {
			list.append(
				h(
					"li",
					{ class: `by-seat-${m.seat}` },
					h("span", { class: "num" }, `${m.seq}.`),
					h("span", { class: `seat-dot color-${cssColor(colorOf(m.seat))}`, title: seatName(m.seat) || seatLabel(m.seat) }),
					h(
						"button",
						{
							class: "mv" + (m.seq === cur ? " current" : ""),
							"data-seq": m.seq,
							title: `Move ${m.seq} by ${seatName(m.seat) || seatLabel(m.seat)}${m.auto ? " — passed automatically (auto-pass)" : ""}${m.auto_play ? " — played by a tactical bot: the Auto-Play Timeout ran out" : ""}`,
							onclick: () => viewMove(m.seq >= S.moves.length ? null : m.seq),
						},
						m.notation + (m.auto ? " (auto)" : "") + (m.auto_play ? " (auto-play)" : "")
					)
				)
			);
		}
		queueMicrotask(() => {
			const el = document.getElementById("move-list");
			if (el) el.scrollTop = el.scrollHeight;
		});
		return list;
	}
	for (let i = 0; i < S.moves.length; i += n) {
		const li = h("li", {}, h("span", { class: "num" }, `${i / n + 1}.`));
		for (let j = 0; j < n; j++) {
			const m = S.moves[i + j];
			if (!m) {
				li.append(h("span", {}));
				continue;
			}
			li.append(
				h(
					"button",
					{
						class: "mv" + (m.seq === cur ? " current" : ""),
						"data-seq": m.seq,
						title: `Move ${m.seq} by ${seatName(m.seat) || seatLabel(m.seat)}${m.created_at ? " — " + ago(m.created_at) : ""}${m.auto_play ? " — played by a tactical bot: the Auto-Play Timeout ran out" : ""}`,
						onclick: () => viewMove(m.seq >= S.moves.length ? null : m.seq),
					},
					(m.notation || `${m.from}-${m.to}`) + (m.auto_play ? " 🤖" : "")
				)
			);
		}
		list.append(li);
	}
	queueMicrotask(() => {
		const el = document.getElementById("move-list");
		if (el) el.scrollTop = el.scrollHeight;
	});
	return list;
}


// saveBoardImage downloads the board as a standalone SVG: the live element
// with the styles that matter inlined, so the file renders anywhere.
function saveBoardImage() {
	const svg = document.getElementById("board-svg");
	if (!svg) return;
	const clone = svg.cloneNode(true);
	const props = ["fill", "stroke", "stroke-width", "stroke-dasharray", "stroke-linejoin", "opacity", "font-size", "font-weight", "font-family", "text-anchor", "dominant-baseline"];
	const src = svg.querySelectorAll("*");
	const dst = clone.querySelectorAll("*");
	src.forEach((el, i) => {
		const cs = getComputedStyle(el);
		const target = dst[i];
		for (const p of props) {
			const v = cs.getPropertyValue(p);
			if (v) target.style.setProperty(p, v);
		}
		target.removeAttribute("tabindex");
	});
	// Hover previews and hit areas are interface, not position.
	clone.querySelectorAll(".ghost-piece, .cell-hit, .target-ring, .target-dot, .quick-dot, .hint-ring, title").forEach((n) => n.remove());
	clone.setAttribute("xmlns", "http://www.w3.org/2000/svg");
	const blob = new Blob([new XMLSerializer().serializeToString(clone)], { type: "image/svg+xml" });
	const a = h("a", { href: URL.createObjectURL(blob), download: `bingle-${code}${isLive() ? "" : "-move" + S.viewSeq}.svg` });
	document.body.append(a);
	a.click();
	setTimeout(() => {
		URL.revokeObjectURL(a.href);
		a.remove();
	}, 1000);
}

// copyTPS puts the position on screen — live, historical or in analysis — on
// the clipboard in Tak's TPS notation (the server formats it).
async function copyTPS() {
	try {
		let sim;
		if (inAnalysis()) {
			sim = await api.simulate(code, S.analysis.moves, S.analysis.base === null ? undefined : S.analysis.base);
		} else if (!isLive()) {
			sim = await api.simulate(code, [], S.viewSeq);
		} else {
			sim = await api.simulate(code, []);
		}
		if (!sim.tps) {
			toast("No TPS for a finished position.", "error");
			return;
		}
		try {
			await navigator.clipboard.writeText(sim.tps);
			toast("TPS copied: " + sim.tps);
		} catch (_) {
			window.prompt("Position as TPS:", sim.tps);
		}
	} catch (err) {
		toast(err.message, "error");
	}
}


function debugPanel() {
	const d = h("details", { class: "debug" }, h("summary", {}, "Raw state (debug)"));
	d.append(
		h(
			"pre",
			{ id: "board-state" },
			JSON.stringify(
				{
					board_state: legacyView(),
					state: currentState(),
					turn_seat: S.game.turn_seat,
					status: S.game.status,
					move_seq: S.game.move_seq,
				},
				null,
				2
			)
		)
	);
	return d;
}

function legacyView() {
	const out = {};
	const cells = currentCells();
	for (const [pos, stack] of Object.entries(cells)) {
		if (stack.length) out[pos] = { Piece: stack[stack.length - 1].piece, Color: stack[stack.length - 1].color };
	}
	return out;
}

// ---------------------------------------------------------------------------
// Keyboard
// ---------------------------------------------------------------------------

document.addEventListener("keydown", (ev) => {
	if (ev.target && /^(input|textarea|select)$/i.test(ev.target.tagName)) return;
	if (ev.metaKey || ev.ctrlKey || ev.altKey) return;
	switch (ev.key) {
		case "Enter":
			if (!inAnalysis() && isMyTurn() && S.builder.complete() && !S.busy) submitMove();
			break;
		case "Escape":
			if (activeBuilder().manualSteps().length) resetMove();
			else if (S.unitInfo) closeUnitInfo();
			else if (inAnalysis()) exitAnalysis();
			break;
		case "Backspace":
			if (activeBuilder().manualSteps().length) {
				ev.preventDefault();
				undoStep();
			}
			break;
		case "ArrowLeft":
			stepHistory(-1);
			break;
		case "ArrowRight":
			stepHistory(1);
			break;
		case "Home":
			viewMove(0);
			break;
		case "End":
			viewMove(null);
			break;
		default:
	}
});

window.addEventListener("beforeunload", () => {
	wsStopped = true;
});

boot();


// ---------------------------------------------------------------------------
// Tutorials (ADR 0077): the room plays a tutorial. The learner plays their
// seat — only the move each step asks for — and the page plays the scripted
// replies for the other seats through the ordinary move endpoint.
// ---------------------------------------------------------------------------

const TUTORIAL_PAUSE = 700; // ms before a scripted reply, so it can be seen

async function loadTutorial() {
	const [family, id] = String(S.game.tutorial).split("/");
	const doc = await api.tutorial(family, id);
	S.tut = { family, ref: S.game.tutorial, doc, read: Number(store.get(tutorialReadKey(), 0)) || 0, hint: -1, pending: false };
}

function tutorialReadKey() {
	return "bingle.tutorial.read." + code;
}

/** Where the learner is, from the moves this room has played. */
function tutorialState() {
	return tutorialProgress(S.tut.doc, (S.moves || []).map((m) => m.notation), S.tut.read);
}

function tutorialStep() {
	const steps = S.tut.doc.steps || [];
	return steps[tutorialState().step] || {};
}

/** Plays the next scripted reply, if one is due; called after every redraw. */
function tutorialTick() {
	if (!S.tut || S.tut.pending || S.busy || !isLive() || S.game.status !== "active") return;
	const p = tutorialState();
	if (p.finished) {
		markFinished(store, S.tut.ref);
		return;
	}
	if (!p.next || p.next.who !== "reply") return;
	const seat = S.game.turn_seat;
	if (!seat || seat === (S.tut.doc.seat || 1)) return;
	S.tut.pending = true;
	window.setTimeout(async () => {
		try {
			await api.move(code, seatName(seat), p.next.notation);
			await settle();
		} catch (err) {
			toast(err.message || "The tutorial's reply was refused.", "error");
		} finally {
			S.tut.pending = false;
			render();
		}
	}, TUTORIAL_PAUSE);
}

/** The learner has read a step without a move of their own: on to its replies, or the next step. */
function tutorialNext() {
	const p = tutorialState();
	S.tut.read = Math.max(S.tut.read, p.step + 1);
	store.set(tutorialReadKey(), S.tut.read);
	render();
}

async function tutorialRestart() {
	const [family, id] = S.tut.ref.split("/");
	try {
		const res = await api.startTutorial({ game_def_id: family, tutorial: id, player_name: ownerName() });
		location.assign(`/game/${res.room_code}`);
	} catch (err) {
		toast(err.message, "error");
	}
}

/** Plays on from here, as an ordinary game against a bot. */
async function tutorialFinishAgainstBot() {
	const learner = S.tut.doc.seat || 1;
	try {
		const res = await api.createGame({
			game_def_id: S.game.game_def_id,
			player_name: ownerName(),
			seat: learner,
			opponent: "tactical",
			start_state: S.state,
			start_seat: S.game.turn_seat,
		});
		location.assign(`/game/${res.room_code}`);
	} catch (err) {
		toast(err.message, "error");
	}
}

/** The learner's own name, without a seat's alias. */
function ownerName() {
	return String(S.me.name || "").split(" · ")[0];
}

function tutorialCard() {
	const t = S.tut.doc;
	const p = tutorialState();
	const steps = t.steps || [];
	const step = steps[p.step] || {};
	const card = h("div", { class: "card tutorial-card", id: "tutorial" });
	card.append(
		h("div", { class: "tutorial-head" }, h("strong", {}, t.title), h("span", { class: "muted small", id: "tutorial-count" }, p.finished ? "Done" : `Step ${p.step + 1} of ${steps.length}`))
	);
	if (p.offScript) {
		card.append(
			h("p", { id: "tutorial-off" }, "This game has left the tutorial's script, so it cannot guide you any further from here."),
			h(
				"div",
				{ class: "tutorial-actions" },
				h("button", { type: "button", id: "tutorial-restart", onclick: tutorialRestart }, "Start the tutorial again"),
				S.game.status === "active" ? h("button", { type: "button", class: "secondary", id: "tutorial-bot", onclick: tutorialFinishAgainstBot }, "Play on against a bot") : null
			)
		);
		return card;
	}
	const base = `/api/game-defs/${encodeURIComponent(S.tut.family)}/assets/`;
	card.append(h("div", { class: "tutorial-text", id: "tutorial-text" }, renderMarkdown(step.text || "", { assetBase: base }).node));
	const actions = h("div", { class: "tutorial-actions" });
	if (p.finished) {
		card.append(h("p", { class: "tutorial-done", id: "tutorial-done" }, "Tutorial complete."));
		if (S.game.status === "active") actions.append(h("button", { type: "button", id: "tutorial-bot", onclick: tutorialFinishAgainstBot }, "Finish this game against a bot"));
		actions.append(h("button", { type: "button", class: "secondary", id: "tutorial-restart", onclick: tutorialRestart }, "Start again"));
		actions.append(h("a", { class: "button secondary", href: "/" }, "Back to the games"));
	} else if (p.needsNext) {
		const last = p.step === steps.length - 1 && !(step.reply || []).length;
		actions.append(h("button", { type: "button", id: "tutorial-next", onclick: tutorialNext }, last ? "Finish" : "Next"));
	} else if (p.next && p.next.who === "learner") {
		if (step.hint) {
			if (S.tut.hint === p.step) actions.append(h("p", { class: "tutorial-hint", id: "tutorial-hint" }, step.hint));
			else
				actions.append(
					h("button", { type: "button", class: "secondary", id: "tutorial-show-hint", onclick: () => ((S.tut.hint = p.step), render()) }, "Need a hint?")
				);
		}
	} else if (p.next) {
		actions.append(h("p", { class: "muted small", id: "tutorial-wait" }, "Your opponent is moving…"));
	}
	card.append(actions);
	return card;
}
