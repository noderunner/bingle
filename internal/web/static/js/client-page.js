// The quick setup page (ADR 0151): the section for this computer's platform comes first, and
// each section's profile configs button makes the setup file for whoever is signed in.

import { whoAmI } from "./api.js";
import { downloadSetup, setupLabel } from "./client-setup.js";
import { ensureName, nameIsMadeUp, setLastName } from "./util.js";

const page = document.getElementById("quick-setup");
if (page) {
	const platform = () => {
		const hint = navigator.userAgentData?.platform || navigator.userAgent || "";
		if (/android/i.test(navigator.userAgent || "")) return "";
		if (/win/i.test(hint)) return "windows";
		if (/linux|x11|cros/i.test(hint)) return "linux";
		return "";
	};
	const mine = page.querySelector(`.quick-platform[data-platform="${platform()}"]`);
	if (mine) {
		mine.classList.add("current");
		page.querySelector("h1").after(mine);
	}
	const buttons = [...page.querySelectorAll(".quick-setup-file")];
	// The file is the signed-in player's or the guest's: as the top bar does, name a browser that has no name, and an account's made-up one for the account.
	const label = () => buttons.forEach((b) => (b.textContent = setupLabel()));
	ensureName();
	label();
	whoAmI()
		.then((me) => {
			if (me.kind === "account" && nameIsMadeUp()) setLastName(me.display_name || me.handle);
			label();
		})
		.catch(() => {});
	for (const button of buttons) button.addEventListener("click", downloadSetup);
}
