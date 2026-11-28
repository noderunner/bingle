// The floating card of slides on the main page, which the welcome tour (ADR
// 0139, js/welcome-tour.js) and announcements (ADR 0155, js/announcements.js)
// both show: one card at a time, with the same controls and keys. A slide has
// a title, its words (text, or a node such as rendered Markdown) and, when it
// has draw, a picture drawn by the page's own code.
//
// The pictures are only pictures: inert, out of the tab order and read to a
// screen reader as one description each.

import { h, clear } from "./util.js";
import { t, forwardKey, backKey } from "./i18n.js";

let current = null;

/** Whether a card is open now. */
export const cardOpen = () => !!current;

/**
 * Opens a card of slides, unless one is already open (then it returns null):
 *   id       the dialog's id
 *   slides   [{id, title, text | body, alt, draw}], body a node to show as the words
 *   skip     the words of the button that closes it before the last slide, shown
 *            when there are several
 *   extra(slide)  nodes to show under a slide's words
 *   onClose  called once, however it is closed (that button, Done or Esc)
 */
export function openCard({ id, slides, skip, extra, onClose }) {
	if (current || !slides.length) return null;
	const dlg = buildCard(id, slides, skip, extra);
	current = dlg;
	dlg.addEventListener("close", () => {
		dlg.remove();
		current = null;
		if (onClose) onClose();
	});
	document.body.append(dlg);
	if (typeof dlg.showModal === "function") dlg.showModal();
	else dlg.setAttribute("open", "");
	dlg.querySelector("#tour-next").focus();
	return dlg;
}

function buildCard(id, slides, skipWords, extraOf) {
	const several = slides.length > 1;
	const count = h("p", { class: "tour-count small muted", id: "tour-count", "aria-live": "polite", hidden: !several });
	const picture = h("div", { class: "tour-picture", id: "tour-picture", role: "img" });
	const title = h("h2", { id: "tour-title" });
	const text = h("div", { class: "tour-text", id: "tour-text" });
	const extra = h("div", { class: "tour-extra", id: "tour-extra" });
	const dots = h("div", { class: "tour-dots", role: "group", "aria-label": t("tour.slides"), hidden: !several });
	const back = h("button", { type: "button", class: "ghost", id: "tour-back", hidden: !several }, t("tour.back"));
	const next = h("button", { type: "button", class: "primary", id: "tour-next" }, t("tour.next"));
	const skip = several && skipWords ? h("button", { type: "button", class: "ghost", id: "tour-skip" }, skipWords) : null;
	const dlg = h(
		"dialog",
		{ id, class: "tour-card", "aria-labelledby": "tour-title", "aria-describedby": "tour-text" },
		h("div", { class: "tour-body" }, count, picture, title, text, extra),
		dots,
		h("div", { class: "tour-foot" }, skip, h("span", { class: "spacer" }), back, next)
	);

	let at = 0;
	let drawing = 0;
	const dotButtons = slides.map((sl, i) => {
		const b = h("button", { type: "button", class: "tour-dot", "data-slide": sl.id, "aria-label": t("tour.dot", { n: i + 1, title: sl.title }), onclick: () => show(i) });
		dots.append(b);
		return b;
	});

	function show(i) {
		at = Math.max(0, Math.min(slides.length - 1, i));
		const sl = slides[at];
		dlg.dataset.slide = sl.id;
		count.textContent = t("tour.count", { n: at + 1, total: slides.length });
		title.textContent = sl.title;
		if (sl.body) clear(text).append(sl.body);
		else text.textContent = sl.text;
		text.scrollTop = 0;
		clear(extra);
		if (extraOf) extra.append(...extraOf(sl));
		dotButtons.forEach((b, j) => {
			b.classList.toggle("on", j === at);
			b.setAttribute("aria-current", j === at ? "step" : "false");
		});
		back.disabled = at === 0;
		next.textContent = at === slides.length - 1 ? t("tour.done") : t("tour.next");
		picture.hidden = !sl.draw;
		if (sl.draw) {
			picture.setAttribute("aria-label", sl.alt);
			drawPicture(picture, sl, ++drawing, () => drawing);
		} else {
			++drawing;
			clear(picture);
		}
	}

	next.addEventListener("click", () => (at === slides.length - 1 ? dlg.close() : show(at + 1)));
	back.addEventListener("click", () => show(at - 1));
	if (skip) skip.addEventListener("click", () => dlg.close());
	dlg.addEventListener("keydown", (ev) => {
		if (ev.target.closest && ev.target.closest("input, textarea, select")) return;
		if (ev.key === forwardKey) show(at + 1);
		else if (ev.key === backKey) show(at - 1);
		else return;
		ev.preventDefault();
	});
	show(0);
	return dlg;
}

// drawPicture draws a slide's picture, unless another slide has been shown
// while it was being drawn. A picture that cannot be drawn leaves the frame
// empty: the words still say what the slide is about.
async function drawPicture(frame, slide, n, latest) {
	clear(frame);
	const stage = h("div", { class: "tour-stage", inert: true, "aria-hidden": "true" });
	try {
		await slide.draw(stage, slide);
	} catch (_) {
		clear(stage);
	}
	if (n !== latest()) return;
	quiet(stage);
	clear(frame).append(stage);
}

// quiet makes a picture only a picture: no ids or names that the page's own
// elements have, nothing to focus, nothing to submit.
function quiet(root) {
	for (const el of root.querySelectorAll("*")) {
		if (el instanceof SVGElement) continue; // an SVG's ids are its own gradients and clips
		el.removeAttribute("id");
		el.removeAttribute("for");
		el.removeAttribute("name");
		if (el.tabIndex >= 0) el.setAttribute("tabindex", "-1");
	}
}
