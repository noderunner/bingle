package app_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/noderunner/bingle/internal/config"
	"github.com/noderunner/bingle/internal/store"
)

// botLadderServer is a server whose bots have played for their ladder: Hex
// and Yavalath, and a prototype whose ratings are shown nowhere.
func botLadderServer(t *testing.T) string {
	t.Helper()
	return newServerWith(t, func(cfg *config.Config) {
		st, err := store.Open(cfg.DBPath)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = st.Close() }()
		now := time.Now()
		row := func(bot, family, name string, r float64) store.BotRating {
			return store.BotRating{Bot: bot, Family: family, FamilyName: name, Rating: r, Deviation: 80, Volatility: 0.06, Games: 40, Won: 20, Drawn: 4, Lost: 16, UpdatedAt: now}
		}
		if err := st.SetBotRatings(context.Background(), []store.BotRating{
			row("tactical", "hex-11", "Hex", 1310), row("minimax", "hex-11", "Hex", 1520), row("mcts", "hex-11", "Hex", 1670),
			row("tactical", "yavalath", "Yavalath", 1450), row("minimax", "yavalath", "Yavalath", 1610), row("mcts", "yavalath", "Yavalath", 1440),
			row("mcts", "mycelium", "Mycelium", 1500),
		}); err != nil {
			t.Fatal(err)
		}
	})
}

// GET /api/bot-ratings is the bot ladder: each game's bots, the highest first,
// with no prototype and nothing of people's ratings.
func TestBotLadder_API(t *testing.T) {
	base := botLadderServer(t)
	resp, raw := getJSON(t, base, "/api/bot-ratings")
	if resp.StatusCode != 200 {
		t.Fatalf("%d %s", resp.StatusCode, raw)
	}
	var got struct {
		Games []struct {
			Family string `json:"family"`
			Name   string `json:"name"`
			Bots   []struct {
				Rank   int    `json:"rank"`
				Bot    string `json:"bot"`
				Label  string `json:"label"`
				Rating int    `json:"rating"`
				Games  int    `json:"games"`
			} `json:"bots"`
		} `json:"games"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Games) != 2 || got.Games[0].Family != "hex-11" || got.Games[1].Family != "yavalath" {
		t.Fatalf("the games: %s", raw)
	}
	hex := got.Games[0].Bots
	if len(hex) != 3 || hex[0].Bot != "mcts" || hex[0].Label != "MCTS" || hex[0].Rank != 1 || hex[0].Rating != 1670 || hex[2].Bot != "tactical" || hex[2].Games != 40 {
		t.Errorf("Hex's ladder: %+v", hex)
	}
	// The people's boards have nobody on them: no bot is among people.
	resp, raw = getJSON(t, base, "/api/ratings")
	var people struct {
		Games []any `json:"games"`
	}
	if err := json.Unmarshal(raw, &people); err != nil || resp.StatusCode != 200 || len(people.Games) != 0 {
		t.Errorf("people's ratings: %d %s", resp.StatusCode, raw)
	}
}

// The Ratings tab and the guide to the bots show the bot ladder, a game at a
// time.
func TestBotLadder_UI(t *testing.T) {
	base := botLadderServer(t)
	ctx, cancel := newChromedpContext(t, 60*time.Second)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.Navigate(base+"/#ratings")); err != nil {
		t.Fatal(err)
	}
	waitJS(t, ctx, "the bot ladder", `document.querySelector('#bot-ladder tr[data-bot="mcts"]') !== null`, 20*time.Second)
	if got := evalString(t, ctx, `[...document.querySelectorAll('#bot-ladder tbody tr')].map((r) => r.dataset.bot).join(",")`); got != "mcts,minimax,tactical" {
		t.Errorf("Hex's bots in order: %s", got)
	}
	if !evalBool(t, ctx, `document.getElementById("leaderboard-empty") !== null`) {
		t.Error("the people's board is not empty")
	}
	evalString(t, ctx, `(function () { var s = document.querySelector("#bot-ladder select"); s.value = "yavalath"; s.dispatchEvent(new Event("change")); return "ok"; })()`)
	if got := evalString(t, ctx, `document.querySelector('#bot-ladder tbody tr').dataset.bot + " " + document.querySelector('#bot-ladder tbody tr').children[2].textContent`); got != "minimax 1610" {
		t.Errorf("Yavalath's first: %s", got)
	}
	if evalBool(t, ctx, `[...document.querySelectorAll('#bot-ladder option')].some((o) => o.value === "mycelium")`) {
		t.Error("a prototype is on the bot ladder")
	}

	evalString(t, ctx, `document.getElementById("tab-analysis").click(); "ok"`)
	waitJS(t, ctx, "the guide's button", `(function () { var b = document.getElementById("open-bot-guide"); return b !== null && !b.hidden; })()`, 20*time.Second)
	evalString(t, ctx, `document.getElementById("open-bot-guide").click(); "ok"`)
	waitJS(t, ctx, "the ladder in the guide", `document.querySelector('#bot-guide-dialog #bot-ladder-guide tr[data-bot="mcts"]') !== null`, 20*time.Second)
	if !evalBool(t, ctx, `document.getElementById("bot-ladder-guide").previousElementSibling.closest("#analysis-guide") !== null && document.querySelector("#bot-ladder-guide h3") === null`) {
		t.Error("the guide's ladder is out of place, or repeats its section's heading")
	}
}
