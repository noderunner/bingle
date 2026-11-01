// Where a learner is in a tutorial (ADR 0077), worked out from the moves the
// room has played, so a reload, a second tab or a phone all agree.
//
// A step's moves are its `play` (the learner's) then its `reply` (played by
// the page for the other seats). A step without `play` waits for the learner
// to read it and press Next before its replies are played; `read` is how many
// steps they have pressed Next on. Moves that do not follow the script (an
// undo, a move made elsewhere) leave the tutorial "off script".

const same = (a, b) => String(a).trim().toLowerCase() === String(b).trim().toLowerCase();

/** The moves of step i, in order: [{who: "learner"|"reply", notation}]. */
export function stepMoves(step) {
	return [...(step.play || []).map((n) => ({ who: "learner", notation: n })), ...(step.reply || []).map((n) => ({ who: "reply", notation: n }))];
}

/**
 * progress(tutorial, played, read) says where the learner is:
 *   {step, next, needsNext, finished, offScript}
 * step:      the index of the step to show
 * next:      the move expected next ({who, notation}), or null
 * needsNext: the step waits for the learner to press Next
 * finished:  every step is done
 * offScript: the moves played are not the script's
 */
export function progress(tut, played, read) {
	const steps = tut.steps || [];
	const moves = played || [];
	let k = 0;
	for (let i = 0; i < steps.length; i++) {
		const seq = stepMoves(steps[i]);
		const reading = !(steps[i].play || []).length && i >= read;
		for (let j = 0; j < seq.length; j++) {
			if (k + j >= moves.length) {
				if (reading) return { step: i, next: null, needsNext: true, finished: false, offScript: false };
				return { step: i, next: seq[j], needsNext: false, finished: false, offScript: false };
			}
			if (!same(moves[k + j], seq[j].notation)) return { step: i, next: null, needsNext: false, finished: false, offScript: true };
		}
		k += seq.length;
		if (reading) return { step: i, next: null, needsNext: true, finished: false, offScript: false };
	}
	if (k < moves.length) return { step: Math.max(steps.length - 1, 0), next: null, needsNext: false, finished: false, offScript: true };
	return { step: Math.max(steps.length - 1, 0), next: null, needsNext: false, finished: true, offScript: false };
}

/** Whether a move's notation is the one expected. */
export function isExpected(next, notation) {
	return !!next && next.who === "learner" && same(next.notation, notation);
}

const DONE_KEY = "bingle.tutorials.done";

/** The tutorials this browser has finished, as "family/id" (kept only to mark them in the list). */
export function finishedTutorials(store) {
	const list = store.get(DONE_KEY, []);
	return new Set(Array.isArray(list) ? list : []);
}

/** Marks a tutorial finished in this browser. */
export function markFinished(store, ref) {
	const done = finishedTutorials(store);
	if (done.has(ref)) return;
	done.add(ref);
	store.set(DONE_KEY, [...done]);
}
