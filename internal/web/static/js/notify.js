import { brandName } from "./util.js";
import { t } from "./i18n.js";
// The page behind the links in a notification mail: a button that POSTs the
// token of the link. (Following a link alone changes nothing.)

const card = document.getElementById("notify-card");
const go = document.getElementById("notify-go");
const result = document.getElementById("notify-result");
const token = new URLSearchParams(location.search).get("t") || "";

if (!token) {
	go.disabled = true;
	result.textContent = t("account.incomplete");
}

go.addEventListener("click", async () => {
	go.disabled = true;
	try {
		const resp = await fetch(`${location.pathname}?t=${encodeURIComponent(token)}`, { method: "POST" });
		const body = await resp.json().catch(() => ({}));
		if (!resp.ok) throw new Error(body.error || t("common.not_done"));
		result.className = "small";
		result.textContent = card.dataset.action === "confirm" ? t("mailpage.confirmed") : t("mailpage.stopped", { brand: brandName() });
		go.hidden = true;
	} catch (err) {
		result.className = "small error";
		result.textContent = err.message;
		go.disabled = false;
	}
});
