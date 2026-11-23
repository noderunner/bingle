// The Language setting (ADR 0141), in the profile window and on the profile
// page: the browser's own language first, then each of the interface's in its
// own name. Choosing one saves it (for an account, on the server) and loads
// the page again in it.

import { h } from "./util.js";
import { api } from "./api.js";
import { t, chosen, browserLanguage, languages } from "./i18n.js";

/** The setting: a labelled select whose id is id. */
export function languageField(id, { label = true } = {}) {
	const nameOf = (code) => (languages.find((l) => l.code === code) || { name: code }).name;
	const select = h(
		"select",
		{ id, "aria-label": label ? null : t("language.label") },
		h("option", { value: "" }, t("language.browser", { language: nameOf(browserLanguage) })),
		...languages.map((l) => h("option", { value: l.code, lang: l.code, dir: l.dir }, l.name)),
		// A language set by hand that the list does not have: the pseudo-locale.
		chosen && !languages.some((l) => l.code === chosen) ? h("option", { value: chosen }, chosen) : null
	);
	select.value = chosen;
	const status = h("span", { class: "small", role: "status" });
	select.addEventListener("change", async () => {
		select.disabled = true;
		status.textContent = "";
		try {
			await api.setLanguage(select.value);
			location.reload();
		} catch (err) {
			select.value = chosen;
			select.disabled = false;
			status.textContent = err.message || t("common.not_saved");
		}
	});
	return h("div", { class: "field" }, label ? h("label", { for: id }, t("language.label")) : null, select, status);
}
