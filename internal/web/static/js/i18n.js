// The interface's own words in the reader's language (ADR 0141, docs/languages.md).
//
// The server decides the page's language (the account's choice, the browser's
// cookie, or Accept-Language) and writes it into <html lang dir>; this module
// loads that language's catalog before any module that imports it runs, so
// t() can be called anywhere, at any time. A word the catalog lacks is in
// English; a key no catalog has shows as itself.
//
// This module imports nothing, so that any other can import it.

/** The page's language (a BCP 47 tag) and its direction, "ltr" or "rtl". */
export const lang = document.documentElement.lang || "en";
export const dir = document.documentElement.dir === "rtl" ? "rtl" : "ltr";

/** The arrow keys that move forward and back along a row (of tabs, say): mirrored right to left. */
export const forwardKey = dir === "rtl" ? "ArrowLeft" : "ArrowRight";
export const backKey = dir === "rtl" ? "ArrowRight" : "ArrowLeft";

const meta = document.querySelector('meta[name="bingle-language"]');
/** The language chosen, "" for the browser's own; and the browser's own. */
export const chosen = meta ? meta.content : "";
export const browserLanguage = (meta && meta.dataset.browser) || "en";

let messages = {};
/** The interface's languages, [{code, name, dir}], in the order to list them. */
export let languages = [];
try {
	const resp = await fetch(`/static/i18n/${encodeURIComponent(lang)}.json`);
	if (resp.ok) ({ messages, languages } = await resp.json());
} catch (_) {
	// Offline: every word shows as its key, and the page still works.
}

const plurals = new Intl.PluralRules(lang);
const numbers = new Intl.NumberFormat(lang);

function words(key, params) {
	let m = messages[key];
	if (m && typeof m === "object") m = m[plurals.select(Number(params.n) || 0)] ?? m.other;
	return typeof m === "string" ? m : key;
}

const show = (v) => (typeof v === "number" ? numbers.format(v) : String(v));

/**
 * The words of key, with params put in for {name}: t("profile.write_to", {name: "Ana"}).
 * A key with plural forms takes the form for params.n.
 */
export function t(key, params = {}) {
	return words(key, params).replace(/\{(\w+)\}/g, (all, name) => (name in params ? show(params[name]) : all));
}

/**
 * The words of key as a list of strings and the values put in, for h()'s
 * children: a value may be an element, which keeps its place in the sentence
 * whatever the language's word order.
 */
export function tParts(key, params = {}) {
	const out = [];
	for (const [i, part] of words(key, params).split(/\{(\w+)\}/).entries()) {
		if (i % 2 === 0) {
			if (part) out.push(part);
		} else {
			const v = part in params ? params[part] : `{${part}}`;
			out.push(v instanceof Node ? v : show(v));
		}
	}
	return out;
}
