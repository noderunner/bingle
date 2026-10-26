// The page behind the links in a notification mail: a button that POSTs the
// token of the link. (Following a link alone changes nothing.)

const card = document.getElementById("notify-card");
const go = document.getElementById("notify-go");
const result = document.getElementById("notify-result");
const token = new URLSearchParams(location.search).get("t") || "";

if (!token) {
	go.disabled = true;
	result.textContent = "This link is incomplete.";
}

go.addEventListener("click", async () => {
	go.disabled = true;
	try {
		const resp = await fetch(`${location.pathname}?t=${encodeURIComponent(token)}`, { method: "POST" });
		const body = await resp.json().catch(() => ({}));
		if (!resp.ok) throw new Error(body.error || "That did not work.");
		result.className = "small";
		result.textContent = card.dataset.action === "confirm" ? "Confirmed. You will get an email when it is your turn." : "Done. Bingle will not email this address again.";
		go.hidden = true;
	} catch (err) {
		result.className = "small error";
		result.textContent = err.message;
		go.disabled = false;
	}
});
