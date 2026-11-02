// The page of one tournament: /tournament/{id}.

import { installTopbar } from "./topbar.js";
import { installNav } from "./nav.js";
import { mountTournamentPage } from "./tournaments.js";

installNav();
installTopbar();
mountTournamentPage(document.getElementById("tournament-page"), window.location.pathname.split("/").pop());
